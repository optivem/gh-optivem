package actions

import (
	"errors"
	"testing"
)

// TestClassifyShellErr exercises the failure-class table. Each case
// comes from a real failure mode the verify action has hit (or is
// expected to hit) when the orchestrator shells out to the system
// test runner. New runner integrations should add a row here, not
// new branches in the classifier.
func TestClassifyShellErr(t *testing.T) {
	exitErr := errors.New("exit status 1")

	cases := []struct {
		name      string
		stderr    string
		err       error
		wantClass failureClass
		wantLabel string
	}{
		{
			name:      "no error is ok regardless of stderr",
			stderr:    "warning: deprecated flag",
			err:       nil,
			wantClass: classOK,
			wantLabel: "",
		},
		{
			name:      "no error and empty stderr is ok",
			stderr:    "",
			err:       nil,
			wantClass: classOK,
			wantLabel: "",
		},
		// ---- infra: cwd / missing system config ---------------------------
		{
			name: "missing systems.json (cwd bug)",
			stderr: "ERROR: read system config ./systems.json: open ./systems.json: " +
				"The system cannot find the file specified.",
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "missing system config",
		},
		{
			name:      "missing systems.json on linux",
			stderr:    "open /tmp/systems.json: no such file or directory",
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "missing system config",
		},
		// ---- infra: missing executable / toolchain ------------------------
		{
			name:      "executable file not found in PATH",
			stderr:    `exec: "node": executable file not found in $PATH`,
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "missing executable",
		},
		{
			name:      "powershell command not recognized",
			stderr:    "node : The term 'node' is not recognized as the name of a cmdlet, function, script file, or operable program.",
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "missing executable",
		},
		{
			name:      "bash command not found",
			stderr:    "bash: npm: command not found",
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "missing executable",
		},
		// ---- infra: permission denied -------------------------------------
		{
			name:      "permission denied on runner binary",
			stderr:    "fork/exec /usr/local/bin/test-runner: permission denied",
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "permission denied launching runner",
		},
		// ---- infra: docker daemon -----------------------------------------
		{
			name:      "docker daemon unreachable",
			stderr:    "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?",
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "docker daemon unreachable",
		},
		{
			name:      "docker windows pipe error",
			stderr:    "error during connect: This error may indicate that the docker daemon is not running",
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "docker daemon unreachable",
		},
		// ---- infra: runner rejected the invocation before any test ran ----
		{
			name:      "unknown test suite (renamed/undeclared suite)",
			stderr:    "ERROR: suite(s) not found: contract. Available: acceptance-api, acceptance-ui, contract-stub, contract-real",
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "unknown test suite",
		},
		{
			name:      "cobra unknown flag from a bad verify invocation",
			stderr:    "Error: unknown flag: --suit",
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "invalid runner invocation",
		},
		{
			name:      "cobra unknown command",
			stderr:    `Error: unknown command "ru" for "optivem test"`,
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "invalid runner invocation",
		},
		// ---- infra: filter selected zero tests (empty selection) ----------
		{
			name:      "gradle no tests found for given includes",
			stderr:    "No tests found for given includes: [com.example.AcceptanceTest.someMethod](filter.includeTestsMatching)",
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "named tests not discoverable — did they compile / are the names correct?",
		},
		{
			name:      "maven no tests were executed",
			stderr:    "[INFO] No tests were executed!  (Set -DfailIfNoTests=false to ignore this error.)",
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "named tests not discoverable — did they compile / are the names correct?",
		},
		{
			name:      "playwright no tests found",
			stderr:    "Error: No tests found.\nMake sure that arguments are regular expressions matching test files.",
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "named tests not discoverable — did they compile / are the names correct?",
		},
		{
			name:      "dotnet no test matches the given filter",
			stderr:    "No test matches the given testcase filter `FullyQualifiedName~SomeMethod` in /app/bin/Tests.dll",
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "named tests not discoverable — did they compile / are the names correct?",
		},
		{
			// The runner's own zero-executed guard (plan 20260608-1502): catches
			// the exit-0-on-empty runners (dotnet) the per-tool patterns can't see.
			name:      "runner zero tests executed marker",
			stderr:    "Error: 0 tests executed for the given selection — the suite/test filter matched nothing on any selected suite; check --suite / --test against the available tests",
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "named tests not discoverable — did they compile / are the names correct?",
		},
		{
			// The runner's presence check (runner.RunTests): every sub-suite ran,
			// but a requested --test name executed in none of them. Distinct from
			// the "no tests found" siblings — a per-partition empty slice is now
			// expected, so this fires only when the name ran nowhere across the union.
			name:      "runner requested test never executed",
			stderr:    "suite acceptance-isolated-api: requested test(s) never executed: cannotCancelAnOrderAt2245OnDec31 — not found in any selected suite; check the test name, that it compiled, and that it isn't gated off (e.g. GH_OPTIVEM_RUN_WIP_TESTS)",
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "requested test never executed — wrong name, gated off (GH_OPTIVEM_RUN_WIP_TESTS), or wrong suite/partition?",
		},
		// ---- infra: test harness deps not installed (missing node_modules) -
		{
			// The rehearsal #61 crash (plan 20260617-1456): a fresh worktree
			// reached run-tests with no node_modules, so the JS loader failed
			// to import a devDependency. Must NOT be classified red — it is an
			// orchestrator-side prerequisite, not a test assertion failure.
			name: "playwright missing @playwright/test (ERR_MODULE_NOT_FOUND)",
			stderr: "Error: Cannot find package '@playwright/test' imported from " +
				"/work/system-test/typescript/playwright.config.ts\n  code: 'ERR_MODULE_NOT_FOUND'",
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "test harness dependencies not installed — run `gh optivem test setup`",
		},
		{
			name:      "node cannot find module",
			stderr:    "Error: Cannot find module 'dotenv'\nRequire stack:\n- /work/system-test/typescript/setup.ts",
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "test harness dependencies not installed — run `gh optivem test setup`",
		},
		{
			name:      "gradle could not resolve dependencies",
			stderr:    "FAILURE: Could not resolve all dependencies for configuration ':testRuntimeClasspath'.",
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "test harness dependencies not installed — run `gh optivem test setup`",
		},
		{
			name:      "nuget unable to find package (NU1101)",
			stderr:    "error NU1101: Unable to find package Microsoft.Playwright. No packages exist with this id in source(s): nuget.org",
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "test harness dependencies not installed — run `gh optivem test setup`",
		},
		// ---- red: tests ran, at least one failed --------------------------
		{
			name: "jest reports failures",
			stderr: "Tests:       1 failed, 5 passed, 6 total\n" +
				"Snapshots:   0 total\n" +
				"Time:        2.345 s",
			err:       exitErr,
			wantClass: classRed,
			wantLabel: "",
		},
		{
			name:      "go test reports FAIL",
			stderr:    "--- FAIL: TestSomething (0.01s)\nFAIL\nexit status 1\nFAIL    pkg/foo  0.123s",
			err:       exitErr,
			wantClass: classRed,
			wantLabel: "",
		},
		{
			name:      "error with empty stderr is red (no infra fingerprint)",
			stderr:    "",
			err:       exitErr,
			wantClass: classRed,
			wantLabel: "",
		},
		{
			name:      "error with unrecognized stderr is red",
			stderr:    "assertion failed: expected 'foo' got 'bar'",
			err:       exitErr,
			wantClass: classRed,
			wantLabel: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, label := classifyShellErr(tc.stderr, tc.err)
			if got != tc.wantClass {
				t.Fatalf("class: got %s, want %s", got, tc.wantClass)
			}
			if label != tc.wantLabel {
				t.Fatalf("label: got %q, want %q", label, tc.wantLabel)
			}
		})
	}
}

// TestFailureClassString locks in the lowercase identifiers the gateway
// binding compares against. The state machine YAML routes on
// `verify_failure_class == "infra"` etc., so changing these strings
// is a contract break.
func TestFailureClassString(t *testing.T) {
	cases := []struct {
		c    failureClass
		want string
	}{
		{classOK, "ok"},
		{classInfra, "infra"},
		{classRed, "red"},
		{failureClass(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.c.String(); got != tc.want {
			t.Errorf("classifyClass(%d).String() = %q, want %q", tc.c, got, tc.want)
		}
	}
}
