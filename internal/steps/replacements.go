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

const dockerComposePrefix = "docker-compose"

// ReplaceRepoReferences rewrites the handful of publisher-real strings that
// must point at the user's GitHub / GHCR / SonarCloud identity in the
// scaffolded repo. These stay in the template verbatim so the template's own
// CI continues to work; the scaffolder rewrites them per user at scaffold
// time.
//
// Everything else — .sln names, Java packages, compose project names, TS
// identifiers — is driven by the MyCompany / MyShop generic placeholders
// handled by ReplaceNamespaces and ReplaceSystemName.
func ReplaceRepoReferences(cfg *config.Config) {
	log.Info("Replacing repository references...")

	rewritePublisherRefs(cfg.RepoDir, cfg.Owner, cfg.FullRepo, cfg.OwnerLower)

	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "multitier" {
			rewritePublisherRefs(cfg.BackendRepoDir, cfg.Owner, cfg.BackendFullRepo, cfg.OwnerLower)
			rewritePublisherRefs(cfg.FrontendRepoDir, cfg.Owner, cfg.FrontendFullRepo, cfg.OwnerLower)
			templates.FixupMultirepoDockerCompose(
				cfg.RepoDir, cfg.Repo, cfg.FrontendRepo, cfg.BackendRepo,
			)
		} else {
			rewritePublisherRefs(cfg.SystemRepoDir, cfg.Owner, cfg.SystemFullRepo, cfg.OwnerLower)
			templates.FixupMonolithMultirepoDockerCompose(
				cfg.RepoDir, cfg.Repo, cfg.SystemRepo,
			)
		}
	}

	log.Success("Repository reference replacement complete")
}

// ReplaceNamespaces swaps the 6 company placeholder forms (MyCompany / myCompany
// / my-company / mycompany / my_company / MY_COMPANY) for the user's owner-name
// casings in file content, filenames, and directory names.
func ReplaceNamespaces(cfg *config.Config) {
	log.Info("Replacing company placeholders...")

	pairs := companyPlaceholderPairs(cfg.OwnerCasings)
	applyToAllRepos(cfg, pairs, "Company")

	log.Success("Company placeholder replacement complete")
}

// ReplaceSystemName swaps the 6 system placeholder forms (MyShop / myShop /
// my-shop / myshop / my_shop / MY_SHOP) for the user's system-name casings in
// file content, filenames, and directory names.
func ReplaceSystemName(cfg *config.Config) {
	log.Info("Replacing system placeholders...")

	pairs := systemPlaceholderPairs(cfg.SysNameCasings)
	applyToAllRepos(cfg, pairs, "System")

	log.Success("System placeholder replacement complete")
}

// ── Placeholder pair tables ────────────────────────────────────────────────

// companyPlaceholderPairs returns the 6 (placeholder, value) pairs for the
// company token. Order within the slice doesn't matter: the 6 forms share no
// common substring with each other or with any system form, so every pass is
// independent.
func companyPlaceholderPairs(c config.Casings) [][2]string {
	return [][2]string{
		{"MyCompany", c.Pascal},
		{"myCompany", c.Camel},
		{"my-company", c.Kebab},
		{"mycompany", c.Lower},
		{"my_company", c.Snake},
		{"MY_COMPANY", c.Screaming},
	}
}

// systemPlaceholderPairs returns the 6 (placeholder, value) pairs for the
// system token.
func systemPlaceholderPairs(c config.Casings) [][2]string {
	return [][2]string{
		{"MyShop", c.Pascal},
		{"myShop", c.Camel},
		{"my-shop", c.Kebab},
		{"myshop", c.Lower},
		{"my_shop", c.Snake},
		{"MY_SHOP", c.Screaming},
	}
}

// ── Generic placeholder application ────────────────────────────────────────

// applyToAllRepos applies the given placeholder pairs to every scaffolded
// repo dir (handles mono/multi-repo strategies). label is used in log output.
func applyToAllRepos(cfg *config.Config, pairs [][2]string, label string) {
	for _, repoDir := range collectRepoDirs(cfg) {
		if repoDir == "" {
			continue
		}
		applyPlaceholderPairs(repoDir, pairs, label)
	}
}

// applyPlaceholderPairs rewrites directories, filenames, and file contents in
// that order for every pair. The three passes are independent (each does its
// own filepath walk), so the order is a readability choice rather than a
// correctness constraint; outside-in matches how a human would rename things
// by hand.
func applyPlaceholderPairs(repoDir string, pairs [][2]string, label string) {
	// 1. Directories (bottom-up inside RenameDirsInTree).
	for _, p := range pairs {
		if n := files.RenameDirsInTree(repoDir, p[0], p[1]); n > 0 {
			log.Successf("%s dirs: %s -> %s (%d renamed)", label, p[0], p[1], n)
		}
	}
	// 2. Filenames.
	for _, p := range pairs {
		if n := files.RenameFilesInTree(repoDir, p[0], p[1]); n > 0 {
			log.Successf("%s files: %s -> %s (%d renamed)", label, p[0], p[1], n)
		}
	}
	// 3. File contents across all non-binary text files and Dockerfiles.
	for _, p := range pairs {
		n := files.ReplaceInTree(repoDir, p[0], p[1], nil)
		n += files.ReplaceInDockerfiles(repoDir, p[0], p[1])
		if n > 0 {
			log.Successf("%s content: %s -> %s (%d files)", label, p[0], p[1], n)
		}
	}
}

// ── Publisher-real hardcoded rewrites ──────────────────────────────────────

// rewritePublisherRefs applies the handful of one-off rewrites that point at
// optivem's real GitHub/GHCR/SonarCloud/npm identity. These are not part of
// the MyCompany/MyShop placeholder system — the template uses real identifiers
// so its own CI works, and the scaffolder projects them onto the user here.
func rewritePublisherRefs(repoDir, owner, fullRepo, ownerLower string) {
	repoOnly := fullRepo
	if i := strings.Index(fullRepo, "/"); i != -1 {
		repoOnly = fullRepo[i+1:]
	}
	underscoreFull := strings.ReplaceAll(fullRepo, "/", "_")

	// 1. optivem/shop -> owner/repo (github.com URLs and ghcr.io image paths).
	n := files.ReplaceInTree(repoDir, "optivem/shop", fullRepo, nil)
	n += files.ReplaceInDockerfiles(repoDir, "optivem/shop", fullRepo)
	log.Successf("Publisher: optivem/shop -> %s (%d files)", fullRepo, n)

	// 2. SonarCloud projectKey underscore form.
	if n := files.ReplaceInTree(repoDir, "optivem_shop", underscoreFull, nil); n > 0 {
		log.Successf("Sonar key: optivem_shop -> %s (%d files)", underscoreFull, n)
	}

	// 3. SonarCloud organization (3 syntactic forms).
	for _, pair := range [][2]string{
		{"'sonar.organization', 'optivem'", "'sonar.organization', '" + ownerLower + "'"},
		{`/o:"optivem"`, `/o:"` + ownerLower + `"`},
		{"-Dsonar.organization=optivem", "-Dsonar.organization=" + ownerLower},
	} {
		files.ReplaceInTree(repoDir, pair[0], pair[1], nil)
	}

	// 4. SonarCloud projectName "shop-" prefix (no owner_ prefix — unlike
	// projectKey, which pass 2 handles).
	for _, pair := range [][2]string{
		{"-Dsonar.projectName=shop-", "-Dsonar.projectName=" + repoOnly + "-"},
		{`/n:"shop-`, `/n:"` + repoOnly + `-`},
		{"'sonar.projectName', 'shop-", "'sonar.projectName', '" + repoOnly + "-"},
	} {
		files.ReplaceInTree(repoDir, pair[0], pair[1], nil)
	}

	// 5. Dedupe sonar component suffix when the multirepo repo name already
	// encodes the component (e.g. "<base>-backend" would otherwise produce
	// "<base>-backend-backend").
	for _, suffix := range []string{"-backend", "-frontend", "-system"} {
		if strings.HasSuffix(repoOnly, suffix) {
			if n := files.ReplaceInTree(repoDir, repoOnly+suffix, repoOnly, nil); n > 0 {
				log.Successf("Sonar suffix: deduped %s (%d files)", suffix, n)
			}
		}
	}

	// 6. Problem-details "type" URI host. example.com is RFC 2606-reserved —
	// stays a safe stub under the user's owner-scoped placeholder.
	if n := files.ReplaceInTree(repoDir, "api.optivem.com", "api."+ownerLower+".example.com", nil); n > 0 {
		log.Successf("Publisher: api.optivem.com -> api.%s.example.com (%d files)", ownerLower, n)
	}

	// 7. npm package name @optivem/shop-system-test (still present in stale
	// package-lock.json files from the pre-rename template state).
	if n := files.ReplaceInTree(repoDir, "@optivem/shop-system-test", "@"+ownerLower+"/"+repoOnly+"-system-test", []string{".json"}); n > 0 {
		log.Successf("Publisher: npm name -> @%s/%s-system-test (%d files)", ownerLower, repoOnly, n)
	}

	// 8. Stale frontend package-lock.json name.
	if n := files.ReplaceInTree(repoDir, "optivem-shop-frontend", repoOnly+"-frontend", []string{".json"}); n > 0 {
		log.Successf("Publisher: frontend pkg name -> %s-frontend (%d files)", repoOnly, n)
	}

	// 9. package.json author field (publisher-real capital-case attribution).
	rewritePackageJSONAuthor(repoDir, owner)

	verifyActionsReferencesIntact(repoDir)
	lowercaseDockerComposeImages(repoDir)
}

// rewritePackageJSONAuthor replaces "author": "Optivem" with the user's owner
// string, case-preserved. Walks only package.json files.
func rewritePackageJSONAuthor(repoDir, owner string) {
	filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || files.IsGitDir(path) {
			return nil
		}
		if info.Name() != "package.json" {
			return nil
		}
		files.ReplaceInFile(path, `"author": "Optivem"`, `"author": "`+owner+`"`)
		return nil
	})
}

// ── Safety + cosmetic passes ───────────────────────────────────────────────

// collectRepoDirs returns every on-disk repo path that participates in this
// scaffold (root + any multirepo companions). Zero-value entries (unused for
// the current strategy) are preserved and filtered by callers.
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

// lowercaseDockerComposeImages normalizes ghcr.io image URLs to lowercase
// (ghcr requires lowercase owner/repo segments, but users often type their
// owner name with mixed case).
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
