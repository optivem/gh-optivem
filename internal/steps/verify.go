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

	"github.com/optivem/gh-optivem/internal/compiler"
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

	if cfg.Arch == "monolith" {
		compileComponent("system source", cfg.Lang, systemDir(cfg))
	} else {
		compileComponent("backend", cfg.BackendLang, backendDir(cfg))
		compileComponent("frontend", cfg.FrontendLang, frontendDir(cfg))
	}
	compileComponent("system tests", cfg.TestLang, systemTestDir(cfg))
}

// compileComponent dispatches to the shared internal/compiler package so the
// init-time verify pass and the runtime `gh optivem compile` use the same
// per-language command tables.
func compileComponent(label, lang, dir string) {
	if err := compiler.CompileIn(lang, dir); err != nil {
		log.Fatalf("Compilation failed for %s in %s: %v", label, dir, err)
	}
	log.Successf("Compiled %s (%s)", label, lang)
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

// VerifyScaffoldWorkflows lints the scaffolded workflows with actionlint.
// Catches broken `uses: ./.github/workflows/*.yml` references, invalid syntax,
// and other static issues that would otherwise only surface 10+ minutes into
// the verification pipeline at workflow-dispatch time (HTTP 422 "workflow was
// not found"). Runs after apply_template has rewritten filenames and content,
// before any push or workflow trigger.
func VerifyScaffoldWorkflows(cfg *config.Config) {
	log.Info("Linting scaffolded workflows with actionlint...")

	if _, err := exec.LookPath("actionlint"); err != nil {
		// CI must have actionlint (the gh-acceptance composite action installs
		// it). On dev machines it's optional — warn and skip rather than
		// blocking the whole scaffold flow.
		if os.Getenv("GITHUB_ACTIONS") == "true" {
			log.Fatalf("actionlint not found on PATH but GITHUB_ACTIONS=true — install step missing from the runner")
		}
		log.Warnf("actionlint not found on PATH — skipping workflow lint. Install: go install github.com/rhysd/actionlint/cmd/actionlint@v1")
		return
	}

	for _, repoDir := range scaffoldRepoDirs(cfg) {
		wfDir := filepath.Join(repoDir, ".github", "workflows")
		if _, err := os.Stat(wfDir); errors.Is(err, os.ErrNotExist) {
			continue
		}
		out, err := shell.Run("actionlint -color", true, repoDir)
		if err != nil {
			log.Fatalf("actionlint failed in %s:\n%s", wfDir, out)
		}
		log.Successf("Linted scaffolded workflows in %s", repoDir)
	}
}

// scaffoldRepoDirs returns every scaffolded repo directory that has its own
// .github/workflows/. For monorepo strategies that's just RepoDir; for
// multirepo, additionally the per-component repos that received commit-stage
// and bump-patch-version workflows.
func scaffoldRepoDirs(cfg *config.Config) []string {
	dirs := []string{cfg.RepoDir}
	if cfg.RepoStrategy != "multirepo" {
		return dirs
	}
	if cfg.Arch == "monolith" {
		if cfg.SystemRepoDir != "" {
			dirs = append(dirs, cfg.SystemRepoDir)
		}
		return dirs
	}
	if cfg.BackendRepoDir != "" {
		dirs = append(dirs, cfg.BackendRepoDir)
	}
	if cfg.FrontendRepoDir != "" {
		dirs = append(dirs, cfg.FrontendRepoDir)
	}
	return dirs
}

// VerifyCommitStage waits for commit stage workflow to pass.
func VerifyCommitStage(cfg *config.Config, gh *shell.GitHub) {
	log.Info("Verifying commit stage workflow...")

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

	verifyWorkflow(gh, "QA stage", "qa-stage.yml", nil, 300)
}

// VerifyQASignoff triggers and verifies QA signoff.
func VerifyQASignoff(cfg *config.Config, gh *shell.GitHub) {
	log.Info("Triggering and verifying QA signoff...")

	verifyWorkflow(gh, "QA signoff", "qa-signoff.yml", map[string]string{"result": "approved"}, 300)
}

// VerifyProdStage triggers and verifies production stage.
func VerifyProdStage(cfg *config.Config, gh *shell.GitHub) {
	log.Info("Triggering and verifying production stage...")

	verifyWorkflow(gh, "Production stage", "prod-stage.yml", nil, 300)
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
	// Cleanup is short-lived; poll every 60s instead of the 300s default.
	verifyWorkflow(gh, label, "cleanup.yml", map[string]string{"dry-run": "true"}, 60)
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
	err := gh.RunWatchWorkflow(triggerWorkflow, intervalSecs)
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
	log.Errorf("Underlying error from gh run watch: %v", err)
	log.Fatalf("%s workflow failed. Check: https://github.com/%s/actions", label, repo)
}

// runLocalTestsViaRunner brings up systems.yaml's stacks (no-op if already up),
// then runs the given tests config against them. Fatals on any error.
//
// dockerDir holds systems.yaml + compose files (compose paths resolve against
// it). testDir holds tests-*.yaml + the test-runner project (setupCommands
// and suite.path resolve against it).
func runLocalTestsViaRunner(label, dockerDir, testDir, testsFile string) {
	log.Infof("Running: %s (system=%s/systems.yaml, tests=%s/%s)", label, dockerDir, testDir, testsFile)

	fail := func(err error) {
		log.Errorf("%s: %v", label, err)
		log.Fatalf(msgStageFailed, label)
	}

	sys, err := runner.LoadSystem(filepath.Join(dockerDir, "systems.yaml"))
	if err != nil {
		fail(err)
	}
	tests, err := runner.LoadTests(filepath.Join(testDir, testsFile))
	if err != nil {
		fail(err)
	}
	// setupCommands aren't run by RunTests (separate verb in the CLI); invoke
	// them here so a fresh runner gets `npx playwright install chromium` etc.
	if err := runner.RunSetup(tests, testDir); err != nil {
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

// VerifyLocalSonar runs the per-component run-sonar.sh script against each
// scaffolded component, pushing analysis to the SonarCloud project created
// earlier in the same run. The token is picked up from $SONAR_TOKEN by each
// script (already validated and set on the parent process env). Requires bash
// on PATH (Git Bash on Windows). Skipped entirely when --no-local-sonar is set
// (the gate lives in main.go).
func VerifyLocalSonar(cfg *config.Config) {
	log.Info("Running local SonarCloud scans...")

	if _, err := exec.LookPath("bash"); err != nil {
		log.Fatalf("bash not found on PATH (required to run run-sonar.sh). Install Git Bash on Windows: https://git-scm.com/download/win")
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
	out, err := shell.Run("bash ./run-sonar.sh", true, dir)
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

	dockerDir := filepath.Join(cfg.RepoDir, dockerDir_name)
	testDir := filepath.Join(cfg.RepoDir, systemTestDir_name)

	setupMultirepoSymlinks(cfg)

	runLocalTestsViaRunner("Local system tests (latest)", dockerDir, testDir, "tests.yaml")

	if !cfg.NoLegacy {
		runLocalTestsViaRunner("Local system tests (legacy)", dockerDir, testDir, "tests.legacy.yaml")
	}
}

