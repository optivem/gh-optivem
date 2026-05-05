// Package files provides file manipulation helpers: replace, rename, walk.
package files

import (
	"os"
	"path/filepath"
	"strings"
)

// binaryExts lists file extensions that should never be treated as text.
var binaryExts = map[string]bool{
	// Images
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".ico": true,
	".svg": true, ".webp": true, ".bmp": true, ".tiff": true,
	// Fonts
	".woff": true, ".woff2": true, ".ttf": true, ".eot": true, ".otf": true,
	// Archives / compiled
	".zip": true, ".tar": true, ".gz": true, ".jar": true, ".war": true,
	".dll": true, ".exe": true, ".so": true, ".dylib": true, ".class": true,
	".wasm": true, ".pyc": true, ".pdb": true,
	// Media
	".mp3": true, ".mp4": true, ".wav": true, ".avi": true, ".mov": true,
	// Lock files / generated
	".lock": true,
	// Documents
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
}

// IsBinaryFile returns true if the file has a known binary extension.
func IsBinaryFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return binaryExts[ext]
}

// IsGitDir returns true if the path contains a .git component.
func IsGitDir(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".git" {
			return true
		}
	}
	return false
}

// ReplaceInFile replaces all occurrences of old with new in a file.
func ReplaceInFile(path, old, new string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	content := string(data)
	if !strings.Contains(content, old) {
		return false
	}
	result := strings.ReplaceAll(content, old, new)
	os.WriteFile(path, []byte(result), 0644)
	return true
}

// ReplaceInTree replaces in files under root filtered by extension.
// If extensions is nil, it replaces in all non-binary text files.
func ReplaceInTree(root, old, new string, extensions []string) int {
	count := 0
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || IsGitDir(path) {
			return nil
		}
		if !shouldProcessForExts(info.Name(), extensions) {
			return nil
		}
		if ReplaceInFile(path, old, new) {
			count++
		}
		return nil
	})
	return count
}

// shouldProcessForExts reports whether a file should be processed by ReplaceInTree
// given its extension filter. A nil filter accepts any non-binary text file.
func shouldProcessForExts(name string, extensions []string) bool {
	if extensions == nil {
		return !IsBinaryFile(name)
	}
	for _, ext := range extensions {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

// FindInTree returns paths of non-binary text files under root that contain the given string.
func FindInTree(root, needle string) []string {
	var matches []string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || IsGitDir(path) || IsBinaryFile(info.Name()) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if strings.Contains(string(data), needle) {
			rel, _ := filepath.Rel(root, path)
			matches = append(matches, rel)
		}
		return nil
	})
	return matches
}

// ReplaceInDockerfiles replaces in all Dockerfile files under root.
func ReplaceInDockerfiles(root, old, new string) int {
	count := 0
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if IsGitDir(path) {
			return nil
		}
		if info.Name() == "Dockerfile" {
			if ReplaceInFile(path, old, new) {
				count++
			}
		}
		return nil
	})
	return count
}

// RenameFilesInTree renames files containing old in their name to new.
func RenameFilesInTree(root, old, new string) int {
	count := 0
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || IsGitDir(path) {
			return nil
		}
		if strings.Contains(info.Name(), old) {
			newName := strings.ReplaceAll(info.Name(), old, new)
			newPath := filepath.Join(filepath.Dir(path), newName)
			os.Rename(path, newPath)
			count++
		}
		return nil
	})
	return count
}

// RenameDirsInTree renames directories containing old in their name to new.
// Walks bottom-up to avoid renaming parent before child.
func RenameDirsInTree(root, old, new string) int {
	return renameDirs(root, old, new, nil)
}

// RenameDirsInSubtree is like RenameDirsInTree but only renames directories
// whose path contains subtreeMarker (e.g. filepath.Join("system-test", "typescript")).
// Used to apply language-specific casing rules to domain folders.
func RenameDirsInSubtree(root, subtreeMarker, old, new string) int {
	return renameDirs(root, old, new, &subtreeMarker)
}

func renameDirs(root, old, new string, subtreeMarker *string) int {
	var dirs []string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() || IsGitDir(path) {
			return nil
		}
		if subtreeMarker != nil && !strings.Contains(path, *subtreeMarker) {
			return nil
		}
		if strings.Contains(info.Name(), old) {
			dirs = append(dirs, path)
		}
		return nil
	})
	count := 0
	for i := len(dirs) - 1; i >= 0; i-- {
		dir := dirs[i]
		newName := strings.ReplaceAll(filepath.Base(dir), old, new)
		newPath := filepath.Join(filepath.Dir(dir), newName)
		if err := os.Rename(dir, newPath); err == nil {
			count++
		}
	}
	return count
}

// CopyDir recursively copies a directory tree.
// skipDirs are directories that should never be copied (build artifacts, caches).
//
// Note: `.claude/` is NOT skipped — `gh optivem atdd install` writes managed
// agents/commands directly into the consumer's `.claude/`, and any
// student-authored `.claude/` content the source happens to include is
// preserved by passing through verbatim.
var skipDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	"dist":         true,
	"build":        true,
	"target":       true,
	"bin":          true,
	"obj":          true,
}

// TODO: When ATDD support is added, generate project-specific CLAUDE.md files
// for scaffolded repos instead of skipping them.
var skipFiles = map[string]bool{}

func CopyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() && skipDirs[info.Name()] {
			return filepath.SkipDir
		}

		if !info.IsDir() && skipFiles[info.Name()] {
			return nil
		}

		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

// CopyFile copies a single file.
func CopyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// EnsureGitignoreLine appends entry as a line in <repoPath>/.gitignore
// when no equivalent line is already present. Equivalence is loose:
// matches with or without a leading "/" and with or without a trailing
// "/" all count as already-ignoring (so `.gh-optivem`, `.gh-optivem/`,
// `/.gh-optivem`, and `/.gh-optivem/` are all considered the same as
// the canonical `.gh-optivem/`). Comments and blank lines are skipped.
//
// Creates the file when missing. The append always lands on its own
// line — a trailing newline is added before the entry if the existing
// file doesn't end with one, so we never glue our entry onto the
// previous line.
//
// Idempotent: safe to call from both first-time setup (`gh optivem
// config init`) and every driver run (the upgrade path for repos that
// pre-date this guardrail). The double call site is the cost of
// supporting both fresh and pre-existing consumer repos in one PR.
func EnsureGitignoreLine(repoPath, entry string) error {
	path := filepath.Join(repoPath, ".gitignore")
	canonical := normalizeGitignoreEntry(entry)

	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	body := string(data)
	for line := range strings.SplitSeq(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if normalizeGitignoreEntry(trimmed) == canonical {
			return nil
		}
	}

	var b strings.Builder
	b.WriteString(body)
	if body != "" && !strings.HasSuffix(body, "\n") {
		b.WriteString("\n")
	}
	b.WriteString(entry)
	b.WriteString("\n")
	return os.WriteFile(path, []byte(b.String()), 0644)
}

// normalizeGitignoreEntry strips leading "/" and trailing "/" from a
// gitignore line so EnsureGitignoreLine can compare entries that mean
// the same thing but are spelled differently.
func normalizeGitignoreEntry(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "/")
	s = strings.TrimSuffix(s, "/")
	return s
}
