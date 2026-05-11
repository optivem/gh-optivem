// verify_paths.go — locate the runner's config files relative to the repo
// root, so verify can pass `--system-config <path> --test-config <path>`
// rather than relying on the runner's `./systems.yaml` default (which fails
// 100% of the time because the orchestrator's cwd is the repo root, not
// the directory holding systems.yaml).
//
// Two layouts to handle:
//
//   - Flat (every scaffolded student repo): `docker/systems.<ext>` and
//     `system-test/tests-latest.<ext>` at fixed paths under the repo root.
//   - Templated (the shop template itself, occasionally used for rehearsal):
//     `docker/<lang>/<arch>/systems.<ext>` and `system-test/<lang>/tests-latest.<ext>`,
//     where <arch> is monolith | multitier.
//
// `<ext>` is probed in priority order `.yaml`, `.yml`, `.json`. YAML is the
// scaffolder default; `.json` remains as a fallback for any legacy repos that
// haven't migrated yet.
//
// We probe flat first because it is the production case. The templated
// fallback discovers <lang> by globbing rather than asking the caller —
// the verify path has no language plumbing, and the templated layout has
// exactly one <lang> per repo by construction (academy repos pick one
// language; the shop template has them side by side but the verify action
// is never run there).
package actions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// configExtensions lists candidate extensions for the runner's config files,
// in probe priority order. YAML is preferred; JSON is kept as a fallback
// until every academy repo has migrated.
var configExtensions = []string{".yaml", ".yml", ".json"}

// ResolveSystemTestPaths returns the paths to systems.<ext> and
// tests-latest.<ext> under repoRoot, discovering whichever layout is
// present. Returns an error when neither layout matches — the caller
// should surface this as an infra-class halt rather than running the
// runner with broken defaults.
//
// The returned paths are absolute when repoRoot is absolute, and relative
// otherwise; the runner accepts both. v1 always pins to tests-latest
// (legacy is only meaningful inside the warm rerun loop, not WRITE-phase
// verify).
func ResolveSystemTestPaths(repoRoot string) (systemConfig, testConfig string, err error) {
	if repoRoot == "" {
		repoRoot = "."
	}

	// 1. Flat layout — `docker/systems.<ext>` + `system-test/tests-latest.<ext>`.
	if sys := firstExisting(filepath.Join(repoRoot, "docker", "systems"), configExtensions); sys != "" {
		if tests := firstExisting(filepath.Join(repoRoot, "system-test", "tests-latest"), configExtensions); tests != "" {
			return sys, tests, nil
		}
	}

	// 2. Templated layout — `docker/<lang>/<arch>/systems.<ext>` paired with
	//    `system-test/<lang>/tests-latest.<ext>`. Probe <arch> in monolith,
	//    multitier order. The first <lang> with both files wins.
	for _, arch := range []string{"monolith", "multitier"} {
		for _, ext := range configExtensions {
			pattern := filepath.Join(repoRoot, "docker", "*", arch, "systems"+ext)
			matches, _ := filepath.Glob(pattern)
			for _, sys := range matches {
				lang := langFromTemplatedSystemPath(repoRoot, sys)
				if lang == "" {
					continue
				}
				if tests := firstExisting(filepath.Join(repoRoot, "system-test", lang, "tests-latest"), configExtensions); tests != "" {
					return sys, tests, nil
				}
			}
		}
	}

	return "", "", fmt.Errorf("could not locate systems.{yaml,yml,json}/tests-latest.{yaml,yml,json} under %q (tried docker/systems.*, docker/*/monolith/systems.*, docker/*/multitier/systems.*)", repoRoot)
}

// firstExisting returns the first file path formed by appending one of exts
// to pathStem that exists on disk, or "" if none exist.
func firstExisting(pathStem string, exts []string) string {
	for _, ext := range exts {
		p := pathStem + ext
		if fileExists(p) {
			return p
		}
	}
	return ""
}

// langFromTemplatedSystemPath extracts <lang> from a path of the form
// <repoRoot>/docker/<lang>/<arch>/systems.<ext>. Returns "" when the path
// does not match that shape (defensive — the glob caller already enforces
// it, but a manual call shouldn't crash).
func langFromTemplatedSystemPath(repoRoot, sys string) string {
	rel, err := filepath.Rel(filepath.Join(repoRoot, "docker"), sys)
	if err != nil {
		return ""
	}
	// rel == "<lang>/<arch>/systems.<ext>"; the OS separator is platform
	// dependent, so split with filepath.SplitList won't work — peel one
	// component off the front.
	dir, _ := filepath.Split(rel)         // dir == "<lang>/<arch>/"
	dir = filepath.Clean(dir)              // -> "<lang>/<arch>"
	dir = filepath.Dir(dir)                // -> "<lang>"
	if dir == "." || dir == string(filepath.Separator) {
		return ""
	}
	return dir
}

// fileExists is a tiny wrapper so callers don't have to remember the
// `os.Stat` + `errors.Is(err, os.ErrNotExist)` pattern. We only care about
// "is this file present", not whether it's readable — the runner will
// surface its own error if it can't open the file.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || !errors.Is(err, os.ErrNotExist)
}
