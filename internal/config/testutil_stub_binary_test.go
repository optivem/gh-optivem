package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// shPath is resolved at package-init time, not inside writeStub: mkPathDir
// points PATH at an empty temp dir, so a LookPath performed during a test
// would never find sh.
var shPath, _ = exec.LookPath("sh")

// writeStub writes an executable stub named `name` into `dir` whose body is
// `body`. On Windows the stub is a `.bat` with `body` interpreted as
// `cmd.exe` script; elsewhere it's a POSIX shell script with `body` written
// after a `#!/bin/sh` shebang. Used by tests that need a tool to exist on
// PATH but behave a specific way (e.g. failing `gh auth status`).
//
// `body` here is a single *portable* string, so it must parse under both
// shells. That is enforced with `sh -n` whenever an `sh` is available (Git
// Bash on the Windows dev box, /bin/sh on the Linux runner) — a body that only
// happens to work under cmd.exe fails on the dev box rather than in CI. The
// guard validates the /bin/sh form only; it does not validate the cmd.exe
// `.bat` form. For a body the caller builds per-OS, use writeStubOSSpecific.
func writeStub(t *testing.T, dir, name, body string) {
	t.Helper()
	checkShSyntax(t, body)
	writeStubOSSpecific(t, dir, name, body)
}

// writeStubOSSpecific is writeStub without the POSIX-sh syntax guard, for the
// callers that branch on runtime.GOOS to build a cmd.exe body and a /bin/sh
// body separately. The guard cannot help there: on Windows it would be
// checking the cmd.exe body, which is not the string Linux ever runs.
func writeStubOSSpecific(t *testing.T, dir, name, body string) {
	t.Helper()
	var path, contents string
	if runtime.GOOS == "windows" {
		path = filepath.Join(dir, name+".bat")
		contents = "@echo off\r\n" + body + "\r\n"
	} else {
		path = filepath.Join(dir, name)
		contents = "#!/bin/sh\n" + body + "\n"
	}
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatalf("writeStub(%s): %v", path, err)
	}
}

// checkShSyntax fails the test if `body` is not valid POSIX sh, reporting
// sh's own parser message. Silently skipped when no `sh` is on PATH.
func checkShSyntax(t *testing.T, body string) {
	t.Helper()
	if shPath == "" {
		return
	}
	script := filepath.Join(t.TempDir(), "stub-syntax-check.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+body+"\n"), 0o644); err != nil {
		t.Fatalf("checkShSyntax: writing %s: %v", script, err)
	}
	out, err := exec.Command(shPath, "-n", script).CombinedOutput()
	if err != nil {
		t.Fatalf("stub body is not valid POSIX sh — a writeStub body must parse under both /bin/sh and cmd.exe (if it is deliberately per-OS, use writeStubOSSpecific):\n%s\nsh said: %v\n%s", body, err, out)
	}
}

// mkPathDir creates an empty temp directory and points PATH at it for the
// duration of the test. On Windows it also sets PATHEXT to ".BAT" so the
// stubs created by writeStub are resolvable by exec.LookPath without an
// explicit extension. Returns the directory so callers can plant stubs.
func mkPathDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	if runtime.GOOS == "windows" {
		t.Setenv("PATHEXT", ".BAT")
	}
	return dir
}
