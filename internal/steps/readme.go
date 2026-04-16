package steps

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/log"
)

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
