package process_test

import (
	"io"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/process"
	"github.com/optivem/gh-optivem/internal/atdd/process/actions"
	"github.com/optivem/gh-optivem/internal/atdd/process/gates"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// reviewLoopRun is the observable trail of one execute-agent walk with the
// Red reviewer loop (plan 20261109-1257).
type reviewLoopRun struct {
	writerCalls   int
	reviewerCalls int
	questions     []string // every approve question, in order
	revisionSeen  []string // review-revision-findings at each writer dispatch
	err           error
}

// runReviewLoop drives execute-agent directly with a synthetic writer and the
// real review-acceptance-tests MID. Validation always passes; the reviewer
// reports blocking findings on its first `blockingPasses` dispatches and a
// clean verdict after. The two reviewer gates and the two review actions are
// the real implementations, so the test pins the YAML wiring and the Go
// bindings together. Loop-hazard audit: the only back-edges reachable are
// FIX -> RUN_AGENT (never taken: validation passes) and REQUEST_REVISION ->
// RUN_AGENT (max-visits 2), so the walk is bounded for any stub verdict.
func runReviewLoop(t *testing.T, reviewer string, blockingPasses int, declineReviewer bool) reviewLoopRun {
	t.Helper()
	eng, err := process.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	realActions := actions.New()
	actions.RegisterAll(realActions, actions.Deps{Stdout: io.Discard, Stderr: io.Discard})
	realGates := gates.New()
	gates.RegisterAll(realGates, gates.Deps{})

	var run reviewLoopRun
	lastAgent := ""
	eng.ActionFn = func(name string) statemachine.NodeFn {
		switch name {
		case "validate-outputs-and-scopes":
			return func(ctx *statemachine.Context) statemachine.Outcome {
				ctx.Set("outputs-and-scopes-valid", true)
				if lastAgent == "acceptance-test-reviewer" {
					blocking := run.reviewerCalls <= blockingPasses
					ctx.Set("review-has-blocking-findings", blocking)
					if blocking {
						ctx.Set("blocking-findings", "FooTest.java: missing .then().shouldSucceed()")
					}
					ctx.Set("non-blocking-findings", "FooTest.java: rename helper")
				}
				return statemachine.Outcome{}
			}
		case "request-review-revision", "summarize-review":
			return realActions.Lookup(name)
		default:
			return func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} }
		}
	}
	eng.AgentFn = func(name string) statemachine.NodeFn {
		n := name
		return func(ctx *statemachine.Context) statemachine.Outcome {
			lastAgent = n
			switch n {
			case "synthetic-writer":
				run.writerCalls++
				run.revisionSeen = append(run.revisionSeen, ctx.GetString("review-revision-findings"))
			case "acceptance-test-reviewer":
				run.reviewerCalls++
			case "human":
				q := ctx.Params["question"]
				run.questions = append(run.questions, q)
				outcome := "approved"
				if declineReviewer && strings.Contains(q, "agent review-acceptance-tests to execute") {
					outcome = "rejected"
				}
				ctx.Set("approval-outcome", outcome)
			}
			return statemachine.Outcome{}
		}
	}
	eng.GateFn = func(name string) statemachine.NodeFn {
		switch name {
		case "reviewer-configured", "review-has-blocking-findings", "approval-outcome":
			return realGates.Lookup(name)
		default:
			return capLoopGateFn()(name)
		}
	}
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	ctx := statemachine.NewContext()
	ctx.Params["agent"] = "synthetic-writer"
	ctx.Params["task-name"] = "synthetic-task"
	ctx.Params["category"] = "test-agent"
	if reviewer != "" {
		ctx.Params["reviewer"] = reviewer
		ctx.Params["approver-role"] = "QA and/or Developers"
	}
	run.err = eng.RunProcess("execute-agent", ctx)
	return run
}

// TestExecuteAgent_NoReviewerUnchanged: a call site without `reviewer` keeps
// today's flow — one writer dispatch, the plain approval question.
func TestExecuteAgent_NoReviewerUnchanged(t *testing.T) {
	run := runReviewLoop(t, "", 0, false)
	if run.err != nil {
		t.Fatalf("RunProcess: %v", run.err)
	}
	if run.writerCalls != 1 || run.reviewerCalls != 0 {
		t.Errorf("writer=%d reviewer=%d, want 1/0", run.writerCalls, run.reviewerCalls)
	}
	last := run.questions[len(run.questions)-1]
	if last != "Approve output from agent synthetic-task?" {
		t.Errorf("POST question = %q, want the unreviewed question", last)
	}
}

// TestExecuteAgent_ReviewerCleanVerdict: a clean review goes straight to the
// reviewed approval, which names the approver role and lists the
// non-blocking findings; the writer is not re-dispatched.
func TestExecuteAgent_ReviewerCleanVerdict(t *testing.T) {
	run := runReviewLoop(t, "review-acceptance-tests", 0, false)
	if run.err != nil {
		t.Fatalf("RunProcess: %v", run.err)
	}
	if run.writerCalls != 1 || run.reviewerCalls != 1 {
		t.Errorf("writer=%d reviewer=%d, want 1/1", run.writerCalls, run.reviewerCalls)
	}
	last := run.questions[len(run.questions)-1]
	for _, want := range []string{"Approver: QA and/or Developers.", "Non-blocking:", "rename helper"} {
		if !strings.Contains(last, want) {
			t.Errorf("POST question missing %q:\n%s", want, last)
		}
	}
	if strings.Contains(last, "Blocking (still open") {
		t.Errorf("clean verdict must not report open blocking findings:\n%s", last)
	}
}

// TestExecuteAgent_ReviewerOneRevision: one blocking verdict re-dispatches the
// writer once, with the findings, then the clean second review approves.
func TestExecuteAgent_ReviewerOneRevision(t *testing.T) {
	run := runReviewLoop(t, "review-acceptance-tests", 1, false)
	if run.err != nil {
		t.Fatalf("RunProcess: %v", run.err)
	}
	if run.writerCalls != 2 || run.reviewerCalls != 2 {
		t.Errorf("writer=%d reviewer=%d, want 2/2", run.writerCalls, run.reviewerCalls)
	}
	if run.revisionSeen[0] != "" {
		t.Errorf("first writer dispatch saw findings %q, want none", run.revisionSeen[0])
	}
	if !strings.Contains(run.revisionSeen[1], "shouldSucceed") {
		t.Errorf("re-dispatched writer saw findings %q, want the blocking finding", run.revisionSeen[1])
	}
}

// TestExecuteAgent_ReviewerRevisionCap: a reviewer that never clears its
// blocking findings gets exactly 2 revision passes (writer 3x, reviewer 3x),
// then the human approves with the open findings listed — no halt, no
// maxDispatchesPerProcess backstop.
func TestExecuteAgent_ReviewerRevisionCap(t *testing.T) {
	run := runReviewLoop(t, "review-acceptance-tests", 99, false)
	if run.err != nil {
		t.Fatalf("RunProcess: %v", run.err)
	}
	if run.writerCalls != 3 || run.reviewerCalls != 3 {
		t.Errorf("writer=%d reviewer=%d, want 3/3 (2 revision passes)", run.writerCalls, run.reviewerCalls)
	}
	last := run.questions[len(run.questions)-1]
	for _, want := range []string{"Blocking (still open after 2 revision passes):", "shouldSucceed"} {
		if !strings.Contains(last, want) {
			t.Errorf("POST question missing %q:\n%s", want, last)
		}
	}
}

// TestExecuteAgent_ReviewerDeclined: an operator declining the reviewer's own
// PRE approval skips the review (no verdict) without a halt; the writer's
// approval says the review was skipped.
func TestExecuteAgent_ReviewerDeclined(t *testing.T) {
	run := runReviewLoop(t, "review-acceptance-tests", 0, true)
	if run.err != nil {
		t.Fatalf("RunProcess: %v", run.err)
	}
	if run.writerCalls != 1 || run.reviewerCalls != 0 {
		t.Errorf("writer=%d reviewer=%d, want 1/0", run.writerCalls, run.reviewerCalls)
	}
	last := run.questions[len(run.questions)-1]
	if !strings.Contains(last, "review skipped") {
		t.Errorf("POST question must say the review was skipped:\n%s", last)
	}
}
