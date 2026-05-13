// atdd_init.go provides the thin wrapper that lets `runInit`'s buildSteps
// call `atdd.Install` as a phase step. There is no standalone
// `gh optivem atdd install` subcommand — ATDD assets are installed only as
// part of `gh optivem init`. To refresh ATDD assets in an existing repo,
// re-run `gh optivem init` (or copy the assets from a fresh shop checkout
// by hand).
package main

import (
	"github.com/optivem/gh-optivem/internal/atdd"
	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/log"
)

// installAtddDuringInit is the buildSteps fn for "Install ATDD assets" inside
// runInit. Reuses the existing shop clone at cfg.ShopPath (no re-clone) and
// derives all install options from cfg. Panics on error — caught by the
// runStep recover so the step appears as a failure in the run summary.
func installAtddDuringInit(cfg *config.Config) {
	opts := atdd.Options{
		ShopPath:     cfg.ShopPath,
		DestDir:      cfg.RepoDir,
		Repo:         cfg.Repo,
		Arch:         cfg.Arch,
		RepoStrategy: cfg.RepoStrategy,
		// init writes into a fresh clone — no local edits to protect, and
		// the pre-flight check would just be wasted I/O.
		Force: true,
	}
	if err := atdd.Install(opts); err != nil {
		log.Fatalf("ATDD install: %v", err)
	}
	log.Successf("Installed ATDD assets into %s", cfg.RepoDir)
}
