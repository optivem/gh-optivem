// Bindings — Go implementations of every service-task `action:` referenced
// in docs/atdd/process/process-flow.yaml.
//
// Actions are the mechanical work of the pipeline: read the project board,
// move cards, classify the ticket, run a smoke test, commit a phase, etc.
// They wrap the existing helpers under runtime/board, runtime/classify, and
// runtime/release where one already exists; everything else is implemented
// directly in this file using the same shell-out + dependency-injection
// pattern (Deps with Gh / Git / Prompter / Stdout, all defaulting to real
// implementations when nil).
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
	"strconv"
	"strings"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/board"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/intake"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/release"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
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
	ProjectURL string // optional — explicit override for board operations
	RepoPath   string // optional — defaults to current working directory
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
// (no argv split) because the academy's compile-all.sh / test-all.sh wrap
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
	r.Register("report_intake_summary", a.reportIntakeSummary)
	r.Register("move_to_in_acceptance", a.moveToInAcceptance)
	r.Register("run_smoke_test", a.runSmokeTest)
	r.Register("compile_all", a.compileAll)
	r.Register("compile_system", a.compileSystem)
	r.Register("compile_system_tests", a.compileSystemTests)
	r.Register("commit_phase", a.commitPhase)
	r.Register("tick_checklist", a.tickChecklist)
	r.Register("select_tests", a.selectTests)
	r.Register("run_tests", a.runTests)
	// red_phase_cycle / green_phase_cycle infrastructure (per
	// plans/20260505-230100-at-ct-cycle-creative-mechanical-split.md): the
	// shared sub-flows' mechanical steps. Compile is dispatched via
	// ${compile_action} resolving to one of compile_{all,system,system_tests}
	// above; the cycle's COMPILE node looks up by templated name at runtime.
	r.Register("run_targeted_tests", a.runTargetedTests)
	r.Register("disable_change_driven", a.disableChangeDriven)
	r.Register("enable_change_driven", a.enableChangeDriven)
	// Optional CT real-vs-stub verification (per AT/CT split plan): runs the
	// suite named in the `verify_real_suite` call_activity param against the
	// just-written tests and asserts every one passes. Driven by the
	// `verify_real_required` gate, which is no-op for AT phases.
	r.Register("verify_real_suite_passes", a.verifyRealSuitePasses)
}

// Context keys consumed by the red_phase_cycle actions. Centralised so the
// agent dispatcher (Step 2 of the AT/CT split) and tests have one place to
// find the contract. The corresponding values are populated from the
// ticket parse (Suite, Language) and the WRITE phase's output (TestNames,
// DisableTargets, DisableReason).
const (
	// CtxKeySuite is the test suite name handed to run_targeted_tests —
	// e.g. "<acceptance-api>", "<suite-contract-real>".
	CtxKeySuite = "suite"

	// CtxKeyTestNames is the []string of test method names run_targeted_tests
	// dispatches against the suite, one per `gh optivem test system --test`
	// invocation.
	CtxKeyTestNames = "test_names"

	// CtxKeyLanguage is the language disable_change_driven hands to
	// ./disable-test.sh: java | csharp | typescript. The script owns the
	// per-language `@Disabled` / `Skip = "..."` / `test.skip(true, "...")`
	// edit syntax.
	CtxKeyLanguage = "language"

	// CtxKeyDisableReason is the reason string written into the disable
	// markup — e.g. "AT - RED - TEST", "CT - RED - TEST". Mirrors the
	// language-equivalents.md contract.
	CtxKeyDisableReason = "disable_reason"

	// CtxKeyDisableTargets is the []string of "<file>:<method>" pairs
	// disable_change_driven applies the disable markup to.
	CtxKeyDisableTargets = "disable_targets"

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
)

type actions struct {
	deps Deps
}

// ---------------------------------------------------------------------------
// Board-backed actions
// ---------------------------------------------------------------------------

// pickTopReady reads the project's Ready column and writes the picked issue
// number, URL, repo, project ID, and item ID into Context state. Downstream
// gates and actions read those keys; the engine itself does not interpret
// them.
//
// On an empty Ready column the action surfaces ErrEmptyReady as Outcome.Err
// so the run halts with a "nothing to do" message. The caller (driver) can
// catch that specific sentinel and exit zero rather than crashing.
func (a actions) pickTopReady(ctx *statemachine.Context) statemachine.Outcome {
	pick, err := board.PickTopReady(context.Background(), board.Options{
		ProjectURL: a.deps.ProjectURL,
		RepoPath:   a.deps.RepoPath,
		GhRunner:   ghAdapter{a.deps.Gh},
	})
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("pick_top_ready: %w", err)}
	}
	ctx.Set("issue_num", strconv.Itoa(pick.IssueNum))
	ctx.Set("issue_url", pick.IssueURL)
	ctx.Set("issue_title", pick.Title)
	ctx.Set("issue_repo", pick.Repo)
	ctx.Set("project_id", pick.ProjectID)
	ctx.Set("item_id", pick.ItemID)
	fmt.Fprintf(a.deps.Stdout, "Picked top Ready: #%d %s (%s)\n", pick.IssueNum, pick.Title, pick.IssueURL)
	return statemachine.Outcome{}
}

// moveToInProgress changes the project item status to "In progress". Reads
// project_id and item_id from Context — populated by pick_top_ready in
// board mode, or seeded by the caller in specific-issue mode.
func (a actions) moveToInProgress(ctx *statemachine.Context) statemachine.Outcome {
	projectID := ctx.GetString("project_id")
	itemID := ctx.GetString("item_id")
	if projectID == "" || itemID == "" {
		// In specific-issue mode the caller may not have populated the
		// board IDs yet — try to resolve them lazily.
		issueNum, err := strconv.Atoi(ctx.GetString("issue_num"))
		if err != nil || issueNum <= 0 {
			return statemachine.Outcome{Err: fmt.Errorf("move_to_in_progress: project_id/item_id not set and issue_num is not a positive integer (%q)", ctx.GetString("issue_num"))}
		}
		// v1 surfaces the gap rather than guessing; the driver wires up
		// resolve-by-issue once Session 3 adds it. For now fail clearly.
		return statemachine.Outcome{Err: fmt.Errorf("move_to_in_progress: project_id/item_id not in Context (specific-issue mode requires explicit pre-resolution)")}
	}
	err := board.MoveToInProgress(context.Background(), projectID, itemID, board.Options{
		ProjectURL: a.deps.ProjectURL,
		RepoPath:   a.deps.RepoPath,
		GhRunner:   ghAdapter{a.deps.Gh},
	})
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("move_to_in_progress: %w", err)}
	}
	fmt.Fprintln(a.deps.Stdout, "Moved card to In progress.")
	return statemachine.Outcome{}
}

// moveToInAcceptance ticks every issue checklist box and changes the project
// item status to "In acceptance". Both halves are best-effort: we report
// each step, and a missing column / field is a hard fail (the column is
// part of the documented kanban shape).
func (a actions) moveToInAcceptance(ctx *statemachine.Context) statemachine.Outcome {
	if err := a.tickRemoteChecklist(ctx); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("move_to_in_acceptance: tick checklist: %w", err)}
	}
	projectID := ctx.GetString("project_id")
	itemID := ctx.GetString("item_id")
	if projectID == "" || itemID == "" {
		return statemachine.Outcome{Err: fmt.Errorf("move_to_in_acceptance: project_id/item_id not in Context")}
	}
	statusFieldID, optionID, err := lookupStatusOption(context.Background(), a.deps.Gh, ctx, "In acceptance")
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("move_to_in_acceptance: %w", err)}
	}
	if _, err := a.deps.Gh.Run(context.Background(),
		"project", "item-edit",
		"--id", itemID,
		"--field-id", statusFieldID,
		"--project-id", projectID,
		"--single-select-option-id", optionID,
	); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("move_to_in_acceptance: gh project item-edit: %w", err)}
	}
	fmt.Fprintln(a.deps.Stdout, "Moved card to In acceptance.")
	return statemachine.Outcome{}
}

// ---------------------------------------------------------------------------
// Classification
// ---------------------------------------------------------------------------

// classifyIssueTypeQuery fetches the native issue type via GraphQL.
// Used in place of `gh issue view --json issueType` because that JSON field
// is not exposed by any released gh CLI as of 2026-05; the GraphQL schema
// has carried `issueType` for some time, so the GraphQL path is the only
// portable way to read it from the CLI today.
const classifyIssueTypeQuery = `query($owner: String!, $name: String!, $number: Int!) { repository(owner: $owner, name: $name) { issue(number: $number) { issueType { name } } } }`

// supportedTicketTypes is the set of native GitHub issue types this pipeline
// knows how to drive. Anything outside it routes to STOP_CLASSIFY_CONFLICT
// — the operator must change the type in GitHub before re-running.
var supportedTicketTypes = map[string]bool{"story": true, "bug": true, "task": true}

// readTicketType reads the issue's native GitHub issue type (Story /
// Bug / Task) and writes the lowercased name to `ticket_type`. The native
// type is authoritative — it's set by the Issue Form's `type:` field at
// filing time and cannot drift from a label-based heuristic.
//
// ticket_type_recognized is set to false (routing to STOP_CLASSIFY_CONFLICT)
// in two cases: the issue has no native type set, or the type is not one of
// Story / Bug / Task. Both resolutions are "set a supported type in GitHub
// and re-run" — there is no LLM fallback.
func (a actions) readTicketType(ctx *statemachine.Context) statemachine.Outcome {
	issueNum, err := strconv.Atoi(ctx.GetString("issue_num"))
	if err != nil || issueNum <= 0 {
		return statemachine.Outcome{Err: fmt.Errorf("read_ticket_type: issue_num not set or not a positive integer (%q)", ctx.GetString("issue_num"))}
	}
	repo := ctx.GetString("issue_repo")
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" {
		return statemachine.Outcome{Err: fmt.Errorf("read_ticket_type: issue_repo must be set as owner/name (got %q)", repo)}
	}
	args := []string{
		"api", "graphql",
		"-f", "owner=" + owner,
		"-f", "name=" + name,
		"-F", "number=" + strconv.Itoa(issueNum),
		"-f", "query=" + classifyIssueTypeQuery,
	}
	out, err := a.deps.Gh.Run(context.Background(), args...)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("read_ticket_type: gh api graphql: %w", err)}
	}
	issueType := extractIssueType(out)
	if issueType == "" {
		ctx.Set("ticket_type_recognized", false)
		fmt.Fprintf(a.deps.Stderr,
			"read_ticket_type: issue #%d has no native issue type — set Story / Bug / Task in the GitHub UI and re-run.\n",
			issueNum)
		return statemachine.Outcome{}
	}
	ticketType := strings.ToLower(issueType)
	if !supportedTicketTypes[ticketType] {
		ctx.Set("ticket_type_recognized", false)
		fmt.Fprintf(a.deps.Stderr,
			"read_ticket_type: issue #%d has unsupported issue type %q — set Story / Bug / Task in the GitHub UI and re-run.\n",
			issueNum, issueType)
		return statemachine.Outcome{}
	}
	ctx.Set("ticket_type", ticketType)
	ctx.Set("ticket_type_recognized", true)
	fmt.Fprintf(a.deps.Stdout, "Read ticket type for #%d: %s.\n", issueNum, ticketType)
	a.printClassifiedSections(ctx, issueNum)
	return statemachine.Outcome{}
}

// extractIssueType pulls .issueType.name out of the GraphQL response (or any
// JSON containing an "issueType" key — jsonFieldRaw byte-searches, so it
// works on the nested .data.repository.issue.issueType envelope too).
// Returns the raw type name as GitHub stores it (e.g. "Story", "Bug", "Task")
// or empty when the issue has no type set.
func extractIssueType(raw []byte) string {
	block, ok := jsonFieldRaw(raw, "issueType")
	if !ok {
		return ""
	}
	if len(block) >= 4 && string(block[:4]) == "null" {
		return ""
	}
	name, _ := jsonFieldString(block, "name")
	return name
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
	issueNum, err := strconv.Atoi(ctx.GetString("issue_num"))
	if err != nil || issueNum <= 0 {
		return statemachine.Outcome{Err: fmt.Errorf("read_subtype: issue_num not set or not a positive integer (%q)", ctx.GetString("issue_num"))}
	}
	args := []string{"issue", "view", strconv.Itoa(issueNum), "--json", "labels"}
	if repo := ctx.GetString("issue_repo"); repo != "" {
		args = append(args, "--repo", repo)
	}
	out, err := a.deps.Gh.Run(context.Background(), args...)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("read_subtype: gh issue view: %w", err)}
	}
	subs := extractSubtypeLabels(out)
	switch len(subs) {
	case 0:
		ctx.Set("subtype_ok", false)
		fmt.Fprintf(a.deps.Stderr,
			"read_subtype: issue #%d has no subtype:* label — apply exactly one of subtype:system-interface-redesign / subtype:external-system-interface-redesign / subtype:system-implementation-change and re-run.\n",
			issueNum)
		return statemachine.Outcome{}
	case 1:
		ctx.Set("subtype", subs[0])
		ctx.Set("subtype_ok", true)
		fmt.Fprintf(a.deps.Stdout, "Subtype for #%d: %s.\n", issueNum, subs[0])
		return statemachine.Outcome{}
	default:
		ctx.Set("subtype_ok", false)
		fmt.Fprintf(a.deps.Stderr,
			"read_subtype: issue #%d has multiple subtype:* labels (%v) — apply exactly one and re-run.\n",
			issueNum, subs)
		return statemachine.Outcome{}
	}
}

// extractSubtypeLabels pulls every `subtype:*` label from `gh issue view
// --json labels`, returning each value with the prefix stripped.
func extractSubtypeLabels(raw []byte) []string {
	arr, ok := jsonFieldRaw(raw, "labels")
	if !ok {
		return nil
	}
	var subs []string
	for _, obj := range splitJSONArray(arr) {
		name, _ := jsonFieldString(obj, "name")
		if strings.HasPrefix(name, "subtype:") {
			subs = append(subs, strings.TrimPrefix(name, "subtype:"))
		}
	}
	return subs
}

// parseTicketBody is the deterministic markdown parser that replaces the
// four LLM-driven intake agents (atdd-story / atdd-bug / atdd-task /
// atdd-chore). Reads the issue body, extracts canonical sections by their
// Issue-Form-enforced headings, and validates the required-section set
// for the ticket's type.
//
// Sets three Context fields the downstream flow consumes:
//   - parse_ok: boolean, drives GATE_PARSE_OK.
//   - legacy_acceptance_criteria_section_present: boolean, drives the
//     existing run_legacy_cycle gate.
//   - ticket_checklist: string, the parsed Checklist body — handed to the
//     task agent via clauderun.Options.Checklist so it doesn't have to
//     re-fetch the issue.
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
	args := []string{"issue", "view", strconv.Itoa(issueNum), "--json", "body"}
	if repo := ctx.GetString("issue_repo"); repo != "" {
		args = append(args, "--repo", repo)
	}
	out, err := a.deps.Gh.Run(context.Background(), args...)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("parse_ticket_body: gh issue view: %w", err)}
	}
	body := extractIssueBody(out)
	result, parseErr := intake.Parse(body, ticketType)
	if parseErr != nil {
		ctx.Set("parse_ok", false)
		fmt.Fprintf(a.deps.Stderr, "parse_ticket_body: %v — fix the ticket body and re-run.\n", parseErr)
		return statemachine.Outcome{}
	}
	ctx.Set("parse_ok", true)
	ctx.Set("legacy_acceptance_criteria_section_present", result.LegacyAcceptanceCriteria.Found)
	ctx.Set("ticket_checklist", result.Checklist.Body)
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
// checkbox state → proceed. Default-N because the destructive direction is
// proceeding when the work is already done; a skipped ticket can be
// re-run, an over-eager edit costs a worktree-discard.
func (a actions) confirmPreCheckedChecklist(issueNum, total int) (bool, error) {
	fmt.Fprintf(a.deps.Stdout, "\nAll %d checklist items are already marked [x].\n", total)
	fmt.Fprintf(a.deps.Stdout, "This usually means either:\n")
	fmt.Fprintf(a.deps.Stdout, "  (a) the work was already done — run `gh issue close #%d` and skip this cycle.\n", issueNum)
	fmt.Fprintf(a.deps.Stdout, "  (b) the checklist is stale — proceed and the agent will inspect the code.\n\n")
	answer, err := a.deps.Prompter.Ask("Proceed with the cycle? [y/N]: ")
	if err != nil {
		return false, fmt.Errorf("confirmation prompt: %w", err)
	}
	yes, _ := parseYesNo(answer)
	return yes, nil
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
			"system-implementation-change":
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

// printClassifiedSections fetches the issue body and prints the three
// canonical sections users want to see after classification: Legacy
// Acceptance Criteria, Acceptance Criteria, and Checklist. Best-effort —
// fetch failures and missing sections are silent. Each section is printed
// only when present in the body.
func (a actions) printClassifiedSections(ctx *statemachine.Context, issueNum int) {
	args := []string{"issue", "view", strconv.Itoa(issueNum), "--json", "body"}
	if repo := ctx.GetString("issue_repo"); repo != "" {
		args = append(args, "--repo", repo)
	}
	out, err := a.deps.Gh.Run(context.Background(), args...)
	if err != nil {
		return
	}
	body := extractIssueBody(out)
	for _, heading := range []string{"Legacy Acceptance Criteria", "Acceptance Criteria", "Checklist"} {
		section, ok := extractIssueSection(body, heading)
		if !ok {
			continue
		}
		fmt.Fprintf(a.deps.Stdout, "\n## %s\n\n%s\n", heading, section)
	}
}

// ---------------------------------------------------------------------------
// Smoke test
// ---------------------------------------------------------------------------

// runSmokeTest prompts the user to run the smoke test and report the result.
// v1 ships with a prompt rather than a real docker invocation because the
// stack-up command is repo-specific (`gh optivem run system`). The Outcome's
// Bool is also recorded under `smoke_test_passes` so the downstream gateway
// reads it back through the standard wrapGateway path.
func (a actions) runSmokeTest(ctx *statemachine.Context) statemachine.Outcome {
	answer, err := a.deps.Prompter.Ask("Run the smoke test now and report the result: did it pass? [y/N]: ")
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("run_smoke_test: %w", err)}
	}
	yes, ok := parseYesNo(answer)
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("run_smoke_test: unrecognised yes/no answer %q", strings.TrimSpace(answer))}
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
//   - AT_RED_TEST / CT_RED_TEST     → compile_system_tests
//   - AT_RED_DSL / *_DRIVER / GREEN → compile_system
//   - structural_cycle              → compile_all
//
// Compile failure is NOT surfaced as Outcome.Err — the cycle's compile-failed
// loop is the intended consumer; routing the bool is the correct behaviour.

func (a actions) compileAll(ctx *statemachine.Context) statemachine.Outcome {
	return a.runCompile(ctx, "compile_all", "gh optivem compile")
}

func (a actions) compileSystem(ctx *statemachine.Context) statemachine.Outcome {
	return a.runCompile(ctx, "compile_system", "gh optivem compile system")
}

func (a actions) compileSystemTests(ctx *statemachine.Context) statemachine.Outcome {
	return a.runCompile(ctx, "compile_system_tests", "gh optivem compile system-tests")
}

func (a actions) runCompile(ctx *statemachine.Context, name, cmdLine string) statemachine.Outcome {
	out, err := a.deps.Shell.Run(context.Background(), cmdLine)
	if len(out) > 0 {
		fmt.Fprintln(a.deps.Stdout, string(out))
	}
	ok := err == nil
	ctx.Set(CtxKeyCompileOK, ok)
	if err != nil {
		fmt.Fprintf(a.deps.Stderr, "%s: %v\n", name, err)
	}
	return statemachine.Outcome{Bool: ok}
}

// runTargetedTests runs each test method named in CtxKeyTestNames against
// the suite captured in CtxKeySuite, classifies any failures as
// compile-vs-runtime, and writes CtxKeyTestsFailedRuntime for the gate.
//
// Reads:
//   - CtxKeySuite (string)        — required; e.g. "<acceptance-api>".
//   - CtxKeyTestNames ([]string)  — required; method names dispatched one
//     per `gh optivem test system --suite <suite> --test <name>` shell-out.
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
	if suite == "" {
		return statemachine.Outcome{Err: fmt.Errorf("run_targeted_tests: %s not set in Context", CtxKeySuite)}
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
	// the per-test loop. `build system --rebuild` (no-cache nuke) and `run
	// system --restart` are issued once for the whole batch — the per-test
	// `test system` shell-outs then run against the fresh image. Today's
	// behavior was per-test, which mainstream CLIs don't do; per-batch is
	// what callers actually want (rebuild between iterations of the WRITE
	// loop, not between every individual test name in one iteration).
	if strings.EqualFold(strings.TrimSpace(ctx.Params["rebuild_before_run"]), "true") {
		buildCmd := "gh optivem build system --rebuild"
		fmt.Fprintf(a.deps.Stdout, "\n$ %s\n", buildCmd)
		if out, err := a.deps.Shell.Run(context.Background(), buildCmd); err != nil {
			if len(out) > 0 {
				fmt.Fprintln(a.deps.Stdout, string(out))
			}
			return statemachine.Outcome{Err: fmt.Errorf("run_targeted_tests: build failed: %w", err)}
		}
		runCmd := "gh optivem run system --restart"
		fmt.Fprintf(a.deps.Stdout, "\n$ %s\n", runCmd)
		if out, err := a.deps.Shell.Run(context.Background(), runCmd); err != nil {
			if len(out) > 0 {
				fmt.Fprintln(a.deps.Stdout, string(out))
			}
			return statemachine.Outcome{Err: fmt.Errorf("run_targeted_tests: restart failed: %w", err)}
		}
	}

	runtimeFailures := 0
	compileFailures := 0
	passed := 0
	for _, name := range names {
		cmd := fmt.Sprintf("gh optivem test system --suite %s --test %s",
			shellEscape(suite), shellEscape(name))
		fmt.Fprintf(a.deps.Stdout, "\n$ %s\n", cmd)
		out, err := a.deps.Shell.Run(context.Background(), cmd)
		if len(out) > 0 {
			fmt.Fprintln(a.deps.Stdout, string(out))
		}
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

	failedRuntime := compileFailures == 0 && runtimeFailures > 0
	ctx.Set(CtxKeyTestsFailedRuntime, failedRuntime)
	allPass := compileFailures == 0 && runtimeFailures == 0 && passed == len(names)
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
		"error cs",     // C# compiler error code prefix (e.g. "error CS0103")
		"error ts",     // TS compiler error code prefix (e.g. "error TS2304")
		"syntax error",
	} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

// disableChangeDriven applies per-language disable markup
// (`@Disabled("reason")` / `[Fact(Skip = "reason")]` / `test.skip(true, "reason")`)
// to the change-driven test methods identified at WRITE. v1 shells out to
// ./disable-test.sh once per target — the script owns the language-specific
// edit syntax (the language-equivalents.md table).
//
// Reads:
//   - CtxKeyLanguage (string)        — required; java | csharp | typescript
//   - CtxKeyDisableReason (string)   — required; the reason written into
//     the markup (e.g. "AT - RED - TEST")
//   - CtxKeyDisableTargets ([]string) — required; one entry per test,
//     formatted "<file>:<method>"
//
// Each target produces:
//
//   ./disable-test.sh <language> "<reason>" <file>:<method>
//
// First failure halts the action with Outcome.Err — committing a partially
// disabled test set would leave the repo in an inconsistent state.
func (a actions) disableChangeDriven(ctx *statemachine.Context) statemachine.Outcome {
	lang := ctx.GetString(CtxKeyLanguage)
	if lang == "" {
		return statemachine.Outcome{Err: fmt.Errorf("disable_change_driven: %s not set in Context", CtxKeyLanguage)}
	}
	reason := ctx.GetString(CtxKeyDisableReason)
	if reason == "" {
		return statemachine.Outcome{Err: fmt.Errorf("disable_change_driven: %s not set in Context", CtxKeyDisableReason)}
	}
	rawTargets, ok := ctx.State[CtxKeyDisableTargets]
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("disable_change_driven: %s not set in Context", CtxKeyDisableTargets)}
	}
	targets, ok := rawTargets.([]string)
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("disable_change_driven: %s is %T, want []string", CtxKeyDisableTargets, rawTargets)}
	}
	if len(targets) == 0 {
		return statemachine.Outcome{Err: fmt.Errorf("disable_change_driven: %s is empty", CtxKeyDisableTargets)}
	}
	for _, target := range targets {
		cmd := fmt.Sprintf("./disable-test.sh %s %s %s",
			shellEscape(lang), shellEscape(reason), shellEscape(target))
		out, err := a.deps.Shell.Run(context.Background(), cmd)
		if len(out) > 0 {
			fmt.Fprintln(a.deps.Stdout, string(out))
		}
		if err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("disable_change_driven (%s): %w", target, err)}
		}
	}
	return statemachine.Outcome{}
}

// enableChangeDriven inverts disableChangeDriven: it removes the
// per-language disable markup matching `reason` from the same change-driven
// test methods. v1 shells out to ./enable-test.sh once per target — the
// script owns the per-language edit syntax (mirroring disable-test.sh).
// Used at the start of green_phase_cycle's parent flow (at_green_system) to
// re-enable tests that the prior RED phase disabled.
//
// Reads:
//   - CtxKeyLanguage (string)        — required; java | csharp | typescript
//   - CtxKeyDisableReason (string)   — required; the reason whose markers
//     this run is removing (e.g. "AT - RED - SYSTEM DRIVER")
//   - CtxKeyDisableTargets ([]string) — required; one entry per test,
//     formatted "<file>:<method>"
//
// First failure halts the action with Outcome.Err — committing a partially
// re-enabled test set would leave the repo in an inconsistent state.
func (a actions) enableChangeDriven(ctx *statemachine.Context) statemachine.Outcome {
	lang := ctx.GetString(CtxKeyLanguage)
	if lang == "" {
		return statemachine.Outcome{Err: fmt.Errorf("enable_change_driven: %s not set in Context", CtxKeyLanguage)}
	}
	reason := ctx.GetString(CtxKeyDisableReason)
	if reason == "" {
		return statemachine.Outcome{Err: fmt.Errorf("enable_change_driven: %s not set in Context", CtxKeyDisableReason)}
	}
	rawTargets, ok := ctx.State[CtxKeyDisableTargets]
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("enable_change_driven: %s not set in Context", CtxKeyDisableTargets)}
	}
	targets, ok := rawTargets.([]string)
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("enable_change_driven: %s is %T, want []string", CtxKeyDisableTargets, rawTargets)}
	}
	if len(targets) == 0 {
		return statemachine.Outcome{Err: fmt.Errorf("enable_change_driven: %s is empty", CtxKeyDisableTargets)}
	}
	for _, target := range targets {
		cmd := fmt.Sprintf("./enable-test.sh %s %s %s",
			shellEscape(lang), shellEscape(reason), shellEscape(target))
		out, err := a.deps.Shell.Run(context.Background(), cmd)
		if len(out) > 0 {
			fmt.Fprintln(a.deps.Stdout, string(out))
		}
		if err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("enable_change_driven (%s): %w", target, err)}
		}
	}
	return statemachine.Outcome{}
}

// verifyRealSuitePasses runs the suite named in the `verify_real_suite`
// call_activity param against the test methods in CtxKeyTestNames and
// asserts every one passes. Used by CT_RED_TEST to prove the new contract
// tests describe behaviour that the real external system actually
// honours, before the regular RUN exercises the dockerized stub. AT
// phases leave the param unset; the surrounding `verify_real_required`
// gate routes around this action without invoking it.
//
// Reads:
//   - ctx.Params["verify_real_suite"] (string) — required; the suite
//     placeholder, e.g. "<suite-contract-real>".
//   - CtxKeyTestNames ([]string) — required; method names dispatched one
//     per `gh optivem test system --suite <suite> --test <name>` shell-out.
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
		cmd := fmt.Sprintf("gh optivem test system --suite %s --test %s",
			shellEscape(suite), shellEscape(name))
		fmt.Fprintf(a.deps.Stdout, "\n$ %s\n", cmd)
		out, err := a.deps.Shell.Run(context.Background(), cmd)
		if len(out) > 0 {
			fmt.Fprintln(a.deps.Stdout, string(out))
		}
		if err != nil {
			allPass = false
		}
	}
	ctx.Set(CtxKeyVerifyRealPass, allPass)
	return statemachine.Outcome{Bool: allPass}
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
// shows the same operator menu runTests would otherwise show inline,
// but only records the choice in ctx[selected_test_commands] without
// executing anything. The structural cycle's CHOOSE_TESTS node binds
// to this; AT/CT verify gates skip it and let runTests do menu+exec
// inline.
//
// Outcomes:
//   - Outcome{} on a successful selection (commands written) or skip
//     [n] (empty selection written so runTests treats it as no-op).
//   - Outcome{Err} on user reject [x], path-resolution failure, or
//     unrecoverable input error.
func (a actions) selectTests(ctx *statemachine.Context) statemachine.Outcome {
	cmds, out := a.gatherTestCommands(ctx)
	if out.Err != nil {
		return out
	}
	ctx.Set(ctxKeySelectedTestCommands, cmds)
	return statemachine.Outcome{}
}

// runTests is the human-driven system-test gate. If an upstream node
// already populated ctx[selected_test_commands] (the BPMN-pure path —
// structural_cycle's CHOOSE_TESTS → RUN_TESTS), runTests reads that
// selection and executes it once. Otherwise it falls back to the
// legacy menu+exec inline (AT/CT verify gates that haven't been
// split): the operator picks scope, the action runs it, on green the
// prompt loops so they can verify additional scopes without leaving
// the cycle; on red the loop exits so the structural gateway can
// dispatch the fix agent.
//
// Top-level menu (inline path):
//
//   [a] all system tests           — `gh optivem test system`
//   [s] some suites                — pick suite ids, run each whole
//   [p] specific tests in a suite  — pick a suite, type test names
//   [n] no tests (approve)         — record nothing, advance
//   [x] reject                     — halt the run
//
// Suite ids come from `gh optivem test system --list`, so the menu is
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

		more, err := a.deps.Prompter.Ask("Run more tests? [y/N]: ")
		if err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("run_tests: %w", err)}
		}
		if yes, _ := parseYesNo(more); !yes {
			return out
		}
	}
}

// gatherTestCommands runs the interactive a/s/p/n/x menu and returns
// the chosen command list. Empty cmds with Outcome{} means the operator
// picked [n] / blank to skip; cmds with Outcome{} means [a]/[s]/[p]
// chose a non-empty scope; nil with Outcome{Err} means [x] reject or
// an input-read error. The re-prompt loop for unrecognised choices and
// empty sub-selections is internal so callers see one outcome per
// top-level choice.
func (a actions) gatherTestCommands(ctx *statemachine.Context) ([]string, statemachine.Outcome) {
	for {
		ans, err := a.deps.Prompter.Ask("Run system tests? [a]ll, [s]ome suites, [p]ick specific tests, [n]one (approve), e[x]it (reject): ")
		if err != nil {
			return nil, statemachine.Outcome{Err: fmt.Errorf("run_tests: %w", err)}
		}
		choice := strings.ToLower(strings.TrimSpace(ans))

		switch choice {
		case "x":
			return nil, statemachine.Outcome{Err: errors.New("run_tests: user rejected and halted the run")}
		case "n", "":
			fmt.Fprintln(a.deps.Stdout, "Skipping system tests for this cycle.")
			return nil, statemachine.Outcome{}
		case "a":
			return []string{"gh optivem test system"}, statemachine.Outcome{}
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

// listSystemSuites shells out to `gh optivem test system --list` and
// returns one suite id per non-empty output line. The action calls this
// at prompt time so the menu always reflects whatever tests.json
// declares — no separate catalog to keep in sync.
func (a actions) listSystemSuites() ([]string, error) {
	out, err := a.deps.Shell.Run(context.Background(), "gh optivem test system --list")
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
	prompt := "Pick one suite (number): "
	if multi {
		prompt = "Pick suites (comma-separated numbers, e.g. 1,3): "
	}
	ans, err := a.deps.Prompter.Ask(prompt)
	if err != nil {
		return nil, err
	}
	picks, err := parsePickNumbers(ans, len(suites))
	if err != nil {
		return nil, err
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

// promptSomeSuites asks the user which suites to run whole and returns
// one `gh optivem test system --suite <id>` command per pick.
func (a actions) promptSomeSuites() ([]string, error) {
	picked, err := a.promptSuiteMenu(true)
	if err != nil {
		return nil, err
	}
	cmds := make([]string, 0, len(picked))
	for _, id := range picked {
		cmds = append(cmds, fmt.Sprintf("gh optivem test system --suite %s", shellEscape(id)))
	}
	return cmds, nil
}

// promptSpecificTests asks the user to pick one suite and then type the
// test names to run within it. Returns a single `gh optivem test system
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
	fmt.Fprintf(&b, "gh optivem test system --suite %s", shellEscape(suite))
	for _, n := range names {
		fmt.Fprintf(&b, " --test %s", shellEscape(n))
	}
	return b.String(), nil
}

// parsePickNumbers parses a comma-separated list of 1-based indices,
// rejecting non-numeric tokens and out-of-range values. Duplicates
// collapse to the first occurrence so the operator can paste a sloppy
// list without surprise re-runs.
func parsePickNumbers(s string, max int) ([]int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var picks []int
	seen := make(map[int]bool)
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		n, err := strconv.Atoi(tok)
		if err != nil {
			return nil, fmt.Errorf("invalid pick %q (expect a number)", tok)
		}
		if n < 1 || n > max {
			return nil, fmt.Errorf("pick %d out of range (expect 1-%d)", n, max)
		}
		if !seen[n] {
			seen[n] = true
			picks = append(picks, n)
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
	// so the fix-verify agent's prompt template can substitute it in via
	// ${verify_results} without the dispatcher needing to import this
	// package's verifyCommandResult type. Skipped on ok/infra: ok needs
	// no agent dispatch, and infra halts at the action level (below).
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
// markdown-style block suitable for substitution into the
// atdd-fix-verify prompt's ${verify_results} placeholder. Each failed
// command becomes one block with the command line, the classification
// label (when present), and the captured stdout/stderr the runner
// produced. Successful commands are omitted — they are not what the
// fix agent needs to read.
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
	fmt.Fprintf(a.deps.Stdout, "\n$ %s\n", cmd)
	out, err := a.deps.Shell.Run(context.Background(), cmd)
	if len(out) > 0 {
		fmt.Fprintln(a.deps.Stdout, string(out))
	}
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

// tickChecklist updates the GitHub issue body to mark every `- [ ]` item as
// `- [x]`. It is a soft action: a missing issue body or a no-op body is
// fine. Errors from `gh issue edit` halt the run because they indicate a
// permission or auth problem.
func (a actions) tickChecklist(ctx *statemachine.Context) statemachine.Outcome {
	if err := a.tickRemoteChecklist(ctx); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("tick_checklist: %w", err)}
	}
	return statemachine.Outcome{}
}

// tickRemoteChecklist is the shared helper used by both tick_checklist and
// move_to_in_acceptance — both want every checkbox ticked when the cycle
// completes. Splitting it out lets move_to_in_acceptance call it inline
// without dispatching the action twice.
func (a actions) tickRemoteChecklist(ctx *statemachine.Context) error {
	issueNum, err := strconv.Atoi(ctx.GetString("issue_num"))
	if err != nil || issueNum <= 0 {
		// Nothing to tick — happens in transitions tests and in dry-runs.
		return nil
	}
	args := []string{"issue", "view", strconv.Itoa(issueNum), "--json", "body"}
	if repo := ctx.GetString("issue_repo"); repo != "" {
		args = append(args, "--repo", repo)
	}
	out, err := a.deps.Gh.Run(context.Background(), args...)
	if err != nil {
		return fmt.Errorf("gh issue view: %w", err)
	}
	body := extractIssueBody(out)
	updated := tickAllCheckboxes(body)
	if updated == body {
		return nil // no `- [ ]` items, nothing to do
	}
	editArgs := []string{"issue", "edit", strconv.Itoa(issueNum), "--body", updated}
	if repo := ctx.GetString("issue_repo"); repo != "" {
		editArgs = append(editArgs, "--repo", repo)
	}
	if _, err := a.deps.Gh.Run(context.Background(), editArgs...); err != nil {
		return fmt.Errorf("gh issue edit: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// confirmer adapts the actions.Prompter into a release.Confirmer. The
// release package owns the explicit "ask before every commit" gate and
// requires a Confirmer at the type level; we route every commit through
// the same prompter that owns the rest of the actions' I/O.
func (a actions) confirmer() release.Confirmer {
	return func(prompt string) (bool, error) {
		ans, err := a.deps.Prompter.Ask(prompt)
		if err != nil {
			return false, err
		}
		yes, _ := parseYesNo(ans)
		return yes, nil
	}
}

// parseYesNo is a tiny duplicate of the gates helper. We do not import gates
// (would create a cycle of registries) — the cost of a five-line copy is
// negligible.
func parseYesNo(s string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "y", "yes", "true", "1":
		return true, true
	case "n", "no", "false", "0", "":
		return false, true
	default:
		return false, false
	}
}

// extractIssueSection finds an H2-or-deeper markdown heading whose text
// matches `heading` (case-insensitive, exact match after dropping leading
// hashes). Returns the section's contents — every line after the heading
// up to (but not including) the next heading at the same depth or
// shallower, with surrounding blank lines trimmed. Returns ok=false when
// the heading is absent.
func extractIssueSection(body, heading string) (string, bool) {
	lines := strings.Split(body, "\n")
	startIdx := -1
	startDepth := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		depth := 0
		for depth < len(trimmed) && trimmed[depth] == '#' {
			depth++
		}
		if depth < 2 {
			continue
		}
		text := strings.TrimSpace(trimmed[depth:])
		if strings.EqualFold(text, heading) {
			startIdx = i + 1
			startDepth = depth
			break
		}
	}
	if startIdx < 0 {
		return "", false
	}
	endIdx := len(lines)
	for i := startIdx; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		depth := 0
		for depth < len(trimmed) && trimmed[depth] == '#' {
			depth++
		}
		if depth <= startDepth {
			endIdx = i
			break
		}
	}
	section := strings.Trim(strings.Join(lines[startIdx:endIdx], "\n"), "\n")
	if section == "" {
		return "", false
	}
	return section, true
}

// extractIssueBody pulls .body out of `gh issue view --json body`. Same
// minimal parser as gates.extractIssueBody — duplicated to keep the
// packages independent.
func extractIssueBody(raw []byte) string {
	const key = `"body"`
	idx := bytes.Index(raw, []byte(key))
	if idx < 0 {
		return ""
	}
	rest := raw[idx+len(key):]
	colon := bytes.IndexByte(rest, ':')
	if colon < 0 {
		return ""
	}
	rest = bytes.TrimLeft(rest[colon+1:], " \t\r\n")
	if len(rest) == 0 || rest[0] != '"' {
		return ""
	}
	rest = rest[1:]
	var sb strings.Builder
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		if c == '\\' && i+1 < len(rest) {
			next := rest[i+1]
			switch next {
			case 'n':
				sb.WriteByte('\n')
			case 'r':
				sb.WriteByte('\r')
			case 't':
				sb.WriteByte('\t')
			default:
				sb.WriteByte(next)
			}
			i++
			continue
		}
		if c == '"' {
			return sb.String()
		}
		sb.WriteByte(c)
	}
	return sb.String()
}

// tickAllCheckboxes returns a copy of body with every `- [ ]` rewritten as
// `- [x]`. Casing of the bracket content is preserved: only an empty box
// (or a box with a single space) is considered unchecked. Already-ticked
// items pass through untouched so the operation is idempotent.
func tickAllCheckboxes(body string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		indent := line[:len(line)-len(trimmed)]
		if !strings.HasPrefix(trimmed, "- [ ]") && !strings.HasPrefix(trimmed, "* [ ]") {
			continue
		}
		// Replace the first `[ ]` with `[x]`, keeping marker and indent.
		updated := strings.Replace(trimmed, "[ ]", "[x]", 1)
		lines[i] = indent + updated
	}
	return strings.Join(lines, "\n")
}

// lookupStatusOption is a smaller, action-local cousin of board's
// lookupStatusField — it accepts an arbitrary option name (e.g. "In
// acceptance") instead of being hard-coded to "In progress". Returns the
// Status field ID and the requested option ID.
func lookupStatusOption(ctx context.Context, gh GhRunner, sCtx *statemachine.Context, optionName string) (fieldID, optionID string, err error) {
	owner, number, err := projectOwnerAndNumber(sCtx)
	if err != nil {
		return "", "", err
	}
	out, err := gh.Run(ctx, "project", "field-list", strconv.Itoa(number), "--owner", owner, "--format", "json")
	if err != nil {
		return "", "", fmt.Errorf("gh project field-list: %w", err)
	}
	field, optionID, err := findStatusOption(out, optionName)
	if err != nil {
		return "", "", err
	}
	return field, optionID, nil
}

// findStatusOption parses field-list JSON and returns (statusFieldID,
// matching optionID). Hand-decoded to avoid importing encoding/json into
// every action — keeps the file small. The shape is well-known.
func findStatusOption(raw []byte, optionName string) (string, string, error) {
	// Find the Status field object.
	statusBlock, ok := findFieldBlock(raw, "Status")
	if !ok {
		return "", "", fmt.Errorf("project has no Status field")
	}
	id, ok := jsonFieldString(statusBlock, "id")
	if !ok {
		return "", "", fmt.Errorf("Status field is missing id")
	}
	// Walk the options array within the Status block.
	options, ok := jsonFieldRaw(statusBlock, "options")
	if !ok {
		return "", "", fmt.Errorf("Status field has no options array")
	}
	target := strings.ToLower(strings.TrimSpace(optionName))
	for _, opt := range splitJSONArray(options) {
		name, _ := jsonFieldString(opt, "name")
		if strings.ToLower(strings.TrimSpace(name)) == target {
			optID, _ := jsonFieldString(opt, "id")
			return id, optID, nil
		}
	}
	return "", "", fmt.Errorf("Status field has no %q option", optionName)
}

// projectOwnerAndNumber resolves the project owner + number from Context.
// Prefers an explicit project_url; otherwise reconstructs from issue_repo
// when it follows the canonical owner/repo form (delegating to gh project
// list via the board package would be cleaner — Session 3 wires that up).
func projectOwnerAndNumber(ctx *statemachine.Context) (string, int, error) {
	if u := ctx.GetString("project_url"); u != "" {
		return parseProjectURL(u)
	}
	return "", 0, fmt.Errorf("project_url not in Context")
}

func parseProjectURL(url string) (string, int, error) {
	const orgPrefix = "https://github.com/orgs/"
	const userPrefix = "https://github.com/users/"
	var rest string
	switch {
	case strings.HasPrefix(url, orgPrefix):
		rest = url[len(orgPrefix):]
	case strings.HasPrefix(url, userPrefix):
		rest = url[len(userPrefix):]
	default:
		return "", 0, fmt.Errorf("not a canonical project URL: %q", url)
	}
	parts := strings.Split(rest, "/")
	if len(parts) < 3 || parts[1] != "projects" {
		return "", 0, fmt.Errorf("not a canonical project URL: %q", url)
	}
	n, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", 0, fmt.Errorf("project number: %w", err)
	}
	return parts[0], n, nil
}

// findFieldBlock returns the JSON object slice for the field with the given
// name. It performs a permissive scan — splits the top-level "fields"
// array, then matches each object's "name". Sufficient for the well-known
// `gh project field-list` shape.
func findFieldBlock(raw []byte, fieldName string) ([]byte, bool) {
	fields, ok := jsonFieldRaw(raw, "fields")
	if !ok {
		return nil, false
	}
	target := strings.ToLower(strings.TrimSpace(fieldName))
	for _, obj := range splitJSONArray(fields) {
		name, _ := jsonFieldString(obj, "name")
		if strings.ToLower(strings.TrimSpace(name)) == target {
			return obj, true
		}
	}
	return nil, false
}

// jsonFieldRaw returns the raw bytes of a top-level JSON field's value.
// Handles strings, objects, and arrays via brace/bracket counting. Stops
// short of full JSON parsing — sufficient for our two-level use.
func jsonFieldRaw(raw []byte, key string) ([]byte, bool) {
	needle := []byte(`"` + key + `"`)
	idx := bytes.Index(raw, needle)
	if idx < 0 {
		return nil, false
	}
	rest := raw[idx+len(needle):]
	colon := bytes.IndexByte(rest, ':')
	if colon < 0 {
		return nil, false
	}
	rest = bytes.TrimLeft(rest[colon+1:], " \t\r\n")
	if len(rest) == 0 {
		return nil, false
	}
	switch rest[0] {
	case '"':
		// Skip the value; not-an-object-or-array but useful to recognise.
		end := matchString(rest)
		if end < 0 {
			return nil, false
		}
		return rest[:end+1], true
	case '{':
		end := matchBraced(rest, '{', '}')
		if end < 0 {
			return nil, false
		}
		return rest[:end+1], true
	case '[':
		end := matchBraced(rest, '[', ']')
		if end < 0 {
			return nil, false
		}
		return rest[:end+1], true
	default:
		// Bare token (number, true, false, null) — read until comma/brace.
		end := bytes.IndexAny(rest, ",}]")
		if end < 0 {
			return rest, true
		}
		return rest[:end], true
	}
}

func jsonFieldString(raw []byte, key string) (string, bool) {
	val, ok := jsonFieldRaw(raw, key)
	if !ok {
		return "", false
	}
	if len(val) < 2 || val[0] != '"' || val[len(val)-1] != '"' {
		return "", false
	}
	return string(val[1 : len(val)-1]), true
}

func matchString(s []byte) int {
	for i := 1; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			continue
		}
		if s[i] == '"' {
			return i
		}
	}
	return -1
}

func matchBraced(s []byte, open, close byte) int {
	depth := 0
	inStr := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			if c == '\\' && i+1 < len(s) {
				i++
				continue
			}
			if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func splitJSONArray(arr []byte) [][]byte {
	if len(arr) < 2 || arr[0] != '[' || arr[len(arr)-1] != ']' {
		return nil
	}
	body := arr[1 : len(arr)-1]
	var out [][]byte
	depth := 0
	inStr := false
	start := 0
	for i := 0; i < len(body); i++ {
		c := body[i]
		if inStr {
			if c == '\\' && i+1 < len(body) {
				i++
				continue
			}
			if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, bytes.TrimSpace(body[start:i]))
				start = i + 1
			}
		}
	}
	tail := bytes.TrimSpace(body[start:])
	if len(tail) > 0 {
		out = append(out, tail)
	}
	return out
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
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out, fmt.Errorf("shell %q: %w (stderr: %s)",
				commandLine, err, strings.TrimSpace(stderr.String()))
		}
		return out, fmt.Errorf("shell %q: %w", commandLine, err)
	}
	return out, nil
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
