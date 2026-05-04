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
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/projectconfig"
	"github.com/optivem/gh-optivem/internal/steps"
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
func newConfigInitCmd() *cobra.Command {
	f := &config.RawFlags{}
	var (
		force bool
		dir   string
	)
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Write a fresh gh-optivem.yaml in the current repo",
		Long: `Write a fresh gh-optivem.yaml in the current working directory (or
the directory passed via --dir) from the supplied flags.

Refuses to overwrite an existing file unless --force is passed. The
file is the single source of truth for the tool and may be hand-edited;
silent overwrite would be a foot-gun.`,
		Example: `  # Monolith, Java
  gh optivem config init --owner acme --repo page-turner \
      --arch monolith --repo-strategy monorepo --monolith-lang java \
      --project-url https://github.com/orgs/acme/projects/1

  # Overwrite an existing file
  gh optivem config init --owner acme ... --force`,
		Run: func(cmd *cobra.Command, args []string) {
			path, err := runConfigInit(f, dir, force)
			exitOnError(err)
			fmt.Printf("Wrote %s\n", path)
		},
	}
	config.BindConfigInitFlags(cmd, f)
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite an existing gh-optivem.yaml")
	cmd.Flags().StringVar(&dir, "dir", "", "Directory to write gh-optivem.yaml into (default: current working directory)")
	return cmd
}

// runConfigInit is the testable core of `gh optivem config init`. It validates
// the flags, resolves the target directory, refuses to overwrite an existing
// file unless force is true, and writes the YAML. Returns the absolute path
// written on success.
func runConfigInit(f *config.RawFlags, dir string, force bool) (string, error) {
	cfg, err := config.ValidateAndDeriveForYAML(f)
	if err != nil {
		return "", err
	}
	target := dir
	if target == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		target = cwd
	}
	yamlPath := filepath.Join(target, projectconfig.Path)
	if _, err := os.Stat(yamlPath); err == nil && !force {
		return "", fmt.Errorf("%s already exists; pass --force to overwrite", yamlPath)
	}
	if err := steps.WriteOptivemYAMLToPath(cfg, target); err != nil {
		return "", err
	}
	return yamlPath, nil
}

// newConfigValidateCmd implements `gh optivem config validate`. Reads
// <dir>/gh-optivem.yaml and runs it through projectconfig.Validate (Load
// invokes Validate internally — successful Load = valid file). Surfaces
// the existing-but-currently-unreachable Validate capability so anyone
// hand-editing the YAML can check it before running implement-ticket.
func newConfigValidateCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate gh-optivem.yaml in the current repo",
		Long: `Read <dir>/gh-optivem.yaml (defaults to the current working directory)
and validate it against the projectconfig schema. Exits 0 when valid,
non-zero with the validation error otherwise.`,
		Example: `  gh optivem config validate
  gh optivem config validate --dir ./some-other-repo`,
		Run: func(cmd *cobra.Command, args []string) {
			path, err := runConfigValidate(dir)
			exitOnError(err)
			fmt.Printf("%s is valid\n", path)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Directory containing gh-optivem.yaml (default: current working directory)")
	return cmd
}

// runConfigValidate is the testable core of `gh optivem config validate`. It
// reads <dir>/gh-optivem.yaml (defaulting dir to CWD), runs it through
// projectconfig.Load (which invokes Validate), and returns the absolute path
// validated on success. Missing file returns a wrapped error pointing the
// user at `gh optivem config init`.
func runConfigValidate(dir string) (string, error) {
	target := dir
	if target == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		target = cwd
	}
	yamlPath := filepath.Join(target, projectconfig.Path)
	if _, err := os.Stat(yamlPath); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no gh-optivem.yaml in %s; run `gh optivem config init` first", target)
		}
		return "", err
	}
	cfg, err := projectconfig.Load(target)
	if err != nil {
		return "", err
	}
	if cfg == nil {
		// Defensive — Load returns (nil, nil) only on missing file, which the
		// os.Stat above already caught. Reaching here means a race between
		// stat and read.
		return "", fmt.Errorf("no gh-optivem.yaml in %s", target)
	}
	return yamlPath, nil
}
