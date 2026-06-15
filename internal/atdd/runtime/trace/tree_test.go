package trace

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// treeDeps wires a TreeWriter that emits to buf while the live colored stream
// is discarded — the tree is what these tests assert on.
func treeDeps(buf *bytes.Buffer) Deps {
	return Deps{Out: io.Discard, Tree: NewTreeWriter(buf)}.withDefaults()
}

func TestTree_NestsChildrenUnderCallActivity(t *testing.T) {
	prevNow := nowFn
	nowFn = fixedClock
	t.Cleanup(func() { nowFn = prevNow })

	var buf bytes.Buffer
	deps := treeDeps(&buf)

	child := wrap(statemachine.Node{
		ID:   "WRITE_AT",
		Kind: statemachine.UserTask,
		Raw:  statemachine.RawNode{Agent: "acceptance-test-writer"},
		Fn:   func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} },
	}, deps)
	parent := wrap(statemachine.Node{
		ID:   "AT_CYCLE",
		Kind: statemachine.CallActivity,
		Raw:  statemachine.RawNode{Process: "acceptance-cycle"},
		Fn: func(ctx *statemachine.Context) statemachine.Outcome {
			child(ctx)
			return statemachine.Outcome{}
		},
	}, deps)

	parent(statemachine.NewContext())

	got := buf.String()
	// Parent header at depth 0 (no indent); child header at depth 1 (2 spaces).
	wantSubs := []string{
		"> AT_CYCLE  kind=call-activity process=acceptance-cycle",
		"  > WRITE_AT  kind=user-task agent=acceptance-test-writer",
		"  agent: acceptance-test-writer",
	}
	for _, s := range wantSubs {
		if !strings.Contains(got, s) {
			t.Errorf("tree missing %q\nfull output:\n%s", s, got)
		}
	}
	// The child's lines must be indented; the parent's must not.
	lines := strings.Split(got, "\n")
	for _, l := range lines {
		if strings.Contains(l, "WRITE_AT") && !strings.HasPrefix(l, "  ") {
			t.Errorf("child line should be indented under the call-activity; got %q", l)
		}
		if strings.Contains(l, "> AT_CYCLE") && strings.HasPrefix(l, " ") {
			t.Errorf("call-activity header should sit at the root; got %q", l)
		}
	}
}

func TestTree_AnnotatesLoopBackRetryWithinScope(t *testing.T) {
	prevNow := nowFn
	nowFn = fixedClock
	t.Cleanup(func() { nowFn = prevNow })

	var buf bytes.Buffer
	deps := treeDeps(&buf)

	child := wrap(statemachine.Node{
		ID:   "RUN_TESTS",
		Kind: statemachine.ServiceTask,
		Raw:  statemachine.RawNode{Action: "run_tests"},
		Fn:   func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} },
	}, deps)
	// One call-activity that dispatches the same node twice — a loop-back.
	parent := wrap(statemachine.Node{
		ID:   "CT_CYCLE",
		Kind: statemachine.CallActivity,
		Raw:  statemachine.RawNode{Process: "contract-cycle"},
		Fn: func(ctx *statemachine.Context) statemachine.Outcome {
			child(ctx)
			child(ctx) // re-dispatch within the same scope instance
			return statemachine.Outcome{}
		},
	}, deps)

	parent(statemachine.NewContext())

	got := buf.String()
	if !strings.Contains(got, "↻ retry 2") {
		t.Errorf("expected the second dispatch of RUN_TESTS to be annotated `↻ retry 2`; got:\n%s", got)
	}
	if strings.Count(got, "↻ retry") != 1 {
		t.Errorf("only the second dispatch should carry a retry marker; got %d markers:\n%s", strings.Count(got, "↻ retry"), got)
	}
}

func TestTree_SameNodeInDistinctScopesIsNotRetry(t *testing.T) {
	prevNow := nowFn
	nowFn = fixedClock
	t.Cleanup(func() { nowFn = prevNow })

	var buf bytes.Buffer
	deps := treeDeps(&buf)

	child := wrap(statemachine.Node{
		ID:   "COMMIT",
		Kind: statemachine.ServiceTask,
		Raw:  statemachine.RawNode{Action: "commit_phase"},
		Fn:   func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} },
	}, deps)
	mkParent := func(id string) statemachine.NodeFn {
		return wrap(statemachine.Node{
			ID:   id,
			Kind: statemachine.CallActivity,
			Raw:  statemachine.RawNode{Process: "cycle"},
			Fn: func(ctx *statemachine.Context) statemachine.Outcome {
				child(ctx)
				return statemachine.Outcome{}
			},
		}, deps)
	}

	ctx := statemachine.NewContext()
	mkParent("CYCLE_A")(ctx)
	mkParent("CYCLE_B")(ctx)

	got := buf.String()
	// COMMIT fires once per distinct call-activity invocation → fresh scope
	// instance each time → never a retry.
	if strings.Contains(got, "↻ retry") {
		t.Errorf("the same node in two distinct sub-process invocations must not read as a retry; got:\n%s", got)
	}
}

func TestTree_EmitsInOutAndCmdLines(t *testing.T) {
	prevNow := nowFn
	nowFn = fixedClock
	t.Cleanup(func() { nowFn = prevNow })

	var buf bytes.Buffer
	tw := NewTreeWriter(&buf)
	deps := Deps{Out: io.Discard, Tree: tw}.withDefaults()

	node := wrap(statemachine.Node{
		ID:   "RUN_TESTS",
		Kind: statemachine.ServiceTask,
		Raw:  statemachine.RawNode{Action: "run_tests"},
		Fn: func(ctx *statemachine.Context) statemachine.Outcome {
			ctx.Set("command-line", "gh optivem test run --suite=acceptance")
			ctx.Set("command-succeeded", false)
			ctx.Set("test-outcome", "fail")
			return statemachine.Outcome{Value: "red"}
		},
	}, deps)

	ctx := statemachine.NewContext()
	ctx.Params["change_type"] = "behavior"
	node(ctx)

	got := buf.String()
	wantSubs := []string{
		"  in: change_type=behavior",
		"out: RED",
		"cmd: gh optivem test run --suite=acceptance  [RED]",
	}
	for _, s := range wantSubs {
		if !strings.Contains(got, s) {
			t.Errorf("tree missing %q\nfull output:\n%s", s, got)
		}
	}
}

func TestTree_InfraHaltRendersDiagnosticInline(t *testing.T) {
	prevNow := nowFn
	nowFn = fixedClock
	t.Cleanup(func() { nowFn = prevNow })

	var buf bytes.Buffer
	deps := treeDeps(&buf)
	node := wrap(statemachine.Node{
		ID:   "TESTS_INFRA_HALT",
		Kind: statemachine.ErrorEndEvent,
		Fn:   func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} },
	}, deps)

	ctx := statemachine.NewContext()
	ctx.Set("test-infra-label", "missing executable")
	ctx.Set("command-line", "gh optivem test run --test=foo")
	ctx.Set("command-stderr-tail", "line one\nline two")
	node(ctx)

	got := buf.String()
	for _, s := range []string{
		"out: HALT — infra failure: missing executable",
		"command: gh optivem test run --test=foo",
		"stderr tail: line one",
		"line two",
	} {
		if !strings.Contains(got, s) {
			t.Errorf("infra-halt tree missing %q\nfull output:\n%s", s, got)
		}
	}
}

func TestTree_HeaderAndFooter(t *testing.T) {
	var buf bytes.Buffer
	tw := NewTreeWriter(&buf)

	tw.WriteHeader(TreeHeader{
		RunTimestamp: "20260604-120000",
		RepoPath:     "/repo",
		Process:      "main",
		IssueNum:     42,
	})
	// Two exits so the footer step count is non-trivial.
	tw.steps = 2
	tw.WriteFooter(TreeFooter{
		Result:     errors.New("process \"main\" reached error end event"),
		WallClock:  90 * time.Second,
		CommitSHA:  "abc1234",
		Dispatches: 3,
		RunDir:     "/repo/.gh-optivem/runs/20260604-120000",
		LogFile:    "/tmp/run.log",
	})

	got := buf.String()
	for _, s := range []string{
		"=== execution flow: 20260604-120000 ===",
		"issue:   #42",
		"=== result: halted — process \"main\" reached error end event ===",
		"wall-clock: 1m30s",
		"commit:     abc1234",
		"steps:      2",
		"dispatches: 3",
		"prompt logs:",
		"log file:   /tmp/run.log",
	} {
		if !strings.Contains(got, s) {
			t.Errorf("footer/header missing %q\nfull output:\n%s", s, got)
		}
	}
}

func TestTree_NilWriterLeavesLiveStreamUnchanged(t *testing.T) {
	// A nil Tree must be a no-op: the live stream renders exactly as before.
	prevNow := nowFn
	nowFn = fixedClock
	t.Cleanup(func() { nowFn = prevNow })

	var buf bytes.Buffer
	node := wrap(statemachine.Node{
		ID:   "MARK_IN_PROGRESS",
		Kind: statemachine.ServiceTask,
		Raw:  statemachine.RawNode{Action: "move-to-in-progress"},
		Fn:   func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} },
	}, Deps{Out: &buf}.withDefaults()) // Tree left nil

	node(statemachine.NewContext())

	got := buf.String()
	if !strings.Contains(got, "> MARK_IN_PROGRESS  kind=service-task action=move-to-in-progress") {
		t.Errorf("live stream should render normally with a nil Tree; got:\n%s", got)
	}
}
