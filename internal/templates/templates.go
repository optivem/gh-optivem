// Package templates provides template helpers: copy workflows, docker-compose selection, fixups.
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
		files.CopyFile(src, filepath.Join(wfDst, dstName))
	}
}

// SelectDockerCompose flattens shop's arch-subdir layout into a single-arch
// scaffolded layout. Shop organizes system-test content as:
//
//	system-test/<lang>/
//	├── Run-SystemTests.ps1             (arch-agnostic)
//	├── Run-SystemTests.*.Config.ps1    (arch-agnostic)
//	├── README.md                       (arch-agnostic, kept)
//	├── monolith/                       ← compose files, arch config, per-arch README
//	└── multitier/                      ← same
//
// A scaffolded repo locks in one arch, so this function:
//  1. Removes the non-selected arch's entire subdirectory.
//  2. Drops the selected arch's README.md (its "../Run-SystemTests.ps1 -Architecture X"
//     examples don't fit a scaffolded repo — the top-level arch-agnostic README is kept
//     instead and fixed up later).
//  3. Moves the selected arch's remaining files (compose + arch config PS1) up into
//     system-test/.
//  4. Removes the now-empty selected-arch subdir.
//
// variant: "single" for monolith, "multi" for multitier.
func SelectDockerCompose(testDst, variant string) {
	keep, remove := "monolith", "multitier"
	if variant != "single" {
		keep, remove = "multitier", "monolith"
	}
	os.RemoveAll(filepath.Join(testDst, remove))

	keepDir := filepath.Join(testDst, keep)
	os.Remove(filepath.Join(keepDir, "README.md"))

	entries, err := os.ReadDir(keepDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		src := filepath.Join(keepDir, e.Name())
		dst := filepath.Join(testDst, e.Name())
		os.Rename(src, dst)
	}
	os.Remove(keepDir)
}

// CopyVersion copies the VERSION file from shop to repo.
func CopyVersion(shop, repoDir string) {
	src := filepath.Join(shop, "VERSION")
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

// FixupMultirepoToken replaces GITHUB_TOKEN with GHCR_TOKEN in acceptance/prod stage workflows.
func FixupMultirepoToken(repoDir string) {
	forEachWorkflowYml(repoDir, func(path string) {
		name := filepath.Base(path)
		if !strings.Contains(name, "acceptance-stage") && !strings.Contains(name, "prod-stage") {
			return
		}
		files.ReplaceInFile(path,
			"GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}",
			"GITHUB_TOKEN: ${{ secrets.GHCR_TOKEN }}")
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
