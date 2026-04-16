// Package config provides CLI parsing, validation, and the Config struct.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/optivem/gh-optivem/internal/log"
	"github.com/optivem/gh-optivem/internal/version"
)

type Config struct {
	Owner      string
	Repo       string
	FullRepo   string
	SystemName string
	Arch         string // "monolith" or "multitier"
	RepoStrategy string // "monorepo" or "multirepo"

	Lang         string // monolith only
	BackendLang  string // multitier only
	FrontendLang string // multitier only
	TestLang     string

	Deploy     string // "docker" (default) or "cloud-run"
	License    string
	DryRun       bool
	TestMode     bool
	VerifyLevel   string // "none", "precommit", "commit", "acceptance", "release"
	ExcludeLegacy bool   // exclude acceptance-stage-legacy verification
	Cleanup       string // "yes", "no", or "ask"
	ForceCleanup bool   // cleanup even on failure
	NoBugReport  bool   // skip auto-creating GitHub issues on failure
	WorkDir    string
	StarterPath string

	DockerHubUsername string
	DockerHubToken   string
	SonarToken       string
	GHCRToken        string

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

	// Set during verification
	RCVersion string
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
			if !isUpper(s[i-1]) {
				// lowercase -> uppercase: split before i
				words = append(words, s[start:i])
				start = i
			} else if i+1 < len(s) && !isUpper(s[i+1]) {
				// uppercase -> lowercase after acronym: split before i
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

// ValidateSystemName checks the system name against all naming constraints.
// Returns an error message or empty string if valid.
func ValidateSystemName(name string) string {
	if len(strings.TrimSpace(name)) == 0 {
		return "system name cannot be empty"
	}
	if name != strings.TrimSpace(name) {
		return "system name cannot have leading or trailing spaces"
	}

	// Check each word
	words := strings.Fields(name)
	for _, w := range words {
		// Only letters allowed
		for _, c := range w {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
				return fmt.Sprintf("system name contains invalid character '%c' — only letters and spaces allowed", c)
			}
		}
	}

	// Check each word against reserved words
	for _, w := range words {
		lower := strings.ToLower(w)
		if isLanguageReserved(lower) {
			return fmt.Sprintf("word %q is a language reserved keyword", w)
		}
		if isScaffoldReserved(lower) {
			return fmt.Sprintf("word %q is a scaffold reserved word (collides with infrastructure names)", w)
		}
	}

	// Check full derived forms against reserved words
	camel := SpacesToCamel(name)
	lower := SpacesToLower(name)
	if isLanguageReserved(lower) {
		return fmt.Sprintf("derived form %q is a language reserved keyword", lower)
	}
	if isLanguageReserved(camel) {
		return fmt.Sprintf("derived form %q is a language reserved keyword", camel)
	}

	// Length check
	if len(name) > 50 {
		return "system name exceeds 50 character limit"
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

func ParseAndValidate() *Config {
	owner := flag.String("owner", "", "GitHub username or org (required)")
	systemName := flag.String("system-name", "", `System name, e.g. "Page Turner" (required)`)
	repo := flag.String("repo", "", "Repository name, e.g. page-turner (required)")
	arch := flag.String("arch", "", "Architecture: monolith or multitier (required)")
	repoStrategy := flag.String("repo-strategy", "", "Repo strategy: monorepo or multirepo (required)")
	lang := flag.String("lang", "", "System language: java, dotnet, typescript (monolith)")
	testLang := flag.String("test-lang", "", "Test language (defaults to --lang or --backend-lang)")
	backendLang := flag.String("backend-lang", "", "Backend language: java, dotnet, typescript (multitier)")
	frontendLang := flag.String("frontend-lang", "", "Frontend language: react (multitier)")
	license := flag.String("license", "mit", "License: mit, apache-2.0, gpl-3.0, bsd-2-clause, bsd-3-clause, unlicense")
	randomSuffix := flag.Bool("random-suffix", false, "Append 4-char hex suffix to repo name")
	dryRun := flag.Bool("dry-run", false, "Print actions without executing")
	testMode := flag.Bool("test", false, "Test mode with optional cleanup")
	cleanupFlag := flag.Bool("cleanup", false, "Auto-cleanup in test mode")
	noCleanup := flag.Bool("no-cleanup", false, "Keep repo in test mode")
	forceCleanup := flag.Bool("force-cleanup", false, "Cleanup even on failure")
	verifyLevel := flag.String("verify-level", "", "Verification level: none, precommit, commit, acceptance, release (default: release)")
	excludeLegacy := flag.Bool("exclude-legacy", false, "Exclude acceptance-stage-legacy verification")
	noBugReport := flag.Bool("no-bug-report", false, "Skip auto-creating GitHub issues on failure")
	deploy := flag.String("deploy", "docker", "Deployment target: docker or cloud-run")
	workDir := flag.String("workdir", "", "Working directory for cloning (default: temp dir)")
	showVersion := flag.Bool("version", false, "Print version and exit")

	flag.Parse()

	if *showVersion {
		fmt.Printf("gh-optivem %s\n", version.Version)
		os.Exit(0)
	}

	if *owner == "" || *systemName == "" || *repo == "" || *arch == "" || *repoStrategy == "" {
		fmt.Fprintln(os.Stderr, "Required flags: --owner, --system-name, --repo, --arch, --repo-strategy")
		flag.Usage()
		os.Exit(1)
	}

	if err := ValidateSystemName(*systemName); err != "" {
		log.FatalExit("--system-name: " + err)
	}

	// Resolve verify level
	resolvedLevel := "release"
	if *verifyLevel != "" {
		validLevels := map[string]bool{"none": true, "precommit": true, "commit": true, "acceptance": true, "release": true}
		if !validLevels[*verifyLevel] {
			log.FatalExit("--verify-level must be none, precommit, commit, acceptance, or release")
		}
		resolvedLevel = *verifyLevel
	}

	if *deploy != "docker" && *deploy != "cloud-run" {
		log.FatalExit("--deploy must be 'docker' or 'cloud-run'")
	}

	if *arch != "monolith" && *arch != "multitier" {
		log.FatalExit("--arch must be 'monolith' or 'multitier'")
	}

	if *repoStrategy != "monorepo" && *repoStrategy != "multirepo" {
		log.FatalExit("--repo-strategy must be 'monorepo' or 'multirepo'")
	}

	validLangs := map[string]bool{"java": true, "dotnet": true, "typescript": true}

	var cfgLang, cfgBackendLang, cfgFrontendLang, cfgTestLang string

	if *arch == "monolith" {
		if *lang == "" {
			log.FatalExit("--lang is required for monolith architecture")
		}
		if !validLangs[*lang] {
			log.FatalExit("--lang must be java, dotnet, or typescript")
		}
		cfgLang = *lang
		cfgTestLang = *testLang
		if cfgTestLang == "" {
			cfgTestLang = cfgLang
		}
	} else {
		if *backendLang == "" {
			log.FatalExit("--backend-lang is required for multitier architecture")
		}
		if *frontendLang == "" {
			log.FatalExit("--frontend-lang is required for multitier architecture")
		}
		if !validLangs[*backendLang] {
			log.FatalExit("--backend-lang must be java, dotnet, or typescript")
		}
		if *frontendLang != "react" {
			log.FatalExit("--frontend-lang must be react")
		}
		cfgBackendLang = *backendLang
		cfgFrontendLang = *frontendLang
		cfgTestLang = *testLang
		if cfgTestLang == "" {
			cfgTestLang = cfgBackendLang
		}
	}

	repoName := *repo
	if *randomSuffix {
		b := make([]byte, 2)
		rand.Read(b)
		repoName = repoName + "-" + hex.EncodeToString(b)
	}

	// Environment variables
	dockerHubUsername := os.Getenv("DOCKERHUB_USERNAME")
	dockerHubToken := os.Getenv("DOCKERHUB_TOKEN")
	sonarToken := os.Getenv("SONAR_TOKEN")
	ghcrToken := os.Getenv("GHCR_TOKEN")

	if !*dryRun {
		required := []struct{ name, val string }{
			{"DOCKERHUB_USERNAME", dockerHubUsername},
			{"DOCKERHUB_TOKEN", dockerHubToken},
			{"SONAR_TOKEN", sonarToken},
		}
		if *repoStrategy == "multirepo" {
			required = append(required, struct{ name, val string }{"GHCR_TOKEN", ghcrToken})
		}
		for _, r := range required {
			if r.val == "" {
				if r.name == "GHCR_TOKEN" {
					log.FatalExit(r.name + " environment variable is required for multirepo setup.\n" +
						"  Create a Personal Access Token (classic) with write:packages + read:packages scopes:\n" +
						"  https://github.com/settings/tokens\n" +
						"  Then: export GHCR_TOKEN=<your-token>")
				}
				log.Fatalf("%s environment variable is required", r.name)
			}
		}
	}

	// Clone starter repo from GitHub into a temp directory.
	starterPath, cloneErr := cloneStarter()
	if cloneErr != nil {
		log.FatalExit("Cannot clone starter repo: " + cloneErr.Error())
	}

	// Check gh auth
	if !*dryRun {
		cmd := exec.Command("gh", "auth", "status")
		if err := cmd.Run(); err != nil {
			log.FatalExit("gh CLI is not authenticated. Run 'gh auth login' first.")
		}
	}

	// Derived naming
	ownerPascal := ToPascalCase(*owner)
	if !strings.Contains(*owner, "-") {
		ownerPascal = strings.ToUpper((*owner)[:1]) + (*owner)[1:]
	}
	ownerLower := strings.ToLower(*owner)
	repoPascal := ToPascalCase(repoName)
	repoNoHyphens := ToJavaLower(repoName)

	frontendRepo := ""
	backendRepo := ""
	frontendFullRepo := ""
	backendFullRepo := ""
	systemRepo := ""
	systemFullRepo := ""
	if *repoStrategy == "multirepo" {
		if *arch == "multitier" {
			frontendRepo = repoName + "-frontend"
			backendRepo = repoName + "-backend"
			frontendFullRepo = *owner + "/" + frontendRepo
			backendFullRepo = *owner + "/" + backendRepo
		} else {
			systemRepo = repoName + "-system"
			systemFullRepo = *owner + "/" + systemRepo
		}
	}

	// Work directory
	wd := *workDir
	if wd == "" {
		var mkErr error
		wd, mkErr = os.MkdirTemp("", "scaffold-")
		if mkErr != nil {
			log.FatalExit("Cannot create temp directory: " + mkErr.Error())
		}
	}

	return &Config{
		Owner:      *owner,
		Repo:       repoName,
		FullRepo:   *owner + "/" + repoName,
		SystemName: *systemName,
		Arch:         *arch,
		RepoStrategy: *repoStrategy,

		Lang:         cfgLang,
		BackendLang:  cfgBackendLang,
		FrontendLang: cfgFrontendLang,
		TestLang:     cfgTestLang,

		Deploy:     *deploy,
		License:    *license,
		DryRun:       *dryRun,
		TestMode:     *testMode,
		VerifyLevel:   resolvedLevel,
		ExcludeLegacy: *excludeLegacy,
		Cleanup:      resolveCleanup(*cleanupFlag, *noCleanup),
		ForceCleanup: *forceCleanup,
		NoBugReport:  *noBugReport,
		WorkDir:    wd,
		StarterPath: starterPath,

		DockerHubUsername: dockerHubUsername,
		DockerHubToken:   dockerHubToken,
		SonarToken:       sonarToken,
		GHCRToken:        ghcrToken,

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
		SysNamePascalNew: SpacesToPascal(*systemName),
		SysNameCamelOld:  "shop",
		SysNameCamelNew:  SpacesToCamel(*systemName),
		SysNameKebabOld:  "shop",
		SysNameKebabNew:  SpacesToKebab(*systemName),
		SysNameLowerOld:  "shop",
		SysNameLowerNew:  SpacesToLower(*systemName),

		FrontendRepo:     frontendRepo,
		BackendRepo:      backendRepo,
		FrontendFullRepo: frontendFullRepo,
		BackendFullRepo:  backendFullRepo,

		SystemRepo:     systemRepo,
		SystemFullRepo: systemFullRepo,
	}
}

// cloneStarter clones optivem/starter from GitHub into a temp directory.
func cloneStarter() (string, error) {
	dir, err := os.MkdirTemp("", "starter-")
	if err != nil {
		return "", fmt.Errorf("cannot create temp dir: %w", err)
	}

	cmd := exec.Command("gh", "repo", "clone", "optivem/starter", dir, "--", "--depth=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("gh repo clone failed: %s\n%s", err, string(out))
	}

	log.OKf("Cloned starter to %s", dir)
	return dir, nil
}

func resolveCleanup(cleanup, noCleanup bool) string {
	if cleanup {
		return "yes"
	}
	if noCleanup {
		return "no"
	}
	return "ask"
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
