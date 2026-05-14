// Package github implements tracker.Tracker against GitHub Projects v2.
// It is the post-board.go shape of the same logic — verb-based methods
// (PickReady / SetStatus / FindIssue / Verify) instead of nouns +
// command-style helpers, and the project-coordinates triple
// (projectID, itemID, projectURL) collapsed into the opaque
// Issue.Handle string the runtime threads through Context.
//
// All projectV2 calls go through `gh api graphql` with minimal queries.
// `gh project view` / `gh project item-list` / `gh project field-list`
// are intentionally avoided — those expand every projectV2 field-value
// fragment per item, a heavy query that has triggered upstream resolver
// regressions on the projectV2 path. See the projectMetaQuery /
// projectItemsQuery / projectFieldsQuery comments for the per-call
// rationale.
//
// All seven Tracker methods are implemented against the projectV2 path
// (workflow methods) and against the issue-body REST path (inspection /
// mutation). Section parsing is a minimal H2/H3 walker inlined here for
// now; chunk B (markdown adapter) extracts it into a shared
// tracker/internal/parse package consumed by both adapters.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/tracker"
)

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

// Tracker is the GitHub adapter's implementation of tracker.Tracker.
// Constructed via New from a project URL plus an optional GhRunner
// (nil falls back to shelling out to the real `gh` CLI).
type Tracker struct {
	projectURL string
	ownerKind  string // "organization" | "user"
	owner      string
	number     int
	gh         GhRunner
}

// GhRunner runs the `gh` CLI. The default implementation is execGh.
// Tests inject fakes to avoid network and to assert argv.
type GhRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

// ErrStatusFieldMissing is returned when the project has no field named
// "Status" or the field has no option matching the requested status name.
// The shop process flow assumes a standard kanban Status field; this
// surfaces misconfiguration loudly instead of silently picking the wrong
// column.
var ErrStatusFieldMissing = errors.New("github: project is missing a Status field with the requested option")

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

// New constructs a github.Tracker bound to the given project URL.
// projectURL must be a canonical GitHub Projects v2 URL of the form
// `https://github.com/orgs/<org>/projects/<n>` or
// `https://github.com/users/<user>/projects/<n>`. gh==nil falls back
// to the real `gh` CLI via execGh.
func New(projectURL string, gh GhRunner) (*Tracker, error) {
	ownerKind, owner, number, err := parseProjectURL(projectURL)
	if err != nil {
		return nil, err
	}
	if gh == nil {
		gh = execGh{}
	}
	return &Tracker{
		projectURL: projectURL,
		ownerKind:  ownerKind,
		owner:      owner,
		number:     number,
		gh:         gh,
	}, nil
}

// ---------------------------------------------------------------------------
// Tracker interface — workflow
// ---------------------------------------------------------------------------

// PickReady returns the topmost item with status "Ready" on the
// configured project. Items are returned in the order GitHub yields
// them, which matches the board's vertical order within a column.
// Returns tracker.ErrEmptyReady when the Ready column has no items.
func (t *Tracker) PickReady(ctx context.Context) (tracker.Issue, error) {
	meta, err := t.fetchProjectMetadata(ctx)
	if err != nil {
		return tracker.Issue{}, fmt.Errorf("github: project metadata: %w", err)
	}
	items, err := t.fetchProjectItems(ctx, 200)
	if err != nil {
		return tracker.Issue{}, fmt.Errorf("github: project items: %w", err)
	}
	for _, it := range items {
		if !equalStatus(it.Status, "Ready") {
			continue
		}
		if it.Content.Type != "Issue" {
			// Skip drafts and PR items — the orchestrator processes
			// only real issues.
			continue
		}
		return tracker.Issue{
			ID:     strconv.Itoa(it.Content.Number),
			Title:  it.Content.Title,
			URL:    it.Content.URL,
			Handle: encodeHandle(meta.ID, it.ID),
		}, nil
	}
	return tracker.Issue{}, tracker.ErrEmptyReady
}

// FindIssue resolves an issue by its numeric ID (e.g. "42") or by its
// canonical issue URL (e.g. "https://github.com/optivem/shop/issues/42").
// Both shapes are accepted; the adapter parses then walks the project
// item list looking for a matching content number.
//
// Returns an error wrapping the input when no project item matches the
// supplied issue number.
func (t *Tracker) FindIssue(ctx context.Context, idOrURL string) (tracker.Issue, error) {
	issueNum, err := parseIssueIDOrURL(idOrURL)
	if err != nil {
		return tracker.Issue{}, err
	}
	meta, err := t.fetchProjectMetadata(ctx)
	if err != nil {
		return tracker.Issue{}, fmt.Errorf("github: project metadata: %w", err)
	}
	items, err := t.fetchProjectItems(ctx, 200)
	if err != nil {
		return tracker.Issue{}, fmt.Errorf("github: project items: %w", err)
	}
	for _, it := range items {
		if it.Content.Type != "Issue" {
			continue
		}
		if it.Content.Number != issueNum {
			continue
		}
		return tracker.Issue{
			ID:     strconv.Itoa(it.Content.Number),
			Title:  it.Content.Title,
			URL:    it.Content.URL,
			Handle: encodeHandle(meta.ID, it.ID),
		}, nil
	}
	return tracker.Issue{}, fmt.Errorf("github: issue #%d not found on project %s/%d", issueNum, t.owner, t.number)
}

// SetStatus sets the project item's Status field to the given option
// name. handle must be the opaque string returned by PickReady or
// FindIssue (encodes "projectID:itemID"). Status name lookup is
// case-insensitive; ErrStatusFieldMissing is returned when the field
// or option is absent.
//
// Placement at the bottom of the destination lane is the GitHub
// default when a card's status changes via the API (`gh project
// item-edit` does not expose a position flag), which matches the
// orchestrator agent's "bottom of the lane" requirement automatically.
func (t *Tracker) SetStatus(ctx context.Context, handle, status string) error {
	projectID, itemID, err := decodeHandle(handle)
	if err != nil {
		return err
	}
	statusFieldID, optionID, err := t.lookupStatusOption(ctx, status)
	if err != nil {
		return err
	}
	if _, err := t.gh.Run(ctx,
		"project", "item-edit",
		"--id", itemID,
		"--field-id", statusFieldID,
		"--project-id", projectID,
		"--single-select-option-id", optionID,
	); err != nil {
		return fmt.Errorf("github: gh project item-edit: %w", err)
	}
	return nil
}

// Verify checks that the configured project URL parses and that a
// minimal GraphQL lookup against it succeeds. Returns nil when the
// project resolves and is visible to the authenticated `gh` CLI;
// otherwise an error describing the parse failure, the not-found
// result, or the transport failure.
//
// Implementation note: uses `gh api graphql` with the minimal id+title
// query rather than `gh project view`, whose internal query expands
// ~50 fields and every field-value-type fragment — heavy enough to
// have triggered upstream resolver bugs on the projectV2 path.
func (t *Tracker) Verify(ctx context.Context) error {
	if _, err := t.fetchProjectMetadata(ctx); err != nil {
		return fmt.Errorf("github: project %s/#%d not accessible: %w", t.owner, t.number, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Tracker interface — inspection / mutation
// ---------------------------------------------------------------------------

// Classify resolves the issue's native GitHub issue type
// (repository.issue.issueType.name) and returns the lowercased value.
// confident is true when the native type is set; false when the issue
// has no type (a state the operator must fix in the GitHub UI before
// the orchestrator can proceed).
//
// Uses `gh api graphql` against repository.issue.issueType.name rather
// than `gh issue view --json issueType` because that JSON field is not
// exposed by any released gh CLI as of 2026-05; the GraphQL schema has
// carried `issueType` for some time and is the only portable way to
// read it from the CLI today.
func (t *Tracker) Classify(ctx context.Context, i tracker.Issue) (string, bool, error) {
	owner, repo, num, err := parseIssueURL(i.URL)
	if err != nil {
		return "", false, err
	}
	out, err := t.gh.Run(ctx, "api", "graphql",
		"-f", "owner="+owner,
		"-f", "name="+repo,
		"-F", "number="+strconv.Itoa(num),
		"-f", "query="+issueTypeQuery,
	)
	if err != nil {
		return "", false, fmt.Errorf("github: gh api graphql issueType: %w", err)
	}
	var resp struct {
		Data struct {
			Repository struct {
				Issue struct {
					IssueType *struct {
						Name string `json:"name"`
					} `json:"issueType"`
				} `json:"issue"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", false, fmt.Errorf("github: parse issueType response: %w", err)
	}
	it := resp.Data.Repository.Issue.IssueType
	if it == nil || it.Name == "" {
		return "", false, nil
	}
	return strings.ToLower(it.Name), true, nil
}

// ReadSections fetches the issue body and returns the named H2/H3
// sections as a map. Every requested heading appears in the result —
// absent or empty sections map to "" so callers see a stable key set.
// Heading matching is case-insensitive on the trimmed heading text.
func (t *Tracker) ReadSections(ctx context.Context, i tracker.Issue, headings []string) (map[string]string, error) {
	owner, repo, num, err := parseIssueURL(i.URL)
	if err != nil {
		return nil, err
	}
	body, err := t.fetchIssueBody(ctx, owner, repo, num)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(headings))
	for _, h := range headings {
		out[h] = extractMarkdownSection(body, h)
	}
	return out, nil
}

// MarkChecklistComplete rewrites every `- [ ]` (and `* [ ]`) line in
// the issue body to `- [x]` / `* [x]` and pushes the updated body via
// gh issue edit. Idempotent: a body with no unchecked items is left
// untouched and no API call is made.
func (t *Tracker) MarkChecklistComplete(ctx context.Context, i tracker.Issue) error {
	owner, repo, num, err := parseIssueURL(i.URL)
	if err != nil {
		return err
	}
	body, err := t.fetchIssueBody(ctx, owner, repo, num)
	if err != nil {
		return err
	}
	updated := tickAllCheckboxes(body)
	if updated == body {
		return nil
	}
	if _, err := t.gh.Run(ctx, "issue", "edit", strconv.Itoa(num),
		"--repo", owner+"/"+repo,
		"--body", updated,
	); err != nil {
		return fmt.Errorf("github: gh issue edit: %w", err)
	}
	return nil
}

// fetchIssueBody runs `gh issue view <num> --json body --repo <owner>/<repo>`
// and returns the decoded body string. Argv order matches the pre-Tracker
// call sites in actions/bindings.go and gates/bindings.go so their tests'
// canned-response fakes match without churn.
func (t *Tracker) fetchIssueBody(ctx context.Context, owner, repo string, num int) (string, error) {
	out, err := t.gh.Run(ctx, "issue", "view", strconv.Itoa(num),
		"--json", "body",
		"--repo", owner+"/"+repo,
	)
	if err != nil {
		return "", fmt.Errorf("github: gh issue view: %w", err)
	}
	var resp struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("github: parse issue body: %w", err)
	}
	return resp.Body, nil
}

// issueTypeQuery fetches the native issue type — set by the Issue Form's
// `type:` field at filing time and authoritative because it cannot drift
// from a label-based heuristic. The classify package this replaced used
// projectV2 Type field + label tokens; native issueType is a simpler
// single source.
//
// Whitespace matches the verbatim string the pre-Tracker actions package
// shipped — argv-keyed test fakes hash on the exact `query=` payload, so
// preserving the formatting keeps existing tests passing across the
// migration.
const issueTypeQuery = `query($owner: String!, $name: String!, $number: Int!) { repository(owner: $owner, name: $name) { issue(number: $number) { issueType { name } } } }`

// ---------------------------------------------------------------------------
// Markdown helpers — H2/H3 section walker + checklist rewrite
// ---------------------------------------------------------------------------

// extractMarkdownSection scans body for an H2-or-deeper markdown heading
// whose text matches name (case-insensitive, exact after dropping
// leading hashes and surrounding whitespace), and returns the section
// body — every line from the next line to (but not including) the next
// heading at the same or shallower depth, with surrounding blank lines
// trimmed. Returns "" when the heading is absent or its body is empty.
func extractMarkdownSection(body, name string) string {
	lines := strings.Split(body, "\n")
	startIdx, startDepth := -1, 0
	for i, line := range lines {
		depth, text, ok := mdHeading(line)
		if !ok || depth < 2 {
			continue
		}
		if strings.EqualFold(text, name) {
			startIdx, startDepth = i+1, depth
			break
		}
	}
	if startIdx < 0 {
		return ""
	}
	endIdx := len(lines)
	for i := startIdx; i < len(lines); i++ {
		depth, _, ok := mdHeading(lines[i])
		if !ok {
			continue
		}
		if depth <= startDepth {
			endIdx = i
			break
		}
	}
	return strings.Trim(strings.Join(lines[startIdx:endIdx], "\n"), "\n")
}

// mdHeading parses one markdown heading line — leading `#`s + whitespace
// + text. Returns depth (count of leading `#`s), trimmed text, and
// ok=true on a heading line; ok=false on non-heading lines.
func mdHeading(line string) (int, string, bool) {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "#") {
		return 0, "", false
	}
	d := 0
	for d < len(t) && t[d] == '#' {
		d++
	}
	return d, strings.TrimSpace(t[d:]), true
}

// tickAllCheckboxes rewrites every `- [ ]` / `* [ ]` to its checked
// equivalent. Indentation and marker character are preserved; already-
// ticked items pass through so the operation is idempotent.
func tickAllCheckboxes(body string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		indent := line[:len(line)-len(trimmed)]
		if !strings.HasPrefix(trimmed, "- [ ]") && !strings.HasPrefix(trimmed, "* [ ]") {
			continue
		}
		lines[i] = indent + strings.Replace(trimmed, "[ ]", "[x]", 1)
	}
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------------
// Issue URL parsing
// ---------------------------------------------------------------------------

// parseIssueURL splits a canonical github issue URL into (owner, repo,
// number). Used by Classify / ReadSections / MarkChecklistComplete to
// address the issue without carrying repo on tracker.Issue. Returns a
// clear error on an empty URL so callers see "tracker.Issue.URL is
// required" rather than a downstream gh failure.
func parseIssueURL(s string) (owner, repo string, num int, err error) {
	if s == "" {
		return "", "", 0, fmt.Errorf("github: tracker.Issue.URL is required for body operations")
	}
	m := issueURLPattern.FindStringSubmatch(s)
	if m == nil {
		return "", "", 0, fmt.Errorf("github: %q is not a github issue URL", s)
	}
	n, err := strconv.Atoi(m[3])
	if err != nil {
		return "", "", 0, fmt.Errorf("github: invalid issue number in %q: %w", s, err)
	}
	return m[1], m[2], n, nil
}

// ---------------------------------------------------------------------------
// Handle encoding
// ---------------------------------------------------------------------------

// handleSeparator is the delimiter between projectID and itemID inside
// Issue.Handle. Both halves are GraphQL node IDs (PVT_… / PVTI_…),
// neither of which contains a colon, so the round-trip is safe.
const handleSeparator = ":"

// encodeHandle packs the github-specific (projectID, itemID) pair into
// the opaque string Issue.Handle the runtime threads around.
func encodeHandle(projectID, itemID string) string {
	return projectID + handleSeparator + itemID
}

// decodeHandle reverses encodeHandle. Returns a clear error when the
// string is malformed so callers see "tracker handle is not a github
// handle" rather than a downstream gh failure with empty IDs.
func decodeHandle(h string) (projectID, itemID string, err error) {
	if h == "" {
		return "", "", fmt.Errorf("github: handle is required")
	}
	parts := strings.SplitN(h, handleSeparator, 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("github: handle %q is not in projectID:itemID form", h)
	}
	return parts[0], parts[1], nil
}

// ---------------------------------------------------------------------------
// URL / ID parsing
// ---------------------------------------------------------------------------

// projectURLPattern matches the canonical org and user project URL forms.
// Capture 1: "orgs"|"users"; capture 2: owner login; capture 3: project number.
var projectURLPattern = regexp.MustCompile(`https://github\.com/(orgs|users)/([A-Za-z0-9][A-Za-z0-9-]*)/projects/(\d+)`)

// parseProjectURL splits a canonical project URL into ownerKind
// ("organization" | "user"), owner login, and number. ownerKind is
// used by the GraphQL queries to issue a targeted query (querying
// both branches with a single query produces a partial NOT_FOUND
// error for the wrong type, which gh treats as fatal).
func parseProjectURL(url string) (ownerKind, owner string, number int, err error) {
	m := projectURLPattern.FindStringSubmatch(url)
	if m == nil {
		return "", "", 0, fmt.Errorf("github: invalid project URL %q (want https://github.com/orgs/<org>/projects/<n>)", url)
	}
	n, convErr := strconv.Atoi(m[3])
	if convErr != nil {
		return "", "", 0, fmt.Errorf("github: invalid project number in %q: %w", url, convErr)
	}
	kind := "organization"
	if m[1] == "users" {
		kind = "user"
	}
	return kind, m[2], n, nil
}

// issueURLPattern matches a canonical issue URL. The first three
// captures (owner / repo / number) carry the parts FindIssue needs.
// Anchored on `/issues/` so PR URLs (which use `/pull/`) don't slip
// through and silently mismatch.
var issueURLPattern = regexp.MustCompile(`https://github\.com/([A-Za-z0-9][A-Za-z0-9-]*)/([A-Za-z0-9._-]+)/issues/(\d+)`)

// parseIssueIDOrURL accepts either a stringified positive integer
// ("42") or a full issue URL and returns the issue number. Used by
// FindIssue to support both shapes via the --issue flag.
func parseIssueIDOrURL(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("github: FindIssue requires an issue ID or URL")
	}
	if n, err := strconv.Atoi(s); err == nil {
		if n <= 0 {
			return 0, fmt.Errorf("github: issue ID %q must be positive", s)
		}
		return n, nil
	}
	m := issueURLPattern.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("github: %q is neither an issue ID nor a github issue URL", s)
	}
	n, err := strconv.Atoi(m[3])
	if err != nil {
		// Should be unreachable — the regex captures \d+.
		return 0, fmt.Errorf("github: invalid issue number in %q: %w", s, err)
	}
	return n, nil
}

// equalStatus compares status strings case-insensitively. GitHub
// Projects stores option names with friendly casing ("In progress")
// but tools and users sometimes spell them differently ("InProgress",
// "IN PROGRESS"). We compare on the normalised form.
func equalStatus(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// ---------------------------------------------------------------------------
// GraphQL queries — minimal, hand-rolled
// ---------------------------------------------------------------------------

// projectMeta is the minimal project metadata consumed by PickReady,
// FindIssue, and Verify — the project node ID and the human title.
// URL and other fields are intentionally omitted; callers that need
// them already have the project URL on hand.
type projectMeta struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// projectItem is the flattened representation of a project item that
// PickReady / FindIssue consume. Status is resolved from the item's
// Status single-select field value (empty when the item has no Status
// set); Content.Type is the content's GraphQL __typename ("Issue",
// "PullRequest", "DraftIssue").
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
// id + title.
const projectMetaQuery = `query($login:String!,$number:Int!){%s(login:$login){projectV2(number:$number){id title}}}`

// projectItemsQuery is the paginated minimal GraphQL query for project
// items. Replaces `gh project item-list`, whose query expands every
// ProjectV2ItemFieldValue type variant per item plus every field-type
// variant per value — a cartesian expansion that triggered the same
// upstream resolver regression that bit `project view`. The minimal
// query asks only for what PickReady / FindIssue consume: per item,
// id + the Status single-select field value + the content union with
// the four scalars callers read.
//
// %s is filled with "organization" or "user" from parseProjectURL.
const projectItemsQuery = `query($login:String!,$number:Int!,$first:Int!,$after:String){%s(login:$login){projectV2(number:$number){items(first:$first,after:$after){pageInfo{hasNextPage endCursor} nodes{id fieldValues(first:20){nodes{__typename ... on ProjectV2ItemFieldSingleSelectValue{name field{... on ProjectV2SingleSelectField{name}}}}} content{__typename ... on Issue{number url title repository{nameWithOwner}} ... on PullRequest{number url title repository{nameWithOwner}} ... on DraftIssue{title}}}}}}}`

// projectFieldsQuery is the minimal GraphQL query for project field
// definitions. Replaces `gh project field-list`, whose query expands
// every field-type variant — same heavy-query class that has triggered
// upstream resolver bugs on the projectV2 path. The minimal query
// asks only for what lookupStatusOption needs: the SingleSelect
// field's id + name, plus its options' id + name.
const projectFieldsQuery = `query($login:String!,$number:Int!){%s(login:$login){projectV2(number:$number){fields(first:100){nodes{__typename ... on ProjectV2FieldCommon{id name} ... on ProjectV2SingleSelectField{options{id name}}}}}}}`

// projectItemsPageSize is the page size used by fetchProjectItems.
// GitHub's GraphQL caps `items(first:N)` at 100, so 100 is the largest
// useful page; matches what `gh project item-list` does internally.
const projectItemsPageSize = 100

// ---------------------------------------------------------------------------
// GraphQL helpers
// ---------------------------------------------------------------------------

// fetchProjectMetadata issues projectMetaQuery against the configured
// project and returns its node ID and title. Surfaces a clear "not
// found" error when the GraphQL response carries a null projectV2
// for the configured owner.
func (t *Tracker) fetchProjectMetadata(ctx context.Context) (projectMeta, error) {
	query := fmt.Sprintf(projectMetaQuery, t.ownerKind)
	out, err := t.gh.Run(ctx, "api", "graphql",
		"-F", "login="+t.owner,
		"-F", "number="+strconv.Itoa(t.number),
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
		return projectMeta{}, fmt.Errorf("github: parse projectV2 metadata: %w", err)
	}
	p := resp.Data[t.ownerKind].ProjectV2
	if p == nil {
		return projectMeta{}, fmt.Errorf("github: project %s/#%d not found", t.owner, t.number)
	}
	return *p, nil
}

// fetchProjectItems issues projectItemsQuery paginated against the
// configured project and returns up to `limit` items. Pagination
// follows pageInfo.endCursor; stops early when hasNextPage is false.
func (t *Tracker) fetchProjectItems(ctx context.Context, limit int) ([]projectItem, error) {
	query := fmt.Sprintf(projectItemsQuery, t.ownerKind)
	var items []projectItem
	var after string
	for len(items) < limit {
		first := projectItemsPageSize
		if remaining := limit - len(items); remaining < first {
			first = remaining
		}
		args := []string{
			"api", "graphql",
			"-F", "login=" + t.owner,
			"-F", "number=" + strconv.Itoa(t.number),
			"-F", "first=" + strconv.Itoa(first),
			"-f", "query=" + query,
		}
		if after != "" {
			args = append(args, "-F", "after="+after)
		}
		out, err := t.gh.Run(ctx, args...)
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
			return nil, fmt.Errorf("github: parse projectV2 items: %w", err)
		}
		owned := resp.Data[t.ownerKind].ProjectV2
		if owned == nil {
			return nil, fmt.Errorf("github: project %s/#%d not found", t.owner, t.number)
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
// item has no Status value set. Field-name comparison ignores case to
// tolerate projects that spell it "status".
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

// lookupStatusOption fetches the project's field list and returns the
// Status field ID and the option ID matching optionName (case-insensitive).
// Errors out with ErrStatusFieldMissing if either the Status field or
// the requested option is absent.
func (t *Tracker) lookupStatusOption(ctx context.Context, optionName string) (fieldID, optionID string, err error) {
	query := fmt.Sprintf(projectFieldsQuery, t.ownerKind)
	out, err := t.gh.Run(ctx, "api", "graphql",
		"-F", "login="+t.owner,
		"-F", "number="+strconv.Itoa(t.number),
		"-f", "query="+query)
	if err != nil {
		return "", "", fmt.Errorf("github: project field-list: %w", err)
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
		return "", "", fmt.Errorf("github: parse project fields: %w", err)
	}
	owned := resp.Data[t.ownerKind].ProjectV2
	if owned == nil {
		return "", "", fmt.Errorf("github: project %s/#%d not found", t.owner, t.number)
	}
	for _, f := range owned.Fields.Nodes {
		if !strings.EqualFold(f.Name, "Status") {
			continue
		}
		for _, o := range f.Options {
			if equalStatus(o.Name, optionName) {
				return f.ID, o.ID, nil
			}
		}
		return "", "", fmt.Errorf("%w: Status field present but no %q option", ErrStatusFieldMissing, optionName)
	}
	return "", "", fmt.Errorf("%w: no Status field on project %s/%d", ErrStatusFieldMissing, t.owner, t.number)
}

// ---------------------------------------------------------------------------
// Default runner
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
