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
// All GitHub state mutations go through `gh project` commands. The
// GhRunner interface lets tests substitute a fake; nil falls back to
// real `os/exec`.
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

	"github.com/optivem/gh-optivem/internal/projectconfig"
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
// RepoPath (which loads gh-optivem.yaml). When set, must be a canonical
// project URL of the form `https://github.com/orgs/<org>/projects/<n>`
// or `https://github.com/users/<user>/projects/<n>`.
//
// RepoPath: optional. Defaults to the current working directory.
//
// GhRunner: optional. nil means "shell out for real". Tests inject fakes
// to avoid network and to assert argv.
type Options struct {
	ProjectURL string
	RepoPath   string
	GhRunner   GhRunner
}

// GhRunner runs the `gh` CLI. The default implementation is execGh.
type GhRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

// ErrNoProjectURL is the sentinel returned (wrapped) when project URL
// resolution finds no `project.url` set in the loaded config (or no config
// was loaded at all). Project URL must be configured explicitly — there is
// no discovery fallback. Callers wrap this with the actual config source
// (path used, or "gh-optivem.yaml" default) so the operator sees which file
// is missing the field; use errors.Is to detect it.
var ErrNoProjectURL = errors.New("board: project.url not configured")

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

// ResolveProjectURL loads <repoPath>/gh-optivem.yaml and returns
// `project.url`. The only supported sources for project URL are this file
// (default) and a file passed via `--config <path>` at the CLI (handled
// upstream by the driver, which calls ResolveProjectURLFromConfig with
// the pre-loaded *Config). There is intentionally no README scrape, no
// `git remote` fallback, and no `gh project list` discovery — earlier
// fallbacks created surprising "the wrong project moved" failures when
// repo names overlapped or README links rotted.
func ResolveProjectURL(repoPath string) (string, error) {
	if repoPath == "" {
		return "", fmt.Errorf("board: repoPath is required")
	}
	cfg, err := projectconfig.Load(repoPath)
	if err != nil {
		return "", fmt.Errorf("board: load config: %w", err)
	}
	return ResolveProjectURLFromConfig(cfg, filepath.Join(repoPath, projectconfig.Path))
}

// VerifyProjectURL checks that a project URL parses and that a minimal
// GraphQL lookup against it succeeds. Returns nil when the project
// resolves and is visible to the authenticated `gh` CLI; otherwise an
// error describing the parse failure, the not-found result, or the
// transport failure.
//
// gh is the runner used for the lookup; nil falls back to the real `gh`
// CLI via execGh. Surfaces "you declared a board URL that doesn't exist
// or your gh auth can't see it" up-front, before any picker / move call.
//
// Implementation note: uses `gh api graphql` with a minimal query (id,
// title) rather than `gh project view`. The latter expands ~50 fields
// and every field-value-type fragment, which has triggered upstream
// resolver bugs on the projectV2 complex-query path. See [fetchProjectMetadata].
func VerifyProjectURL(ctx context.Context, projectURL string, gh GhRunner) error {
	ownerKind, owner, number, err := parseProjectURL(projectURL)
	if err != nil {
		return err
	}
	if gh == nil {
		gh = execGh{}
	}
	if _, err := fetchProjectMetadata(ctx, gh, ownerKind, owner, number); err != nil {
		return fmt.Errorf("board: project %s/#%d not accessible: %w", owner, number, err)
	}
	return nil
}

// ResolveProjectURLFromConfig is the explicit-config variant of
// ResolveProjectURL. The caller passes a pre-loaded *Config (or nil for
// "no config available") and `source` — the path the config was loaded
// from (the file passed via `--config <path>`, or the default
// `gh-optivem.yaml` location). Used by the driver when the operator passed
// `--config <path>` so the alternate config takes precedence over the
// default `gh-optivem.yaml` lookup.
//
// A nil *Config or an empty `project.url` returns an error wrapping
// ErrNoProjectURL with the actual source path — so the operator who passed
// `--config foo.yaml` sees `foo.yaml` named in the failure, not the
// misleading "gh-optivem.yaml". Empty source falls back to the default
// filename string. Project URL must be configured explicitly.
func ResolveProjectURLFromConfig(cfg *projectconfig.Config, source string) (string, error) {
	if cfg == nil || cfg.Project.URL == "" {
		if source == "" {
			source = projectconfig.Path
		}
		return "", fmt.Errorf("%w: project.url is not set in %s; set it or pass --config <path>", ErrNoProjectURL, source)
	}
	return cfg.Project.URL, nil
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
	ownerKind, owner, number, err := parseProjectURL(projectURL)
	if err != nil {
		return Pick{}, err
	}
	gh := opts.GhRunner
	if gh == nil {
		gh = execGh{}
	}

	// Project metadata — needed for the project node ID returned in Pick
	// and consumed by MoveToInProgress.
	meta, err := fetchProjectMetadata(ctx, gh, ownerKind, owner, number)
	if err != nil {
		return Pick{}, fmt.Errorf("board: project metadata: %w", err)
	}

	// Items — filter by status=Ready and pick the first.
	items, err := fetchProjectItems(ctx, gh, ownerKind, owner, number, 200)
	if err != nil {
		return Pick{}, fmt.Errorf("board: project items: %w", err)
	}

	for _, it := range items {
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
			ProjectID:    meta.ID,
			ProjectTitle: meta.Title,
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
	ownerKind, owner, number, err := parseProjectURL(projectURL)
	if err != nil {
		return Pick{}, err
	}
	gh := opts.GhRunner
	if gh == nil {
		gh = execGh{}
	}

	meta, err := fetchProjectMetadata(ctx, gh, ownerKind, owner, number)
	if err != nil {
		return Pick{}, fmt.Errorf("board: project metadata: %w", err)
	}

	items, err := fetchProjectItems(ctx, gh, ownerKind, owner, number, 200)
	if err != nil {
		return Pick{}, fmt.Errorf("board: project items: %w", err)
	}

	for _, it := range items {
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
			ProjectID:    meta.ID,
			ProjectTitle: meta.Title,
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
	ownerKind, owner, number, err := parseProjectURL(projectURL)
	if err != nil {
		return err
	}
	gh := opts.GhRunner
	if gh == nil {
		gh = execGh{}
	}

	statusFieldID, inProgressOptionID, err := lookupStatusField(ctx, gh, ownerKind, owner, number)
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

// parseProjectURL splits a canonical project URL into ownerKind
// ("organization" | "user"), owner login, and number. ownerKind is used
// by fetchProjectMetadata to issue a targeted GraphQL query (querying
// both branches with a single query produces a partial NOT_FOUND error
// for the wrong type, which gh treats as fatal).
func parseProjectURL(url string) (ownerKind, owner string, number int, err error) {
	m := projectURLPattern.FindStringSubmatch(url)
	if m == nil {
		return "", "", 0, fmt.Errorf("board: invalid project URL %q (want https://github.com/orgs/<org>/projects/<n>)", url)
	}
	n, convErr := strconv.Atoi(m[3])
	if convErr != nil {
		return "", "", 0, fmt.Errorf("board: invalid project number in %q: %w", url, convErr)
	}
	kind := "organization"
	if m[1] == "users" {
		kind = "user"
	}
	return kind, m[2], n, nil
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
	return ResolveProjectURL(repoPath)
}

// equalStatus compares status strings case-insensitively. GitHub Projects
// stores option names with friendly casing ("In progress") but tools and
// users sometimes spell them differently ("InProgress", "IN PROGRESS").
// We compare on the normalised form.
func equalStatus(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// projectMeta is the minimal project metadata consumed by PickTopReady,
// FindIssue, and VerifyProjectURL — the project node ID and the human
// title. URL and other fields are intentionally omitted; callers that
// need them already have the project URL on hand.
type projectMeta struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// projectItem is the flattened representation of a project item that
// PickTopReady / FindIssue consume. Status is resolved from the item's
// Status single-select field value (empty when the item has no Status
// set); Content.Type is the content's GraphQL __typename ("Issue",
// "PullRequest", "DraftIssue"). The shape intentionally mirrors what
// the now-removed `gh project item-list` JSON gave us, so the picker
// loops downstream did not change.
type projectItem struct {
	ID      string
	Status  string
	Content projectItemContent
}

type projectItemContent struct {
	Type       string // GraphQL __typename
	Number     int
	URL        string
	Title      string
	Repository string // owner/name — empty for DraftIssue
}

// projectMetaQuery is a minimal GraphQL query that fetches the two
// scalars callers actually need. The `%s` placeholder is filled with
// "organization" or "user" based on parseProjectURL's ownerKind.
//
// Replaces `gh project view --format json`, whose internal query
// expands ~50 field definitions and every projectV2 field-value-type
// fragment per item. That heavy query has triggered upstream resolver
// bugs (intermittent 500s with GitHub-side correlation IDs while the
// minimal query succeeds against the same project) and produces a
// response 50–200× larger than necessary for callers that only consume
// id + title. See `gh-optivem` issue tracker for the upstream incident.
const projectMetaQuery = `query($login:String!,$number:Int!){%s(login:$login){projectV2(number:$number){id title}}}`

// projectItemsQuery is the paginated minimal GraphQL query for project
// items. Replaces `gh project item-list`, whose query expands every
// ProjectV2ItemFieldValue type variant per item (Date, Iteration, Label,
// Number, SingleSelect, Text, Milestone, PullRequest, Repository, User,
// Reviewer) plus every field-type variant per value — a cartesian
// expansion that has triggered the same upstream resolver regression
// that bit `project view`. The minimal query asks only for what
// PickTopReady / FindIssue consume:
//   - per item: id + the Status single-select field value + content
//     union (Issue / PullRequest / DraftIssue) with the four scalars
//     callers read (number, url, title, repository.nameWithOwner).
//
// %s is filled with "organization" or "user" from parseProjectURL.
const projectItemsQuery = `query($login:String!,$number:Int!,$first:Int!,$after:String){%s(login:$login){projectV2(number:$number){items(first:$first,after:$after){pageInfo{hasNextPage endCursor} nodes{id fieldValues(first:20){nodes{__typename ... on ProjectV2ItemFieldSingleSelectValue{name field{... on ProjectV2SingleSelectField{name}}}}} content{__typename ... on Issue{number url title repository{nameWithOwner}} ... on PullRequest{number url title repository{nameWithOwner}} ... on DraftIssue{title}}}}}}}`

// projectItemsPageSize is the page size used by fetchProjectItems.
// GitHub's GraphQL caps `items(first:N)` at 100, so 100 is the largest
// useful page; matches what `gh project item-list` does internally.
const projectItemsPageSize = 100

// fetchProjectItems issues projectItemsQuery paginated against the
// given project and returns up to `limit` items. Pagination follows
// pageInfo.endCursor; stops early when hasNextPage is false. ownerKind
// dispatches to organization vs user (see parseProjectURL).
func fetchProjectItems(ctx context.Context, gh GhRunner, ownerKind, owner string, number, limit int) ([]projectItem, error) {
	query := fmt.Sprintf(projectItemsQuery, ownerKind)
	var items []projectItem
	var after string
	for len(items) < limit {
		first := projectItemsPageSize
		if remaining := limit - len(items); remaining < first {
			first = remaining
		}
		args := []string{
			"api", "graphql",
			"-F", "login=" + owner,
			"-F", "number=" + strconv.Itoa(number),
			"-F", "first=" + strconv.Itoa(first),
			"-f", "query=" + query,
		}
		if after != "" {
			args = append(args, "-F", "after="+after)
		}
		out, err := gh.Run(ctx, args...)
		if err != nil {
			return nil, err
		}
		// Decode page. Use map[string]... at the data level so the same
		// shape decodes whether ownerKind is "organization" or "user".
		var resp struct {
			Data map[string]struct {
				ProjectV2 *struct {
					Items struct {
						PageInfo struct {
							HasNextPage bool   `json:"hasNextPage"`
							EndCursor   string `json:"endCursor"`
						} `json:"pageInfo"`
						Nodes []struct {
							ID          string `json:"id"`
							FieldValues struct {
								Nodes []struct {
									Typename string `json:"__typename"`
									Name     string `json:"name"`
									Field    struct {
										Name string `json:"name"`
									} `json:"field"`
								} `json:"nodes"`
							} `json:"fieldValues"`
							Content struct {
								Typename   string `json:"__typename"`
								Number     int    `json:"number"`
								URL        string `json:"url"`
								Title      string `json:"title"`
								Repository struct {
									NameWithOwner string `json:"nameWithOwner"`
								} `json:"repository"`
							} `json:"content"`
						} `json:"nodes"`
					} `json:"items"`
				} `json:"projectV2"`
			} `json:"data"`
		}
		if err := json.Unmarshal(out, &resp); err != nil {
			return nil, fmt.Errorf("board: parse projectV2 items: %w", err)
		}
		owned := resp.Data[ownerKind].ProjectV2
		if owned == nil {
			return nil, fmt.Errorf("board: project %s/#%d not found", owner, number)
		}
		for _, n := range owned.Items.Nodes {
			items = append(items, projectItem{
				ID:     n.ID,
				Status: extractStatusFieldValue(n.FieldValues.Nodes),
				Content: projectItemContent{
					Type:       n.Content.Typename,
					Number:     n.Content.Number,
					URL:        n.Content.URL,
					Title:      n.Content.Title,
					Repository: n.Content.Repository.NameWithOwner,
				},
			})
			if len(items) >= limit {
				break
			}
		}
		if !owned.Items.PageInfo.HasNextPage || owned.Items.PageInfo.EndCursor == "" {
			break
		}
		after = owned.Items.PageInfo.EndCursor
	}
	return items, nil
}

// extractStatusFieldValue walks an item's field-value nodes and returns
// the option name for the Status single-select field, or "" when the
// item has no Status value set. Matches the resolved "status" string
// that `gh project item-list` synthesised; case of the field name is
// ignored to tolerate projects that spell it "status".
func extractStatusFieldValue(nodes []struct {
	Typename string `json:"__typename"`
	Name     string `json:"name"`
	Field    struct {
		Name string `json:"name"`
	} `json:"field"`
}) string {
	for _, v := range nodes {
		if v.Typename != "ProjectV2ItemFieldSingleSelectValue" {
			continue
		}
		if strings.EqualFold(v.Field.Name, "Status") {
			return v.Name
		}
	}
	return ""
}

// fetchProjectMetadata issues projectMetaQuery against the given owner
// and returns the project's node ID and title. ownerKind is
// "organization" or "user" (see parseProjectURL); it controls which
// top-level field is queried. Querying both in a single call produces a
// partial NOT_FOUND error from GitHub's GraphQL for the wrong type,
// which `gh api graphql` treats as fatal.
func fetchProjectMetadata(ctx context.Context, gh GhRunner, ownerKind, owner string, number int) (projectMeta, error) {
	query := fmt.Sprintf(projectMetaQuery, ownerKind)
	out, err := gh.Run(ctx, "api", "graphql",
		"-F", "login="+owner,
		"-F", "number="+strconv.Itoa(number),
		"-f", "query="+query)
	if err != nil {
		return projectMeta{}, err
	}
	var resp struct {
		Data map[string]struct {
			ProjectV2 *projectMeta `json:"projectV2"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return projectMeta{}, fmt.Errorf("board: parse projectV2 metadata: %w", err)
	}
	p := resp.Data[ownerKind].ProjectV2
	if p == nil {
		return projectMeta{}, fmt.Errorf("board: project %s/#%d not found", owner, number)
	}
	return *p, nil
}

// projectFieldsQuery is the minimal GraphQL query for project field
// definitions. Replaces `gh project field-list`, whose query expands
// every field-type variant (Field, IterationField, SingleSelectField,
// repeating field properties under each fragment) — same heavy-query
// class that has triggered upstream resolver bugs on the projectV2
// path. The minimal query asks only for what lookupStatusField needs:
// the SingleSelect field's id + name, plus its options' id + name. %s
// dispatches to organization vs user (parseProjectURL ownerKind).
const projectFieldsQuery = `query($login:String!,$number:Int!){%s(login:$login){projectV2(number:$number){fields(first:100){nodes{__typename ... on ProjectV2FieldCommon{id name} ... on ProjectV2SingleSelectField{options{id name}}}}}}}`

// lookupStatusField fetches the project's field list and returns the
// Status field ID and the "In progress" option ID. Errors out with
// ErrStatusFieldMissing if either is absent.
func lookupStatusField(ctx context.Context, gh GhRunner, ownerKind, owner string, number int) (fieldID, inProgressOptionID string, err error) {
	query := fmt.Sprintf(projectFieldsQuery, ownerKind)
	out, err := gh.Run(ctx, "api", "graphql",
		"-F", "login="+owner,
		"-F", "number="+strconv.Itoa(number),
		"-f", "query="+query)
	if err != nil {
		return "", "", fmt.Errorf("board: project field-list: %w", err)
	}
	type fieldOption struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	type field struct {
		Typename string        `json:"__typename"`
		ID       string        `json:"id"`
		Name     string        `json:"name"`
		Options  []fieldOption `json:"options"`
	}
	var resp struct {
		Data map[string]struct {
			ProjectV2 *struct {
				Fields struct {
					Nodes []field `json:"nodes"`
				} `json:"fields"`
			} `json:"projectV2"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", "", fmt.Errorf("board: parse project fields: %w", err)
	}
	owned := resp.Data[ownerKind].ProjectV2
	if owned == nil {
		return "", "", fmt.Errorf("board: project %s/#%d not found", owner, number)
	}
	for _, f := range owned.Fields.Nodes {
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

