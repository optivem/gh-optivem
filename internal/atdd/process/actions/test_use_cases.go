package actions

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// Narrowed refactor verify runs (plan 20261111-1023). A refactor inside a use
// case runs only that use case's acceptance tests locally (the pipeline still
// runs everything after push): `gh optivem system-test run --use-case <list>`.
// No known use case means the whole suite.
const (
	// ParamTestUseCases is the call-activity param runCommand renders as
	// `--use-case=<list>` onto `gh optivem system-test run`. Bound only by the
	// refactor cycles and the review's guideline round; the processes between
	// them and run-tests do not re-bind it, it reaches execute-command by
	// Params inheritance. Empty or absent means the whole suite.
	ParamTestUseCases = "test-use-cases"
	// CtxKeyRefactorUseCaseIDs is the code-architecture-reviewer's optional
	// output: the use cases its refactoring plan touches. Normalised by
	// summarize-refactoring-plan, which always stamps it ("" when the plan
	// names none, so the call sites binding it never see it unset).
	CtxKeyRefactorUseCaseIDs = "refactor-use-case-ids"
	// CtxKeyRefactorTestUseCases is what resolve-test-use-cases stamps for
	// the refactor cycles' verify run.
	CtxKeyRefactorTestUseCases = "refactor-test-use-cases"
)

var useCaseSelectionPattern = regexp.MustCompile(`^(UC-\d+)(\.\d+)?$`)

// useCaseSelection reduces a comma- or space-separated list of use case or
// criterion ids to their use cases (`UC-001.3` → `UC-001`), deduplicated in
// first-seen order: a refactor inside a use case can break any of its
// criteria. An id of any other shape is an error, not dropped: dropping it
// would narrow the run past what the source named.
func useCaseSelection(raw string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, id := range splitUseCaseIDs(raw) {
		m := useCaseSelectionPattern.FindStringSubmatch(id)
		if m == nil {
			return nil, fmt.Errorf("%q is not a use case id (UC-NNN) or criterion id (UC-NNN.M)", id)
		}
		if !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out, nil
}

// resolveTestUseCases is RESOLVE_TEST_USE_CASES, the first node of
// refactor-system-structure and refactor-test-structure. It stamps
// refactor-test-use-cases, which the cycle's verify call binds as
// test-use-cases:
//
//   - Called by the Code & Architecture Review (EXECUTE_*_PLAN binds
//     test-use-cases to the review plan's use cases): the review plan's use
//     cases, even when empty — a cross-cutting plan naming none runs the
//     whole suite.
//   - Otherwise (the refactor menu, inside a story or not, or a refactor
//     ticket): the story's merged-criteria, reduced to their use cases. A
//     bug, a refactor ticket or an ad-hoc refactor has none: whole suite.
func (a actions) resolveTestUseCases(ctx *statemachine.Context) statemachine.Outcome {
	raw, source := ctx.GetString(CtxKeyMergedCriteria), "the story"
	if v, ok := ctx.Params[ParamTestUseCases]; ok {
		raw, source = v, "the review plan"
	}
	ids, err := useCaseSelection(raw)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("resolve-test-use-cases: %s: %w", source, err)}
	}
	ctx.Set(CtxKeyRefactorTestUseCases, strings.Join(ids, ","))
	if len(ids) == 0 {
		fmt.Fprintln(a.deps.Out.Phase, "No use case known for this refactor; verifying on the whole suite.")
	} else {
		fmt.Fprintf(a.deps.Out.Phase, "Verifying this refactor on the tests of %s (from %s).\n", strings.Join(ids, ", "), source)
	}
	return statemachine.Outcome{}
}
