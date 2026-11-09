package process_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/process"
	"github.com/optivem/gh-optivem/internal/atdd/process/actions"
	"github.com/optivem/gh-optivem/internal/atdd/process/gates"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// scriptedPrompter answers every Ask with the same line and counts the calls.
type scriptedPrompter struct {
	answer string
	asks   int
}

func (p *scriptedPrompter) Ask(string) (string, error) {
	p.asks++
	return p.answer + "\n", nil
}

// refineLoopRun is the observable trail of one refine-backlog-item walk with
// the Align loop (plan 20261109-1255).
type refineLoopRun struct {
	refinerCalls  int
	feedbackSeen  []string // refinement-feedback at each refiner dispatch
	feedbackAsks  int
	stderr        string
	finalFeedback string
	err           error
}

// runRefineLoop drives refine-backlog-item with a synthetic refiner that
// always raises one open question. The Three Amigos decline the refiner's
// POST approval on the first `disagreeRounds` rounds and agree after. The
// two Align gates and the two Align actions are the real implementations, so
// the test pins the YAML wiring and the Go code together. Loop-hazard audit:
// the reachable back-edges are FIX -> RUN_AGENT (never taken: validation
// passes), REQUEST_REVISION -> RUN_AGENT (never taken: no reviewer) and
// RECORD_REFINEMENT_FEEDBACK -> REFINE_ACCEPTANCE_CRITERIA (max-visits 5), so
// the walk is bounded for any stub verdict.
func runRefineLoop(t *testing.T, disagreeRounds int) refineLoopRun {
	t.Helper()
	eng, err := process.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	prompter := &scriptedPrompter{answer: "the fee applies per order, not per item"}
	var stderr bytes.Buffer
	realActions := actions.New()
	actions.RegisterAll(realActions, actions.Deps{Stdout: &bytes.Buffer{}, Stderr: &stderr, Prompter: prompter})
	realGates := gates.New()
	gates.RegisterAll(realGates, gates.Deps{})

	var run refineLoopRun
	eng.ActionFn = func(name string) statemachine.NodeFn {
		switch name {
		case "validate-outputs-and-scopes":
			return func(ctx *statemachine.Context) statemachine.Outcome {
				ctx.Set("outputs-and-scopes-valid", true)
				return statemachine.Outcome{}
			}
		case "record-refinement-feedback", "report-unresolved-refinement":
			return realActions.Lookup(name)
		default:
			return func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} }
		}
	}
	eng.AgentFn = func(name string) statemachine.NodeFn {
		n := name
		return func(ctx *statemachine.Context) statemachine.Outcome {
			switch n {
			case "acceptance-criteria-refiner":
				run.refinerCalls++
				run.feedbackSeen = append(run.feedbackSeen, ctx.GetString("refinement-feedback"))
				ctx.Set("open-questions", "Is the fee waived for orders over $100?")
			case "human":
				outcome := "approved"
				if strings.HasPrefix(ctx.Params["question"], "Approve output") && run.refinerCalls <= disagreeRounds {
					outcome = "rejected"
				}
				ctx.Set("approval-outcome", outcome)
			}
			return statemachine.Outcome{}
		}
	}
	eng.GateFn = func(name string) statemachine.NodeFn {
		switch name {
		case "approval-outcome", "return-on-post-rejection", "reviewer-configured":
			return realGates.Lookup(name)
		default:
			return capLoopGateFn()(name)
		}
	}
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	ctx := statemachine.NewContext()
	run.err = eng.RunProcess("refine-backlog-item", ctx)
	run.feedbackAsks = prompter.asks
	run.stderr = stderr.String()
	run.finalFeedback = ctx.GetString("refinement-feedback")
	return run
}

// TestRefineBacklogItem_AgreedFirstRound: the Three Amigos agree on the
// first draft — one refiner dispatch, no feedback prompt, no feedback shown.
func TestRefineBacklogItem_AgreedFirstRound(t *testing.T) {
	run := runRefineLoop(t, 0)
	if run.err != nil {
		t.Fatalf("RunProcess: %v", run.err)
	}
	if run.refinerCalls != 1 || run.feedbackAsks != 0 {
		t.Errorf("refiner=%d asks=%d, want 1/0", run.refinerCalls, run.feedbackAsks)
	}
	if run.feedbackSeen[0] != "" {
		t.Errorf("first round saw feedback %q, want none", run.feedbackSeen[0])
	}
}

// TestRefineBacklogItem_LoopsUntilAgreed: a "no" at the refiner's POST
// approval returns instead of halting; the next round sees the operator's
// feedback and the previous round's open questions; the loop ends on "yes".
func TestRefineBacklogItem_LoopsUntilAgreed(t *testing.T) {
	run := runRefineLoop(t, 2)
	if run.err != nil {
		t.Fatalf("RunProcess: %v", run.err)
	}
	if run.refinerCalls != 3 || run.feedbackAsks != 2 {
		t.Errorf("refiner=%d asks=%d, want 3/2", run.refinerCalls, run.feedbackAsks)
	}
	for _, want := range []string{"per order, not per item", "- Is the fee waived for orders over $100?"} {
		if !strings.Contains(run.feedbackSeen[1], want) {
			t.Errorf("second round feedback missing %q:\n%s", want, run.feedbackSeen[1])
		}
	}
}

// TestRefineBacklogItem_RoundCapHalts: Three Amigos that never agree get
// exactly 5 refinement rounds, then the run halts at AC_REFINEMENT_EXHAUSTED
// with the unresolved questions printed.
func TestRefineBacklogItem_RoundCapHalts(t *testing.T) {
	run := runRefineLoop(t, 99)
	if run.err == nil {
		t.Fatal("RunProcess returned nil; want halt at AC_REFINEMENT_EXHAUSTED")
	}
	if !strings.Contains(run.err.Error(), "AC_REFINEMENT_EXHAUSTED") {
		t.Fatalf("error = %q, want it to reference AC_REFINEMENT_EXHAUSTED", run.err.Error())
	}
	if run.refinerCalls != 5 {
		t.Errorf("refiner=%d, want 5 rounds", run.refinerCalls)
	}
	for _, want := range []string{"did not agree", "per order, not per item", "Is the fee waived"} {
		if !strings.Contains(run.stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, run.stderr)
		}
	}
}

// TestExecuteAgent_PostRejectionStillHalts: without return-on-post-rejection
// (every call site but refine-acceptance-criteria), a rejected output still
// halts at EXECUTE_AGENT_OUTPUT_REJECTED_END.
func TestExecuteAgent_PostRejectionStillHalts(t *testing.T) {
	eng, err := process.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	realGates := gates.New()
	gates.RegisterAll(realGates, gates.Deps{})
	eng.ActionFn = func(name string) statemachine.NodeFn {
		return func(ctx *statemachine.Context) statemachine.Outcome {
			if name == "validate-outputs-and-scopes" {
				ctx.Set("outputs-and-scopes-valid", true)
			}
			return statemachine.Outcome{}
		}
	}
	eng.AgentFn = func(name string) statemachine.NodeFn {
		return func(ctx *statemachine.Context) statemachine.Outcome {
			if name == "human" {
				outcome := "approved"
				if strings.HasPrefix(ctx.Params["question"], "Approve output") {
					outcome = "rejected"
				}
				ctx.Set("approval-outcome", outcome)
			}
			return statemachine.Outcome{}
		}
	}
	eng.GateFn = func(name string) statemachine.NodeFn {
		switch name {
		case "approval-outcome", "return-on-post-rejection", "reviewer-configured":
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
	err = eng.RunProcess("execute-agent", ctx)
	if err == nil || !strings.Contains(err.Error(), "EXECUTE_AGENT_OUTPUT_REJECTED_END") {
		t.Fatalf("RunProcess err = %v, want halt at EXECUTE_AGENT_OUTPUT_REJECTED_END", err)
	}
}
