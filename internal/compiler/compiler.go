// Package compiler runs source-level compile sequences for one tier of a
// scaffolded project, dispatching by language.
//
// Compile is the entry point used by the `gh optivem compile` Cobra commands
// (compile_commands.go) and, indirectly, the structural-cycle compile_in_scope
// action — which shells out to `gh optivem compile`.
//
// Per-language commands match what `verify-compilation` does at scaffold time
// (internal/steps/verify.go:buildCommands), trimmed to compileJava only for
// the java case (the structural-cycle compile sweep is a source-level "does
// it parse" check, not a test compile). TypeScript runs `npm ci` first
// because bare `npx tsc --noEmit` errors with "This is not the tsc command
// you are looking for" when no install ever ran.
package compiler

import (
	"fmt"
	"path/filepath"

	"github.com/optivem/gh-optivem/internal/projectconfig"
	"github.com/optivem/gh-optivem/internal/shell"
)

// Shell abstracts subprocess execution so tests can capture invocations
// without spawning real processes. The real implementation streams the
// child's stdout/stderr through to the user's terminal, matching today's
// `compile-all.sh` verbosity (so warnings — Sonar, nullability, etc. —
// stay visible and failure output stays informative).
type Shell interface {
	Run(commandLine, cwd string) error
}

type passthroughShell struct{}

func (passthroughShell) Run(commandLine, cwd string) error {
	return shell.RunPassthrough(commandLine, cwd)
}

// Compile runs the per-language compile sequence for tier inside repoRoot.
// repoRoot is typically "." (the user's cwd, where gh-optivem.yaml lives);
// tier.Path is appended to form the per-tier cwd. Sequential commands run
// in declaration order; the first non-zero exit halts the rest.
func Compile(tier projectconfig.TierSpec, repoRoot string) error {
	return CompileWith(tier, repoRoot, passthroughShell{})
}

// CompileWith is the testable variant of Compile. Production callers use
// Compile, which injects the passthrough Shell. Tests inject a fake Shell
// to assert the dispatched command sequence.
func CompileWith(tier projectconfig.TierSpec, repoRoot string, sh Shell) error {
	cmds, err := commandsFor(tier.Lang)
	if err != nil {
		return err
	}
	cwd := filepath.Join(repoRoot, tier.Path)
	for _, c := range cmds {
		if err := sh.Run(c, cwd); err != nil {
			return fmt.Errorf("compile (%s) %q in %s: %w", tier.Lang, c, cwd, err)
		}
	}
	return nil
}

// commandsFor returns the command sequence for the given language. Returned
// command lines are bash-style; shell.RunPassthrough splits them on
// whitespace and normalizes the executable path cross-platform via
// pathx.NormalizeExe — so `.\gradlew.bat` works on Windows and resolves to
// `./gradlew` elsewhere.
func commandsFor(lang string) ([]string, error) {
	switch lang {
	case projectconfig.LangDotnet:
		return []string{"dotnet build"}, nil
	case projectconfig.LangJava:
		return []string{`.\gradlew.bat compileJava`}, nil
	case projectconfig.LangTypescript:
		return []string{"npm ci", "npx tsc --noEmit"}, nil
	default:
		return nil, fmt.Errorf("compile: unsupported lang %q (want one of %q, %q, %q)",
			lang, projectconfig.LangDotnet, projectconfig.LangJava, projectconfig.LangTypescript)
	}
}
