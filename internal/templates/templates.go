// Package templates provides template helpers: copy workflows, docker-compose selection, fixups.
package templates

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/optivem/gh-optivem/internal/files"
	"github.com/optivem/gh-optivem/internal/log"
)

// CopyWorkflows copies workflow files from starter to repo, renaming them.
// mappings is a map of source filename -> destination filename.
func CopyWorkflows(mappings map[string]string, starter, repoDir string) {
	wfSrc := filepath.Join(starter, ".github", "workflows")
	wfDst := filepath.Join(repoDir, ".github", "workflows")
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

// SelectDockerCompose keeps the chosen variant and removes the other.
// variant: "single" for monolith, "multi" for multitier.
func SelectDockerCompose(testDst, variant string) {
	remove := "multitier"
	if variant != "single" {
		remove = "monolith"
	}
	for _, prefix := range []string{"local", "pipeline"} {
		for _, suffix := range []string{"real", "stub"} {
			path := filepath.Join(testDst, "docker-compose."+prefix+"."+remove+"."+suffix+".yml")
			os.Remove(path)
		}
	}
}

// CopyVersion copies the VERSION file from starter to repo.
func CopyVersion(starter, repoDir string) {
	src := filepath.Join(starter, "VERSION")
	if _, err := os.Stat(src); err == nil {
		files.CopyFile(src, filepath.Join(repoDir, "VERSION"))
	}
}

// FixupMultirepoImageURLs replaces image URLs in system workflows for multi-repo setup.
// In multirepo, images are pulled from component repos, not the root repo.
func FixupMultirepoImageURLs(repoDir, frontendRepo, backendRepo string) {
	wfDir := filepath.Join(repoDir, ".github", "workflows")
	if _, err := os.Stat(wfDir); err != nil {
		return
	}

	oldFrontend := "${{ github.event.repository.name }}/frontend"
	newFrontend := frontendRepo + "/frontend"
	oldBackend := "${{ github.event.repository.name }}/backend"
	newBackend := backendRepo + "/backend"

	entries, _ := os.ReadDir(wfDir)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".yml") {
			continue
		}
		path := filepath.Join(wfDir, e.Name())
		files.ReplaceInFile(path, oldFrontend, newFrontend)
		files.ReplaceInFile(path, oldBackend, newBackend)
	}
}

// FixupMonolithMultirepoImageURLs replaces image URLs in root repo workflows for monolith multi-repo.
func FixupMonolithMultirepoImageURLs(repoDir, systemRepo string) {
	wfDir := filepath.Join(repoDir, ".github", "workflows")
	if _, err := os.Stat(wfDir); err != nil {
		return
	}

	oldSystem := "${{ github.event.repository.name }}/system"
	newSystem := systemRepo + "/system"

	entries, _ := os.ReadDir(wfDir)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".yml") {
			continue
		}
		path := filepath.Join(wfDir, e.Name())
		files.ReplaceInFile(path, oldSystem, newSystem)
	}
}

// FixupMultirepoToken replaces GITHUB_TOKEN with GHCR_TOKEN in acceptance/prod stage workflows.
func FixupMultirepoToken(repoDir string) {
	wfDir := filepath.Join(repoDir, ".github", "workflows")
	if _, err := os.Stat(wfDir); err != nil {
		return
	}

	entries, _ := os.ReadDir(wfDir)
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".yml") {
			continue
		}
		if !strings.Contains(name, "acceptance-stage") && !strings.Contains(name, "prod-stage") {
			continue
		}
		path := filepath.Join(wfDir, name)
		files.ReplaceInFile(path,
			"GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}",
			"GITHUB_TOKEN: ${{ secrets.GHCR_TOKEN }}")
	}
}

// FixupMultirepoDockerCompose replaces root repo name with component repo names in docker-compose.
func FixupMultirepoDockerCompose(repoDir, repoName, frontendRepo, backendRepo string) {
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
		files.ReplaceInFile(path,
			repoName+"/frontend",
			frontendRepo+"/frontend")
		files.ReplaceInFile(path,
			repoName+"/backend",
			backendRepo+"/backend")
		return nil
	})
}

// FixupMonolithMultirepoDockerCompose replaces root repo name with system repo name in docker-compose.
func FixupMonolithMultirepoDockerCompose(repoDir, repoName, systemRepo string) {
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
		files.ReplaceInFile(path,
			repoName+"/system",
			systemRepo+"/system")
		return nil
	})
}

// FixupWorkflowContent replaces starter-specific paths and image names inside all workflow files.
// replacements is a list of old -> new pairs to apply.
func FixupWorkflowContent(repoDir string, replacements [][2]string) {
	wfDir := filepath.Join(repoDir, ".github", "workflows")
	entries, err := os.ReadDir(wfDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".yml" {
			continue
		}
		path := filepath.Join(wfDir, e.Name())
		for _, r := range replacements {
			files.ReplaceInFile(path, r[0], r[1])
		}
	}
}

// FixupDockerComposeContent replaces starter-specific paths and image names inside docker-compose files.
func FixupDockerComposeContent(repoDir string, replacements [][2]string) {
	filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || files.IsGitDir(path) {
			return nil
		}
		if !strings.Contains(info.Name(), "docker-compose") || !strings.HasSuffix(info.Name(), ".yml") {
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
		".yml", ".yaml", ".gradle", ".gradle.kts",
		".csproj", ".sln", ".slnx", ".json",
		".xml", ".properties", ".cfg", ".txt",
	}
	for _, r := range replacements {
		files.ReplaceInTree(repoDir, r[0], r[1], textExts)
	}
}

