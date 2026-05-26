// test_commands.go wires the `gh optivem test <verb>` subtree. The test
// noun owns the test-tier lifecycle verbs: `run` against an already-running
// system, `setup` for harness preparation, and `compile` for the test-tier
// source build.
//
// Working-dir contract: every command runs against the user's cwd and
// reads the tests config path from gh-optivem.yaml's system-test.config:
// field (legacy `.json` files still resolve via the loader's extension
// dispatch). An alternate gh-optivem.yaml can be selected via --config /
// -c. Missing gh-optivem.yaml or empty system-test.config: are hard
// errors. Helpers live in runner_helpers.go.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/testselect"
	"github.com/optivem/gh-optivem/internal/log"
	"github.com/optivem/gh-optivem/internal/runner"
)

// newTestCmd builds the `gh optivem test` parent. The parent has no Run, so
// invoking it without a subcommand prints help.
func newTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Operate on the test tier",
	}
	cmd.AddCommand(
		newTestRunCmd(),
		newTestSetupCmd(),
		newTestCompileCmd(),
	)
	return cmd
}

// newTestRunCmd implements:
//
//	gh optivem test run [--suite id] [--test name] [--sample] [--list]
//
// Runs suites against an already-running system. The caller is responsible
// for the lifecycle: `gh optivem test setup` (once) → `gh optivem system start`
// (once) → `gh optivem test run ...` (one or more times). The verb
// health-probes every entry in systems.yaml first; if any aren't up it errors
// with "start it first with `gh optivem system start`". Mirrors mainstream
// service-lifecycle CLIs (docker compose, systemctl, kubectl) where each
// phase is a separate verb.
func newTestRunCmd() *cobra.Command {
	var (
		suites []string
		test   []string
		sample bool
		list   bool
	)
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run suites from tests.yaml against an already-running system",
		Example: `  gh optivem test run
  gh optivem test run --suite smoke
  gh optivem test run --suite acceptance-api --suite acceptance-ui
  gh optivem test run --suite acceptance-api,acceptance-ui
  gh optivem test run --suite acceptance --test shouldRejectOrderWithQuantityOf100
  gh optivem test run --suite smoke --test T1 --test T2
  gh optivem test run --suite smoke --test T1,T2
  gh optivem test run --list`,
		Run: func(cmd *cobra.Command, args []string) {
			resolvedTests, err := resolveTestsPath()
			exitOnError(err)
			tests, err := runner.LoadTests(resolvedTests)
			exitOnError(err)
			if list {
				for _, id := range tests.SuiteIDs() {
					fmt.Fprintln(os.Stdout, id)
				}
				return
			}
			// Validate operator intent on the raw (pre-expansion) slice:
			// `--suite=acceptance --test=foo` is a single raw value and
			// therefore allowed; `--suite=acceptance-api --suite=acceptance-ui
			// --test=foo` is two raw values and rejected. Expansion happens
			// after validation so the runner only sees canonical suite ids.
			exitOnError(validateSuiteTestCombo(suites, test))
			suites = testselect.ExpandSuiteGroups(suites)
			resolvedSystem, err := resolveSystemPath()
			exitOnError(err)
			sys, err := runner.LoadSystem(resolvedSystem)
			exitOnError(err)
			opts := runner.TestOptions{
				Suite:  suites,
				Test:   test,
				Sample: sample,
			}
			exitOnError(runner.RunTests(sys, tests, cwdForPath(resolvedSystem), cwdForPath(resolvedTests), opts))
		},
	}
	cmd.Flags().StringSliceVar(&suites, "suite", nil, "Run only the suite(s) with these id(s); repeatable, also accepts comma-separated values, and the group alias `acceptance` (expands to all acceptance-* suites)")
	cmd.Flags().StringSliceVar(&test, "test", nil, "Narrow execution to the given test name(s); repeatable, also accepts comma-separated values (substituted into the suite's testFilter)")
	cmd.Flags().BoolVar(&sample, "sample", false, "Use each suite's sampleTest field as the test name")
	cmd.Flags().BoolVar(&list, "list", false, "Print suite ids from tests.yaml (one per line) and exit without running")
	return cmd
}

// newTestSetupCmd implements `gh optivem test setup`. Runs the setupCommands
// block from tests.yaml — the test-harness preparation step (npm ci,
// dependency restore, test-source compile, browser asset downloads, etc.).
// Split out from `test run` so each lifecycle phase has its own verb; CI
// workflows call it once per job, not per suite.
func newTestSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "setup",
		Short:   "Run setupCommands from tests.yaml (prepare the test harness)",
		Example: `  gh optivem test setup`,
		Run: func(cmd *cobra.Command, args []string) {
			resolvedTests, err := resolveTestsPath()
			exitOnError(err)
			tests, err := runner.LoadTests(resolvedTests)
			exitOnError(err)
			exitOnError(runner.RunSetup(tests, cwdForPath(resolvedTests)))
		},
	}
}

// newTestCompileCmd implements `gh optivem test compile`. Compiles only the
// test tier. Helpers (compileSystemTests, loadProjectConfigOrExit) live in
// compile_commands.go since they are also used by the bare `compile` verb.
func newTestCompileCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "compile",
		Short:   "Compile the test tier",
		Example: `  gh optivem test compile`,
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			sum := newCompileSummary()
			log.PhaseHeader(1, 1, "Compile system-tests")
			err := compileSystemTests(loadProjectConfigOrExit(), sum)
			sum.Print()
			exitOnError(err)
		},
	}
}

// validateSuiteTestCombo rejects --test alongside multiple --suite values:
// --test is substituted into one suite's testFilter, so applying it across
// suites is ambiguous (test names rarely exist in more than one suite).
func validateSuiteTestCombo(suites, tests []string) error {
	if len(suites) > 1 && len(tests) > 0 {
		return fmt.Errorf(
			"--test cannot be combined with multiple --suite values " +
				"(test names are substituted into one suite's testFilter); " +
				"narrow to a single --suite, or run the command once per suite")
	}
	return nil
}
