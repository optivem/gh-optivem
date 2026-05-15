// pr_commands.go wires the `gh optivem pr <verb>` subtree. The pr noun
// wraps `gh pr` with TBD-discipline defaults: squash- or rebase-merges only,
// never a merge commit on main. See plans/20260514-2043-tbd-discipline-in-
// workspace-tool.md (Layer 3, item 10) and docs/tbd.md for the linear-trunk
// invariant this enforces.
package main

import (
	"errors"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// prMergeOptions captures the per-invocation flags for `pr merge`. Mirrors
// the upstream `gh pr merge` flag names so muscle memory transfers.
type prMergeOptions struct {
	Squash       bool
	Rebase       bool
	Auto         bool
	DeleteBranch bool
}

// newPrCmd builds the `gh optivem pr` parent. The parent has no Run, so
// invoking it without a subcommand prints help.
func newPrCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr",
		Short: "TBD-safe wrappers over `gh pr`",
	}
	cmd.AddCommand(newPrMergeCmd())
	return cmd
}

func newPrMergeCmd() *cobra.Command {
	opts := prMergeOptions{}
	cmd := &cobra.Command{
		Use:   "merge [<pr-number>]",
		Short: "Squash- or rebase-merge a PR via `gh pr merge`; never a merge commit",
		Long: `Wrapper over gh pr merge. Defaults to --squash; --rebase opt-in. The --merge mode is intentionally not exposed because merge commits on main break docs/tbd.md's linear-trunk invariant. Pass any other gh pr merge flags directly to the underlying CLI.`,
		Example: `  gh optivem pr merge
  gh optivem pr merge 123 --rebase
  gh optivem pr merge --squash --delete-branch
  gh optivem pr merge --auto --squash`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			exitOnError(runPrMerge(opts, args))
		},
	}
	cmd.Flags().BoolVar(&opts.Squash, "squash", false, "Squash-merge (default when neither --squash nor --rebase is set)")
	cmd.Flags().BoolVar(&opts.Rebase, "rebase", false, "Rebase-merge")
	cmd.Flags().BoolVar(&opts.Auto, "auto", false, "Enable auto-merge once required checks pass (gh pr merge --auto)")
	cmd.Flags().BoolVar(&opts.DeleteBranch, "delete-branch", false, "Delete the branch after merging (gh pr merge --delete-branch)")
	return cmd
}

// buildPrMergeArgs converts prMergeOptions plus any positional PR number into
// the argv slice passed to `gh`. Pure function — no exec, no stdout — so
// pr_commands_test.go can assert on the produced slice without invoking the
// real `gh` CLI. Defaults --squash when neither merge-mode flag is set, and
// rejects --squash + --rebase together (no implicit precedence).
func buildPrMergeArgs(opts prMergeOptions, args []string) ([]string, error) {
	if opts.Squash && opts.Rebase {
		return nil, errors.New("--squash and --rebase are mutually exclusive; pick one")
	}
	if !opts.Squash && !opts.Rebase {
		opts.Squash = true
	}

	ghArgs := []string{"pr", "merge"}
	if opts.Squash {
		ghArgs = append(ghArgs, "--squash")
	}
	if opts.Rebase {
		ghArgs = append(ghArgs, "--rebase")
	}
	if opts.Auto {
		ghArgs = append(ghArgs, "--auto")
	}
	if opts.DeleteBranch {
		ghArgs = append(ghArgs, "--delete-branch")
	}
	ghArgs = append(ghArgs, args...)
	return ghArgs, nil
}

// runPrMerge shells out to `gh pr merge` with the discipline-enforced argv
// from buildPrMergeArgs. stdout/stderr stream through to the operator so gh's
// own prompts, errors, and progress are visible. Auth and repo-state checks
// are left to gh itself.
func runPrMerge(opts prMergeOptions, args []string) error {
	ghArgs, err := buildPrMergeArgs(opts, args)
	if err != nil {
		return err
	}
	cmd := exec.Command("gh", ghArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
