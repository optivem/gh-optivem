package runner

import (
	"os/exec"
	"strings"
	"testing"
)

// TestRunComposeError_SurfacesStderr is the canary regression: runCompose
// must fold a failing child's stderr into its returned error so the caller's
// FATAL line is self-contained, not just "exit status N".
//
// Skipped when docker is not on PATH. One positive case is sufficient — the
// sibling helpers (runDocker, runShell, RunPassthrough) share the same code
// shape and do not each need their own end-to-end test.
func TestRunComposeError_SurfacesStderr(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not on PATH")
	}

	// Deliberately failing invocation: a compose file path that does not exist.
	// docker compose's stderr will mention the missing file; we assert that
	// mention reaches the returned error.
	err := runCompose("", "--file", "/this/path/does/not/exist.yml", "config")
	if err == nil {
		t.Fatal("expected runCompose to fail on a nonexistent compose file, got nil")
	}

	msg := err.Error()
	if !strings.Contains(msg, "docker compose") {
		t.Errorf("error missing %q prefix from runCompose wrap: %s", "docker compose", msg)
	}
	if !strings.Contains(msg, "stderr tail:") {
		t.Errorf("error missing %q section from runCompose wrap: %s", "stderr tail:", msg)
	}
	// The exact stderr varies across docker versions and platforms, but every
	// docker-compose flavour mentions the path it could not open.
	if !strings.Contains(msg, "exist") && !strings.Contains(msg, "nonexistent") && !strings.Contains(msg, "no such file") {
		t.Errorf("error did not surface child stderr referencing the missing file: %s", msg)
	}
}

func TestTransientNetRE(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want bool
	}{
		{
			name: "MCR 403 manifest fetch",
			msg:  "unexpected status from HEAD request to https://mcr.microsoft.com/v2/dotnet/aspnet/manifests/8.0: 403 Forbidden",
			want: true,
		},
		{
			name: "buildx failed to resolve source metadata",
			msg:  "failed to solve: failed to resolve source metadata for mcr.microsoft.com/dotnet/aspnet:8.0",
			want: true,
		},
		{
			name: "Docker Hub rate limit (lowercase)",
			msg:  "toomanyrequests: You have reached your pull rate limit",
			want: true,
		},
		{
			name: "Docker Hub rate limit (HTTP phrase)",
			msg:  "received unexpected HTTP status: 429 Too Many Requests",
			want: true,
		},
		{
			name: "manifest unknown",
			msg:  "manifest unknown: manifest tag not found",
			want: true,
		},
		{
			name: "ECONNRESET",
			msg:  "read tcp: connection reset by peer ECONNRESET",
			want: true,
		},
		{
			name: "503 Service Unavailable",
			msg:  "Service Unavailable from upstream registry",
			want: true,
		},
		{
			name: "non-network exit code",
			msg:  "exit status 1",
			want: false,
		},
		{
			name: "yaml parse error",
			msg:  "yaml: line 12: did not find expected key",
			want: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := transientNetRE.MatchString(c.msg); got != c.want {
				t.Errorf("MatchString(%q) = %v, want %v", c.msg, got, c.want)
			}
		})
	}
}
