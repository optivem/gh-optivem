package steps

import (
	"os"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/kernel/log"
)

// Cleanup deletes the local scaffold dir(s) on success so the user is left with
// just the remote repo(s) + SonarCloud projects. main.go forces KeepLocal=true
// on failure so the broken scaffold can be inspected.
func Cleanup(cfg *config.Config) {
	if cfg.KeepLocal {
		logKeptLocalDirs(cfg)
		return
	}
	deleteLocalDirs(cfg)
	log.Success("Deleted local scaffold dir(s)")
}

func deleteLocalDirs(cfg *config.Config) {
	for _, dir := range []string{cfg.RepoDir, cfg.FrontendRepoDir, cfg.BackendRepoDir, cfg.SystemRepoDir} {
		if dir != "" {
			os.RemoveAll(dir)
		}
	}
}

func logKeptLocalDirs(cfg *config.Config) {
	for _, dir := range []string{cfg.RepoDir, cfg.FrontendRepoDir, cfg.BackendRepoDir, cfg.SystemRepoDir} {
		if dir != "" {
			log.Infof("Keeping local dir: %s", dir)
		}
	}
}
