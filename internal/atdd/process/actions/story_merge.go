package actions

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
	"github.com/optivem/gh-optivem/internal/spec"
)

// State keys of the story-merge step.
const (
	// CtxKeyStoryMergeNeeded is the bool check-story-merge-needed stamps; the
	// GATE_STORY_MERGE_NEEDED gateway routes on it.
	CtxKeyStoryMergeNeeded = "story-merge-needed"
	// CtxKeyMergedCriteria is the comma-separated list of UC-NNN.M ids the
	// story's scenarios trace to. Both the no-op path and verify-story-merge
	// stamp it; the acceptance-test writer reads it so the tests it writes
	// carry those ids.
	CtxKeyMergedCriteria = "merged-criteria"
	// CtxKeyScenarioCriteria holds one "<scenario title> => UC-NNN.M" line per
	// story scenario; clauderun renders it into the acceptance-test writer's
	// ${use-case-traceability} section.
	CtxKeyScenarioCriteria = "scenario-criteria"
	// ctxKeyScenarioMap and ctxKeyContradictions are the merge agent's outputs.
	ctxKeyScenarioMap    = "scenario-map"
	ctxKeyContradictions = "contradictions"
)

const specConfigFile = "spec.yaml"

// loadUseCases reads the repo's use cases from the spec directory named by
// spec.yaml (the default directory when the repo has no spec.yaml). A spec
// directory that does not exist yet is the legitimate "no use cases yet"
// state: the first merge creates it. Every other failure surfaces.
func (a actions) loadUseCases() ([]spec.UseCase, string, error) {
	root := a.deps.RepoPath
	if root == "" {
		root = "."
	}
	cfg := spec.Config{SpecDir: spec.DefaultSpecDir}
	cfgFile := filepath.Join(root, specConfigFile)
	if loaded, err := spec.LoadConfig(cfgFile); err == nil {
		cfg = loaded
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, "", err
	}
	dir := cfg.SpecDir
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(root, dir)
	}
	ucs, err := spec.LoadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, dir, nil
		}
		return nil, dir, err
	}
	return ucs, dir, nil
}

// checkStoryMergeNeeded decides, before any agent runs, whether the story must
// be merged into the use cases. A story whose every scenario already names an
// existing UC-NNN.M criterion (stories generated from the use cases do) needs
// no merge: the step is skipped without a human approval, so unattended runs do
// not stall on it. Anything else goes to the merge agent.
func (a actions) checkStoryMergeNeeded(ctx *statemachine.Context) statemachine.Outcome {
	criteria := ctx.GetString("acceptance-criteria")
	if strings.TrimSpace(criteria) == "" {
		return statemachine.Outcome{Err: fmt.Errorf("check-story-merge-needed: the story has no acceptance criteria (parse-ticket stamped an empty acceptance-criteria); a story needs scenarios to merge")}
	}
	ucs, _, err := a.loadUseCases()
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("check-story-merge-needed: %w", err)}
	}
	scenarios := spec.StoryScenarios(criteria)
	merged, ids := spec.StoryAlreadyMerged(scenarios, ucs)
	ctx.Set(CtxKeyStoryMergeNeeded, !merged)
	if merged {
		known := spec.CriterionIDs(ucs)
		mapping := map[string][]string{}
		for _, sc := range scenarios {
			for _, id := range sc.Refs {
				if known[id] {
					mapping[sc.Title] = append(mapping[sc.Title], id)
				}
			}
		}
		ctx.Set(CtxKeyMergedCriteria, strings.Join(ids, ", "))
		ctx.Set(CtxKeyScenarioCriteria, scenarioCriteriaLines(scenarios, mapping))
		fmt.Fprintf(a.deps.Out.Phase, "Story already matches the use cases (%s); no merge needed.\n", strings.Join(ids, ", "))
	}
	return statemachine.Outcome{}
}

// verifyStoryMerge is the consistency check after the merge agent: the agent
// raised no contradiction, the merged use cases validate, every story scenario
// is mapped to a criterion that now exists, and in the use cases the story
// touched every @narrative-Xy tag and BR-n reference resolves
// (useCaseFormatProblems). Any gap fails loudly with every
// problem listed; nothing is repaired or skipped.
func (a actions) verifyStoryMerge(ctx *statemachine.Context) statemachine.Outcome {
	if c := stringListState(ctx, ctxKeyContradictions); len(c) > 0 {
		return statemachine.Outcome{Err: fmt.Errorf("verify-story-merge: the merge agent reported %d contradiction(s) between the story and the use cases; resolve them (edit the story or the use case) and re-run:\n  - %s", len(c), strings.Join(c, "\n  - "))}
	}
	mapping, err := spec.ParseScenarioMap(stringListState(ctx, ctxKeyScenarioMap))
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("verify-story-merge: %w", err)}
	}
	ucs, dir, err := a.loadUseCases()
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("verify-story-merge: %w", err)}
	}
	scenarios := spec.StoryScenarios(ctx.GetString("acceptance-criteria"))
	problems, covered := spec.CheckStoryMerge(scenarios, mapping, ucs)
	formatProblems, err := useCaseFormatProblems(dir, ucs, covered)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("verify-story-merge: %w", err)}
	}
	problems = append(problems, formatProblems...)
	if len(problems) > 0 {
		return statemachine.Outcome{Err: fmt.Errorf("verify-story-merge: the story is not fully merged into the use cases in %s:\n  - %s", dir, strings.Join(problems, "\n  - "))}
	}
	ctx.Set(CtxKeyMergedCriteria, strings.Join(covered, ", "))
	byScenario := map[string][]string{}
	for title, ids := range mapping {
		byScenario[title] = ids
	}
	ctx.Set(CtxKeyScenarioCriteria, scenarioCriteriaLines(scenarios, byScenario))
	fmt.Fprintf(a.deps.Out.Phase, "Story merged into the use cases (%s).\n", strings.Join(covered, ", "))
	return statemachine.Outcome{}
}

// scenarioCriteriaLines renders "<title> => ids" per story scenario, in story
// order. mapping is keyed by scenario title (normalised by whitespace).
func scenarioCriteriaLines(scenarios []spec.StoryScenario, mapping map[string][]string) string {
	var lines []string
	for _, sc := range scenarios {
		ids := mapping[strings.Join(strings.Fields(sc.Title), " ")]
		if len(ids) == 0 {
			ids = mapping[sc.Title]
		}
		if len(ids) == 0 {
			continue
		}
		slices.Sort(ids)
		lines = append(lines, sc.Title+" => "+strings.Join(slices.Compact(ids), ", "))
	}
	return strings.Join(lines, "\n")
}

// stringListState reads a string-list State value, tolerating the []any form
// JSON decoding yields and a newline-joined string.
func stringListState(ctx *statemachine.Context, key string) []string {
	switch v := ctx.Get(key).(type) {
	case []string:
		return v
	case []any:
		var out []string
		for _, e := range v {
			if str, ok := e.(string); ok {
				out = append(out, str)
			}
		}
		return out
	case string:
		var out []string
		for _, p := range strings.Split(v, "\n") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	return nil
}
