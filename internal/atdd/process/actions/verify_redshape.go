// verify_redshape.go — "is this non-zero test run a real red?" check.
//
// classifyShellErr (verify_classify.go) only recognises a runner that never
// started. A runner that started but whose tests then CRASHED — a browser that
// cannot launch (missing Chromium libs → Playwright TargetClosedError), a test
// host abort, a worker OOM, the SUT refusing connections — still exits
// non-zero, and used to read as test-outcome=fail. verify-tests-fail then
// accepted it as the expected red ("Tests Fail Verified") without a single
// test having reached its assertion. This file closes that hole: a non-zero
// run counts as red only when the evidence shows tests executed and failed for
// a test-level reason. Anything else is reclassified infra, which both
// verify-tests-* processes already route to TESTS_INFRA_HALT.
//
// Evidence, strongest first:
//
//  1. Structured: the selected suites' machine-readable reports (TestCountPath:
//     JUnit XML / TRX / Playwright JSON) written by THIS run. Red requires at
//     least one failed test case, no run-level global error, and no failed
//     case whose detail carries a crash signature.
//  2. Fallback (no tests.yaml wired, or a selected suite lacks TestCountPath and
//     the fresh reports show no failure): scan the captured stdout+stderr for
//     the same crash signatures.
//
// "Test-level reason" is deliberately broader than a literal assertion type:
// the expected red of an acceptance test is often a `TODO: DSL` / `TODO:
// System Driver` stub exception, not an AssertionError, so requiring an
// assertion exception class would reject every legitimate red.
package actions

import (
	"regexp"

	"github.com/optivem/gh-optivem/internal/build/runner"
)

// crashPatterns are failure signatures that mean the test could not reach its
// assertion because the environment broke — never a statement about the SUT's
// behaviour. Order only affects which label surfaces.
var crashPatterns = []infraPattern{
	{
		label: "browser could not launch / crashed (Playwright target closed) — check the browser and its system dependencies are installed",
		re: regexp.MustCompile(`(?i)(TargetClosedError|TargetClosedException|target page, context or browser has been closed|` +
			`host system is missing dependencies|browserType\.launch|executable doesn't exist at|error while loading shared libraries|failed to launch (the )?browser)`),
	},
	{
		label: "test process crashed or was aborted before reporting a verdict",
		re: regexp.MustCompile(`(?i)(worker process exited unexpectedly|test host process crashed|the active test run was aborted|` +
			`process 'gradle test executor \d+' finished with non-zero exit value|OutOfMemoryError|JavaScript heap out of memory)`),
	},
	{
		label: "system under test unreachable (connection refused) — is the system running?",
		re:    regexp.MustCompile(`(?i)(ECONNREFUSED|connection refused|java\.net\.ConnectException|actively refused it)`),
	},
}

func matchCrash(s string) (string, bool) {
	for _, p := range crashPatterns {
		if p.re.MatchString(s) {
			return p.label, true
		}
	}
	return "", false
}

// Labels for the structured "no verdict" cases.
const (
	labelNoReport  = "tests did not run — no test report was written by this run (crash, build error, or runner abort), so the failure is not a red"
	labelNoFailure = "test run failed but its report records no failing test — the failure is outside any test (crash, teardown, or runner error), so it is not a red"
	labelGlobalErr = "test run aborted with a run-level error (e.g. global setup) before per-test verdicts — not a red"
)

// classifyRedShape decides whether a non-zero test run (already judged not
// infra by classifyShellErr) is a genuine red. It returns (classRed, "") or
// (classInfra, label). output is the captured stdout+stderr, used only by the
// fallback path.
func classifyRedShape(ev runner.ReportFailures, output string) (failureClass, string) {
	if len(ev.GlobalErrors) > 0 {
		for _, g := range ev.GlobalErrors {
			if label, ok := matchCrash(g); ok {
				return classInfra, label
			}
		}
		return classInfra, labelGlobalErr
	}
	if len(ev.Failures) > 0 {
		// Conservative: one crashed case taints the run. A crashed channel
		// (e.g. UI) would otherwise hide behind a legitimate red on another
		// (e.g. API), leaving that channel's red unverified.
		for _, f := range ev.Failures {
			if label, ok := matchCrash(f.Detail); ok {
				return classInfra, label
			}
		}
		return classRed, ""
	}
	if ev.Complete {
		if ev.FreshReports == 0 {
			return classInfra, labelNoReport
		}
		return classInfra, labelNoFailure
	}
	if label, ok := matchCrash(output); ok {
		return classInfra, label
	}
	return classRed, ""
}
