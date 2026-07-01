// Package config — environment verification: local-tool presence + token
// authentication.
//
// VerifyEnvironment is the single entry point. It runs three classes of
// check in one pass:
//
//   - Local-tool presence (gh CLI auth, actionlint) — always runs, since
//     these have no dependency on env-var values. See tool_checks.go.
//   - Env-var presence — collects every missing required variable.
//   - Live provider auth — POST/GET against Docker Hub, SonarCloud, and
//     GitHub /user for each token, in parallel. Only runs when all env
//     vars are present (each call needs its token value).
//
// All failures are collected and surfaced together, so the user fixes
// everything in one pass instead of fix-one-rerun-discover-next.
//
// Note: provider checks run from the user's local IP, not the GitHub
// Actions runner IP that will later execute the scaffolded workflows. A
// token valid here can still be rate-limited from the runner side (Docker
// Hub free tier in particular). This catches the common cases —
// expired/revoked PATs, wrong username, missing scopes — not IP-based
// throttling.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/optivem/gh-optivem/internal/kernel/log"
	"github.com/optivem/gh-optivem/internal/kernel/shell"
)

// tokenAuthTimeout is the per-request cap. Providers usually answer in
// <500ms; anything over this means a network problem the user needs to
// resolve before scaffolding anyway.
const tokenAuthTimeout = 10 * time.Second

// Shared error format strings so failures across providers look identical.
const (
	errFmtBuildRequest = "build request: %w"
	errFmtNetwork      = "network error: %w"
)

type checkResult struct {
	name string // e.g. "gh CLI auth", "DOCKERHUB_TOKEN"
	err  error  // nil on success
}

// verifyDockerHubAuth posts username+token to Docker Hub's login endpoint.
// A valid PAT returns 200 with a JWT; anything else is an auth failure.
//
// Wrapped in shell.RetryWithPolicy so a transient 5xx / network blip from
// hub.docker.com doesn't kill the verify pass — same regex/backoff bash
// gh-retry / sonar-retry use. 4xx (incl. 401) is hard-fail and surfaces
// immediately as today.
func verifyDockerHubAuth(client *http.Client, username, token string) error {
	body, _ := json.Marshal(map[string]string{"username": username, "password": token})

	var statusCode int
	var respBody []byte
	_, retryErr := shell.RetryWithPolicy(
		shell.RetryTransient(), shell.RetryHardFail(), "dockerhub-retry",
		func() (string, error) {
			req, err := http.NewRequest("POST", "https://hub.docker.com/v2/users/login", strings.NewReader(string(body)))
			if err != nil {
				return "", fmt.Errorf(errFmtBuildRequest, err)
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				return err.Error(), fmt.Errorf(errFmtNetwork, err)
			}
			defer resp.Body.Close()
			statusCode = resp.StatusCode
			respBody, _ = io.ReadAll(resp.Body)
			summary := fmt.Sprintf("HTTP %d\n%s", statusCode, string(respBody))
			if statusCode >= 500 {
				return summary, fmt.Errorf("HTTP %d from Docker Hub", statusCode)
			}
			return summary, nil
		})
	if retryErr != nil && statusCode == 0 {
		return retryErr
	}

	switch statusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return fmt.Errorf("Docker Hub rejected credentials (HTTP 401).\n    "+
			"Check DOCKERHUB_USERNAME (%q) matches the owner of DOCKERHUB_TOKEN,\n    "+
			"and that the PAT is Active at https://app.docker.com/settings/personal-access-tokens", username)
	default:
		return fmt.Errorf("unexpected HTTP %d from Docker Hub: %s", statusCode, truncate(string(respBody), 200))
	}
}

// verifySonarToken calls SonarCloud's token validation endpoint.
// A revoked/wrong token returns {"valid":false} with HTTP 200, not an error
// status, so we need to parse the body.
//
// Wrapped in shell.RetryWithPolicy so a transient 5xx / network blip from
// sonarcloud.io doesn't kill the verify pass.
func verifySonarToken(client *http.Client, token string) error {
	var statusCode int
	var respBody []byte
	_, retryErr := shell.RetryWithPolicy(
		shell.RetryTransient(), shell.RetryHardFail(), "sonar-retry",
		func() (string, error) {
			req, err := http.NewRequest("GET", "https://sonarcloud.io/api/authentication/validate", nil)
			if err != nil {
				return "", fmt.Errorf(errFmtBuildRequest, err)
			}
			req.SetBasicAuth(token, "")
			resp, err := client.Do(req)
			if err != nil {
				return err.Error(), fmt.Errorf(errFmtNetwork, err)
			}
			defer resp.Body.Close()
			statusCode = resp.StatusCode
			respBody, _ = io.ReadAll(resp.Body)
			summary := fmt.Sprintf("HTTP %d\n%s", statusCode, string(respBody))
			if statusCode >= 500 {
				return summary, fmt.Errorf("HTTP %d from SonarCloud", statusCode)
			}
			return summary, nil
		})
	if retryErr != nil && statusCode == 0 {
		return retryErr
	}

	if statusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP %d from SonarCloud", statusCode)
	}
	var v struct {
		Valid bool `json:"valid"`
	}
	if err := json.Unmarshal(respBody, &v); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	if !v.Valid {
		return fmt.Errorf("SonarCloud token is not valid (expired or revoked).\n    " +
			"Generate a new one at https://sonarcloud.io/account/security\n    " +
			"Then: export SONAR_TOKEN=<your-token>")
	}
	return nil
}

// githubUserAuthCheck calls GitHub's /user endpoint with the given Bearer
// token. Two retry layers compose:
//
//   - Inner (shell.RetryWithPolicy): retries 5xx / network transients with the
//     standard 4-attempt 5s/15s/45s backoff. A successful 401 still returns to
//     the outer layer for the per-token-throttle 401-retry below.
//   - Outer (one-shot 401-retry): when concurrent matrix jobs hit
//     api.github.com with the same PAT, GitHub's per-token throttling can
//     return a transient 401 (rather than 429/403) even though the token is
//     valid. Without this, a single transient miss kills the whole acceptance
//     job; one retry makes that vanishingly rare.
//
// Caller is responsible for closing the returned response body.
func githubUserAuthCheck(client *http.Client, token string) (*http.Response, error) {
	do := func() (*http.Response, error) {
		var statusCode int
		var hdrs http.Header
		var respBody []byte
		_, retryErr := shell.RetryWithPolicy(
			shell.RetryTransient(), shell.RetryHardFail(), "github-auth-retry",
			func() (string, error) {
				req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
				if err != nil {
					return "", fmt.Errorf(errFmtBuildRequest, err)
				}
				req.Header.Set("Authorization", "Bearer "+token)
				req.Header.Set("Accept", "application/vnd.github+json")
				resp, err := client.Do(req)
				if err != nil {
					return err.Error(), fmt.Errorf(errFmtNetwork, err)
				}
				defer resp.Body.Close()
				statusCode = resp.StatusCode
				hdrs = resp.Header
				respBody, _ = io.ReadAll(resp.Body)
				summary := fmt.Sprintf("HTTP %d\n%s", statusCode, string(respBody))
				if statusCode >= 500 {
					return summary, fmt.Errorf("HTTP %d from GitHub", statusCode)
				}
				return summary, nil
			})
		if retryErr != nil && statusCode == 0 {
			return nil, retryErr
		}
		// Reconstruct a response so the outer caller can keep using
		// resp.Header.Get / resp.Body.Close / resp.StatusCode unchanged.
		return &http.Response{
			StatusCode: statusCode,
			Header:     hdrs,
			Body:       io.NopCloser(strings.NewReader(string(respBody))),
		}, nil
	}

	resp, err := do()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}
	resp.Body.Close()

	// 2-5s jittered backoff so concurrent retriers don't re-collide.
	time.Sleep(2*time.Second + time.Duration(rand.IntN(3001))*time.Millisecond)

	return do()
}

// verifyGitHubToken calls GitHub's /user endpoint and checks the token
// carries every scope in requiredScopes via the X-OAuth-Scopes response
// header. Fine-grained tokens don't set that header — accept them as-is,
// since later steps will surface a clearer error if a specific permission
// is missing.
func verifyGitHubToken(client *http.Client, token, name string, requiredScopes []string) error {
	resp, err := githubUserAuthCheck(client, token)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("GitHub rejected %s (HTTP 401 — token expired or revoked).\n    "+
			"Create a new Personal Access Token (classic) at https://github.com/settings/tokens\n    "+
			"Then: export %s=<your-token>", name, name)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP %d from GitHub", resp.StatusCode)
	}

	warnIfExpiringSoon(resp, name)

	// X-OAuth-Scopes is comma-separated for classic PATs (e.g. "repo, workflow").
	scopes := resp.Header.Get("X-OAuth-Scopes")
	if scopes == "" {
		return nil
	}
	var missingScopes []string
	for _, s := range requiredScopes {
		if !scopeContains(scopes, s) {
			missingScopes = append(missingScopes, s)
		}
	}
	if len(missingScopes) > 0 {
		return fmt.Errorf("%s is missing required scope(s) %s (current scopes: %s).\n    "+
			"Regenerate the token with %s scopes at https://github.com/settings/tokens",
			name, strings.Join(missingScopes, " + "), scopes, strings.Join(requiredScopes, " + "))
	}
	return nil
}

// verifyGHCRToken validates the GHCR_TOKEN as a GitHub PAT via the GitHub
// API, confirms it carries the 'read:packages' (and ideally
// 'write:packages') scope required to pull/push images from GHCR, then
// exercises the actual ghcr.io OCI bearer-token exchange the runtime
// pipelines depend on (see ghcrOCITokenExchange) — the api.github.com check
// alone can pass for a token that ghcr.io's own auth flow still rejects.
func verifyGHCRToken(client *http.Client, token string) error {
	resp, err := githubUserAuthCheck(client, token)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("GitHub rejected GHCR_TOKEN (HTTP 401 — token expired or revoked).\n    " +
			"Create a new Personal Access Token (classic) with write:packages + read:packages scopes\n    " +
			"at https://github.com/settings/tokens, then: export GHCR_TOKEN=<your-token>")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP %d from GitHub", resp.StatusCode)
	}

	// Fine-grained tokens don't set X-OAuth-Scopes. For classic PATs, require
	// at least read:packages — write:packages implies it but we accept either.
	scopes := resp.Header.Get("X-OAuth-Scopes")
	if scopes != "" && !scopeContains(scopes, "read:packages") && !scopeContains(scopes, "write:packages") {
		return fmt.Errorf("GHCR_TOKEN is missing required package scopes (current scopes: %s).\n    "+
			"Regenerate the token with write:packages + read:packages at https://github.com/settings/tokens", scopes)
	}

	warnIfExpiringSoon(resp, "GHCR_TOKEN")

	body, _ := io.ReadAll(resp.Body)
	var user struct {
		Login string `json:"login"`
	}
	_ = json.Unmarshal(body, &user)
	if user.Login == "" {
		return fmt.Errorf("could not determine the GitHub account for GHCR_TOKEN (empty /user response) — " +
			"cannot verify the ghcr.io token exchange")
	}

	return ghcrOCITokenExchange(client, token, user.Login)
}

// ghcrOCITokenExchange performs the same ghcr.io OCI bearer-token exchange
// the runtime pipelines rely on (academy/actions/shared/ghcr-probe.sh:38-49),
// so a token that fails the real registry auth flow is caught here instead
// of only in a scheduled CI run.
//
// The scope path is scopeOwner + a fixed "probe" placeholder repo, not a
// real package — confirmed live against ghcr.io/token that the exchange
// issues a bearer identically whether the scoped repo exists or not (a
// missing package fails the later manifest request, not the token exchange
// itself), so no real target package is needed to validate the token.
func ghcrOCITokenExchange(client *http.Client, token, scopeOwner string) error {
	url := fmt.Sprintf("https://ghcr.io/token?service=ghcr.io&scope=repository:%s/probe:pull", scopeOwner)

	var statusCode int
	var respBody []byte
	_, retryErr := shell.RetryWithPolicy(
		shell.RetryTransient(), shell.RetryHardFail(), "ghcr-token-exchange-retry",
		func() (string, error) {
			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				return "", fmt.Errorf(errFmtBuildRequest, err)
			}
			req.SetBasicAuth("x-access-token", token)
			resp, err := client.Do(req)
			if err != nil {
				return err.Error(), fmt.Errorf(errFmtNetwork, err)
			}
			defer resp.Body.Close()
			statusCode = resp.StatusCode
			respBody, _ = io.ReadAll(resp.Body)
			summary := fmt.Sprintf("HTTP %d\n%s", statusCode, string(respBody))
			if statusCode >= 500 {
				return summary, fmt.Errorf("HTTP %d from ghcr.io", statusCode)
			}
			return summary, nil
		})
	if retryErr != nil && statusCode == 0 {
		return retryErr
	}

	var body struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(respBody, &body)
	if body.Token == "" {
		return fmt.Errorf("Failed to obtain a GHCR registry token from ghcr.io (HTTP %d) — the OCI token "+
			"exchange the runtime pipelines depend on returned no bearer.\n    "+
			"Verify GHCR_TOKEN carries write:packages + read:packages scopes and is not expired/revoked\n    "+
			"at https://github.com/settings/tokens\n    Response: %s",
			statusCode, truncate(string(respBody), 200))
	}
	return nil
}

// githubTokenExpirationHeader is set by GitHub on classic-PAT responses
// (absent for fine-grained tokens and OAuth apps).
const githubTokenExpirationHeader = "github-authentication-token-expiration"

// githubTokenExpirationLayout matches GitHub's documented format, e.g.
// "2026-07-08 00:00:00 UTC".
const githubTokenExpirationLayout = "2006-01-02 15:04:05 MST"

// warnIfExpiringSoon reads the classic-PAT expiration header, if present,
// and logs a warning with the expiration date — escalated when the token
// expires within 7 days, since these PATs back cron-scheduled pipelines
// that will start failing silently once they lapse.
func warnIfExpiringSoon(resp *http.Response, name string) {
	raw := resp.Header.Get(githubTokenExpirationHeader)
	if raw == "" {
		return
	}
	expiresAt, err := time.Parse(githubTokenExpirationLayout, raw)
	if err != nil {
		return
	}
	until := time.Until(expiresAt)
	if until <= 7*24*time.Hour {
		log.Warnf("%s expires %s (in %s) — rotate it soon; this backs a cron-scheduled pipeline "+
			"that will start failing silently once it lapses.", name, expiresAt.Format("2006-01-02"), until.Round(time.Hour))
		return
	}
	log.Warnf("%s expires %s.", name, expiresAt.Format("2006-01-02"))
}

// VerifyEnvironment runs every readiness check the gh-optivem CLI needs
// before scaffolding can succeed:
//
//   - Local-tool presence (gh CLI auth, actionlint) — see tool_checks.go.
//   - Per-language compiler presence (npm / dotnet / java) — gated on
//     `langs`; nil/empty `langs` skips this class entirely, which is the
//     `gh optivem environment verify` (no --lang) behaviour. With `langs`
//     populated, each unique entry adds one presence check that runs in
//     the same parallel fan-out as the rest.
//   - Deploy-conditional tool presence (docker) — gated on `deploy`;
//     empty `deploy` skips this class entirely, mirroring the `langs`
//     idiom. With `deploy="docker"`, the local-verify lifecycle's
//     `docker compose` dependency is checked alongside the rest.
//   - Required env-var presence: DOCKERHUB_TOKEN, SONAR_TOKEN, GHCR_TOKEN,
//     WORKFLOW_TOKEN, REPO_TOKEN, plus DOCKERHUB_USERNAME (an account
//     name, not a token).
//   - Live provider auth for each token (parallel HTTP calls).
//
// Local-tool checks always run, since they have no env-var dependency.
// Live HTTP checks only run when all required env vars are present — each
// call needs its token value — but missing-var errors are aggregated
// alongside any tool-check failures so the user sees everything broken in
// one pass.
//
// Workflow-only inputs (e.g. VERIFY_TOKEN, used by the gh-acceptance
// meta-test for `gh api` calls) are out of scope — those are validated by
// the workflow's own preflight steps.
//
// Designed to be invoked from a CI preflight job — fails fast before the
// scaffolding matrix fans out, so a single missing tool or expired
// credential surfaces once instead of once per matrix combo.
//
// Returns nil on full success, otherwise an aggregated error listing every
// failure. On nil return, prints one success line per check via the log
// package (caller is responsible for log.Init).
func VerifyEnvironment(langs []string, deploy string) error {
	return verifyEnvironmentWithClient(langs, deploy, &http.Client{Timeout: tokenAuthTimeout})
}

// verifyEnvironmentWithClient is the testable form of VerifyEnvironment: the
// caller supplies the *http.Client used for every live provider auth call.
// Tests substitute a fakeRoundTripper (see token_auth_test.go) so the
// aggregated-failure surface can be exercised without real network. The
// public VerifyEnvironment is a one-line wrapper that injects the default
// timeout-bounded client.
func verifyEnvironmentWithClient(langs []string, deploy string, client *http.Client) error {
	e := readEnvTokens()

	// requiredEnvVars is the single source shared with the presence-only
	// preflight check (MissingRequiredEnvVars), so both surfaces agree on
	// which credentials count as required.
	var missing []string
	for _, r := range requiredEnvVars() {
		if r.val == "" {
			missing = append(missing, r.name)
		}
	}

	log.Info("Verifying environment...")

	// Local-tool checks always run; they have no dependency on env-var values.
	// `check` is package-level (see tool_checks.go) so compilerChecksFor can
	// return []check directly.
	checks := []check{
		{"gh CLI auth", verifyGhAuth},
		{"actionlint", verifyActionlint},
	}
	// Per-language compiler presence, gated on the caller-supplied langs.
	// Nil/empty langs => no compiler checks (the standalone
	// `environment verify` surface with no --lang flag).
	checks = append(checks, compilerChecksFor(langs)...)
	// Deploy-conditional tool presence, gated on the caller-supplied deploy
	// target. Empty deploy => no deploy-conditional check (the standalone
	// `environment verify` surface with no --deploy flag).
	checks = append(checks, deployChecksFor(deploy)...)
	// Live HTTP checks only run when every required env var is present —
	// each one needs its token value. Missing-var errors are reported
	// separately in the aggregated error below.
	if len(missing) == 0 {
		checks = append(checks,
			check{"DOCKERHUB_TOKEN", func() error { return verifyDockerHubAuth(client, e.dockerHubUsername, e.dockerHubToken) }},
			check{"SONAR_TOKEN", func() error { return verifySonarToken(client, e.sonarToken) }},
			check{"GHCR_TOKEN", func() error { return verifyGHCRToken(client, e.ghcrToken) }},
			check{"WORKFLOW_TOKEN", func() error {
				return verifyGitHubToken(client, e.workflowToken, "WORKFLOW_TOKEN", []string{"repo", "workflow"})
			}},
			check{"REPO_TOKEN", func() error {
				return verifyGitHubToken(client, e.repoToken, "REPO_TOKEN", []string{"repo"})
			}},
		)
	}

	results := make([]checkResult, len(checks))
	var wg sync.WaitGroup
	for i, c := range checks {
		wg.Add(1)
		go func(i int, c check) {
			defer wg.Done()
			results[i] = checkResult{name: c.name, err: c.fn()}
		}(i, c)
	}
	wg.Wait()

	var failures []checkResult
	for _, r := range results {
		if r.err == nil {
			log.Successf("  %s: valid", r.name)
			continue
		}
		failures = append(failures, r)
	}

	if len(missing) == 0 && len(failures) == 0 {
		return nil
	}
	return buildEnvVerifyError(missing, failures)
}

func buildEnvVerifyError(missing []string, failures []checkResult) error {
	var b strings.Builder
	if len(missing) > 0 {
		fmt.Fprintf(&b, "Missing required environment variable(s): %s\n", strings.Join(missing, ", "))
		for _, name := range missing {
			b.WriteString("\n")
			b.WriteString(missingEnvHint(name))
		}
	}
	if len(failures) > 0 {
		if len(missing) > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "Verification failed for %d check(s):\n", len(failures))
		for _, f := range failures {
			b.WriteString("\n  ")
			b.WriteString(f.name)
			b.WriteString(": ")
			b.WriteString(f.err.Error())
		}
	}
	return errors.New(b.String())
}

func scopeContains(scopesHeader, want string) bool {
	for _, s := range strings.Split(scopesHeader, ",") {
		if strings.TrimSpace(s) == want {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

const (
	envRequiredSuffix  = " environment variable is required.\n"
	patSettingsURLLine = "  https://github.com/settings/tokens\n"
)

// missingEnvHint returns a multi-line hint explaining what the named env var
// is used for and how to create it. Returns a string (no exit) so callers can
// aggregate hints for several missing vars into one error and surface them
// together — fix-all-at-once instead of fix-one-rerun-discover-next.
func missingEnvHint(name string) string {
	switch name {
	case "GHCR_TOKEN":
		return name + envRequiredSuffix +
			"  The scaffolded repo's acceptance/prod stages use it to tag images in GHCR.\n" +
			"  Create a Personal Access Token (classic) with write:packages + read:packages scopes:\n" +
			patSettingsURLLine +
			"  Then: export GHCR_TOKEN=<your-token>"
	case "WORKFLOW_TOKEN":
		return name + envRequiredSuffix +
			"  The scaffolded repo's acceptance/QA/prod stages use it to push release tags\n" +
			"  (default GITHUB_TOKEN cannot push tags whose commit diffs workflow files).\n" +
			"  Create a Personal Access Token (classic) with repo + workflow scopes:\n" +
			patSettingsURLLine +
			"  Then: export WORKFLOW_TOKEN=<your-token>"
	case "REPO_TOKEN":
		return name + envRequiredSuffix +
			"  In multitier+multirepo scaffolds, the system-level prod-stage uses it to read\n" +
			"  each component repo's VERSION file via the GitHub API (cross-repo Contents:read).\n" +
			"  Create a Personal Access Token (classic) with repo scope, OR a fine-grained PAT\n" +
			"  with Contents:Read on the component repos:\n" +
			patSettingsURLLine +
			"  Then: export REPO_TOKEN=<your-token>"
	default:
		return fmt.Sprintf("%s environment variable is required", name)
	}
}
