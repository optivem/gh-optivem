package kickoff

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// ExecImplement returns an ImplementFunc that runs `gh optivem implement` on a
// story in a child process, headless and auto-approving below the human tier.
func ExecImplement(root string, stdout, stderr io.Writer, extraArgs ...string) ImplementFunc {
	return func(ctx context.Context, s Created) error {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("locate gh-optivem binary: %w", err)
		}
		args := append([]string{"--auto"}, extraArgs...)
		args = append(args, "implement", s.ID, "--headless")
		cmd := exec.CommandContext(ctx, exe, args...)
		cmd.Dir = root
		cmd.Stdout, cmd.Stderr = stdout, stderr
		return cmd.Run()
	}
}
