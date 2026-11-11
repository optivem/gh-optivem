package actions

import (
	"errors"
	"fmt"
	"strings"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// State keys of the Code & Architecture Review (plan 20261109-1259).
//
// The reviewer MID (review-code-architecture) declares the first four, and
// CtxKeyRefactorUseCaseIDs (test_use_cases.go), as outputs;
// validate-outputs-and-scopes flattens them into ctx.State.
// CtxKeyRefactoringPlanSummary is appended to APPROVE_PLAN's question.
// CtxKeyHumanReviewIssues is what the human found at HUMAN_REVIEW, one issue
// per line, handed to the guidelines-updater as ${human-review-issues}.
const (
	CtxKeyRefactoringNeeded       = "refactoring-needed"
	CtxKeySystemRefactoringPlan   = "system-refactoring-plan"
	CtxKeyTestRefactoringPlan     = "test-refactoring-plan"
	CtxKeyCodeReviewFindings      = "findings"
	CtxKeyRefactoringPlanSummary  = "refactoring-plan-summary"
	CtxKeyHumanReviewIssues       = "human-review-issues"
	maxHumanReviewIssuesPerRound  = 50
	humanReviewIssuePromptPattern = "Issue %d you found in the refactored code (empty line = no more issues): "
)

// resetCodeReview is review-code-and-architecture's RESET_CODE_REVIEW, its
// first node. The reviewer's outputs are optional apart from
// refactoring-needed, and ctx.State is run-wide, so a second review in the
// same run (the refactor menu loops) would otherwise inherit the first
// review's plans, and a reviewer that emits nothing would pass validation on
// the leftover refactoring-needed.
func (a actions) resetCodeReview(ctx *statemachine.Context) statemachine.Outcome {
	for _, k := range []string{
		CtxKeyRefactoringNeeded,
		CtxKeySystemRefactoringPlan,
		CtxKeyTestRefactoringPlan,
		CtxKeyCodeReviewFindings,
		CtxKeyRefactoringPlanSummary,
		CtxKeyHumanReviewIssues,
		CtxKeyRefactorUseCaseIDs,
	} {
		ctx.Unset(k)
	}
	return statemachine.Outcome{}
}

// summarizeRefactoringPlan is SUMMARIZE_REFACTORING_PLAN, after the reviewer
// and before GATE_REFACTORING_NEEDED. It renders the findings and both plan
// halves into refactoring-plan-summary for APPROVE_PLAN's question, and fails
// loudly when the verdict and the plans disagree: refactoring-needed=true with
// no plan would approve nothing, and a plan with refactoring-needed=false
// would be dropped silently.
//
// It also normalises refactor-use-case-ids (the use cases the plan touches)
// to deduplicated UC ids and always stamps it, "" when the plan names none:
// the refactor cycles and the guideline round verify on those use cases'
// tests, and on the whole suite when it is empty. A malformed id, or use
// cases with no plan, fails loudly like the verdict/plan mismatch.
func (a actions) summarizeRefactoringPlan(ctx *statemachine.Context) statemachine.Outcome {
	needed := ctx.GetString(CtxKeyRefactoringNeeded) == "true"
	system := ctx.GetString(CtxKeySystemRefactoringPlan)
	test := ctx.GetString(CtxKeyTestRefactoringPlan)
	hasPlan := strings.TrimSpace(system) != "" || strings.TrimSpace(test) != ""
	if needed && !hasPlan {
		return statemachine.Outcome{Err: errors.New("summarize-refactoring-plan: the reviewer reported refactoring-needed=true but emitted neither system-refactoring-plan nor test-refactoring-plan")}
	}
	if !needed && hasPlan {
		return statemachine.Outcome{Err: errors.New("summarize-refactoring-plan: the reviewer reported refactoring-needed=false but emitted a refactoring plan")}
	}
	useCases, err := useCaseSelection(ctx.GetString(CtxKeyRefactorUseCaseIDs))
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("summarize-refactoring-plan: %s: %w", CtxKeyRefactorUseCaseIDs, err)}
	}
	if !needed && len(useCases) > 0 {
		return statemachine.Outcome{Err: fmt.Errorf("summarize-refactoring-plan: the reviewer reported refactoring-needed=false but named use cases in %s", CtxKeyRefactorUseCaseIDs)}
	}
	ctx.Set(CtxKeyRefactorUseCaseIDs, strings.Join(useCases, ","))
	ctx.Set(CtxKeyRefactoringPlanSummary, renderRefactoringPlanSummary(ctx.GetString(CtxKeyCodeReviewFindings), system, test, useCases))
	return statemachine.Outcome{}
}

// renderRefactoringPlanSummary formats the review for the approval question.
func renderRefactoringPlanSummary(findings, system, test string, useCases []string) string {
	var b strings.Builder
	for _, section := range []struct{ title, body string }{
		{"Findings", findings},
		{"System refactoring plan", system},
		{"Test refactoring plan", test},
	} {
		b.WriteString("\n\n" + section.title + ":\n")
		if !writeLines(&b, section.body) {
			b.WriteString("  none\n")
		}
	}
	verifiedOn := "the whole suite"
	if len(useCases) > 0 {
		verifiedOn = "the tests of " + strings.Join(useCases, ", ")
	}
	b.WriteString("\n\nVerified on: " + verifiedOn)
	return strings.TrimRight(b.String(), "\n")
}

// writeLines writes one "  - line" entry per non-blank line and reports
// whether it wrote any.
func writeLines(b *strings.Builder, body string) bool {
	wrote := false
	for line := range strings.SplitSeq(body, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			b.WriteString("  - " + strings.TrimLeft(line, "-* ") + "\n")
			wrote = true
		}
	}
	return wrote
}

// recordHumanReviewIssues is HUMAN_REVIEW. It asks the human for the issues
// they still find in the refactored code, one per prompt, until an empty
// line, and stores them one per line as human-review-issues. No issues
// unsets the key, which GATE_HUMAN_ISSUES reads as "done".
func (a actions) recordHumanReviewIssues(ctx *statemachine.Context) statemachine.Outcome {
	var issues []string
	for len(issues) < maxHumanReviewIssuesPerRound {
		answer, err := a.deps.Prompter.Ask(fmt.Sprintf(humanReviewIssuePromptPattern, len(issues)+1))
		if err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("record-human-review-issues: read issue: %w", err)}
		}
		answer = strings.TrimSpace(answer)
		if answer == "" {
			break
		}
		issues = append(issues, answer)
	}
	if len(issues) == 0 {
		ctx.Unset(CtxKeyHumanReviewIssues)
		return statemachine.Outcome{}
	}
	ctx.Set(CtxKeyHumanReviewIssues, strings.Join(issues, "\n"))
	return statemachine.Outcome{}
}
