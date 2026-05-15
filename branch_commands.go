// branch_commands.go wires the `gh optivem branch` subtree. Encapsulates the
// Scaled-TBD branch primitives documented in docs/tbd.md so the operator
// runs one command instead of three. Today the only child is `start <name>`,
// which freshens local `main` against origin before forking the new branch —
// preventing the common foot-gun of branching off a stale local `main`.
package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newBranchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "branch",
		Short: "Scaled-TBD branch primitives",
	}
	cmd.AddCommand(
		newBranchStartCmd(),
		newBranchRefreshCmd(),
	)
	return cmd
}

func newBranchStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <name>",
		Short: "Start a new feature branch off latest origin/main",
		Long: `git checkout main && git pull --rebase && git checkout -b <name>. Encapsulates the Scaled-TBD branch-start ritual from docs/tbd.md — one command instead of three. Refuses if the name is empty.`,
		Example: `  gh optivem branch start feature/payments`,
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			exitOnError(runBranchStart(args[0]))
		},
	}
}

// runBranchStart performs the Scaled-TBD branch-start ritual in the current
// working directory's repo: checkout main, pull --rebase, checkout -b <name>.
//
// Dirty-tree policy: errors from git are propagated verbatim. We do not
// auto-stash here. `git checkout main` will refuse to switch branches when
// local edits would be overwritten, and surfacing that error directly is the
// safest behaviour — the operator should consciously commit, stash, or
// discard their work before starting a new branch. Auto-stashing would hide
// state changes across a branch switch and is reserved for the pre-commit
// pull path (pullWithAutoStash) where staying on the same branch is implicit.
func runBranchStart(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("branch name must not be empty")
	}
	if err := runGit(".", "checkout", "main"); err != nil {
		return fmt.Errorf("git checkout main in .: %w", err)
	}
	if err := runGit(".", "pull", "--rebase"); err != nil {
		return fmt.Errorf("git pull --rebase in .: %w", err)
	}
	if err := runGit(".", "checkout", "-b", trimmed); err != nil {
		return fmt.Errorf("git checkout -b %s in .: %w", trimmed, err)
	}
	return nil
}
