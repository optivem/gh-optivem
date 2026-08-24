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
	"github.com/optivem/gh-optivem/internal/kernel/log"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
	"github.com/optivem/gh-optivem/internal/kernel/shell"
	"github.com/optivem/gh-optivem/internal/scaffolding/assets"
	"github.com/optivem/gh-optivem/internal/scaffolding/files"
)

// WriteLicense writes the LICENSE file into every scaffolded repo and aligns
// the package manifests' license field with it.
//
// The text comes from an embedded asset (internal/scaffolding/assets/licenses),
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

	body, err := assets.RenderLicense(cfg.License, strconv.Itoa(time.Now().Year()), copyrightHolder(cfg))
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
		// Every repo reaching finalize() was freshly created — confirmReposExist
		// (internal/config/config.go) fails the run before this point if any
		// target repo already exists. A clean tree here means the template apply
		// produced nothing, which is a real bug — fail loudly.
		log.Fatalf("git tree is clean in freshly created repo %s -- template apply produced no changes (bug)", fullRepo)
	}
	if _, err := shell.Run(fmt.Sprintf(`git commit -m %q`, commitMsg), true, repoDir); err != nil {
		log.Fatalf("git commit failed in %s: %v", fullRepo, err)
	}
	// shell.MustRunPostCreatePush retries narrowly on the two GitHub ref-store
	// replica-lag messages ("cannot lock ref" / "reference already exists")
	// observed in acceptance run 26456900412 job 77901798586. The earlier
	// assumption that Phase 5's clone + gh api calls had warmed every replica
	// was falsified by that run. The retry is bounded (4 attempts, 5s→15s→45s)
	// and classifier-pinned (TestMustRunPostCreatePush_Classifier in
	// shell/retry_test.go); auth, permission, non-fast-forward, and
	// branch-protection failures still fail fast on the first attempt.
	shell.MustRunPostCreatePush("git push -u origin main", repoDir)
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
