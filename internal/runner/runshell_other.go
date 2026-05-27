//go:build !windows

package runner

import "os/exec"

// applyCmdExeQuoting is a no-op on non-Windows platforms: cmd.exe is not in
// the picture, so batch-file metacharacter escaping doesn't apply.
func applyCmdExeQuoting(cmd *exec.Cmd, parts []string) {
	_ = cmd
	_ = parts
}
