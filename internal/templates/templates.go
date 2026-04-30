// Package templates provides template helpers: copy workflows, fixups.
package templates

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/optivem/gh-optivem/internal/files"
	"github.com/optivem/gh-optivem/internal/log"
)

const (
	githubDir           = ".github"
	workflowsDir        = "workflows"
	ymlExt              = ".yml"
	dockerComposePrefix = "docker-compose"
	frontendSuffix      = "/frontend"
	backendSuffix       = "/backend"
	systemSuffix        = "/system"

	ghRepoNameExpr = "${{ github.event.repository.name }}"
)

func workflowDir(repoDir string) string {
	return filepath.Join(repoDir, githubDir, workflowsDir)
}

func forEachWorkflowYml(repoDir string, fn func(path string)) {
	wfDir := workflowDir(repoDir)
	entries, err := os.ReadDir(wfDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ymlExt) {
			continue
		}
		fn(filepath.Join(wfDir, e.Name()))
	}
}

func isDockerComposeYml(name string) bool {
	return strings.Contains(name, dockerComposePrefix) && strings.HasSuffix(name, ymlExt)
}

// CopyWorkflows copies workflow files from shop to repo, renaming them.
// mappings is a map of source filename -> destination filename.
// When the names differ, the top-level `name:` field inside the YAML is also
// rewritten to match the destination basename, so the workflow's display name
// in GitHub Actions matches its filename (e.g. `bump-patch-version-multirepo`
// → `bump-patch-version` when the file is renamed to `bump-patch-version.yml`).
func CopyWorkflows(mappings map[string]string, shop, repoDir string) {
	wfSrc := filepath.Join(shop, githubDir, workflowsDir)
	wfDst := workflowDir(repoDir)
	os.MkdirAll(wfDst, 0755)

	for srcName, dstName := range mappings {
		src := filepath.Join(wfSrc, srcName)
		if _, err := os.Stat(src); err != nil {
			log.Warnf("Workflow not found: %s", srcName)
			continue
		}
		dst := filepath.Join(wfDst, dstName)
		files.CopyFile(src, dst)
		if srcName != dstName {
			rewriteWorkflowName(dst, strings.TrimSuffix(dstName, ymlExt))
		}
	}
}

// rewriteWorkflowName replaces the top-level `name:` line in a workflow YAML.
// Only the first column-0 `name:` line is rewritten; per-job and per-step
// `name:` keys are indented and therefore untouched.
func rewriteWorkflowName(path, newName string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "name:") {
			lines[i] = "name: " + newName
			os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
			return
		}
	}
}

// CopyVersion copies the per-system VERSION file from shop to the scaffolded
// repo's root. Source: `shop/system/<arch>/<lang>/VERSION`. Destination:
// `repoDir/VERSION`. The shop holds one VERSION file per (arch, lang) flavor
// (decoupled from shop's root meta VERSION); scaffolded repos host one
// system, so root VERSION is the system version.
func CopyVersion(shop, repoDir, arch, lang string) {
	src := filepath.Join(shop, "system", arch, lang, "VERSION")
	if _, err := os.Stat(src); err == nil {
		files.CopyFile(src, filepath.Join(repoDir, "VERSION"))
	}
}

// FixupMultirepoImageURLs replaces image URLs in system workflows for multi-repo setup.
// In multirepo, images are pulled from component repos, not the root repo.
func FixupMultirepoImageURLs(repoDir, frontendRepo, backendRepo string) {
	oldFrontend := ghRepoNameExpr + frontendSuffix
	newFrontend := frontendRepo + frontendSuffix
	oldBackend := ghRepoNameExpr + backendSuffix
	newBackend := backendRepo + backendSuffix

	forEachWorkflowYml(repoDir, func(path string) {
		files.ReplaceInFile(path, oldFrontend, newFrontend)
		files.ReplaceInFile(path, oldBackend, newBackend)
	})
}

// FixupMonolithMultirepoImageURLs replaces image URLs in root repo workflows for monolith multi-repo.
func FixupMonolithMultirepoImageURLs(repoDir, systemRepo string) {
	oldSystem := ghRepoNameExpr + systemSuffix
	newSystem := systemRepo + systemSuffix

	forEachWorkflowYml(repoDir, func(path string) {
		files.ReplaceInFile(path, oldSystem, newSystem)
	})
}

// FixupMultirepoToken replaces GITHUB_TOKEN with GHCR_TOKEN in acceptance/qa/prod stage workflows.
// In multirepo, images live in sibling component repos, so the workflow's default GITHUB_TOKEN
// cannot pull them — a PAT (GHCR_TOKEN) with cross-repo `read:packages` is required.
func FixupMultirepoToken(repoDir string) {
	forEachWorkflowYml(repoDir, func(path string) {
		name := filepath.Base(path)
		if !strings.Contains(name, "acceptance-stage") &&
			!strings.Contains(name, "qa-stage") &&
			!strings.Contains(name, "prod-stage") {
			return
		}
		files.ReplaceInFile(path,
			"password: ${{ secrets.GITHUB_TOKEN }}",
			"password: ${{ secrets.GHCR_TOKEN }}")
	})
}

// FixupMultirepoVersionEntries rewrites `read-base-versions` entries in workflow files
// to fetch each component's VERSION cross-repo via the GitHub API instead of from the
// local working tree. In multirepo, the system-level prod-stage runs in the root repo,
// but `backend/VERSION` and `frontend/VERSION` live inside the component repos at the
// same `backend/` / `frontend/` paths (applyMultitierMultirepo copies the component
// source into a `backend/` or `frontend/` subdir of the component repo, mirroring the
// monorepo layout). Adds the cross-repo `repo` field while preserving the path:
//
//	"file": "backend/VERSION"  -> "file": "backend/VERSION", "repo": "<owner>/<backendRepo>"
//	"file": "frontend/VERSION" -> "file": "frontend/VERSION", "repo": "<owner>/<frontendRepo>"
//
// Requires the read-base-versions action @v1 (or later) which understands the optional
// `repo` field. The action's `token` input must be a PAT/app token with cross-repo
// `Contents: read` — the shop template wires it to `secrets.WORKFLOW_TOKEN`, which is
// already defined in scaffolded repos.
func FixupMultirepoVersionEntries(repoDir, owner, frontendRepo, backendRepo string) {
	replacements := [][2]string{
		{`"file": "backend/VERSION"`, `"file": "backend/VERSION", "repo": "` + owner + `/` + backendRepo + `"`},
		{`"file": "frontend/VERSION"`, `"file": "frontend/VERSION", "repo": "` + owner + `/` + frontendRepo + `"`},
	}
	forEachWorkflowYml(repoDir, func(path string) {
		for _, r := range replacements {
			files.ReplaceInFile(path, r[0], r[1])
		}
	})
}

// FixupMultirepoDockerCompose replaces root repo name with component repo names in docker-compose.
func FixupMultirepoDockerCompose(repoDir, repoName, frontendRepo, backendRepo string) {
	filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || files.IsGitDir(path) {
			return nil
		}
		if !isDockerComposeYml(info.Name()) {
			return nil
		}
		files.ReplaceInFile(path, repoName+frontendSuffix, frontendRepo+frontendSuffix)
		files.ReplaceInFile(path, repoName+backendSuffix, backendRepo+backendSuffix)
		return nil
	})
}

// FixupMonolithMultirepoDockerCompose replaces root repo name with system repo name in docker-compose.
func FixupMonolithMultirepoDockerCompose(repoDir, repoName, systemRepo string) {
	filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || files.IsGitDir(path) {
			return nil
		}
		if !isDockerComposeYml(info.Name()) {
			return nil
		}
		files.ReplaceInFile(path, repoName+systemSuffix, systemRepo+systemSuffix)
		return nil
	})
}

// FixupWorkflowContent replaces shop-specific paths and image names inside all workflow files.
// replacements is a list of old -> new pairs to apply.
func FixupWorkflowContent(repoDir string, replacements [][2]string) {
	forEachWorkflowYml(repoDir, func(path string) {
		for _, r := range replacements {
			files.ReplaceInFile(path, r[0], r[1])
		}
	})
}

// FixupDockerComposeContent replaces shop-specific paths and image names inside docker-compose files.
func FixupDockerComposeContent(repoDir string, replacements [][2]string) {
	filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || files.IsGitDir(path) {
			return nil
		}
		if !isDockerComposeYml(info.Name()) {
			return nil
		}
		for _, r := range replacements {
			files.ReplaceInFile(path, r[0], r[1])
		}
		return nil
	})
}

// FixupAllTextFiles applies replacements across all text files in a repo (not just workflows).
// Used for SonarCloud key suffix changes that appear in build files (build.gradle, .csproj, etc.).
func FixupAllTextFiles(repoDir string, replacements [][2]string) {
	textExts := []string{
		ymlExt, ".yaml", ".gradle", ".gradle.kts",
		".csproj", ".sln", ".slnx", ".json",
		".xml", ".properties", ".cfg", ".txt",
	}
	for _, r := range replacements {
		files.ReplaceInTree(repoDir, r[0], r[1], textExts)
	}
}
