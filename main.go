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

type stepDef struct {
	name string
	fn   func()
}

func main() {
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
	// Check for updates (non-blocking warning)
	checkForUpdate()

	cfg := config.ParseAndValidate()

	gh := shell.NewGitHub(cfg)
	sc := shell.NewSonarCloud(cfg.SonarToken, cfg.OwnerLower)

	printBanner(cfg)

	allSteps := buildSteps(cfg, gh, sc)
	errors, totalDuration := executeSteps(allSteps)
	printSummary(cfg, errors, totalDuration)

	// Cleanup (test mode only) — skip on failure so repo can be inspected
	if errors > 0 && !cfg.ForceCleanup {
		cfg.Cleanup = "no"
	}
	steps.Cleanup(cfg, gh, sc)

	if errors > 0 {
		os.Exit(1)
	}
}

func buildSteps(cfg *config.Config, gh *shell.GitHub, sc *shell.SonarCloud) []stepDef {
	allSteps := []stepDef{
		{"Create repositories", func() { steps.CreateRepos(cfg, gh) }},
		{"Setup environments", func() { steps.SetupEnvironments(cfg, gh) }},
		{"Setup secrets and variables", func() { steps.SetupSecretsAndVariables(cfg, gh) }},
		{"Clone repos", func() { steps.CloneRepos(cfg, gh) }},
		{"Apply template", func() { steps.ApplyTemplate(cfg) }},
		{"Replace repository references", func() { steps.ReplaceRepoReferences(cfg) }},
		{"Replace namespaces", func() { steps.ReplaceNamespaces(cfg) }},
		{"Replace system name", func() { steps.ReplaceSystemName(cfg) }},
		{"Update README", func() { steps.UpdateReadme(cfg) }},
		{"Write project config", func() { steps.WriteProjectConfig(cfg) }},
		{"Create SonarCloud projects", func() { steps.CreateSonarCloudProjects(cfg, sc) }},
		{"Commit and push", func() { steps.CommitAndPush(cfg) }},
		{"Enable GitHub Pages", func() { steps.EnablePages(cfg, gh) }},
		{"Verify compilation", func() { steps.VerifyCompilation(cfg) }},
	}

	allSteps = append(allSteps, buildVerifySteps(cfg, gh)...)

	allSteps = append(allSteps,
		stepDef{"Print project registration", func() { steps.PrintRegistration(cfg) }},
	)

	return allSteps
}

func buildVerifySteps(cfg *config.Config, gh *shell.GitHub) []stepDef {
	if cfg.VerifyLevel == "none" {
		log.Logf("Skipping workflow verification (--verify-level none)")
		return nil
	}

	var s []stepDef

	// smoke tier — local smoke tests only, no CI
	if cfg.VerifyLevel == "local" {
		s = append(s,
			stepDef{"Run local smoke tests", func() { steps.RunLocalSystemTests(cfg) }},
		)
	}

	// commit tier and above — CI workflow verification
	if cfg.VerifyLevel == "commit" || cfg.VerifyLevel == "acceptance" || cfg.VerifyLevel == "release" {
		s = append(s,
			stepDef{"Verify commit stage", func() { steps.VerifyCommitStage(cfg, gh) }},
		)
	}

	if cfg.VerifyLevel == "acceptance" || cfg.VerifyLevel == "release" {
		s = append(s,
			stepDef{"Verify acceptance stage", func() { steps.VerifyAcceptanceStage(cfg, gh) }},
		)
		if !cfg.ExcludeLegacy {
			s = append(s,
				stepDef{"Verify acceptance stage legacy", func() { steps.VerifyAcceptanceStageLegacy(cfg, gh) }},
			)
		}
		s = append(s,
			stepDef{"Run local system tests", func() { steps.RunLocalSystemTests(cfg) }},
		)
	}

	if cfg.VerifyLevel == "release" {
		s = append(s,
			stepDef{"Verify QA stage", func() { steps.VerifyQAStage(cfg, gh) }},
			stepDef{"Verify QA signoff", func() { steps.VerifyQASignoff(cfg, gh) }},
			stepDef{"Verify production stage", func() { steps.VerifyProdStage(cfg, gh) }},
		)
	}

	return s
}

func executeSteps(allSteps []stepDef) (int, time.Duration) {
	errors := 0
	totalStart := time.Now()
	for i, s := range allSteps {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Failf("Step failed: %s -- %v", s.name, r)
					errors++
				}
			}()
			stepStart := time.Now()
			s.fn()
			log.OKf("Step %d done (%s)", i+1, formatDuration(time.Since(stepStart)))
		}()
		if errors > 0 {
			break
		}
	}
	return errors, time.Since(totalStart)
}

func printSummary(cfg *config.Config, errors int, totalDuration time.Duration) {
	fmt.Println()
	fmt.Println(separator)
	if errors > 0 {
		log.Failf("Setup completed with %d error(s) in %s", errors, formatDuration(totalDuration))
		if !cfg.NoBugReport {
			createBugReport(cfg, errors)
		}
	} else {
		log.OKf("All steps passed! Completed in %s", formatDuration(totalDuration))
	}
	fmt.Println()
	fmt.Printf("  System:     %s\n", cfg.SystemName)
	fmt.Printf("  Repository: https://github.com/%s\n", cfg.FullRepo)
	fmt.Printf("  Actions:    https://github.com/%s/actions\n", cfg.FullRepo)
	fmt.Printf("  Docs:       https://%s.github.io/%s/\n", cfg.OwnerLower, cfg.Repo)
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
		log.Logf("WARN: Failed to create temp file for bug report: %v", err)
		return
	}
	defer os.Remove(bodyFile.Name())
	bodyFile.WriteString(body)
	bodyFile.Close()

	out, err := shell.Run(
		fmt.Sprintf(`gh issue create --repo optivem/gh-optivem --title %q --body-file %s --assignee valentinajemuovic`, title, bodyFile.Name()),
		false, false, "")
	if err != nil {
		log.Logf("WARN: Failed to create bug report: %v\n%s", err, out)
	} else {
		log.OKf("Bug report created: %s", strings.TrimSpace(out))
	}
}

func checkForUpdate() {
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

	fmt.Fprintf(os.Stderr, "\n%sUPDATE AVAILABLE:%s You are running %s, but %s is available.\n", "\033[0;33m", "\033[0m", version.Version, latest)
	fmt.Fprintf(os.Stderr, "  Run: gh extension upgrade optivem\n\n")
}

func printBanner(cfg *config.Config) {
	fmt.Println()
	fmt.Println(separator)
	fmt.Println("  Pipeline Project Setup")
	fmt.Println(separator)
	fmt.Println()
	log.Logf("Owner:       %s", cfg.Owner)
	log.Logf("Repo:        %s", cfg.Repo)
	log.Logf("System:      %s", cfg.SystemName)
	log.Logf("Arch:        %s", cfg.Arch)
	if cfg.Arch == "monolith" {
		log.Logf("Language:    %s", cfg.Lang)
		if cfg.RepoStrategy == "multirepo" {
			log.Logf("System repo: %s", cfg.SystemFullRepo)
		}
	} else {
		log.Logf("Backend:     %s", cfg.BackendLang)
		log.Logf("Frontend:    %s", cfg.FrontendLang)
		if cfg.RepoStrategy == "multirepo" {
			log.Logf("Backend repo: %s", cfg.BackendFullRepo)
			log.Logf("Frontend repo: %s", cfg.FrontendFullRepo)
		}
	}
	log.Logf("Test lang:   %s", cfg.TestLang)
	log.Logf("Dry run:     %v", cfg.DryRun)
	log.Logf("Test mode:   %v", cfg.TestMode)
	log.Logf("Workdir:     %s", cfg.WorkDir)
	fmt.Println()
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
