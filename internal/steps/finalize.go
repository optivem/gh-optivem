package steps

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/log"
	"github.com/optivem/gh-optivem/internal/projectconfig"
	"github.com/optivem/gh-optivem/internal/shell"
)

// WriteLicense writes a LICENSE file to the scaffolded repo(s) using the
// GitHub licenses API. Runs as a regular scaffold step (not part of repo
// initialization) so it works whether the repo was created by gh-optivem or
// pre-created by a wrapper script.
func WriteLicense(cfg *config.Config) {
	log.Info("Writing LICENSE...")

	if cfg.License == "" {
		log.Info("No license configured -- skipping LICENSE file")
		return
	}

	body, err := shell.RunCapture(fmt.Sprintf("gh api licenses/%s --jq .body", cfg.License), "")
	switch {
	case err != nil:
		log.Warnf("Could not fetch license template %q: %v -- skipping LICENSE file", cfg.License, err)
		return
	case body == "":
		log.Warnf("License template %q returned empty body -- skipping LICENSE file", cfg.License)
		return
	}

	writeLicenseToDir(cfg.RepoDir, body)

	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "multitier" {
			writeLicenseToDir(cfg.BackendRepoDir, body)
			writeLicenseToDir(cfg.FrontendRepoDir, body)
		} else {
			writeLicenseToDir(cfg.SystemRepoDir, body)
		}
	}

	log.Success("Wrote LICENSE")
}

func writeLicenseToDir(dir, body string) {
	licensePath := filepath.Join(dir, "LICENSE")
	if err := os.WriteFile(licensePath, []byte(body+"\n"), 0644); err != nil {
		log.Warnf("Could not write LICENSE file at %s: %v -- continuing without LICENSE", licensePath, err)
	}
}

// CreateSonarCloudProjects creates the SonarCloud org and the per-code-tier
// projects named in gh-optivem.yaml. Walks the loaded projectconfig's
// sonar_project fields (system or backend+frontend, plus system_test) so
// the YAML is the single source of truth for which projects exist; an
// operator hand-edit (or future `config refresh`) propagates here for
// free. Now creates 3 projects for monolith (system + system_test) and
// 4 for multitier (backend + frontend + system_test) — up from 1/2 in
// the pre-materialization world that relied on SonarCloud's auto-provision
// of the system-test project on first scan.
func CreateSonarCloudProjects(cfg *config.Config, pc *projectconfig.Config, sc *shell.SonarCloud) {
	log.Info("Creating SonarCloud projects...")

	sc.CreateOrg()
	for _, key := range sonarProjectKeysFromConfig(pc) {
		sc.CreateProject(key)
	}
}

// sonarProjectKeysFromConfig returns the per-code-tier SonarCloud project
// keys carried by pc, in the order CreateSonarCloudProjects creates them:
// system (monolith) or backend+frontend (multitier), then system_test.
// Empty fields are skipped — Validate already guarantees presence when
// architecture is set, so an empty value here means the projectconfig
// itself was partial (architecture unset) and CreateSonarCloudProjects
// wouldn't have been called via the normal `init` flow.
func sonarProjectKeysFromConfig(pc *projectconfig.Config) []string {
	if pc == nil {
		return nil
	}
	var out []string
	if pc.System.SonarProject != "" {
		out = append(out, pc.System.SonarProject)
	}
	if pc.System.Backend.SonarProject != "" {
		out = append(out, pc.System.Backend.SonarProject)
	}
	if pc.System.Frontend.SonarProject != "" {
		out = append(out, pc.System.Frontend.SonarProject)
	}
	if pc.SystemTest.SonarProject != "" {
		out = append(out, pc.SystemTest.SonarProject)
	}
	return out
}

// CommitAndPush commits and pushes changes to GitHub.
//
// failureNote is empty on a clean run. When non-empty, earlier scaffold steps
// failed and this is an intentional partial push for troubleshooting — the
// note is included in the commit message so git history flags it.
func CommitAndPush(cfg *config.Config, failureNote string) {
	log.Info("Committing and pushing...")

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
	if _, err := shell.Run("git add -A", true, repoDir); err != nil {
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
		if _, err := shell.Run(fmt.Sprintf(`git commit -m %q`, commitMsg), true, repoDir); err != nil {
			log.Fatalf("git commit failed in %s: %v", fullRepo, err)
		}
	}
	// `-u origin main` works for both fresh repos (ref-creation push, since
	// CreateRepo no longer pre-pushes a placeholder commit) and existing repos
	// (the upstream is already set; -u just re-applies it). No retry: post-create
	// replica lag is documented at clone time (see MustRunPostCreate) but not
	// at push time -- by Phase 5 the repo has been touched by clone + several
	// gh api calls, so every replica has caught up. Auth, permission, and
	// branch-protection failures are permanent and should fail fast.
	if out, err := shell.Run("git push -u origin main", true, repoDir); err != nil {
		log.Fatalf("git push failed in %s: %v\n%s", fullRepo, err, out)
	}
	log.Successf("Pushed template to %s", fullRepo)
}

func fixExecBits(repoDir, fullRepo string) {
	execNames := map[string]bool{
		"gradlew":         true,
		"setup-gcp.sh":    true,
		"teardown-gcp.sh": true,
		"run-sonar.sh":    true,
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
			shell.MustRun(fmt.Sprintf(`git update-index --chmod=+x "%s"`, path), repoDir)
		}
	}
}

