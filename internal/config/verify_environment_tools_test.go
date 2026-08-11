package config

import (
	"io"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// stubGhAuthRetrySleep overrides verifyGhAuth's backoff to a no-op for the
// duration of the test, so retry-path tests don't pay the real 2-5s wait.
func stubGhAuthRetrySleep(t *testing.T) {
	t.Helper()
	orig := ghAuthRetrySleep
	ghAuthRetrySleep = func(time.Duration) {}
	t.Cleanup(func() { ghAuthRetrySleep = orig })
}

// happyAuthClient returns an http.Client whose fake transport answers every
// supported provider URL with a "valid" response. Used by tool-presence
// tests to isolate the failure to the tool check under test (HTTP layer is
// fully populated with passing responses, so no spurious assertions fire).
//
// Routes by req.URL.Host:
//   - hub.docker.com         → 200 {"token":"jwt"}
//   - sonarcloud.io          → 200 {"valid":true}
//   - api.github.com         → 200 with X-OAuth-Scopes covering every scope
//     VerifyEnvironment asks for
//   - ghcr.io                → 200 {"token":"jwt"} (OCI token exchange)
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
		case "ghcr.io":
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"token":"jwt"}`)
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

	err := verifyEnvironmentWithClient(nil, happyAuthClient())
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
	stubGhAuthRetrySleep(t) // always-fail path retries once; skip the real backoff
	dir := mkPathDir(t)
	writeStub(t, dir, "gh", "echo You are not logged into any GitHub hosts\nexit 1")
	writeStub(t, dir, "actionlint", "exit 0")
	writeStub(t, dir, "docker", "exit 0")
	writeStub(t, dir, "bash", "exit 0")
	writeStub(t, dir, "claude", "exit 0")
	setAllEnvTokens(t)

	err := verifyEnvironmentWithClient(nil, happyAuthClient())
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

// TestVerifyEnvironment_GhAuthRetryRecovers covers the transient-flake path:
// a `gh` stub that fails its FIRST `gh auth status` and succeeds on the
// SECOND (a marker file toggles the exit code between invocations). With the
// one-shot retry in verifyGhAuth, VerifyEnvironment must recover and return
// nil — a single blip under concurrent matrix load no longer kills the combo.
func TestVerifyEnvironment_GhAuthRetryRecovers(t *testing.T) {
	stubGhAuthRetrySleep(t) // no real backoff between the two attempts
	dir := mkPathDir(t)

	// Fail while the marker is absent (first call), plant it, exit 1; on the
	// second call the marker exists, so exit 0.
	marker := filepath.Join(dir, "gh_flaky_marker")
	var body string
	if runtime.GOOS == "windows" {
		body = "if exist \"" + marker + "\" exit 0\n" +
			"type nul > \"" + marker + "\"\n" +
			"echo transient auth blip\n" +
			"exit 1"
	} else {
		body = "if [ -f \"" + marker + "\" ]; then exit 0; fi\n" +
			": > \"" + marker + "\"\n" +
			"echo transient auth blip\n" +
			"exit 1"
	}
	writeStub(t, dir, "gh", body)
	writeStub(t, dir, "actionlint", "exit 0")
	writeStub(t, dir, "docker", "exit 0")
	writeStub(t, dir, "bash", "exit 0")
	writeStub(t, dir, "claude", "exit 0")
	setAllEnvTokens(t)

	err := verifyEnvironmentWithClient(nil, happyAuthClient())
	if err != nil {
		t.Fatalf("expected nil after the retry recovered gh auth, got:\n%s", err)
	}
}

// TestVerifyEnvironment_ActionlintMissing covers actionlint-not-on-PATH.
// gh and docker are planted as happy stubs so their checks pass; only
// actionlint should fail.
func TestVerifyEnvironment_ActionlintMissing(t *testing.T) {
	dir := mkPathDir(t)
	writeStub(t, dir, "gh", "echo Logged in to github.com\nexit 0")
	writeStub(t, dir, "docker", "exit 0")
	writeStub(t, dir, "bash", "exit 0")
	writeStub(t, dir, "claude", "exit 0")
	setAllEnvTokens(t)

	err := verifyEnvironmentWithClient(nil, happyAuthClient())
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
// gated on --lang. Each subtest plants happy gh + actionlint + docker stubs
// and the two non-target compilers so only the one under test is absent.
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
			runCompilerMissingSubtest(t, tc.lang, tc.missingTool, tc.hintSubstr, allCompilers)
		})
	}
}

func runCompilerMissingSubtest(t *testing.T, lang, missingTool, hintSubstr string, allCompilers []string) {
	t.Helper()
	dir := mkPathDir(t)
	writeStub(t, dir, "gh", "echo Logged in to github.com\nexit 0")
	writeStub(t, dir, "actionlint", "exit 0")
	writeStub(t, dir, "docker", "exit 0")
	writeStub(t, dir, "bash", "exit 0")
	writeStub(t, dir, "claude", "exit 0")
	for _, c := range allCompilers {
		if c == missingTool {
			continue
		}
		writeStub(t, dir, c, "exit 0")
	}
	setAllEnvTokens(t)

	err := verifyEnvironmentWithClient([]string{lang}, happyAuthClient())
	if err == nil {
		t.Fatalf("expected error when %s is missing for --lang %s, got nil", missingTool, lang)
	}
	msg := err.Error()
	if !strings.Contains(msg, missingTool+" not found on PATH") {
		t.Errorf("error did not mention %s-missing. Got:\n%s", missingTool, msg)
	}
	if !strings.Contains(msg, hintSubstr) {
		t.Errorf("error missing install hint %q for %s. Got:\n%s", hintSubstr, missingTool, msg)
	}
}

// TestVerifyEnvironment_DockerMissing pins the docker check as
// unconditional: no --deploy flag exists and no lang is passed, yet docker
// absent from PATH must still fail. The local system runs on
// `docker compose` whatever the project deploys to, so gating this behind a
// deploy target would let a cloud-run scaffold pass verification and then
// fail at `gh optivem system start`.
func TestVerifyEnvironment_DockerMissing(t *testing.T) {
	dir := mkPathDir(t)
	writeStub(t, dir, "gh", "echo Logged in to github.com\nexit 0")
	writeStub(t, dir, "actionlint", "exit 0")
	writeStub(t, dir, "bash", "exit 0")
	writeStub(t, dir, "claude", "exit 0")
	setAllEnvTokens(t)

	err := verifyEnvironmentWithClient(nil, happyAuthClient())
	if err == nil {
		t.Fatal("expected error when docker is missing, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "docker not found on PATH") {
		t.Errorf("error did not mention docker-missing. Got:\n%s", msg)
	}
	if !strings.Contains(msg, "docker.com") {
		t.Errorf("error did not include a docker install URL. Got:\n%s", msg)
	}
}

// TestVerifyEnvironment_BashMissing pins the bash check as unconditional and
// as part of the no-flags surface. VerifyLocalSonar shells out to
// run-sonar.sh, so a Windows user with no Git Bash would otherwise pass
// verification and then die partway through `init` — after the repos have
// already been created.
func TestVerifyEnvironment_BashMissing(t *testing.T) {
	dir := mkPathDir(t)
	writeStub(t, dir, "gh", "echo Logged in to github.com\nexit 0")
	writeStub(t, dir, "actionlint", "exit 0")
	writeStub(t, dir, "docker", "exit 0")
	writeStub(t, dir, "claude", "exit 0")
	setAllEnvTokens(t)

	err := verifyEnvironmentWithClient(nil, happyAuthClient())
	if err == nil {
		t.Fatal("expected error when bash is missing, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "bash not found on PATH") {
		t.Errorf("error did not mention bash-missing. Got:\n%s", msg)
	}
	if !strings.Contains(msg, "git-scm.com/download/win") {
		t.Errorf("error did not include the Git Bash install URL. Got:\n%s", msg)
	}
}

// TestVerifyEnvironment_ClaudeMissing pins the claude check as part of the
// no-flags surface. Every ATDD agent runs as a `claude` subprocess, so a
// missing binary would otherwise pass verification and then surface during
// `implement` as an exec error naming the subprocess rather than the missing
// install.
func TestVerifyEnvironment_ClaudeMissing(t *testing.T) {
	dir := mkPathDir(t)
	writeStub(t, dir, "gh", "echo Logged in to github.com\nexit 0")
	writeStub(t, dir, "actionlint", "exit 0")
	writeStub(t, dir, "docker", "exit 0")
	writeStub(t, dir, "bash", "exit 0")
	setAllEnvTokens(t)

	err := verifyEnvironmentWithClient(nil, happyAuthClient())
	if err == nil {
		t.Fatal("expected error when claude is missing, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "claude not found on PATH") {
		t.Errorf("error did not mention claude-missing. Got:\n%s", msg)
	}
	if !strings.Contains(msg, "claude.com/claude-code") {
		t.Errorf("error did not include the Claude Code install URL. Got:\n%s", msg)
	}
	// Presence is all this check can prove — `claude --version` exits 0 when
	// signed out — so the sign-in step rides along in the message instead of
	// becoming a check that would have to guess.
	if !strings.Contains(msg, "sign in") {
		t.Errorf("error did not include the sign-in hint. Got:\n%s", msg)
	}
}

// TestVerifyEnvironment_ClaudeSkipped covers the CI opt-out: with
// GH_OPTIVEM_SKIP_CLAUDE_CHECK set, an absent claude must NOT fail
// verification. GitHub runners scaffold without ever reaching `implement`,
// so failing them for a tool that path never invokes would be noise.
func TestVerifyEnvironment_ClaudeSkipped(t *testing.T) {
	dir := mkPathDir(t)
	writeStub(t, dir, "gh", "echo Logged in to github.com\nexit 0")
	writeStub(t, dir, "actionlint", "exit 0")
	writeStub(t, dir, "docker", "exit 0")
	writeStub(t, dir, "bash", "exit 0")
	// claude deliberately NOT planted.
	setAllEnvTokens(t)
	t.Setenv(skipClaudeCheckEnv, "true")

	if err := verifyEnvironmentWithClient(nil, happyAuthClient()); err != nil {
		t.Fatalf("expected nil with the claude check skipped, got:\n%s", err)
	}
}

// TestVerifyEnvironment_ClaudeSkipUnsetRunsCheck guards the default. An empty
// or unset opt-out must leave the check enabled — a regression here would be
// invisible, since verification would simply go green on machines that cannot
// run a single ATDD agent.
func TestVerifyEnvironment_ClaudeSkipUnsetRunsCheck(t *testing.T) {
	dir := mkPathDir(t)
	writeStub(t, dir, "gh", "echo Logged in to github.com\nexit 0")
	writeStub(t, dir, "actionlint", "exit 0")
	writeStub(t, dir, "docker", "exit 0")
	writeStub(t, dir, "bash", "exit 0")
	// claude deliberately NOT planted.
	setAllEnvTokens(t)
	t.Setenv(skipClaudeCheckEnv, "")

	err := verifyEnvironmentWithClient(nil, happyAuthClient())
	if err == nil {
		t.Fatal("expected error with an empty skip value, got nil")
	}
	if !strings.Contains(err.Error(), "claude not found on PATH") {
		t.Errorf("empty skip value did not run the claude check. Got:\n%s", err)
	}
}

// TestVerifyEnvironment_ClaudeSkipInvalid pins the fail-loud contract on the
// opt-out itself. `SKIP=yes` reads as an opt-out to whoever wrote the
// workflow; running the check anyway would fail the job with a message about
// claude that says nothing about the typo that actually caused it.
func TestVerifyEnvironment_ClaudeSkipInvalid(t *testing.T) {
	dir := mkPathDir(t)
	writeStub(t, dir, "gh", "echo Logged in to github.com\nexit 0")
	writeStub(t, dir, "actionlint", "exit 0")
	writeStub(t, dir, "docker", "exit 0")
	writeStub(t, dir, "bash", "exit 0")
	writeStub(t, dir, "claude", "exit 0")
	setAllEnvTokens(t)
	t.Setenv(skipClaudeCheckEnv, "yes")

	err := verifyEnvironmentWithClient(nil, happyAuthClient())
	if err == nil {
		t.Fatal("expected error for a non-boolean skip value, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, skipClaudeCheckEnv) {
		t.Errorf("error did not name the offending variable. Got:\n%s", msg)
	}
	if !strings.Contains(msg, "not a valid boolean") {
		t.Errorf("error did not explain the parse failure. Got:\n%s", msg)
	}
}
