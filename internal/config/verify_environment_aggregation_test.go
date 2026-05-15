package config

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestVerifyEnvironment_AggregatedFailures pins the "fix everything in one
// pass" contract: when several classes of check fail simultaneously
// (missing env vars + missing tool + rejected token), the single returned
// error mentions every one of them.
//
// This is the regression guard for any future refactor that re-classifies
// failures into separate return paths — the user MUST see them all at once.
//
// Scenario:
//   - SONAR_TOKEN and REPO_TOKEN cleared (2 missing vars).
//   - actionlint absent on PATH (1 missing tool).
//   - Docker Hub returns 401 for DOCKERHUB_TOKEN (1 rejected token).
//
// Note: when env vars are missing, VerifyEnvironment gates the live-HTTP
// layer off entirely. So the "rejected token" leg of this scenario uses
// SONAR_TOKEN+REPO_TOKEN missing PLUS some tool-class failure — Docker Hub
// rejection cannot fire when env vars are missing. To exercise all three
// classes together, set every env var, mark one missing-tool, AND have the
// HTTP layer reject one token.
func TestVerifyEnvironment_AggregatedFailures(t *testing.T) {
	// All 6 tokens set so the HTTP layer fires; then actionlint missing
	// + Docker Hub 401 produce two different non-env-var failures.
	dir := mkPathDir(t)
	writeStub(t, dir, "gh", "echo Logged in to github.com\nexit 0")
	// actionlint deliberately NOT planted.
	setAllEnvTokens(t)

	client := routedAuthClient(tokenRoutes{
		dockerhub: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, "denied")
		},
	})

	err := verifyEnvironmentWithClient(nil, "", client)
	if err == nil {
		t.Fatal("expected aggregated error, got nil")
	}
	msg := err.Error()

	wants := []string{
		"actionlint not found on PATH",
		"Docker Hub rejected credentials",
		"Verification failed for 2 check(s)",
	}
	for _, w := range wants {
		if !strings.Contains(msg, w) {
			t.Errorf("aggregated error missing substring %q. Got:\n%s", w, msg)
		}
	}

	_ = dir // referenced only via writeStub; kept for readability
}

// TestVerifyEnvironment_AggregatedFailures_MissingVarsAndTool covers the
// missing-env-vars + missing-tool combination, since the live-HTTP layer
// is gated off when any env var is missing. This is the "user has nothing
// configured" path — the report still lists both classes together.
func TestVerifyEnvironment_AggregatedFailures_MissingVarsAndTool(t *testing.T) {
	dir := mkPathDir(t)
	writeStub(t, dir, "gh", "echo Logged in to github.com\nexit 0")
	// actionlint deliberately NOT planted.
	setAllEnvTokens(t)
	t.Setenv("SONAR_TOKEN", "")
	t.Setenv("REPO_TOKEN", "")

	err := verifyEnvironmentWithClient(nil, "", neverFireClient(t))
	if err == nil {
		t.Fatal("expected aggregated error, got nil")
	}
	msg := err.Error()

	wants := []string{
		"Missing required environment variable(s)",
		"SONAR_TOKEN",
		"REPO_TOKEN",
		"actionlint not found on PATH",
	}
	for _, w := range wants {
		if !strings.Contains(msg, w) {
			t.Errorf("aggregated error missing substring %q. Got:\n%s", w, msg)
		}
	}

	_ = dir
}
