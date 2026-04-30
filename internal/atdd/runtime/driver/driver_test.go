// Integration tests for the driver's agent-dispatch wiring.
//
// Strategy: drive wrapAgentDispatchers through fakeClaude / fakeGit so the
// suite is hermetic — no real `claude`, `git`, or YAML file required. We
// build a minimal Engine in-memory via statemachine.LoadBytes, register
// the no-op base dispatchers, bind, then exercise the wrapped NodeFn
// directly. This mirrors the production flow inside Run() while skipping
// the parts that depend on the consumer repo (YAML on disk, real shell
// outs, an actual project board).
package driver

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/agents"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/clauderun"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/override"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakeClaude records each RunOpts so tests can assert prompt content and
// returns a canned error. headFn (when set) is invoked inside Run so a test
// can simulate the agent producing a commit during the subprocess by
// mutating the next fakeGit response in lock-step.
type fakeClaude struct {
	calls  []clauderun.RunOpts
	err    error
	headFn func()
}

func (f *fakeClaude) Run(_ context.Context, opts clauderun.RunOpts) error {
	f.calls = append(f.calls, opts)
	if f.headFn != nil {
		f.headFn()
	}
	return f.err
}

// fakeGit serves canned outputs in call order. Dispatch calls
// rev-parse twice (before / after) and `log -1 --format=%s` once on
// success, so a single FIFO of byte-slices covers every path.
type fakeGit struct {
	out [][]byte
	err error
}

func (f *fakeGit) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
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

// minimalYAML is the smallest flow that exercises the agent-dispatch path:
// START → user_task → END. Nothing in the engine cares about the surrounding
// edges or descriptions, but they're spelled out so the YAML parses cleanly.
const minimalYAML = `
flows:
  main:
    start: START
    nodes:
      - id: START
        type: start_event
      - id: AT_RED_TEST
        type: user_task
        agent: atdd-test
        description: Write the AT-RED scenario
        phase_doc: docs/atdd/process/at-red-test.md
      - id: END
        type: end_event
    sequence_flows:
      - { from: START, to: AT_RED_TEST }
      - { from: AT_RED_TEST, to: END }
`

// buildEngine returns a freshly-bound engine + the wrapped NodeFn for
// AT_RED_TEST. Callers supply fakes via opts.ClaudeRunDeps. Verification /
// override decorators are intentionally NOT applied — those layers have
// their own tests; this fixture targets the agent-dispatch wiring alone.
func buildEngine(t *testing.T, opts Options) statemachine.NodeFn {
	t.Helper()
	eng, err := statemachine.LoadBytes([]byte(minimalYAML))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	agentReg := agents.New()
	registerAgentDispatchers(agentReg)
	eng.AgentFn = agentReg.Lookup
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	wrapAgentDispatchers(eng, opts)
	return eng.Flows["main"].Nodes["AT_RED_TEST"].Fn
}

func newDriverOpts(deps clauderun.Deps) Options {
	return Options{
		ClaudeRunDeps: deps,
		Stdout:        io.Discard,
		Stderr:        io.Discard,
		Stdin:         strings.NewReader(""),
	}
}

func newCtxWithIssue() *statemachine.Context {
	c := statemachine.NewContext()
	c.Set("issue_num", "42")
	c.Set("issue_title", "Add PUT /carts/{id}/items endpoint")
	c.Set("issue_repo", "optivem/shop")
	c.Set("project_title", "Shop ATDD")
	c.Set("project_url", "https://github.com/orgs/optivem/projects/1")
	return c
}

// ---------------------------------------------------------------------------
// Default (clauderun) dispatch — happy path
// ---------------------------------------------------------------------------

func TestClaudeRunDispatch_AdvancesOnFreshCommit(t *testing.T) {
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaaaaa1\n"), // before
			[]byte("bbbbbbb2\n"), // after
			[]byte("AT-RED-TEST: scenario for PUT\n"),
		},
	}
	claudeFake := &fakeClaude{}
	fn := buildEngine(t, newDriverOpts(clauderun.Deps{Claude: claudeFake, Git: gitFake}))

	out := fn(newCtxWithIssue())
	if out.Err != nil {
		t.Fatalf("dispatch: %v", out.Err)
	}
	if out.Commit != "bbbbbbb2" {
		t.Errorf("Commit: got %q, want %q", out.Commit, "bbbbbbb2")
	}
	if len(claudeFake.calls) != 1 {
		t.Fatalf("expected 1 claude call, got %d", len(claudeFake.calls))
	}
	got := claudeFake.calls[0].Prompt
	// Prompt should be constructed from the YAML node + ticket context.
	if !strings.Contains(got, "Launch the atdd-test subagent") {
		t.Errorf("prompt missing launch line:\n%s", got)
	}
	if !strings.Contains(got, "#42") || !strings.Contains(got, "optivem/shop") {
		t.Errorf("prompt missing ticket context:\n%s", got)
	}
	if !strings.Contains(got, "docs/atdd/process/at-red-test.md") {
		t.Errorf("prompt missing phase doc:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// Default dispatch — failure paths
// ---------------------------------------------------------------------------

func TestClaudeRunDispatch_HaltsWhenHEADUnchanged(t *testing.T) {
	// Subprocess succeeds but produces no commit. Driver must surface this
	// as Outcome.Err so the engine halts rather than advancing on a clean
	// exit alone — same shape as v1's "abort" path.
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("samesha\n"),
			[]byte("samesha\n"),
		},
	}
	fn := buildEngine(t, newDriverOpts(clauderun.Deps{Claude: &fakeClaude{}, Git: gitFake}))

	out := fn(newCtxWithIssue())
	if out.Err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(out.Err.Error(), "no commit") {
		t.Errorf("error wording: got %q", out.Err.Error())
	}
}

func TestClaudeRunDispatch_HaltsWhenSubprocessFails(t *testing.T) {
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaa\n"), // only the "before" rev-parse should run
		},
	}
	claudeFake := &fakeClaude{err: errors.New("exit status 1")}
	fn := buildEngine(t, newDriverOpts(clauderun.Deps{Claude: claudeFake, Git: gitFake}))

	out := fn(newCtxWithIssue())
	if out.Err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(out.Err.Error(), "exited non-zero") {
		t.Errorf("error wording: got %q", out.Err.Error())
	}
}

// ---------------------------------------------------------------------------
// Override hint flow
// ---------------------------------------------------------------------------

func TestClaudeRunDispatch_AppendsOverrideExtraToPrompt(t *testing.T) {
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaa\n"),
			[]byte("bbbb\n"),
			[]byte("subject\n"),
		},
	}
	claudeFake := &fakeClaude{}
	fn := buildEngine(t, newDriverOpts(clauderun.Deps{Claude: claudeFake, Git: gitFake}))

	ctx := newCtxWithIssue()
	// Simulates override.Wrap publishing the per-node extra hint before
	// the dispatcher runs (production: this happens in the outermost
	// decorator layer applied by wrapOverride).
	ctx.Set(override.KeyExtra, "prefer record types")

	out := fn(ctx)
	if out.Err != nil {
		t.Fatalf("dispatch: %v", out.Err)
	}
	if !strings.Contains(claudeFake.calls[0].Prompt, "prefer record types") {
		t.Errorf("prompt missing override extra:\n%s", claudeFake.calls[0].Prompt)
	}
}

func TestClaudeRunDispatch_ReplaceOverrideShortCircuitsTemplate(t *testing.T) {
	// --replace swaps the entire prompt. The templated launch line must
	// be absent and only the operator-supplied text reaches the runner.
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaa\n"),
			[]byte("bbbb\n"),
			[]byte("subject\n"),
		},
	}
	claudeFake := &fakeClaude{}
	fn := buildEngine(t, newDriverOpts(clauderun.Deps{Claude: claudeFake, Git: gitFake}))

	ctx := newCtxWithIssue()
	custom := "do something completely different"
	ctx.Set(override.KeyReplace, custom)

	if out := fn(ctx); out.Err != nil {
		t.Fatalf("dispatch: %v", out.Err)
	}
	got := claudeFake.calls[0].Prompt
	if got != custom {
		t.Errorf("prompt: got %q, want %q", got, custom)
	}
	if strings.Contains(got, "Launch the atdd-test subagent") {
		t.Errorf("templated prompt leaked through --replace:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// Manual fallback
// ---------------------------------------------------------------------------

func TestManualAgents_PausesAndAdvancesOnEnter(t *testing.T) {
	// In manual mode the dispatcher must NOT shell out to clauderun.
	// We verify by giving it a fake that fails if called.
	claudeFake := &fakeClaude{err: errors.New("clauderun must not run in manual mode")}
	gitFake := &fakeGit{}

	opts := Options{
		ManualAgents:  true,
		ClaudeRunDeps: clauderun.Deps{Claude: claudeFake, Git: gitFake},
		Stdout:        io.Discard,
		Stderr:        io.Discard,
		Stdin:         strings.NewReader("\n"), // operator presses Enter
	}
	fn := buildEngine(t, opts)

	if out := fn(newCtxWithIssue()); out.Err != nil {
		t.Fatalf("dispatch: %v", out.Err)
	}
	if len(claudeFake.calls) != 0 {
		t.Errorf("--manual-agents must not invoke clauderun, got %d calls", len(claudeFake.calls))
	}
}

func TestManualAgents_AbortHaltsRun(t *testing.T) {
	opts := Options{
		ManualAgents: true,
		Stdout:       io.Discard,
		Stderr:       io.Discard,
		Stdin:        strings.NewReader("abort\n"),
	}
	fn := buildEngine(t, opts)

	out := fn(newCtxWithIssue())
	if out.Err == nil {
		t.Fatalf("expected abort to halt, got nil error")
	}
	if !strings.Contains(out.Err.Error(), "aborted") {
		t.Errorf("error wording: got %q", out.Err.Error())
	}
}
