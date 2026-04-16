package steps

import (
	"fmt"
	"os"
	"strings"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/log"
	"github.com/optivem/gh-optivem/internal/shell"
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

	shouldCleanup := cfg.Cleanup
	if shouldCleanup == "ask" {
		fmt.Printf("\nDelete test repository %s? [y/N] ", cfg.FullRepo)
		var answer string
		fmt.Scanln(&answer)
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer == "y" || answer == "yes" {
			shouldCleanup = "yes"
		} else {
			shouldCleanup = "no"
		}
	}

	if shouldCleanup == "yes" {
		log.Logf("Cleaning up: deleting %s...", cfg.FullRepo)
		gh.Delete()
		log.OKf("Deleted repository %s", cfg.FullRepo)

		if cfg.RepoStrategy == "multirepo" {
			if cfg.Arch == "multitier" {
				ghFrontend := gh.ForRepo(cfg.FrontendFullRepo)
				ghBackend := gh.ForRepo(cfg.BackendFullRepo)
				ghBackend.Delete()
				log.OKf("Deleted repository %s", cfg.BackendFullRepo)
				ghFrontend.Delete()
				log.OKf("Deleted repository %s", cfg.FrontendFullRepo)
			} else {
				ghSystem := gh.ForRepo(cfg.SystemFullRepo)
				ghSystem.Delete()
				log.OKf("Deleted repository %s", cfg.SystemFullRepo)
			}
		}

		for _, key := range GetSonarProjectKeys(cfg) {
			sc.DeleteProject(key)
		}

		if cfg.RepoDir != "" {
			os.RemoveAll(cfg.RepoDir)
		}
		if cfg.FrontendRepoDir != "" {
			os.RemoveAll(cfg.FrontendRepoDir)
		}
		if cfg.BackendRepoDir != "" {
			os.RemoveAll(cfg.BackendRepoDir)
		}
		if cfg.SystemRepoDir != "" {
			os.RemoveAll(cfg.SystemRepoDir)
		}

		log.OK("Cleanup complete")
	} else {
		log.Logf("Keeping repository: https://github.com/%s", cfg.FullRepo)
		if cfg.RepoStrategy == "multirepo" {
			if cfg.Arch == "multitier" {
				log.Logf("Keeping repository: https://github.com/%s", cfg.FrontendFullRepo)
				log.Logf("Keeping repository: https://github.com/%s", cfg.BackendFullRepo)
			} else {
				log.Logf("Keeping repository: https://github.com/%s", cfg.SystemFullRepo)
			}
		}
	}
}
