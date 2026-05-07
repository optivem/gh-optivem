package actions

import (
	"errors"
	"testing"
)

// TestClassifyShellErr exercises the failure-class table. Each case
// comes from a real failure mode the verify action has hit (or is
// expected to hit) when the orchestrator shells out to the system
// test runner. New runner integrations should add a row here, not
// new branches in the classifier.
func TestClassifyShellErr(t *testing.T) {
	exitErr := errors.New("exit status 1")

	cases := []struct {
		name      string
		stderr    string
		err       error
		wantClass failureClass
		wantLabel string
	}{
		{
			name:      "no error is ok regardless of stderr",
			stderr:    "warning: deprecated flag",
			err:       nil,
			wantClass: classOK,
			wantLabel: "",
		},
		{
			name:      "no error and empty stderr is ok",
			stderr:    "",
			err:       nil,
			wantClass: classOK,
			wantLabel: "",
		},
		// ---- infra: cwd / missing system config ---------------------------
		{
			name: "missing system.json (cwd bug)",
			stderr: "ERROR: read system config ./system.json: open ./system.json: " +
				"The system cannot find the file specified.",
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "missing system config",
		},
		{
			name:      "missing system.json on linux",
			stderr:    "open /tmp/system.json: no such file or directory",
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "missing system config",
		},
		// ---- infra: missing executable / toolchain ------------------------
		{
			name:      "executable file not found in PATH",
			stderr:    `exec: "node": executable file not found in $PATH`,
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "missing executable",
		},
		{
			name:      "powershell command not recognized",
			stderr:    "node : The term 'node' is not recognized as the name of a cmdlet, function, script file, or operable program.",
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "missing executable",
		},
		{
			name:      "bash command not found",
			stderr:    "bash: npm: command not found",
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "missing executable",
		},
		// ---- infra: permission denied -------------------------------------
		{
			name:      "permission denied on runner binary",
			stderr:    "fork/exec /usr/local/bin/test-runner: permission denied",
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "permission denied launching runner",
		},
		// ---- infra: docker daemon -----------------------------------------
		{
			name:      "docker daemon unreachable",
			stderr:    "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?",
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "docker daemon unreachable",
		},
		{
			name:      "docker windows pipe error",
			stderr:    "error during connect: This error may indicate that the docker daemon is not running",
			err:       exitErr,
			wantClass: classInfra,
			wantLabel: "docker daemon unreachable",
		},
		// ---- red: tests ran, at least one failed --------------------------
		{
			name: "jest reports failures",
			stderr: "Tests:       1 failed, 5 passed, 6 total\n" +
				"Snapshots:   0 total\n" +
				"Time:        2.345 s",
			err:       exitErr,
			wantClass: classRed,
			wantLabel: "",
		},
		{
			name:      "go test reports FAIL",
			stderr:    "--- FAIL: TestSomething (0.01s)\nFAIL\nexit status 1\nFAIL    pkg/foo  0.123s",
			err:       exitErr,
			wantClass: classRed,
			wantLabel: "",
		},
		{
			name:      "error with empty stderr is red (no infra fingerprint)",
			stderr:    "",
			err:       exitErr,
			wantClass: classRed,
			wantLabel: "",
		},
		{
			name:      "error with unrecognized stderr is red",
			stderr:    "assertion failed: expected 'foo' got 'bar'",
			err:       exitErr,
			wantClass: classRed,
			wantLabel: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, label := classifyShellErr(tc.stderr, tc.err)
			if got != tc.wantClass {
				t.Fatalf("class: got %s, want %s", got, tc.wantClass)
			}
			if label != tc.wantLabel {
				t.Fatalf("label: got %q, want %q", label, tc.wantLabel)
			}
		})
	}
}

// TestFailureClassString locks in the lowercase identifiers the gateway
// binding compares against. The state machine YAML routes on
// `verify_failure_class == "infra"` etc., so changing these strings
// is a contract break.
func TestFailureClassString(t *testing.T) {
	cases := []struct {
		c    failureClass
		want string
	}{
		{classOK, "ok"},
		{classInfra, "infra"},
		{classRed, "red"},
		{failureClass(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.c.String(); got != tc.want {
			t.Errorf("classifyClass(%d).String() = %q, want %q", tc.c, got, tc.want)
		}
	}
}
