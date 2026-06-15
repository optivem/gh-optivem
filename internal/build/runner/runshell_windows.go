//go:build windows

package runner

import (
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// applyCmdExeQuoting overrides cmd's default command-line composition when the
// resolved target is a batch file (`.bat` / `.cmd`). Without this, args
// containing cmd.exe metacharacters (`|`, `&`, `<`, `>`, `^`, `(`, `)`, `%`,
// `!`) are interpreted by cmd.exe before the batch file's CRT parser ever
// sees them — `A|B` becomes a pipe, parens become grouping, etc.
//
// The workaround composes a CmdLine where:
//
//  1. Each arg is escaped per the MS C runtime rules (via syscall.EscapeArg),
//     so the batch file's CRT re-tokeniser preserves it intact.
//  2. Args containing cmd metacharacters that aren't already protected by the
//     CRT-escape's quotes get an additional outer pair of double quotes so
//     cmd.exe leaves them as literal argv to the batch file.
//
// The double-wrap is the canonical workaround: the inner CRT escape protects
// from CRT re-tokenisation; the outer quotes protect from cmd.exe.
//
// parts[0] is the user-typed command (e.g. "npx"); cmd.Path is the resolved
// executable that will actually run (e.g. "C:\Program Files\nodejs\npx.cmd"
// after LookPath). The check fires off cmd.Path so a bare `npx` that resolves
// to `npx.cmd` is caught.
func applyCmdExeQuoting(cmd *exec.Cmd, parts []string) {
	if !isBatchTarget(cmd.Path) {
		return
	}
	escaped := make([]string, 0, len(parts))
	escaped = append(escaped, syscall.EscapeArg(cmd.Path))
	for _, a := range parts[1:] {
		escaped = append(escaped, quoteForCmdExe(a))
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// Go wraps .bat/.cmd execution through `cmd.exe /c <cmdline>`. cmd.exe's
	// /c rule strips the first and last quote on the line when the tail
	// contains any of &<>()@^|, multiple quote pairs, or otherwise fails
	// rule 1 in `cmd /?` (/S section). Our composed line starts with a
	// quoted executable path (needed when the path has spaces, e.g.
	// `C:\Program Files\nodejs\npx.cmd`) and usually carries quoted
	// metacharacter-bearing args, so without compensation cmd.exe eats the
	// quotes around the path and chokes on the embedded space —
	// `'C:\Program' is not recognized`. Wrap in an extra outer pair so the
	// strip absorbs them and leaves the original quoting intact for the
	// batch file's CRT parser.
	cmd.SysProcAttr.CmdLine = `"` + strings.Join(escaped, " ") + `"`
}

func isBatchTarget(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".bat" || ext == ".cmd"
}

// cmdExeMeta is the set of cmd.exe shell metacharacters that, if left
// unquoted in argv to a batch-file target, get interpreted by cmd.exe before
// the batch file's CRT parser sees them.
const cmdExeMeta = "|&<>^()%!"

func quoteForCmdExe(arg string) string {
	escaped := syscall.EscapeArg(arg)
	if !strings.ContainsAny(arg, cmdExeMeta) {
		return escaped
	}
	// If syscall.EscapeArg already wrapped the arg in quotes (whitespace /
	// embedded "), the metacharacters are inside a quoted run and cmd.exe
	// won't touch them.
	if len(escaped) >= 2 && strings.HasPrefix(escaped, `"`) && strings.HasSuffix(escaped, `"`) {
		return escaped
	}
	return `"` + escaped + `"`
}
