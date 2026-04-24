// Package config provides CLI parsing, validation, and the Config struct.
package config

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/optivem/gh-optivem/internal/log"
	"github.com/optivem/gh-optivem/internal/version"
)

// RawInputs captures the raw CLI flag values before any resolution/defaulting.
// Surfaced in the startup banner so the user can audit exactly what they typed
// (vs. the resolved/derived values the scaffold will actually act on).
type RawInputs struct {
	Repo            string // --repo as-passed
	ShopTag         string // --shop-tag before resolving empty → latest meta-v*
	TestLang        string // --test-lang before defaulting to --lang / --backend-lang
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
	ExcludeLegacy  bool   // exclude legacy from local tests and acceptance stage
	SkipLocalTests bool   // skip the "Verify local testing" step (Run-SystemTests.ps1)
	SampleTests   bool   // run only sample local tests instead of all
	KeepLocal    bool   // keep the local scaffolded clone dir after a successful run (default: delete it)
	NoBugReport  bool   // skip auto-creating GitHub issues on failure
	Verbose      bool   // enable debug output
	Quiet        bool   // suppress info-level output
	LogFile      string // optional path to mirror plain-text log output
	NoAutoUpgrade bool  // skip auto-upgrade when a newer release is available
	WorkDir    string
	ShopPath string
	ShopRef  string // Pinned optivem/shop ref (SHA or meta-v* tag). Never empty, never main.

	DockerHubUsername string
	DockerHubToken   string
	SonarToken       string
	GHCRToken        string
	WorkflowToken    string

	// Derived naming
	OwnerPascal   string
	OwnerLower    string
	RepoPascal    string
	RepoNoHyphens string

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

type rawFlags struct {
	owner, systemName, repo, arch, repoStrategy                  *string
	lang, testLang, backendLang, frontendLang                    *string
	license, verifyLevel, deploy, workDir, shopTag               *string
	dryRun, keepLocal                                            *bool
	excludeLegacy, skipLocalTests, sampleTests                   *bool
	noBugReport, showVersion                                     *bool
	verbose, verboseShort, quiet, quietShort, noAutoUpgrade      *bool
	logFile                                                      *string
}

func registerFlags() rawFlags {
	return rawFlags{
		owner:         flag.String("owner", "", "GitHub username or org (required)"),
		systemName:    flag.String("system-name", "", `System name, e.g. "Page Turner" (required)`),
		repo:          flag.String("repo", "", "Repository name, e.g. page-turner (required)"),
		arch:          flag.String("arch", "", "Architecture: monolith or multitier (required)"),
		repoStrategy:  flag.String("repo-strategy", "", "Repo strategy: monorepo or multirepo (required)"),
		lang:          flag.String("lang", "", "System language: java, dotnet, typescript (monolith)"),
		testLang:      flag.String("test-lang", "", "Test language (defaults to --lang or --backend-lang)"),
		backendLang:   flag.String("backend-lang", "", "Backend language: java, dotnet, typescript (multitier)"),
		frontendLang:  flag.String("frontend-lang", "", "Frontend language: react (multitier)"),
		license:       flag.String("license", "mit", "License: mit, apache-2.0, gpl-3.0, bsd-2-clause, bsd-3-clause, unlicense"),
		dryRun:        flag.Bool("dry-run", false, "Print actions without executing"),
		keepLocal:     flag.Bool("keep-local", false, "Keep the local scaffolded clone dir instead of deleting it on success"),
		verifyLevel:    flag.String("verify-level", "release", "Verification level: none, local, commit, acceptance, qa, release"),
		excludeLegacy:  flag.Bool("exclude-legacy", false, "Exclude legacy from local tests and acceptance stage"),
		skipLocalTests: flag.Bool("skip-local-tests", false, "Skip the 'Verify local testing' step (Run-SystemTests.ps1)"),
		sampleTests:    flag.Bool("sample-tests", false, "Run only sample local tests instead of all"),
		noBugReport:   flag.Bool("no-bug-report", false, "Skip auto-creating GitHub issues on failure"),
		deploy:        flag.String("deploy", "docker", "Deployment target: docker or cloud-run"),
		workDir:       flag.String("workdir", "", "Working directory for cloning (default: temp dir)"),
		shopTag:       flag.String("shop-tag", "", "Pin optivem/shop to this meta-v* release tag. Overrides build-time pin. Default: latest meta-v* release."),
		showVersion:   flag.Bool("version", false, "Print version and exit"),
		verbose:       flag.Bool("verbose", false, "Enable debug output (retry/wait chatter, diagnostics)"),
		verboseShort:  flag.Bool("v", false, "Short for --verbose"),
		quiet:         flag.Bool("quiet", false, "Suppress info-level output (warnings and errors still shown)"),
		quietShort:    flag.Bool("q", false, "Short for --quiet"),
		logFile:       flag.String("log-file", "", "Also write plain-text log output to this file (no ANSI colors, all levels)"),
		noAutoUpgrade: flag.Bool("no-auto-upgrade", false, "Skip auto-upgrade when a newer release is available (useful for CI/debugging)"),
	}
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

func resolveLangs(f rawFlags) langChoice {
	validLangs := map[string]bool{"java": true, "dotnet": true, "typescript": true}
	var c langChoice
	if *f.arch == "monolith" {
		if *f.lang == "" {
			log.FatalExit("--lang is required for monolith architecture")
		}
		if !validLangs[*f.lang] {
			log.FatalExit("--lang must be java, dotnet, or typescript")
		}
		c.lang = *f.lang
		c.testLang = *f.testLang
		if c.testLang == "" {
			c.testLang = c.lang
		}
		return c
	}
	if *f.backendLang == "" {
		log.FatalExit("--backend-lang is required for multitier architecture")
	}
	if *f.frontendLang == "" {
		log.FatalExit("--frontend-lang is required for multitier architecture")
	}
	if !validLangs[*f.backendLang] {
		log.FatalExit("--backend-lang must be java, dotnet, or typescript")
	}
	if *f.frontendLang != "react" {
		log.FatalExit("--frontend-lang must be react")
	}
	c.backendLang = *f.backendLang
	c.frontendLang = *f.frontendLang
	c.testLang = *f.testLang
	if c.testLang == "" {
		c.testLang = c.backendLang
	}
	return c
}

type envTokens struct {
	dockerHubUsername, dockerHubToken, sonarToken, ghcrToken, workflowToken string
}

func readEnvTokens() envTokens {
	return envTokens{
		dockerHubUsername: os.Getenv("DOCKERHUB_USERNAME"),
		dockerHubToken:    os.Getenv("DOCKERHUB_TOKEN"),
		sonarToken:        os.Getenv("SONAR_TOKEN"),
		ghcrToken:         os.Getenv("GHCR_TOKEN"),
		workflowToken:     os.Getenv("WORKFLOW_TOKEN"),
	}
}

func validateEnvTokens(e envTokens, repoStrategy string) {
	required := []struct{ name, val string }{
		{"DOCKERHUB_USERNAME", e.dockerHubUsername},
		{"DOCKERHUB_TOKEN", e.dockerHubToken},
		{"SONAR_TOKEN", e.sonarToken},
		{"WORKFLOW_TOKEN", e.workflowToken},
	}
	if repoStrategy == "multirepo" {
		required = append(required, struct{ name, val string }{"GHCR_TOKEN", e.ghcrToken})
	}
	for _, r := range required {
		if r.val == "" {
			failMissingEnv(r.name)
		}
	}
}

func failMissingEnv(name string) {
	switch name {
	case "GHCR_TOKEN":
		log.FatalExit(name + " environment variable is required for multirepo setup.\n" +
			"  Create a Personal Access Token (classic) with write:packages + read:packages scopes:\n" +
			"  https://github.com/settings/tokens\n" +
			"  Then: export GHCR_TOKEN=<your-token>")
	case "WORKFLOW_TOKEN":
		log.FatalExit(name + " environment variable is required.\n" +
			"  The scaffolded repo's acceptance/QA/prod stages use it to push release tags\n" +
			"  (default GITHUB_TOKEN cannot push tags whose commit diffs workflow files).\n" +
			"  Create a Personal Access Token (classic) with repo + workflow scopes:\n" +
			"  https://github.com/settings/tokens\n" +
			"  Then: export WORKFLOW_TOKEN=<your-token>")
	default:
		log.Fatalf("%s environment variable is required", name)
	}
}

func resolveShopRef(shopTag string) string {
	if shopTag != "" && isMainRef(shopTag) {
		log.FatalExit("Invalid --shop-tag: 'main'/'master' is not allowed — pass a published meta-v* release tag.")
	}
	ref := shopTag
	if ref == "" {
		ref = version.ShopRef
	}
	if ref == "" {
		latest, err := latestMetaRelease()
		if err != nil {
			log.FatalExit("Cannot resolve shop tag: " + err.Error())
		}
		ref = latest
		// Surfaced in the banner's Derived block; log at debug for --verbose traces.
		log.Debugf("Resolved empty shop-tag to latest meta-v* release: %s", ref)
	}
	if isMainRef(ref) {
		log.FatalExit("Invalid shop ref: 'main'/'master' is not allowed — acceptance requires a published meta-v* release tag.")
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

// confirmRepoExists checks whether fullRepo ("<owner>/<name>") already exists.
// If it does, prompts the user to confirm scaffolding into the existing repo.
// Aborts via FatalExit if the user declines or stdin is not available.
func confirmRepoExists(fullRepo string) {
	cmd := exec.Command("gh", "api", "repos/"+fullRepo, ghAPISilent)
	cmd.Stderr = nil // 404 is the expected case — suppress the noise
	if err := cmd.Run(); err != nil {
		// Repo doesn't exist (or API is unreachable). Continue — if it's really
		// unreachable, later steps will fail with a clearer error.
		return
	}

	log.Warnf("Repository %s already exists on GitHub.", fullRepo)
	log.Warnf("Proceeding will scaffold into the existing repository and may overwrite its contents.")
	fmt.Fprint(os.Stderr, "Proceed? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	resp, err := reader.ReadString('\n')
	if err != nil {
		// EOF or non-interactive stdin — treat as "no" with an actionable message.
		log.FatalExit(fmt.Sprintf("Aborted: repository %s already exists and no confirmation was provided", fullRepo))
	}
	resp = strings.TrimSpace(strings.ToLower(resp))
	if resp != "y" && resp != "yes" {
		log.FatalExit(fmt.Sprintf("Aborted: repository %s already exists", fullRepo))
	}
}

func ParseAndValidate() *Config {
	f := registerFlags()
	flag.Parse()

	userSet := make(map[string]bool)
	flag.Visit(func(fl *flag.Flag) { userSet[fl.Name] = true })

	if *f.showVersion {
		fmt.Println(version.Full())
		os.Exit(0)
	}

	if *f.owner == "" || *f.systemName == "" || *f.repo == "" || *f.arch == "" || *f.repoStrategy == "" {
		fmt.Fprintln(os.Stderr, "Required flags: --owner, --system-name, --repo, --arch, --repo-strategy")
		flag.Usage()
		os.Exit(1)
	}

	verbose := *f.verbose || *f.verboseShort
	quiet := *f.quiet || *f.quietShort
	if verbose && quiet {
		log.FatalExit("--verbose and --quiet are mutually exclusive")
	}

	// === Phase 1: in-memory format validation (fast, no network) ===
	if err := ValidateOwnerFormat(*f.owner); err != "" {
		log.FatalExit("--owner: " + err)
	}
	if err := ValidateRepoFormat(*f.repo); err != "" {
		log.FatalExit("--repo: " + err)
	}
	if err := ValidateSystemName(*f.systemName); err != "" {
		log.FatalExit("--system-name: " + err)
	}

	resolvedLevel := resolveVerifyLevel(*f.verifyLevel)
	validateCommonFlags(*f.deploy, *f.arch, *f.repoStrategy)
	lc := resolveLangs(f)

	repoName := *f.repo

	env := readEnvTokens()
	if !*f.dryRun {
		validateEnvTokens(env, *f.repoStrategy)
	}

	mr := deriveMultirepoNames(*f.repoStrategy, *f.arch, *f.owner, repoName)

	// === Phase 2: GitHub existence checks (fail fast before slow operations) ===
	checkGhAuth(*f.dryRun)
	checkOwnerExists(*f.owner)
	confirmRepoExists(*f.owner + "/" + repoName)
	if mr.backendFullRepo != "" {
		confirmRepoExists(mr.backendFullRepo)
	}
	if mr.frontendFullRepo != "" {
		confirmRepoExists(mr.frontendFullRepo)
	}
	if mr.systemFullRepo != "" {
		confirmRepoExists(mr.systemFullRepo)
	}

	// === Phase 3: resolve shop ref (fast API call; actual clone happens in the Prepare step) ===
	resolvedShopRef := resolveShopRef(*f.shopTag)

	ownerPascal := computeOwnerPascal(*f.owner)
	ownerLower := strings.ToLower(*f.owner)
	repoPascal := ToPascalCase(repoName)
	repoNoHyphens := ToJavaLower(repoName)
	wd := resolveWorkDir(*f.workDir)
	clones := resolveCloneDirs(wd, *f.repoStrategy, *f.arch)

	return &Config{
		Owner:      *f.owner,
		Repo:       repoName,
		FullRepo:   *f.owner + "/" + repoName,
		SystemName: *f.systemName,
		Arch:         *f.arch,
		RepoStrategy: *f.repoStrategy,

		Raw: RawInputs{
			Repo:        *f.repo,
			ShopTag:     *f.shopTag,
			TestLang:    *f.testLang,
			VerifyLevel: *f.verifyLevel,
			WorkDir:     *f.workDir,
			KeepLocal:   *f.keepLocal,
		},
		UserSetFlags: userSet,

		Lang:         lc.lang,
		BackendLang:  lc.backendLang,
		FrontendLang: lc.frontendLang,
		TestLang:     lc.testLang,

		Deploy:     *f.deploy,
		License:    *f.license,
		DryRun:       *f.dryRun,
		VerifyLevel:    resolvedLevel,
		ExcludeLegacy:  *f.excludeLegacy,
		SkipLocalTests: *f.skipLocalTests,
		SampleTests:    *f.sampleTests,
		KeepLocal:    *f.keepLocal,
		NoBugReport:  *f.noBugReport,
		Verbose:      verbose,
		Quiet:        quiet,
		LogFile:      *f.logFile,
		NoAutoUpgrade: *f.noAutoUpgrade,
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

		OwnerPascal:   ownerPascal,
		OwnerLower:    ownerLower,
		RepoPascal:    repoPascal,
		RepoNoHyphens: repoNoHyphens,

		JavaNsOld:   "com.optivem.shop",
		JavaNsNew:   "com." + ownerLower + "." + repoNoHyphens,
		DotnetNsOld: "Optivem.Shop",
		DotnetNsNew: ownerPascal + "." + repoPascal,
		TsPkgOld:    "@optivem/shop-system-test",
		TsPkgNew:    "@" + ownerLower + "/" + repoName + "-system-test",

		SysNamePascalOld: "Shop",
		SysNamePascalNew: SpacesToPascal(*f.systemName),
		SysNameCamelOld:  "shop",
		SysNameCamelNew:  SpacesToCamel(*f.systemName),
		SysNameKebabOld:  "shop",
		SysNameKebabNew:  SpacesToKebab(*f.systemName),
		SysNameLowerOld:  "shop",
		SysNameLowerNew:  SpacesToLower(*f.systemName),

		FrontendRepo:     mr.frontendRepo,
		BackendRepo:      mr.backendRepo,
		FrontendFullRepo: mr.frontendFullRepo,
		BackendFullRepo:  mr.backendFullRepo,

		SystemRepo:     mr.systemRepo,
		SystemFullRepo: mr.systemFullRepo,
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

// isMainRef reports whether ref names the main/master branch in any form.
func isMainRef(ref string) bool {
	switch ref {
	case "main", "master", "refs/heads/main", "refs/heads/master", "HEAD":
		return true
	}
	return false
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
