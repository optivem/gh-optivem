package spec

import (
	"fmt"
	"slices"
	"strings"
)

// DeleteCriteria removes the criteria with the given ids from the markdown
// content of one use-case criteria file and returns the new content. It is the
// acceptance-criteria policy applied once a story's acceptance tests are green:
// the tagged tests are now the spec, so the criterion text goes (`gh optivem
// spec generate` renders it back from the tests on demand).
//
// Both criterion forms are handled:
//   - a bullet `- UC-NNN.M: text` line, with its indented continuation lines;
//   - every Gherkin scenario tagged @UC-NNN.M, with the tag and comment lines
//     directly above its header, its steps and its Examples, up to the next
//     scenario block, Rule/Feature/Background line or the end of the fence.
//
// Everything else (front matter, headings, prose, other criteria) is kept as
// is. An id that the content does not define is an error, and the result is
// re-parsed to prove that exactly the named criteria are gone and every other
// criterion survived; nothing is skipped silently.
func DeleteCriteria(file, content string, ids []string) (string, error) {
	if len(ids) == 0 {
		return "", fmt.Errorf("%s: no criterion ids to delete", file)
	}
	before := Parse(file, content)
	defined := map[string]bool{}
	for _, c := range before.Criteria {
		defined[c.ID] = true
	}
	var missing []string
	for _, id := range ids {
		if !defined[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("%s: criteria %s are not defined in this file", file, strings.Join(missing, ", "))
	}

	crlf := strings.Contains(content, "\r\n")
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	drop := make([]bool, len(lines))
	start := 0
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		if end := frontMatterEnd(lines); end >= 0 {
			start = end + 1
		}
	}
	del := toSet(ids)
	for i := start; i < len(lines); i++ {
		if fenceOpenPattern.MatchString(lines[i]) {
			close := i + 1
			for close < len(lines) && !fenceAnyPattern.MatchString(lines[close]) {
				close++
			}
			markGherkinDeletions(lines, i+1, close, del, drop)
			i = close
			continue
		}
		m := criterionPattern.FindStringSubmatch(strings.TrimRight(lines[i], " \t"))
		if m == nil || !del[m[1]] {
			continue
		}
		drop[i] = true
		for j := i + 1; j < len(lines) && isContinuation(lines[j]); j++ {
			drop[j] = true
			i = j
		}
	}

	var kept []string
	for i, l := range lines {
		if !drop[i] {
			kept = append(kept, l)
		}
	}
	nl := "\n"
	if crlf {
		nl = "\r\n"
	}
	out := strings.Join(kept, nl)

	if err := checkDeletion(file, before, Parse(file, out), del); err != nil {
		return "", err
	}
	return out, nil
}

// isContinuation reports whether line continues the bullet above it: it is
// indented and not blank.
func isContinuation(line string) bool {
	return strings.TrimSpace(line) != "" && (line[0] == ' ' || line[0] == '\t')
}

// markGherkinDeletions marks the lines of every scenario block in
// lines[from:to] (the body of one gherkin fence) whose tags name an id in del.
func markGherkinDeletions(lines []string, from, to int, del map[string]bool, drop []bool) {
	var headers []int
	for i := from; i < to; i++ {
		if scenarioHeader.MatchString(lines[i]) {
			headers = append(headers, i)
		}
	}
	starts := make([]int, len(headers))
	for k, h := range headers {
		s := h
		for s > from && isTagOrComment(lines[s-1]) {
			s--
		}
		starts[k] = s
	}
	for k, h := range headers {
		end, boundary := to, true
		if k+1 < len(headers) {
			end, boundary = starts[k+1], false
		}
		for i := h + 1; i < end; i++ {
			if blockBoundary.MatchString(lines[i]) {
				end, boundary = i, true
				break
			}
		}
		if !taggedWithAny(lines[starts[k]:h], del) {
			continue
		}
		s := starts[k]
		if boundary {
			// The block is last before a boundary: keep the blank line that
			// separates it from what follows and drop the one before it, so
			// no double blank line is left behind.
			for end > h+1 && strings.TrimSpace(lines[end-1]) == "" {
				end--
			}
			for s > from && strings.TrimSpace(lines[s-1]) == "" {
				s--
			}
		}
		for i := s; i < end; i++ {
			drop[i] = true
		}
	}
}

// taggedWithAny reports whether the tag lines name a criterion id in del.
func taggedWithAny(tagLines []string, del map[string]bool) bool {
	for _, l := range tagLines {
		t := strings.TrimSpace(l)
		if !strings.HasPrefix(t, "@") {
			continue
		}
		for _, tag := range strings.Fields(t) {
			if m := criterionTagRe.FindStringSubmatch(tag); m != nil && del[m[1]] {
				return true
			}
		}
	}
	return false
}

// checkDeletion proves the rewrite removed exactly the criteria in del.
func checkDeletion(file string, before, after UseCase, del map[string]bool) error {
	var want, got []string
	for _, c := range before.Criteria {
		if !del[c.ID] {
			want = append(want, c.ID)
		}
	}
	for _, c := range after.Criteria {
		got = append(got, c.ID)
	}
	if !slices.Equal(want, got) {
		return fmt.Errorf("%s: deleting %s would leave criteria %v, want %v; the file layout is not one the deletion understands, edit it by hand", file, strings.Join(sortedKeys(del), ", "), got, want)
	}
	return nil
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func toSet(ids []string) map[string]bool {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}
