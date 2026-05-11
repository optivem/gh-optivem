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
	deployCloudRun = "cloud-run"
	cloudSuffix    = "-cloud"

	// Substring fragments still consumed by the content-replacement helpers
	// (monolithContentReplacements, multitierContentReplacements, etc.).
	// Filename and folder-path templates live in Names; see names.go.
	suffixAcceptanceStage   = "-acceptance-stage"
	suffixQAStage           = "-qa-stage"
	suffixQASignoff         = "-qa-signoff"
	suffixProdStage         = "-prod-stage"
	suffixCommitStage       = "-commit-stage"
	prefixMonolith          = "monolith-"
	prefixMultitier         = "multitier-"
	prefixMultitierBackend  = "multitier-backend-"
	prefixMultitierFrontend = "multitier-frontend-"
	prefixMonolithSystem    = "monolith-system-"
	envPrefixYAML           = "environment: "
	wdPrefixYAML            = "working-directory: "
	dirSystemTest           = "system-test"
	dirDocker               = "docker"
	dirExternalRealSim         = "external-real-sim"
	dirExternalStub            = "external-stub"
	shopSystemPrefix           = "../../../system/"
	shopExternalSystemsPrefix  = "../../../external-systems/"
	systemMonolithDir       = "system/monolith/"
	systemMultitierDir      = "system/multitier/"
	systemMultitierBackend  = systemMultitierDir + "backend-"

	infoCopyingExternals   = "Copying external simulators..."
	infoCopyingSystemTests = "Copying system-tests..."
	infoCopyingCloudRun    = "Copying Cloud Run scripts..."
	infoCopyingDocs        = "Copying docs..."
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

// copyExternals copies external system simulator directories from shop to
// repo, preserving shop's external-systems/ parent. Source and destination
// both live at <root>/external-systems/<dir>; gh-optivem.yaml's
// external_systems.{stubs,simulators}.path values match this layout.
func copyExternals(shop, repoDir string) {
	for _, dir := range externalSimDirs {
		src := filepath.Join(shop, "external-systems", dir)
		if _, err := os.Stat(src); err == nil {
			files.CopyDir(src, filepath.Join(repoDir, "external-systems", dir))
		}
	}
}

// copyIssueTemplates copies shop/.github/ISSUE_TEMPLATE/ into the scaffolded
// repo. Issue forms have no templated content (no language / arch / lang
// substitutions), so a plain directory copy is the whole job.
func copyIssueTemplates(shop, repoDir string) {
	src := filepath.Join(shop, ".github", "ISSUE_TEMPLATE")
	if _, err := os.Stat(src); err == nil {
		files.CopyDir(src, filepath.Join(repoDir, ".github", "ISSUE_TEMPLATE"))
	}
}

// copySystemTests copies system-test/{testLang}/ -> system-test/ (tests +
// README only) and docker/{testLang}/{arch}/ -> docker/ (systems.yaml +
// compose files for the selected arch). composeVariant is "single" (monolith)
// or "multi" (multitier). systemLang identifies the per-system VERSION file
// to copy from shop (cfg.Lang for monolith, cfg.BackendLang for multitier);
// the file at shop/system/<arch>/<systemLang>/VERSION becomes repoDir/VERSION.
func copySystemTests(shop, repoDir, testLang, composeVariant, systemLang string) string {
	arch := "monolith"
	if composeVariant != "single" {
		arch = "multitier"
	}
	vars := map[string]string{"testLang": testLang, "arch": arch}

	testDst := filepath.Join(repoDir, Names.TargetSystemTestDir)
	files.CopyDir(filepath.Join(shop, Expand(Names.ShopSystemTestDir, vars)), testDst)

	dockerDst := filepath.Join(repoDir, Names.TargetDockerDir)
	files.CopyDir(filepath.Join(shop, Expand(Names.ShopDockerDir, vars)), dockerDst)

	templates.CopyVersion(shop, repoDir, arch, systemLang)
	return testDst
}

// pipelineWorkflows builds the workflow source->dest map for the four
// pipeline-stage workflows (acceptance/qa/prod take stageSuffix; qa-signoff
// does not, since it does not vary by deploy target).
func pipelineWorkflows(srcTmpl string, baseVars map[string]string) map[string]string {
	out := map[string]string{}
	for _, stage := range []string{"acceptance-stage", "qa-stage", "prod-stage"} {
		v := MergeVars(baseVars, map[string]string{"stage": stage})
		out[Expand(srcTmpl, v)] = Expand(Names.DestPipelineStageWf, v)
	}
	v := MergeVars(baseVars, map[string]string{"stage": "qa-signoff", "stageSuffix": ""})
	out[Expand(srcTmpl, v)] = Expand(Names.DestPipelineStageWf, v)
	return out
}

func monolithPipelineWorkflows(vars map[string]string) map[string]string {
	return pipelineWorkflows(Names.MonolithPipelineStageWf, vars)
}

func multitierPipelineWorkflows(vars map[string]string) map[string]string {
	return pipelineWorkflows(Names.MultitierPipelineStageWf, vars)
}

// addLegacyWorkflow adds the acceptance-stage-legacy workflow for docker deploy.
func addLegacyWorkflow(wfMap map[string]string, srcTmpl string, vars map[string]string, deploy string) {
	if deploy == "docker" {
		wfMap[Expand(srcTmpl, vars)] = Names.DestLegacyAcceptWf
	}
}

// ApplyTemplate copies template files into the cloned repo(s).
func ApplyTemplate(cfg *config.Config) {
	log.Info("Applying template files...")

	if cfg.DryRun {
		log.Info("[DRY RUN] Would apply template files")
		return
	}

	EnsureWorkflowDir(cfg.RepoDir)

	// Copy architecture-independent workflows
	log.Info("Copying cleanup workflow...")
	templates.CopyWorkflows(map[string]string{
		Names.CleanupWf: Names.CleanupWf,
	}, cfg.ShopPath, cfg.RepoDir)

	// Issue forms (.github/ISSUE_TEMPLATE/*.yml) — same files in every
	// scaffolded repo regardless of arch / lang.
	log.Info("Copying issue templates...")
	copyIssueTemplates(cfg.ShopPath, cfg.RepoDir)

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

	log.Success("Applied template files")
}

// ── Monolith Monorepo ──────────────────────────────────────────────────────

func applyMonolithMonorepo(cfg *config.Config) {
	lang := cfg.Lang
	testLang := cfg.TestLang
	shop := cfg.ShopPath
	repoDir := cfg.RepoDir
	vars := VarsForCfg(cfg)

	// Workflows: rename to language-agnostic names
	log.Info("Copying pipeline and commit-stage workflows...")
	wfMap := monolithPipelineWorkflows(vars)
	wfMap[Expand(Names.MonolithCommitStageWf, vars)] = Names.DestCommitStageWf
	wfMap[Expand(Names.MonolithBumpPatchVersionWf, vars)] = Names.DestBumpPatchVersionWf
	addLegacyWorkflow(wfMap, Names.MonolithLegacyAcceptWf, vars, cfg.Deploy)
	templates.CopyWorkflows(wfMap, shop, repoDir)

	// System code: system/monolith/{lang}/ -> system/
	log.Info("Copying system code...")
	files.CopyDir(
		filepath.Join(shop, Expand(Names.ShopSystemMonolithDir, vars)),
		filepath.Join(repoDir, Names.TargetSystemDir),
	)

	log.Info(infoCopyingExternals)
	copyExternals(shop, repoDir)

	log.Info(infoCopyingSystemTests)
	copySystemTests(shop, repoDir, testLang, "single", lang)

	// Fix workflow content: paths, image names, workflow names
	log.Info("Fixing up workflow and docker-compose content...")
	contentReplacements := appendCloudReplacement(monolithContentReplacements(lang, testLang), cfg.Deploy)
	contentReplacements = append(contentReplacements, systemPrefixDropReplacements(prefixMonolith+testLang)...)
	templates.FixupWorkflowContent(repoDir, contentReplacements)
	templates.FixupDockerComposeContent(repoDir, monolithDockerComposeReplacements(lang, testLang))

	// Fix SonarCloud key suffixes in build files (build.gradle, .csproj, etc.)
	log.Info("Fixing up SonarCloud keys...")
	templates.FixupAllTextFiles(repoDir, monolithSonarKeyReplacements(lang))

	if cfg.Deploy == deployCloudRun {
		log.Info(infoCopyingCloudRun)
		copyCloudRunScripts(shop, repoDir)
	}

	log.Info(infoCopyingDocs)
	copyDocs(shop, repoDir, "monolith")
	log.Success("Applied template files (monolith monorepo)")
}

// ── Monolith Multirepo ─────────────────────────────────────────────────────

func applyMonolithMultirepo(cfg *config.Config) {
	lang := cfg.Lang
	testLang := cfg.TestLang
	shop := cfg.ShopPath
	repoDir := cfg.RepoDir
	sysDir := cfg.SystemRepoDir
	vars := VarsForCfg(cfg)

	// Root repo: pipeline stage workflows + system-test
	log.Info("Copying root repo pipeline workflows...")
	rootWfMap := monolithPipelineWorkflows(vars)
	rootWfMap[Names.BumpPatchVersionMultirepoWf] = Names.DestBumpPatchVersionWf
	addLegacyWorkflow(rootWfMap, Names.MonolithLegacyAcceptWf, vars, cfg.Deploy)
	templates.CopyWorkflows(rootWfMap, shop, repoDir)

	log.Info(infoCopyingExternals)
	copyExternals(shop, repoDir)

	log.Info(infoCopyingSystemTests)
	copySystemTests(shop, repoDir, testLang, "single", lang)

	// Fix root repo workflow content
	log.Info("Fixing up root repo workflow and docker-compose content...")
	contentReplacements := appendCloudReplacement(monolithContentReplacements(lang, testLang), cfg.Deploy)
	contentReplacements = append(contentReplacements, systemPrefixDropReplacements(prefixMonolith+testLang)...)
	templates.FixupWorkflowContent(repoDir, contentReplacements)
	templates.FixupDockerComposeContent(repoDir, monolithDockerComposeReplacements(lang, testLang))

	log.Info("Fixing up SonarCloud keys in root repo...")
	templates.FixupAllTextFiles(repoDir, monolithSonarKeyReplacements(lang))

	if cfg.Deploy == deployCloudRun {
		log.Info(infoCopyingCloudRun)
		copyCloudRunScripts(shop, repoDir)
	}

	log.Info(infoCopyingDocs)
	copyDocs(shop, repoDir, "monolith")

	// Fix multirepo image URLs and tokens
	log.Info("Fixing up multirepo image URLs and tokens...")
	templates.FixupMonolithMultirepoImageURLs(repoDir, cfg.SystemRepo)
	templates.FixupMultirepoToken(repoDir)

	// Rewrite the bump-patch-version-multirepo dispatcher's __SIBLING_REPOS__
	// placeholder to the system repo's full name (single sibling).
	log.Info("Wiring bump-patch-version-multirepo dispatcher with sibling repo...")
	templates.FixupWorkflowContent(repoDir, [][2]string{
		{"__SIBLING_REPOS__", cfg.SystemFullRepo},
	})

	log.Success("Applied root repo template (monolith multirepo)")

	// System repo: system code + commit stage + bump-patch-version (per-component)
	EnsureWorkflowDir(sysDir)

	log.Info("Copying system code to system repo...")
	systemSrc := filepath.Join(shop, Expand(Names.ShopSystemMonolithDir, vars))
	files.CopyDir(systemSrc, filepath.Join(sysDir, Names.TargetSystemDir))
	templates.CopyVersion(shop, sysDir, "monolith", lang)

	log.Info("Copying commit-stage and bump-patch-version workflows to system repo...")
	systemWfMap := map[string]string{
		Expand(Names.MonolithCommitStageWf, vars):      Names.DestCommitStageWf,
		Expand(Names.MonolithBumpPatchVersionWf, vars): Names.DestBumpPatchVersionWf,
		Names.CleanupWf:                                Names.CleanupWf,
	}
	templates.CopyWorkflows(systemWfMap, shop, sysDir)

	log.Info("Fixing up system repo workflow content and SonarCloud keys...")
	sysContentReplacements := [][2]string{
		{ExpandRef(Names.MonolithCommitStageWf, vars), "commit-stage"},
		{ExpandRef(Names.MonolithBumpPatchVersionWf, vars), "bump-patch-version"},
		// Same precedence rule as monolithContentReplacements: VERSION-specific
		// rule before the broader system/monolith/<lang> -> system rule.
		{Expand(Names.ShopVersionFile, vars), "VERSION"},
		{Expand(Names.ShopSystemMonolithDir, vars), Names.TargetSystemDir},
		{Expand(Names.MonolithImageRef, vars), Names.TargetSystemDir},
	}
	templates.FixupWorkflowContent(sysDir, sysContentReplacements)
	templates.FixupAllTextFiles(sysDir, monolithSonarKeyReplacements(lang))
	log.Success("Applied system repo template (monolith multirepo)")
}

// ── Multitier Monorepo ─────────────────────────────────────────────────────

func applyMultitierMonorepo(cfg *config.Config) {
	backendLang := cfg.BackendLang
	// "react" is the shop-side framework directory token (frontend-react/,
	// multitier-frontend-react-*.yml). cfg.FrontendLang is the user-facing
	// source language ("typescript") and is not used for shop-path lookups.
	frontendLang := "react"
	testLang := cfg.TestLang
	shop := cfg.ShopPath
	repoDir := cfg.RepoDir
	vars := VarsForCfg(cfg)

	// Workflows: rename to language-agnostic names
	log.Info("Copying pipeline and commit-stage workflows...")
	wfMap := multitierPipelineWorkflows(vars)
	wfMap[Expand(Names.MultitierBackendCommitStageWf, vars)] = Names.DestBackendCommitStageWf
	wfMap[Expand(Names.MultitierFrontendCommitStageWf, vars)] = Names.DestFrontendCommitStageWf
	wfMap[Expand(Names.MultitierBumpPatchVersionWf, vars)] = Names.DestBumpPatchVersionWf
	addLegacyWorkflow(wfMap, Names.MultitierLegacyAcceptWf, vars, cfg.Deploy)
	templates.CopyWorkflows(wfMap, shop, repoDir)

	// Backend code: system/multitier/backend-{lang}/ -> backend/
	log.Info("Copying backend code...")
	backendSrc := filepath.Join(shop, Expand(Names.ShopSystemMultitierBackend, vars))
	files.CopyDir(backendSrc, filepath.Join(repoDir, Names.TargetBackendDir))
	log.Success("Applied backend template")

	// Frontend code: system/multitier/frontend-{lang}/ -> frontend/
	log.Info("Copying frontend code...")
	frontendSrc := filepath.Join(shop, Expand(Names.ShopSystemMultitierFrontend, vars))
	files.CopyDir(frontendSrc, filepath.Join(repoDir, Names.TargetFrontendDir))
	log.Success("Applied frontend template")

	log.Info(infoCopyingExternals)
	copyExternals(shop, repoDir)

	log.Info(infoCopyingSystemTests)
	copySystemTests(shop, repoDir, testLang, "multi", backendLang)

	// Fix workflow content: paths and image names
	log.Info("Fixing up workflow and docker-compose content...")
	contentReplacements := appendCloudReplacement(multitierContentReplacements(backendLang, frontendLang, testLang), cfg.Deploy)
	// Prepend prefix-drops so 3-segment "multitier-backend-{lang}-v" / "multitier-frontend-react-v"
	// collapse to "v" before the bare 3-segment image-name rules in
	// multitierContentReplacements partial-match them into "backend-v"/"frontend-v".
	contentReplacements = append(
		systemPrefixDropReplacements(
			prefixMultitier+testLang,
			prefixMultitierBackend+backendLang,
			prefixMultitierFrontend+frontendLang,
		),
		contentReplacements...,
	)
	templates.FixupWorkflowContent(repoDir, contentReplacements)
	templates.FixupDockerComposeContent(repoDir, multitierDockerComposeReplacements(backendLang, frontendLang, testLang))

	log.Info("Fixing up SonarCloud keys...")
	templates.FixupAllTextFiles(repoDir, multitierSonarKeyReplacements(backendLang, frontendLang))

	if cfg.Deploy == deployCloudRun {
		log.Info(infoCopyingCloudRun)
		copyCloudRunScripts(shop, repoDir)
	}

	log.Info(infoCopyingDocs)
	copyDocs(shop, repoDir, "multitier")
	log.Success("Applied template files (multitier monorepo)")
}

// ── Multitier Multirepo ────────────────────────────────────────────────────

func applyMultitierMultirepo(cfg *config.Config) {
	backendLang := cfg.BackendLang
	// See applyMultitierMonorepo: shop-side framework token, not cfg.FrontendLang.
	frontendLang := "react"
	testLang := cfg.TestLang
	shop := cfg.ShopPath
	repoDir := cfg.RepoDir
	bDir := cfg.BackendRepoDir
	fDir := cfg.FrontendRepoDir
	vars := VarsForCfg(cfg)

	// Root repo: pipeline stage workflows + system-test + externals
	log.Info("Copying root repo pipeline workflows...")
	rootWfMap := multitierPipelineWorkflows(vars)
	rootWfMap[Names.BumpPatchVersionMultirepoWf] = Names.DestBumpPatchVersionWf
	addLegacyWorkflow(rootWfMap, Names.MultitierLegacyAcceptWf, vars, cfg.Deploy)
	templates.CopyWorkflows(rootWfMap, shop, repoDir)

	log.Info(infoCopyingExternals)
	copyExternals(shop, repoDir)

	log.Info(infoCopyingSystemTests)
	copySystemTests(shop, repoDir, testLang, "multi", backendLang)

	// Fix root repo workflow content
	log.Info("Fixing up root repo workflow and docker-compose content...")
	contentReplacements := appendCloudReplacement(multitierContentReplacements(backendLang, frontendLang, testLang), cfg.Deploy)
	// Prepend prefix-drops; see comment in applyMultitierMonorepo for rationale.
	contentReplacements = append(
		systemPrefixDropReplacements(
			prefixMultitier+testLang,
			prefixMultitierBackend+backendLang,
			prefixMultitierFrontend+frontendLang,
		),
		contentReplacements...,
	)
	templates.FixupWorkflowContent(repoDir, contentReplacements)
	templates.FixupDockerComposeContent(repoDir, multitierDockerComposeReplacements(backendLang, frontendLang, testLang))

	log.Info("Fixing up SonarCloud keys in root repo...")
	templates.FixupAllTextFiles(repoDir, multitierSonarKeyReplacements(backendLang, frontendLang))

	if cfg.Deploy == deployCloudRun {
		log.Info(infoCopyingCloudRun)
		copyCloudRunScripts(shop, repoDir)
	}

	log.Info(infoCopyingDocs)
	copyDocs(shop, repoDir, "multitier")

	// Fix multirepo image URLs and tokens
	log.Info("Fixing up multirepo image URLs and tokens...")
	templates.FixupMultirepoImageURLs(repoDir, cfg.FrontendRepo, cfg.BackendRepo)
	templates.FixupMultirepoToken(repoDir)

	// Rewrite read-base-versions entries to fetch VERSIONs cross-repo via API
	log.Info("Fixing up multirepo VERSION entries to fetch cross-repo...")
	templates.FixupMultirepoVersionEntries(repoDir, cfg.Owner, cfg.FrontendRepo, cfg.BackendRepo)

	// Rewrite the bump-patch-version-multirepo dispatcher's __SIBLING_REPOS__
	// placeholder to the backend + frontend full names (space-separated list
	// consumed by the dispatcher's shell loop).
	log.Info("Wiring bump-patch-version-multirepo dispatcher with sibling repos...")
	templates.FixupWorkflowContent(repoDir, [][2]string{
		{"__SIBLING_REPOS__", cfg.BackendFullRepo + " " + cfg.FrontendFullRepo},
	})

	log.Success("Applied root repo template (multitier multirepo)")

	// Backend repo: code + commit stage
	EnsureWorkflowDir(bDir)
	log.Info("Copying backend code to backend repo...")
	backendSrc := filepath.Join(shop, Expand(Names.ShopSystemMultitierBackend, vars))
	files.CopyDir(backendSrc, filepath.Join(bDir, Names.TargetBackendDir))

	log.Info("Copying commit-stage and bump-patch-version workflows to backend repo...")
	backendWfMap := map[string]string{
		Expand(Names.MultitierBackendCommitStageWf, vars): Names.DestBackendCommitStageWf,
		Expand(Names.MultitierBackendBumpPatchWf, vars):   Names.DestBumpPatchVersionWf,
		Names.CleanupWf:                                   Names.CleanupWf,
	}
	templates.CopyWorkflows(backendWfMap, shop, bDir)

	log.Info("Fixing up backend repo workflow content and SonarCloud keys...")
	// Prepend prefix-drop so "multitier-backend-{lang}-v" (in bump-patch-version.yml's
	// git-tag value) collapses to "v" before the bare 3-segment "multitier-backend-{lang}"
	// → "backend" rule below partial-matches it into "backend-v".
	backendReplacements := append(
		systemPrefixDropReplacements(Expand(Names.MultitierBackendRef, vars)),
		[2]string{ExpandRef(Names.MultitierBackendCommitStageWf, vars), "backend-commit-stage"},
		[2]string{Expand(Names.ShopSystemMultitierBackend, vars), Names.TargetBackendDir},
		[2]string{Expand(Names.MultitierBackendRef, vars), Names.TargetBackendDir},
		[2]string{"backend-bump-patch-version", "bump-patch-version"},
	)
	templates.FixupWorkflowContent(bDir, backendReplacements)
	templates.FixupAllTextFiles(bDir, multitierSonarKeyReplacements(backendLang, frontendLang))
	log.Success("Applied backend repo template")

	// Frontend repo: code + commit stage
	EnsureWorkflowDir(fDir)
	log.Info("Copying frontend code to frontend repo...")
	frontendSrc := filepath.Join(shop, Expand(Names.ShopSystemMultitierFrontend, vars))
	files.CopyDir(frontendSrc, filepath.Join(fDir, Names.TargetFrontendDir))

	log.Info("Copying commit-stage and bump-patch-version workflows to frontend repo...")
	frontendWfMap := map[string]string{
		Expand(Names.MultitierFrontendCommitStageWf, vars): Names.DestFrontendCommitStageWf,
		Expand(Names.MultitierFrontendBumpPatchWf, vars):   Names.DestBumpPatchVersionWf,
		Names.CleanupWf:                                    Names.CleanupWf,
	}
	templates.CopyWorkflows(frontendWfMap, shop, fDir)

	log.Info("Fixing up frontend repo workflow content and SonarCloud keys...")
	// Prepend prefix-drop so "multitier-frontend-react-v" (in bump-patch-version.yml's
	// git-tag value) collapses to "v" before the bare 3-segment "multitier-frontend-react"
	// → "frontend" rule below partial-matches it into "frontend-v".
	frontendReplacements := append(
		systemPrefixDropReplacements(Expand(Names.MultitierFrontendRef, vars)),
		[2]string{ExpandRef(Names.MultitierFrontendCommitStageWf, vars), "frontend-commit-stage"},
		[2]string{Expand(Names.ShopSystemMultitierFrontend, vars), Names.TargetFrontendDir},
		[2]string{Expand(Names.MultitierFrontendRef, vars), Names.TargetFrontendDir},
		[2]string{"frontend-bump-patch-version", "bump-patch-version"},
	)
	templates.FixupWorkflowContent(fDir, frontendReplacements)
	templates.FixupAllTextFiles(fDir, multitierSonarKeyReplacements(backendLang, frontendLang))
	log.Success("Applied frontend repo template")
}

// ── Content replacement helpers ────────────────────────────────────────────

// monolithContentReplacements returns workflow content replacements for monolith.
func monolithContentReplacements(lang, testLang string) [][2]string {
	mono := prefixMonolith
	monoTest := mono + testLang
	// Shop's pipeline-stage env names derive from the workflow filename, which
	// is monolith-<testLang>-*-stage.yml. So the env prefix is testLang, not lang.
	envPrefix := monoTest + "-"
	r := [][2]string{
		// Environment references — scaffolded repos have unprefixed env names
		// (SetupEnvironments creates bare `acceptance`/`qa`/`production`).
		{envPrefixYAML + envPrefix + "acceptance", envPrefixYAML + "acceptance"},
		{envPrefixYAML + envPrefix + "qa", envPrefixYAML + "qa"},
		{envPrefixYAML + envPrefix + "production", envPrefixYAML + "production"},
		// Workflow names (longer patterns first to avoid partial matches)
		{mono + lang + suffixCommitStage, "commit-stage"},
		{mono + lang + "-bump-patch-version", "bump-patch-version"},
		// Filename form used in `uses: ./.github/workflows/<flavor>-bump-patch-version.yml`
		// references inside prod-stage. Same shape as the rule above but keyed by
		// testLang because the prod-stage source is `monolith-<testLang>-prod-stage.yml`
		// and its `uses:` line hardcodes the same flavor's bump-patch-version filename —
		// `monolith-<testLang>-bump-patch-version.yml`. Without this, polyglot scaffolds
		// (lang != testLang) leave the shop filename in place and actionlint fails with
		// "could not read reusable workflow file".
		{mono + testLang + "-bump-patch-version", "bump-patch-version"},
		{monoTest + suffixAcceptanceStage, "acceptance-stage"},
		{monoTest + suffixQAStage, "qa-stage"},
		{monoTest + suffixQASignoff, "qa-signoff"},
		{monoTest + suffixProdStage, "prod-stage"},
		{monoTest + "-verify", "verify"},
		// VERSION file: shop holds per-flavor system/monolith/<lang>/VERSION,
		// scaffolded student repos hold one root VERSION. Must precede the
		// broader system/monolith/<lang> -> system rule below; otherwise the
		// path collapses to "system/VERSION" instead of "VERSION".
		{systemMonolithDir + lang + "/VERSION", "VERSION"},
		// Working directory
		{systemMonolithDir + lang, "system"},
		// System-test path
		{dirSystemTest + "/" + testLang + "/", dirSystemTest + "/"},
		{dirSystemTest + "/" + testLang, dirSystemTest},
		// Docker working-directory: shop keeps per-lang/arch subdirs under
		// docker/, but the scaffolder copies only the selected lang/arch up
		// into docker/, so "working-directory: docker/<testLang>/<arch>" must
		// flatten to "working-directory: docker".
		{wdPrefixYAML + dirDocker + "/" + testLang + "/monolith", wdPrefixYAML + dirDocker},
		// Same flattening for unprefixed CLI-arg forms (e.g. "--system-config
		// docker/<testLang>/monolith/systems.yaml" in per-suite acceptance-stage
		// `run:` lines), which the working-directory rule above does not cover.
		{dirDocker + "/" + testLang + "/monolith/", dirDocker + "/"},
		// Docker image names
		{prefixMonolithSystem + lang, "system"},
	}
	if lang != testLang {
		r = append(r,
			[2]string{prefixMonolithSystem + testLang, "system"},
			// Pipeline-stage workflows (acceptance/qa/prod) come from the testLang
			// template, so their `read-base-version` step references
			// system/monolith/<testLang>/VERSION. Scaffolded repos hold one root
			// VERSION (CopyVersion → VERSION; system/monolith/<testLang>/ does not
			// exist in the scaffold), so collapse this path to root VERSION too.
			[2]string{systemMonolithDir + testLang + "/VERSION", "VERSION"},
		)
	}
	return r
}

// systemPrefixDropReplacements strips system tag prefixes from workflow
// content. Each input is a tag prefix (the part before "-v") whose
// "<prefix>-v" form should collapse to "v" and whose "prefix: <prefix>"
// form should collapse to "prefix: ''".
//
// Callers pass:
//   - 2-segment prefixes (e.g. "monolith-typescript", "multitier-java") —
//     the flavor-level tag prefix used by every prod-stage's flavor-tag
//     publish and prerelease-version step.
//   - 3-segment per-component prefixes (e.g. "multitier-backend-dotnet",
//     "multitier-frontend-react") — the per-component release tags that
//     multitier prod-stages publish in addition to the flavor tag, and
//     that bump-patch-version files probe via the git-tag signal.
//
// Multi-prefix multitier callers (monorepo and multirepo root) must
// **prepend** the result before multitierContentReplacements: the bare
// 3-segment "multitier-backend-{lang}" → "backend" rule there is a
// substring of "multitier-backend-{lang}-v" and would partial-match into
// "backend-v" otherwise.
//
// Single-prefix monolith callers can keep appending — monolith content
// replacements have no bare 2-segment rule to collide with.
//
// Ordering inside the function: each prefix's "-v" form is emitted before
// its bare "prefix:" form so that "prefix: monolith-typescript" is not
// consumed early as a partial match of "tag-prefix: monolith-typescript-v".
func systemPrefixDropReplacements(archTests ...string) [][2]string {
	r := make([][2]string, 0, 2*len(archTests))
	for _, at := range archTests {
		r = append(r,
			// tag: / tag-prefix: inputs and description examples — rewrites
			// "<prefix>-v" to "v".
			[2]string{at + "-v", "v"},
			// compose-prerelease-version step — explicit empty string preserves
			// the key so the YAML shape matches what the action expects.
			[2]string{"prefix: " + at, "prefix: ''"},
		)
	}
	return r
}

// monolithDockerComposeReplacements returns docker-compose content replacements for monolith.
func monolithDockerComposeReplacements(lang, testLang string) [][2]string {
	r := [][2]string{
		{dirSystemTest + "/" + testLang + "/", dirSystemTest + "/"},
		{dirSystemTest + "/" + testLang, dirSystemTest},
		{prefixMonolithSystem + lang, "system"},
		// Docker build context: shop has system-test/{lang}/<arch>/ so ../../../system/monolith/{lang} is correct
		// there, but scaffold flattens arch subdir → system-test/, so the context becomes ../system.
		{shopSystemPrefix + "monolith/" + lang, "../system"},
		// External-system volume mounts + build contexts: shop's compose lives at
		// docker/<lang>/<arch>/, so it reaches external-systems/ via ../../../external-systems/.
		// Scaffold flattens that to docker/, so it reaches external-systems/ via ../external-systems/.
		{shopExternalSystemsPrefix, "../external-systems/"},
	}
	if lang != testLang {
		r = append(r, [2]string{shopSystemPrefix + "monolith/" + testLang, "../system"})
		r = append(r, [2]string{prefixMonolithSystem + testLang, "system"})
	}
	return r
}

// multitierContentReplacements returns workflow content replacements for multitier.
func multitierContentReplacements(backendLang, frontendLang, testLang string) [][2]string {
	multiTest := prefixMultitier + testLang
	// Shop's pipeline-stage env names derive from the workflow filename, which
	// is multitier-<testLang>-*-stage.yml. So the env prefix is testLang, not backendLang.
	envPrefix := multiTest + "-"
	r := [][2]string{
		// Environment references — scaffolded repos have unprefixed env names.
		{envPrefixYAML + envPrefix + "acceptance", envPrefixYAML + "acceptance"},
		{envPrefixYAML + envPrefix + "qa", envPrefixYAML + "qa"},
		{envPrefixYAML + envPrefix + "production", envPrefixYAML + "production"},
		// Workflow names for pipeline stages (longer patterns first)
		{multiTest + suffixAcceptanceStage, "acceptance-stage"},
		{multiTest + suffixQAStage, "qa-stage"},
		{multiTest + suffixQASignoff, "qa-signoff"},
		{multiTest + suffixProdStage, "prod-stage"},
		{multiTest + "-verify", "verify"},
		// bump-patch-version variant is keyed by backendLang (frontend is always react),
		// distinct from the pipeline-stage rewrites above which use testLang. This rule
		// catches the `name:` / `concurrency.group:` form inside the bump-patch-version
		// source itself (`multitier-<backendLang>-bump-patch-version.yml`).
		{prefixMultitier + backendLang + "-bump-patch-version", "bump-patch-version"},
		// Filename form used in `uses: ./.github/workflows/<flavor>-bump-patch-version.yml`
		// references inside prod-stage. Same shape as the rule above but keyed by
		// testLang because the prod-stage source is `multitier-<testLang>-prod-stage.yml`
		// and its `uses:` line hardcodes the same flavor's bump-patch-version filename —
		// `multitier-<testLang>-bump-patch-version.yml`. Without this, polyglot scaffolds
		// (backendLang != testLang) leave the shop filename in place and actionlint fails
		// with "could not read reusable workflow file".
		{prefixMultitier + testLang + "-bump-patch-version", "bump-patch-version"},
		// Docker working-directory: shop keeps per-lang/arch subdirs under
		// docker/, but the scaffolder copies only the selected lang/arch up
		// into docker/, so "working-directory: docker/<testLang>/<arch>" must
		// flatten to "working-directory: docker".
		{wdPrefixYAML + dirDocker + "/" + testLang + "/multitier", wdPrefixYAML + dirDocker},
		// Same flattening for unprefixed CLI-arg forms (e.g. "--system-config
		// docker/<testLang>/multitier/systems.yaml" in per-suite acceptance-stage
		// `run:` lines), which the working-directory rule above does not cover.
		{dirDocker + "/" + testLang + "/multitier/", dirDocker + "/"},
		// VERSION file: shop holds per-flavor system/multitier/<backendLang>/VERSION
		// (the system bundle), scaffolded multitier monorepo student repos hold
		// one root VERSION. Must precede the broader system/multitier/<lang>
		// rules below.
		{systemMultitierDir + backendLang + "/VERSION", "VERSION"},
		// Working directories (these also transform commit stage workflow names:
		// backend-{lang}-commit-stage -> backend-commit-stage, etc.)
		{systemMultitierBackend + backendLang, "backend"},
		{"system/multitier/frontend-" + frontendLang, "frontend"},
		// System-test path
		{dirSystemTest + "/" + testLang + "/", dirSystemTest + "/"},
		{dirSystemTest + "/" + testLang, dirSystemTest},
		// Docker image names (also transforms remaining workflow name references)
		{prefixMultitierBackend + backendLang, "backend"},
		{prefixMultitierFrontend + frontendLang, "frontend"},
	}
	// Pipeline-stage workflows (cloned from multitier-<testLang>-*-stage.yml) reference
	// backend-<testLang> in image URLs and VERSION paths — shop authors them assuming
	// backendLang == testLang. When the scaffold runs with a different backendLang, we
	// need the testLang variant too, in addition to the backendLang variant above (which
	// handles the backend commit-stage workflow).
	if backendLang != testLang {
		r = append(r,
			[2]string{systemMultitierBackend + testLang, "backend"},
			[2]string{prefixMultitierBackend + testLang, "backend"},
			// Pipeline-stage workflows (acceptance/qa/prod) come from the testLang
			// template, so their `read-base-version` step references
			// system/multitier/<testLang>/VERSION. Scaffolded repos hold one root
			// VERSION (CopyVersion → VERSION; system/multitier/<testLang>/ does not
			// exist in the scaffold), so collapse this path to root VERSION too.
			[2]string{systemMultitierDir + testLang + "/VERSION", "VERSION"},
		)
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
		// External-system volume mounts + build contexts: shop's compose lives at
		// docker/<lang>/<arch>/, so it reaches external-systems/ via ../../../external-systems/.
		// Scaffold flattens that to docker/, so it reaches external-systems/ via ../external-systems/.
		{shopExternalSystemsPrefix, "../external-systems/"},
	}
	// Docker build contexts always reference the test-lang backend and the frontend lang in the
	// shop layout (e.g. backend-typescript, frontend-react). After scaffolding these become
	// ../backend and ../frontend respectively, so we always need both replacements.
	r = append(r, [2]string{"../../../" + systemMultitierBackend + testLang, "../backend"})
	r = append(r, [2]string{prefixMultitierBackend + testLang, "backend"})
	r = append(r, [2]string{shopSystemPrefix + "multitier/frontend-" + frontendLang, "../frontend"})
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
func copyDocs(shop, repoDir, arch string) {
	dst := filepath.Join(repoDir, Names.TargetDocsDir)
	files.CopyDir(filepath.Join(shop, Expand(Names.ShopDocsArchDir, map[string]string{"arch": arch})), dst)
	files.CopyDir(filepath.Join(shop, Names.ShopDocsSharedDir), dst)
}

// copyCloudRunScripts copies setup-gcp.sh and teardown-gcp.sh from shop to repo.
func copyCloudRunScripts(shop, repoDir string) {
	for _, name := range []string{"setup-gcp.sh", "teardown-gcp.sh"} {
		src := filepath.Join(shop, name)
		if _, err := os.Stat(src); err == nil {
			files.CopyFile(src, filepath.Join(repoDir, name))
		}
	}
}

// ── Post-condition validation ──────────────────────────────────────────────

// ValidateNoLeftoverTemplateRefs checks that ApplyTemplate's content
// replacements covered every case. Each forbidden substring is a literal
// that the replacement rules promise to rewrite — if any remain anywhere in
// the scaffolded repo (workflows, source, tests, docs, build files), a
// replacement rule is missing a case. Fail with the concrete paths so the
// gap is obvious.
//
// Scanning the whole repo (not just workflows/compose) is intentional: these
// substrings have no legitimate place in a scaffolded project. A leftover in
// source or tests is just as much a bug as one in a workflow — it means a
// user's clone will reference paths or names that don't exist.
//
// Runs after commit+push, following the convention of
// ValidateNoLeftoverSystemNames (broken output visible in the remote for
// troubleshooting).
func ValidateNoLeftoverTemplateRefs(cfg *config.Config) {
	log.Info("Validating no leftover template references...")

	if cfg.DryRun {
		log.Info("[DRY RUN] Would validate no leftover template references")
		return
	}

	refs := forbiddenTemplateRefs(cfg)
	for _, repoDir := range collectRepoDirs(cfg) {
		if repoDir == "" {
			continue
		}
		checkNoTemplateRefs(repoDir, refs)
	}

	log.Success("No leftover template references")
}

// forbiddenTemplateRefs returns substrings that must not appear anywhere in
// the scaffolded repo after ApplyTemplate. Each is a literal the replacement
// rules target; any survivor indicates a missed case or an unhandled file
// extension.
func forbiddenTemplateRefs(cfg *config.Config) []string {
	if cfg.Arch == "monolith" {
		return monolithForbiddenRefs(cfg.Lang, cfg.TestLang)
	}
	// "react" is the shop-side framework token in forbidden refs
	// (multitier-frontend-react, system/multitier/frontend-react), not
	// cfg.FrontendLang ("typescript").
	return multitierForbiddenRefs(cfg.BackendLang, "react", cfg.TestLang)
}

func monolithForbiddenRefs(lang, testLang string) []string {
	refs := []string{
		prefixMonolith + lang + "-",                     // commit-stage workflow refs
		prefixMonolith + testLang + "-",                 // pipeline-stage refs + sweep residue
		prefixMonolithSystem + lang,                     // docker image name
		systemMonolithDir,                               // template source path
		"-" + prefixMonolith + lang,                     // sonar key suffix
		dirSystemTest + "/" + testLang + "/",            // un-flattened system-test path
		dirDocker + "/" + testLang + "/",                // un-flattened docker path
	}
	if lang != testLang {
		refs = append(refs, prefixMonolithSystem+testLang)
	}
	return refs
}

func multitierForbiddenRefs(backendLang, frontendLang, testLang string) []string {
	refs := []string{
		prefixMultitierBackend + backendLang + "-",
		prefixMultitierFrontend + frontendLang + "-",
		prefixMultitier + testLang + "-",
		systemMultitierDir,
		"-" + prefixMultitierBackend + backendLang,   // sonar key suffix
		"-" + prefixMultitierFrontend + frontendLang, // sonar key suffix
		dirSystemTest + "/" + testLang + "/",
		dirDocker + "/" + testLang + "/", // un-flattened docker path
	}
	if backendLang != testLang {
		refs = append(refs, prefixMultitierBackend+testLang)
	}
	return refs
}

func checkNoTemplateRefs(repoDir string, refs []string) {
	var failed bool
	for _, needle := range refs {
		hits := files.FindInTree(repoDir, needle)
		if len(hits) == 0 {
			continue
		}
		log.Warnf("Leftover template ref %q in %d file(s):", needle, len(hits))
		for _, f := range hits {
			log.Warnf("  %s", f)
		}
		failed = true
	}
	if failed {
		log.Fatalf("Template replacement incomplete in %s: leftover template references found.", repoDir)
	}
}
