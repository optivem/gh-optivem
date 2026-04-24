// Package config — token authentication checks.
//
// Presence checks in validateEnvTokens only prove the env vars are set; the
// values may still be expired, revoked, or wrong for the account. These
// checks call each provider with a minimal authenticated request so we fail
// fast before any repos or Sonar projects get created.
//
// All checks run in parallel and every failure is collected, so the user
// sees every broken token in one pass instead of fix-one-see-the-next.
//
// Note: these checks run from the user's local IP, not the GitHub Actions
// runner IP that will later execute the scaffolded workflows. A token valid
// here can still be rate-limited from the runner side (Docker Hub free tier
// in particular). This catches the common cases — expired/revoked PATs,
// wrong username, missing scopes — not IP-based throttling.
package config

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/optivem/gh-optivem/internal/log"
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

type tokenAuthResult struct {
	name string // e.g. "DOCKERHUB_TOKEN"
	err  error  // nil on success
}

// validateTokensAuth runs live auth checks against each provider in parallel.
// Aborts via FatalExit if any token fails so the user sees every broken one
// at once instead of fix-one-retry-discover-next.
func validateTokensAuth(e envTokens, dryRun bool) {
	if dryRun {
		return
	}
	log.Info("Verifying credentials with providers...")

	client := &http.Client{Timeout: tokenAuthTimeout}

	type check struct {
		name string
		fn   func() error
	}
	checks := []check{
		{"DOCKERHUB_TOKEN", func() error { return verifyDockerHubAuth(client, e.dockerHubUsername, e.dockerHubToken) }},
		{"SONAR_TOKEN", func() error { return verifySonarToken(client, e.sonarToken) }},
		{"WORKFLOW_TOKEN", func() error { return verifyGitHubToken(client, e.workflowToken, "WORKFLOW_TOKEN") }},
		{"GHCR_TOKEN", func() error { return verifyGHCRToken(client, e.dockerHubUsername, e.ghcrToken) }},
	}

	results := make([]tokenAuthResult, len(checks))
	var wg sync.WaitGroup
	for i, c := range checks {
		wg.Add(1)
		go func(i int, c check) {
			defer wg.Done()
			results[i] = tokenAuthResult{name: c.name, err: c.fn()}
		}(i, c)
	}
	wg.Wait()

	var failures []tokenAuthResult
	for _, r := range results {
		if r.err == nil {
			log.Successf("  %s: valid", r.name)
			continue
		}
		failures = append(failures, r)
	}
	if len(failures) == 0 {
		return
	}

	// Aggregate all failures into one FatalExit so the user sees them together.
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Credential verification failed for %d token(s):\n", len(failures)))
	for _, f := range failures {
		b.WriteString("\n  ")
		b.WriteString(f.name)
		b.WriteString(": ")
		b.WriteString(f.err.Error())
	}
	log.FatalExit(b.String())
}

// verifyDockerHubAuth posts username+token to Docker Hub's login endpoint.
// A valid PAT returns 200 with a JWT; anything else is an auth failure.
func verifyDockerHubAuth(client *http.Client, username, token string) error {
	body, _ := json.Marshal(map[string]string{"username": username, "password": token})
	req, err := http.NewRequest("POST", "https://hub.docker.com/v2/users/login", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf(errFmtBuildRequest, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf(errFmtNetwork, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return fmt.Errorf("Docker Hub rejected credentials (HTTP 401).\n    "+
			"Check DOCKERHUB_USERNAME (%q) matches the owner of DOCKERHUB_TOKEN,\n    "+
			"and that the PAT is Active at https://app.docker.com/settings/personal-access-tokens", username)
	default:
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected HTTP %d from Docker Hub: %s", resp.StatusCode, truncate(string(b), 200))
	}
}

// verifySonarToken calls SonarCloud's token validation endpoint.
// A revoked/wrong token returns {"valid":false} with HTTP 200, not an error
// status, so we need to parse the body.
func verifySonarToken(client *http.Client, token string) error {
	req, err := http.NewRequest("GET", "https://sonarcloud.io/api/authentication/validate", nil)
	if err != nil {
		return fmt.Errorf(errFmtBuildRequest, err)
	}
	req.SetBasicAuth(token, "")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf(errFmtNetwork, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP %d from SonarCloud", resp.StatusCode)
	}
	var v struct {
		Valid bool `json:"valid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	if !v.Valid {
		return fmt.Errorf("SonarCloud token is not valid (expired or revoked).\n    " +
			"Generate a new one at https://sonarcloud.io/account/security\n    " +
			"Then: export SONAR_TOKEN=<your-token>")
	}
	return nil
}

// verifyGitHubToken calls GitHub's /user endpoint. Also checks the token
// carries 'repo' scope via the X-OAuth-Scopes response header — workflow
// pushes and tag creation both require it.
func verifyGitHubToken(client *http.Client, token, name string) error {
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return fmt.Errorf(errFmtBuildRequest, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf(errFmtNetwork, err)
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

	// X-OAuth-Scopes is "repo, workflow" (comma-separated) for classic PATs.
	// Fine-grained tokens don't set this header — accept them as-is, since
	// later steps will surface a clearer error if a specific permission is missing.
	scopes := resp.Header.Get("X-OAuth-Scopes")
	if scopes != "" && !scopeContains(scopes, "repo") {
		return fmt.Errorf("%s is missing required 'repo' scope (current scopes: %s).\n    "+
			"Regenerate the token with repo + workflow scopes at https://github.com/settings/tokens", name, scopes)
	}
	return nil
}

// verifyGHCRToken validates the GHCR_TOKEN as a GitHub PAT via the GitHub
// API, then confirms it carries the 'read:packages' (and ideally
// 'write:packages') scope required to pull/push images from GHCR.
//
// We don't authenticate against ghcr.io/v2/ directly because Docker
// Registry v2 uses a bearer-token exchange flow — plain basic auth against
// /v2/ can return 401 even for valid credentials. Verifying the underlying
// PAT via api.github.com is faster and gives a clearer error message.
func verifyGHCRToken(client *http.Client, username, token string) error {
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return fmt.Errorf(errFmtBuildRequest, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf(errFmtNetwork, err)
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
	return nil
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
