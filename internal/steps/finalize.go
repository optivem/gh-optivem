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
	log.Log("Writing project config...")

	if cfg.DryRun {
		log.Log("[DRY RUN] Would write .optivem/config.json")
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

	log.OK("Wrote .optivem/config.json")
}

func writeConfigToDir(dir string, jsonBytes []byte) {
	configDir := filepath.Join(dir, ".optivem")
	os.MkdirAll(configDir, 0755)
	os.WriteFile(filepath.Join(configDir, "config.json"), jsonBytes, 0644)
}

// CreateSonarCloudProjects creates SonarCloud org and projects.
func CreateSonarCloudProjects(cfg *config.Config, sc *shell.SonarCloud) {
	log.Log("Step 9: Creating SonarCloud projects...")

	if cfg.DryRun {
		log.Log("[DRY RUN] Would create SonarCloud org and project(s)")
		return
	}

	sc.CreateOrg()
	for _, key := range GetSonarProjectKeys(cfg) {
		sc.CreateProject(key)
	}
}

// CommitAndPush commits and pushes changes to GitHub.
func CommitAndPush(cfg *config.Config) {
	log.Log("Step 10: Committing and pushing...")

	if cfg.DryRun {
		log.Log("[DRY RUN] Would git add, commit, push")
		return
	}

	commitAndPushRepo(cfg.RepoDir, cfg.FullRepo)

	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "multitier" {
			commitAndPushRepo(cfg.BackendRepoDir, cfg.BackendFullRepo)
			commitAndPushRepo(cfg.FrontendRepoDir, cfg.FrontendFullRepo)
		} else {
			commitAndPushRepo(cfg.SystemRepoDir, cfg.SystemFullRepo)
		}
	}
}

func commitAndPushRepo(repoDir, fullRepo string) {
	if _, err := shell.Run("git add -A", false, true, repoDir); err != nil {
		log.Fatalf("git add failed in %s: %v", fullRepo, err)
	}
	// Fix executable permissions for shell scripts (Windows doesn't track the +x bit)
	for _, script := range []string{"gradlew", "setup-gcp.sh", "teardown-gcp.sh"} {
		if _, err := os.Stat(filepath.Join(repoDir, script)); err == nil {
			shell.MustRun(fmt.Sprintf("git update-index --chmod=+x %s", script), false, repoDir)
		}
	}
	if _, err := shell.Run(`git commit -m "Apply pipeline template"`, false, true, repoDir); err != nil {
		log.Fatalf("git commit failed in %s: %v", fullRepo, err)
	}
	if out, err := shell.Run("git push", false, true, repoDir); err != nil {
		log.Fatalf("git push failed in %s: %v\n%s", fullRepo, err, out)
	}
	log.OKf("Pushed template to %s", fullRepo)
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
