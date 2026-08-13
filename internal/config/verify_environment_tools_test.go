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
	writeGhStub(t, dir, "echo You are not logged into any GitHub hosts\nexit 1")
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
	writeStubOSSpecific(t, dir, "gh", ghVersionGuard(ghVersionLine)+body) // body is built per-OS just above
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

// ghAuthStubBody builds a `gh auth status` stub that exits 0 and prints
// scopeLine verbatim (pass "" for a token gh reports no scopes for). The
// per-OS split exists because the real scope line is single-quoted: `sh`
// would strip those quotes as its own, while `cmd` keeps them, so only the
// POSIX form needs the outer double quotes to survive echo intact.
func ghAuthStubBody(scopeLine string) string {
	lines := []string{"echo Logged in to github.com account test-user"}
	if scopeLine != "" {
		if runtime.GOOS == "windows" {
			lines = append(lines, "echo "+scopeLine)
		} else {
			lines = append(lines, "echo \""+scopeLine+"\"")
		}
	}
	return strings.Join(append(lines, "exit 0"), "\n")
}

// plantHappyToolStubs plants passing stubs for every non-gh tool check, so a
// scope-focused test's only variable is what `gh auth status` prints.
func plantHappyToolStubs(t *testing.T, dir string) {
	t.Helper()
	for _, tool := range []string{"actionlint", "docker", "bash", "claude"} {
		writeStub(t, dir, tool, "exit 0")
	}
}

// TestVerifyEnvironment_GhScopesPresent pins the happy path: a token whose
// scope line covers everything in requiredGhScopes passes verification.
func TestVerifyEnvironment_GhScopesPresent(t *testing.T) {
	dir := mkPathDir(t)
	writeGhStub(t, dir, ghAuthStubBody(
		"  - Token scopes: 'gist', 'project', 'read:org', 'repo', 'workflow'"))
	plantHappyToolStubs(t, dir)
	setAllEnvTokens(t)

	if err := verifyEnvironmentWithClient(nil, happyAuthClient()); err != nil {
		t.Fatalf("expected nil for a token carrying every required scope, got:\n%s", err)
	}
}

// TestVerifyEnvironment_GhScopeMissing is the regression this check exists
// for (issue #58): `gh auth status` exits 0 for a token without `project`, so
// exit-code-only verification passed and `init` went on to create the repo,
// then died one step later at Ensure project board. Verification must now
// fail here — before any GitHub side effect — naming the scope and the
// command that grants it.
func TestVerifyEnvironment_GhScopeMissing(t *testing.T) {
	dir := mkPathDir(t)
	writeGhStub(t, dir, ghAuthStubBody(
		"  - Token scopes: 'gist', 'read:org', 'repo', 'workflow'"))
	plantHappyToolStubs(t, dir)
	setAllEnvTokens(t)

	err := verifyEnvironmentWithClient(nil, happyAuthClient())
	if err == nil {
		t.Fatal("expected error when the gh token lacks the project scope, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "project") {
		t.Errorf("error did not name the missing scope. Got:\n%s", msg)
	}
	if !strings.Contains(msg, "gh auth refresh -s project") {
		t.Errorf("error did not include the refresh command. Got:\n%s", msg)
	}
	// The scopes the token DOES carry must not be reported as missing.
	if strings.Contains(msg, "repo, project") || strings.Contains(msg, "workflow") {
		t.Errorf("error named a scope the token already has. Got:\n%s", msg)
	}
}

// TestVerifyEnvironment_GhScopesUnreported pins the indeterminate case as a
// pass, not a block. A fine-grained PAT or a GH_TOKEN/GITHUB_TOKEN env token
// has no classic OAuth scopes for gh to report; rejecting one would be a
// false negative against a perfectly valid token.
func TestVerifyEnvironment_GhScopesUnreported(t *testing.T) {
	dir := mkPathDir(t)
	writeGhStub(t, dir, ghAuthStubBody("")) // authenticated, no scope line
	plantHappyToolStubs(t, dir)
	setAllEnvTokens(t)

	if err := verifyEnvironmentWithClient(nil, happyAuthClient()); err != nil {
		t.Fatalf("expected nil when gh reports no token scopes, got:\n%s", err)
	}
}

// TestGhTokenScopes covers the parser directly against the shapes `gh auth
// status` actually emits, including the two the wiring above treats as
// indeterminate (line absent, and `none`).
func TestGhTokenScopes(t *testing.T) {
	cases := []struct {
		name     string
		output   string
		want     []string
		reported bool
	}{
		{
			name: "real gh line",
			output: "github.com\n" +
				"  ✓ Logged in to github.com account test-user (keyring)\n" +
				"  - Active account: true\n" +
				"  - Token scopes: 'admin:org', 'delete_repo', 'gist', 'project', 'read:org', 'repo', 'workflow'\n",
			want:     []string{"admin:org", "delete_repo", "gist", "project", "read:org", "repo", "workflow"},
			reported: true,
		},
		{
			name:     "none",
			output:   "  - Token scopes: none\n",
			want:     nil,
			reported: true,
		},
		{
			name:     "line absent",
			output:   "  ✓ Logged in to github.com account test-user (keyring)\n",
			want:     nil,
			reported: false,
		},
		{
			// Several accounts authenticated and the active one is printed
			// second. gh-optivem's own `gh` calls use the active account, so
			// its scopes are the ones that must be asserted — reading the
			// first block here would fail a perfectly good token.
			name: "active account is not first",
			output: "github.com\n" +
				"  ✓ Logged in to github.com account other-user (keyring)\n" +
				"  - Active account: false\n" +
				"  - Token scopes: 'gist', 'read:org'\n" +
				"\n" +
				"  ✓ Logged in to github.com account test-user (keyring)\n" +
				"  - Active account: true\n" +
				"  - Token scopes: 'project', 'repo', 'workflow'\n",
			want:     []string{"project", "repo", "workflow"},
			reported: true,
		},
		{
			// No block marked active: fall back to the first rather than
			// merging, which would assert a union no single token holds.
			name: "no active marker falls back to first",
			output: "  ✓ Logged in to github.com account a (keyring)\n" +
				"  - Token scopes: 'repo'\n" +
				"  ✓ Logged in to github.com account b (keyring)\n" +
				"  - Token scopes: 'project', 'workflow'\n",
			want:     []string{"repo"},
			reported: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, reported := ghTokenScopes(tc.output)
			if reported != tc.reported {
				t.Errorf("reported = %v, want %v", reported, tc.reported)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("scopes = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestMissingGhScopes pins the comparison: exact names, required-set order,
// and no modelling of scope containment.
func TestMissingGhScopes(t *testing.T) {
	cases := []struct {
		name string
		have []string
		want []string
	}{
		{name: "all present", have: []string{"repo", "workflow", "project", "gist"}, want: nil},
		{name: "project absent", have: []string{"repo", "workflow"}, want: []string{"project"}},
		{name: "none present", have: []string{"gist"}, want: []string{"repo", "workflow", "project"}},
		{
			// read:org is in gh's documented login minimum and is deliberately
			// not asserted — a token carrying only the required three passes.
			name: "read:org not required",
			have: []string{"repo", "workflow", "project"},
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := missingGhScopes(tc.have); strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("missingGhScopes(%v) = %v, want %v", tc.have, got, tc.want)
			}
		})
	}
}

// TestVerifyEnvironment_ActionlintMissing covers actionlint-not-on-PATH.
// gh and docker are planted as happy stubs so their checks pass; only
// actionlint should fail.
func TestVerifyEnvironment_ActionlintMissing(t *testing.T) {
	dir := mkPathDir(t)
	writeGhStub(t, dir, "echo Logged in to github.com\nexit 0")
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
	writeGhStub(t, dir, "echo Logged in to github.com\nexit 0")
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
	writeGhStub(t, dir, "echo Logged in to github.com\nexit 0")
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
	writeGhStub(t, dir, "echo Logged in to github.com\nexit 0")
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
	writeGhStub(t, dir, "echo Logged in to github.com\nexit 0")
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
	writeGhStub(t, dir, "echo Logged in to github.com\nexit 0")
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
	writeGhStub(t, dir, "echo Logged in to github.com\nexit 0")
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
	writeGhStub(t, dir, "echo Logged in to github.com\nexit 0")
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

// TestVerifyEnvironment_DockerDaemonUnreachable is the regression guard for
// issue #59: a `docker` binary that is present but whose daemon is not
// running (Docker Desktop installed but not started) used to pass
// verification on `exec.LookPath` alone, then die 6 phases into scaffolding
// at `docker compose build` — after a repo, board, environments, secrets and
// SonarCloud projects had already been created. `docker info` is the
// discriminator that catches this before any of that happens.
func TestVerifyEnvironment_DockerDaemonUnreachable(t *testing.T) {
	dir := mkPathDir(t)
	writeStub(t, dir, "docker",
		"echo Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running? >&2\nexit 1")
	writeGhStub(t, dir, "echo Logged in to github.com\nexit 0")
	writeStub(t, dir, "actionlint", "exit 0")
	writeStub(t, dir, "bash", "exit 0")
	writeStub(t, dir, "claude", "exit 0")
	setAllEnvTokens(t)

	err := verifyEnvironmentWithClient(nil, happyAuthClient())
	if err == nil {
		t.Fatal("expected error when the docker daemon is unreachable, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "docker daemon is not reachable") {
		t.Errorf("error did not mention daemon unreachability. Got:\n%s", msg)
	}
	if !strings.Contains(msg, "systemctl start docker") {
		t.Errorf("error did not include the start-the-daemon hint. Got:\n%s", msg)
	}
	if !strings.Contains(msg, "Cannot connect to the Docker daemon") {
		t.Errorf("error did not include the docker stub's output. Got:\n%s", msg)
	}
}

// TestVerifyEnvironment_DockerDaemonTimeout covers a daemon that never
// answers `docker info` at all — e.g. Docker Desktop still booting.
// dockerProbeTimeout is shrunk for the test so this doesn't cost a real 20s;
// the `docker` stub sleeps well past the shrunk deadline so
// exec.CommandContext's kill-on-deadline path fires for real.
func TestVerifyEnvironment_DockerDaemonTimeout(t *testing.T) {
	orig := dockerProbeTimeout
	dockerProbeTimeout = 200 * time.Millisecond
	t.Cleanup(func() { dockerProbeTimeout = orig })

	dir := mkPathDir(t)
	var body string
	if runtime.GOOS == "windows" {
		// mkPathDir points PATH at nothing but the stub dir, so a bare
		// `ping` would fail to resolve — %SystemRoot% survives that and
		// still finds the real binary.
		body = "\"%SystemRoot%\\System32\\ping.exe\" -n 3 127.0.0.1 >nul\nexit /b 0"
	} else {
		body = "/bin/sleep 2\nexit 0"
	}
	writeStubOSSpecific(t, dir, "docker", body)
	writeGhStub(t, dir, "echo Logged in to github.com\nexit 0")
	writeStub(t, dir, "actionlint", "exit 0")
	writeStub(t, dir, "bash", "exit 0")
	writeStub(t, dir, "claude", "exit 0")
	setAllEnvTokens(t)

	err := verifyEnvironmentWithClient(nil, happyAuthClient())
	if err == nil {
		t.Fatal("expected error when docker info times out, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "did not respond within") {
		t.Errorf("error did not mention the timeout. Got:\n%s", msg)
	}
	if !strings.Contains(msg, "systemctl start docker") {
		t.Errorf("error did not include the start-the-daemon hint. Got:\n%s", msg)
	}
}

// TestVerifyEnvironment_DockerComposeMissing covers a reachable daemon with
// no Compose v2 plugin installed — the case tool_checks.go used to assume
// away ("compose v2 is a docker sub-command, so the docker binary alone is
// sufficient"). `docker info` passes; `docker compose version` is the one
// that must fail and be believed.
func TestVerifyEnvironment_DockerComposeMissing(t *testing.T) {
	dir := mkPathDir(t)
	var body string
	if runtime.GOOS == "windows" {
		body = "if \"%1\"==\"info\" exit /b 0\n" +
			"if \"%1\"==\"compose\" (\n" +
			"echo docker: 'compose' is not a docker command. 1>&2\n" +
			"exit /b 1\n" +
			")\n" +
			"exit /b 0"
	} else {
		body = "if [ \"$1\" = \"info\" ]; then exit 0; fi\n" +
			"if [ \"$1\" = \"compose\" ]; then echo \"docker: 'compose' is not a docker command.\" >&2; exit 1; fi\n" +
			"exit 0"
	}
	writeStubOSSpecific(t, dir, "docker", body)
	writeGhStub(t, dir, "echo Logged in to github.com\nexit 0")
	writeStub(t, dir, "actionlint", "exit 0")
	writeStub(t, dir, "bash", "exit 0")
	writeStub(t, dir, "claude", "exit 0")
	setAllEnvTokens(t)

	err := verifyEnvironmentWithClient(nil, happyAuthClient())
	if err == nil {
		t.Fatal("expected error when the Compose v2 plugin is missing, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "docker Compose v2 plugin not found") {
		t.Errorf("error did not mention the missing Compose plugin. Got:\n%s", msg)
	}
	if !strings.Contains(msg, "docs.docker.com/compose/install") {
		t.Errorf("error did not include the Compose install URL. Got:\n%s", msg)
	}
}

// TestVerifyEnvironment_GhVersionBelowFloor is the regression guard for the
// other half of issue #59: gh 2.42.0 (the reporter's actual version)
// predates `gh project link` entirely, so a run with that CLI must fail
// preflight with an upgrade hint rather than surface much later as
// `unknown flag: --owner`.
func TestVerifyEnvironment_GhVersionBelowFloor(t *testing.T) {
	dir := mkPathDir(t)
	writeStubOSSpecific(t, dir, "gh",
		ghVersionGuard("gh version 2.42.0 2024-01-01")+"echo Logged in to github.com\nexit 0")
	writeStub(t, dir, "actionlint", "exit 0")
	writeStub(t, dir, "docker", "exit 0")
	writeStub(t, dir, "bash", "exit 0")
	writeStub(t, dir, "claude", "exit 0")
	setAllEnvTokens(t)

	err := verifyEnvironmentWithClient(nil, happyAuthClient())
	if err == nil {
		t.Fatal("expected error when gh is older than the version floor, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "gh CLI 2.42.0 is older than the required v2.45.0") {
		t.Errorf("error did not name the installed and required versions. Got:\n%s", msg)
	}
	if !strings.Contains(msg, "cli.github.com") {
		t.Errorf("error did not include the gh upgrade URL. Got:\n%s", msg)
	}
	if !strings.Contains(msg, "gh project link") {
		t.Errorf("error did not name the feature the floor exists for. Got:\n%s", msg)
	}
}
