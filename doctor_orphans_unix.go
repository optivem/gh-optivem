//go:build !windows

package main

import (
	"errors"
	"os"
	"syscall"
)

// processAlive returns true iff a process with the given PID is still
// running and the current user has permission to signal it. On POSIX
// os.FindProcess always succeeds, so the real probe is
// process.Signal(syscall.Signal(0)) — kernel returns ESRCH for an unknown
// PID and nil when the process exists (including the zombie case, where
// the PID is reaped but the entry still exists; that's fine — the doctor
// will clean the marker on its next pass).
//
// An EPERM ("operation not permitted") response means the process exists
// but lives in a different uid/security domain. From the doctor's
// perspective that's still "alive" — it's the operator's choice whether
// to escalate. We never see EPERM in practice (the marker was written by
// the same user), but the explicit branch documents the contract.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	if errors.Is(err, syscall.EPERM) {
		return true
	}
	return false
}
