// Package configinit owns the shared "produce a gh-optivem.yaml" code
// path. Two flavours sit side by side, picked by whether the operator
// has chosen a specific on-disk location:
//
//  1. Explicit-path callers (`gh optivem config init`, and `gh optivem
//     init` when --config / $GH_OPTIVEM_CONFIG names a path) funnel
//     through Run + ResolveTarget, writing the YAML to disk at the
//     chosen path and emitting the .gitignore side-effect.
//  2. Default-path callers (`gh optivem init` with no flag and no env
//     var) funnel through BuildConfig — same validation, no disk
//     write. steps.WriteOptivemYAML then materializes the only on-disk
//     copy inside the scaffold dir.
//
// Prompt + EnsureExists / EnsureExistsOrBuild layer an interactive
// recovery on top. EnsureExists writes the YAML in place (for the
// runner-tier read sites and the explicit-path init arm).
// EnsureExistsOrBuild keeps the config in memory (for the default-path
// init arm), returning the in-memory value plus an empty sourcePath so
// downstream guards on SourceConfigPath == "" short-circuit correctly.
package configinit

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/optivem/gh-optivem/internal/kernel/approval"
	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/files"
	"github.com/optivem/gh-optivem/internal/projectconfig"
	"github.com/optivem/gh-optivem/internal/steps"
)

// pkgApproval holds the resolved auto-approve policy for the whole
// configinit package. main.go's PersistentPreRunE calls SetApproval once
// at startup; askProjectURL reads it back when gating the "Do you have an
// existing GitHub Project?" question.
//
// A package-level slot (rather than threading approval.Resolved through
// every entry point's signature) keeps the interactive recovery path's
// public API stable: EnsureExists, EnsureExistsOrBuild, and Prompt have
// many callers (production and tests) and adding a parameter to each
// would ripple across the codebase for one downstream consumer. The
// trade-off is explicit: production sets the slot once before any
// confirmation site can read it; tests that want to exercise the auto-yes
// path call SetApproval explicitly in their setup.
var pkgApproval approval.Resolved

// SetApproval publishes the resolved auto-approve policy used by
// askProjectURL. Safe to call repeatedly — last writer wins. The zero
// value (Auto=false) is the safe cautious default, so callers that
// never call SetApproval get today's "always prompt" behaviour.
func SetApproval(r approval.Resolved) {
	pkgApproval = r
}

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

// BuildConfig validates the raw flags and returns an in-memory
// *projectconfig.Config — the same content runWithBanner writes to disk,
// minus the write. Used by callers that want to keep the config in memory
// (default-path init, where there is no on-disk source file) and let
// steps.WriteOptivemYAML be the sole disk writer. Does not touch the
// filesystem and does not emit the .gitignore side-effect Run does —
// both belong to operator-chosen on-disk paths.
func BuildConfig(f *config.RawFlags) (*projectconfig.Config, error) {
	cfg, err := config.ValidateAndDeriveForYAML(f)
	if err != nil {
		return nil, err
	}
	return steps.BuildOptivemYAML(cfg), nil
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
