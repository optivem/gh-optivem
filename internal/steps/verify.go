package steps

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/log"
	"github.com/optivem/gh-optivem/internal/runner"
	"github.com/optivem/gh-optivem/internal/shell"
)

const (
	systemTestDir_name = "system-test"
	dockerDir_name     = "docker"

	msgStagePassed = "%s passed!"
	msgStageFailed = "%s failed!"
)

// VerifyCompilation compiles all source and test components locally to catch
// broken imports, type errors, and case-sensitive path mismatches before pushing.
func VerifyCompilation(cfg *config.Config) {
	log.Info("Verifying local compilation...")

	if cfg.DryRun {
		log.Info("[DRY RUN] Would verify compilation of all components")
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
		out, err := shell.Run(cmd, false, true, dir)
		if err != nil {
			log.Fatalf("Compilation failed for %s (%s) in %s: %v\n%s", label, lang, dir, err, out)
		}
	}
	log.Successf("Compiled %s (%s)", label, lang)
}

func buildCommands(lang string) []string {
	switch lang {
	case "typescript":
		return []string{"npm ci", "npx tsc --noEmit"}
	case "dotnet":
		return []string{"dotnet build"}
	case "java":
		// shell.Run normalizes `.\gradlew.bat` to `./gradlew` on non-Windows
		// hosts via pathx.NormalizeExe, so a single literal works on both.
		return []string{`.\gradlew.bat compileJava compileTestJava`}
	case "react":
		return []string{"npm ci", "npm run build"}
	default:
		log.Fatalf("Unknown language for compilation: %s", lang)
		return nil
	}
}

func systemDir(cfg *config.Config) string {
	if cfg.RepoStrategy == "multirepo" {
		return filepath.Join(cfg.SystemRepoDir, "system")
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
	return filepath.Join(cfg.RepoDir, systemTestDir_name)
}

// VerifyCommitStage waits for commit stage workflow to pass.
func VerifyCommitStage(cfg *config.Config, gh *shell.GitHub) {
	log.Info("Verifying commit stage workflow...")

	if cfg.DryRun {
		log.Info("[DRY RUN] Would wait for commit stage workflow")
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

// VerifyAcceptanceStages triggers the latest and legacy acceptance-stage
// workflows and watches both runs in parallel. Legacy is skipped when
// --no-legacy is set. Downstream stages (QA, prod) resolve the latest RC
// internally when dispatched with an empty version, so no RC lookup is needed
// here.
func VerifyAcceptanceStages(cfg *config.Config, gh *shell.GitHub) {
	includeLegacy := !cfg.NoLegacy

	if includeLegacy {
		log.Info("Triggering acceptance stage (latest + legacy in parallel)...")
	} else {
		log.Info("Triggering acceptance stage (latest)...")
	}

	if cfg.DryRun {
		log.Info("[DRY RUN] Would trigger and wait for acceptance stage workflow(s)")
		return
	}

	shell.CheckRateLimit()
	gh.WorkflowRun("acceptance-stage.yml", nil)

	if includeLegacy {
		shell.CheckRateLimit()
		gh.WorkflowRun("acceptance-stage-legacy.yml", nil)
	}

	// Give GitHub a moment to register both runs before we start watching,
	// otherwise the `gh run list` lookup can return an older run.
	time.Sleep(5 * time.Second)

	var wg sync.WaitGroup
	var latestErr, legacyErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		shell.CheckRateLimit()
		latestErr = gh.RunWatchWorkflow("acceptance-stage.yml", 300)
	}()

	if includeLegacy {
		wg.Add(1)
		go func() {
			defer wg.Done()
			shell.CheckRateLimit()
			legacyErr = gh.RunWatchWorkflow("acceptance-stage-legacy.yml", 300)
		}()
	}

	wg.Wait()

	// Report both results before any fatal, so the user sees the legacy status
	// even when latest failed first (or vice versa).
	reportParallelResult(latestErr, "Acceptance stage", gh.Repo)
	if includeLegacy {
		reportParallelResult(legacyErr, "Acceptance stage legacy", gh.Repo)
	}
	if latestErr != nil {
		handleWorkflowResult(latestErr, "Acceptance stage", gh.Repo)
	}
	if legacyErr != nil {
		handleWorkflowResult(legacyErr, "Acceptance stage legacy", gh.Repo)
	}
}

// reportParallelResult prints the status of one half of a parallel pair
// without fataling, so the other half's result can also be printed before
// the caller decides whether to fatal.
func reportParallelResult(err error, label, repo string) {
	if err == nil {
		log.Successf(msgStagePassed, label)
		return
	}
	log.Errorf("%s failed! See: https://github.com/%s/actions", label, repo)
}

// VerifyQA runs the QA stage followed by the QA signoff. Each workflow
// resolves the latest relevant RC internally when dispatched with an empty
// version.
func VerifyQA(cfg *config.Config, gh *shell.GitHub) {
	VerifyQAStage(cfg, gh)
	VerifyQASignoff(cfg, gh)
}

// VerifyQAStage triggers and verifies QA stage.
func VerifyQAStage(cfg *config.Config, gh *shell.GitHub) {
	log.Info("Triggering and verifying QA stage...")

	if cfg.DryRun {
		log.Info("[DRY RUN] Would trigger and wait for QA stage workflow")
		return
	}

	verifyWorkflow(gh, "QA stage", "qa-stage.yml", nil, 300)
}

// VerifyQASignoff triggers and verifies QA signoff.
func VerifyQASignoff(cfg *config.Config, gh *shell.GitHub) {
	log.Info("Triggering and verifying QA signoff...")

	if cfg.DryRun {
		log.Info("[DRY RUN] Would trigger and wait for QA signoff workflow")
		return
	}

	verifyWorkflow(gh, "QA signoff", "qa-signoff.yml", map[string]string{"result": "approved"}, 300)
}

// VerifyProdStage triggers and verifies production stage.
func VerifyProdStage(cfg *config.Config, gh *shell.GitHub) {
	log.Info("Triggering and verifying production stage...")

	if cfg.DryRun {
		log.Info("[DRY RUN] Would trigger and wait for production stage workflow")
		return
	}

	verifyWorkflow(gh, "Production stage", "prod-stage.yml", nil, 300)
}

// VerifyBumpPatchVersion verifies that bump-patch-version.yml ran successfully.
// It is invoked automatically by prod-stage as a downstream called workflow,
// so prod-stage's overall conclusion already reflects its outcome — we add
// this explicit step for clearer failure attribution if the bump itself
// breaks (syntax error, action breakage, permission issue).
//
// Skipped in multirepo setups: cross-repo bump support is deferred, so
// multirepo students do not have a bump-patch-version.yml workflow.
func VerifyBumpPatchVersion(cfg *config.Config, gh *shell.GitHub) {
	log.Info("Verifying bump-patch-version workflow...")

	if cfg.DryRun {
		log.Info("[DRY RUN] Would verify bump-patch-version workflow")
		return
	}

	if cfg.RepoStrategy == "multirepo" {
		log.Info("Skipping (multirepo: bump-patch-version not scaffolded)")
		return
	}

	verifyNamedWorkflow(gh, "bump-patch-version", "bump-patch-version.yml", 30)
}

// VerifyCleanup triggers cleanup.yml in dry-run mode in every repo where it
// was scaffolded. cleanup.yml is otherwise only fired by its nightly schedule,
// so a syntax error or stale action reference would silently slip past init
// and surface the next night when the student isn't looking. Dry-run prevents
// it from touching the freshly published v1.0.0 release. In multirepo setups
// cleanup.yml is also copied to the sister repos (system / backend+frontend),
// so each is exercised in turn — apply_template.go writes the same template
// content to all of them but each repo has its own actions enablement and
// permissions, so we still verify each.
func VerifyCleanup(cfg *config.Config, gh *shell.GitHub) {
	log.Info("Triggering and verifying cleanup workflow (dry-run)...")

	if cfg.DryRun {
		log.Info("[DRY RUN] Would trigger and wait for cleanup workflow")
		return
	}

	verifyCleanupIn(gh, "Cleanup")

	if cfg.RepoStrategy != "multirepo" {
		return
	}
	if cfg.Arch == "monolith" {
		verifyCleanupIn(gh.ForRepo(cfg.SystemFullRepo), "Cleanup (system repo)")
	} else {
		verifyCleanupIn(gh.ForRepo(cfg.BackendFullRepo), "Cleanup (backend repo)")
		verifyCleanupIn(gh.ForRepo(cfg.FrontendFullRepo), "Cleanup (frontend repo)")
	}
}

func verifyCleanupIn(gh *shell.GitHub, label string) {
	verifyWorkflow(gh, label, "cleanup.yml", map[string]string{"dry-run": "true"}, 300)
}

func verifyNamedWorkflow(gh *shell.GitHub, label, workflowFile string, intervalSecs int) {
	shell.CheckRateLimit()
	err := gh.RunWatchWorkflow(workflowFile, intervalSecs)
	handleWorkflowResult(err, label, gh.Repo)
}

func verifyWorkflow(gh *shell.GitHub, label, triggerWorkflow string, fields map[string]string, intervalSecs int) {
	shell.CheckRateLimit()
	if triggerWorkflow != "" {
		gh.WorkflowRun(triggerWorkflow, fields)
		time.Sleep(5 * time.Second)
	}

	shell.CheckRateLimit()
	err := gh.RunWatch(intervalSecs)
	handleWorkflowResult(err, label, gh.Repo)
}

func handleWorkflowResult(err error, label, repo string) {
	if err == nil {
		log.Successf(msgStagePassed, label)
		return
	}
	var rle *shell.RateLimitExceeded
	if errors.As(err, &rle) {
		log.Errorf("%s failed due to GitHub API rate limiting (the workflow itself may have succeeded)!", label)
		log.Fatalf("Rate limit exceeded while watching %s workflow. The workflow run may still be passing — check manually: https://github.com/%s/actions", label, repo)
	}
	log.Errorf(msgStageFailed, label)
	log.Fatalf("%s workflow failed. Check: https://github.com/%s/actions", label, repo)
}

// runLocalTestsViaRunner brings up system.json's stacks (no-op if already up),
// then runs the given tests config against them. Fatals on any error.
//
// dockerDir holds system.json + compose files (compose paths resolve against
// it). testDir holds tests-*.json + the test-runner project (setupCommands
// and suite.path resolve against it).
func runLocalTestsViaRunner(label, dockerDir, testDir, testsFile string) {
	log.Infof("Running: %s (system=%s/system.json, tests=%s/%s)", label, dockerDir, testDir, testsFile)

	fail := func(err error) {
		log.Errorf("%s: %v", label, err)
		log.Fatalf(msgStageFailed, label)
	}

	sys, err := runner.LoadSystem(filepath.Join(dockerDir, "system.json"))
	if err != nil {
		fail(err)
	}
	tests, err := runner.LoadTests(filepath.Join(testDir, testsFile))
	if err != nil {
		fail(err)
	}
	if err := runner.Up(sys, dockerDir, runner.SystemOptions{}); err != nil {
		fail(err)
	}
	if err := runner.RunTests(sys, tests, dockerDir, testDir, runner.TestOptions{}); err != nil {
		fail(err)
	}
	log.Successf(msgStagePassed, label)
}

// setupMultirepoSymlinks creates symlinks inside the root repo so that Docker Compose
// build contexts (e.g. ../backend, ../frontend, ../system) resolve to the component
// repos cloned in separate directories.
func setupMultirepoSymlinks(cfg *config.Config) {
	if cfg.RepoStrategy != "multirepo" {
		return
	}

	var links [][2]string // {linkPath, target}
	if cfg.Arch == "multitier" {
		links = [][2]string{
			{filepath.Join(cfg.RepoDir, "backend"), filepath.Join(cfg.BackendRepoDir, "backend")},
			{filepath.Join(cfg.RepoDir, "frontend"), filepath.Join(cfg.FrontendRepoDir, "frontend")},
		}
	} else {
		links = [][2]string{
			{filepath.Join(cfg.RepoDir, "system"), filepath.Join(cfg.SystemRepoDir, "system")},
		}
	}

	for _, link := range links {
		linkPath, target := link[0], link[1]
		if _, err := os.Stat(linkPath); err == nil {
			continue // already exists (e.g. monorepo layout)
		}
		if err := createDirLink(linkPath, target); err != nil {
			log.Fatalf("Cannot link %s -> %s: %v\nPass --no-local-tests to skip local verification.",
				linkPath, target, err)
		}
	}
}

// createDirLink links linkPath to the target directory. It tries a real
// symlink first (works on Linux/macOS, and on Windows when SeCreateSymbolicLinkPrivilege
// is held — i.e. admin or Developer Mode). On Windows, when the symlink is
// rejected for privilege reasons, falls back to a directory junction
// (mklink /J), which is the standard NTFS reparse point used by pnpm/Yarn/WSL
// for the same problem. Junctions don't require any privilege and are resolved
// transparently by Docker, Go, and modern Windows tooling.
func createDirLink(linkPath, target string) error {
	err := os.Symlink(target, linkPath)
	if err == nil {
		log.Successf("Symlinked %s -> %s", linkPath, target)
		return nil
	}
	if runtime.GOOS != "windows" || !isWindowsPrivilegeError(err) {
		return err
	}
	if jerr := mklinkJ(linkPath, target); jerr != nil {
		return fmt.Errorf("symlink failed for privilege reasons (Developer Mode off?); junction fallback also failed: %w", jerr)
	}
	log.Successf("Junctioned %s -> %s (symlink privilege not held; junction fallback)", linkPath, target)
	return nil
}

// mklinkJ creates a Windows directory junction at link pointing to target,
// using the cmd builtin "mklink /J". Junctions are NTFS reparse points that
// require no special privilege (unlike symlinks). Same-volume only.
func mklinkJ(link, target string) error {
	cmd := exec.Command("cmd", "/c", "mklink", "/J", link, target)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mklink /J: %v: %s", err, out)
	}
	return nil
}

// isWindowsPrivilegeError reports whether err is ERROR_PRIVILEGE_NOT_HELD (1314),
// the Windows error returned by os.Symlink when the user lacks
// SeCreateSymbolicLinkPrivilege (not admin, Developer Mode off).
func isWindowsPrivilegeError(err error) bool {
	var errno syscall.Errno
	return errors.As(err, &errno) && errno == 1314
}

// VerifyLocalSonar runs the per-component Run-Sonar.ps1 script against each
// scaffolded component, pushing analysis to the SonarCloud project created
// earlier in the same run. The token is picked up from $SONAR_TOKEN by each
// script (already validated and set on the parent process env). Requires pwsh
// (PowerShell 7+) on PATH. Skipped entirely when --no-local-sonar is set
// (the gate lives in main.go).
func VerifyLocalSonar(cfg *config.Config) {
	log.Info("Running local SonarCloud scans...")

	if cfg.DryRun {
		log.Info("[DRY RUN] Would run local SonarCloud scans for all components")
		return
	}

	if cfg.Arch == "monolith" {
		sonarComponent("system source", systemDir(cfg))
	} else {
		sonarComponent("backend", backendDir(cfg))
		sonarComponent("frontend", frontendDir(cfg))
	}
	sonarComponent("system tests", systemTestDir(cfg))
}

func sonarComponent(label, dir string) {
	out, err := shell.Run(`pwsh -File .\Run-Sonar.ps1`, false, true, dir)
	if err != nil {
		log.Fatalf("SonarCloud scan failed for %s in %s: %v\n%s", label, dir, err, out)
	}
	log.Successf("SonarCloud scanned %s", label)
}

// VerifyLocalTesting runs the runner package against the scaffolded project's
// system-test/ directory — latest plus legacy (unless --no-legacy).
// Skipped entirely when --no-local-tests is set (the gate lives in main.go).
func VerifyLocalTesting(cfg *config.Config) {
	log.Info("Running local system tests...")

	if cfg.DryRun {
		log.Info("[DRY RUN] Would run local system tests")
		return
	}

	dockerDir := filepath.Join(cfg.RepoDir, dockerDir_name)
	testDir := filepath.Join(cfg.RepoDir, systemTestDir_name)

	setupMultirepoSymlinks(cfg)

	runLocalTestsViaRunner("Local system tests (latest)", dockerDir, testDir, "tests-latest.json")

	if !cfg.NoLegacy {
		runLocalTestsViaRunner("Local system tests (legacy)", dockerDir, testDir, "tests-legacy.json")
	}
}

