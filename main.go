// gh-optivem: A gh CLI extension for pipeline project management.
//
// Usage:
//
//	Monolith:
//	  gh optivem init --owner acme --system-name "Page Turner" --repo page-turner \
//	      --arch monolith --monolith-lang java
//
//	Multitier:
//	  gh optivem init --owner acme --system-name "Page Turner" --repo page-turner \
//	      --arch multitier --backend-lang java --frontend-lang react
//
//	Dry run:
//	  gh optivem init ... --dry-run
package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/log"
	"github.com/optivem/gh-optivem/internal/shell"
	"github.com/optivem/gh-optivem/internal/steps"
	"github.com/optivem/gh-optivem/internal/version"
)

// bugReportLogMaxBytes is the inline log budget for a filed issue body.
// GitHub caps issue bodies at 65,536 chars; 50 KB leaves comfortable room
// for the metadata block plus the <details> wrapper. The whole log is
// embedded when it fits; otherwise the trailing 50 KB (trimmed to the next
// newline) is embedded with a "truncated" note.
const bugReportLogMaxBytes = 50 * 1024

const separator = "=========================================="

type stepDef struct {
	name      string
	phase     string
	alwaysRun bool
	fn        func()
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

// newRootCmd builds the top-level `optivem` Cobra command and attaches every
// subcommand. Cobra handles --help/-h on every level, --version at the root,
// shell completion (via the auto-added `completion` subcommand), and unknown-
// subcommand suggestions. Subcommands still call os.Exit on validation
// failures, so Execute returns an error only for Cobra-level problems
// (unknown flag, malformed args).
func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "optivem",
		Short:   "Scaffold and operate Optivem academy pipeline projects",
		Version: version.Full(),
		// Subcommands print their own usage on validation errors via log.FatalExit;
		// avoid double-printing usage when Cobra returns an error.
		SilenceUsage: true,
	}
	cmd.SetVersionTemplate("{{.Version}}\n")
	// Pre-register --version without the -v shorthand so `init -v` keeps
	// resolving to --verbose. Cobra's InitDefaultVersionFlag detects the
	// flag is already present and skips adding its default `-v` alias.
	cmd.Flags().Bool("version", false, "Print version and exit")
	cmd.AddCommand(
		newInitCmd(),
		newBuildCmd(),
		newRunCmd(),
		newTestCmd(),
		newStopCmd(),
		newCleanCmd(),
		newUpgradeCmd(),
	)
	return cmd
}

// newUpgradeCmd implements `gh optivem upgrade` — a thin wrapper around
// `gh extension upgrade optivem` so users don't have to remember the longer
// gh-extension form. Streams stdout/stderr through unchanged.
func newUpgradeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade this extension to the latest release",
		Long:  `Upgrade gh-optivem to the latest release. Equivalent to running "gh extension upgrade optivem".`,
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			upgrade := exec.Command("gh", "extension", "upgrade", "optivem")
			upgrade.Stdout = os.Stdout
			upgrade.Stderr = os.Stderr
			if err := upgrade.Run(); err != nil {
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					os.Exit(exitErr.ExitCode())
				}
				fmt.Fprintf(os.Stderr, "ERROR: gh extension upgrade optivem failed: %v\n", err)
				os.Exit(1)
			}
		},
	}
}

// newInitCmd builds the `init` subcommand. The repo name can be passed
// positionally (`gh optivem init page-turner ...`) or via --repo; if both are
// given, the explicit flag wins.
func newInitCmd() *cobra.Command {
	f := &config.RawFlags{}
	cmd := &cobra.Command{
		Use:   "init [repo]",
		Short: "Scaffold a new pipeline project",
		Long: `Scaffold a new pipeline project: create the GitHub repo(s), apply the shop
template with naming substitutions, wire up the SonarCloud project(s), and
verify the pipeline up to the requested --verify-level.`,
		Example: `  # Monolith, Java
  gh optivem init page-turner --owner acme --system-name "Page Turner" \
      --arch monolith --repo-strategy monorepo --monolith-lang java

  # Multitier (Java backend, React frontend), one repo per tier
  gh optivem init page-turner --owner acme --system-name "Page Turner" \
      --arch multitier --repo-strategy multirepo \
      --backend-lang java --frontend-lang react`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			// Positional repo wins only if --repo wasn't passed explicitly.
			if len(args) == 1 && f.Repo == "" {
				f.Repo = args[0]
			}
			runInit(cmd, f)
		},
	}
	config.BindInitFlags(cmd, f)
	return cmd
}

func runInit(cmd *cobra.Command, f *config.RawFlags) {
	cfg := config.ParseAndValidate(cmd, f)
	if err := log.Init(cfg.Verbose, cfg.Quiet, cfg.LogFile); err != nil {
		log.FatalExit(err.Error())
	}
	defer log.Close()

	// Print update notice if a newer release is available. Notify-only — the
	// user upgrades explicitly via `gh optivem upgrade` (or `gh extension
	// upgrade optivem`).
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

	// Keep the local scaffold dir on failure so it can be inspected.
	if errors > 0 {
		cfg.KeepLocal = true
	}
	steps.Cleanup(cfg)

	if errors > 0 {
		os.Exit(1)
	}
}

// Phase labels — printed as section headers by executeSteps when the phase changes.
const (
	phasePrepare            = "Prepare"
	phaseSetupRepo          = "Setup repository"
	phaseApplyTemplate      = "Apply template"
	phaseVerifyLocal        = "Verify local"
	phaseVerifyCommit       = "Verify commit stage"
	phaseVerifyAcceptance   = "Verify acceptance stage"
	phaseVerifyQA           = "Verify QA stage"
	phaseVerifyProduction   = "Verify production stage"
	phaseFinalize           = "Finalize"
)

func buildSteps(cfg *config.Config, gh *shell.GitHub, sc *shell.SonarCloud, failureNote *string) []stepDef {
	allSteps := []stepDef{
		// PHASE: PREPARE — pull prerequisites (shop template) needed by all later phases
		{name: "Clone shop template", phase: phasePrepare, fn: func() { steps.CloneShopTemplate(cfg) }},

		// PHASE: SETUP REPOSITORY — remote infra for this repo
		{name: "Create repositories", phase: phaseSetupRepo, fn: func() { steps.CreateRepos(cfg, gh) }},
		{name: "Setup environments", phase: phaseSetupRepo, fn: func() { steps.SetupEnvironments(cfg, gh) }},
		{name: "Setup variables and secrets", phase: phaseSetupRepo, fn: func() { steps.SetupVariablesAndSecrets(cfg, gh) }},

		// PHASE: APPLY TEMPLATE — clone, templatize locally, push (even on failure)
		{name: "Clone repos", phase: phaseApplyTemplate, fn: func() { steps.CloneRepos(cfg, gh) }},
		{name: "Apply template", phase: phaseApplyTemplate, fn: func() { steps.ApplyTemplate(cfg) }},
		{name: "Replace repository references", phase: phaseApplyTemplate, fn: func() { steps.ReplaceRepoReferences(cfg) }},
		{name: "Replace namespaces", phase: phaseApplyTemplate, fn: func() { steps.ReplaceNamespaces(cfg) }},
		{name: "Replace system name", phase: phaseApplyTemplate, fn: func() { steps.ReplaceSystemName(cfg) }},
		{name: "Update README", phase: phaseApplyTemplate, fn: func() { steps.UpdateReadme(cfg) }},
		{name: "Write project config", phase: phaseApplyTemplate, fn: func() { steps.WriteProjectConfig(cfg) }},
		{name: "Write LICENSE", phase: phaseApplyTemplate, fn: func() { steps.WriteLicense(cfg) }},
		{name: "Create SonarCloud projects", phase: phaseApplyTemplate, fn: func() { steps.CreateSonarCloudProjects(cfg, sc) }},

		// Validation is grouped into the Apply template phase but runs last —
		// i.e. after Commit and push — so broken output is already visible in
		// the remote repo for troubleshooting.
		// TODO: re-enable after the template-name rewrite lands. The current rule sets flag
		// patterns (e.g. "system/monolith/" in docker-compose.local.monolith.*.yml) that the
		// upcoming rename will eliminate at the source, making these checks either trivially
		// satisfied or redundant. Rewriting the validators before the rename risks baking in
		// rules against names that are about to change.
		// {name: "Validate no leftover system names", phase: phaseApplyTemplate, fn: func() { steps.ValidateNoLeftoverSystemNames(cfg) }},
		// {name: "Validate no leftover shop template refs", phase: phaseApplyTemplate, fn: func() { steps.ValidateNoLeftoverShopRefs(cfg) }},
		// {name: "Validate no leftover template references", phase: phaseApplyTemplate, fn: func() { steps.ValidateNoLeftoverTemplateRefs(cfg) }},
	}

	// ATDD assets (agents, commands, prompts) get installed before Commit and
	// push so they end up in the initial scaffold commit, just like the
	// system code, workflows, and externals. Skipped when --no-atdd is set.
	if !cfg.NoAtdd {
		allSteps = append(allSteps, stepDef{
			name:  "Install ATDD assets",
			phase: phaseApplyTemplate,
			fn:    func() { installAtddDuringInit(cfg) },
		})
	}

	allSteps = append(allSteps, stepDef{
		name:      "Commit and push",
		phase:     phaseApplyTemplate,
		alwaysRun: true,
		fn:        func() { steps.CommitAndPush(cfg, *failureNote) },
	})

	allSteps = append(allSteps, buildVerifySteps(cfg, gh)...)

	allSteps = append(allSteps,
		stepDef{name: "Print project registration", phase: phaseFinalize, fn: func() { steps.PrintRegistration(cfg) }},
	)

	return allSteps
}

// verifyLevelOrder maps --verify-level values to a rank. A step runs when the
// configured level's rank is >= the step's minimum rank.
var verifyLevelOrder = map[string]int{
	"none":       0,
	"local":      1,
	"commit":     2,
	"acceptance": 3,
	"qa":         4,
	"release":    5,
}

// buildVerifySteps assembles the verify pipeline in a fixed order that mirrors
// the CI pipeline stages: local compile → local tests → local sonar → commit →
// acceptance (latest + legacy in parallel) → QA (stage + signoff) → production
// → bump-patch-version (called downstream of prod-stage, verified explicitly)
// → cleanup (dry-run, schedule-only workflows only fire when init runs).
// Each step is gated by --verify-level; local tests and local sonar can
// additionally be skipped with --no-local-tests / --no-local-sonar, and legacy
// is dropped everywhere via --no-legacy.
func buildVerifySteps(cfg *config.Config, gh *shell.GitHub) []stepDef {
	level := verifyLevelOrder[cfg.VerifyLevel]
	if level == 0 {
		log.Infof("Skipping workflow verification (--verify-level none)")
		return nil
	}

	var s []stepDef

	// Step 1: Local compilation — runs at every level above none.
	s = append(s,
		stepDef{name: "Verify local compilation", phase: phaseVerifyLocal, fn: func() { steps.VerifyCompilation(cfg) }},
		stepDef{name: "Verify scaffolded workflows", phase: phaseVerifyLocal, fn: func() { steps.VerifyScaffoldWorkflows(cfg) }},
	)

	// Step 2: Local testing — runner package against system-test/ (latest + legacy).
	if !cfg.NoLocalTests {
		s = append(s,
			stepDef{name: "Verify local testing", phase: phaseVerifyLocal, fn: func() { steps.VerifyLocalTesting(cfg) }},
		)
	}

	// Step 3: Local SonarCloud scan — per-component Run-Sonar.ps1 against the
	// SonarCloud project created in the apply-template phase.
	if !cfg.NoLocalSonar {
		s = append(s,
			stepDef{name: "Verify local SonarCloud scan", phase: phaseVerifyLocal, fn: func() { steps.VerifyLocalSonar(cfg) }},
		)
	}

	// Step 4: Commit stage CI.
	if level >= verifyLevelOrder["commit"] {
		s = append(s,
			stepDef{name: "Verify commit stage", phase: phaseVerifyCommit, fn: func() { steps.VerifyCommitStage(cfg, gh) }},
		)
	}

	// Step 5: Acceptance stage CI (latest + legacy parallel, legacy optional).
	if level >= verifyLevelOrder["acceptance"] {
		s = append(s,
			stepDef{name: "Verify acceptance stage", phase: phaseVerifyAcceptance, fn: func() { steps.VerifyAcceptanceStages(cfg, gh) }},
		)
	}

	// Step 6: QA stage CI (stage + signoff).
	if level >= verifyLevelOrder["qa"] {
		s = append(s,
			stepDef{name: "Verify QA stage", phase: phaseVerifyQA, fn: func() { steps.VerifyQA(cfg, gh) }},
		)
	}

	// Step 7: Production stage CI.
	if level >= verifyLevelOrder["release"] {
		s = append(s,
			stepDef{name: "Verify production stage", phase: phaseVerifyProduction, fn: func() { steps.VerifyProdStage(cfg, gh) }},
		)
	}

	// Step 8: bump-patch-version (post-release VERSION bump). Invoked as a
	// downstream called workflow by prod-stage; we verify explicitly here for
	// clearer failure attribution. Skipped in multirepo (bump deferred).
	if level >= verifyLevelOrder["release"] {
		s = append(s,
			stepDef{name: "Verify bump-patch-version workflow", phase: phaseVerifyProduction, fn: func() { steps.VerifyBumpPatchVersion(cfg, gh) }},
		)
	}

	// Step 9: Cleanup workflow (dry-run). cleanup.yml otherwise only fires on
	// schedule, so a syntax error or stale action reference would silently slip
	// past init and surface the next night.
	if level >= verifyLevelOrder["release"] {
		s = append(s,
			stepDef{name: "Verify cleanup workflow", phase: phaseVerifyProduction, fn: func() { steps.VerifyCleanup(cfg, gh) }},
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
		fileIt := false
		if cfg.BugReport {
			fileIt = confirmBugReport(cfg)
		} else {
			fileIt = offerBugReport(cfg)
		}
		if fileIt {
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

// confirmBugReport shows what will be sent and asks the user to confirm.
// Returns true only on an explicit "y"/"yes". On a non-tty (CI, piped input)
// the opt-in flag alone is treated as consent and it returns true without
// prompting.
func confirmBugReport(cfg *config.Config) bool {
	fmt.Println()
	fmt.Println(separator)
	fmt.Println("  Bug report — review before filing")
	fmt.Println(separator)
	fmt.Println()
	fmt.Println("  On confirm, the following will be filed upstream:")
	fmt.Println()
	fmt.Printf("  - Issue created at: https://github.com/optivem/gh-optivem/issues\n")
	fmt.Printf("  - Linking to your repo: https://github.com/%s\n", cfg.FullRepo)
	fmt.Printf("  - Log file: %s\n", cfg.LogFile)
	fmt.Println()

	if cfg.AssumeYes {
		log.Infof("Proceeding without confirmation (--yes).")
		return true
	}

	if !isInteractive() {
		log.Infof("Non-interactive session detected; proceeding with --report-bug opt-in.")
		return true
	}

	fmt.Print("  Proceed? [y/N]: ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		log.Warnf("Could not read confirmation: %v. Skipping bug report.", err)
		return false
	}
	resp := strings.ToLower(strings.TrimSpace(line))
	return resp == "y" || resp == "yes"
}

// offerBugReport prompts the user (after a failure) whether they want to file
// a bug report. Used when --report-bug was NOT set explicitly — opt-out by
// default. Skipped without prompting in unattended modes (--yes or
// non-interactive stdin), so failures in CI don't pester or auto-file silently.
// Shows the same "what will be sent" details as confirmBugReport so the user
// can decide informed before answering.
func offerBugReport(cfg *config.Config) bool {
	if cfg.AssumeYes {
		return false // unattended: don't pester; user must opt in via --report-bug
	}
	if !isInteractive() {
		return false // can't prompt
	}

	fmt.Println()
	fmt.Println(separator)
	fmt.Println("  Bug report — share this failure with the maintainers?")
	fmt.Println(separator)
	fmt.Println()
	fmt.Println("  On confirm, the following will be filed upstream:")
	fmt.Println()
	fmt.Printf("  - Issue created at: https://github.com/optivem/gh-optivem/issues\n")
	fmt.Printf("  - Linking to your repo: https://github.com/%s\n", cfg.FullRepo)
	fmt.Printf("  - Log file: %s\n", cfg.LogFile)
	fmt.Println()
	fmt.Print("  File a bug report? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		log.Warnf("Could not read confirmation: %v. Skipping bug report.", err)
		return false
	}
	resp := strings.ToLower(strings.TrimSpace(line))
	return resp == "y" || resp == "yes"
}

func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// readLogForBugReport returns the log contents to embed in a bug-report body,
// capped at bugReportLogMaxBytes so the issue body stays under GitHub's
// 65 KB limit. When the log is small enough, returns the whole thing
// (truncated=false). Otherwise returns the trailing bugReportLogMaxBytes,
// trimmed forward to the next newline so embedding never starts mid-line
// (truncated=true). Returns ("", false) when the log is unreadable or empty.
func readLogForBugReport(path string) (content string, truncated bool) {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return "", false
	}
	if len(data) <= bugReportLogMaxBytes {
		return strings.TrimRight(string(data), "\n"), false
	}
	tail := data[len(data)-bugReportLogMaxBytes:]
	if idx := bytes.IndexByte(tail, '\n'); idx != -1 && idx < len(tail)-1 {
		tail = tail[idx+1:]
	}
	return strings.TrimRight(string(tail), "\n"), true
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
	if content, truncated := readLogForBugReport(cfg.LogFile); content != "" {
		summary := fmt.Sprintf("Log (%s)", cfg.LogFile)
		if truncated {
			summary = fmt.Sprintf("Last %d KB of log — truncated; full log at %s",
				bugReportLogMaxBytes/1024, cfg.LogFile)
		}
		body += fmt.Sprintf("\n<details>\n<summary>%s</summary>\n\n```\n%s\n```\n</details>\n",
			summary, content)
	}

	bodyFile, err := os.CreateTemp("", "gh-issue-body-*.md")
	if err != nil {
		log.Infof("WARN: Failed to create temp file for bug report: %v", err)
		return
	}
	defer os.Remove(bodyFile.Name())
	bodyFile.WriteString(body)
	bodyFile.Close()

	out, err := shell.Run(
		fmt.Sprintf(`gh issue create --repo optivem/gh-optivem --title %q --body-file %s`, title, bodyFile.Name()),
		false, false, "")
	if err != nil {
		log.Infof("WARN: Failed to create bug report: %v\n%s", err, out)
	} else {
		log.Successf("Bug report created: %s", strings.TrimSpace(out))
	}
}

// checkForUpdate queries the latest release and, when the running binary is
// outdated, prints a notice telling the user to run `gh optivem upgrade`.
// Notify-only — never auto-upgrades; users decide when to upgrade.
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

	log.Warnf("Update available: %s → %s. Run `gh optivem upgrade` to upgrade.", version.Version, latest)
}

func printBanner(cfg *config.Config) {
	fmt.Println()
	fmt.Println(separator)
	fmt.Println("  Pipeline Project Setup")
	fmt.Println(separator)

	fmt.Println()
	fmt.Println("  Inputs")
	fmt.Println()
	tag := func(name string) string {
		if cfg.UserSetFlags[name] {
			return ""
		}
		return " (default)"
	}
	log.Infof("--owner:           %s%s", cfg.Owner, tag("owner"))
	log.Infof("--repo:            %s%s", cfg.Raw.Repo, tag("repo"))
	log.Infof("--system-name:     %s%s", cfg.SystemName, tag("system-name"))
	log.Infof("--arch:            %s%s", cfg.Arch, tag("arch"))
	log.Infof("--repo-strategy:   %s%s", cfg.RepoStrategy, tag("repo-strategy"))
	log.Infof("--monolith-lang:   %s%s", cfg.Lang, tag("monolith-lang"))
	log.Infof("--backend-lang:    %s%s", cfg.BackendLang, tag("backend-lang"))
	log.Infof("--frontend-lang:   %s%s", cfg.FrontendLang, tag("frontend-lang"))
	log.Infof("--test-lang:       %s%s", cfg.Raw.TestLang, tag("test-lang"))
	log.Infof("--license:         %s%s", cfg.License, tag("license"))
	log.Infof("--deploy:          %s%s", cfg.Deploy, tag("deploy"))
	log.Infof("--verify-level:      %s%s", cfg.Raw.VerifyLevel, tag("verify-level"))
	log.Infof("--no-legacy:         %v%s", cfg.NoLegacy, tag("no-legacy"))
	log.Infof("--no-local-tests:    %v%s", cfg.NoLocalTests, tag("no-local-tests"))
	log.Infof("--no-local-sonar:    %v%s", cfg.NoLocalSonar, tag("no-local-sonar"))
	log.Infof("--shop-ref:        %s%s", cfg.Raw.ShopRef, tag("shop-ref"))
	log.Infof("--dry-run:         %v%s", cfg.DryRun, tag("dry-run"))
	log.Infof("--keep-local:      %v%s", cfg.Raw.KeepLocal, tag("keep-local"))
	log.Infof("--report-bug:      %v%s", cfg.BugReport, tag("report-bug"))
	log.Infof("--yes:             %v%s", cfg.AssumeYes, tag("yes"))
	log.Infof("--workdir:         %s%s", cfg.Raw.WorkDir, tag("workdir"))
	log.Infof("--log-file:        %s%s", cfg.LogFile, tag("log-file"))
	log.Infof("--verbose:         %v%s", cfg.Verbose, tag("verbose"))
	log.Infof("--quiet:           %v%s", cfg.Quiet, tag("quiet"))

	fmt.Println()
	fmt.Println("  Derived values")
	fmt.Println()
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
	log.Infof("Cleanup local:   %v  (--keep-local to keep)", !cfg.KeepLocal)
	log.Infof("System pascal:   %s", cfg.SysNamePascalNew)
	log.Infof("System camel:    %s", cfg.SysNameCamelNew)
	log.Infof("System kebab:    %s", cfg.SysNameKebabNew)
	log.Infof("System lower:    %s", cfg.SysNameLowerNew)
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
	log.Infof("Variables:       DOCKERHUB_USERNAME")
	log.Infof("Secrets:         %s", strings.Join(willCreateSecrets(cfg), ", "))

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
// in the order SetupVariablesAndSecrets writes them.
func willCreateSecrets(cfg *config.Config) []string {
	return []string{"DOCKERHUB_TOKEN", "SONAR_TOKEN", "WORKFLOW_TOKEN", "GHCR_TOKEN"}
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
