package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/tracker"
)

// ---------------------------------------------------------------------------
// Compile-time interface assertion
// ---------------------------------------------------------------------------

// *Tracker must satisfy tracker.Tracker. The Classify / ReadSections
// methods are stubbed in this step but they are present on the
// receiver, so the assignment compiles today.
var _ tracker.Tracker = (*Tracker)(nil)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakeRunner records each invocation and returns canned responses keyed
// by the joined argv. Unmatched invocations are an explicit test failure
// — surfaces argv drift loudly.
type fakeRunner struct {
	t        *testing.T
	calls    [][]string
	canned   map[string]cannedResponse
	fallback func(args []string) ([]byte, error)
}

type cannedResponse struct {
	out []byte
	err error
}

func newFakeRunner(t *testing.T) *fakeRunner {
	return &fakeRunner{t: t, canned: map[string]cannedResponse{}}
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
	f.t.Fatalf("gh: unexpected invocation %v (no canned response)", args)
	return nil, fmt.Errorf("unreachable")
}

func joinArgs(args []string) string {
	return strings.Join(args, "\x00")
}

// ---------------------------------------------------------------------------
// argv builders — pinned to the production query constants
// ---------------------------------------------------------------------------

func projectMetaArgs(ownerKind, owner string, number int) []string {
	return []string{
		"api", "graphql",
		"-F", "login=" + owner,
		"-F", "number=" + strconv.Itoa(number),
		"-f", "query=" + fmt.Sprintf(projectMetaQuery, ownerKind),
	}
}

func projectItemsArgs(ownerKind, owner string, number, first int) []string {
	return []string{
		"api", "graphql",
		"-F", "login=" + owner,
		"-F", "number=" + strconv.Itoa(number),
		"-F", "first=" + strconv.Itoa(first),
		"-f", "query=" + fmt.Sprintf(projectItemsQuery, ownerKind),
	}
}

func projectFieldsArgs(ownerKind, owner string, number int) []string {
	return []string{
		"api", "graphql",
		"-F", "login=" + owner,
		"-F", "number=" + strconv.Itoa(number),
		"-f", "query=" + fmt.Sprintf(projectFieldsQuery, ownerKind),
	}
}

// ---------------------------------------------------------------------------
// Fixtures — minimal-GraphQL response shapes
// ---------------------------------------------------------------------------

const projectMetaGraphQLJSON = `{"data":{"organization":{"projectV2":{"id":"PVT_xyz","title":"Shop Project"}}}}`

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

// ---------------------------------------------------------------------------
// New
// ---------------------------------------------------------------------------

func TestNew_RejectsBadProjectURL(t *testing.T) {
	_, err := New("https://example.com/not-a-project", nil)
	if err == nil {
		t.Fatalf("expected error for bad project URL")
	}
}

func TestNew_AcceptsOrgAndUserURLs(t *testing.T) {
	cases := []string{
		"https://github.com/orgs/optivem/projects/20",
		"https://github.com/users/alice/projects/3",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			tr, err := New(in, newFakeRunner(t))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tr == nil {
				t.Fatalf("nil tracker")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FindIssue
// ---------------------------------------------------------------------------

func TestFindIssue_ByID(t *testing.T) {
	gh := newFakeRunner(t)
	gh.on(projectMetaArgs("organization", "optivem", 20),
		[]byte(projectMetaGraphQLJSON), nil)
	gh.on(projectItemsArgs("organization", "optivem", 20, 100),
		[]byte(itemListJSON), nil)

	tr := mustNew(t, "https://github.com/orgs/optivem/projects/20", gh)
	issue, err := tr.FindIssue(context.Background(), "43")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issue.ID != "43" || issue.Title != "second ready" {
		t.Errorf("got (%q, %q), want (43, second ready)", issue.ID, issue.Title)
	}
}

func TestFindIssue_ByURL(t *testing.T) {
	gh := newFakeRunner(t)
	gh.on(projectMetaArgs("organization", "optivem", 20),
		[]byte(projectMetaGraphQLJSON), nil)
	gh.on(projectItemsArgs("organization", "optivem", 20, 100),
		[]byte(itemListJSON), nil)

	tr := mustNew(t, "https://github.com/orgs/optivem/projects/20", gh)
	issue, err := tr.FindIssue(context.Background(),
		"https://github.com/optivem/shop/issues/42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issue.ID != "42" {
		t.Errorf("ID = %q, want 42", issue.ID)
	}
}

func TestFindIssue_NotFound(t *testing.T) {
	gh := newFakeRunner(t)
	gh.on(projectMetaArgs("organization", "optivem", 20),
		[]byte(projectMetaGraphQLJSON), nil)
	gh.on(projectItemsArgs("organization", "optivem", 20, 100),
		[]byte(itemListJSON), nil)

	tr := mustNew(t, "https://github.com/orgs/optivem/projects/20", gh)
	_, err := tr.FindIssue(context.Background(), "999")
	if err == nil {
		t.Fatalf("expected error for missing issue")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error did not mention 'not found': %v", err)
	}
}

func TestFindIssue_RejectsEmptyAndBadInput(t *testing.T) {
	cases := []string{
		"",
		"0",
		"-1",
		"https://example.com/foo/bar/issues/1", // wrong host
		"not a url",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			gh := newFakeRunner(t)
			tr := mustNew(t, "https://github.com/orgs/optivem/projects/20", gh)
			if _, err := tr.FindIssue(context.Background(), in); err == nil {
				t.Errorf("expected error for input %q", in)
			}
			if len(gh.calls) > 0 {
				t.Errorf("invalid input must not call gh; got %v", gh.calls)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SetStatus
// ---------------------------------------------------------------------------

func TestSetStatus_PassesExpectedArgsToGh(t *testing.T) {
	gh := newFakeRunner(t)
	gh.on(projectFieldsArgs("organization", "optivem", 20),
		[]byte(fieldListJSON), nil)
	gh.on([]string{
		"project", "item-edit",
		"--id", "item-B",
		"--field-id", "PVTSSF_status",
		"--project-id", "PVT_xyz",
		"--single-select-option-id", "opt-inprogress",
	}, []byte(`{}`), nil)

	tr := mustNew(t, "https://github.com/orgs/optivem/projects/20", gh)
	err := tr.SetStatus(context.Background(),
		encodeHandle("PVT_xyz", "item-B"), "In progress")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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

func TestSetStatus_RejectsMalformedHandle(t *testing.T) {
	cases := []string{
		"",
		"only-one-part",
		":missing-project",
		"missing-item:",
	}
	for _, h := range cases {
		t.Run(h, func(t *testing.T) {
			gh := newFakeRunner(t)
			tr := mustNew(t, "https://github.com/orgs/optivem/projects/20", gh)
			if err := tr.SetStatus(context.Background(), h, "In progress"); err == nil {
				t.Errorf("expected error for handle %q", h)
			}
			if len(gh.calls) > 0 {
				t.Errorf("malformed handle must not call gh; got %v", gh.calls)
			}
		})
	}
}

func TestSetStatus_StatusFieldMissing(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"no Status field", fieldListNoStatusJSON},
		{"Status field missing 'In progress' option", fieldListNoInProgressJSON},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gh := newFakeRunner(t)
			gh.on(projectFieldsArgs("organization", "optivem", 20),
				[]byte(tc.body), nil)
			tr := mustNew(t, "https://github.com/orgs/optivem/projects/20", gh)
			err := tr.SetStatus(context.Background(),
				encodeHandle("PVT_xyz", "item-B"), "In progress")
			if !errors.Is(err, ErrStatusFieldMissing) {
				t.Errorf("expected ErrStatusFieldMissing, got %v", err)
			}
		})
	}
}

func TestSetStatus_GhItemEditError(t *testing.T) {
	gh := newFakeRunner(t)
	gh.on(projectFieldsArgs("organization", "optivem", 20),
		[]byte(fieldListJSON), nil)
	gh.on([]string{
		"project", "item-edit",
		"--id", "item-B",
		"--field-id", "PVTSSF_status",
		"--project-id", "PVT_xyz",
		"--single-select-option-id", "opt-inprogress",
	}, nil, fmt.Errorf("gh: HTTP 403"))

	tr := mustNew(t, "https://github.com/orgs/optivem/projects/20", gh)
	err := tr.SetStatus(context.Background(),
		encodeHandle("PVT_xyz", "item-B"), "In progress")
	if err == nil {
		t.Fatalf("expected wrapped error, got nil")
	}
	if !strings.Contains(err.Error(), "gh project item-edit") {
		t.Errorf("error did not wrap gh failure: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Verify
// ---------------------------------------------------------------------------

func TestVerify_HappyPath(t *testing.T) {
	gh := newFakeRunner(t)
	gh.on(projectMetaArgs("organization", "optivem", 20),
		[]byte(projectMetaGraphQLJSON), nil)

	tr := mustNew(t, "https://github.com/orgs/optivem/projects/20", gh)
	if err := tr.Verify(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerify_GraphQLNotFound(t *testing.T) {
	gh := newFakeRunner(t)
	gh.on(projectMetaArgs("organization", "optivem", 20),
		[]byte(`{"data":{"organization":{"projectV2":null}}}`), nil)

	tr := mustNew(t, "https://github.com/orgs/optivem/projects/20", gh)
	err := tr.Verify(context.Background())
	if err == nil {
		t.Fatalf("expected not-accessible error")
	}
	if !strings.Contains(err.Error(), "not accessible") {
		t.Errorf("error did not mention 'not accessible': %v", err)
	}
}

// ---------------------------------------------------------------------------
// Classify / ReadSections
// ---------------------------------------------------------------------------

func TestClassify_NativeIssueType(t *testing.T) {
	for _, tc := range []struct {
		name         string
		responseJSON string
		wantKind     string
		wantOK       bool
	}{
		{name: "story", responseJSON: `{"data":{"repository":{"issue":{"issueType":{"name":"Story"}}}}}`, wantKind: "story", wantOK: true},
		{name: "bug", responseJSON: `{"data":{"repository":{"issue":{"issueType":{"name":"Bug"}}}}}`, wantKind: "bug", wantOK: true},
		{name: "task_uppercase", responseJSON: `{"data":{"repository":{"issue":{"issueType":{"name":"TASK"}}}}}`, wantKind: "task", wantOK: true},
		{name: "no_type", responseJSON: `{"data":{"repository":{"issue":{"issueType":null}}}}`, wantKind: "", wantOK: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gh := newFakeRunner(t)
			gh.on(
				[]string{
					"api", "graphql",
					"-f", "owner=optivem",
					"-f", "name=shop",
					"-F", "number=42",
					"-f", "query=" + issueTypeQuery,
				},
				[]byte(tc.responseJSON),
				nil,
			)
			tr := mustNew(t, "https://github.com/orgs/optivem/projects/20", gh)
			issue := tracker.Issue{ID: "42", URL: "https://github.com/optivem/shop/issues/42"}
			kind, ok, err := tr.Classify(context.Background(), issue)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if kind != tc.wantKind || ok != tc.wantOK {
				t.Errorf("got (%q, %v), want (%q, %v)", kind, ok, tc.wantKind, tc.wantOK)
			}
		})
	}
}

func TestSubtypes(t *testing.T) {
	for _, tc := range []struct {
		name         string
		responseJSON string
		want         []string
	}{
		{
			name:         "single",
			responseJSON: `{"labels":[{"name":"area:billing"},{"name":"subtype:system-interface-redesign"}]}`,
			want:         []string{"system-interface-redesign"},
		},
		{
			name:         "multiple",
			responseJSON: `{"labels":[{"name":"subtype:system-interface-redesign"},{"name":"subtype:system-implementation-refactoring"}]}`,
			want:         []string{"system-interface-redesign", "system-implementation-refactoring"},
		},
		{
			name:         "none",
			responseJSON: `{"labels":[{"name":"area:billing"}]}`,
			want:         nil,
		},
		{
			name:         "empty_labels",
			responseJSON: `{"labels":[]}`,
			want:         nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gh := newFakeRunner(t)
			gh.on(
				[]string{"issue", "view", "42", "--json", "labels", "--repo", "optivem/shop"},
				[]byte(tc.responseJSON),
				nil,
			)
			tr := mustNew(t, "https://github.com/orgs/optivem/projects/20", gh)
			issue := tracker.Issue{ID: "42", URL: "https://github.com/optivem/shop/issues/42"}
			got, err := tr.Subtypes(context.Background(), issue)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReadSections_ReturnsRequestedHeadings(t *testing.T) {
	body := "## Description\n\nIntro paragraph.\n\n" +
		"## Acceptance Criteria\n\n- AC1\n- AC2\n\n" +
		"## Checklist\n\n- [ ] Step\n"
	bodyJSON, _ := json.Marshal(body)
	gh := newFakeRunner(t)
	gh.on(
		[]string{"issue", "view", "42", "--json", "body", "--repo", "optivem/shop"},
		[]byte(`{"body":` + string(bodyJSON) + `}`),
		nil,
	)
	tr := mustNew(t, "https://github.com/orgs/optivem/projects/20", gh)
	issue := tracker.Issue{ID: "42", URL: "https://github.com/optivem/shop/issues/42"}
	sections, err := tr.ReadSections(context.Background(), issue, []string{"Acceptance Criteria", "Checklist", "Missing"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := sections["Acceptance Criteria"], "- AC1\n- AC2"; got != want {
		t.Errorf("Acceptance Criteria: got %q, want %q", got, want)
	}
	if got, want := sections["Checklist"], "- [ ] Step"; got != want {
		t.Errorf("Checklist: got %q, want %q", got, want)
	}
	if got := sections["Missing"]; got != "" {
		t.Errorf("Missing: got %q, want empty string", got)
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
			ownerKind, owner, number, err := parseProjectURL(tc.in)
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

func TestEncodeDecodeHandle_RoundTrip(t *testing.T) {
	const proj, item = "PVT_xyz", "PVTI_abc"
	h := encodeHandle(proj, item)
	gotProj, gotItem, err := decodeHandle(h)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if gotProj != proj || gotItem != item {
		t.Errorf("round-trip got (%q, %q), want (%q, %q)", gotProj, gotItem, proj, item)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func mustNew(t *testing.T, projectURL string, gh GhRunner) *Tracker {
	t.Helper()
	tr, err := New(projectURL, gh)
	if err != nil {
		t.Fatalf("New(%q): %v", projectURL, err)
	}
	return tr
}
