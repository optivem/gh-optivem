// doctor_orphans.go adds the `gh optivem doctor --orphans` recovery path:
// scan the user-level state dir for PID marker files left behind by crashed
// `gh optivem implement` runs, classify each as stale / live-dispatch /
// orphan, and prompt the operator to kill the orphans.
//
// The PID marker schema and the state-dir resolver are owned by
// internal/userstate so the runtime writer side (driver, clauderun) and
// this reader side share one source of truth.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/optivem/gh-optivem/internal/promptio"
	"github.com/optivem/gh-optivem/internal/userstate"
)

// orphanScanResult bundles the per-orphan kill-failure tally so runDoctor
// can decide its exit code. stale-removed and declined-kept are not
// failures.
type orphanScanResult struct {
	killFailures int
}

// pidMarkerFile is one parsed entry from the user-level state tree:
// the on-disk path, the JSON payload, and the bits parsed out of the
// surrounding path (run timestamp, parent-pid, sequence, agent name).
// File-derived fields are split out so the doctor listing can show
// per-agent context without re-parsing during the loop body.
type pidMarkerFile struct {
	Path         string
	Marker       userstate.PidMarker
	RunTimestamp string
	DirParentPid int
	Seq          int
	Agent        string
	ModTime      time.Time
}

// pidFileRe matches the `<seq>-<agent>.pid` filename shape the driver
// writes. The driver formats seq with `%03d` (see
// driver.runState.dispatchPaths) but accepting >=1 digit keeps this
// reader robust against a future width change. Agent names are
// kebab-lowercase (see internal/atdd/runtime/agents).
var pidFileRe = regexp.MustCompile(`^([0-9]+)-([a-z0-9][a-z0-9-]*)\.pid$`)

// runDirRe matches the `<runTimestamp>-<parent-pid>` directory name shape
// resolvePidRunDir composes. runTimestamp uses driver.runState's format
// (`20060102-150405`); we extract the trailing `-<pid>` and treat the
// remainder as the timestamp without re-validating its layout — the
// listing is informational, not machine-parsed downstream.
var runDirRe = regexp.MustCompile(`^(.+)-([0-9]+)$`)

func runDoctorOrphans(stdin io.Reader, stdout io.Writer) error {
	fmt.Fprintln(stdout, separator)
	fmt.Fprintln(stdout, "  Doctor — orphan claude subprocesses from crashed implement runs")
	fmt.Fprintln(stdout, separator)
	fmt.Fprintln(stdout)

	stateDir, err := userstate.Dir()
	if err != nil {
		return fmt.Errorf("doctor --orphans: resolve user state dir: %w", err)
	}
	runsDir := filepath.Join(stateDir, "runs")

	files, err := loadPidMarkers(runsDir)
	if err != nil {
		return fmt.Errorf("doctor --orphans: scan %s: %w", runsDir, err)
	}

	var orphans []pidMarkerFile
	staleCleaned := 0
	live := 0
	for _, f := range files {
		switch {
		case !processAlive(f.Marker.ChildPid):
			// Child is gone — the marker is leftover bookkeeping;
			// drop it silently so the user's tree stays tidy.
			_ = os.Remove(f.Path)
			staleCleaned++
		case processAlive(f.Marker.ParentPid):
			// Parent still running → live dispatch in progress.
			// Touching it would race the running implement; skip.
			live++
		default:
			orphans = append(orphans, f)
		}
	}

	if staleCleaned > 0 {
		fmt.Fprintf(stdout, "  Cleaned up %d stale marker file(s) (child already dead).\n", staleCleaned)
	}
	if live > 0 {
		fmt.Fprintf(stdout, "  Skipped %d marker(s) for live dispatches in progress.\n", live)
	}
	if len(orphans) == 0 {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "  No orphan claude subprocesses found.")
		fmt.Fprintln(stdout, separator)
		return nil
	}

	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "  Found %d orphan claude subprocess(es):\n\n", len(orphans))
	now := time.Now()
	for _, o := range orphans {
		age := now.Sub(o.ModTime).Round(time.Second)
		fmt.Fprintf(stdout, "  - pid %d  agent %s  run %s  age %s\n", o.Marker.ChildPid, o.Agent, o.RunTimestamp, age)
		fmt.Fprintf(stdout, "      cwd: %s\n", o.Marker.Cwd)
	}
	fmt.Fprintln(stdout)

	result := orphanScanResult{}
	for _, o := range orphans {
		prompt := fmt.Sprintf("Kill pid %d (%s, %s)?", o.Marker.ChildPid, o.Agent, o.RunTimestamp)
		yes, err := promptio.ConfirmYN(stdin, stdout, prompt)
		if err != nil {
			return fmt.Errorf("doctor --orphans: prompt: %w", err)
		}
		if !yes {
			fmt.Fprintf(stdout, "  Kept pid %d.\n", o.Marker.ChildPid)
			continue
		}
		if killErr := killProcess(o.Marker.ChildPid); killErr != nil {
			fmt.Fprintf(stdout, "  ✗ Failed to kill pid %d: %v\n", o.Marker.ChildPid, killErr)
			result.killFailures++
			continue
		}
		if rmErr := os.Remove(o.Path); rmErr != nil && !os.IsNotExist(rmErr) {
			fmt.Fprintf(stdout, "  ! Killed pid %d but failed to remove marker %s: %v\n", o.Marker.ChildPid, o.Path, rmErr)
			continue
		}
		fmt.Fprintf(stdout, "  ✓ Killed pid %d and removed marker.\n", o.Marker.ChildPid)
	}

	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, separator)
	if result.killFailures > 0 {
		return fmt.Errorf("doctor --orphans: %d kill(s) failed", result.killFailures)
	}
	return nil
}

// loadPidMarkers walks runsDir and returns every parseable PID marker
// file. Unparseable filenames, unreadable bodies, and malformed JSON are
// silently skipped — recovery is best-effort and a single junk file should
// not abort the sweep. A missing runsDir is not an error: it means no
// dispatch has ever written a marker (e.g. fresh install), which is a
// no-orphans state.
//
// Results are sorted by filename for deterministic output (run dir, then
// seq within run).
func loadPidMarkers(runsDir string) ([]pidMarkerFile, error) {
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []pidMarkerFile
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirName := entry.Name()
		dirMatch := runDirRe.FindStringSubmatch(dirName)
		if dirMatch == nil {
			continue
		}
		runTs := dirMatch[1]
		dirParentPid, convErr := strconv.Atoi(dirMatch[2])
		if convErr != nil {
			continue
		}

		runDir := filepath.Join(runsDir, dirName)
		pidEntries, err := os.ReadDir(runDir)
		if err != nil {
			continue
		}
		for _, pidEntry := range pidEntries {
			if pidEntry.IsDir() {
				continue
			}
			fileMatch := pidFileRe.FindStringSubmatch(pidEntry.Name())
			if fileMatch == nil {
				continue
			}
			seq, convErr := strconv.Atoi(fileMatch[1])
			if convErr != nil {
				continue
			}
			path := filepath.Join(runDir, pidEntry.Name())
			body, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			var marker userstate.PidMarker
			if err := json.Unmarshal(body, &marker); err != nil {
				continue
			}
			info, err := pidEntry.Info()
			var mod time.Time
			if err == nil {
				mod = info.ModTime()
			}
			out = append(out, pidMarkerFile{
				Path:         path,
				Marker:       marker,
				RunTimestamp: runTs,
				DirParentPid: dirParentPid,
				Seq:          seq,
				Agent:        fileMatch[2],
				ModTime:      mod,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RunTimestamp != out[j].RunTimestamp {
			return out[i].RunTimestamp < out[j].RunTimestamp
		}
		return out[i].Seq < out[j].Seq
	})
	return out, nil
}

// killProcess sends a hard kill to pid. os.Process.Kill maps to
// TerminateProcess on Windows and SIGKILL on Unix, so this is the same
// implementation everywhere; the wrap exists only to keep the doctor's
// loop readable and to pair with the build-tagged processAlive helper.
func killProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}
