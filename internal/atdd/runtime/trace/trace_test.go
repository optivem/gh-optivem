package trace

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
)

// fixedClock returns a deterministic time so banner output is stable across
// test runs.
func fixedClock() time.Time {
	return time.Date(2026, 5, 4, 12, 34, 56, 0, time.UTC)
}

// fakeGit is a stub GitRunner used to drive the working-tree snapshots
// without shelling out. Each call consumes the next entry of `outs` (so a
// pre/post pair returns two distinct porcelains in order). lastArgs
// records every argv for assertions.
type fakeGit struct {
	outs     [][]byte
	err      error
	lastDir  string
	lastArgs [][]string
}

func (g *fakeGit) Run(_ context.Context, dir string, args ...string) ([]byte, error) {
	g.lastDir = dir
	g.lastArgs = append(g.lastArgs, args)
	if g.err != nil {
		return nil, g.err
	}
	if len(g.outs) == 0 {
		return nil, nil
	}
	v := g.outs[0]
	g.outs = g.outs[1:]
	return v, nil
}

func TestWrap_ServiceTaskLogsEntryAndExit(t *testing.T) {
	prevNow := nowFn
	nowFn = fixedClock
	t.Cleanup(func() { nowFn = prevNow })

	var buf bytes.Buffer
	node := statemachine.Node{
		ID:   "MARK_IN_PROGRESS",
		Kind: statemachine.ServiceTask,
		Raw:  statemachine.RawNode{Action: "move-to-in-progress"},
		Fn: func(ctx *statemachine.Context) statemachine.Outcome {
			ctx.Set("issue_num", "42")
			return statemachine.Outcome{}
		},
	}
	wrapped := wrap(node, Deps{Out: &buf}.withDefaults())

	out := wrapped(statemachine.NewContext())
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}

	got := buf.String()
	// State delta is hoisted into the OK line when Outcome is empty —
	// `OK ... -> issue_num=42` replaces the misleading `OK ... -> (no result)`
	// the writer would otherwise print. The standalone `state:` follow-on
	// line is suppressed to avoid duplication.
	wantSubs := []string{
		"> MARK_IN_PROGRESS  kind=service-task action=move-to-in-progress",
		"OK MARK_IN_PROGRESS -> issue_num=42",
	}
	for _, s := range wantSubs {
		if !strings.Contains(got, s) {
			t.Errorf("trace output missing %q\nfull output:\n%s", s, got)
		}
	}
	if strings.Contains(got, "state: issue_num=42") {
		t.Errorf("state delta should be hoisted into OK line, not duplicated as follow-on; got:\n%s", got)
	}
}

func TestWrap_GatewayLogsBindingAndStateDelta(t *testing.T) {
	prevNow := nowFn
	nowFn = fixedClock
	t.Cleanup(func() { nowFn = prevNow })

	var buf bytes.Buffer
	node := statemachine.Node{
		ID:   "GATE_TICKET_TYPE",
		Kind: statemachine.Gateway,
		Raw:  statemachine.RawNode{Binding: "ticket_type"},
		Fn: func(ctx *statemachine.Context) statemachine.Outcome {
			ctx.Set("ticket_type", "story")
			return statemachine.Outcome{Value: "story"}
		},
	}
	wrapped := wrap(node, Deps{Out: &buf}.withDefaults())

	wrapped(statemachine.NewContext())

	got := buf.String()
	wantSubs := []string{
		"> GATE_TICKET_TYPE  kind=gateway binding=ticket_type", // kept snake — ticket_type is a legacy registered binding pending dead-code audit
		"OK GATE_TICKET_TYPE -> value=story",
		"state: ticket_type=story",
	}
	for _, s := range wantSubs {
		if !strings.Contains(got, s) {
			t.Errorf("trace output missing %q\nfull output:\n%s", s, got)
		}
	}
}

func TestWrap_UserTaskLogsAgentAndFiles(t *testing.T) {
	prevNow := nowFn
	nowFn = fixedClock
	t.Cleanup(func() { nowFn = prevNow })

	var buf bytes.Buffer
	// Pre-snapshot: clean. Post-snapshot: two new files plus one
	// pre-existing dirty path that should be filtered out by dirtyDelta.
	git := &fakeGit{outs: [][]byte{
		[]byte(" M existing.go\n"),
		[]byte(" M existing.go\n M path/a.go\n?? path/b.go\n"),
	}}
	node := statemachine.Node{
		ID:   "AT_RED_TEST_WRITE",
		Kind: statemachine.UserTask,
		Raw:  statemachine.RawNode{Agent: "acceptance-test-writer"},
		Fn: func(ctx *statemachine.Context) statemachine.Outcome {
			return statemachine.Outcome{}
		},
	}
	deps := Deps{Out: &buf, Git: git, RepoPath: "/repo"}.withDefaults()
	wrapped := wrap(node, deps)

	wrapped(statemachine.NewContext())

	got := buf.String()
	wantSubs := []string{
		"> AT_RED_TEST_WRITE  kind=user-task agent=acceptance-test-writer",
		"OK AT_RED_TEST_WRITE",
		"files: path/a.go, path/b.go",
	}
	for _, s := range wantSubs {
		if !strings.Contains(got, s) {
			t.Errorf("trace output missing %q\nfull output:\n%s", s, got)
		}
	}
	if strings.Contains(got, "existing.go") {
		t.Errorf("trace must filter pre-existing dirty paths; got:\n%s", got)
	}
	if git.lastDir != "/repo" {
		t.Errorf("git called with dir %q, want /repo", git.lastDir)
	}
	if len(git.lastArgs) != 2 {
		t.Fatalf("expected 2 git calls (pre+post snapshot); got %d", len(git.lastArgs))
	}
	wantArgs := []string{"status", "--porcelain"}
	for i, args := range git.lastArgs {
		if fmt.Sprint(args) != fmt.Sprint(wantArgs) {
			t.Errorf("snapshot[%d]: git args %v, want %v", i, args, wantArgs)
		}
	}
}

func TestWrap_ServiceTaskSkipsWorkingTreeSnapshot(t *testing.T) {
	// Service tasks shouldn't trigger git status — those calls add up
	// fast over a full pipeline run. Only user-task nodes need the
	// snapshot.
	prevNow := nowFn
	nowFn = fixedClock
	t.Cleanup(func() { nowFn = prevNow })

	var buf bytes.Buffer
	git := &fakeGit{}
	node := statemachine.Node{
		ID:   "MARK_IN_PROGRESS",
		Kind: statemachine.ServiceTask,
		Raw:  statemachine.RawNode{Action: "move-to-in-progress"},
		Fn: func(ctx *statemachine.Context) statemachine.Outcome {
			return statemachine.Outcome{}
		},
	}
	wrapped := wrap(node, Deps{Out: &buf, Git: git}.withDefaults())

	wrapped(statemachine.NewContext())

	if len(git.lastArgs) != 0 {
		t.Errorf("service-task triggered %d git call(s); should be 0", len(git.lastArgs))
	}
}

func TestWrap_UserTaskExpandsTemplatedAgent(t *testing.T) {
	prevNow := nowFn
	nowFn = fixedClock
	t.Cleanup(func() { nowFn = prevNow })

	var buf bytes.Buffer
	node := statemachine.Node{
		ID:   "STRUCT_RED_TEST_WRITE",
		Kind: statemachine.UserTask,
		Raw:  statemachine.RawNode{Agent: "${agent}"},
		Fn: func(ctx *statemachine.Context) statemachine.Outcome {
			return statemachine.Outcome{}
		},
	}
	wrapped := wrap(node, Deps{Out: &buf}.withDefaults())

	ctx := statemachine.NewContext()
	ctx.Params["agent"] = "atdd-action"
	wrapped(ctx)

	got := buf.String()
	if !strings.Contains(got, "agent=atdd-action") {
		t.Errorf("trace output should expand ${agent} param; got:\n%s", got)
	}
}

func TestWrap_FailureExitLogsError(t *testing.T) {
	prevNow := nowFn
	nowFn = fixedClock
	t.Cleanup(func() { nowFn = prevNow })

	var buf bytes.Buffer
	node := statemachine.Node{
		ID:   "FAILS",
		Kind: statemachine.ServiceTask,
		Raw:  statemachine.RawNode{Action: "go_boom"},
		Fn: func(ctx *statemachine.Context) statemachine.Outcome {
			return statemachine.Outcome{Err: fmt.Errorf("boom")}
		},
	}
	wrapped := wrap(node, Deps{Out: &buf}.withDefaults())

	out := wrapped(statemachine.NewContext())
	if out.Err == nil {
		t.Fatal("expected err to propagate")
	}
	got := buf.String()
	if !strings.Contains(got, "FAIL FAILS -> boom") {
		t.Errorf("expected FAIL line; got:\n%s", got)
	}
	if strings.Contains(got, "OK FAILS") {
		t.Errorf("expected no OK line on failure; got:\n%s", got)
	}
}

func TestWrap_VerifyClassRendersAsBannerStatusWord(t *testing.T) {
	// The verify action stamps Outcome.Value with one of {ok, red, infra}
	// so the trace banner can render the failure class as the status
	// word — fixing the "OK RUN_TESTS -> (no result)" line
	// that contradicted the inline "(test run failed: ... — continuing)"
	// the same node had just printed.
	prevNow := nowFn
	nowFn = fixedClock
	t.Cleanup(func() { nowFn = prevNow })

	for _, tc := range []struct {
		name      string
		value     string
		wantWord  string
		wantSlash bool // expect "-> ..." suffix?
	}{
		{name: "red", value: "red", wantWord: "RED RUN_TESTS", wantSlash: false},
		{name: "infra", value: "infra", wantWord: "INFRA RUN_TESTS", wantSlash: false},
		{name: "ok", value: "ok", wantWord: "OK RUN_TESTS", wantSlash: false},
		// Empty Value preserves the historic "(no result)" rendering —
		// no verify happened (e.g. approve-without-running), so the
		// banner shouldn't claim a class.
		{name: "empty_no_result", value: "", wantWord: "OK RUN_TESTS -> (no result)", wantSlash: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			node := statemachine.Node{
				ID:   "RUN_TESTS",
				Kind: statemachine.ServiceTask,
				Raw:  statemachine.RawNode{Action: "run_tests"},
				Fn: func(ctx *statemachine.Context) statemachine.Outcome {
					return statemachine.Outcome{Value: tc.value}
				},
			}
			wrapped := wrap(node, Deps{Out: &buf}.withDefaults())
			out := wrapped(statemachine.NewContext())
			if out.Err != nil {
				t.Fatalf("unexpected err: %v", out.Err)
			}
			got := buf.String()
			if !strings.Contains(got, tc.wantWord) {
				t.Errorf("trace output missing %q\nfull output:\n%s", tc.wantWord, got)
			}
			// Verify-class banners drop the "-> value=red" suffix; the
			// status word already conveys the class.
			if !tc.wantSlash && tc.value != "" && strings.Contains(got, "-> value=") {
				t.Errorf("trace output should not include redundant `-> value=%s` suffix when status word already shows it; got:\n%s", tc.value, got)
			}
		})
	}
}

func TestWrap_HoistsStateDeltaWhenOutcomeIsEmpty(t *testing.T) {
	// Regression: when a `user-task agent: human` recorded approval rejection
	// via ctx.Set("approval-outcome", "rejected") but returned an empty
	// Outcome, the trace used to read `OK ASK_HUMAN -> (no result)` with
	// the actual answer one line below in `state: approval-outcome=rejected`.
	// Operators reading the banner couldn't tell from the OK line whether
	// they'd hit y or n. The hoist puts the answer on the OK line itself.
	prevNow := nowFn
	nowFn = fixedClock
	t.Cleanup(func() { nowFn = prevNow })

	var buf bytes.Buffer
	node := statemachine.Node{
		ID:   "ASK_HUMAN",
		Kind: statemachine.UserTask,
		Raw:  statemachine.RawNode{Agent: "human"},
		Fn: func(ctx *statemachine.Context) statemachine.Outcome {
			ctx.Set("approval-outcome", "rejected")
			return statemachine.Outcome{}
		},
	}
	wrapped := wrap(node, Deps{Out: &buf, Git: &fakeGit{}}.withDefaults())

	wrapped(statemachine.NewContext())

	got := buf.String()
	if !strings.Contains(got, "OK ASK_HUMAN -> approval-outcome=rejected") {
		t.Errorf("expected state delta hoisted into OK line; got:\n%s", got)
	}
	if strings.Contains(got, "(no result)") {
		t.Errorf("OK line should not read `(no result)` when state was written; got:\n%s", got)
	}
	if strings.Contains(got, "state: approval-outcome=rejected") {
		t.Errorf("state delta should not be duplicated as follow-on line after hoist; got:\n%s", got)
	}
}

func TestWrap_PopulatedOutcomeDoesNotHoistStateDelta(t *testing.T) {
	// Inverse of the hoist regression: when the node returns a non-empty
	// Outcome (e.g. a gateway whose binding evaluated to a string value),
	// the state delta stays on its own follow-on line — the OK line already
	// carries the outcome and replacing it would lose the gateway's value.
	prevNow := nowFn
	nowFn = fixedClock
	t.Cleanup(func() { nowFn = prevNow })

	var buf bytes.Buffer
	node := statemachine.Node{
		ID:   "GATE_TICKET_TYPE",
		Kind: statemachine.Gateway,
		Raw:  statemachine.RawNode{Binding: "ticket_type"},
		Fn: func(ctx *statemachine.Context) statemachine.Outcome {
			ctx.Set("ticket_type", "story")
			return statemachine.Outcome{Value: "story"}
		},
	}
	wrapped := wrap(node, Deps{Out: &buf}.withDefaults())

	wrapped(statemachine.NewContext())

	got := buf.String()
	if !strings.Contains(got, "OK GATE_TICKET_TYPE -> value=story") {
		t.Errorf("OK line should show the Outcome.Value; got:\n%s", got)
	}
	if !strings.Contains(got, "state: ticket_type=story") {
		t.Errorf("state delta should remain on its own line when Outcome is populated; got:\n%s", got)
	}
}

func TestWrap_CallActivityIndentsNestedChildren(t *testing.T) {
	// Top-level call-activity entry sits at column 0, its inner children
	// are bumped two spaces per nesting level so the trace reads as a
	// visible BPMN tree. The exit banner pairs vertically with the entry
	// banner (decrement happens before writeExit).
	prevNow := nowFn
	nowFn = fixedClock
	t.Cleanup(func() { nowFn = prevNow })

	var buf bytes.Buffer
	deps := Deps{Out: &buf}.withDefaults()

	leaf := wrap(statemachine.Node{
		ID:   "LEAF",
		Kind: statemachine.ServiceTask,
		Raw:  statemachine.RawNode{Action: "do-leaf"},
		Fn:   func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} },
	}, deps)

	mid := wrap(statemachine.Node{
		ID:   "MID",
		Kind: statemachine.CallActivity,
		Raw:  statemachine.RawNode{Process: "mid-sub"},
		Fn:   func(ctx *statemachine.Context) statemachine.Outcome { return leaf(ctx) },
	}, deps)

	top := wrap(statemachine.Node{
		ID:   "TOP",
		Kind: statemachine.CallActivity,
		Raw:  statemachine.RawNode{Process: "top-sub"},
		Fn:   func(ctx *statemachine.Context) statemachine.Outcome { return mid(ctx) },
	}, deps)

	top(statemachine.NewContext())

	got := buf.String()
	// TOP is at depth 0 (no indent between `] ` and `>`).
	// MID is at depth 1 (two spaces).
	// LEAF is at depth 2 (four spaces).
	wantSubs := []string{
		"] > TOP  kind=call-activity process=top-sub",
		"]   > MID  kind=call-activity process=mid-sub",
		"]     > LEAF  kind=service-task action=do-leaf",
		"]     OK LEAF",
		"]   OK MID",
		"] OK TOP",
	}
	for _, s := range wantSubs {
		if !strings.Contains(got, s) {
			t.Errorf("trace output missing %q\nfull output:\n%s", s, got)
		}
	}
}

func TestStateDelta_IgnoresOverrideKeysAndSortsKeys(t *testing.T) {
	pre := map[string]string{"a": "1", "_override_extra": ""}
	post := map[string]string{"a": "2", "b": "3", "_override_extra": "hint"}

	got := stateDelta(pre, post)
	want := "a=2, b=3"
	if got != want {
		t.Errorf("stateDelta = %q, want %q", got, want)
	}
}

func TestWrapAll_DecoratesEveryNodeInEveryFlow(t *testing.T) {
	prevNow := nowFn
	nowFn = fixedClock
	t.Cleanup(func() { nowFn = prevNow })

	calls := 0
	body := func(ctx *statemachine.Context) statemachine.Outcome {
		calls++
		return statemachine.Outcome{}
	}
	eng := &statemachine.Engine{
		Processes: map[string]*statemachine.Process{
			"main": {
				Nodes: map[string]statemachine.Node{
					"A": {ID: "A", Kind: statemachine.ServiceTask, Raw: statemachine.RawNode{Action: "a"}, Fn: body},
					"B": {ID: "B", Kind: statemachine.Gateway, Raw: statemachine.RawNode{Binding: "b"}, Fn: body},
				},
			},
		},
	}

	var buf bytes.Buffer
	WrapAll(eng, Deps{Out: &buf})

	for _, n := range eng.Processes["main"].Nodes {
		n.Fn(statemachine.NewContext())
	}
	if calls != 2 {
		t.Errorf("expected each inner body to fire once; got %d calls", calls)
	}
	got := buf.String()
	if !strings.Contains(got, "> A") || !strings.Contains(got, "> B") {
		t.Errorf("expected both nodes traced; got:\n%s", got)
	}
}
