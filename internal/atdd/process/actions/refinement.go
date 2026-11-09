package actions

import (
	"fmt"
	"strings"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// State keys of the Align loop in refine-backlog-item (plan 20261109-1255).
//
// CtxKeyOpenQuestions is the refiner's optional `open-questions` output: what
// it could not settle alone (a rule without an agreed example, a contradiction
// with a use case). CtxKeyRefinementFeedback is what the next refiner round
// reads as ${refinement-feedback}.
const (
	CtxKeyOpenQuestions      = "open-questions"
	CtxKeyRefinementFeedback = "refinement-feedback"
)

// refinementFeedbackPrompt is asked after the Three Amigos answer "no" at the
// refiner's POST approval.
const refinementFeedbackPrompt = "The Three Amigos have not agreed yet. What should change in the acceptance criteria? (one line; empty re-runs on the refiner's open questions only): "

// recordRefinementFeedback is refine-backlog-item's RECORD_REFINEMENT_FEEDBACK
// service task, on GATE_AC_AGREED's "not agreed" branch before the loop back
// to REFINE_ACCEPTANCE_CRITERIA. It asks the operator what should change and
// stores that answer, with the open questions the refiner raised this round,
// as the next round's refinement-feedback.
//
// open-questions is cleared: it is an optional output, so a value left over
// from this round would otherwise be reported again after a round that raised
// none.
func (a actions) recordRefinementFeedback(ctx *statemachine.Context) statemachine.Outcome {
	answer, err := a.deps.Prompter.Ask(refinementFeedbackPrompt)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("record-refinement-feedback: read feedback: %w", err)}
	}
	ctx.Set(CtxKeyRefinementFeedback, renderRefinementFeedback(answer, ctx.GetString(CtxKeyOpenQuestions)))
	ctx.Unset(CtxKeyOpenQuestions)
	return statemachine.Outcome{}
}

// reportUnresolvedRefinement is refine-backlog-item's
// REPORT_UNRESOLVED_REFINEMENT service task, reached when the loop cap on
// REFINE_ACCEPTANCE_CRITERIA fires. It prints what is still unresolved (the
// last round's feedback and open questions) so the halt at
// AC_REFINEMENT_EXHAUSTED names it.
func (a actions) reportUnresolvedRefinement(ctx *statemachine.Context) statemachine.Outcome {
	fmt.Fprintln(a.deps.Stderr, "refine-backlog-item: the Three Amigos did not agree on the acceptance criteria within the refinement round cap. Unresolved:")
	for line := range strings.SplitSeq(ctx.GetString(CtxKeyRefinementFeedback), "\n") {
		fmt.Fprintln(a.deps.Stderr, "  "+line)
	}
	return statemachine.Outcome{}
}

// renderRefinementFeedback formats one round's feedback for the refiner.
func renderRefinementFeedback(answer, openQuestions string) string {
	var b strings.Builder
	b.WriteString("Three Amigos feedback: ")
	if answer = strings.TrimSpace(answer); answer != "" {
		b.WriteString(answer)
	} else {
		b.WriteString("(none given)")
	}
	b.WriteString("\nOpen questions from the previous round:")
	wrote := false
	for line := range strings.SplitSeq(openQuestions, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			b.WriteString("\n- " + strings.TrimLeft(line, "-* "))
			wrote = true
		}
	}
	if !wrote {
		b.WriteString(" none")
	}
	return b.String()
}
