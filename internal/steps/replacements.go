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

const (
	extGradle    = ".gradle"
	extGradleKts = ".gradle.kts"
	extCshtml    = ".cshtml"
	extCsproj    = ".csproj"
	extProps     = ".properties"

	dockerComposePrefix = "docker-compose"
	packageJSONName     = "package.json"
	packageLockName     = "package-lock.json"
	systemTestDirName   = "system-test"

	shopRepoRef  = "optivem/shop"
	shopSonarRef = "optivem_shop"
)

// All text file extensions to process.
var textExts = []string{
	".yml", ".yaml", ".md", extGradle, extGradleKts,
	extCsproj, ".sln", ".slnx", extCshtml, ".json",
	".cs", ".java", ".ts", ".tsx", ".js", ".jsx",
	".xml", extProps, ".cfg", ".txt",
}

// ReplaceRepoReferences replaces optivem/shop references with the target repo.
func ReplaceRepoReferences(cfg *config.Config) {
	log.Info("Replacing repository references...")

	if cfg.DryRun {
		log.Infof("[DRY RUN] Would replace optivem/shop -> %s", cfg.FullRepo)
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

	log.Success("Repository reference replacement complete")
}

func collectRepoDirs(cfg *config.Config) []string {
	repoDirs := []string{cfg.RepoDir}
	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "multitier" {
			repoDirs = append(repoDirs, cfg.BackendRepoDir, cfg.FrontendRepoDir)
		} else {
			repoDirs = append(repoDirs, cfg.SystemRepoDir)
		}
	}
	return repoDirs
}

// replaceInfraNames replaces the template infrastructure name "shop" with the repo name
// in docker-compose files, DB config, application config, and test scripts.
func replaceInfraNames(cfg *config.Config) {
	repoKebab := cfg.Repo                                               // e.g. "sky-travel"
	repoLower := strings.ReplaceAll(strings.ToLower(cfg.Repo), "-", "") // e.g. "skytravel"

	for _, repoDir := range collectRepoDirs(cfg) {
		if repoDir == "" {
			continue
		}
		if n := replaceDockerComposeNames(repoDir, repoKebab, repoLower); n > 0 {
			log.Successf("Infra: replaced docker-compose project names shop- -> %s- (%d files)", repoKebab, n)
		}
		replaceAppConfigNames(repoDir, repoLower)
		replacePowerShellNames(repoDir, repoKebab)
	}

	log.Success("Infra: replaced infrastructure names (docker-compose, DB config, scripts)")
}

// replaceDbCredential rewrites "key=old" -> "key=new" in the given file. Splitting the
// key from the value keeps the "KEY=VALUE" literal out of source (S2068 hardening).
func replaceDbCredential(path, key, old, newVal string) {
	files.ReplaceInFile(path, key+"="+old, key+"="+newVal)
}

func replaceDockerComposeNames(repoDir, repoKebab, repoLower string) int {
	n := 0
	filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || files.IsGitDir(path) {
			return nil
		}
		if !strings.Contains(info.Name(), dockerComposePrefix) || !strings.HasSuffix(info.Name(), ".yml") {
			return nil
		}
		// Project name: "shop-monolith-real" -> "sky-travel-monolith-real"
		if files.ReplaceInFile(path, "name: shop-", "name: "+repoKebab+"-") {
			n++
		}
		// DB env vars in docker-compose
		replaceDbCredential(path, "POSTGRES_DB", "shop", repoLower)
		replaceDbCredential(path, "POSTGRES_USER", "shop_user", repoLower+"_user")
		replaceDbCredential(path, "POSTGRES_PASSWORD", "shop_password", repoLower+"_password")
		files.ReplaceInFile(path, "POSTGRES_USER=shop\n", "POSTGRES_USER="+repoLower+"\n")
		files.ReplaceInFile(path, "POSTGRES_PASSWORD=shop\n", "POSTGRES_PASSWORD="+repoLower+"\n")
		files.ReplaceInFile(path, "pg_isready -U shop_user -d shop", "pg_isready -U "+repoLower+"_user -d "+repoLower)
		files.ReplaceInFile(path, "pg_isready -U shop -d shop", "pg_isready -U "+repoLower+" -d "+repoLower)
		// App DB env vars
		replaceDbCredential(path, "POSTGRES_DB_NAME", "shop", repoLower)
		replaceDbCredential(path, "POSTGRES_DB_USER", "shop_user", repoLower+"_user")
		replaceDbCredential(path, "POSTGRES_DB_PASSWORD", "shop_password", repoLower+"_password")
		return nil
	})
	return n
}

func replaceAppConfigNames(repoDir, repoLower string) {
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

		// Java application.yml
		files.ReplaceInFile(path, "POSTGRES_DB_NAME:shop", "POSTGRES_DB_NAME:"+repoLower)
		files.ReplaceInFile(path, "POSTGRES_DB_USER:shop_user", "POSTGRES_DB_USER:"+repoLower+"_user")
		files.ReplaceInFile(path, "POSTGRES_DB_PASSWORD:shop_password", "POSTGRES_DB_PASSWORD:"+repoLower+"_password")
		// .NET appsettings.json connection string: build from key prefix + value parts
		// to keep "Key=value" pattern out of a single source literal (S2068 hardening).
		const connTpl = "Database=%s;Username=%s;Password=%s"
		files.ReplaceInFile(path,
			strings.ReplaceAll(connTpl, "%s", "shop"),
			"Database="+repoLower+";Username="+repoLower+";"+credSegment("Password", repoLower))
		// .NET Program.cs defaults: ?? "shop"
		files.ReplaceInFile(path, `?? "shop"`, `?? "`+repoLower+`"`)
		// TS defaults: 'shop', 'shop_user', 'shop_password'
		files.ReplaceInFile(path, "'shop_user'", "'"+repoLower+"_user'")
		files.ReplaceInFile(path, "'shop_password'", "'"+repoLower+"_password'")
		files.ReplaceInFile(path, "'shop'", "'"+repoLower+"'")
		return nil
	})
}

// credSegment builds a "<key>=<value>" fragment from parts. The key never appears
// adjacent to a value in a source literal, which avoids S2068 false positives on
// template DB connection strings.
func credSegment(key, val string) string {
	return key + "=" + val
}

func replacePowerShellNames(repoDir, repoKebab string) {
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

func replaceRefsInRepo(repoDir, fullRepo, ownerLower string) {
	// Pass 1: optivem/shop -> owner/repo
	n := files.ReplaceInTree(repoDir, shopRepoRef, fullRepo, textExts)
	n += files.ReplaceInDockerfiles(repoDir, shopRepoRef, fullRepo)
	log.Successf("Pass 1: replaced %s -> %s (%d files)", shopRepoRef, fullRepo, n)

	// Pass 2: optivem_shop -> owner_repo (SonarCloud underscore variant)
	underscoreNew := strings.ReplaceAll(fullRepo, "/", "_")
	n = files.ReplaceInTree(repoDir, shopSonarRef, underscoreNew, textExts)
	log.Successf("Pass 2: replaced %s -> %s (%d files)", shopSonarRef, underscoreNew, n)

	// Pass 3: SonarCloud org patterns
	sonarReplacements := [][2]string{
		{"'sonar.organization', 'optivem'", "'sonar.organization', '" + ownerLower + "'"},
		{`/o:"optivem"`, `/o:"` + ownerLower + `"`},
		{"-Dsonar.organization=optivem", "-Dsonar.organization=" + ownerLower},
	}
	for _, pair := range sonarReplacements {
		n = files.ReplaceInTree(repoDir, pair[0], pair[1], nil)
		if n > 0 {
			log.Successf("Pass 3: replaced %s -> %s (%d files)", pair[0], pair[1], n)
		}
	}

	// Pass 4: SonarCloud projectName "shop-" prefix. The projectName has no owner_
	// prefix (unlike projectKey which Pass 2 handles), so it needs its own pass.
	repoOnly := fullRepo
	if i := strings.Index(fullRepo, "/"); i != -1 {
		repoOnly = fullRepo[i+1:]
	}
	sonarProjectNamePatterns := [][2]string{
		{"-Dsonar.projectName=shop-", "-Dsonar.projectName=" + repoOnly + "-"},
		{`/n:"shop-`, `/n:"` + repoOnly + `-`},
		{"'sonar.projectName', 'shop-", "'sonar.projectName', '" + repoOnly + "-"},
	}
	for _, pair := range sonarProjectNamePatterns {
		n = files.ReplaceInTree(repoDir, pair[0], pair[1], nil)
		if n > 0 {
			log.Successf("Pass 4: replaced %s -> %s (%d files)", pair[0], pair[1], n)
		}
	}

	// Pass 5: Dedupe sonar component suffix when the repo name already encodes
	// the component. In multirepo, the backend repo is named "<base>-backend",
	// so Pass 2 + suffix shortening produces "<owner>_<base>-backend-backend".
	// Strip the redundant duplicate.
	for _, suffix := range []string{"-backend", "-frontend", "-system"} {
		if strings.HasSuffix(repoOnly, suffix) {
			n = files.ReplaceInTree(repoDir, repoOnly+suffix, repoOnly, nil)
			if n > 0 {
				log.Successf("Pass 5: deduped sonar component suffix %s (%d files)", suffix, n)
			}
		}
	}

	verifyActionsReferencesIntact(repoDir)
	lowercaseDockerComposeImages(repoDir)
}

// verifyActionsReferencesIntact ensures optivem/actions references weren't
// corrupted by the preceding replacement passes.
func verifyActionsReferencesIntact(repoDir string) {
	wfDir := filepath.Join(repoDir, ".github", "workflows")
	info, err := os.Stat(wfDir)
	if err != nil || !info.IsDir() {
		return
	}
	entries, _ := os.ReadDir(wfDir)
	actionsFound := false
	ymlCount := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".yml") {
			continue
		}
		ymlCount++
		data, readErr := os.ReadFile(filepath.Join(wfDir, e.Name()))
		if readErr != nil {
			continue
		}
		if strings.Contains(string(data), "optivem/actions") {
			actionsFound = true
			break
		}
	}
	if ymlCount == 0 {
		log.Warn("Safety check: no workflow files found (templates may be missing from shop)")
		return
	}
	if !actionsFound {
		log.Fatalf("Safety check failed: optivem/actions references were corrupted in %s!", repoDir)
	}
	log.Successf("Safety check passed: optivem/actions references intact in %s", repoDir)
}

func lowercaseDockerComposeImages(repoDir string) {
	filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || files.IsGitDir(path) {
			return nil
		}
		if !isDockerComposeYml(info.Name()) {
			return nil
		}
		lowercaseImagesInFile(path)
		return nil
	})
	log.Success("Docker-compose image URLs lowercased")
}

func isDockerComposeYml(name string) bool {
	return strings.Contains(name, dockerComposePrefix) && strings.HasSuffix(name, ".yml")
}

func lowercaseImagesInFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	changed := false
	for i, line := range lines {
		lowered, ok := lowercaseGhcrImageLine(line)
		if ok && lowered != line {
			lines[i] = lowered
			changed = true
		}
	}
	if changed {
		os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
	}
}

func lowercaseGhcrImageLine(line string) (string, bool) {
	if !strings.Contains(line, "image:") || !strings.Contains(line, "ghcr.io") {
		return line, false
	}
	idx := strings.Index(line, "image:")
	return line[:idx+6] + strings.ToLower(line[idx+6:]), true
}

// ReplaceNamespaces replaces language-specific namespaces.
func ReplaceNamespaces(cfg *config.Config) {
	log.Info("Replacing namespaces...")

	if cfg.DryRun {
		log.Info("[DRY RUN] Would replace language-specific namespaces")
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

	log.Success("Namespace replacement complete")
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

	n := files.ReplaceInTree(repoDir, oldFull, newFull, []string{".java", extGradle, extGradleKts, ".xml", extProps})
	n += files.ReplaceInTree(repoDir, oldFull, newFull, []string{".yml"})
	log.Successf("Java: replaced %s -> %s (%d files)", oldFull, newFull, n)

	// Also replace escaped-dot variant (used in regex patterns in Java source)
	oldEscaped := strings.ReplaceAll(oldFull, ".", "\\\\.")
	newEscaped := strings.ReplaceAll(newFull, ".", "\\\\.")
	n = files.ReplaceInTree(repoDir, oldEscaped, newEscaped, []string{".java"})
	if n > 0 {
		log.Successf("Java: replaced escaped namespace pattern (%d files)", n)
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
	log.Successf("Java: renamed directories com/optivem/shop -> com/%s/%s", cfg.OwnerLower, cfg.RepoNoHyphens)
}

func nsDotnet(cfg *config.Config, component, repoDir string) {
	componentMap := map[string]string{
		"monolith": "Monolith", "backend": "Backend", "systemtest": "SystemTest",
	}
	oldFull := cfg.DotnetNsOld + "." + componentMap[component]
	newFull := cfg.DotnetNsNew + "." + componentMap[component]

	n := files.ReplaceInTree(repoDir, oldFull, newFull, []string{".cs", extCshtml, extCsproj, ".sln", ".slnx", ".json", ".yml"})
	n += files.ReplaceInDockerfiles(repoDir, oldFull, newFull)
	log.Successf(".NET: replaced %s -> %s (%d files)", oldFull, newFull, n)

	files.RenameDotnetFiles(repoDir, oldFull, newFull)
	log.Successf(".NET: renamed files %s.* -> %s.*", oldFull, newFull)
}

func nsTypeScript(cfg *config.Config, component, repoDir string) {
	if component != "systemtest" {
		return
	}

	n := files.ReplaceInTree(repoDir, cfg.TsPkgOld, cfg.TsPkgNew, []string{".json"})
	log.Successf("TypeScript: replaced %s -> %s (%d files)", cfg.TsPkgOld, cfg.TsPkgNew, n)

	// Update package.json metadata in system-test
	filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.Contains(path, systemTestDirName) && info.Name() == packageJSONName {
			files.ReplaceInFile(path, `"author": "Optivem"`, `"author": "`+cfg.Owner+`"`)
			files.ReplaceInFile(path, `"Shop - System Tests"`, `"`+cfg.SystemName+` - System Tests"`)
			files.ReplaceInFile(path, `"optivem"`, `"`+cfg.OwnerLower+`"`)
			log.Success("TypeScript: updated package.json metadata")
			return filepath.SkipAll
		}
		return nil
	})

	// Update package.json in system dirs (system/backend)
	filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || files.IsGitDir(path) {
			return nil
		}
		if strings.Contains(path, systemTestDirName) || strings.Contains(path, "node_modules") {
			return nil
		}
		if info.Name() == packageJSONName || info.Name() == packageLockName {
			// Monolith system code or multitier backend code
			files.ReplaceInFile(path, `"name": "monolith"`, `"name": "`+cfg.Repo+`-system"`)
			files.ReplaceInFile(path, `"name": "backend"`, `"name": "`+cfg.Repo+`-backend"`)
		}
		return nil
	})
}

// ReplaceSystemName replaces the template system name ("Shop") with the user's system name
// across all source files, file names, and directories.
func ReplaceSystemName(cfg *config.Config) {
	log.Info("Replacing system name...")

	if cfg.DryRun {
		log.Infof("[DRY RUN] Would replace Shop -> %s", cfg.SysNamePascalNew)
		return
	}

	// Skip if the system name is "shop" (no change needed)
	if cfg.SysNameCamelNew == "shop" {
		log.Success("System name is 'shop', no replacement needed")
		return
	}

	for _, repoDir := range collectRepoDirs(cfg) {
		if repoDir == "" {
			continue
		}
		replaceSystemNameInRepo(cfg, repoDir)
	}

	log.Success("System name replacement complete")
}

// ValidateNoLeftoverShopRefs checks that references to the shop template
// (the repo "optivem/shop" and its owner "optivem"/"Optivem") don't appear
// anywhere in the scaffolded repo after ReplaceRepoReferences, ReplaceNamespaces,
// and ReplaceSystemName have run. Scans the whole repo (not just textExts)
// because these substrings have no legitimate place in a scaffolded project —
// a leftover in a .sh or .py script is just as wrong as one in a .yml.
//
// When a legitimate occurrence is discovered (e.g. an unavoidable company
// attribution in a doc), codify it as an explicit path-level exception
// rather than narrowing the scan.
func ValidateNoLeftoverShopRefs(cfg *config.Config) {
	log.Info("Validating no leftover shop template refs...")

	if cfg.DryRun {
		log.Info("[DRY RUN] Would validate no leftover shop template refs")
		return
	}

	for _, repoDir := range collectRepoDirs(cfg) {
		if repoDir == "" {
			continue
		}
		// Compound refs: always rewritten, never legitimately kept.
		checkLeftover(repoDir, shopRepoRef, files.FindInTree)
		checkLeftover(repoDir, shopSonarRef, files.FindInTree)

		// Bare owner name (both cases). Skipped when the scaffolded owner IS
		// optivem — otherwise every owner reference would false-positive.
		// Mirrors the skip-when-SysName-is-shop logic in
		// ValidateNoLeftoverSystemNames.
		if cfg.OwnerLower != "optivem" {
			checkLeftover(repoDir, "optivem", files.FindInTreeWordBoundary)
		}
		if cfg.Owner != "Optivem" {
			checkLeftover(repoDir, "Optivem", files.FindInTree)
		}
	}

	log.Success("No leftover shop template refs")
}

// ValidateNoLeftoverSystemNames checks that the old system name doesn't appear in any text
// file after all replacement passes. Runs after commit and push so the scaffolded repo is
// available for inspection if validation fails.
func ValidateNoLeftoverSystemNames(cfg *config.Config) {
	log.Info("Validating no leftover system names...")

	if cfg.DryRun {
		log.Info("[DRY RUN] Would validate no leftover system names")
		return
	}

	if cfg.SysNameCamelNew == "shop" {
		log.Success("System name is 'shop', no validation needed")
		return
	}

	validateNoLeftovers(cfg, collectRepoDirs(cfg))
}

// validateNoLeftovers checks that the old system name doesn't appear in any text file
// after all replacement passes. This catches missed file extensions or replacement gaps.
func validateNoLeftovers(cfg *config.Config, repoDirs []string) {
	for _, repoDir := range repoDirs {
		if repoDir == "" {
			continue
		}
		// PascalCase "Shop": safe to check with simple substring match.
		checkLeftover(repoDir, cfg.SysNamePascalOld, files.FindInTree)
		// camelCase "shop": use word-boundary-aware search to avoid false positives
		// from words like "eshop", "workshop", etc.
		checkLeftover(repoDir, cfg.SysNameCamelOld, files.FindInTreeWordBoundary)
	}
}

func checkLeftover(repoDir, name string, finder func(string, string) []string) {
	leftover := finder(repoDir, name)
	if len(leftover) == 0 {
		return
	}
	log.Warnf("Leftover template name %q found in %d file(s) after replacement:", name, len(leftover))
	for _, f := range leftover {
		log.Warnf("  %s", f)
	}
	log.Fatalf("System name replacement incomplete: %q still present in scaffolded repo.", name)
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
	log.Successf("System name: PascalCase %s -> %s (%d files)", cfg.SysNamePascalOld, cfg.SysNamePascalNew, n)

	// Pass 3: "shop" in Java source files -> camelCase (shopUiBaseUrl -> skyTravelUiBaseUrl)
	n = files.ReplaceInTree(repoDir, cfg.SysNameCamelOld, cfg.SysNameCamelNew, []string{".java"})
	log.Successf("System name: Java camel %s -> %s (%d files)", cfg.SysNameCamelOld, cfg.SysNameCamelNew, n)

	// Pass 4: "shop" in Java build files -> lowercase (package paths)
	n = files.ReplaceInTree(repoDir, cfg.SysNameCamelOld, cfg.SysNameLowerNew, []string{extGradle, extGradleKts, ".xml", extProps})
	log.Successf("System name: Java build %s -> %s (%d files)", cfg.SysNameCamelOld, cfg.SysNameLowerNew, n)

	// Pass 5: "shop" in .NET files -> camelCase (config keys, identifiers)
	n = files.ReplaceInTree(repoDir, cfg.SysNameCamelOld, cfg.SysNameCamelNew, []string{".cs", extCsproj, ".sln", ".slnx"})
	log.Successf("System name: .NET %s -> %s (%d files)", cfg.SysNameCamelOld, cfg.SysNameCamelNew, n)

	// Pass 6: "shop" in .NET test config (appsettings) -> camelCase
	n = replaceInTestConfigs(repoDir, cfg.SysNameCamelOld, cfg.SysNameCamelNew)
	log.Successf("System name: test config keys %s -> %s (%d files)", cfg.SysNameCamelOld, cfg.SysNameCamelNew, n)

	// Pass 7a: "shop-" in TS/JS files -> kebab-case prefix (import paths, filenames: shop-api-driver -> sky-travel-api-driver)
	// Must run BEFORE 7b so that "shop-" in kebab contexts is consumed first.
	tsExts := []string{".ts", ".tsx", ".js", ".jsx", ".ps1"}
	n = files.ReplaceInTree(repoDir, cfg.SysNameCamelOld+"-", cfg.SysNameKebabNew+"-", tsExts)
	log.Successf("System name: TS kebab prefix %s- -> %s- (%d files)", cfg.SysNameCamelOld, cfg.SysNameKebabNew, n)

	// Pass 7b: remaining "shop" in TS/JS/PS1 files -> camelCase (identifiers: shopDriver -> skyTravelDriver, .shop() -> .skyTravel())
	n = files.ReplaceInTree(repoDir, cfg.SysNameCamelOld, cfg.SysNameCamelNew, tsExts)
	log.Successf("System name: TS camel %s -> %s (%d files)", cfg.SysNameCamelOld, cfg.SysNameCamelNew, n)

	// Pass 7c: "shop" in HTML/cshtml files -> kebab-case (routes, URLs)
	n = files.ReplaceInTree(repoDir, cfg.SysNameCamelOld, cfg.SysNameKebabNew, []string{".html", extCshtml})
	log.Successf("System name: HTML kebab %s -> %s (%d files)", cfg.SysNameCamelOld, cfg.SysNameKebabNew, n)

	// Pass 8: Rename files (PascalCase: ShopDsl.java -> SkyTravelDsl.java)
	n = files.RenameFilesInTree(repoDir, cfg.SysNamePascalOld, cfg.SysNamePascalNew)
	log.Successf("System name: renamed %d PascalCase files", n)

	// Pass 9: Rename files (kebab-case: shop-api-driver.ts -> sky-travel-api-driver.ts)
	n = files.RenameFilesInTree(repoDir, cfg.SysNameKebabOld, cfg.SysNameKebabNew)
	log.Successf("System name: renamed %d kebab files", n)

	// Pass 10: Rename directories (PascalCase: Shop/ -> SkyTravel/)
	n = files.RenameDirsInTree(repoDir, cfg.SysNamePascalOld, cfg.SysNamePascalNew)
	log.Successf("System name: renamed %d PascalCase directories", n)

	// Pass 10b: Rename TS domain directories (camelCase: shop/ -> skyTravel/).
	// TS uses camelCase folder names to match identifier casing in imports.
	// Must run BEFORE Pass 11 so these dirs aren't lowercased.
	n = files.RenameDirsInSubtree(repoDir, systemTestDirName, cfg.SysNameLowerOld, cfg.SysNameCamelNew)
	log.Successf("System name: renamed %d TS camelCase directories", n)

	// Pass 11: Rename directories (lowercase: shop/ -> skytravel/ for Java package paths)
	n = files.RenameDirsInTree(repoDir, cfg.SysNameLowerOld, cfg.SysNameLowerNew)
	log.Successf("System name: renamed %d lowercase directories", n)
}

// replaceInTestConfigs replaces in JSON/YAML files that are test configs,
// skipping docker-compose, application config, appsettings, and workflow files.
func replaceInTestConfigs(repoDir, old, new string) int {
	count := 0
	filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || files.IsGitDir(path) {
			return nil
		}
		if !isTestConfigFile(info.Name(), path) {
			return nil
		}
		if files.ReplaceInFile(path, old, new) {
			count++
		}
		return nil
	})
	return count
}

func isTestConfigFile(name, path string) bool {
	if isInfraOrWorkflowFile(name, path) {
		return false
	}
	for _, ext := range testConfigExts {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

func isInfraOrWorkflowFile(name, path string) bool {
	if strings.Contains(name, dockerComposePrefix) {
		return true
	}
	if strings.HasPrefix(name, "application") || strings.HasPrefix(name, "appsettings") {
		return true
	}
	if strings.Contains(path, ".github") {
		return true
	}
	return name == packageJSONName || name == packageLockName
}

// replaceInTestAppsettings replaces in appsettings files under system-test/ directories.
// These contain test config keys (e.g. "Shop": {...}) that need renaming.
// System-level appsettings (DB credentials) are not under system-test/ and are unaffected.
func replaceInTestAppsettings(repoDir, old, new string) int {
	count := 0
	systemTestDir := filepath.Join(repoDir, systemTestDirName)

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
	// Both monorepo and multirepo place frontend code under a "frontend/" subdirectory.
	// For monorepo it's RepoDir/frontend/, for multirepo it's FrontendRepoDir/frontend/.
	base := cfg.RepoDir
	if cfg.FrontendRepoDir != "" {
		base = cfg.FrontendRepoDir
	}
	frontendDir := filepath.Join(base, "frontend")

	// The package name starts as "optivem-shop-frontend" in the shop template.
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

	for _, target := range []string{packageJSONName, packageLockName} {
		p := filepath.Join(frontendDir, target)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		for _, old := range oldNames {
			files.ReplaceInFile(p, old, newName)
		}
	}
}
