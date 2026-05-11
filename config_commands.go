// config_commands.go wires the `gh optivem config …` subcommands into the
// root Cobra command. The `config` namespace owns operations that read or
// write gh-optivem.yaml — the central per-project config file produced by
// `gh optivem init` and consumed by `gh optivem atdd implement-ticket`.
//
//	gh optivem config init     — write a fresh gh-optivem.yaml from CLI flags
//	gh optivem config validate — parse <CWD>/gh-optivem.yaml and validate it
//
// `config init` reuses the same render path as `gh optivem init`
// (steps.WriteOptivemYAMLToPath / config.ValidateAndDeriveForYAML) so a new
// YAML-affecting flag flows to both surfaces with no per-command duplication.
package main

import (
	"fmt"

	"github.com/spf13/cobra"

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
	)
	return cmd
}

// newConfigInitCmd implements `gh optivem config init`. Writes a fresh
// gh-optivem.yaml from CLI flags. Refuses to overwrite an existing file
// unless --force is passed (the file may be hand-edited; silent overwrite
// is a foot-gun). Pure local file write — no network, no GitHub, no
// SonarCloud.
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
(or $GH_OPTIVEM_CONFIG, or ./gh-optivem.yaml).`,
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
