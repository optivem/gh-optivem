// Package parse holds the shared markdown helpers the github and markdown
// tracker adapters use to source an Issue.Title from a body — currently
// just FirstH1. Section extraction + validation lives in the intake
// package, which parses the raw body returned by Tracker.ReadBody.
//
// Import path is intentionally inside tracker/internal/ so only the
// tracker subtree depends on it.
package parse

import "strings"

// FirstH1 returns the trimmed text of the first H1 (`# Title`) heading
// in body, or "" when no H1 is present. Used by the markdown adapter
// to source Issue.Title — closest analogue to GitHub's issue title.
func FirstH1(body string) string {
	for line := range strings.SplitSeq(body, "\n") {
		depth, text, ok := mdHeading(line)
		if ok && depth == 1 {
			return text
		}
	}
	return ""
}

// mdHeading parses one markdown heading line — leading `#`s + whitespace
// + text. Returns depth (count of leading `#`s), trimmed text, and
// ok=true on a heading line; ok=false on non-heading lines.
func mdHeading(line string) (int, string, bool) {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "#") {
		return 0, "", false
	}
	d := 0
	for d < len(t) && t[d] == '#' {
		d++
	}
	return d, strings.TrimSpace(t[d:]), true
}
