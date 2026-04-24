package steps

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/log"
	"github.com/optivem/gh-optivem/internal/shell"
)

const (
	systemTestDir_name = "system-test"

	msgStagePassed = "%s passed!"
	msgStageFailed = "%s failed!"
)

// transientNetworkPattern matches errors that indicate an upstream package
// registry (Maven Central, npm, NuGet) is temporarily unavailable — not a
// problem with the user's code. When detected after retries are exhausted,
// local verification is skipped with a warning and CI becomes the gate.
var transientNetworkPattern = regexp.MustCompile(
	`(?i)` +
		`502 bad gateway|` +
		`503 service unavailable|` +
		`504 gateway time-?out|` +
		`received status code 5\d\d|` +
		`could not get '[^']*\.pom'|` +
		`could not resolve [^ ]+:[^ ]+:[^ ]+|` +
		`connection reset by peer|` +
		`connection timed out|` +
		`read: connection reset|` +
		`econnreset|` +
		`etimedout|` +
		`network.?error|` +
		`getaddrinfo (?:enotfound|eai_again)|` +
		`tls handshake timeout|` +
		`err_socket_timeout|` +
		`request to https?://[^ ]+ failed`,
)

const (
	compileRetryAttempts = 3
	compileRetryInitial  = 15 * time.Second
	compileRetryMax      = 60 * time.Second
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
		if err := runWithRetry(label, lang, dir, cmd); err != nil {
			log.Fatalf("Compilation failed for %s (%s) in %s: %v", label, lang, dir, err)
		}
	}
	log.Successf("Compiled %s (%s)", label, lang)
}

// runWithRetry executes cmd with retries on transient network failures.
// After retries are exhausted:
//   - If the last error looks transient (e.g. Maven Central 502), log a warning
//     and return nil — CI will re-verify with a fresh network. This prevents
//     external registry outages from blocking students or CI with a false
//     "your code is broken" signal.
//   - Otherwise return the error so the caller can fail hard.
func runWithRetry(label, lang, dir, cmd string) error {
	var lastOut string
	var lastErr error

	backoff := compileRetryInitial
	for attempt := 1; attempt <= compileRetryAttempts; attempt++ {
		out, err := shell.Run(cmd, false, true, dir)
		if err == nil {
			return nil
		}
		lastOut, lastErr = out, err

		if !isTransient(out) {
			return fmt.Errorf("%w\n%s", err, out)
		}

		if attempt < compileRetryAttempts {
			log.Warnf("Transient network error compiling %s (%s), attempt %d/%d — retrying in %s", label, lang, attempt, compileRetryAttempts, backoff)
			time.Sleep(backoff)
			if backoff *= 4; backoff > compileRetryMax {
				backoff = compileRetryMax
			}
		}
	}

	if isTransient(lastOut) {
		log.Warn(strings.Repeat("─", 72))
		log.Warnf("Skipping local verification of %s (%s) — upstream package registry appears unavailable.", label, lang)
		log.Warn("This is a temporary outage on the registry side (e.g. Maven Central, npm, NuGet),")
		log.Warn("not a problem with your code. CI will re-verify compilation once network is healthy.")
		log.Warn(strings.Repeat("─", 72))
		return nil
	}

	return fmt.Errorf("%w\n%s", lastErr, lastOut)
}

func isTransient(output string) bool {
	return transientNetworkPattern.MatchString(output)
}

func buildCommands(lang string) []string {
	switch lang {
	case "typescript":
		return []string{"npm ci", "npx tsc --noEmit"}
	case "dotnet":
		return []string{"dotnet build"}
	case "java":
		if runtime.GOOS == "windows" {
			return []string{`.\gradlew.bat compileJava compileTestJava`}
		}
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
// --exclude-legacy is set. Downstream stages (QA, prod) resolve the latest RC
// internally when dispatched with an empty version, so no RC lookup is needed
// here.
func VerifyAcceptanceStages(cfg *config.Config, gh *shell.GitHub) {
	includeLegacy := !cfg.ExcludeLegacy

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

// runLocalTests runs a pwsh test command and reports the result.
func runLocalTests(label, cmd, dir string) {
	log.Infof("Running: %s (in %s)", cmd, dir)
	output, err := shell.Run(cmd, false, true, dir)
	if err != nil {
		log.Errorf("%s output:\n%s", label, output)
		log.Fatalf(msgStageFailed, label)
	}
	log.Successf(msgStagePassed, label)
}

// canRunLocalTests checks common preconditions for local test execution.
// Returns the test directory path, or empty string if tests should be skipped.
// Whether to call this step at all is gated by --verify-level in main.go.
func canRunLocalTests(cfg *config.Config, testKind string) string {
	testDir := filepath.Join(cfg.RepoDir, systemTestDir_name)
	if _, err := os.Stat(filepath.Join(testDir, "Run-SystemTests.ps1")); err != nil {
		log.Warnf("Run-SystemTests.ps1 not found in scaffolded project, skipping local %s", testKind)
		return ""
	}

	return testDir
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
		if err := os.Symlink(target, linkPath); err != nil {
			log.Warnf("Failed to create symlink %s -> %s: %v", linkPath, target, err)
		} else {
			log.Successf("Symlinked %s -> %s", linkPath, target)
		}
	}
}

// VerifyLocalTesting runs Run-SystemTests.ps1 locally against the scaffolded
// project — latest plus legacy (unless --exclude-legacy). Skipped entirely
// when --skip-local-tests is set (the gate lives in main.go).
func VerifyLocalTesting(cfg *config.Config) {
	log.Info("Running local system tests...")

	if cfg.DryRun {
		log.Info("[DRY RUN] Would run local system tests")
		return
	}

	testDir := canRunLocalTests(cfg, "system tests")
	if testDir == "" {
		return
	}

	setupMultirepoSymlinks(cfg)

	runLocalTests(
		"Local system tests (latest)",
		"pwsh -NonInteractive -Command ./Run-SystemTests.ps1",
		testDir,
	)

	if !cfg.ExcludeLegacy {
		runLocalTests(
			"Local system tests (legacy)",
			"pwsh -NonInteractive -Command ./Run-SystemTests.ps1 -Legacy",
			testDir,
		)
	}
}

