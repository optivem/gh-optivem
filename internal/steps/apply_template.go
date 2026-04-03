package steps

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/files"
	"github.com/optivem/gh-optivem/internal/log"
	"github.com/optivem/gh-optivem/internal/templates"
)

// ApplyTemplate copies template files into the cloned repo(s).
func ApplyTemplate(cfg *config.Config) {
	log.Log("Step 5: Applying template files...")

	if cfg.DryRun {
		log.Log("[DRY RUN] Would apply template files")
		return
	}

	EnsureWorkflowDir(cfg.RepoDir)

	if cfg.Arch == "monolith" {
		if cfg.RepoStrategy == "monorepo" {
			applyMonolithMonorepo(cfg)
		} else {
			applyMonolithMultirepo(cfg)
		}
	} else {
		if cfg.RepoStrategy == "monorepo" {
			applyMultitierMonorepo(cfg)
		} else {
			applyMultitierMultirepo(cfg)
		}
	}

	log.OK("Applied template files")
}

// ── Monolith Monorepo ──────────────────────────────────────────────────────

func applyMonolithMonorepo(cfg *config.Config) {
	lang := cfg.Lang
	testLang := cfg.TestLang
	starter := cfg.StarterPath
	repoDir := cfg.RepoDir

	// Workflows: rename to language-agnostic names
	wfMap := map[string]string{
		"monolith-" + lang + "-commit-stage.yml":         "commit-stage.yml",
		"monolith-" + testLang + "-acceptance-stage.yml":  "acceptance-stage.yml",
		"monolith-" + testLang + "-qa-stage.yml":          "qa-stage.yml",
		"monolith-" + testLang + "-qa-signoff.yml":        "qa-signoff.yml",
		"monolith-" + testLang + "-prod-stage.yml":        "prod-stage.yml",
	}
	templates.CopyWorkflows(wfMap, starter, repoDir)

	// System code: system/monolith/{lang}/ -> system/
	files.CopyDir(
		filepath.Join(starter, "system", "monolith", lang),
		filepath.Join(repoDir, "system"),
	)

	// External system simulators (needed for system test docker-compose)
	for _, dir := range []string{"external-real-sim", "external-stub"} {
		src := filepath.Join(starter, "system", dir)
		if _, err := os.Stat(src); err == nil {
			files.CopyDir(src, filepath.Join(repoDir, dir))
		}
	}

	// System tests: system-test/{testLang}/ -> system-test/
	testDst := filepath.Join(repoDir, "system-test")
	files.CopyDir(filepath.Join(starter, "system-test", testLang), testDst)
	templates.SelectDockerCompose(testDst, "single")
	templates.CopyVersion(starter, repoDir)

	// Fix workflow content: paths, image names, workflow names
	contentReplacements := monolithContentReplacements(lang, testLang)
	templates.FixupWorkflowContent(repoDir, contentReplacements)
	templates.FixupDockerComposeContent(repoDir, monolithDockerComposeReplacements(lang, testLang))

	// Fix SonarCloud key suffixes in build files (build.gradle, .csproj, etc.)
	templates.FixupAllTextFiles(repoDir, monolithSonarKeyReplacements(lang))

	// Docs templates
	copyDocs(starter, repoDir, "monolith")

	log.OK("Applied template files (monolith monorepo)")
}

// ── Monolith Multirepo ─────────────────────────────────────────────────────

func applyMonolithMultirepo(cfg *config.Config) {
	lang := cfg.Lang
	testLang := cfg.TestLang
	starter := cfg.StarterPath
	repoDir := cfg.RepoDir
	systemDir := cfg.SystemRepoDir

	// Root repo: pipeline stage workflows + system-test
	rootWfMap := map[string]string{
		"monolith-" + testLang + "-acceptance-stage.yml": "acceptance-stage.yml",
		"monolith-" + testLang + "-qa-stage.yml":         "qa-stage.yml",
		"monolith-" + testLang + "-qa-signoff.yml":       "qa-signoff.yml",
		"monolith-" + testLang + "-prod-stage.yml":       "prod-stage.yml",
	}
	templates.CopyWorkflows(rootWfMap, starter, repoDir)

	// External system simulators
	for _, dir := range []string{"external-real-sim", "external-stub"} {
		src := filepath.Join(starter, "system", dir)
		if _, err := os.Stat(src); err == nil {
			files.CopyDir(src, filepath.Join(repoDir, dir))
		}
	}

	testDst := filepath.Join(repoDir, "system-test")
	files.CopyDir(filepath.Join(starter, "system-test", testLang), testDst)
	templates.SelectDockerCompose(testDst, "single")
	templates.CopyVersion(starter, repoDir)

	// Fix root repo workflow content
	contentReplacements := monolithContentReplacements(lang, testLang)
	templates.FixupWorkflowContent(repoDir, contentReplacements)
	templates.FixupDockerComposeContent(repoDir, monolithDockerComposeReplacements(lang, testLang))

	// Fix SonarCloud key suffixes in build files
	templates.FixupAllTextFiles(repoDir, monolithSonarKeyReplacements(lang))

	// Docs templates
	copyDocs(starter, repoDir, "monolith")

	// Fix multirepo image URLs and tokens
	templates.FixupMonolithMultirepoImageURLs(repoDir, cfg.SystemRepo)
	templates.FixupMultirepoToken(repoDir)
	log.OK("Applied root repo template (monolith multirepo)")

	// System repo: system code + commit stage
	EnsureWorkflowDir(systemDir)

	// Copy system code into system repo root (contents of monolith/{lang}/ -> repo root files)
	systemSrc := filepath.Join(starter, "system", "monolith", lang)
	entries, _ := os.ReadDir(systemSrc)
	for _, e := range entries {
		src := filepath.Join(systemSrc, e.Name())
		dst := filepath.Join(systemDir, e.Name())
		if e.IsDir() {
			files.CopyDir(src, dst)
		} else {
			files.CopyFile(src, dst)
		}
	}

	systemWfMap := map[string]string{
		"monolith-" + lang + "-commit-stage.yml": "commit-stage.yml",
	}
	templates.CopyWorkflows(systemWfMap, starter, systemDir)

	// Fix system repo workflow content
	sysContentReplacements := [][2]string{
		{"monolith-" + lang + "-commit-stage", "commit-stage"},
		{"system/monolith/" + lang, "."},
		{"monolith-system-" + lang, "system"},
	}
	templates.FixupWorkflowContent(systemDir, sysContentReplacements)
	templates.FixupAllTextFiles(systemDir, monolithSonarKeyReplacements(lang))
	templates.FixupCommitStageForStandalone(systemDir, ".")
	log.OK("Applied system repo template (monolith multirepo)")
}

// ── Multitier Monorepo ─────────────────────────────────────────────────────

func applyMultitierMonorepo(cfg *config.Config) {
	backendLang := cfg.BackendLang
	frontendLang := cfg.FrontendLang
	testLang := cfg.TestLang
	starter := cfg.StarterPath
	repoDir := cfg.RepoDir

	// Workflows: rename to language-agnostic names
	wfMap := map[string]string{
		"multitier-backend-" + backendLang + "-commit-stage.yml":  "backend-commit-stage.yml",
		"multitier-frontend-" + frontendLang + "-commit-stage.yml": "frontend-commit-stage.yml",
		"multitier-" + testLang + "-acceptance-stage.yml":          "acceptance-stage.yml",
		"multitier-" + testLang + "-qa-stage.yml":                  "qa-stage.yml",
		"multitier-" + testLang + "-qa-signoff.yml":                "qa-signoff.yml",
		"multitier-" + testLang + "-prod-stage.yml":                "prod-stage.yml",
	}
	templates.CopyWorkflows(wfMap, starter, repoDir)

	// Backend code: system/multitier/backend-{lang}/ -> backend/
	backendSrc := filepath.Join(starter, "system", "multitier", "backend-"+backendLang)
	files.CopyDir(backendSrc, filepath.Join(repoDir, "backend"))
	log.OK("Applied backend template")

	// Frontend code: system/multitier/frontend-{lang}/ -> frontend/
	frontendSrc := filepath.Join(starter, "system", "multitier", "frontend-"+frontendLang)
	files.CopyDir(frontendSrc, filepath.Join(repoDir, "frontend"))
	log.OK("Applied frontend template")

	// Shared external system simulators -> top level
	for _, dir := range []string{"external-real-sim", "external-stub"} {
		src := filepath.Join(starter, "system", dir)
		if _, err := os.Stat(src); err == nil {
			files.CopyDir(src, filepath.Join(repoDir, dir))
		}
	}

	// System tests: system-test/{testLang}/ -> system-test/
	testDst := filepath.Join(repoDir, "system-test")
	files.CopyDir(filepath.Join(starter, "system-test", testLang), testDst)
	templates.SelectDockerCompose(testDst, "multi")
	templates.CopyVersion(starter, repoDir)

	// Fix workflow content: paths and image names
	contentReplacements := multitierContentReplacements(backendLang, frontendLang, testLang)
	templates.FixupWorkflowContent(repoDir, contentReplacements)
	templates.FixupDockerComposeContent(repoDir, multitierDockerComposeReplacements(backendLang, frontendLang, testLang))

	// Fix SonarCloud key suffixes in build files
	templates.FixupAllTextFiles(repoDir, multitierSonarKeyReplacements(backendLang, frontendLang))

	// Docs templates
	copyDocs(starter, repoDir, "multitier")

	log.OK("Applied template files (multitier monorepo)")
}

// ── Multitier Multirepo ────────────────────────────────────────────────────

func applyMultitierMultirepo(cfg *config.Config) {
	backendLang := cfg.BackendLang
	frontendLang := cfg.FrontendLang
	testLang := cfg.TestLang
	starter := cfg.StarterPath
	repoDir := cfg.RepoDir
	frontendDir := cfg.FrontendRepoDir
	backendDir := cfg.BackendRepoDir

	// Root repo: pipeline stage workflows + system-test + externals
	rootWfMap := map[string]string{
		"multitier-" + testLang + "-acceptance-stage.yml": "acceptance-stage.yml",
		"multitier-" + testLang + "-qa-stage.yml":         "qa-stage.yml",
		"multitier-" + testLang + "-qa-signoff.yml":       "qa-signoff.yml",
		"multitier-" + testLang + "-prod-stage.yml":       "prod-stage.yml",
	}
	templates.CopyWorkflows(rootWfMap, starter, repoDir)

	// Shared external system simulators
	for _, dir := range []string{"external-real-sim", "external-stub"} {
		src := filepath.Join(starter, "system", dir)
		if _, err := os.Stat(src); err == nil {
			files.CopyDir(src, filepath.Join(repoDir, dir))
		}
	}

	testDst := filepath.Join(repoDir, "system-test")
	files.CopyDir(filepath.Join(starter, "system-test", testLang), testDst)
	templates.SelectDockerCompose(testDst, "multi")
	templates.CopyVersion(starter, repoDir)

	// Fix root repo workflow content
	contentReplacements := multitierContentReplacements(backendLang, frontendLang, testLang)
	templates.FixupWorkflowContent(repoDir, contentReplacements)
	templates.FixupDockerComposeContent(repoDir, multitierDockerComposeReplacements(backendLang, frontendLang, testLang))

	// Fix SonarCloud key suffixes in build files
	templates.FixupAllTextFiles(repoDir, multitierSonarKeyReplacements(backendLang, frontendLang))

	// Docs templates
	copyDocs(starter, repoDir, "multitier")

	// Fix multirepo image URLs and tokens
	templates.FixupMultirepoImageURLs(repoDir, cfg.FrontendRepo, cfg.BackendRepo)
	templates.FixupMultirepoToken(repoDir)
	log.OK("Applied root repo template (multitier multirepo)")

	// Backend repo: code + commit stage
	EnsureWorkflowDir(backendDir)
	backendSrc := filepath.Join(starter, "system", "multitier", "backend-"+backendLang)
	entries, _ := os.ReadDir(backendSrc)
	for _, e := range entries {
		src := filepath.Join(backendSrc, e.Name())
		dst := filepath.Join(backendDir, e.Name())
		if e.IsDir() {
			files.CopyDir(src, dst)
		} else {
			files.CopyFile(src, dst)
		}
	}
	backendWfMap := map[string]string{
		"multitier-backend-" + backendLang + "-commit-stage.yml": "backend-commit-stage.yml",
	}
	templates.CopyWorkflows(backendWfMap, starter, backendDir)

	// Fix backend workflow content
	backendReplacements := [][2]string{
		{"multitier-backend-" + backendLang + "-commit-stage", "backend-commit-stage"},
		{"system/multitier/backend-" + backendLang, "."},
		{"multitier-backend-" + backendLang, "backend"},
	}
	templates.FixupWorkflowContent(backendDir, backendReplacements)
	templates.FixupAllTextFiles(backendDir, multitierSonarKeyReplacements(backendLang, frontendLang))
	templates.FixupCommitStageForStandalone(backendDir, ".")
	log.OK("Applied backend repo template")

	// Frontend repo: code + commit stage
	EnsureWorkflowDir(frontendDir)
	frontendSrc := filepath.Join(starter, "system", "multitier", "frontend-"+frontendLang)
	entries, _ = os.ReadDir(frontendSrc)
	for _, e := range entries {
		src := filepath.Join(frontendSrc, e.Name())
		dst := filepath.Join(frontendDir, e.Name())
		if e.IsDir() {
			files.CopyDir(src, dst)
		} else {
			files.CopyFile(src, dst)
		}
	}
	frontendWfMap := map[string]string{
		"multitier-frontend-" + frontendLang + "-commit-stage.yml": "frontend-commit-stage.yml",
	}
	templates.CopyWorkflows(frontendWfMap, starter, frontendDir)

	// Fix frontend workflow content
	frontendReplacements := [][2]string{
		{"multitier-frontend-" + frontendLang + "-commit-stage", "frontend-commit-stage"},
		{"system/multitier/frontend-" + frontendLang, "."},
		{"multitier-frontend-" + frontendLang, "frontend"},
	}
	templates.FixupWorkflowContent(frontendDir, frontendReplacements)
	templates.FixupAllTextFiles(frontendDir, multitierSonarKeyReplacements(backendLang, frontendLang))
	templates.FixupCommitStageForStandalone(frontendDir, ".")
	log.OK("Applied frontend repo template")
}

// ── Content replacement helpers ────────────────────────────────────────────

// monolithContentReplacements returns workflow content replacements for monolith.
func monolithContentReplacements(lang, testLang string) [][2]string {
	r := [][2]string{
		// Workflow names (longer patterns first to avoid partial matches)
		{"monolith-" + lang + "-commit-stage", "commit-stage"},
		{"monolith-" + testLang + "-acceptance-stage", "acceptance-stage"},
		{"monolith-" + testLang + "-qa-stage", "qa-stage"},
		{"monolith-" + testLang + "-qa-signoff", "qa-signoff"},
		{"monolith-" + testLang + "-prod-stage", "prod-stage"},
		{"monolith-" + testLang + "-verify", "verify"},
		// Working directory
		{"system/monolith/" + lang, "system"},
		// System-test path
		{"system-test/" + testLang + "/", "system-test/"},
		{"system-test/" + testLang, "system-test"},
		// Docker image names
		{"monolith-system-" + lang, "system"},
	}
	if lang != testLang {
		r = append(r, [2]string{"monolith-system-" + testLang, "system"})
	}
	return r
}

// monolithDockerComposeReplacements returns docker-compose content replacements for monolith.
func monolithDockerComposeReplacements(lang, testLang string) [][2]string {
	r := [][2]string{
		{"system-test/" + testLang + "/", "system-test/"},
		{"system-test/" + testLang, "system-test"},
		{"monolith-system-" + lang, "system"},
		// Volume mount paths: old layout had system-test/{lang}/, new has system-test/
		{"../../system/external-real-sim", "../external-real-sim"},
		{"../../system/external-stub", "../external-stub"},
	}
	if lang != testLang {
		r = append(r, [2]string{"monolith-system-" + testLang, "system"})
	}
	return r
}

// multitierContentReplacements returns workflow content replacements for multitier.
func multitierContentReplacements(backendLang, frontendLang, testLang string) [][2]string {
	r := [][2]string{
		// Workflow names for pipeline stages (longer patterns first)
		{"multitier-" + testLang + "-acceptance-stage", "acceptance-stage"},
		{"multitier-" + testLang + "-qa-stage", "qa-stage"},
		{"multitier-" + testLang + "-qa-signoff", "qa-signoff"},
		{"multitier-" + testLang + "-prod-stage", "prod-stage"},
		{"multitier-" + testLang + "-verify", "verify"},
		// Working directories (these also transform commit stage workflow names:
		// multitier-backend-{lang}-commit-stage -> backend-commit-stage, etc.)
		{"system/multitier/backend-" + backendLang, "backend"},
		{"system/multitier/frontend-" + frontendLang, "frontend"},
		// System-test path
		{"system-test/" + testLang + "/", "system-test/"},
		{"system-test/" + testLang, "system-test"},
		// Docker image names (also transforms remaining workflow name references)
		{"multitier-backend-" + backendLang, "backend"},
		{"multitier-frontend-" + frontendLang, "frontend"},
	}
	if backendLang != testLang {
		r = append(r, [2]string{"multitier-backend-" + testLang, "backend"})
	}
	return r
}

// multitierDockerComposeReplacements returns docker-compose content replacements for multitier.
func multitierDockerComposeReplacements(backendLang, frontendLang, testLang string) [][2]string {
	r := [][2]string{
		{"system-test/" + testLang + "/", "system-test/"},
		{"system-test/" + testLang, "system-test"},
		{"multitier-backend-" + backendLang, "backend"},
		{"multitier-frontend-" + frontendLang, "frontend"},
		// Volume mount paths: old layout had system-test/{lang}/, new has system-test/
		{"../../system/external-real-sim", "../external-real-sim"},
		{"../../system/external-stub", "../external-stub"},
	}
	if backendLang != testLang {
		r = append(r, [2]string{"multitier-backend-" + testLang, "backend"})
	}
	return r
}

// monolithSonarKeyReplacements returns SonarCloud key suffix replacements for monolith.
// Applied to all text files (build.gradle, .csproj, etc.), not just workflows.
func monolithSonarKeyReplacements(lang string) [][2]string {
	return [][2]string{
		{"-monolith-" + lang, "-system"},
	}
}

// multitierSonarKeyReplacements returns SonarCloud key suffix replacements for multitier.
func multitierSonarKeyReplacements(backendLang, frontendLang string) [][2]string {
	return [][2]string{
		{"-multitier-backend-" + backendLang, "-backend"},
		{"-multitier-frontend-" + frontendLang, "-frontend"},
	}
}

// copyDocs copies arch-specific and shared docs templates into {repoDir}/docs/.
func copyDocs(starter, repoDir, arch string) {
	dst := filepath.Join(repoDir, "docs")
	files.CopyDir(filepath.Join(starter, "docs", arch), dst)
	files.CopyDir(filepath.Join(starter, "docs", "shared"), dst)
}
