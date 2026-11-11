package actions

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/testselect"
	"github.com/optivem/gh-optivem/internal/build/runner"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// CtxKeyTestOutcome is the state key run-tests stamps with "pass"|"fail"|"infra"
// so the verify-tests-pass gate can route on the test result.
const CtxKeyTestOutcome = "test-outcome"

// ---------------------------------------------------------------------------
// Shell dispatch
// ---------------------------------------------------------------------------

// runShell prints the about-to-run command as a "$ <cmd>" banner so the
// operator can see which gh-optivem invocation the orchestrator is firing,
// then dispatches it. Used by the BPMN Phase D `run-command` primitive
// below; centralises the banner+run pair so any future shell-out action
// inherits the same trace shape.
func (a actions) runShell(cmdLine string) (ShellResult, error) {
	// "$ <cmdline>" echo pairs with the verbose subprocess output that
	// follows — both Detail level. The BPMN trace line emitted by
	// WriteBpmnTaskTiming below is the Phase-level summary.
	fmt.Fprintf(a.deps.Out.Detail, "\n$ %s\n", cmdLine)
	return a.deps.Shell.Run(context.Background(), cmdLine)
}

// ---------------------------------------------------------------------------
// Shell-escape helper (used by the BPMN Phase D `run-command` primitive)
// ---------------------------------------------------------------------------

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
// BPMN Phase D — LOW execute-command + execute-agent primitives
// ---------------------------------------------------------------------------

// runCommand is the body of the LOW `execute-command` primitive (per
// plans/20260525-2348-bpmn-phase-d-bindings.md Item 1, Q-D5). The
// caller's `call-activity.params:` block is expanded against the parent
// scope before dispatch, so by the time the action fires:
//
//   - ctx.Params["command"]     — the fully-resolved bash command line
//     (e.g. "gh optivem system-test run")
//   - ctx.Params["suite"]       — optional; appended as --suite=…
//     pins the test category (acceptance,
//     contract-real, contract-stub)
//   - ctx.Params["test-names"]  — optional; appended as --test=…
//     comma-separated list of bare test
//     method names (the writer-agent's
//     emitted test-names, joined via
//     coerceStateValue's []string case)
//   - ctx.Params["test-use-cases"] — optional; appended as --use-case=…
//     comma-separated UC ids the refactor
//     cycles narrow their verify run to
//     (resolve-test-use-cases); never
//     bound by an intermediate process, it
//     reaches here by Params inheritance.
//     Empty or absent ⇒ no flag (whole suite)
//   - ctx.Params["message"]     — optional; appended as a positional
//     `"<msg>"` argument when the command
//     starts with `gh optivem commit`,
//     escaped via shellEscape
//
// Writes ctx.State["command-succeeded"] = (exit == 0). For the
// `gh optivem system-test run` family it additionally stamps
// ctx.State["test-outcome"] = "pass"|"fail" so the verify-tests-pass /
// verify-tests-fail gateways downstream of run-tests route without a
// second shell-out. On failure it also stamps a diagnostic payload —
// failure-kind = "command-failed", command-line, command-exit-code,
// command-stderr-tail — which the `fix-command-failed` dispatch consumes
// via its prompt placeholders (see clauderun.Options.Command*).
//
// Does NOT surface command failure as Outcome.Err — the
// execute-command primitive's GATE_COMMAND_SUCCEEDED is the intended
// consumer of the false branch (it dispatches `fix` with
// failure-kind = "command-failed"). Empty `command` is a wiring bug, so
// surfaces as Err.
// wipTestsEnvVar gates the work-in-progress acceptance tests the
// acceptance-test-writer emits. Only the orchestrator's own verify runs
// set it (=1), lifting the gate for that invocation; see runCommand and
// clauderun.renderGateMarkerExample. The literal must stay in sync with
// the value the per-language gate annotations check against.
const wipTestsEnvVar = "GH_OPTIVEM_RUN_WIP_TESTS"

func (a actions) runCommand(ctx *statemachine.Context) statemachine.Outcome {
	cmd := strings.TrimSpace(ctx.Params["command"])
	if cmd == "" {
		return statemachine.Outcome{Err: fmt.Errorf("run-command: command param not set — call-activity must pass `command:`")}
	}
	isTestRun := strings.HasPrefix(cmd, "gh optivem system-test run")
	isCommit := strings.HasPrefix(cmd, "gh optivem commit")
	// `suite` / `test-names` are only meaningful for `gh optivem system-test run`.
	// They MUST NOT leak into other commands: BPMN call-activities inherit
	// the parent scope's ctx.Params (run.go:168-180), so an outer process
	// that binds `suite:`/`test-names:` for a downstream `verify-tests-*`
	// would otherwise have those flags rendered into every intermediate
	// `system build` / `system start` / `commit` shell-out, which the
	// receiving CLIs reject with "unknown flag: --suite".
	cmd, err := a.appendCommandFlags(ctx, cmd, isTestRun, isCommit)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("run-command: %w", err)}
	}
	// Lift the WIP gate on the acceptance tests for the
	// orchestrator's own verify runs only. The gated AT methods key on
	// GH_OPTIVEM_RUN_WIP_TESTS (see clauderun.renderGateMarkerExample);
	// we set it to "1" here — and nowhere else — so the child
	// `gh optivem system-test run`, and the `mvn` / `dotnet` / `playwright` it
	// shells out to, inherits it through the process environment.
	// Operator, CI, and IDE invocations never traverse this path, so the
	// gate keeps the WIP tests silently skipped there. Restored on return
	// so the var cannot leak into a later non-test dispatch in the same
	// (single-threaded) orchestrator process.
	if isTestRun {
		defer liftWipGate()()
	}
	runStarted := time.Now()
	result, err := a.runShell(cmd)
	succeeded := err == nil
	ctx.Set("command-succeeded", succeeded)
	if isTestRun {
		ctx.Set(CtxKeyTestOutcome, testOutcomeLabel(succeeded))
	}
	if err != nil {
		a.recordCommandFailure(ctx, cmd, result, err)
	} else {
		clearCommandDiagnostics(ctx)
	}
	if isTestRun && !succeeded {
		a.classifyTestRunFailure(ctx, result, err, runStarted)
	}
	return statemachine.Outcome{}
}

// appendCommandFlags renders the optional `--suite` / `--test` / `--use-case`
// flags onto a `gh optivem system-test run` command and the `message`
// positional onto a `gh optivem commit` command; any other command is returned
// unchanged.
//
// A commit made inside a resumable unit also carries the unit's `--trailer`
// flags (ticket, unit id, saved state) so a later run can prove the unit done.
func (a actions) appendCommandFlags(ctx *statemachine.Context, cmd string, isTestRun, isCommit bool) (string, error) {
	if isTestRun {
		if suite := strings.TrimSpace(ctx.Params["suite"]); suite != "" {
			cmd += " --suite=" + shellEscape(suite)
		}
		if testNames := strings.TrimSpace(ctx.Params["test-names"]); testNames != "" {
			cmd += " --test=" + shellEscape(testNames)
		}
		if useCases := strings.TrimSpace(ctx.Params[ParamTestUseCases]); useCases != "" {
			cmd += " --use-case=" + shellEscape(useCases)
		}
	}
	if isCommit {
		trailers, err := a.commitTrailers(ctx)
		if err != nil {
			return "", err
		}
		for _, t := range trailers {
			cmd += " --trailer " + shellEscape(t)
		}
		if msg := strings.TrimSpace(ctx.Params["message"]); msg != "" {
			cmd += " " + shellEscape(msg)
		}
	}
	return cmd, nil
}

// commitTrailers returns the resume trailers for a commit, or none when the
// commit is not inside a resumable unit.
func (a actions) commitTrailers(ctx *statemachine.Context) ([]string, error) {
	if ctx.Unit == "" {
		return nil, nil
	}
	if a.deps.Engine == nil {
		return nil, fmt.Errorf("commit inside resumable unit %q but no engine is wired to build its trailers", ctx.Unit)
	}
	if err := a.snapshotAcceptanceReports(ctx); err != nil {
		return nil, err
	}
	return a.deps.Engine.CommitTrailers(ctx)
}

// liftWipGate sets the WIP-tests env var to "1" and returns a function that
// restores its previous state (set to the prior value, or unset).
func liftWipGate() func() {
	prev, had := os.LookupEnv(wipTestsEnvVar)
	os.Setenv(wipTestsEnvVar, "1")
	return func() {
		if had {
			os.Setenv(wipTestsEnvVar, prev)
		} else {
			os.Unsetenv(wipTestsEnvVar)
		}
	}
}

func testOutcomeLabel(succeeded bool) string {
	if succeeded {
		return "pass"
	}
	return "fail"
}

// recordCommandFailure stamps the diagnostic keys the downstream fix-* prompts read.
func (a actions) recordCommandFailure(ctx *statemachine.Context, cmd string, result ShellResult, err error) {
	ctx.Set("failure-kind", "command-failed")
	ctx.Set("command-line", cmd)
	ctx.Set("command-exit-code", result.ExitCode)
	ctx.Set("command-stderr-tail", lastNLines(string(result.Stderr), commandStderrTailLines))
	fmt.Fprintf(a.deps.Stderr, "run-command: %v\n", err)
}

// clearCommandDiagnostics clears the diagnostic keys owned by run-command on
// success so a later success doesn't carry residue from an earlier failure
// into the trace or into a downstream fix-* prompt via ExpandParams's
// state-fallback. It also clears the no-progress baseline
// (check-fix-progress): a green run ends the verify-tests-pass loop, so the
// baseline must not survive into a later invocation where its first fail
// would compare against a stale signature.
func clearCommandDiagnostics(ctx *statemachine.Context) {
	ctx.Unset("failure-kind")
	ctx.Unset("command-line")
	ctx.Unset("command-exit-code")
	ctx.Unset("command-stderr-tail")
	ctx.Unset("verify_failure_output")
	ctx.Unset("fix-prev-failure-signature")
	ctx.Unset(CtxKeyFixLoopEditedFiles)
}

// classifyTestRunFailure distinguishes "the runner couldn't start" or "the
// tests crashed" (infra) from "tests ran and failed" (red) so the gateway
// downstream of run-tests does not take an infra failure as the expected red
// outcome. Two passes: the verify_classify table (runner never started), then
// classifyRedShape (verify_redshape.go), which requires this run's test
// reports — or, lacking them, the output — to show a test-level failure.
func (a actions) classifyTestRunFailure(ctx *statemachine.Context, result ShellResult, err error, runStarted time.Time) {
	suite := strings.TrimSpace(ctx.Params["suite"])
	isContractSuite := strings.HasPrefix(suite, "contract")
	class, label := classifyShellErr(string(result.Stderr), err, isContractSuite)
	if class != classInfra {
		ev, evErr := runner.FailuresInReports(a.deps.TestsConfig, a.deps.TestsCwd, a.verifySuiteIDs(suite), runStarted)
		if evErr != nil {
			// An unreadable fresh report is not evidence of a red either.
			class, label = classInfra, "test report unreadable: "+evErr.Error()
		} else {
			class, label = classifyRedShape(ev, string(result.Stdout)+"\n"+string(result.Stderr))
		}
	}
	if class == classInfra {
		ctx.Set(CtxKeyTestOutcome, "infra")
		ctx.Set("test-infra-label", label)
	}
	ctx.Set("verify_failure_output", formatVerifyFailureOutput(result.Stdout, result.Stderr))
}

// verifySuiteIDs expands the run's `suite` param (possibly a group alias such
// as `acceptance`, possibly comma-separated) to concrete suite ids, the same
// way `gh optivem system-test run --suite` does. Empty means every suite.
func (a actions) verifySuiteIDs(suite string) []string {
	if suite == "" {
		return nil
	}
	var names []string
	for _, n := range strings.Split(suite, ",") {
		if n = strings.TrimSpace(n); n != "" {
			names = append(names, n)
		}
	}
	var projectGroups map[string][]string
	if a.deps.TestsConfig != nil {
		projectGroups = a.deps.TestsConfig.SuiteGroups
	}
	var channels []string
	if a.deps.Config != nil {
		channels = a.deps.Config.Channels
	}
	return testselect.ExpandSuiteGroups(names, projectGroups, channels)
}

// formatVerifyFailureOutput builds the ${verify-failure-output} payload the
// fix-unexpected-{failing,passing}-tests prompts consume. It combines
// runCommand's captured stdout and stderr into a single block, capping
// each stream individually via lastNLines(s, commandStderrTailLines) so
// a chatty runner can't blow the prompt size. The shape is:
//
//   - stdout alone when stderr is empty
//   - stderr alone when stdout is empty
//   - <stdout-tail>\n--- stderr ---\n<stderr-tail> when both are non-empty
//
// Test runners typically print the failing test name + assertion on
// stdout and the stack trace on stderr; preserving both streams gives
// the diagnosing fixer everything the operator saw inline.
//
// The tail block is purely additive over a head window: when the combined
// output contains an error marker (see errorMarkers) that the tail does not
// already show, a window anchored on the first marker is prepended, followed
// by headTailSeparator. The assertion of a failed test usually sits mid-output
// (Gradle/JUnit end with the BUILD FAILED banner), so the tail alone misses
// it. With no marker, or when the window is already inside the tail, the
// result is exactly the tail block.
func formatVerifyFailureOutput(stdout, stderr []byte) string {
	tail := formatVerifyFailureTail(stdout, stderr)
	head := firstErrorWindow(string(stdout)+"\n"+string(stderr), errorMarkers, verifyFailureHeadLines)
	if head == "" || strings.Contains(tail, head) {
		return tail
	}
	return head + headTailSeparator + tail
}

func formatVerifyFailureTail(stdout, stderr []byte) string {
	out := lastNLines(string(stdout), commandStderrTailLines)
	errs := lastNLines(string(stderr), commandStderrTailLines)
	switch {
	case out == "" && errs == "":
		return ""
	case errs == "":
		return out
	case out == "":
		return errs
	default:
		return out + "\n--- stderr ---\n" + errs
	}
}

// verifyFailureHeadLines is the size of the head window (marker line plus
// following lines) prepended to verify_failure_output; budgeted separately
// from commandStderrTailLines so neither block truncates the other.
const verifyFailureHeadLines = 15

const headTailSeparator = "\n--- (later output) ---\n"

// errorMarkers anchor the head window across the SUT languages (Java/Gradle,
// .NET, Playwright/TypeScript, Go).
var errorMarkers = []string{"AssertionError", "Exception", "FAILED", "panic:", "Error:"}

// firstErrorWindow returns the first line containing any marker plus the
// following n-1 lines, or "" when no line matches.
func firstErrorWindow(s string, markers []string, n int) string {
	if s == "" || n <= 0 {
		return ""
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, line := range lines {
		for _, m := range markers {
			if strings.Contains(line, m) {
				end := i + n
				if end > len(lines) {
					end = len(lines)
				}
				return strings.TrimRight(strings.Join(lines[i:end], "\n"), "\n")
			}
		}
	}
	return ""
}

// commandStderrTailLines caps the stderr block stashed in ctx.State for
// the fix-command-failed prompt. 20 lines is enough to carry a typical
// stack trace tail without blowing the prompt size on a runaway log.
const commandStderrTailLines = 20

// lastNLines returns the trailing n non-empty-bounded lines of s, joined
// by "\n". When s has fewer than n lines, returns s with a single
// trailing newline trimmed (so the rendered ${command-stderr-tail}
// block doesn't gain an extra blank line). Used to bound the stderr
// payload fed to the fix-command-failed prompt.
func lastNLines(s string, n int) string {
	if s == "" || n <= 0 {
		return ""
	}
	trimmed := strings.TrimRight(s, "\n")
	lines := strings.Split(trimmed, "\n")
	if len(lines) <= n {
		return trimmed
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}
