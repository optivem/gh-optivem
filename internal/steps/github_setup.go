// Package steps implements the scaffold pipeline steps.
package steps

import (
	"os"
	"path/filepath"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/log"
	"github.com/optivem/gh-optivem/internal/shell"
)

func logCreated(fullRepo string) {
	log.OKf("Created repository: %s", fullRepo)
	log.OKf("  https://github.com/%s", fullRepo)
}

func logCloned(fullRepo, localPath string) {
	log.OKf("Cloned %s", fullRepo)
	log.OKf("  -> %s", localPath)
}

// CreateRepos creates the GitHub repository (and component repos for multitier).
func CreateRepos(cfg *config.Config, gh *shell.GitHub) {
	log.Logf("Step 1: Creating repository %s...", cfg.FullRepo)

	if cfg.DryRun {
		log.Logf("[DRY RUN] gh repo create %s --public", cfg.FullRepo)
		if cfg.RepoStrategy == "multirepo" {
			log.Logf("[DRY RUN] gh repo create %s --public", cfg.FrontendFullRepo)
			log.Logf("[DRY RUN] gh repo create %s --public", cfg.BackendFullRepo)
		}
		return
	}

	gh.CreateRepo()
	logCreated(cfg.FullRepo)

	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "multitier" {
			ghFrontend := gh.ForRepo(cfg.FrontendFullRepo)
			ghBackend := gh.ForRepo(cfg.BackendFullRepo)

			ghFrontend.CreateRepo()
			logCreated(cfg.FrontendFullRepo)

			ghBackend.CreateRepo()
			logCreated(cfg.BackendFullRepo)
		} else {
			ghSystem := gh.ForRepo(cfg.SystemFullRepo)
			ghSystem.CreateRepo()
			logCreated(cfg.SystemFullRepo)
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

	setSecret(gh, "DOCKERHUB_TOKEN", cfg.DockerHubToken)
	setSecret(gh, "SONAR_TOKEN", cfg.SonarToken)
	setSecret(gh, "WORKFLOW_TOKEN", cfg.WorkflowToken)
	setVariable(gh, "DOCKERHUB_USERNAME", cfg.DockerHubUsername)

	if cfg.RepoStrategy == "multirepo" {
		setSecret(gh, "GHCR_TOKEN", cfg.GHCRToken)

		var componentRepos []string
		if cfg.Arch == "multitier" {
			componentRepos = []string{cfg.FrontendFullRepo, cfg.BackendFullRepo}
		} else {
			componentRepos = []string{cfg.SystemFullRepo}
		}
		for _, fullRepo := range componentRepos {
			ghComp := gh.ForRepo(fullRepo)
			setSecret(ghComp, "DOCKERHUB_TOKEN", cfg.DockerHubToken)
			setSecret(ghComp, "SONAR_TOKEN", cfg.SonarToken)
			setVariable(ghComp, "DOCKERHUB_USERNAME", cfg.DockerHubUsername)
		}
		log.OK("Set secrets and variables on component repositories")
	}

	log.OK("Set secrets and variables")
}

func setSecret(gh *shell.GitHub, name, value string) {
	gh.SecretSet(name, value)
	log.OKf("  secret: %s", name)
}

func setVariable(gh *shell.GitHub, name, value string) {
	gh.VariableSet(name, value)
	log.OKf("  variable: %s", name)
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
	logCloned(cfg.FullRepo, repoDir)

	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "multitier" {
			frontendDir := filepath.Join(cfg.WorkDir, "repo-frontend")
			backendDir := filepath.Join(cfg.WorkDir, "repo-backend")

			gh.ForRepo(cfg.FrontendFullRepo).Clone(frontendDir)
			cfg.FrontendRepoDir = frontendDir
			logCloned(cfg.FrontendFullRepo, frontendDir)

			gh.ForRepo(cfg.BackendFullRepo).Clone(backendDir)
			cfg.BackendRepoDir = backendDir
			logCloned(cfg.BackendFullRepo, backendDir)
		} else {
			systemDir := filepath.Join(cfg.WorkDir, "repo-system")

			gh.ForRepo(cfg.SystemFullRepo).Clone(systemDir)
			cfg.SystemRepoDir = systemDir
			logCloned(cfg.SystemFullRepo, systemDir)
		}
	}
}

// EnsureWorkflowDir creates the .github/workflows directory in a repo.
func EnsureWorkflowDir(repoDir string) {
	os.MkdirAll(filepath.Join(repoDir, ".github", "workflows"), 0755)
}
