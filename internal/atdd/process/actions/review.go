package actions

import (
	"strings"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// State keys of the Red reviewer loop in execute-agent (plan 20261109-1257).
//
// The reviewer MIDs (review-acceptance-tests, review-dsl,
// review-system-driver-adapters) declare the first three as outputs;
// validate-outputs-and-scopes flattens them into ctx.State after the
// reviewer runs. GATE_REVIEW_FINDINGS routes on CtxKeyReviewHasBlocking.
const (
	CtxKeyReviewHasBlocking      = "review-has-blocking-findings"
	CtxKeyBlockingFindings       = "blocking-findings"
	CtxKeyNonBlockingFindings    = "non-blocking-findings"
	CtxKeyReviewRevisionFindings = "review-revision-findings"
	CtxKeyReviewSummary          = "review-summary"
)

// requestReviewRevision is execute-agent's REQUEST_REVISION service task, on
// the GATE_REVIEW_FINDINGS true branch before the loop back to RUN_AGENT. It
// hands the reviewer's blocking findings to the writer's re-dispatch (the
// driver renders review-revision-findings into the writer prompt's
// ${review-findings-section}) and clears the reviewer's verdict keys.
//
// Clearing is load-bearing: ctx.State is shared and run-wide, and
// validate-outputs-and-scopes' presence check reads ctx.State — a verdict left
// over from this pass would satisfy the next reviewer pass's required
// `review-has-blocking-findings` even if that reviewer never emitted it, and a
// stale blocking-findings would surface at approval after a clean pass.
func (a actions) requestReviewRevision(ctx *statemachine.Context) statemachine.Outcome {
	ctx.Set(CtxKeyReviewRevisionFindings, ctx.GetString(CtxKeyBlockingFindings))
	clearReviewVerdict(ctx)
	return statemachine.Outcome{}
}

// summarizeReview is execute-agent's SUMMARIZE_REVIEW service task, the last
// step of a reviewed dispatch before APPROVE_POST_REVIEWED. It renders the
// reviewer's findings into review-summary (appended to the approval question)
// and clears every review key so the next reviewed phase starts clean — its
// writer's first dispatch must not see this phase's findings.
//
// Reached either on a clean verdict (no blocking findings) or when
// REQUEST_REVISION's max-visits cap routes here after two revision passes;
// in the second case blocking findings remain and the summary says so.
func (a actions) summarizeReview(ctx *statemachine.Context) statemachine.Outcome {
	if ctx.GetString("approval-outcome") == "rejected" {
		// The operator declined the reviewer's PRE approval, so no review
		// ran (see the review-has-blocking-findings gate). Any other path
		// here passed the reviewer's POST approval ("approved"); a rejected
		// reviewer POST halts the run before this node.
		ctx.Set(CtxKeyReviewSummary, "\n\nReviewer findings: review skipped (the reviewer dispatch was declined).")
		ctx.Unset(CtxKeyReviewRevisionFindings)
		return statemachine.Outcome{}
	}
	ctx.Set(CtxKeyReviewSummary, renderReviewSummary(
		ctx.GetString(CtxKeyReviewHasBlocking) == "true",
		ctx.GetString(CtxKeyBlockingFindings),
		ctx.GetString(CtxKeyNonBlockingFindings),
	))
	clearReviewVerdict(ctx)
	ctx.Unset(CtxKeyReviewRevisionFindings)
	return statemachine.Outcome{}
}

// clearReviewVerdict unsets the reviewer's output keys.
func clearReviewVerdict(ctx *statemachine.Context) {
	ctx.Unset(CtxKeyReviewHasBlocking)
	ctx.Unset(CtxKeyBlockingFindings)
	ctx.Unset(CtxKeyNonBlockingFindings)
}

// renderReviewSummary formats the findings for the approval question. It
// always returns a non-empty block so the human sees that a review ran.
func renderReviewSummary(blocking bool, blockingFindings, nonBlockingFindings string) string {
	var b strings.Builder
	b.WriteString("\n\nReviewer findings:\n")
	if blocking {
		b.WriteString("  Blocking (still open after 2 revision passes):\n")
		writeIndentedFindings(&b, blockingFindings)
	}
	if strings.TrimSpace(nonBlockingFindings) != "" {
		b.WriteString("  Non-blocking:\n")
		writeIndentedFindings(&b, nonBlockingFindings)
	}
	if !blocking && strings.TrimSpace(nonBlockingFindings) == "" {
		b.WriteString("  none\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// writeIndentedFindings writes one "    - finding" line per non-blank line.
func writeIndentedFindings(b *strings.Builder, findings string) {
	wrote := false
	for line := range strings.SplitSeq(findings, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		b.WriteString("    - " + strings.TrimLeft(line, "-* ") + "\n")
		wrote = true
	}
	if !wrote {
		b.WriteString("    - (the reviewer reported blocking findings but listed none)\n")
	}
}
