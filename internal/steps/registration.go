package steps

import (
	"fmt"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/log"
)

// PrintRegistration outputs structured project registration info that can be
// copy-pasted into the course registration form or tracking sheet.
func PrintRegistration(cfg *config.Config) {
	log.Info("Generating project registration info...")

	lang := cfg.Lang
	if cfg.Arch == "multitier" {
		lang = cfg.BackendLang + " + " + cfg.FrontendLang
	}

	repoURL := fmt.Sprintf("https://github.com/%s", cfg.FullRepo)
	actionsURL := fmt.Sprintf("https://github.com/%s/actions", cfg.FullRepo)
	sonarURL := fmt.Sprintf("https://sonarcloud.io/project/overview?id=%s_%s-system", cfg.Owner, cfg.Repo)
	if cfg.Arch == "multitier" {
		sonarURL = fmt.Sprintf("https://sonarcloud.io/organizations/%s/projects", cfg.OwnerLower)
	}

	fmt.Println()
	fmt.Println("------------------------------------------")
	fmt.Println("  Project Registration Info")
	fmt.Println("------------------------------------------")
	fmt.Println()
	fmt.Printf("  Owner:         %s\n", cfg.Owner)
	fmt.Printf("  System Name:   %s\n", cfg.SystemName)
	fmt.Printf("  Architecture:  %s\n", cfg.Arch)
	fmt.Printf("  Language:      %s\n", lang)
	fmt.Printf("  Repo Strategy: %s\n", cfg.RepoStrategy)
	fmt.Println()
	fmt.Printf("  Repository:    %s\n", repoURL)
	fmt.Printf("  Actions:       %s\n", actionsURL)
	fmt.Printf("  SonarCloud:    %s\n", sonarURL)

	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "multitier" {
			fmt.Printf("  Backend:       https://github.com/%s\n", cfg.BackendFullRepo)
			fmt.Printf("  Frontend:      https://github.com/%s\n", cfg.FrontendFullRepo)
		} else {
			fmt.Printf("  System:        https://github.com/%s\n", cfg.SystemFullRepo)
		}
	}

	fmt.Println()
	fmt.Println("------------------------------------------")
	fmt.Println()

	log.Success("Project registration info printed above — copy-paste into your registration form.")
}
