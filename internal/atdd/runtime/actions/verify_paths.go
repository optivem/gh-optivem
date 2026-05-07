// verify_paths.go — locate the runner's config files relative to the repo
// root, so verify can pass `--system-config <path> --test-config <path>`
// rather than relying on the runner's `./system.json` default (which fails
// 100% of the time because the orchestrator's cwd is the repo root, not
// the directory holding system.json).
//
// Two layouts to handle:
//
//   - Flat (every scaffolded student repo): `docker/system.json` and
//     `system-test/tests-latest.json` at fixed paths under the repo root.
//   - Templated (the shop template itself, occasionally used for rehearsal):
//     `docker/<lang>/<arch>/system.json` and `system-test/<lang>/tests-latest.json`,
//     where <arch> is monolith | multitier.
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

// ResolveSystemTestPaths returns the paths to system.json and
// tests-latest.json under repoRoot, discovering whichever layout is
// present. Returns an error when neither layout matches — the caller
// should surface this as an infra-class halt rather than running the
// runner with broken defaults.
//
// The returned paths are absolute when repoRoot is absolute, and relative
// otherwise; the runner accepts both. v1 always pins to tests-latest.json
// (legacy is only meaningful inside the warm rerun loop, not WRITE-phase
// verify).
func ResolveSystemTestPaths(repoRoot string) (systemConfig, testConfig string, err error) {
	if repoRoot == "" {
		repoRoot = "."
	}

	// 1. Flat layout — `docker/system.json` + `system-test/tests-latest.json`.
	flatSys := filepath.Join(repoRoot, "docker", "system.json")
	flatTests := filepath.Join(repoRoot, "system-test", "tests-latest.json")
	if fileExists(flatSys) && fileExists(flatTests) {
		return flatSys, flatTests, nil
	}

	// 2. Templated layout — `docker/<lang>/<arch>/system.json` paired with
	//    `system-test/<lang>/tests-latest.json`. Probe <arch> in monolith,
	//    multitier order. The first <lang> with both files wins.
	for _, arch := range []string{"monolith", "multitier"} {
		pattern := filepath.Join(repoRoot, "docker", "*", arch, "system.json")
		matches, _ := filepath.Glob(pattern)
		for _, sys := range matches {
			lang := langFromTemplatedSystemPath(repoRoot, sys)
			if lang == "" {
				continue
			}
			tests := filepath.Join(repoRoot, "system-test", lang, "tests-latest.json")
			if fileExists(tests) {
				return sys, tests, nil
			}
		}
	}

	return "", "", fmt.Errorf("could not locate system.json/tests-latest.json under %q (tried docker/system.json, docker/*/monolith/system.json, docker/*/multitier/system.json)", repoRoot)
}

// langFromTemplatedSystemPath extracts <lang> from a path of the form
// <repoRoot>/docker/<lang>/<arch>/system.json. Returns "" when the path
// does not match that shape (defensive — the glob caller already enforces
// it, but a manual call shouldn't crash).
func langFromTemplatedSystemPath(repoRoot, sys string) string {
	rel, err := filepath.Rel(filepath.Join(repoRoot, "docker"), sys)
	if err != nil {
		return ""
	}
	// rel == "<lang>/<arch>/system.json"; the OS separator is platform
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
