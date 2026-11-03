package steps

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/kernel/license"
	"github.com/optivem/gh-optivem/internal/kernel/log"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
	"github.com/optivem/gh-optivem/internal/kernel/shell"
	"github.com/optivem/gh-optivem/internal/scaffolding/files"
)

// WriteLicense writes the LICENSE file into every scaffolded repo and aligns
// the package manifests' license field with it.
//
// The text comes from an embedded asset (internal/kernel/license/texts),
// not from the GitHub licenses API. The former fetch-based implementation
// downgraded both a failed call and an empty body to a warning and returned,
// so a network blip, an auth failure or a rate limit produced a repo with no
// LICENSE at all while the scaffold still reported success — which under
// copyright default means all rights reserved, the opposite of what the
// operator asked for. There is no indeterminate outcome left to downgrade:
// the asset is either compiled into the binary or the build is broken, and
// anything that goes wrong from here is fatal.
//
// Runs as a regular scaffold step (not part of repo initialization) so it
// works whether the repo was created by gh-optivem or pre-created by a
// wrapper script.
func WriteLicense(cfg *config.Config) {
	log.Info("Writing LICENSE...")

	if cfg.License == "" {
		log.Info("No license configured -- skipping LICENSE file")
		return
	}

	body, err := license.Render(cfg.License, strconv.Itoa(time.Now().Year()), copyrightHolder(cfg))
	if err != nil {
		log.Fatalf("Cannot write LICENSE for license %q: %v", cfg.License, err)
	}

	for _, dir := range licenseTargetDirs(cfg) {
		writeLicenseToDir(dir, body)
		templateManifestLicense(dir, cfg.License)
	}

	log.Successf("Wrote LICENSE (%s)", projectconfig.LicenseName(cfg.License))
}

// copyrightHolder resolves the name that goes in the LICENSE copyright
// notice: the explicit copyright-holder key when set, else the GitHub owner
// handle. The handle is a serviceable default but not a legal entity
// ("Copyright (c) 2026 optivem"), which is why the explicit key exists.
func copyrightHolder(cfg *config.Config) string {
	if cfg.CopyrightHolder != "" {
		return cfg.CopyrightHolder
	}
	return cfg.Owner
}

// licenseTargetDirs returns every repo dir that receives a LICENSE — the
// root plus any multirepo companions.
func licenseTargetDirs(cfg *config.Config) []string {
	dirs := []string{cfg.RepoDir}
	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "multitier" {
			dirs = append(dirs, cfg.BackendRepoDir, cfg.FrontendRepoDir)
		} else {
			dirs = append(dirs, cfg.SystemRepoDir)
		}
	}
	return dirs
}

func writeLicenseToDir(dir, body string) {
	licensePath := filepath.Join(dir, "LICENSE")
	if err := os.WriteFile(licensePath, []byte(body), 0644); err != nil {
		// Same rule as a missing asset: a LICENSE that could not be written
		// is not a LICENSE the operator chose to skip.
		log.Fatalf("Could not write LICENSE file at %s: %v", licensePath, err)
	}
}

// manifestLicenseLine matches the "license" entry of a package.json. Anchored
// per-line and applied to the first match only: the scaffolded manifests are
// hand-authored and conventionally formatted, so the first "license" line is
// the top-level one.
var manifestLicenseLine = regexp.MustCompile(`(?m)^(\s*"license"\s*:\s*")[^"]*("\s*,?\s*)$`)

// templateManifestLicense rewrites the license field of every package.json in
// the repo to the SPDX id for cfg.License.
//
// Without this the scaffolded project inherits whatever the template repo
// carries — as of shop 6b69c9e6 that is "MIT-0" in
// system/multitier/backend-typescript/package.json and
// system-test/typescript/package.json — so `--license apache-2.0` produced a
// repo whose LICENSE granted Apache-2.0 while npm, SBOM tooling and GitHub's
// own license detection all read MIT-0 from the manifest.
//
// Only manifests that already declare a license are rewritten; one that omits
// the field states nothing and so contradicts nothing. No .NET equivalent is
// applied because no .csproj in the template declares PackageLicenseExpression
// (checked against shop 6b69c9e6) — add the same treatment here if one ever
// does.
func templateManifestLicense(repoDir, licenseKey string) {
	spdx := projectconfig.LicenseSPDXID(licenseKey)
	if spdx == "" {
		// Unreachable via the CLI (applyLicenseAndDeployDefaults rejects an
		// unknown key first) but fatal rather than silent if it ever is: an
		// empty license field reads as "unlicensed" to npm.
		log.Fatalf("No SPDX identifier for license key %q -- cannot template package manifests", licenseKey)
	}

	count := 0
	err := filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if isVendoredDir(path, info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Name() != "package.json" {
			return nil
		}
		if rewriteManifestLicense(path, spdx) {
			count++
		}
		return nil
	})
	if err != nil {
		log.Fatalf("Could not scan %s for package manifests: %v", repoDir, err)
	}
	if count > 0 {
		log.Successf("License: package.json license -> %s (%d files)", spdx, count)
	}
}

// isVendoredDir reports whether a directory holds third-party or build-output
// package.json files, which carry their dependencies' licenses and must not be
// rewritten (shop's system-test/dotnet/**/bin/Debug/**/.playwright/package/
// package.json declares Apache-2.0 and belongs to Playwright, not to us).
func isVendoredDir(path, name string) bool {
	if files.IsGitDir(path) {
		return true
	}
	switch name {
	case "node_modules", "bin", "obj", "dist", "build", "target", "out", "coverage":
		return true
	}
	return false
}

// rewriteManifestLicense sets the first "license" entry in the package.json at
// path to spdx, reporting whether the file changed.
func rewriteManifestLicense(path, spdx string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("Could not read package manifest %s: %v", path, err)
	}
	replaced := false
	out := manifestLicenseLine.ReplaceAllStringFunc(string(b), func(line string) string {
		if replaced {
			return line
		}
		replaced = true
		m := manifestLicenseLine.FindStringSubmatch(line)
		return m[1] + spdx + m[2]
	})
	if !replaced || out == string(b) {
		return false
	}
	if err := os.WriteFile(path, []byte(out), 0644); err != nil {
		log.Fatalf("Could not write package manifest %s: %v", path, err)
	}
	return true
}

// CreateSonarCloudProjects creates the SonarCloud org and the per-code-tier
// projects named in gh-optivem.yaml. Walks the loaded projectconfig's
// sonar-project fields (system or backend+frontend, plus system-test) so
// the YAML is the single source of truth for which projects exist; an
// operator hand-edit (or future `config refresh`) propagates here for
// free. Now creates 3 projects for monolith (system + system-test) and
// 4 for multitier (backend + frontend + system-test) — up from 1/2 in
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
// system (monolith) or backend+frontend (multitier), then system-test.
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

	commitAndPushRepo(cfg, cfg.RepoDir, cfg.FullRepo, commitMsg)

	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "multitier" {
			commitAndPushRepo(cfg, cfg.BackendRepoDir, cfg.BackendFullRepo, commitMsg)
			commitAndPushRepo(cfg, cfg.FrontendRepoDir, cfg.FrontendFullRepo, commitMsg)
		} else {
			commitAndPushRepo(cfg, cfg.SystemRepoDir, cfg.SystemFullRepo, commitMsg)
		}
	}
}

func commitAndPushRepo(cfg *config.Config, repoDir, fullRepo, commitMsg string) {
	// Skip cleanly when the local clone never landed (earlier scaffold step
	// failed before clone). Without this guard, `git add -A` cd's into a
	// non-existent dir and surfaces a misleading "chdir ... no such file or
	// directory" panic that drowns out the actual upstream failure — see the
	// acceptance run 25877369208 "Commit and push" noise.
	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		log.Infof("Skipping commit/push for %s: local dir %s not created (earlier step failed before clone)",
			fullRepo, repoDir)
		return
	}
	pushed, err := stageCommitAndPush(repoDir, fullRepo, commitMsg, cfg.PreExistingRepos[fullRepo])
	if err != nil {
		log.Fatalf("%v", err)
	}
	if !pushed {
		log.Successf("Nothing to change in %s: the scaffold matches what is already there", fullRepo)
		return
	}
	log.Successf("Pushed template to %s", fullRepo)
}

// stageCommitAndPush stages everything in repoDir, commits and pushes to main.
// It returns pushed=false (and no error) only for a clean tree in a repo that
// already existed before this run: re-running on an already-adopted repo has
// nothing to change. For a repo created by this run a clean tree means the
// template apply produced nothing, which is a bug and returns an error.
func stageCommitAndPush(repoDir, fullRepo, commitMsg string, preExisting bool) (pushed bool, err error) {
	if _, err := shell.Run("git add -A", true, repoDir); err != nil {
		return false, fmt.Errorf("git add failed in %s: %w", fullRepo, err)
	}
	// Fix executable permissions for shell scripts (Windows doesn't track the +x
	// bit). Scan every tracked path — scripts like gradlew can live in nested
	// subprojects (e.g. system/gradlew, system-test/gradlew in a monorepo).
	if err := fixExecBits(repoDir, fullRepo); err != nil {
		return false, err
	}
	status, err := shell.RunCapture("git status --porcelain", repoDir)
	if err != nil {
		return false, fmt.Errorf("git status failed in %s: %w", fullRepo, err)
	}
	if strings.TrimSpace(status) == "" {
		if preExisting {
			return false, nil
		}
		return false, fmt.Errorf("git tree is clean in newly created repo %s -- template apply produced no changes (bug)", fullRepo)
	}
	if _, err := shell.Run(fmt.Sprintf(`git commit -m %q`, commitMsg), true, repoDir); err != nil {
		return false, fmt.Errorf("git commit failed in %s: %w", fullRepo, err)
	}
	// shell.MustRunPostCreatePush retries narrowly on the two GitHub ref-store
	// replica-lag messages ("cannot lock ref" / "reference already exists")
	// observed in acceptance run 26456900412 job 77901798586. The earlier
	// assumption that Phase 5's clone + gh api calls had warmed every replica
	// was falsified by that run. The retry is bounded (4 attempts, 5s→15s→45s)
	// and classifier-pinned (TestMustRunPostCreatePush_Classifier in
	// shell/retry_test.go); auth, permission, non-fast-forward, and
	// branch-protection failures still fail fast on the first attempt. The
	// same push serves repos that already existed (--overwrite): it is a
	// direct push to the default branch, and the retry stays narrow.
	shell.MustRunPostCreatePush("git push -u origin main", repoDir)
	return true, nil
}

func fixExecBits(repoDir, fullRepo string) error {
	execNames := map[string]bool{
		"gradlew":         true,
		"setup-gcp.sh":    true,
		"teardown-gcp.sh": true,
		"run-sonar.sh":    true,
	}
	out, err := shell.RunCapture("git ls-files", repoDir)
	if err != nil {
		return fmt.Errorf("git ls-files failed in %s: %w", fullRepo, err)
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
	return nil
}
