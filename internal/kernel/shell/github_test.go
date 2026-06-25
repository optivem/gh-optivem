package shell

import (
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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

// TestRunWatchWorkflow_AppearPollRetries504OnFirstAttempt covers Item 5: the
// inner appear-poll RunCapture must retry on a transient before giving up.
// We don't assert on the eventual RunWatchWorkflow return — gh run watch is
// invoked via direct Run (not RunWithRetry, intentionally; it streams) so
// it isn't stubbed and will fail. The test's assertion is "was the appear-
// poll retry-aware?", which is verified by the runCaptureFn call count.
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

// TestRunWatchPushWorkflow_FailsLoudWhenNoStartupFailure verifies the honest
// gate: when the push-triggered run never appears AND there is no
// startup_failure, the push trigger genuinely did not fire (e.g. a bad paths:
// filter). We must fail loud and NOT workflow_dispatch — dispatch bypasses the
// on.push filter and would mask the real bug.
func TestRunWatchPushWorkflow_FailsLoudWhenNoStartupFailure(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)

	// Every `gh run list` is empty: no run appears, no startup_failure exists.
	orig := runCaptureFn
	runCaptureFn = func(_, _ string) (string, error) { return "", nil }
	t.Cleanup(func() { runCaptureFn = orig })

	// runFn backs WorkflowRun (the dispatch). It must never be called here.
	var dispatched int32
	withFakeRunFn(t, func(int) (string, error) {
		atomic.AddInt32(&dispatched, 1)
		return "", nil
	})

	gh := &GitHub{Repo: "myorg/myrepo"}
	err := gh.RunWatchPushWorkflow("backend-commit-stage.yml", 1)
	if err == nil || !strings.Contains(err.Error(), "push trigger did not fire") {
		t.Fatalf("err = %v, want one mentioning 'push trigger did not fire'", err)
	}
	if got := atomic.LoadInt32(&dispatched); got != 0 {
		t.Fatalf("dispatch calls = %d, want 0 (must not workflow_dispatch without a startup_failure)", got)
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
