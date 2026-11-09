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
	ucs, dir, err := a.loadUseCases()
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("apply-ac-policy: %w", err)}
	}

	// Group the ids by the criteria file that defines them; an id no use case
	// defines is an error, reported before any file is touched.
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
		return statemachine.Outcome{Err: fmt.Errorf("apply-ac-policy: the story's criteria %s are not defined in the use cases in %s; the merge must have added them, so the use cases changed since", strings.Join(missing, ", "), dir)}
	}

	// Rewrite every file in memory first, then write; a file the deletion
	// cannot rewrite exactly fails the step with nothing written.
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
			return statemachine.Outcome{Err: fmt.Errorf("apply-ac-policy: %w", err)}
		}
		out, err := spec.DeleteCriteria(f, string(data), byFile[f])
		if err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("apply-ac-policy: %w", err)}
		}
		updated[path] = out
	}
	for _, f := range files {
		path := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.WriteFile(path, []byte(updated[path]), 0o644); err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("apply-ac-policy: write %s: %w", path, err)}
		}
	}
	fmt.Fprintf(a.deps.Out.Phase, "Deleted the story's green acceptance criteria from the use cases (%s); `gh optivem spec generate` renders them from the tests.\n", strings.Join(ids, ", "))
	return statemachine.Outcome{}
}
