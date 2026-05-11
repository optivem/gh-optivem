// ensure.go bridges the three "read gh-optivem.yaml" entry points
// (config validate, compile, atdd implement-ticket) to the interactive
// Prompt. EnsureExists is the only function the entry points need to
// call: it checks the file, returns nil if present, and on
// fs.ErrNotExist + TTY stdin drives Prompt + Run to write a fresh file
// in place. Non-TTY (pipes, CI, redirected stdin) reverts to the existing
// terse error so unattended runs fail fast with a stable message.
package configinit

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/mattn/go-isatty"
)

// EnsureExists returns nil if a regular gh-optivem.yaml exists at path.
// On fs.ErrNotExist + an interactive stdin (terminal), prints a banner,
// runs Prompt against stdin/stderr, and writes the file via Run. On
// non-TTY stdin or any other Stat error, returns the existing terse
// error verbatim so callers don't change behaviour for the
// non-interactive case.
//
// The terse-error wording is single-sourced here and matches what
// `config validate` printed before this prompt existed (`no
// gh-optivem.yaml at <path>; run `gh optivem config init` first`), so
// CI logs and any operator muscle memory don't shift.
func EnsureExists(path string) error {
	return ensureExists(path, isatty.IsTerminal(os.Stdin.Fd()), os.Stdin, os.Stderr)
}

// ensureExists is the testable core. The public EnsureExists supplies
// the real TTY check + stdio; tests inject false/true and an in-memory
// bufferpair so the prompt path can be exercised deterministically.
func ensureExists(path string, isTTY bool, in io.Reader, out io.Writer) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if !isTTY {
		return missingFileError(path)
	}
	fmt.Fprintf(out, "no gh-optivem.yaml at %s; creating one interactively\n", path)
	f, err := Prompt(in, out)
	if err != nil {
		return missingFileError(path)
	}
	if _, err := Run(f, path, false); err != nil {
		return err
	}
	return nil
}

// missingFileError formats the terse error verbatim — kept private so
// every call site that wants the existing wording references one
// constant instead of duplicating the format string.
func missingFileError(path string) error {
	return fmt.Errorf("no gh-optivem.yaml at %s; run `gh optivem config init` first", path)
}
