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

	log.OK("Repository reference replacement complete")
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
}

func replaceSystemNameInRepo(cfg *config.Config, repoDir string) {
	// For the template name "Shop", camelCase/kebab/lowercase are all "shop".
	// We must replace in the right order and use context-aware filtering.
	//
	// Key insight: "shop" in Java files means camelCase (identifiers like shopUiBaseUrl),
	// NOT lowercase (that's only for directory/package paths, handled by dir rename).

	// Pass 1: PascalCase content replacement (all text files)
	n := files.ReplaceInTree(repoDir, cfg.SysNamePascalOld, cfg.SysNamePascalNew, textExts)
	log.OKf("System name: PascalCase %s -> %s (%d files)", cfg.SysNamePascalOld, cfg.SysNamePascalNew, n)

	// Pass 2: "shop" in Java source files -> camelCase (e.g. shopUiBaseUrl -> skyTravelUiBaseUrl)
	javaExts := []string{".java"}
	n = files.ReplaceInTree(repoDir, cfg.SysNameCamelOld, cfg.SysNameCamelNew, javaExts)
	log.OKf("System name: Java camel %s -> %s (%d files)", cfg.SysNameCamelOld, cfg.SysNameCamelNew, n)

	// Pass 3: "shop" in Java build files -> lowercase (e.g. package paths in gradle/xml)
	javaBuildExts := []string{".gradle", ".gradle.kts", ".xml", ".properties"}
	n = files.ReplaceInTree(repoDir, cfg.SysNameCamelOld, cfg.SysNameLowerNew, javaBuildExts)
	log.OKf("System name: Java build %s -> %s (%d files)", cfg.SysNameCamelOld, cfg.SysNameLowerNew, n)

	// Pass 4: "shop" in .NET config and C# files -> camelCase (e.g. config keys, identifiers)
	dotnetExts := []string{".cs", ".json", ".csproj", ".sln", ".slnx"}
	n = files.ReplaceInTree(repoDir, cfg.SysNameCamelOld, cfg.SysNameCamelNew, dotnetExts)
	log.OKf("System name: .NET %s -> %s (%d files)", cfg.SysNameCamelOld, cfg.SysNameCamelNew, n)

	// Pass 5: "shop" in TS/HTML/YAML files -> kebab-case (e.g. imports, routes, config)
	tsExts := []string{".ts", ".tsx", ".js", ".jsx", ".html", ".cshtml", ".yml", ".yaml"}
	n = files.ReplaceInTree(repoDir, cfg.SysNameCamelOld, cfg.SysNameKebabNew, tsExts)
	log.OKf("System name: TS/HTML kebab %s -> %s (%d files)", cfg.SysNameCamelOld, cfg.SysNameKebabNew, n)

	// Pass 6: "shop" in Dockerfiles -> kebab-case
	n = files.ReplaceInDockerfiles(repoDir, cfg.SysNameCamelOld, cfg.SysNameKebabNew)
	if n > 0 {
		log.OKf("System name: Dockerfiles %s -> %s (%d files)", cfg.SysNameCamelOld, cfg.SysNameKebabNew, n)
	}

	// Pass 7: Rename files (PascalCase: ShopDsl.java -> SkyTravelDsl.java)
	n = files.RenameFilesInTree(repoDir, cfg.SysNamePascalOld, cfg.SysNamePascalNew)
	log.OKf("System name: renamed %d PascalCase files", n)

	// Pass 8: Rename files (kebab-case: shop-api-driver.ts -> sky-travel-api-driver.ts)
	n = files.RenameFilesInTree(repoDir, cfg.SysNameKebabOld, cfg.SysNameKebabNew)
	log.OKf("System name: renamed %d kebab files", n)

	// Pass 9: Rename directories (PascalCase: Shop/ -> SkyTravel/)
	n = files.RenameDirsInTree(repoDir, cfg.SysNamePascalOld, cfg.SysNamePascalNew)
	log.OKf("System name: renamed %d PascalCase directories", n)

	// Pass 10: Rename directories (lowercase: shop/ -> skytravel/ for Java package paths)
	n = files.RenameDirsInTree(repoDir, cfg.SysNameLowerOld, cfg.SysNameLowerNew)
	log.OKf("System name: renamed %d lowercase directories", n)
}

func fixupFrontendPackageJSON(cfg *config.Config) {
	pkgPath := filepath.Join(cfg.FrontendRepoDir, "package.json")
	if _, err := os.Stat(pkgPath); err == nil {
		files.ReplaceInFile(pkgPath, `"name": "optivem-shop-frontend"`, `"name": "`+cfg.Repo+`-frontend"`)
		files.ReplaceInFile(pkgPath, `"name": "frontend-react"`, `"name": "`+cfg.Repo+`-frontend"`)
	}
}
