// Package steps implements the scaffold pipeline steps.
package steps

import (
	"os"
	"path/filepath"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/kernel/log"
	"github.com/optivem/gh-optivem/internal/kernel/shell"
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
// Repos in cfg.PreExistingRepos (only populated under --overwrite) are reused
// as-is and not created.
func CreateRepos(cfg *config.Config, gh *shell.GitHub) {
	log.Infof("Creating repository %s...", cfg.FullRepo)

	createOrReuseRepo(cfg, gh, cfg.FullRepo)

	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "multitier" {
			createOrReuseRepo(cfg, gh.ForRepo(cfg.FrontendFullRepo), cfg.FrontendFullRepo)
			createOrReuseRepo(cfg, gh.ForRepo(cfg.BackendFullRepo), cfg.BackendFullRepo)
		} else {
			createOrReuseRepo(cfg, gh.ForRepo(cfg.SystemFullRepo), cfg.SystemFullRepo)
		}
	}
}

func createOrReuseRepo(cfg *config.Config, gh *shell.GitHub, fullRepo string) {
	if cfg.PreExistingRepos[fullRepo] {
		log.Successf("Reusing existing repository %s (--overwrite)", fullRepo)
		return
	}
	gh.CreateRepo()
	logCreated(fullRepo)
}

// scaffoldEnvironments are the GitHub environments SetupEnvironments creates.
//
// Scaffolded repos only ever host one arch+lang combination, so the env names
// drop the <arch>-<lang>- prefix used inside optivem/shop and are just the
// bare stage names. The workflow rewrites match: sharedWorkflowRewrites turns
// the shared _*.yml files' "${{ inputs.architecture }}-${{ inputs.language }}-<stage>"
// and "${{ env.PREFIX }}-qa" environments into bare names, and the per-flavor
// content replacements (apply_template.go) do the same for the callers'
// literal "<arch>-<lang>-<stage>" ones. TestScaffoldAgainstShopCheckout checks
// every scaffolded environment reference against this list.
var scaffoldEnvironments = []string{"acceptance", "qa", "production"}

// SetupEnvironments creates GitHub environments on the main repo.
func SetupEnvironments(cfg *config.Config, gh *shell.GitHub) {
	log.Info("Creating environments...")
	for _, stage := range scaffoldEnvironments {
		gh.CreateEnvironment(stage)
		log.Successf("  environment: %s", stage)
	}
}

// SeedSubtypeLabels creates the three `subtype:*` labels the ATDD runtime
// reads from task tickets to dispatch the right structural cycle. Idempotent
// (gh label create --force updates color/description on existing labels).
func SeedSubtypeLabels(cfg *config.Config, gh *shell.GitHub) {
	log.Info("Seeding subtype:* labels...")
	for _, l := range subtypeLabels {
		gh.LabelCreate(l.name, l.color, l.description)
		log.Successf("  label: %s", l.name)
	}
}

var subtypeLabels = []struct {
	name        string
	color       string
	description string
}{
	{"subtype:system-interface-redesign", "1d76db", "Structural change to a system Driver Adapter"},
	{"subtype:external-system-interface-redesign", "0e8a16", "Structural change to an External System Driver Adapter"},
	{"subtype:system-implementation-refactoring", "5319e7", "Refactoring of system internals (no boundary or behavioral change)"},
}

type nameValue struct{ name, value string }

// settingsPlan is the Actions variables and secrets init sets on one repo.
type settingsPlan struct {
	Repo      string
	Variables []nameValue
	Secrets   []nameValue
}

// variableSecretPlan lists, per repo, what SetupVariablesAndSecrets sets. It is
// the single source for both the setup step and the read-only remote probes.
func variableSecretPlan(cfg *config.Config) []settingsPlan {
	plans := []settingsPlan{{
		Repo:      cfg.FullRepo,
		Variables: []nameValue{{"DOCKERHUB_USERNAME", cfg.DockerHubUsername}},
		Secrets: []nameValue{
			{"DOCKERHUB_TOKEN", cfg.DockerHubToken},
			{"SONAR_TOKEN", cfg.SonarToken},
			{"WORKFLOW_TOKEN", cfg.WorkflowToken},
			{"GHCR_TOKEN", cfg.GHCRToken},
			{"REPO_TOKEN", cfg.RepoToken},
		},
	}}
	if cfg.RepoStrategy == "multirepo" {
		var componentRepos []string
		if cfg.Arch == "multitier" {
			componentRepos = []string{cfg.FrontendFullRepo, cfg.BackendFullRepo}
		} else {
			componentRepos = []string{cfg.SystemFullRepo}
		}
		for _, fullRepo := range componentRepos {
			p := settingsPlan{
				Repo:      fullRepo,
				Variables: []nameValue{{"DOCKERHUB_USERNAME", cfg.DockerHubUsername}},
				Secrets: []nameValue{
					{"DOCKERHUB_TOKEN", cfg.DockerHubToken},
					{"SONAR_TOKEN", cfg.SonarToken},
					{"GHCR_TOKEN", cfg.GHCRToken},
				},
			}
			if cfg.Arch == "multitier" && fullRepo == cfg.FrontendFullRepo {
				p.Secrets = append(p.Secrets, nameValue{"REPO_TOKEN", cfg.RepoToken})
			}
			plans = append(plans, p)
		}
	}
	return plans
}

// SetupVariablesAndSecrets sets GitHub Actions variables and secrets. Re-running
// is safe: `gh variable set` and `gh secret set` overwrite an existing value.
func SetupVariablesAndSecrets(cfg *config.Config, gh *shell.GitHub) {
	log.Info("Setting variables and secrets...")

	for i, plan := range variableSecretPlan(cfg) {
		target := gh
		if i > 0 {
			target = gh.ForRepo(plan.Repo)
		}
		for _, v := range plan.Variables {
			setVariable(target, v.name, v.value)
		}
		for _, s := range plan.Secrets {
			setSecret(target, s.name, s.value)
		}
	}
	if cfg.RepoStrategy == "multirepo" {
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
