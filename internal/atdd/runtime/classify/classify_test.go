package classify

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGh is the test-side implementation of GhRunner. It returns a canned
// JSON payload regardless of the args, and records the last invocation so
// individual cases can assert on it if they need to.
type fakeGh struct {
	json    string
	err     error
	gotArgs []string
}

func (f *fakeGh) Run(_ context.Context, args ...string) ([]byte, error) {
	f.gotArgs = append([]string{}, args...)
	if f.err != nil {
		return nil, f.err
	}
	return []byte(f.json), nil
}

// TestClassify_TableDriven covers the deterministic rule set end to end,
// including the fast-path mappings, conflict detection, and unknown-Type
// handling. Each case asserts Route + Classification; reasoning is spot-
// checked via substring so wording tweaks don't churn the test.
func TestClassify_TableDriven(t *testing.T) {
	cases := []struct {
		name        string
		json        string
		wantRoute   Route
		wantClass   Classification
		wantReasonContains string
	}{
		// --- Type field fast-paths -----------------------------------
		{
			name:      "type field Bug → bug",
			json:      `{"number":1,"title":"oops","labels":[],"projectItems":[{"title":"ATDD","type":{"name":"Bug"}}]}`,
			wantRoute: FastPath,
			wantClass: Bug,
			wantReasonContains: "bug (fast path)",
		},
		{
			name:      "type field Task → task",
			json:      `{"number":2,"title":"rename","labels":[],"projectItems":[{"type":{"name":"Task"}}]}`,
			wantRoute: FastPath,
			wantClass: Task,
			wantReasonContains: "task (fast path)",
		},
		{
			name:      "type field Chore → chore",
			json:      `{"number":3,"title":"tidy","labels":[],"projectItems":[{"type":{"name":"Chore"}}]}`,
			wantRoute: FastPath,
			wantClass: Chore,
			wantReasonContains: "chore (fast path)",
		},
		{
			name:      "type field Feature → story",
			json:      `{"number":4,"title":"add x","labels":[],"projectItems":[{"type":{"name":"Feature"}}]}`,
			wantRoute: FastPath,
			wantClass: Story,
			wantReasonContains: "story (fast path)",
		},
		{
			name:      "type field Story (alt name) → story",
			json:      `{"number":5,"title":"add y","labels":[],"projectItems":[{"type":{"name":"Story"}}]}`,
			wantRoute: FastPath,
			wantClass: Story,
		},

		// --- Type field as bare string (alt schema shape) ------------
		{
			name:      "type field encoded as bare string",
			json:      `{"number":6,"title":"oops","labels":[],"projectItems":[{"type":"Bug"}]}`,
			wantRoute: FastPath,
			wantClass: Bug,
		},

		// --- Label-only fast paths -----------------------------------
		{
			name:      "single bug label → bug",
			json:      `{"number":10,"title":"x","labels":[{"name":"bug"}],"projectItems":[]}`,
			wantRoute: FastPath,
			wantClass: Bug,
			wantReasonContains: "Single canonical type label",
		},
		{
			name:      "ui-bug label (contains bug token) → bug",
			json:      `{"number":11,"title":"x","labels":[{"name":"ui-bug"}],"projectItems":[]}`,
			wantRoute: FastPath,
			wantClass: Bug,
		},
		{
			name:      "system-api-redesign-add-endpoint → task",
			json:      `{"number":12,"title":"x","labels":[{"name":"system-api-redesign-add-endpoint"}],"projectItems":[]}`,
			wantRoute: FastPath,
			wantClass: Task,
		},
		{
			name:      "story label → story",
			json:      `{"number":13,"title":"x","labels":[{"name":"story"}],"projectItems":[]}`,
			wantRoute: FastPath,
			wantClass: Story,
		},
		{
			name:      "feature label → story",
			json:      `{"number":14,"title":"x","labels":[{"name":"feature"}],"projectItems":[]}`,
			wantRoute: FastPath,
			wantClass: Story,
		},
		{
			name:      "chore label → chore",
			json:      `{"number":15,"title":"x","labels":[{"name":"chore"}],"projectItems":[]}`,
			wantRoute: FastPath,
			wantClass: Chore,
		},
		{
			name:      "refactor label → task",
			json:      `{"number":16,"title":"x","labels":[{"name":"refactor"}],"projectItems":[]}`,
			wantRoute: FastPath,
			wantClass: Task,
		},
		{
			name:      "non-type labels ignored, single bug wins",
			json:      `{"number":17,"title":"x","labels":[{"name":"priority-high"},{"name":"bug"},{"name":"area/api"}],"projectItems":[]}`,
			wantRoute: FastPath,
			wantClass: Bug,
		},

		// --- Fallback paths ------------------------------------------
		{
			name:      "no labels and no type → fallback",
			json:      `{"number":20,"title":"x","labels":[],"projectItems":[]}`,
			wantRoute: Fallback,
			wantReasonContains: "No Projects v2 Type field and no type-bearing label",
		},
		{
			name:      "no labels, projectItems with no type → fallback",
			json:      `{"number":21,"title":"x","labels":[],"projectItems":[{"title":"ATDD"}]}`,
			wantRoute: Fallback,
		},
		{
			name:      "two conflicting type-labels → fallback",
			json:      `{"number":22,"title":"x","labels":[{"name":"bug"},{"name":"task"}],"projectItems":[]}`,
			wantRoute: Fallback,
			wantReasonContains: "Conflicting type-bearing labels",
		},
		{
			name:      "story + bug labels → fallback",
			json:      `{"number":23,"title":"x","labels":[{"name":"story"},{"name":"bug"}],"projectItems":[]}`,
			wantRoute: Fallback,
		},
		{
			name:      "unknown type field value → fallback",
			json:      `{"number":24,"title":"x","labels":[{"name":"bug"}],"projectItems":[{"type":{"name":"Spike"}}]}`,
			wantRoute: Fallback,
			wantReasonContains: "is not a recognised value",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			logPath := filepath.Join(tmp, "classify.log")

			runner := &fakeGh{json: tc.json}
			res, err := Classify(context.Background(), 1, Options{
				LogPath:  logPath,
				GhRunner: runner,
			})
			if err != nil {
				t.Fatalf("Classify: unexpected error: %v", err)
			}
			if res.Route != tc.wantRoute {
				t.Fatalf("Route = %q, want %q (reasoning=%q)",
					res.Route, tc.wantRoute, res.Reasoning)
			}
			if tc.wantRoute == FastPath && res.Classification != tc.wantClass {
				t.Fatalf("Classification = %q, want %q (reasoning=%q)",
					res.Classification, tc.wantClass, res.Reasoning)
			}
			if tc.wantRoute == Fallback && res.Classification != "" {
				t.Fatalf("Fallback Result must have empty Classification, got %q",
					res.Classification)
			}
			if tc.wantReasonContains != "" && !strings.Contains(res.Reasoning, tc.wantReasonContains) {
				t.Fatalf("Reasoning %q does not contain %q",
					res.Reasoning, tc.wantReasonContains)
			}
		})
	}
}

// TestClassify_AppendsLogLine asserts that each call appends exactly one
// well-formed line to the configured log path, and that consecutive calls
// accumulate rather than overwrite.
func TestClassify_AppendsLogLine(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "classify.log")

	runner := &fakeGh{json: `{"number":42,"title":"x","labels":[{"name":"bug"}],"projectItems":[]}`}
	if _, err := Classify(context.Background(), 42, Options{LogPath: logPath, GhRunner: runner}); err != nil {
		t.Fatalf("first Classify: %v", err)
	}

	runner.json = `{"number":43,"title":"x","labels":[],"projectItems":[]}`
	if _, err := Classify(context.Background(), 43, Options{LogPath: logPath, GhRunner: runner}); err != nil {
		t.Fatalf("second Classify: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 log lines, got %d:\n%s", len(lines), string(data))
	}

	// First line: fastpath bug for issue 42.
	if !strings.Contains(lines[0], "issue=42") ||
		!strings.Contains(lines[0], "classification=bug") ||
		!strings.Contains(lines[0], "route=fastpath") ||
		!strings.Contains(lines[0], "labels=[bug]") {
		t.Errorf("line 0 malformed: %q", lines[0])
	}

	// Second line: fallback inconclusive for issue 43.
	if !strings.Contains(lines[1], "issue=43") ||
		!strings.Contains(lines[1], "classification=inconclusive") ||
		!strings.Contains(lines[1], "route=fallback") ||
		!strings.Contains(lines[1], "labels=[]") {
		t.Errorf("line 1 malformed: %q", lines[1])
	}
}

// TestClassify_GhArgs asserts the exact argv handed to gh, including the
// --repo passthrough when Options.Repo is set.
func TestClassify_GhArgs(t *testing.T) {
	runner := &fakeGh{json: `{"number":1,"labels":[],"projectItems":[]}`}
	tmp := t.TempDir()
	if _, err := Classify(context.Background(), 7, Options{
		Repo:     "optivem/shop",
		LogPath:  filepath.Join(tmp, "classify.log"),
		GhRunner: runner,
	}); err != nil {
		t.Fatalf("Classify: %v", err)
	}
	got := strings.Join(runner.gotArgs, " ")
	want := "issue view 7 --json number,title,labels,projectItems --repo optivem/shop"
	if got != want {
		t.Fatalf("gh args = %q, want %q", got, want)
	}

	// Without Repo, the --repo flag must not appear (gh falls back to
	// the current git remote).
	runner.gotArgs = nil
	if _, err := Classify(context.Background(), 7, Options{
		LogPath:  filepath.Join(tmp, "classify.log"),
		GhRunner: runner,
	}); err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if strings.Contains(strings.Join(runner.gotArgs, " "), "--repo") {
		t.Fatalf("expected no --repo flag when Repo=='', got %v", runner.gotArgs)
	}
}

// TestClassify_RejectsInvalidIssueNum guards the precondition.
func TestClassify_RejectsInvalidIssueNum(t *testing.T) {
	if _, err := Classify(context.Background(), 0, Options{GhRunner: &fakeGh{json: "{}"}}); err == nil {
		t.Fatal("expected error for issueNum=0, got nil")
	}
}
