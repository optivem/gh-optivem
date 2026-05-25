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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/agents"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/clauderun"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/override"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakeClaude records each RunOpts so tests can assert prompt content and
// returns a canned error. resultText, when non-empty, becomes
// RunResult.ResultText so the dispatcher's `outputs:` parser sees a
// canned final-response body — used by the outputs-plumbing tests to
// drive ParseOutputs through the dispatcher seam.
type fakeClaude struct {
	calls      []clauderun.RunOpts
	err        error
	resultText string
}

func (f *fakeClaude) Run(_ context.Context, opts clauderun.RunOpts) (clauderun.RunResult, error) {
	f.calls = append(f.calls, opts)
	return clauderun.RunResult{ResultText: f.resultText}, f.err
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
processes:
  main:
    start: START
    nodes:
      - id: START
        type: start_event
      - id: AT_RED_TEST
        type: user_task
        agent: at-red-test
        documentation: Write the AT-RED scenario
      - id: END
        type: end_event
    sequence_flows:
      - { from: START, to: AT_RED_TEST }
      - { from: AT_RED_TEST, to: END }
`

// templatedYAML mirrors the structural_cycle's parameterised user_task: the
// agent / description fields carry ${…} placeholders that only resolve once
// Context.Params is populated by the calling call_activity. The dispatcher
// must expand these before printing them or passing them to clauderun.
const templatedYAML = `
processes:
  main:
    start: START
    nodes:
      - id: START
        type: start_event
      - id: WRITE
        type: user_task
        agent: ${agent}
        documentation: ${change_type} - WRITE
      - id: END
        type: end_event
    sequence_flows:
      - { from: START, to: WRITE }
      - { from: WRITE, to: END }
`

// buildEngine returns a freshly-bound engine + the wrapped NodeFn for
// AT_RED_TEST. Callers supply fakes via opts.ClaudeRunDeps. Verification /
// override decorators are intentionally NOT applied — those layers have
// their own tests; this fixture targets the agent-dispatch wiring alone.
func buildEngine(t *testing.T, opts Options) statemachine.NodeFn {
	t.Helper()
	return buildEngineFrom(t, opts, minimalYAML, "AT_RED_TEST", nil)
}

// buildEngineFrom is the parameterisable form: it loads the supplied YAML
// and returns the wrapped NodeFn for the named node. Used by the templated
// regression cases that need a node whose agent: is a ${…} placeholder.
// cfg, when non-nil, is threaded into wrapAgentDispatchers so the
// clauderun dispatcher receives a real PlaceholderMap — required by tests
// whose prompt body references Family B path placeholders.
func buildEngineFrom(t *testing.T, opts Options, yamlSrc, nodeID string, cfg *projectconfig.Config) statemachine.NodeFn {
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
	wrapAgentDispatchers(eng, opts, cfg, nil)
	return eng.Processes["main"].Nodes[nodeID].Fn
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
	c.Set("issue_url", "https://github.com/optivem/shop/issues/42")
	c.Set("issue_title", "Add PUT /carts/{id}/items endpoint")
	// Tests using atdd-{test,dsl,driver}-{at,ct} reference ${language} in
	// the language-equivalents pointer; seedScopeState would set it from
	// cfg in production. Seed a default here so test fixtures don't have
	// to thread the config through.
	c.Set("language", "java")
	// at-red-test references ${acceptance_criteria}; parseTicketBody would
	// set it from intake.Result.AcceptanceCriteria.Body in production. Seed
	// a default here so dispatch fixtures don't have to thread a parsed
	// ticket body through.
	c.Set("ticket_acceptance_criteria", "Scenario: placeholder\n  Given x\n  When y\n  Then z")
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
	if !strings.Contains(got, "#42") || !strings.Contains(got, "Add PUT") {
		t.Errorf("prompt missing ticket context")
	}
	if strings.Contains(got, "Phase doc:") {
		t.Errorf("prompt still carries a `Phase doc:` line; the field was dropped")
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
	// node_replacements swap: the entire prompt is replaced. The embedded
	// agent body must be absent and only the operator-supplied text
	// (sourced from gh-optivem.yaml's node_replacements: file body via
	// override.Hooks.Replace) reaches the runner.
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

func TestManualAgents_PausesAndAdvancesOnYes(t *testing.T) {
	// In manual mode the dispatcher must NOT shell out to clauderun.
	// We verify by giving it a fake that fails if called.
	claudeFake := &fakeClaude{err: errors.New("clauderun must not run in manual mode")}
	gitFake := &fakeGit{}

	opts := Options{
		ManualAgents:  true,
		ClaudeRunDeps: clauderun.Deps{Claude: claudeFake, Git: gitFake},
		Stdout:        io.Discard,
		Stderr:        io.Discard,
		Stdin:         strings.NewReader("y\n"), // operator approves
	}
	fn := buildEngine(t, opts)

	if out := fn(newCtxWithIssue()); out.Err != nil {
		t.Fatalf("dispatch: %v", out.Err)
	}
	if len(claudeFake.calls) != 0 {
		t.Errorf("--manual-agents must not invoke clauderun, got %d calls", len(claudeFake.calls))
	}
}

func TestManualAgents_NoHaltsRun(t *testing.T) {
	// Operator declines the dispatch prompt → driver returns "aborted" error.
	// The legacy `abort` verb is gone; explicit `n` is the halt signal now,
	// matching every other y/n prompt in the CLI.
	opts := Options{
		ManualAgents: true,
		Stdout:       io.Discard,
		Stderr:       io.Discard,
		Stdin:        strings.NewReader("n\n"),
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
// Templated dispatch — ${agent} / ${change_type} expansion
// ---------------------------------------------------------------------------

func TestClaudeRunDispatch_ExpandsTemplatedNodeFields(t *testing.T) {
	// The structural_cycle reuses one set of YAML nodes across
	// SYSTEM_INTERFACE_REDESIGN_CYCLE / SYSTEM_IMPLEMENTATION_REFACTORING_CYCLE by injecting
	// ${agent} / ${change_type} via call_activity params. The dispatcher
	// must resolve raw.Agent before looking up the embedded prompt —
	// otherwise it would try to load a prompt named "${agent}", which
	// doesn't exist.
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaaaaa1\n"),
			[]byte("aaaaaaa1\n"),
		},
	}
	claudeFake := &fakeClaude{}
	// task-system-interface-redesign's prompt now inlines phase-doc
	// placeholders (${sut_namespace}, ${driver-adapter}, ${driver-port},
	// ${system_test_path}); a cfg with populated Paths is required so
	// the dispatcher's PlaceholderMap fills them.
	cfg := &projectconfig.Config{
		System: projectconfig.System{
			Architecture: projectconfig.ArchMonolith,
			Path:         "system/monolith/typescript",
			Repo:         "optivem/shop",
			Lang:         projectconfig.LangTypescript,
		},
		SystemTest: projectconfig.TierSpec{
			Path:  "system-test/typescript",
			Repo:  "optivem/shop",
			Lang:  projectconfig.LangTypescript,
			Paths: projectconfig.DefaultPaths(projectconfig.LangTypescript, "system-test", "shop"),
		},
	}
	fn := buildEngineFrom(t, newDriverOpts(clauderun.Deps{Claude: claudeFake, Git: gitFake}),
		templatedYAML, "WRITE", cfg)

	ctx := newCtxWithIssue()
	ctx.Params = map[string]string{
		"agent":       "task-system-interface-redesign",
		"change_type": "SYSTEM UI REDESIGN",
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
		t.Errorf("prompt missing expanded agent identity line (task-system-interface-redesign → Task Agent)")
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
		Stdin:        strings.NewReader("y\n"),
	}
	// Manual mode short-circuits clauderun.Dispatch — no PlaceholderMap
	// pull-through, so the cfg arg stays nil.
	fn := buildEngineFrom(t, opts, templatedYAML, "WRITE", nil)

	ctx := newCtxWithIssue()
	ctx.Params = map[string]string{
		"agent":       "task-system-interface-redesign",
		"change_type": "SYSTEM UI REDESIGN",
	}

	if out := fn(ctx); out.Err != nil {
		t.Fatalf("dispatch: %v", out.Err)
	}
	got := buf.String()
	if strings.Contains(got, "${") {
		t.Errorf("banner still contains ${...} placeholder:\n%s", got)
	}
	if !strings.Contains(got, "DISPATCH: task-system-interface-redesign") {
		t.Errorf("banner missing expanded DISPATCH line:\n%s", got)
	}
	if !strings.Contains(got, "Launch the task-system-interface-redesign agent") {
		t.Errorf("banner missing expanded launch line:\n%s", got)
	}
	if strings.Contains(got, "Phase doc:") {
		t.Errorf("banner still carries a `Phase doc:` line; the field was dropped:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// --log-file mirroring (installLogFileMirror)
// ---------------------------------------------------------------------------

func TestInstallLogFileMirror_TeesStdoutAndStderrToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.log")

	var stdout, stderr bytes.Buffer
	opts := Options{
		Stdout:  &stdout,
		Stderr:  &stderr,
		LogFile: path,
	}
	closeFn, err := installLogFileMirror(&opts)
	if err != nil {
		t.Fatalf("installLogFileMirror: %v", err)
	}
	defer closeFn()

	io.WriteString(opts.Stdout, "stdout-line\n")
	io.WriteString(opts.Stderr, "stderr-line\n")

	// Close the file before reading so the buffered bytes flush.
	closeFn()

	// Live streams still got the bytes — file mirroring must not steal
	// the operator's view.
	if got := stdout.String(); got != "stdout-line\n" {
		t.Errorf("stdout buffer = %q, want %q", got, "stdout-line\n")
	}
	if got := stderr.String(); got != "stderr-line\n" {
		t.Errorf("stderr buffer = %q, want %q", got, "stderr-line\n")
	}

	// File got both streams in source order.
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	want := "stdout-line\nstderr-line\n"
	if got := string(body); got != want {
		t.Errorf("log file body = %q, want %q", got, want)
	}
}

func TestInstallLogFileMirror_EmptyPathIsNoOp(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	origStdout := stdout
	origStderr := stderr
	opts := Options{
		Stdout:  stdout,
		Stderr:  stderr,
		LogFile: "",
	}
	closeFn, err := installLogFileMirror(&opts)
	if err != nil {
		t.Fatalf("installLogFileMirror: %v", err)
	}
	defer closeFn()

	if opts.Stdout != origStdout {
		t.Errorf("Stdout was wrapped despite empty LogFile")
	}
	if opts.Stderr != origStderr {
		t.Errorf("Stderr was wrapped despite empty LogFile")
	}
}

func TestInstallLogFileMirror_OpenFailureReturnsError(t *testing.T) {
	// A path inside a non-existent parent dir is the typo case --log-file
	// is supposed to surface up-front.
	opts := Options{
		Stdout:  io.Discard,
		Stderr:  io.Discard,
		LogFile: filepath.Join(t.TempDir(), "no", "such", "dir", "run.log"),
	}
	closeFn, err := installLogFileMirror(&opts)
	defer closeFn()
	if err == nil {
		t.Fatal("expected error for unreachable log path, got nil")
	}
}

// ---------------------------------------------------------------------------
// End-to-end prompt substitution regression (item 6)
// ---------------------------------------------------------------------------

// TestEndToEnd_SubstitutionAndPromptLog drives a fake clauderun.Options
// build through the same seedScopeState + newClaudeRunDispatcher path
// production uses, with a fake runner that captures the prompt argument.
// Asserts the captured prompt contains the substituted scope values
// (Architecture, AllowedRoots system+test+external lines) and that the
// per-dispatch prompt log file was written byte-for-byte. This pins down
// the seedScopeParams → seedScopeState fix end-to-end: a regression that
// re-introduced the wrong-map bug would here surface as an unsubstituted
// `${architecture}` placeholder instead of the literal "monolith".
func TestEndToEnd_SubstitutionAndPromptLog(t *testing.T) {
	tmpRepo := t.TempDir()

	cfg := &projectconfig.Config{
		RepoStrategy: projectconfig.RepoStrategyMonoRepo,
		System: projectconfig.System{
			Architecture: projectconfig.ArchMonolith,
			Path:         "system/monolith/typescript",
			Repo:         "optivem/shop",
			Lang:         projectconfig.LangTypescript,
		},
		SystemTest: projectconfig.TierSpec{
			Path: "system-test/typescript",
			Repo: "optivem/shop",
			Lang: projectconfig.LangTypescript,
			// Family B paths feed the dispatcher's PlaceholderMap so inlined
			// phase-doc references in task-system-interface-redesign's body
			// (${driver-port}, ${driver-adapter}, …) resolve at render time.
			Paths: projectconfig.DefaultPaths(projectconfig.LangTypescript, "system-test", "shop"),
		},
		ExternalSystems: projectconfig.ExternalSystems{
			Stubs:      projectconfig.ExternalSpec{Path: "external-systems/stubs", Repo: "optivem/shop"},
			Simulators: projectconfig.ExternalSpec{Path: "external-systems/simulators", Repo: "optivem/shop"},
		},
	}

	sCtx := newCtxWithIssue()
	seedScopeState(sCtx, cfg)

	rs := &runState{runTimestamp: "20260505-150000", repoPath: tmpRepo}

	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaaaaa1\n"),
			[]byte("aaaaaaa1\n"),
		},
	}
	claudeFake := &fakeClaude{}
	opts := newDriverOpts(clauderun.Deps{Claude: claudeFake, Git: gitFake})
	opts.RepoPath = tmpRepo

	// minimalYAML's user_task uses agent: at-red-test, but the prompt-
	// substitution failure mode is most visible on agents whose prompt
	// body references ${architecture} / ${allowed_roots}
	// (task-*). Use a YAML variant with the system
	// task agent so wrapAgentDispatchers picks the right closure on
	// first walk.
	yamlSrc := strings.Replace(minimalYAML, "agent: at-red-test", "agent: task-system-interface-redesign", 1)

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
	wrapAgentDispatchers(eng, opts, cfg, rs)
	fn := eng.Processes["main"].Nodes["AT_RED_TEST"].Fn

	out := fn(sCtx)
	if out.Err != nil {
		t.Fatalf("dispatch: %v", out.Err)
	}
	if len(claudeFake.calls) != 1 {
		t.Fatalf("expected 1 claude call, got %d", len(claudeFake.calls))
	}
	prompt := claudeFake.calls[0].Prompt

	// Substitution assertions — these would fail if scope params were
	// being written to Context.Params (the original bug) instead of
	// Context.State (the fix). Empty Architecture would render as
	// "Architecture: " and the AllowedRoots block would be absent.
	mustContainHere(t, prompt, "Architecture: monolith")
	mustContainHere(t, prompt, "- System: system/monolith/typescript (lang: typescript)")
	mustContainHere(t, prompt, "- System tests: system-test/typescript (lang: typescript)")
	mustContainHere(t, prompt, "- Stubs: external-systems/stubs")
	mustContainHere(t, prompt, "- Simulators: external-systems/simulators")
	if strings.Contains(prompt, "${") {
		t.Errorf("prompt still contains ${...} placeholder:\n%s", prompt)
	}

	// Bonus: the log file path is composed deterministically from
	// runState. Read it back and compare byte-for-byte against the
	// captured prompt — this pins down item 2 (PromptLogPath plumbing)
	// alongside item 1 (the substitution fix).
	logPath := filepath.Join(tmpRepo, ".gh-optivem", "runs", "20260505-150000", "001-task-system-interface-redesign.prompt.md")
	body, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read prompt log: %v", err)
	}
	if string(body) != prompt {
		t.Errorf("log file does not match captured prompt byte-for-byte:\n got %d bytes\nwant %d bytes", len(body), len(prompt))
	}
}

func mustContainHere(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("missing %q in:\n%s", needle, haystack)
	}
}

// ---------------------------------------------------------------------------
// pruneOldRuns (item 2)
// ---------------------------------------------------------------------------

func TestPruneOldRuns_KeepsMostRecent(t *testing.T) {
	dir := t.TempDir()
	// Make 5 dirs with explicit increasing mtimes (oldest first).
	names := []string{"run-a", "run-b", "run-c", "run-d", "run-e"}
	for i, n := range names {
		p := filepath.Join(dir, n)
		if err := os.Mkdir(p, 0o755); err != nil {
			t.Fatal(err)
		}
		// Time deltas large enough not to collide on any FS — 1 day apart.
		ts := time.Date(2026, 1, 1+i, 0, 0, 0, 0, time.UTC)
		if err := os.Chtimes(p, ts, ts); err != nil {
			t.Fatal(err)
		}
	}

	if err := pruneOldRuns(dir, 3); err != nil {
		t.Fatalf("pruneOldRuns: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	gotNames := make(map[string]bool)
	for _, e := range entries {
		gotNames[e.Name()] = true
	}
	// Keep N=3 means keep N-1=2 entries (room for the run we're about
	// to create). Two newest are run-d and run-e.
	want := map[string]bool{"run-d": true, "run-e": true}
	if len(gotNames) != len(want) {
		t.Fatalf("expected %d dirs after prune, got %d: %v", len(want), len(gotNames), gotNames)
	}
	for n := range want {
		if !gotNames[n] {
			t.Errorf("expected %q to remain, missing", n)
		}
	}
}

func TestPruneOldRuns_NoOpOnMissingDir(t *testing.T) {
	if err := pruneOldRuns(filepath.Join(t.TempDir(), "no", "such", "runs"), 5); err != nil {
		t.Errorf("missing runs/ should be a no-op, got %v", err)
	}
}

func TestPruneOldRuns_ZeroKeepIsNoOp(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := pruneOldRuns(dir, 0); err != nil {
		t.Fatalf("pruneOldRuns: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "old")); err != nil {
		t.Errorf("KeepRuns=0 must skip pruning, but old/ was removed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// runState.promptLogPath
// ---------------------------------------------------------------------------

func TestRunState_PromptLogPathSequencesPerDispatch(t *testing.T) {
	rs := &runState{runTimestamp: "20260505-150000", repoPath: "/tmp/repo"}
	got := []string{
		rs.promptLogPath("task"),
		rs.promptLogPath("at-red-test"),
		rs.promptLogPath("task"),
	}
	want := []string{
		filepath.Join("/tmp/repo", ".gh-optivem", "runs", "20260505-150000", "001-task.prompt.md"),
		filepath.Join("/tmp/repo", ".gh-optivem", "runs", "20260505-150000", "002-at-red-test.prompt.md"),
		filepath.Join("/tmp/repo", ".gh-optivem", "runs", "20260505-150000", "003-task.prompt.md"),
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("got[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestRunState_PromptLogPathNilIsEmpty(t *testing.T) {
	var rs *runState
	if got := rs.promptLogPath("task"); got != "" {
		t.Errorf("nil runState should return empty path, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Agent outputs → ctx.State plumbing
// ---------------------------------------------------------------------------
//
// These tests cover the end-to-end seam introduced by plan
// 20260520-1945-user-task-output-context-plumbing.md: the WRITE agent's
// `outputs:` YAML block in its final response text is parsed by the
// user_task dispatcher and flattened into ctx.State so downstream
// gates / actions (run_targeted_tests, scope_exception_requested, …)
// see the populated values.
//
// The fake claude returns a canned ResultText with the YAML block; the
// dispatcher's call to clauderun.ParseOutputs runs against it and we
// assert ctx.State after dispatch.

func TestClaudeRunDispatch_OutputsBlockPopulatesContext(t *testing.T) {
	// Agent emits both test_names and suite under outputs:. The dispatcher
	// must coerce test_names to []string and write both keys so the
	// downstream RUN action's `ctx.State[CtxKeyTestNames].([]string)`
	// assertion succeeds.
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaaaaa1\n"),
			[]byte("aaaaaaa1\n"),
		},
	}
	claudeFake := &fakeClaude{
		resultText: "I authored the failing tests.\n\n" +
			"```yaml\n" +
			"outputs:\n" +
			"  test_names:\n" +
			"    - shouldRegisterCustomer\n" +
			"    - shouldRejectDuplicateCustomer\n" +
			"  suite: <acceptance-api>\n" +
			"```\n",
	}
	fn := buildEngine(t, newDriverOpts(clauderun.Deps{Claude: claudeFake, Git: gitFake}))

	ctx := newCtxWithIssue()
	out := fn(ctx)
	if out.Err != nil {
		t.Fatalf("dispatch: %v", out.Err)
	}

	names, ok := ctx.Get("test_names").([]string)
	if !ok {
		t.Fatalf("ctx.test_names: want []string, got %T (%v)", ctx.Get("test_names"), ctx.Get("test_names"))
	}
	wantNames := []string{"shouldRegisterCustomer", "shouldRejectDuplicateCustomer"}
	if len(names) != len(wantNames) {
		t.Fatalf("test_names: got %v, want %v", names, wantNames)
	}
	for i, w := range wantNames {
		if names[i] != w {
			t.Errorf("test_names[%d]: got %q, want %q", i, names[i], w)
		}
	}
	if got := ctx.GetString("suite"); got != "<acceptance-api>" {
		t.Errorf("ctx.suite: got %q, want <acceptance-api>", got)
	}
}

func TestClaudeRunDispatch_ScopeExceptionFlattensIntoContext(t *testing.T) {
	// The scope_exception envelope (per internal/assets/runtime/shared/
	// scope.md) flattens to scope_exception_files ([]string) and
	// scope_exception_reason (string) — exactly the keys
	// gates.scopeExceptionRequested reads via ctx.Get(...).([]string).
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaaaaa1\n"),
			[]byte("aaaaaaa1\n"),
		},
	}
	claudeFake := &fakeClaude{
		resultText: "```yaml\n" +
			"scope_exception:\n" +
			"  files:\n" +
			"    - internal/shared/clock.go\n" +
			"  reason: depends on a clock helper outside scope\n" +
			"```\n",
	}
	fn := buildEngine(t, newDriverOpts(clauderun.Deps{Claude: claudeFake, Git: gitFake}))

	ctx := newCtxWithIssue()
	out := fn(ctx)
	if out.Err != nil {
		t.Fatalf("dispatch: %v", out.Err)
	}

	files, ok := ctx.Get("scope_exception_files").([]string)
	if !ok {
		t.Fatalf("ctx.scope_exception_files: want []string, got %T", ctx.Get("scope_exception_files"))
	}
	if len(files) != 1 || files[0] != "internal/shared/clock.go" {
		t.Errorf("scope_exception_files: got %v", files)
	}
	if got := ctx.GetString("scope_exception_reason"); got != "depends on a clock helper outside scope" {
		t.Errorf("scope_exception_reason: got %q", got)
	}
}

func TestClaudeRunDispatch_MalformedOutputsBlockFailsLoud(t *testing.T) {
	// A fenced block that starts with `outputs:` but contains broken YAML
	// must surface a clear error rather than silently zeroing state. The
	// dispatcher routes the parser's error as Outcome.Err so the cycle
	// stops at the user_task boundary.
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaaaaa1\n"),
			[]byte("aaaaaaa1\n"),
		},
	}
	claudeFake := &fakeClaude{
		resultText: "```yaml\n" +
			"outputs:\n" +
			"  test_names: [unterminated\n" +
			"```\n",
	}
	fn := buildEngine(t, newDriverOpts(clauderun.Deps{Claude: claudeFake, Git: gitFake}))

	out := fn(newCtxWithIssue())
	if out.Err == nil {
		t.Fatalf("expected error for malformed outputs block, got nil")
	}
	if !strings.Contains(out.Err.Error(), "parse outputs") {
		t.Errorf("error wording should surface parse-outputs context, got %q", out.Err.Error())
	}
}

func TestClaudeRunDispatch_MissingOutputsBlockIsNoOp(t *testing.T) {
	// Agents that have nothing to emit (or pre-amendment prompts) leave
	// the block out entirely. The dispatcher must NOT fail — the
	// downstream consumer's existing "not set in Context" error path
	// still surfaces the missing key, which is the same behaviour as
	// before structured output was wired up.
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaaaaa1\n"),
			[]byte("aaaaaaa1\n"),
		},
	}
	claudeFake := &fakeClaude{resultText: "Done. No structured output today."}
	fn := buildEngine(t, newDriverOpts(clauderun.Deps{Claude: claudeFake, Git: gitFake}))

	ctx := newCtxWithIssue()
	out := fn(ctx)
	if out.Err != nil {
		t.Fatalf("dispatch: %v", out.Err)
	}
	if v := ctx.Get("test_names"); v != nil {
		t.Errorf("test_names should be unset, got %v", v)
	}
}
