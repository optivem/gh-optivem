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
		if err != nil || info.IsDir() {
			return nil
		}
		if IsGitDir(path) {
			return nil
		}
		if extensions != nil {
			matched := false
			for _, ext := range extensions {
				if strings.HasSuffix(info.Name(), ext) {
					matched = true
					break
				}
			}
			if !matched {
				return nil
			}
		} else if IsBinaryFile(info.Name()) {
			return nil
		}
		if ReplaceInFile(path, old, new) {
			count++
		}
		return nil
	})
	return count
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

// FindInTreeWordBoundary returns paths of non-binary text files under root that contain
// the given needle where it is NOT preceded by a lowercase letter. This avoids false
// positives like "eshop" or "workshop" when searching for "shop".
func FindInTreeWordBoundary(root, needle string) []string {
	var matches []string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || IsGitDir(path) || IsBinaryFile(info.Name()) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := string(data)
		idx := 0
		for {
			pos := strings.Index(content[idx:], needle)
			if pos < 0 {
				break
			}
			absPos := idx + pos
			// Check that the character before the match is not a lowercase letter.
			if absPos == 0 || !isLowerLetter(content[absPos-1]) {
				rel, _ := filepath.Rel(root, path)
				matches = append(matches, rel)
				break
			}
			idx = absPos + len(needle)
		}
		return nil
	})
	return matches
}

func isLowerLetter(b byte) bool {
	return b >= 'a' && b <= 'z'
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

// RenameJavaDirs renames Java package directories: com/optivem/shop -> com/owner/repo.
func RenameJavaDirs(root string, oldParts, newParts []string) {
	oldPath := filepath.Join(oldParts...)
	newPath := filepath.Join(newParts...)

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if strings.Contains(filepath.ToSlash(rel), filepath.ToSlash(oldPath)) {
			newDir := strings.Replace(path, oldPath, newPath, 1)
			os.MkdirAll(filepath.Dir(newDir), 0755)
			if _, err := os.Stat(newDir); os.IsNotExist(err) {
				os.Rename(path, newDir)
			}
			return filepath.SkipDir
		}
		return nil
	})

	// Clean up empty old directories
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		parts := strings.Split(filepath.ToSlash(path), "/")
		for _, p := range parts {
			if p == oldParts[1] {
				entries, _ := os.ReadDir(path)
				if len(entries) == 0 {
					os.Remove(path)
				}
				break
			}
		}
		return nil
	})
}

// RenameDotnetFiles renames .NET files: Optivem.Starter.X.csproj -> NewNs.X.csproj etc.
func RenameDotnetFiles(root, oldPrefix, newPrefix string) {
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.Contains(info.Name(), oldPrefix) {
			newName := strings.Replace(info.Name(), oldPrefix, newPrefix, 1)
			newPath := filepath.Join(filepath.Dir(path), newName)
			os.Rename(path, newPath)
		}
		return nil
	})
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
var skipDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	".claude":      true,
	"dist":         true,
	"build":        true,
	"target":       true,
	"bin":          true,
	"obj":          true,
}

// TODO: When ATDD support is added, generate project-specific CLAUDE.md files
// for scaffolded repos instead of skipping them.
var skipFiles = map[string]bool{
	"CLAUDE.md": true,
}

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
