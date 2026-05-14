package board

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
// parses but has no project.url returns ErrNoProjectURL. Validate no
// longer rejects empty project.url at Load time (auto-create in `gh
// optivem init` Path A is now the canonical way to populate it), so the
// sentinel case covers both "no config file" and "config without URL".
func TestResolveProjectURL_ConfigPresentButURLEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gh-optivem.yaml"),
		[]byte("project:\n  name: Shop Project\n"), 0o644); err != nil {
		t.Fatalf("write gh-optivem.yaml: %v", err)
	}

	_, err := ResolveProjectURL(dir)
	if !errors.Is(err, ErrNoProjectURL) {
		t.Errorf("expected ErrNoProjectURL for config without project.url, got: %v", err)
	}
}

func TestResolveProjectURLFromConfig_NilConfig(t *testing.T) {
	_, err := ResolveProjectURLFromConfig(nil, "")
	if !errors.Is(err, ErrNoProjectURL) {
		t.Errorf("expected ErrNoProjectURL, got %v", err)
	}
}

// TestResolveProjectURLFromConfig_SourcePathInMessage — when --config <path>
// was passed but project.url is missing, the wrapped error must name that
// exact path so the operator isn't misled into editing the wrong file
// (the original sentinel hard-coded "gh-optivem.yaml").
func TestResolveProjectURLFromConfig_SourcePathInMessage(t *testing.T) {
	const source = "/some/path/gh-optivem-monolith-typescript.yaml"
	_, err := ResolveProjectURLFromConfig(nil, source)
	if !errors.Is(err, ErrNoProjectURL) {
		t.Fatalf("expected ErrNoProjectURL, got %v", err)
	}
	if !strings.Contains(err.Error(), source) {
		t.Errorf("error message %q should mention source path %q", err.Error(), source)
	}
}

// ---------------------------------------------------------------------------
// PickTopReady
// ---------------------------------------------------------------------------

// projectMetaGraphQLJSON is the response shape fetchProjectMetadata
// parses — the minimal GraphQL query response with id + title under
// data.organization.projectV2 (or data.user.projectV2 for user-owned
// projects).
const projectMetaGraphQLJSON = `{"data":{"organization":{"projectV2":{"id":"PVT_xyz","title":"Shop Project"}}}}`

// projectMetaArgs returns the exact argv that fetchProjectMetadata
// passes to gh.Run. Tests use this to pin canned responses by argv —
// keeping the test fixtures in lockstep with the production query
// string declared in board.go (projectMetaQuery).
func projectMetaArgs(ownerKind, owner string, number int) []string {
	return []string{
		"api", "graphql",
		"-F", "login=" + owner,
		"-F", "number=" + strconv.Itoa(number),
		"-f", "query=" + fmt.Sprintf(projectMetaQuery, ownerKind),
	}
}

// projectItemsArgs returns the argv that fetchProjectItems sends on its
// first page (no cursor). Keeps the test argv pinned to the production
// projectItemsQuery constant — query changes auto-propagate to tests.
func projectItemsArgs(ownerKind, owner string, number, first int) []string {
	return []string{
		"api", "graphql",
		"-F", "login=" + owner,
		"-F", "number=" + strconv.Itoa(number),
		"-F", "first=" + strconv.Itoa(first),
		"-f", "query=" + fmt.Sprintf(projectItemsQuery, ownerKind),
	}
}

// itemListJSON mirrors the previous gh-flat-JSON fixture but in the
// GraphQL response shape that fetchProjectItems decodes. Status is
// carried under fieldValues as a ProjectV2ItemFieldSingleSelectValue
// for the Status field; content carries __typename so the picker can
// distinguish Issue / PullRequest / DraftIssue.
const itemListJSON = `{
  "data": {
    "organization": {
      "projectV2": {
        "items": {
          "pageInfo": {"hasNextPage": false, "endCursor": null},
          "nodes": [
            {"id":"item-A","fieldValues":{"nodes":[{"__typename":"ProjectV2ItemFieldSingleSelectValue","name":"In progress","field":{"name":"Status"}}]},"content":{"__typename":"Issue","number":36,"url":"https://github.com/optivem/shop/issues/36","title":"already moving","repository":{"nameWithOwner":"optivem/shop"}}},
            {"id":"item-B","fieldValues":{"nodes":[{"__typename":"ProjectV2ItemFieldSingleSelectValue","name":"Ready","field":{"name":"Status"}}]},"content":{"__typename":"Issue","number":42,"url":"https://github.com/optivem/shop/issues/42","title":"top ready","repository":{"nameWithOwner":"optivem/shop"}}},
            {"id":"item-C","fieldValues":{"nodes":[{"__typename":"ProjectV2ItemFieldSingleSelectValue","name":"Ready","field":{"name":"Status"}}]},"content":{"__typename":"Issue","number":43,"url":"https://github.com/optivem/shop/issues/43","title":"second ready","repository":{"nameWithOwner":"optivem/shop"}}},
            {"id":"item-D","fieldValues":{"nodes":[{"__typename":"ProjectV2ItemFieldSingleSelectValue","name":"Backlog","field":{"name":"Status"}}]},"content":{"__typename":"Issue","number":44,"url":"https://github.com/optivem/shop/issues/44","title":"backlog","repository":{"nameWithOwner":"optivem/shop"}}}
          ]
        }
      }
    }
  }
}`

const itemListEmptyReadyJSON = `{
  "data": {
    "organization": {
      "projectV2": {
        "items": {
          "pageInfo": {"hasNextPage": false, "endCursor": null},
          "nodes": [
            {"id":"item-A","fieldValues":{"nodes":[{"__typename":"ProjectV2ItemFieldSingleSelectValue","name":"Backlog","field":{"name":"Status"}}]},"content":{"__typename":"Issue","number":1,"url":"https://github.com/optivem/shop/issues/1","title":"backlog only","repository":{"nameWithOwner":"optivem/shop"}}},
            {"id":"item-B","fieldValues":{"nodes":[{"__typename":"ProjectV2ItemFieldSingleSelectValue","name":"In progress","field":{"name":"Status"}}]},"content":{"__typename":"Issue","number":2,"url":"https://github.com/optivem/shop/issues/2","title":"in flight","repository":{"nameWithOwner":"optivem/shop"}}}
          ]
        }
      }
    }
  }
}`

// itemListReadyDraftJSON has a Ready DraftIssue (no Number/URL/Repository
// in the GraphQL response — drafts live in the project itself) followed
// by a Ready Issue, to exercise the draft-skip path.
const itemListReadyDraftJSON = `{
  "data": {
    "organization": {
      "projectV2": {
        "items": {
          "pageInfo": {"hasNextPage": false, "endCursor": null},
          "nodes": [
            {"id":"item-D","fieldValues":{"nodes":[{"__typename":"ProjectV2ItemFieldSingleSelectValue","name":"Ready","field":{"name":"Status"}}]},"content":{"__typename":"DraftIssue","title":"draft"}},
            {"id":"item-E","fieldValues":{"nodes":[{"__typename":"ProjectV2ItemFieldSingleSelectValue","name":"Ready","field":{"name":"Status"}}]},"content":{"__typename":"Issue","number":99,"url":"https://github.com/optivem/shop/issues/99","title":"real issue","repository":{"nameWithOwner":"optivem/shop"}}}
          ]
        }
      }
    }
  }
}`

func TestPickTopReady_PicksFirstReadyIssue(t *testing.T) {
	gh := newFakeRunner(t, "gh")
	gh.on(projectMetaArgs("organization", "optivem", 20),
		[]byte(projectMetaGraphQLJSON), nil)
	gh.on(projectItemsArgs("organization", "optivem", 20, 100),
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
	gh.on(projectMetaArgs("organization", "optivem", 20),
		[]byte(projectMetaGraphQLJSON), nil)
	gh.on(projectItemsArgs("organization", "optivem", 20, 100),
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
	gh.on(projectMetaArgs("organization", "optivem", 20),
		[]byte(projectMetaGraphQLJSON), nil)
	gh.on(projectItemsArgs("organization", "optivem", 20, 100),
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
  "data": {
    "organization": {
      "projectV2": {
        "items": {
          "pageInfo": {"hasNextPage": false, "endCursor": null},
          "nodes": [
            {"id":"item-X","fieldValues":{"nodes":[{"__typename":"ProjectV2ItemFieldSingleSelectValue","name":"READY","field":{"name":"Status"}}]},"content":{"__typename":"Issue","number":7,"url":"https://github.com/optivem/shop/issues/7","title":"shouty","repository":{"nameWithOwner":"optivem/shop"}}}
          ]
        }
      }
    }
  }
}`
	gh := newFakeRunner(t, "gh")
	gh.on(projectMetaArgs("organization", "optivem", 20),
		[]byte(projectMetaGraphQLJSON), nil)
	gh.on(projectItemsArgs("organization", "optivem", 20, 100),
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

// projectFieldsArgs mirrors what LookupStatusOption sends to gh.Run, so
// the test argv stays pinned to projectFieldsQuery (the production
// constant). Mirror of projectMetaArgs / projectItemsArgs.
func projectFieldsArgs(ownerKind, owner string, number int) []string {
	return []string{
		"api", "graphql",
		"-F", "login=" + owner,
		"-F", "number=" + strconv.Itoa(number),
		"-f", "query=" + fmt.Sprintf(projectFieldsQuery, ownerKind),
	}
}

const fieldListJSON = `{
  "data": {
    "organization": {
      "projectV2": {
        "fields": {
          "nodes": [
            {"__typename":"ProjectV2Field","id":"PVTF_title","name":"Title"},
            {"__typename":"ProjectV2SingleSelectField","id":"PVTSSF_status","name":"Status","options":[
              {"id":"opt-backlog","name":"Backlog"},
              {"id":"opt-ready","name":"Ready"},
              {"id":"opt-inprogress","name":"In progress"},
              {"id":"opt-done","name":"Done"}
            ]}
          ]
        }
      }
    }
  }
}`

const fieldListNoStatusJSON = `{
  "data": {
    "organization": {
      "projectV2": {
        "fields": {
          "nodes": [
            {"__typename":"ProjectV2Field","id":"PVTF_title","name":"Title"}
          ]
        }
      }
    }
  }
}`

const fieldListNoInProgressJSON = `{
  "data": {
    "organization": {
      "projectV2": {
        "fields": {
          "nodes": [
            {"__typename":"ProjectV2SingleSelectField","id":"PVTSSF_status","name":"Status","options":[
              {"id":"opt-backlog","name":"Backlog"},
              {"id":"opt-ready","name":"Ready"},
              {"id":"opt-done","name":"Done"}
            ]}
          ]
        }
      }
    }
  }
}`

func TestMoveToInProgress_PassesExpectedArgsToGh(t *testing.T) {
	gh := newFakeRunner(t, "gh")
	gh.on(projectFieldsArgs("organization", "optivem", 20),
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
			gh.on(projectFieldsArgs("organization", "optivem", 20),
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
	gh.on(projectFieldsArgs("organization", "optivem", 20),
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
		in        string
		ownerKind string
		owner     string
		number    int
		wantErr   bool
	}{
		{"https://github.com/orgs/optivem/projects/20", "organization", "optivem", 20, false},
		{"https://github.com/users/alice/projects/3", "user", "alice", 3, false},
		{"https://github.com/optivem/shop", "", "", 0, true},
		{"", "", "", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			ownerKind, owner, number, err := ParseProjectURL(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ownerKind != tc.ownerKind || owner != tc.owner || number != tc.number {
				t.Errorf("got (%q, %q, %d), want (%q, %q, %d)",
					ownerKind, owner, number, tc.ownerKind, tc.owner, tc.number)
			}
		})
	}
}
