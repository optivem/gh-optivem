// Package spec parses use-case markdown files into a criteria model and validates them.
package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Criterion is one acceptance path of a use case.
type Criterion struct {
	ID    string
	Text  string
	Group string
	Flag  string
}

// UseCase is a parsed use-case markdown file.
type UseCase struct {
	ID    string
	Title string
	// Status is the front matter status: StatusImplemented (the default when
	// the field is absent) or StatusPlanned. Planned use cases are validated
	// but left out of coverage and the threshold.
	Status   string
	File     string
	Criteria []Criterion
	FrontErr string
	// ParseErrs are content problems found while parsing (for example a
	// scenario tagged with two criterion IDs); Validate reports them.
	ParseErrs []string
	// Warnings are problems that don't fail validation (for example a
	// Gherkin scenario with no criterion tag).
	Warnings []string
}

// Use case statuses (front matter `status:`).
const (
	StatusImplemented = "implemented"
	StatusPlanned     = "planned"
)

// Planned reports whether the use case is specified but not built yet.
func (uc UseCase) Planned() bool { return uc.Status == StatusPlanned }

// ValidationError describes one problem found by Validate.
type ValidationError struct {
	Code    string
	Message string
}

var (
	useCaseIDPattern = regexp.MustCompile(`^UC-\d+$`)
	criterionPattern = regexp.MustCompile(`^[-*]\s+(UC-\d+\.\d+)(?:\s+\[flag:\s*([^\]\s][^\]]*?)\s*\])?\s*:\s*(.*)$`)
	headingPattern   = regexp.MustCompile(`^#{1,6}\s+(.*?)\s*#*\s*$`)
	fenceOpenPattern = regexp.MustCompile("^\\s*```+\\s*gherkin\\s*$")
	fenceAnyPattern  = regexp.MustCompile("^\\s*```")
	ruleHeader       = regexp.MustCompile(`^\s*Rule\s*:\s*(.*?)\s*$`)
	criterionTagRe   = regexp.MustCompile(`^@(UC-\d+\.\d+)$`)
)

// Parse parses the markdown content of one use-case file. A front matter
// problem is recorded in UseCase.FrontErr rather than returned, so that
// Validate can report it alongside other errors.
func Parse(file, content string) UseCase {
	uc := UseCase{File: file}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")

	body, frontErr := parseFrontMatter(lines, &uc)
	uc.FrontErr = frontErr

	group := ""
	var g *gherkinParser
	for _, line := range body {
		if g != nil {
			if fenceAnyPattern.MatchString(line) {
				g = nil
				continue
			}
			g.line(line)
			continue
		}
		if fenceOpenPattern.MatchString(line) {
			g = &gherkinParser{uc: &uc, group: group, byID: map[string]int{}}
			continue
		}
		if m := headingPattern.FindStringSubmatch(line); m != nil {
			group = m[1]
			continue
		}
		if m := criterionPattern.FindStringSubmatch(strings.TrimRight(line, " \t")); m != nil {
			uc.Criteria = append(uc.Criteria, Criterion{
				ID:    m[1],
				Flag:  m[2],
				Text:  strings.TrimSpace(m[3]),
				Group: group,
			})
		}
	}
	return uc
}

// parseFrontMatter fills id and title and returns the lines after the front
// matter plus an error message ("" when valid).
func parseFrontMatter(lines []string, uc *UseCase) ([]string, string) {
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return lines, "missing front matter (expected a leading --- block with id and title)"
	}
	end := frontMatterEnd(lines)
	if end < 0 {
		return lines, "front matter is not closed with ---"
	}
	rest := lines[end+1:]
	fields, msg := parseFrontMatterFields(lines[1:end])
	if msg != "" {
		return rest, msg
	}
	uc.ID = fields["id"]
	uc.Title = fields["title"]
	uc.Status = fields["status"]
	if uc.Status == "" {
		uc.Status = StatusImplemented
	}
	return rest, validateFrontMatter(uc)
}

// frontMatterEnd returns the index of the closing --- line of the front
// matter that opens at lines[0], or -1 when it is not closed.
func frontMatterEnd(lines []string) int {
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return i
		}
	}
	return -1
}

// parseFrontMatterFields parses the key: value lines between the front matter
// delimiters and returns an error message ("" when valid).
func parseFrontMatterFields(body []string) (map[string]string, string) {
	fields := map[string]string{}
	for _, l := range body {
		if strings.TrimSpace(l) == "" {
			continue
		}
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			return nil, fmt.Sprintf("malformed front matter line %q", l)
		}
		fields[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), "\"'")
	}
	return fields, ""
}

// validateFrontMatter checks the parsed front matter fields of uc and returns
// an error message ("" when valid).
func validateFrontMatter(uc *UseCase) string {
	if uc.ID == "" {
		return "front matter is missing id"
	}
	if !useCaseIDPattern.MatchString(uc.ID) {
		return fmt.Sprintf("front matter id %q does not match UC-<n>", uc.ID)
	}
	if uc.Title == "" {
		return "front matter is missing title"
	}
	if uc.Status != StatusImplemented && uc.Status != StatusPlanned {
		return fmt.Sprintf("front matter status %q must be %s or %s", uc.Status, StatusImplemented, StatusPlanned)
	}
	return ""
}

// criteriaFileName is the spec file inside a use-case folder. Other files in
// the folder (narrative, business rules) are for people and are not parsed.
const criteriaFileName = "acceptance-criteria.md"

var useCaseFolderPattern = regexp.MustCompile(`^(UC-\d+)-`)

// LoadDir parses the use cases under dir, in two layouts that can be mixed:
//   - flat: every .md file directly inside dir except README.md;
//   - folder per use case: <dir>/<folder>/acceptance-criteria.md, one level deep.
//
// Results are sorted by file path (folder name for the folder layout).
func LoadDir(dir string) ([]UseCase, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read spec dir %s: %w", dir, err)
	}
	var names []string
	for _, e := range entries {
		switch {
		case e.IsDir():
			p := filepath.Join(dir, e.Name(), criteriaFileName)
			if _, err := os.Stat(p); err == nil {
				names = append(names, e.Name()+"/"+criteriaFileName)
			} else if !os.IsNotExist(err) {
				return nil, fmt.Errorf("stat %s: %w", p, err)
			}
		case strings.EqualFold(filepath.Ext(e.Name()), ".md") && !strings.EqualFold(e.Name(), "README.md"):
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	var out []UseCase
	for _, n := range names {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(n)))
		if err != nil {
			return nil, fmt.Errorf("read use case %s: %w", n, err)
		}
		uc := Parse(n, string(data))
		if folder, _, ok := strings.Cut(n, "/"); ok {
			if m := useCaseFolderPattern.FindStringSubmatch(folder); m != nil && uc.FrontErr == "" && m[1] != uc.ID {
				uc.Warnings = append(uc.Warnings, fmt.Sprintf("folder %s is named for %s but its front matter id is %s", folder, m[1], uc.ID))
			}
		}
		out = append(out, uc)
	}
	return out, nil
}

// Validate checks malformed front matter, criterion IDs whose prefix does not
// match the use case id, and duplicate criterion IDs across all use cases.
func Validate(useCases []UseCase) []ValidationError {
	var errs []ValidationError
	seen := map[string]string{}
	for _, uc := range useCases {
		if uc.FrontErr != "" {
			errs = append(errs, ValidationError{"front-matter", fmt.Sprintf("%s: %s", uc.File, uc.FrontErr)})
		}
		for _, pe := range uc.ParseErrs {
			errs = append(errs, ValidationError{"criteria", fmt.Sprintf("%s: %s", uc.File, pe)})
		}
		if uc.FrontErr == "" && !uc.Planned() && len(uc.Criteria) == 0 {
			errs = append(errs, ValidationError{"no-criteria",
				fmt.Sprintf("%s: use case %s has no criteria (expected `- %s.N: text` bullets or Gherkin scenarios tagged @%s.N)", uc.File, uc.ID, uc.ID, uc.ID)})
		}
		errs = append(errs, validateCriteria(uc, seen)...)
	}
	return errs
}

// validateCriteria checks the criterion IDs of uc for a prefix mismatch with
// the use case id and for duplicates against seen (criterion ID -> file),
// recording each new ID in seen.
func validateCriteria(uc UseCase, seen map[string]string) []ValidationError {
	var errs []ValidationError
	for _, c := range uc.Criteria {
		prefix, _, _ := strings.Cut(c.ID, ".")
		if uc.FrontErr == "" && prefix != uc.ID {
			errs = append(errs, ValidationError{"prefix-mismatch",
				fmt.Sprintf("%s: criterion %s does not match use case id %s", uc.File, c.ID, uc.ID)})
		}
		if first, dup := seen[c.ID]; dup {
			errs = append(errs, ValidationError{"duplicate-id",
				fmt.Sprintf("%s: criterion %s already defined in %s", uc.File, c.ID, first)})
			continue
		}
		seen[c.ID] = uc.File
	}
	return errs
}

// Warnings returns the non-failing problems found in the use cases, each
// prefixed with its file.
func Warnings(useCases []UseCase) []string {
	var out []string
	for _, uc := range useCases {
		for _, w := range uc.Warnings {
			out = append(out, uc.File+": "+w)
		}
	}
	return out
}

// gherkinParser turns the scenarios of one fenced gherkin block into
// criteria. Each distinct @UC-NNN.M tag is one criterion; several scenarios
// may share a tag (for example a scenario and an outline covering the same
// path), and their titles are joined into the criterion text so a change to
// any of them changes the criterion fingerprint.
type gherkinParser struct {
	uc    *UseCase
	group string
	tags  []string
	byID  map[string]int // criterion ID -> index in uc.Criteria (Gherkin only)
}

func (g *gherkinParser) line(line string) {
	t := strings.TrimSpace(line)
	switch {
	case t == "" || strings.HasPrefix(t, "#"):
		return
	case strings.HasPrefix(t, "@"):
		g.tags = append(g.tags, strings.Fields(t)...)
		return
	}
	if m := ruleHeader.FindStringSubmatch(t); m != nil {
		g.group = m[1]
		g.tags = nil
		return
	}
	m := scenarioHeader.FindStringSubmatch(t)
	if m == nil {
		g.tags = nil
		return
	}
	title := m[1]
	var ids []string
	for _, tag := range g.tags {
		if tm := criterionTagRe.FindStringSubmatch(tag); tm != nil && !slices.Contains(ids, tm[1]) {
			ids = append(ids, tm[1])
		}
	}
	g.tags = nil
	switch len(ids) {
	case 0:
		g.uc.Warnings = append(g.uc.Warnings, fmt.Sprintf("scenario %q has no @UC-N.M tag, so it is not a criterion", title))
	case 1:
		if i, ok := g.byID[ids[0]]; ok {
			g.uc.Criteria[i].Text += "; " + title
			return
		}
		g.byID[ids[0]] = len(g.uc.Criteria)
		g.uc.Criteria = append(g.uc.Criteria, Criterion{ID: ids[0], Text: title, Group: g.group})
	default:
		g.uc.ParseErrs = append(g.uc.ParseErrs, fmt.Sprintf("scenario %q is tagged with %d criterion IDs (%s); tag each scenario with one", title, len(ids), strings.Join(ids, ", ")))
	}
}
