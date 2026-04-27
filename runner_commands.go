// runner_commands.go wires the `build system`, `run system`, `test system`,
// `stop system`, and `clean system` subcommands into the root Cobra command.
// The runner package is fully agnostic — these handlers just translate Cobra
// flags into runner.* calls.
//
// Working-dir contract: each command operates against the user's current
// working directory. JSON config paths default to ./system.json and
// ./tests.json; both can be overridden with --system / --tests.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/runner"
)

// cwdForPath returns the directory to run docker / setup / suite commands in
// for a given config file. Compose paths in system.json are relative to
// system.json's directory; setup commands and suite.path in tests.json are
// relative to tests.json's directory. This lets shop's source layout
// (system.json under <lang>/<arch>/, tests-*.json + package.json under
// <lang>/) work without per-layout flags. In a scaffolded project both files
// live in the same directory and this is just ".".
func cwdForPath(configPath string) string {
	dir := filepath.Dir(configPath)
	if dir == "" {
		return "."
	}
	return dir
}

const (
	defaultSystemConfig = "./system.json"
	defaultTestsConfig  = "./tests.json"

	flagSystemUsage = "Path to system.json"
	flagTestsUsage  = "Path to tests.json"

	errorFormat = "ERROR: %v\n"
)

// exitOnError prints a formatted error to stderr and exits with status 1 when
// err is non-nil. Used by every runner subcommand to keep the load/run
// boilerplate terse.
func exitOnError(err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, errorFormat, err)
	os.Exit(1)
}

// newBuildCmd wires `gh optivem build` and its `system` child. The parent has
// no Run, so Cobra prints help if the user invokes `gh optivem build` alone.
func newBuildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build a scaffolded project",
	}
	cmd.AddCommand(newBuildSystemCmd())
	return cmd
}

// newBuildSystemCmd implements `gh optivem build system [--system path] [--rebuild]`.
// Builds every entry in systems[] via `docker compose build`. With --rebuild,
// every layer is rebuilt from scratch (internally `docker compose build
// --no-cache`). Analog of dotnet's --no-incremental and gradle's
// --rerun-tasks — outcome-oriented naming.
func newBuildSystemCmd() *cobra.Command {
	var (
		systemPath string
		rebuild    bool
	)
	cmd := &cobra.Command{
		Use:     "system",
		Short:   "docker compose build for every entry in system.json",
		Example: `  gh optivem build system --rebuild`,
		Run: func(cmd *cobra.Command, args []string) {
			sys, err := runner.LoadSystem(systemPath)
			exitOnError(err)
			exitOnError(runner.Build(sys, cwdForPath(systemPath), runner.BuildOptions{Rebuild: rebuild}))
		},
	}
	cmd.Flags().StringVar(&systemPath, "system", defaultSystemConfig, flagSystemUsage)
	cmd.Flags().BoolVar(&rebuild, "rebuild", false, "Force a full rebuild from scratch (no layer cache reuse)")
	return cmd
}

// newRunCmd wires `gh optivem run` and its `system` child.
func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a scaffolded project",
	}
	cmd.AddCommand(newRunSystemCmd())
	return cmd
}

// newRunSystemCmd implements `gh optivem run system [--system path] [--restart] [--log-lines 50]`.
// Brings up every entry in systems[] and waits for health.
func newRunSystemCmd() *cobra.Command {
	var (
		systemPath string
		restart    bool
		logLines   int
	)
	cmd := &cobra.Command{
		Use:     "system",
		Short:   "docker compose up + wait for health",
		Example: `  gh optivem run system --restart`,
		Run: func(cmd *cobra.Command, args []string) {
			sys, err := runner.LoadSystem(systemPath)
			exitOnError(err)
			opts := runner.SystemOptions{LogLines: logLines, Restart: restart}
			exitOnError(runner.Up(sys, cwdForPath(systemPath), opts))
		},
	}
	cmd.Flags().StringVar(&systemPath, "system", defaultSystemConfig, flagSystemUsage)
	cmd.Flags().BoolVar(&restart, "restart", false, "Force tear-down + restart even if the system is already up")
	cmd.Flags().IntVar(&logLines, "log-lines", 50, "Lines of compose logs to dump on health-probe failure")
	return cmd
}

// newStopCmd wires `gh optivem stop` and its `system` child.
func newStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop a scaffolded project",
	}
	cmd.AddCommand(newStopSystemCmd())
	return cmd
}

// newStopSystemCmd implements `gh optivem stop system [--system path]`.
// Tears down every entry in systems[] and force-removes stray containers.
func newStopSystemCmd() *cobra.Command {
	var systemPath string
	cmd := &cobra.Command{
		Use:     "system",
		Short:   "docker compose down + container cleanup",
		Example: `  gh optivem stop system`,
		Run: func(cmd *cobra.Command, args []string) {
			sys, err := runner.LoadSystem(systemPath)
			exitOnError(err)
			exitOnError(runner.Down(sys, cwdForPath(systemPath)))
		},
	}
	cmd.Flags().StringVar(&systemPath, "system", defaultSystemConfig, flagSystemUsage)
	return cmd
}

// newCleanCmd wires `gh optivem clean` and its `system` child.
func newCleanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Delete build outputs of a scaffolded project",
	}
	cmd.AddCommand(newCleanSystemCmd())
	return cmd
}

// newCleanSystemCmd implements `gh optivem clean system [--system path]`.
// Tears down every entry in systems[] and removes its named volumes plus
// locally-built images (`docker compose down -v --rmi local`). Analog of
// `dotnet clean` and `./gradlew clean` — deletes build outputs without
// touching dependency caches (registry-pulled images are kept).
func newCleanSystemCmd() *cobra.Command {
	var systemPath string
	cmd := &cobra.Command{
		Use:     "system",
		Short:   "docker compose down -v --rmi local (delete volumes + locally-built images)",
		Example: `  gh optivem clean system && gh optivem test system`,
		Run: func(cmd *cobra.Command, args []string) {
			sys, err := runner.LoadSystem(systemPath)
			exitOnError(err)
			exitOnError(runner.Clean(sys, cwdForPath(systemPath)))
		},
	}
	cmd.Flags().StringVar(&systemPath, "system", defaultSystemConfig, flagSystemUsage)
	return cmd
}

// newTestCmd wires `gh optivem test` and its `system` child.
func newTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test a scaffolded project",
	}
	cmd.AddCommand(newTestSystemCmd())
	return cmd
}

// newTestSystemCmd implements:
//
//	gh optivem test system [--system path] [--tests path]
//	                       [--suite id] [--test name] [--sample]
//	                       [--no-build] [--no-start] [--restart]
//
// By default, builds images (incremental), starts the system if not already
// up, then runs setup commands and suites. Inspired by `dotnet test` and
// `./gradlew test` which build the test code implicitly before running.
//
// --no-build skips our explicit Build step (compose `up` may still build
// missing images). --no-start skips Up; the system must already be up or
// the runner errors out. --restart forces tear-down + restart during Up.
func newTestSystemCmd() *cobra.Command {
	var (
		systemPath string
		testsPath  string
		suite      string
		test       string
		sample     bool
		noBuild    bool
		noStart    bool
		restart    bool
	)
	cmd := &cobra.Command{
		Use:   "system",
		Short: "Build + start (if needed) + run setup commands and suites from tests.json",
		Example: `  gh optivem test system
  gh optivem test system --suite smoke
  gh optivem test system --no-build --no-start`,
		Run: func(cmd *cobra.Command, args []string) {
			sys, err := runner.LoadSystem(systemPath)
			exitOnError(err)
			tests, err := runner.LoadTests(testsPath)
			exitOnError(err)
			opts := runner.TestOptions{
				Suite:   suite,
				Test:    test,
				Sample:  sample,
				NoBuild: noBuild,
				NoStart: noStart,
				Restart: restart,
			}
			exitOnError(runner.RunTests(sys, tests, cwdForPath(testsPath), opts))
		},
	}
	cmd.Flags().StringVar(&systemPath, "system", defaultSystemConfig, flagSystemUsage)
	cmd.Flags().StringVar(&testsPath, "tests", defaultTestsConfig, flagTestsUsage)
	cmd.Flags().StringVar(&suite, "suite", "", "Run only the suite with this id")
	cmd.Flags().StringVar(&test, "test", "", "Narrow execution to one test name (substituted into the suite's testFilter)")
	cmd.Flags().BoolVar(&sample, "sample", false, "Use each suite's sampleTest field as the test name")
	cmd.Flags().BoolVar(&noBuild, "no-build", false, "Skip the implicit build step (analog of dotnet test --no-build)")
	cmd.Flags().BoolVar(&noStart, "no-start", false, "Skip the implicit start step; system must already be up")
	cmd.Flags().BoolVar(&restart, "restart", false, "Force tear-down + restart during the implicit start step")
	return cmd
}
