package testselect

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// parseChangedMethods produces one ChangedMethod per (file, method) pair
// where any added/changed line of the diff intersects the method's body
// region. Files outside the driver-adapter convention are dropped.
//
// The diff command is invoked once for the whole repo (no pathspec). We
// filter in Go with each language's AdapterMatch so the regex anchor for
// "this looks like a driver-adapter" can stay in the layout config.
func parseChangedMethods(repoRoot, baseRef string, deps *Deps) ([]ChangedMethod, error) {
	if baseRef == "" {
		baseRef = "HEAD"
	}
	out, err := deps.Git(context.Background(), repoRoot,
		"diff", "--unified=0", "--no-color", baseRef)
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}

	files := parseDiffFiles(out)

	var changed []ChangedMethod
	for _, f := range files {
		lang := languageOf(f.path)
		if lang == "" {
			continue
		}
		lay := layouts[lang]
		if lay == nil {
			continue
		}
		if !lay.AdapterMatch(f.path) {
			continue
		}
		// Read the post-image of the file; we only care about methods that
		// exist in HEAD (a renamed-away method is gone and can't be a
		// candidate).
		body, err := deps.Read(repoRoot, f.path)
		if err != nil {
			// Deleted files leave hunks but no current contents — skip.
			continue
		}
		regions := lay.MethodIndexer(string(body))
		seen := map[string]bool{}
		for _, hunk := range f.addedLines {
			for _, m := range regions {
				if hunk.start <= m.endLine && hunk.end >= m.startLine {
					if !seen[m.name] {
						seen[m.name] = true
						changed = append(changed, ChangedMethod{
							File:   f.path,
							Method: m.name,
							Layer:  inferLayer(f.path),
							Lang:   lang,
						})
					}
				}
			}
		}
		// Whole-file additions (no prior content) report a single hunk
		// covering all lines, so the loop above already catches every
		// method. New files with no diff hunks (rare) are ignored.
	}

	return changed, nil
}

// inferLayer maps a path under `driver/adapter/` (or `Driver.Adapter/`) to
// `"shop"` (default) or `"external"` when the path contains `external` as
// a segment. The path-based heuristic matches all three languages.
func inferLayer(p string) string {
	low := filepath.ToSlash(strings.ToLower(p))
	if strings.Contains(low, "/external/") || strings.Contains(low, "/driver.adapter/external/") {
		return "external"
	}
	return "shop"
}

// languageOf maps a file path's extension to a language code.
func languageOf(p string) string {
	switch filepath.Ext(strings.ToLower(p)) {
	case ".java":
		return "java"
	case ".cs":
		return "dotnet"
	case ".ts":
		return "typescript"
	default:
		return ""
	}
}

type diffFile struct {
	path       string
	addedLines []lineRange
}

type lineRange struct {
	start int // inclusive, 1-based
	end   int // inclusive
}

// parseDiffFiles walks a unified=0 diff and returns one diffFile per
// `+++ b/<path>` group, with the post-image line ranges from each `@@`
// header.
func parseDiffFiles(diff []byte) []diffFile {
	var files []diffFile
	scanner := bufio.NewScanner(bytes.NewReader(diff))
	scanner.Buffer(make([]byte, 1<<20), 1<<24)
	var current *diffFile
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "+++ ") {
			path := strings.TrimSpace(strings.TrimPrefix(line, "+++ "))
			path = strings.TrimPrefix(path, "b/")
			if path == "/dev/null" {
				current = nil
				continue
			}
			files = append(files, diffFile{path: path})
			current = &files[len(files)-1]
			continue
		}
		if current == nil {
			continue
		}
		if strings.HasPrefix(line, "@@") {
			if r, ok := parseHunkHeader(line); ok {
				current.addedLines = append(current.addedLines, r)
			}
		}
	}
	return files
}

// parseHunkHeader parses `@@ -<old> +<new> @@` and returns the post-image
// line range. `<new>` is `start[,length]` where length defaults to 1.
//
// A length of 0 (pure deletion) yields an empty range — we represent that
// by start=newStart, end=newStart-1 so the intersection check skips it.
func parseHunkHeader(s string) (lineRange, bool) {
	// Find the +X[,Y] segment.
	plus := strings.Index(s, "+")
	if plus < 0 {
		return lineRange{}, false
	}
	rest := s[plus+1:]
	end := strings.IndexAny(rest, " @")
	if end < 0 {
		return lineRange{}, false
	}
	spec := rest[:end]
	startStr := spec
	lengthStr := "1"
	if comma := strings.Index(spec, ","); comma >= 0 {
		startStr = spec[:comma]
		lengthStr = spec[comma+1:]
	}
	start, err1 := strconv.Atoi(startStr)
	length, err2 := strconv.Atoi(lengthStr)
	if err1 != nil || err2 != nil {
		return lineRange{}, false
	}
	if length == 0 {
		return lineRange{start: start, end: start - 1}, true
	}
	return lineRange{start: start, end: start + length - 1}, true
}
