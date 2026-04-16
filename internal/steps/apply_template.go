package steps

import (
	"os"
	"path/filepath"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/files"
	"github.com/optivem/gh-optivem/internal/log"
	"github.com/optivem/gh-optivem/internal/templates"
)

const (
	cleanupPrereleaseWorkflow = "cleanup-prereleases.yml"
	deployCloudRun            = "cloud-run"
	cloudSuffix               = "-cloud"
	commitStageYml            = "-commit-stage.yml"
	acceptStageYml            = "acceptance-stage.yml"
	qaStageYml                = "qa-stage.yml"
	qaSignoffYml              = "qa-signoff.yml"
	prodStageYml              = "prod-stage.yml"
	acceptStageLegacyYml      = "acceptance-stage-legacy.yml"
	suffixAcceptanceStage     = "-acceptance-stage"
	suffixQAStage             = "-qa-stage"
	suffixQASignoff           = "-qa-signoff"
	suffixProdStage           = "-prod-stage"
	suffixCommitStage         = "-commit-stage"
	prefixMonolith            = "monolith-"
	prefixMultitier           = "multitier-"
	prefixMultitierBackend    = "multitier-backend-"
	prefixMultitierFrontend   = "multitier-frontend-"
	prefixMonolithSystem      = "monolith-system-"
	dirSystemTest             = "system-test"
	dirExternalRealSim        = "external-real-sim"
	dirExternalStub           = "external-stub"
	starterSystemPrefix       = "../../system/"
)

var externalSimDirs = []string{dirExternalRealSim, dirExternalStub}

// cloudRunSuffix returns "-cloud" for cloud-run deploy, empty string otherwise.
func cloudRunSuffix(deploy string) string {
	if deploy == deployCloudRun {
		return cloudSuffix
	}
	return ""
}

// appendCloudReplacement appends the -cloud -> "" replacement for cloud-run deploy.
func appendCloudReplacement(r [][2]string, deploy string) [][2]string {
	if deploy == deployCloudRun {
		return append(r, [2]string{cloudSuffix, ""})
	}
	return r
}

// copyExternals copies external system simulator directories from starter to repo.
func copyExternals(starter, repoDir string) {
	for _, dir := range externalSimDirs {
		src := filepath.Join(starter, "system", dir)
		if _, err := os.Stat(src); err == nil {
			files.CopyDir(src, filepath.Join(repoDir, dir))
		}
	}
}

// copySystemTests copies system-test/{testLang}/ -> system-test/ and selects docker-compose variant.
func copySystemTests(starter, repoDir, testLang, composeVariant string) string {
	testDst := filepath.Join(repoDir, dirSystemTest)
	files.CopyDir(filepath.Join(starter, dirSystemTest, testLang), testDst)
	templates.SelectDockerCompose(testDst, composeVariant)
	templates.CopyVersion(starter, repoDir)
	return testDst
}

// monolithPipelineWorkflows builds workflow source->dest map for monolith pipeline stages.
func monolithPipelineWorkflows(testLang, stageSuffix string) map[string]string {
	p := prefixMonolith + testLang
	return map[string]string{
		p + suffixAcceptanceStage + stageSuffix + ".yml": acceptStageYml,
		p + suffixQAStage + stageSuffix + ".yml":         qaStageYml,
		p + suffixQASignoff + ".yml":                     qaSignoffYml,
		p + suffixProdStage + stageSuffix + ".yml":       prodStageYml,
	}
}

// multitierPipelineWorkflows builds workflow source->dest map for multitier pipeline stages.
func multitierPipelineWorkflows(testLang, stageSuffix string) map[string]string {
	p := prefixMultitier + testLang
	return map[string]string{
		p + suffixAcceptanceStage + stageSuffix + ".yml": acceptStageYml,
		p + suffixQAStage + stageSuffix + ".yml":         qaStageYml,
		p + suffixQASignoff + ".yml":                     qaSignoffYml,
		p + suffixProdStage + stageSuffix + ".yml":       prodStageYml,
	}
}

// addLegacyWorkflow adds the acceptance-stage-legacy workflow for docker deploy.
func addLegacyWorkflow(wfMap map[string]string, prefix, testLang, deploy string) {
	if deploy == "docker" {
		wfMap[prefix+testLang+"-acceptance-stage-legacy.yml"] = acceptStageLegacyYml
	}
}

// ApplyTemplate copies template files into the cloned repo(s).
func ApplyTemplate(cfg *config.Config) {
	log.Log("Step 5: Applying template files...")

	if cfg.DryRun {
		log.Log("[DRY RUN] Would apply template files")
		return
	}

	EnsureWorkflowDir(cfg.RepoDir)

	// Copy architecture-independent workflows
	templates.CopyWorkflows(map[string]string{
		cleanupPrereleaseWorkflow: cleanupPrereleaseWorkflow,
	}, cfg.StarterPath, cfg.RepoDir)

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
	stageSuffix := cloudRunSuffix(cfg.Deploy)

	// Workflows: rename to language-agnostic names
	wfMap := monolithPipelineWorkflows(testLang, stageSuffix)
	wfMap[prefixMonolith+lang+commitStageYml] = "commit-stage.yml"
	addLegacyWorkflow(wfMap, prefixMonolith, testLang, cfg.Deploy)
	templates.CopyWorkflows(wfMap, starter, repoDir)

	// System code: system/monolith/{lang}/ -> system/
	files.CopyDir(
		filepath.Join(starter, "system", "monolith", lang),
		filepath.Join(repoDir, "system"),
	)

	copyExternals(starter, repoDir)
	copySystemTests(starter, repoDir, testLang, "single")

	// Fix workflow content: paths, image names, workflow names
	contentReplacements := appendCloudReplacement(monolithContentReplacements(lang, testLang), cfg.Deploy)
	templates.FixupWorkflowContent(repoDir, contentReplacements)
	templates.FixupDockerComposeContent(repoDir, monolithDockerComposeReplacements(lang, testLang))

	// Fix SonarCloud key suffixes in build files (build.gradle, .csproj, etc.)
	templates.FixupAllTextFiles(repoDir, monolithSonarKeyReplacements(lang))

	if cfg.Deploy == deployCloudRun {
		copyCloudRunScripts(starter, repoDir)
	}

	copyDocs(starter, repoDir, "monolith")
	log.OK("Applied template files (monolith monorepo)")
}

// ── Monolith Multirepo ─────────────────────────────────────────────────────

func applyMonolithMultirepo(cfg *config.Config) {
	lang := cfg.Lang
	testLang := cfg.TestLang
	starter := cfg.StarterPath
	repoDir := cfg.RepoDir
	sysDir := cfg.SystemRepoDir
	stageSuffix := cloudRunSuffix(cfg.Deploy)

	// Root repo: pipeline stage workflows + system-test
	rootWfMap := monolithPipelineWorkflows(testLang, stageSuffix)
	addLegacyWorkflow(rootWfMap, prefixMonolith, testLang, cfg.Deploy)
	templates.CopyWorkflows(rootWfMap, starter, repoDir)

	copyExternals(starter, repoDir)
	copySystemTests(starter, repoDir, testLang, "single")

	// Fix root repo workflow content
	contentReplacements := appendCloudReplacement(monolithContentReplacements(lang, testLang), cfg.Deploy)
	templates.FixupWorkflowContent(repoDir, contentReplacements)
	templates.FixupDockerComposeContent(repoDir, monolithDockerComposeReplacements(lang, testLang))

	templates.FixupAllTextFiles(repoDir, monolithSonarKeyReplacements(lang))

	if cfg.Deploy == deployCloudRun {
		copyCloudRunScripts(starter, repoDir)
	}

	copyDocs(starter, repoDir, "monolith")

	// Fix multirepo image URLs and tokens
	templates.FixupMonolithMultirepoImageURLs(repoDir, cfg.SystemRepo)
	templates.FixupMultirepoToken(repoDir)
	log.OK("Applied root repo template (monolith multirepo)")

	// System repo: system code + commit stage
	EnsureWorkflowDir(sysDir)

	systemSrc := filepath.Join(starter, "system", "monolith", lang)
	files.CopyDir(systemSrc, filepath.Join(sysDir, "system"))

	systemWfMap := map[string]string{
		prefixMonolith + lang + commitStageYml: "commit-stage.yml",
	}
	templates.CopyWorkflows(systemWfMap, starter, sysDir)

	sysContentReplacements := [][2]string{
		{prefixMonolith + lang + suffixCommitStage, "commit-stage"},
		{"system/monolith/" + lang, "system"},
		{prefixMonolithSystem + lang, "system"},
	}
	templates.FixupWorkflowContent(sysDir, sysContentReplacements)
	templates.FixupAllTextFiles(sysDir, monolithSonarKeyReplacements(lang))
	log.OK("Applied system repo template (monolith multirepo)")
}

// ── Multitier Monorepo ─────────────────────────────────────────────────────

func applyMultitierMonorepo(cfg *config.Config) {
	backendLang := cfg.BackendLang
	frontendLang := cfg.FrontendLang
	testLang := cfg.TestLang
	starter := cfg.StarterPath
	repoDir := cfg.RepoDir
	stageSuffix := cloudRunSuffix(cfg.Deploy)

	// Workflows: rename to language-agnostic names
	wfMap := multitierPipelineWorkflows(testLang, stageSuffix)
	wfMap[prefixMultitierBackend+backendLang+commitStageYml] = "backend-commit-stage.yml"
	wfMap[prefixMultitierFrontend+frontendLang+commitStageYml] = "frontend-commit-stage.yml"
	addLegacyWorkflow(wfMap, prefixMultitier, testLang, cfg.Deploy)
	templates.CopyWorkflows(wfMap, starter, repoDir)

	// Backend code: system/multitier/backend-{lang}/ -> backend/
	backendSrc := filepath.Join(starter, "system", "multitier", "backend-"+backendLang)
	files.CopyDir(backendSrc, filepath.Join(repoDir, "backend"))
	log.OK("Applied backend template")

	// Frontend code: system/multitier/frontend-{lang}/ -> frontend/
	frontendSrc := filepath.Join(starter, "system", "multitier", "frontend-"+frontendLang)
	files.CopyDir(frontendSrc, filepath.Join(repoDir, "frontend"))
	log.OK("Applied frontend template")

	copyExternals(starter, repoDir)
	copySystemTests(starter, repoDir, testLang, "multi")

	// Fix workflow content: paths and image names
	contentReplacements := appendCloudReplacement(multitierContentReplacements(backendLang, frontendLang, testLang), cfg.Deploy)
	templates.FixupWorkflowContent(repoDir, contentReplacements)
	templates.FixupDockerComposeContent(repoDir, multitierDockerComposeReplacements(backendLang, frontendLang, testLang))

	templates.FixupAllTextFiles(repoDir, multitierSonarKeyReplacements(backendLang, frontendLang))

	if cfg.Deploy == deployCloudRun {
		copyCloudRunScripts(starter, repoDir)
	}

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
	bDir := cfg.BackendRepoDir
	fDir := cfg.FrontendRepoDir
	stageSuffix := cloudRunSuffix(cfg.Deploy)

	// Root repo: pipeline stage workflows + system-test + externals
	rootWfMap := multitierPipelineWorkflows(testLang, stageSuffix)
	addLegacyWorkflow(rootWfMap, prefixMultitier, testLang, cfg.Deploy)
	templates.CopyWorkflows(rootWfMap, starter, repoDir)

	copyExternals(starter, repoDir)
	copySystemTests(starter, repoDir, testLang, "multi")

	// Fix root repo workflow content
	contentReplacements := appendCloudReplacement(multitierContentReplacements(backendLang, frontendLang, testLang), cfg.Deploy)
	templates.FixupWorkflowContent(repoDir, contentReplacements)
	templates.FixupDockerComposeContent(repoDir, multitierDockerComposeReplacements(backendLang, frontendLang, testLang))

	templates.FixupAllTextFiles(repoDir, multitierSonarKeyReplacements(backendLang, frontendLang))

	if cfg.Deploy == deployCloudRun {
		copyCloudRunScripts(starter, repoDir)
	}

	copyDocs(starter, repoDir, "multitier")

	// Fix multirepo image URLs and tokens
	templates.FixupMultirepoImageURLs(repoDir, cfg.FrontendRepo, cfg.BackendRepo)
	templates.FixupMultirepoToken(repoDir)
	log.OK("Applied root repo template (multitier multirepo)")

	// Backend repo: code + commit stage
	EnsureWorkflowDir(bDir)
	backendSrc := filepath.Join(starter, "system", "multitier", "backend-"+backendLang)
	files.CopyDir(backendSrc, filepath.Join(bDir, "backend"))

	backendWfMap := map[string]string{
		prefixMultitierBackend + backendLang + commitStageYml: "backend-commit-stage.yml",
	}
	templates.CopyWorkflows(backendWfMap, starter, bDir)

	backendReplacements := [][2]string{
		{prefixMultitierBackend + backendLang + suffixCommitStage, "backend-commit-stage"},
		{"system/multitier/backend-" + backendLang, "backend"},
		{prefixMultitierBackend + backendLang, "backend"},
	}
	templates.FixupWorkflowContent(bDir, backendReplacements)
	templates.FixupAllTextFiles(bDir, multitierSonarKeyReplacements(backendLang, frontendLang))
	log.OK("Applied backend repo template")

	// Frontend repo: code + commit stage
	EnsureWorkflowDir(fDir)
	frontendSrc := filepath.Join(starter, "system", "multitier", "frontend-"+frontendLang)
	files.CopyDir(frontendSrc, filepath.Join(fDir, "frontend"))

	frontendWfMap := map[string]string{
		prefixMultitierFrontend + frontendLang + commitStageYml: "frontend-commit-stage.yml",
	}
	templates.CopyWorkflows(frontendWfMap, starter, fDir)

	frontendReplacements := [][2]string{
		{prefixMultitierFrontend + frontendLang + suffixCommitStage, "frontend-commit-stage"},
		{"system/multitier/frontend-" + frontendLang, "frontend"},
		{prefixMultitierFrontend + frontendLang, "frontend"},
	}
	templates.FixupWorkflowContent(fDir, frontendReplacements)
	templates.FixupAllTextFiles(fDir, multitierSonarKeyReplacements(backendLang, frontendLang))
	log.OK("Applied frontend repo template")
}

// ── Content replacement helpers ────────────────────────────────────────────

// monolithContentReplacements returns workflow content replacements for monolith.
func monolithContentReplacements(lang, testLang string) [][2]string {
	mono := prefixMonolith
	monoTest := mono + testLang
	r := [][2]string{
		// Workflow names (longer patterns first to avoid partial matches)
		{mono + lang + suffixCommitStage, "commit-stage"},
		{monoTest + suffixAcceptanceStage, "acceptance-stage"},
		{monoTest + suffixQAStage, "qa-stage"},
		{monoTest + suffixQASignoff, "qa-signoff"},
		{monoTest + suffixProdStage, "prod-stage"},
		{monoTest + "-verify", "verify"},
		// Working directory
		{"system/monolith/" + lang, "system"},
		// System-test path
		{dirSystemTest + "/" + testLang + "/", dirSystemTest + "/"},
		{dirSystemTest + "/" + testLang, dirSystemTest},
		// Docker image names
		{prefixMonolithSystem + lang, "system"},
	}
	if lang != testLang {
		r = append(r, [2]string{prefixMonolithSystem + testLang, "system"})
	}
	return r
}

// monolithDockerComposeReplacements returns docker-compose content replacements for monolith.
func monolithDockerComposeReplacements(lang, testLang string) [][2]string {
	r := [][2]string{
		{dirSystemTest + "/" + testLang + "/", dirSystemTest + "/"},
		{dirSystemTest + "/" + testLang, dirSystemTest},
		{prefixMonolithSystem + lang, "system"},
		// Docker build context: starter has system-test/{lang}/ so ../../system/monolith/{lang} is correct there,
		// but scaffold flattens to system-test/ (one level up), so the context becomes ../system
		{"../../system/monolith/" + lang, "../system"},
		// Volume mount paths: old layout had system-test/{lang}/, new has system-test/
		{starterSystemPrefix + dirExternalRealSim, "../" + dirExternalRealSim},
		{starterSystemPrefix + dirExternalStub, "../" + dirExternalStub},
	}
	if lang != testLang {
		r = append(r, [2]string{"../../system/monolith/" + testLang, "../system"})
		r = append(r, [2]string{prefixMonolithSystem + testLang, "system"})
	}
	return r
}

// multitierContentReplacements returns workflow content replacements for multitier.
func multitierContentReplacements(backendLang, frontendLang, testLang string) [][2]string {
	multiTest := prefixMultitier + testLang
	r := [][2]string{
		// Workflow names for pipeline stages (longer patterns first)
		{multiTest + suffixAcceptanceStage, "acceptance-stage"},
		{multiTest + suffixQAStage, "qa-stage"},
		{multiTest + suffixQASignoff, "qa-signoff"},
		{multiTest + suffixProdStage, "prod-stage"},
		{multiTest + "-verify", "verify"},
		// Working directories (these also transform commit stage workflow names:
		// multitier-backend-{lang}-commit-stage -> backend-commit-stage, etc.)
		{"system/multitier/backend-" + backendLang, "backend"},
		{"system/multitier/frontend-" + frontendLang, "frontend"},
		// System-test path
		{dirSystemTest + "/" + testLang + "/", dirSystemTest + "/"},
		{dirSystemTest + "/" + testLang, dirSystemTest},
		// Docker image names (also transforms remaining workflow name references)
		{prefixMultitierBackend + backendLang, "backend"},
		{prefixMultitierFrontend + frontendLang, "frontend"},
	}
	if backendLang != testLang {
		r = append(r, [2]string{prefixMultitierBackend + testLang, "backend"})
	}
	return r
}

// multitierDockerComposeReplacements returns docker-compose content replacements for multitier.
func multitierDockerComposeReplacements(backendLang, frontendLang, testLang string) [][2]string {
	r := [][2]string{
		{dirSystemTest + "/" + testLang + "/", dirSystemTest + "/"},
		{dirSystemTest + "/" + testLang, dirSystemTest},
		{prefixMultitierBackend + backendLang, "backend"},
		{prefixMultitierFrontend + frontendLang, "frontend"},
		// Volume mount paths: old layout had system-test/{lang}/, new has system-test/
		{starterSystemPrefix + dirExternalRealSim, "../" + dirExternalRealSim},
		{starterSystemPrefix + dirExternalStub, "../" + dirExternalStub},
	}
	// Docker build contexts always reference the test-lang backend and the frontend lang in the
	// starter layout (e.g. backend-typescript, frontend-react). After scaffolding these become
	// ../backend and ../frontend respectively, so we always need both replacements.
	r = append(r, [2]string{"../../system/multitier/backend-" + testLang, "../backend"})
	r = append(r, [2]string{prefixMultitierBackend + testLang, "backend"})
	r = append(r, [2]string{"../../system/multitier/frontend-" + frontendLang, "../frontend"})
	r = append(r, [2]string{prefixMultitierFrontend + frontendLang, "frontend"})
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
		{"-" + prefixMultitierBackend + backendLang, "-backend"},
		{"-" + prefixMultitierFrontend + frontendLang, "-frontend"},
	}
}

// copyDocs copies arch-specific and shared docs templates into {repoDir}/docs/.
func copyDocs(starter, repoDir, arch string) {
	dst := filepath.Join(repoDir, "docs")
	files.CopyDir(filepath.Join(starter, "docs", arch), dst)
	files.CopyDir(filepath.Join(starter, "docs", "shared"), dst)
}

// copyCloudRunScripts copies setup-gcp.sh and teardown-gcp.sh from starter to repo.
func copyCloudRunScripts(starter, repoDir string) {
	for _, name := range []string{"setup-gcp.sh", "teardown-gcp.sh"} {
		src := filepath.Join(starter, name)
		if _, err := os.Stat(src); err == nil {
			files.CopyFile(src, filepath.Join(repoDir, name))
		}
	}
}
