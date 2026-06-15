// compile_commands.go wires the bare `gh optivem compile` verb and the
// tier-level helpers (compileSystem, compileSystemTests, monolithTier,
// loadProjectConfigOrExit) shared with the noun-scoped forms
// (`gh optivem system compile` in system_commands.go and `gh optivem test
// compile` in test_commands.go).
//
// Naming: `compile` is source-level build (dotnet build / gradlew compileJava
// / npx tsc). `build` is reserved for `docker compose build` (system_commands.go,
// under the `system` noun). The two are distinct enough to coexist; they
// must not be conflated.
//
// `compile` stays a bare verb (no parent noun) because it spans tiers — the
// bare form runs `system compile` then `test compile` in sequence, halting on
// first failure. This shortcut is the dominant use case (the structural-cycle
// compile_all action shells out to it as a single command). Same category as
// `gh browse` / `gh status` in gh's own surface: cross-resource, no single
// noun to slot under. The noun-scoped forms stay available for scoped local
// use.
package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/configcheck"
	"github.com/optivem/gh-optivem/internal/build/compiler"
	"github.com/optivem/gh-optivem/internal/configinit"
	"github.com/optivem/gh-optivem/internal/kernel/log"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// newCompileCmd builds the bare `compile` verb. Unlike most top-level
// commands, the bare form has a Run that walks the tiers (system then
// system-tests, halting on first failure) so the structural cycle can shell
// out to a single command. The noun-scoped forms — `gh optivem system
// compile` and `gh optivem test compile` — are sibling commands registered
// under their respective parents, not children of this verb.
func newCompileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compile",
		Short: "Compile a scaffolded project's source code (system + test tiers)",
		Long: `Compile the in-scope source code for a scaffolded project.

Runs "system compile" then "test compile" in sequence, halting on first
failure. Distinct from "gh optivem system build", which runs "docker compose
build" against the system's container images.

Per-language commands are dispatched from gh-optivem.yaml:
  dotnet     -> dotnet build
  java       -> .\gradlew.bat compileJava compileTestJava
  typescript -> npm ci && npx tsc --noEmit

Use the noun-scoped forms to narrow to one tier:
  gh optivem system compile   # system tier only
  gh optivem test compile     # test tier only`,
		Example: `  gh optivem compile               # compile both tiers
  gh optivem system compile        # narrow to system tier
  gh optivem test compile          # narrow to test tier`,
		Args: cobra.NoArgs,
		Run: func(c *cobra.Command, args []string) {
			cfg := loadProjectConfigOrExit()
			sum := newCompileSummary()

			log.PhaseHeader(1, 2, "Compile system")
			err := compileSystem(cfg, sum)
			if err == nil {
				log.PhaseHeader(2, 2, "Compile system-tests")
				err = compileSystemTests(cfg, sum)
			} else {
				sum.MarkSkipped("Compile system-tests")
			}
			sum.Print()
			exitOnError(err)
		},
	}
	return cmd
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
	cfg, err := configcheck.LoadFromPath(path)
	exitOnError(err)
	return cfg
}

// compileSystem dispatches by architecture. Monolith uses the System tier;
// multitier compiles Backend then Frontend, halting on first failure (matches
// today's `compile-all.sh` behavior — first error wins). Each `compiler.Compile`
// call is timed and recorded on sum so the tail summary can report per-tier
// outcomes.
func compileSystem(cfg *projectconfig.Config, sum *compileSummary) error {
	const phase = "Compile system"
	switch cfg.System.Architecture {
	case projectconfig.ArchMonolith:
		tier := monolithTier(cfg)
		log.Infof("Compiling system (%s) in %s", tier.Lang, tier.Path)
		return recordCompile(sum, phase, "system", tier, func() error {
			return compiler.Compile(tier, ".")
		})
	case projectconfig.ArchMultitier:
		log.Infof("Compiling backend (%s) in %s", cfg.System.Backend.Lang, cfg.System.Backend.Path)
		if err := recordCompile(sum, phase, "backend", cfg.System.Backend, func() error {
			return compiler.Compile(cfg.System.Backend, ".")
		}); err != nil {
			return err
		}
		log.Infof("Compiling frontend (%s) in %s", cfg.System.Frontend.Lang, cfg.System.Frontend.Path)
		return recordCompile(sum, phase, "frontend", cfg.System.Frontend, func() error {
			return compiler.Compile(cfg.System.Frontend, ".")
		})
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

func compileSystemTests(cfg *projectconfig.Config, sum *compileSummary) error {
	if cfg.SystemTest.IsEmpty() {
		return fmt.Errorf("compile system-tests: %s has no system-test set", projectconfig.Path)
	}
	log.Infof("Compiling system-tests (%s) in %s", cfg.SystemTest.Lang, cfg.SystemTest.Path)
	return recordCompile(sum, "Compile system-tests", "system-tests", cfg.SystemTest, func() error {
		return compiler.Compile(cfg.SystemTest, ".")
	})
}
