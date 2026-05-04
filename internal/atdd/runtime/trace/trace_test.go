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

// fakeGit is a stub GitRunner used to assert filesInCommit calls without
// shelling out. It records the last argv and returns the canned output.
type fakeGit struct {
	out     []byte
	err     error
	lastDir string
	lastArg []string
}

func (g *fakeGit) Run(_ context.Context, dir string, args ...string) ([]byte, error) {
	g.lastDir = dir
	g.lastArg = args
	return g.out, g.err
}

func TestWrap_ServiceTaskLogsEntryAndExit(t *testing.T) {
	prevNow := nowFn
	nowFn = fixedClock
	t.Cleanup(func() { nowFn = prevNow })

	var buf bytes.Buffer
	node := statemachine.Node{
		ID:   "PICK_TOP_READY",
		Kind: statemachine.ServiceTask,
		Raw:  statemachine.RawNode{Action: "pick_top_ready"},
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
	wantSubs := []string{
		"> PICK_TOP_READY  kind=service_task action=pick_top_ready",
		"OK PICK_TOP_READY",
		"state: issue_num=42",
	}
	for _, s := range wantSubs {
		if !strings.Contains(got, s) {
			t.Errorf("trace output missing %q\nfull output:\n%s", s, got)
		}
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
		"> GATE_TICKET_TYPE  kind=gateway binding=ticket_type",
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
	git := &fakeGit{out: []byte("path/a.go\npath/b.go\n")}
	node := statemachine.Node{
		ID:   "AT_RED_TEST_WRITE",
		Kind: statemachine.UserTask,
		Raw:  statemachine.RawNode{Agent: "atdd-test"},
		Fn: func(ctx *statemachine.Context) statemachine.Outcome {
			return statemachine.Outcome{Commit: "deadbeef0000000000000000000000000000beef"}
		},
	}
	deps := Deps{Out: &buf, Git: git, RepoPath: "/repo"}.withDefaults()
	wrapped := wrap(node, deps)

	wrapped(statemachine.NewContext())

	got := buf.String()
	wantSubs := []string{
		"> AT_RED_TEST_WRITE  kind=user_task agent=atdd-test",
		"OK AT_RED_TEST_WRITE -> commit=deadbee",
		"files: path/a.go, path/b.go",
	}
	for _, s := range wantSubs {
		if !strings.Contains(got, s) {
			t.Errorf("trace output missing %q\nfull output:\n%s", s, got)
		}
	}
	if git.lastDir != "/repo" {
		t.Errorf("git called with dir %q, want /repo", git.lastDir)
	}
	wantArgs := []string{"show", "--name-only", "--format=", "deadbeef0000000000000000000000000000beef"}
	if fmt.Sprint(git.lastArg) != fmt.Sprint(wantArgs) {
		t.Errorf("git called with args %v, want %v", git.lastArg, wantArgs)
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
		Flows: map[string]*statemachine.Flow{
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

	for _, n := range eng.Flows["main"].Nodes {
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
