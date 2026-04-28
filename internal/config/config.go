// Package config provides CLI parsing, validation, and the Config struct.
package config

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/optivem/gh-optivem/internal/log"
	"github.com/optivem/gh-optivem/internal/version"
)

// RawInputs captures the raw CLI flag values before any resolution/defaulting.
// Surfaced in the startup banner so the user can audit exactly what they typed
// (vs. the resolved/derived values the scaffold will actually act on).
type RawInputs struct {
	Repo            string // --repo as-passed
	ShopRef         string // --shop-ref before resolving empty → baked-in SHA → latest meta-v*
	TestLang        string // --test-lang before defaulting to --monolith-lang / --backend-lang
	VerifyLevel     string // --verify-level as-passed (flag default: "release")
	WorkDir         string // --workdir before defaulting to temp dir
	KeepLocal       bool   // --keep-local flag as-passed
	RandomSuffix    bool   // --random-suffix flag as-passed
}

type Config struct {
	Owner      string
	Repo       string
	FullRepo   string
	SystemName string
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

	Deploy     string // "docker" (default) or "cloud-run"
	License    string
	DryRun       bool
	VerifyLevel    string // "none", "local", "commit", "acceptance", "qa", "release"
	NoLegacy       bool   // exclude legacy from local tests and acceptance stage
	NoLocalTests   bool   // skip the "Verify local testing" step (runner package over system-test/)
	NoLocalSonar   bool   // skip the "Verify local SonarCloud scan" step (per-component Run-Sonar.ps1)
	NoAtdd         bool   // skip the "Install ATDD assets" step (skip copying agents/commands/prompts from shop)
	KeepLocal    bool   // keep the local scaffolded clone dir after a successful run (default: delete it)
	BugReport    bool   // opt in to auto-creating a GitHub issue on failure (default: off)
	Verbose      bool   // enable debug output
	Quiet        bool   // suppress info-level output
	LogFile      string // optional path to mirror plain-text log output
	AssumeYes    bool   // skip all interactive confirmations (existing-repo, --report-bug)
	WorkDir    string
	ShopPath string
	ShopRef  string // Pinned optivem/shop ref (SHA, tag, or branch). Never empty.

	DockerHubUsername string
	DockerHubToken   string
	SonarToken       string
	GHCRToken        string
	WorkflowToken    string
	RepoToken        string // Classic PAT with `repo` scope for cross-repo Contents:read in multirepo prod-stage

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
func ValidateSystemName(name string) string {
	if msg := checkNameTrim(name); msg != "" {
		return msg
	}
	words := strings.Fields(name)
	if msg := checkWordChars(words); msg != "" {
		return msg
	}
	if msg := checkReservedWords(words); msg != "" {
		return msg
	}
	if msg := checkReservedDerived(name); msg != "" {
		return msg
	}
	if len(name) > 50 {
		return "system name exceeds 50 character limit"
	}
	return ""
}

func checkNameTrim(name string) string {
	if len(strings.TrimSpace(name)) == 0 {
		return "system name cannot be empty"
	}
	if name != strings.TrimSpace(name) {
		return "system name cannot have leading or trailing spaces"
	}
	return ""
}

func checkWordChars(words []string) string {
	for _, w := range words {
		for _, c := range w {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
				return fmt.Sprintf("system name contains invalid character '%c' — only letters and spaces allowed", c)
			}
		}
	}
	return ""
}

func checkReservedWords(words []string) string {
	for _, w := range words {
		lower := strings.ToLower(w)
		if isLanguageReserved(lower) {
			return fmt.Sprintf("word %q is a language reserved keyword", w)
		}
		if isScaffoldReserved(lower) {
			return fmt.Sprintf("word %q is a scaffold reserved word (collides with infrastructure names)", w)
		}
	}
	return ""
}

func checkReservedDerived(name string) string {
	camel := SpacesToCamel(name)
	lower := SpacesToLower(name)
	if isLanguageReserved(lower) {
		return fmt.Sprintf("derived form %q is a language reserved keyword", lower)
	}
	if isLanguageReserved(camel) {
		return fmt.Sprintf("derived form %q is a language reserved keyword", camel)
	}
	return ""
}

func isLanguageReserved(word string) bool {
	reserved := map[string]bool{
		// Java
		"abstract": true, "assert": true, "boolean": true, "break": true, "byte": true,
		"case": true, "catch": true, "char": true, "class": true, "const": true,
		"continue": true, "default": true, "do": true, "double": true, "else": true,
		"enum": true, "extends": true, "final": true, "finally": true, "float": true,
		"for": true, "goto": true, "if": true, "implements": true, "import": true,
		"instanceof": true, "int": true, "interface": true, "long": true, "native": true,
		"new": true, "null": true, "package": true, "private": true, "protected": true,
		"public": true, "return": true, "short": true, "static": true, "strictfp": true,
		"super": true, "switch": true, "synchronized": true, "this": true, "throw": true,
		"throws": true, "transient": true, "try": true, "void": true, "volatile": true,
		"while": true,
		// C# additional
		"as": true, "base": true, "bool": true, "checked": true, "decimal": true,
		"delegate": true, "event": true, "explicit": true, "extern": true, "fixed": true,
		"foreach": true, "implicit": true, "in": true, "is": true, "lock": true,
		"namespace": true, "object": true, "operator": true, "out": true, "override": true,
		"params": true, "readonly": true, "ref": true, "sbyte": true, "sealed": true,
		"sizeof": true, "stackalloc": true, "string": true, "struct": true, "typeof": true,
		"uint": true, "ulong": true, "unchecked": true, "unsafe": true, "ushort": true,
		"using": true, "virtual": true, "where": true, "yield": true,
		// TypeScript additional
		"any": true, "async": true, "await": true, "constructor": true, "declare": true,
		"from": true, "get": true, "let": true, "module": true, "of": true,
		"require": true, "set": true, "symbol": true, "type": true, "var": true,
	}
	return reserved[word]
}

func isScaffoldReserved(word string) bool {
	reserved := map[string]bool{
		"system": true, "backend": true, "frontend": true, "test": true, "api": true,
		"external": true, "stub": true, "real": true, "monolith": true, "multitier": true,
		"health": true, "postgres": true, "docker": true, "compose": true, "pipeline": true,
		"local": true, "stage": true, "commit": true, "acceptance": true, "production": true,
		"workflow": true, "action": true, "build": true, "deploy": true, "version": true,
		"config": true, "app": true, "network": true, "service": true, "port": true,
		"image": true, "container": true, "volume": true, "env": true, "run": true,
		"src": true, "main": true, "lib": true, "bin": true, "dist": true,
		"node": true, "gradle": true, "dotnet": true, "java": true, "typescript": true,
		"react": true, "spring": true, "next": true, "shop": true,
	}
	return reserved[word]
}

// RawFlags holds the unparsed CLI flag values for `init`. Flags bind directly
// to these fields on the Cobra command via BindInitFlags; ParseAndValidate then
// consumes the populated struct after Cobra has parsed the command line.
type RawFlags struct {
	Owner             string
	SystemName        string
	Repo              string
	Arch              string
	RepoStrategy      string
	Lang              string
	TestLang          string
	BackendLang       string
	FrontendLang      string
	License           string
	VerifyLevel       string
	Deploy            string
	WorkDir           string
	ShopRef           string
	LogFile           string
	DryRun            bool
	KeepLocal         bool
	NoLegacy          bool
	NoLocalTests      bool
	NoLocalSonar      bool
	NoAtdd            bool
	BugReport         bool
	Verbose           bool
	Quiet             bool
	AssumeYes         bool
}

// BindInitFlags binds every `gh optivem init` CLI flag to the corresponding
// field on f. Cobra parses on Execute(); ParseAndValidate then validates the
// populated struct.
func BindInitFlags(cmd *cobra.Command, f *RawFlags) {
	fs := cmd.Flags()
	fs.StringVar(&f.Owner, "owner", "", "GitHub username or org (required)")
	fs.StringVar(&f.SystemName, "system-name", "", `System name, e.g. "Page Turner" (required)`)
	fs.StringVar(&f.Repo, "repo", "", "Repository name, e.g. page-turner (required, or pass positionally)")
	fs.StringVar(&f.Arch, "arch", "", "Architecture: monolith or multitier (required)")
	fs.StringVar(&f.RepoStrategy, "repo-strategy", "", "Repo strategy: monorepo or multirepo (required)")
	fs.StringVar(&f.Lang, "monolith-lang", "", "System language: java, dotnet, typescript (monolith)")
	fs.StringVar(&f.TestLang, "test-lang", "", "Test language (defaults to --monolith-lang or --backend-lang)")
	fs.StringVar(&f.BackendLang, "backend-lang", "", "Backend language: java, dotnet, typescript (multitier)")
	fs.StringVar(&f.FrontendLang, "frontend-lang", "", "Frontend language: react (multitier)")
	fs.StringVar(&f.License, "license", "mit", "License: mit, apache-2.0, gpl-3.0, bsd-2-clause, bsd-3-clause, unlicense")
	fs.BoolVar(&f.DryRun, "dry-run", false, "Print actions without executing")
	fs.BoolVar(&f.KeepLocal, "keep-local", false, "Keep the local scaffolded clone dir instead of deleting it on success")
	fs.StringVar(&f.VerifyLevel, "verify-level", "release", "Verification level: none, local, commit, acceptance, qa, release")
	fs.BoolVar(&f.NoLegacy, "no-legacy", false, "Exclude legacy from local tests and acceptance stage")
	fs.BoolVar(&f.NoLocalTests, "no-local-tests", false, "Skip the 'Verify local testing' step (runner package over system-test/)")
	fs.BoolVar(&f.NoLocalSonar, "no-local-sonar", false, "Skip the 'Verify local SonarCloud scan' step (per-component Run-Sonar.ps1 against the SonarCloud project)")
	fs.BoolVar(&f.NoAtdd, "no-atdd", false, "Skip installing ATDD agents/commands/prompts from shop into the scaffolded repo")
	fs.BoolVar(&f.BugReport, "report-bug", false, "On failure, auto-create a GitHub issue in optivem/gh-optivem with scaffold config. Off by default — file one yourself if the failure is worth reporting.")
	fs.StringVar(&f.Deploy, "deploy", "docker", "Deployment target: docker or cloud-run")
	fs.StringVar(&f.WorkDir, "workdir", "", "Working directory for cloning (default: temp dir)")
	fs.StringVar(&f.ShopRef, "shop-ref", "", "Pin optivem/shop to this ref (tag, SHA, or branch — e.g. meta-v1.2.3, main, a1b2c3d). Overrides build-time pin. Default: latest meta-v* release.")
	fs.BoolVarP(&f.Verbose, "verbose", "v", false, "Enable debug output (retry/wait chatter, diagnostics)")
	fs.BoolVarP(&f.Quiet, "quiet", "q", false, "Suppress info-level output (warnings and errors still shown)")
	fs.StringVar(&f.LogFile, "log-file", "", "Also write plain-text log output to this file (no ANSI colors, all levels)")
	fs.BoolVarP(&f.AssumeYes, "yes", "y", false, "Skip all interactive confirmations (existing-repo prompt, --report-bug confirmation). Expected pattern for CI/unattended runs.")
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
	if arch != "monolith" && arch != "multitier" {
		log.FatalExit("--arch must be 'monolith' or 'multitier'")
	}
	if repoStrategy != "monorepo" && repoStrategy != "multirepo" {
		log.FatalExit("--repo-strategy must be 'monorepo' or 'multirepo'")
	}
}

type langChoice struct {
	lang, backendLang, frontendLang, testLang string
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
		c.testLang = f.TestLang
		if c.testLang == "" {
			c.testLang = c.lang
		}
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
	if f.FrontendLang != "react" {
		log.FatalExit("--frontend-lang must be react")
	}
	c.backendLang = f.BackendLang
	c.frontendLang = f.FrontendLang
	c.testLang = f.TestLang
	if c.testLang == "" {
		c.testLang = c.backendLang
	}
	return c
}

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

func validateEnvTokens(e envTokens) {
	required := []struct{ name, val string }{
		{"DOCKERHUB_USERNAME", e.dockerHubUsername},
		{"DOCKERHUB_TOKEN", e.dockerHubToken},
		{"SONAR_TOKEN", e.sonarToken},
		{"WORKFLOW_TOKEN", e.workflowToken},
		{"GHCR_TOKEN", e.ghcrToken},
		{"REPO_TOKEN", e.repoToken},
	}
	for _, r := range required {
		if r.val == "" {
			failMissingEnv(r.name)
		}
	}
}

const (
	envRequiredSuffix  = " environment variable is required.\n"
	patSettingsURLLine = "  https://github.com/settings/tokens\n"
)

func failMissingEnv(name string) {
	switch name {
	case "GHCR_TOKEN":
		log.FatalExit(name + envRequiredSuffix +
			"  The scaffolded repo's acceptance/prod stages use it to tag images in GHCR.\n" +
			"  Create a Personal Access Token (classic) with write:packages + read:packages scopes:\n" +
			patSettingsURLLine +
			"  Then: export GHCR_TOKEN=<your-token>")
	case "WORKFLOW_TOKEN":
		log.FatalExit(name + envRequiredSuffix +
			"  The scaffolded repo's acceptance/QA/prod stages use it to push release tags\n" +
			"  (default GITHUB_TOKEN cannot push tags whose commit diffs workflow files).\n" +
			"  Create a Personal Access Token (classic) with repo + workflow scopes:\n" +
			patSettingsURLLine +
			"  Then: export WORKFLOW_TOKEN=<your-token>")
	case "REPO_TOKEN":
		log.FatalExit(name + envRequiredSuffix +
			"  In multitier+multirepo scaffolds, the system-level prod-stage uses it to read\n" +
			"  each component repo's VERSION file via the GitHub API (cross-repo Contents:read).\n" +
			"  Create a Personal Access Token (classic) with repo scope, OR a fine-grained PAT\n" +
			"  with Contents:Read on the component repos:\n" +
			patSettingsURLLine +
			"  Then: export REPO_TOKEN=<your-token>")
	default:
		log.Fatalf("%s environment variable is required", name)
	}
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

func checkGhAuth(dryRun bool) {
	if dryRun {
		return
	}
	cmd := exec.Command("gh", "auth", "status")
	if err := cmd.Run(); err != nil {
		log.FatalExit("gh CLI is not authenticated. Run 'gh auth login' first.")
	}
}

// ghAPISilent is the flag used with `gh api` to suppress the response body on
// success. 4xx/5xx still produce non-zero exit codes, which is what we key on.
const ghAPISilent = "--silent"

// checkOwnerExists verifies the owner resolves as a GitHub user or organization.
// Aborts via FatalExit if neither endpoint returns 200.
func checkOwnerExists(owner string) {
	// gh api returns non-zero on HTTP 4xx/5xx. Try user first (more common),
	// then org. Stderr is suppressed so the 404 from the user lookup doesn't
	// leak when we fall back to the org endpoint.
	userCmd := exec.Command("gh", "api", "users/"+owner, ghAPISilent)
	userCmd.Stderr = nil
	if err := userCmd.Run(); err == nil {
		return
	}
	orgCmd := exec.Command("gh", "api", "orgs/"+owner, ghAPISilent)
	orgCmd.Stderr = nil
	if err := orgCmd.Run(); err == nil {
		return
	}
	log.FatalExit(fmt.Sprintf("--owner %q: no GitHub user or organization with that name", owner))
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
func confirmReposExist(fullRepos []string, assumeYes bool) []string {
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

	fmt.Fprint(os.Stderr, "Proceed? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	resp, err := reader.ReadString('\n')
	if err != nil {
		// EOF or non-interactive stdin — treat as "no" with an actionable message.
		log.FatalExit(fmt.Sprintf("Aborted: %d repositor%s already exist and no confirmation was provided (pass --yes to proceed unattended)",
			len(existing), pluralY(len(existing))))
	}
	resp = strings.TrimSpace(strings.ToLower(resp))
	if resp != "y" && resp != "yes" {
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
	if !f.DryRun {
		validateEnvTokens(env)
	}

	mr := deriveMultirepoNames(f.RepoStrategy, f.Arch, f.Owner, repoName)

	// === Phase 2: network auth checks (fail fast before any mutation) ===
	// Token auth runs first because it's the most common failure mode and
	// aborts before we touch gh / GitHub / SonarCloud for existence checks.
	validateTokensAuth(env, f.DryRun)
	checkGhAuth(f.DryRun)
	checkOwnerExists(f.Owner)
	preExisting := confirmReposExist([]string{
		f.Owner + "/" + repoName,
		mr.backendFullRepo,
		mr.frontendFullRepo,
		mr.systemFullRepo,
	}, f.AssumeYes)
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
		Owner:      f.Owner,
		Repo:       repoName,
		FullRepo:   f.Owner + "/" + repoName,
		SystemName: f.SystemName,
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

		Deploy:     f.Deploy,
		License:    f.License,
		DryRun:       f.DryRun,
		VerifyLevel:    resolvedLevel,
		NoLegacy:       f.NoLegacy,
		NoLocalTests:   f.NoLocalTests,
		NoLocalSonar:   f.NoLocalSonar,
		NoAtdd:         f.NoAtdd,
		KeepLocal:    f.KeepLocal,
		BugReport:    f.BugReport,
		Verbose:      f.Verbose,
		Quiet:        f.Quiet,
		LogFile:      logFilePath,
		AssumeYes:    f.AssumeYes,
		WorkDir:    wd,
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
		DockerHubToken:   env.dockerHubToken,
		SonarToken:       env.sonarToken,
		GHCRToken:        env.ghcrToken,
		WorkflowToken:    env.workflowToken,
		RepoToken:        env.repoToken,

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

// LicenseName returns the human-readable license name.
func (c *Config) LicenseName() string {
	names := map[string]string{
		"mit":          "MIT License",
		"apache-2.0":   "Apache License 2.0",
		"gpl-3.0":      "GNU General Public License v3.0",
		"bsd-2-clause": "BSD 2-Clause License",
		"bsd-3-clause": "BSD 3-Clause License",
		"unlicense":    "The Unlicense",
	}
	if name, ok := names[c.License]; ok {
		return name
	}
	return c.License
}

// EffectiveLang returns the primary system language (lang for monolith, backend-lang for multitier).
func (c *Config) EffectiveLang() string {
	if c.Arch == "monolith" {
		return c.Lang
	}
	return c.BackendLang
}
