package actions

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/optivem/gh-optivem/internal/spec"
)

// Files of the folder-per-use-case layout (<specDir>/UC-NNN-<slug>/...). Only
// the criteria file is a spec file for internal/spec; the narrative and the
// business rules are prose the merge keeps in step with it.
const (
	useCaseNarrativeFile = "use-case-narrative.md"
	businessRulesFile    = "business-rules.md"
)

var (
	// businessRuleRef is any mention of a business rule (BR-n).
	businessRuleRef = regexp.MustCompile(`\bBR-(\d+)\b`)
	// businessRuleDef is a business rule definition line: "- BR-1. <rule>"
	// (also "BR-1:" and bold "**BR-1.**").
	businessRuleDef = regexp.MustCompile(`(?m)^\s*(?:[-*]\s+)?\**BR-(\d+)\**[.:]`)
	// narrativeTag is the Gherkin tag linking a criterion scenario to the
	// narrative step or extension it exercises (@narrative-4a, @narrative-7).
	narrativeTag = regexp.MustCompile(`@narrative-([0-9]+[a-z]*)\b`)
	// narrativeStep is a main-scenario step ("7. The system ...") and
	// narrativeExtension an extension ("- 4a. The quantity is ...").
	narrativeStep      = regexp.MustCompile(`(?m)^\s*(\d+)\.\s`)
	narrativeExtension = regexp.MustCompile(`(?m)^\s*(?:[-*]\s+)?\**(\d+[a-z]+)\**\.`)
)

// useCaseFormatProblems checks the use case format beyond what spec.Validate
// covers, for the use cases that own one of the story's criteria (others are
// not this merge's business): every @narrative-Xy tag names a step or extension
// of the narrative, and every BR-n the narrative or criteria mention is defined
// in the business rules. Both layouts are handled: in the folder layout the
// narrative and rules are sibling files of acceptance-criteria.md; in the flat
// layout they all live in the one use case file.
func useCaseFormatProblems(dir string, ucs []spec.UseCase, criteria []string) ([]string, error) {
	var problems []string
	for _, uc := range ucs {
		if !ownsAny(uc, criteria) {
			continue
		}
		p, err := useCaseFormatProblemsFor(dir, uc)
		if err != nil {
			return nil, err
		}
		problems = append(problems, p...)
	}
	return problems, nil
}

func ownsAny(uc spec.UseCase, ids []string) bool {
	for _, c := range uc.Criteria {
		if slices.Contains(ids, c.ID) {
			return true
		}
	}
	return false
}

func useCaseFormatProblemsFor(dir string, uc spec.UseCase) ([]string, error) {
	criteriaPath := filepath.Join(dir, filepath.FromSlash(uc.File))
	criteriaText, err := readOptional(criteriaPath)
	if err != nil {
		return nil, err
	}
	narrativeName, rulesName := uc.File, uc.File
	narrativeText, rulesText := criteriaText, criteriaText
	if folder, _, ok := strings.Cut(uc.File, "/"); ok {
		narrativeName = folder + "/" + useCaseNarrativeFile
		rulesName = folder + "/" + businessRulesFile
		if narrativeText, err = readOptional(filepath.Join(dir, folder, useCaseNarrativeFile)); err != nil {
			return nil, err
		}
		if rulesText, err = readOptional(filepath.Join(dir, folder, businessRulesFile)); err != nil {
			return nil, err
		}
	}

	var problems []string
	anchors := map[string]bool{}
	for _, re := range []*regexp.Regexp{narrativeStep, narrativeExtension} {
		for _, m := range re.FindAllStringSubmatch(narrativeText, -1) {
			anchors[m[1]] = true
		}
	}
	for _, tag := range uniqueSubmatches(narrativeTag, criteriaText) {
		if !anchors[tag] {
			problems = append(problems, fmt.Sprintf("%s: @narrative-%s names no step or extension %s in %s", uc.File, tag, tag, narrativeName))
		}
	}

	defined := map[string]bool{}
	for _, m := range businessRuleDef.FindAllStringSubmatch(rulesText, -1) {
		defined[m[1]] = true
	}
	for _, src := range []struct{ name, text string }{{uc.File, criteriaText}, {narrativeName, narrativeText}} {
		for _, n := range uniqueSubmatches(businessRuleRef, src.text) {
			if !defined[n] {
				problems = append(problems, fmt.Sprintf("%s: BR-%s is referenced but not defined in %s", src.name, n, rulesName))
			}
		}
	}
	slices.Sort(problems)
	return slices.Compact(problems), nil
}

// readOptional reads a file, treating a missing file as empty (the narrative
// and business rules files are optional; a reference into a missing one is
// then reported as unresolved).
func readOptional(path string) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(data), nil
}

func uniqueSubmatches(re *regexp.Regexp, text string) []string {
	var out []string
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		if !slices.Contains(out, m[1]) {
			out = append(out, m[1])
		}
	}
	return out
}
