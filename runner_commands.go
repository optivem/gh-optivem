// runner_commands.go wires the `build system`, `run system`, `test system`,
// `stop system`, and `clean system` subcommands into the root Cobra command.
// The runner package is fully agnostic — these handlers just translate Cobra
// flags into runner.* calls.
//
// Working-dir contract: each command operates against the user's current
// working directory. JSON config paths default to ./system.json and
// ./tests.json; both can be overridden with --system-config / --test-config.
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

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

// hintIfMissing wraps a "file not found" error from runner.Load* with a hint
// telling the user which flag overrides the default path. Other errors are
// returned unchanged.
func hintIfMissing(err error, flag, defaultPath string) error {
	if err == nil || !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return fmt.Errorf("%w\n  hint: pass %s <path> to point at a different file (default: %s)", err, flag, defaultPath)
}

// loadSystem wraps runner.LoadSystem so a missing file points the user at
// --system-config instead of just reporting "file not found".
func loadSystem(path string) (*runner.SystemConfig, error) {
	sys, err := runner.LoadSystem(path)
	return sys, hintIfMissing(err, "--system-config", defaultSystemConfig)
}

// loadTests wraps runner.LoadTests with the same flag hint as loadSystem.
func loadTests(path string) (*runner.TestsConfig, error) {
	tests, err := runner.LoadTests(path)
	return tests, hintIfMissing(err, "--test-config", defaultTestsConfig)
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

// newBuildSystemCmd implements `gh optivem build system [--system-config path] [--rebuild]`.
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
			sys, err := loadSystem(systemPath)
			exitOnError(err)
			exitOnError(runner.Build(sys, cwdForPath(systemPath), runner.BuildOptions{Rebuild: rebuild}))
		},
	}
	cmd.Flags().StringVar(&systemPath, "system-config", defaultSystemConfig, flagSystemUsage)
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

// newRunSystemCmd implements `gh optivem run system [--system-config path] [--restart] [--log-lines 50] [--up-timeout 5m]`.
// Brings up every entry in systems[] and waits for health.
func newRunSystemCmd() *cobra.Command {
	var (
		systemPath string
		restart    bool
		logLines   int
		upTimeout  time.Duration
	)
	cmd := &cobra.Command{
		Use:     "system",
		Short:   "docker compose up + wait for health",
		Example: `  gh optivem run system --restart`,
		Run: func(cmd *cobra.Command, args []string) {
			sys, err := loadSystem(systemPath)
			exitOnError(err)
			opts := runner.SystemOptions{LogLines: logLines, Restart: restart, UpTimeout: upTimeout}
			exitOnError(runner.Up(sys, cwdForPath(systemPath), opts))
		},
	}
	cmd.Flags().StringVar(&systemPath, "system-config", defaultSystemConfig, flagSystemUsage)
	cmd.Flags().BoolVar(&restart, "restart", false, "Force tear-down + restart even if the system is already up")
	cmd.Flags().IntVar(&logLines, "log-lines", 50, "Lines of compose logs to dump on health-probe failure")
	cmd.Flags().DurationVar(&upTimeout, "up-timeout", 0, "Per-attempt timeout for `docker compose up -d` (zero = 5m default)")
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

// newStopSystemCmd implements `gh optivem stop system [--system-config path]`.
// Tears down every entry in systems[] and force-removes stray containers.
func newStopSystemCmd() *cobra.Command {
	var systemPath string
	cmd := &cobra.Command{
		Use:     "system",
		Short:   "docker compose down + container cleanup",
		Example: `  gh optivem stop system`,
		Run: func(cmd *cobra.Command, args []string) {
			sys, err := loadSystem(systemPath)
			exitOnError(err)
			exitOnError(runner.Down(sys, cwdForPath(systemPath)))
		},
	}
	cmd.Flags().StringVar(&systemPath, "system-config", defaultSystemConfig, flagSystemUsage)
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

// newCleanSystemCmd implements `gh optivem clean system [--system-config path]`.
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
			sys, err := loadSystem(systemPath)
			exitOnError(err)
			exitOnError(runner.Clean(sys, cwdForPath(systemPath)))
		},
	}
	cmd.Flags().StringVar(&systemPath, "system-config", defaultSystemConfig, flagSystemUsage)
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
//	gh optivem test system [--system-config path] [--test-config path]
//	                       [--suite id] [--test name] [--sample]
//	                       [--no-build] [--rebuild] [--no-start] [--restart]
//	                       [--no-setup]
//
// By default, builds images (incremental), starts the system if not already
// up, then runs setup commands and suites. Inspired by `dotnet test` and
// `./gradlew test` which build the test code implicitly before running.
//
// --no-build skips our explicit Build step (compose `up` may still build
// missing images). --rebuild forces a full rebuild from scratch in the
// implicit Build step (ignored with --no-build). --no-start skips Up; the
// system must already be up or the runner errors out. --restart forces
// tear-down + restart during Up.
func newTestSystemCmd() *cobra.Command {
	var (
		systemPath string
		testsPath  string
		suite      string
		test       []string
		sample     bool
		noBuild    bool
		rebuild    bool
		noStart    bool
		restart    bool
		noSetup    bool
		list       bool
	)
	cmd := &cobra.Command{
		Use:   "system",
		Short: "Build + start (if needed) + run setup commands and suites from tests.json",
		Example: `  gh optivem test system
  gh optivem test system --suite smoke
  gh optivem test system --rebuild --suite smoke
  gh optivem test system --no-build --no-start
  gh optivem test system --suite smoke --test T1 --test T2
  gh optivem test system --suite smoke --test T1,T2
  gh optivem test system --list`,
		Run: func(cmd *cobra.Command, args []string) {
			tests, err := loadTests(testsPath)
			exitOnError(err)
			if list {
				for _, id := range tests.SuiteIDs() {
					fmt.Fprintln(os.Stdout, id)
				}
				return
			}
			sys, err := loadSystem(systemPath)
			exitOnError(err)
			opts := runner.TestOptions{
				Suite:   suite,
				Test:    test,
				Sample:  sample,
				NoBuild: noBuild,
				Rebuild: rebuild,
				NoStart: noStart,
				Restart: restart,
				NoSetup: noSetup,
			}
			exitOnError(runner.RunTests(sys, tests, cwdForPath(systemPath), cwdForPath(testsPath), opts))
		},
	}
	cmd.Flags().StringVar(&systemPath, "system-config", defaultSystemConfig, flagSystemUsage)
	cmd.Flags().StringVar(&testsPath, "test-config", defaultTestsConfig, flagTestsUsage)
	cmd.Flags().StringVar(&suite, "suite", "", "Run only the suite with this id")
	cmd.Flags().StringSliceVar(&test, "test", nil, "Narrow execution to the given test name(s); repeatable, also accepts comma-separated values (substituted into the suite's testFilter)")
	cmd.Flags().BoolVar(&sample, "sample", false, "Use each suite's sampleTest field as the test name")
	cmd.Flags().BoolVar(&noBuild, "no-build", false, "Skip the implicit build step (analog of dotnet test --no-build)")
	cmd.Flags().BoolVar(&rebuild, "rebuild", false, "Force a full rebuild from scratch in the implicit build step (ignored with --no-build)")
	cmd.Flags().BoolVar(&noStart, "no-start", false, "Skip the implicit start step; system must already be up")
	cmd.Flags().BoolVar(&restart, "restart", false, "Force tear-down + restart during the implicit start step")
	cmd.Flags().BoolVar(&noSetup, "no-setup", false, "Skip the setupCommands block from tests.json (use when an earlier invocation in the same job already ran setup)")
	cmd.Flags().BoolVar(&list, "list", false, "Print suite ids from tests.json (one per line) and exit without running")
	return cmd
}
