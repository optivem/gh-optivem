package steps

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/log"
)

const (
	readmeFile      = "README.md"
	projectInfoHdr  = "## Project Info\n\n"
	workflowPath    = "/actions/workflows"
	githubBaseURL   = "https://github.com/"
	testLangFmt     = "- **Test language:** %s\n"
	badgeFmt        = "[![%s](%s/badge.svg)](%s)\n"
	acceptanceStage       = "acceptance-stage"
	acceptanceStageLegacy = "acceptance-stage-legacy"
	qaStage               = "qa-stage"
	qaSignoff       = "qa-signoff"
	prodStage       = "prod-stage"
	commitStage     = "commit-stage"
)

func actionsBase(fullRepo string) string {
	return githubBaseURL + fullRepo + workflowPath
}

func writeProjectInfoCommon(info *strings.Builder, cfg *config.Config) {
	fmt.Fprintf(info, projectInfoHdr)
	fmt.Fprintf(info, "- **Owner:** %s\n", cfg.Owner)
	fmt.Fprintf(info, "- **System:** %s\n", cfg.SystemName)
}

func writeReadmeFile(dir, content string) {
	os.WriteFile(filepath.Join(dir, readmeFile), []byte(content), 0644)
}

func readmeFooter(licenseName, owner string) string {
	return fmt.Sprintf("## License\n\n%s\n\n## Contributors\n\n- [%s](%s%s)\n",
		licenseName, owner, githubBaseURL, owner)
}

// docsSection returns the Documentation section for scaffolded repos that
// receive the /docs folder (monolith/multitier monorepos and multirepo roots).
func docsSection() string {
	return "## Documentation\n\n" +
		"- [Architecture](docs/architecture.md)\n" +
		"- [Use Cases](docs/use-cases.md)\n" +
		"- [Use Case Narrative](docs/use-case-narrative.md)\n" +
		"- [Project Registration](docs/project-registration.md)\n\n"
}

// pipelineBadges returns badge [url, label] pairs for the shared pipeline stages.
func pipelineBadges(base, deploy string) [][2]string {
	badges := [][2]string{
		{base + "/" + acceptanceStage + ".yml", acceptanceStage},
	}
	if deploy == "docker" {
		badges = append(badges, [2]string{base + "/" + acceptanceStageLegacy + ".yml", acceptanceStageLegacy})
	}
	badges = append(badges,
		[2]string{base + "/" + qaStage + ".yml", qaStage},
		[2]string{base + "/" + qaSignoff + ".yml", qaSignoff},
		[2]string{base + "/" + prodStage + ".yml", prodStage},
	)
	return badges
}

func writeBadges(b *strings.Builder, items [][2]string) {
	for _, item := range items {
		fmt.Fprintf(b, badgeFmt, item[1], item[0], item[0])
	}
}

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
	writeProjectInfoCommon(&info, cfg)
	fmt.Fprintf(&info, "- **Architecture:** %s\n", cfg.Arch)
	fmt.Fprintf(&info, "- **Repo strategy:** %s\n", cfg.RepoStrategy)
	if cfg.Arch == "monolith" {
		fmt.Fprintf(&info, "- **Language:** %s\n", cfg.Lang)
	} else {
		fmt.Fprintf(&info, "- **Backend language:** %s\n", cfg.BackendLang)
		fmt.Fprintf(&info, "- **Frontend language:** %s\n", cfg.FrontendLang)
	}
	if cfg.TestLang != cfg.EffectiveLang() {
		fmt.Fprintf(&info, testLangFmt, cfg.TestLang)
	}
	fmt.Fprintf(&info, "\n")

	content := fmt.Sprintf("# %s\n\n%s\n%s%s%s",
		title, badges, info.String(), docsSection(), readmeFooter(cfg.LicenseName(), cfg.Owner))
	writeReadmeFile(repoDir, content)
}

func writeMonolithMultirepoReadme(cfg *config.Config) {
	base := actionsBase(cfg.FullRepo)
	systemBase := actionsBase(cfg.SystemFullRepo)

	var badges strings.Builder
	writeBadges(&badges, [][2]string{
		{systemBase + "/commit-stage.yml", commitStage},
	})
	writeBadges(&badges, pipelineBadges(base, cfg.Deploy))

	reposSection := fmt.Sprintf("## Repositories\n\n- [%s](%s%s) — System (%s)\n",
		cfg.SystemRepo, githubBaseURL, cfg.SystemFullRepo, cfg.Lang)

	var info strings.Builder
	writeProjectInfoCommon(&info, cfg)
	fmt.Fprintf(&info, "- **Architecture:** monolith\n")
	fmt.Fprintf(&info, "- **Repo strategy:** multirepo\n")
	fmt.Fprintf(&info, "- **Language:** %s\n", cfg.Lang)
	if cfg.TestLang != cfg.Lang {
		fmt.Fprintf(&info, testLangFmt, cfg.TestLang)
	}
	fmt.Fprintf(&info, "\n")

	content := fmt.Sprintf("# %s\n\n%s\n%s%s%s\n%s",
		cfg.SystemName, badges.String(), reposSection, info.String(), docsSection(), readmeFooter(cfg.LicenseName(), cfg.Owner))
	writeReadmeFile(cfg.RepoDir, content)

	writeComponentReadme(
		cfg.SystemRepoDir, cfg.SystemName, "System",
		cfg.SystemFullRepo, cfg.Lang, cfg.LicenseName(), cfg.Owner,
	)
}

func writeMultitierMultirepoReadme(cfg *config.Config) {
	bl, fl := cfg.BackendLang, cfg.FrontendLang
	base := actionsBase(cfg.FullRepo)
	backendBase := actionsBase(cfg.BackendFullRepo)
	frontendBase := actionsBase(cfg.FrontendFullRepo)

	var badges strings.Builder
	writeBadges(&badges, [][2]string{
		{backendBase + "/backend-commit-stage.yml", "backend-commit-stage"},
		{frontendBase + "/frontend-commit-stage.yml", "frontend-commit-stage"},
	})
	writeBadges(&badges, pipelineBadges(base, cfg.Deploy))

	reposSection := fmt.Sprintf("## Repositories\n\n- [%s](%s%s) — Backend (%s)\n- [%s](%s%s) — Frontend (%s)\n",
		cfg.BackendRepo, githubBaseURL, cfg.BackendFullRepo, bl,
		cfg.FrontendRepo, githubBaseURL, cfg.FrontendFullRepo, fl)

	var info strings.Builder
	writeProjectInfoCommon(&info, cfg)
	fmt.Fprintf(&info, "- **Architecture:** multitier\n")
	fmt.Fprintf(&info, "- **Repo strategy:** multirepo\n")
	fmt.Fprintf(&info, "- **Backend language:** %s\n", cfg.BackendLang)
	fmt.Fprintf(&info, "- **Frontend language:** %s\n", cfg.FrontendLang)
	if cfg.TestLang != cfg.BackendLang {
		fmt.Fprintf(&info, testLangFmt, cfg.TestLang)
	}
	fmt.Fprintf(&info, "\n")

	content := fmt.Sprintf("# %s\n\n%s\n%s%s%s\n%s",
		cfg.SystemName, badges.String(), reposSection, info.String(), docsSection(), readmeFooter(cfg.LicenseName(), cfg.Owner))
	writeReadmeFile(cfg.RepoDir, content)

	writeComponentReadme(
		cfg.BackendRepoDir, cfg.SystemName, "Backend",
		cfg.BackendFullRepo, bl, cfg.LicenseName(), cfg.Owner,
	)
	writeComponentReadme(
		cfg.FrontendRepoDir, cfg.SystemName, "Frontend",
		cfg.FrontendFullRepo, fl, cfg.LicenseName(), cfg.Owner,
	)
}

func writeComponentReadme(repoDir, systemName, componentLabel, fullRepo, _, licenseName, owner string) {
	wfName := strings.ToLower(componentLabel) + "-commit-stage.yml"
	if componentLabel == "System" {
		wfName = "commit-stage.yml"
	}
	base := actionsBase(fullRepo)

	var badges strings.Builder
	writeBadges(&badges, [][2]string{
		{base + "/" + wfName, commitStage},
	})

	content := fmt.Sprintf("# %s — %s\n\n%s\n%s",
		systemName, componentLabel, badges.String(), readmeFooter(licenseName, owner))
	writeReadmeFile(repoDir, content)
}

func generateBadges(cfg *config.Config) string {
	base := actionsBase(cfg.FullRepo)

	var commitStages [][2]string
	if cfg.Arch == "monolith" {
		commitStages = [][2]string{
			{base + "/commit-stage.yml", commitStage},
		}
	} else {
		commitStages = [][2]string{
			{base + "/backend-commit-stage.yml", "backend-commit-stage"},
			{base + "/frontend-commit-stage.yml", "frontend-commit-stage"},
		}
	}

	var b strings.Builder
	writeBadges(&b, commitStages)
	writeBadges(&b, pipelineBadges(base, cfg.Deploy))
	return b.String()
}
