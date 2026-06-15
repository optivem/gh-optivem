package config

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/optivem/gh-optivem/internal/kernel/shell"
)

// fakeRoundTripper routes every outgoing request to a single test handler,
// regardless of the hardcoded host (hub.docker.com / sonarcloud.io /
// api.github.com). Lets us exercise the retry wrappers without exposing
// per-provider URL injection seams.
type fakeRoundTripper struct {
	handler http.HandlerFunc
}

func (f *fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	f.handler(rec, req)
	return rec.Result(), nil
}

func newTestClient(handler http.HandlerFunc) *http.Client {
	return &http.Client{Transport: &fakeRoundTripper{handler: handler}}
}

// TestTokenAuth_RetriesOn5xxThenSucceeds is the table-driven retry-loop test
// the plan calls for: each verifier observes 504 twice then 200 and the
// transport is hit three times for one logical call.
func TestTokenAuth_RetriesOn5xxThenSucceeds(t *testing.T) {
	cases := []struct {
		name string
		// successResponder writes the 200 body the verifier needs to consider
		// the response a success (some need a JSON shape, others just 200).
		successResponder func(http.ResponseWriter)
		invoke           func(client *http.Client) error
	}{
		{
			name: "verifyDockerHubAuth",
			successResponder: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(w, `{"token":"jwt"}`)
			},
			invoke: func(c *http.Client) error {
				return verifyDockerHubAuth(c, "user", "tok")
			},
		},
		{
			name: "verifySonarToken",
			successResponder: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(w, `{"valid":true}`)
			},
			invoke: func(c *http.Client) error {
				return verifySonarToken(c, "tok")
			},
		},
		{
			name: "githubUserAuthCheck",
			successResponder: func(w http.ResponseWriter) {
				w.Header().Set("X-OAuth-Scopes", "repo, workflow")
				w.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(w, `{"login":"user"}`)
			},
			invoke: func(c *http.Client) error {
				resp, err := githubUserAuthCheck(c, "tok")
				if err != nil {
					return err
				}
				resp.Body.Close()
				return nil
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sleeps []time.Duration
			restore := shell.SetSleepFnForTest(func(d time.Duration) { sleeps = append(sleeps, d) })
			defer restore()

			var calls int32
			client := newTestClient(func(w http.ResponseWriter, _ *http.Request) {
				n := atomic.AddInt32(&calls, 1)
				switch n {
				case 1, 2:
					w.WriteHeader(504)
					_, _ = io.WriteString(w, "Gateway Timeout")
				default:
					tc.successResponder(w)
				}
			})

			if err := tc.invoke(client); err != nil {
				t.Fatalf("expected success after retry, got error: %v", err)
			}
			if got := atomic.LoadInt32(&calls); got != 3 {
				t.Fatalf("transport calls = %d, want 3 (504, 504, 200)", got)
			}
			if len(sleeps) != 2 {
				t.Fatalf("sleeps = %d, want 2 backoffs between 3 attempts", len(sleeps))
			}
		})
	}
}

// TestTokenAuth_HardFailOn4xxNoRetry is the parity case: 4xx must not retry.
// Each verifier surfaces 4xx as its own kind of error (not all do the same
// thing on 401), so we just assert the transport observes exactly one call.
func TestTokenAuth_HardFailOn4xxNoRetry(t *testing.T) {
	cases := []struct {
		name   string
		status int
		invoke func(*http.Client) (callsAfter int32, err error)
	}{
		{
			name:   "verifyDockerHubAuth on 401",
			status: 401,
			invoke: func(c *http.Client) (int32, error) {
				return 0, verifyDockerHubAuth(c, "user", "tok")
			},
		},
		{
			name:   "verifySonarToken on 403",
			status: 403,
			invoke: func(c *http.Client) (int32, error) {
				return 0, verifySonarToken(c, "tok")
			},
		},
		// githubUserAuthCheck returns the 401 response to the caller (the
		// outer layer takes over for the per-token-throttle retry). Verify
		// the inner shell.RetryWithPolicy doesn't re-fire on 401 — the
		// transport must see exactly one call inside one do().
		{
			name:   "githubUserAuthCheck inner do on 403",
			status: 403,
			invoke: func(c *http.Client) (int32, error) {
				resp, err := githubUserAuthCheck(c, "tok")
				if err != nil {
					return 0, err
				}
				resp.Body.Close()
				return 0, nil
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sleeps []time.Duration
			restore := shell.SetSleepFnForTest(func(d time.Duration) { sleeps = append(sleeps, d) })
			defer restore()

			var calls int32
			client := newTestClient(func(w http.ResponseWriter, _ *http.Request) {
				atomic.AddInt32(&calls, 1)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, "denied")
			})

			_, _ = tc.invoke(client)
			if got := atomic.LoadInt32(&calls); got != 1 {
				t.Fatalf("transport calls = %d, want 1 (no retry on 4xx)", got)
			}
			if len(sleeps) != 0 {
				t.Fatalf("sleeps = %d, want 0", len(sleeps))
			}
		})
	}
}

// TestGithubUserAuthCheck_OuterRetryStillFiresOn401 confirms the existing
// one-shot 401-retry layer still composes correctly: a 401 response from
// the inner layer triggers the outer sleep + second do() call. The outer
// sleep uses time.Sleep directly (not the shell sleepFn), so we accept it
// — the test only verifies the request count.
func TestGithubUserAuthCheck_OuterRetryStillFiresOn401(t *testing.T) {
	restore := shell.SetSleepFnForTest(func(d time.Duration) {})
	defer restore()

	var calls int32
	client := newTestClient(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(401)
			_, _ = io.WriteString(w, "Unauthorized")
			return
		}
		w.Header().Set("X-OAuth-Scopes", "repo")
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{"login":"user"}`)
	})

	// Skip the real 2-5s outer sleep by patching it via a brief retry.
	// The outer sleep is plain time.Sleep — we live with it being short by
	// using a small jitter window via a subtest run that we'll drop in as
	// a follow-up if needed. For now: just confirm two transport calls.
	resp, err := githubUserAuthCheck(client, "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("transport calls = %d, want 2 (one initial + one outer 401-retry)", got)
	}
	if !strings.Contains(resp.Header.Get("X-OAuth-Scopes"), "repo") {
		t.Errorf("expected X-OAuth-Scopes header on second response, got %q", resp.Header.Get("X-OAuth-Scopes"))
	}
}
