// Package userstate owns the user-level gh-optivem state directory and the
// schemas of the small files written there. Today the only file kind is a
// per-dispatch PID marker that `gh optivem implement` writes between
// cmd.Start() and cmd.Wait() and removes on clean exit; the same struct is
// read back by `gh optivem doctor --orphans` to recover from crashes.
//
// Why a dedicated package rather than a helper on driver: the runtime
// pipeline (driver, clauderun) is the writer, but the recovery tool
// (doctor) is the reader. Doctor cannot import from internal/atdd/runtime
// without pulling in the full pipeline graph, so the schema and the
// state-dir resolver live here, where both sides can depend on them.
package userstate

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// stateDirName is the gh-optivem-owned leaf directory appended to the
// platform's machine-local state base (LOCALAPPDATA / XDG_STATE_HOME / home).
const stateDirName = "gh-optivem"

// PidMarker is the JSON shape persisted at <Dir()>/runs/<ts>-<parent-pid>/<seq>-<agent>.pid
// while a dispatch is running. Read by doctor --orphans to distinguish a
// force-cancelled dispatch (parent dead → orphan) from a live rehearsal
// (parent alive → skip):
//
//   - ChildPid:  spawned claude PID — kill/probe target.
//   - ParentPid: gh-optivem PID that spawned the child — alive means "still
//     a live dispatch, leave it alone."
//   - Cwd:       dispatch working directory at write time — human context
//     in the doctor listing, since the file path itself
//     (user-level state dir) is project-agnostic.
type PidMarker struct {
	ChildPid  int    `json:"child_pid"`
	ParentPid int    `json:"parent_pid"`
	Cwd       string `json:"cwd"`
}

// Dir returns the user-level gh-optivem state directory — the
// machine-local home for transient OS-resource markers (notably
// per-dispatch PID files). Distinct from the per-project
// .gh-optivem/runs/ tree because the markers must survive worktree
// deletion: the motivating bug is `rm -rf worktrees/rehearsal-XYZ/`
// failing because orphan claude.exe holds handles inside the worktree
// — a sidecar marker would die with the worktree and leave the orphan
// untrackable.
//
// Resolution order:
//   - Windows: %LOCALAPPDATA%\gh-optivem, falling back to
//     <userhome>\AppData\Local\gh-optivem when LOCALAPPDATA is unset.
//     Deliberately NOT os.UserConfigDir — that returns %APPDATA%
//     (roaming), and PID files must stay machine-local.
//   - Linux/Mac: $XDG_STATE_HOME/gh-optivem, falling back to
//     <userhome>/.local/state/gh-optivem when XDG_STATE_HOME is unset.
//
// Returns ("", err) when even the home-dir fallback fails (e.g. no
// HOME in a stripped-down container). Callers downgrade to a stderr
// warning and skip marker writes / reads.
func Dir() (string, error) {
	if runtime.GOOS == "windows" {
		if base := os.Getenv("LOCALAPPDATA"); base != "" {
			return filepath.Join(base, stateDirName), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locate user home dir: %w", err)
		}
		return filepath.Join(home, "AppData", "Local", stateDirName), nil
	}
	if base := os.Getenv("XDG_STATE_HOME"); base != "" {
		return filepath.Join(base, stateDirName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate user home dir: %w", err)
	}
	return filepath.Join(home, ".local", "state", stateDirName), nil
}
