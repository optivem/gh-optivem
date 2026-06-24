// system_commands.go wires the `gh optivem system <verb>` subtree. The
// system noun spans the lifecycle verbs that operate on the scaffolded
// project's running containers (build, start, stop, clean) plus the
// source-level system tier compile.
//
// Working-dir contract: every command runs against the user's cwd and
// reads the systems config path from gh-optivem.yaml's system.config:
// field (legacy `.json` files still resolve via the loader's extension
// dispatch). An alternate gh-optivem.yaml can be selected via --config /
// -c. Missing gh-optivem.yaml or empty system.config: are hard errors.
// Helpers live in runner_helpers.go.
package main

import (
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/kernel/log"
	"github.com/optivem/gh-optivem/internal/build/runner"
)

// newSystemCmd builds the `gh optivem system` parent. The parent has no Run,
// so invoking it without a subcommand prints help (Cobra default).
func newSystemCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "system",
		Short: "Operate on the system tier",
	}
	cmd.AddCommand(
		newSystemBuildCmd(),
		newSystemStartCmd(),
		newSystemStatusCmd(),
		newSystemStopCmd(),
		newSystemCleanCmd(),
		newSystemCompileCmd(),
	)
	return cmd
}

// newSystemBuildCmd implements `gh optivem system build [--rebuild]`.
// Builds every entry in systems[] via `docker compose build`. With --rebuild,
// every layer is rebuilt from scratch (internally `docker compose build
// --no-cache`). Analog of dotnet's --no-incremental and gradle's
// --rerun-tasks — outcome-oriented naming.
func newSystemBuildCmd() *cobra.Command {
	var rebuild bool
	cmd := &cobra.Command{
		Use:     "build",
		Short:   "docker compose build for every entry in systems.yaml",
		Example: `  gh optivem system build --rebuild`,
		Run: func(cmd *cobra.Command, args []string) {
			resolved, err := resolveSystemPath()
			exitOnError(err)
			sys, err := runner.LoadSystem(resolved)
			exitOnError(err)
			exitOnError(runner.Build(sys, cwdForPath(resolved), runner.BuildOptions{Rebuild: rebuild}))
		},
	}
	cmd.Flags().BoolVar(&rebuild, "rebuild", false, "Force a full rebuild from scratch (no layer cache reuse)")
	return cmd
}

// newSystemStartCmd implements `gh optivem system start [--restart] [--log-lines 50] [--up-timeout 5m]`.
// Brings up every entry in systems[] and waits for health. Pairs with
// `system stop`; `start` rather than `run` so the verb doesn't collide with
// `test run`.
func newSystemStartCmd() *cobra.Command {
	var (
		restart   bool
		logLines  int
		upTimeout time.Duration
	)
	cmd := &cobra.Command{
		Use:     "start",
		Short:   "docker compose up + wait for health",
		Example: `  gh optivem system start --restart`,
		Run: func(cmd *cobra.Command, args []string) {
			resolved, err := resolveSystemPath()
			exitOnError(err)
			sys, err := runner.LoadSystem(resolved)
			exitOnError(err)
			opts := runner.SystemOptions{LogLines: logLines, Restart: restart, UpTimeout: upTimeout}
			exitOnError(runner.Up(sys, cwdForPath(resolved), opts))
		},
	}
	cmd.Flags().BoolVar(&restart, "restart", false, "Recreate changed services to pick up new code/stubs/migrations; keeps the database running (incremental, no full down/up)")
	cmd.Flags().IntVar(&logLines, "log-lines", 50, "Lines of compose logs to dump on health-probe failure")
	cmd.Flags().DurationVar(&upTimeout, "up-timeout", 0, "Per-attempt timeout for `docker compose up -d` (zero = 5m default)")
	return cmd
}

// newSystemStatusCmd implements `gh optivem system status [--timeout 2s]`.
// Probes each component + external-system URL once and prints OK/DOWN per
// entry. Does not start or stop anything; exits non-zero if any component
// is DOWN so it can be used in shell pipelines.
func newSystemStatusCmd() *cobra.Command {
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:     "status",
		Short:   "Probe every component URL once and print OK/DOWN per entry",
		Example: `  gh optivem system status`,
		Run: func(cmd *cobra.Command, args []string) {
			resolved, err := resolveSystemPath()
			exitOnError(err)
			sys, err := runner.LoadSystem(resolved)
			exitOnError(err)
			down := runner.Status(os.Stdout, sys, runner.StatusOptions{Timeout: timeout})
			if down > 0 {
				os.Exit(1)
			}
		},
	}
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "Per-URL probe timeout (zero = 2s default)")
	return cmd
}

// newSystemStopCmd implements `gh optivem system stop`.
// Tears down every entry in systems[] and force-removes stray containers.
func newSystemStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "stop",
		Short:   "docker compose down + container cleanup",
		Example: `  gh optivem system stop`,
		Run: func(cmd *cobra.Command, args []string) {
			resolved, err := resolveSystemPath()
			exitOnError(err)
			sys, err := runner.LoadSystem(resolved)
			exitOnError(err)
			exitOnError(runner.Down(sys, cwdForPath(resolved)))
		},
	}
}

// newSystemCleanCmd implements `gh optivem system clean`.
// Tears down every entry in systems[] and removes its named volumes plus
// locally-built images (`docker compose down -v --rmi local`). Analog of
// `dotnet clean` and `./gradlew clean` — deletes build outputs without
// touching dependency caches (registry-pulled images are kept).
func newSystemCleanCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "clean",
		Short:   "docker compose down -v --rmi local (delete volumes + locally-built images)",
		Example: `  gh optivem system clean && gh optivem system-test run`,
		Run: func(cmd *cobra.Command, args []string) {
			resolved, err := resolveSystemPath()
			exitOnError(err)
			sys, err := runner.LoadSystem(resolved)
			exitOnError(err)
			exitOnError(runner.Clean(sys, cwdForPath(resolved)))
		},
	}
}

// newSystemCompileCmd implements `gh optivem system compile`. Compiles only
// the system tier. Helpers (compileSystem, loadProjectConfigOrExit) live in
// compile_commands.go since they are also used by the bare `compile` verb.
func newSystemCompileCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "compile",
		Short:   "Compile the system tier(s)",
		Long:    `Compile the system source. Monolith projects compile the single system tier; multitier projects compile backend then frontend in sequence, halting on first failure.`,
		Example: `  gh optivem system compile`,
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			sum := newCompileSummary()
			log.PhaseHeader(1, 1, "Compile system")
			err := compileSystem(loadProjectConfigOrExit(), sum)
			sum.Print()
			exitOnError(err)
		},
	}
}
