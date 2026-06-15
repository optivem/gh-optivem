package config

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/optivem/gh-optivem/internal/kernel/shell"
)

// routedAuthClient builds an http.Client whose fake transport dispatches by
// (Host, GitHub bearer token) so per-token GitHub assertions can target the
// right verifier even though WORKFLOW / REPO / GHCR all hit the same URL.
//
// The github map's keys match the placeholder values set by setAllEnvTokens
// — "test-workflow-token", "test-repo-token", "test-ghcr-token". Any token
// not in the map gets a 200 OK with maximally-permissive scopes (so the
// non-target verifiers don't pollute the aggregated error).
type tokenRoutes struct {
	dockerhub http.HandlerFunc
	sonar     http.HandlerFunc
	github    map[string]http.HandlerFunc
}

func routedAuthClient(routes tokenRoutes) *http.Client {
	return &http.Client{Transport: &fakeRoundTripper{handler: func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Host {
		case "hub.docker.com":
			if routes.dockerhub != nil {
				routes.dockerhub(w, req)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"token":"jwt"}`)
		case "sonarcloud.io":
			if routes.sonar != nil {
				routes.sonar(w, req)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"valid":true}`)
		case "api.github.com":
			auth := req.Header.Get("Authorization")
			token := strings.TrimPrefix(auth, "Bearer ")
			if h, ok := routes.github[token]; ok {
				h(w, req)
				return
			}
			// Default: happy with every scope present.
			w.Header().Set("X-OAuth-Scopes", "repo, workflow, write:packages, read:packages")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"login":"test-user"}`)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}}}
}

// stubShellSleep silences the inner shell.RetryWithPolicy backoff so 5xx
// retries don't dilate test time. The outer 401-retry inside
// githubUserAuthCheck uses plain time.Sleep (2-5s); tests that exercise the
// 401 path pay that cost — same trade-off as
// TestGithubUserAuthCheck_OuterRetryStillFiresOn401 in token_auth_test.go.
func stubShellSleep(t *testing.T) {
	t.Helper()
	restore := shell.SetSleepFnForTest(func(time.Duration) {})
	t.Cleanup(restore)
}

func TestVerifyEnvironment_DockerHubRejected(t *testing.T) {
	plantHappyTools(t)
	setAllEnvTokens(t)
	stubShellSleep(t)

	client := routedAuthClient(tokenRoutes{
		dockerhub: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, "denied")
		},
	})

	err := verifyEnvironmentWithClient(nil, "", client)
	if err == nil {
		t.Fatal("expected error when Docker Hub rejects credentials, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "DOCKERHUB_TOKEN") {
		t.Errorf("error did not name DOCKERHUB_TOKEN. Got:\n%s", msg)
	}
	if !strings.Contains(msg, "Docker Hub rejected credentials") {
		t.Errorf("error did not include the Docker Hub rejection hint. Got:\n%s", msg)
	}
}

func TestVerifyEnvironment_SonarRejected(t *testing.T) {
	plantHappyTools(t)
	setAllEnvTokens(t)
	stubShellSleep(t)

	client := routedAuthClient(tokenRoutes{
		sonar: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"valid":false}`)
		},
	})

	err := verifyEnvironmentWithClient(nil, "", client)
	if err == nil {
		t.Fatal("expected error when Sonar reports valid:false, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "SONAR_TOKEN") {
		t.Errorf("error did not name SONAR_TOKEN. Got:\n%s", msg)
	}
	if !strings.Contains(msg, "SonarCloud token is not valid") {
		t.Errorf("error did not include the Sonar invalid-token hint. Got:\n%s", msg)
	}
}

func TestVerifyEnvironment_GitHubTokenRejected(t *testing.T) {
	cases := []struct {
		name       string
		token      string
		envVar     string
		hintSubstr string
	}{
		{
			name:       "WORKFLOW_TOKEN 401",
			token:      "test-workflow-token",
			envVar:     "WORKFLOW_TOKEN",
			hintSubstr: "GitHub rejected WORKFLOW_TOKEN",
		},
		{
			name:       "REPO_TOKEN 401",
			token:      "test-repo-token",
			envVar:     "REPO_TOKEN",
			hintSubstr: "GitHub rejected REPO_TOKEN",
		},
		{
			name:       "GHCR_TOKEN 401",
			token:      "test-ghcr-token",
			envVar:     "GHCR_TOKEN",
			hintSubstr: "GitHub rejected GHCR_TOKEN",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plantHappyTools(t)
			setAllEnvTokens(t)
			stubShellSleep(t)

			client := routedAuthClient(tokenRoutes{
				github: map[string]http.HandlerFunc{
					tc.token: func(w http.ResponseWriter, _ *http.Request) {
						w.WriteHeader(http.StatusUnauthorized)
						_, _ = io.WriteString(w, "Bad credentials")
					},
				},
			})

			err := verifyEnvironmentWithClient(nil, "", client)
			if err == nil {
				t.Fatalf("expected error when GitHub rejects %s, got nil", tc.envVar)
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.hintSubstr) {
				t.Errorf("error missing hint %q for %s. Got:\n%s", tc.hintSubstr, tc.envVar, msg)
			}
		})
	}
}

func TestVerifyEnvironment_GitHubMissingScope(t *testing.T) {
	cases := []struct {
		name       string
		token      string
		envVar     string
		grantScope string // scope returned in X-OAuth-Scopes (insufficient for envVar)
		hintSubstr string
	}{
		{
			name:       "WORKFLOW_TOKEN missing workflow",
			token:      "test-workflow-token",
			envVar:     "WORKFLOW_TOKEN",
			grantScope: "repo",
			hintSubstr: "WORKFLOW_TOKEN is missing required scope(s) workflow",
		},
		{
			name:       "GHCR_TOKEN missing package scopes",
			token:      "test-ghcr-token",
			envVar:     "GHCR_TOKEN",
			grantScope: "repo",
			hintSubstr: "GHCR_TOKEN is missing required package scopes",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plantHappyTools(t)
			setAllEnvTokens(t)
			stubShellSleep(t)

			grant := tc.grantScope
			client := routedAuthClient(tokenRoutes{
				github: map[string]http.HandlerFunc{
					tc.token: func(w http.ResponseWriter, _ *http.Request) {
						w.Header().Set("X-OAuth-Scopes", grant)
						w.WriteHeader(http.StatusOK)
						_, _ = io.WriteString(w, `{"login":"test-user"}`)
					},
				},
			})

			err := verifyEnvironmentWithClient(nil, "", client)
			if err == nil {
				t.Fatalf("expected error when %s has insufficient scopes, got nil", tc.envVar)
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.hintSubstr) {
				t.Errorf("error missing hint %q for %s. Got:\n%s", tc.hintSubstr, tc.envVar, msg)
			}
		})
	}
}
