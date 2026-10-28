// Package spec parses use-case markdown files into a criteria model and validates them.
package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	ID       string
	Title    string
	File     string
	Criteria []Criterion
	FrontErr string
}

// ValidationError describes one problem found by Validate.
type ValidationError struct {
	Code    string
	Message string
}

var (
	useCaseIDPattern = regexp.MustCompile(`^UC-\d+$`)
	criterionPattern = regexp.MustCompile(`^[-*]\s+(UC-\d+\.\d+)(?:\s+\[flag:\s*([^\]\s][^\]]*?)\s*\])?\s*:\s*(.*)$`)
	headingPattern   = regexp.MustCompile(`^#{1,6}\s+(.*?)\s*#*\s*$`)
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
	for _, line := range body {
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
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return lines, "front matter is not closed with ---"
	}
	rest := lines[end+1:]
	fields := map[string]string{}
	for _, l := range lines[1:end] {
		if strings.TrimSpace(l) == "" {
			continue
		}
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			return rest, fmt.Sprintf("malformed front matter line %q", l)
		}
		fields[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), "\"'")
	}
	uc.ID = fields["id"]
	uc.Title = fields["title"]
	if uc.ID == "" {
		return rest, "front matter is missing id"
	}
	if !useCaseIDPattern.MatchString(uc.ID) {
		return rest, fmt.Sprintf("front matter id %q does not match UC-<n>", uc.ID)
	}
	if uc.Title == "" {
		return rest, "front matter is missing title"
	}
	return rest, ""
}

// LoadDir parses every .md file directly inside dir, sorted by file name.
func LoadDir(dir string) ([]UseCase, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read spec dir %s: %w", dir, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	var out []UseCase
	for _, n := range names {
		data, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			return nil, fmt.Errorf("read use case %s: %w", n, err)
		}
		out = append(out, Parse(n, string(data)))
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
	}
	return errs
}
