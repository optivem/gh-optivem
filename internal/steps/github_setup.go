// Package steps implements the scaffold pipeline steps.
package steps

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/log"
	"github.com/optivem/gh-optivem/internal/shell"
)

// waitForRepo polls until the GitHub repo has a default branch (i.e. is fully initialized).
func waitForRepo(ghRepo *shell.GitHub, maxWait time.Duration) {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		out, err := shell.RunCapture(
			fmt.Sprintf("gh repo view %s --json defaultBranchRef --jq .defaultBranchRef.name", ghRepo.Repo), "")
		if err == nil && strings.TrimSpace(out) != "" {
			return
		}
		time.Sleep(3 * time.Second)
	}
	log.Fatalf("repo %s not initialized after %s: default branch not found", ghRepo.Repo, maxWait)
}

// CreateRepos creates the GitHub repository (and component repos for multitier).
func CreateRepos(cfg *config.Config, gh *shell.GitHub) {
	log.Logf("Step 1: Creating repository %s...", cfg.FullRepo)

	if cfg.DryRun {
		log.Logf("[DRY RUN] gh repo create %s --public --add-readme --license mit", cfg.FullRepo)
		if cfg.RepoStrategy == "multirepo" {
			log.Logf("[DRY RUN] gh repo create %s --public --add-readme --license mit", cfg.FrontendFullRepo)
			log.Logf("[DRY RUN] gh repo create %s --public --add-readme --license mit", cfg.BackendFullRepo)
		}
		return
	}

	gh.CreateRepo()
	waitForRepo(gh, 3*time.Minute)
	log.OKf("Created repository: %s", cfg.FullRepo)

	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "multitier" {
			ghFrontend := gh.ForRepo(cfg.FrontendFullRepo)
			ghBackend := gh.ForRepo(cfg.BackendFullRepo)

			ghFrontend.CreateRepo()
			waitForRepo(ghFrontend, 3*time.Minute)
			log.OKf("Created repository: %s", cfg.FrontendFullRepo)

			ghBackend.CreateRepo()
			waitForRepo(ghBackend, 3*time.Minute)
			log.OKf("Created repository: %s", cfg.BackendFullRepo)
		} else {
			ghSystem := gh.ForRepo(cfg.SystemFullRepo)
			ghSystem.CreateRepo()
			waitForRepo(ghSystem, 3*time.Minute)
			log.OKf("Created repository: %s", cfg.SystemFullRepo)
		}
	}
}

// SetupEnvironments creates GitHub environments on the main repo.
func SetupEnvironments(cfg *config.Config, gh *shell.GitHub) {
	log.Log("Step 2: Creating environments...")

	lang := cfg.Lang
	if cfg.Arch == "multitier" {
		lang = cfg.BackendLang
	}
	prefix := cfg.Arch + "-" + lang

	for _, stage := range []string{"acceptance", "qa", "production"} {
		envName := prefix + "-" + stage
		gh.CreateEnvironment(envName)
	}
	log.OKf("Created environments: %s-acceptance, %s-qa, %s-production", prefix, prefix, prefix)
}

// SetupSecretsAndVariables sets GitHub Actions secrets and variables.
func SetupSecretsAndVariables(cfg *config.Config, gh *shell.GitHub) {
	log.Log("Step 3: Setting secrets and variables...")

	gh.SecretSet("DOCKERHUB_TOKEN", cfg.DockerHubToken)
	gh.SecretSet("SONAR_TOKEN", cfg.SonarToken)
	gh.VariableSet("DOCKERHUB_USERNAME", cfg.DockerHubUsername)

	if cfg.RepoStrategy == "multirepo" {
		gh.SecretSet("GHCR_TOKEN", cfg.GHCRToken)

		var componentRepos []string
		if cfg.Arch == "multitier" {
			componentRepos = []string{cfg.FrontendFullRepo, cfg.BackendFullRepo}
		} else {
			componentRepos = []string{cfg.SystemFullRepo}
		}
		for _, fullRepo := range componentRepos {
			ghComp := gh.ForRepo(fullRepo)
			ghComp.SecretSet("DOCKERHUB_TOKEN", cfg.DockerHubToken)
			ghComp.SecretSet("SONAR_TOKEN", cfg.SonarToken)
			ghComp.VariableSet("DOCKERHUB_USERNAME", cfg.DockerHubUsername)
		}
		log.OK("Set secrets and variables on component repositories")
	}

	log.OK("Set secrets and variables")
}

// CloneRepos clones the repository (and component repos for multitier).
func CloneRepos(cfg *config.Config, gh *shell.GitHub) {
	log.Log("Step 4: Cloning repo(s)...")

	if cfg.DryRun {
		log.Log("[DRY RUN] Would clone repo(s)")
		return
	}

	repoDir := filepath.Join(cfg.WorkDir, "repo")
	gh.Clone(repoDir)
	cfg.RepoDir = repoDir
	log.OKf("Cloned %s", cfg.FullRepo)

	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "multitier" {
			frontendDir := filepath.Join(cfg.WorkDir, "repo-frontend")
			backendDir := filepath.Join(cfg.WorkDir, "repo-backend")

			gh.ForRepo(cfg.FrontendFullRepo).Clone(frontendDir)
			cfg.FrontendRepoDir = frontendDir
			log.OKf("Cloned %s", cfg.FrontendFullRepo)

			gh.ForRepo(cfg.BackendFullRepo).Clone(backendDir)
			cfg.BackendRepoDir = backendDir
			log.OKf("Cloned %s", cfg.BackendFullRepo)
		} else {
			systemDir := filepath.Join(cfg.WorkDir, "repo-system")

			gh.ForRepo(cfg.SystemFullRepo).Clone(systemDir)
			cfg.SystemRepoDir = systemDir
			log.OKf("Cloned %s", cfg.SystemFullRepo)
		}
	}
}

// EnsureWorkflowDir creates the .github/workflows directory in a repo.
func EnsureWorkflowDir(repoDir string) {
	os.MkdirAll(filepath.Join(repoDir, ".github", "workflows"), 0755)
}
