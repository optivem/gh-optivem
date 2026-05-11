package steps

import (
	"maps"
	"os"
	"sort"
	"strings"

	"github.com/optivem/gh-optivem/internal/config"
)

// recognizedPlaceholders is the set of placeholder names that may appear in
// Names templates. Expand panics on any other placeholder, surfacing typos
// like ${langg} or ${LANG} at runtime; names_test.go enforces the same rule
// across every Names field at test time.
var recognizedPlaceholders = map[string]bool{
	"lang":         true,
	"backendLang":  true,
	"frontendLang": true,
	"testLang":     true,
	"arch":         true,
	"stage":        true,
	"stageSuffix":  true,
}

// Names is the central registry of file and folder name templates produced or
// consumed by the scaffolder. Each field is a template string with ${...}
// placeholders. Use Expand(template, vars) to substitute.
//
// Two-step process at every call site:
//
//	dst := Expand(Names.MonolithBumpPatchVersionWf, map[string]string{"lang": cfg.Lang})
//
// Keeping every naming convention in one struct lets a maintainer answer "what
// filenames does the scaffolder emit?" by reading this file alone.
var Names = struct {
	// ── Workflow source filenames in the shop ────────────────────────────

	MonolithCommitStageWf      string
	MonolithBumpPatchVersionWf string
	MonolithPipelineStageWf    string
	MonolithLegacyAcceptWf     string

	MultitierBackendCommitStageWf  string
	MultitierFrontendCommitStageWf string
	MultitierBumpPatchVersionWf    string
	MultitierBackendBumpPatchWf    string
	MultitierFrontendBumpPatchWf   string
	MultitierPipelineStageWf       string
	MultitierLegacyAcceptWf        string

	BumpPatchVersionMultirepoWf string
	CleanupWf                   string

	// ── Workflow destination filenames (after rename) ────────────────────

	DestCommitStageWf         string
	DestBackendCommitStageWf  string
	DestFrontendCommitStageWf string
	DestPipelineStageWf       string
	DestLegacyAcceptWf        string
	DestBumpPatchVersionWf    string

	// ── Shop directory templates ─────────────────────────────────────────

	ShopSystemMonolithDir       string
	ShopSystemMultitierBackend  string
	ShopSystemMultitierFrontend string
	ShopSystemTestDir           string
	ShopDockerDir               string
	ShopDocsArchDir             string
	ShopDocsSharedDir           string
	ShopVersionFile             string
	ShopExternalRealSimDir      string
	ShopExternalStubDir         string

	// ── Target repo directory names ──────────────────────────────────────

	TargetSystemDir     string
	TargetBackendDir    string
	TargetFrontendDir   string
	TargetSystemTestDir string
	TargetDockerDir     string
	TargetDocsDir       string
	TargetWorkflowsDir  string

	// ── Image / tag-prefix fragments ─────────────────────────────────────

	MonolithImageRef      string
	MultitierBackendRef   string
	MultitierFrontendRef  string
	MonolithFlavorPrefix  string
	MultitierFlavorPrefix string
}{
	MonolithCommitStageWf:      "monolith-${lang}-commit-stage.yml",
	MonolithBumpPatchVersionWf: "monolith-${lang}-bump-patch-version.yml",
	MonolithPipelineStageWf:    "monolith-${testLang}-${stage}${stageSuffix}.yml",
	MonolithLegacyAcceptWf:     "monolith-${testLang}-acceptance-stage-legacy.yml",

	MultitierBackendCommitStageWf:  "multitier-backend-${backendLang}-commit-stage.yml",
	MultitierFrontendCommitStageWf: "multitier-frontend-${frontendLang}-commit-stage.yml",
	MultitierBumpPatchVersionWf:    "multitier-${backendLang}-bump-patch-version.yml",
	MultitierBackendBumpPatchWf:    "multitier-backend-${backendLang}-bump-patch-version.yml",
	MultitierFrontendBumpPatchWf:   "multitier-frontend-${frontendLang}-bump-patch-version.yml",
	MultitierPipelineStageWf:       "multitier-${testLang}-${stage}${stageSuffix}.yml",
	MultitierLegacyAcceptWf:        "multitier-${testLang}-acceptance-stage-legacy.yml",

	BumpPatchVersionMultirepoWf: "bump-patch-version-multirepo.yml",
	CleanupWf:                   "cleanup.yml",

	DestCommitStageWf:         "commit-stage.yml",
	DestBackendCommitStageWf:  "backend-commit-stage.yml",
	DestFrontendCommitStageWf: "frontend-commit-stage.yml",
	DestPipelineStageWf:       "${stage}.yml",
	DestLegacyAcceptWf:        "acceptance-stage-legacy.yml",
	DestBumpPatchVersionWf:    "bump-patch-version.yml",

	ShopSystemMonolithDir:       "system/monolith/${lang}",
	ShopSystemMultitierBackend:  "system/multitier/backend-${backendLang}",
	ShopSystemMultitierFrontend: "system/multitier/frontend-${frontendLang}",
	ShopSystemTestDir:           "system-test/${testLang}",
	ShopDockerDir:               "docker/${testLang}/${arch}",
	ShopDocsArchDir:             "docs/design/${arch}",
	ShopDocsSharedDir:           "docs/design/shared",
	ShopVersionFile:             "system/${arch}/${lang}/VERSION",
	ShopExternalRealSimDir:      "external-systems/external-real-sim",
	ShopExternalStubDir:         "external-systems/external-stub",

	TargetSystemDir:     "system",
	TargetBackendDir:    "backend",
	TargetFrontendDir:   "frontend",
	TargetSystemTestDir: "system-test",
	TargetDockerDir:     "docker",
	TargetDocsDir:       "docs",
	TargetWorkflowsDir:  ".github/workflows",

	MonolithImageRef:      "monolith-system-${lang}",
	MultitierBackendRef:   "multitier-backend-${backendLang}",
	MultitierFrontendRef:  "multitier-frontend-${frontendLang}",
	MonolithFlavorPrefix:  "monolith-${lang}",
	MultitierFlavorPrefix: "multitier-${lang}",
}

// Expand substitutes ${name} placeholders in tmpl using vars. Unknown
// placeholders panic — the placeholder name must be in
// recognizedPlaceholders. Missing keys in vars expand to empty string, which
// is intentional for optional fields (e.g. ${stageSuffix} is "" for docker
// deploy, "-cloud" for cloud-run).
func Expand(tmpl string, vars map[string]string) string {
	return os.Expand(tmpl, func(k string) string {
		if !recognizedPlaceholders[k] {
			panic("unknown placeholder ${" + k + "} in template; recognized: " + recognizedNames())
		}
		return vars[k]
	})
}

// ExpandRef is Expand followed by stripping a trailing ".yml" — the form used
// in workflow `name:` / `concurrency.group:` / `uses:` substring patterns.
func ExpandRef(tmpl string, vars map[string]string) string {
	return strings.TrimSuffix(Expand(tmpl, vars), ".yml")
}

// VarsForCfg builds the placeholder map for a Config. Stage and stageSuffix
// are not set (callers add per-call values).
//
// frontendLang is hardcoded to "react" — the placeholder feeds shop-side
// paths (system/multitier/frontend-react/, multitier-frontend-react-*.yml)
// where "react" is the framework directory token, not the source language.
// cfg.FrontendLang ("typescript") is the user-facing source language and is
// not used here.
func VarsForCfg(cfg *config.Config) map[string]string {
	return map[string]string{
		"lang":         cfg.Lang,
		"backendLang":  cfg.BackendLang,
		"frontendLang": "react",
		"testLang":     cfg.TestLang,
		"arch":         cfg.Arch,
		"stageSuffix":  cloudRunSuffix(cfg.Deploy),
	}
}

// MergeVars returns a new map with base entries overridden by override entries.
func MergeVars(base, override map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(override))
	maps.Copy(out, base)
	maps.Copy(out, override)
	return out
}

func recognizedNames() string {
	out := make([]string, 0, len(recognizedPlaceholders))
	for k := range recognizedPlaceholders {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}
