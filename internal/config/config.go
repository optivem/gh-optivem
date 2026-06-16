// Package config provides CLI parsing, validation, and the Config struct.
package config

import (
	"fmt"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/optivem/gh-optivem/internal/kernel/approval"
	"github.com/optivem/gh-optivem/internal/kernel/cmdctx"
	"github.com/optivem/gh-optivem/internal/kernel/log"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
	"github.com/optivem/gh-optivem/internal/kernel/version"
)

// RawInputs captures the raw CLI flag values before any resolution/defaulting.
// Surfaced in the startup banner so the user can audit exactly what they typed
// (vs. the resolved/derived values the scaffold will actually act on).
type RawInputs struct {
	Repo         string // --repo as-passed
	ShopRef      string // --shop-ref before resolving empty → baked-in SHA → latest meta-v*
	TestLang     string // --test-lang (required; the system-test tier's language is independent of the impl lang)
	VerifyLevel  string // --verify-level as-passed (flag default: "release")
	WorkDir      string // --workdir before defaulting to temp dir
	KeepLocal    bool   // --keep-local flag as-passed
	RandomSuffix bool   // --random-suffix flag as-passed
}

type Config struct {
	Owner        string
	Repo         string
	FullRepo     string
	SystemName   string
	Arch         string // "monolith" or "multitier"
	RepoStrategy string // "monorepo" or "multirepo"

	Raw RawInputs

	// UserSetFlags is the set of flag names the user passed explicitly on the
	// command line (populated via flag.Visit after flag.Parse). Lets the banner
	// distinguish user-provided values from defaults by appending "(default)"
	// to unset flags.
	UserSetFlags map[string]bool

	Lang         string // monolith only
	BackendLang  string // multitier only
	FrontendLang string // multitier only
	TestLang     string

	// Tier paths written into gh-optivem.yaml. Each is required when Arch is
	// set (the YAML emitter does no path derivation — every call site that
	// produces a Config supplies the paths matching its own on-disk layout).
	// SystemPath applies only to monolith; BackendPath/FrontendPath apply only
	// to multitier. SystemTestPath applies to both.
	SystemPath     string
	SystemTestPath string
	BackendPath    string
	FrontendPath   string

	Deploy  string // "docker" (default) or "cloud-run"
	License string

	// ProjectURL is the GitHub Project URL written into gh-optivem.yaml at the
	// "Write gh-optivem.yaml" step. Empty is allowed — the file is still
	// generated, just with project.url absent. If left empty here, the
	// operator must set it before running the ATDD pipeline (the runtime
	// has no discovery fallback).
	ProjectURL string

	// SourceConfigPath is the absolute path of an on-disk gh-optivem.yaml
	// that init read or was asked to write to via --config / $GH_OPTIVEM_CONFIG
	// (or a pre-existing file at the default CWD path). Empty when init ran
	// with the default path and auto-generated the config in memory —
	// steps.WriteOptivemYAML materializes the only on-disk copy inside the
	// scaffold dir. Downstream guards on SourceConfigPath == "" rely on this
	// contract: empty is not a defensive corner case but the expected state
	// for default-path runs that didn't find a pre-existing file.
	SourceConfigPath string

	// SourceProjectURLWasEmpty records whether project.url in the source
	// gh-optivem.yaml was empty at init startup. The Path A write-back gate
	// reads this so reused-by-title runs (where the URL is already set in
	// the source file) leave the file alone — no churn, no marshalling
	// round-trip that drops comments.
	SourceProjectURLWasEmpty bool

	VerifyLevel  string // "none", "local", "commit", "acceptance", "qa", "release"
	NoLegacy     bool   // exclude legacy from local tests and acceptance stage
	NoLocalTests bool   // skip the "Verify local testing" step (runner package over system-test/)
	NoLocalSonar bool   // skip the "Verify local SonarCloud scan" step (per-component run-sonar.sh)
	NoAtdd       bool   // skip the "Install ATDD assets" step (skip copying agents/commands/prompts from shop)
	NoProject    bool   // skip the "Ensure project board" step entirely (no auto-create, no status-ensure on supplied URL)
	KeepLocal    bool   // keep the local scaffolded clone dir after a successful run (default: delete it)
	BugReport    bool   // opt in to auto-creating a GitHub issue on failure (default: off)
	Verbose      bool   // enable debug output
	Quiet        bool   // suppress info-level output
	LogFile      string // optional path to mirror plain-text log output
	AssumeYes    bool   // skip all interactive confirmations (existing-repo, --report-bug)

	// Approval is the global auto-approve policy resolved from --auto /
	// --confirm at root command startup. Init reads it in runInit and the
	// scaffolder threads it through to every y/n confirmation site
	// (confirmReposExist, readProjectConfirmation, bug-report) so the
	// policy applies consistently regardless of where the prompt lives.
	// Zero value (Auto=false, ConfirmFloor at the zero tier) is the safe
	// cautious default — confirmations fall through to the interactive
	// prompt.
	Approval approval.Resolved
	WorkDir  string
	ShopPath string
	ShopRef  string // Pinned optivem/shop ref (SHA, tag, or branch). Never empty.

	DockerHubUsername string
	DockerHubToken    string
	SonarToken        string
	GHCRToken         string
	WorkflowToken     string
	RepoToken         string // Classic PAT with `repo` scope for cross-repo Contents:read in multirepo prod-stage

	// Derived naming
	OwnerPascal   string
	OwnerLower    string
	RepoPascal    string
	RepoNoHyphens string

	// Casings for the generic placeholder passes. OwnerCasings maps the 6
	// company forms (MyCompany / myCompany / my-company / mycompany /
	// my_company / MY_COMPANY) to the user's owner; SysNameCasings maps the
	// 6 system forms (MyShop / myShop / my-shop / myshop / my_shop / MY_SHOP)
	// to the user's system name.
	OwnerCasings   Casings
	SysNameCasings Casings

	// Namespace patterns
	JavaNsOld   string
	JavaNsNew   string
	DotnetNsOld string
	DotnetNsNew string
	TsPkgOld    string
	TsPkgNew    string

	// System name casing variants (template -> user)
	SysNamePascalOld string // "Shop"
	SysNamePascalNew string // "SkyTravel"
	SysNameCamelOld  string // "shop"
	SysNameCamelNew  string // "skyTravel"
	SysNameKebabOld  string // "shop"
	SysNameKebabNew  string // "sky-travel"
	SysNameLowerOld  string // "shop"
	SysNameLowerNew  string // "skytravel"

	// Multi-repo (multitier)
	FrontendRepo     string
	BackendRepo      string
	FrontendFullRepo string
	BackendFullRepo  string

	// Multi-repo (monolith)
	SystemRepo     string
	SystemFullRepo string

	// Set after clone
	RepoDir         string
	FrontendRepoDir string
	BackendRepoDir  string
	SystemRepoDir   string

	// PreExistingRepos is the set of "owner/name" repos that already existed on
	// GitHub when scaffold started. Used by finalize: a clean working tree at
	// commit time is acceptable for these (re-scaffold of identical content is
	// a no-op), but is treated as a hard error for freshly created repos
	// (signals the template apply produced nothing).
	PreExistingRepos map[string]bool
}

// Casings is the set of case variants for a single identifier token (owner or
// system name). Produced by OwnerCasings / SystemCasings and consumed by the
// generic placeholder replacement passes in internal/steps.
type Casings struct {
	Pascal    string // "MyCompany"
	Camel     string // "myCompany"
	Kebab     string // "my-company"
	Lower     string // "mycompany" (no separator)
	Snake     string // "my_company"
	Screaming string // "MY_COMPANY"
}

// OwnerCasings derives the 6 case variants from a GitHub owner name. Owner
// names follow GitHub rules (alphanumeric + single hyphens), so word
// boundaries are the hyphens.
func OwnerCasings(owner string) Casings {
	return casingsFromWords(strings.Split(strings.ToLower(owner), "-"))
}

// SystemCasings derives the 6 case variants from a space-separated system
// name, e.g. "Page Turner" → Pascal "PageTurner", Kebab "page-turner", etc.
func SystemCasings(name string) Casings {
	words := strings.Fields(name)
	lower := make([]string, len(words))
	for i, w := range words {
		lower[i] = strings.ToLower(w)
	}
	return casingsFromWords(lower)
}

func casingsFromWords(words []string) Casings {
	if len(words) == 0 {
		return Casings{}
	}
	var pascal, camel strings.Builder
	for i, w := range words {
		if len(w) == 0 {
			continue
		}
		titled := strings.ToUpper(w[:1]) + w[1:]
		pascal.WriteString(titled)
		if i == 0 {
			camel.WriteString(w)
		} else {
			camel.WriteString(titled)
		}
	}
	kebab := strings.Join(words, "-")
	snake := strings.Join(words, "_")
	return Casings{
		Pascal:    pascal.String(),
		Camel:     camel.String(),
		Kebab:     kebab,
		Lower:     strings.Join(words, ""),
		Snake:     snake,
		Screaming: strings.ToUpper(snake),
	}
}

func ToPascalCase(s string) string {
	parts := strings.Split(s, "-")
	var b strings.Builder
	for _, p := range parts {
		if len(p) > 0 {
			b.WriteString(strings.ToUpper(p[:1]) + p[1:])
		}
	}
	return b.String()
}

func ToJavaLower(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, "-", ""))
}

// SplitCamelCase splits a camelCase or PascalCase string into words.
// Consecutive uppercase letters are treated as an acronym.
// Examples: "skyTravel" -> ["sky", "Travel"], "ABCStore" -> ["ABC", "Store"],
// "myAPIClient" -> ["my", "API", "Client"], "eShop" -> ["e", "Shop"]
func SplitCamelCase(s string) []string {
	if len(s) == 0 {
		return nil
	}
	var words []string
	start := 0
	for i := 1; i < len(s); i++ {
		if isUpper(s[i]) {
			// Split before s[i] when either:
			//   - previous char was lowercase (lowercase -> uppercase boundary), or
			//   - next char is lowercase (end of an acronym run).
			if !isUpper(s[i-1]) || (i+1 < len(s) && !isUpper(s[i+1])) {
				words = append(words, s[start:i])
				start = i
			}
		}
	}
	words = append(words, s[start:])
	return words
}

func isUpper(b byte) bool {
	return b >= 'A' && b <= 'Z'
}

// CamelCaseToPascal converts camelCase to PascalCase: "skyTravel" -> "SkyTravel"
func CamelCaseToPascal(s string) string {
	if len(s) == 0 {
		return s
	}
	words := SplitCamelCase(s)
	var b strings.Builder
	for _, w := range words {
		b.WriteString(strings.ToUpper(w[:1]) + w[1:])
	}
	return b.String()
}

// CamelCaseToKebab converts camelCase to kebab-case: "skyTravel" -> "sky-travel"
func CamelCaseToKebab(s string) string {
	words := SplitCamelCase(s)
	lower := make([]string, len(words))
	for i, w := range words {
		lower[i] = strings.ToLower(w)
	}
	return strings.Join(lower, "-")
}

// CamelCaseToLower converts camelCase to lowercase: "skyTravel" -> "skytravel"
func CamelCaseToLower(s string) string {
	return strings.ToLower(s)
}

// SpacesToCamel converts space-separated words to camelCase: "Sky Travel" -> "skyTravel"
func SpacesToCamel(s string) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(strings.ToLower(words[0]))
	for _, w := range words[1:] {
		if len(w) > 0 {
			b.WriteString(strings.ToUpper(w[:1]) + w[1:])
		}
	}
	return b.String()
}

// SpacesToPascal converts space-separated words to PascalCase: "Sky Travel" -> "SkyTravel"
func SpacesToPascal(s string) string {
	words := strings.Fields(s)
	var b strings.Builder
	for _, w := range words {
		if len(w) > 0 {
			b.WriteString(strings.ToUpper(w[:1]) + w[1:])
		}
	}
	return b.String()
}

// SpacesToKebab converts space-separated words to kebab-case: "Sky Travel" -> "sky-travel"
func SpacesToKebab(s string) string {
	words := strings.Fields(s)
	lower := make([]string, len(words))
	for i, w := range words {
		lower[i] = strings.ToLower(w)
	}
	return strings.Join(lower, "-")
}

// SpacesToLower converts space-separated words to lowercase: "Sky Travel" -> "skytravel"
func SpacesToLower(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, " ", ""))
}

// ValidateOwnerFormat checks the owner against GitHub's username/org naming rules.
// Returns an error message or empty string if valid.
//
// GitHub rules: 1–39 chars; alphanumeric or single hyphens; cannot begin or end
// with a hyphen; cannot contain consecutive hyphens.
func ValidateOwnerFormat(owner string) string {
	if len(owner) == 0 {
		return "owner cannot be empty"
	}
	if len(owner) > 39 {
		return "owner exceeds 39 character limit"
	}
	if strings.HasPrefix(owner, "-") || strings.HasSuffix(owner, "-") {
		return "owner cannot start or end with a hyphen"
	}
	if strings.Contains(owner, "--") {
		return "owner cannot contain consecutive hyphens"
	}
	for _, c := range owner {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-') {
			return fmt.Sprintf("owner contains invalid character %q — only alphanumeric and hyphens allowed", c)
		}
	}
	return ""
}

// ValidateRepoFormat checks the repo name against GitHub's repository naming rules.
// Returns an error message or empty string if valid.
//
// GitHub rules: 1–100 chars; alphanumeric, hyphen, underscore, or period;
// cannot start with hyphen or period; cannot be "." or "..".
func ValidateRepoFormat(repo string) string {
	if len(repo) == 0 {
		return "repo cannot be empty"
	}
	if len(repo) > 100 {
		return "repo exceeds 100 character limit"
	}
	if repo == "." || repo == ".." {
		return fmt.Sprintf("repo cannot be %q", repo)
	}
	if strings.HasPrefix(repo, ".") || strings.HasPrefix(repo, "-") {
		return "repo cannot start with '.' or '-'"
	}
	for _, c := range repo {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.') {
			return fmt.Sprintf("repo contains invalid character %q — only alphanumeric, hyphen, underscore, period allowed", c)
		}
	}
	return ""
}

// ValidateSystemName checks the system name against all naming constraints.
// Returns an error message or empty string if valid.
//
// Thin shim over projectconfig.ValidateSystemName — the canonical rules
// live with the YAML schema so `gh-optivem.yaml` round-trip validation
// and `--system-name` flag validation share one source of truth.
func ValidateSystemName(name string) string {
	return projectconfig.ValidateSystemName(name)
}

// ValidateArch checks --arch / interactive arch input.
// Returns an error message or empty string if valid.
func ValidateArch(arch string) string {
	if arch != "monolith" && arch != "multitier" {
		return "must be 'monolith' or 'multitier'"
	}
	return ""
}

// ValidateRepoStrategy checks --repo-strategy / interactive repo-strategy input.
// Returns an error message or empty string if valid.
func ValidateRepoStrategy(rs string) string {
	if rs != "monorepo" && rs != "multirepo" {
		return "must be 'monorepo' or 'multirepo'"
	}
	return ""
}

// ValidateBackendLang checks the backend / monolith language input.
// Returns an error message or empty string if valid.
//
// One validator covers both --monolith-lang and --backend-lang (the
// interactive prompt asks one or the other depending on arch); the
// frontend lang is a separate validator because its allowed set is
// narrower today.
func ValidateBackendLang(lang string) string {
	if !IsValidLang(lang) {
		return "must be 'java', 'dotnet', or 'typescript'"
	}
	return ""
}

// ValidateProjectURLFormat checks --project-url / interactive project-url
// input for canonical https://github.com/{orgs,users}/<owner>/projects/<n>
// shape. Empty is treated as valid: the flag path documents empty as
// "let `gh optivem init` Path A auto-create a board", and the interactive
// prompt mirrors that. Existence is a separate concern — see
// CheckProjectExists.
func ValidateProjectURLFormat(url string) string {
	if url == "" {
		return ""
	}
	if !strings.HasPrefix(url, "https://github.com/") {
		return "must be a https://github.com/... URL (e.g. https://github.com/orgs/acme/projects/1)"
	}
	if _, _, _, err := parseProjectURL(url); err != nil {
		return "must match https://github.com/orgs/<org>/projects/<n> or https://github.com/users/<user>/projects/<n>"
	}
	return ""
}

// RawFlags holds the unparsed CLI flag values for `init`. Flags bind directly
// to these fields on the Cobra command via BindInitFlags; ParseAndValidate then
// consumes the populated struct after Cobra has parsed the command line.
type RawFlags struct {
	Owner          string
	SystemName     string
	Repo           string
	Arch           string
	RepoStrategy   string
	Lang           string
	TestLang       string
	BackendLang    string
	FrontendLang   string
	License        string
	VerifyLevel    string
	Deploy         string
	WorkDir        string
	ShopRef        string
	LogFile        string
	ProjectURL     string
	SystemPath     string
	SystemTestPath string
	BackendPath    string
	FrontendPath   string
	KeepLocal      bool
	NoLegacy       bool
	NoLocalTests   bool
	NoLocalSonar   bool
	NoAtdd         bool
	NoProject      bool
	BugReport      bool
	Verbose        bool
	Quiet          bool
	AssumeYes      bool
}

// bindYAMLAffectingFlags binds the subset of init flags that influence the
// generated gh-optivem.yaml. Shared between BindInitFlags and BindConfigInitFlags
// so adding a new YAML-affecting flag (e.g. another scope axis) flows to both
// the `init` and `config init` surfaces in lockstep.
//
// Project-stable scalars (--system-name, --license, --deploy) live here too:
// the YAML now carries the templating inputs `init` needs to scaffold, not
// just the scope axes the ATDD runtime reads. See the corresponding
// system_name / license / deploy YAML fields on projectconfig.Config.
func bindYAMLAffectingFlags(fs *pflag.FlagSet, f *RawFlags) {
	fs.StringVar(&f.Owner, "owner", "", "GitHub username or org (required)")
	fs.StringVar(&f.Repo, "repo", "", "Repository name, e.g. page-turner (required, or pass positionally)")
	fs.StringVar(&f.SystemName, "system-name", "", `System name, e.g. "Page Turner" (required)`)
	fs.StringVar(&f.Arch, "arch", "", "Architecture: monolith or multitier (required)")
	fs.StringVar(&f.RepoStrategy, "repo-strategy", "", "Repo strategy: monorepo or multirepo (required)")
	fs.StringVar(&f.Lang, "monolith-lang", "", "System language: java, dotnet, typescript (monolith)")
	fs.StringVar(&f.TestLang, "test-lang", "", "System-test language: java, dotnet, typescript (required; independent of --monolith-lang / --backend-lang)")
	fs.StringVar(&f.BackendLang, "backend-lang", "", "Backend language: java, dotnet, typescript (multitier)")
	fs.StringVar(&f.FrontendLang, "frontend-lang", "", "Frontend language: typescript (multitier)")
	fs.StringVar(&f.License, "license", "mit", "License: mit, apache-2.0, gpl-3.0, bsd-2-clause, bsd-3-clause, unlicense")
	fs.StringVar(&f.Deploy, "deploy", "docker", "Deployment target: docker or cloud-run")
	fs.StringVar(&f.ProjectURL, "project-url", "", "GitHub Project URL to bake into gh-optivem.yaml (required; e.g. https://github.com/orgs/<org>/projects/<n>)")
	// Tier paths default to the flat scaffold layout — the same layout
	// `gh optivem init` itself produces. Pass these flags only to point
	// the YAML at a non-flat existing repo.
	fs.StringVar(&f.SystemPath, "system-path", "", "Repo-relative path to system code (monolith only; default: "+DefaultSystemPath+")")
	fs.StringVar(&f.SystemTestPath, "system-test-path", "", "Repo-relative path to system tests (default: "+DefaultSystemTestPath+")")
	fs.StringVar(&f.BackendPath, "backend-path", "", "Repo-relative path to backend code (multitier only; default: "+DefaultBackendPath+")")
	fs.StringVar(&f.FrontendPath, "frontend-path", "", "Repo-relative path to frontend code (multitier only; default: "+DefaultFrontendPath+")")
}

// BindInitFlags binds every `gh optivem init` CLI flag to the corresponding
// field on f. Cobra parses on Execute(); ParseAndValidate then validates the
// populated struct.
//
// Two flag groups land here:
//   - YAML-affecting flags (--owner, --repo, --system-name, --arch,
//     --repo-strategy, --monolith-lang / --backend-lang / --frontend-lang,
//     --test-lang, --project-url, --license, --deploy, tier-path overrides)
//     bound via bindYAMLAffectingFlags — same set lives on `config init`.
//   - Per-invocation flags (--keep-local, --verify-level, --no-*, --workdir,
//     --shop-ref, logging flags, --yes) bound directly below.
//
// The YAML-affecting flags live on `init` so non-interactive callers (CI,
// `go test` smoke matrix) can scaffold in one command without a pre-existing
// gh-optivem.yaml. When passed, runInit writes them to the YAML before
// loading; when absent, runInit prompts on a TTY or returns
// MissingFileError non-TTY.
func BindInitFlags(cmd *cobra.Command, f *RawFlags) {
	bindYAMLAffectingFlags(cmd.Flags(), f)
	fs := cmd.Flags()
	fs.BoolVar(&f.KeepLocal, "keep-local", false, "Keep the local scaffolded clone dir instead of deleting it on success")
	fs.StringVar(&f.VerifyLevel, "verify-level", "release", "Verification level: none, local, commit, acceptance, qa, release")
	fs.BoolVar(&f.NoLegacy, "no-legacy", false, "Exclude legacy from local tests and acceptance stage")
	fs.BoolVar(&f.NoLocalTests, "no-local-tests", false, "Skip the 'Verify local testing' step (runner package over system-test/)")
	fs.BoolVar(&f.NoLocalSonar, "no-local-sonar", false, "Skip the 'Verify local SonarCloud scan' step (per-component run-sonar.sh against the SonarCloud project)")
	fs.BoolVar(&f.NoAtdd, "no-atdd", false, "Skip installing ATDD agents/commands/prompts from shop into the scaffolded repo")
	fs.BoolVar(&f.NoProject, "no-project", false, "Skip the 'Ensure project board' step entirely (no auto-create, no status-ensure on a supplied --project-url). For CI smoke tests of the scaffolder, or to manage the board out-of-band.")
	fs.BoolVar(&f.BugReport, "report-bug", false, "On failure, auto-create a GitHub issue in optivem/gh-optivem with scaffold config. Off by default — file one yourself if the failure is worth reporting.")
	fs.StringVar(&f.WorkDir, "workdir", "", "Working directory for cloning (default: temp dir)")
	fs.StringVar(&f.ShopRef, "shop-ref", "", "Pin optivem/shop to this ref (tag, SHA, or branch — e.g. meta-v1.2.3, main, a1b2c3d). Overrides build-time pin. Default: latest meta-v* release.")
	fs.BoolVarP(&f.Verbose, "verbose", "v", false, "Enable debug output (retry/wait chatter, diagnostics)")
	fs.BoolVarP(&f.Quiet, "quiet", "q", false, "Suppress info-level output (warnings and errors still shown)")
	fs.StringVar(&f.LogFile, "log-file", "", "Override path for the plain-text log mirror (default: $TEMP/gh-optivem-<timestamp>.log; always written so failures can be attached to bug reports)")
	fs.BoolVarP(&f.AssumeYes, "yes", "y", false, "Skip all interactive confirmations (existing-repo prompt, --report-bug confirmation, project-board status-ensure on supplied --project-url). Expected pattern for CI/unattended runs.")
}

// BindConfigInitFlags binds the YAML-affecting flag subset for `gh optivem
// config init`. The --force / --dir flags are command-local and bound by the
// caller (newConfigInitCmd); they don't belong on RawFlags because they have
// no analog on `init`.
func BindConfigInitFlags(cmd *cobra.Command, f *RawFlags) {
	bindYAMLAffectingFlags(cmd.Flags(), f)
}

func resolveVerifyLevel(level string) string {
	validLevels := map[string]bool{"none": true, "local": true, "commit": true, "acceptance": true, "qa": true, "release": true}
	if !validLevels[level] {
		log.FatalExit("--verify-level must be none, local, commit, acceptance, qa, or release")
	}
	return level
}

func validateCommonFlags(deploy, arch, repoStrategy string) {
	if deploy != "docker" && deploy != "cloud-run" {
		log.FatalExit("--deploy must be 'docker' or 'cloud-run'")
	}
	// TODO: --deploy cloud-run is in development. The scaffolded cloud
	// workflows still carry shop-prefixed service/endpoint identifiers
	// ("service: monolith-<lang>-acceptance", "endpoints: [{\"name\": ...}]")
	// that aren't rewritten during template application. Enable once
	// apply_template.go strips these prefixes the same way env names were
	// stripped (see monolithContentReplacements / multitierContentReplacements).
	if deploy == "cloud-run" {
		log.FatalExit("--deploy cloud-run is in development and may be available in a future release. Use --deploy docker for now.")
	}
	if msg := ValidateArch(arch); msg != "" {
		log.FatalExit("--arch " + msg)
	}
	if msg := ValidateRepoStrategy(repoStrategy); msg != "" {
		log.FatalExit("--repo-strategy " + msg)
	}
}

type langChoice struct {
	lang, backendLang, frontendLang, testLang string
}

// IsValidLang reports whether s is one of the languages gh-optivem knows how
// to scaffold and compile. The set is shared between resolveLangs (the
// init-time validator) and the `environment verify --lang` flag validator,
// per feedback_interactive_validation_parity.md — one source of truth.
func IsValidLang(s string) bool {
	switch s {
	case "java", "dotnet", "typescript":
		return true
	}
	return false
}

// collectLangs flattens a langChoice into the unique non-empty languages the
// scaffold will compile. The slice is the input to compilerChecksFor —
// order doesn't matter (the dispatcher dedupes internally). resolveLangs
// runs upstream of every caller, so each non-empty entry is already one of
// the IsValidLang set.
func collectLangs(lc langChoice) []string {
	var out []string
	for _, l := range []string{lc.lang, lc.backendLang, lc.frontendLang, lc.testLang} {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func resolveLangs(f *RawFlags) langChoice {
	validLangs := map[string]bool{"java": true, "dotnet": true, "typescript": true}
	var c langChoice
	if f.Arch == "monolith" {
		if f.Lang == "" {
			log.FatalExit("--monolith-lang is required for monolith architecture")
		}
		if !validLangs[f.Lang] {
			log.FatalExit("--monolith-lang must be java, dotnet, or typescript")
		}
		c.lang = f.Lang
		if f.TestLang == "" {
			log.FatalExit("--test-lang is required (the system-test tier's language is not derived from --monolith-lang)")
		}
		if !validLangs[f.TestLang] {
			log.FatalExit("--test-lang must be java, dotnet, or typescript")
		}
		c.testLang = f.TestLang
		return c
	}
	if f.BackendLang == "" {
		log.FatalExit("--backend-lang is required for multitier architecture")
	}
	if f.FrontendLang == "" {
		log.FatalExit("--frontend-lang is required for multitier architecture")
	}
	if !validLangs[f.BackendLang] {
		log.FatalExit("--backend-lang must be java, dotnet, or typescript")
	}
	if f.FrontendLang != "typescript" {
		log.FatalExit("--frontend-lang must be typescript")
	}
	c.backendLang = f.BackendLang
	c.frontendLang = f.FrontendLang
	if f.TestLang == "" {
		log.FatalExit("--test-lang is required (the system-test tier's language is not derived from --backend-lang)")
	}
	if !validLangs[f.TestLang] {
		log.FatalExit("--test-lang must be java, dotnet, or typescript")
	}
	c.testLang = f.TestLang
	return c
}

// Default tier paths — the flat layout `gh optivem init` itself produces.
// resolvePathFlagsForYAML fills empties with these so `gh optivem config
// init` defaults to the same layout without the operator having to type
// six flags whose values are already the obvious ones.
const (
	DefaultSystemPath     = "system"
	DefaultSystemTestPath = "system-test"
	DefaultBackendPath    = "backend"
	DefaultFrontendPath   = "frontend"
)

type envTokens struct {
	dockerHubUsername, dockerHubToken, sonarToken, ghcrToken, workflowToken, repoToken string
}

func readEnvTokens() envTokens {
	return envTokens{
		dockerHubUsername: os.Getenv("DOCKERHUB_USERNAME"),
		dockerHubToken:    os.Getenv("DOCKERHUB_TOKEN"),
		sonarToken:        os.Getenv("SONAR_TOKEN"),
		ghcrToken:         os.Getenv("GHCR_TOKEN"),
		workflowToken:     os.Getenv("WORKFLOW_TOKEN"),
		repoToken:         os.Getenv("REPO_TOKEN"),
	}
}

// requiredEnvVar pairs a credential env-var name with its current value,
// so presence checks and live-auth checks iterate the same source.
type requiredEnvVar struct{ name, val string }

// requiredEnvVars returns the canonical (name, current-value) list of the
// environment variables every gh-optivem pipeline run needs. It is the
// single source of truth shared by the live-auth pass (VerifyEnvironment)
// and the presence-only preflight check (MissingRequiredEnvVars) so the two
// can never drift on which vars count as required.
func requiredEnvVars() []requiredEnvVar {
	e := readEnvTokens()
	return []requiredEnvVar{
		{"DOCKERHUB_USERNAME", e.dockerHubUsername},
		{"DOCKERHUB_TOKEN", e.dockerHubToken},
		{"SONAR_TOKEN", e.sonarToken},
		{"GHCR_TOKEN", e.ghcrToken},
		{"WORKFLOW_TOKEN", e.workflowToken},
		{"REPO_TOKEN", e.repoToken},
	}
}

// MissingRequiredEnvVars returns the names of every required credential env
// var that is currently unset, in canonical order. Presence-only: it does
// not hit the network. Preflight wires this into preflight.Run so a missing
// var folds into the one aggregated failure block alongside repo/tier/suite
// failures — the operator sees every gap in one pass and fixes them with a
// single shell restart instead of fix-one-restart-discover-next.
func MissingRequiredEnvVars() []string {
	var missing []string
	for _, r := range requiredEnvVars() {
		if r.val == "" {
			missing = append(missing, r.name)
		}
	}
	return missing
}

func resolveShopRef(shopRef string) string {
	ref := shopRef
	if ref == "" {
		ref = version.ShopRef
	}
	if ref == "" {
		latest, err := latestMetaRelease()
		if err != nil {
			log.FatalExit("Cannot resolve shop ref: " + err.Error())
		}
		ref = latest
		// Surfaced in the banner's Derived block; log at debug for --verbose traces.
		log.Debugf("Resolved empty shop-ref to latest meta-v* release: %s", ref)
	}
	return ref
}

type multirepoNames struct {
	frontendRepo, backendRepo, frontendFullRepo, backendFullRepo string
	systemRepo, systemFullRepo                                   string
}

func deriveMultirepoNames(strategy, arch, owner, repoName string) multirepoNames {
	var m multirepoNames
	if strategy != "multirepo" {
		return m
	}
	if arch == "multitier" {
		m.frontendRepo = repoName + "-frontend"
		m.backendRepo = repoName + "-backend"
		m.frontendFullRepo = owner + "/" + m.frontendRepo
		m.backendFullRepo = owner + "/" + m.backendRepo
	} else {
		m.systemRepo = repoName + "-system"
		m.systemFullRepo = owner + "/" + m.systemRepo
	}
	return m
}

func resolveWorkDir(wd string) string {
	if wd != "" {
		return wd
	}
	dir, err := os.MkdirTemp("", "scaffold-")
	if err != nil {
		log.FatalExit("Cannot create temp directory: " + err.Error())
	}
	return dir
}

// cloneDirs holds the absolute local paths where the shop template and every
// scaffolded repo will be cloned. Pre-computed up-front (before Phase 1) so the
// startup banner can display them deterministically. Fields not applicable to
// the current arch+strategy combination are left empty.
type cloneDirs struct {
	Shop     string
	Repo     string
	Frontend string
	Backend  string
	System   string
}

func resolveCloneDirs(workDir, strategy, arch string) cloneDirs {
	d := cloneDirs{
		Shop: filepath.Join(workDir, "shop"),
		Repo: filepath.Join(workDir, "repo"),
	}
	if strategy == "multirepo" {
		if arch == "multitier" {
			d.Frontend = filepath.Join(workDir, "repo-frontend")
			d.Backend = filepath.Join(workDir, "repo-backend")
		} else {
			d.System = filepath.Join(workDir, "repo-system")
		}
	}
	return d
}

func computeOwnerPascal(owner string) string {
	p := ToPascalCase(owner)
	if !strings.Contains(owner, "-") {
		return strings.ToUpper(owner[:1]) + owner[1:]
	}
	return p
}

// ghAPISilent is the flag used with `gh api` to suppress the response body on
// success. 4xx/5xx still produce non-zero exit codes, which is what we key on.
const ghAPISilent = "--silent"

// CheckOwnerExistsFn and CheckProjectExistsFn are the network-validation
// seams shared by ValidateAndDeriveForYAML (flag-driven `gh optivem config
// init` + `gh optivem init`) and configinit.Prompt (interactive recovery
// + interactive `config init`). Tests override these to keep the unit
// surface offline; production wires both to the real `gh` subshell
// probes.
//
// Exported (vs. the unexported steps/project.go test seams) because
// configinit is a sibling package — see interactive-validation-parity
// feedback: prompts and flags must call the same validators.
var (
	CheckOwnerExistsFn   = realCheckOwnerExists
	CheckProjectExistsFn = realCheckProjectExists
)

// CheckOwnerExists verifies the owner resolves as a GitHub user or
// organization. Returns nil on success, an error describing the miss
// otherwise. Empty owner returns nil (callers run format validation
// first; empty is a different failure mode).
func CheckOwnerExists(owner string) error {
	if owner == "" {
		return nil
	}
	return CheckOwnerExistsFn(owner)
}

// CheckProjectExists verifies the project URL resolves to a real GitHub
// Project (v2) the operator can read. Returns nil on success or when url
// is empty (empty is intentionally allowed — `gh optivem init` Path A
// auto-creates a board on first run; see ValidateAndDeriveForYAML).
// Returns an error describing the miss otherwise.
func CheckProjectExists(url string) error {
	if url == "" {
		return nil
	}
	return CheckProjectExistsFn(url)
}

// realCheckOwnerExists is the production CheckOwnerExistsFn. Hits
// `gh api users/<owner>` first (the more common case), falls back to
// `gh api orgs/<owner>` on 404. Stderr is suppressed so the first 404
// doesn't leak when we fall back.
func realCheckOwnerExists(owner string) error {
	userCmd := exec.Command("gh", "api", "users/"+owner, ghAPISilent)
	userCmd.Stderr = nil
	if err := userCmd.Run(); err == nil {
		return nil
	}
	orgCmd := exec.Command("gh", "api", "orgs/"+owner, ghAPISilent)
	orgCmd.Stderr = nil
	if err := orgCmd.Run(); err == nil {
		return nil
	}
	return fmt.Errorf("no GitHub user or organization named %q", owner)
}

// projectExistsQuery is the minimal GraphQL query for project existence.
// Replaces `gh project view`, whose internal query expands every projectV2
// field-value-type variant and has been hitting upstream resolver bugs on
// the projectV2 path. We only need to know that the project resolves — we
// ask for the smallest possible scalar (id). %s dispatches to "organization"
// or "user" based on parseProjectURL's ownerKind: querying both branches
// in one request produces a partial NOT_FOUND for the wrong type that gh
// treats as fatal.
//
// Duplicated from internal/atdd/runtime/tracker/github's projectMetaQuery
// rather than imported, for the same reason parseProjectURL is duplicated below.
const projectExistsQuery = `query($login:String!,$number:Int!){%s(login:$login){projectV2(number:$number){id}}}`

// realCheckProjectExists is the production CheckProjectExistsFn. Parses
// the URL into (ownerKind, owner, number) and probes the project via a
// minimal GraphQL query. A non-zero exit covers both "doesn't exist" and
// "exists but caller can't read it" — same operator-visible failure
// either way (`gh optivem init` will fail at the same step).
func realCheckProjectExists(url string) error {
	ownerKind, owner, number, err := parseProjectURL(url)
	if err != nil {
		return fmt.Errorf("project URL %q: %w", url, err)
	}
	query := fmt.Sprintf(projectExistsQuery, ownerKind)
	cmd := exec.Command("gh", "api", "graphql",
		"-F", "login="+owner,
		"-F", "number="+strconv.Itoa(number),
		"-f", "query="+query)
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("project %s/%d not found or not accessible", owner, number)
	}
	return nil
}

// parseProjectURL splits a GitHub Project (v2) URL into ownerKind
// ("organization" | "user"), owner login, and project number. ownerKind
// is used by realCheckProjectExists to issue a targeted GraphQL query
// (querying both branches in a single request produces a partial
// NOT_FOUND for the wrong type, which gh treats as fatal).
//
// Duplicated from internal/steps/project.go and internal/atdd/runtime/tracker/github
// rather than imported to keep internal/config dependency-free of the
// runtime-side packages it underpins.
func parseProjectURL(s string) (ownerKind, owner string, number int, err error) {
	u, perr := neturl.Parse(s)
	if perr != nil {
		return "", "", 0, fmt.Errorf("malformed URL: %w", perr)
	}
	if u.Host != "github.com" {
		return "", "", 0, fmt.Errorf("expected host github.com, got %q", u.Host)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 4 || (parts[0] != "users" && parts[0] != "orgs") || parts[2] != "projects" {
		return "", "", 0, fmt.Errorf("expected URL of form https://github.com/{users|orgs}/<owner>/projects/<n>")
	}
	n, cerr := strconv.Atoi(parts[3])
	if cerr != nil || n <= 0 {
		return "", "", 0, fmt.Errorf("project number must be a positive integer: %s", parts[3])
	}
	kind := "organization"
	if parts[0] == "users" {
		kind = "user"
	}
	return kind, parts[1], n, nil
}

// confirmReposExist probes every repo in fullRepos ("<owner>/<name>") and, if
// any already exist on GitHub, asks the user once whether to proceed scaffolding
// into them. Aborts via FatalExit if the user declines or stdin is not available.
// When assumeYes is true (--yes/-y), the prompt is skipped and the run proceeds
// after a warning — the expected pattern for CI/unattended use.
//
// Batched (vs one prompt per repo) so multirepo runs with all three repos
// already present don't ask the user the same question three times in a row.
//
// Returns the subset of fullRepos that already existed, so callers can record
// which repos were pre-existing — finalize uses this to allow a clean working
// tree at commit time (legitimate "re-scaffold same content" case) only for
// pre-existing repos, while still treating a clean tree on a freshly created
// repo as a hard error (something went wrong with the template apply).
func confirmReposExist(fullRepos []string, assumeYes bool, resolved approval.Resolved) []string {
	var existing []string
	for _, fullRepo := range fullRepos {
		if fullRepo == "" {
			continue
		}
		cmd := exec.Command("gh", "api", "repos/"+fullRepo, ghAPISilent)
		cmd.Stderr = nil // 404 is the expected case — suppress the noise
		if err := cmd.Run(); err == nil {
			existing = append(existing, fullRepo)
		}
		// On error, repo doesn't exist (or API is unreachable). Continue —
		// if it's really unreachable, later steps will fail with a clearer error.
	}

	if len(existing) == 0 {
		return nil
	}

	if len(existing) == 1 {
		log.Warnf("Repository %s already exists on GitHub.", existing[0])
		log.Warnf("Proceeding will scaffold into the existing repository and may overwrite its contents.")
	} else {
		log.Warnf("The following repositories already exist on GitHub:")
		for _, r := range existing {
			log.Warnf("  - %s", r)
		}
		log.Warnf("Proceeding will scaffold into the existing repositories and may overwrite their contents.")
	}

	if assumeYes {
		log.Infof("Proceeding without confirmation (--yes).")
		return existing
	}

	// Overwriting an existing GitHub repo is destructive — gate at the
	// always-prompt human tier so the operator must either pass --yes or
	// answer the prompt.
	// TODO(non-implement-tiering): placeholder; proper tier assignment
	// deferred to the follow-up plan. See plan
	// 20260528-0930-approval-tier-ladder.md §D5.
	ok, err := approval.Confirm(resolved, approval.CategoryHuman, os.Stdin, os.Stderr, "Proceed?")
	if err != nil {
		log.FatalExit(fmt.Sprintf("Aborted: %d repositor%s already exist and no confirmation was provided (pass --yes to proceed unattended)",
			len(existing), pluralY(len(existing))))
	}
	if !ok {
		log.FatalExit(fmt.Sprintf("Aborted: %d repositor%s already exist", len(existing), pluralY(len(existing))))
	}
	return existing
}

// pluralY returns the suffix for "repositor{y,ies}" given a count.
func pluralY(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

// resolveLogFilePath returns the log-file destination for this run. Always
// writes a log to a predictable temp path when --log-file isn't given, so the
// on-failure bug-report prompt can always offer to attach a log tail without
// requiring upfront opt-in via --report-bug.
func resolveLogFilePath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	return filepath.Join(os.TempDir(),
		fmt.Sprintf("gh-optivem-%s.log", time.Now().UTC().Format("20060102-150405")))
}

// ParseAndValidate validates the populated RawFlags (Cobra parses on
// Execute()), runs the network/format validation phases, and returns the
// resolved Config. The cmd is used to detect which flags the user passed
// explicitly (via cmd.Flags().Visit) so the banner can tag defaults.
func ParseAndValidate(cmd *cobra.Command, f *RawFlags) *Config {
	userSet := make(map[string]bool)
	cmd.Flags().Visit(func(fl *pflag.Flag) { userSet[fl.Name] = true })
	resolved := cmdctx.Approval(cmd)

	if f.Owner == "" || f.SystemName == "" || f.Repo == "" || f.Arch == "" || f.RepoStrategy == "" {
		log.FatalExit("Required flags: --owner, --system-name, --repo, --arch, --repo-strategy")
	}

	if f.Verbose && f.Quiet {
		log.FatalExit("--verbose and --quiet are mutually exclusive")
	}

	// === Phase 1: in-memory format validation (fast, no network) ===
	if err := ValidateOwnerFormat(f.Owner); err != "" {
		log.FatalExit("--owner: " + err)
	}
	if err := ValidateRepoFormat(f.Repo); err != "" {
		log.FatalExit("--repo: " + err)
	}
	if err := ValidateSystemName(f.SystemName); err != "" {
		log.FatalExit("--system-name: " + err)
	}

	resolvedLevel := resolveVerifyLevel(f.VerifyLevel)
	validateCommonFlags(f.Deploy, f.Arch, f.RepoStrategy)
	lc := resolveLangs(f)

	repoName := f.Repo

	env := readEnvTokens()

	mr := deriveMultirepoNames(f.RepoStrategy, f.Arch, f.Owner, repoName)

	// === Phase 2: token presence + provider auth (fail fast before any mutation) ===
	// Same helper as `gh optivem environment verify`, so init shares one
	// definition of "valid environment". Aborts before we touch gh / GitHub /
	// SonarCloud for existence checks since this is the most common failure
	// mode.
	if err := VerifyEnvironment(collectLangs(lc), f.Deploy); err != nil {
		log.FatalExit(err.Error())
	}
	if err := CheckOwnerExists(f.Owner); err != nil {
		log.FatalExit("--owner: " + err.Error())
	}
	if err := CheckProjectExists(f.ProjectURL); err != nil {
		log.FatalExit("--project-url: " + err.Error())
	}
	preExisting := confirmReposExist([]string{
		f.Owner + "/" + repoName,
		mr.backendFullRepo,
		mr.frontendFullRepo,
		mr.systemFullRepo,
	}, f.AssumeYes, resolved)
	preExistingSet := make(map[string]bool, len(preExisting))
	for _, r := range preExisting {
		preExistingSet[r] = true
	}

	// === Phase 3: resolve shop ref (fast API call; actual clone happens in the Prepare step) ===
	resolvedShopRef := resolveShopRef(f.ShopRef)

	ownerPascal := computeOwnerPascal(f.Owner)
	ownerLower := strings.ToLower(f.Owner)
	repoPascal := ToPascalCase(repoName)
	repoNoHyphens := ToJavaLower(repoName)
	wd := resolveWorkDir(f.WorkDir)
	clones := resolveCloneDirs(wd, f.RepoStrategy, f.Arch)

	logFilePath := resolveLogFilePath(f.LogFile)

	return &Config{
		Owner:        f.Owner,
		Repo:         repoName,
		FullRepo:     f.Owner + "/" + repoName,
		SystemName:   f.SystemName,
		Arch:         f.Arch,
		RepoStrategy: f.RepoStrategy,

		Raw: RawInputs{
			Repo:        f.Repo,
			ShopRef:     f.ShopRef,
			TestLang:    f.TestLang,
			VerifyLevel: f.VerifyLevel,
			WorkDir:     f.WorkDir,
			KeepLocal:   f.KeepLocal,
		},
		UserSetFlags: userSet,

		Lang:         lc.lang,
		BackendLang:  lc.backendLang,
		FrontendLang: lc.frontendLang,
		TestLang:     lc.testLang,

		Deploy:         f.Deploy,
		License:        f.License,
		ProjectURL:     f.ProjectURL,
		SystemPath:     f.SystemPath,
		SystemTestPath: f.SystemTestPath,
		BackendPath:    f.BackendPath,
		FrontendPath:   f.FrontendPath,
		VerifyLevel:    resolvedLevel,
		NoLegacy:       f.NoLegacy,
		NoLocalTests:   f.NoLocalTests,
		NoLocalSonar:   f.NoLocalSonar,
		NoAtdd:         f.NoAtdd,
		NoProject:      f.NoProject,
		KeepLocal:      f.KeepLocal,
		BugReport:      f.BugReport,
		Verbose:        f.Verbose,
		Quiet:          f.Quiet,
		LogFile:        logFilePath,
		AssumeYes:      f.AssumeYes,
		Approval:       resolved,
		WorkDir:        wd,
		// ShopPath, RepoDir, and the multirepo-component dirs are pre-computed
		// from WorkDir so the startup banner can show them before Phase 1. The
		// Prepare and Apply Template phases clone into these paths directly.
		ShopPath:        clones.Shop,
		RepoDir:         clones.Repo,
		FrontendRepoDir: clones.Frontend,
		BackendRepoDir:  clones.Backend,
		SystemRepoDir:   clones.System,
		ShopRef:         resolvedShopRef,

		DockerHubUsername: env.dockerHubUsername,
		DockerHubToken:    env.dockerHubToken,
		SonarToken:        env.sonarToken,
		GHCRToken:         env.ghcrToken,
		WorkflowToken:     env.workflowToken,
		RepoToken:         env.repoToken,

		OwnerPascal:   ownerPascal,
		OwnerLower:    ownerLower,
		RepoPascal:    repoPascal,
		RepoNoHyphens: repoNoHyphens,

		OwnerCasings:   OwnerCasings(f.Owner),
		SysNameCasings: SystemCasings(f.SystemName),

		JavaNsOld:   "com.optivem.shop",
		JavaNsNew:   "com." + ownerLower + "." + repoNoHyphens,
		DotnetNsOld: "Optivem.Shop",
		DotnetNsNew: ownerPascal + "." + repoPascal,
		TsPkgOld:    "@optivem/shop-system-test",
		TsPkgNew:    "@" + ownerLower + "/" + repoName + "-system-test",

		SysNamePascalOld: "Shop",
		SysNamePascalNew: SpacesToPascal(f.SystemName),
		SysNameCamelOld:  "shop",
		SysNameCamelNew:  SpacesToCamel(f.SystemName),
		SysNameKebabOld:  "shop",
		SysNameKebabNew:  SpacesToKebab(f.SystemName),
		SysNameLowerOld:  "shop",
		SysNameLowerNew:  SpacesToLower(f.SystemName),

		FrontendRepo:     mr.frontendRepo,
		BackendRepo:      mr.backendRepo,
		FrontendFullRepo: mr.frontendFullRepo,
		BackendFullRepo:  mr.backendFullRepo,

		SystemRepo:     mr.systemRepo,
		SystemFullRepo: mr.systemFullRepo,

		PreExistingRepos: preExistingSet,
	}
}

// CloneShop clones optivem/shop from GitHub into dir, then checks out ref.
// dir is pre-computed during ParseAndValidate (so the startup banner can show
// it); this function creates it on disk and populates it. ref must be non-empty
// (SHA or tag) — HEAD of main is never cloned as a final state. Called by the
// Prepare-phase step (not during ParseAndValidate) so the clone appears as a
// normal phased step in the output.
func CloneShop(ref, dir string) error {
	if ref == "" {
		return fmt.Errorf("ref must be non-empty — refusing to clone HEAD of main")
	}
	if dir == "" {
		return fmt.Errorf("dir must be non-empty")
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0755); err != nil {
		return fmt.Errorf("cannot create parent dir: %w", err)
	}
	// Clear any leftover shop clone from a previous run against the same
	// --workdir. `gh repo clone` refuses to clone into an existing dir.
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("cannot clear existing shop dir %s: %w", dir, err)
	}

	cmd := exec.Command("gh", "repo", "clone", "optivem/shop", dir)
	if out, cerr := cmd.CombinedOutput(); cerr != nil {
		os.RemoveAll(dir)
		return fmt.Errorf("gh repo clone failed: %s\n%s", cerr, string(out))
	}

	checkout := exec.Command("git", "-C", dir, "checkout", ref)
	if cout, cerr := checkout.CombinedOutput(); cerr != nil {
		os.RemoveAll(dir)
		return fmt.Errorf("git checkout %s failed: %s\n%s", ref, cerr, string(cout))
	}
	log.Successf("Cloned shop to %s (pinned to %s)", dir, ref)
	return nil
}

// latestMetaRelease returns the tag of the most recently created meta-v* release in optivem/shop.
// Mirrors the resolution logic in .github/workflows/gh-acceptance-stage.yml.
func latestMetaRelease() (string, error) {
	cmd := exec.Command("gh", "api", "repos/optivem/shop/releases?per_page=100",
		"--jq", `[.[] | select(.tag_name | startswith("meta-v"))] | sort_by(.created_at) | last | .tag_name`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh api failed: %s\n%s", err, string(out))
	}
	tag := strings.TrimSpace(string(out))
	if tag == "" || tag == "null" {
		return "", fmt.Errorf("no meta-v* release found in optivem/shop — run shop's meta-prerelease-stage (and publish its tag as a release) first, or pass an explicit --shop-ref")
	}
	return tag, nil
}

// LicenseName returns the human-readable license name. Thin shim over
// projectconfig.LicenseName — the canonical key→name map lives with the
// YAML schema so `gh-optivem.yaml`'s license field and the `--license`
// flag stay in lockstep.
func (c *Config) LicenseName() string {
	return projectconfig.LicenseName(c.License)
}

// EffectiveLang returns the primary system language (lang for monolith, backend-lang for multitier).
func (c *Config) EffectiveLang() string {
	if c.Arch == "monolith" {
		return c.Lang
	}
	return c.BackendLang
}

// ValidateAndDeriveForYAML validates the YAML-affecting flag subset (the flags
// BindConfigInitFlags binds) and returns a *Config populated with just the
// fields steps.WriteOptivemYAMLToPath reads. Used by `gh optivem config init`
// to render gh-optivem.yaml without doing the full ParseAndValidate — no
// GitHub network checks, no env tokens, no workdir/clone-dir derivation.
//
// Returns an error rather than calling log.FatalExit so callers can drive
// behaviour from tests and so the pure local-file-write CLI surface fails
// with a regular error instead of a hard exit.
func ValidateAndDeriveForYAML(f *RawFlags) (*Config, error) {
	if f.Owner == "" || f.Repo == "" || f.SystemName == "" || f.Arch == "" || f.RepoStrategy == "" {
		return nil, fmt.Errorf("required flags: --owner, --repo, --system-name, --arch, --repo-strategy")
	}
	if msg := ValidateOwnerFormat(f.Owner); msg != "" {
		return nil, fmt.Errorf("--owner: %s", msg)
	}
	if msg := ValidateRepoFormat(f.Repo); msg != "" {
		return nil, fmt.Errorf("--repo: %s", msg)
	}
	if msg := ValidateSystemName(f.SystemName); msg != "" {
		return nil, fmt.Errorf("--system-name: %s", msg)
	}
	if msg := ValidateArch(f.Arch); msg != "" {
		return nil, fmt.Errorf("--arch %s", msg)
	}
	if msg := ValidateRepoStrategy(f.RepoStrategy); msg != "" {
		return nil, fmt.Errorf("--repo-strategy %s", msg)
	}
	// License/Deploy default to the same values bindYAMLAffectingFlags
	// uses when the operator doesn't pass the flag. Defaulting here too
	// means callers who build RawFlags by hand (tests, library users)
	// see identical behaviour to flag-driven callers.
	if f.License == "" {
		f.License = projectconfig.LicenseMIT
	}
	if !projectconfig.IsValidLicense(f.License) {
		return nil, fmt.Errorf("--license %q must be one of mit, apache-2.0, gpl-3.0, bsd-2-clause, bsd-3-clause, unlicense", f.License)
	}
	if f.Deploy == "" {
		f.Deploy = projectconfig.DeployDocker
	}
	if !projectconfig.IsValidDeploy(f.Deploy) {
		return nil, fmt.Errorf("--deploy %q must be 'docker' or 'cloud-run'", f.Deploy)
	}
	if msg := ValidateProjectURLFormat(f.ProjectURL); msg != "" {
		return nil, fmt.Errorf("--project-url %s", msg)
	}
	lc, err := resolveLangsForYAML(f)
	if err != nil {
		return nil, err
	}
	if err := resolvePathFlagsForYAML(f); err != nil {
		return nil, err
	}
	// --project-url may be omitted: the resulting gh-optivem.yaml has
	// project.url empty, and `gh optivem init` Path A (EnsureProjectBoard
	// → findOrCreateProject in internal/steps/project.go) auto-creates a
	// GitHub Project named SystemName under Owner on first run, then
	// rewrites the yaml with the resulting URL. Pass --project-url
	// explicitly to bind the scaffold to an existing board (Path B,
	// status-set verification instead of creation).
	//
	// Existence checks: owner must resolve as a GitHub user/org; project
	// URL (when supplied) must resolve to a real Project (v2) the operator
	// can read. Both go through the package-level CheckOwnerExistsFn /
	// CheckProjectExistsFn seams so test surfaces stay offline.
	if err := CheckOwnerExists(f.Owner); err != nil {
		return nil, fmt.Errorf("--owner: %s", err.Error())
	}
	if err := CheckProjectExists(f.ProjectURL); err != nil {
		return nil, fmt.Errorf("--project-url: %s", err.Error())
	}
	mr := deriveMultirepoNames(f.RepoStrategy, f.Arch, f.Owner, f.Repo)
	return &Config{
		Owner:            f.Owner,
		Repo:             f.Repo,
		FullRepo:         f.Owner + "/" + f.Repo,
		SystemName:       f.SystemName,
		Arch:             f.Arch,
		RepoStrategy:     f.RepoStrategy,
		Lang:             lc.lang,
		BackendLang:      lc.backendLang,
		FrontendLang:     lc.frontendLang,
		TestLang:         lc.testLang,
		License:          f.License,
		Deploy:           f.Deploy,
		ProjectURL:       f.ProjectURL,
		SystemPath:       f.SystemPath,
		SystemTestPath:   f.SystemTestPath,
		BackendPath:      f.BackendPath,
		FrontendPath:     f.FrontendPath,
		FrontendRepo:     mr.frontendRepo,
		BackendRepo:      mr.backendRepo,
		FrontendFullRepo: mr.frontendFullRepo,
		BackendFullRepo:  mr.backendFullRepo,
		SystemRepo:       mr.systemRepo,
		SystemFullRepo:   mr.systemFullRepo,
	}, nil
}

// resolvePathFlagsForYAML fills any empty tier-path flag with the flat
// scaffold layout — the same defaults `gh optivem init` itself produces
// — so an operator running `gh optivem config init` for a freshly
// scaffolded project doesn't have to retype values that match the
// scaffolder's own output. Override individual paths only when the YAML
// is being written for a non-flat existing repo.
//
// Mismatched flags (e.g. --system-path on multitier) are still rejected
// so a typo doesn't silently land in the YAML.
func resolvePathFlagsForYAML(f *RawFlags) error {
	reject := func(name, val string) error {
		if val != "" {
			return fmt.Errorf("--%s is not valid for --arch %s", name, f.Arch)
		}
		return nil
	}
	if f.SystemTestPath == "" {
		f.SystemTestPath = DefaultSystemTestPath
	}
	switch f.Arch {
	case "monolith":
		if f.SystemPath == "" {
			f.SystemPath = DefaultSystemPath
		}
		if err := reject("backend-path", f.BackendPath); err != nil {
			return err
		}
		if err := reject("frontend-path", f.FrontendPath); err != nil {
			return err
		}
	case "multitier":
		if f.BackendPath == "" {
			f.BackendPath = DefaultBackendPath
		}
		if f.FrontendPath == "" {
			f.FrontendPath = DefaultFrontendPath
		}
		if err := reject("system-path", f.SystemPath); err != nil {
			return err
		}
	}
	return nil
}

// resolveLangsForYAML mirrors resolveLangs but returns errors instead of
// calling log.FatalExit. The two paths can't share a body without rewiring
// resolveLangs's call site (ParseAndValidate) to handle errors, which is out
// of scope for this change.
func resolveLangsForYAML(f *RawFlags) (langChoice, error) {
	validLangs := map[string]bool{"java": true, "dotnet": true, "typescript": true}
	var c langChoice
	if f.Arch == "monolith" {
		if f.Lang == "" {
			return c, fmt.Errorf("--monolith-lang is required for monolith architecture")
		}
		if !validLangs[f.Lang] {
			return c, fmt.Errorf("--monolith-lang must be java, dotnet, or typescript")
		}
		c.lang = f.Lang
		if f.TestLang == "" {
			return c, fmt.Errorf("--test-lang is required (the system-test tier's language is not derived from --monolith-lang)")
		}
		if !validLangs[f.TestLang] {
			return c, fmt.Errorf("--test-lang must be java, dotnet, or typescript")
		}
		c.testLang = f.TestLang
		return c, nil
	}
	if f.BackendLang == "" {
		return c, fmt.Errorf("--backend-lang is required for multitier architecture")
	}
	if f.FrontendLang == "" {
		return c, fmt.Errorf("--frontend-lang is required for multitier architecture")
	}
	if !validLangs[f.BackendLang] {
		return c, fmt.Errorf("--backend-lang must be java, dotnet, or typescript")
	}
	if f.FrontendLang != "typescript" {
		return c, fmt.Errorf("--frontend-lang must be typescript")
	}
	c.backendLang = f.BackendLang
	c.frontendLang = f.FrontendLang
	if f.TestLang == "" {
		return c, fmt.Errorf("--test-lang is required (the system-test tier's language is not derived from --backend-lang)")
	}
	if !validLangs[f.TestLang] {
		return c, fmt.Errorf("--test-lang must be java, dotnet, or typescript")
	}
	c.testLang = f.TestLang
	return c, nil
}
