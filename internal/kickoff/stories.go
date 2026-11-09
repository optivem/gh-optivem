package kickoff

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/intake"
	"github.com/optivem/gh-optivem/internal/spec"
)

var (
	criterionIDRe   = regexp.MustCompile(`UC-\d+\.\d+`)
	criterionLineRe = regexp.MustCompile(`(?m)^[-*]\s+(UC-\d+\.\d+)(?:\s+\[flag:[^\]]*\])?\s*:`)
	draftNameRe     = regexp.MustCompile(`^\d{3}-[a-z0-9][a-z0-9-]*\.md$`)
)

// StoryDraft is one validated story: a title and a ticket body in the ATDD
// intake format.
type StoryDraft struct {
	File     string
	Title    string
	Body     string
	Covers   []string
	External bool
}

// LoadStoryDrafts reads and validates the story drafts in dir, in build order
// (file name order). Every draft must parse as an ATDD intake ticket with a
// Description and Acceptance Criteria; every criterion of every use case must
// be covered by at least one story; and a story may only cite criteria that
// exist.
func LoadStoryDrafts(l Layout) ([]StoryDraft, error) {
	dir := l.Abs(StoriesDraft)
	names, err := draftFileNames(dir)
	if err != nil {
		return nil, err
	}
	known, err := criterionIDs(l)
	if err != nil {
		return nil, err
	}
	var drafts []StoryDraft
	covered := map[string]bool{}
	var problems []string
	for _, name := range names {
		if !draftNameRe.MatchString(name) {
			problems = append(problems, fmt.Sprintf("%s: file name must be NNN-<slug>.md", name))
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		d, err := parseDraft(name, string(data))
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		problems = append(problems, recordCovers(name, d.Covers, known, covered)...)
		drafts = append(drafts, d)
	}
	if missing := uncoveredCriteria(known, covered); len(missing) > 0 {
		problems = append(problems, "criteria not covered by any story: "+strings.Join(missing, ", "))
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("story drafts are invalid:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return drafts, nil
}

// draftFileNames lists the non-directory entries of dir in file name order,
// failing when there are none.
func draftFileNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("no story drafts in %s", StoriesDraft)
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("no story drafts in %s", StoriesDraft)
	}
	return names, nil
}

// recordCovers marks each known criterion the draft cites as covered and
// returns a problem for each cited criterion that does not exist.
func recordCovers(name string, covers []string, known, covered map[string]bool) []string {
	var problems []string
	for _, id := range covers {
		if !known[id] {
			problems = append(problems, fmt.Sprintf("%s: cites %s, which is not a criterion of any use case", name, id))
			continue
		}
		covered[id] = true
	}
	return problems
}

// uncoveredCriteria returns the known criteria no story covers, sorted.
func uncoveredCriteria(known, covered map[string]bool) []string {
	var missing []string
	for id := range known {
		if !covered[id] {
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)
	return missing
}

func parseDraft(file, content string) (StoryDraft, error) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	first, rest, _ := strings.Cut(content, "\n")
	title := strings.TrimSpace(strings.TrimPrefix(first, "# "))
	if !strings.HasPrefix(first, "# ") || title == "" {
		return StoryDraft{}, errors.New("the first line must be `# <story title>`")
	}
	body := strings.Trim(rest, "\n") + "\n"
	res, err := intake.Parse(body)
	if err != nil {
		return StoryDraft{}, err
	}
	if !res.Description.Found {
		return StoryDraft{}, errors.New("missing Description section")
	}
	if !res.AcceptanceCriteria.Found {
		return StoryDraft{}, errors.New("missing Acceptance Criteria section")
	}
	scenarios := spec.StoryScenarios(res.AcceptanceCriteria.Body)
	if len(scenarios) == 0 {
		return StoryDraft{}, errors.New("the Acceptance Criteria must contain at least one Scenario")
	}
	for _, sc := range scenarios {
		if len(sc.Refs) == 0 {
			return StoryDraft{}, fmt.Errorf("scenario %q has no @UC-NNN.M tag line above it; every scenario must name the criterion it covers", sc.Title)
		}
	}
	covers := uniqueSorted(criterionIDRe.FindAllString(res.Description.Body, -1))
	if len(covers) == 0 {
		return StoryDraft{}, errors.New("the Description must name the UC-NNN.M criteria the story covers")
	}
	return StoryDraft{File: file, Title: title, Body: body, Covers: covers, External: res.ExternalSystemContractCriteria.Section.Found}, nil
}

// criterionIDs returns every UC-NNN.M criterion defined by the narratives.
func criterionIDs(l Layout) (map[string]bool, error) {
	files, err := narrativeFiles(l)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no use case narratives in %s", UseCasesDir)
	}
	ids := map[string]bool{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		for _, m := range criterionLineRe.FindAllStringSubmatch(string(data), -1) {
			ids[m[1]] = true
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no UC-NNN.M criteria found in %s", UseCasesDir)
	}
	return ids, nil
}

func uniqueSorted(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// Created is one tracker item kickoff created.
type Created struct {
	ID    string
	Title string
	URL   string
}

// Publisher creates the story tickets in the project's tracker and lists the
// ones kickoff already created.
type Publisher interface {
	// Publish creates one ticket per draft, in order. It is not retried: a
	// failure part-way leaves the earlier tickets in place and is reported.
	Publish(ctx context.Context, drafts []StoryDraft) ([]Created, error)
	// Existing lists the open and closed tickets kickoff created, in build order.
	Existing(ctx context.Context) ([]Created, error)
	// Open lists the not-yet-closed kickoff tickets in build order.
	Open(ctx context.Context) ([]Created, error)
}

// GHRunner runs the gh CLI.
type GHRunner interface {
	Run(ctx context.Context, stdin string, args ...string) ([]byte, error)
}

// ExecGH is the production GHRunner.
type ExecGH struct{}

// Run executes `gh <args>`; stderr is folded into the error.
func (ExecGH) Run(ctx context.Context, stdin string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

var projectURLRe = regexp.MustCompile(`^https://github\.com/(?:users|orgs)/([^/]+)/projects/(\d+)/?$`)

// GitHubPublisher creates GitHub issues of type Story in Repo and adds them to
// the project board.
type GitHubPublisher struct {
	GH         GHRunner
	Repo       string // owner/name
	ProjectURL string
}

// Publish creates the label, then one issue per draft, and adds each to the board.
func (p GitHubPublisher) Publish(ctx context.Context, drafts []StoryDraft) ([]Created, error) {
	m := projectURLRe.FindStringSubmatch(p.ProjectURL)
	if m == nil {
		return nil, fmt.Errorf("project.url %q is not a GitHub project URL (https://github.com/<users|orgs>/<owner>/projects/<n>)", p.ProjectURL)
	}
	if _, err := p.GH.Run(ctx, "", "label", "create", StoryLabel, "--repo", p.Repo, "--force", "--description", "Created by gh optivem kickoff"); err != nil {
		return nil, err
	}
	var created []Created
	for _, d := range drafts {
		payload, err := json.Marshal(map[string]any{"title": d.Title, "body": d.Body, "type": "Story", "labels": []string{StoryLabel}})
		if err != nil {
			return created, err
		}
		out, err := p.GH.Run(ctx, string(payload), "api", "--method", "POST", "repos/"+p.Repo+"/issues", "--input", "-")
		if err != nil {
			return created, fmt.Errorf("create story %q: %w", d.Title, err)
		}
		var issue struct {
			Number  int    `json:"number"`
			HTMLURL string `json:"html_url"`
		}
		if err := json.Unmarshal(out, &issue); err != nil || issue.Number == 0 {
			return created, fmt.Errorf("create story %q: unexpected response %q", d.Title, strings.TrimSpace(string(out)))
		}
		c := Created{ID: fmt.Sprint(issue.Number), Title: d.Title, URL: issue.HTMLURL}
		created = append(created, c)
		if _, err := p.GH.Run(ctx, "", "project", "item-add", m[2], "--owner", m[1], "--url", c.URL); err != nil {
			return created, fmt.Errorf("add %s to the project board: %w (if this says missing scopes, run: gh auth refresh -s project)", c.URL, err)
		}
	}
	return created, nil
}

type ghIssue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
}

func (p GitHubPublisher) list(ctx context.Context, state string) ([]Created, error) {
	out, err := p.GH.Run(ctx, "", "issue", "list", "--repo", p.Repo, "--label", StoryLabel, "--state", state, "--limit", "500", "--json", "number,title,url")
	if err != nil {
		return nil, err
	}
	var issues []ghIssue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parse gh issue list: %w", err)
	}
	sort.Slice(issues, func(i, j int) bool { return issues[i].Number < issues[j].Number })
	var res []Created
	for _, i := range issues {
		res = append(res, Created{ID: fmt.Sprint(i.Number), Title: i.Title, URL: i.URL})
	}
	return res, nil
}

// Existing lists every kickoff issue, open or closed.
func (p GitHubPublisher) Existing(ctx context.Context) ([]Created, error) { return p.list(ctx, "all") }

// Open lists the kickoff issues that are not closed yet.
func (p GitHubPublisher) Open(ctx context.Context) ([]Created, error) { return p.list(ctx, "open") }

// MarkdownPublisher writes the stories into a markdown board's ready/ column.
// Files are named story-kickoff-NNN-<slug>.md so the board classifies them as
// stories by filename.
type MarkdownPublisher struct{ BoardDir string }

const markdownPrefix = "story-kickoff-"

// Publish writes one file per draft; an existing file is never overwritten.
func (p MarkdownPublisher) Publish(_ context.Context, drafts []StoryDraft) ([]Created, error) {
	ready := filepath.Join(p.BoardDir, "ready")
	if err := os.MkdirAll(ready, 0o755); err != nil {
		return nil, err
	}
	var created []Created
	for _, d := range drafts {
		id := markdownPrefix + strings.TrimSuffix(d.File, ".md")
		path := filepath.Join(ready, id+".md")
		if _, err := os.Stat(path); err == nil {
			return created, fmt.Errorf("%s already exists", path)
		}
		if err := os.WriteFile(path, []byte("# "+d.Title+"\n\n"+d.Body), 0o644); err != nil {
			return created, err
		}
		created = append(created, Created{ID: id, Title: d.Title})
	}
	return created, nil
}

func (p MarkdownPublisher) scan(dirs []string) ([]Created, error) {
	var out []Created
	for _, d := range dirs {
		files, err := filepath.Glob(filepath.Join(p.BoardDir, d, markdownPrefix+"*.md"))
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			id := strings.TrimSuffix(filepath.Base(f), ".md")
			out = append(out, Created{ID: id, Title: id})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Existing lists every kickoff story on the board, in any column.
func (p MarkdownPublisher) Existing(context.Context) ([]Created, error) {
	return p.scan([]string{"ready", "in-progress", "in-acceptance", "done"})
}

// Open lists the kickoff stories not yet in the done column.
func (p MarkdownPublisher) Open(context.Context) ([]Created, error) {
	return p.scan([]string{"ready", "in-progress", "in-acceptance"})
}
