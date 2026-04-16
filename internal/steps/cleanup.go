package steps

import (
	"fmt"
	"os"
	"strings"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/log"
	"github.com/optivem/gh-optivem/internal/shell"
)

const (
	deletedRepoFmt  = "Deleted repository %s"
	keepingRepoFmt  = "Keeping repository: https://github.com/%s"
)

// GetSonarProjectKeys returns the SonarCloud project keys for the given config.
func GetSonarProjectKeys(cfg *config.Config) []string {
	if cfg.Arch == "monolith" {
		if cfg.RepoStrategy == "monorepo" {
			return []string{cfg.Owner + "_" + cfg.Repo + "-system"}
		}
		return []string{cfg.Owner + "_" + cfg.SystemRepo + "-system"}
	}
	if cfg.RepoStrategy == "monorepo" {
		prefix := cfg.Owner + "_" + cfg.Repo
		return []string{
			prefix + "-backend",
			prefix + "-frontend",
		}
	}
	return []string{
		cfg.Owner + "_" + cfg.BackendRepo + "-backend",
		cfg.Owner + "_" + cfg.FrontendRepo + "-frontend",
	}
}

// Cleanup deletes repos, SonarCloud projects, and local directories (test mode only).
func Cleanup(cfg *config.Config, gh *shell.GitHub, sc *shell.SonarCloud) {
	if !cfg.TestMode {
		return
	}

	if resolveCleanup(cfg) {
		deleteRepos(cfg, gh)
		deleteSonarProjects(cfg, sc)
		deleteLocalDirs(cfg)
		log.OK("Cleanup complete")
	} else {
		logKeptRepos(cfg)
	}
}

func resolveCleanup(cfg *config.Config) bool {
	if cfg.Cleanup != "ask" {
		return cfg.Cleanup == "yes"
	}
	fmt.Printf("\nDelete test repository %s? [y/N] ", cfg.FullRepo)
	var answer string
	fmt.Scanln(&answer)
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

func deleteRepos(cfg *config.Config, gh *shell.GitHub) {
	log.Logf("Cleaning up: deleting %s...", cfg.FullRepo)
	gh.Delete()
	log.OKf(deletedRepoFmt, cfg.FullRepo)

	if cfg.RepoStrategy != "multirepo" {
		return
	}
	if cfg.Arch == "multitier" {
		ghBackend := gh.ForRepo(cfg.BackendFullRepo)
		ghFrontend := gh.ForRepo(cfg.FrontendFullRepo)
		ghBackend.Delete()
		log.OKf(deletedRepoFmt, cfg.BackendFullRepo)
		ghFrontend.Delete()
		log.OKf(deletedRepoFmt, cfg.FrontendFullRepo)
	} else {
		ghSystem := gh.ForRepo(cfg.SystemFullRepo)
		ghSystem.Delete()
		log.OKf(deletedRepoFmt, cfg.SystemFullRepo)
	}
}

func deleteSonarProjects(cfg *config.Config, sc *shell.SonarCloud) {
	for _, key := range GetSonarProjectKeys(cfg) {
		sc.DeleteProject(key)
	}
}

func deleteLocalDirs(cfg *config.Config) {
	for _, dir := range []string{cfg.RepoDir, cfg.FrontendRepoDir, cfg.BackendRepoDir, cfg.SystemRepoDir} {
		if dir != "" {
			os.RemoveAll(dir)
		}
	}
}

func logKeptRepos(cfg *config.Config) {
	log.Logf(keepingRepoFmt, cfg.FullRepo)
	if cfg.RepoStrategy != "multirepo" {
		return
	}
	if cfg.Arch == "multitier" {
		log.Logf(keepingRepoFmt, cfg.FrontendFullRepo)
		log.Logf(keepingRepoFmt, cfg.BackendFullRepo)
	} else {
		log.Logf(keepingRepoFmt, cfg.SystemFullRepo)
	}
}
