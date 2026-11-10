package actions

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
	"github.com/optivem/gh-optivem/internal/spec"
)

// Acceptance-criteria policy at Green (plan 20261109-1258).
//
// Each acceptance criterion and its acceptance test say the same thing; kept
// side by side they drift, and only the test is guaranteed current because it
// must pass. So once a story's acceptance tests are green on every channel,
// apply-ac-policy deletes the story's criteria (the merged-criteria
// VERIFY_STORY_MERGE stamped) from the use cases. The use case keeps its
// narrative and business rules, each test keeps its UC-NNN.M tag, and `gh
// optivem spec generate` renders readable Gherkin back from the tests. A later
// story that modifies the behaviour re-adds the criterion under the same id.
// Deterministic — no agent.

// applyAcPolicy is the APPLY_AC_POLICY service task.
func (a actions) applyAcPolicy(ctx *statemachine.Context) statemachine.Outcome {
	var ids []string
	for id := range strings.SplitSeq(ctx.GetString(CtxKeyMergedCriteria), ",") {
		if id = strings.TrimSpace(id); id != "" && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return statemachine.Outcome{Err: fmt.Errorf("apply-ac-policy: %s is empty; the story merge stamps it for every story, so this node must run after MERGE_STORY_INTO_USE_CASES", CtxKeyMergedCriteria)}
	}
	if err := a.deleteCriteria(ids, "the story's", "the merge must have added them, so the use cases changed since"); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("apply-ac-policy: %w", err)}
	}
	fmt.Fprintf(a.deps.Out.Phase, "Deleted the story's green acceptance criteria from the use cases (%s); `gh optivem spec generate` renders them from the tests.\n", strings.Join(ids, ", "))
	return statemachine.Outcome{}
}

// deleteCriteria deletes the criteria ids from the use cases that define
// them. An id no use case defines is an error ("<whose> criteria ... are not
// defined ...; <hint>"), reported before any file is touched;
// every file is rewritten in memory first, so a file the deletion cannot
// rewrite exactly fails with nothing written.
func (a actions) deleteCriteria(ids []string, whose, hint string) error {
	ucs, dir, err := a.loadUseCases()
	if err != nil {
		return err
	}
	byFile := map[string][]string{}
	owner := map[string]string{}
	for _, uc := range ucs {
		for _, c := range uc.Criteria {
			owner[c.ID] = uc.File
		}
	}
	var missing []string
	for _, id := range ids {
		f, ok := owner[id]
		if !ok {
			missing = append(missing, id)
			continue
		}
		byFile[f] = append(byFile[f], id)
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s criteria %s are not defined in the use cases in %s; %s", whose, strings.Join(missing, ", "), dir, hint)
	}
	files := make([]string, 0, len(byFile))
	for f := range byFile {
		files = append(files, f)
	}
	slices.Sort(files)
	updated := map[string]string{}
	for _, f := range files {
		path := filepath.Join(dir, filepath.FromSlash(f))
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out, err := spec.DeleteCriteria(f, string(data), byFile[f])
		if err != nil {
			return err
		}
		updated[path] = out
	}
	for _, f := range files {
		path := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.WriteFile(path, []byte(updated[path]), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

// deleteUseCaseCriteria is characterize's DELETE_USE_CASE_CRITERIA (plan
// 20261110-1130): once the current use case's tests are green and un-gated,
// every criterion its use case files define is deleted, the same deletion as
// apply-ac-policy. A use case is done when it has tagged tests and no
// criteria left (order-use-cases), so all of them go, including those a late
// discovery re-wrote. A use case with no criteria left (a resumed run that
// deleted them but did not commit yet) is a no-op.
func (a actions) deleteUseCaseCriteria(ctx *statemachine.Context) statemachine.Outcome {
	id := strings.TrimSpace(ctx.GetString(CtxKeyUseCaseID))
	if id == "" {
		return statemachine.Outcome{Err: fmt.Errorf("delete-use-case-criteria: %s is not in State; a use case must have been selected first", CtxKeyUseCaseID)}
	}
	ucs, _, err := a.loadUseCases()
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("delete-use-case-criteria: %w", err)}
	}
	var ids []string
	for _, uc := range ucs {
		if uc.ID != id {
			continue
		}
		for _, c := range uc.Criteria {
			if !slices.Contains(ids, c.ID) {
				ids = append(ids, c.ID)
			}
		}
	}
	if len(ids) == 0 {
		fmt.Fprintf(a.deps.Out.Phase, "Use case %s has no criteria left to delete.\n", id)
		return statemachine.Outcome{}
	}
	if err := a.deleteCriteria(ids, "use case "+id+"'s", "they were read from its files a moment ago, so the files changed meanwhile"); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("delete-use-case-criteria: %w", err)}
	}
	fmt.Fprintf(a.deps.Out.Phase, "Deleted use case %s's criteria (%s); `gh optivem spec generate` renders them from the tests.\n", id, strings.Join(ids, ", "))
	return statemachine.Outcome{}
}
