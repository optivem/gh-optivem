// Tests for clauderun.Dispatch.
//
// Strategy: drive Dispatch through fakeClaude / fakeGit so the suite is
// hermetic — no real `claude` or `git` invocations. Each fake captures
// the args / Run call it received and emits canned values, letting us
// assert prompt construction, commit-detection branches, and error paths.
package clauderun

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakeClaude records the RunOpts it was called with and returns a canned
// error. headFn (when set) is called inside Run so the test can simulate
// the agent producing a commit during the subprocess — by mutating
// fakeGit.heads in lock-step with the call sequence.
type fakeClaude struct {
	calls  []RunOpts
	err    error
	headFn func()
}

func (f *fakeClaude) Run(_ context.Context, opts RunOpts) error {
	f.calls = append(f.calls, opts)
	if f.headFn != nil {
		f.headFn()
	}
	return f.err
}

// fakeGit serves canned outputs in call order. revparse and log calls
// alternate predictably (Dispatch calls rev-parse twice — before and after
// — and log once on success), so a single FIFO of byte-slices is enough
// for every test case below.
type fakeGit struct {
	out  [][]byte
	err  error
	args [][]string
}

func (f *fakeGit) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	f.args = append(f.args, args)
	if f.err != nil {
		return nil, f.err
	}
	if len(f.out) == 0 {
		return nil, errors.New("fakeGit: no canned output left")
	}
	v := f.out[0]
	f.out = f.out[1:]
	return v, nil
}

func newOpts() Options {
	return Options{
		Agent:           "atdd-test",
		PhaseDoc:        "docs/atdd/process/at-red-test.md",
		NodeDescription: "Write the AT-RED scenario",
		IssueNum:        42,
		IssueTitle:      "Add PUT /carts/{id}/items endpoint",
		IssueRepo:       "optivem/shop",
		ProjectTitle:    "Shop ATDD",
		ProjectURL:      "https://github.com/orgs/optivem/projects/1",
		// Discard banners so test output stays clean.
		Stdout: io.Discard,
		Stderr: io.Discard,
		Stdin:  strings.NewReader(""),
	}
}

// ---------------------------------------------------------------------------
// Prompt construction
// ---------------------------------------------------------------------------

func TestRenderPrompt_IncludesAllFields(t *testing.T) {
	opts := newOpts()
	opts.OverrideText = "prefer record types"

	got, err := renderPrompt(opts)
	if err != nil {
		t.Fatalf("renderPrompt: %v", err)
	}

	mustContain(t, got, "Launch the atdd-test subagent")
	mustContain(t, got, `#42 "Add PUT /carts/{id}/items endpoint"`)
	mustContain(t, got, "(optivem/shop)")
	mustContain(t, got, "Shop ATDD (https://github.com/orgs/optivem/projects/1)")
	mustContain(t, got, "Phase: Write the AT-RED scenario")
	mustContain(t, got, "Phase doc: docs/atdd/process/at-red-test.md")
	mustContain(t, got, "prefer record types")
	mustContain(t, got, "your COMMIT must land on HEAD")
}

func TestRenderPrompt_OmitsOverrideTextSection_WhenEmpty(t *testing.T) {
	opts := newOpts()
	opts.OverrideText = ""

	got, err := renderPrompt(opts)
	if err != nil {
		t.Fatalf("renderPrompt: %v", err)
	}
	// The header line above the override block is "Phase doc: ...", and
	// the line after is "When the agent finishes". With empty override
	// there should not be a stray double-blank between them.
	if strings.Contains(got, "\n\n\n") {
		t.Fatalf("expected no triple-newline (orphan override block), got:\n%s", got)
	}
}

func TestRenderPrompt_OmitsProjectLine_WhenURLMissing(t *testing.T) {
	opts := newOpts()
	opts.ProjectTitle = ""
	opts.ProjectURL = ""

	got, err := renderPrompt(opts)
	if err != nil {
		t.Fatalf("renderPrompt: %v", err)
	}
	if strings.Contains(got, "Project:") {
		t.Fatalf("expected no Project: line when URL is empty, got:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// Dispatch — happy path
// ---------------------------------------------------------------------------

func TestDispatch_SuccessReturnsCommitInfo(t *testing.T) {
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaaaaa1111111\n"), // rev-parse before
			[]byte("bbbbbbb2222222\n"), // rev-parse after
			[]byte("AT-RED-TEST: scenario for PUT /carts/{id}/items\n"), // log subject
		},
	}
	claudeFake := &fakeClaude{}

	got, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, newOpts())
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if got.SHA != "bbbbbbb2222222" {
		t.Errorf("SHA: got %q, want %q", got.SHA, "bbbbbbb2222222")
	}
	if got.Subject != "AT-RED-TEST: scenario for PUT /carts/{id}/items" {
		t.Errorf("Subject: got %q", got.Subject)
	}
	if len(claudeFake.calls) != 1 {
		t.Fatalf("expected 1 claude call, got %d", len(claudeFake.calls))
	}
	// Prompt is constructed and passed through to the runner.
	if !strings.Contains(claudeFake.calls[0].Prompt, "Launch the atdd-test subagent") {
		t.Errorf("prompt missing launch line:\n%s", claudeFake.calls[0].Prompt)
	}
}

func TestDispatch_AutonomousFlagPropagates(t *testing.T) {
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaa\n"),
			[]byte("bbbb\n"),
			[]byte("subject\n"),
		},
	}
	claudeFake := &fakeClaude{}

	opts := newOpts()
	opts.Autonomous = true
	opts.MaxTurns = 50

	if _, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	got := claudeFake.calls[0]
	if !got.Autonomous {
		t.Errorf("Autonomous: got false, want true")
	}
	if got.MaxTurns != 50 {
		t.Errorf("MaxTurns: got %d, want 50", got.MaxTurns)
	}
}

// ---------------------------------------------------------------------------
// Dispatch — failure paths
// ---------------------------------------------------------------------------

func TestDispatch_FailsWhenSubprocessExitsNonZero(t *testing.T) {
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaa\n"), // only the "before" rev-parse should land
		},
	}
	claudeFake := &fakeClaude{err: errors.New("exit status 1")}

	_, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, newOpts())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "exited non-zero") {
		t.Errorf("error wording: got %q", err.Error())
	}
	// The "after" rev-parse and the log call must NOT happen on the
	// non-zero-exit path — surfacing stderr is the only useful action.
	if len(gitFake.args) != 1 {
		t.Errorf("expected 1 git call (rev-parse before only), got %d: %v", len(gitFake.args), gitFake.args)
	}
}

func TestDispatch_FailsWhenHEADUnchanged(t *testing.T) {
	// Same HEAD before and after → "subprocess succeeded but produced no
	// commit". Important: we still expect the rev-parse-after call to land
	// (so we can compare), but no `git log` since there's no new SHA.
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("samesha\n"),
			[]byte("samesha\n"),
		},
	}
	claudeFake := &fakeClaude{}

	_, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, newOpts())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no commit") {
		t.Errorf("error wording: got %q", err.Error())
	}
	if len(gitFake.args) != 2 {
		t.Errorf("expected 2 git calls (both rev-parses, no log), got %d: %v", len(gitFake.args), gitFake.args)
	}
}

func TestDispatch_PropagatesGitFailureBeforeRun(t *testing.T) {
	gitFake := &fakeGit{err: errors.New("not a git repo")}
	claudeFake := &fakeClaude{}

	_, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, newOpts())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "read HEAD before dispatch") {
		t.Errorf("error wording: got %q", err.Error())
	}
	if len(claudeFake.calls) != 0 {
		t.Errorf("claude must not run when pre-flight git fails, got %d calls", len(claudeFake.calls))
	}
}

// ---------------------------------------------------------------------------
// Banner output (smoke check — we do not lock in exact ANSI bytes)
// ---------------------------------------------------------------------------

func TestDispatch_WritesEnterAndExitBanners(t *testing.T) {
	var buf bytes.Buffer
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaa\n"),
			[]byte("bbbb\n"),
			[]byte("subject\n"),
		},
	}
	opts := newOpts()
	opts.Stdout = &buf

	if _, err := Dispatch(context.Background(), Deps{Claude: &fakeClaude{}, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	got := buf.String()
	mustContain(t, got, "ENTERING AGENT")
	mustContain(t, got, "atdd-test")
	mustContain(t, got, "EXITED AGENT")
	mustContain(t, got, "bbbb") // short SHA prefix
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("missing %q in:\n%s", needle, haystack)
	}
}
