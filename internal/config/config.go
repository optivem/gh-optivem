// Package config provides CLI parsing, validation, and the Config struct.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/optivem/gh-optivem/internal/log"
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

	License    string
	DryRun     bool
	TestMode   bool
	Cleanup      string // "yes", "no", or "ask"
	ForceCleanup bool   // cleanup even on failure
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
	workDir := flag.String("workdir", "", "Working directory for cloning (default: temp dir)")

	flag.Parse()

	if *owner == "" || *systemName == "" || *repo == "" || *arch == "" || *repoStrategy == "" {
		fmt.Fprintln(os.Stderr, "Required flags: --owner, --system-name, --repo, --arch, --repo-strategy")
		flag.Usage()
		os.Exit(1)
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

	// Find starter path: the gh-optivem binary lives inside starter/gh-optivem/
	// so starter is the parent of the directory containing the executable.
	exe, err := os.Executable()
	if err != nil {
		log.FatalExit("Cannot determine executable path")
	}
	starterPath := filepath.Dir(filepath.Dir(exe))
	// Fallback: check if we're running from source (go run)
	if _, err := os.Stat(filepath.Join(starterPath, "VERSION")); err != nil {
		// Try current working directory's parent
		cwd, _ := os.Getwd()
		starterPath = filepath.Dir(cwd)
		if _, err := os.Stat(filepath.Join(starterPath, "VERSION")); err != nil {
			// Try environment variable
			if envPath := os.Getenv("OPTIVEM_STARTER_PATH"); envPath != "" {
				starterPath = envPath
			} else {
				log.FatalExit("Cannot find VERSION file. Set OPTIVEM_STARTER_PATH to the starter repo root.")
			}
		}
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
		wd, err = os.MkdirTemp("", "scaffold-")
		if err != nil {
			log.FatalExit("Cannot create temp directory: " + err.Error())
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

		License:    *license,
		DryRun:     *dryRun,
		TestMode:   *testMode,
		Cleanup:      resolveCleanup(*cleanupFlag, *noCleanup),
		ForceCleanup: *forceCleanup,
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

		FrontendRepo:     frontendRepo,
		BackendRepo:      backendRepo,
		FrontendFullRepo: frontendFullRepo,
		BackendFullRepo:  backendFullRepo,

		SystemRepo:     systemRepo,
		SystemFullRepo: systemFullRepo,
	}
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
