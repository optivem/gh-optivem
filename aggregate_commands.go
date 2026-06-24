// aggregate_commands.go wires the bare `gh optivem test` aggregate verb — the
// for-all counterpart to the bare `compile` aggregate (compile_commands.go).
//
// `test` is no longer a tier (that ambiguity is resolved by the `system-test` /
// `component-test` level nouns — plan 20260624-1221). The bare word is now the
// run-everything verb: it runs every test level in cheap→expensive pyramid
// order, halting on first failure —
//
//	component-test suites  →  system start  →  system-test run  →  system stop
//
// and manages the system lifecycle itself by default. CI, which already starts
// and stops the system explicitly around its acceptance stage, opts out of the
// start/stop with --assume-running so the aggregate does not double-manage the
// system. Bare = for-all, qualified (`system-test` / `component-test`) = scoped:
// the `make` / `make test` mental model. Bare `compile` has no lifecycle analog
// — it is a pure source build, so it carries no such flag.
package main

import (
	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/testselect"
	"github.com/optivem/gh-optivem/internal/build/componenttest"
	"github.com/optivem/gh-optivem/internal/build/runner"
	"github.com/optivem/gh-optivem/internal/kernel/log"
)

// newTestAggregateCmd builds the bare `gh optivem test` aggregate. Unlike the
// `system-test` / `component-test` level nouns (which have no Run and print
// help), this bare verb has a Run that walks every test level in pyramid order.
func newTestAggregateCmd() *cobra.Command {
	var assumeRunning bool
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run every test level in pyramid order (component-test suites → system tests)",
		Long: `Run every test level for a scaffolded project, cheapest first, halting on the
first failure:

  component-test suites  ->  system start  ->  system-test run  ->  system stop

By default the aggregate manages the system lifecycle itself (start before the
system tests, stop after). Pass --assume-running to skip the start/stop — CI
already starts and stops the system explicitly around its acceptance stage, so
it opts out to avoid double-managing the system.

Use the level nouns to narrow to one level:
  gh optivem component-test run   # component-test level only
  gh optivem system-test run      # system-test level only (against a running system)`,
		Example: `  gh optivem test
  gh optivem test --assume-running   # CI: system already started/stopped externally`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			// Total phase count drives the "N/total" headers: 4 with lifecycle
			// management, 2 when the caller owns start/stop.
			total := 4
			if assumeRunning {
				total = 2
			}
			phase := 0
			next := func(name string) {
				phase++
				log.PhaseHeader(phase, total, name)
			}

			// 1. Component-test level — every suite × every component, no system.
			all, err := discoverComponentsOrExit()
			exitOnError(err)
			next("Component tests")
			exitOnError(componenttest.Run(all, componenttest.Options{}))

			// Resolve the system + system-test config before any lifecycle action
			// so a wiring error fails before we start a system we'd have to stop.
			resolvedSystem, err := resolveSystemPath()
			exitOnError(err)
			sys, err := runner.LoadSystem(resolvedSystem)
			exitOnError(err)
			resolvedTests, err := resolveTestsPath()
			exitOnError(err)
			tests, err := runner.LoadTests(resolvedTests)
			exitOnError(err)

			// 2. Start the system (unless the caller already owns the lifecycle).
			if !assumeRunning {
				next("Start system")
				exitOnError(runner.Up(sys, cwdForPath(resolvedSystem), runner.SystemOptions{}))
			}

			// 3. System-test level — all suites against the running system.
			next("System tests")
			runErr := runner.RunTests(sys, tests, cwdForPath(resolvedSystem), cwdForPath(resolvedTests), runner.TestOptions{
				// Mirror `system-test run`'s membership safety net so a named
				// cross-channel test isn't misread as a typo.
				MembershipProbeSuites: testselect.ExpandSuiteGroups([]string{"acceptance"}, tests.SuiteGroups, nil),
			})

			// 4. Stop the system regardless of the run outcome, so a red run never
			// leaks a running system. The run error still decides the exit code.
			if !assumeRunning {
				next("Stop system")
				if stopErr := runner.Down(sys, cwdForPath(resolvedSystem)); stopErr != nil && runErr == nil {
					runErr = stopErr
				}
			}
			exitOnError(runErr)
		},
	}
	cmd.Flags().BoolVar(&assumeRunning, "assume-running", false,
		"Skip the system start/stop — assume the system is already running (CI starts/stops it explicitly around the acceptance stage)")
	return cmd
}
