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
	"bytes"
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
// returns a canned error.
type fakeClaude struct {
	calls []clauderun.RunOpts
	err   error
}

func (f *fakeClaude) Run(_ context.Context, opts clauderun.RunOpts) (clauderun.RunResult, error) {
	f.calls = append(f.calls, opts)
	return clauderun.RunResult{}, f.err
}

// fakeGit serves canned outputs. The HEAD rev-parse and log calls
// consume the `out` FIFO. Snapshot calls (rev-parse --abbrev-ref HEAD,
// status --porcelain) get sensible defaults so existing tests don't
// have to enumerate them — the dispatcher's branch-switch /
// stranded-untracked detection (clauderun item 2) is exercised in
// clauderun's own test suite, not here.
type fakeGit struct {
	out [][]byte
	err error
}

func (f *fakeGit) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	if len(args) >= 3 && args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
		return []byte("main\n"), nil
	}
	if len(args) >= 2 && args[0] == "status" && args[1] == "--porcelain" {
		return []byte(""), nil
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

// templatedYAML mirrors the structural_cycle's parameterised user_task: the
// agent / phase_doc / description fields all carry ${…} placeholders that
// only resolve once Context.Params is populated by the calling
// call_activity. The dispatcher must expand these before printing them or
// passing them to clauderun.
const templatedYAML = `
flows:
  main:
    start: START
    nodes:
      - id: START
        type: start_event
      - id: STRUCT_WRITE
        type: user_task
        agent: ${agent}
        description: ${change_type} - WRITE
        phase_doc: ${phase_doc}
      - id: END
        type: end_event
    sequence_flows:
      - { from: START, to: STRUCT_WRITE }
      - { from: STRUCT_WRITE, to: END }
`

// buildEngine returns a freshly-bound engine + the wrapped NodeFn for
// AT_RED_TEST. Callers supply fakes via opts.ClaudeRunDeps. Verification /
// override decorators are intentionally NOT applied — those layers have
// their own tests; this fixture targets the agent-dispatch wiring alone.
func buildEngine(t *testing.T, opts Options) statemachine.NodeFn {
	t.Helper()
	return buildEngineFrom(t, opts, minimalYAML, "AT_RED_TEST")
}

// buildEngineFrom is the parameterisable form: it loads the supplied YAML
// and returns the wrapped NodeFn for the named node. Used by the templated
// regression cases that need a node whose agent: is a ${…} placeholder.
func buildEngineFrom(t *testing.T, opts Options, yamlSrc, nodeID string) statemachine.NodeFn {
	t.Helper()
	eng, err := statemachine.LoadBytes([]byte(yamlSrc))
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
	return eng.Flows["main"].Nodes[nodeID].Fn
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

func TestClaudeRunDispatch_AdvancesOnCleanExit(t *testing.T) {
	// Subprocess exits zero. clauderun no longer commits, so HEAD is
	// unchanged and the dispatcher just hands control back to the engine.
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaaaaa1\n"), // pre rev-parse HEAD
			[]byte("aaaaaaa1\n"), // post rev-parse HEAD (same)
		},
	}
	claudeFake := &fakeClaude{}
	fn := buildEngine(t, newDriverOpts(clauderun.Deps{Claude: claudeFake, Git: gitFake}))

	out := fn(newCtxWithIssue())
	if out.Err != nil {
		t.Fatalf("dispatch: %v", out.Err)
	}
	if len(claudeFake.calls) != 1 {
		t.Fatalf("expected 1 claude call, got %d", len(claudeFake.calls))
	}
	got := claudeFake.calls[0].Prompt
	// Prompt should be the embedded agent's body with ${name} placeholders
	// substituted from ticket context; v2 has no parent-claude wrapper.
	if !strings.Contains(got, "You are the Test Agent") {
		t.Errorf("prompt missing agent identity line")
	}
	if !strings.Contains(got, "#42") || !strings.Contains(got, "optivem/shop") {
		t.Errorf("prompt missing ticket context")
	}
	if !strings.Contains(got, "docs/atdd/process/at-red-test.md") {
		t.Errorf("prompt missing phase doc")
	}
	if strings.Contains(got, "${") {
		t.Errorf("prompt still contains ${...} placeholder")
	}
}

// ---------------------------------------------------------------------------
// Default dispatch — failure paths
// ---------------------------------------------------------------------------

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
			[]byte("aaaa\n"),
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
	// --replace swaps the entire prompt. The embedded agent body must be
	// absent and only the operator-supplied text reaches the runner.
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaa\n"),
			[]byte("aaaa\n"),
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
	if strings.Contains(got, "You are the Test Agent") {
		t.Errorf("embedded agent body leaked through --replace")
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

// ---------------------------------------------------------------------------
// Pre-flight (clauderun item 3)
// ---------------------------------------------------------------------------

func TestPreflightSkippedUnderManualAgents(t *testing.T) {
	// Pre-flight must NOT run when --manual-agents is set: the v1
	// fallback doesn't need the CLI, and forcing a `claude --version`
	// would defeat the purpose of the operator's escape hatch.
	called := false
	prev := preflightFn
	t.Cleanup(func() { preflightFn = prev })
	preflightFn = func(ctx context.Context) error {
		called = true
		return errors.New("should not be called")
	}

	// Run will fail on YAML load (we're not in a consumer repo), but we
	// only care that pre-flight wasn't invoked first.
	_ = Run(context.Background(), Options{
		ManualAgents: true,
		YAMLPath:     "/nonexistent.yaml",
		Stdout:       io.Discard,
		Stderr:       io.Discard,
		Stdin:        strings.NewReader(""),
	})
	if called {
		t.Errorf("preflightFn must not run under --manual-agents")
	}
}

func TestPreflightFailureSurfacesEarly(t *testing.T) {
	// When pre-flight fails, Run must return its error before doing any
	// flow-walking work. We assert by setting a non-existent YAMLPath:
	// if pre-flight didn't short-circuit, we'd see a YAML-load error.
	prev := preflightFn
	t.Cleanup(func() { preflightFn = prev })
	preflightFn = func(ctx context.Context) error {
		return errors.New("claude CLI pre-flight failed: not on PATH")
	}

	err := Run(context.Background(), Options{
		ManualAgents: false,
		YAMLPath:     "/nonexistent.yaml",
		Stdout:       io.Discard,
		Stderr:       io.Discard,
		Stdin:        strings.NewReader(""),
	})
	if err == nil {
		t.Fatalf("expected pre-flight error, got nil")
	}
	if !strings.Contains(err.Error(), "pre-flight failed") {
		t.Errorf("error should surface pre-flight wording, got %q", err.Error())
	}
	if strings.Contains(err.Error(), "load YAML") {
		t.Errorf("YAML load must not run when pre-flight fails: %q", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Templated dispatch — ${agent} / ${change_type} / ${phase_doc} expansion
// ---------------------------------------------------------------------------

func TestClaudeRunDispatch_ExpandsTemplatedNodeFields(t *testing.T) {
	// The structural_cycle reuses one set of YAML nodes across
	// SYSTEM_INTERFACE_REDESIGN_CYCLE / CHORE_CYCLE by injecting
	// ${agent} / ${change_type} / ${phase_doc} via
	// call_activity params. The dispatcher must resolve raw.Agent before
	// looking up the embedded prompt — otherwise it would try to load a
	// prompt named "${agent}", which doesn't exist.
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaaaaa1\n"),
			[]byte("aaaaaaa1\n"),
		},
	}
	claudeFake := &fakeClaude{}
	fn := buildEngineFrom(t, newDriverOpts(clauderun.Deps{Claude: claudeFake, Git: gitFake}),
		templatedYAML, "STRUCT_WRITE")

	ctx := newCtxWithIssue()
	ctx.Params = map[string]string{
		"agent":       "atdd-task",
		"change_type": "SYSTEM UI REDESIGN",
		"phase_doc":   "docs/atdd/process/sysui-redesign.md",
	}

	out := fn(ctx)
	if out.Err != nil {
		t.Fatalf("dispatch: %v", out.Err)
	}
	if len(claudeFake.calls) != 1 {
		t.Fatalf("expected 1 claude call, got %d", len(claudeFake.calls))
	}
	prompt := claudeFake.calls[0].Prompt
	if strings.Contains(prompt, "${") {
		t.Errorf("prompt still contains ${...} placeholder")
	}
	if !strings.Contains(prompt, "You are the Task Agent") {
		t.Errorf("prompt missing expanded agent identity line (atdd-task → Task Agent)")
	}
	if !strings.Contains(prompt, "docs/atdd/process/sysui-redesign.md") {
		t.Errorf("prompt missing expanded phase_doc")
	}
	if !strings.Contains(prompt, "SYSTEM UI REDESIGN - WRITE") {
		t.Errorf("prompt missing expanded phase description")
	}
}

func TestManualAgents_BannerSubstitutesTemplatedFields(t *testing.T) {
	// The v1/manual fallback prints a "DISPATCH: <agent>" banner directly to
	// stdout. The templated structural_cycle exposed the same leak: operator
	// saw "DISPATCH: ${agent}" instead of the substituted name.
	var buf bytes.Buffer
	opts := Options{
		ManualAgents: true,
		Stdout:       &buf,
		Stderr:       io.Discard,
		Stdin:        strings.NewReader("\n"),
	}
	fn := buildEngineFrom(t, opts, templatedYAML, "STRUCT_WRITE")

	ctx := newCtxWithIssue()
	ctx.Params = map[string]string{
		"agent":       "atdd-task",
		"change_type": "SYSTEM UI REDESIGN",
		"phase_doc":   "docs/atdd/process/sysui-redesign.md",
	}

	if out := fn(ctx); out.Err != nil {
		t.Fatalf("dispatch: %v", out.Err)
	}
	got := buf.String()
	if strings.Contains(got, "${") {
		t.Errorf("banner still contains ${...} placeholder:\n%s", got)
	}
	if !strings.Contains(got, "DISPATCH: atdd-task") {
		t.Errorf("banner missing expanded DISPATCH line:\n%s", got)
	}
	if !strings.Contains(got, "Launch the atdd-task agent") {
		t.Errorf("banner missing expanded launch line:\n%s", got)
	}
	if !strings.Contains(got, "docs/atdd/process/sysui-redesign.md") {
		t.Errorf("banner missing expanded phase doc:\n%s", got)
	}
}
