package config

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// happyAuthClient returns an http.Client whose fake transport answers every
// supported provider URL with a "valid" response. Used by tool-presence
// tests to isolate the failure to the tool check under test (HTTP layer is
// fully populated with passing responses, so no spurious assertions fire).
//
// Routes by req.URL.Host:
//   - hub.docker.com         → 200 {"token":"jwt"}
//   - sonarcloud.io          → 200 {"valid":true}
//   - api.github.com         → 200 with X-OAuth-Scopes covering every scope
//                              VerifyEnvironment asks for
func happyAuthClient() *http.Client {
	return &http.Client{Transport: &fakeRoundTripper{handler: func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Host {
		case "hub.docker.com":
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"token":"jwt"}`)
		case "sonarcloud.io":
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"valid":true}`)
		case "api.github.com":
			w.Header().Set("X-OAuth-Scopes", "repo, workflow, write:packages, read:packages")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"login":"test-user"}`)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}}}
}

// TestVerifyEnvironment_GhMissing covers the gh-CLI-not-on-PATH path: an
// empty PATH dir means exec.LookPath returns ErrNotFound and the verifier
// surfaces the install URL.
func TestVerifyEnvironment_GhMissing(t *testing.T) {
	mkPathDir(t) // empty — nothing planted
	setAllEnvTokens(t)

	err := verifyEnvironmentWithClient(nil, "", happyAuthClient())
	if err == nil {
		t.Fatal("expected error when gh is missing, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "gh CLI not found on PATH") {
		t.Errorf("error did not mention gh-missing. Got:\n%s", msg)
	}
	if !strings.Contains(msg, "https://cli.github.com/") {
		t.Errorf("error did not include the gh install URL. Got:\n%s", msg)
	}
}

// TestVerifyEnvironment_GhAuthFails covers the gh-present-but-unauthenticated
// path. The stub `gh` writes a "not logged in" body and exits non-zero;
// VerifyEnvironment must surface the auth-failure hint and include the
// stub's output.
func TestVerifyEnvironment_GhAuthFails(t *testing.T) {
	dir := mkPathDir(t)
	writeStub(t, dir, "gh", "echo You are not logged into any GitHub hosts\nexit 1")
	writeStub(t, dir, "actionlint", "exit 0")
	setAllEnvTokens(t)

	err := verifyEnvironmentWithClient(nil, "", happyAuthClient())
	if err == nil {
		t.Fatal("expected error when gh auth fails, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "gh CLI is not authenticated") {
		t.Errorf("error did not mention auth failure. Got:\n%s", msg)
	}
	if !strings.Contains(msg, "gh auth login") {
		t.Errorf("error did not include the auth-login hint. Got:\n%s", msg)
	}
	if !strings.Contains(msg, "You are not logged into any GitHub hosts") {
		t.Errorf("error did not include the gh stub's output. Got:\n%s", msg)
	}
}

// TestVerifyEnvironment_ActionlintMissing covers actionlint-not-on-PATH.
// gh is planted as a happy stub so its check passes; only actionlint should
// fail.
func TestVerifyEnvironment_ActionlintMissing(t *testing.T) {
	dir := mkPathDir(t)
	writeStub(t, dir, "gh", "echo Logged in to github.com\nexit 0")
	setAllEnvTokens(t)

	err := verifyEnvironmentWithClient(nil, "", happyAuthClient())
	if err == nil {
		t.Fatal("expected error when actionlint is missing, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "actionlint not found on PATH") {
		t.Errorf("error did not mention actionlint-missing. Got:\n%s", msg)
	}
	if !strings.Contains(msg, "actionlint@v1") {
		t.Errorf("error did not include the actionlint install hint. Got:\n%s", msg)
	}
}

// TestVerifyEnvironment_CompilerMissing fans out over the three compilers
// gated on --lang. Each subtest plants happy gh + actionlint stubs and the
// two non-target compilers so only the one under test is absent.
func TestVerifyEnvironment_CompilerMissing(t *testing.T) {
	cases := []struct {
		lang        string
		missingTool string
		hintSubstr  string
	}{
		{lang: projectconfig.LangTypescript, missingTool: "npm", hintSubstr: "https://nodejs.org/"},
		{lang: projectconfig.LangDotnet, missingTool: "dotnet", hintSubstr: "https://dotnet.microsoft.com/download"},
		{lang: projectconfig.LangJava, missingTool: "java", hintSubstr: "https://adoptium.net/"},
	}

	allCompilers := []string{"npm", "dotnet", "java"}

	for _, tc := range cases {
		t.Run(tc.lang, func(t *testing.T) {
			dir := mkPathDir(t)
			writeStub(t, dir, "gh", "echo Logged in to github.com\nexit 0")
			writeStub(t, dir, "actionlint", "exit 0")
			for _, c := range allCompilers {
				if c == tc.missingTool {
					continue
				}
				writeStub(t, dir, c, "exit 0")
			}
			setAllEnvTokens(t)

			err := verifyEnvironmentWithClient([]string{tc.lang}, "", happyAuthClient())
			if err == nil {
				t.Fatalf("expected error when %s is missing for --lang %s, got nil", tc.missingTool, tc.lang)
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.missingTool+" not found on PATH") {
				t.Errorf("error did not mention %s-missing. Got:\n%s", tc.missingTool, msg)
			}
			if !strings.Contains(msg, tc.hintSubstr) {
				t.Errorf("error missing install hint %q for %s. Got:\n%s", tc.hintSubstr, tc.missingTool, msg)
			}
		})
	}
}

// TestVerifyEnvironment_DockerMissing covers the deploy-conditional check.
// docker absent + --deploy docker → docker error.
func TestVerifyEnvironment_DockerMissing(t *testing.T) {
	dir := mkPathDir(t)
	writeStub(t, dir, "gh", "echo Logged in to github.com\nexit 0")
	writeStub(t, dir, "actionlint", "exit 0")
	setAllEnvTokens(t)

	err := verifyEnvironmentWithClient(nil, projectconfig.DeployDocker, happyAuthClient())
	if err == nil {
		t.Fatal("expected error when docker is missing for --deploy docker, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "docker not found on PATH") {
		t.Errorf("error did not mention docker-missing. Got:\n%s", msg)
	}
	if !strings.Contains(msg, "docker.com") {
		t.Errorf("error did not include a docker install URL. Got:\n%s", msg)
	}
}
