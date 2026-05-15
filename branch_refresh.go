// branch_refresh.go wires the `gh optivem branch refresh` subcommand. It
// encapsulates the Scaled-TBD "main moved while my PR was open" ritual from
// docs/tbd.md:75-81 — fetch origin, rebase the current feature branch onto
// origin/main, then push with --force-with-lease.
//
// --force-with-lease is hardcoded; plain --force is not exposed so the
// operator cannot accidentally pick the unsafe variant. The command also
// refuses to run on `main` — refresh is for feature branches only.
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newBranchRefreshCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "refresh",
		Short: "Rebase current feature branch onto latest origin/main and force-with-lease push",
		Long: `git fetch origin && git rebase origin/main && git push --force-with-lease.
Encapsulates the Scaled-TBD "main moved while my PR was open" ritual from
docs/tbd.md. Hardcodes --force-with-lease; plain --force is not exposed.
Refuses to run on main.`,
		Example: `  gh optivem branch refresh`,
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			exitOnError(runBranchRefresh())
		},
	}
}

// runBranchRefresh runs against the repo at the current working directory.
// Refuses when HEAD is `main` — the ritual is for feature branches; refreshing
// `main` would mean rebasing trunk on itself and then force-pushing it, which
// is the exact thing docs/tbd.md forbids.
func runBranchRefresh() error {
	branch := currentBranch(".")
	if branch == "main" {
		return fmt.Errorf("refusing to refresh `main`: this command is for feature branches.\n       Refreshing main would force-push trunk, which is forbidden (docs/tbd.md).")
	}

	if err := runGit(".", "fetch", "origin"); err != nil {
		return fmt.Errorf("git fetch origin: %w", err)
	}
	if err := runGit(".", "rebase", "origin/main"); err != nil {
		return fmt.Errorf("git rebase origin/main: %w", err)
	}
	if err := runGit(".", "push", "--force-with-lease"); err != nil {
		return fmt.Errorf("git push --force-with-lease: %w", err)
	}
	return nil
}
