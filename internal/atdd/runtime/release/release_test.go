// Tests for the release package.
//
// Strategy:
//   - RemoveDisabledMarkers is exercised via golden-file fixtures under
//     testdata/{java,dotnet,typescript}. Each input file has a paired
//     -expected sibling; the test copies the input into a per-test temp
//     dir, runs RemoveDisabledMarkers, and byte-compares the result
//     against the expected file. This covers the "marker present",
//     "multiple markers", "no marker" and "import cleanup" cases the
//     contract requires.
//   - Commit and CloseIssue are exercised against fake runners; we never
//     actually invoke `git commit` or `gh issue close`.
//   - The Confirmer policy is verified at the type level: a nil Confirmer
//     returns ErrConfirmerRequired, a Confirmer returning false returns
//     ErrCommitDeclined and never calls `git commit`, and an erroring
//     Confirmer wraps the error.
package release

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// -------------------------------------------------------------------------
// Golden-file fixture tests
// -------------------------------------------------------------------------

// fixtureCase pairs an input file (under testdata/) with its expected
// output. The Name is used as the t.Run subtest name.
type fixtureCase struct {
	Name     string
	Input    string // path under testdata/
	Expected string // path under testdata/
}

func TestRemoveDisabledMarkers_Java(t *testing.T) {
	cases := []fixtureCase{
		{
			Name:     "single_disabled_with_import_cleanup",
			Input:    "java/disabled-with-import-input.java",
			Expected: "java/disabled-with-import-expected.java",
		},
		{
			Name:     "multiple_disabled_markers",
			Input:    "java/multi-disabled-input.java",
			Expected: "java/multi-disabled-expected.java",
		},
		{
			Name:     "no_marker_is_noop",
			Input:    "java/no-marker-input.java",
			Expected: "java/no-marker-expected.java",
		},
	}
	runFixtureCases(t, cases)
}

func TestRemoveDisabledMarkers_DotNet(t *testing.T) {
	cases := []fixtureCase{
		{
			Name:     "fact_theory_skip_param_drops_skip_keeps_attribute",
			Input:    "dotnet/fact-skip-input.cs",
			Expected: "dotnet/fact-skip-expected.cs",
		},
		{
			Name:     "standalone_skip_attribute_removed",
			Input:    "dotnet/skip-attribute-input.cs",
			Expected: "dotnet/skip-attribute-expected.cs",
		},
		{
			Name:     "no_marker_is_noop",
			Input:    "dotnet/no-marker-input.cs",
			Expected: "dotnet/no-marker-expected.cs",
		},
	}
	runFixtureCases(t, cases)
}

func TestRemoveDisabledMarkers_TypeScript(t *testing.T) {
	cases := []fixtureCase{
		{
			Name:     "single_test_skip",
			Input:    "typescript/test-skip-input.spec.ts",
			Expected: "typescript/test-skip-expected.spec.ts",
		},
		{
			Name:     "multiple_test_skip",
			Input:    "typescript/multi-skip-input.spec.ts",
			Expected: "typescript/multi-skip-expected.spec.ts",
		},
		{
			Name:     "no_marker_is_noop",
			Input:    "typescript/no-marker-input.spec.ts",
			Expected: "typescript/no-marker-expected.spec.ts",
		},
	}
	runFixtureCases(t, cases)
}

func runFixtureCases(t *testing.T, cases []fixtureCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			tmp := t.TempDir()
			// Stage the fixture under tmp/<basename> so the walker uses
			// the same extension as production. We strip the "-input"
			// suffix so the file looks like a real test file.
			inputBytes, err := os.ReadFile(filepath.Join("testdata", tc.Input))
			if err != nil {
				t.Fatalf("read input: %v", err)
			}
			expectedBytes, err := os.ReadFile(filepath.Join("testdata", tc.Expected))
			if err != nil {
				t.Fatalf("read expected: %v", err)
			}
			stagedName := stripInputSuffix(filepath.Base(tc.Input))
			stagedPath := filepath.Join(tmp, stagedName)
			if err := os.WriteFile(stagedPath, inputBytes, 0o644); err != nil {
				t.Fatalf("write staged: %v", err)
			}

			changes, err := RemoveDisabledMarkers(context.Background(), RemoveOptions{
				Roots: []string{tmp},
			})
			if err != nil {
				t.Fatalf("RemoveDisabledMarkers: %v", err)
			}

			gotBytes, err := os.ReadFile(stagedPath)
			if err != nil {
				t.Fatalf("read staged after run: %v", err)
			}
			if string(gotBytes) != string(expectedBytes) {
				t.Errorf("output mismatch for %s\n--- got ---\n%s\n--- want ---\n%s",
					tc.Input, string(gotBytes), string(expectedBytes))
			}

			// Sanity: a noop fixture (input == expected) must produce zero
			// FileChange entries; everything else must produce exactly one.
			isNoop := string(inputBytes) == string(expectedBytes)
			if isNoop && len(changes.Files) != 0 {
				t.Errorf("expected no FileChange for noop fixture, got %d", len(changes.Files))
			}
			if !isNoop && len(changes.Files) == 0 {
				t.Errorf("expected at least one FileChange for non-noop fixture, got 0")
			}
		})
	}
}

func stripInputSuffix(name string) string {
	// "disabled-with-import-input.java" → "disabled-with-import.java"
	// "multi-skip-input.spec.ts"        → "multi-skip.spec.ts"
	const marker = "-input"
	idx := strings.Index(name, marker)
	if idx < 0 {
		return name
	}
	return name[:idx] + name[idx+len(marker):]
}

// TestRemoveDisabledMarkers_MissingRoot asserts that a non-existent root is
// silently skipped (per the soft-skip contract for callers passing all
// three language roots unconditionally).
func TestRemoveDisabledMarkers_MissingRoot(t *testing.T) {
	tmp := t.TempDir()
	missing := filepath.Join(tmp, "does-not-exist")
	changes, err := RemoveDisabledMarkers(context.Background(), RemoveOptions{
		Roots: []string{missing},
	})
	if err != nil {
		t.Fatalf("RemoveDisabledMarkers on missing root: %v", err)
	}
	if len(changes.Files) != 0 {
		t.Errorf("expected zero FileChanges for missing root, got %d", len(changes.Files))
	}
}

// -------------------------------------------------------------------------
// Commit tests
// -------------------------------------------------------------------------

// fakeGit records every Run invocation. Tests can pre-load Outputs to
// return per call (rotating through them); RunErr, if non-nil, is returned
// for every call.
type fakeGit struct {
	Calls   [][]string
	Outputs [][]byte
	RunErr  error
}

func (f *fakeGit) Run(ctx context.Context, args ...string) ([]byte, error) {
	f.Calls = append(f.Calls, append([]string(nil), args...))
	if f.RunErr != nil {
		return nil, f.RunErr
	}
	if len(f.Outputs) > 0 {
		out := f.Outputs[0]
		f.Outputs = f.Outputs[1:]
		return out, nil
	}
	return nil, nil
}

func TestCommit_NilConfirmerRejected(t *testing.T) {
	fg := &fakeGit{}
	err := Commit(context.Background(), CommitOptions{
		Message:   "msg",
		Confirm:   nil,
		GitRunner: fg,
		Stdout:    io.Discard,
	})
	if !errors.Is(err, ErrConfirmerRequired) {
		t.Fatalf("expected ErrConfirmerRequired, got %v", err)
	}
	if len(fg.Calls) != 0 {
		t.Errorf("expected no git calls when Confirmer is nil, got %v", fg.Calls)
	}
}

func TestCommit_EmptyMessageRejected(t *testing.T) {
	fg := &fakeGit{}
	err := Commit(context.Background(), CommitOptions{
		Message:   "   ",
		Confirm:   func(string) (bool, error) { return true, nil },
		GitRunner: fg,
		Stdout:    io.Discard,
	})
	if err == nil || !strings.Contains(err.Error(), "non-empty Message") {
		t.Fatalf("expected non-empty Message error, got %v", err)
	}
	if len(fg.Calls) != 0 {
		t.Errorf("expected no git calls for empty message, got %v", fg.Calls)
	}
}

func TestCommit_ConfirmTrueRunsAddAndCommit(t *testing.T) {
	fg := &fakeGit{Outputs: [][]byte{nil, []byte(" M file.txt\n"), nil}}
	err := Commit(context.Background(), CommitOptions{
		Message:   "#42 | Register Customer | AT - GREEN - SYSTEM",
		Confirm:   func(string) (bool, error) { return true, nil },
		GitRunner: fg,
		Stdout:    io.Discard,
	})
	if err != nil {
		t.Fatalf("Commit returned error: %v", err)
	}
	wantCalls := [][]string{
		{"add", "-A"},
		{"status", "--short"},
		{"commit", "-m", "#42 | Register Customer | AT - GREEN - SYSTEM"},
	}
	if !equalCalls(fg.Calls, wantCalls) {
		t.Errorf("git calls mismatch:\ngot:  %v\nwant: %v", fg.Calls, wantCalls)
	}
}

func TestCommit_ConfirmFalseSkipsCommit(t *testing.T) {
	fg := &fakeGit{Outputs: [][]byte{nil, []byte(" M file.txt\n")}}
	err := Commit(context.Background(), CommitOptions{
		Message:   "msg",
		Confirm:   func(string) (bool, error) { return false, nil },
		GitRunner: fg,
		Stdout:    io.Discard,
	})
	if !errors.Is(err, ErrCommitDeclined) {
		t.Fatalf("expected ErrCommitDeclined, got %v", err)
	}
	for _, c := range fg.Calls {
		if len(c) > 0 && c[0] == "commit" {
			t.Errorf("git commit was invoked despite confirmer returning false: %v", c)
		}
	}
}

func TestCommit_ConfirmerErrorWrapped(t *testing.T) {
	fg := &fakeGit{}
	confErr := errors.New("stdin closed")
	err := Commit(context.Background(), CommitOptions{
		Message:   "msg",
		Confirm:   func(string) (bool, error) { return false, confErr },
		GitRunner: fg,
		Stdout:    io.Discard,
	})
	if err == nil {
		t.Fatal("expected error from confirmer, got nil")
	}
	if !errors.Is(err, confErr) {
		t.Errorf("expected wrapped confirmer error, got %v", err)
	}
}

func TestCommit_GitAddFailureSurfaced(t *testing.T) {
	fg := &fakeGit{RunErr: errors.New("not a git repo")}
	err := Commit(context.Background(), CommitOptions{
		Message:   "msg",
		Confirm:   func(string) (bool, error) { return true, nil },
		GitRunner: fg,
		Stdout:    io.Discard,
	})
	if err == nil {
		t.Fatal("expected error from git add failure, got nil")
	}
	if !strings.Contains(err.Error(), "git add -A") {
		t.Errorf("expected wrapped git add error, got %v", err)
	}
}

// -------------------------------------------------------------------------
// CloseIssue tests
// -------------------------------------------------------------------------

type fakeGh struct {
	Calls  [][]string
	RunErr error
}

func (f *fakeGh) Run(ctx context.Context, args ...string) ([]byte, error) {
	f.Calls = append(f.Calls, append([]string(nil), args...))
	return nil, f.RunErr
}

func TestCloseIssue_HappyPath(t *testing.T) {
	fg := &fakeGh{}
	if err := CloseIssue(context.Background(), 42, fg); err != nil {
		t.Fatalf("CloseIssue: %v", err)
	}
	want := [][]string{{"issue", "close", "42"}}
	if !equalCalls(fg.Calls, want) {
		t.Errorf("gh calls mismatch:\ngot:  %v\nwant: %v", fg.Calls, want)
	}
}

func TestCloseIssue_RejectsZeroOrNegative(t *testing.T) {
	fg := &fakeGh{}
	for _, n := range []int{0, -1, -99} {
		err := CloseIssue(context.Background(), n, fg)
		if err == nil {
			t.Errorf("expected error for issue %d, got nil", n)
		}
	}
	if len(fg.Calls) != 0 {
		t.Errorf("expected no gh calls for invalid issue numbers, got %v", fg.Calls)
	}
}

func TestCloseIssue_GhErrorWrapped(t *testing.T) {
	fg := &fakeGh{RunErr: errors.New("not authenticated")}
	err := CloseIssue(context.Background(), 7, fg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "gh issue close 7") {
		t.Errorf("expected wrapped gh error mentioning the issue number, got %v", err)
	}
}

// -------------------------------------------------------------------------
// InteractiveConfirmer tests
// -------------------------------------------------------------------------

func TestInteractiveConfirmer_YesAccepted(t *testing.T) {
	for _, in := range []string{"y\n", "Y\n", "yes\n", "YES\n", "  yes  \n"} {
		conf := InteractiveConfirmer(strings.NewReader(in), io.Discard)
		ok, err := conf("commit? ")
		if err != nil {
			t.Errorf("input %q: unexpected err %v", in, err)
		}
		if !ok {
			t.Errorf("input %q: expected true, got false", in)
		}
	}
}

func TestInteractiveConfirmer_NoVariants(t *testing.T) {
	for _, in := range []string{"n\n", "N\n", "no\n", "NO\n"} {
		conf := InteractiveConfirmer(strings.NewReader(in), io.Discard)
		ok, err := conf("commit?")
		if err != nil {
			t.Errorf("input %q: unexpected err %v", in, err)
		}
		if ok {
			t.Errorf("input %q: expected false, got true", in)
		}
	}
}

func TestInteractiveConfirmer_BareEnterRepromptsThenResolves(t *testing.T) {
	// Bare Enter is no longer a default-decline shortcut; the loop re-prompts
	// until an explicit y/n arrives. This is the property that prevents the
	// originating incident (a stray Enter silently aborting a 2m40s cycle).
	var out bytes.Buffer
	conf := InteractiveConfirmer(strings.NewReader("\n\ny\n"), &out)
	ok, err := conf("commit?")
	if err != nil {
		t.Fatalf("unexpected err %v", err)
	}
	if !ok {
		t.Fatalf("expected true after reprompt, got false")
	}
	if got := strings.Count(out.String(), "commit? [y/n]: "); got != 3 {
		t.Errorf("expected prompt reprinted 3 times, got %d: %q", got, out.String())
	}
}

func TestInteractiveConfirmer_EOFReturnsFalse(t *testing.T) {
	// EOF terminates the loop and declines — "silence = no" terminator for
	// non-interactive callers; without it the loop would spin on a closed
	// stdin. (Detailed coverage lives in promptio_test.go.)
	conf := InteractiveConfirmer(strings.NewReader("maybe\n"), io.Discard)
	ok, err := conf("commit?")
	if err != nil {
		t.Fatalf("unexpected err %v", err)
	}
	if ok {
		t.Fatalf("expected false on EOF, got true")
	}
}

// -------------------------------------------------------------------------
// helpers
// -------------------------------------------------------------------------

func equalCalls(got, want [][]string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if len(got[i]) != len(want[i]) {
			return false
		}
		for j := range got[i] {
			if got[i][j] != want[i][j] {
				return false
			}
		}
	}
	return true
}

// debugDump is used during development to inspect a Changes value.
// Kept in production tests because it documents the intended FileChange
// shape and is cheap.
func debugDump(c Changes) string {
	var b strings.Builder
	for _, f := range c.Files {
		fmt.Fprintf(&b, "  %s: removed=%v edited=%v pattern=%s\n", f.Path, f.LinesRemoved, f.LinesEdited, f.PatternName)
	}
	return b.String()
}

var _ = debugDump // referenced in commented-out diagnostic code; keep available
