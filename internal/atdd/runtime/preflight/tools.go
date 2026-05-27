// tools.go holds the local-tool presence checks the preflight package
// invokes alongside its structural (repo / tier / sonar / board) checks.
// Lifted out of the driver so every "preflight" failure — missing
// directory, missing remote, missing CLI — surfaces through one
// aggregated error block at startup instead of in two separate phases
// (structural first, claude second).
package preflight

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// VerifyClaude runs `claude --no-update-check --version` as a cheap
// health check. Used by the implement-time preflight via Options.ClaudeCheck
// so missing-binary or missing-credentials failures surface at startup
// rather than several service-task spinners into the run.
//
// Returns nil on success; otherwise an error with operator guidance
// pointing at `claude /login` and the --manual-agents fallback.
func VerifyClaude(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "claude", "--no-update-check", "--version")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		tail := strings.TrimSpace(stderr.String())
		if tail == "" {
			tail = err.Error()
		}
		return fmt.Errorf(
			"claude CLI pre-flight failed: %s\n  Ensure `claude` is on PATH and authenticated via `claude /login` (credentials live in ~/.claude/).\n  Use --manual-agents to fall back to the v1 two-window workflow without the CLI.",
			tail)
	}
	return nil
}
