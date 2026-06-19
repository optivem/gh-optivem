package actions

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

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
//     (e.g. "gh optivem test run")
//   - ctx.Params["suite"]       — optional; appended as --suite=…
//     pins the test category (acceptance,
//     contract-real, contract-stub)
//   - ctx.Params["test-names"]  — optional; appended as --test=…
//     comma-separated list of bare test
//     method names (the writer-agent's
//     emitted test-names, joined via
//     coerceStateValue's []string case)
//   - ctx.Params["message"]     — optional; appended as a positional
//     `"<msg>"` argument when the command
//     starts with `gh optivem commit`,
//     escaped via shellEscape
//
// Writes ctx.State["command-succeeded"] = (exit == 0). For the
// `gh optivem test run` family it additionally stamps
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
	isTestRun := strings.HasPrefix(cmd, "gh optivem test run")
	isCommit := strings.HasPrefix(cmd, "gh optivem commit")
	// `suite` / `test-names` are only meaningful for `gh optivem test run`.
	// They MUST NOT leak into other commands: BPMN call-activities inherit
	// the parent scope's ctx.Params (run.go:168-180), so an outer process
	// that binds `suite:`/`test-names:` for a downstream `verify-tests-*`
	// would otherwise have those flags rendered into every intermediate
	// `system build` / `system start` / `commit` shell-out, which the
	// receiving CLIs reject with "unknown flag: --suite".
	if isTestRun {
		if suite := strings.TrimSpace(ctx.Params["suite"]); suite != "" {
			cmd += " --suite=" + shellEscape(suite)
		}
		if testNames := strings.TrimSpace(ctx.Params["test-names"]); testNames != "" {
			cmd += " --test=" + shellEscape(testNames)
		}
	}
	if isCommit {
		if msg := strings.TrimSpace(ctx.Params["message"]); msg != "" {
			cmd += " " + shellEscape(msg)
		}
	}
	// Lift the permanent WIP gate on the acceptance tests for the
	// orchestrator's own verify runs only. The gated AT methods key on
	// GH_OPTIVEM_RUN_WIP_TESTS (see clauderun.renderGateMarkerExample);
	// we set it to "1" here — and nowhere else — so the child
	// `gh optivem test run`, and the `mvn` / `dotnet` / `playwright` it
	// shells out to, inherits it through the process environment.
	// Operator, CI, and IDE invocations never traverse this path, so the
	// gate keeps the WIP tests silently skipped there. Restored on return
	// so the var cannot leak into a later non-test dispatch in the same
	// (single-threaded) orchestrator process.
	if isTestRun {
		if prev, had := os.LookupEnv(wipTestsEnvVar); had {
			defer os.Setenv(wipTestsEnvVar, prev)
		} else {
			defer os.Unsetenv(wipTestsEnvVar)
		}
		os.Setenv(wipTestsEnvVar, "1")
	}
	result, err := a.runShell(cmd)
	succeeded := err == nil
	ctx.Set("command-succeeded", succeeded)
	if isTestRun {
		if succeeded {
			ctx.Set("test-outcome", "pass")
		} else {
			ctx.Set("test-outcome", "fail")
		}
	}
	if err != nil {
		ctx.Set("failure-kind", "command-failed")
		ctx.Set("command-line", cmd)
		ctx.Set("command-exit-code", result.ExitCode)
		ctx.Set("command-stderr-tail", lastNLines(string(result.Stderr), commandStderrTailLines))
		fmt.Fprintf(a.deps.Stderr, "run-command: %v\n", err)
	} else {
		// Diagnostic keys owned by this action; clear on success so a
		// later success doesn't carry residue from an earlier failure
		// into the trace or into a downstream fix-* prompt via
		// ExpandParams's state-fallback.
		ctx.Unset("failure-kind")
		ctx.Unset("command-line")
		ctx.Unset("command-exit-code")
		ctx.Unset("command-stderr-tail")
		ctx.Unset("verify_failure_output")
		// A green test run ends the verify-tests-pass loop, so the
		// no-progress baseline (check-fix-progress, plan 20260615-1845 Step 4)
		// must not survive into a later verify-tests-pass invocation in the
		// same run, where its first fail would compare against a stale
		// signature. Cleared here for the same reason verify_failure_output is.
		ctx.Unset("fix-prev-failure-signature")
	}
	if isTestRun && !succeeded {
		// Classify the failure so the gateway downstream of run-tests can
		// distinguish "the runner couldn't start" (infra) from "a test
		// assertion failed" (red). Pre-classifier, both showed up as
		// test-outcome="fail" — which the verify-tests-fail phase took as
		// the expected outcome, silently advancing the pipeline past a
		// runner that never produced a report. The verify_classify table
		// is the authoritative pattern set; new infra modes are added as
		// rows there, not as branches here.
		isContractSuite := strings.HasPrefix(strings.TrimSpace(ctx.Params["suite"]), "contract")
		if class, label := classifyShellErr(string(result.Stderr), err, isContractSuite); class == classInfra {
			ctx.Set("test-outcome", "infra")
			ctx.Set("test-infra-label", label)
		}
		ctx.Set("verify_failure_output", formatVerifyFailureOutput(result.Stdout, result.Stderr))
	}
	return statemachine.Outcome{}
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
func formatVerifyFailureOutput(stdout, stderr []byte) string {
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
