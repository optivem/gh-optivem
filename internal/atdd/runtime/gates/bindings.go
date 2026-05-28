// Bindings — Go implementations of every gateway `binding:` referenced in
// internal/atdd/runtime/statemachine/process-flow.yaml.
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

	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/tracker"
	trackergithub "github.com/optivem/gh-optivem/internal/atdd/runtime/tracker/github"
	"github.com/optivem/gh-optivem/internal/promptio"
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
	r.Register("expected-test-result", b.expectedTestResult)
	r.Register("fix-on-failure-enabled", b.fixOnFailureEnabled)
	r.Register("dsl-port-changed", b.dslPortChanged)
	r.Register("system-driver-port-changed", b.systemDriverPortChanged)
	r.Register("external-driver-port-changed", b.externalDriverPortChanged)
	r.Register("refactor-type-choice", b.refactorTypeChoice)
	r.Register("approval-outcome", b.approvalOutcome)
	r.Register("outputs-and-scopes-valid", b.outputsAndScopesValid)
	r.Register("ticket-kind", b.ticketKind)
	// task-subtype: Item-11 second-level gateway. Stub binding for now —
	// reads ctx.State["task-subtype"] if preseeded, errors otherwise.
	// Phase D wires the real implementation alongside the ticketKind
	// split (the existing ticketKind binding still emits the composite
	// `task/<subtype>` value; reconciling that with the new two-gateway
	// YAML is Phase D's job — see plans/20260526-0832 Item 11 Q11.2).
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
// JSONL by the validator) lives in internal/assets/runtime/shared/scope.md.
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
// internal/atdd/runtime/actions/verify_classify.go). Both verify-tests-*
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

// dslPortChanged, systemDriverPortChanged, externalDriverPortChanged
// are the writing-agent output flags consumed by the per-test-layer
// fanout in implement-and-verify-dsl. Each reads the kebab ctx key the
// agent emits via `gh optivem output write KEY=VAL` (flattened from the
// per-dispatch JSONL file by validate-outputs-and-scopes). Missing/unset
// is a bug — the writing-agent MID's BPMN `outputs:` list MUST declare
// the key as required, so unset means the agent skipped the `output
// write` call — and we halt rather than mis-route (same doctrine as the
// older dslFlagsPresent gate).
func (b bindings) dslPortChanged(ctx *statemachine.Context) statemachine.Outcome {
	return boolStateGate(ctx, "dsl-port-changed")
}

func (b bindings) systemDriverPortChanged(ctx *statemachine.Context) statemachine.Outcome {
	return boolStateGate(ctx, "system-driver-port-changed")
}

func (b bindings) externalDriverPortChanged(ctx *statemachine.Context) statemachine.Outcome {
	return boolStateGate(ctx, "external-driver-port-changed")
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
// `change-system-behavior`). Empty reply → `none` (exit the loop).
// Mirrors the structuralTestMode shape (this file, top of file):
// preseed via ctx.State to short-circuit the prompt for hand-debug or
// transitions tests; otherwise inline ask → trim/lower → switch on
// the four enum values.
func (b bindings) refactorTypeChoice(ctx *statemachine.Context) statemachine.Outcome {
	if v := ctx.GetString("refactor-type-choice"); v != "" {
		return statemachine.Outcome{Value: v}
	}
	answer, err := b.deps.Prompter.Ask("Refactor type? (refactor-system-structure | refactor-test-structure | redesign-system-structure | redesign-external-system-structure | none) [none]: ")
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("refactor-type-choice: %w", err)}
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer == "" {
		answer = "none"
	}
	switch answer {
	case "refactor-system-structure",
		"refactor-test-structure",
		"redesign-system-structure",
		"redesign-external-system-structure",
		"none":
		return statemachine.Outcome{Value: answer}
	default:
		return statemachine.Outcome{Err: fmt.Errorf("refactor-type-choice: unrecognised value %q", answer)}
	}
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

// ticketKind composes the TOP `implement-ticket` dispatch
// discriminator from (Tracker.Classify, Tracker.Subtypes). Lookup
// table per Q-D3:
//
//	ticket-type | subtype                    | discriminator
//	---         | ---                        | ---
//	story       | (any/none)                 | story
//	bug         | (any/none)                 | bug
//	task        | legacy-coverage            | task/legacy-coverage
//	task        | system-redesign            | task/system-redesign
//	task        | external-system-redesign   | task/external-system-redesign
//	task        | system-refactor            | task/system-refactor
//	task        | test-refactor              | task/test-refactor
//
// Preseed via ctx.State["ticket-kind"] short-circuits the
// classification (hand-debug / transitions tests). Tracker.Classify
// returns the GitHub native issue type; ticketTypeAliases is the same
// "feature → story" normalization the older read_ticket_type action
// applies. Tracker.Subtypes returns the `subtype:*` label values for
// task tickets; exactly one is expected (multiple labels or none on a
// task is unrecognised composition).
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
		return statemachine.Outcome{Err: fmt.Errorf("ticket-kind: issue %s has no native issue type — set one of: Feature, Bug, Task (and for Task, also exactly one subtype:* label: %s) and re-run", issue.ID, strings.Join(ticketKindTaskSubtypes, ", "))}
	}
	if alias, ok := ticketKindAliases[kind]; ok {
		kind = alias
	}
	switch kind {
	case "story", "bug":
		return statemachine.Outcome{Value: kind}
	case "task":
		subs, err := b.deps.Tracker.Subtypes(context.Background(), issue)
		if err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("ticket-kind: %w", err)}
		}
		if len(subs) != 1 {
			return statemachine.Outcome{Err: fmt.Errorf("ticket-kind: task %s has %d subtype:* labels (want exactly one of: %s)", issue.ID, len(subs), strings.Join(ticketKindTaskSubtypes, ", "))}
		}
		sub := subs[0]
		if !ticketKindTaskSubtypeSet[sub] {
			return statemachine.Outcome{Err: fmt.Errorf("ticket-kind: task %s has unrecognised subtype:%s label (valid subtype:* labels are: %s)", issue.ID, sub, strings.Join(ticketKindTaskSubtypes, ", "))}
		}
		return statemachine.Outcome{Value: "task/" + sub}
	default:
		return statemachine.Outcome{Err: fmt.Errorf("ticket-kind: unsupported ticket type %q (expected one of: story, bug, task)", kind)}
	}
}

// taskSubtype is the Item-11 second-level gateway stub. Reads a
// preseeded `task-subtype` from ctx.State if present, errors
// otherwise. Phase D wires the real implementation that lifts the
// subtype out of `Tracker.Subtypes` so task-kind tickets dispatch
// end-to-end without manual preseed.
func (b bindings) taskSubtype(ctx *statemachine.Context) statemachine.Outcome {
	if v := ctx.GetString("task-subtype"); v != "" {
		return statemachine.Outcome{Value: v}
	}
	return statemachine.Outcome{Err: fmt.Errorf("task-subtype: not preseeded in ctx.State and the real binding is Phase D scope (see plans/20260526-0832 Item 11 Q11.2)")}
}

// ticketKindAliases mirrors the older read_ticket_type behaviour
// (actions/bindings.go ticketTypeAliases): GitHub's "Feature" native
// type is the new spelling of what the runtime calls "story".
var ticketKindAliases = map[string]string{"feature": "story"}

// ticketKindTaskSubtypes is the closed, canonically-ordered set of
// `subtype:*` labels the implement-ticket gateway dispatches on.
// Anything outside this set surfaces as unrecognised — the operator
// re-labels the ticket and re-runs. The order is the order shown in
// operator-facing error messages, so keep it in sync with the lookup
// table in the ticketKind doc comment.
var ticketKindTaskSubtypes = []string{
	"legacy-coverage",
	"system-redesign",
	"external-system-redesign",
	"system-refactor",
	"test-refactor",
}

// ticketKindTaskSubtypeSet is the O(1) membership view of
// ticketKindTaskSubtypes, built at package init. Keep the slice
// authoritative for ordering; the set is derived.
var ticketKindTaskSubtypeSet = func() map[string]bool {
	m := make(map[string]bool, len(ticketKindTaskSubtypes))
	for _, s := range ticketKindTaskSubtypes {
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
