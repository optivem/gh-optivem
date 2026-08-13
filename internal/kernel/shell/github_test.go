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
	_ = gh.RunWatchWorkflow("ci.yml", 1) // outer outcome irrelevant; see comment above.
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
