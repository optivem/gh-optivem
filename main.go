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

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/log"
	"github.com/optivem/gh-optivem/internal/shell"
	"github.com/optivem/gh-optivem/internal/steps"
)

type stepDef struct {
	name string
	fn   func()
}

func main() {
	cfg := config.ParseAndValidate()

	gh := shell.NewGitHub(cfg)
	sc := shell.NewSonarCloud(cfg.SonarToken, cfg.OwnerLower)

	printBanner(cfg)

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
		{"Create SonarCloud projects", func() { steps.CreateSonarCloudProjects(cfg, sc) }},
		{"Commit and push", func() { steps.CommitAndPush(cfg) }},
		{"Enable GitHub Pages", func() { steps.EnablePages(cfg, gh) }},
		{"Verify commit stage", func() { steps.VerifyCommitStage(cfg, gh) }},
		{"Verify acceptance stage", func() { steps.VerifyAcceptanceStage(cfg, gh) }},
		{"Verify acceptance stage legacy", func() { steps.VerifyAcceptanceStageLegacy(cfg, gh) }},
		{"Verify QA stage", func() { steps.VerifyQAStage(cfg, gh) }},
		{"Verify QA signoff", func() { steps.VerifyQASignoff(cfg, gh) }},
		{"Verify production stage", func() { steps.VerifyProdStage(cfg, gh) }},
	}

	errors := 0
	for _, s := range allSteps {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Failf("Step failed: %s -- %v", s.name, r)
					errors++
				}
			}()
			s.fn()
		}()
		if errors > 0 {
			break
		}
	}

	fmt.Println()
	fmt.Println("==========================================")
	if errors > 0 {
		log.Failf("Setup completed with %d error(s)", errors)
	} else {
		log.OK("All steps passed!")
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

	// Cleanup (test mode only) — skip on failure so repo can be inspected
	if errors > 0 && !cfg.ForceCleanup {
		cfg.Cleanup = "no"
	}
	steps.Cleanup(cfg, gh, sc)

	if errors > 0 {
		os.Exit(1)
	}
}

func printBanner(cfg *config.Config) {
	fmt.Println()
	fmt.Println("==========================================")
	fmt.Println("  Pipeline Project Setup")
	fmt.Println("==========================================")
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
