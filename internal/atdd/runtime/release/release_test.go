// Tests for the release package.
//
// Strategy:
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
	"io"
	"strings"
	"testing"
)

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

