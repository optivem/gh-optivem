// gh-optivem: A gh CLI extension for pipeline project management.
//
// Usage:
//
//	Monolith:
//	  gh optivem init --owner acme --system-name "Page Turner" --repo page-turner \
//	      --arch monolith --lang java
//
//	Multitier:
//	  gh optivem init --owner acme --system-name "Page Turner" --repo page-turner \
//	      --arch multitier --backend-lang java --frontend-lang react
//
//	Dry run:
//	  gh optivem init ... --dry-run
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/log"
	"github.com/optivem/gh-optivem/internal/shell"
	"github.com/optivem/gh-optivem/internal/steps"
	"github.com/optivem/gh-optivem/internal/version"
)

const separator = "=========================================="

// originalArgs is captured before the subcommand-strip in main(). Used by
// checkForUpdate to re-exec the freshly upgraded binary with the user's full
// original command line (`gh optivem init ...` with all their flags intact).
var originalArgs []string

type stepDef struct {
	name      string
	phase     string
	alwaysRun bool
	fn        func()
}

func main() {
	// Snapshot os.Args before any mutation. checkForUpdate uses this to spawn
	// the upgraded binary with the original command line preserved.
	originalArgs = append([]string{}, os.Args...)

	// Handle --version before anything else
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println(version.Full())
		os.Exit(0)
	}

	// Require a subcommand
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	subcommand := os.Args[1]

	switch subcommand {
	case "init":
		// Strip the subcommand so flag.Parse() sees only the flags
		os.Args = append(os.Args[:1], os.Args[2:]...)
		runInit()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", subcommand)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: gh optivem <command> [flags]\n\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  init        Scaffold a new pipeline project\n\n")
	fmt.Fprintf(os.Stderr, "Flags:\n")
	fmt.Fprintf(os.Stderr, "  --version   Print version and exit\n")
}

func runInit() {
	cfg := config.ParseAndValidate()
	if err := log.Init(cfg.Verbose, cfg.Quiet, cfg.LogFile); err != nil {
		log.FatalExit(err.Error())
	}
	defer log.Close()

	// Auto-upgrade if outdated. On success, re-execs the new binary with the
	// user's original command and exits with the child's exit code.
	checkForUpdate(cfg)

	gh := shell.NewGitHub(cfg)
	sc := shell.NewSonarCloud(cfg.SonarToken, cfg.OwnerLower)

	printBanner(cfg)

	// failureNote is captured by the Commit-and-push closure so, if an earlier
	// step failed, the commit message can flag the push as a partial scaffold.
	var failureNote string
	allSteps := buildSteps(cfg, gh, sc, &failureNote)
	errors, totalDuration := executeSteps(allSteps, &failureNote)
	printSummary(cfg, errors, totalDuration)

	// Cleanup (test mode only) — skip on failure so repo can be inspected
	if errors > 0 && !cfg.ForceCleanup {
		cfg.KeepLocal = true
		cfg.DeleteTestRepos = false
	}
	steps.Cleanup(cfg, gh, sc)

	if errors > 0 {
		os.Exit(1)
	}
}

// Phase labels — printed as section headers by executeSteps when the phase changes.
const (
	phasePrepare       = "Prepare"
	phaseSetupRepo     = "Setup repository"
	phaseApplyTemplate = "Apply template"
	phaseValidate      = "Validate template"
	phaseVerify        = "Verify pipeline"
	phaseFinalize      = "Finalize"
)

func buildSteps(cfg *config.Config, gh *shell.GitHub, sc *shell.SonarCloud, failureNote *string) []stepDef {
	allSteps := []stepDef{
		// PHASE: PREPARE — pull prerequisites (shop template) needed by all later phases
		{name: "Clone shop template", phase: phasePrepare, fn: func() { steps.CloneShopTemplate(cfg) }},

		// PHASE: SETUP REPOSITORY — remote infra for this repo
		{name: "Create repositories", phase: phaseSetupRepo, fn: func() { steps.CreateRepos(cfg, gh) }},
		{name: "Setup environments", phase: phaseSetupRepo, fn: func() { steps.SetupEnvironments(cfg, gh) }},
		{name: "Setup secrets and variables", phase: phaseSetupRepo, fn: func() { steps.SetupSecretsAndVariables(cfg, gh) }},

		// PHASE: APPLY TEMPLATE — clone, templatize locally, push (even on failure)
		{name: "Clone repos", phase: phaseApplyTemplate, fn: func() { steps.CloneRepos(cfg, gh) }},
		{name: "Apply template", phase: phaseApplyTemplate, fn: func() { steps.ApplyTemplate(cfg) }},
		{name: "Replace repository references", phase: phaseApplyTemplate, fn: func() { steps.ReplaceRepoReferences(cfg) }},
		{name: "Replace namespaces", phase: phaseApplyTemplate, fn: func() { steps.ReplaceNamespaces(cfg) }},
		{name: "Replace system name", phase: phaseApplyTemplate, fn: func() { steps.ReplaceSystemName(cfg) }},
		{name: "Update README", phase: phaseApplyTemplate, fn: func() { steps.UpdateReadme(cfg) }},
		{name: "Write project config", phase: phaseApplyTemplate, fn: func() { steps.WriteProjectConfig(cfg) }},
		{name: "Create SonarCloud projects", phase: phaseApplyTemplate, fn: func() { steps.CreateSonarCloudProjects(cfg, sc) }},
		{name: "Commit and push", phase: phaseApplyTemplate, alwaysRun: true, fn: func() { steps.CommitAndPush(cfg, *failureNote) }},

		// PHASE: VALIDATE TEMPLATE — runs after push so broken output is already
		// visible in the remote repo for troubleshooting.
		{name: "Validate no leftover system names", phase: phaseValidate, fn: func() { steps.ValidateNoLeftoverSystemNames(cfg) }},
		{name: "Verify compilation", phase: phaseValidate, fn: func() { steps.VerifyCompilation(cfg) }},
	}

	allSteps = append(allSteps, buildVerifySteps(cfg, gh)...)

	allSteps = append(allSteps,
		stepDef{name: "Print project registration", phase: phaseFinalize, fn: func() { steps.PrintRegistration(cfg) }},
	)

	return allSteps
}

func buildVerifySteps(cfg *config.Config, gh *shell.GitHub) []stepDef {
	if cfg.VerifyLevel == "none" {
		log.Infof("Skipping workflow verification (--verify-level none)")
		return nil
	}

	var s []stepDef

	// smoke tier — local smoke tests only, no CI
	if cfg.VerifyLevel == "local" {
		s = append(s,
			stepDef{name: "Run local smoke tests", phase: phaseVerify, fn: func() { steps.RunLocalSystemTests(cfg) }},
		)
	}

	// commit tier and above — CI workflow verification
	if cfg.VerifyLevel == "commit" || cfg.VerifyLevel == "acceptance" || cfg.VerifyLevel == "release" {
		s = append(s,
			stepDef{name: "Verify commit stage", phase: phaseVerify, fn: func() { steps.VerifyCommitStage(cfg, gh) }},
		)
	}

	if cfg.VerifyLevel == "acceptance" || cfg.VerifyLevel == "release" {
		s = append(s,
			stepDef{name: "Verify acceptance stage", phase: phaseVerify, fn: func() { steps.VerifyAcceptanceStage(cfg, gh) }},
		)
		if !cfg.ExcludeLegacy {
			s = append(s,
				stepDef{name: "Verify acceptance stage legacy", phase: phaseVerify, fn: func() { steps.VerifyAcceptanceStageLegacy(cfg, gh) }},
			)
		}
		s = append(s,
			stepDef{name: "Run local system tests", phase: phaseVerify, fn: func() { steps.RunLocalSystemTests(cfg) }},
		)
	}

	if cfg.VerifyLevel == "release" {
		s = append(s,
			stepDef{name: "Verify QA stage", phase: phaseVerify, fn: func() { steps.VerifyQAStage(cfg, gh) }},
			stepDef{name: "Verify QA signoff", phase: phaseVerify, fn: func() { steps.VerifyQASignoff(cfg, gh) }},
			stepDef{name: "Verify production stage", phase: phaseVerify, fn: func() { steps.VerifyProdStage(cfg, gh) }},
		)
	}

	return s
}

func executeSteps(allSteps []stepDef, failureNote *string) (int, time.Duration) {
	errors := 0
	totalStart := time.Now()
	totalByPhase := countStepsByPhase(allSteps)
	order := phaseOrder(allSteps)
	phaseIdx := make(map[string]int, len(order))
	for i, p := range order {
		phaseIdx[p] = i + 1
	}
	totalPhases := len(order)

	posInPhase := 0
	prevPhase := ""
	for _, s := range allSteps {
		// Skip non-alwaysRun steps once something has failed, so a broken
		// scaffold still pushes the partial output (for troubleshooting) but
		// stops running the downstream validate/verify steps.
		if errors > 0 && !s.alwaysRun {
			continue
		}

		if s.phase != prevPhase {
			if s.phase != "" {
				log.PhaseHeader(phaseIdx[s.phase], totalPhases, s.phase)
			}
			prevPhase = s.phase
			posInPhase = 0
		}
		posInPhase++

		if !runStep(s, posInPhase, totalByPhase[s.phase], failureNote) {
			errors++
		}
	}
	return errors, time.Since(totalStart)
}

func countStepsByPhase(allSteps []stepDef) map[string]int {
	totals := make(map[string]int)
	for _, s := range allSteps {
		totals[s.phase]++
	}
	return totals
}

// phaseOrder returns the distinct phase labels in order of first appearance.
func phaseOrder(allSteps []stepDef) []string {
	var order []string
	seen := make(map[string]bool)
	for _, s := range allSteps {
		if s.phase == "" || seen[s.phase] {
			continue
		}
		seen[s.phase] = true
		order = append(order, s.phase)
	}
	return order
}

// runStep runs a single step, recovering from panics. Returns true on success.
// On failure it sets *failureNote (unless already set by an earlier failure) so
// the later always-run commit step can flag the push as partial.
func runStep(s stepDef, pos, total int, failureNote *string) (ok bool) {
	ok = true
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Step failed: %s -- %v", s.name, r)
			if *failureNote == "" {
				*failureNote = fmt.Sprintf("%s step %d/%d: %s", s.phase, pos, total, s.name)
			}
			ok = false
		}
	}()
	stepStart := time.Now()
	s.fn()
	log.StepDone(pos, total, formatDuration(time.Since(stepStart)))
	return
}

func printSummary(cfg *config.Config, errors int, totalDuration time.Duration) {
	fmt.Println()
	fmt.Println(separator)
	if errors > 0 {
		log.Errorf("Setup completed with %d error(s) in %s", errors, formatDuration(totalDuration))
		if !cfg.NoBugReport {
			createBugReport(cfg, errors)
		}
	} else {
		log.Successf("All steps passed! Completed in %s", formatDuration(totalDuration))
	}
	fmt.Println()
	fmt.Printf("  System:     %s\n", cfg.SystemName)
	fmt.Printf("  Repository: https://github.com/%s\n", cfg.FullRepo)
	fmt.Printf("  Actions:    https://github.com/%s/actions\n", cfg.FullRepo)
	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "multitier" {
			fmt.Printf("  Backend:    https://github.com/%s\n", cfg.BackendFullRepo)
			fmt.Printf("  Frontend:   https://github.com/%s\n", cfg.FrontendFullRepo)
		} else {
			fmt.Printf("  System:     https://github.com/%s\n", cfg.SystemFullRepo)
		}
	}
	fmt.Println()
}

func createBugReport(cfg *config.Config, errorCount int) {
	lang := cfg.Lang
	if cfg.Arch == "multitier" {
		lang = fmt.Sprintf("backend=%s, frontend=%s", cfg.BackendLang, cfg.FrontendLang)
	}

	title := fmt.Sprintf("Scaffolding failure: %s (%s, %s)", cfg.SystemName, cfg.Arch, cfg.RepoStrategy)
	body := fmt.Sprintf(
		"Scaffolding failed with %d error(s).\n\n"+
			"- **System:** %s\n"+
			"- **Architecture:** %s\n"+
			"- **Repo strategy:** %s\n"+
			"- **Language:** %s\n"+
			"- **Test language:** %s\n"+
			"- **Repository:** https://github.com/%s\n",
		errorCount, cfg.SystemName, cfg.Arch, cfg.RepoStrategy,
		lang, cfg.TestLang, cfg.FullRepo)

	bodyFile, err := os.CreateTemp("", "gh-issue-body-*.md")
	if err != nil {
		log.Infof("WARN: Failed to create temp file for bug report: %v", err)
		return
	}
	defer os.Remove(bodyFile.Name())
	bodyFile.WriteString(body)
	bodyFile.Close()

	out, err := shell.Run(
		fmt.Sprintf(`gh issue create --repo optivem/gh-optivem --title %q --body-file %s --assignee valentinajemuovic`, title, bodyFile.Name()),
		false, false, "")
	if err != nil {
		log.Infof("WARN: Failed to create bug report: %v\n%s", err, out)
	} else {
		log.Successf("Bug report created: %s", strings.TrimSpace(out))
	}
}

// checkForUpdate queries the latest release. When the running binary is
// outdated, it auto-upgrades via `gh extension upgrade optivem` and re-execs
// the new binary with the user's original command line (then exits with the
// child's exit code). Users can opt out with --no-auto-upgrade — in that case
// they get the old passive "please upgrade" notice and the scaffold continues.
func checkForUpdate(cfg *config.Config) {
	if version.Version == "dev" {
		return // skip check for development builds
	}

	cmd := exec.Command("gh", "api", "repos/optivem/gh-optivem/releases/latest", "--jq", ".tag_name")
	out, err := cmd.Output()
	if err != nil {
		return // fail silently — don't block usage if offline or rate-limited
	}
	latest := strings.TrimSpace(string(out))
	if latest == "" || latest == version.Version {
		return
	}

	if cfg.NoAutoUpgrade {
		log.Warnf("Update available: %s → %s. Auto-upgrade disabled (--no-auto-upgrade).", version.Version, latest)
		log.Warnf("To upgrade manually: gh extension upgrade optivem")
		return
	}

	log.Warnf("Update available: %s → %s. Upgrading...", version.Version, latest)
	upgrade := exec.Command("gh", "extension", "upgrade", "optivem")
	upgrade.Stdout = os.Stdout
	upgrade.Stderr = os.Stderr
	if err := upgrade.Run(); err != nil {
		log.Errorf("Auto-upgrade failed: %v", err)
		log.Errorf("Please run manually: gh extension upgrade optivem")
		log.Close()
		os.Exit(1)
	}

	log.Successf("Upgraded to %s. Restarting...", latest)
	log.Close() // flush/close log file before handing off to child
	child := exec.Command(originalArgs[0], originalArgs[1:]...)
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	if err := child.Run(); err != nil {
		// Child wrote its own diagnostics via its own log helpers.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
	os.Exit(0)
}

func printBanner(cfg *config.Config) {
	fmt.Println()
	fmt.Println(separator)
	fmt.Println("  Pipeline Project Setup")
	fmt.Println(separator)

	fmt.Println()
	fmt.Println("  Inputs (raw CLI flags)")
	fmt.Println()
	log.Infof("--owner:           %s", cfg.Owner)
	log.Infof("--repo:            %s", cfg.Raw.Repo)
	log.Infof("--system-name:     %s", cfg.SystemName)
	log.Infof("--arch:            %s", cfg.Arch)
	log.Infof("--repo-strategy:   %s", cfg.RepoStrategy)
	log.Infof("--lang:            %s", cfg.Lang)
	log.Infof("--backend-lang:    %s", cfg.BackendLang)
	log.Infof("--frontend-lang:   %s", cfg.FrontendLang)
	log.Infof("--test-lang:       %s", cfg.Raw.TestLang)
	log.Infof("--license:         %s", cfg.License)
	log.Infof("--deploy:          %s", cfg.Deploy)
	log.Infof("--verify-level:    %s", cfg.Raw.VerifyLevel)
	log.Infof("--exclude-legacy:  %v", cfg.ExcludeLegacy)
	log.Infof("--sample-tests:    %v", cfg.SampleTests)
	log.Infof("--shop-tag:        %s", cfg.Raw.ShopTag)
	log.Infof("--random-suffix:   %v", cfg.Raw.RandomSuffix)
	log.Infof("--dry-run:         %v", cfg.DryRun)
	log.Infof("--test:            %v", cfg.TestMode)
	log.Infof("--keep-local:         %v", cfg.Raw.KeepLocal)
	log.Infof("--delete-test-repos:  %v", cfg.Raw.DeleteTestRepos)
	log.Infof("--force-cleanup:      %v", cfg.ForceCleanup)
	log.Infof("--no-bug-report:   %v", cfg.NoBugReport)
	log.Infof("--no-auto-upgrade: %v", cfg.NoAutoUpgrade)
	log.Infof("--workdir:         %s", cfg.Raw.WorkDir)
	log.Infof("--log-file:        %s", cfg.LogFile)
	log.Infof("--verbose:         %v", cfg.Verbose)
	log.Infof("--quiet:           %v", cfg.Quiet)

	fmt.Println()
	fmt.Println("  Derived values")
	fmt.Println()
	log.Infof("Repo (final):    %s", cfg.Repo)
	log.Infof("Full repo:       %s", cfg.FullRepo)
	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "monolith" {
			log.Infof("System repo:     %s", cfg.SystemFullRepo)
		} else {
			log.Infof("Backend repo:    %s", cfg.BackendFullRepo)
			log.Infof("Frontend repo:   %s", cfg.FrontendFullRepo)
		}
	}
	log.Infof("Test lang:       %s", cfg.TestLang)
	log.Infof("Verify level:    %s", cfg.VerifyLevel)
	log.Infof("Cleanup local:     %v  (--keep-local to keep)", !cfg.KeepLocal)
	log.Infof("Cleanup test repos: %v  (--delete-test-repos to delete)", cfg.DeleteTestRepos)
	log.Infof("System name:     pascal=%s  camel=%s  kebab=%s  lower=%s",
		cfg.SysNamePascalNew, cfg.SysNameCamelNew, cfg.SysNameKebabNew, cfg.SysNameLowerNew)
	log.Infof("Java ns:         %s", cfg.JavaNsNew)
	log.Infof(".NET ns:         %s", cfg.DotnetNsNew)
	log.Infof("TS package:      %s", cfg.TsPkgNew)
	for _, key := range steps.GetSonarProjectKeys(cfg) {
		log.Infof("Sonar key:       %s", key)
	}

	fmt.Println()
	fmt.Println("  Environment")
	fmt.Println()
	log.Infof("gh-optivem:      %s", version.Version)
	log.Infof("gh CLI:          %s", ghCLIVersion())
	log.Infof("Shop ref:        %s", cfg.ShopRef)

	fmt.Println()
	fmt.Println("  Will create")
	fmt.Println()
	log.Infof("Environments:    acceptance, qa, production")
	log.Infof("Secrets:         %s", strings.Join(willCreateSecrets(cfg), ", "))
	log.Infof("Variables:       DOCKERHUB_USERNAME")

	fmt.Println()
	fmt.Println("  Local clones")
	fmt.Println()
	log.Infof("Workdir:         %s", cfg.WorkDir)
	log.Infof("Shop:            %s", cfg.ShopPath)
	log.Infof("Repo:            %s", cfg.RepoDir)
	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "multitier" {
			log.Infof("Frontend repo:   %s", cfg.FrontendRepoDir)
			log.Infof("Backend repo:    %s", cfg.BackendRepoDir)
		} else {
			log.Infof("System repo:     %s", cfg.SystemRepoDir)
		}
	}
	fmt.Println()
}

// willCreateSecrets lists the GitHub Actions secrets this scaffold will set,
// in the order SetupSecretsAndVariables writes them. GHCR_TOKEN only applies
// to multirepo scaffolds (component repos need to pull the main repo's image).
func willCreateSecrets(cfg *config.Config) []string {
	secrets := []string{"DOCKERHUB_TOKEN", "SONAR_TOKEN", "WORKFLOW_TOKEN"}
	if cfg.RepoStrategy == "multirepo" {
		secrets = append(secrets, "GHCR_TOKEN")
	}
	return secrets
}

// ghCLIVersion returns the first line of `gh --version` (e.g. "gh version 2.50.0 ...").
// Returns "unknown" if gh is missing or the call fails — the scaffold still
// proceeds, and later steps will surface a clearer error if gh is actually
// unavailable.
func ghCLIVersion() string {
	out, err := exec.Command("gh", "--version").Output()
	if err != nil {
		return "unknown"
	}
	line, _, _ := strings.Cut(string(out), "\n")
	return strings.TrimSpace(line)
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
