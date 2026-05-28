//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// processAlive returns true iff a process with the given PID is still
// running. On Windows, os.FindProcess calls OpenProcess with limited
// query rights and returns an error when the PID is no longer present —
// so a non-nil error from FindProcess is sufficient for "dead". When
// FindProcess succeeds we additionally probe GetExitCodeProcess: a
// returned exit code of STILL_ACTIVE (259) means the process is still
// running; any other value means it terminated (and the kernel kept the
// handle entry around for us to query).
//
// The two-step check matters because Windows recycles PIDs aggressively
// and OpenProcess will happily hand out a handle to a freshly-reaped
// process whose entry has not yet been cleaned. Without the exit-code
// probe the doctor would classify those as "alive" and refuse to clean
// the marker.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// proc.Pid is the same int we passed in; on Windows there's no
	// extra handle stored on the os.Process struct that we can probe
	// here, so go straight to the syscall layer.
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(proc.Pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	const stillActive = 259
	return code == stillActive
}
