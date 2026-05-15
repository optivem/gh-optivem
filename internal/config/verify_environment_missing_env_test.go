package config

import (
	"net/http"
	"strings"
	"testing"
)

// neverFireClient is an http.Client whose RoundTripper marks the test as
// failed if any request is issued. Used by negative-scenario tests that
// must short-circuit before the live-HTTP layer fires — e.g. missing env
// vars gate the HTTP block off in VerifyEnvironment.
func neverFireClient(t *testing.T) *http.Client {
	t.Helper()
	return &http.Client{Transport: &fakeRoundTripper{handler: func(_ http.ResponseWriter, req *http.Request) {
		t.Fatalf("HTTP layer must not fire when env vars are missing — got request to %s", req.URL.String())
	}}}
}

// setAllEnvTokens plants placeholder values for every required env var so a
// test can then clear specific ones via t.Setenv(name, "") and assert the
// resulting failure path. Placeholders are non-empty but otherwise meaningless
// — the HTTP layer never sees them because either (a) we provide
// neverFireClient when a var is missing, or (b) the fakeRoundTripper handles
// every URL anyway.
func setAllEnvTokens(t *testing.T) {
	t.Helper()
	t.Setenv("DOCKERHUB_USERNAME", "test-user")
	t.Setenv("DOCKERHUB_TOKEN", "test-dockerhub-token")
	t.Setenv("SONAR_TOKEN", "test-sonar-token")
	t.Setenv("GHCR_TOKEN", "test-ghcr-token")
	t.Setenv("WORKFLOW_TOKEN", "test-workflow-token")
	t.Setenv("REPO_TOKEN", "test-repo-token")
}

// plantHappyTools points PATH at a fresh dir containing stub `gh` and
// `actionlint` binaries. The `gh` stub answers `auth status` with a "logged
// in" line and exit 0; `actionlint` exits 0. This isolates env-var /
// HTTP-rejection tests from the host's actual `gh` / `actionlint` install.
func plantHappyTools(t *testing.T) {
	t.Helper()
	dir := mkPathDir(t)
	writeStub(t, dir, "gh", "echo Logged in to github.com\nexit 0")
	writeStub(t, dir, "actionlint", "exit 0")
}

// TestVerifyEnvironment_MissingEnvVar exercises the env-var-presence gate:
// each required variable, when cleared, must surface by name in the
// aggregated error, and the HTTP layer must NOT fire.
func TestVerifyEnvironment_MissingEnvVar(t *testing.T) {
	cases := []struct {
		envVar      string
		hintSubstr  string // additional substring expected in the error for PAT vars
	}{
		{envVar: "DOCKERHUB_USERNAME"},
		{envVar: "DOCKERHUB_TOKEN"},
		{envVar: "SONAR_TOKEN"},
		{envVar: "GHCR_TOKEN", hintSubstr: "write:packages + read:packages"},
		{envVar: "WORKFLOW_TOKEN", hintSubstr: "repo + workflow scopes"},
		{envVar: "REPO_TOKEN", hintSubstr: "Contents:Read on the component repos"},
	}

	for _, tc := range cases {
		t.Run(tc.envVar, func(t *testing.T) {
			plantHappyTools(t)
			setAllEnvTokens(t)
			t.Setenv(tc.envVar, "")

			err := verifyEnvironmentWithClient(nil, "", neverFireClient(t))
			if err == nil {
				t.Fatalf("expected error when %s is missing, got nil", tc.envVar)
			}
			msg := err.Error()

			if !strings.Contains(msg, "Missing required environment variable(s)") {
				t.Errorf("error missing the aggregated-missing-vars header. Got:\n%s", msg)
			}
			if !strings.Contains(msg, tc.envVar) {
				t.Errorf("error did not name %s. Got:\n%s", tc.envVar, msg)
			}
			if tc.hintSubstr != "" && !strings.Contains(msg, tc.hintSubstr) {
				t.Errorf("error missing hint substring %q for %s. Got:\n%s", tc.hintSubstr, tc.envVar, msg)
			}
		})
	}
}

// TestVerifyEnvironment_AllEnvVarsMissing pins the all-missing path: every
// required variable surfaces in one error so the user fixes them all at once.
func TestVerifyEnvironment_AllEnvVarsMissing(t *testing.T) {
	plantHappyTools(t)
	t.Setenv("DOCKERHUB_USERNAME", "")
	t.Setenv("DOCKERHUB_TOKEN", "")
	t.Setenv("SONAR_TOKEN", "")
	t.Setenv("GHCR_TOKEN", "")
	t.Setenv("WORKFLOW_TOKEN", "")
	t.Setenv("REPO_TOKEN", "")

	err := verifyEnvironmentWithClient(nil, "", neverFireClient(t))
	if err == nil {
		t.Fatal("expected error when every env var is missing, got nil")
	}
	msg := err.Error()

	for _, name := range []string{
		"DOCKERHUB_USERNAME",
		"DOCKERHUB_TOKEN",
		"SONAR_TOKEN",
		"GHCR_TOKEN",
		"WORKFLOW_TOKEN",
		"REPO_TOKEN",
	} {
		if !strings.Contains(msg, name) {
			t.Errorf("error did not name %s. Got:\n%s", name, msg)
		}
	}
}
