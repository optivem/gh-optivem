// Package expand provides the shared ${name} placeholder substitution
// primitive used by both ATDD agent-prompt rendering (clauderun) and
// phase-doc sync materialization (assets/sync). Both call sites use the
// same syntax against a flat name→value map, so the substitution + the
// guardrail that catches missing keys live here as one package.
//
// The package is a leaf — it depends only on stdlib regexp/strings — so
// it can be imported from any layer without creating a cycle.
package expand

import (
	"regexp"
	"strings"
)

// Apply substitutes every ${name} occurrence in s using params. Idempotent
// on already-substituted strings (no ${...} → identity); a nil params map
// returns s unchanged.
func Apply(s string, params map[string]string) string {
	for k, v := range params {
		s = strings.ReplaceAll(s, "${"+k+"}", v)
	}
	return s
}

// unfilledRE matches a syntactically valid ${name} placeholder. The name
// must start with a letter or underscore and contain only word
// characters thereafter, matching the convention Apply substitutes. The
// anchoring is intentional: a substring like `\$amount{}` does not match
// (the `${` is split), which is the correct behaviour because it isn't a
// placeholder.
var unfilledRE = regexp.MustCompile(`\$\{[a-zA-Z_][a-zA-Z0-9_]*\}`)

// FindUnfilled returns each distinct ${name} token still present in s,
// preserving first-seen order. An empty slice means every placeholder
// was substituted.
//
// Callers use this as a fast-fail guardrail: any field a template
// references but the caller never seeded into the params map shows up
// here, and the dispatch / sync refuses to write a broken artifact.
func FindUnfilled(s string) []string {
	matches := unfilledRE.FindAllString(s, -1)
	seen := map[string]bool{}
	var out []string
	for _, m := range matches {
		if seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	return out
}
