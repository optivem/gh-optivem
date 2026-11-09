package gates

import (
	"strings"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// Gateways of the Code & Architecture Review (plan 20261109-1259,
// review-code-and-architecture in process-flow.yaml).
const (
	bindingRefactoringNeeded            = "refactoring-needed"
	bindingSystemRefactoringPlanPresent = "system-refactoring-plan-present"
	bindingTestRefactoringPlanPresent   = "test-refactoring-plan-present"
	bindingHumanReviewIssuesFound       = "human-review-issues-found"
)

// registerCodeReview wires the review's four gateways.
func registerCodeReview(r *Registry, b bindings) {
	r.Register(bindingRefactoringNeeded, b.refactoringNeeded)
	r.Register(bindingSystemRefactoringPlanPresent, b.systemRefactoringPlanPresent)
	r.Register(bindingTestRefactoringPlanPresent, b.testRefactoringPlanPresent)
	r.Register(bindingHumanReviewIssuesFound, b.humanReviewIssuesFound)
}

// refactoringNeeded is GATE_REFACTORING_NEEDED: the reviewer's required bool
// verdict, strict like the port-changed flags (missing is a bug, not "no").
func (b bindings) refactoringNeeded(ctx *statemachine.Context) statemachine.Outcome {
	return boolStateGate(ctx, "refactoring-needed")
}

// systemRefactoringPlanPresent is GATE_SYSTEM_PLAN_PRESENT: whether the
// approved plan has a system half for system-refactorer. Presence-style: the
// output is optional, and summarize-refactoring-plan already failed the run
// when refactoring-needed and the plans disagree.
func (b bindings) systemRefactoringPlanPresent(ctx *statemachine.Context) statemachine.Outcome {
	return nonBlankStateGate(ctx, "system-refactoring-plan")
}

// testRefactoringPlanPresent is GATE_TEST_PLAN_PRESENT, the test half's
// counterpart of systemRefactoringPlanPresent.
func (b bindings) testRefactoringPlanPresent(ctx *statemachine.Context) statemachine.Outcome {
	return nonBlankStateGate(ctx, "test-refactoring-plan")
}

// humanReviewIssuesFound is GATE_HUMAN_ISSUES: whether the human listed any
// issue at HUMAN_REVIEW (record-human-review-issues unsets the key when the
// first answer is empty, which ends the review).
func (b bindings) humanReviewIssuesFound(ctx *statemachine.Context) statemachine.Outcome {
	return nonBlankStateGate(ctx, "human-review-issues")
}

// nonBlankStateGate is true iff ctx.State[key] renders as a non-blank string.
func nonBlankStateGate(ctx *statemachine.Context, key string) statemachine.Outcome {
	return statemachine.Outcome{Bool: strings.TrimSpace(ctx.GetString(key)) != ""}
}
