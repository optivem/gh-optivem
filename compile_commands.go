// compile_commands.go wires the bare `gh optivem compile` aggregate verb and
// the tier-level helpers (compileSystem, compileComponentTests,
// compileSystemTests, monolithTier, loadProjectConfigOrExit) shared with the
// noun-scoped forms (`gh optivem system compile` in system_commands.go,
// `gh optivem component-test compile` in component_commands.go, and
// `gh optivem system-test compile` in test_commands.go).
//
// Naming: `compile` is source-level build (dotnet build / gradlew compileJava
// / npx tsc). `build` is reserved for `docker compose build` (system_commands.go,
// under the `system` noun). The two are distinct enough to coexist; they
// must not be conflated.
//
// `compile` is a bare "for-all" aggregate verb (no parent noun): it spans every
// tier, running `system compile` → `component-test compile` → `system-test
// compile` in sequence, halting on first failure. Bare = for-all, qualified =
// scoped — the `make` / `make test` mental model. This shortcut is the dominant
// use case (the structural-cycle compile_all action shells out to it as a single
// command). Same category as `gh browse` / `gh status` in gh's own surface:
// cross-resource, no single noun to slot under. The noun-scoped forms stay
// available for scoped local use.
package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/atdd/process/configcheck"
	"github.com/optivem/gh-optivem/internal/build/compiler"
	"github.com/optivem/gh-optivem/internal/build/componenttest"
	"github.com/optivem/gh-optivem/internal/config/configinit"
	"github.com/optivem/gh-optivem/internal/kernel/log"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// Phase/summary labels for the compile tiers, shared across the for-all walk and
// the per-tier recorders.
const (
	labelCompileComponentTests = "Compile component-tests"
	labelCompileSystemTests    = "Compile system-tests"
)

// newCompileCmd builds the bare `compile` for-all aggregate verb. Unlike most
// top-level commands, the bare form has a Run that walks every tier (system →
// component-test → system-test, halting on first failure) so the structural
// cycle can shell out to a single command. The noun-scoped forms — `gh optivem
// system compile`, `gh optivem component-test compile`, and `gh optivem
// system-test compile` — are sibling commands registered under their respective
// parents, not children of this verb.
func newCompileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compile",
		Short: "Compile a scaffolded project's source code (all tiers: system + component-test + system-test)",
		Long: `Compile the in-scope source code for a scaffolded project.

"compile" is the bare "for-all" aggregate: it runs "system compile" then
"component-test compile" then "system-test compile" in sequence, halting on
first failure. Distinct from "gh optivem system build", which runs "docker
compose build" against the system's container images.

The system and system-test tiers dispatch per-language commands from
gh-optivem.yaml:
  dotnet     -> dotnet build
  java       -> .\gradlew.bat compileJava compileTestJava
  typescript -> npm ci && npx tsc --noEmit

The component-test tier runs each component's compileCommands from its own
component-tests.yaml (so the component/integration test source sets compile up
front, failing fast before the gating suites). A component with no
compileCommands is skipped.

Use the noun-scoped forms to narrow to one tier:
  gh optivem system compile          # system tier only
  gh optivem component-test compile  # component-test tier only
  gh optivem system-test compile     # system-test tier only`,
		Example: `  gh optivem compile                 # compile every tier
  gh optivem system compile          # narrow to system tier
  gh optivem component-test compile  # narrow to component-test tier
  gh optivem system-test compile     # narrow to system-test tier`,
		Args: cobra.NoArgs,
		Run: func(c *cobra.Command, args []string) {
			cfg := loadProjectConfigOrExit()
			sum := newCompileSummary()

			log.PhaseHeader(1, 3, "Compile system")
			err := compileSystem(cfg, sum)
			if err == nil {
				log.PhaseHeader(2, 3, labelCompileComponentTests)
				err = compileComponentTests(cfg, sum, nil)
			} else {
				sum.MarkSkipped(labelCompileComponentTests)
			}
			if err == nil {
				log.PhaseHeader(3, 3, labelCompileSystemTests)
				err = compileSystemTests(cfg, sum)
			} else {
				sum.MarkSkipped(labelCompileSystemTests)
			}
			sum.Print()
			exitOnError(err)
		},
	}
	return cmd
}

// compileComponentTests compiles the component-test source sets by running each
// selected component's compileCommands (from its component-tests.yaml) and
// records one summary row per component that ran a compile. components narrows
// to the named component(s); nil means every discovered component. A component
// with no compileCommands (or no component-tests.yaml) is skipped silently —
// the field is additive, so the aggregate never regresses a project that has
// not declared a component-test compile.
func compileComponentTests(cfg *projectconfig.Config, sum *compileSummary, components []string) error {
	results, err := componenttest.Compile(discoverComponents(cfg), components)
	for _, r := range results {
		sum.Record(labelCompileComponentTests, r.Component, r.Lang, r.Path, r.Duration, r.Err)
	}
	return err
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
	return recordCompile(sum, labelCompileSystemTests, "system-tests", cfg.SystemTest, func() error {
		return compiler.Compile(cfg.SystemTest, ".")
	})
}
