package shell

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestClassifyError(t *testing.T) {
	err := errors.New("exit 1")
	cases := []struct {
		name string
		out  string
		err  error
		want bool
	}{
		// 5xx + GitHub-flavoured transients
		{"HTTP 500", "HTTP 500 Internal Server Error", err, true},
		{"HTTP 502", "Bad Gateway (HTTP 502)", err, true},
		{"HTTP 503", "Service Unavailable\nHTTP 503", err, true},
		{"i/o timeout", "dial tcp: i/o timeout", err, true},
		{"connection reset", "read: connection reset by peer", err, true},
		{"TLS handshake", "TLS handshake failure", err, true},
		{"tls handshake lowercase", "tls: handshake failure", err, true},
		{"no such host", "dial tcp: lookup api.github.com: no such host", err, true},
		{"bad gateway text", "Bad Gateway", err, true},
		{"graphql internal error", "Something went wrong while executing your query", err, true},
		// Docker / sonarcloud / git transients picked up by the union
		{"sonar 5xx wording", "Error 504 on https://sonarcloud.io/api/...", err, true},
		{"sonar endpoint timeout", "Endpoint request timed out", err, true},
		{"docker context deadline", "context deadline exceeded", err, true},
		{"git RPC 5xx", "RPC failed; HTTP 502 curl 22", err, true},
		{"git could not resolve host", "fatal: unable to access 'https://...': Could not resolve host: github.com", err, true},
		{"http2 GOAWAY", "http2: server sent GOAWAY and closed the connection", err, true},
		// gh-optivem run 35614000906: empty API response body on env PUT.
		{"gh empty JSON body", "command failed: gh api repos/o/r/environments/production -X PUT: exit status 1\nunexpected end of JSON input", err, true},
		{"sonar bootstrapper 5xx", "Request failed with status code 502", err, true},
		{"sonar bootstrapper error", "Bootstrapper: An error occurred: Request failed with status code 403", err, true},
		{"docker daemon get unknown", `Error response from daemon: Get "https://ghcr.io/v2/": unknown`, err, true},

		// Hard-fail
		{"HTTP 404", "HTTP 404: Not Found", err, false},
		{"HTTP 403 rate limit", "HTTP 403: API rate limit exceeded", err, false},
		{"HTTP 422", "HTTP 422 Unprocessable Entity", err, false},
		{"docker manifest unknown", "manifest unknown: manifest unknown", err, false},
		{"docker name unknown", "name unknown: repository name not known to registry", err, false},
		{"sonar project not found", "Project key acme:foo does not exist", err, false},
		{"git remote rejected", "! [remote rejected] main -> main (pre-receive hook declined)", err, false},
		{"git fatal protocol", "fatal: protocol error: bad pack header", err, false},
		{"unauthorized", "unauthorized: authentication required", err, false},
		// Scanner-engine 4xx phrasing: `HTTP 4\d\d` does not match it, so
		// without the `Error 4\d\d on https://` clause this classified as
		// neither transient nor hard-fail. Mirror of the bash fix for
		// gh-optivem run 35261501113.
		{"sonar 4xx wording", "Error 401 on https://sonarcloud.io/batch/project.protobuf?key=x", err, false},
		{"RateLimitExceeded typed", "", &RateLimitExceeded{Msg: "rl"}, false},
		{"unknown error", "some unrelated failure", err, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyError(tc.out, tc.err)
			if got != tc.want {
				t.Fatalf("classifyError(%q) = %v, want %v", tc.out, got, tc.want)
			}
		})
	}
}

// withFakeSleep swaps sleepFn for the duration of a test. Restores on cleanup.
func withFakeSleep(t *testing.T, calls *[]time.Duration) {
	t.Helper()
	orig := sleepFn
	sleepFn = func(d time.Duration) { *calls = append(*calls, d) }
	t.Cleanup(func() { sleepFn = orig })
}

func TestRunWithRetryLoop_ImmediateSuccess(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)

	attempts := 0
	attempt := func() (string, error) {
		attempts++
		return "ok", nil
	}
	out, err := runWithRetryLoop(attempt, classifyError, 4, defaultRetryDelays, "retry")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "ok" {
		t.Fatalf("out = %q, want %q", out, "ok")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	if len(sleeps) != 0 {
		t.Fatalf("sleeps = %v, want none on immediate success", sleeps)
	}
}

func TestRunWithRetryLoop_TransientThenSuccess(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)

	transient := errors.New("exit 1")
	attempts := 0
	attempt := func() (string, error) {
		attempts++
		if attempts < 3 {
			return "HTTP 503 Service Unavailable", transient
		}
		return "ok", nil
	}
	out, err := runWithRetryLoop(attempt, classifyError, 4, defaultRetryDelays, "retry")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "ok" {
		t.Fatalf("out = %q, want %q", out, "ok")
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
	if len(sleeps) != 2 {
		t.Fatalf("sleeps = %v, want 2 retries before success", sleeps)
	}
	if sleeps[0] != 5*time.Second || sleeps[1] != 15*time.Second {
		t.Fatalf("sleeps = %v, want [5s 15s] per backoff schedule", sleeps)
	}
}

func TestRunWithRetryLoop_TransientExhausted(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)

	transient := errors.New("exit 1")
	attempts := 0
	attempt := func() (string, error) {
		attempts++
		return "HTTP 500 Internal Server Error", transient
	}
	out, err := runWithRetryLoop(attempt, classifyError, 4, defaultRetryDelays, "retry")
	if err == nil {
		t.Fatalf("expected error after exhausting retries, got nil")
	}
	if attempts != 4 {
		t.Fatalf("attempts = %d, want 4", attempts)
	}
	if out != "HTTP 500 Internal Server Error" {
		t.Fatalf("out = %q, want the last attempt's output", out)
	}
	// 4 attempts → 3 sleeps between them.
	if len(sleeps) != 3 {
		t.Fatalf("sleeps = %v, want 3 inter-attempt waits", sleeps)
	}
}

func TestRunWithRetryLoop_HardFailPassthrough(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)

	hardFail := errors.New("exit 1")
	attempts := 0
	attempt := func() (string, error) {
		attempts++
		return "HTTP 404: Not Found", hardFail
	}
	out, err := runWithRetryLoop(attempt, classifyError, 4, defaultRetryDelays, "retry")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 — hard-fail must not retry", attempts)
	}
	if out != "HTTP 404: Not Found" {
		t.Fatalf("out = %q, want the single attempt's output", out)
	}
	if len(sleeps) != 0 {
		t.Fatalf("sleeps = %v, want zero — hard-fail returns before any sleep", sleeps)
	}
}

// TestRunWithRetryLoop_PermissiveClassifier exercises the classifier shape
// MustRunPostCreate uses: retry on any non-RateLimitExceeded error, regardless
// of output wording. This is the property that makes it robust to GitHub
// changing the "Could not resolve to a Repository" message in the future.
func TestRunWithRetryLoop_PermissiveClassifier(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)

	permissive := func(_ string, err error) bool {
		var rle *RateLimitExceeded
		return !errors.As(err, &rle)
	}

	t.Run("retries arbitrary error wording", func(t *testing.T) {
		attempts := 0
		attempt := func() (string, error) {
			attempts++
			if attempts < 3 {
				return "totally novel vendor error message", errors.New("exit 1")
			}
			return "ok", nil
		}
		out, err := runWithRetryLoop(attempt, permissive, 4, defaultRetryDelays, "retry")
		if err != nil || out != "ok" || attempts != 3 {
			t.Fatalf("got attempts=%d out=%q err=%v, want attempts=3 out=ok err=nil", attempts, out, err)
		}
	})

	t.Run("rate limit still passes through", func(t *testing.T) {
		attempts := 0
		rlErr := &RateLimitExceeded{Msg: "rl"}
		attempt := func() (string, error) {
			attempts++
			return "", rlErr
		}
		_, err := runWithRetryLoop(attempt, permissive, 4, defaultRetryDelays, "retry")
		if attempts != 1 || err != rlErr {
			t.Fatalf("got attempts=%d err=%v, want attempts=1 err=rlErr", attempts, err)
		}
	})
}

// TestMustRunPostCreatePush_Classifier pins the exact server messages that
// drive classifyPostCreatePush. If GitHub changes either wording, this test
// fails and we update the regex deliberately — instead of silently bypassing
// the retry the next time replica lag bites at the post-create push site.
//
// Failure fixtures: "cannot lock ref 'refs/heads/main'" + "reference already
// exists" come from acceptance run 26456900412 job 77901798586 (matrix leg
// "Run (monolith, multirepo, java, typescript)"). The remaining fixtures lock
// in the negative cases — push failures that must fail fast.
func TestMustRunPostCreatePush_Classifier(t *testing.T) {
	err := errors.New("exit 1")
	cases := []struct {
		name string
		out  string
		err  error
		want bool
	}{
		{
			"cannot lock ref retries",
			"remote: cannot lock ref 'refs/heads/main': reference already exists\n ! [remote rejected] main -> main",
			err, true,
		},
		{
			"reference already exists retries",
			"remote: error: reference already exists",
			err, true,
		},
		{
			"pre-receive hook decline does not retry",
			"! [remote rejected] main -> main (pre-receive hook declined)",
			err, false,
		},
		{
			"non-fast-forward does not retry",
			"! [rejected]        main -> main (non-fast-forward)",
			err, false,
		},
		{
			"permission denied does not retry",
			"remote: Permission to owner/repo.git denied to user.",
			err, false,
		},
		{
			"rate limit passes through immediately",
			"", &RateLimitExceeded{Msg: "rl"}, false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyPostCreatePush(tc.out, tc.err)
			if got != tc.want {
				t.Fatalf("classifyPostCreatePush(%q) = %v, want %v", tc.out, got, tc.want)
			}
		})
	}
}

func TestRunWithRetryLoop_RateLimitPassthrough(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)

	rlErr := &RateLimitExceeded{Msg: "rate limited"}
	attempts := 0
	attempt := func() (string, error) {
		attempts++
		return "", rlErr
	}
	out, err := runWithRetryLoop(attempt, classifyError, 4, defaultRetryDelays, "retry")
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 — rate-limit must not retry", attempts)
	}
	if err != rlErr {
		t.Fatalf("err = %v, want the typed RateLimitExceeded to pass through", err)
	}
	if out != "" {
		t.Fatalf("out = %q, want empty", out)
	}
	if len(sleeps) != 0 {
		t.Fatalf("sleeps = %v, want zero", sleeps)
	}
}

// intentionalRetryShDifferences lists retry.sh alternatives that have no
// literal twin in the Go regexes because Go already covers them another way.
// Go compiles both regexes with (?i), so case-only variants live here.
var intentionalRetryShDifferences = map[string]string{
	`Connection reset by peer`: "covered by case-insensitive `connection reset`",
	`[Uu]nauthorized`:          "covered by case-insensitive `unauthorized`",
}

// TestRetryPatternsMatchVendoredRetrySh pins the Go transient/hard-fail lists
// to the vendored .github/scripts/retry.sh, so a pattern added on the bash
// side (e.g. `unexpected end of JSON input`, gh-optivem run 35614000906) can't
// silently go missing in Go again.
func TestRetryPatternsMatchVendoredRetrySh(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "..", ".github", "scripts", "retry.sh"))
	if err != nil {
		t.Fatalf("read vendored retry.sh: %v", err)
	}
	for _, tc := range []struct {
		bashVar string
		goRe    *regexp.Regexp
	}{
		{"_RETRY_RETRYABLE", retryTransient},
		{"_RETRY_HARD_FAIL", retryHardFail},
	} {
		m := regexp.MustCompile(tc.bashVar + `='([^']*)'`).FindSubmatch(src)
		if m == nil {
			t.Fatalf("%s='...' not found in retry.sh — was it renamed?", tc.bashVar)
		}
		goAlts := map[string]bool{}
		for _, a := range splitAlternation(strings.TrimPrefix(tc.goRe.String(), "(?i)")) {
			goAlts[strings.ToLower(a)] = true
		}
		for _, alt := range splitAlternation(string(m[1])) {
			if _, ok := intentionalRetryShDifferences[alt]; ok {
				continue
			}
			norm := strings.ReplaceAll(alt, "[0-9]", `\d`)
			if !goAlts[strings.ToLower(norm)] {
				t.Errorf("retry.sh %s has %q with no Go twin in retry.go; add it to the Go regex, or to intentionalRetryShDifferences with a reason", tc.bashVar, alt)
			}
		}
	}
}

// splitAlternation splits a regex on top-level `|`, ignoring pipes inside
// (...) groups and [...] classes.
func splitAlternation(re string) []string {
	var parts []string
	depth, inClass, start := 0, false, 0
	for i := 0; i < len(re); i++ {
		switch c := re[i]; {
		case c == '\\':
			i++
		case inClass:
			if c == ']' {
				inClass = false
			}
		case c == '[':
			inClass = true
		case c == '(':
			depth++
		case c == ')':
			depth--
		case c == '|' && depth == 0:
			parts = append(parts, re[start:i])
			start = i + 1
		}
	}
	return append(parts, re[start:])
}
