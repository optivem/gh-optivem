package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// writeStub writes an executable stub named `name` into `dir` whose body is
// `body`. On Windows the stub is a `.bat` with `body` interpreted as
// `cmd.exe` script; elsewhere it's a POSIX shell script with `body` written
// after a `#!/bin/sh` shebang. Used by tests that need a tool to exist on
// PATH but behave a specific way (e.g. failing `gh auth status`).
func writeStub(t *testing.T, dir, name, body string) {
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
