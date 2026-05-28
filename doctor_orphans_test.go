package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/optivem/gh-optivem/internal/userstate"
)

// isolateUserStateDir points userstate.Dir at a fresh t.TempDir for the
// duration of the test. It sets the platform-appropriate env var
// (LOCALAPPDATA on Windows, XDG_STATE_HOME elsewhere) so userstate.Dir
// resolves under tempdir + "/gh-optivem". Returns the runs/ subdir the
// driver would write into so callers can drop marker files there
// without recomputing the path.
func isolateUserStateDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("LOCALAPPDATA", root)
	} else {
		t.Setenv("XDG_STATE_HOME", root)
	}
	runs := filepath.Join(root, "gh-optivem", "runs")
	if err := os.MkdirAll(runs, 0o755); err != nil {
		t.Fatalf("mkdir runs: %v", err)
	}
	return runs
}

// spawnLongRunner starts a child that will stay alive for ~60s (enough
// for the doctor sweep to observe it) and registers a cleanup that kills
// + waits for the process so the test does not leak it. Returns the PID
// and a reap function the caller can invoke to Wait() on the child — the
// test must call reap after the doctor's SIGKILL on Unix, otherwise the
// child becomes a zombie that processAlive (via signal-0) still reports
// as alive. Calling reap is a no-op on the second invocation, so the
// cleanup hook's own Wait remains safe.
func spawnLongRunner(t *testing.T) (int, func()) {
	t.Helper()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell", "-NoProfile", "-Command", "Start-Sleep -Seconds 60")
	} else {
		cmd = exec.Command("sleep", "60")
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("spawn long-runner: %v", err)
	}
	pid := cmd.Process.Pid
	reaped := false
	reap := func() {
		if reaped {
			return
		}
		reaped = true
		_, _ = cmd.Process.Wait()
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		reap()
	})
	return pid, reap
}

// knownDeadPid spawns a fast-exiting child and waits for it to finish,
// returning the PID. PID recycling could in principle make this PID
// belong to a new process by the time the test reads it, but on a
// reasonably idle test host the window is too small to matter — and a
// flaky stale-classification is harmless: the doctor would prompt
// instead of cleaning silently.
func knownDeadPid(t *testing.T) int {
	t.Helper()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "exit", "0")
	} else {
		cmd = exec.Command("true")
	}
	if err := cmd.Run(); err != nil {
		// `true` / `cmd /c exit 0` should never fail; fail loudly so a
		// genuine spawn problem (PATH issue) is not silently masked.
		t.Fatalf("spawn known-dead helper: %v", err)
	}
	return cmd.ProcessState.Pid()
}

// writeMarker writes a PID marker into the per-run subdir of stateRuns,
// creating the dir if needed. Returns the full file path.
func writeMarker(t *testing.T, stateRuns string, runTimestamp string, parentPid, seq int, agent string, marker userstate.PidMarker) string {
	t.Helper()
	dir := filepath.Join(stateRuns, fmt.Sprintf("%s-%d", runTimestamp, parentPid))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("%03d-%s.pid", seq, agent))
	body, err := json.Marshal(marker)
	if err != nil {
		t.Fatalf("marshal marker: %v", err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	return path
}

func TestRunDoctorOrphans_Orphan_KilledOnYes(t *testing.T) {
	runs := isolateUserStateDir(t)

	childPid, reap := spawnLongRunner(t)
	deadParentPid := knownDeadPid(t)

	marker := userstate.PidMarker{ChildPid: childPid, ParentPid: deadParentPid, Cwd: t.TempDir()}
	path := writeMarker(t, runs, "20260528-100000", deadParentPid, 1, "test-writer", marker)

	stdin := strings.NewReader("y\n")
	var stdout bytes.Buffer
	if err := runDoctorOrphans(stdin, &stdout); err != nil {
		t.Fatalf("runDoctorOrphans: %v\n--- output ---\n%s", err, stdout.String())
	}

	// On Unix, SIGKILL leaves the child as a zombie until its parent
	// (this test process) reaps it; processAlive via signal-0 still
	// reports the zombie as alive. Reap so the assertion below sees
	// the PID actually gone.
	reap()

	if processAlive(childPid) {
		t.Errorf("child pid %d still alive after kill", childPid)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("marker file should be removed; stat err: %v", err)
	}
	if !strings.Contains(stdout.String(), "Killed pid") {
		t.Errorf("expected 'Killed pid' in output, got:\n%s", stdout.String())
	}
}

func TestRunDoctorOrphans_Orphan_KeptOnNo(t *testing.T) {
	runs := isolateUserStateDir(t)

	childPid, _ := spawnLongRunner(t)
	deadParentPid := knownDeadPid(t)

	marker := userstate.PidMarker{ChildPid: childPid, ParentPid: deadParentPid, Cwd: t.TempDir()}
	path := writeMarker(t, runs, "20260528-100000", deadParentPid, 1, "test-writer", marker)

	stdin := strings.NewReader("n\n")
	var stdout bytes.Buffer
	if err := runDoctorOrphans(stdin, &stdout); err != nil {
		t.Fatalf("runDoctorOrphans: %v", err)
	}

	if !processAlive(childPid) {
		t.Errorf("child pid %d was killed after 'n'", childPid)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("marker file should be preserved; stat err: %v", err)
	}
	if !strings.Contains(stdout.String(), "Kept pid") {
		t.Errorf("expected 'Kept pid' in output, got:\n%s", stdout.String())
	}
}

func TestRunDoctorOrphans_StaleChildDead_SilentlyCleaned(t *testing.T) {
	runs := isolateUserStateDir(t)

	deadChildPid := knownDeadPid(t)
	deadParentPid := knownDeadPid(t)

	marker := userstate.PidMarker{ChildPid: deadChildPid, ParentPid: deadParentPid, Cwd: t.TempDir()}
	path := writeMarker(t, runs, "20260528-100000", deadParentPid, 1, "test-writer", marker)

	// Empty stdin: if the doctor prompts at all we'd see a "Please
	// answer y or n." reminder loop and EOF would return false; making
	// stdin empty (rather than scripted) forces a failure if any prompt
	// is unexpectedly emitted.
	stdin := strings.NewReader("")
	var stdout bytes.Buffer
	if err := runDoctorOrphans(stdin, &stdout); err != nil {
		t.Fatalf("runDoctorOrphans: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("stale marker should be silently removed; stat err: %v", err)
	}
	if !strings.Contains(stdout.String(), "Cleaned up") {
		t.Errorf("expected 'Cleaned up' in output, got:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "[y/n]") {
		t.Errorf("doctor must not prompt for stale markers, but output contained '[y/n]':\n%s", stdout.String())
	}
}

func TestRunDoctorOrphans_LiveDispatch_SkippedNotPrompted(t *testing.T) {
	runs := isolateUserStateDir(t)

	childPid, _ := spawnLongRunner(t)
	// Parent is this test process — definitionally alive — so the
	// classifier should treat the dispatch as in-flight.
	parentPid := os.Getpid()

	marker := userstate.PidMarker{ChildPid: childPid, ParentPid: parentPid, Cwd: t.TempDir()}
	path := writeMarker(t, runs, "20260528-100000", parentPid, 1, "test-writer", marker)

	stdin := strings.NewReader("")
	var stdout bytes.Buffer
	if err := runDoctorOrphans(stdin, &stdout); err != nil {
		t.Fatalf("runDoctorOrphans: %v", err)
	}

	if !processAlive(childPid) {
		t.Errorf("live-dispatch child pid %d was killed (should be untouched)", childPid)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("live-dispatch marker should be preserved; stat err: %v", err)
	}
	if !strings.Contains(stdout.String(), "Skipped") {
		t.Errorf("expected 'Skipped' in output for live dispatch, got:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "[y/n]") {
		t.Errorf("doctor must not prompt for live dispatches, but output contained '[y/n]':\n%s", stdout.String())
	}
}

func TestRunDoctorOrphans_NoMarkers_ReportsCleanState(t *testing.T) {
	// Empty isolation dir — the runs/ subtree has no marker files at all.
	isolateUserStateDir(t)

	stdin := strings.NewReader("")
	var stdout bytes.Buffer
	if err := runDoctorOrphans(stdin, &stdout); err != nil {
		t.Fatalf("runDoctorOrphans on clean state: %v", err)
	}
	if !strings.Contains(stdout.String(), "No orphan claude subprocesses found") {
		t.Errorf("expected clean-state banner, got:\n%s", stdout.String())
	}
}

// TestProcessAlive_DeadAfterShortLivedHelper sanity-checks the
// platform-specific processAlive helper against a child that was just
// reaped, which is the easy-to-control side of the dead/alive boundary.
// Catches build-tag regressions before they cascade into doctor-level
// false positives.
func TestProcessAlive_DeadAfterShortLivedHelper(t *testing.T) {
	pid := knownDeadPid(t)
	// Give the OS a beat to fully retire the process entry; on Windows
	// the PID can briefly remain "still active" in GetExitCodeProcess
	// terms.
	time.Sleep(50 * time.Millisecond)
	if processAlive(pid) {
		t.Errorf("processAlive(%d) = true for reaped helper", pid)
	}
}

func TestProcessAlive_LiveLongRunner(t *testing.T) {
	pid, _ := spawnLongRunner(t)
	if !processAlive(pid) {
		t.Errorf("processAlive(%d) = false for live long-runner", pid)
	}
}
