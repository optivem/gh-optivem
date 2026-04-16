package steps

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/files"
	"github.com/optivem/gh-optivem/internal/log"
	"github.com/optivem/gh-optivem/internal/templates"
)

// All text file extensions to process.
var textExts = []string{
	".yml", ".yaml", ".md", ".gradle", ".gradle.kts",
	".csproj", ".sln", ".slnx", ".cshtml", ".json",
	".cs", ".java", ".ts", ".tsx", ".js", ".jsx",
	".xml", ".properties", ".cfg", ".txt",
}

// ReplaceRepoReferences replaces optivem/starter references with the target repo.
func ReplaceRepoReferences(cfg *config.Config) {
	log.Log("Step 6: Replacing repository references...")

	if cfg.DryRun {
		log.Logf("[DRY RUN] Would replace optivem/starter -> %s", cfg.FullRepo)
		return
	}

	replaceRefsInRepo(cfg.RepoDir, cfg.FullRepo, cfg.OwnerLower)

	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "multitier" {
			replaceRefsInRepo(cfg.BackendRepoDir, cfg.BackendFullRepo, cfg.OwnerLower)
			replaceRefsInRepo(cfg.FrontendRepoDir, cfg.FrontendFullRepo, cfg.OwnerLower)

			// Fix docker-compose image URLs
			templates.FixupMultirepoDockerCompose(
				cfg.RepoDir, cfg.Repo, cfg.FrontendRepo, cfg.BackendRepo,
			)
		} else {
			replaceRefsInRepo(cfg.SystemRepoDir, cfg.SystemFullRepo, cfg.OwnerLower)

			// Fix docker-compose image URLs
			templates.FixupMonolithMultirepoDockerCompose(
				cfg.RepoDir, cfg.Repo, cfg.SystemRepo,
			)
		}
	}

	// Replace "shop" in infrastructure files (docker-compose, DB config, PowerShell scripts)
	// with the repo name. This is separate from system name replacement.
	replaceInfraNames(cfg)

	log.OK("Repository reference replacement complete")
}

// replaceInfraNames replaces the template infrastructure name "shop" with the repo name
// in docker-compose files, DB config, application config, and test scripts.
func replaceInfraNames(cfg *config.Config) {
	repoKebab := cfg.Repo                                          // e.g. "sky-travel"
	repoLower := strings.ReplaceAll(strings.ToLower(cfg.Repo), "-", "") // e.g. "skytravel"

	// Collect all repo dirs
	repoDirs := []string{cfg.RepoDir}
	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "multitier" {
			repoDirs = append(repoDirs, cfg.BackendRepoDir, cfg.FrontendRepoDir)
		} else {
			repoDirs = append(repoDirs, cfg.SystemRepoDir)
		}
	}

	for _, repoDir := range repoDirs {
		if repoDir == "" {
			continue
		}

		// Docker-compose project names: "shop-" -> "sky-travel-" (kebab)
		n := 0
		filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || files.IsGitDir(path) {
				return nil
			}
			if strings.Contains(info.Name(), "docker-compose") && strings.HasSuffix(info.Name(), ".yml") {
				// Project name: "shop-monolith-real" -> "sky-travel-monolith-real"
				if files.ReplaceInFile(path, "name: shop-", "name: "+repoKebab+"-") {
					n++
				}
				// DB env vars in docker-compose
				files.ReplaceInFile(path, "POSTGRES_DB=shop", "POSTGRES_DB="+repoLower)
				files.ReplaceInFile(path, "POSTGRES_USER=shop_user", "POSTGRES_USER="+repoLower+"_user")
				files.ReplaceInFile(path, "POSTGRES_PASSWORD=shop_password", "POSTGRES_PASSWORD="+repoLower+"_password")
				files.ReplaceInFile(path, "POSTGRES_USER=shop\n", "POSTGRES_USER="+repoLower+"\n")
				files.ReplaceInFile(path, "POSTGRES_PASSWORD=shop\n", "POSTGRES_PASSWORD="+repoLower+"\n")
				files.ReplaceInFile(path, "pg_isready -U shop_user -d shop", "pg_isready -U "+repoLower+"_user -d "+repoLower)
				files.ReplaceInFile(path, "pg_isready -U shop -d shop", "pg_isready -U "+repoLower+" -d "+repoLower)
				// App DB env vars
				files.ReplaceInFile(path, "POSTGRES_DB_NAME=shop", "POSTGRES_DB_NAME="+repoLower)
				files.ReplaceInFile(path, "POSTGRES_DB_USER=shop_user", "POSTGRES_DB_USER="+repoLower+"_user")
				files.ReplaceInFile(path, "POSTGRES_DB_PASSWORD=shop_password", "POSTGRES_DB_PASSWORD="+repoLower+"_password")
			}
			return nil
		})
		if n > 0 {
			log.OKf("Infra: replaced docker-compose project names shop- -> %s- (%d files)", repoKebab, n)
		}

		// Application config DB defaults (application.yml, appsettings.json, .ts config)
		filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || files.IsGitDir(path) {
				return nil
			}
			name := info.Name()
			isAppConfig := strings.HasPrefix(name, "application") ||
				strings.HasPrefix(name, "appsettings") ||
				name == "db.ts" || name == "app.module.ts" || name == "app.config.ts"
			if !isAppConfig {
				return nil
			}

			// Java application.yml: POSTGRES_DB_NAME:shop
			files.ReplaceInFile(path, "POSTGRES_DB_NAME:shop", "POSTGRES_DB_NAME:"+repoLower)
			files.ReplaceInFile(path, "POSTGRES_DB_USER:shop_user", "POSTGRES_DB_USER:"+repoLower+"_user")
			files.ReplaceInFile(path, "POSTGRES_DB_PASSWORD:shop_password", "POSTGRES_DB_PASSWORD:"+repoLower+"_password")
			// .NET appsettings.json: Database=shop;Username=shop;Password=shop
			files.ReplaceInFile(path, "Database=shop;Username=shop;Password=shop",
				"Database="+repoLower+";Username="+repoLower+";Password="+repoLower)
			// .NET Program.cs defaults: ?? "shop"
			files.ReplaceInFile(path, `?? "shop"`, `?? "`+repoLower+`"`)
			// TS defaults: 'shop', 'shop_user', 'shop_password'
			files.ReplaceInFile(path, "'shop_user'", "'"+repoLower+"_user'")
			files.ReplaceInFile(path, "'shop_password'", "'"+repoLower+"_password'")
			files.ReplaceInFile(path, "'shop'", "'"+repoLower+"'")
			return nil
		})

		// PowerShell test scripts: container names "shop-" -> repo kebab
		filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || files.IsGitDir(path) {
				return nil
			}
			if strings.HasSuffix(info.Name(), ".ps1") {
				files.ReplaceInFile(path, `"shop-`, `"`+repoKebab+`-`)
			}
			return nil
		})
	}

	log.OK("Infra: replaced infrastructure names (docker-compose, DB config, scripts)")
}

func replaceRefsInRepo(repoDir, fullRepo, ownerLower string) {
	// Pass 1: optivem/starter -> owner/repo
	n := files.ReplaceInTree(repoDir, "optivem/starter", fullRepo, textExts)
	n += files.ReplaceInDockerfiles(repoDir, "optivem/starter", fullRepo)
	log.OKf("Pass 1: replaced optivem/starter -> %s (%d files)", fullRepo, n)

	// Pass 2: optivem_starter -> owner_repo (SonarCloud underscore variant)
	underscoreNew := strings.ReplaceAll(fullRepo, "/", "_")
	n = files.ReplaceInTree(repoDir, "optivem_starter", underscoreNew, textExts)
	log.OKf("Pass 2: replaced optivem_starter -> %s (%d files)", underscoreNew, n)

	// Pass 3: SonarCloud org patterns
	sonarReplacements := [][2]string{
		{"'sonar.organization', 'optivem'", "'sonar.organization', '" + ownerLower + "'"},
		{`/o:"optivem"`, `/o:"` + ownerLower + `"`},
		{"-Dsonar.organization=optivem", "-Dsonar.organization=" + ownerLower},
	}
	for _, pair := range sonarReplacements {
		n = files.ReplaceInTree(repoDir, pair[0], pair[1], nil)
		if n > 0 {
			log.OKf("Pass 3: replaced sonar org pattern (%d files)", n)
		}
	}

	// Safety check: optivem/actions must still be intact in any copied workflows
	wfDir := filepath.Join(repoDir, ".github", "workflows")
	if info, err := os.Stat(wfDir); err == nil && info.IsDir() {
		actionsFound := false
		ymlCount := 0
		entries, _ := os.ReadDir(wfDir)
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".yml") {
				continue
			}
			ymlCount++
			data, err := os.ReadFile(filepath.Join(wfDir, e.Name()))
			if err != nil {
				continue
			}
			if strings.Contains(string(data), "optivem/actions") {
				actionsFound = true
				break
			}
		}
		if ymlCount == 0 {
			log.Warn("Safety check: no workflow files found (templates may be missing from starter)")
		} else if !actionsFound {
			log.Fatalf("Safety check failed: optivem/actions references were corrupted in %s!", repoDir)
		} else {
			log.OKf("Safety check passed: optivem/actions references intact in %s", repoDir)
		}
	}

	lowercaseDockerComposeImages(repoDir)
}

func lowercaseDockerComposeImages(repoDir string) {
	filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if files.IsGitDir(path) {
			return nil
		}
		if !strings.Contains(info.Name(), "docker-compose") || !strings.HasSuffix(info.Name(), ".yml") {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		lines := strings.Split(string(data), "\n")
		changed := false
		for i, line := range lines {
			if strings.Contains(line, "image:") && strings.Contains(line, "ghcr.io") {
				idx := strings.Index(line, "image:")
				prefix := line[:idx+6]
				rest := line[idx+6:]
				lowered := prefix + strings.ToLower(rest)
				if lowered != lines[i] {
					lines[i] = lowered
					changed = true
				}
			}
		}
		if changed {
			os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
		}
		return nil
	})
	log.OK("Docker-compose image URLs lowercased")
}

// ReplaceNamespaces replaces language-specific namespaces.
func ReplaceNamespaces(cfg *config.Config) {
	log.Log("Step 7: Replacing namespaces...")

	if cfg.DryRun {
		log.Log("[DRY RUN] Would replace language-specific namespaces")
		return
	}

	if cfg.Arch == "monolith" {
		if cfg.RepoStrategy == "monorepo" {
			nsForLang(cfg, cfg.Lang, "monolith", cfg.RepoDir)
		} else {
			nsForLang(cfg, cfg.Lang, "monolith", cfg.SystemRepoDir)
		}
		nsForLang(cfg, cfg.TestLang, "systemtest", cfg.RepoDir)
	} else if cfg.RepoStrategy == "monorepo" {
		// Monorepo: all namespaces in the single repo
		nsForLang(cfg, cfg.TestLang, "systemtest", cfg.RepoDir)
		nsForLang(cfg, cfg.BackendLang, "backend", cfg.RepoDir)
		if cfg.FrontendLang == "react" {
			fixupFrontendPackageJSON(cfg)
		}
	} else {
		// Multirepo: namespaces in separate repos
		nsForLang(cfg, cfg.TestLang, "systemtest", cfg.RepoDir)
		nsForLang(cfg, cfg.BackendLang, "backend", cfg.BackendRepoDir)
		if cfg.FrontendLang == "react" {
			fixupFrontendPackageJSON(cfg)
		}
	}

	log.OK("Namespace replacement complete")
}

func nsForLang(cfg *config.Config, lang, component, repoDir string) {
	switch lang {
	case "java":
		nsJava(cfg, component, repoDir)
	case "dotnet":
		nsDotnet(cfg, component, repoDir)
	case "typescript":
		nsTypeScript(cfg, component, repoDir)
	}
}

func nsJava(cfg *config.Config, component, repoDir string) {
	oldFull := cfg.JavaNsOld + "." + component
	newFull := cfg.JavaNsNew + "." + component

	n := files.ReplaceInTree(repoDir, oldFull, newFull, []string{".java", ".gradle", ".gradle.kts", ".xml", ".properties"})
	n += files.ReplaceInTree(repoDir, oldFull, newFull, []string{".yml"})
	log.OKf("Java: replaced %s -> %s (%d files)", oldFull, newFull, n)

	// Also replace escaped-dot variant (used in regex patterns in Java source)
	oldEscaped := strings.ReplaceAll(oldFull, ".", "\\\\.")
	newEscaped := strings.ReplaceAll(newFull, ".", "\\\\.")
	n = files.ReplaceInTree(repoDir, oldEscaped, newEscaped, []string{".java"})
	if n > 0 {
		log.OKf("Java: replaced escaped namespace pattern (%d files)", n)
	}

	oldDirParts := []string{"com", "optivem", "shop"}
	newDirParts := []string{"com", cfg.OwnerLower, cfg.RepoNoHyphens}

	filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() || files.IsGitDir(path) {
			return nil
		}
		check := filepath.Join(path, "com", "optivem", "shop")
		if _, err := os.Stat(check); err == nil {
			files.RenameJavaDirs(path, oldDirParts, newDirParts)
			return filepath.SkipDir
		}
		return nil
	})
	log.OKf("Java: renamed directories com/optivem/shop -> com/%s/%s", cfg.OwnerLower, cfg.RepoNoHyphens)
}

func nsDotnet(cfg *config.Config, component, repoDir string) {
	componentMap := map[string]string{
		"monolith": "Monolith", "backend": "Backend", "systemtest": "SystemTest",
	}
	oldFull := cfg.DotnetNsOld + "." + componentMap[component]
	newFull := cfg.DotnetNsNew + "." + componentMap[component]

	n := files.ReplaceInTree(repoDir, oldFull, newFull, []string{".cs", ".cshtml", ".csproj", ".sln", ".slnx", ".json", ".yml"})
	n += files.ReplaceInDockerfiles(repoDir, oldFull, newFull)
	log.OKf(".NET: replaced %s -> %s (%d files)", oldFull, newFull, n)

	files.RenameDotnetFiles(repoDir, oldFull, newFull)
	log.OKf(".NET: renamed files %s.* -> %s.*", oldFull, newFull)
}

func nsTypeScript(cfg *config.Config, component, repoDir string) {
	if component != "systemtest" {
		return
	}

	n := files.ReplaceInTree(repoDir, cfg.TsPkgOld, cfg.TsPkgNew, []string{".json"})
	log.OKf("TypeScript: replaced %s -> %s (%d files)", cfg.TsPkgOld, cfg.TsPkgNew, n)

	// Update package.json metadata in system-test
	filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.Contains(path, "system-test") && info.Name() == "package.json" {
			files.ReplaceInFile(path, `"author": "Optivem"`, `"author": "`+cfg.Owner+`"`)
			files.ReplaceInFile(path, `"Shop - System Tests"`, `"`+cfg.SystemName+` - System Tests"`)
			files.ReplaceInFile(path, `"optivem"`, `"`+cfg.OwnerLower+`"`)
			log.OK("TypeScript: updated package.json metadata")
			return filepath.SkipAll
		}
		return nil
	})

	// Update package.json in system dirs (system/backend)
	filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || files.IsGitDir(path) {
			return nil
		}
		if strings.Contains(path, "system-test") || strings.Contains(path, "node_modules") {
			return nil
		}
		if info.Name() == "package.json" {
			// Monolith system code or multitier backend code
			files.ReplaceInFile(path, `"name": "shop-monolith"`, `"name": "`+cfg.Repo+`-system"`)
			files.ReplaceInFile(path, `"name": "shop-backend"`, `"name": "`+cfg.Repo+`-backend"`)
		}
		return nil
	})
}

// ReplaceSystemName replaces the template system name ("Shop") with the user's system name
// across all source files, file names, and directories.
func ReplaceSystemName(cfg *config.Config) {
	log.Log("Step 8: Replacing system name...")

	if cfg.DryRun {
		log.Logf("[DRY RUN] Would replace Shop -> %s", cfg.SysNamePascalNew)
		return
	}

	// Skip if the system name is "shop" (no change needed)
	if cfg.SysNameCamelNew == "shop" {
		log.OK("System name is 'shop', no replacement needed")
		return
	}

	// Collect all repo dirs to process
	repoDirs := []string{cfg.RepoDir}
	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "multitier" {
			repoDirs = append(repoDirs, cfg.BackendRepoDir, cfg.FrontendRepoDir)
		} else {
			repoDirs = append(repoDirs, cfg.SystemRepoDir)
		}
	}

	for _, repoDir := range repoDirs {
		if repoDir == "" {
			continue
		}
		replaceSystemNameInRepo(cfg, repoDir)
	}

	log.OK("System name replacement complete")

	// Validate no leftover template names remain in any text file.
	validateNoLeftovers(cfg, repoDirs)
}

// validateNoLeftovers checks that the old system name doesn't appear in any text file
// after all replacement passes. This catches missed file extensions or replacement gaps.
func validateNoLeftovers(cfg *config.Config, repoDirs []string) {
	for _, repoDir := range repoDirs {
		if repoDir == "" {
			continue
		}
		// PascalCase "Shop": safe to check with simple substring match since
		// a capital letter mid-word is unambiguous.
		leftover := files.FindInTree(repoDir, cfg.SysNamePascalOld)
		if len(leftover) > 0 {
			log.Warnf("Leftover template name %q found in %d file(s) after replacement:", cfg.SysNamePascalOld, len(leftover))
			for _, f := range leftover {
				log.Warnf("  %s", f)
			}
			log.Fatalf("System name replacement incomplete: %q still present in scaffolded repo.", cfg.SysNamePascalOld)
		}

		// camelCase "shop": use word-boundary-aware search to avoid false positives
		// from words like "eshop", "workshop", etc. We check that "shop" is not
		// preceded by a lowercase letter.
		leftover = files.FindInTreeWordBoundary(repoDir, cfg.SysNameCamelOld)
		if len(leftover) > 0 {
			log.Warnf("Leftover template name %q found in %d file(s) after replacement:", cfg.SysNameCamelOld, len(leftover))
			for _, f := range leftover {
				log.Warnf("  %s", f)
			}
			log.Fatalf("System name replacement incomplete: %q still present in scaffolded repo.", cfg.SysNameCamelOld)
		}
	}
}

// Test config extensions (JSON/YAML that contain system name as config keys).
// These are test configuration files, NOT docker-compose or application config.
var testConfigExts = []string{".json", ".yml", ".yaml"}

func replaceSystemNameInRepo(cfg *config.Config, repoDir string) {
	// IMPORTANT: camelCase system name replacement must NOT touch infrastructure files
	// (docker-compose, application config) because "shop" appears as DB names, service
	// names, etc. that use the repo name, not the system name.
	// PascalCase "Shop" is safe in all text files — it only appears as display names.

	// Pass 1: PascalCase in ALL text files (Shop -> SkyTravel).
	// Safe everywhere: display names, type names, config keys, docs, workflows, HTML.
	n := files.ReplaceInTree(repoDir, cfg.SysNamePascalOld, cfg.SysNamePascalNew, nil)
	log.OKf("System name: PascalCase %s -> %s (%d files)", cfg.SysNamePascalOld, cfg.SysNamePascalNew, n)

	// Pass 3: "shop" in Java source files -> camelCase (shopUiBaseUrl -> skyTravelUiBaseUrl)
	n = files.ReplaceInTree(repoDir, cfg.SysNameCamelOld, cfg.SysNameCamelNew, []string{".java"})
	log.OKf("System name: Java camel %s -> %s (%d files)", cfg.SysNameCamelOld, cfg.SysNameCamelNew, n)

	// Pass 4: "shop" in Java build files -> lowercase (package paths)
	n = files.ReplaceInTree(repoDir, cfg.SysNameCamelOld, cfg.SysNameLowerNew, []string{".gradle", ".gradle.kts", ".xml", ".properties"})
	log.OKf("System name: Java build %s -> %s (%d files)", cfg.SysNameCamelOld, cfg.SysNameLowerNew, n)

	// Pass 5: "shop" in .NET files -> camelCase (config keys, identifiers)
	n = files.ReplaceInTree(repoDir, cfg.SysNameCamelOld, cfg.SysNameCamelNew, []string{".cs", ".csproj", ".sln", ".slnx"})
	log.OKf("System name: .NET %s -> %s (%d files)", cfg.SysNameCamelOld, cfg.SysNameCamelNew, n)

	// Pass 6: "shop" in .NET test config (appsettings) -> camelCase
	n = replaceInTestConfigs(repoDir, cfg.SysNameCamelOld, cfg.SysNameCamelNew)
	log.OKf("System name: test config keys %s -> %s (%d files)", cfg.SysNameCamelOld, cfg.SysNameCamelNew, n)

	// Pass 7a: "shop-" in TS/JS files -> kebab-case prefix (import paths, filenames: shop-api-driver -> sky-travel-api-driver)
	// Must run BEFORE 7b so that "shop-" in kebab contexts is consumed first.
	tsExts := []string{".ts", ".tsx", ".js", ".jsx", ".ps1"}
	n = files.ReplaceInTree(repoDir, cfg.SysNameCamelOld+"-", cfg.SysNameKebabNew+"-", tsExts)
	log.OKf("System name: TS kebab prefix %s- -> %s- (%d files)", cfg.SysNameCamelOld, cfg.SysNameKebabNew, n)

	// Pass 7b: remaining "shop" in TS/JS/PS1 files -> camelCase (identifiers: shopDriver -> skyTravelDriver, .shop() -> .skyTravel())
	n = files.ReplaceInTree(repoDir, cfg.SysNameCamelOld, cfg.SysNameCamelNew, tsExts)
	log.OKf("System name: TS camel %s -> %s (%d files)", cfg.SysNameCamelOld, cfg.SysNameCamelNew, n)

	// Pass 7c: "shop" in HTML/cshtml files -> kebab-case (routes, URLs)
	n = files.ReplaceInTree(repoDir, cfg.SysNameCamelOld, cfg.SysNameKebabNew, []string{".html", ".cshtml"})
	log.OKf("System name: HTML kebab %s -> %s (%d files)", cfg.SysNameCamelOld, cfg.SysNameKebabNew, n)

	// Pass 8: Rename files (PascalCase: ShopDsl.java -> SkyTravelDsl.java)
	n = files.RenameFilesInTree(repoDir, cfg.SysNamePascalOld, cfg.SysNamePascalNew)
	log.OKf("System name: renamed %d PascalCase files", n)

	// Pass 9: Rename files (kebab-case: shop-api-driver.ts -> sky-travel-api-driver.ts)
	n = files.RenameFilesInTree(repoDir, cfg.SysNameKebabOld, cfg.SysNameKebabNew)
	log.OKf("System name: renamed %d kebab files", n)

	// Pass 10: Rename directories (PascalCase: Shop/ -> SkyTravel/)
	n = files.RenameDirsInTree(repoDir, cfg.SysNamePascalOld, cfg.SysNamePascalNew)
	log.OKf("System name: renamed %d PascalCase directories", n)

	// Pass 10b: Rename TS domain directories (camelCase: shop/ -> skyTravel/).
	// TS uses camelCase folder names to match identifier casing in imports.
	// Must run BEFORE Pass 11 so these dirs aren't lowercased.
	n = files.RenameDirsInSubtree(repoDir, "system-test", cfg.SysNameLowerOld, cfg.SysNameCamelNew)
	log.OKf("System name: renamed %d TS camelCase directories", n)

	// Pass 11: Rename directories (lowercase: shop/ -> skytravel/ for Java package paths)
	n = files.RenameDirsInTree(repoDir, cfg.SysNameLowerOld, cfg.SysNameLowerNew)
	log.OKf("System name: renamed %d lowercase directories", n)
}

// replaceInTestConfigs replaces in JSON/YAML files that are test configs,
// skipping docker-compose, application config, appsettings, and workflow files.
func replaceInTestConfigs(repoDir, old, new string) int {
	count := 0
	filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || files.IsGitDir(path) {
			return nil
		}
		name := info.Name()
		// Skip infrastructure files
		if strings.Contains(name, "docker-compose") {
			return nil
		}
		if strings.HasPrefix(name, "application") {
			return nil
		}
		if strings.HasPrefix(name, "appsettings") {
			return nil
		}
		if strings.Contains(path, ".github") {
			return nil
		}
		if name == "package.json" || name == "package-lock.json" {
			return nil
		}
		// Only process JSON/YAML test config files
		isConfig := false
		for _, ext := range testConfigExts {
			if strings.HasSuffix(name, ext) {
				isConfig = true
				break
			}
		}
		if !isConfig {
			return nil
		}
		if files.ReplaceInFile(path, old, new) {
			count++
		}
		return nil
	})
	return count
}

// replaceInTestAppsettings replaces in appsettings files under system-test/ directories.
// These contain test config keys (e.g. "Shop": {...}) that need renaming.
// System-level appsettings (DB credentials) are not under system-test/ and are unaffected.
func replaceInTestAppsettings(repoDir, old, new string) int {
	count := 0
	systemTestDir := filepath.Join(repoDir, "system-test")

	filepath.Walk(systemTestDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || files.IsGitDir(path) {
			return nil
		}
		if !strings.HasPrefix(info.Name(), "appsettings") {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".json") {
			return nil
		}
		if files.ReplaceInFile(path, old, new) {
			count++
		}
		return nil
	})
	return count
}

func fixupFrontendPackageJSON(cfg *config.Config) {
	// For multirepo the frontend is a separate repo; for monorepo it's a subdirectory.
	frontendDir := cfg.FrontendRepoDir
	if frontendDir == "" {
		frontendDir = filepath.Join(cfg.RepoDir, "frontend")
	}

	// The package name starts as "optivem-shop-frontend" in the starter template.
	// By the time this runs (Step 7), the repo reference pass (Step 6) has already
	// replaced "optivem" with the owner name, so the current value is
	// "<owner>-shop-frontend" (e.g. "valentinajemuovic-shop-frontend").
	// We match both the original and post-replacement forms to be safe.
	oldNames := []string{
		`"name": "` + cfg.OwnerLower + `-shop-frontend"`,
		`"name": "optivem-shop-frontend"`,
		`"name": "frontend-react"`,
	}
	newName := `"name": "` + cfg.Repo + `-frontend"`

	for _, target := range []string{"package.json", "package-lock.json"} {
		p := filepath.Join(frontendDir, target)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		for _, old := range oldNames {
			files.ReplaceInFile(p, old, newName)
		}
	}
}
