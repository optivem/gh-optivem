// fix_progress.go — no-progress guard for the verify-tests-pass fix loop.
//
// The verify-tests-pass loop (RUN_TESTS → GATE_TESTS_OUTCOME → fail →
// FIX_UNEXPECTED_FAILING_TESTS → RUN_TESTS) is bounded by a *count* cap
// (max-visits: 2 → FIX_LOOP_EXHAUSTED). A count cap cannot distinguish a
// fixer that is making progress but not yet done from one that is
// spinning — editing without changing the failure at all. This action
// adds the behaviour-based half (plan 20260615-1845 Step 4): it
// fingerprints each failing test run and halts the loop the moment two
// consecutive runs fail identically, i.e. the last fixer pass changed
// nothing about the failure.
//
// It runs on the gateway's fail branch, BEFORE re-dispatching the fixer,
// and stamps ctx.State["fix-loop-progressing"] for the downstream
// GATE_FIX_PROGRESSING gateway:
//
//   - first fail of the loop (no prior signature) → record + progressing
//   - signature changed since the previous fail   → record + progressing
//   - signature identical to the previous fail     → NOT progressing (halt)
//
// The prior signature lives in ctx.State["fix-prev-failure-signature"];
// run-command clears it on any successful test run (a green loop), so a
// later verify-tests-pass invocation in the same run starts fresh rather
// than comparing against a stale signature from an earlier loop.
//
// Complements — does not replace — the two existing bounds: the
// max-visits count cap (FIX_LOOP_EXHAUSTED) still backstops a loop that
// keeps changing the failure without ever greening it, and the fixer's
// own exit-on-uncertainty self-bail (plan Step 3) still lets a single
// pass hand back. This guard catches the case both miss: a fixer that
// keeps editing, never bails, and never moves the failure.
package actions

import (
	"regexp"
	"strings"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// State keys the no-progress guard reads/writes: the prior failing-run
// signature it compares against, and the bool GATE_FIX_PROGRESSING routes on.
const (
	CtxKeyFixPrevFailureSignature = "fix-prev-failure-signature"
	CtxKeyFixLoopProgressing      = "fix-loop-progressing"
)

// checkFixProgress is the verify-tests-pass no-progress guard
// (CHECK_FIX_PROGRESS service task). See the file header for the loop it
// guards and the routing contract. Never surfaces Outcome.Err: a missing
// or empty failure payload fails safe (treated as "can't tell → let the
// fixer run"), deferring to the max-visits count cap rather than halting
// on uncertainty.
func (a actions) checkFixProgress(ctx *statemachine.Context) statemachine.Outcome {
	cur := fixFailureSignature(ctx.GetString("verify_failure_output"))
	prev := ctx.GetString(CtxKeyFixPrevFailureSignature)

	// Indeterminate (the failing run produced no capturable output) or the
	// first fail of this loop (no prior signature to compare against):
	// record what we have and let the fixer run. An empty current signature
	// never halts — we defer to the existing count cap rather than guess.
	if cur == "" || prev == "" {
		if cur != "" {
			ctx.Set(CtxKeyFixPrevFailureSignature, cur)
		}
		ctx.Set(CtxKeyFixLoopProgressing, true)
		return statemachine.Outcome{}
	}

	if cur == prev {
		// Two consecutive failing runs with an identical signature: the last
		// fixer pass did not change the failure at all. Halt for a human
		// rather than spend another opus·high pass on a spinning loop.
		ctx.Set(CtxKeyFixLoopProgressing, false)
		return statemachine.Outcome{}
	}

	// The failure changed — the fixer moved the needle. Advance the
	// baseline and let the loop continue.
	ctx.Set(CtxKeyFixPrevFailureSignature, cur)
	ctx.Set(CtxKeyFixLoopProgressing, true)
	return statemachine.Outcome{}
}

// fixFailureSignature normalises a verify_failure_output payload into a
// stable comparison key: it strips the volatile tokens that differ
// between two runs of the *same* failing tests (durations, wall-clock
// times, ANSI colour, cosmetic whitespace) so an unchanged failure
// compares equal across passes while genuinely different failures still
// differ.
//
// The error budget is deliberately one-sided. A false "equal" — halting a
// loop that was actually progressing — would only happen if normalisation
// stripped something that carried failure identity; the tokens removed
// here (timings, colour codes, surrounding whitespace) never do. The
// opposite error — a missed volatile token making two identical failures
// look different — merely defers to the max-visits count cap, costing one
// extra pass. So the function leans conservative: strip only what is
// unambiguously volatile.
func fixFailureSignature(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	s = ansiSeq.ReplaceAllString(s, "")
	s = clockToken.ReplaceAllString(s, "<time>")
	// Applied AFTER clockToken so a full HH:MM:SS clock is already "<time>"
	// and the MM:SS shape can't eat its "MM:SS" tail.
	s = suiteDurationToken.ReplaceAllString(s, "<suite-dur>")
	s = durationToken.ReplaceAllString(s, "<dur>")
	// Collapse each line's internal whitespace and drop blank lines so
	// cosmetic reflow doesn't perturb the key.
	lines := strings.Split(s, "\n")
	kept := lines[:0]
	for _, ln := range lines {
		ln = strings.TrimSpace(whitespaceRun.ReplaceAllString(ln, " "))
		if ln != "" {
			kept = append(kept, ln)
		}
	}
	return strings.Join(kept, "\n")
}

var (
	// An ANSI SGR colour/reset sequence ("\x1b[31m", "\x1b[0m").
	ansiSeq = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	// A wall-clock time, matched before durations so "07:10:29" isn't
	// partially eaten by the duration rule ("29" is not unit-suffixed, so it
	// would survive anyway, but ordering keeps the intent explicit).
	clockToken = regexp.MustCompile(`\b\d{1,2}:\d{2}:\d{2}(\.\d+)?\b`)
	// The runner's Suite-Results duration column, MM:SS with an optional
	// millisecond fraction ("00:02.158", "01:04"). This is the timing column
	// in lines like "latest - Contract (real)  FAILED  00:02.158", which
	// varies run-to-run for a byte-identical failure. Matched after
	// clockToken (above) so an HH:MM:SS clock — already "<time>" — can't be
	// re-matched on its trailing MM:SS segment.
	suiteDurationToken = regexp.MustCompile(`\b\d{1,2}:\d{2}(\.\d{1,3})?\b`)
	// A duration: one or more number+time-unit parts ("12s", "1.234 s",
	// "350ms", composite "1m 4s 300ms"). Composite runs collapse to a single
	// <dur> so "1m 4s" and "12s" — the same build, different wall-clock —
	// compare equal. Word-boundary-anchored so it cannot eat a unit-less
	// number (a test name like "...Of100") or a version fragment.
	durationToken = regexp.MustCompile(`(?i)\b\d+(\.\d+)?\s?(ms|s|m|h)\b(\s+\d+(\.\d+)?\s?(ms|s|m|h)\b)*`)
	// A run of whitespace, collapsed to a single space per line.
	whitespaceRun = regexp.MustCompile(`\s+`)
)
