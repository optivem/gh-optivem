// Package gitignore provides a helper to idempotently ensure a line is
// present in a repo's .gitignore.
package gitignore

import (
	"os"
	"path/filepath"
	"strings"
)

// EnsureLine appends entry as a line in <repoPath>/.gitignore
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
func EnsureLine(repoPath, entry string) error {
	path := filepath.Join(repoPath, ".gitignore")
	canonical := normalizeEntry(entry)

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
		if normalizeEntry(trimmed) == canonical {
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

// normalizeEntry strips leading "/" and trailing "/" from a
// gitignore line so EnsureLine can compare entries that mean
// the same thing but are spelled differently.
func normalizeEntry(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "/")
	s = strings.TrimSuffix(s, "/")
	return s
}
