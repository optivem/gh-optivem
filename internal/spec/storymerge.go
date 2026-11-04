package spec

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Story merge helpers. A user story's Gherkin acceptance criteria are merged
// into the use cases before ATDD builds the story. These pure functions decide
// whether a merge is needed at all and whether a finished merge accounts for
// every scenario of the story.

var (
	criterionRefPattern = regexp.MustCompile(`UC-\d+\.\d+`)
	scenarioHeader      = regexp.MustCompile(`^\s*(?:Scenario|Scenario Outline|Example)\s*:\s*(.*?)\s*$`)
	blockBoundary       = regexp.MustCompile(`^\s*(?:Feature|Rule|Background)\s*:`)
)

// StoryScenario is one Gherkin scenario of a story with the UC-NNN.M ids it
// names (in its tag lines, comment lines, title or steps).
type StoryScenario struct {
	Title string
	Refs  []string
}

// StoryScenarios extracts the scenarios of a story's acceptance-criteria body.
// A scenario's block starts at the tag and comment lines directly above its
// header and runs to the next scenario block or Feature/Rule/Background line.
func StoryScenarios(criteria string) []StoryScenario {
	lines := strings.Split(strings.ReplaceAll(criteria, "\r\n", "\n"), "\n")
	var headers []int
	for i, l := range lines {
		if scenarioHeader.MatchString(l) {
			headers = append(headers, i)
		}
	}
	starts := make([]int, len(headers))
	for k, h := range headers {
		s := h
		for s > 0 && isTagOrComment(lines[s-1]) {
			s--
		}
		starts[k] = s
	}
	var out []StoryScenario
	for k, h := range headers {
		end := len(lines)
		if k+1 < len(headers) {
			end = starts[k+1]
		}
		for i := h + 1; i < end; i++ {
			if blockBoundary.MatchString(lines[i]) {
				end = i
				break
			}
		}
		title := scenarioHeader.FindStringSubmatch(lines[h])[1]
		out = append(out, StoryScenario{
			Title: title,
			Refs:  ReferencedCriteria(strings.Join(lines[starts[k]:end], "\n")),
		})
	}
	return out
}

func isTagOrComment(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "@") || strings.HasPrefix(t, "#")
}

// ReferencedCriteria returns the sorted, de-duplicated UC-NNN.M ids in text.
func ReferencedCriteria(text string) []string {
	ids := criterionRefPattern.FindAllString(text, -1)
	slices.Sort(ids)
	return slices.Compact(ids)
}

// CriterionIDs returns the set of criterion ids defined by the use cases.
func CriterionIDs(useCases []UseCase) map[string]bool {
	known := map[string]bool{}
	for _, uc := range useCases {
		for _, c := range uc.Criteria {
			known[c.ID] = true
		}
	}
	return known
}

// StoryAlreadyMerged reports whether a story needs no merge: it has at least
// one scenario and every scenario names a criterion that already exists in the
// use cases (stories generated from the use cases do this). It also returns
// the sorted ids the story covers when that holds, and nil otherwise.
func StoryAlreadyMerged(scenarios []StoryScenario, useCases []UseCase) (bool, []string) {
	if len(scenarios) == 0 {
		return false, nil
	}
	known := CriterionIDs(useCases)
	var covered []string
	for _, sc := range scenarios {
		hit := false
		for _, id := range sc.Refs {
			if known[id] {
				hit = true
				covered = append(covered, id)
			}
		}
		if !hit {
			return false, nil
		}
	}
	slices.Sort(covered)
	return true, slices.Compact(covered)
}

// ParseScenarioMap parses the merge agent's scenario-map entries, each of the
// form "<scenario title> => UC-001.1[, UC-001.2]", into title -> ids.
func ParseScenarioMap(entries []string) (map[string][]string, error) {
	out := map[string][]string{}
	for _, e := range entries {
		title, ids, ok := strings.Cut(e, "=>")
		if !ok {
			return nil, fmt.Errorf("scenario-map entry %q is not of the form \"<scenario title> => UC-NNN.M\"", e)
		}
		title = normTitle(title)
		refs := ReferencedCriteria(ids)
		if title == "" || len(refs) == 0 {
			return nil, fmt.Errorf("scenario-map entry %q needs a scenario title and at least one UC-NNN.M id", e)
		}
		out[title] = append(out[title], refs...)
	}
	return out, nil
}

func normTitle(s string) string { return strings.Join(strings.Fields(s), " ") }

// CheckStoryMerge verifies a finished merge: the use cases validate, every
// story scenario is mapped, and every mapped id exists in the merged use
// cases. It returns one message per gap; an empty result means the merge
// accounts for the whole story. Mapped ids are returned sorted.
func CheckStoryMerge(scenarios []StoryScenario, mapping map[string][]string, useCases []UseCase) (problems []string, covered []string) {
	for _, ve := range Validate(useCases) {
		problems = append(problems, fmt.Sprintf("use case validation (%s): %s", ve.Code, ve.Message))
	}
	known := CriterionIDs(useCases)
	if len(scenarios) == 0 {
		problems = append(problems, "the story has no scenarios to merge")
	}
	for _, sc := range scenarios {
		ids, ok := mapping[normTitle(sc.Title)]
		if !ok {
			problems = append(problems, fmt.Sprintf("story scenario %q is not mapped to any use case criterion", sc.Title))
			continue
		}
		for _, id := range ids {
			if !known[id] {
				problems = append(problems, fmt.Sprintf("story scenario %q maps to %s, which does not exist in the use cases", sc.Title, id))
				continue
			}
			covered = append(covered, id)
		}
	}
	slices.Sort(covered)
	return problems, slices.Compact(covered)
}
