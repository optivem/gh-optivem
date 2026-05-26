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

	"github.com/optivem/gh-optivem/internal/atdd/runtime/intake"
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
	r.Register("dsl_interface_changed", b.dslInterfaceChanged)
	r.Register("external_system_driver_interface_changed", b.externalSystemDriverInterfaceChanged)
	r.Register("system_driver_interface_changed", b.systemDriverInterfaceChanged)
	r.Register("ticket_type", b.ticketType)
	r.Register("subtype", b.subtype)
	r.Register("change_type", b.changeType)
	r.Register("ticket_type_recognized", b.ticketTypeRecognized)
	r.Register("subtype_ok", b.subtypeOK)
	r.Register("parse_ok", b.parseOK)
	r.Register("legacy_acceptance_criteria_section_present", b.legacyAcceptanceCriteriaSectionPresent)
	// Legacy coverage cycle dispatch (per plans/20260518-1116-legacy-coverage-cycle.md
	// item 1b): two presence sub-flags branch the `legacy_acceptance_criteria`
	// wrapper into one or both of legacy_at_cycle / legacy_ct_cycle. v1 wires a
	// prompt fallback; an intake-side parser that derives them from the issue
	// body would replace the prompt with a Context-fed value.
	r.Register("legacy_at_acceptance_criteria_present", b.legacyATAcceptanceCriteriaPresent)
	r.Register("legacy_ct_acceptance_criteria_present", b.legacyCTAcceptanceCriteriaPresent)
	// Inverted-RED verify gates terminating each legacy sub-cycle. Read
	// ctx[verify_class] stamped by the cycle-final run_tests service-task.
	// `ok` (or empty) → cycle ends green; `red` → STOP - HUMAN REVIEW with
	// no loopback (per plan: no loopback edge from VERIFY back to TEST,
	// to avoid the statemachine-test loop hazard).
	r.Register("legacy_at_verify_outcome", b.legacyATVerifyOutcome)
	r.Register("legacy_ct_verify_outcome", b.legacyCTVerifyOutcome)
	r.Register("refine_requested", b.refineRequested)
	r.Register("refinement_changed", b.refinementChanged)
	r.Register("refactor_changed", b.refactorChanged)
	r.Register("external_system_driver_exists", b.externalSystemDriverExists)
	r.Register("external_system_test_instance_accessible", b.externalSystemTestInstanceAccessible)
	r.Register("smoke_test_passes", b.smokeTestPasses)
	r.Register("structural_test_mode", b.structuralTestMode)
	// red_phase_cycle infrastructure (per
	// plans/20260505-230100-at-ct-cycle-creative-mechanical-split.md):
	// the two new gates that route the inner compile-then-run loop. No
	// YAML node references them yet; Step 2 of the AT/CT split wires
	// AT_RED_TEST through the shared sub-flow that will use them.
	r.Register("compile_ok", b.compileOK)
	r.Register("tests_failed_runtime", b.testsFailedRuntime)
	r.Register("tests_pass", b.testsPass)
	// Optional CT real-vs-stub verification (per AT/CT split plan): gates the
	// pre-RUN verification step. `verify_real_required` reads the
	// `verify_real_suite` call-activity param (set only by CT_RED_TEST today),
	// so AT phases route past it as a no-op. `verify_real_pass` reads the
	// flag set by `verify_real_suite_passes`.
	r.Register("verify_real_required", b.verifyRealRequired)
	r.Register("verify_real_pass", b.verifyRealPass)
	// structural-cycle verify routing (per
	// plans/20260505-214300-verify-failure-dispatch-fix-agent.md, Item 3):
	// reads ctx.State["verify_class"] (stamped by the verify action's
	// finalizeVerify) and decides ok → review, red → fix-agent retry,
	// red after one retry → halt. Infra is intercepted upstream by the
	// action's halt-on-infra; reaching the gate with class=infra is a
	// bug.
	r.Register("structural_verify_outcome", b.structuralVerifyOutcome)
	// tests_selected routes the post-CHOOSE_TESTS branch: true → run
	// BUILD_SYSTEM → START_SYSTEM → RUN_TESTS, false → skip straight to
	// COMMIT. Reads ctx.State["selected_test_commands"] (a []string) which
	// select_tests writes — empty slice means the operator picked "no".
	r.Register("tests_selected", b.testsSelected)
	// Phase-scope enforcement (per plan 20260518-1144 items 5, 6):
	//   - Layer 1: scope_exception_requested reads the agent's structured
	//     COMMIT output for a scope_exception block; true → STOP_SCOPE_VIOLATION.
	//   - Layer 2: phase_scope_clean runs after the agent commits; reads the
	//     check_phase_scope action's structured result; false → STOP_SCOPE_VIOLATION.
	r.Register("scope_exception_requested", b.scopeExceptionRequested)
	r.Register("phase_scope_clean", b.phaseScopeClean)
	// Post-RED-DSL flag-presence validation (per plan 20260518-1144 item 4):
	// the two RED-DSL phase-output flags (system_driver_interface_changed,
	// external_system_driver_interface_changed) MUST be explicitly emitted
	// by the agent's COMMIT output. Treats unset as an error — the
	// downstream GATE_EXT_AT / GATE_SYS_AT gates rely on a real yes|no,
	// not a default no.
	r.Register("dsl_flags_present", b.dslFlagsPresent)
	// BPMN Phase D bindings (per
	// plans/20260525-2348-bpmn-phase-d-bindings.md). Kebab-case here
	// matches the YAML `binding:` references; the ctx state/params keys
	// the bodies read are also kebab (agent-emitted outputs, call-site
	// params). Old snake-case registrations above are tolerated by the
	// registry and will be swept once the new YAML shape is proven on a
	// full ticket.
	r.Register("command-succeeded", b.commandSucceeded)
	r.Register("test-outcome", b.testOutcome)
	r.Register("expected-test-result", b.expectedTestResult)
	r.Register("fix-on-failure-enabled", b.fixOnFailureEnabled)
	r.Register("dsl-port-changed", b.dslPortChanged)
	r.Register("system-driver-ports-changed", b.systemDriverPortsChanged)
	r.Register("external-driver-ports-changed", b.externalDriverPortsChanged)
	r.Register("refactor-type-choice", b.refactorTypeChoice)
	r.Register("approval-outcome", b.approvalOutcome)
	r.Register("outputs-and-scopes-valid", b.outputsAndScopesValid)
	r.Register("ticket-kind", b.ticketKind)
}

// bindings is a thin closure-receiver so each method has access to deps
// without taking it as a parameter. Keeping the methods pure-as-possible
// (only Context + deps) lets the test suite exercise them through the same
// NodeFn contract the engine sees.
type bindings struct {
	deps Deps
}

// ---------------------------------------------------------------------------
// Boolean gates after WRITE phases — forward-looking, prompt-driven
// ---------------------------------------------------------------------------

func (b bindings) dslInterfaceChanged(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"dsl_interface_changed",
		"DSL interface changed in this phase?")
}

func (b bindings) externalSystemDriverInterfaceChanged(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"external_system_driver_interface_changed",
		"External System Driver interface changed?")
}

func (b bindings) systemDriverInterfaceChanged(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"system_driver_interface_changed",
		"System Driver interface changed?")
}

// refineRequested is the pre-BACKLOG_REFINEMENT branch: asks the operator
// whether to invoke refine-acceptance-criteria at all for this ticket. true → run the full
// refine pass (refine-acceptance-criteria → CONFIRM_REFINEMENT → optional UPDATE_TICKET);
// false → skip straight to BR_END, leaving the materialized parsed-concepts
// artifact unread. No upstream action pre-decides this — refine is per-ticket
// operator intent — so the binding is always prompt-driven (boolGate still
// reads any pre-set ctx value, which is what the cycle tests rely on).
func (b bindings) refineRequested(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"refine_requested",
		"Refine acceptance criteria for this ticket?")
}

// refinementChanged is the post-BACKLOG_REFINEMENT branch: routes the
// backlog_refinement sub-process to UPDATE_TICKET when the refiner
// mutated the parsed-concepts artifact, and skips to the sub-process
// end-event when refinement was a no-op. Reads the `refinement_changed`
// flag set by the refine-acceptance-criteria agent's COMMIT (`Refinement Changed:
// yes|no`); falls back to a prompt for hand-debugging if the upstream
// dispatch hasn't run.
func (b bindings) refinementChanged(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"refinement_changed",
		"Refinement changed acceptance criteria?")
}

// refactorChanged is the post-AT_REFACTOR branch: routes the
// at_refactor_system sub-process to COMMIT when the refactor agent
// touched production code, and skips to the sub-process end-event when
// the refactor was a no-op (no improvement seen). Reads the
// `refactor_changed` flag set by the refactor-system agent's COMMIT
// (`Refactor Changed: yes|no`); falls back to a prompt for hand-debugging
// if the upstream dispatch hasn't run.
func (b bindings) refactorChanged(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"refactor_changed",
		"Refactor changed production code?")
}

// ---------------------------------------------------------------------------
// String / enum gates — backed by upstream actions, with prompt fallback
// ---------------------------------------------------------------------------

// ticketType reads the classification produced by the classify_ticket_type
// service task — the lowercased name of the issue's native GitHub type.
// Range matches the three configured types: story | bug | task. The
// action is expected to write the canonical lowercased form.
func (b bindings) ticketType(ctx *statemachine.Context) statemachine.Outcome {
	v := ctx.GetString("ticket_type")
	if v != "" {
		return statemachine.Outcome{Value: v}
	}
	answer, err := b.deps.Prompter.Ask("Ticket type? (story | bug | task): ")
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("ticket_type: %w", err)}
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	switch answer {
	case "story", "bug", "task":
		return statemachine.Outcome{Value: answer}
	default:
		return statemachine.Outcome{Err: fmt.Errorf("ticket_type: unrecognised value %q", answer)}
	}
}

// changeType reads the single-axis change-type derived during intake
// from (ticket_type, subtype). Range: behavioral |
// system-interface-redesign | external-system-interface-redesign |
// system-implementation-refactoring. Falls back to a prompt for
// hand-debugging if the upstream derivation hasn't run.
func (b bindings) changeType(ctx *statemachine.Context) statemachine.Outcome {
	v := ctx.GetString("change_type")
	if v != "" {
		return statemachine.Outcome{Value: v}
	}
	answer, err := b.deps.Prompter.Ask(
		"Change type? (behavioral | system-interface-redesign | external-system-interface-redesign | system-implementation-refactoring): ")
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("change_type: %w", err)}
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	switch answer {
	case "behavioral",
		"system-interface-redesign",
		"external-system-interface-redesign",
		"system-implementation-refactoring":
		return statemachine.Outcome{Value: answer}
	default:
		return statemachine.Outcome{Err: fmt.Errorf("change_type: unrecognised value %q", answer)}
	}
}

// subtype reads the structural-change subtype produced by the
// classify_subtype service task — the trimmed value of the `subtype:*`
// label on a task ticket. Only reached for tasks; behavioral tickets
// route past this gate.
func (b bindings) subtype(ctx *statemachine.Context) statemachine.Outcome {
	v := ctx.GetString("subtype")
	if v != "" {
		return statemachine.Outcome{Value: v}
	}
	answer, err := b.deps.Prompter.Ask(
		"Subtype? (system-interface-redesign | external-system-interface-redesign | system-implementation-refactoring): ")
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("subtype: %w", err)}
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	switch answer {
	case "system-interface-redesign",
		"external-system-interface-redesign",
		"system-implementation-refactoring":
		return statemachine.Outcome{Value: answer}
	default:
		return statemachine.Outcome{Err: fmt.Errorf("subtype: unrecognised value %q", answer)}
	}
}

// structuralVerifyOutcome routes the post-RUN_TESTS gateway based on the
// failure class the verify action stamped into ctx.State["verify_class"].
// Behaviour-preserving structural cycles are the *only* place we want to
// auto-dispatch a fix agent on red — RED is not expected here, so the
// cycle should heal itself once before surfacing to a human.
//
// Routing tokens (consumed by the YAML's `when:` clauses):
//
//   - "ok"       — green (or no commands ran). Continue to APPROVE_COMMIT.
//   - "red"      — first red of this cycle. Increments verify_retries and
//                  returns "red" so the gateway routes to FIX_STRUCT_VERIFY,
//                  which dispatches the appropriate fix-* diagnosis agent
//                  (fix-unexpected-passing-tests / fix-unexpected-failing-tests)
//                  and loops back to CHOOSE_TESTS so the operator can re-pick
//                  scope.
//
// Halt paths return Outcome.Err directly (no routing token):
//
//   - infra      — defensive. The verify action's finalizeVerify halts on
//                  infra at the action level (Item 5); reaching the gate
//                  with infra means that halt was bypassed. Surface as a
//                  bug rather than silently routing.
//   - red after  — the fix agent had its one retry and the cycle is still
//     a retry      red. Halt with a diagnostic so the human takes over.
//   - unknown    — class outside {ok, red, infra} means the action
//                  contract drifted from the gate; halt loudly.
//
// Empty class is treated as "ok": the verify action stamps an empty
// value when no commands ran (approve-without-running, no driver-adapter
// changes), which the trace honestly renders as "(no result)" — the
// human still owns the review decision.
func (b bindings) structuralVerifyOutcome(ctx *statemachine.Context) statemachine.Outcome {
	class := ctx.GetString("verify_class")
	switch class {
	case "", "ok":
		return statemachine.Outcome{Value: "ok"}
	case "infra":
		return statemachine.Outcome{Err: fmt.Errorf(
			"structural_verify_outcome: infra-class verify reached gateway — verify action's halt-on-infra (Item 5) was bypassed")}
	case "red":
		retries, _ := ctx.Get("verify_retries").(int)
		if retries >= 1 {
			return statemachine.Outcome{Err: fmt.Errorf(
				"structural_verify_outcome: structural cycle still RED after %d fix-agent retry — see verify_results above", retries)}
		}
		ctx.Set("verify_retries", retries+1)
		return statemachine.Outcome{Value: "red"}
	default:
		return statemachine.Outcome{Err: fmt.Errorf(
			"structural_verify_outcome: unrecognised verify_class %q (action stamped a value the gate does not handle)", class)}
	}
}

// structuralTestMode prompts for the TEST gate's three-way choice. Always
// asks the user — there is no upstream action that pre-decides this. Empty
// reply (Enter) defaults to `compile` because that is the safest option that
// still surfaces drift without consuming the docker stack.
func (b bindings) structuralTestMode(ctx *statemachine.Context) statemachine.Outcome {
	if v := ctx.GetString("structural_test_mode"); v != "" {
		return statemachine.Outcome{Value: v}
	}
	answer, err := b.deps.Prompter.Ask("TEST mode? (full | compile | skip) [compile]: ")
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("structural_test_mode: %w", err)}
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer == "" {
		answer = "compile"
	}
	switch answer {
	case "full", "compile", "skip":
		return statemachine.Outcome{Value: answer}
	default:
		return statemachine.Outcome{Err: fmt.Errorf("structural_test_mode: unrecognised value %q", answer)}
	}
}

// ---------------------------------------------------------------------------
// Boolean gates — backed by upstream actions or one-off prompts
// ---------------------------------------------------------------------------

// ticketTypeRecognized reads the recognition flag set by the read_ticket_type
// service task. The action is expected to write `true` when the ticket's
// native GitHub type is one of Story / Bug / Task and `false` when it
// needs human resolution. Falls back to a prompt for hand-debugging.
func (b bindings) ticketTypeRecognized(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"ticket_type_recognized",
		"Ticket type recognized?")
}

// subtypeOK reads the flag set by the classify_subtype service task.
// true → exactly one subtype:* label was found; false → 0 or 2+, route to
// STOP_SUBTYPE_MISSING. Falls back to a prompt for hand-debugging.
func (b bindings) subtypeOK(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"subtype_ok",
		"Subtype label detected?")
}

// parseOK reads the parse-success flag set by the parse_ticket_body
// service task. true → intake completes; false → STOP_PARSE_ERROR. Falls
// back to a prompt for hand-debugging.
func (b bindings) parseOK(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"parse_ok",
		"Ticket body parsed OK?")
}

// legacyAcceptanceCriteriaSectionPresent asks Tracker.ReadSections for
// the `Legacy Acceptance Criteria` section and routes on whether the
// returned body is non-empty. Falls back to a prompt when no issue
// number is in the Context (off-board mode).
func (b bindings) legacyAcceptanceCriteriaSectionPresent(ctx *statemachine.Context) statemachine.Outcome {
	if v := ctx.Get("legacy_acceptance_criteria_section_present"); v != nil {
		return outcomeFromBoolish(v)
	}
	if ctx.GetString("issue_num") == "" {
		return b.boolGate(ctx,
			"legacy_acceptance_criteria_section_present",
			"Legacy Acceptance Criteria section present in the issue?")
	}
	issue, err := issueFromContext(ctx)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("legacy_acceptance_criteria_section_present: %w", err)}
	}
	sections, err := b.deps.Tracker.ReadSections(context.Background(), issue, []string{intake.SectionLegacyAcceptanceCriteria})
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("legacy_acceptance_criteria_section_present: %w", err)}
	}
	return statemachine.Outcome{Bool: sections[intake.SectionLegacyAcceptanceCriteria] != ""}
}

// legacyATAcceptanceCriteriaPresent routes the `legacy_acceptance_criteria`
// wrapper's first branch — does the ticket carry any AT-style legacy
// criteria (behavioural scenarios that need acceptance-test backfill)?
// v1 falls back to a prompt; an intake-side parser that classifies
// criteria by type would seed ctx ahead of time.
func (b bindings) legacyATAcceptanceCriteriaPresent(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"legacy_at_acceptance_criteria_present",
		"Legacy AT-style acceptance criteria present in the issue?")
}

// legacyCTAcceptanceCriteriaPresent routes the wrapper's second branch —
// CT-style legacy criteria (external-system contract scenarios that need
// contract-test backfill).
func (b bindings) legacyCTAcceptanceCriteriaPresent(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"legacy_ct_acceptance_criteria_present",
		"Legacy CT-style acceptance criteria present in the issue?")
}

// legacyATVerifyOutcome terminates the legacy AT sub-cycle's inverted-RED
// verify gate. Reads ctx[verify_class] stamped by the cycle-final
// run_tests service-task. Routing tokens:
//
//   - "ok"   — assembled legacy AT tests passed on first run as expected;
//              cycle ends green.
//   - "red"  — at least one test failed; route to STOP - HUMAN REVIEW.
//              No retry, no loopback (per plan: the test / DSL / driver is
//              suspect, never the SUT; operator edits the offending layer
//              and re-runs the legacy cycle from scratch).
//
// Halt paths:
//   - "infra"   — environment failure (e.g. docker stack down). Surface as
//                 an error rather than misclassifying as a layer bug.
//   - unknown   — action contract drift; halt loudly.
//
// Empty class is treated as "ok": no commands ran (defensive — every
// legacy cycle should run tests, but matches structuralVerifyOutcome).
func (b bindings) legacyATVerifyOutcome(ctx *statemachine.Context) statemachine.Outcome {
	return legacyVerifyOutcome(ctx, "legacy_at_verify_outcome")
}

// legacyCTVerifyOutcome is the legacy CT sub-cycle's verify gate — same
// shape as legacyATVerifyOutcome.
func (b bindings) legacyCTVerifyOutcome(ctx *statemachine.Context) statemachine.Outcome {
	return legacyVerifyOutcome(ctx, "legacy_ct_verify_outcome")
}

// legacyVerifyOutcome is the shared body of both legacy verify gates.
// Free function (not a bindings method) because it carries no deps —
// the gate is a pure read of ctx[verify_class].
func legacyVerifyOutcome(ctx *statemachine.Context, gateName string) statemachine.Outcome {
	class := ctx.GetString("verify_class")
	switch class {
	case "", "ok":
		return statemachine.Outcome{Value: "ok"}
	case "red":
		return statemachine.Outcome{Value: "red"}
	case "infra":
		return statemachine.Outcome{Err: fmt.Errorf(
			"%s: infra-class verify reached gateway — verify action's halt-on-infra was bypassed", gateName)}
	default:
		return statemachine.Outcome{Err: fmt.Errorf(
			"%s: unrecognised verify_class %q (action stamped a value the gate does not handle)", gateName, class)}
	}
}

// issueFromContext builds a tracker.Issue from the conventional Context
// keys actions.pickTopReady writes (issue_num, issue_url, issue_title,
// issue_handle). issue_url is the addressable form every Tracker call
// site needs; callers that don't seed it get a clear error rather than
// a downstream parse failure.
func issueFromContext(ctx *statemachine.Context) (tracker.Issue, error) {
	url := ctx.GetString("issue_url")
	if url == "" {
		return tracker.Issue{}, fmt.Errorf("issue_url not in Context")
	}
	return tracker.Issue{
		ID:     ctx.GetString("issue_num"),
		Title:  ctx.GetString("issue_title"),
		URL:    url,
		Handle: ctx.GetString("issue_handle"),
	}, nil
}

// externalSystemDriverExists is asked once at the top of the onboarding
// sub-flow. The semantic check is "does this repo already have a driver
// implementation for the external system?" — file-system inspection is
// possible but the path schema varies per consumer, so v1 prompts.
func (b bindings) externalSystemDriverExists(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"external_system_driver_exists",
		"External System Driver already exists in this repo?")
}

// externalSystemTestInstanceAccessible is the second onboarding gate.
// Same prompt-or-Context shape as the driver-exists check.
func (b bindings) externalSystemTestInstanceAccessible(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"external_system_test_instance_accessible",
		"Test instance of the external system accessible?")
}

// smokeTestPasses is set by the run_smoke_test service task's Outcome.
// When called without that upstream having run (e.g. a transitions test
// driving the gateway directly), fall back to a prompt for the sake of
// hand-debugging.
func (b bindings) smokeTestPasses(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"smoke_test_passes",
		"Smoke test passed?")
}

// compileOK reads the `compile_ok` flag set by the active compile action
// (compile_all / compile_system / compile_system_tests).
// true → continue to the RUN node; false → route to WRITE_PROTOTYPES so the
// agent adds prototype methods (TODO: DSL / TODO: Driver / ...) for whatever
// the WRITE phase referenced that does not yet exist. Falls back to a prompt
// for hand-debugging when no upstream action ran.
func (b bindings) compileOK(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"compile_ok",
		"Compile passed?")
}

// testsFailedRuntime reads the `tests_failed_runtime` flag set by the
// run_targeted_tests action. true → tests failed at runtime as expected
// for RED, route to DISABLE; false → either the tests passed (suspicious;
// the WRITE phase did not produce a failing test) or some failed at
// compile (the compile-loop stabilised by the gate above is not actually
// stable). Falls back to a prompt for hand-debugging.
func (b bindings) testsFailedRuntime(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"tests_failed_runtime",
		"Tests failed at runtime (not compile)?")
}

// testsPass reads the `tests_pass` flag set by run_targeted_tests. true →
// every test in the run passed, route to GREEN_END; false → at least one
// failed (compile or runtime), route to STOP_GREEN_TEST_FAIL so the human
// can review before the agent re-dispatches. Used by green_phase_cycle.
// Falls back to a prompt for hand-debugging.
func (b bindings) testsPass(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"tests_pass",
		"All tests passed?")
}

// verifyRealRequired routes the optional "verify against real suite" branch
// of red_phase_cycle. Reads the `verify_real_suite` param the calling
// call-activity stamped onto Context.Params: a non-empty value means the
// caller wants the orchestrator to run that suite before the regular RUN.
// CT_RED_TEST sets it to <suite-contract-real>; AT phases leave it unset
// and the gate routes straight through to RUN.
//
// No prompt fallback: the param is structural metadata of the call site,
// not a runtime decision the user re-makes per cycle.
func (b bindings) verifyRealRequired(ctx *statemachine.Context) statemachine.Outcome {
	suite := strings.TrimSpace(ctx.Params["verify_real_suite"])
	return statemachine.Outcome{Bool: suite != ""}
}

// verifyRealPass reads the `verify_real_pass` flag set by the
// verify_real_suite_passes action. true → real-suite contract holds, route
// to RUN; false → the new tests do not pass against the real external
// system, route to STOP_VERIFY_REAL_FAIL so the human can decide whether
// to fix the test or escalate the contract problem. Falls back to a prompt
// for hand-debugging.
func (b bindings) verifyRealPass(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"verify_real_pass",
		"Real-suite verification passed?")
}

// testsSelected reports whether select_tests recorded any commands to run
// in ctx[selected_test_commands]. The slice is the contract: a non-empty
// []string means the operator picked all / some / specific; an empty (or
// nil) slice means they picked "no". No prompt fallback — the gate runs
// immediately after select_tests, so the value must be set.
func (b bindings) testsSelected(ctx *statemachine.Context) statemachine.Outcome {
	cmds, _ := ctx.Get("selected_test_commands").([]string)
	return statemachine.Outcome{Bool: len(cmds) > 0}
}

// scopeExceptionRequested is Layer 1 of phase-scope enforcement (per plan
// 20260518-1144 item 6): the agent-triggered escape hatch. The agent emits
// a structured `scope_exception` block in its COMMIT output when it
// recognises it cannot complete the phase within `scope:`; a COMMIT-output
// parsing layer populates ctx[scope_exception_files] ([]string) and
// ctx[scope_exception_reason] (string). This binding returns true when
// scope_exception_files is non-empty, routing the cycle to
// STOP_SCOPE_VIOLATION (skipping DISABLE / Layer 2 / COMMIT).
//
// No prompt fallback: the gate fires immediately after the agent's WRITE
// node; an absent value means "no exception was signalled" and routes to
// the normal continuation. The shape contract (yaml emitted by the agent,
// flattened into two context keys by the parser) lives in
// internal/assets/runtime/shared/scope.md.
func (b bindings) scopeExceptionRequested(ctx *statemachine.Context) statemachine.Outcome {
	files, _ := ctx.Get("scope_exception_files").([]string)
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
// ctx.State, not via boolGate (which would conflate "unset" with
// "answered no at the prompt").
func (b bindings) dslFlagsPresent(ctx *statemachine.Context) statemachine.Outcome {
	_, sys := ctx.State["system_driver_interface_changed"]
	_, ext := ctx.State["external_system_driver_interface_changed"]
	return statemachine.Outcome{Bool: sys && ext}
}

// phaseScopeClean is Layer 2 of phase-scope enforcement (per plan
// 20260518-1144 item 5): the post-phase scripted check. The
// check_phase_scope action diffs the working tree against the phase's
// allowed-paths (from internal/atdd/phase-scopes.yaml + gh-optivem.yaml
// paths:) and writes the boolean result to ctx[phase_scope_clean] plus
// the violating paths to ctx[phase_scope_violating_paths] for the
// STOP_SCOPE_VIOLATION payload. This binding returns the boolean
// verbatim; true → continue to COMMIT, false → STOP_SCOPE_VIOLATION.
//
// No prompt fallback: the gate fires after the action that stamps the
// flag, so the value must be set; reaching the gate with an unset value
// is a bug, not a hand-debugging affordance.
func (b bindings) phaseScopeClean(ctx *statemachine.Context) statemachine.Outcome {
	v, ok := ctx.State["phase_scope_clean"]
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("phase_scope_clean: not set in Context — check_phase_scope action did not run")}
	}
	return outcomeFromBoolish(v)
}

// ---------------------------------------------------------------------------
// BPMN Phase D bindings (plans/20260525-2348-bpmn-phase-d-bindings.md)
// ---------------------------------------------------------------------------
//
// All keys read/written below are kebab-case to match the YAML
// `binding:` references, the YAML `params:` block keys, and the
// agent-emitted `outputs:` yaml keys flattened into ctx.State by
// clauderun.ParseOutputs. Asymmetry vs. the snake-case keys used by
// older bindings/actions above is accepted per Q-D1 — the kebab
// vocabulary is the target shape; the old snake keys are tolerated
// until the dead-binding sweep follow-up.

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
// per-suite pass|fail value run-command stamps after a
// `gh optivem run-tests` invocation. Halt on unset (the gate fires
// immediately after RUN_TESTS, so the action's stamp is required) and
// halt on any value outside {pass, fail} so a future action contract
// drift surfaces loudly instead of silently mis-routing.
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
	case "pass", "fail":
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

// fixOnFailureEnabled gates the LOW `execute-agent` primitive's
// validation-failure → CALL_FIX edge. Reads the `fix-on-failure`
// call-activity param: only the `fix` primitive's recursive
// `execute-agent` call sets it to "false" (single-attempt
// remediation), so the default (missing/empty) is true — every other
// caller wants fix dispatch on validation failure. Coerces via
// promptio.ParseYN so "true"/"false"/"yes"/"no" all round-trip.
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

// dslPortChanged, systemDriverPortsChanged, externalDriverPortsChanged
// are the writing-agent output flags consumed by the per-test-layer
// fanout in implement-and-verify-dsl. Each reads the kebab ctx key
// the agent's `outputs:` YAML block emits (flattened verbatim by
// clauderun.ParseOutputs into ctx.State). Missing/unset is a bug —
// the writing-agent prompt MUST list the flag in `outputs:`, so unset
// means the agent's COMMIT block was malformed, not "no" — and we
// halt rather than mis-route (same doctrine as the older
// dslFlagsPresent gate).
func (b bindings) dslPortChanged(ctx *statemachine.Context) statemachine.Outcome {
	return boolStateGate(ctx, "dsl-port-changed")
}

func (b bindings) systemDriverPortsChanged(ctx *statemachine.Context) statemachine.Outcome {
	return boolStateGate(ctx, "system-driver-ports-changed")
}

func (b bindings) externalDriverPortsChanged(ctx *statemachine.Context) statemachine.Outcome {
	return boolStateGate(ctx, "external-driver-ports-changed")
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
	answer, err := b.deps.Prompter.Ask("Refactor type? (refactor-system-structure | refactor-test-structure | redesign-system-structure | none) [none]: ")
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
//	task        | cover-legacy               | task/cover-legacy
//	task        | redesign-system            | task/redesign-system
//	task        | refactor-system            | task/refactor-system
//	task        | refactor-tests             | task/refactor-tests
//	task        | onboard-external-system    | task/onboard-external-system
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
		return statemachine.Outcome{Err: fmt.Errorf("ticket-kind: issue %s has no native issue type — set Feature / Bug / Task and re-run", issue.ID)}
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
			return statemachine.Outcome{Err: fmt.Errorf("ticket-kind: task %s has %d subtype:* labels (want exactly one)", issue.ID, len(subs))}
		}
		sub := subs[0]
		if !ticketKindTaskSubtypes[sub] {
			return statemachine.Outcome{Err: fmt.Errorf("ticket-kind: task %s has unrecognised subtype %q", issue.ID, sub)}
		}
		return statemachine.Outcome{Value: "task/" + sub}
	default:
		return statemachine.Outcome{Err: fmt.Errorf("ticket-kind: unsupported ticket type %q (expected story | bug | task)", kind)}
	}
}

// ticketKindAliases mirrors the older read_ticket_type behaviour
// (actions/bindings.go ticketTypeAliases): GitHub's "Feature" native
// type is the new spelling of what the runtime calls "story".
var ticketKindAliases = map[string]string{"feature": "story"}

// ticketKindTaskSubtypes is the closed set of `subtype:*` labels the
// implement-ticket gateway dispatches on. Anything outside this set
// surfaces as unrecognised — the operator re-labels the ticket and
// re-runs.
var ticketKindTaskSubtypes = map[string]bool{
	"cover-legacy":             true,
	"redesign-system":          true,
	"refactor-system":          true,
	"refactor-tests":           true,
	"onboard-external-system":  true,
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// boolGate is the canonical Context-or-prompt shape: read an existing value,
// otherwise ask the user a yes/no question through promptio (which loops on
// unrecognised input and appends the " [y/n]: " hint itself). Callers pass
// the bare question text, no hint suffix.
func (b bindings) boolGate(ctx *statemachine.Context, key, prompt string) statemachine.Outcome {
	if v, ok := ctx.State[key]; ok {
		return outcomeFromBoolish(v)
	}
	yes, err := promptio.ConfirmYNVia(b.deps.Prompter, os.Stderr, prompt)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("%s: %w", key, err)}
	}
	return statemachine.Outcome{Bool: yes}
}

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
