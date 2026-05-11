package board

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakeRunner records each invocation and returns canned responses keyed by
// the joined argv. Unmatched invocations are an explicit test failure —
// surfaces argv drift loudly.
type fakeRunner struct {
	t        *testing.T
	name     string // "gh" or "git" — for assertion messages only
	calls    [][]string
	canned   map[string]cannedResponse
	fallback func(args []string) ([]byte, error)
}

type cannedResponse struct {
	out []byte
	err error
}

func newFakeRunner(t *testing.T, name string) *fakeRunner {
	return &fakeRunner{t: t, name: name, canned: map[string]cannedResponse{}}
}

func (f *fakeRunner) on(args []string, out []byte, err error) {
	f.canned[joinArgs(args)] = cannedResponse{out: out, err: err}
}

func (f *fakeRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string(nil), args...))
	if r, ok := f.canned[joinArgs(args)]; ok {
		return r.out, r.err
	}
	if f.fallback != nil {
		return f.fallback(args)
	}
	f.t.Fatalf("%s: unexpected invocation %v (no canned response)", f.name, args)
	return nil, fmt.Errorf("unreachable")
}

func joinArgs(args []string) string {
	return strings.Join(args, "\x00")
}

// ---------------------------------------------------------------------------
// ResolveProjectURL — config-only sources
// ---------------------------------------------------------------------------
//
// Project URL is sourced exclusively from gh-optivem.yaml (default lookup)
// or from a path passed via --config (handled by the driver, which calls
// ResolveProjectURLFromConfig with a pre-loaded *Config). There is no
// README scrape, no `git remote` fallback, and no `gh project list`
// discovery — those were removed because they produced surprising
// "wrong project moved" failures when repo names overlapped.

func TestResolveProjectURL_FromOptivemYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gh-optivem.yaml"),
		[]byte("project:\n  url: https://github.com/orgs/optivem/projects/20\n"), 0o644); err != nil {
		t.Fatalf("write gh-optivem.yaml: %v", err)
	}

	got, err := ResolveProjectURL(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "https://github.com/orgs/optivem/projects/20"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveProjectURL_NoConfigFile(t *testing.T) {
	dir := t.TempDir()
	// No gh-optivem.yaml — must fail with ErrNoProjectURL.
	_, err := ResolveProjectURL(dir)
	if !errors.Is(err, ErrNoProjectURL) {
		t.Errorf("expected ErrNoProjectURL, got %v", err)
	}
}

// TestResolveProjectURL_ConfigPresentButURLEmpty — a gh-optivem.yaml that
// parses but has no project.url no longer reaches ResolveProjectURL: it
// fails projectconfig.Validate at Load time. The error surfaces from
// Load (wrapped as "board: load config: ...") rather than as the
// ErrNoProjectURL sentinel — the sentinel is now reserved for the
// no-config-file / nil-config case.
func TestResolveProjectURL_ConfigPresentButURLEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gh-optivem.yaml"),
		[]byte("project:\n  name: Shop Project\n"), 0o644); err != nil {
		t.Fatalf("write gh-optivem.yaml: %v", err)
	}

	_, err := ResolveProjectURL(dir)
	if err == nil {
		t.Fatal("expected error for config without project.url, got nil")
	}
	if errors.Is(err, ErrNoProjectURL) {
		t.Errorf("missing-url is now a Validate failure, not ErrNoProjectURL: %v", err)
	}
	if !strings.Contains(err.Error(), "project.url") {
		t.Errorf("error should mention project.url, got: %v", err)
	}
}

func TestResolveProjectURLFromConfig_NilConfig(t *testing.T) {
	_, err := ResolveProjectURLFromConfig(nil)
	if !errors.Is(err, ErrNoProjectURL) {
		t.Errorf("expected ErrNoProjectURL, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// PickTopReady
// ---------------------------------------------------------------------------

const projectViewJSON = `{"closed":false,"id":"PVT_xyz","number":20,"owner":{"login":"optivem","type":"Organization"},"title":"Shop Project","url":"https://github.com/orgs/optivem/projects/20"}`

const itemListJSON = `{
  "items": [
    {"id":"item-A","status":"In progress","title":"already moving","content":{"number":36,"repository":"optivem/shop","title":"already moving","type":"Issue","url":"https://github.com/optivem/shop/issues/36"}},
    {"id":"item-B","status":"Ready","title":"top ready","content":{"number":42,"repository":"optivem/shop","title":"top ready","type":"Issue","url":"https://github.com/optivem/shop/issues/42"}},
    {"id":"item-C","status":"Ready","title":"second ready","content":{"number":43,"repository":"optivem/shop","title":"second ready","type":"Issue","url":"https://github.com/optivem/shop/issues/43"}},
    {"id":"item-D","status":"Backlog","title":"backlog","content":{"number":44,"repository":"optivem/shop","title":"backlog","type":"Issue","url":"https://github.com/optivem/shop/issues/44"}}
  ],
  "totalCount": 4
}`

const itemListEmptyReadyJSON = `{
  "items": [
    {"id":"item-A","status":"Backlog","title":"backlog only","content":{"number":1,"repository":"optivem/shop","title":"backlog only","type":"Issue","url":"https://github.com/optivem/shop/issues/1"}},
    {"id":"item-B","status":"In progress","title":"in flight","content":{"number":2,"repository":"optivem/shop","title":"in flight","type":"Issue","url":"https://github.com/optivem/shop/issues/2"}}
  ],
  "totalCount": 2
}`

const itemListReadyDraftJSON = `{
  "items": [
    {"id":"item-D","status":"Ready","title":"draft","content":{"title":"draft","type":"DraftIssue"}},
    {"id":"item-E","status":"Ready","title":"real issue","content":{"number":99,"repository":"optivem/shop","title":"real issue","type":"Issue","url":"https://github.com/optivem/shop/issues/99"}}
  ],
  "totalCount": 2
}`

func TestPickTopReady_PicksFirstReadyIssue(t *testing.T) {
	gh := newFakeRunner(t, "gh")
	gh.on([]string{"project", "view", "20", "--owner", "optivem", "--format", "json"},
		[]byte(projectViewJSON), nil)
	gh.on([]string{"project", "item-list", "20", "--owner", "optivem", "--format", "json", "--limit", "200"},
		[]byte(itemListJSON), nil)

	pick, err := PickTopReady(context.Background(), Options{
		ProjectURL: "https://github.com/orgs/optivem/projects/20",
		GhRunner:   gh,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pick.IssueNum != 42 {
		t.Errorf("IssueNum = %d, want 42", pick.IssueNum)
	}
	if pick.ItemID != "item-B" {
		t.Errorf("ItemID = %q, want item-B", pick.ItemID)
	}
	if pick.ProjectID != "PVT_xyz" {
		t.Errorf("ProjectID = %q, want PVT_xyz", pick.ProjectID)
	}
	if pick.Repo != "optivem/shop" {
		t.Errorf("Repo = %q, want optivem/shop", pick.Repo)
	}
	if pick.IssueURL != "https://github.com/optivem/shop/issues/42" {
		t.Errorf("IssueURL = %q", pick.IssueURL)
	}
	if pick.Title != "top ready" {
		t.Errorf("Title = %q", pick.Title)
	}
}

func TestPickTopReady_EmptyReadyColumn(t *testing.T) {
	gh := newFakeRunner(t, "gh")
	gh.on([]string{"project", "view", "20", "--owner", "optivem", "--format", "json"},
		[]byte(projectViewJSON), nil)
	gh.on([]string{"project", "item-list", "20", "--owner", "optivem", "--format", "json", "--limit", "200"},
		[]byte(itemListEmptyReadyJSON), nil)

	_, err := PickTopReady(context.Background(), Options{
		ProjectURL: "https://github.com/orgs/optivem/projects/20",
		GhRunner:   gh,
	})
	if !errors.Is(err, ErrEmptyReady) {
		t.Errorf("expected ErrEmptyReady, got %v", err)
	}
}

func TestPickTopReady_SkipsDraftItems(t *testing.T) {
	gh := newFakeRunner(t, "gh")
	gh.on([]string{"project", "view", "20", "--owner", "optivem", "--format", "json"},
		[]byte(projectViewJSON), nil)
	gh.on([]string{"project", "item-list", "20", "--owner", "optivem", "--format", "json", "--limit", "200"},
		[]byte(itemListReadyDraftJSON), nil)

	pick, err := PickTopReady(context.Background(), Options{
		ProjectURL: "https://github.com/orgs/optivem/projects/20",
		GhRunner:   gh,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pick.IssueNum != 99 {
		t.Errorf("IssueNum = %d, want 99 (drafts must be skipped)", pick.IssueNum)
	}
}

func TestPickTopReady_StatusComparisonIsCaseInsensitive(t *testing.T) {
	const wonkyCasing = `{
  "items": [
    {"id":"item-X","status":"READY","title":"shouty","content":{"number":7,"repository":"optivem/shop","title":"shouty","type":"Issue","url":"https://github.com/optivem/shop/issues/7"}}
  ]
}`
	gh := newFakeRunner(t, "gh")
	gh.on([]string{"project", "view", "20", "--owner", "optivem", "--format", "json"},
		[]byte(projectViewJSON), nil)
	gh.on([]string{"project", "item-list", "20", "--owner", "optivem", "--format", "json", "--limit", "200"},
		[]byte(wonkyCasing), nil)

	pick, err := PickTopReady(context.Background(), Options{
		ProjectURL: "https://github.com/orgs/optivem/projects/20",
		GhRunner:   gh,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pick.IssueNum != 7 {
		t.Errorf("IssueNum = %d, want 7", pick.IssueNum)
	}
}

func TestPickTopReady_BadProjectURL(t *testing.T) {
	gh := newFakeRunner(t, "gh")
	_, err := PickTopReady(context.Background(), Options{
		ProjectURL: "https://example.com/not-a-project",
		GhRunner:   gh,
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if len(gh.calls) > 0 {
		t.Errorf("invalid URL must not invoke gh; got %v", gh.calls)
	}
}

// ---------------------------------------------------------------------------
// MoveToInProgress
// ---------------------------------------------------------------------------

const fieldListJSON = `{
  "fields": [
    {"id":"PVTF_title","name":"Title","type":"ProjectV2Field"},
    {"id":"PVTSSF_status","name":"Status","options":[
      {"id":"opt-backlog","name":"Backlog"},
      {"id":"opt-ready","name":"Ready"},
      {"id":"opt-inprogress","name":"In progress"},
      {"id":"opt-done","name":"Done"}
    ],"type":"ProjectV2SingleSelectField"}
  ]
}`

const fieldListNoStatusJSON = `{
  "fields": [
    {"id":"PVTF_title","name":"Title","type":"ProjectV2Field"}
  ]
}`

const fieldListNoInProgressJSON = `{
  "fields": [
    {"id":"PVTSSF_status","name":"Status","options":[
      {"id":"opt-backlog","name":"Backlog"},
      {"id":"opt-ready","name":"Ready"},
      {"id":"opt-done","name":"Done"}
    ],"type":"ProjectV2SingleSelectField"}
  ]
}`

func TestMoveToInProgress_PassesExpectedArgsToGh(t *testing.T) {
	gh := newFakeRunner(t, "gh")
	gh.on([]string{"project", "field-list", "20", "--owner", "optivem", "--format", "json"},
		[]byte(fieldListJSON), nil)
	gh.on([]string{
		"project", "item-edit",
		"--id", "item-B",
		"--field-id", "PVTSSF_status",
		"--project-id", "PVT_xyz",
		"--single-select-option-id", "opt-inprogress",
	}, []byte(`{}`), nil)

	err := MoveToInProgress(context.Background(), "PVT_xyz", "item-B", Options{
		ProjectURL: "https://github.com/orgs/optivem/projects/20",
		GhRunner:   gh,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Sanity-check that exactly the two expected calls were made.
	if len(gh.calls) != 2 {
		t.Fatalf("expected 2 gh calls (field-list, item-edit), got %d: %v", len(gh.calls), gh.calls)
	}
	wantEdit := []string{
		"project", "item-edit",
		"--id", "item-B",
		"--field-id", "PVTSSF_status",
		"--project-id", "PVT_xyz",
		"--single-select-option-id", "opt-inprogress",
	}
	if joinArgs(gh.calls[1]) != joinArgs(wantEdit) {
		t.Errorf("item-edit argv = %v\nwant %v", gh.calls[1], wantEdit)
	}
}

func TestMoveToInProgress_RequiresIDs(t *testing.T) {
	gh := newFakeRunner(t, "gh")
	cases := []struct {
		name      string
		projectID string
		itemID    string
	}{
		{"missing projectID", "", "item-B"},
		{"missing itemID", "PVT_xyz", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := MoveToInProgress(context.Background(), tc.projectID, tc.itemID, Options{
				ProjectURL: "https://github.com/orgs/optivem/projects/20",
				GhRunner:   gh,
			})
			if err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
	if len(gh.calls) > 0 {
		t.Errorf("validation errors must not call gh; got %v", gh.calls)
	}
}

func TestMoveToInProgress_StatusFieldMissing(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"no Status field", fieldListNoStatusJSON},
		{"Status field missing 'In progress' option", fieldListNoInProgressJSON},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gh := newFakeRunner(t, "gh")
			gh.on([]string{"project", "field-list", "20", "--owner", "optivem", "--format", "json"},
				[]byte(tc.body), nil)

			err := MoveToInProgress(context.Background(), "PVT_xyz", "item-B", Options{
				ProjectURL: "https://github.com/orgs/optivem/projects/20",
				GhRunner:   gh,
			})
			if !errors.Is(err, ErrStatusFieldMissing) {
				t.Errorf("expected ErrStatusFieldMissing, got %v", err)
			}
		})
	}
}

func TestMoveToInProgress_GhItemEditError(t *testing.T) {
	gh := newFakeRunner(t, "gh")
	gh.on([]string{"project", "field-list", "20", "--owner", "optivem", "--format", "json"},
		[]byte(fieldListJSON), nil)
	gh.on([]string{
		"project", "item-edit",
		"--id", "item-B",
		"--field-id", "PVTSSF_status",
		"--project-id", "PVT_xyz",
		"--single-select-option-id", "opt-inprogress",
	}, nil, fmt.Errorf("gh: HTTP 403"))

	err := MoveToInProgress(context.Background(), "PVT_xyz", "item-B", Options{
		ProjectURL: "https://github.com/orgs/optivem/projects/20",
		GhRunner:   gh,
	})
	if err == nil {
		t.Fatalf("expected wrapped error, got nil")
	}
	if !strings.Contains(err.Error(), "gh project item-edit") {
		t.Errorf("error did not wrap gh failure: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Lower-level helpers
// ---------------------------------------------------------------------------

func TestParseProjectURL(t *testing.T) {
	cases := []struct {
		in      string
		owner   string
		number  int
		wantErr bool
	}{
		{"https://github.com/orgs/optivem/projects/20", "optivem", 20, false},
		{"https://github.com/users/alice/projects/3", "alice", 3, false},
		{"https://github.com/optivem/shop", "", 0, true},
		{"", "", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			owner, number, err := parseProjectURL(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if owner != tc.owner || number != tc.number {
				t.Errorf("got (%q, %d), want (%q, %d)", owner, number, tc.owner, tc.number)
			}
		})
	}
}
