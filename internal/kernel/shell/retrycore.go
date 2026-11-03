package shell

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/optivem/gh-optivem/internal/kernel/log"
)

// defaultRetryAttempts and defaultRetryDelays are the canonical backoff
// schedule: 4 attempts, 5s → 15s → 45s between them. Mirrors
// _RETRY_CORE_ATTEMPTS / _RETRY_CORE_DELAYS in optivem/actions/shared/retry-core.sh
// so a transient pattern observed in CI logs has identical retry timing whether
// the caller is bash (gh_retry / docker_retry / sonar_retry) or Go.
var (
	defaultRetryAttempts = 4
	defaultRetryDelays   = []time.Duration{
		5 * time.Second,
		15 * time.Second,
		45 * time.Second,
	}
)

// sleepFn is package-level so tests can replace it with a no-op.
var sleepFn = time.Sleep

// SetSleepForTest replaces the inter-attempt backoff sleep with fn and returns
// a restore function. Test-only: lets callers in OTHER packages (e.g.
// clauderun, whose Dispatch routes through RetryWithPolicy) no-op the backoff
// so retry-path tests don't sleep for real seconds. Mirrors the unexported
// sleepFn/nowFn seam convention used elsewhere in the codebase; kept minimal
// and clearly test-only. Restore with the returned func in a defer.
func SetSleepForTest(fn func(time.Duration)) func() {
	prev := sleepFn
	sleepFn = fn
	return func() { sleepFn = prev }
}

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
// it rather than rolling their own. prefix appears in the inter-attempt log
// line so log readers can tell which tool's wrapper is retrying (gh-retry,
// docker-retry, sonar-retry, …).
func runWithRetryLoop(
	attempt attemptFn,
	classify classifyFn,
	maxAttempts int,
	delays []time.Duration,
	prefix string,
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
			log.Warnf("[%s] attempt %d/%d failed, retrying in %s", prefix, i, maxAttempts, delay)
			sleepFn(delay)
		}
	}
	return out, err
}

// RetryOption customises one RetryWithPolicy call. Without options the call
// uses the canonical defaultRetryAttempts / defaultRetryDelays schedule.
type RetryOption func(*retryPolicy)

type retryPolicy struct {
	attempts     int
	delays       []time.Duration
	forceRetry   *regexp.Regexp
	messageLines bool
}

// WithForceRetry adds a force-retry override: a failed attempt whose output
// matches re is retried even if it also matches the hard-fail regex (and even
// if it matches no transient pattern). For known-transient infra calls that
// phrase their failure like a hard-fail (e.g. SonarCloud's JRE-provisioning
// endpoint printing `HTTP 403 Forbidden` under load). Checked before hard-fail.
func WithForceRetry(re *regexp.Regexp) RetryOption {
	return func(p *retryPolicy) { p.forceRetry = re }
}

// WithMessageLines classifies on failure messages only: JVM stack-frame lines
// are dropped from the text the force-retry / hard-fail / transient regexes
// see. A frame such as `at ...DefaultScannerWsClient.failIfUnauthorized(...)`
// is a code identifier, not a failure message, yet matches the hard-fail
// pattern `unauthorized` and would otherwise suppress the retry of a transient
// 5xx. The caller still receives the complete unfiltered output.
func WithMessageLines() RetryOption {
	return func(p *retryPolicy) { p.messageLines = true }
}

// stackFrameLine matches a Java/JVM stack-frame line (`at pkg.Class.method(`)
// or its elision marker (`... 12 more`, `... 5 common frames omitted`).
var stackFrameLine = regexp.MustCompile(
	`^[ \t]*(at [A-Za-z_$][A-Za-z0-9_.$/]*\(|\.\.\.[ \t]+[0-9]+[ \t]+(common frames omitted|more)[ \t]*$)`)

// StripStackFrames returns out without its JVM stack-frame lines.
func StripStackFrames(out string) string {
	lines := strings.Split(out, "\n")
	kept := lines[:0]
	for _, l := range lines {
		if !stackFrameLine.MatchString(strings.TrimRight(l, "\r")) {
			kept = append(kept, l)
		}
	}
	return strings.Join(kept, "\n")
}

// WithSchedule overrides the backoff schedule for one RetryWithPolicy call:
// attempts is the total number of tries, delays[i] the sleep after try i+1
// (the last delay repeats for any tail attempts). Use it for call sites whose
// cost per attempt or outage profile differs from the canonical cheap-HTTP
// schedule (a minutes-long scanner run, a multi-minute API outage). It panics
// on attempts < 1 or empty delays: a malformed schedule is a programming error
// and must not silently fall back to the default.
func WithSchedule(attempts int, delays ...time.Duration) RetryOption {
	if attempts < 1 {
		panic(fmt.Sprintf("shell.WithSchedule: attempts must be >= 1, got %d", attempts))
	}
	if len(delays) == 0 {
		panic("shell.WithSchedule: at least one delay is required")
	}
	return func(p *retryPolicy) {
		p.attempts = attempts
		p.delays = append([]time.Duration(nil), delays...)
	}
}

// RetryWithPolicy is the generic retry engine for tool-specific wrappers. Pass
// a transient regex, an optional hard-fail regex, a log prefix, and the
// function to retry. Returns the last attempt's output and error. Optional
// RetryOptions (e.g. WithSchedule) tune the call; the default is the canonical
// 4 attempts, 5s -> 15s -> 45s.
//
// Mirrors retry_with_policy in optivem/actions/shared/retry-core.sh so adding
// a new transient pattern is a one-line edit in one place — not five. Use this
// for non-gh callers (sonar, docker, future tools). The gh path stays on
// classifyGHError because rate-limit pass-through requires typed-error
// inspection (errors.As against RateLimitExceeded), not just regex.
func RetryWithPolicy(
	transient, hardFail *regexp.Regexp,
	prefix string,
	fn func() (string, error),
	opts ...RetryOption,
) (string, error) {
	policy := retryPolicy{attempts: defaultRetryAttempts, delays: defaultRetryDelays}
	for _, opt := range opts {
		opt(&policy)
	}
	classify := func(out string, _ error) bool {
		if policy.messageLines {
			out = StripStackFrames(out)
		}
		if policy.forceRetry != nil && policy.forceRetry.MatchString(out) {
			return true
		}
		if hardFail != nil && hardFail.MatchString(out) {
			return false
		}
		return transient.MatchString(out)
	}
	return runWithRetryLoop(fn, classify, policy.attempts, policy.delays, prefix)
}
