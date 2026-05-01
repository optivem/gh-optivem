// Package board owns the GitHub Project board interactions for the ATDD
// pipeline driver. It resolves which project belongs to a repo, picks the
// top item in the Ready column, and moves an item to In Progress at the
// bottom of the lane.
//
// This package is the deterministic, mechanical replacement for what the
// Markdown `atdd-orchestrator` agent does today via `gh` MCP calls. It is
// intentionally a pure library — it does not prompt the user for
// "board mode vs specific issue" (that is a higher-level concern) and it
// does not classify tickets (that is `atdd-dispatcher`'s job).
//
// All GitHub state mutations go through `gh` (project commands and
// optionally `git remote get-url origin`). The runners are interface-typed
// so tests can substitute fakes; nil falls back to real `os/exec`.
package board

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/config"
)

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

// Pick is the result of selecting an issue from the Ready column. All
// fields are populated when err is nil; ItemID is the project-item node ID
// (used by MoveToInProgress), and ProjectID is the project node ID (also
// required by `gh project item-edit`).
type Pick struct {
	IssueNum     int
	IssueURL     string
	Title        string
	Repo         string // owner/repo where the issue lives
	ProjectID    string // node id (gh project's --format json field)
	ProjectTitle string // human-readable project title (e.g. "Shop Project")
	ItemID       string // node id of the project item (for later move calls)
}

// Options bundles the inputs for PickTopReady and MoveToInProgress.
//
// ProjectURL: optional. When empty, ResolveProjectURL is invoked against
// RepoPath. When set, must be a canonical project URL of the form
// `https://github.com/orgs/<org>/projects/<n>` or
// `https://github.com/users/<user>/projects/<n>`.
//
// RepoPath: optional. Defaults to the current working directory.
//
// GhRunner / GitRunner: optional. nil means "shell out for real". Tests
// inject fakes to avoid network and to assert argv.
type Options struct {
	ProjectURL string
	RepoPath   string
	GhRunner   GhRunner
	GitRunner  GitRunner
}

// GhRunner runs the `gh` CLI. The default implementation is execGh.
type GhRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// GitRunner runs the `git` CLI. The default implementation is execGit.
type GitRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

// ErrNoProjectLink is returned when ResolveProjectURL cannot find a
// project link in README.md and the git-remote fallback fails to identify
// an unambiguous project. Callers (e.g. the future Cobra command) typically
// translate this into a "stop and ask the user" prompt.
var ErrNoProjectLink = errors.New("board: no GitHub Project link found in README.md or via git remote")

// ErrEmptyReady is returned when PickTopReady runs successfully but the
// Ready column has no items. This is a normal "nothing to do" outcome,
// not an error in the I/O sense — callers usually report it and stop.
var ErrEmptyReady = errors.New("board: Ready column is empty")

// ErrStatusFieldMissing is returned when the project has no field named
// "Status" or the field has no "Ready" / "In progress" option. The shop
// process flow assumes a standard kanban Status field; this surfaces
// misconfiguration loudly instead of silently picking the wrong column.
var ErrStatusFieldMissing = errors.New("board: project is missing a Status field with Ready / In progress options")

// ---------------------------------------------------------------------------
// Public functions
// ---------------------------------------------------------------------------

// ResolveProjectURL implements the config-then-README-then-git-remote
// fallback chain:
//
//  1. Read <repoPath>/docs/atdd/config.yaml; if `project.url` is set,
//     return it.
//  2. Read <repoPath>/README.md and look for the canonical pattern
//     `https://github.com/orgs/<org>/projects/<n>` or the user variant
//     `https://github.com/users/<user>/projects/<n>`.
//  3. On miss, shell out to `git -C <repoPath> remote get-url origin`,
//     extract the org, list projects for that org, and return the URL if
//     exactly one matches the repo name in title (case-insensitive
//     substring) — otherwise ErrNoProjectLink.
//
// The fallback is intentionally conservative: when there is more than one
// candidate project, the agent body says "stop and ask the user", and we
// surface that as ErrNoProjectLink so the caller can do the asking.
func ResolveProjectURL(repoPath string, git GitRunner) (string, error) {
	if repoPath == "" {
		return "", fmt.Errorf("board: repoPath is required")
	}
	cfg, err := config.Load(repoPath)
	if err != nil {
		return "", fmt.Errorf("board: load config: %w", err)
	}
	return ResolveProjectURLFromConfig(cfg, repoPath, git)
}

// ResolveProjectURLFromConfig is the explicit-config variant of
// ResolveProjectURL. The caller passes a pre-loaded *Config (or nil to
// skip the config branch entirely). Used by the driver when the operator
// passed `--config <path>` so the alternate config takes precedence over
// the default `docs/atdd/config.yaml` lookup.
//
// README + git-remote fallback semantics are otherwise identical to
// ResolveProjectURL.
func ResolveProjectURLFromConfig(cfg *config.Config, repoPath string, git GitRunner) (string, error) {
	if repoPath == "" {
		return "", fmt.Errorf("board: repoPath is required")
	}

	if cfg != nil && cfg.Project.URL != "" {
		return cfg.Project.URL, nil
	}

	if url, ok := readProjectURLFromReadme(repoPath); ok {
		return url, nil
	}

	// README miss — fall back to git remote + project listing.
	if git == nil {
		git = execGit{}
	}
	ctx := context.Background()
	out, err := git.Run(ctx, "-C", repoPath, "remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("board: read git remote origin: %w", err)
	}
	owner, repoName, ok := parseRepoFromRemote(strings.TrimSpace(string(out)))
	if !ok {
		return "", fmt.Errorf("%w: could not parse owner/repo from git remote %q", ErrNoProjectLink, strings.TrimSpace(string(out)))
	}

	gh := execGh{}
	listOut, err := gh.Run(ctx, "project", "list", "--owner", owner, "--format", "json")
	if err != nil {
		return "", fmt.Errorf("board: gh project list: %w", err)
	}

	var listResp struct {
		Projects []struct {
			Closed bool   `json:"closed"`
			Title  string `json:"title"`
			URL    string `json:"url"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(listOut, &listResp); err != nil {
		return "", fmt.Errorf("board: parse gh project list output: %w", err)
	}

	var matches []string
	needle := strings.ToLower(repoName)
	for _, p := range listResp.Projects {
		if p.Closed {
			continue
		}
		if strings.Contains(strings.ToLower(p.Title), needle) {
			matches = append(matches, p.URL)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("%w: no project for %s/%s", ErrNoProjectLink, owner, repoName)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("%w: ambiguous — %d projects match repo %q (%s)", ErrNoProjectLink, len(matches), repoName, strings.Join(matches, ", "))
	}
}

// PickTopReady reads the project and returns the topmost item with
// status "Ready". Order is the order returned by `gh project item-list`,
// which matches the board's vertical order within a column.
//
// On a Ready column with no items, returns Pick{} and ErrEmptyReady.
func PickTopReady(ctx context.Context, opts Options) (Pick, error) {
	projectURL, err := resolveProjectURL(opts)
	if err != nil {
		return Pick{}, err
	}
	owner, number, err := parseProjectURL(projectURL)
	if err != nil {
		return Pick{}, err
	}
	gh := opts.GhRunner
	if gh == nil {
		gh = execGh{}
	}

	// Project metadata — needed for the project node ID returned in Pick
	// and consumed by MoveToInProgress.
	viewOut, err := gh.Run(ctx, "project", "view", strconv.Itoa(number), "--owner", owner, "--format", "json")
	if err != nil {
		return Pick{}, fmt.Errorf("board: gh project view: %w", err)
	}
	var view struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(viewOut, &view); err != nil {
		return Pick{}, fmt.Errorf("board: parse gh project view output: %w", err)
	}

	// Items — filter by status=Ready and pick the first.
	listOut, err := gh.Run(ctx, "project", "item-list", strconv.Itoa(number), "--owner", owner, "--format", "json", "--limit", "200")
	if err != nil {
		return Pick{}, fmt.Errorf("board: gh project item-list: %w", err)
	}
	type itemContent struct {
		Number     int    `json:"number"`
		Repository string `json:"repository"`
		Title      string `json:"title"`
		Type       string `json:"type"`
		URL        string `json:"url"`
	}
	type item struct {
		ID      string      `json:"id"`
		Status  string      `json:"status"`
		Title   string      `json:"title"`
		Content itemContent `json:"content"`
	}
	var listResp struct {
		Items []item `json:"items"`
	}
	if err := json.Unmarshal(listOut, &listResp); err != nil {
		return Pick{}, fmt.Errorf("board: parse gh project item-list output: %w", err)
	}

	for _, it := range listResp.Items {
		if !equalStatus(it.Status, "Ready") {
			continue
		}
		if it.Content.Type != "Issue" {
			// Skip draft items and PR items — the orchestrator processes
			// only real issues.
			continue
		}
		return Pick{
			IssueNum:     it.Content.Number,
			IssueURL:     it.Content.URL,
			Title:        it.Content.Title,
			Repo:         it.Content.Repository,
			ProjectID:    view.ID,
			ProjectTitle: view.Title,
			ItemID:       it.ID,
		}, nil
	}
	return Pick{}, ErrEmptyReady
}

// FindIssue is the specific-issue counterpart to PickTopReady: given an
// explicit issue number, it walks the project item list and returns the
// matching item's metadata. Used by the implement-ticket mode of the driver
// when the user has chosen an issue and the picker is bypassed.
//
// Returns ErrEmptyReady's sibling — a fmt.Errorf wrapping issue not found —
// if no project item matches the supplied issue number. The caller typically
// surfaces that as "issue #N is not on the board".
func FindIssue(ctx context.Context, issueNum int, opts Options) (Pick, error) {
	if issueNum <= 0 {
		return Pick{}, fmt.Errorf("board: FindIssue requires a positive issue number, got %d", issueNum)
	}
	projectURL, err := resolveProjectURL(opts)
	if err != nil {
		return Pick{}, err
	}
	owner, number, err := parseProjectURL(projectURL)
	if err != nil {
		return Pick{}, err
	}
	gh := opts.GhRunner
	if gh == nil {
		gh = execGh{}
	}

	viewOut, err := gh.Run(ctx, "project", "view", strconv.Itoa(number), "--owner", owner, "--format", "json")
	if err != nil {
		return Pick{}, fmt.Errorf("board: gh project view: %w", err)
	}
	var view struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(viewOut, &view); err != nil {
		return Pick{}, fmt.Errorf("board: parse gh project view output: %w", err)
	}

	listOut, err := gh.Run(ctx, "project", "item-list", strconv.Itoa(number), "--owner", owner, "--format", "json", "--limit", "200")
	if err != nil {
		return Pick{}, fmt.Errorf("board: gh project item-list: %w", err)
	}
	type itemContent struct {
		Number     int    `json:"number"`
		Repository string `json:"repository"`
		Title      string `json:"title"`
		Type       string `json:"type"`
		URL        string `json:"url"`
	}
	type item struct {
		ID      string      `json:"id"`
		Status  string      `json:"status"`
		Title   string      `json:"title"`
		Content itemContent `json:"content"`
	}
	var listResp struct {
		Items []item `json:"items"`
	}
	if err := json.Unmarshal(listOut, &listResp); err != nil {
		return Pick{}, fmt.Errorf("board: parse gh project item-list output: %w", err)
	}

	for _, it := range listResp.Items {
		if it.Content.Type != "Issue" {
			continue
		}
		if it.Content.Number != issueNum {
			continue
		}
		return Pick{
			IssueNum:     it.Content.Number,
			IssueURL:     it.Content.URL,
			Title:        it.Content.Title,
			Repo:         it.Content.Repository,
			ProjectID:    view.ID,
			ProjectTitle: view.Title,
			ItemID:       it.ID,
		}, nil
	}
	return Pick{}, fmt.Errorf("board: issue #%d not found on project %s/%d", issueNum, owner, number)
}

// MoveToInProgress sets the project item's Status field to "In progress".
// Placement at the bottom of the lane is the GitHub default when a card's
// status changes via the API — `gh project item-edit` does not expose a
// position flag, so this matches the orchestrator agent's "bottom of the
// lane" requirement automatically.
//
// projectID and itemID are typically taken from a Pick returned by
// PickTopReady. Options is consulted for GhRunner injection and (only when
// the Status field IDs need looking up) for ProjectURL / RepoPath.
func MoveToInProgress(ctx context.Context, projectID, itemID string, opts Options) error {
	if projectID == "" {
		return fmt.Errorf("board: MoveToInProgress: projectID is required")
	}
	if itemID == "" {
		return fmt.Errorf("board: MoveToInProgress: itemID is required")
	}

	projectURL, err := resolveProjectURL(opts)
	if err != nil {
		return err
	}
	owner, number, err := parseProjectURL(projectURL)
	if err != nil {
		return err
	}
	gh := opts.GhRunner
	if gh == nil {
		gh = execGh{}
	}

	statusFieldID, inProgressOptionID, err := lookupStatusField(ctx, gh, owner, number)
	if err != nil {
		return err
	}

	if _, err := gh.Run(ctx,
		"project", "item-edit",
		"--id", itemID,
		"--field-id", statusFieldID,
		"--project-id", projectID,
		"--single-select-option-id", inProgressOptionID,
	); err != nil {
		return fmt.Errorf("board: gh project item-edit: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// projectURLPattern matches the canonical org and user project URL forms.
// Capture 1: "orgs"|"users"; capture 2: owner login; capture 3: project number.
var projectURLPattern = regexp.MustCompile(`https://github\.com/(orgs|users)/([A-Za-z0-9][A-Za-z0-9-]*)/projects/(\d+)`)

// readProjectURLFromReadme scans <repoPath>/README.md for the first
// occurrence of the canonical project URL pattern and returns it. Any I/O
// error (missing README, permission denied, etc.) is reported as a miss —
// the fallback chain handles the actual error reporting.
func readProjectURLFromReadme(repoPath string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(repoPath, "README.md"))
	if err != nil {
		return "", false
	}
	if m := projectURLPattern.FindString(string(data)); m != "" {
		return m, true
	}
	return "", false
}

// parseProjectURL splits a canonical project URL into owner login + number.
// Both org and user variants resolve identically — `gh project` accepts
// either as `--owner`.
func parseProjectURL(url string) (owner string, number int, err error) {
	m := projectURLPattern.FindStringSubmatch(url)
	if m == nil {
		return "", 0, fmt.Errorf("board: invalid project URL %q (want https://github.com/orgs/<org>/projects/<n>)", url)
	}
	n, convErr := strconv.Atoi(m[3])
	if convErr != nil {
		return "", 0, fmt.Errorf("board: invalid project number in %q: %w", url, convErr)
	}
	return m[2], n, nil
}

// remoteOriginPattern matches the two canonical git remote forms for
// GitHub: `https://github.com/<owner>/<repo>(.git)?` and
// `git@github.com:<owner>/<repo>(.git)?`.
var remoteOriginPattern = regexp.MustCompile(`(?:https://github\.com/|git@github\.com:)([A-Za-z0-9][A-Za-z0-9-]*)/([A-Za-z0-9._-]+?)(?:\.git)?$`)

func parseRepoFromRemote(remote string) (owner, repo string, ok bool) {
	m := remoteOriginPattern.FindStringSubmatch(remote)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// resolveProjectURL prefers an explicit ProjectURL, then falls back to
// ResolveProjectURL with RepoPath (defaulting to cwd).
func resolveProjectURL(opts Options) (string, error) {
	if opts.ProjectURL != "" {
		return opts.ProjectURL, nil
	}
	repoPath := opts.RepoPath
	if repoPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("board: get working directory: %w", err)
		}
		repoPath = cwd
	}
	return ResolveProjectURL(repoPath, opts.GitRunner)
}

// equalStatus compares status strings case-insensitively. GitHub Projects
// stores option names with friendly casing ("In progress") but tools and
// users sometimes spell them differently ("InProgress", "IN PROGRESS").
// We compare on the normalised form.
func equalStatus(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// lookupStatusField fetches the project's field list and returns the
// Status field ID and the "In progress" option ID. Errors out with
// ErrStatusFieldMissing if either is absent.
func lookupStatusField(ctx context.Context, gh GhRunner, owner string, number int) (fieldID, inProgressOptionID string, err error) {
	out, err := gh.Run(ctx, "project", "field-list", strconv.Itoa(number), "--owner", owner, "--format", "json")
	if err != nil {
		return "", "", fmt.Errorf("board: gh project field-list: %w", err)
	}
	type fieldOption struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	type field struct {
		ID      string         `json:"id"`
		Name    string         `json:"name"`
		Type    string         `json:"type"`
		Options []fieldOption  `json:"options"`
	}
	var resp struct {
		Fields []field `json:"fields"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", "", fmt.Errorf("board: parse gh project field-list output: %w", err)
	}
	for _, f := range resp.Fields {
		if !strings.EqualFold(f.Name, "Status") {
			continue
		}
		for _, o := range f.Options {
			if equalStatus(o.Name, "In progress") {
				return f.ID, o.ID, nil
			}
		}
		return "", "", fmt.Errorf("%w: Status field present but no 'In progress' option", ErrStatusFieldMissing)
	}
	return "", "", fmt.Errorf("%w: no Status field on project %s/%d", ErrStatusFieldMissing, owner, number)
}

// ---------------------------------------------------------------------------
// Default runners
// ---------------------------------------------------------------------------

type execGh struct{}

func (execGh) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

type execGit struct{}

func (execGit) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}
