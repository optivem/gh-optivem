package steps

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/log"
	"github.com/optivem/gh-optivem/internal/shell"
)

// VerifyCompilation compiles all source and test components locally to catch
// broken imports, type errors, and case-sensitive path mismatches before pushing.
func VerifyCompilation(cfg *config.Config) {
	log.Log("Verifying local compilation...")

	if cfg.DryRun {
		log.Log("[DRY RUN] Would verify compilation of all components")
		return
	}

	if cfg.Arch == "monolith" {
		compileComponent("system source", cfg.Lang, systemDir(cfg))
	} else {
		compileComponent("backend", cfg.BackendLang, backendDir(cfg))
		compileComponent("frontend", cfg.FrontendLang, frontendDir(cfg))
	}
	compileComponent("system tests", cfg.TestLang, systemTestDir(cfg))
}

func compileComponent(label, lang, dir string) {
	cmds := buildCommands(lang)
	for _, cmd := range cmds {
		if out, err := shell.Run(cmd, false, true, dir); err != nil {
			log.Fatalf("Compilation failed for %s (%s) in %s: %v\n%s", label, lang, dir, err, out)
		}
	}
	log.OKf("Compiled %s (%s)", label, lang)
}

func buildCommands(lang string) []string {
	switch lang {
	case "typescript":
		return []string{"npm ci", "npx tsc --noEmit"}
	case "dotnet":
		return []string{"dotnet build"}
	case "java":
		return []string{"./gradlew compileJava compileTestJava"}
	case "react":
		return []string{"npm ci", "npm run build"}
	default:
		log.Fatalf("Unknown language for compilation: %s", lang)
		return nil
	}
}

func systemDir(cfg *config.Config) string {
	if cfg.RepoStrategy == "multirepo" {
		return cfg.SystemRepoDir
	}
	return filepath.Join(cfg.RepoDir, "system")
}

func backendDir(cfg *config.Config) string {
	if cfg.RepoStrategy == "multirepo" {
		return filepath.Join(cfg.BackendRepoDir, "backend")
	}
	return filepath.Join(cfg.RepoDir, "backend")
}

func frontendDir(cfg *config.Config) string {
	if cfg.RepoStrategy == "multirepo" {
		return filepath.Join(cfg.FrontendRepoDir, "frontend")
	}
	return filepath.Join(cfg.RepoDir, "frontend")
}

func systemTestDir(cfg *config.Config) string {
	return filepath.Join(cfg.RepoDir, "system-test")
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

// RunLocalSmokeTests runs only smoke test suites locally against the scaffolded project
// to verify that Docker builds and basic requests work (test mode only, no CI).
func RunLocalSmokeTests(cfg *config.Config) {
	log.Log("Running local smoke tests...")

	if cfg.DryRun {
		log.Log("[DRY RUN] Would run local smoke tests")
		return
	}

	if !cfg.TestMode {
		return
	}

	testDir := filepath.Join(cfg.RepoDir, "system-test")
	if _, err := os.Stat(filepath.Join(testDir, "Run-SystemTests.ps1")); err != nil {
		log.Warn("Run-SystemTests.ps1 not found in scaffolded project, skipping local smoke tests")
		return
	}

	arch := cfg.Arch

	if cfg.TestLang != "typescript" {
		log.Warn("Skipping local smoke tests: only supported for TypeScript test lang")
		return
	}
	if cfg.RepoStrategy == "multirepo" {
		log.Warn("Skipping local smoke tests: multirepo Docker Compose build contexts reference separate repos not available locally")
		return
	}

	smokeSuites := []struct{ id, label string }{
		{"smoke-stub", "Local smoke tests (stub)"},
		{"smoke-real", "Local smoke tests (real)"},
		{"e2e-ui", "Local E2E tests (UI)"},
	}

	for _, suite := range smokeSuites {
		cmd := fmt.Sprintf("pwsh -NonInteractive -Command ./Run-SystemTests.ps1 -Architecture %s -Suite %s", arch, suite.id)
		log.Logf("Running: %s (in %s)", cmd, testDir)

		output, err := shell.Run(cmd, false, true, testDir)
		if err != nil {
			log.Failf("%s output:\n%s", suite.label, output)
			log.Fatalf("%s failed!", suite.label)
		}
		log.OKf("%s passed!", suite.label)
	}
}
