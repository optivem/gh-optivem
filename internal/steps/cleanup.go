package steps

import (
	"os"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/log"
)

// GetSonarProjectKeys returns the SonarCloud project keys for the given config.
// Keys must match the final value in scaffolded workflows after Pass 5 suffix
// dedupe in replacements.go: for multirepo, the repo name already encodes the
// component (e.g. "<base>-backend"), so no extra suffix is appended.
func GetSonarProjectKeys(cfg *config.Config) []string {
	if cfg.Arch == "monolith" {
		if cfg.RepoStrategy == "monorepo" {
			return []string{cfg.Owner + "_" + cfg.Repo + "-system"}
		}
		return []string{cfg.Owner + "_" + cfg.SystemRepo}
	}
	if cfg.RepoStrategy == "monorepo" {
		prefix := cfg.Owner + "_" + cfg.Repo
		return []string{
			prefix + "-backend",
			prefix + "-frontend",
		}
	}
	return []string{
		cfg.Owner + "_" + cfg.BackendRepo,
		cfg.Owner + "_" + cfg.FrontendRepo,
	}
}

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
