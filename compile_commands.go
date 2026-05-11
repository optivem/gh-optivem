// compile_commands.go wires the `compile`, `compile system`, and
// `compile system-tests` Cobra subcommands.
//
// Naming: `compile` is source-level build (dotnet build / gradlew compileJava
// / npx tsc). `build` is reserved for `docker compose build` (runner_commands.go).
// The two are distinct enough to coexist; they must not be conflated.
//
// Bare `gh optivem compile` runs `system` then `system-tests` sequentially,
// halting on first failure. This shortcut is the dominant use case (the
// structural-cycle compile_in_scope action shells out to it as a single
// command), and is a deliberate departure from build/run/test/stop/clean
// which all require an explicit subcommand. The explicit subcommands stay
// available for scoped local use.
package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/compiler"
	"github.com/optivem/gh-optivem/internal/configinit"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// newCompileCmd builds the `compile` parent. Unlike build/run/test, the
// parent itself runs (system + system-tests in sequence) so the structural
// cycle can shell out to a single command.
func newCompileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compile",
		Short: "Compile a scaffolded project's source code (system + system-tests)",
		Long: `Compile the in-scope source code for a scaffolded project.

Per-language commands are dispatched from gh-optivem.yaml:
  dotnet     -> dotnet build
  java       -> .\gradlew.bat compileJava
  typescript -> npm ci && npx tsc --noEmit

Distinct from "gh optivem build", which runs "docker compose build" against
the system's container images.

Bare "gh optivem compile" runs "system" then "system-tests" in sequence,
halting on first failure. Use the explicit subcommands to scope to one tier.`,
		Example: `  gh optivem compile               # compile system + system-tests
  gh optivem compile system        # compile only the system tier(s)
  gh optivem compile system-tests  # compile only the system-tests tier`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			cfg := loadProjectConfigOrExit()
			exitOnError(compileSystem(cfg))
			exitOnError(compileSystemTests(cfg))
		},
	}
	cmd.AddCommand(newCompileSystemCmd(), newCompileSystemTestsCmd())
	return cmd
}

func newCompileSystemCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "system",
		Short:   "Compile the system tier(s)",
		Long:    `Compile the system source. Monolith projects compile the single system tier; multitier projects compile backend then frontend in sequence, halting on first failure.`,
		Example: `  gh optivem compile system`,
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			exitOnError(compileSystem(loadProjectConfigOrExit()))
		},
	}
}

func newCompileSystemTestsCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "system-tests",
		Short:   "Compile the system-tests tier",
		Example: `  gh optivem compile system-tests`,
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			exitOnError(compileSystemTests(loadProjectConfigOrExit()))
		},
	}
}

// loadProjectConfigOrExit resolves the project config path via the
// persistent --config / -c flag (or $GH_OPTIVEM_CONFIG, or cwd) and loads
// it. Missing file is a hard error — compile dispatches by language
// fields that only exist in this file. EnsureExists owns the recovery
// flow (interactive prompt on a TTY, terse "run config init first"
// error otherwise).
func loadProjectConfigOrExit() *projectconfig.Config {
	path, _ := projectconfig.ResolvePath(projectConfigPath)
	exitOnError(configinit.EnsureExists(path))
	cfg, err := projectconfig.LoadFromPath(path)
	exitOnError(err)
	return cfg
}

// compileSystem dispatches by architecture. Monolith uses the System tier;
// multitier compiles Backend then Frontend, halting on first failure (matches
// today's `compile-all.sh` behavior — first error wins).
func compileSystem(cfg *projectconfig.Config) error {
	switch cfg.System.Architecture {
	case projectconfig.ArchMonolith:
		return compiler.Compile(monolithTier(cfg), ".")
	case projectconfig.ArchMultitier:
		if err := compiler.Compile(cfg.System.Backend, "."); err != nil {
			return err
		}
		return compiler.Compile(cfg.System.Frontend, ".")
	case "":
		return fmt.Errorf("compile system: %s has no system.architecture set", projectconfig.Path)
	default:
		return fmt.Errorf("compile system: unknown system.architecture %q", cfg.System.Architecture)
	}
}

// monolithTier projects a monolith System onto a TierSpec so compiler.Compile
// can dispatch on Lang uniformly with the multitier branches.
func monolithTier(cfg *projectconfig.Config) projectconfig.TierSpec {
	return projectconfig.TierSpec{
		Path: cfg.System.Path,
		Repo: cfg.System.Repo,
		Lang: cfg.System.Lang,
	}
}

func compileSystemTests(cfg *projectconfig.Config) error {
	if cfg.SystemTest.IsEmpty() {
		return fmt.Errorf("compile system-tests: %s has no system_test set", projectconfig.Path)
	}
	return compiler.Compile(cfg.SystemTest, ".")
}
