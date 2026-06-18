// verify_classify.go — failure classification for `runVerifyCommand`.
//
// The verify action shells out to `gh optivem test ...`. Two very
// different failure modes can come back through the same `(stdout,
// stderr, err)` channel:
//
//   - infra: the runner never produced a test report. Examples are
//     missing config files (the cwd bug), missing language toolchains,
//     a permission-denied executable, or docker not reachable. These
//     are *orchestrator-side* problems; the SUT may be perfectly fine.
//   - red: the runner ran tests and at least one failed. This is a
//     real signal about the system under test.
//
// Earlier code lumped both into "test run failed: ... — continuing",
// which let the structural cycle quietly advance to human review with
// zero signal about whether anything had actually been verified. The
// classifier below is the pure-function half of fixing that; the
// gateway and fix-agent dispatch land in later items of the same
// plan.
package actions

import "regexp"

// failureClass labels the outcome of a single `runVerifyCommand`
// invocation. The state machine uses this to decide whether to halt
// (infra), dispatch a fix agent (red on a structural cycle), or
// continue (ok, or red on a WRITE cycle where RED is expected).
type failureClass int

const (
	classOK failureClass = iota
	classInfra
	classRed
)

// String renders the class as the lowercase identifier the gateway
// binding compares against. Kept short so the trace line ("RED
// RUN_TESTS ...") is easy to scan.
func (c failureClass) String() string {
	switch c {
	case classOK:
		return "ok"
	case classInfra:
		return "infra"
	case classRed:
		return "red"
	default:
		return "unknown"
	}
}

// infraPattern is one row of the classification table: a regex against
// stderr/stdout combined, with a short label that names *why* this
// looks like infra. The label is what the halt message prints back to
// the user, so it should describe the orchestrator-side problem in
// human terms (not just echo the matched substring).
type infraPattern struct {
	label string
	re    *regexp.Regexp
}

// infraPatterns is the table classifyShellErr walks. Order is
// significant only for the label that surfaces — the first match wins.
// Patterns are deliberately conservative: a real red test run can
// produce noisy stderr, so we only flag infra when the wording is
// unambiguous about the runner not having started.
//
// Adding a row: prefer matching the *runner's* fixed prefix (e.g.
// "ERROR: read system config") over the OS error suffix, since the
// suffix varies by platform.
var infraPatterns = []infraPattern{
	{
		label: "missing system config",
		re:    regexp.MustCompile(`(?i)read system config[^\n]*open [^\n]*\.json`),
	},
	{
		label: "missing system config",
		re:    regexp.MustCompile(`(?i)open [^\n]*\.json:[^\n]*(no such file|cannot find the file)`),
	},
	{
		label: "missing executable",
		re:    regexp.MustCompile(`(?i)executable file not found`),
	},
	{
		label: "missing executable",
		re:    regexp.MustCompile(`(?i)\b(command not found|is not recognized as (an internal or external command|the name of a cmdlet))`),
	},
	{
		label: "permission denied launching runner",
		re:    regexp.MustCompile(`(?i)permission denied`),
	},
	{
		label: "docker daemon unreachable",
		re:    regexp.MustCompile(`(?i)(cannot connect to the docker daemon|could not connect to docker|error during connect.*docker)`),
	},
	{
		label: "missing language toolchain",
		re:    regexp.MustCompile(`(?i)(go: cannot find|no such file or directory.*\b(go|node|npm|python|java|dotnet)\b|\b(node|npm|python|java|dotnet)\b: command not found)`),
	},
	{
		// The runner rejected the requested suite id before any test ran
		// (runner.selectSuites, internal/runner/tests.go: "suite(s) not
		// found: <id>. Available: …"). A verify that names a renamed or
		// undeclared suite produced no test report at all, so it is an
		// orchestrator-side wiring fault — not a red test. Classifying it
		// red is exactly what spun unexpected-failing-tests-fixer for hours
		// on a non-signal (plan 20260606-1458). Match the runner's fixed
		// prefix, not the id, per the table's own guidance.
		label: "unknown test suite",
		re:    regexp.MustCompile(`(?i)suite\(s\) not found:`),
	},
	{
		// `gh optivem test run` itself rejected the invocation before
		// running anything — a bad flag or subcommand emitted by a verify
		// call site (cobra: "unknown flag: --x", "unknown shorthand flag",
		// "unknown command \"x\""). Like the suite case, no test ran, so
		// this is infra, not a red signal.
		label: "invalid runner invocation",
		re:    regexp.MustCompile(`(?i)unknown (flag|shorthand flag|command)`),
	},
	{
		// The runner started but the `suite`/`test-names` filter selected
		// ZERO tests, so the language tool exited non-zero without running
		// anything (Gradle "No tests found for given includes: …", Maven
		// "No tests were executed", Playwright "Error: No tests found",
		// dotnet "No test matches the given testcase filter"). A fail-
		// expecting verify (verify-tests-fail / VERIFY_TESTS_FAIL_FILTERED)
		// would otherwise read this non-zero exit as a satisfied failure and
		// green WITHOUT exercising a single test — the silent hole plan
		// 20260608-1240 closes. An empty selection is never a valid result on
		// either polarity: it means the suite/test-names pair was mis-resolved
		// upstream, so route it to the infra halt for a human (same as the
		// "unknown test suite" sibling above). Matches the tools' fixed "no
		// tests …" wording, not the echoed filter, per the table's guidance.
		label: "named tests not discoverable — did they compile / are the names correct?",
		re:    regexp.MustCompile(`(?i)\bno tests? (found|were executed|match(es)? the given)`),
	},
	{
		// The runner itself counted zero executed tests across the selected
		// suites and failed the run with its own marker (runner.RunTests, plan
		// 20260608-1502). This closes the empty-selection hole for runners that
		// exit 0 on a zero-match filter (dotnet), which the per-tool "no tests …"
		// patterns above can't see — on a zero exit cobra prints no error, so
		// the tool's own warning never reaches this classifier. The runner's
		// report-count guard turns that silent success into this explicit marker.
		// Same outcome as its siblings: an empty selection is never a valid
		// result on either polarity, so route it to the infra halt.
		label: "named tests not discoverable — did they compile / are the names correct?",
		re:    regexp.MustCompile(`(?i)\b0 tests executed for the given selection`),
	},
	{
		// The runner ran every selected sub-suite but a specifically requested
		// --test name executed in NONE of them (runner.RunTests presence check).
		// Unlike the "no tests found" siblings, a per-partition empty slice is
		// now expected (a named test lives only in the partition matching its
		// tags/channel), so the runner no longer fails per-suite — it fails only
		// when the name ran nowhere across the union. That is a wiring/typo/
		// gated-off fault (e.g. GH_OPTIVEM_RUN_WIP_TESTS never lifted), never a
		// legitimate red, so route it to the infra halt with a presence-specific
		// label rather than the stale "did they compile?" wording.
		label: "requested test never executed — wrong name, gated off (GH_OPTIVEM_RUN_WIP_TESTS), or wrong suite/partition?",
		re:    regexp.MustCompile(`(?i)requested test\S* never executed`),
	},
	{
		// The test harness could not resolve its OWN dependencies before any
		// test ran — the missing-node_modules defect plan 20260617-1456 closes.
		// A fresh worktree reaches `run-tests` with no `node_modules`, so the
		// JS loader fails to import a devDependency (`ERR_MODULE_NOT_FOUND`,
		// "Cannot find package '@playwright/test'", "Cannot find module …").
		// This is an orchestrator-side prerequisite failure, not a red test —
		// system-updater can never resolve a missing npm package, so routing it
		// red burns both opus·high fix passes on an unfixable condition. The
		// structural fix (a setup-tests BPMN step) installs deps up front; this
		// row is defense-in-depth that halts loud with the true cause if setup
		// is ever skipped or fails. Match the loader's fixed wording, not the
		// echoed package name, per the table's guidance.
		label: "test harness dependencies not installed — run `gh optivem test setup`",
		re:    regexp.MustCompile(`(?i)(err_module_not_found|cannot find (package|module)\b)`),
	},
	{
		// Gradle / NuGet equivalents of the missing-deps case above: the build
		// tool failed to resolve a declared dependency before compiling or
		// running tests (Gradle "Could not resolve all (files|dependencies|
		// artifacts)", NuGet "NU1101"/"Unable to find package"). Same root
		// cause and same remedy as the JS row — the harness's own deps are not
		// installed — so route it to the infra halt rather than the fix loop.
		label: "test harness dependencies not installed — run `gh optivem test setup`",
		re:    regexp.MustCompile(`(?i)(could not resolve all (files|dependencies|artifacts)|\bnu1101\b|unable to find package\b)`),
	},
}

// classifyShellErr is the pure-function entry point. It takes the
// captured stderr (combined stderr+stdout is fine — patterns are
// runner-prefix-anchored) and the exit error from the shell call.
//
//   - err == nil → classOK regardless of stderr content.
//   - err != nil and stderr matches an infraPatterns row → classInfra.
//     The matching pattern's label is returned alongside, so the halt
//     message can quote it.
//   - err != nil and no infra pattern matched → classRed. We assume
//     the runner ran and at least one test failed.
//
// The label is "" for ok and red; only infra carries a label. Callers
// that want a one-liner reason for a non-infra failure should pull
// from err.Error() directly.
func classifyShellErr(stderr string, err error) (failureClass, string) {
	if err == nil {
		return classOK, ""
	}
	if stderr == "" {
		return classRed, ""
	}
	for _, p := range infraPatterns {
		if p.re.MatchString(stderr) {
			return classInfra, p.label
		}
	}
	return classRed, ""
}
