package shell

import (
	"errors"
	"regexp"

	"github.com/optivem/gh-optivem/internal/log"
)

// Unified retry policy for any external-service shell call (gh CLI, sonarcloud
// HTTP, docker registry, git push/fetch). Mirrors the union of patterns in
// optivem/actions/shared/retry.sh so a transient phrase observed in a bash CI
// log retries with identical timing whether the caller is Go or bash.
//
// 4 attempts, 5s → 15s → 45s backoff (defaultRetryAttempts / defaultRetryDelays
// in retrycore.go). Retries 5xx + network/TLS/DNS transients + known transient
// phrases across gh/docker/sonar/git wording. Passes 4xx through immediately
// so callers using exit code as a probe (rc-as-truth-value) keep working, and
// surfaces auth/config errors as themselves rather than as a retried failure.
var (
	retryTransient = regexp.MustCompile(
		`(?i)` +
			`HTTP 5\d\d|` +
			`Error 5\d\d on https://|` +
			`received unexpected HTTP status:? 5\d\d|` +
			`RPC failed.*HTTP 5\d\d|` +
			`Internal Server Error|Bad Gateway|Service Unavailable|Gateway Timeout|server error|` +
			`Something went wrong while executing your query|` +
			`Endpoint request timed out|context deadline exceeded|Client\.Timeout|Operation timed out|` +
			`timeout|timed out|i/o timeout|net/http: TLS handshake timeout|` +
			`connection reset|connection refused|` +
			`\bEOF\b|unexpected EOF|was closed|http2: server sent GOAWAY|` +
			`TLS handshake|tls:.*handshake|server certificate verification failed|` +
			`temporary failure in name resolution|no such host|Could not resolve host|unable to access`)

	retryHardFail = regexp.MustCompile(
		`(?i)` +
			`HTTP 4\d\d|HTTP 403.*rate limit|` +
			`unauthorized|forbidden|not authorized|` +
			`permission denied|denied: permission|denied: requested access|` +
			`requested access to the resource is denied|insufficient_scope|` +
			`manifest unknown|name unknown|repository name not known|` +
			`Project key .* does not exist|Project .* not found|repository .* not found|` +
			`! \[remote rejected\]|pre-receive hook declined|fatal: protocol|fatal: bad refspec`)
)

// RetryTransient is the unified transient-pattern regex. Exported so callers
// in sibling packages (internal/sonar, internal/config) can plug it into
// RetryWithPolicy without redefining the pattern.
func RetryTransient() *regexp.Regexp { return retryTransient }

// RetryHardFail is the unified hard-fail-pattern regex. Exported alongside
// RetryTransient.
func RetryHardFail() *regexp.Regexp { return retryHardFail }

// classifyError returns true if the failure matches a known transient pattern
// AND does not match a hard-fail pattern. Rate-limit (RateLimitExceeded typed
// error from Run) is hard-fail on purpose — callers that care about rate-limit
// handle it via CheckRateLimit-driven backoff elsewhere.
func classifyError(out string, err error) bool {
	var rle *RateLimitExceeded
	if errors.As(err, &rle) {
		return false
	}
	if retryHardFail.MatchString(out) {
		return false
	}
	return retryTransient.MatchString(out)
}

// runFn and runCaptureFn are package-level seams so cross-package tests can
// exercise the retry wrappers (RunWithRetry / RunCaptureWithRetry) without
// shelling out to a real `gh` binary. Production code never reassigns them.
var (
	runFn        = Run
	runCaptureFn = RunCapture
)

// RunWithRetry is the retry-wrapped sibling of Run. Use for `gh` CLI calls
// that talk to the GitHub API. Git calls and other local commands should use
// plain Run — retrying a local git operation rarely helps and can mask bugs.
func RunWithRetry(cmdStr string, check bool, cwd string) (string, error) {
	return runWithRetryLoop(
		func() (string, error) { return runFn(cmdStr, check, cwd) },
		classifyError,
		defaultRetryAttempts,
		defaultRetryDelays,
		"retry",
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
		defaultRetryAttempts,
		defaultRetryDelays,
		"retry",
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
		classifyError,
		defaultRetryAttempts,
		defaultRetryDelays,
		"retry",
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
			out, err := runCaptureFn(cmdStr, cwd)
			if err != nil {
				// classifyError matches against the combined message — surface
				// err.Error() (which RunCapture builds from stderr) so the regex
				// sees the same string a bash caller would.
				return err.Error(), err
			}
			return out, nil
		},
		classifyError,
		defaultRetryAttempts,
		defaultRetryDelays,
		"retry",
	)
}
