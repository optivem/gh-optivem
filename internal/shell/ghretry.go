package shell

import (
	"errors"
	"regexp"
	"time"

	"github.com/optivem/gh-optivem/internal/log"
)

// Retry policy for `gh` CLI invocations. Mirrors the bash gh_retry wrapper
// in optivem/actions/shared/gh-retry.sh: 4 attempts, 5s → 15s → 45s backoff,
// retries 5xx/network/TLS/DNS transients, passes through 4xx (incl. rate limit)
// immediately so CheckRateLimit / callers can make their own decision.
var (
	ghRetryAttempts = 4
	ghRetryDelays   = []time.Duration{
		5 * time.Second,
		15 * time.Second,
		45 * time.Second,
	}

	// Patterns that indicate a transient failure worth retrying.
	ghRetryTransient = regexp.MustCompile(
		`(?i)` +
			`HTTP 5\d\d|` +
			`timeout|timed out|i/o timeout|` +
			`connection reset|connection refused|` +
			`\bEOF\b|was closed|broken pipe|` +
			`TLS handshake|tls:.*handshake|` +
			`temporary failure in name resolution|no such host|` +
			`Bad Gateway|Service Unavailable|Gateway Timeout|server error`)

	// Patterns that must NOT be retried — would burn quota or mask a real bug.
	ghRetryHardFail = regexp.MustCompile(
		`(?i)HTTP 4\d\d|HTTP 403.*rate limit`)
)

// sleepFn is package-level so tests can replace it with a no-op.
var sleepFn = time.Sleep

// attemptFn runs one attempt and returns (combined output, error).
type attemptFn func() (string, error)

// classifyFn inspects a failed attempt's output+error and decides whether
// another attempt is worthwhile. Returning false means hard-fail — return the
// error to the caller immediately.
type classifyFn func(out string, err error) bool

// runWithRetryLoop runs attempt() up to maxAttempts times. Between attempts,
// sleeps for delays[i] (capped at len(delays)-1 for tail attempts). Stops early
// on success or when classify returns false (hard-fail pass-through). Returns
// the final attempt's output and error.
//
// This is the one retry loop in the package; higher-level wrappers parameterise
// it rather than rolling their own.
func runWithRetryLoop(
	attempt attemptFn,
	classify classifyFn,
	maxAttempts int,
	delays []time.Duration,
) (string, error) {
	var out string
	var err error
	for i := 1; i <= maxAttempts; i++ {
		out, err = attempt()
		if err == nil {
			return out, nil
		}
		if !classify(out, err) {
			return out, err
		}
		if i < maxAttempts {
			delay := delays[len(delays)-1]
			if idx := i - 1; idx < len(delays) {
				delay = delays[idx]
			}
			log.Warnf("[gh-retry] attempt %d/%d failed, retrying in %s", i, maxAttempts, delay)
			sleepFn(delay)
		}
	}
	return out, err
}

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
	)
	if err != nil {
		log.Fatalf("%v", err)
	}
	return out
}
