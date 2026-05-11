// Package configinit owns the shared "write a fresh gh-optivem.yaml" code
// path. The `gh optivem config init` command, `gh optivem init`, and the
// missing-file recovery prompt invoked by `compile`, `config validate`,
// and `atdd implement-ticket` all funnel through Run + ResolveTarget here
// so the YAML emission and the gitignore side-effect stay single-sourced.
//
// Prompt + EnsureExists layer an interactive recovery on top: when one of
// the read sites can't find gh-optivem.yaml on a TTY, EnsureExists asks
// the user for the required flags and writes the file in place rather
// than returning the terse "run config init first" error.
package configinit

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/files"
	"github.com/optivem/gh-optivem/internal/projectconfig"
	"github.com/optivem/gh-optivem/internal/steps"
)

// ResolveTarget picks the YAML file path `config init` should write to:
// the persistent --config / -c flag (or $GH_OPTIVEM_CONFIG via
// projectconfig.ResolvePath's explicit=true) wins; otherwise dir +
// canonical filename; otherwise cwd + canonical filename.
func ResolveTarget(flagVal, dir string) (string, error) {
	if path, explicit := projectconfig.ResolvePath(flagVal); explicit {
		return path, nil
	}
	target := dir
	if target == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		target = cwd
	}
	return filepath.Join(target, projectconfig.Path), nil
}

// Run is the testable core of `gh optivem config init`. It validates the
// flags, refuses to overwrite an existing file unless force is true, and
// writes the YAML to yamlPath. Also ensures the consumer's .gitignore
// excludes .gh-optivem/ — the runtime's per-run state dir, which is a
// foot-gun if accidentally committed. Returns yamlPath on success.
func Run(f *config.RawFlags, yamlPath string, force bool) (string, error) {
	return runWithBanner(f, yamlPath, force, "")
}

// RunWithBanner is Run plus a comment block prepended to the YAML. Used
// by the interactive missing-file recovery path so the operator sees
// which fields were defaulted and can run `gh optivem config validate`
// after editing. The non-interactive `config init` command uses Run
// (no banner) — operators running that one supplied every flag and
// don't need a review checklist.
func RunWithBanner(f *config.RawFlags, yamlPath string, force bool, banner string) (string, error) {
	return runWithBanner(f, yamlPath, force, banner)
}

func runWithBanner(f *config.RawFlags, yamlPath string, force bool, banner string) (string, error) {
	cfg, err := config.ValidateAndDeriveForYAML(f)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(yamlPath); err == nil && !force {
		return "", fmt.Errorf("%s already exists; pass --force to overwrite", yamlPath)
	}
	if banner == "" {
		if err := steps.WriteOptivemYAMLToFilePath(cfg, yamlPath); err != nil {
			return "", err
		}
	} else {
		if err := steps.WriteOptivemYAMLToFilePathWithBanner(cfg, yamlPath, banner); err != nil {
			return "", err
		}
	}
	if err := files.EnsureGitignoreLine(filepath.Dir(yamlPath), ".gh-optivem/"); err != nil {
		return "", fmt.Errorf("ensure .gitignore: %w", err)
	}
	return yamlPath, nil
}
