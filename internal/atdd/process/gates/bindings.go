// Bindings — Go implementations of every gateway `binding:` referenced in
// internal/atdd/process/process-flow.yaml.
//
// Each binding is a `statemachine.NodeFn` that:
//
//  1. Reads the live Context for a pre-set value under the binding key. If
//     an upstream service task or intake agent has already declared the
//     result (e.g. classify_ticket_type sets ticket_type, run_smoke_test sets
//     smoke_test_passes), that value is returned verbatim. This is the
//     common path in production runs and lets transitions tests seed state
//     directly without touching shell-outs.
//
//  2. Falls back to a forward-looking user prompt when the value is absent.
//     Gates after a WRITE phase ("did the DSL interface change?") are
//     questions only the user can answer in v1 — git-diff inspection is a
//     v2 candidate. Prompt strings come from the YAML node descriptions so
//     the engine and the diagram stay in sync.
//
// Tests substitute fake Prompter / GhRunner / GitRunner implementations via
// Deps; production callers pass a Deps zero-value and the package falls back
// to real stdin / `gh` / `git`.
package gates

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/optivem/gh-optivem/internal/kernel/approval"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/tracker"
	trackergithub "github.com/optivem/gh-optivem/internal/atdd/runtime/tracker/github"
	"github.com/optivem/gh-optivem/internal/kernel/promptio"
)

// Deps bundles the side-effecting collaborators every binding may need.
// All fields are optional — a zero-value Deps falls back to real shell-outs
// and the OS stdin/stdout. Tests pass non-nil fakes for hermeticity.
type Deps struct {
	Gh         GhRunner
	Git        GitRunner
	Prompter   Prompter
	ProjectURL string // optional — explicit override for tracker construction
	// Tracker is the seam tracker-shaped gate logic goes through (today
	// just legacyAcceptanceCriteriaSectionPresent's body fetch).
	// Optional — withDefaults constructs a github adapter from ProjectURL
	// + Gh when unset. Tests inject fakes the same way as actions: set Gh
	// to a canned-response runner, leave Tracker nil.
	Tracker tracker.Tracker
	// Approval is the resolved auto-approve policy. Bindings that prompt
	// operator-skippable decisions (refactor-type-choice) take their
	// natural default under --auto instead of stalling on stdin.
	// Zero value (Auto=false) preserves prompt-always semantics, so
	// tests that don't set Approval keep their existing behaviour.
	Approval approval.Resolved
}

// Prompter asks the user one yes/no/value question and returns the trimmed
// reply. Implementations must surface I/O errors rather than silently
// returning the empty string — empty replies have meaning ("user pressed
// Enter") that the caller may want to interpret.
type Prompter interface {
	Ask(prompt string) (string, error)
}

// GhRunner runs the `gh` CLI. The default implementation is execGh.
type GhRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// GitRunner runs the `git` CLI. The default implementation is execGit.
type GitRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// withDefaults populates any nil collaborator with its real exec / stdin
// counterpart. Returns a copy so tests that pass a partial Deps observe the
// real defaults for unset fields rather than getting silently nil-Run errors.
func (d Deps) withDefaults() Deps {
	if d.Gh == nil {
		d.Gh = execGh{}
	}
	if d.Git == nil {
		d.Git = execGit{}
	}
	if d.Prompter == nil {
		d.Prompter = stdinPrompter{}
	}
	if d.Tracker == nil {
		url := d.ProjectURL
		if url == "" {
			// Placeholder — body ops only need Issue.URL, so any valid
			// project URL keeps github.New from rejecting the call. Matches
			// the actions package fallback so test wiring is symmetric.
			url = "https://github.com/orgs/placeholder/projects/0"
		}
		if t, err := trackergithub.New(url, ghAdapter{d.Gh}); err == nil {
			d.Tracker = t
		}
	}
	return d
}

// ghAdapter shims gates.GhRunner into trackergithub.GhRunner. Both
// interfaces have the same shape; the adapter exists because Go's
// structural typing collapses the conversion at a single point rather
// than scattering it.
type ghAdapter struct{ inner GhRunner }

func (g ghAdapter) Run(ctx context.Context, args ...string) ([]byte, error) {
	return g.inner.Run(ctx, args...)
}

// RegisterAll wires every YAML binding name to its Go implementation under
// the supplied registry. Call this once during driver startup before
// engine.Bind so the engine's GateFn lookup hits a populated registry.
//
// Listed in YAML order so a reader can scan top-to-bottom and confirm every
// `binding:` has a matching entry. Adding a new gate = one line here, one
// new method on bindings, plus a transitions-test case.
func RegisterAll(r *Registry, deps Deps) {
	deps = deps.withDefaults()
	b := bindings{deps: deps}
	// Phase-scope enforcement (per plan 20260518-1144 items 5, 6):
	//   - Layer 1: scope-exception-requested reads the agent's structured
	//     COMMIT output for a scope_exception block; true → STOP_SCOPE_VIOLATION.
	//   - Layer 2: phase-scope-clean runs after the agent commits; reads the
	//     check-phase-scope action's structured result; false → STOP_SCOPE_VIOLATION.
	r.Register("scope-exception-requested", b.scopeExceptionRequested)
	r.Register("phase-scope-clean", b.phaseScopeClean)
	// Post-RED-DSL flag-presence validation (per plan 20260518-1144 item 4):
	// the two RED-DSL phase-output flags (system_driver_interface_changed,
	// external_system_driver_interface_changed) MUST be explicitly emitted
	// by the agent's COMMIT output. Treats unset as an error — the
	// downstream GATE_EXT_AT / GATE_SYS_AT gates rely on a real yes|no,
	// not a default no.
	r.Register("dsl-flags-present", b.dslFlagsPresent)
	// BPMN Phase D bindings (per
	// plans/20260525-2348-bpmn-phase-d-bindings.md). Kebab-case matches
	// the YAML `binding:` references; the ctx state/params keys the
	// bodies read are also kebab (agent-emitted outputs, call-site
	// params).
	r.Register("command-succeeded", b.commandSucceeded)
	r.Register("test-outcome", b.testOutcome)
	// verify-tests-pass no-progress gateway (plan 20260615-1845 Step 4).
	// Reads the bool the check-fix-progress action stamps after a failing
	// test run; true re-dispatches the fixer, false halts at
	// FIX_LOOP_NO_PROGRESS. Strict on unset (the action runs immediately
	// upstream).
	r.Register("fix-loop-progressing", b.fixLoopProgressing)
	r.Register("expected-test-result", b.expectedTestResult)
	r.Register("fix-on-failure-enabled", b.fixOnFailureEnabled)
	// Cascade-namespaced port-changed verdicts (plan 20260606-1525). The
	// landing layer (validate-outputs-and-scopes) writes the agent's bare
	// `*-port-changed` output under an `at-`/`ct-` key chosen by the active
	// `tests` cascade, so the nested contract excursion can't clobber the
	// acceptance cascade's verdict. Each gateway is cascade-fixed, so each
	// reads its own namespaced key. Only the verdicts actually gated are
	// registered: `ct-system-driver-port-changed` / `ct-external-driver-
	// port-changed` are written but read directly by the CT-path fence /
	// never gated, so they need no binding.
	r.Register("at-dsl-port-changed", b.atDslPortChanged)
	r.Register("at-system-driver-port-changed", b.atSystemDriverPortChanged)
	r.Register("at-external-driver-port-changed", b.atExternalDriverPortChanged)
	r.Register("ct-dsl-port-changed", b.ctDslPortChanged)
	// Cover-path mode-aware acceptance-test verify gate + its case-D
	// terminal-verify gate (plan 20260606-1518). The shared verify gate
	// routes on at-verify-expectation so the cover path expects AT green
	// only when the plumbing it needs is complete; the change path (red /
	// unset verify-mode) is unchanged.
	r.Register("at-verify-expectation", b.atVerifyExpectation)
	r.Register("at-external-terminal-verify-needed", b.atExternalTerminalVerifyNeeded)
	// CT-HIGH real-side fork (plan 20260606-1356). Routes contract-real on
	// the active clone's real-kind. The value is copied into state by the
	// upstream resolve-external-system ACTION from the clone's baked real-kind
	// param (plan 20260615-0755); this gate is a pure state-reader, like
	// expectedTestResult / testOutcome.
	r.Register("real-kind", b.realKind)
	// Per-external-system clone guard (plan 20260615-0755). Each unrolled
	// external-system contract-cycle clone is guarded by this gate so it runs
	// iff its baked external-system-name is in the names-set the port change
	// touched. resolve-external-system stamps the bool (it has Config to derive
	// the names-set); this gate is a pure state-reader.
	r.Register("external-system-touched", b.externalSystemTouched)
	// Per-channel clone guard (plan 20260619-1139). Each unrolled per-channel
	// system / system-driver-adapter clone is guarded by this gate so it runs iff
	// at least one of the ticket's acceptance tests registered for its baked
	// channel. resolve-channel stamps the bool (it reads the RED acceptance run's
	// on-disk acceptance-<ch> report); this gate is a pure state-reader, the exact
	// mirror of external-system-touched.
	r.Register("channel-touched", b.channelTouched)
	r.Register("refactor-type-choice", b.refactorTypeChoice)
	r.Register("approval-outcome", b.approvalOutcome)
	r.Register("outputs-and-scopes-valid", b.outputsAndScopesValid)
	// Two-axis implement-ticket gateway (Item 11): ticketKind resolves the
	// kind (story | bug | task) at GATE_TICKET_KIND; taskSubtype resolves
	// the subtype (legacy-coverage | system-redesign | …) at
	// GATE_TASK_SUBTYPE, reached only when ticketKind emits bare `task`.
	// Both resolve from the tracker (Classify / Subtypes respectively).
	r.Register("ticket-kind", b.ticketKind)
	r.Register("task-subtype", b.taskSubtype)
}

// bindings is a thin closure-receiver so each method has access to deps
// without taking it as a parameter. Keeping the methods pure-as-possible
// (only Context + deps) lets the test suite exercise them through the same
// NodeFn contract the engine sees.
type bindings struct {
	deps Deps
}

// issueFromContext builds a tracker.Issue from the conventional Context
// keys driver.preResolveIssue writes (issue-num, issue-url, issue-title,
// issue-handle). issue-url is the addressable form every Tracker call
// site needs; callers that don't seed it get a clear error rather than
// a downstream parse failure.
func issueFromContext(ctx *statemachine.Context) (tracker.Issue, error) {
	url := ctx.GetString("issue-url")
	if url == "" {
		return tracker.Issue{}, fmt.Errorf("issue-url not in Context")
	}
	return tracker.Issue{
		ID:     ctx.GetString("issue-num"),
		Title:  ctx.GetString("issue-title"),
		URL:    url,
		Handle: ctx.GetString("issue-handle"),
	}, nil
}

// scopeExceptionRequested is Layer 1 of phase-scope enforcement (per plan
// 20260518-1144 item 6): the agent-triggered escape hatch. The agent
// invokes `gh optivem output write scope-exception-files=... \
// scope-exception-reason=...` when it cannot complete the phase within
// `scope:`; validate-outputs-and-scopes reads the per-dispatch JSONL
// file and populates ctx[scope-exception-files] ([]string) and
// ctx[scope-exception-reason] (string). This binding returns true when
// scope-exception-files is non-empty, routing the cycle to
// STOP_SCOPE_VIOLATION (skipping DISABLE / Layer 2 / COMMIT).
//
// No prompt fallback: the gate fires after validate-outputs-and-scopes;
// an absent value means "no exception was signalled" and routes to the
// normal continuation. The shape contract (kebab keys flattened from
// JSONL by the validator) lives in internal/atdd/assets/runtime/shared/scope.md.
func (b bindings) scopeExceptionRequested(ctx *statemachine.Context) statemachine.Outcome {
	files, _ := ctx.Get("scope-exception-files").([]string)
	return statemachine.Outcome{Bool: len(files) > 0}
}

// dslFlagsPresent is the flag-presence-validation gateway sitting
// between the parameterized implement-dsl phase and the existing
// GATE_EXT_AT in at_cycle (plan 20260518-1144 item 4). implement-dsl
// must emit BOTH phase-output flags per implement-dsl.md — "unset is a
// bug, not a default no". This gate returns true only when BOTH state
// keys are explicitly present; otherwise the cycle routes to
// STOP_FLAG_UNSET and loops back to implement-dsl for the agent to set
// them.
//
// No prompt fallback: a missing value is the bug this gate exists to
// catch, so coercing it to "no" via prompt would silently route the
// cycle past the regression. Presence is checked directly against
// ctx.State so that "unset" and "answered no" stay distinct.
func (b bindings) dslFlagsPresent(ctx *statemachine.Context) statemachine.Outcome {
	_, sys := ctx.State["system_driver_interface_changed"]
	_, ext := ctx.State["external_system_driver_interface_changed"]
	return statemachine.Outcome{Bool: sys && ext}
}

// phaseScopeClean is Layer 2 of phase-scope enforcement (per plan
// 20260518-1144 item 5, retargeted at process-flow.yaml node scope per
// plan 20260526-1536): the post-phase scripted check. The
// check_phase_scope action diffs the working tree against the
// writing-agent MID's inline `write:` scope (joined with
// gh-optivem.yaml paths:) and writes the boolean result to
// ctx[phase-scope-clean] plus the violating paths to
// ctx[phase-scope-violating-paths] for the STOP_SCOPE_VIOLATION payload.
// This binding returns the boolean verbatim; true → continue to COMMIT,
// false → STOP_SCOPE_VIOLATION.
//
// No prompt fallback: the gate fires after the action that stamps the
// flag, so the value must be set; reaching the gate with an unset value
// is a bug, not a hand-debugging affordance.
func (b bindings) phaseScopeClean(ctx *statemachine.Context) statemachine.Outcome {
	v, ok := ctx.State["phase-scope-clean"]
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("phase-scope-clean: not set in Context — check_phase_scope action did not run")}
	}
	return outcomeFromBoolish(v)
}

// ---------------------------------------------------------------------------
// BPMN Phase D bindings (plans/20260525-2348-bpmn-phase-d-bindings.md)
// ---------------------------------------------------------------------------
//
// All keys read/written below are kebab-case to match the YAML
// `binding:` references, the YAML `params:` block keys, and the
// agent-emitted output keys flattened into ctx.State by
// validate-outputs-and-scopes (per-dispatch JSONL channel; see plan
// 20260526-2118).

// commandSucceeded is the LOW `execute-command` primitive's
// GATE_COMMAND_SUCCEEDED. Reads the boolean run-command stamped into
// ctx.State["command-succeeded"]; missing value is a wiring bug (the
// action MUST have run upstream).
func (b bindings) commandSucceeded(ctx *statemachine.Context) statemachine.Outcome {
	v, ok := ctx.State["command-succeeded"]
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("command-succeeded: not set in Context — run-command action did not run")}
	}
	return outcomeFromBoolish(v)
}

// testOutcome routes verify-tests-pass / verify-tests-fail on the
// per-suite pass|fail|infra value run-command stamps after a
// `gh optivem run-tests` invocation. Halt on unset (the gate fires
// immediately after RUN_TESTS, so the action's stamp is required) and
// halt on any value outside {pass, fail, infra} so a future action
// contract drift surfaces loudly instead of silently mis-routing.
//
// The "infra" value signals "the runner could not start" (binary
// missing, docker down, permission denied — see
// internal/atdd/process/actions/verify_classify.go). Both verify-tests-*
// processes route it to TESTS_INFRA_HALT — neither the pass nor fail
// fixer is appropriate because the runner produced no test report.
func (b bindings) testOutcome(ctx *statemachine.Context) statemachine.Outcome {
	v, ok := ctx.State["test-outcome"]
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("test-outcome: not set in Context — run-command did not classify (only `gh optivem run-tests` stamps test-outcome)")}
	}
	s, ok := v.(string)
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("test-outcome: %T, want string", v)}
	}
	switch s {
	case "pass", "fail", "infra":
		return statemachine.Outcome{Value: s}
	default:
		return statemachine.Outcome{Err: fmt.Errorf("test-outcome: unrecognised value %q (action stamped a value the gate does not handle)", s)}
	}
}

// fixLoopProgressing is the verify-tests-pass no-progress gateway
// (GATE_FIX_PROGRESSING, plan 20260615-1845 Step 4). The check-fix-progress
// action stamps ctx.State["fix-loop-progressing"] from a per-pass failure
// fingerprint: true when the failure changed (or this is the loop's first
// fail), false when two consecutive failing runs are byte-identical (the
// fixer is spinning). True → re-dispatch the fixer; false →
// FIX_LOOP_NO_PROGRESS. Strict on unset — the action runs immediately
// upstream on the fail branch, so a missing value is a wiring bug, not a
// default. Layers under the FIX node's max-visits count cap: this gate
// catches a spinning fixer earlier and more precisely than the count.
func (b bindings) fixLoopProgressing(ctx *statemachine.Context) statemachine.Outcome {
	v, ok := ctx.State["fix-loop-progressing"]
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("fix-loop-progressing: not set in Context — check-fix-progress action did not run")}
	}
	return outcomeFromBoolish(v)
}

// expectedTestResult is the implement-test-layer fork gate. The
// pinned-result wrapper passes `expected-test-result: success` or
// `failure` via call-activity params (and parameterised callers forward
// `${expected-test-result}`). Reads ctx.Params verbatim — this is
// structural metadata of the call site, not a runtime decision the
// operator re-makes — and halts on empty (the caller forgot to pin
// the param). No prompt fallback.
func (b bindings) expectedTestResult(ctx *statemachine.Context) statemachine.Outcome {
	v := strings.TrimSpace(ctx.Params["expected-test-result"])
	if v == "" {
		return statemachine.Outcome{Err: fmt.Errorf("expected-test-result: call-activity param not set — caller must pin `expected-test-result: success` or `failure`")}
	}
	return statemachine.Outcome{Value: v}
}

// fixOnFailureEnabled gates the LOW primitives' failure → FIX edges.
// Used by both `execute-agent` (validation-failure branch) and
// `execute-command` (command-failure branch). Reads the
// `fix-on-failure` call-activity param: the `fix` primitive's
// recursive `execute-agent` call sets it to "false" (single-attempt
// remediation, no further recursion); `run-tests` sets it to "false"
// on its `execute-command` call so verify-tests-pass / verify-tests-
// fail can own failure routing via `test-outcome` instead of being
// pre-empted by the inner FIX branch. The default (missing/empty) is
// true — every other caller wants fix dispatch on failure. Coerces
// via promptio.ParseYN so "true"/"false"/"yes"/"no" all round-trip.
func (b bindings) fixOnFailureEnabled(ctx *statemachine.Context) statemachine.Outcome {
	raw := strings.TrimSpace(ctx.Params["fix-on-failure"])
	if raw == "" {
		return statemachine.Outcome{Bool: true}
	}
	yes, ok := promptio.ParseYN(raw)
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("fix-on-failure-enabled: unrecognised value %q (expected true|false|yes|no)", raw)}
	}
	return statemachine.Outcome{Bool: yes}
}

// atDslPortChanged, atSystemDriverPortChanged, atExternalDriverPortChanged,
// ctDslPortChanged are the writing-agent output flags consumed by the
// per-test-layer fanout in implement-and-verify-dsl and the parent re-gates.
// The agent emits the bare key via `gh optivem output write KEY=VAL`; the
// landing layer (validate-outputs-and-scopes) flattens it from the
// per-dispatch JSONL file into the cascade-namespaced `at-`/`ct-` ctx key
// (plan 20260606-1525), so the nested contract excursion can't overwrite the
// acceptance cascade's verdict. Each gateway is cascade-fixed and reads its
// own namespaced key. Missing/unset is a bug — the writing-agent MID's BPMN
// `outputs:` list MUST declare the key as required, so unset means the agent
// skipped the `output write` call — and we halt rather than mis-route (same
// doctrine as the older dslFlagsPresent gate).
func (b bindings) atDslPortChanged(ctx *statemachine.Context) statemachine.Outcome {
	return boolStateGate(ctx, "at-dsl-port-changed")
}

func (b bindings) atSystemDriverPortChanged(ctx *statemachine.Context) statemachine.Outcome {
	return boolStateGate(ctx, "at-system-driver-port-changed")
}

func (b bindings) atExternalDriverPortChanged(ctx *statemachine.Context) statemachine.Outcome {
	return boolStateGate(ctx, "at-external-driver-port-changed")
}

func (b bindings) ctDslPortChanged(ctx *statemachine.Context) statemachine.Outcome {
	return boolStateGate(ctx, "ct-dsl-port-changed")
}

// atVerifyExpectation is the mode-aware acceptance-test verify gate (plan
// 20260606-1518). The shared verify gate in
// write-and-verify-acceptance-test-code and implement-test-layer routes on
// this binding instead of the raw expected-test-result, so the cover path can
// expect the AT to PASS only when the test plumbing it needs is complete.
//
//   - verify-mode == "green-when-complete" (cover path): the system already
//     has the behaviour, so the AT is expected to PASS exactly when no further
//     plumbing change is pending and FAIL otherwise. "Pending" is scoped to
//     this layer by verify-pending-on (which ports' changes still imply a
//     downstream plumbing layer): dsl → at-dsl-port-changed (test-code layer);
//     drivers → at-system-driver-port-changed || at-external-driver-port-changed
//     (DSL layer); none → terminal layer, never pending (always PASS).
//   - verify-mode == "red" or unset (change path, --target, no-arg full run):
//     routes on the caller-pinned expected-test-result verbatim — the change
//     cascade is uniformly red and greened by implement-and-verify-system,
//     exactly as before this plan.
//   - verify-mode == "none" (compile-only layer, plan 20260606-2330): the
//     caller skips the suite-polarity assertion entirely; the gate returns
//     "none" and the process routes straight to COMMIT_LAYER, so neither
//     VERIFY_TESTS_* node runs and expected-test-result is never consulted.
//     Used by the CT-HIGH DSL/port step, whose contract-suite red/green is
//     owned solely by PROBE_CONTRACT_REAL post-adapters — the DSL layer only
//     changes the port, ensures it compiles, and commits.
//
// Returns success|failure|none so the existing verify-gate edges (now
// `at-verify-expectation == success|failure|none`) and their
// UNKNOWN_EXPECTED_TEST_RESULT catch-all are unchanged.
func (b bindings) atVerifyExpectation(ctx *statemachine.Context) statemachine.Outcome {
	if strings.TrimSpace(ctx.Params["verify-mode"]) == "none" {
		return statemachine.Outcome{Value: "none"}
	}
	if strings.TrimSpace(ctx.Params["verify-mode"]) == "green-when-complete" {
		pending, err := plumbingPending(ctx)
		if err != nil {
			return statemachine.Outcome{Err: err}
		}
		if pending {
			return statemachine.Outcome{Value: "failure"}
		}
		return statemachine.Outcome{Value: "success"}
	}
	// red / unset: unchanged — route on the caller-pinned expected result.
	return b.expectedTestResult(ctx)
}

// plumbingPending reports whether a further test-plumbing layer must still run
// after the current one, scoped by the verify-pending-on call param (plan
// 20260606-1518). The flags are the acceptance-cascade-namespaced verdicts
// (at-*, plan 20260606-1525); each is strict (the writer/impl that runs
// immediately before this gate must have emitted it) so an unset flag halts
// rather than defaulting to "not pending" and mis-routing a still-red AT into
// verify-pass.
func plumbingPending(ctx *statemachine.Context) (bool, error) {
	switch on := strings.TrimSpace(ctx.Params["verify-pending-on"]); on {
	case "dsl":
		return boolState(ctx, "at-dsl-port-changed")
	case "drivers":
		sys, err := boolState(ctx, "at-system-driver-port-changed")
		if err != nil {
			return false, err
		}
		ext, err := boolState(ctx, "at-external-driver-port-changed")
		if err != nil {
			return false, err
		}
		return sys || ext, nil
	case "none":
		return false, nil
	case "":
		return false, fmt.Errorf("at-verify-expectation: verify-pending-on not set on the green-when-complete path — the caller must pin dsl|drivers|none")
	default:
		return false, fmt.Errorf("at-verify-expectation: unknown verify-pending-on %q (expected dsl|drivers|none)", on)
	}
}

// atExternalTerminalVerifyNeeded gates the cover path's terminal AT-green
// assertion on the external-driver-only branch (plan 20260606-1518, case D).
// The external CT-HIGH verifies the contract tests (real/stub), not the AT, so
// on the cover path the AT would otherwise end last-verified as FAILING at the
// DSL layer. This gate fires a trailing AT-pass verify exactly when the run is
// green-when-complete AND no system-driver adapter step follows (that step,
// when present, owns the terminal PASS instead). Reached only from the
// external-driver-port-changed == true branch, so at-external is implied true.
func (b bindings) atExternalTerminalVerifyNeeded(ctx *statemachine.Context) statemachine.Outcome {
	if strings.TrimSpace(ctx.Params["verify-mode"]) != "green-when-complete" {
		return statemachine.Outcome{Bool: false}
	}
	sys, err := boolState(ctx, "at-system-driver-port-changed")
	if err != nil {
		return statemachine.Outcome{Err: err}
	}
	return statemachine.Outcome{Bool: !sys}
}

// boolState reads a strict boolean ctx.State flag for the plumbing-pending
// derivation: a missing key halts (same doctrine as boolStateGate — the
// writer/impl that precedes the gate must have emitted it).
func boolState(ctx *statemachine.Context, key string) (bool, error) {
	v, ok := ctx.State[key]
	if !ok {
		return false, fmt.Errorf("%s: not set in Context — the agent's `outputs:` block must emit %q (unset is a bug, not a default no)", key, key)
	}
	out := outcomeFromBoolish(v)
	if out.Err != nil {
		return false, out.Err
	}
	return out.Bool, nil
}

// realKind is the CT-HIGH real-side fork gate (plan 20260606-1356). It
// routes contract-real on the active clone's real-kind:
// `test-instance` collapses to a single pass-verify (the vendor sandbox
// already honors the contract); `simulator` takes the red→implement→green
// branch. The value is baked into each clone at load by UnrollExternalSystems
// (from cfg.ExternalSystems[name].RealKind) and copied into ctx.State by the
// upstream resolve-external-system action (plan 20260615-0755), so this gate is
// a pure state-reader — halt on unset (resolve must have run) and on any
// value outside the closed enum so a config drift surfaces loudly instead of
// mis-routing.
func (b bindings) realKind(ctx *statemachine.Context) statemachine.Outcome {
	v, ok := ctx.State["real-kind"]
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("real-kind: not set in Context — resolve-external-system action must run before the gate")}
	}
	s, ok := v.(string)
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("real-kind: %T, want string", v)}
	}
	switch s {
	case "test-instance", "simulator":
		return statemachine.Outcome{Value: s}
	default:
		return statemachine.Outcome{Err: fmt.Errorf("real-kind: unrecognised value %q (resolve-external-system copied a value the gate does not handle; expected test-instance | simulator)", s)}
	}
}

// externalSystemTouched is the per-clone entry guard for the unrolled
// external-system contract cycle (plan 20260615-0755). resolve-external-system
// stamps ctx.State["external-system-touched"] = (this clone's baked name ∈ the
// port-change names-set) at the start of every clone; this gate reads it back
// and routes the clone into the cycle (true) or past it to the skip end-event
// (false). Strict — a missing key means resolve-external-system did not run
// before the gate, which is a wiring bug, not a default no.
func (b bindings) externalSystemTouched(ctx *statemachine.Context) statemachine.Outcome {
	return boolStateGate(ctx, "external-system-touched")
}

// channelTouched is the per-clone entry guard for the unrolled per-channel
// system / system-driver-adapter cycle (plan 20260619-1139). resolve-channel
// stamps ctx.State["channel-touched"] = (≥1 of the ticket's acceptance tests
// registered for this clone's baked channel) at the start of every clone; this
// gate reads it back and routes the clone into the cycle (true) or past it to
// the skip end-event (false). Strict — a missing key means resolve-channel did
// not run before the gate, a wiring bug, not a default no. The exact mirror of
// externalSystemTouched.
func (b bindings) channelTouched(ctx *statemachine.Context) statemachine.Outcome {
	return boolStateGate(ctx, "channel-touched")
}

// boolStateGate is the shared body of the three driver-port-changed
// gates. Strict — missing key halts (the agent's `outputs:` block MUST
// emit the flag explicitly), value type-flexible (outcomeFromBoolish
// accepts bool, "true"/"false", "yes"/"no").
func boolStateGate(ctx *statemachine.Context, key string) statemachine.Outcome {
	v, ok := ctx.State[key]
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("%s: not set in Context — the agent's `outputs:` block must emit %q (unset is a bug, not a default no)", key, key)}
	}
	return outcomeFromBoolish(v)
}

// refactorTypeChoice prompts the operator for the loopable refactor
// menu (TOP `refactor` and the opportunistic-refactor branch inside
// `change-system-behavior`). Preseed via ctx.State to short-circuit
// the prompt for hand-debug or transitions tests; under --auto, the
// menu's natural default (none) is taken without prompting so an
// autonomous run does not stall on stdin. Otherwise delegate to
// promptio.SelectOneOfVia: bare Enter → none, unrecognised reply
// → re-prompt (matching the y/n loop in ConfirmYNVia).
func (b bindings) refactorTypeChoice(ctx *statemachine.Context) statemachine.Outcome {
	if v := ctx.GetString("refactor-type-choice"); v != "" {
		return statemachine.Outcome{Value: v}
	}
	if b.deps.Approval.Auto {
		return statemachine.Outcome{Value: "none"}
	}
	allowed := []string{
		"refactor-system-structure",
		"refactor-test-structure",
		"redesign-system-structure",
		"redesign-external-system-structure",
		"none",
	}
	answer, err := promptio.SelectOneOfVia(
		b.deps.Prompter,
		os.Stderr,
		"Refactor type? (refactor-system-structure | refactor-test-structure | redesign-system-structure | redesign-external-system-structure | none) [none]: ",
		allowed,
		"none",
	)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("refactor-type-choice: %w", err)}
	}
	return statemachine.Outcome{Value: answer}
}

// approvalOutcome reads the value newApproveDispatcher (driver.go)
// writes when the operator answers ASK_HUMAN inside the LOW `approve`
// primitive. Range: approved | rejected. The dispatcher writes one of
// the two on every approve invocation, so missing is a wiring bug
// (the dispatcher was not installed for this approve process) — halt
// rather than route silently.
func (b bindings) approvalOutcome(ctx *statemachine.Context) statemachine.Outcome {
	v := ctx.GetString("approval-outcome")
	switch v {
	case "approved", "rejected":
		return statemachine.Outcome{Value: v}
	case "":
		return statemachine.Outcome{Err: fmt.Errorf("approval-outcome: not set in Context — approve dispatcher must have run before the gate")}
	default:
		return statemachine.Outcome{Err: fmt.Errorf("approval-outcome: unrecognised value %q (dispatcher stamped a value the gate does not handle)", v)}
	}
}

// outputsAndScopesValid reads the boolean validate-outputs-and-scopes
// stamped after the LOW `execute-agent` primitive's RUN_AGENT. The
// action MUST run before the gate, so unset is a wiring bug.
func (b bindings) outputsAndScopesValid(ctx *statemachine.Context) statemachine.Outcome {
	v, ok := ctx.State["outputs-and-scopes-valid"]
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("outputs-and-scopes-valid: not set in Context — validate-outputs-and-scopes action did not run")}
	}
	return outcomeFromBoolish(v)
}

// ticketKind is the TOP `implement-ticket` first-level
// (GATE_TICKET_KIND) dispatch discriminator. It resolves only the
// *kind* axis from Tracker.Classify; for task tickets the subtype is
// resolved one axis down by the taskSubtype binding
// (GATE_TASK_SUBTYPE). Lookup table:
//
//	ticket-type | discriminator
//	---         | ---
//	story       | story
//	bug         | bug
//	task        | task   (subtype resolved on the downstream GATE_TASK_SUBTYPE axis)
//
// Preseed via ctx.State["ticket-kind"] short-circuits the
// classification (hand-debug / transitions tests). Tracker.Classify
// returns the GitHub native issue type; ticketKindAliases is the same
// "feature → story" normalization the older read_ticket_type action
// applies.
func (b bindings) ticketKind(ctx *statemachine.Context) statemachine.Outcome {
	if v := ctx.GetString("ticket-kind"); v != "" {
		return statemachine.Outcome{Value: v}
	}
	issue, err := issueFromContext(ctx)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("ticket-kind: %w", err)}
	}
	kind, confident, err := b.deps.Tracker.Classify(context.Background(), issue)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("ticket-kind: %w", err)}
	}
	if !confident || kind == "" {
		return statemachine.Outcome{Err: fmt.Errorf("ticket-kind: issue %s has no native issue type — set one of: Feature, Bug, Task (and for Task, also exactly one subtype:* label: %s) and re-run", issue.ID, strings.Join(taskSubtypes, ", "))}
	}
	if alias, ok := ticketKindAliases[kind]; ok {
		kind = alias
	}
	switch kind {
	case "story", "bug":
		return statemachine.Outcome{Value: kind}
	case "task":
		// Subtype resolution lives on the downstream GATE_TASK_SUBTYPE
		// axis (taskSubtype binding) — this gateway only discriminates
		// the kind, so it emits bare `task`.
		return statemachine.Outcome{Value: "task"}
	default:
		return statemachine.Outcome{Err: fmt.Errorf("ticket-kind: unsupported ticket type %q (expected one of: story, bug, task)", kind)}
	}
}

// taskSubtype is the TOP `implement-ticket` second-level
// (GATE_TASK_SUBTYPE) gateway, reached only after ticketKind emits
// bare `task`. It resolves the subtype axis from Tracker.Subtypes —
// the structural mirror of ticketKind's kind resolution. Exactly one
// `subtype:*` label is expected; zero, multiple, or an out-of-set
// label is unrecognised composition and surfaces a clear operator
// error so the ticket can be re-labelled and re-run.
//
// Preseed via ctx.State["task-subtype"] short-circuits the resolution
// (hand-debug / transitions tests), matching ticketKind's preseed
// affordance.
func (b bindings) taskSubtype(ctx *statemachine.Context) statemachine.Outcome {
	if v := ctx.GetString("task-subtype"); v != "" {
		return statemachine.Outcome{Value: v}
	}
	issue, err := issueFromContext(ctx)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("task-subtype: %w", err)}
	}
	subs, err := b.deps.Tracker.Subtypes(context.Background(), issue)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("task-subtype: %w", err)}
	}
	if len(subs) != 1 {
		return statemachine.Outcome{Err: fmt.Errorf("task-subtype: task %s has %d subtype:* labels (want exactly one of: %s)", issue.ID, len(subs), strings.Join(taskSubtypes, ", "))}
	}
	sub := subs[0]
	if !taskSubtypeSet[sub] {
		return statemachine.Outcome{Err: fmt.Errorf("task-subtype: task %s has unrecognised subtype:%s label (valid subtype:* labels are: %s)", issue.ID, sub, strings.Join(taskSubtypes, ", "))}
	}
	return statemachine.Outcome{Value: sub}
}

// ticketKindAliases mirrors the older read_ticket_type behaviour
// (actions/bindings.go ticketTypeAliases): GitHub's "Feature" native
// type is the new spelling of what the runtime calls "story".
var ticketKindAliases = map[string]string{"feature": "story"}

// taskSubtypes is the closed, canonically-ordered set of `subtype:*`
// labels the GATE_TASK_SUBTYPE axis (taskSubtype binding) dispatches
// on. Anything outside this set surfaces as unrecognised — the
// operator re-labels the ticket and re-runs. The order is the order
// shown in operator-facing error messages. (ticketKind's no-native-
// type error also lists it as the set of valid Task subtypes.)
var taskSubtypes = []string{
	"legacy-coverage",
	"system-redesign",
	"external-system-redesign",
	"system-refactor",
	"test-refactor",
}

// taskSubtypeSet is the O(1) membership view of taskSubtypes, built at
// package init. Keep the slice authoritative for ordering; the set is
// derived.
var taskSubtypeSet = func() map[string]bool {
	m := make(map[string]bool, len(taskSubtypes))
	for _, s := range taskSubtypes {
		m[s] = true
	}
	return m
}()

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// outcomeFromBoolish coerces an arbitrary Context value to an Outcome. We
// accept native bool, the strings "true"/"false"/"yes"/"no", and anything
// else that GetString-style coercion produces — robust to upstream service
// tasks that store the value in any of those forms.
func outcomeFromBoolish(v any) statemachine.Outcome {
	switch t := v.(type) {
	case bool:
		return statemachine.Outcome{Bool: t}
	case string:
		yes, ok := promptio.ParseYN(t)
		if !ok {
			// Treat the string as already-canonical; the engine writes
			// whatever it was set to back to State, so unrecognised text
			// surfaces upstream rather than silently flipping false.
			return statemachine.Outcome{Value: t}
		}
		return statemachine.Outcome{Bool: yes}
	default:
		return statemachine.Outcome{Bool: false}
	}
}

// ---------------------------------------------------------------------------
// Default exec runners + stdin prompter
// ---------------------------------------------------------------------------

type execGh struct{}

func (execGh) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out, fmt.Errorf("gh %s: %w (stderr: %s)",
				strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
		}
		return out, fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

type execGit struct{}

func (execGit) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out, fmt.Errorf("git %s: %w (stderr: %s)",
				strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
		}
		return out, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// stdinPrompter is the default Prompter — writes the prompt to stderr (so
// it does not contaminate any stdout-captured output downstream of the
// driver) and reads a single line from stdin.
type stdinPrompter struct{}

func (stdinPrompter) Ask(prompt string) (string, error) {
	if _, err := fmt.Fprint(os.Stderr, prompt); err != nil {
		return "", err
	}
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return line, nil
}
