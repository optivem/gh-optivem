// component_commands.go wires the `gh optivem component test <verb>` subtree.
// It is a deliberate sibling of `gh optivem test` (test_commands.go), NOT a
// child of it: component-level tests are part of the system tier — in-process,
// white-box, no running system — whereas `gh optivem test` drives the test tier
// against an already-running, deployed system (compose, channels, external
// stub/real). Keeping the two command namespaces apart preserves the
// inside-vs-outside-the-SUT boundary the course teaches.
//
// Working-dir contract: every verb runs against the user's cwd and discovers
// the components from the active gh-optivem.yaml — monolith system.path, or
// multitier system.backend.path / system.frontend.path. Each component carries
// its own component-tests.yaml (by convention, in the component directory). An
// alternate gh-optivem.yaml is selected via --config / -c.
package main

import (
	"sort"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/build/componenttest"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// newComponentCmd builds the `gh optivem component` parent. It currently hosts
// the single `test` sub-noun; the extra level keeps room for future
// component-scoped verbs without colliding with the test-tier surface.
func newComponentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "component",
		Short: "Operate on individual system components",
	}
	cmd.AddCommand(newComponentTestCmd())
	return cmd
}

// newComponentTestCmd builds `gh optivem component test`, the parent for the
// component-level (commit-stage) suite lifecycle verbs.
func newComponentTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run component-level (commit-stage) test suites from component-tests.yaml",
	}
	cmd.AddCommand(
		newComponentTestRunCmd(),
		newComponentTestSetupCmd(),
	)
	return cmd
}

// newComponentTestRunCmd implements:
//
//	gh optivem component test run [--suite id] [--component name] [--sample] [--test name] [--list]
//
// Bare `run` is every suite × every component — the full set CI gates on. The
// runner is lighter than `test run`: no compose, no system start/stop, no health
// probe; for each component it just resolves the requested suites and runs each
// suite's command in the component directory. `--suite` and `--component` narrow
// the run for local feedback only and never weaken the gate (CI hardcodes the
// full run). Pending suites print a notice and pass; Docker-requiring suites
// fail fast when no daemon is reachable.
func newComponentTestRunCmd() *cobra.Command {
	var (
		suites     []string
		components []string
		test       []string
		sample     bool
		list       bool
	)
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run component suites from each component's component-tests.yaml",
		Example: `  gh optivem component test run
  gh optivem component test run --suite unit
  gh optivem component test run --suite component --component backend
  gh optivem component test run --suite unit --sample
  gh optivem component test run --list`,
		Run: func(cmd *cobra.Command, args []string) {
			all, err := discoverComponentsOrExit()
			exitOnError(err)
			if list {
				exitOnError(componenttest.List(all, components))
				return
			}
			exitOnError(componenttest.Run(all, componenttest.Options{
				Suites:     suites,
				Components: components,
				Sample:     sample,
				Test:       test,
			}))
		},
	}
	cmd.Flags().StringSliceVar(&suites, "suite", nil, "Run only the suite(s) with these id(s); repeatable, also accepts comma-separated values and a `suiteGroups` alias (e.g. `all`)")
	cmd.Flags().StringSliceVar(&components, "component", nil, "Run only the component(s) with these name(s) (e.g. backend, frontend); repeatable, also accepts comma-separated values; omit to run every component")
	cmd.Flags().StringSliceVar(&test, "test", nil, "Narrow execution to the given test name(s); repeatable, also accepts comma-separated values (substituted into the suite's testFilter)")
	cmd.Flags().BoolVar(&sample, "sample", false, "Use each suite's sampleTest field as the test name")
	cmd.Flags().BoolVar(&list, "list", false, "Print each component's suite ids and exit without running")
	return cmd
}

// newComponentTestSetupCmd implements `gh optivem component test setup`. Runs
// the setupCommands block of each selected component's component-tests.yaml —
// npm ci, gradle warm, dependency restore. CI calls it once per job before the
// first run, not per suite.
func newComponentTestSetupCmd() *cobra.Command {
	var components []string
	cmd := &cobra.Command{
		Use:     "setup",
		Short:   "Run setupCommands from each component's component-tests.yaml",
		Example: `  gh optivem component test setup`,
		Run: func(cmd *cobra.Command, args []string) {
			all, err := discoverComponentsOrExit()
			exitOnError(err)
			exitOnError(componenttest.RunSetup(all, components))
		},
	}
	cmd.Flags().StringSliceVar(&components, "component", nil, "Run setup only for the component(s) with these name(s); omit for every component")
	return cmd
}

// discoverComponentsOrExit reads the active gh-optivem.yaml and returns the
// component set the runner operates over. Discovery is by convention: each
// returned component's component-tests.yaml is expected at
// <component.Path>/component-tests.yaml. Resolution mirrors the system-test
// runner — the component Path is taken verbatim from gh-optivem.yaml and
// resolved against the caller's cwd (the repo root, where gh-optivem.yaml lives).
func discoverComponentsOrExit() ([]componenttest.Component, error) {
	cfg, _, err := loadProjectConfigForRunner()
	if err != nil {
		return nil, err
	}
	return discoverComponents(cfg), nil
}

// discoverComponents maps a project config to its components by architecture
// shape:
//   - multitier:     backend + frontend (each its own logical component);
//   - microservices: each backend service (by name) + the frontend;
//   - monolith:      the single `monolith` component.
//
// Logical names (backend / frontend / <service> / monolith) are stable across
// languages, so `--component backend` reads the same on every stack. Backend
// services are emitted in sorted name order for deterministic output.
func discoverComponents(cfg *projectconfig.Config) []componenttest.Component {
	sys := cfg.System
	var out []componenttest.Component
	if sys.Backend.Path != "" || sys.Frontend.Path != "" || len(sys.BackendServices) > 0 {
		if sys.Backend.Path != "" {
			out = append(out, componenttest.Component{Name: "backend", Path: sys.Backend.Path, Lang: sys.Backend.Lang})
		}
		names := make([]string, 0, len(sys.BackendServices))
		for name := range sys.BackendServices {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			ts := sys.BackendServices[name]
			out = append(out, componenttest.Component{Name: name, Path: ts.Path, Lang: ts.Lang})
		}
		if sys.Frontend.Path != "" {
			out = append(out, componenttest.Component{Name: "frontend", Path: sys.Frontend.Path, Lang: sys.Frontend.Lang})
		}
		return out
	}
	return []componenttest.Component{{Name: "monolith", Path: sys.Path, Lang: sys.Lang}}
}
