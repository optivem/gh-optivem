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
// ResolveProjectURL — README path
// ---------------------------------------------------------------------------

func TestResolveProjectURL_FromReadme(t *testing.T) {
	cases := []struct {
		name    string
		readme  string
		want    string
		wantErr bool
	}{
		{
			name:   "canonical org link",
			readme: "# MyShop\n\nProject board: https://github.com/orgs/optivem/projects/20\n",
			want:   "https://github.com/orgs/optivem/projects/20",
		},
		{
			name:   "user variant",
			readme: "Solo project: https://github.com/users/alice/projects/3 see board\n",
			want:   "https://github.com/users/alice/projects/3",
		},
		{
			name:   "embedded in markdown link",
			readme: "See [the board](https://github.com/orgs/optivem/projects/12).\n",
			want:   "https://github.com/orgs/optivem/projects/12",
		},
		{
			name:    "missing — empty README",
			readme:  "# MyShop\n\nNo project link here.\n",
			wantErr: true,
		},
		{
			name:    "malformed — wrong path",
			readme:  "Bad URL: https://github.com/optivem/projects/20\n",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(tc.readme), 0o644); err != nil {
				t.Fatalf("write README: %v", err)
			}
			git := newFakeRunner(t, "git")
			// Wire a fallback so the README-miss cases fail closed
			// (no remote configured).
			git.fallback = func(args []string) ([]byte, error) {
				return nil, fmt.Errorf("no remote configured")
			}

			got, err := ResolveProjectURL(dir, git)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
			// README path must not invoke git.
			if len(git.calls) > 0 {
				t.Errorf("README-path resolution invoked git: %v", git.calls)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ResolveProjectURL — git remote fallback
// ---------------------------------------------------------------------------

func TestResolveProjectURL_GitRemoteFallback(t *testing.T) {
	dir := t.TempDir()
	// No README — force the fallback.

	git := newFakeRunner(t, "git")
	git.on([]string{"-C", dir, "remote", "get-url", "origin"},
		[]byte("https://github.com/optivem/shop.git\n"), nil)

	// Note: this test path *would* shell out to a real `gh` for the
	// project list — we don't want that. We assert the git invocation
	// happened and accept the inevitable failure on the unmocked gh
	// step. The dedicated PickTopReady tests below cover gh interaction.
	_, err := ResolveProjectURL(dir, git)
	if err == nil {
		t.Fatalf("expected error (real gh not mocked) but got nil")
	}
	if len(git.calls) == 0 {
		t.Fatalf("expected git remote call, got none")
	}
	wantArgs := []string{"-C", dir, "remote", "get-url", "origin"}
	if joinArgs(git.calls[0]) != joinArgs(wantArgs) {
		t.Errorf("git argv = %v, want %v", git.calls[0], wantArgs)
	}
}

func TestResolveProjectURL_GitRemoteFails(t *testing.T) {
	dir := t.TempDir()
	git := newFakeRunner(t, "git")
	git.on([]string{"-C", dir, "remote", "get-url", "origin"},
		nil, fmt.Errorf("not a git repo"))

	_, err := ResolveProjectURL(dir, git)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "read git remote origin") {
		t.Errorf("error message did not wrap git failure: %v", err)
	}
}

func TestResolveProjectURL_UnparseableRemote(t *testing.T) {
	dir := t.TempDir()
	git := newFakeRunner(t, "git")
	git.on([]string{"-C", dir, "remote", "get-url", "origin"},
		[]byte("file:///some/local/path\n"), nil)

	_, err := ResolveProjectURL(dir, git)
	if !errors.Is(err, ErrNoProjectLink) {
		t.Errorf("expected ErrNoProjectLink, got %v", err)
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

func TestParseRepoFromRemote(t *testing.T) {
	cases := []struct {
		in        string
		owner     string
		repo      string
		ok        bool
	}{
		{"https://github.com/optivem/shop.git", "optivem", "shop", true},
		{"https://github.com/optivem/shop", "optivem", "shop", true},
		{"git@github.com:optivem/shop.git", "optivem", "shop", true},
		{"git@github.com:optivem/shop", "optivem", "shop", true},
		{"https://github.com/some-org/some-repo-name.git", "some-org", "some-repo-name", true},
		{"file:///local/path", "", "", false},
		{"random garbage", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			owner, repo, ok := parseRepoFromRemote(tc.in)
			if ok != tc.ok || owner != tc.owner || repo != tc.repo {
				t.Errorf("got (%q, %q, %v), want (%q, %q, %v)",
					owner, repo, ok, tc.owner, tc.repo, tc.ok)
			}
		})
	}
}

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
