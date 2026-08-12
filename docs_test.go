//go:build docs

// Package main's documentation gate.
//
// Tagged `docs` so it stays out of the default `go test ./...` build: this
// tests no production code path, it checks that the Markdown we ship still
// points at files that exist. Run it with
//
//	go test -tags=docs -v .
//
// Scope is deliberately narrow. Whether the *commands* in the README still
// work is answered by executing them — see scripts/readme-steps.sh — not by
// static analysis. This file only defends the links, which execution can
// never cover.
package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// docFiles are the Markdown files whose cited paths must resolve.
var docFiles = []string{
	"README.md",
	"CONTRIBUTING.md",
	filepath.Join("docs", "cli-reference.md"),
}

// readDoc loads a doc file with line endings normalized. Normalizing matters:
// on Windows these files check out CRLF, and the regexps below are written
// against \n. Without this the gate passes on CI and fails on a maintainer's
// machine, which is precisely backwards.
func readDoc(t *testing.T, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(rel)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return strings.ReplaceAll(string(raw), "\r\n", "\n")
}

// linkRe matches Markdown link targets: [text](target).
var linkRe = regexp.MustCompile(`\[[^\]]*\]\(([^)]+)\)`)

// backtickPathRe matches backticked repo-relative paths under the directories
// worth defending. Scoped deliberately: a broad "anything with a slash" match
// would sweep in URLs, shell fragments, and illustrative paths that are not
// supposed to exist on disk.
var backtickPathRe = regexp.MustCompile(
	"`((?:internal|docs|scripts|\\.github)/[A-Za-z0-9_./-]+)`")

// TestDocsPathsExist asserts that repo-relative paths cited in the docs are
// real. Dead links rot silently — a package move leaves every reference to it
// pointing at nothing, and nobody notices until a reader follows one. This
// caught internal/projectconfig -> internal/kernel/projectconfig the first
// time it ran.
func TestDocsPathsExist(t *testing.T) {
	var checked int

	for _, file := range docFiles {
		doc := readDoc(t, file)
		base := filepath.Dir(file)

		var targets []string
		for _, m := range linkRe.FindAllStringSubmatch(doc, -1) {
			targets = append(targets, m[1])
		}
		for _, m := range backtickPathRe.FindAllStringSubmatch(doc, -1) {
			targets = append(targets, m[1])
		}

		// A path cited as [`x`](x) matches both patterns; report it once.
		seen := map[string]bool{}
		for _, target := range targets {
			if skipTarget(target) || seen[target] {
				continue
			}
			seen[target] = true

			// Drop any #fragment; anchors are not checked.
			clean := target
			if i := strings.Index(clean, "#"); i >= 0 {
				clean = clean[:i]
			}
			if clean == "" {
				continue
			}

			checked++
			rel := filepath.Join(base, filepath.FromSlash(clean))
			if _, err := os.Stat(rel); err != nil {
				t.Errorf("%s: cites %q, which does not exist (resolved to %s)",
					file, target, filepath.ToSlash(rel))
			}
		}
	}

	// A regexp that silently stops matching would leave this test green while
	// checking nothing. Fail loudly instead.
	if checked == 0 {
		t.Fatal("no repo-relative paths found in any doc — the extractor is broken, not the docs")
	}
	t.Logf("checked %d cited paths across %d files", checked, len(docFiles))
}

// skipTarget filters link targets that are not repo-relative file paths.
func skipTarget(target string) bool {
	switch {
	case target == "",
		strings.HasPrefix(target, "http://"),
		strings.HasPrefix(target, "https://"),
		strings.HasPrefix(target, "mailto:"),
		strings.HasPrefix(target, "#"),
		strings.HasPrefix(target, "$"),
		strings.Contains(target, "<"),
		strings.Contains(target, "*"):
		return true
	}
	return false
}
