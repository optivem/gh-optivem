// Bindings — Go implementations of every service-task `action:` referenced
// in internal/atdd/runtime/statemachine/process-flow.yaml.
//
// Actions are the mechanical work of the pipeline: read the project board,
// move cards, classify the ticket, run a smoke test, commit a phase, etc.
// They route tracker-shaped work (PickReady, SetStatus, ReadSections,
// MarkChecklistComplete, Classify, Subtypes) through the Tracker
// interface and wrap runtime/release for commit work; everything else
// is implemented directly in this file using the same shell-out +
// dependency-injection pattern (Deps with Gh / Git / Prompter / Stdout,
// all defaulting to real implementations when nil).
//
// Every action returns `statemachine.Outcome` with Err set on hard failures.
// User-driven aborts (e.g. answering "no" to "Can I commit?") also surface
// as Err so the engine halts the run — silent decline would route past a
// gate the user explicitly closed.
package actions

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/optivem/gh-optivem/internal/atdd"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/intake"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/release"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/testselect"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/tracker"
	trackergithub "github.com/optivem/gh-optivem/internal/atdd/runtime/tracker/github"
	"github.com/optivem/gh-optivem/internal/projectconfig"
	"github.com/optivem/gh-optivem/internal/promptio"
)

// Deps bundles the side-effecting collaborators every action may need. All
// fields are optional; a zero-value Deps falls back to real shell-outs and
// the OS stdin/stdout. Tests pass non-nil fakes for hermeticity.
type Deps struct {
	Gh         GhRunner
	Git        GitRunner
	Shell      ShellRunner // for compile-all.sh / test-all.sh / docker compose
	Prompter   Prompter
	Stdout     io.Writer
	Stderr     io.Writer
	ProjectURL string // optional — explicit override for tracker operations
	RepoPath   string // optional — defaults to current working directory
	// Tracker is the seam every issue-tracker operation (PickReady,
	// SetStatus, ReadSections, MarkChecklistComplete, Classify) goes
	// through. Optional — withDefaults constructs a github adapter from
	// ProjectURL + Gh when unset. Tests inject fakes either by setting
	// ProjectURL + a fake Gh (the constructed github tracker then routes
	// through the fake), or by setting Tracker directly for full control.
	Tracker tracker.Tracker
	// Autonomous mirrors driver.Opts.Autonomous: when true, actions that
	// would prompt the operator instead emit a warning and proceed. Today
	// only parseTicketBody's "all checklist items already [x]" guard reads
	// this; other prompts (smoke test, can-I-commit) do not yet have an
	// autonomous-mode codepath.
	Autonomous bool
}

// Prompter is the same interface gates uses; redefined here so the actions
// package does not import gates (each registry stays self-contained).
type Prompter interface {
	Ask(prompt string) (string, error)
}

// GhRunner runs the `gh` CLI.
type GhRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// GitRunner runs the `git` CLI.
type GitRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// ShellRunner runs an arbitrary command. We use a bash-style CommandLine
// (no argv split) because the pipeline's compile-all.sh / test-all.sh wrap
// multi-stage shell pipelines and the most testable contract is "give the
// fake the exact string you would type at a prompt".
type ShellRunner interface {
	Run(ctx context.Context, commandLine string) ([]byte, error)
}

func (d Deps) withDefaults() Deps {
	if d.Gh == nil {
		d.Gh = realGh{}
	}
	if d.Git == nil {
		d.Git = realGit{}
	}
	if d.Shell == nil {
		d.Shell = realShell{}
	}
	if d.Prompter == nil {
		d.Prompter = stdinPrompter{}
	}
	if d.Stdout == nil {
		d.Stdout = os.Stdout
	}
	if d.Stderr == nil {
		d.Stderr = os.Stderr
	}
	if d.Tracker == nil {
		// Default to a github adapter wrapping the (possibly fake) Gh
		// runner. Production callers set ProjectURL from gh-optivem.yaml
		// (driver.go) so PickReady / SetStatus / Verify resolve against a
		// real project; tests that don't exercise project ops can omit
		// ProjectURL and the placeholder below keeps github.New from
		// rejecting the call — issue-body ops (ReadSections /
		// MarkChecklistComplete / Classify) only need Issue.URL anyway.
		url := d.ProjectURL
		if url == "" {
			url = "https://github.com/orgs/placeholder/projects/0"
		}
		if t, err := trackergithub.New(url, ghAdapter{d.Gh}); err == nil {
			d.Tracker = t
		}
	}
	return d
}

// RegisterAll wires every YAML action name to its implementation.
func RegisterAll(r *Registry, deps Deps) {
	deps = deps.withDefaults()
	a := actions{deps: deps}
	r.Register("pick_top_ready", a.pickTopReady)
	r.Register("move_to_in_progress", a.moveToInProgress)
	r.Register("read_ticket_type", a.readTicketType)
	r.Register("read_subtype", a.readSubtype)
	r.Register("parse_ticket_body", a.parseTicketBody)
	r.Register("materialize_parsed_concepts", a.materializeParsedConcepts)
	r.Register("report_intake_summary", a.reportIntakeSummary)
	r.Register("move_to_in_acceptance", a.moveToInAcceptance)
	r.Register("run_smoke_test", a.runSmokeTest)
	r.Register("compile_all", a.compileAll)
	r.Register("compile_system", a.compileSystem)
	r.Register("compile_system_tests", a.compileSystemTests)
	r.Register("commit_phase", a.commitPhase)
	r.Register("tick_checklist", a.tickChecklist)
	r.Register("select_tests", a.selectTests)
	r.Register("build_system", a.buildSystem)
	r.Register("start_system", a.startSystem)
	r.Register("run_tests", a.runTests)
	// red_phase_cycle / green_phase_cycle infrastructure (per
	// plans/20260505-230100-at-ct-cycle-creative-mechanical-split.md): the
	// shared sub-flows' mechanical steps. Compile is dispatched via
	// ${compile_action} resolving to one of compile_{all,system,system_tests}
	// above; the cycle's COMPILE node looks up by templated name at runtime.
	r.Register("run_targeted_tests", a.runTargetedTests)
	// Optional CT real-vs-stub verification (per AT/CT split plan): runs the
	// suite named in the `verify_real_suite` call_activity param against the
	// just-written tests and asserts every one passes. Driven by the
	// `verify_real_required` gate, which is no-op for AT phases.
	r.Register("verify_real_suite_passes", a.verifyRealSuitePasses)
	// Phase-scope enforcement Layer 2 (per plan 20260518-1144 item 5): runs
	// after the agent commits, diffs the working tree against the phase's
	// allowed paths joined from internal/atdd/phase-scopes.yaml +
	// gh-optivem.yaml paths:, and writes phase_scope_clean +
	// phase_scope_violating_paths to context. The downstream
	// phase_scope_clean gate consumes the boolean.
	r.Register("check_phase_scope", a.checkPhaseScope)
}

// Context keys consumed by the red_phase_cycle actions. Centralised so the
// agent dispatcher (Step 2 of the AT/CT split) and tests have one place to
// find the contract. The corresponding values are populated from the
// ticket parse (Suite) and the WRITE phase's output (TestNames).
//
// Note: the `language`, `ticket_id`, `loop`, `phase`, `prev_phase`, and
// `disable_targets` keys used to be consumed by the deterministic
// disable_change_driven / enable_change_driven Go actions; those actions
// were replaced on 2026-05-20 by the `disable-tests` / `enable-tests` agent
// prompts (user_task in BPMN) and the corresponding keys are now consumed
// by the agent template engine's ${var} substitution rather than by Go
// code. See plans/20260520-0001-switch-disable-enable-tests-to-agents.md.
const (
	// CtxKeySuite is the test suite name handed to run_targeted_tests —
	// e.g. "<acceptance-api>", "<suite-contract-real>".
	CtxKeySuite = "suite"

	// CtxKeyTestNames is the []string of test method names run_targeted_tests
	// dispatches against the suite, one per `gh optivem test run --test`
	// invocation.
	CtxKeyTestNames = "test_names"

	// CtxKeyCompileOK is the bool the compile actions
	// (compile_all / compile_system / compile_system_tests) write to record
	// whether the compile passed. Read by the compile_ok gate.
	CtxKeyCompileOK = "compile_ok"

	// CtxKeyTestsFailedRuntime is the bool run_targeted_tests writes to
	// record that every observed failure was a runtime failure (not
	// compile). Read by the tests_failed_runtime gate.
	CtxKeyTestsFailedRuntime = "tests_failed_runtime"

	// CtxKeyTestsPass is the bool run_targeted_tests writes to record
	// whether every test in the run passed. Read by the tests_pass gate
	// (green_phase_cycle's success-or-loop signal).
	CtxKeyTestsPass = "tests_pass"

	// CtxKeyVerifyRealPass is the bool verify_real_suite_passes writes to
	// record whether every test passed against the suite named in the
	// `verify_real_suite` call_activity param. Read by the verify_real_pass
	// gate.
	CtxKeyVerifyRealPass = "verify_real_pass"

	// CtxKeyPhaseScopeClean is the bool check_phase_scope writes to record
	// whether every modified path in the phase fell within the allowed-paths
	// join (phase-scopes.yaml ∘ gh-optivem.yaml paths:). Read by the
	// phase_scope_clean gate.
	CtxKeyPhaseScopeClean = "phase_scope_clean"

	// CtxKeyPhaseScopeViolatingPaths is the []string of modified paths
	// check_phase_scope found outside scope. Populated only on violations;
	// consumed by the STOP_SCOPE_VIOLATION user_task to render the
	// human-review payload.
	CtxKeyPhaseScopeViolatingPaths = "phase_scope_violating_paths"
)

type actions struct {
	deps Deps
}

// ---------------------------------------------------------------------------
// Board-backed actions
// ---------------------------------------------------------------------------

// pickTopReady reads the project's Ready column via Tracker.PickReady and
// writes the picked issue's number, URL, title, repo, and opaque handle
// into Context state. Downstream gates and actions read those keys; the
// engine itself does not interpret them.
//
// On an empty Ready column Tracker.PickReady returns tracker.ErrEmptyReady,
// which the action surfaces as Outcome.Err. The driver catches that
// specific sentinel and exits zero rather than crashing — a normal
// "nothing to do" outcome.
func (a actions) pickTopReady(ctx *statemachine.Context) statemachine.Outcome {
	issue, err := a.deps.Tracker.PickReady(context.Background())
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("pick_top_ready: %w", err)}
	}
	writeIssueToContext(ctx, issue)
	fmt.Fprintf(a.deps.Stdout, "Picked top Ready: #%s %s (%s)\n", issue.ID, issue.Title, issue.URL)
	return statemachine.Outcome{}
}

// moveToInProgress sets the picked issue's status to "In progress" via
// Tracker.SetStatus. Reads issue_handle from Context — populated by
// pick_top_ready (board mode) or by the driver's issue-lookup path
// (specific-issue mode).
func (a actions) moveToInProgress(ctx *statemachine.Context) statemachine.Outcome {
	handle := ctx.GetString("issue_handle")
	if handle == "" {
		return statemachine.Outcome{Err: fmt.Errorf("move_to_in_progress: issue_handle not in Context (specific-issue mode requires explicit pre-resolution)")}
	}
	if err := a.deps.Tracker.SetStatus(context.Background(), handle, "In progress"); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("move_to_in_progress: %w", err)}
	}
	fmt.Fprintln(a.deps.Stdout, "Moved card to In progress.")
	return statemachine.Outcome{}
}

// moveToInAcceptance ticks every issue checklist box via
// Tracker.MarkChecklistComplete and sets the item status to "In
// acceptance" via Tracker.SetStatus. Both halves error out hard on
// failure — a missing Status option or a permission failure on edit is
// a misconfiguration the operator must fix before re-running.
func (a actions) moveToInAcceptance(ctx *statemachine.Context) statemachine.Outcome {
	if err := a.markChecklistComplete(ctx); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("move_to_in_acceptance: tick checklist: %w", err)}
	}
	handle := ctx.GetString("issue_handle")
	if handle == "" {
		return statemachine.Outcome{Err: fmt.Errorf("move_to_in_acceptance: issue_handle not in Context")}
	}
	if err := a.deps.Tracker.SetStatus(context.Background(), handle, "In acceptance"); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("move_to_in_acceptance: %w", err)}
	}
	fmt.Fprintln(a.deps.Stdout, "Moved card to In acceptance.")
	return statemachine.Outcome{}
}

// ---------------------------------------------------------------------------
// Classification
// ---------------------------------------------------------------------------

// supportedTicketTypes is the set of native GitHub issue types this pipeline
// knows how to drive. Anything outside it routes to STOP_CLASSIFY_CONFLICT
// — the operator must change the type in GitHub before re-running.
var supportedTicketTypes = map[string]bool{"story": true, "bug": true, "task": true}

// ticketTypeAliases normalizes GitHub's current native-type names to the
// internal vocabulary the rest of the pipeline (parse.go, deriveChangeType,
// process-flow.yaml gates) speaks. GitHub renamed "Story" to "Feature" in
// 2026; this map keeps the rest of the runtime unchanged while accepting
// the new name. See plans/20260515-*-ticket-type-feature-rename-and-config.md
// for the longer-term rename + configurability proposal.
var ticketTypeAliases = map[string]string{"feature": "story"}

// readTicketType resolves the issue's native type via Tracker.Classify
// and writes the lowercased name to `ticket_type`. The native type is
// authoritative — it's set by the Issue Form's `type:` field at filing
// time and cannot drift from a label-based heuristic.
//
// ticket_type_recognized is set to false (routing to STOP_CLASSIFY_CONFLICT)
// in two cases: the issue has no native type set, or the type is not one of
// Feature / Bug / Task (Feature is normalized to "story" internally via
// ticketTypeAliases). Both resolutions are "set a supported type in GitHub
// and re-run" — there is no LLM fallback.
func (a actions) readTicketType(ctx *statemachine.Context) statemachine.Outcome {
	issueNum, err := strconv.Atoi(ctx.GetString("issue_num"))
	if err != nil || issueNum <= 0 {
		return statemachine.Outcome{Err: fmt.Errorf("read_ticket_type: issue_num not set or not a positive integer (%q)", ctx.GetString("issue_num"))}
	}
	issue, err := issueFromContext(ctx)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("read_ticket_type: %w", err)}
	}
	kind, confident, err := a.deps.Tracker.Classify(context.Background(), issue)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("read_ticket_type: %w", err)}
	}
	if !confident || kind == "" {
		ctx.Set("ticket_type_recognized", false)
		fmt.Fprintf(a.deps.Stderr,
			"read_ticket_type: issue #%d has no native issue type — set Feature / Bug / Task in the GitHub UI and re-run.\n",
			issueNum)
		return statemachine.Outcome{}
	}
	if alias, ok := ticketTypeAliases[kind]; ok {
		kind = alias
	}
	if !supportedTicketTypes[kind] {
		ctx.Set("ticket_type_recognized", false)
		fmt.Fprintf(a.deps.Stderr,
			"read_ticket_type: issue #%d has unsupported issue type %q — set Feature / Bug / Task in the GitHub UI and re-run.\n",
			issueNum, kind)
		return statemachine.Outcome{}
	}
	ctx.Set("ticket_type", kind)
	ctx.Set("ticket_type_recognized", true)
	fmt.Fprintf(a.deps.Stdout, "Read ticket type for #%d: %s.\n", issueNum, kind)
	a.printClassifiedSections(ctx, issueNum)
	return statemachine.Outcome{}
}

// readSubtype reads `subtype:*` labels on the ticket and writes the
// trimmed value to `subtype`. The intake flow's GATE_NEEDS_SUBTYPE only
// routes here for task tickets, so behavioral tickets never reach this
// action.
//
// Sets `subtype_ok`: true on a single label (with `subtype` populated),
// false on 0 or 2+ labels. The downstream GATE_SUBTYPE_OK routes to
// STOP_SUBTYPE_MISSING on false so the operator can fix the labels and
// re-run — same shape as read_ticket_type → STOP_CLASSIFY_CONFLICT and
// parse_ticket_body → STOP_PARSE_ERROR.
func (a actions) readSubtype(ctx *statemachine.Context) statemachine.Outcome {
	issue, err := issueFromContext(ctx)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("read_subtype: %w", err)}
	}
	subs, err := a.deps.Tracker.Subtypes(context.Background(), issue)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("read_subtype: %w", err)}
	}
	switch len(subs) {
	case 0:
		ctx.Set("subtype_ok", false)
		fmt.Fprintf(a.deps.Stderr,
			"read_subtype: issue #%s has no subtype:* label — apply exactly one of subtype:system-interface-redesign / subtype:external-system-interface-redesign / subtype:system-implementation-refactoring and re-run.\n",
			issue.ID)
		return statemachine.Outcome{}
	case 1:
		ctx.Set("subtype", subs[0])
		ctx.Set("subtype_ok", true)
		fmt.Fprintf(a.deps.Stdout, "Subtype for #%s: %s.\n", issue.ID, subs[0])
		return statemachine.Outcome{}
	default:
		ctx.Set("subtype_ok", false)
		fmt.Fprintf(a.deps.Stderr,
			"read_subtype: issue #%s has multiple subtype:* labels (%v) — apply exactly one and re-run.\n",
			issue.ID, subs)
		return statemachine.Outcome{}
	}
}

// parseTicketBody is the deterministic markdown parser that replaces the
// three LLM-driven intake agents (atdd-story / atdd-bug / task).
// Reads the issue body, extracts canonical sections by their
// Issue-Form-enforced headings, and validates the required-section set
// for the ticket's type.
//
// Sets six Context fields the downstream flow consumes:
//   - parse_ok: boolean, drives GATE_PARSE_OK.
//   - legacy_acceptance_criteria_section_present: boolean, drives the
//     existing run_legacy_cycle gate.
//   - ticket_checklist: string, the parsed Checklist body — handed to the
//     task agent via clauderun.Options.Checklist so it doesn't have to
//     re-fetch the issue.
//   - ticket_acceptance_criteria: string, the parsed Acceptance Criteria
//     body — handed to write-acceptance-tests via clauderun.Options.AcceptanceCriteria
//     so the write-acceptance-tests agent doesn't have to shell out to
//     `gh issue view` to read scenarios intake already extracted.
//   - ticket_description: string, the parsed Description body — read by
//     materialize_parsed_concepts when composing the parsed-concepts
//     artifact refine-acceptance-criteria reads.
//   - ticket_legacy_acceptance_criteria: string, the parsed Legacy
//     Acceptance Criteria body — same consumer as ticket_description.
//     Distinct from legacy_acceptance_criteria_section_present, which
//     is the boolean used by the run_legacy_cycle gate.
//
// On parse failure (missing required section), parse_ok is set to false
// and the gateway routes to STOP_PARSE_ERROR. Resolution is "fix the
// ticket body to match the form template and re-run."
func (a actions) parseTicketBody(ctx *statemachine.Context) statemachine.Outcome {
	issueNum, err := strconv.Atoi(ctx.GetString("issue_num"))
	if err != nil || issueNum <= 0 {
		return statemachine.Outcome{Err: fmt.Errorf("parse_ticket_body: issue_num not set or not a positive integer (%q)", ctx.GetString("issue_num"))}
	}
	ticketType := ctx.GetString("ticket_type")
	if ticketType == "" {
		return statemachine.Outcome{Err: fmt.Errorf("parse_ticket_body: ticket_type not set — classify_ticket_type must run first")}
	}
	issue, err := issueFromContext(ctx)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("parse_ticket_body: %w", err)}
	}
	sections, err := a.deps.Tracker.ReadSections(context.Background(), issue, intake.CanonicalHeadings)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("parse_ticket_body: %w", err)}
	}
	result, parseErr := intake.ParseSections(sections, ticketType)
	if parseErr != nil {
		ctx.Set("parse_ok", false)
		fmt.Fprintf(a.deps.Stderr, "parse_ticket_body: %v — fix the ticket body and re-run.\n", parseErr)
		return statemachine.Outcome{}
	}
	ctx.Set("parse_ok", true)
	ctx.Set("legacy_acceptance_criteria_section_present", result.LegacyAcceptanceCriteria.Found)
	ctx.Set("ticket_checklist", result.Checklist.Body)
	ctx.Set("ticket_acceptance_criteria", result.AcceptanceCriteria.Body)
	ctx.Set("ticket_description", result.Description.Body)
	ctx.Set("ticket_legacy_acceptance_criteria", result.LegacyAcceptanceCriteria.Body)
	ctx.Set("parsed_section_names", parsedSectionNames(result))
	if ct := deriveChangeType(ticketType, ctx.GetString("subtype")); ct != "" {
		ctx.Set("change_type", ct)
	}
	fmt.Fprintf(a.deps.Stdout, "Parsed #%d (%s): all required sections present.\n", issueNum, ticketType)
	printChecklistSummary(a.deps.Stdout, result.Checklist)
	if total := len(result.Checklist.Items); total > 0 && result.Checklist.CheckedCount() == total {
		if a.deps.Autonomous {
			fmt.Fprintf(a.deps.Stderr, "warning: all %d checklist items are already marked [x] — proceeding anyway in autonomous mode\n", total)
		} else {
			proceed, err := a.confirmPreCheckedChecklist(issueNum, total)
			if err != nil {
				return statemachine.Outcome{Err: fmt.Errorf("parse_ticket_body: %w", err)}
			}
			if !proceed {
				ctx.Set("parse_ok", false)
				fmt.Fprintf(a.deps.Stdout, "Skipped #%d per operator request — checklist was already fully checked.\n", issueNum)
			}
		}
	}
	return statemachine.Outcome{}
}

// confirmPreCheckedChecklist prompts the operator to disambiguate a
// fully-pre-checked checklist: (a) work already done → skip; (b) stale
// checkbox state → proceed. Routes through promptio so an explicit y/n is
// required — bare Enter on this prompt would risk a worktree-discard when
// the work was actually already done.
func (a actions) confirmPreCheckedChecklist(issueNum, total int) (bool, error) {
	fmt.Fprintf(a.deps.Stdout, "\nAll %d checklist items are already marked [x].\n", total)
	fmt.Fprintf(a.deps.Stdout, "This usually means either:\n")
	fmt.Fprintf(a.deps.Stdout, "  (a) the work was already done — run `gh issue close #%d` and skip this cycle.\n", issueNum)
	fmt.Fprintf(a.deps.Stdout, "  (b) the checklist is stale — proceed and the agent will inspect the code.\n\n")
	ok, err := promptio.ConfirmYNVia(a.deps.Prompter, a.deps.Stdout, "Proceed with the cycle?")
	if err != nil {
		return false, fmt.Errorf("confirmation prompt: %w", err)
	}
	return ok, nil
}

// deriveChangeType maps (ticket_type, subtype) to the single-axis
// change_type that drives the run_cycle dispatch. story / bug both
// produce "behavioral"; task tickets use the subtype directly.
// Returns "" when the inputs are insufficient — the caller leaves
// change_type unset and a downstream gate will surface the issue.
func deriveChangeType(ticketType, subtype string) string {
	switch ticketType {
	case "story", "bug":
		return "behavioral"
	case "task":
		switch subtype {
		case "system-interface-redesign",
			"external-system-interface-redesign",
			"system-implementation-refactoring":
			return subtype
		}
	}
	return ""
}

// parsedSectionNames returns the canonical heading names of every section
// that the parser found populated in the ticket body. Order matches the
// canonical document order so the summary reads top-to-bottom.
func parsedSectionNames(r *intake.Result) []string {
	var names []string
	if r.Description.Found {
		names = append(names, intake.SectionDescription)
	}
	if r.AcceptanceCriteria.Found {
		names = append(names, intake.SectionAcceptanceCriteria)
	}
	if r.StepsToReproduce.Found {
		names = append(names, intake.SectionStepsToReproduce)
	}
	if r.Checklist.Found {
		names = append(names, intake.SectionChecklist)
	}
	if r.LegacyAcceptanceCriteria.Found {
		names = append(names, intake.SectionLegacyAcceptanceCriteria)
	}
	return names
}

// reportIntakeSummary prints the consolidated intake outcome — ticket
// number, ticket type, subtype (if any), change_type (if derived), and
// the canonical section names found in the body. The action is the
// single observer-facing checkpoint at the end of github_intake.
func (a actions) reportIntakeSummary(ctx *statemachine.Context) statemachine.Outcome {
	w := a.deps.Stdout
	fmt.Fprintln(w, "Intake summary:")
	if v := ctx.GetString("issue_num"); v != "" {
		fmt.Fprintf(w, "  ticket: #%s\n", v)
	}
	if v := ctx.GetString("ticket_type"); v != "" {
		fmt.Fprintf(w, "  ticket_type: %s\n", v)
	}
	if v := ctx.GetString("subtype"); v != "" {
		fmt.Fprintf(w, "  subtype: %s\n", v)
	}
	if v := ctx.GetString("change_type"); v != "" {
		fmt.Fprintf(w, "  change_type: %s\n", v)
	}
	names, _ := ctx.Get("parsed_section_names").([]string)
	if len(names) > 0 {
		fmt.Fprintf(w, "  parsed sections: %s\n", strings.Join(names, ", "))
	}
	return statemachine.Outcome{}
}

// materializeParsedConcepts writes the parsed-concepts artifact refine-acceptance-criteria
// reads (and update-ticket reads back after refinement) and stashes its
// path into ctx.State["parsed_concepts"] for substitution into both
// agents' ${parsed_concepts} placeholder.
//
// The artifact is a plain markdown file under <run_dir>/parsed-concepts.md
// containing two named H2 sections — Legacy Acceptance Criteria,
// Acceptance Criteria — populated from the strings parse_ticket_body
// already stashed in ctx.State. Description is parsed and stashed
// upstream but deliberately excluded here: refine-acceptance-criteria's contract is
// "rewrite acceptance criteria", not "rewrite prose context", and
// carrying Description through the round-trip risks spurious
// whitespace-normalization diffs back to the ticket source. No
// structural transformation otherwise: the artifact is the in-memory
// parsed sections dropped to disk so the agents have a stable path to
// read and mutate across the CONFIRM_REFINEMENT human gate. (A future
// "extract concepts" upgrade could replace this body with a structured
// representation; the path contract stays the same.)
//
// Always writes the file, even when every section is empty — refine-acceptance-criteria
// then discharges as a no-op (Refinement Changed: no) and update-ticket
// is gated past. An empty artifact is a valid state, not a wiring bug;
// the alternative (skip materialize → ${parsed_concepts} unfilled →
// dispatch error) would conflate "nothing to refine" with "broken
// wiring".
//
// run_dir is seeded into ctx.State by the driver at startup
// (<repoPath>/.gh-optivem/runs/<run-ts>). Missing run_dir is a
// hard error — without it the artifact path is undefined.
func (a actions) materializeParsedConcepts(ctx *statemachine.Context) statemachine.Outcome {
	runDir := ctx.GetString("run_dir")
	if runDir == "" {
		return statemachine.Outcome{Err: fmt.Errorf("materialize_parsed_concepts: run_dir not set in context — driver should have seeded it")}
	}
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("materialize_parsed_concepts: create run dir: %w", err)}
	}
	path := filepath.Join(runDir, "parsed-concepts.md")
	body := renderParsedConcepts(
		ctx.GetString("ticket_legacy_acceptance_criteria"),
		ctx.GetString("ticket_acceptance_criteria"),
	)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("materialize_parsed_concepts: write %s: %w", path, err)}
	}
	ctx.Set("parsed_concepts", path)
	fmt.Fprintf(a.deps.Stdout, "Materialized parsed-concepts artifact at %s.\n", path)
	return statemachine.Outcome{}
}

// renderParsedConcepts composes the artifact body from the two section
// strings parse_ticket_body stashed in ctx.State. Empty sections are
// emitted with their heading and a single blank line so refine-acceptance-criteria sees a
// uniform shape regardless of which sections the ticket carried — easier
// to diff across runs than a body whose section set varies by ticket type.
func renderParsedConcepts(legacyAC, acceptanceCriteria string) string {
	var b strings.Builder
	for _, sec := range []struct {
		heading string
		body    string
	}{
		{intake.SectionLegacyAcceptanceCriteria, legacyAC},
		{intake.SectionAcceptanceCriteria, acceptanceCriteria},
	} {
		fmt.Fprintf(&b, "## %s\n\n", sec.heading)
		if sec.body != "" {
			b.WriteString(strings.TrimRight(sec.body, "\n"))
			b.WriteString("\n\n")
		}
	}
	return b.String()
}

// printChecklistSummary writes a structured summary of the parsed checklist
// to w. It is the operator-facing signal that the checklist arrives
// pre-checked — without it, an "all [x]" intake is invisible on stdout and
// the operator cannot tell (a) the work was already done from (b) a stale
// checklist. Skipped silently when the section is absent (story / bug
// tickets) or has no `- [ ]` / `- [x]` lines.
func printChecklistSummary(w io.Writer, c intake.ChecklistResult) {
	if len(c.Items) == 0 {
		return
	}
	checked := c.CheckedCount()
	total := len(c.Items)
	mixed := checked > 0 && checked < total
	fmt.Fprintf(w, "Checklist (%d items, %d already [x]):\n", total, checked)
	for _, it := range c.Items {
		mark := " "
		suffix := ""
		if it.Checked {
			mark = "x"
			if mixed {
				suffix = " (already done)"
			}
		}
		fmt.Fprintf(w, "  [%s] %s%s\n", mark, it.Text, suffix)
	}
}

// printClassifiedSections fetches three canonical sections via
// Tracker.ReadSections and prints whichever are non-empty: Legacy
// Acceptance Criteria, Acceptance Criteria, and Checklist. Best-effort
// — fetch failures and missing sections are silent.
func (a actions) printClassifiedSections(ctx *statemachine.Context, issueNum int) {
	issue, err := issueFromContext(ctx)
	if err != nil {
		return
	}
	headings := []string{"Legacy Acceptance Criteria", "Acceptance Criteria", "Checklist"}
	sections, err := a.deps.Tracker.ReadSections(context.Background(), issue, headings)
	if err != nil {
		return
	}
	for _, heading := range headings {
		body := sections[heading]
		if body == "" {
			continue
		}
		fmt.Fprintf(a.deps.Stdout, "\n## %s\n\n%s\n", heading, body)
	}
}

// ---------------------------------------------------------------------------
// Smoke test
// ---------------------------------------------------------------------------

// runSmokeTest prompts the user to run the smoke test and report the result.
// v1 ships with a prompt rather than a real docker invocation because the
// stack-up command is repo-specific (`gh optivem system start`). The Outcome's
// Bool is also recorded under `smoke_test_passes` so the downstream gateway
// reads it back through the standard wrapGateway path.
func (a actions) runSmokeTest(ctx *statemachine.Context) statemachine.Outcome {
	yes, err := promptio.ConfirmYNVia(a.deps.Prompter, a.deps.Stdout, "Run the smoke test now and report the result: did it pass?")
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("run_smoke_test: %w", err)}
	}
	ctx.Set("smoke_test_passes", yes)
	if yes {
		fmt.Fprintln(a.deps.Stdout, "Smoke test passed.")
	} else {
		fmt.Fprintln(a.deps.Stderr, "Smoke test failed — flow will route to ASK_SUPPORT.")
	}
	return statemachine.Outcome{Bool: yes}
}

// ---------------------------------------------------------------------------
// Commit / release actions
// ---------------------------------------------------------------------------

// commitPhase creates the phase commit. The message format mirrors
// cycles.md: `<Ticket Title> | <CHANGE TYPE>`. Both pieces come from
// Context — issue_title from pick_top_ready / move_to_in_progress,
// change_type from the call_activity params (substituted into the action's
// params at dispatch time).
func (a actions) commitPhase(ctx *statemachine.Context) statemachine.Outcome {
	title := ctx.GetString("issue_title")
	if title == "" {
		title = "<unknown ticket>"
	}
	changeType := ctx.Params["change_type"]
	if changeType == "" {
		changeType = "CHANGE_TYPE"
	}
	msg := fmt.Sprintf("%s | %s", title, changeType)
	if err := release.Commit(context.Background(), release.CommitOptions{
		Message:   msg,
		Confirm:   a.confirmer(),
		GitRunner: gitReleaseAdapter{a.deps.Git},
		Stdout:    a.deps.Stdout,
	}); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("commit_phase: %w", err)}
	}
	return statemachine.Outcome{}
}

// ---------------------------------------------------------------------------
// Test-mode actions
// ---------------------------------------------------------------------------

// compileAll, compileSystem, compileSystemTests are the three tier-targeted
// compile actions. Each shells out to the matching `gh optivem compile`
// subcommand (see compile_commands.go) and writes CtxKeyCompileOK so the
// shared `compile_ok` gate can route the compile-failed loop to
// WRITE_PROTOTYPES or proceed. The cycle's COMPILE node picks one via the
// `${compile_action}` template param at the call site:
//
//   - WRITE_ACCEPTANCE_TESTS / WRITE_CONTRACT_TESTS → compile_system_tests
//   - IMPLEMENT_DSL / *_DRIVER / IMPLEMENT_SYSTEM    → compile_system
//   - structural_cycle                               → compile_all
//
// Compile failure is NOT surfaced as Outcome.Err — the cycle's compile-failed
// loop is the intended consumer; routing the bool is the correct behaviour.

func (a actions) compileAll(ctx *statemachine.Context) statemachine.Outcome {
	return a.runCompile(ctx, "compile_all", "gh optivem compile")
}

func (a actions) compileSystem(ctx *statemachine.Context) statemachine.Outcome {
	return a.runCompile(ctx, "compile_system", "gh optivem system compile")
}

func (a actions) compileSystemTests(ctx *statemachine.Context) statemachine.Outcome {
	return a.runCompile(ctx, "compile_system_tests", "gh optivem test compile")
}

func (a actions) runCompile(ctx *statemachine.Context, name, cmdLine string) statemachine.Outcome {
	_, err := a.deps.Shell.Run(context.Background(), cmdLine)
	ok := err == nil
	ctx.Set(CtxKeyCompileOK, ok)
	if err != nil {
		fmt.Fprintf(a.deps.Stderr, "%s: %v\n", name, err)
	}
	return statemachine.Outcome{Bool: ok}
}

// runShell prints the about-to-run command as a "$ <cmd>" banner so the
// operator can see which gh-optivem invocation the orchestrator is firing,
// then dispatches it. Centralizes the banner+run pair used by every
// system-visible shell-out (build/start/run-tests/verify); compile tiers
// and change-driven script loops skip the banner because their surrounding
// output already names what's running.
func (a actions) runShell(cmdLine string) ([]byte, error) {
	fmt.Fprintf(a.deps.Stdout, "\n$ %s\n", cmdLine)
	return a.deps.Shell.Run(context.Background(), cmdLine)
}

// runTargetedTests runs each test method named in CtxKeyTestNames against
// the suite captured in CtxKeySuite, classifies any failures as
// compile-vs-runtime, and writes CtxKeyTestsFailedRuntime for the gate.
//
// Reads:
//   - CtxKeySuite (string)        — optional; e.g. "<acceptance-api>". When
//     absent or empty, falls back to testselect.AcceptanceSuites() (the
//     channel-agnostic dispatch path used by the collapsed IMPLEMENT_SYSTEM node).
//   - CtxKeyTestNames ([]string)  — required; method names dispatched one
//     per `gh optivem test run --suite <suite> --test <name>` shell-out
//     for each resolved suite.
//
// Writes:
//   - CtxKeyTestsFailedRuntime (bool) — true iff at least one test failed
//     and every observed failure was a runtime failure (not compile). The
//     RED phase's gate routes downstream only on true; false means the
//     RED loop has not yet stabilised (tests passed, or some failed at
//     compile).
//
// The runtime-vs-compile classifier scans the captured stdout/stderr for
// language-specific compile-error markers (see isCompileFailureOutput).
// It is conservative: any compile marker present demotes the run to
// "not yet runtime-failing".
func (a actions) runTargetedTests(ctx *statemachine.Context) statemachine.Outcome {
	suite := ctx.GetString(CtxKeySuite)
	suites := []string{suite}
	if suite == "" {
		suites = testselect.AcceptanceSuites()
	}
	rawNames, ok := ctx.State[CtxKeyTestNames]
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("run_targeted_tests: %s not set in Context", CtxKeyTestNames)}
	}
	names, ok := rawNames.([]string)
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("run_targeted_tests: %s is %T, want []string", CtxKeyTestNames, rawNames)}
	}
	if len(names) == 0 {
		return statemachine.Outcome{Err: fmt.Errorf("run_targeted_tests: %s is empty", CtxKeyTestNames)}
	}

	// rebuild_before_run hoists a clean rebuild + restart of the SUT out of
	// the per-test loop. `system build --rebuild` (no-cache nuke) and
	// `system start --restart` are issued once for the whole batch — the
	// per-test `test run` shell-outs then run against the fresh image.
	// Today's behavior was per-test, which mainstream CLIs don't do;
	// per-batch is what callers actually want (rebuild between iterations
	// of the WRITE loop, not between every individual test name in one
	// iteration).
	if strings.EqualFold(strings.TrimSpace(ctx.Params["rebuild_before_run"]), "true") {
		buildCmd := "gh optivem system build --rebuild"
		if _, err := a.runShell(buildCmd); err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("run_targeted_tests: build failed: %w", err)}
		}
		runCmd := "gh optivem system start --restart"
		if _, err := a.runShell(runCmd); err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("run_targeted_tests: restart failed: %w", err)}
		}
	}

	runtimeFailures := 0
	compileFailures := 0
	passed := 0
	totalInvocations := 0
	for _, s := range suites {
		for _, name := range names {
			totalInvocations++
			cmd := fmt.Sprintf("gh optivem test run --suite %s --test %s",
				shellEscape(s), shellEscape(name))
			out, err := a.runShell(cmd)
			if err == nil {
				passed++
				continue
			}
			if isCompileFailureOutput(out) {
				compileFailures++
				continue
			}
			runtimeFailures++
		}
	}

	failedRuntime := compileFailures == 0 && runtimeFailures > 0
	ctx.Set(CtxKeyTestsFailedRuntime, failedRuntime)
	allPass := compileFailures == 0 && runtimeFailures == 0 && passed == totalInvocations
	ctx.Set(CtxKeyTestsPass, allPass)
	return statemachine.Outcome{Bool: failedRuntime}
}

// isCompileFailureOutput reports whether the captured test runner output
// contains a language-specific compile-error marker. Conservative match —
// a single hit demotes the failure to "compile".
func isCompileFailureOutput(out []byte) bool {
	s := strings.ToLower(string(out))
	for _, marker := range []string{
		"compilation failed",
		"compile error",
		"cannot find symbol",
		"error cs", // C# compiler error code prefix (e.g. "error CS0103")
		"error ts", // TS compiler error code prefix (e.g. "error TS2304")
		"syntax error",
	} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

// verifyRealSuitePasses runs the suite named in the `verify_real_suite`
// call_activity param against the test methods in CtxKeyTestNames and
// asserts every one passes. Used by WRITE_CONTRACT_TESTS to prove the new contract
// tests describe behaviour that the real external system actually
// honours, before the regular RUN exercises the dockerized stub. AT
// phases leave the param unset; the surrounding `verify_real_required`
// gate routes around this action without invoking it.
//
// Reads:
//   - ctx.Params["verify_real_suite"] (string) — required; the suite
//     placeholder, e.g. "<suite-contract-real>".
//   - CtxKeyTestNames ([]string) — required; method names dispatched one
//     per `gh optivem test run --suite <suite> --test <name>` shell-out.
//
// Writes:
//   - CtxKeyVerifyRealPass (bool) — true iff every test passed; read by
//     the verify_real_pass gate.
//
// Pass/fail is the only signal: classification (compile vs runtime) is
// not relevant — any failure means the contract does not hold against the
// real instance, which is a STOP-and-ask-the-human condition.
func (a actions) verifyRealSuitePasses(ctx *statemachine.Context) statemachine.Outcome {
	suite := strings.TrimSpace(ctx.Params["verify_real_suite"])
	if suite == "" {
		return statemachine.Outcome{Err: fmt.Errorf("verify_real_suite_passes: verify_real_suite param not set")}
	}
	rawNames, ok := ctx.State[CtxKeyTestNames]
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("verify_real_suite_passes: %s not set in Context", CtxKeyTestNames)}
	}
	names, ok := rawNames.([]string)
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("verify_real_suite_passes: %s is %T, want []string", CtxKeyTestNames, rawNames)}
	}
	if len(names) == 0 {
		return statemachine.Outcome{Err: fmt.Errorf("verify_real_suite_passes: %s is empty", CtxKeyTestNames)}
	}
	allPass := true
	for _, name := range names {
		cmd := fmt.Sprintf("gh optivem test run --suite %s --test %s",
			shellEscape(suite), shellEscape(name))
		if _, err := a.runShell(cmd); err != nil {
			allPass = false
		}
	}
	ctx.Set(CtxKeyVerifyRealPass, allPass)
	return statemachine.Outcome{Bool: allPass}
}

// ---------------------------------------------------------------------------
// Phase-scope enforcement Layer 2 (post-phase scripted check)
// ---------------------------------------------------------------------------

// checkPhaseScope is Layer 2 of phase-scope enforcement (plan
// 20260518-1144 item 5). After the agent commits, the action joins:
//
//   - internal/atdd/phase-scopes.yaml (SSoT: BPMN phase id → layer list)
//   - the project's gh-optivem.yaml paths: (layer name → resolved path)
//     plus Family A path-shaped keys in FamilyAPathKeysInScope (system-path
//     today).
//
// It then enumerates the working-tree changes via `git diff --name-only HEAD`
// + `git status --porcelain` (union covers staged, unstaged, and untracked
// paths since the last commit baseline) and checks each modified path
// against the allowed set with directory-aware prefix matching:
// diffPath ∈ scope iff diffPath == P || diffPath.startsWith(P + "/").
//
// Phase id source: the call_activity invoking red_phase_cycle /
// green_phase_cycle passes phase_id: <NODE_ID> in its params; this action
// reads ctx.Params["phase_id"].
//
// Writes:
//   - CtxKeyPhaseScopeClean (bool)            — false on violation
//   - CtxKeyPhaseScopeViolatingPaths ([]string) — populated on violation
//
// The phase_scope_clean gate reads the boolean; the STOP_SCOPE_VIOLATION
// user_task reads the slice to render the human-review payload.
func (a actions) checkPhaseScope(ctx *statemachine.Context) statemachine.Outcome {
	phaseID := ctx.Params["phase_id"]
	if phaseID == "" {
		return statemachine.Outcome{Err: fmt.Errorf("check_phase_scope: phase_id not set in Params — the call_activity invoking red_phase_cycle / green_phase_cycle must pass phase_id")}
	}

	scopes, err := atdd.LoadPhaseScopes()
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("check_phase_scope: %w", err)}
	}
	layers, ok := scopes.Phases[phaseID]
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf(
			"check_phase_scope: phase id %q not in internal/atdd/phase-scopes.yaml — add an entry", phaseID)}
	}

	cfg, err := projectconfig.Load(a.deps.RepoPath)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("check_phase_scope: load gh-optivem.yaml: %w", err)}
	}
	if cfg == nil {
		return statemachine.Outcome{Err: fmt.Errorf("check_phase_scope: gh-optivem.yaml not found under %s", a.deps.RepoPath)}
	}

	allowed, err := resolveLayerPaths(layers, cfg)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("check_phase_scope (%s): %w", phaseID, err)}
	}

	modified, err := a.modifiedPathsSinceHead(context.Background())
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("check_phase_scope: %w", err)}
	}

	var violating []string
	for _, m := range modified {
		if !pathInScope(m, allowed) {
			violating = append(violating, m)
		}
	}
	if len(violating) > 0 {
		ctx.Set(CtxKeyPhaseScopeClean, false)
		ctx.State[CtxKeyPhaseScopeViolatingPaths] = violating
		fmt.Fprintf(a.deps.Stderr,
			"check_phase_scope: %s scope violation — %d path(s) outside scope.\nResolve by: (1) accept the diff if intentional; (2) rewind to an upstream phase; (3) revert and rerun; (4) abort the cycle.\n",
			phaseID, len(violating))
		for _, v := range violating {
			fmt.Fprintf(a.deps.Stderr, "  out-of-scope: %s\n", v)
		}
		return statemachine.Outcome{Bool: false}
	}
	ctx.Set(CtxKeyPhaseScopeClean, true)
	return statemachine.Outcome{Bool: true}
}

// resolveLayerPaths joins a phase's layer list against the project's
// configured paths: Family A path-shaped keys go through their dedicated
// Config accessor (system-path → cfg.System.Path); everything else is a
// Family B key in cfg.SystemTest.Paths. Missing values surface as errors
// rather than silently shrinking the allowed set.
func resolveLayerPaths(layers []string, cfg *projectconfig.Config) ([]string, error) {
	out := make([]string, 0, len(layers))
	for _, layer := range layers {
		if atdd.FamilyAPathKeysInScope[layer] {
			switch layer {
			case "system-path":
				if cfg.System.Path == "" {
					return nil, fmt.Errorf("layer %q resolves to empty system.path in gh-optivem.yaml", layer)
				}
				out = append(out, cfg.System.Path)
			default:
				return nil, fmt.Errorf("layer %q is in FamilyAPathKeysInScope but has no Config accessor", layer)
			}
			continue
		}
		v, ok := cfg.SystemTest.Paths[layer]
		if !ok || v == "" {
			return nil, fmt.Errorf("layer %q not present in gh-optivem.yaml system_test.paths:", layer)
		}
		out = append(out, v)
	}
	return out, nil
}

// pathInScope returns true if diffPath is within any allowed path P
// with directory-aware prefix matching: diffPath == P, or diffPath
// starts with P + "/". Raw HasPrefix(P) is wrong — it would let
// ".../shop2/..." match ".../shop". This contract is shared with the
// `gh optivem process scope` CLI projection.
func pathInScope(diffPath string, allowed []string) bool {
	for _, p := range allowed {
		if diffPath == p || strings.HasPrefix(diffPath, p+"/") {
			return true
		}
	}
	return false
}

// modifiedPathsSinceHead enumerates working-tree paths touched in the
// current phase by unioning `git diff --name-only HEAD` (tracked,
// staged + unstaged) with `git status --porcelain` (also covers untracked
// `??` and rename `R  old -> new` endpoints). Returns a sorted,
// de-duplicated slice so violating-paths reads deterministically.
func (a actions) modifiedPathsSinceHead(ctx context.Context) ([]string, error) {
	seen := map[string]bool{}
	gitArgs := func(extra ...string) []string {
		if a.deps.RepoPath == "" {
			return extra
		}
		return append([]string{"-C", a.deps.RepoPath}, extra...)
	}

	diff, err := a.deps.Git.Run(ctx, gitArgs("diff", "--name-only", "HEAD")...)
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only HEAD: %w", err)
	}
	for _, line := range strings.Split(string(diff), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			seen[line] = true
		}
	}

	status, err := a.deps.Git.Run(ctx, gitArgs("status", "--porcelain")...)
	if err != nil {
		return nil, fmt.Errorf("git status --porcelain: %w", err)
	}
	for _, line := range strings.Split(string(status), "\n") {
		// porcelain v1 format: "XY path" or "XY old -> new"; X is the
		// staged status, Y the unstaged. Two status chars + one space
		// before the path.
		if len(line) < 4 {
			continue
		}
		rest := line[3:]
		if i := strings.Index(rest, " -> "); i >= 0 {
			oldPath := strings.TrimSpace(rest[:i])
			newPath := strings.TrimSpace(rest[i+4:])
			if oldPath != "" {
				seen[oldPath] = true
			}
			if newPath != "" {
				seen[newPath] = true
			}
			continue
		}
		path := strings.TrimSpace(rest)
		if path != "" {
			seen[path] = true
		}
	}

	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths, nil
}

// ---------------------------------------------------------------------------
// Targeted-subset test verification (post driver-adapter WRITE)
// ---------------------------------------------------------------------------

// ctxKeySelectedTestCommands is where selectTests stores the chosen
// command list for runTests to pick up. The structural cycle splits
// selection (CHOOSE_TESTS → select_tests) from execution (RUN_TESTS →
// run_tests) so the BPMN diagram expresses both as separate nodes.
// Empty []string means "skip" (operator chose [n]) and runTests no-ops.
const ctxKeySelectedTestCommands = "selected_test_commands"

// selectTests is the BPMN-pure selection step paired with runTests:
// shows the same operator prompts runTests would otherwise show inline,
// but only records the choice in ctx[selected_test_commands] without
// executing anything. The structural cycle's CHOOSE_TESTS node binds
// to this; AT/CT verify gates skip it and let runTests do menu+exec
// inline.
//
// Outcomes:
//   - Outcome{} on a successful selection (commands written) or skip
//     (empty selection written so the downstream tests_selected gate
//     routes past BUILD/START/RUN to COMMIT).
//   - Outcome{Err} on path-resolution failure or unrecoverable input
//     error. Operator-driven halt is via Ctrl+C; there is no in-prompt
//     reject option.
func (a actions) selectTests(ctx *statemachine.Context) statemachine.Outcome {
	cmds, out := a.gatherTestCommands(ctx)
	if out.Err != nil {
		return out
	}
	ctx.Set(ctxKeySelectedTestCommands, cmds)
	return statemachine.Outcome{}
}

// buildSystem rebuilds the system under test from scratch via
// `gh optivem system build --rebuild`. Hoisted out of run_tests so the
// BPMN diagram shows the build phase as its own node, and gated by
// GATE_TESTS_SELECTED so the rebuild cost is paid only when tests will
// actually run. Halts the cycle on failure with Outcome{Err} — a broken
// build is an infra-class problem, not something the fix-* diagnosis
// agents could recover from at the test-RED gateway.
func (a actions) buildSystem(ctx *statemachine.Context) statemachine.Outcome {
	if _, err := a.runShell("gh optivem system build --rebuild"); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("build_system: %w", err)}
	}
	return statemachine.Outcome{}
}

// startSystem restarts the running system via
// `gh optivem system start --restart`, which stops any prior container,
// recreates it from the freshly-built image, and waits for readiness. Paired
// with buildSystem ahead of run_tests so the in-container implementation
// matches the source the operator just approved. Halts on failure for the
// same reason as buildSystem.
func (a actions) startSystem(ctx *statemachine.Context) statemachine.Outcome {
	if _, err := a.runShell("gh optivem system start --restart"); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("start_system: %w", err)}
	}
	return statemachine.Outcome{}
}

// runTests is the human-driven system-test gate. If an upstream node
// already populated ctx[selected_test_commands] (the BPMN-pure path —
// structural_cycle's CHOOSE_TESTS → ... → RUN_TESTS), runTests reads that
// selection and executes it once. Otherwise it falls back to the
// legacy menu+exec inline (AT/CT verify gates that haven't been
// split): the operator picks scope, the action runs it, on green the
// prompt loops so they can verify additional scopes without leaving
// the cycle; on red the loop exits so the structural gateway can
// dispatch the fix agent.
//
// Prompts (inline path):
//
//  1. "Run system tests?" (y/n via promptio)
//     - n → record nothing, advance
//     - y → fall through to scope prompt
//  2. "Scope?" (one of):
//     [a] all system tests           — `gh optivem test run`
//     [s] some suites                — pick suite ids, run each whole
//     [p] specific tests in a suite  — pick a suite, type test names
//
// Operator-driven halt is via Ctrl+C; there is no in-prompt reject option.
// Suite ids come from `gh optivem test run --list`, so the menu is
// always whatever tests.json declares for the project.
func (a actions) runTests(ctx *statemachine.Context) statemachine.Outcome {
	if preset, ok := ctx.Get(ctxKeySelectedTestCommands).([]string); ok {
		return a.executeAndFinalize(ctx, preset)
	}

	for {
		cmds, out := a.gatherTestCommands(ctx)
		if out.Err != nil {
			return out
		}
		if len(cmds) == 0 {
			return statemachine.Outcome{}
		}
		out = a.executeAndFinalize(ctx, cmds)
		if out.Err != nil {
			return out
		}
		if out.Value == classRed.String() {
			return out
		}

		more, err := promptio.ConfirmYNVia(a.deps.Prompter, a.deps.Stdout, "Run more tests?")
		if err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("run_tests: %w", err)}
		}
		if !more {
			return out
		}
	}
}

// gatherTestCommands prompts the operator in two stages and returns the
// chosen command list. First a strict y/n ("Run system tests?") via
// promptio so unrecognised input loops at the y/n step; on yes a scope
// prompt offers [a]ll / [s]ome suites / [p]ick specific tests. Empty cmds
// with Outcome{} means "no" (skip) — the structural cycle's
// tests_selected gate routes that past BUILD/START/RUN to COMMIT. Cmds
// with Outcome{} means the operator picked a non-empty scope. Operator-
// driven halt is via Ctrl+C; there is no in-prompt reject option.
func (a actions) gatherTestCommands(ctx *statemachine.Context) ([]string, statemachine.Outcome) {
	yes, err := promptio.ConfirmYNVia(a.deps.Prompter, a.deps.Stdout, "Run system tests?")
	if err != nil {
		return nil, statemachine.Outcome{Err: fmt.Errorf("run_tests: %w", err)}
	}
	if !yes {
		fmt.Fprintln(a.deps.Stdout, "Skipping system tests for this cycle.")
		return nil, statemachine.Outcome{}
	}
	return a.gatherTestScope(ctx)
}

// gatherTestScope prompts the operator to pick the scope of the test run
// once they've already said "yes" to running tests at all. Loops on
// unrecognised input and on sub-pickers that return an empty selection
// (e.g. the suite picker with no picks) so the caller sees one outcome
// per scope decision.
func (a actions) gatherTestScope(ctx *statemachine.Context) ([]string, statemachine.Outcome) {
	for {
		ans, err := a.deps.Prompter.Ask("Scope? [a]ll, [s]ome suites, [p]ick specific tests: ")
		if err != nil {
			return nil, statemachine.Outcome{Err: fmt.Errorf("run_tests: %w", err)}
		}
		choice := strings.ToLower(strings.TrimSpace(ans))

		switch choice {
		case "a":
			return []string{"gh optivem test run"}, statemachine.Outcome{}
		case "s":
			cmds, err := a.promptSomeSuites()
			if err != nil {
				fmt.Fprintf(a.deps.Stderr, "%v — try again.\n", err)
				continue
			}
			if len(cmds) == 0 {
				continue
			}
			return cmds, statemachine.Outcome{}
		case "p":
			cmd, err := a.promptSpecificTests()
			if err != nil {
				fmt.Fprintf(a.deps.Stderr, "%v — try again.\n", err)
				continue
			}
			if cmd == "" {
				continue
			}
			return []string{cmd}, statemachine.Outcome{}
		default:
			fmt.Fprintf(a.deps.Stderr, "Unrecognised choice %q — try again.\n", choice)
			continue
		}
	}
}

// executeAndFinalize runs the resolved command list, captures every
// invocation's verifyCommandResult, and stamps verify_class on ctx
// via finalizeVerify. Empty cmds means "skip" → no-op Outcome (gate
// treats absent verify_class as ok). Shared by selectTests-driven
// preset path and the inline runTests menu+exec path.
func (a actions) executeAndFinalize(ctx *statemachine.Context, cmds []string) statemachine.Outcome {
	if len(cmds) == 0 {
		return statemachine.Outcome{}
	}
	results := make([]verifyCommandResult, 0, len(cmds))
	for _, cmd := range cmds {
		results = append(results, a.runVerifyCommand(cmd))
	}
	return a.finalizeVerify(ctx, statemachine.Outcome{}, results)
}

// listSystemSuites shells out to `gh optivem test run --list` and
// returns one suite id per non-empty output line. The action calls this
// at prompt time so the menu always reflects whatever tests.json
// declares — no separate catalog to keep in sync.
func (a actions) listSystemSuites() ([]string, error) {
	out, err := a.deps.Shell.Run(context.Background(), "gh optivem test run --list")
	if err != nil {
		return nil, fmt.Errorf("list suites failed: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	var suites []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			suites = append(suites, line)
		}
	}
	if len(suites) == 0 {
		return nil, errors.New("no suites declared in tests.json")
	}
	return suites, nil
}

// promptSuiteMenu prints a numbered list of suites and asks the user to
// pick one or more by number. `multi` controls whether the prompt
// accepts a comma-separated list. Returns the selected suite ids in
// pick order; an empty answer yields an empty result so the caller can
// loop back to the top-level menu.
func (a actions) promptSuiteMenu(multi bool) ([]string, error) {
	suites, err := a.listSystemSuites()
	if err != nil {
		return nil, err
	}
	fmt.Fprintln(a.deps.Stdout, "Available suites:")
	for i, id := range suites {
		fmt.Fprintf(a.deps.Stdout, "  [%d] %s\n", i+1, id)
	}
	prompt := "Pick one suite (number or name): "
	if multi {
		prompt = "Pick suites (comma-separated, numbers or names): "
	}
	for {
		ans, err := a.deps.Prompter.Ask(prompt)
		if err != nil {
			return nil, err
		}
		picks, err := parsePicks(ans, suites)
		if err != nil {
			fmt.Fprintf(a.deps.Stderr, "%v — try again.\n", err)
			continue
		}
		if !multi && len(picks) > 1 {
			picks = picks[:1]
		}
		out := make([]string, 0, len(picks))
		for _, idx := range picks {
			out = append(out, suites[idx-1])
		}
		return out, nil
	}
}

// promptSomeSuites asks the user which suites to run whole and returns
// one `gh optivem test run --suite <id>` command per pick.
func (a actions) promptSomeSuites() ([]string, error) {
	picked, err := a.promptSuiteMenu(true)
	if err != nil {
		return nil, err
	}
	cmds := make([]string, 0, len(picked))
	for _, id := range picked {
		cmds = append(cmds, fmt.Sprintf("gh optivem test run --suite %s", shellEscape(id)))
	}
	return cmds, nil
}

// promptSpecificTests asks the user to pick one suite and then type the
// test names to run within it. Returns a single `gh optivem test run
// --suite <id> --test <n1> --test <n2>` command, or "" if the user
// declined to name any tests.
func (a actions) promptSpecificTests() (string, error) {
	picked, err := a.promptSuiteMenu(false)
	if err != nil {
		return "", err
	}
	if len(picked) == 0 {
		return "", nil
	}
	suite := picked[0]
	ans, err := a.deps.Prompter.Ask(fmt.Sprintf("Test names in %s (comma-separated): ", suite))
	if err != nil {
		return "", err
	}
	var names []string
	for _, t := range strings.Split(ans, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			names = append(names, t)
		}
	}
	if len(names) == 0 {
		return "", nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "gh optivem test run --suite %s", shellEscape(suite))
	for _, n := range names {
		fmt.Fprintf(&b, " --test %s", shellEscape(n))
	}
	return b.String(), nil
}

// parsePicks parses a comma-separated list of suite selectors. Each
// token is either a 1-based index (e.g. "3") or a suite id (e.g.
// "acceptance-api"); names match case-insensitively. Returns the
// resolved 1-based indices in pick order. Duplicates — including
// number/name pairs that resolve to the same suite — collapse to the
// first occurrence so the operator can paste a sloppy list without
// surprise re-runs.
func parsePicks(s string, suites []string) ([]int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	max := len(suites)
	var picks []int
	seen := make(map[int]bool)
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		var idx int
		if n, err := strconv.Atoi(tok); err == nil {
			if n < 1 || n > max {
				return nil, fmt.Errorf("pick %d out of range (expect 1-%d)", n, max)
			}
			idx = n
		} else {
			lower := strings.ToLower(tok)
			for i, id := range suites {
				if strings.ToLower(id) == lower {
					idx = i + 1
					break
				}
			}
			if idx == 0 {
				return nil, fmt.Errorf("unknown suite %q (expect a number or suite name)", tok)
			}
		}
		if !seen[idx] {
			seen[idx] = true
			picks = append(picks, idx)
		}
	}
	return picks, nil
}

// finalizeVerify aggregates per-command results into a single failure
// class and stashes everything the gateway-and-fix-loop (Item 3) will
// need: ctx.State["verify_class"] for predicate evaluation,
// ctx.State["verify_results"] for the fix agent's prompt template, and
// Outcome.Value so the trace decorator (Item 6) renders RED/INFRA/OK
// instead of the misleading blanket OK.
//
// On `infra` (orchestrator-side blow-up: missing config, missing
// runner, etc.) finalizeVerify halts the run by returning Outcome.Err
// rather than letting the gateway route. Item 5 of the
// verify-failure-dispatch plan: the cwd-bug case stops silently
// advancing into APPROVE_CHANGE, regardless of whether the
// downstream gateway has been wired in yet. The detailed banner is
// printed to stderr first so the operator sees *why* the run halted
// (which infra pattern matched, which command was tried, the cwd) —
// the engine wraps Outcome.Err with the node ID and surfaces it, but
// the captured stderr lines are what point at the actual fix.
//
// When no commands ran (approve-without-running, no driver-adapter
// changes, or an early bailout), Outcome.Value is left empty. The trace
// then falls through to its default "(no result)" rendering, which is
// honest: nothing was verified.
func (a actions) finalizeVerify(ctx *statemachine.Context, out statemachine.Outcome, results []verifyCommandResult) statemachine.Outcome {
	if out.Err != nil || len(results) == 0 {
		return out
	}
	class := aggregateVerifyClass(results)
	ctx.Set("verify_class", class.String())
	ctx.Set("verify_results", results)
	// Pre-format the per-command failures into one human-readable block
	// so the fix-* diagnosis agents' prompt templates can substitute it
	// in via ${verify_results} without the dispatcher needing to import
	// this package's verifyCommandResult type. Skipped on ok/infra: ok
	// needs no agent dispatch, and infra halts at the action level
	// (below).
	if class == classRed {
		ctx.Set("verify_results_text", formatVerifyResultsText(results))
	}
	if class == classInfra {
		a.printInfraHalt(results)
		return statemachine.Outcome{
			Err: errors.New("run_tests: runner failed before any test ran (infra) — see banner above"),
		}
	}
	out.Value = class.String()
	return out
}

// formatVerifyResultsText renders the failed verifyCommandResults as a
// markdown-style block suitable for substitution into the fix-* (both
// fix-unexpected-passing-tests and fix-unexpected-failing-tests)
// prompt's ${verify_results} placeholder. Each failed command becomes
// one block with the command line, the classification label (when
// present), and the captured stdout/stderr the runner produced.
// Successful commands are omitted — they are not what the fix agent
// needs to read.
//
// Output is plain text (no syntax highlighting) so the same string
// renders the same way in any LLM's context window. Ordering follows
// the input slice so the operator can correlate the prompt with the
// inline "(test run failed [class]: ...)" lines they already saw on
// the trace.
func formatVerifyResultsText(results []verifyCommandResult) string {
	var b strings.Builder
	for _, r := range results {
		if r.Class == classOK {
			continue
		}
		fmt.Fprintf(&b, "Command: %s\n", r.Cmd)
		fmt.Fprintf(&b, "Classification: %s", r.Class)
		if r.Label != "" {
			fmt.Fprintf(&b, " (%s)", r.Label)
		}
		fmt.Fprintln(&b)
		out := strings.TrimRight(r.Output, "\n")
		if out == "" {
			fmt.Fprintln(&b, "(no output captured)")
		} else {
			fmt.Fprintln(&b, out)
		}
		fmt.Fprintln(&b)
	}
	return strings.TrimRight(b.String(), "\n")
}

// printInfraHalt writes the user-facing diagnostic for the infra-class
// halt. The banner cites:
//
//   - the friendly classification label from the matched infraPattern
//     (e.g. "missing system config"), so the operator does not have to
//     re-read regex tables to understand which row fired;
//   - the first stderr line from the captured output, which is the
//     literal prefix the runner emitted (e.g. the `ERROR: read system
//     config ./systems.json` from the morning's reproducer);
//   - the exact command tried, including any --suite / --test flags;
//   - the cwd the orchestrator was running from, since the canonical
//     infra failure mode is "verify ran from the wrong directory and
//     couldn't find the runner config".
//
// When the matched label fingerprints the cwd-bug, cross-link the
// sibling plan so the operator does not have to re-diagnose what is
// already a known issue.
func (a actions) printInfraHalt(results []verifyCommandResult) {
	var first verifyCommandResult
	for _, r := range results {
		if r.Class == classInfra {
			first = r
			break
		}
	}
	cwd := a.deps.RepoPath
	if cwd == "" {
		cwd = "."
	}
	w := a.deps.Stderr
	fmt.Fprintln(w)
	fmt.Fprintln(w, "run_tests: runner failed before any test ran.")
	fmt.Fprintf(w, "Classified as: infra (orchestrator-side problem, not SUT) — %s.\n", first.Label)
	fmt.Fprintf(w, "Detail: %s\n", firstNonEmptyLine(first.Output))
	fmt.Fprintf(w, "Tried:  %s\n", first.Cmd)
	fmt.Fprintf(w, "Cwd:    %s\n", cwd)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "This is an orchestrator bug. Halting before human review so the")
	fmt.Fprintln(w, "review prompt isn't asked under false assumptions.")
	if first.Label == "missing system config" {
		fmt.Fprintln(w, "See plans/20260505-220100-verify-runs-from-wrong-cwd.md.")
	}
}

// firstNonEmptyLine returns the first non-blank line from s, trimmed.
// Used by printInfraHalt to surface the runner's leading error line —
// usually the only line a human needs to confirm which infra mode
// fired.
func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

// verifyCommandResult is the captured outcome of one `gh optivem test ...`
// invocation. The verify action collects a slice of these per cycle and
// aggregates them into a single failureClass — see aggregateVerifyClass.
//
// Output is the combined stdout/stderr the runner produced (Shell.Run
// returns one stream); classifyShellErr's regex anchors are runner-prefix
// based, so feeding the combined stream is fine. ExitErr mirrors what
// Shell.Run returned; nil means the command succeeded.
//
// The fix-agent dispatch (Item 3 of the verify-failure-dispatch-fix-agent
// plan) reads this struct out of ctx.State["verify_results"]; the trace
// banner reads the aggregated class out of Outcome.Value (Item 6).
type verifyCommandResult struct {
	Cmd     string
	Output  string
	ExitErr error
	Class   failureClass
	Label   string // populated only for classInfra (the matched pattern's label)
}

// runVerifyCommand shells out and captures the per-command outcome. The
// caller in runTests collects results across commands;
// aggregation into a single class happens in finalizeVerify.
//
// Failures still surface inline as before — the design point ("verification
// is feedback, not gating") stays intact — but the returned result carries
// the classification so a structural-cycle gateway can route on it without
// re-parsing the printed line.
func (a actions) runVerifyCommand(cmd string) verifyCommandResult {
	out, err := a.runShell(cmd)
	// realShell.Run returns stdout in `out` and inlines stderr into err.Error()
	// (as `(stderr: ...)`), so feeding only `string(out)` to the classifier
	// blinds it to the runner's error output — every infra failure ends up
	// classified as red because no infra pattern can match an empty stdout.
	// Combine both streams here so the regex table actually sees the runner's
	// fixed-prefix error lines.
	class, label := classifyShellErr(combineShellText(out, err), err)
	if err != nil {
		fmt.Fprintf(a.deps.Stderr, "(test run failed [%s]: %v — continuing)\n", class, err)
	}
	return verifyCommandResult{
		Cmd:     cmd,
		Output:  string(out),
		ExitErr: err,
		Class:   class,
		Label:   label,
	}
}

// combineShellText folds stdout and any err-embedded stderr into the single
// text blob the classifier scans. realShell wraps the OS error as
// `shell %q: <inner> (stderr: <captured>)`; including err.Error() captures
// that stderr substring without parsing it back out. Test fakes that put
// runner output directly in `out` and use a plain error string still match
// because we always include err.Error() too.
func combineShellText(out []byte, err error) string {
	if err == nil {
		return string(out)
	}
	if len(out) == 0 {
		return err.Error()
	}
	return string(out) + "\n" + err.Error()
}

// aggregateVerifyClass returns the worst class across results. Infra
// dominates red dominates ok: an orchestrator-side failure means we
// never learned what the runner would have said about the SUT, so we
// surface infra over red rather than letting a phantom red trigger the
// fix agent on a problem the SUT can't actually solve.
//
// An empty result slice means no commands ran (approve-without-running,
// no driver-adapter changes); the caller (finalizeVerify) treats that
// case specially and does not stamp Outcome.Value.
func aggregateVerifyClass(results []verifyCommandResult) failureClass {
	worst := classOK
	for _, r := range results {
		if r.Class == classInfra {
			return classInfra
		}
		if r.Class == classRed {
			worst = classRed
		}
	}
	return worst
}

// shellEscape quotes a value for safe insertion into a bash command line.
// We use single quotes so `$`, backticks, and other meta-characters are
// taken literally; embedded single quotes are split-and-rejoined.
func shellEscape(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t'\"`$\\;|&<>(){}[]?*~#!") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ---------------------------------------------------------------------------
// Checklist
// ---------------------------------------------------------------------------

// tickChecklist marks every `- [ ]` item in the issue body as `- [x]`
// via Tracker.MarkChecklistComplete. Idempotent — a body with no
// unchecked items is left untouched and no API call is made.
func (a actions) tickChecklist(ctx *statemachine.Context) statemachine.Outcome {
	if err := a.markChecklistComplete(ctx); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("tick_checklist: %w", err)}
	}
	return statemachine.Outcome{}
}

// markChecklistComplete is the shared helper used by both tick_checklist
// and move_to_in_acceptance — both want every checkbox ticked when the
// cycle completes. Splitting it out lets move_to_in_acceptance call it
// inline without dispatching the action twice. A missing or non-positive
// issue_num is silently skipped (transitions tests and dry-runs).
func (a actions) markChecklistComplete(ctx *statemachine.Context) error {
	if issueNum, err := strconv.Atoi(ctx.GetString("issue_num")); err != nil || issueNum <= 0 {
		return nil
	}
	issue, err := issueFromContext(ctx)
	if err != nil {
		return err
	}
	return a.deps.Tracker.MarkChecklistComplete(context.Background(), issue)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// confirmer adapts the actions.Prompter into a release.Confirmer. The
// release package owns the explicit "ask before every commit" gate and
// requires a Confirmer at the type level; we route every commit through
// the same prompter (via promptio) that owns the rest of the actions' I/O.
func (a actions) confirmer() release.Confirmer {
	return func(prompt string) (bool, error) {
		return promptio.ConfirmYNVia(a.deps.Prompter, a.deps.Stdout, prompt)
	}
}

// issueFromContext builds a tracker.Issue from the conventional Context
// keys pickTopReady writes (issue_num, issue_url, issue_title,
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

// writeIssueToContext mirrors the Tracker.PickReady result into the
// conventional Context keys downstream actions read.
func writeIssueToContext(ctx *statemachine.Context, issue tracker.Issue) {
	ctx.Set("issue_num", issue.ID)
	ctx.Set("issue_url", issue.URL)
	ctx.Set("issue_title", issue.Title)
	ctx.Set("issue_handle", issue.Handle)
}

// ---------------------------------------------------------------------------
// Adapter shims (different runner interfaces across packages must not leak)
// ---------------------------------------------------------------------------

// ghAdapter / gitReleaseAdapter exist because each underlying package
// (board, release) defines its own GhRunner / GitRunner interface — Go's
// structural typing means we can wrap once instead of teaching every
// package to depend on a shared runner type. The wrappers are zero-cost.
type ghAdapter struct{ inner GhRunner }

func (g ghAdapter) Run(ctx context.Context, args ...string) ([]byte, error) {
	return g.inner.Run(ctx, args...)
}

type gitReleaseAdapter struct{ inner GitRunner }

func (g gitReleaseAdapter) Run(ctx context.Context, args ...string) ([]byte, error) {
	return g.inner.Run(ctx, args...)
}

// ---------------------------------------------------------------------------
// Default exec runners + stdin prompter
// ---------------------------------------------------------------------------

type realGh struct{}

func (realGh) Run(ctx context.Context, args ...string) ([]byte, error) {
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

type realGit struct{}

func (realGit) Run(ctx context.Context, args ...string) ([]byte, error) {
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

type realShell struct{}

func (realShell) Run(ctx context.Context, commandLine string) ([]byte, error) {
	// We deliberately route through the user's shell so command lines like
	// `./test-all.sh --sample` and `bash -lc compile-all.sh` work uniformly.
	// On Windows, gh-optivem ships against bash via the Git Bash shim; if
	// that is missing the user gets a clear "executable file not found"
	// from os/exec.
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "bash"
	}
	cmd := exec.CommandContext(ctx, shell, "-c", commandLine)
	// Tee the child's stdio: stream live to the operator's terminal so
	// long-running commands (docker compose build, gradle, etc.) show
	// progress instead of looking hung, and capture into buffers so the
	// returned []byte still carries stdout for callers that parse it
	// (e.g. `gh optivem test run --list`) and stderr is still inlined
	// into the error message on failure.
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	err := cmd.Run()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return stdoutBuf.Bytes(), fmt.Errorf("shell %q: %w (stderr: %s)",
				commandLine, err, strings.TrimSpace(stderrBuf.String()))
		}
		return stdoutBuf.Bytes(), fmt.Errorf("shell %q: %w", commandLine, err)
	}
	return stdoutBuf.Bytes(), nil
}

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
