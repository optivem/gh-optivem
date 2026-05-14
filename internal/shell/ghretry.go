package shell

import (
	"errors"
	"regexp"

	"github.com/optivem/gh-optivem/internal/log"
)

// Retry policy for `gh` CLI invocations. Mirrors the bash gh_retry wrapper
// in optivem/actions/shared/gh-retry.sh: 4 attempts, 5s → 15s → 45s backoff,
// retries 5xx/network/TLS/DNS transients, passes through 4xx (incl. rate limit)
// immediately so CheckRateLimit / callers can make their own decision.
//
// Engine (runWithRetryLoop / sleepFn / classifyFn) lives in retrycore.go. This
// file only owns gh-specific knobs and the typed-error-aware classifier.
var (
	ghRetryAttempts = defaultRetryAttempts
	ghRetryDelays   = defaultRetryDelays

	// Patterns that indicate a transient failure worth retrying. The
	// "Something went wrong while executing your query" alternative covers
	// GitHub's GraphQL internal-error wording that surfaced in acceptance
	// run 25877369208's "Ensure project board" step.
	ghRetryTransient = regexp.MustCompile(
		`(?i)` +
			`HTTP 5\d\d|` +
			`timeout|timed out|i/o timeout|` +
			`connection reset|connection refused|` +
			`\bEOF\b|was closed|broken pipe|` +
			`TLS handshake|tls:.*handshake|` +
			`temporary failure in name resolution|no such host|` +
			`Bad Gateway|Service Unavailable|Gateway Timeout|server error|` +
			`Something went wrong while executing your query`)

	// Patterns that must NOT be retried — would burn quota or mask a real bug.
	ghRetryHardFail = regexp.MustCompile(
		`(?i)HTTP 4\d\d|HTTP 403.*rate limit`)
)

// classifyGHError returns true if the failure matches a known transient pattern
// AND does not match a hard-fail pattern. Rate-limit (403) is hard-fail on
// purpose — callers that care about rate-limit handle it via RateLimitExceeded.
func classifyGHError(out string, err error) bool {
	// Rate-limit is its own typed error from Run. Never retry here; let the
	// caller decide (CheckRateLimit-driven backoff lives elsewhere).
	// Use errors.As so a wrapped RateLimitExceeded is also caught.
	var rle *RateLimitExceeded
	if errors.As(err, &rle) {
		return false
	}
	if ghRetryHardFail.MatchString(out) {
		return false
	}
	return ghRetryTransient.MatchString(out)
}

// RunWithRetry is the retry-wrapped sibling of Run. Use for `gh` CLI calls
// that talk to the GitHub API. Git calls and other local commands should use
// plain Run — retrying a local git operation rarely helps and can mask bugs.
func RunWithRetry(cmdStr string, check bool, cwd string) (string, error) {
	return runWithRetryLoop(
		func() (string, error) { return Run(cmdStr, check, cwd) },
		classifyGHError,
		ghRetryAttempts,
		ghRetryDelays,
		"gh-retry",
	)
}

// MustRunWithRetry is the retry-wrapped sibling of MustRun. Aborts the program
// on hard-fail or after retries are exhausted.
func MustRunWithRetry(cmdStr, cwd string) string {
	out, err := RunWithRetry(cmdStr, true, cwd)
	if err != nil {
		log.Fatalf("%v", err)
	}
	return out
}

// MustRunPostCreate runs cmdStr with retry on ANY non-rate-limit error using
// the standard backoff schedule. Use ONLY for `gh` operations that happen
// immediately after `gh repo create`, where any failure is overwhelmingly
// likely to be GraphQL/replica index lag rather than a real problem (the
// repo was just created successfully on the primary). After ~65s of retries
// the call still aborts — at that point something is genuinely wrong.
//
// Unlike MustRunWithRetry, this does NOT inspect output for transient
// patterns: vendor error wording can change ("Could not resolve to a
// Repository" today, something else tomorrow), and in this narrow post-create
// window a permissive classifier is safe because the only plausible cause is
// lag we already know about. See waitForRepoVisible in github.go for the
// related polling mitigation; this helper is the safety net when view-poll
// and the subsequent operation land on different replicas.
func MustRunPostCreate(cmdStr, cwd string) string {
	out, err := runWithRetryLoop(
		func() (string, error) { return Run(cmdStr, true, cwd) },
		func(_ string, err error) bool {
			var rle *RateLimitExceeded
			return !errors.As(err, &rle)
		},
		ghRetryAttempts,
		ghRetryDelays,
		"gh-retry",
	)
	if err != nil {
		log.Fatalf("%v", err)
	}
	return out
}

// MustRunStdinWithRetry is the retry-wrapped sibling of RunStdin. Aborts on
// hard-fail or after retries are exhausted. The stdin value never appears in
// logs, retry chatter, or error messages — safe for secrets.
func MustRunStdinWithRetry(cmdStr, stdin, cwd string) string {
	out, err := runWithRetryLoop(
		func() (string, error) { return RunStdin(cmdStr, stdin, true, cwd) },
		classifyGHError,
		ghRetryAttempts,
		ghRetryDelays,
		"gh-retry",
	)
	if err != nil {
		log.Fatalf("%v", err)
	}
	return out
}

// RunCaptureWithRetry is the retry-wrapped sibling of RunCapture. Use for
// `gh` CLI calls whose stdout is parsed (e.g. JSON capture from
// `gh project list --format json`). Classification still inspects the
// returned error string, which RunCapture builds from stderr.
func RunCaptureWithRetry(cmdStr, cwd string) (string, error) {
	return runWithRetryLoop(
		func() (string, error) {
			out, err := RunCapture(cmdStr, cwd)
			if err != nil {
				// classifyGHError matches against the combined message —
				// surface err.Error() (which RunCapture builds from stderr)
				// so the regex sees the same string a bash caller would.
				return err.Error(), err
			}
			return out, nil
		},
		classifyGHError,
		ghRetryAttempts,
		ghRetryDelays,
		"gh-retry",
	)
}
