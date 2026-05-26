// Package parse is the shared markdown body parser consumed by the
// github and markdown tracker adapters. Both adapters operate on the
// same content model — H2/H3 headings carve the body into named
// sections, and `- [ ]` / `* [ ]` lines drive checklist completion —
// so the parse code lives in one place.
//
// Import path is intentionally inside tracker/internal/ so only the
// tracker subtree depends on it. Runtime code outside tracker/ never
// parses issue bodies directly; it goes through Tracker.ReadSections
// instead.
package parse

import "strings"

// ExtractSection scans body for an H2-or-deeper markdown heading whose
// text matches name (case-insensitive, exact after dropping leading
// hashes and surrounding whitespace), and returns the section body —
// every line from the next line to (but not including) the next heading
// at the same or shallower depth, with surrounding blank lines trimmed.
// Returns "" when the heading is absent or its body is empty.
func ExtractSection(body, name string) string {
	lines := strings.Split(body, "\n")
	startIdx, startDepth := -1, 0
	for i, line := range lines {
		depth, text, ok := mdHeading(line)
		if !ok || depth < 2 {
			continue
		}
		if strings.EqualFold(text, name) {
			startIdx, startDepth = i+1, depth
			break
		}
	}
	if startIdx < 0 {
		return ""
	}
	endIdx := len(lines)
	for i := startIdx; i < len(lines); i++ {
		depth, _, ok := mdHeading(lines[i])
		if !ok {
			continue
		}
		if depth <= startDepth {
			endIdx = i
			break
		}
	}
	return strings.Trim(strings.Join(lines[startIdx:endIdx], "\n"), "\n")
}

// TickCheckboxes rewrites every `- [ ]` / `* [ ]` to its checked
// equivalent. Indentation and marker character are preserved; already-
// ticked items pass through so the operation is idempotent.
func TickCheckboxes(body string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		indent := line[:len(line)-len(trimmed)]
		if !strings.HasPrefix(trimmed, "- [ ]") && !strings.HasPrefix(trimmed, "* [ ]") {
			continue
		}
		lines[i] = indent + strings.Replace(trimmed, "[ ]", "[x]", 1)
	}
	return strings.Join(lines, "\n")
}

// HasUnchecked reports whether body contains at least one `- [ ]` or
// `* [ ]` line. Lets callers skip a no-op rewrite + commit when the
// checklist is already fully ticked.
func HasUnchecked(body string) bool {
	for line := range strings.SplitSeq(body, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "- [ ]") || strings.HasPrefix(trimmed, "* [ ]") {
			return true
		}
	}
	return false
}

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
