// asset_commands.go wires the `gh optivem asset <verb>` subtree. Asset
// commands manage the embedded global asset tree (methodology docs and
// Claude Code subagents) that gh-optivem syncs to per-user paths
// (~/.gh-optivem/docs/, ~/.claude/). Auto-sync runs at startup on every
// invocation; this subtree exists for the explicit-force form needed
// when the auto-sync escape hatch is set.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	assetsync "github.com/optivem/gh-optivem/internal/assets/sync"
	"github.com/optivem/gh-optivem/internal/log"
	"github.com/optivem/gh-optivem/internal/version"
)

func newAssetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "asset",
		Short: "Manage gh-optivem embedded assets (methodology docs, Claude Code subagents)",
	}
	cmd.AddCommand(newAssetSyncCmd())
	return cmd
}

func newAssetSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Force-sync embedded assets to per-user paths",
		Long: `Walk the binary's embedded global/ tree and write methodology docs to
~/.gh-optivem/docs/ and Claude Code subagents to ~/.claude/{agents,commands}/atdd/.

Auto-sync runs on every gh-optivem invocation when the per-user stamp does not
match the binary's version. ` + "`gh optivem asset sync`" + ` is the explicit form
for users with the auto-sync escape hatch set (` + assetsync.EscapeHatchEnv + `).`,
		Args: cobra.NoArgs,
		Run: func(_ *cobra.Command, _ []string) {
			res, err := assetsync.ForceSync(version.Version)
			if err != nil {
				log.Fatalf("asset sync: %v", err)
			}
			fmt.Fprintln(os.Stderr, res.Notice)
		},
	}
}
