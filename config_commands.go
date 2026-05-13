// config_commands.go wires the `gh optivem config …` subcommands into the
// root Cobra command. The `config` namespace owns operations that read or
// write gh-optivem.yaml — the central per-project config file produced by
// `gh optivem init` and consumed by `gh optivem implement`.
//
//	gh optivem config init      — write a fresh gh-optivem.yaml from CLI flags
//	gh optivem config validate  — parse <CWD>/gh-optivem.yaml and validate it
//	gh optivem config preflight — validate + check on-disk layout exists
//
// `config init` reuses the same render path as `gh optivem init`
// (steps.WriteOptivemYAMLToPath / config.ValidateAndDeriveForYAML) so a new
// YAML-affecting flag flows to both surfaces with no per-command duplication.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/preflight"
	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/configinit"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// newConfigCmd builds the `gh optivem config` parent. The parent has no Run,
// so invoking it without a subcommand prints help (Cobra default).
func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage gh-optivem.yaml in a consumer repo",
		Long: `Manage gh-optivem.yaml — the per-project configuration file consumed by
the ATDD pipeline (project URL, repo strategy, scope axes).

Normally produced by ` + "`gh optivem init`" + `; these subcommands let you
write or validate the file standalone (e.g. retrofitting it into a
non-scaffolded repo, or re-validating after a hand edit).`,
	}
	cmd.AddCommand(
		newConfigInitCmd(),
		newConfigValidateCmd(),
		newConfigPreflightCmd(),
	)
	return cmd
}

// newConfigInitCmd implements `gh optivem config init`. Writes a fresh
// gh-optivem.yaml from CLI flags. Refuses to overwrite an existing file
// unless --force is passed (the file may be hand-edited; silent overwrite
// is a foot-gun).
//
// TODO: document the standalone retrofit flow (running `config init`
// from inside a hand-rolled, non-scaffolded repo) in the README once
// the UX is validated. For now the README leads with `gh optivem init`,
// which folds in the same prompt via configinit.EnsureExists.
//
// Validations run before the file is written, in two phases: (1) format
// (owner naming rules, license key, arch/repo-strategy enums, project
// URL shape) and (2) existence (owner resolves as a real GitHub user or
// org; project URL — when supplied — resolves to a real Project v2 the
// caller can read). The interactive prompt path shares the same
// validators, so flag-driven and interactive `config init` produce the
// same accept/reject decisions on every field.
//
// Target path precedence: persistent --config / -c (or $GH_OPTIVEM_CONFIG)
// > --dir > current working directory. --config names an exact target
// file (any filename); --dir names a parent directory and the canonical
// `gh-optivem.yaml` filename is appended.
func newConfigInitCmd() *cobra.Command {
	f := &config.RawFlags{}
	var (
		force bool
		dir   string
	)
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Write a fresh gh-optivem.yaml in the current repo",
		Long: `Write a fresh gh-optivem.yaml from the supplied flags.

Target path precedence: --config <path> (also honored as $GH_OPTIVEM_CONFIG)
> --dir <dir> (writes <dir>/gh-optivem.yaml) > current working directory.

Refuses to overwrite an existing file unless --force is passed. The
file is the single source of truth for the tool and may be hand-edited;
silent overwrite would be a foot-gun.`,
		Example: `  # Monolith, Java
  gh optivem config init --owner acme --repo page-turner \
      --arch monolith --repo-strategy monorepo --monolith-lang java \
      --project-url https://github.com/orgs/acme/projects/1

  # Write to a non-default filename (shop's multi-combination matrix)
  gh optivem -c ./gh-optivem.monolith-java.yaml config init --owner acme ...

  # Overwrite an existing file
  gh optivem config init --owner acme ... --force`,
		Run: func(cmd *cobra.Command, args []string) {
			yamlPath, err := configinit.ResolveTarget(projectConfigPath, dir)
			exitOnError(err)
			// No required YAML flags + TTY → drop into the same Prompt path
			// EnsureExists uses for missing-file recovery. Non-TTY falls
			// through to configinit.Run and surfaces the existing
			// "required flags" error from ValidateAndDeriveForYAML.
			if noRequiredConfigInitFlagsSet(f) && isatty.IsTerminal(os.Stdin.Fd()) {
				// Fail fast before entering the prompt session: otherwise
				// the operator fills in every field only for runWithBanner
				// to refuse at the very end. Same error string the flag
				// path produces — runWithBanner still re-checks under the
				// covers so this isn't load-bearing for correctness, just
				// UX.
				if _, statErr := os.Stat(yamlPath); statErr == nil && !force {
					exitOnError(fmt.Errorf("%s already exists; pass --force to overwrite", yamlPath))
				}
				if force {
					fmt.Fprintf(os.Stderr, "Overwriting %s interactively (--force).\n", yamlPath)
				} else {
					fmt.Fprintf(os.Stderr, "Creating %s interactively.\n", yamlPath)
				}
				prompted, perr := configinit.Prompt(os.Stdin, os.Stderr)
				exitOnError(perr)
				written, werr := configinit.RunWithBanner(prompted, yamlPath, force, configinit.Banner)
				exitOnError(werr)
				fmt.Printf("Wrote %s\n", written)
				return
			}
			written, err := configinit.Run(f, yamlPath, force)
			exitOnError(err)
			fmt.Printf("Wrote %s\n", written)
		},
	}
	config.BindConfigInitFlags(cmd, f)
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite an existing gh-optivem.yaml")
	cmd.Flags().StringVar(&dir, "dir", "", "Directory to write gh-optivem.yaml into (ignored if --config is set; default: current working directory)")
	return cmd
}

// newConfigValidateCmd implements `gh optivem config validate`. Reads the
// gh-optivem.yaml at the path resolved by the persistent --config / -c flag
// (or $GH_OPTIVEM_CONFIG, or cwd) and runs it through projectconfig.Validate
// (LoadFromPath invokes Validate internally — successful load = valid file).
// Surfaces the existing-but-otherwise-unreachable Validate capability so
// anyone hand-editing the YAML can check it before running implement-ticket.
func newConfigValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate gh-optivem.yaml in the current repo",
		Long: `Validate gh-optivem.yaml against the projectconfig schema. Exits 0
when valid, non-zero with the validation error otherwise.

The target file is resolved via the persistent --config / -c flag
(or $GH_OPTIVEM_CONFIG, or ./gh-optivem.yaml).

Coverage includes the SonarCloud block (when system.architecture is
set): sonar.organization plus sonar_project on every code tier (system
or backend+frontend, plus system_test) must be present. The YAML is
the source of truth for these keys — the scaffolder seeds defaults via
DeriveSonarProjects but the values may be hand-edited afterwards (e.g.
multi-stack reference repos that need per-variant SonarCloud projects
the single-stack deriver cannot express).`,
		Example: `  gh optivem config validate
  gh optivem -c ./gh-optivem.shop-monolith.yaml config validate`,
		Run: func(cmd *cobra.Command, args []string) {
			path, _ := projectconfig.ResolvePath(projectConfigPath)
			validated, err := runConfigValidate(path)
			exitOnError(err)
			fmt.Printf("%s is valid\n", validated)
		},
	}
	return cmd
}

// noRequiredConfigInitFlagsSet reports whether the operator passed none of
// the five required YAML-affecting flags. Trigger for `config init` to
// drop into the interactive Prompt (on a TTY) instead of erroring with
// "required flags: --owner, --repo, …". Mirrors the precondition in
// config.ValidateAndDeriveForYAML.
func noRequiredConfigInitFlagsSet(f *config.RawFlags) bool {
	return f.Owner == "" && f.Repo == "" && f.SystemName == "" && f.Arch == "" && f.RepoStrategy == ""
}

// runConfigValidate is the testable core of `gh optivem config validate`. It
// runs EnsureExists (which on a TTY offers to create the file
// interactively) and then validates via projectconfig.LoadFromPath.
// Missing file on a non-TTY returns the terse error pointing the user at
// `gh optivem config init`.
func runConfigValidate(yamlPath string) (string, error) {
	if err := configinit.EnsureExists(yamlPath); err != nil {
		return "", err
	}
	if _, err := projectconfig.LoadFromPath(yamlPath); err != nil {
		return "", err
	}
	return yamlPath, nil
}

// newConfigPreflightCmd implements `gh optivem config preflight`. Runs the
// same schema validation as `config validate`, then the on-disk preflight
// check (every declared repo and tier path actually exists in the
// workspace). Surfaces the late "preflight failed" errors that otherwise
// only fire deep inside `implement`.
//
// Schema-only validation stays on `config validate`: that command must keep
// passing for the half-built state right after `gh optivem config init`,
// where the YAML is well-formed but the sibling repos haven't been cloned
// yet. `preflight` is the stronger contract — "I'm about to actually use
// this config" — and is expected to fail in that intermediate state.
func newConfigPreflightCmd() *cobra.Command {
	var workspace string
	cmd := &cobra.Command{
		Use:   "preflight",
		Short: "Validate gh-optivem.yaml and check its declared paths exist on disk",
		Long: `Run schema validation (same as ` + "`config validate`" + `) and additionally
verify that every repo and tier path declared in gh-optivem.yaml resolves
to a real directory on disk. This is the same check ` + "`implement`" + `
runs at startup — run it standalone to catch missing clones or mistyped
paths before kicking off a pipeline.

Exits 0 when both schema and on-disk layout check out, non-zero with one
multi-line error block listing every failure otherwise.`,
		Example: `  gh optivem config preflight
  gh optivem config preflight --workspace /abs/path/to/workspace
  gh optivem -c ./gh-optivem.shop-monolith.yaml config preflight`,
		Run: func(cmd *cobra.Command, args []string) {
			path, _ := projectconfig.ResolvePath(projectConfigPath)
			cwd, err := os.Getwd()
			exitOnError(err)
			validated, err := runConfigPreflight(path, func(cfg *projectconfig.Config) (preflight.Options, error) {
				return defaultPreflightOptions(cfg, workspace, cwd)
			})
			exitOnError(err)
			fmt.Printf("%s passes preflight\n", validated)
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace root containing one clone per repo (default: parent directory of CWD). Each clone dir must be named after the repo-name component of its slug; symlink outliers into place.")
	return cmd
}

// runConfigPreflight is the testable core of `gh optivem config preflight`.
// Mirrors runConfigValidate's EnsureExists + LoadFromPath chain, then
// delegates to preflight.Run for the on-disk + remote check.
//
// optsFor is a factory invoked once cfg has loaded successfully: the
// cobra command path returns defaultPreflightOptions (real remote
// clients + SONAR_TOKEN check); tests return a bare Options with just
// Workspace/Cwd set, so the test surface stays offline. The factory
// gets to see cfg so it can decide whether SonarCloud wiring applies
// (cfg.Sonar.Organization set) and surface a clean SONAR_TOKEN-missing
// error before any remote calls fire.
func runConfigPreflight(yamlPath string, optsFor func(*projectconfig.Config) (preflight.Options, error)) (string, error) {
	if err := configinit.EnsureExists(yamlPath); err != nil {
		return "", err
	}
	cfg, err := projectconfig.LoadFromPath(yamlPath)
	if err != nil {
		return "", err
	}
	opts, err := optsFor(cfg)
	if err != nil {
		return "", err
	}
	if err := preflight.Run(context.Background(), cfg, opts); err != nil {
		return "", err
	}
	return yamlPath, nil
}
