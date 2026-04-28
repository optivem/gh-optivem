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

// WriteProjectConfig writes .system/config.json to the scaffolded project root(s).
func WriteProjectConfig(cfg *config.Config) {
	log.Info("Writing project config...")

	if cfg.DryRun {
		log.Info("[DRY RUN] Would write .system/config.json")
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

	log.Success("Wrote .system/config.json")
}

func writeConfigToDir(dir string, jsonBytes []byte) {
	configDir := filepath.Join(dir, ".system")
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

	commitAndPushRepo(cfg.RepoDir, cfg.FullRepo, commitMsg, cfg.PreExistingRepos[cfg.FullRepo])

	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "multitier" {
			commitAndPushRepo(cfg.BackendRepoDir, cfg.BackendFullRepo, commitMsg, cfg.PreExistingRepos[cfg.BackendFullRepo])
			commitAndPushRepo(cfg.FrontendRepoDir, cfg.FrontendFullRepo, commitMsg, cfg.PreExistingRepos[cfg.FrontendFullRepo])
		} else {
			commitAndPushRepo(cfg.SystemRepoDir, cfg.SystemFullRepo, commitMsg, cfg.PreExistingRepos[cfg.SystemFullRepo])
		}
	}
}

func commitAndPushRepo(repoDir, fullRepo, commitMsg string, preExisted bool) {
	if _, err := shell.Run("git add -A", false, true, repoDir); err != nil {
		log.Fatalf("git add failed in %s: %v", fullRepo, err)
	}
	// Fix executable permissions for shell scripts (Windows doesn't track the +x
	// bit). Scan every tracked path — scripts like gradlew can live in nested
	// subprojects (e.g. system/gradlew, system-test/gradlew in a monorepo).
	fixExecBits(repoDir, fullRepo)
	status, err := shell.RunCapture("git status --porcelain", repoDir)
	if err != nil {
		log.Fatalf("git status failed in %s: %v", fullRepo, err)
	}
	cleanTree := strings.TrimSpace(status) == ""
	if cleanTree {
		// A clean tree at commit time only makes sense for a re-scaffold of an
		// already-existing repo whose contents already match the template. On a
		// freshly created repo it means the template apply produced nothing,
		// which is a real bug — fail loudly.
		if !preExisted {
			log.Fatalf("git tree is clean in freshly created repo %s -- template apply produced no changes (bug)", fullRepo)
		}
		log.Infof("No changes to commit in %s -- skipping commit (repo already existed with matching content)", fullRepo)
	} else {
		if _, err := shell.Run(fmt.Sprintf(`git commit -m %q`, commitMsg), false, true, repoDir); err != nil {
			log.Fatalf("git commit failed in %s: %v", fullRepo, err)
		}
	}
	if out, err := shell.Run("git push", false, true, repoDir); err != nil {
		log.Fatalf("git push failed in %s: %v\n%s", fullRepo, err, out)
	}
	log.Successf("Pushed template to %s", fullRepo)
}

func fixExecBits(repoDir, fullRepo string) {
	execNames := map[string]bool{
		"gradlew":         true,
		"setup-gcp.sh":    true,
		"teardown-gcp.sh": true,
	}
	out, err := shell.RunCapture("git ls-files", repoDir)
	if err != nil {
		log.Fatalf("git ls-files failed in %s: %v", fullRepo, err)
	}
	for _, path := range strings.Split(out, "\n") {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if execNames[filepath.Base(path)] {
			shell.MustRun(fmt.Sprintf(`git update-index --chmod=+x "%s"`, path), false, repoDir)
		}
	}
}

