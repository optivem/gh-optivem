package shell

import "time"

// SetSleepFnForTest swaps the package-level sleepFn so tests in sibling
// packages (e.g. internal/sonar) can stub out the 5s/15s/45s backoff and
// finish in milliseconds instead of seconds. Returns a restore func to be
// deferred. NOT for production use — the only reason it's exported is
// cross-package retry-loop tests can't reach the unexported sleepFn directly.
func SetSleepFnForTest(fn func(time.Duration)) (restore func()) {
	orig := sleepFn
	sleepFn = fn
	return func() { sleepFn = orig }
}

// SetRunFnForTest swaps the package-level runFn (the function RunWithRetry
// shells out to) so tests can exercise call sites like RepoExists,
// waitForRepoVisible, and pollRunUntilComplete without invoking a real `gh`
// binary. Returns a restore func to be deferred.
func SetRunFnForTest(fn func(string, bool, string) (string, error)) (restore func()) {
	orig := runFn
	runFn = fn
	return func() { runFn = orig }
}

// SetRunCaptureFnForTest swaps the package-level runCaptureFn (the function
// RunCaptureWithRetry shells out to) so tests can exercise call sites like
// RunWatchWorkflow's appear-poll loop without a real `gh` binary.
func SetRunCaptureFnForTest(fn func(string, string) (string, error)) (restore func()) {
	orig := runCaptureFn
	runCaptureFn = fn
	return func() { runCaptureFn = orig }
}
