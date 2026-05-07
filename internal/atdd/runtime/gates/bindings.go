// Bindings — Go implementations of every gateway `binding:` referenced in
// docs/atdd/process/process-flow.yaml.
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
)

// Deps bundles the side-effecting collaborators every binding may need.
// All fields are optional — a zero-value Deps falls back to real shell-outs
// and the OS stdin/stdout. Tests pass non-nil fakes for hermeticity.
type Deps struct {
	Gh       GhRunner
	Git      GitRunner
	Prompter Prompter
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
	return d
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
	// `verify_real_suite` call_activity param (set only by CT_RED_TEST today),
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
		"DSL interface changed in this phase? [y/N]: ")
}

func (b bindings) externalSystemDriverInterfaceChanged(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"external_system_driver_interface_changed",
		"External System Driver interface changed? [y/N]: ")
}

func (b bindings) systemDriverInterfaceChanged(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"system_driver_interface_changed",
		"System Driver interface changed? [y/N]: ")
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
// system-implementation-change. Falls back to a prompt for
// hand-debugging if the upstream derivation hasn't run.
func (b bindings) changeType(ctx *statemachine.Context) statemachine.Outcome {
	v := ctx.GetString("change_type")
	if v != "" {
		return statemachine.Outcome{Value: v}
	}
	answer, err := b.deps.Prompter.Ask(
		"Change type? (behavioral | system-interface-redesign | external-system-interface-redesign | system-implementation-change): ")
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("change_type: %w", err)}
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	switch answer {
	case "behavioral",
		"system-interface-redesign",
		"external-system-interface-redesign",
		"system-implementation-change":
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
		"Subtype? (system-interface-redesign | external-system-interface-redesign | system-implementation-change): ")
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("subtype: %w", err)}
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	switch answer {
	case "system-interface-redesign",
		"external-system-interface-redesign",
		"system-implementation-change":
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
//   - "ok"       — green (or no commands ran). Continue to STOP_STRUCT_TEST.
//   - "red"      — first red of this cycle. Increments verify_retries and
//                  returns "red" so the gateway routes to FIX_STRUCT_VERIFY,
//                  which dispatches atdd-fix-verify and loops back to
//                  CHOOSE_TESTS so the operator can re-pick scope.
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
		"Ticket type recognized? [Y/n]: ")
}

// subtypeOK reads the flag set by the classify_subtype service task.
// true → exactly one subtype:* label was found; false → 0 or 2+, route to
// STOP_SUBTYPE_MISSING. Falls back to a prompt for hand-debugging.
func (b bindings) subtypeOK(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"subtype_ok",
		"Subtype label detected? [Y/n]: ")
}

// parseOK reads the parse-success flag set by the parse_ticket_body
// service task. true → intake completes; false → STOP_PARSE_ERROR. Falls
// back to a prompt for hand-debugging.
func (b bindings) parseOK(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"parse_ok",
		"Ticket body parsed OK? [Y/n]: ")
}

// legacyAcceptanceCriteriaSectionPresent reads the issue body via `gh issue view` and
// scans for an H2 heading named `Legacy Acceptance Criteria`. Falls back to a prompt
// when no issue number is in the Context (off-board mode).
func (b bindings) legacyAcceptanceCriteriaSectionPresent(ctx *statemachine.Context) statemachine.Outcome {
	if v := ctx.Get("legacy_acceptance_criteria_section_present"); v != nil {
		return outcomeFromBoolish(v)
	}
	issueNum := ctx.GetString("issue_num")
	if issueNum == "" {
		return b.boolGate(ctx,
			"legacy_acceptance_criteria_section_present",
			"Legacy Acceptance Criteria section present in the issue? [y/N]: ")
	}
	args := []string{"issue", "view", issueNum, "--json", "body"}
	if repo := ctx.GetString("issue_repo"); repo != "" {
		args = append(args, "--repo", repo)
	}
	out, err := b.deps.Gh.Run(context.Background(), args...)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("legacy_acceptance_criteria_section_present: gh issue view: %w", err)}
	}
	body := extractIssueBody(out)
	return statemachine.Outcome{Bool: containsLegacyAcceptanceCriteriaHeading(body)}
}

// externalSystemDriverExists is asked once at the top of the onboarding
// sub-flow. The semantic check is "does this repo already have a driver
// implementation for the external system?" — file-system inspection is
// possible but the path schema varies per consumer, so v1 prompts.
func (b bindings) externalSystemDriverExists(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"external_system_driver_exists",
		"External System Driver already exists in this repo? [y/N]: ")
}

// externalSystemTestInstanceAccessible is the second onboarding gate.
// Same prompt-or-Context shape as the driver-exists check.
func (b bindings) externalSystemTestInstanceAccessible(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"external_system_test_instance_accessible",
		"Test instance of the external system accessible? [y/N]: ")
}

// smokeTestPasses is set by the run_smoke_test service task's Outcome.
// When called without that upstream having run (e.g. a transitions test
// driving the gateway directly), fall back to a prompt for the sake of
// hand-debugging.
func (b bindings) smokeTestPasses(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"smoke_test_passes",
		"Smoke test passed? [y/N]: ")
}

// compileOK reads the `compile_ok` flag set by the compile_targeted action.
// true → continue to the RUN node; false → route to WRITE_PROTOTYPES so the
// agent adds prototype methods (TODO: DSL / TODO: Driver / ...) for whatever
// the WRITE phase referenced that does not yet exist. Falls back to a prompt
// for hand-debugging when no upstream action ran.
func (b bindings) compileOK(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"compile_ok",
		"Compile passed? [Y/n]: ")
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
		"Tests failed at runtime (not compile)? [Y/n]: ")
}

// testsPass reads the `tests_pass` flag set by run_targeted_tests. true →
// every test in the run passed, route to GREEN_END; false → at least one
// failed (compile or runtime), route to STOP_GREEN_TEST_FAIL so the human
// can review before the agent re-dispatches. Used by green_phase_cycle.
// Falls back to a prompt for hand-debugging.
func (b bindings) testsPass(ctx *statemachine.Context) statemachine.Outcome {
	return b.boolGate(ctx,
		"tests_pass",
		"All tests passed? [Y/n]: ")
}

// verifyRealRequired routes the optional "verify against real suite" branch
// of red_phase_cycle. Reads the `verify_real_suite` param the calling
// call_activity stamped onto Context.Params: a non-empty value means the
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
		"Real-suite verification passed? [Y/n]: ")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// boolGate is the canonical Context-or-prompt shape: read an existing value,
// otherwise ask the user a yes/no question and coerce the answer.
func (b bindings) boolGate(ctx *statemachine.Context, key, prompt string) statemachine.Outcome {
	if v, ok := ctx.State[key]; ok {
		return outcomeFromBoolish(v)
	}
	answer, err := b.deps.Prompter.Ask(prompt)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("%s: %w", key, err)}
	}
	yes, ok := parseYesNo(answer)
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("%s: unrecognised yes/no answer %q", key, strings.TrimSpace(answer))}
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
		yes, ok := parseYesNo(t)
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

// parseYesNo accepts the yes/no shorthands the academy prompts use across
// the rest of the toolchain: "y", "yes", "true", "1" → true; "n", "no",
// "false", "0", "" → false. Unrecognised replies fail the second-return
// flag so the caller can surface "unrecognised answer" instead of silently
// counting them as `no`.
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

// extractIssueBody pulls .body out of a `gh issue view --json body` payload
// without dragging encoding/json into the gate body — keeps the dependency
// surface small. The format is well-defined: a JSON object with a string
// "body" field. We do a minimal hand-parse to find the body value.
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
	// Walk until the matching closing quote, honouring \\ escapes and \" escapes.
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

// containsLegacyAcceptanceCriteriaHeading scans an issue body for an H2 (or deeper)
// markdown heading whose text matches "Legacy Acceptance Criteria" case-insensitively.
// Per cycles.md, the section is conventionally `## Legacy Acceptance Criteria`.
func containsLegacyAcceptanceCriteriaHeading(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Drop leading hashes and any single space separator.
		text := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		if strings.EqualFold(text, "Legacy Acceptance Criteria") {
			return true
		}
	}
	return false
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
