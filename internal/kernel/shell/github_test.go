package shell

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/optivem/gh-optivem/internal/kernel/log"
)

func TestSplitCommand(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    []string
		wantErr bool
	}{
		{
			name: "simple words",
			in:   "git status",
			want: []string{"git", "status"},
		},
		{
			name: "double-quoted message",
			in:   `git commit -m "hello world"`,
			want: []string{"git", "commit", "-m", "hello world"},
		},
		{
			name: "single-quoted literal",
			in:   `echo 'a b c'`,
			want: []string{"echo", "a b c"},
		},
		{
			// Regression: fmt.Sprintf("git commit -m %q", msg) emits \" for
			// embedded quotes; without escape handling, splitCommand used to
			// terminate the quoted run early and git received the rest as
			// pathspecs, failing with "pathspec did not match any file(s)".
			name: "double-quoted with escaped quote",
			in:   `git commit -m "msg with \"inner\" quotes"`,
			want: []string{"git", "commit", "-m", `msg with "inner" quotes`},
		},
		{
			name: "double-quoted with escaped backslash",
			in:   `cmd "a\\b"`,
			want: []string{"cmd", `a\b`},
		},
		{
			name:    "unterminated double quote",
			in:      `cmd "oops`,
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := splitCommand(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil; parts=%q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// withFakeRunFn swaps runFn to return the (output, err) the script function
// dictates per call number. Restores on cleanup.
func withFakeRunFn(t *testing.T, script func(callNum int) (string, error)) {
	t.Helper()
	var calls int32
	orig := runFn
	runFn = func(_ string, _ bool, _ string) (string, error) {
		n := atomic.AddInt32(&calls, 1)
		out, err := script(int(n))
		if err != nil {
			// Mirror the wrapping Run does so the engine's classifier sees
			// a string shape comparable to a real failure ("...: <output>").
			return out, errors.New(out)
		}
		return out, nil
	}
	t.Cleanup(func() { runFn = orig })
}

// withFakeWatchFn swaps watchRunFn — the deadline-bounded runner `gh run watch`
// goes through — to return what script dictates per call number. The returned
// slice records the timeout each call was handed, which is how the deadline
// tests prove a bound was actually applied. Restores on cleanup.
func withFakeWatchFn(t *testing.T, script func(callNum int) (string, error)) *[]time.Duration {
	t.Helper()
	var mu sync.Mutex
	var timeouts []time.Duration
	orig := watchRunFn
	watchRunFn = func(_ string, _ bool, _ string, timeout time.Duration) (string, error) {
		mu.Lock()
		timeouts = append(timeouts, timeout)
		n := len(timeouts)
		mu.Unlock()
		out, err := script(n)
		if err == nil {
			return out, nil
		}
		// Deadline errors must stay matchable via errors.Is — that's the signal
		// watchRunID routes on. Anything else is rewrapped the way Run wraps a
		// real failure, so the retry classifier sees a realistic string shape.
		if errors.Is(err, ErrCommandDeadlineExceeded) {
			return out, err
		}
		return out, errors.New(out)
	}
	t.Cleanup(func() { watchRunFn = orig })
	return &timeouts
}

// fakeClock drives nowFn and sleepFn from a virtual clock, so bounded waits
// (the 30m watch deadline, the 60m poll deadline) are reachable in
// microseconds. Every sleepFn call advances the clock by its own duration —
// the same relationship the real pair has, minus the wall-clock cost.
type fakeClock struct {
	mu    sync.Mutex
	now   time.Time
	slept []time.Duration
}

func withFakeClock(t *testing.T) *fakeClock {
	t.Helper()
	c := &fakeClock{now: time.Unix(0, 0).UTC()}
	origNow, origSleep := nowFn, sleepFn
	nowFn = func() time.Time {
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.now
	}
	sleepFn = func(d time.Duration) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.now = c.now.Add(d)
		c.slept = append(c.slept, d)
	}
	t.Cleanup(func() { nowFn, sleepFn = origNow, origSleep })
	return c
}

func (c *fakeClock) sleeps() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.slept)
}

// advance moves the virtual clock without recording a sleep — for time spent
// inside a stubbed subprocess, or for placing a test's start offset.
func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// withHardDeadline pins the process-wide hard deadline for one test, bypassing
// the environment read so the test doesn't depend on GH_OPTIVEM_HARD_DEADLINE_MINUTES.
func withHardDeadline(t *testing.T, at time.Time) {
	t.Helper()
	orig := hardDeadlineFn
	hardDeadlineFn = func() (time.Time, bool) { return at, true }
	t.Cleanup(func() { hardDeadlineFn = orig })
}

// withFakeRateLimitOK stubs runCaptureFn so the CheckRateLimit call inside each
// poll iteration reports ample budget instead of shelling out to a real
// `gh api rate_limit`.
func withFakeRateLimitOK(t *testing.T) {
	t.Helper()
	orig := runCaptureFn
	runCaptureFn = func(string, string) (string, error) {
		return `{"remaining":5000,"reset":0}`, nil
	}
	t.Cleanup(func() { runCaptureFn = orig })
}

// TestRepoExists_Retries504sThen404Returns covers Item 4 from the retry-gaps
// plan: with RepoExists wrapping Run via RunWithRetry, a transient 504 must
// retry and an eventual 404 must surface as the "not found" outcome
// (false, nil) — not a fatal error.
func TestRepoExists_Retries504sThen404Returns(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)

	script := func(n int) (string, error) {
		switch n {
		case 1, 2:
			return "HTTP 504: Gateway Timeout", errors.New("exit 1")
		default:
			return "HTTP 404: Not Found\nGraphQL: Could not resolve to a Repository", errors.New("exit 1")
		}
	}
	withFakeRunFn(t, script)

	exists, err := RepoExists("myorg/myrepo")
	if err != nil {
		t.Fatalf("RepoExists: unexpected error after 504→504→404: %v", err)
	}
	if exists {
		t.Fatal("RepoExists returned true on 404")
	}
	if len(sleeps) != 2 {
		t.Fatalf("sleeps = %d, want 2 backoffs (3 attempts)", len(sleeps))
	}
}

// TestRepoExists_HardFail4xxNotARepoNotFoundStillErrors confirms the
// classifier still passes through 4xx as hard-fail without retrying.
// Forbidden (403) is not "not found", so the function returns an error.
func TestRepoExists_HardFail4xxNotARepoNotFoundStillErrors(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)

	withFakeRunFn(t, func(int) (string, error) {
		return "HTTP 403: Forbidden", errors.New("exit 1")
	})

	_, err := RepoExists("myorg/myrepo")
	if err == nil {
		t.Fatal("expected error on 403, got nil")
	}
	if len(sleeps) != 0 {
		t.Fatalf("sleeps = %d, want 0 (hard-fail must not retry)", len(sleeps))
	}
}

// TestCreateRepo_ExistingRepoFailsLoud is the regression test for issue #60:
// a repeat `init` run against an already-scaffolded repo must abort at the
// existence check instead of logging a warning and falling through to
// re-scaffold (which corrupted output by colliding with the stale tree from
// the first run).
func TestCreateRepo_ExistingRepoFailsLoud(t *testing.T) {
	withFakeRunFn(t, func(int) (string, error) {
		return `{"name":"myrepo"}`, nil // gh repo view succeeds -> repo exists
	})

	gh := &GitHub{Repo: "myorg/myrepo"}
	var caught *log.StepError
	func() {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			var ok bool
			caught, ok = r.(*log.StepError)
			if !ok {
				t.Fatalf("panic value is %T, want *log.StepError", r)
			}
		}()
		gh.CreateRepo()
	}()
	if caught == nil {
		t.Fatal("CreateRepo: want a fatal abort when the repo already exists, got none")
	}
	for _, want := range []string{"myorg/myrepo", "already exists", "not supported"} {
		if !strings.Contains(caught.Error(), want) {
			t.Fatalf("error %q does not mention %q", caught.Error(), want)
		}
	}
}

// TestWatchRunID_RetriesTransient401ThenSucceeds pins the per-token-throttle
// mitigation: a transient HTTP 401 "Bad credentials" from `gh run watch` is
// retried on the canonical backoff schedule, and a subsequent success returns
// nil. Without this, a single throttle miss failed the whole stage even though
// the token was valid (run 28361866952, prod-stage watch).
func TestWatchRunID_RetriesTransient401ThenSucceeds(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)

	withFakeWatchFn(t, func(n int) (string, error) {
		if n == 1 {
			return "failed to get run: HTTP 401: Bad credentials", errors.New("x")
		}
		return "", nil // watch succeeds on the retry
	})

	gh := &GitHub{Repo: "myorg/myrepo"}
	if err := gh.watchRunID("12345", 1); err != nil {
		t.Fatalf("watchRunID: want nil after 401→success, got %v", err)
	}
	if len(sleeps) != 1 {
		t.Fatalf("sleeps = %d, want 1 backoff between the 401 and the retry", len(sleeps))
	}
}

// TestWatchRunID_NonTransientFailsFast confirms the watch retry is narrow: a
// genuine non-401, non-rate-limit failure surfaces immediately with no retry,
// so real breakage isn't papered over by the throttle mitigation.
func TestWatchRunID_NonTransientFailsFast(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)

	var calls int32
	withFakeWatchFn(t, func(int) (string, error) {
		atomic.AddInt32(&calls, 1)
		return "command failed: some genuine error", errors.New("x")
	})

	gh := &GitHub{Repo: "myorg/myrepo"}
	if err := gh.watchRunID("12345", 1); err == nil {
		t.Fatal("watchRunID: want error on non-transient failure, got nil")
	}
	if len(sleeps) != 0 {
		t.Fatalf("sleeps = %d, want 0 (non-transient must not retry)", len(sleeps))
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("watchRunFn calls = %d, want 1 (no retry on non-transient)", got)
	}
}

// TestWatchRunID_HappyPathIsBoundedButDoesNotWait pins that adding the deadline
// changed nothing on the happy path: the watch is handed a bound, returns
// immediately, and never touches the polling fallback or any backoff.
func TestWatchRunID_HappyPathIsBoundedButDoesNotWait(t *testing.T) {
	clock := withFakeClock(t)

	timeouts := withFakeWatchFn(t, func(int) (string, error) { return "", nil })

	var pollCalls int32
	withFakeRunFn(t, func(int) (string, error) {
		atomic.AddInt32(&pollCalls, 1)
		return "completed,success", nil
	})

	gh := &GitHub{Repo: "myorg/myrepo"}
	if err := gh.watchRunID("12345", 1); err != nil {
		t.Fatalf("watchRunID: want nil on the happy path, got %v", err)
	}
	if len(*timeouts) != 1 {
		t.Fatalf("watchRunFn calls = %d, want 1", len(*timeouts))
	}
	if got := (*timeouts)[0]; got != watchMaxDuration {
		t.Fatalf("watch timeout = %s, want the full %s budget on the first attempt", got, watchMaxDuration)
	}
	if got := atomic.LoadInt32(&pollCalls); got != 0 {
		t.Fatalf("poll calls = %d, want 0 (a successful watch must not fall back to polling)", got)
	}
	if clock.sleeps() != 0 {
		t.Fatalf("sleeps = %d, want 0 (the happy path must not wait out any deadline)", clock.sleeps())
	}
}

// TestWatchRunID_DeadlineExpiryFallsBackToPolling covers the core of the fix:
// when `gh run watch` outruns watchMaxDuration — GitHub holding the run object
// non-terminal long after its jobs finished — the watch is abandoned and the
// already-bounded polling fallback takes over and can still report success.
// The deadline must not be retried: re-spending an exhausted budget is exactly
// the two-hour silence this change removes.
func TestWatchRunID_DeadlineExpiryFallsBackToPolling(t *testing.T) {
	withFakeClock(t)
	withFakeRateLimitOK(t)

	timeouts := withFakeWatchFn(t, func(int) (string, error) {
		return "", fmt.Errorf("%w: gh run watch stalled", ErrCommandDeadlineExceeded)
	})

	var pollCalls int32
	withFakeRunFn(t, func(int) (string, error) {
		atomic.AddInt32(&pollCalls, 1)
		return "completed,success", nil
	})

	gh := &GitHub{Repo: "myorg/myrepo"}
	if err := gh.watchRunID("12345", 1); err != nil {
		t.Fatalf("watchRunID: want nil (polling fallback saw success), got %v", err)
	}
	if len(*timeouts) != 1 {
		t.Fatalf("watchRunFn calls = %d, want 1 (deadline expiry must not be retried)", len(*timeouts))
	}
	if got := atomic.LoadInt32(&pollCalls); got == 0 {
		t.Fatal("poll calls = 0, want >= 1 (deadline expiry must fall through to pollRunUntilComplete)")
	}
}

// TestWatchRunID_BothDeadlinesExpireFailsLoud pins the fail-loud contract: a
// run that outlives the watch deadline AND the polling deadline is an error,
// never a silent pass, and the message must be actionable on its own — the run
// URL so the operator doesn't have to reconstruct it, and the elapsed time so
// the scale of the stall is visible without reading timestamps.
func TestWatchRunID_BothDeadlinesExpireFailsLoud(t *testing.T) {
	withFakeClock(t)
	withFakeRateLimitOK(t)

	withFakeWatchFn(t, func(int) (string, error) {
		return "", fmt.Errorf("%w: gh run watch stalled", ErrCommandDeadlineExceeded)
	})

	// The run never leaves in_progress, so the fallback polls until its own
	// bound expires — the virtual clock advances 60s per iteration.
	withFakeRunFn(t, func(int) (string, error) { return "in_progress,", nil })

	gh := &GitHub{Repo: "myorg/myrepo"}
	err := gh.watchRunID("12345", 1)
	if err == nil {
		t.Fatal("watchRunID: want an error when both the watch and the poll deadline expire, got nil")
	}
	for _, want := range []string{
		"https://github.com/myorg/myrepo/actions/runs/12345",
		"polling run 12345 timed out",
		fmt.Sprintf("watch deadline of %s expired", watchMaxDuration),
		"elapsed across watch + polling",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q — the message is the whole point of this path", err, want)
		}
	}
}

// TestRunWatchWorkflow_AppearPollRetries504OnFirstAttempt covers Item 5: the
// inner appear-poll RunCapture must retry on a transient before giving up.
// We don't assert on the eventual RunWatchWorkflow return — gh run watch runs
// via runFn (the Run seam), left unstubbed here so it uses the real Run and
// fails. The test's assertion is "was the appear-poll retry-aware?", which is
// verified by the runCaptureFn call count.
func TestRunWatchWorkflow_AppearPollRetries504OnFirstAttempt(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)

	var captureCalls int32
	orig := runCaptureFn
	runCaptureFn = func(_, _ string) (string, error) {
		n := atomic.AddInt32(&captureCalls, 1)
		if n == 1 {
			return "", errors.New("HTTP 504: Gateway Timeout")
		}
		return "12345", nil
	}
	t.Cleanup(func() { runCaptureFn = orig })

	gh := &GitHub{Repo: "myorg/myrepo"}
	_ = gh.RunWatchWorkflow("ci.yml", nil, 1) // outer outcome irrelevant; see comment above.
	if got := atomic.LoadInt32(&captureCalls); got < 2 {
		t.Fatalf("runCaptureFn calls = %d, want >= 2 (proves RunCaptureWithRetry retried after 504)", got)
	}
	if len(sleeps) < 1 {
		t.Fatalf("sleeps = %d, want at least 1 (retry between 504 and success)", len(sleeps))
	}
}

// TestRunWatchPushWorkflow_RecoversMissingRunWithoutStartupFailure verifies the
// recovery path for the no-startup_failure variant of the GitHub first-push
// flake: when the push-triggered run never appears and there is no
// startup_failure, RunWatchPushWorkflow re-dispatches via workflow_dispatch
// (bounded to maxReDispatches) rather than failing loud. The on.push.paths
// filter is validated statically before push (VerifyPushPathsFilter), so
// unconditional re-dispatch is safe.
func TestRunWatchPushWorkflow_RecoversMissingRunWithoutStartupFailure(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)

	// Every `gh run list` is empty: no run appears, no startup_failure exists.
	orig := runCaptureFn
	runCaptureFn = func(_, _ string) (string, error) { return "", nil }
	t.Cleanup(func() { runCaptureFn = orig })

	var dispatched int32
	withFakeRunFn(t, func(int) (string, error) {
		atomic.AddInt32(&dispatched, 1)
		return "", nil
	})

	gh := &GitHub{Repo: "myorg/myrepo"}
	err := gh.RunWatchPushWorkflow("backend-commit-stage.yml", 1)
	if err == nil || !strings.Contains(err.Error(), "re-dispatch attempts") {
		t.Fatalf("err = %v, want one mentioning 're-dispatch attempts'", err)
	}
	if got := atomic.LoadInt32(&dispatched); got != int32(maxReDispatches) {
		t.Fatalf("dispatch calls = %d, want %d (one per re-dispatch)", got, maxReDispatches)
	}
}

// TestRunWatchPushWorkflow_ReDispatchesOnStartupFailure verifies the recovery
// path: when the push-triggered run never appears but a startup_failure is
// present (the fresh-repo first-push flake), we re-fire via workflow_dispatch,
// bounded to maxReDispatches, then fail loud.
func TestRunWatchPushWorkflow_ReDispatchesOnStartupFailure(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)

	// The expected run never appears; the startup_failure query always finds
	// the phantom run, so the gate stays open across attempts.
	orig := runCaptureFn
	runCaptureFn = func(cmd, _ string) (string, error) {
		if strings.Contains(cmd, "startup_failure") {
			return "999", nil
		}
		return "", nil
	}
	t.Cleanup(func() { runCaptureFn = orig })

	var dispatched int32
	withFakeRunFn(t, func(int) (string, error) {
		atomic.AddInt32(&dispatched, 1)
		return "", nil
	})

	gh := &GitHub{Repo: "myorg/myrepo"}
	err := gh.RunWatchPushWorkflow("backend-commit-stage.yml", 1)
	if err == nil || !strings.Contains(err.Error(), "re-dispatch attempts") {
		t.Fatalf("err = %v, want one mentioning 're-dispatch attempts'", err)
	}
	if got := atomic.LoadInt32(&dispatched); got != int32(maxReDispatches) {
		t.Fatalf("dispatch calls = %d, want %d (one per re-dispatch)", got, maxReDispatches)
	}
}

// TestPollRunUntilComplete_GhRunViewRetries504 covers Item 6: the per-iter
// gh run view call must retry on a transient and then surface the parsed
// status. We make the first call 504 and the second return "completed,success".
func TestPollRunUntilComplete_GhRunViewRetries504(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)

	withFakeRunFn(t, func(n int) (string, error) {
		if n == 1 {
			return "HTTP 504 Gateway Timeout", errors.New("exit 1")
		}
		return "completed,success", nil
	})

	gh := &GitHub{Repo: "myorg/myrepo"}
	if err := gh.pollRunUntilComplete("12345"); err != nil {
		t.Fatalf("pollRunUntilComplete: %v", err)
	}
	if len(sleeps) < 1 {
		t.Fatalf("sleeps = %d, want at least 1 (gh run view retried once)", len(sleeps))
	}
}

// TestWaitForRepoVisible_RetriesTransient covers Item 7: a 504 mid-poll must
// not be treated as fatal — the inner Run is now retry-aware so the
// surrounding 15-attempt visibility loop still gets a chance to succeed.
func TestWaitForRepoVisible_RetriesTransient(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)

	withFakeRunFn(t, func(n int) (string, error) {
		// First attempt: transient 504. Subsequent attempts: success.
		if n == 1 {
			return "HTTP 504 Gateway Timeout", errors.New("exit 1")
		}
		return `{"name":"myrepo"}`, nil
	})

	// log.Fatalf would call os.Exit(1) if waitForRepoVisible decides the
	// retry exhausted. The test passing without panic-or-exit means it
	// got through. We don't assert sleep count here because the visibility
	// loop also has its own pollDelay sleep that goes through sleepFn.
	gh := &GitHub{Repo: "myorg/myrepo"}
	gh.waitForRepoVisible()
	if len(sleeps) < 1 {
		t.Fatalf("sleeps = %d, want at least 1 (504 retried)", len(sleeps))
	}
	// Avoid unused-var warning for strings — kept for future test extension.
	_ = strings.TrimSpace
}

// createRepoScript is the shared call script for the CreateRepo lost-response
// tests. Call 1 is the pre-flight `gh repo view` (404 — name is free); call 2
// is create attempt 1, lost to a transient AFTER GitHub created the repo;
// call 3 is the retry, which necessarily reports the name as taken. recheck is
// what the post-name-taken `gh repo view` returns; anything after that is
// waitForRepoVisible's poll, which is handed viewOK.
func createRepoScript(recheck func() (string, error)) func(int) (string, error) {
	const viewOK = `{"name":"myrepo-system"}`
	return func(n int) (string, error) {
		switch n {
		case 1:
			return "GraphQL: Could not resolve to a Repository with the name 'myorg/myrepo-system'. (repository)",
				errors.New("exit 1")
		case 2:
			return "Post \"https://api.github.com/graphql\": net/http: TLS handshake timeout",
				errors.New("exit 1")
		case 3:
			return "GraphQL: Name already exists on this account (createRepository)", errors.New("exit 1")
		case 4:
			return recheck()
		default:
			return viewOK, nil
		}
	}
}

// catchFatal runs fn and returns the *log.StepError it aborted with, or nil if
// it returned normally.
func catchFatal(t *testing.T, fn func()) *log.StepError {
	t.Helper()
	var caught *log.StepError
	func() {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			var ok bool
			caught, ok = r.(*log.StepError)
			if !ok {
				t.Fatalf("panic value is %T, want *log.StepError", r)
			}
		}()
		fn()
	}()
	return caught
}

// TestCreateRepo_AdoptsRepoCreatedOnLostAttempt is the regression test for
// acceptance run 32874584907 (job 97898649633). `gh repo create` is a
// non-idempotent write: when a transient swallows the response of an attempt
// that already succeeded on GitHub's side, the retry comes back "Name already
// exists on this account" — wording the shared classifier hard-fails on. That
// aborted the whole scaffold over a repo that existed and was ours.
//
// The exact call script below reproduced the CI failure byte-for-byte against
// the pre-fix code. CreateRepo must now re-check reality and adopt the repo.
func TestCreateRepo_AdoptsRepoCreatedOnLostAttempt(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)
	withFakeRunFn(t, createRepoScript(func() (string, error) {
		return `{"name":"myrepo-system"}`, nil // re-check: the repo our lost attempt created
	}))

	gh := &GitHub{Repo: "myorg/myrepo-system"}
	if caught := catchFatal(t, gh.CreateRepo); caught != nil {
		t.Fatalf("CreateRepo aborted on an adoptable lost-response create: %v", caught.Error())
	}
	if len(sleeps) == 0 {
		t.Fatal("expected at least one retry backoff before the name-taken response")
	}
}

// TestCreateRepo_IndeterminateRecheckFailsLoud pins the other half of the rule:
// a name-taken response is only adoptable when the re-check gives a definitive
// "yes, it exists". An indeterminate re-check (403 — couldn't tell) must abort
// naming the repo, never be coerced into success. Same rule the check-* probes
// follow: returning success on an indeterminate result is a lie.
func TestCreateRepo_IndeterminateRecheckFailsLoud(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)
	withFakeRunFn(t, createRepoScript(func() (string, error) {
		return "HTTP 403: Forbidden", errors.New("exit 1") // re-check: couldn't tell
	}))

	caught := catchFatal(t, (&GitHub{Repo: "myorg/myrepo-system"}).CreateRepo)
	if caught == nil {
		t.Fatal("want a fatal abort when the existence re-check is indeterminate, got none")
	}
	for _, want := range []string{"myorg/myrepo-system", "already taken"} {
		if !strings.Contains(caught.Error(), want) {
			t.Fatalf("error %q does not mention %q", caught.Error(), want)
		}
	}
}

// TestCreateRepo_NameTakenButNotVisibleFailsLoud covers the contradiction case:
// gh says the name is taken, but the re-check definitively 404s. The two
// answers disagree, so there is no definitive verdict — abort rather than
// adopt a repo we cannot see.
func TestCreateRepo_NameTakenButNotVisibleFailsLoud(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)
	withFakeRunFn(t, createRepoScript(func() (string, error) {
		return "GraphQL: Could not resolve to a Repository with the name 'myorg/myrepo-system'. (repository)",
			errors.New("exit 1")
	}))

	caught := catchFatal(t, (&GitHub{Repo: "myorg/myrepo-system"}).CreateRepo)
	if caught == nil {
		t.Fatal("want a fatal abort when gh says name-taken but the repo is not visible, got none")
	}
	for _, want := range []string{"myorg/myrepo-system", "not visible"} {
		if !strings.Contains(caught.Error(), want) {
			t.Fatalf("error %q does not mention %q", caught.Error(), want)
		}
	}
}

// TestCreateRepo_NonNameTakenFailureStillFatals guards the narrowness of the
// adoption path: only the name-taken wording routes through the re-check.
// Any other create failure aborts on the spot, as before.
func TestCreateRepo_NonNameTakenFailureStillFatals(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)
	withFakeRunFn(t, func(n int) (string, error) {
		if n == 1 {
			return "GraphQL: Could not resolve to a Repository (repository)", errors.New("exit 1")
		}
		return "HTTP 403: Forbidden -- token lacks the repo scope", errors.New("exit 1")
	})

	caught := catchFatal(t, (&GitHub{Repo: "myorg/myrepo-system"}).CreateRepo)
	if caught == nil {
		t.Fatal("want a fatal abort on a non-name-taken create failure, got none")
	}
	if strings.Contains(caught.Error(), "already taken") {
		t.Fatalf("a 403 must not be routed through the adoption path: %v", caught.Error())
	}
}

// TestWatchRunID_HardDeadlineKeepsBothPhasesInsideTheHarnessBudget reproduces
// gh-acceptance-stage run 32976603425 (job 98212141832), where the per-phase
// bounds were individually correct and the process still died the wrong way.
// The watch began 42m25s into a 2h `go test`, expired correctly at 30m, and the
// poll phase then started a fresh 60m budget — putting its loud, actionable
// error 12 minutes past the harness's kill time. The operator got
// "panic: test timed out after 2h0m0s" and a goroutine dump instead of the run
// URL. Both phases now clamp to the process-wide hard deadline, so the error
// lands first from any start offset.
func TestWatchRunID_HardDeadlineKeepsBothPhasesInsideTheHarnessBudget(t *testing.T) {
	c := withFakeClock(t)
	withFakeRateLimitOK(t)

	const (
		processBudget = 105 * time.Minute               // what CI exports
		startOffset   = 42*time.Minute + 25*time.Second // where the real stall began
		pollSlop      = 60 * time.Second                // the loop sleeps, then re-checks
	)
	origin := nowFn()
	hard := origin.Add(processBudget)
	withHardDeadline(t, hard)
	c.advance(startOffset)

	// A stalled `gh run watch` burns its whole allotted budget before reporting
	// the deadline — the stub has to spend that time too, or the clamp is never
	// under any pressure and the test proves nothing.
	origWatch := watchRunFn
	watchRunFn = func(_ string, _ bool, _ string, timeout time.Duration) (string, error) {
		c.advance(timeout)
		return "", fmt.Errorf("%w: gh run watch stalled", ErrCommandDeadlineExceeded)
	}
	t.Cleanup(func() { watchRunFn = origWatch })

	// The run never leaves in_progress, so the fallback polls until a bound stops it.
	withFakeRunFn(t, func(int) (string, error) { return "in_progress,", nil })

	gh := &GitHub{Repo: "myorg/myrepo"}
	err := gh.watchRunID("12345", 1)
	if err == nil {
		t.Fatal("watchRunID: want an error when the run never reports a terminal state, got nil")
	}

	gaveUp := nowFn()
	if latest := hard.Add(pollSlop); gaveUp.After(latest) {
		t.Fatalf("gave up %s in, past the %s hard deadline — the harness would have killed the process first, replacing this error with a goroutine dump",
			gaveUp.Sub(origin), processBudget)
	}
	// Without the clamp the poll phase runs its full pollMaxDuration from
	// wherever the watch left off. That is the overrun this test exists for.
	unclamped := origin.Add(startOffset).Add(watchMaxDuration).Add(pollMaxDuration)
	if !gaveUp.Before(unclamped) {
		t.Fatalf("gave up %s in, no earlier than the unclamped %s — the hard deadline did not bite",
			gaveUp.Sub(origin), unclamped.Sub(origin))
	}
	if !strings.Contains(err.Error(), "https://github.com/myorg/myrepo/actions/runs/12345") {
		t.Fatalf("error %q must carry the run URL — it is the one thing the operator needs", err)
	}
}

// TestPollRunUntilComplete_HeartbeatsSoAStallIsVisible pins the poll phase's
// progress line. GitHub Actions logs are non-TTY, so the spinner renders as
// nothing: on 2026-08-26 the poll fallback emitted zero output between 15:33:00
// and 16:20:48 — 47m47s of dead log while the process was very much alive. The
// watch phase had a heartbeat for exactly this reason; the poll phase did not.
func TestPollRunUntilComplete_HeartbeatsSoAStallIsVisible(t *testing.T) {
	withFakeClock(t)
	withFakeRateLimitOK(t)
	withFakeRunFn(t, func(int) (string, error) { return "in_progress,", nil })

	type beat struct {
		gerund, boundName, runURL string
		elapsed                   time.Duration
	}
	var beats []beat
	orig := stillWaitingFn
	stillWaitingFn = func(gerund, boundName, runURL string, started time.Time, _ time.Duration) time.Duration {
		el := elapsedSince(started)
		beats = append(beats, beat{gerund, boundName, runURL, el})
		return el
	}
	t.Cleanup(func() { stillWaitingFn = orig })

	gh := &GitHub{Repo: "myorg/myrepo"}
	if err := gh.pollRunUntilComplete("12345"); err == nil {
		t.Fatal("pollRunUntilComplete: want an error when the run never completes, got nil")
	}

	wantBeats := int(pollMaxDuration / watchHeartbeatInterval)
	if len(beats) != wantBeats {
		t.Fatalf("heartbeats = %d, want %d (one every %s across a %s poll) — a silent poll is what left 47m47s of dead CI log",
			len(beats), wantBeats, watchHeartbeatInterval, pollMaxDuration)
	}
	if got := beats[0].elapsed; got != watchHeartbeatInterval {
		t.Fatalf("first heartbeat at %s, want %s", got, watchHeartbeatInterval)
	}
	if beats[0].gerund != "polling" || beats[0].boundName != "poll deadline" {
		t.Fatalf("heartbeat = %+v, want it to name the polling phase and its own bound", beats[0])
	}
	if wantURL := "https://github.com/myorg/myrepo/actions/runs/12345"; beats[0].runURL != wantURL {
		t.Fatalf("heartbeat URL = %q, want %q — a progress line without the link is not actionable", beats[0].runURL, wantURL)
	}
}

// TestRunWatchWorkflow_ReDispatchesOnStartupFailureThenFailsLoud covers the
// dispatch path's half of the startup_failure recovery, which until now existed
// only for push-triggered runs. When GitHub stamps a dispatched run
// startup_failure the run object exists but the workflow never started, and
// `gh run watch --exit-status` exits 1 — surfacing as "<stage> workflow failed"
// and sending the operator to hunt a bug in a scaffolded app that never ran a
// line. Observed 2026-08-26 on runs 32983779814 and 32983833982. Recovery is
// bounded and still fails loud: never a silent pass.
func TestRunWatchWorkflow_ReDispatchesOnStartupFailureThenFailsLoud(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)

	// The run appears every time, and GitHub calls it startup_failure every time.
	orig := runCaptureFn
	runCaptureFn = func(cmd, _ string) (string, error) {
		if strings.Contains(cmd, "--json conclusion") {
			return "startup_failure", nil
		}
		return "12345", nil
	}
	t.Cleanup(func() { runCaptureFn = orig })

	withFakeWatchFn(t, func(int) (string, error) {
		return "", errors.New("command failed: gh run watch 12345 --exit-status: exit status 1")
	})

	var dispatched int32
	withFakeRunFn(t, func(int) (string, error) {
		atomic.AddInt32(&dispatched, 1)
		return "", nil
	})

	gh := &GitHub{Repo: "myorg/myrepo"}
	err := gh.RunWatchWorkflow("prod-stage.yml", nil, 1)
	if err == nil {
		t.Fatal("RunWatchWorkflow: want an error once the re-dispatch budget is spent — recovery must never coerce startup_failure into a pass")
	}
	if !strings.Contains(err.Error(), "startup_failure") {
		t.Fatalf("error %q must name startup_failure — otherwise the operator hunts a bug in a workflow that never started", err)
	}
	if got := atomic.LoadInt32(&dispatched); got != int32(maxReDispatches) {
		t.Fatalf("dispatch calls = %d, want %d (one per re-dispatch)", got, maxReDispatches)
	}
}

// TestRunWatchWorkflow_NonStartupFailureIsNotReDispatched is the other half of
// the contract: a workflow that genuinely failed must surface immediately. Only
// a definitive startup_failure buys a retry — an indeterminate or ordinary
// failure returns the real error, unretried.
func TestRunWatchWorkflow_NonStartupFailureIsNotReDispatched(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)

	orig := runCaptureFn
	runCaptureFn = func(cmd, _ string) (string, error) {
		if strings.Contains(cmd, "--json conclusion") {
			return "failure", nil
		}
		return "12345", nil
	}
	t.Cleanup(func() { runCaptureFn = orig })

	withFakeWatchFn(t, func(int) (string, error) {
		return "", errors.New("command failed: gh run watch 12345 --exit-status: exit status 1")
	})

	var dispatched int32
	withFakeRunFn(t, func(int) (string, error) {
		atomic.AddInt32(&dispatched, 1)
		return "", nil
	})

	gh := &GitHub{Repo: "myorg/myrepo"}
	if err := gh.RunWatchWorkflow("prod-stage.yml", nil, 1); err == nil {
		t.Fatal("RunWatchWorkflow: want the underlying watch error for a genuine failure, got nil")
	}
	if got := atomic.LoadInt32(&dispatched); got != 0 {
		t.Fatalf("dispatch calls = %d, want 0 — a real workflow failure must not be re-fired", got)
	}
}
