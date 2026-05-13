// runner_helpers.go holds the cross-cutting helpers used by the runner-tier
// commands. The command wiring itself lives in `system_commands.go` and
// `test_commands.go` — split per the noun-first surface (`gh optivem system
// <verb>` / `gh optivem test <verb>`). The runner package is fully
// agnostic — these helpers translate Cobra flags into runner.* calls and
// resolve config paths against gh-optivem.yaml.
//
// Working-dir contract: each command operates against the user's current
// working directory. A gh-optivem.yaml is required; its system.config: /
// system_test.config: fields drive path resolution. An alternate
// gh-optivem.yaml can be selected via --config / -c.
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/optivem/gh-optivem/internal/projectconfig"
)

const errorFormat = "ERROR: %v\n"

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

// cwdForPath returns the directory to run docker / setup / suite commands in
// for a given config file. Compose paths in the system config are relative
// to its directory; setup commands and suite.path in the tests config are
// relative to its directory. This lets shop's source layout (system config
// under <lang>/<arch>/, tests-*.yaml + package.json under <lang>/) work
// without per-layout flags. In a scaffolded project both files live in the
// same directory and this is just ".".
func cwdForPath(configPath string) string {
	dir := filepath.Dir(configPath)
	if dir == "" {
		return "."
	}
	return dir
}

// resolveSystemPath returns the systems.yaml path declared in gh-optivem.yaml.
// gh-optivem.yaml is required — there is no default-name fallback. A missing
// file or a missing system.config: field is a hard error pointing the
// operator at the two knobs (create the YAML / fill the field) plus
// --config for the case where the file lives elsewhere.
func resolveSystemPath() (string, error) {
	cfg, path, err := loadProjectConfigForRunner()
	if err != nil {
		return "", err
	}
	if cfg.System.Config == "" {
		return "", fmt.Errorf("%s: system.config: not set; add it (path to your systems.yaml/.json) or pass --config <path> to a gh-optivem.yaml that has it", path)
	}
	return cfg.System.Config, nil
}

// resolveTestsPath mirrors resolveSystemPath for system_test.config:.
func resolveTestsPath() (string, error) {
	cfg, path, err := loadProjectConfigForRunner()
	if err != nil {
		return "", err
	}
	if cfg.SystemTest.Config == "" {
		return "", fmt.Errorf("%s: system_test.config: not set; add it (path to your tests.yaml/.json) or pass --config <path> to a gh-optivem.yaml that has it", path)
	}
	return cfg.SystemTest.Config, nil
}

// loadProjectConfigForRunner reads gh-optivem.yaml via the persistent
// --config / $GH_OPTIVEM_CONFIG resolution. A missing file is a hard error
// (whether at an explicit path or the default location) pointing the
// operator at `gh optivem config init` and --config. Mirrors the same
// hard-error contract as `gh optivem init`'s loadProjectConfigForInit —
// gh-optivem.yaml is the single entry point across the CLI.
func loadProjectConfigForRunner() (*projectconfig.Config, string, error) {
	path, _ := projectconfig.ResolvePath(projectConfigPath)
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, "", fmt.Errorf("resolve absolute path for %s: %w", path, err)
	}
	cfg, err := projectconfig.LoadFromPath(abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, "", fmt.Errorf("no gh-optivem.yaml at %s; run `gh optivem config init` to create one, or pass --config <path> to use an existing one", abs)
		}
		return nil, "", err
	}
	return cfg, abs, nil
}
