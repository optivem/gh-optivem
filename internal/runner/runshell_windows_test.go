//go:build windows

package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunShellBatchPreservesCmdMetacharsInArgs writes a tiny `.bat` echo
// fixture and invokes it through runShell with args that contain cmd.exe
// metacharacters (`|`, `&`, `(`, `)`). Without the applyCmdExeQuoting
// workaround, cmd.exe would interpret `|` as a pipe at the top-level
// command line and the .bat would never start (`'B' is not recognized as
// an internal or external command`).
//
// The fixture uses `%*` rather than `%~1` because `%~1` strips the outer
// quotes and inlines the metacharacter into an `echo` line, which cmd.exe
// would then re-interpret — that's a fixture quirk, not the bug we're
// testing. `%*` echoes argv as a single line with quotes intact, so the
// `|` / `&` stay protected inside the `echo`.
func TestRunShellBatchPreservesCmdMetacharsInArgs(t *testing.T) {
	dir := t.TempDir()
	bat := filepath.Join(dir, "echoargs.bat")
	content := "@echo off\r\necho ALL=%*\r\n"
	if err := os.WriteFile(bat, []byte(content), 0o644); err != nil {
		t.Fatalf("write .bat: %v", err)
	}

	stdoutFile := filepath.Join(dir, "stdout.txt")
	stdout, err := os.Create(stdoutFile)
	if err != nil {
		t.Fatalf("create stdout file: %v", err)
	}
	// runShell streams to os.Stdout/os.Stderr; redirect them for the
	// duration of this test so the captured output lands in a file.
	origStdout := os.Stdout
	origStderr := os.Stderr
	os.Stdout = stdout
	os.Stderr = stdout
	defer func() {
		os.Stdout = origStdout
		os.Stderr = origStderr
	}()

	// splitCommand strips single quotes around the arg, so `'A|B'` reaches
	// runShell as the literal arg `A|B` — the exact shape the broken
	// playwright invocation produces.
	cmd := bat + ` 'A|B' 'C&D' '(E)' plainF`
	if err := runShell(cmd, dir, nil); err != nil {
		stdout.Close()
		raw, _ := os.ReadFile(stdoutFile)
		t.Fatalf("runShell: %v\noutput:\n%s", err, raw)
	}
	if err := stdout.Close(); err != nil {
		t.Fatalf("close stdout: %v", err)
	}

	raw, err := os.ReadFile(stdoutFile)
	if err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	out := string(raw)
	// The .bat sees argv with the outer double-quotes that protected the
	// metacharacters from cmd.exe still attached, so the echoed `ALL=`
	// line carries them too.
	wants := []string{
		`"A|B"`,
		`"C&D"`,
		`"(E)"`,
		"plainF",
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q in batch echo output:\n%s", w, out)
		}
	}
}

func TestQuoteForCmdExe(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// Plain — no quoting needed.
		{"plain", "plain"},
		// Cmd metacharacters that EscapeArg leaves alone — double-wrapped.
		{"A|B", `"A|B"`},
		{"C&D", `"C&D"`},
		{"(E)", `"(E)"`},
		{"a>b", `"a>b"`},
		// Whitespace forces EscapeArg to quote — no double wrap needed.
		{"hello world", `"hello world"`},
		// Metacharacter inside an already-quoted whitespace arg — single
		// pair of quotes is enough.
		{"hello|world arg", `"hello|world arg"`},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := quoteForCmdExe(c.in); got != c.want {
				t.Errorf("quoteForCmdExe(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestIsBatchTarget(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{`C:\Program Files\nodejs\npx.cmd`, true},
		{`./gradlew.bat`, true},
		{`C:\Windows\System32\notepad.exe`, false},
		{`/usr/bin/go`, false},
		{"", false},
		// Case-insensitive — Windows extensions are case-fold.
		{`foo.CMD`, true},
		{`foo.BAT`, true},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			if got := isBatchTarget(c.path); got != c.want {
				t.Errorf("isBatchTarget(%q) = %v, want %v", c.path, got, c.want)
			}
		})
	}
}
