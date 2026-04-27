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
	log.Successf("Created repository: %s", fullRepo)
	log.Successf("  https://github.com/%s", fullRepo)
}

func logCloned(fullRepo, localPath string) {
	log.Successf("Cloned %s", fullRepo)
	log.Successf("  -> %s", localPath)
}

// CreateRepos creates the GitHub repository (and component repos for multitier).
func CreateRepos(cfg *config.Config, gh *shell.GitHub) {
	log.Infof("Creating repository %s...", cfg.FullRepo)

	if cfg.DryRun {
		log.Infof("[DRY RUN] gh repo create %s --public", cfg.FullRepo)
		if cfg.RepoStrategy == "multirepo" {
			log.Infof("[DRY RUN] gh repo create %s --public", cfg.FrontendFullRepo)
			log.Infof("[DRY RUN] gh repo create %s --public", cfg.BackendFullRepo)
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
//
// Scaffolded repos only ever host one arch+lang combination, so the env names
// drop the arch+lang prefix used inside optivem/shop and are just the bare
// stage names. The workflow content rewrite in apply_template.go rewrites the
// template's prefixed "environment:" references to match.
func SetupEnvironments(cfg *config.Config, gh *shell.GitHub) {
	log.Info("Creating environments...")
	for _, stage := range []string{"acceptance", "qa", "production"} {
		gh.CreateEnvironment(stage)
		log.Successf("  environment: %s", stage)
	}
}

// SetupVariablesAndSecrets sets GitHub Actions variables and secrets.
func SetupVariablesAndSecrets(cfg *config.Config, gh *shell.GitHub) {
	log.Info("Setting variables and secrets...")

	setVariable(gh, "DOCKERHUB_USERNAME", cfg.DockerHubUsername)
	setSecret(gh, "DOCKERHUB_TOKEN", cfg.DockerHubToken)
	setSecret(gh, "SONAR_TOKEN", cfg.SonarToken)
	setSecret(gh, "WORKFLOW_TOKEN", cfg.WorkflowToken)
	setSecret(gh, "GHCR_TOKEN", cfg.GHCRToken)
	setSecret(gh, "REPO_TOKEN", cfg.RepoToken)

	if cfg.RepoStrategy == "multirepo" {
		var componentRepos []string
		if cfg.Arch == "multitier" {
			componentRepos = []string{cfg.FrontendFullRepo, cfg.BackendFullRepo}
		} else {
			componentRepos = []string{cfg.SystemFullRepo}
		}
		for _, fullRepo := range componentRepos {
			ghComp := gh.ForRepo(fullRepo)
			setVariable(ghComp, "DOCKERHUB_USERNAME", cfg.DockerHubUsername)
			setSecret(ghComp, "DOCKERHUB_TOKEN", cfg.DockerHubToken)
			setSecret(ghComp, "SONAR_TOKEN", cfg.SonarToken)
			setSecret(ghComp, "GHCR_TOKEN", cfg.GHCRToken)
		}
		log.Success("Set variables and secrets on component repositories")
	}

	log.Success("Set variables and secrets")
}

func setSecret(gh *shell.GitHub, name, value string) {
	gh.SecretSet(name, value)
	log.Successf("  secret: %s", name)
}

func setVariable(gh *shell.GitHub, name, value string) {
	gh.VariableSet(name, value)
	log.Successf("  variable: %s", name)
}

// CloneRepos clones the repository (and component repos for multitier) into
// the destination dirs pre-computed during ParseAndValidate (cfg.RepoDir etc).
func CloneRepos(cfg *config.Config, gh *shell.GitHub) {
	log.Info("Cloning repo(s)...")

	if cfg.DryRun {
		log.Info("[DRY RUN] Would clone repo(s)")
		return
	}

	gh.Clone(cfg.RepoDir)
	logCloned(cfg.FullRepo, cfg.RepoDir)

	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "multitier" {
			gh.ForRepo(cfg.FrontendFullRepo).Clone(cfg.FrontendRepoDir)
			logCloned(cfg.FrontendFullRepo, cfg.FrontendRepoDir)

			gh.ForRepo(cfg.BackendFullRepo).Clone(cfg.BackendRepoDir)
			logCloned(cfg.BackendFullRepo, cfg.BackendRepoDir)
		} else {
			gh.ForRepo(cfg.SystemFullRepo).Clone(cfg.SystemRepoDir)
			logCloned(cfg.SystemFullRepo, cfg.SystemRepoDir)
		}
	}
}

// EnsureWorkflowDir creates the .github/workflows directory in a repo.
func EnsureWorkflowDir(repoDir string) {
	os.MkdirAll(filepath.Join(repoDir, ".github", "workflows"), 0755)
}
