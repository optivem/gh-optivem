package steps

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/log"
	"github.com/optivem/gh-optivem/internal/shell"
)

// WriteProjectConfig writes .optivem/config.json to the scaffolded project root(s).
func WriteProjectConfig(cfg *config.Config) {
	log.Info("Writing project config...")

	if cfg.DryRun {
		log.Info("[DRY RUN] Would write .optivem/config.json")
		return
	}

	configData := map[string]string{
		"architecture": cfg.Arch,
		"deploy":       cfg.Deploy,
	}
	jsonBytes, _ := json.MarshalIndent(configData, "", "  ")
	jsonBytes = append(jsonBytes, '\n')

	writeConfigToDir(cfg.RepoDir, jsonBytes)

	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "multitier" {
			writeConfigToDir(cfg.BackendRepoDir, jsonBytes)
			writeConfigToDir(cfg.FrontendRepoDir, jsonBytes)
		} else {
			writeConfigToDir(cfg.SystemRepoDir, jsonBytes)
		}
	}

	log.Success("Wrote .optivem/config.json")
}

func writeConfigToDir(dir string, jsonBytes []byte) {
	configDir := filepath.Join(dir, ".optivem")
	os.MkdirAll(configDir, 0755)
	os.WriteFile(filepath.Join(configDir, "config.json"), jsonBytes, 0644)
}

// CreateSonarCloudProjects creates SonarCloud org and projects.
func CreateSonarCloudProjects(cfg *config.Config, sc *shell.SonarCloud) {
	log.Info("Creating SonarCloud projects...")

	if cfg.DryRun {
		log.Info("[DRY RUN] Would create SonarCloud org and project(s)")
		return
	}

	sc.CreateOrg()
	for _, key := range GetSonarProjectKeys(cfg) {
		sc.CreateProject(key)
	}
}

// CommitAndPush commits and pushes changes to GitHub.
//
// failureNote is empty on a clean run. When non-empty, earlier scaffold steps
// failed and this is an intentional partial push for troubleshooting — the
// note is included in the commit message so git history flags it.
func CommitAndPush(cfg *config.Config, failureNote string) {
	log.Info("Committing and pushing...")

	if cfg.DryRun {
		log.Info("[DRY RUN] Would git add, commit, push")
		return
	}

	commitMsg := "Apply pipeline template"
	if failureNote != "" {
		commitMsg = fmt.Sprintf("Apply pipeline template [PARTIAL: scaffold failed at %s]", failureNote)
		log.Warnf("Committing partial scaffold for troubleshooting (failed at %s)", failureNote)
	}

	commitAndPushRepo(cfg.RepoDir, cfg.FullRepo, commitMsg)

	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "multitier" {
			commitAndPushRepo(cfg.BackendRepoDir, cfg.BackendFullRepo, commitMsg)
			commitAndPushRepo(cfg.FrontendRepoDir, cfg.FrontendFullRepo, commitMsg)
		} else {
			commitAndPushRepo(cfg.SystemRepoDir, cfg.SystemFullRepo, commitMsg)
		}
	}
}

func commitAndPushRepo(repoDir, fullRepo, commitMsg string) {
	if _, err := shell.Run("git add -A", false, true, repoDir); err != nil {
		log.Fatalf("git add failed in %s: %v", fullRepo, err)
	}
	// Fix executable permissions for shell scripts (Windows doesn't track the +x bit)
	for _, script := range []string{"gradlew", "setup-gcp.sh", "teardown-gcp.sh"} {
		if _, err := os.Stat(filepath.Join(repoDir, script)); err == nil {
			shell.MustRun(fmt.Sprintf("git update-index --chmod=+x %s", script), false, repoDir)
		}
	}
	if _, err := shell.Run(fmt.Sprintf(`git commit -m %q`, commitMsg), false, true, repoDir); err != nil {
		log.Fatalf("git commit failed in %s: %v", fullRepo, err)
	}
	if out, err := shell.Run("git push", false, true, repoDir); err != nil {
		log.Fatalf("git push failed in %s: %v\n%s", fullRepo, err, out)
	}
	log.Successf("Pushed template to %s", fullRepo)
}

func getRCVersion(gh *shell.GitHub) string {
	shell.CheckRateLimit()

	out, err := shell.RunCapture(
		fmt.Sprintf("gh api repos/%s/releases --jq .[0].tag_name", gh.Repo), "")
	if err == nil && strings.Contains(out, "-rc.") {
		return out
	}

	// Fallback: parse JSON
	out, err = shell.RunCapture(
		fmt.Sprintf("gh api repos/%s/releases", gh.Repo), "")
	if err == nil {
		var releases []struct {
			TagName string `json:"tag_name"`
		}
		if json.Unmarshal([]byte(out), &releases) == nil && len(releases) > 0 {
			if strings.Contains(releases[0].TagName, "-rc.") {
				return releases[0].TagName
			}
		}
	}

	return ""
}
