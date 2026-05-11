// runner_helpers.go holds the cross-cutting helpers used by the runner-tier
// commands. The command wiring itself lives in `system_commands.go` and
// `test_commands.go` — split per the noun-first surface (`gh optivem system
// <verb>` / `gh optivem test <verb>`). The runner package is fully
// agnostic — these helpers translate Cobra flags into runner.* calls and
// resolve config paths against gh-optivem.yaml.
//
// Working-dir contract: each command operates against the user's current
// working directory. Config paths default to ./systems.yaml and ./tests.yaml
// (legacy ./systems.json / ./tests.json still resolve via the loader's
// extension dispatch); both can be overridden via gh-optivem.yaml's
// system.config: / system_test.config: fields, or by selecting an alternate
// gh-optivem.yaml via --config / -c.
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/optivem/gh-optivem/internal/projectconfig"
	"github.com/optivem/gh-optivem/internal/runner"
)

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

const (
	defaultSystemConfig = "./systems.yaml"
	defaultTestsConfig  = "./tests.yaml"

	errorFormat = "ERROR: %v\n"
)

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

// hintIfMissing wraps a "file not found" error from runner.Load* with a hint
// listing the two knobs in resolution-precedence order (YAML field → default
// path), so the hint also documents lookup order. Other errors are returned
// unchanged.
func hintIfMissing(err error, yamlField, defaultPath string) error {
	if err == nil || !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return fmt.Errorf("%w\n  hint: set %s: in gh-optivem.yaml, or create %s",
		err, yamlField, defaultPath)
}

// resolveSystemPath applies the runner's two-tier path lookup:
//  1. gh-optivem.yaml's system.config: field
//  2. defaultSystemConfig (./systems.yaml)
//
// A missing gh-optivem.yaml is "no preference" and falls through to the
// default — runner commands still work in repos without one. A YAML that
// was loaded but has system.config empty falls through likewise. Parse or
// validation errors propagate (a broken file shouldn't silently pick a
// fallback the operator didn't ask for); a missing file at an *explicit*
// --config / $GH_OPTIVEM_CONFIG target also propagates for the same reason.
// Operators with multiple variants per project select an alternate YAML
// via the persistent --config / -c flag.
func resolveSystemPath() (string, error) {
	cfg, err := loadProjectConfigForRunner()
	if err != nil {
		return "", err
	}
	if cfg != nil && cfg.System.Config != "" {
		return cfg.System.Config, nil
	}
	return defaultSystemConfig, nil
}

// resolveTestsPath mirrors resolveSystemPath for the tests config / system_test.config:.
func resolveTestsPath() (string, error) {
	cfg, err := loadProjectConfigForRunner()
	if err != nil {
		return "", err
	}
	if cfg != nil && cfg.SystemTest.Config != "" {
		return cfg.SystemTest.Config, nil
	}
	return defaultTestsConfig, nil
}

// loadProjectConfigForRunner reads gh-optivem.yaml via the persistent
// --config / $GH_OPTIVEM_CONFIG resolution. A missing default-location file
// returns (nil, nil) — runner subcommands must keep working in repos that
// have no gh-optivem.yaml yet. A missing file at an *explicit* path is an
// error: the operator pointed at it on purpose.
func loadProjectConfigForRunner() (*projectconfig.Config, error) {
	path, explicit := projectconfig.ResolvePath(projectConfigPath)
	cfg, err := projectconfig.LoadFromPath(path)
	if err == nil {
		return cfg, nil
	}
	if errors.Is(err, fs.ErrNotExist) && !explicit {
		return nil, nil
	}
	return nil, err
}

// loadSystem wraps runner.LoadSystem so a missing file points the user at
// the two resolution knobs (system.config: in YAML, default path).
func loadSystem(path string) (*runner.SystemConfig, error) {
	sys, err := runner.LoadSystem(path)
	return sys, hintIfMissing(err, "system.config", defaultSystemConfig)
}

// loadTests wraps runner.LoadTests with the same two-knob hint as loadSystem.
func loadTests(path string) (*runner.TestsConfig, error) {
	tests, err := runner.LoadTests(path)
	return tests, hintIfMissing(err, "system_test.config", defaultTestsConfig)
}
