package steps

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/log"
	"github.com/optivem/gh-optivem/internal/shell"
)

// GetSonarProjectKeys returns the SonarCloud project keys for the given config.
func GetSonarProjectKeys(cfg *config.Config) []string {
	if cfg.Arch == "monolith" {
		if cfg.RepoStrategy == "monorepo" {
			return []string{cfg.Owner + "_" + cfg.Repo + "-system"}
		}
		return []string{cfg.Owner + "_" + cfg.SystemRepo + "-system"}
	}
	if cfg.RepoStrategy == "monorepo" {
		prefix := cfg.Owner + "_" + cfg.Repo
		return []string{
			prefix + "-backend",
			prefix + "-frontend",
		}
	}
	return []string{
		cfg.Owner + "_" + cfg.BackendRepo + "-backend",
		cfg.Owner + "_" + cfg.FrontendRepo + "-frontend",
	}
}

// UpdateReadme generates README.md for the repo(s).
func UpdateReadme(cfg *config.Config) {
	log.Log("Step 8: Generating README...")

	if cfg.DryRun {
		log.Log("[DRY RUN] Would generate README.md")
		return
	}

	if cfg.Arch == "monolith" {
		if cfg.RepoStrategy == "monorepo" {
			badges := generateBadges(cfg)
			writeReadme(cfg.RepoDir, cfg.SystemName, badges, cfg)
		} else {
			writeMonolithMultirepoReadme(cfg)
		}
	} else if cfg.RepoStrategy == "monorepo" {
		badges := generateBadges(cfg)
		writeReadme(cfg.RepoDir, cfg.SystemName, badges, cfg)
	} else {
		writeMultitierMultirepoReadme(cfg)
	}

	log.OK("Generated README.md")
}

func writeReadme(repoDir, title, badges string, cfg *config.Config) {
	var info strings.Builder
	fmt.Fprintf(&info, "## Project Info\n\n")
	fmt.Fprintf(&info, "- **Owner:** %s\n", cfg.Owner)
	fmt.Fprintf(&info, "- **System:** %s\n", cfg.SystemName)
	fmt.Fprintf(&info, "- **Architecture:** %s\n", cfg.Arch)
	fmt.Fprintf(&info, "- **Repo strategy:** %s\n", cfg.RepoStrategy)
	if cfg.Arch == "monolith" {
		fmt.Fprintf(&info, "- **Language:** %s\n", cfg.Lang)
	} else {
		fmt.Fprintf(&info, "- **Backend language:** %s\n", cfg.BackendLang)
		fmt.Fprintf(&info, "- **Frontend language:** %s\n", cfg.FrontendLang)
	}
	if cfg.TestLang != cfg.EffectiveLang() {
		fmt.Fprintf(&info, "- **Test language:** %s\n", cfg.TestLang)
	}
	fmt.Fprintf(&info, "\n")

	content := fmt.Sprintf("# %s\n\n%s\n%s## License\n\n%s\n\n## Contributors\n\n- [%s](https://github.com/%s)\n",
		title, badges, info.String(), cfg.LicenseName(), cfg.Owner, cfg.Owner)
	os.WriteFile(filepath.Join(repoDir, "README.md"), []byte(content), 0644)
}

func writeMonolithMultirepoReadme(cfg *config.Config) {
	base := "https://github.com/" + cfg.FullRepo + "/actions/workflows"
	systemBase := "https://github.com/" + cfg.SystemFullRepo + "/actions/workflows"

	badgeItems := [][2]string{
		{systemBase + "/commit-stage.yml", "commit-stage"},
		{base + "/acceptance-stage.yml", "acceptance-stage"},
		{base + "/qa-stage.yml", "qa-stage"},
		{base + "/qa-signoff.yml", "qa-signoff"},
		{base + "/prod-stage.yml", "prod-stage"},
	}

	var badges strings.Builder
	for _, item := range badgeItems {
		fmt.Fprintf(&badges, "[![%s](%s/badge.svg)](%s)\n", item[1], item[0], item[0])
	}

	reposSection := fmt.Sprintf("## Repositories\n\n- [%s](https://github.com/%s) — System (%s)\n",
		cfg.SystemRepo, cfg.SystemFullRepo, cfg.Lang)

	var info strings.Builder
	fmt.Fprintf(&info, "## Project Info\n\n")
	fmt.Fprintf(&info, "- **Owner:** %s\n", cfg.Owner)
	fmt.Fprintf(&info, "- **System:** %s\n", cfg.SystemName)
	fmt.Fprintf(&info, "- **Architecture:** monolith\n")
	fmt.Fprintf(&info, "- **Repo strategy:** multirepo\n")
	fmt.Fprintf(&info, "- **Language:** %s\n", cfg.Lang)
	if cfg.TestLang != cfg.Lang {
		fmt.Fprintf(&info, "- **Test language:** %s\n", cfg.TestLang)
	}
	fmt.Fprintf(&info, "\n")

	content := fmt.Sprintf("# %s\n\n%s\n%s%s\n## License\n\n%s\n\n## Contributors\n\n- [%s](https://github.com/%s)\n",
		cfg.SystemName, badges.String(), reposSection, info.String(), cfg.LicenseName(), cfg.Owner, cfg.Owner)
	os.WriteFile(filepath.Join(cfg.RepoDir, "README.md"), []byte(content), 0644)

	// System repo README
	writeComponentReadme(
		cfg.SystemRepoDir, cfg.SystemName, "System",
		cfg.SystemFullRepo, cfg.Lang, cfg.LicenseName(), cfg.Owner,
	)
}

func writeMultitierMultirepoReadme(cfg *config.Config) {
	bl, fl := cfg.BackendLang, cfg.FrontendLang
	base := "https://github.com/" + cfg.FullRepo + "/actions/workflows"
	backendBase := "https://github.com/" + cfg.BackendFullRepo + "/actions/workflows"
	frontendBase := "https://github.com/" + cfg.FrontendFullRepo + "/actions/workflows"

	badgeItems := [][2]string{
		{backendBase + "/backend-commit-stage.yml", "backend-commit-stage"},
		{frontendBase + "/frontend-commit-stage.yml", "frontend-commit-stage"},
		{base + "/acceptance-stage.yml", "acceptance-stage"},
		{base + "/qa-stage.yml", "qa-stage"},
		{base + "/qa-signoff.yml", "qa-signoff"},
		{base + "/prod-stage.yml", "prod-stage"},
	}

	var badges strings.Builder
	for _, item := range badgeItems {
		fmt.Fprintf(&badges, "[![%s](%s/badge.svg)](%s)\n", item[1], item[0], item[0])
	}

	reposSection := fmt.Sprintf("## Repositories\n\n- [%s](https://github.com/%s) — Backend (%s)\n- [%s](https://github.com/%s) — Frontend (%s)\n",
		cfg.BackendRepo, cfg.BackendFullRepo, bl,
		cfg.FrontendRepo, cfg.FrontendFullRepo, fl)

	var info strings.Builder
	fmt.Fprintf(&info, "## Project Info\n\n")
	fmt.Fprintf(&info, "- **Owner:** %s\n", cfg.Owner)
	fmt.Fprintf(&info, "- **System:** %s\n", cfg.SystemName)
	fmt.Fprintf(&info, "- **Architecture:** multitier\n")
	fmt.Fprintf(&info, "- **Repo strategy:** multirepo\n")
	fmt.Fprintf(&info, "- **Backend language:** %s\n", cfg.BackendLang)
	fmt.Fprintf(&info, "- **Frontend language:** %s\n", cfg.FrontendLang)
	if cfg.TestLang != cfg.BackendLang {
		fmt.Fprintf(&info, "- **Test language:** %s\n", cfg.TestLang)
	}
	fmt.Fprintf(&info, "\n")

	content := fmt.Sprintf("# %s\n\n%s\n%s%s\n## License\n\n%s\n\n## Contributors\n\n- [%s](https://github.com/%s)\n",
		cfg.SystemName, badges.String(), reposSection, info.String(), cfg.LicenseName(), cfg.Owner, cfg.Owner)
	os.WriteFile(filepath.Join(cfg.RepoDir, "README.md"), []byte(content), 0644)

	writeComponentReadme(
		cfg.BackendRepoDir, cfg.SystemName, "Backend",
		cfg.BackendFullRepo, bl, cfg.LicenseName(), cfg.Owner,
	)
	writeComponentReadme(
		cfg.FrontendRepoDir, cfg.SystemName, "Frontend",
		cfg.FrontendFullRepo, fl, cfg.LicenseName(), cfg.Owner,
	)
}

func writeComponentReadme(repoDir, systemName, componentLabel, fullRepo, lang, licenseName, owner string) {
	wfName := strings.ToLower(componentLabel) + "-commit-stage.yml"
	if componentLabel == "System" {
		wfName = "commit-stage.yml"
	}
	base := "https://github.com/" + fullRepo + "/actions/workflows"
	badges := fmt.Sprintf("[![commit-stage](%s/%s/badge.svg)](%s/%s)\n", base, wfName, base, wfName)

	content := fmt.Sprintf("# %s — %s\n\n%s\n## License\n\n%s\n\n## Contributors\n\n- [%s](https://github.com/%s)\n",
		systemName, componentLabel, badges, licenseName, owner, owner)
	os.WriteFile(filepath.Join(repoDir, "README.md"), []byte(content), 0644)
}

func generateBadges(cfg *config.Config) string {
	base := "https://github.com/" + cfg.FullRepo + "/actions/workflows"

	var items [][2]string
	if cfg.Arch == "monolith" {
		items = [][2]string{
			{"commit-stage.yml", "commit-stage"},
			{"acceptance-stage.yml", "acceptance-stage"},
			{"qa-stage.yml", "qa-stage"},
			{"qa-signoff.yml", "qa-signoff"},
			{"prod-stage.yml", "prod-stage"},
		}
	} else {
		items = [][2]string{
			{"backend-commit-stage.yml", "backend-commit-stage"},
			{"frontend-commit-stage.yml", "frontend-commit-stage"},
			{"acceptance-stage.yml", "acceptance-stage"},
			{"qa-stage.yml", "qa-stage"},
			{"qa-signoff.yml", "qa-signoff"},
			{"prod-stage.yml", "prod-stage"},
		}
	}

	var b strings.Builder
	for _, item := range items {
		fmt.Fprintf(&b, "[![%s](%s/%s/badge.svg)](%s/%s)\n", item[1], base, item[0], base, item[0])
	}
	return b.String()
}

// WriteProjectConfig writes .optivem/config.json to the scaffolded project root(s).
func WriteProjectConfig(cfg *config.Config) {
	log.Log("Writing project config...")

	if cfg.DryRun {
		log.Log("[DRY RUN] Would write .optivem/config.json")
		return
	}

	configData := map[string]string{
		"architecture": cfg.Arch,
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
	if _, err := shell.Run(`git commit -m "Apply pipeline template"`, false, true, repoDir); err != nil {
		log.Fatalf("git commit failed in %s: %v", fullRepo, err)
	}
	if out, err := shell.Run("git push", false, true, repoDir); err != nil {
		log.Fatalf("git push failed in %s: %v\n%s", fullRepo, err, out)
	}
	log.OKf("Pushed template to %s", fullRepo)
}

// EnablePages enables GitHub Pages on the main repo (source: main branch, /docs path).
func EnablePages(cfg *config.Config, gh *shell.GitHub) {
	log.Log("Step 11: Enabling GitHub Pages...")

	if cfg.DryRun {
		log.Logf("[DRY RUN] Would enable GitHub Pages on %s", cfg.FullRepo)
		return
	}

	gh.EnablePages()
	log.OKf("Enabled GitHub Pages on %s", cfg.FullRepo)
}

// VerifyCommitStage waits for commit stage workflow to pass.
func VerifyCommitStage(cfg *config.Config, gh *shell.GitHub) {
	log.Log("Step 11: Verifying commit stage workflow...")

	if cfg.DryRun {
		log.Log("[DRY RUN] Would wait for commit stage workflow")
		return
	}

	time.Sleep(5 * time.Second)

	if cfg.Arch == "monolith" {
		if cfg.RepoStrategy == "monorepo" {
			verifyNamedWorkflow(gh, "Commit stage", "commit-stage.yml", 60)
		} else {
			ghSystem := gh.ForRepo(cfg.SystemFullRepo)
			verifyNamedWorkflow(ghSystem, "Commit stage", "commit-stage.yml", 60)
		}
	} else if cfg.RepoStrategy == "monorepo" {
		verifyNamedWorkflow(gh, "Backend commit stage", "backend-commit-stage.yml", 60)
		verifyNamedWorkflow(gh, "Frontend commit stage", "frontend-commit-stage.yml", 60)
	} else {
		ghBackend := gh.ForRepo(cfg.BackendFullRepo)
		ghFrontend := gh.ForRepo(cfg.FrontendFullRepo)
		verifyNamedWorkflow(ghBackend, "Backend commit stage", "backend-commit-stage.yml", 60)
		verifyNamedWorkflow(ghFrontend, "Frontend commit stage", "frontend-commit-stage.yml", 60)
	}
}

// VerifyAcceptanceStage triggers and verifies acceptance stage.
func VerifyAcceptanceStage(cfg *config.Config, gh *shell.GitHub) {
	log.Log("Step 12: Triggering and verifying acceptance stage...")

	if cfg.DryRun {
		log.Log("[DRY RUN] Would trigger and wait for acceptance stage workflow")
		return
	}

	verifyWorkflow(gh, "Acceptance stage", "acceptance-stage.yml", nil, 300)

	rcVersion := getRCVersion(gh)
	if rcVersion != "" {
		cfg.RCVersion = rcVersion
		log.OKf("RC version: %s", rcVersion)
	} else {
		log.Warn("No RC version found — acceptance stage may have skipped promotion (e.g. no artifacts yet). Downstream stages will be skipped.")
	}
}

// VerifyAcceptanceStageLegacy triggers and verifies acceptance stage legacy.
func VerifyAcceptanceStageLegacy(cfg *config.Config, gh *shell.GitHub) {
	log.Log("Step 13: Triggering and verifying acceptance stage legacy...")

	if cfg.DryRun {
		log.Log("[DRY RUN] Would trigger and wait for acceptance stage legacy workflow")
		return
	}

	verifyWorkflow(gh, "Acceptance stage legacy", "acceptance-stage-legacy.yml", nil, 300)
}

// VerifyQAStage triggers and verifies QA stage.
func VerifyQAStage(cfg *config.Config, gh *shell.GitHub) {
	log.Log("Step 14: Triggering and verifying QA stage...")

	if cfg.DryRun {
		log.Log("[DRY RUN] Would trigger and wait for QA stage workflow")
		return
	}

	if cfg.RCVersion == "" {
		log.Warn("Skipping QA stage — no RC version available")
		return
	}

	verifyWorkflow(gh, "QA stage", "qa-stage.yml", map[string]string{"version": cfg.RCVersion}, 300)
}

// VerifyQASignoff triggers and verifies QA signoff.
func VerifyQASignoff(cfg *config.Config, gh *shell.GitHub) {
	log.Log("Step 15: Triggering and verifying QA signoff...")

	if cfg.DryRun {
		log.Log("[DRY RUN] Would trigger and wait for QA signoff workflow")
		return
	}

	if cfg.RCVersion == "" {
		log.Warn("Skipping QA signoff — no RC version available")
		return
	}

	verifyWorkflow(gh, "QA signoff", "qa-signoff.yml", map[string]string{"version": cfg.RCVersion, "result": "approved"}, 300)
}

// VerifyProdStage triggers and verifies production stage.
func VerifyProdStage(cfg *config.Config, gh *shell.GitHub) {
	log.Log("Step 16: Triggering and verifying production stage...")

	if cfg.DryRun {
		log.Log("[DRY RUN] Would trigger and wait for production stage workflow")
		return
	}

	if cfg.RCVersion == "" {
		log.Warn("Skipping production stage — no RC version available")
		return
	}

	verifyWorkflow(gh, "Production stage", "prod-stage.yml", map[string]string{"version": cfg.RCVersion}, 300)
}

func verifyNamedWorkflow(gh *shell.GitHub, label, workflowFile string, intervalSecs int) {
	shell.CheckRateLimit()
	err := gh.RunWatchWorkflow(workflowFile, intervalSecs)
	if err != nil {
		var rle *shell.RateLimitExceeded
		if errors.As(err, &rle) {
			log.Failf("%s failed due to GitHub API rate limiting (the workflow itself may have succeeded)!", label)
			log.Fatalf("Rate limit exceeded while watching %s workflow. The workflow run may still be passing — check manually: https://github.com/%s/actions", label, gh.Repo)
		}
		log.Failf("%s failed!", label)
		log.Fatalf("%s workflow failed. Check: https://github.com/%s/actions", label, gh.Repo)
	}
	log.OKf("%s passed!", label)
}

func verifyWorkflow(gh *shell.GitHub, label, triggerWorkflow string, fields map[string]string, intervalSecs int) {
	shell.CheckRateLimit()
	if triggerWorkflow != "" {
		gh.WorkflowRun(triggerWorkflow, fields)
		time.Sleep(5 * time.Second)
	}

	shell.CheckRateLimit()
	err := gh.RunWatch(intervalSecs)
	if err != nil {
		var rle *shell.RateLimitExceeded
		if errors.As(err, &rle) {
			log.Failf("%s failed due to GitHub API rate limiting (the workflow itself may have succeeded)!", label)
			log.Fatalf("Rate limit exceeded while watching %s workflow. The workflow run may still be passing — check manually: https://github.com/%s/actions", label, gh.Repo)
		}
		log.Failf("%s failed!", label)
		log.Fatalf("%s workflow failed. Check: https://github.com/%s/actions", label, gh.Repo)
	}
	log.OKf("%s passed!", label)
}

// RunLocalSystemTests runs Run-SystemTests.ps1 locally against the scaffolded project
// to verify that Docker builds and test suites work (test mode only).
func RunLocalSystemTests(cfg *config.Config) {
	log.Log("Step 17: Running local system tests...")

	if cfg.DryRun {
		log.Log("[DRY RUN] Would run local system tests")
		return
	}

	if !cfg.TestMode {
		return
	}

	testDir := filepath.Join(cfg.RepoDir, "system-test")
	if _, err := os.Stat(filepath.Join(testDir, "Run-SystemTests.ps1")); err != nil {
		log.Warn("Run-SystemTests.ps1 not found in scaffolded project, skipping local system tests")
		return
	}

	arch := cfg.Arch

	// Local system tests are only supported for TypeScript test lang + monorepo.
	// TODO: Support multirepo by cloning the backend/frontend repos into sibling
	// directories so that Docker Compose build contexts (e.g. ../backend) resolve
	// correctly. Currently skipped because the root repo alone doesn't have them.
	if cfg.TestLang != "typescript" {
		log.Warn("Skipping local system tests: only supported for TypeScript test lang")
		return
	}
	if cfg.RepoStrategy == "multirepo" {
		log.Warn("Skipping local system tests: multirepo Docker Compose build contexts reference separate repos not available locally")
		return
	}

	// Run latest tests
	latestLabel := "Local system tests (latest)"
	latestCmd := fmt.Sprintf("pwsh -NonInteractive -Command ./Run-SystemTests.ps1 -Architecture %s", arch)
	log.Logf("Running: %s (in %s)", latestCmd, testDir)

	output, err := shell.Run(latestCmd, false, true, testDir)
	if err != nil {
		log.Failf("%s output:\n%s", latestLabel, output)
		log.Fatalf("%s failed!", latestLabel)
	}
	log.OKf("%s passed!", latestLabel)

	// Run legacy tests if not excluded
	if !cfg.ExcludeLegacy {
		legacyLabel := "Local system tests (legacy)"
		legacyCmd := fmt.Sprintf("pwsh -NonInteractive -Command ./Run-SystemTests.ps1 -Architecture %s -Legacy", arch)
		log.Logf("Running: %s (in %s)", legacyCmd, testDir)

		output, err = shell.Run(legacyCmd, false, true, testDir)
		if err != nil {
			log.Failf("%s output:\n%s", legacyLabel, output)
			log.Fatalf("%s failed!", legacyLabel)
		}
		log.OKf("%s passed!", legacyLabel)
	}
}

// Cleanup deletes repos, SonarCloud projects, and local directories (test mode only).
func Cleanup(cfg *config.Config, gh *shell.GitHub, sc *shell.SonarCloud) {
	if !cfg.TestMode {
		return
	}

	shouldCleanup := cfg.Cleanup
	if shouldCleanup == "ask" {
		fmt.Printf("\nDelete test repository %s? [y/N] ", cfg.FullRepo)
		var answer string
		fmt.Scanln(&answer)
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer == "y" || answer == "yes" {
			shouldCleanup = "yes"
		} else {
			shouldCleanup = "no"
		}
	}

	if shouldCleanup == "yes" {
		log.Logf("Cleaning up: deleting %s...", cfg.FullRepo)
		gh.Delete()
		log.OKf("Deleted repository %s", cfg.FullRepo)

		if cfg.RepoStrategy == "multirepo" {
			if cfg.Arch == "multitier" {
				ghFrontend := gh.ForRepo(cfg.FrontendFullRepo)
				ghBackend := gh.ForRepo(cfg.BackendFullRepo)
				ghBackend.Delete()
				log.OKf("Deleted repository %s", cfg.BackendFullRepo)
				ghFrontend.Delete()
				log.OKf("Deleted repository %s", cfg.FrontendFullRepo)
			} else {
				ghSystem := gh.ForRepo(cfg.SystemFullRepo)
				ghSystem.Delete()
				log.OKf("Deleted repository %s", cfg.SystemFullRepo)
			}
		}

		for _, key := range GetSonarProjectKeys(cfg) {
			sc.DeleteProject(key)
		}

		if cfg.RepoDir != "" {
			os.RemoveAll(cfg.RepoDir)
		}
		if cfg.FrontendRepoDir != "" {
			os.RemoveAll(cfg.FrontendRepoDir)
		}
		if cfg.BackendRepoDir != "" {
			os.RemoveAll(cfg.BackendRepoDir)
		}
		if cfg.SystemRepoDir != "" {
			os.RemoveAll(cfg.SystemRepoDir)
		}

		log.OK("Cleanup complete")
	} else {
		log.Logf("Keeping repository: https://github.com/%s", cfg.FullRepo)
		if cfg.RepoStrategy == "multirepo" {
			if cfg.Arch == "multitier" {
				log.Logf("Keeping repository: https://github.com/%s", cfg.FrontendFullRepo)
				log.Logf("Keeping repository: https://github.com/%s", cfg.BackendFullRepo)
			} else {
				log.Logf("Keeping repository: https://github.com/%s", cfg.SystemFullRepo)
			}
		}
	}
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
