// asset_commands.go wires the `gh optivem asset <verb>` subtree. Asset
// commands manage the embedded references asset tree (architecture
// doctrine + per-language equivalents) that gh-optivem syncs to
// ~/.gh-optivem/references/. Auto-sync runs at startup on every
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
		Short: "Manage gh-optivem embedded assets (reference docs)",
	}
	cmd.AddCommand(newAssetSyncCmd())
	return cmd
}

func newAssetSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Force-sync embedded assets to per-user paths",
		Long: `Walk the binary's embedded runtime/references/ tree and write reference
docs (architecture doctrine + per-language equivalents) to
~/.gh-optivem/references/.

Auto-sync runs on every gh-optivem invocation when the per-user stamp does not
match the binary's version. ` + "`gh optivem asset sync`" + ` is the explicit form
for users with the auto-sync escape hatch set (` + assetsync.EscapeHatchEnv + `).`,
		Example: `  gh optivem asset sync`,
		Args:    cobra.NoArgs,
		Run: func(_ *cobra.Command, _ []string) {
			res, err := assetsync.ForceSync(version.Version)
			if err != nil {
				log.Fatalf("asset sync: %v", err)
			}
			fmt.Fprintln(os.Stderr, res.Notice)
		},
	}
}
