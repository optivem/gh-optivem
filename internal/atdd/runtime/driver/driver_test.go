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
// START → user-task → END. Nothing in the engine cares about the surrounding
// edges or descriptions, but they're spelled out so the YAML parses cleanly.
//
// Post-plan-1701 split: the user-task's agent is the noun
// (acceptance-test-writer); the MID scope-defining process keeps the verb
// (write-acceptance-tests) as its process ID since engine.Scope is indexed
// by task-name.
const minimalYAML = `
processes:
  main:
    name: "Main"
    start: START
    nodes:
      - id: START
        type: start-event
        name: "Synthetic Test Event"
      - id: AT_RED_TEST
        type: user-task
        agent: acceptance-test-writer
        name: Write the AT-RED scenario
      - id: END
        type: end-event
        name: "Synthetic Test Event"
    sequence-flows:
      - { from: START, to: AT_RED_TEST }
      - { from: AT_RED_TEST, to: END }

  # MID-style scope-defining processes for the dispatcher's engine.Scope
  # lookup (plan 20260526-1448 Item 4 — the dispatcher keys off the
  # task-name to find per-phase read:/write: lists). Mirrors the
  # writing-agent MID convention from production process-flow.yaml. The
  # two below cover the two agents driver_test.go's fixtures dispatch
  # (acceptance-test-writer via the as-shipped minimalYAML, and
  # system-implementer via the agent-replacement in TestEndToEnd_*).
  write-acceptance-tests:
    name: "Write Acceptance Tests"
    start: EXECUTE_AGENT
    nodes:
      - id: EXECUTE_AGENT
        type: call-activity
        process: execute-agent
        name: "Dispatch the Agent"
        params:
          task-name: write-acceptance-tests
          agent: acceptance-test-writer
        read:  [at-test, dsl-port]
        write: [at-test, dsl-port, dsl-core]
      - id: END
        type: end-event
        name: "Synthetic Test End"
    sequence-flows:
      - { from: EXECUTE_AGENT, to: END }

  implement-system:
    name: "Implement System"
    start: EXECUTE_AGENT
    nodes:
      - id: EXECUTE_AGENT
        type: call-activity
        process: execute-agent
        name: "Dispatch the Agent"
        params:
          task-name: implement-system
          agent: system-implementer
        read:  [system-path]
        write: [system-path]
      - id: END
        type: end-event
        name: "Synthetic Test End"
    sequence-flows:
      - { from: EXECUTE_AGENT, to: END }
`

// templatedYAML mirrors the structural_cycle's parameterised user-task: the
// agent / description fields carry ${…} placeholders that only resolve once
// Context.Params is populated by the calling call-activity. The dispatcher
// must expand these before printing them or passing them to clauderun.
const templatedYAML = `
processes:
  main:
    name: "Main"
    start: START
    nodes:
      - id: START
        type: start-event
        name: "Synthetic Test Event"
      - id: WRITE
        type: user-task
        agent: ${agent}
        name: ${change_type} - WRITE
      - id: END
        type: end-event
        name: "Synthetic Test Event"
    sequence-flows:
      - { from: START, to: WRITE }
      - { from: WRITE, to: END }

  # MID-style scope-defining process for implement-system, looked up by
  # the dispatcher at runtime via engine.Scope(task-name). Mirrors the
  # production writing-agent MID convention. Used by tests that set
  # ctx.Params["agent"]     = "system-implementer" (noun, dispatch identity)
  # and ctx.Params["task-name"] = "implement-system" (verb, scope key) on
  # the main user-task. The plan-1701 split separated the two; the MID
  # process ID stays the task-name (verb) since that is what indexes Scope.
  implement-system:
    name: "Implement System"
    start: EXECUTE_AGENT
    nodes:
      - id: EXECUTE_AGENT
        type: call-activity
        process: execute-agent
        name: "Dispatch the Agent"
        params:
          task-name: implement-system
          agent: system-implementer
        read:  [system-path]
        write: [system-path]
      - id: END
        type: end-event
        name: "Synthetic Test End"
    sequence-flows:
      - { from: EXECUTE_AGENT, to: END }
`

// buildEngine returns a freshly-bound engine + the wrapped NodeFn for
// AT_RED_TEST. Callers supply fakes via opts.ClaudeRunDeps. Verification /
// override decorators are intentionally NOT applied — those layers have
// their own tests; this fixture targets the agent-dispatch wiring alone.
//
// A default monolith-java shop-shaped cfg is threaded through so
// ${scope-block} and inlined Family B / system-path placeholders (added
// to every prompt body by plan 20260526-1448 Item 4) resolve cleanly.
// Tests that need to exercise the nil-cfg path call buildEngineFrom
// directly with cfg=nil.
func buildEngine(t *testing.T, opts Options) statemachine.NodeFn {
	t.Helper()
	return buildEngineFrom(t, opts, minimalYAML, "AT_RED_TEST", defaultTestConfig())
}

// defaultTestConfig is the monolith-java shop-shaped projectconfig.Config
// the driver tests use to populate cfg.PlaceholderMap() with Family B path
// keys (at-test, dsl-port, …) + system-path. Production builds an
// equivalent cfg from gh-optivem.yaml; here we synthesise it inline so
// the test fixtures don't have to thread a real config through.
func defaultTestConfig() *projectconfig.Config {
	return &projectconfig.Config{
		RepoStrategy: projectconfig.RepoStrategyMonoRepo,
		System: projectconfig.System{
			Architecture: projectconfig.ArchMonolith,
			Path:         "system",
			Repo:         "optivem/shop",
			Lang:         projectconfig.LangJava,
		},
		SystemTest: projectconfig.TierSpec{
			Path:  "system-test",
			Repo:  "optivem/shop",
			Lang:  projectconfig.LangJava,
			Paths: projectconfig.DefaultPaths(projectconfig.LangJava, "system-test", "shop"),
		},
	}
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
	c.Set("issue-num", "42")
	c.Set("issue-url", "https://github.com/optivem/shop/issues/42")
	c.Set("issue-title", "Add PUT /carts/{id}/items endpoint")
	// Post-plan-1701 split: the dispatcher's scopeKey precedence is
	// originating-task-name → task-name → agent. minimalYAML's user-task
	// dispatches the acceptance-test-writer agent (noun) but the
	// MID-scope process is keyed by write-acceptance-tests (verb).
	// Seed task-name here so eng.Scope finds the read/write lists; tests
	// dispatching a different agent override task-name in their own ctx.
	c.Params["task-name"] = "write-acceptance-tests"
	// Tests using atdd-{test,dsl,driver}-{at,ct} reference ${language} in
	// the language-equivalents pointer; seedScopeState would set it from
	// cfg in production. Seed a default here so test fixtures don't have
	// to thread the config through.
	c.Set("language", "java")
	// write-acceptance-tests references ${acceptance-criteria}; parse-ticket would
	// set it from intake.Result.AcceptanceCriteria.Body in production. Seed
	// a default here so dispatch fixtures don't have to thread a parsed
	// ticket body through.
	c.Set("acceptance-criteria", "Scenario: placeholder\n  Given x\n  When y\n  Then z")
	// implement-system / refactor-system / refactor-tests / implement-system-driver-adapters
	// reference ${checklist} (load-bearing). parse-ticket would set this in
	// production; seed a default so structural-task dispatch fixtures don't
	// have to thread a parsed Checklist through.
	c.Set("checklist", "- [ ] Placeholder checklist item")
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
	if !strings.Contains(got, "The Acceptance Criteria below were parsed") {
		t.Errorf("prompt missing expected write-acceptance-tests body marker")
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
// Templated dispatch — ${agent} / ${change_type} expansion
// ---------------------------------------------------------------------------

func TestClaudeRunDispatch_ExpandsTemplatedNodeFields(t *testing.T) {
	// The structural_cycle reuses one set of YAML nodes across
	// SYSTEM_INTERFACE_REDESIGN_CYCLE / SYSTEM_IMPLEMENTATION_REFACTORING_CYCLE by injecting
	// ${agent} / ${change_type} via call-activity params. The dispatcher
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
	// implement-system's prompt now inlines phase-doc
	// placeholders (${sut-namespace}, ${driver-adapter}, ${driver-port},
	// ${system-test-path}); a cfg with populated Paths is required so
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
		"agent":       "system-implementer",
		"task-name":   "implement-system",
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
	if !strings.Contains(prompt, "implement-system task writes production code under the system surface") {
		t.Errorf("prompt missing expected implement-system body marker")
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
		"agent":       "system-implementer",
		"task-name":   "implement-system",
		"change_type": "SYSTEM UI REDESIGN",
	}

	if out := fn(ctx); out.Err != nil {
		t.Fatalf("dispatch: %v", out.Err)
	}
	got := buf.String()
	if strings.Contains(got, "${") {
		t.Errorf("banner still contains ${...} placeholder:\n%s", got)
	}
	if !strings.Contains(got, "DISPATCH: system-implementer") {
		t.Errorf("banner missing expanded DISPATCH line:\n%s", got)
	}
	if !strings.Contains(got, "Launch the system-implementer agent") {
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
// Asserts the captured prompt contains the substituted ${architecture}
// value AND the rendered ${scope-block} from the BPMN node's read:/write:
// lists (plan 20260526-1448 Item 4), and that the per-dispatch prompt log
// file was written byte-for-byte. A regression that broke either piece
// (the substitution fix or the engine.Scope lookup) would surface as an
// unsubstituted `${architecture}` / `${scope-block}` placeholder instead
// of the literal contents.
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
			// phase-doc references in implement-system's body
			// (${driver-port}, ${driver-adapter}, …) resolve at render time.
			Paths: projectconfig.DefaultPaths(projectconfig.LangTypescript, "system-test", "shop"),
		},
		ExternalSystems: projectconfig.ExternalSystems{
			Stubs:      projectconfig.ExternalSpec{Path: "external-systems/stubs", Repo: "optivem/shop"},
			Simulators: projectconfig.ExternalSpec{Path: "external-systems/simulators", Repo: "optivem/shop"},
		},
	}

	sCtx := newCtxWithIssue()
	// Override the default newCtxWithIssue task-name (verb) to match the
	// MID this test dispatches (implement-system, via the agent-replacement
	// below). The dispatcher's scopeKey precedence resolves to task-name
	// post-1701 split.
	sCtx.Params["task-name"] = "implement-system"
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

	// minimalYAML's user-task uses agent: acceptance-test-writer, but the
	// prompt-substitution failure mode is most visible on agents whose
	// prompt body references ${architecture} / ${scope-block}
	// (system-implementer). Use a YAML variant with the system implement
	// agent so wrapAgentDispatchers picks the right closure on
	// first walk.
	yamlSrc := strings.Replace(minimalYAML, "agent: acceptance-test-writer", "agent: system-implementer", 1)

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
	// "Architecture: " and the scope block would be absent.
	mustContainHere(t, prompt, "Architecture: monolith")
	// The ${scope-block} placeholder is rendered from minimalYAML's
	// EXECUTE_AGENT read:/write: list (system-path on both sides — the
	// implement-system MID node's scope). The renderer joins each key
	// against cfg.PlaceholderMap(), so the resolved path appears.
	mustContainHere(t, prompt, "You may **read** files under these paths:")
	mustContainHere(t, prompt, "You may **modify** files under these paths:")
	mustContainHere(t, prompt, "- `system-path`: system/monolith/typescript")
	mustContainHere(t, prompt, "`scope_exception`")
	if strings.Contains(prompt, "${") {
		t.Errorf("prompt still contains ${...} placeholder:\n%s", prompt)
	}

	// Bonus: the log file path is composed deterministically from
	// runState. Read it back and compare byte-for-byte against the
	// captured prompt — this pins down item 2 (PromptLogPath plumbing)
	// alongside item 1 (the substitution fix).
	logPath := filepath.Join(tmpRepo, ".gh-optivem", "runs", "20260505-150000", "001-system-implementer.prompt.md")
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
		rs.promptLogPath("acceptance-test-writer"),
		rs.promptLogPath("task"),
	}
	want := []string{
		filepath.Join("/tmp/repo", ".gh-optivem", "runs", "20260505-150000", "001-task.prompt.md"),
		filepath.Join("/tmp/repo", ".gh-optivem", "runs", "20260505-150000", "002-acceptance-test-writer.prompt.md"),
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
// Agent outputs flow through the per-dispatch JSONL channel since plan
// 20260526-2118 (GH_OPTIVEM_OUTPUT_FILE). Tests for the read/flatten side
// live in actions/bindings_test.go::TestValidateOutputsAndScopes_JSONL_*,
// not here — the dispatcher's job is just to set the env var and stash
// the path on ctx.State (the action does the parsing).

// ---------------------------------------------------------------------------
// approve dispatcher (BPMN Phase D Item 6, Q-D2)
// ---------------------------------------------------------------------------

// approveYAML is a stripped-down `approve` process — just the
// ASK_HUMAN user-task + an end event — used to verify
// wrapAgentDispatchers installs the approve-specific dispatcher (not
// the hard-halting human-stop one) for the ASK_HUMAN node. We omit
// the gateway here because Bind() requires a GateFn registry, which
// is out of scope for these tests — the dispatcher writes the state
// key directly, and the gateway is exercised by the gates package
// tests.
const approveYAML = `
processes:
  approve:
    name: "Approve"
    start: ASK_HUMAN
    nodes:
      - id: ASK_HUMAN
        type: user-task
        agent: human
        name: "Do you approve?"
      - id: APPROVE_END
        type: end-event
        name: "Synthetic Test Event"
    sequence-flows:
      - {from: ASK_HUMAN, to: APPROVE_END}
`

func TestApproveDispatcher_YesWritesApproved(t *testing.T) {
	opts := newDriverOpts(clauderun.Deps{})
	opts.Stdin = strings.NewReader("y\n")
	var stdout bytes.Buffer
	opts.Stdout = &stdout
	fn := buildEngineFromApprove(t, opts)

	ctx := statemachine.NewContext()
	out := fn(ctx)
	if out.Err != nil {
		t.Fatalf("approve YES should not surface err: %v", out.Err)
	}
	if got := ctx.GetString("approval-outcome"); got != "approved" {
		t.Fatalf("approval-outcome: got %q, want %q", got, "approved")
	}
}

func TestApproveDispatcher_NoWritesRejected_NoErr(t *testing.T) {
	// CRITICAL: this is the asymmetry from newHumanStopDispatcher.
	// approve's NO must NOT halt — the gateway routes the reject branch.
	opts := newDriverOpts(clauderun.Deps{})
	opts.Stdin = strings.NewReader("n\n")
	opts.Stdout = &bytes.Buffer{}
	fn := buildEngineFromApprove(t, opts)

	ctx := statemachine.NewContext()
	out := fn(ctx)
	if out.Err != nil {
		t.Fatalf("approve NO must route (not halt): %v", out.Err)
	}
	if got := ctx.GetString("approval-outcome"); got != "rejected" {
		t.Fatalf("approval-outcome: got %q, want %q", got, "rejected")
	}
}

func TestApproveDispatcher_QuestionExpandsParams(t *testing.T) {
	// The YAML's ${task-name} placeholder in the documentation field
	// must be resolved against ctx.Params before the prompt is printed.
	opts := newDriverOpts(clauderun.Deps{})
	opts.Stdin = strings.NewReader("y\n")
	var stdout bytes.Buffer
	opts.Stdout = &stdout
	yaml := strings.Replace(approveYAML, `"Do you approve?"`, `"Do you approve agent ${task-name} to execute?"`, 1)
	fn := buildEngineFromApproveYAML(t, opts, yaml)

	ctx := statemachine.NewContext()
	ctx.Params["task-name"] = "write-acceptance-tests"
	out := fn(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if !strings.Contains(stdout.String(), "agent write-acceptance-tests") {
		t.Fatalf("stdout missing expanded task-name: %q", stdout.String())
	}
}

// buildEngineFromApprove loads approveYAML, binds, wraps dispatchers,
// and returns the wrapped NodeFn for ASK_HUMAN — which must be the
// approve dispatcher (not the human-stop one), per the case ordering
// in wrapAgentDispatchers.
func buildEngineFromApprove(t *testing.T, opts Options) statemachine.NodeFn {
	t.Helper()
	return buildEngineFromApproveYAML(t, opts, approveYAML)
}

func buildEngineFromApproveYAML(t *testing.T, opts Options, yamlSrc string) statemachine.NodeFn {
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
	wrapAgentDispatchers(eng, opts, nil, nil)
	return eng.Processes["approve"].Nodes["ASK_HUMAN"].Fn
}

func TestClaudeRunDispatch_NoStructuredOutputIsNoOp(t *testing.T) {
	// Agents that emit no structured output via the JSONL channel must
	// still complete cleanly at the dispatcher level — downstream
	// validate-outputs-and-scopes is what fires missing-output (with
	// proper diagnostics) if required keys are absent.
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaaaaa1\n"),
			[]byte("aaaaaaa1\n"),
		},
	}
	claudeFake := &fakeClaude{}
	fn := buildEngine(t, newDriverOpts(clauderun.Deps{Claude: claudeFake, Git: gitFake}))

	ctx := newCtxWithIssue()
	out := fn(ctx)
	if out.Err != nil {
		t.Fatalf("dispatch: %v", out.Err)
	}
	if v := ctx.Get("test-names"); v != nil {
		t.Errorf("test-names should be unset, got %v", v)
	}
}
