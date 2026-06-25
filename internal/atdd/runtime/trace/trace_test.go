package trace

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
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
			ctx.Set("issue-num", "42")
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
	// `OK ... -> issue-num=42` replaces the misleading `OK ... -> (no result)`
	// the writer would otherwise print. The standalone `state:` follow-on
	// line is suppressed to avoid duplication.
	wantSubs := []string{
		"> MARK_IN_PROGRESS  kind=service-task action=move-to-in-progress",
		"OK MARK_IN_PROGRESS -> issue-num=42",
	}
	for _, s := range wantSubs {
		if !strings.Contains(got, s) {
			t.Errorf("trace output missing %q\nfull output:\n%s", s, got)
		}
	}
	if strings.Contains(got, "state: issue-num=42") {
		t.Errorf("state delta should be hoisted into OK line, not duplicated as follow-on; got:\n%s", got)
	}
}

// TestWrap_TestsInfraHaltBannerRendersDiagnosticPayload exercises the
// special-case banner the wrap decorator emits when an infra-classified
// test-run failure routes to TESTS_INFRA_HALT. The runner-not-started
// case (binary missing, docker down, etc.) was silently advancing the
// pipeline pre-classifier; the banner must surface the infra label,
// the failing command, and the stderr tail so the operator can
// diagnose without parsing the raw state dump.
func TestWrap_TestsInfraHaltBannerRendersDiagnosticPayload(t *testing.T) {
	prevNow := nowFn
	nowFn = fixedClock
	t.Cleanup(func() { nowFn = prevNow })

	var buf bytes.Buffer
	// The actual TESTS_INFRA_HALT node is an error-end-event whose
	// NodeFn is the no-op returned by Engine.resolve. Earlier nodes
	// (runCommand) populate the diagnostic state this banner reads.
	node := statemachine.Node{
		ID:   "TESTS_INFRA_HALT",
		Kind: statemachine.ErrorEndEvent,
		Fn:   func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} },
	}
	wrapped := wrap(node, Deps{Out: &buf}.withDefaults())
	ctx := statemachine.NewContext()
	ctx.Set("test-infra-label", "missing executable")
	ctx.Set("command-line", "gh optivem system-test run --suite=acceptance --test=foo")
	ctx.Set("command-stderr-tail", "'C:\\Program' is not recognized as an internal or external command,\noperable program or batch file.")
	wrapped(ctx)

	got := buf.String()
	wantSubs := []string{
		"HALT TESTS_INFRA_HALT — infra failure: missing executable",
		"command: gh optivem system-test run --suite=acceptance --test=foo",
		"stderr tail: 'C:\\Program' is not recognized as an internal or external command,",
		"operable program or batch file.",
	}
	for _, s := range wantSubs {
		if !strings.Contains(got, s) {
			t.Errorf("trace output missing %q\nfull output:\n%s", s, got)
		}
	}
	// Generic exit-line shape must NOT also render — banner replaces it.
	if strings.Contains(got, "OK TESTS_INFRA_HALT") {
		t.Errorf("infra halt banner must replace the generic OK line; got:\n%s", got)
	}
}

// TestWrap_TestsInfraHaltBannerSurfacesContractDrift guards the
// "(unset)" rendering for missing diagnostic state. Pre-classifier
// drift (e.g. an upstream change that stops stamping test-infra-label)
// must surface visibly in the banner instead of producing a misleading
// "infra failure:" with empty fields.
func TestWrap_TestsInfraHaltBannerSurfacesContractDrift(t *testing.T) {
	prevNow := nowFn
	nowFn = fixedClock
	t.Cleanup(func() { nowFn = prevNow })

	var buf bytes.Buffer
	node := statemachine.Node{
		ID:   "TESTS_INFRA_HALT",
		Kind: statemachine.ErrorEndEvent,
		Fn:   func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} },
	}
	wrapped := wrap(node, Deps{Out: &buf}.withDefaults())
	// Empty state — no infra-payload keys set.
	wrapped(statemachine.NewContext())

	got := buf.String()
	for _, s := range []string{
		"HALT TESTS_INFRA_HALT — infra failure: (unset)",
		"command: (unset)",
		"stderr tail: (unset)",
	} {
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

func TestWrap_GatewayBoolFalseRendersExplicitly(t *testing.T) {
	// Regression: a gateway that returns Bool:false used to render as
	// `OK GATE_X -> (no result)` when the binding had already been set to
	// false by an upstream service-task (no state delta to hoist). This
	// contradicted `OK GATE_X -> bool=true` for the symmetric case and
	// hid the gate's actual decision behind a `(no result)` label.
	prevNow := nowFn
	nowFn = fixedClock
	t.Cleanup(func() { nowFn = prevNow })

	t.Run("no_delta_substitutes_bool_false", func(t *testing.T) {
		var buf bytes.Buffer
		node := statemachine.Node{
			ID:   "GATE_X",
			Kind: statemachine.Gateway,
			Raw:  statemachine.RawNode{Binding: "x-enabled"},
			Fn: func(ctx *statemachine.Context) statemachine.Outcome {
				// Re-affirm: caller has already written x-enabled=false upstream.
				ctx.Set("x-enabled", false)
				return statemachine.Outcome{Bool: false}
			},
		}
		wrapped := wrap(node, Deps{Out: &buf}.withDefaults())
		ctx := statemachine.NewContext()
		ctx.Set("x-enabled", false) // pre-state already false
		wrapped(ctx)

		got := buf.String()
		if !strings.Contains(got, "OK GATE_X -> bool=false") {
			t.Errorf("expected explicit bool=false for gateway; got:\n%s", got)
		}
		if strings.Contains(got, "(no result)") {
			t.Errorf("gateway with Bool:false must not render as (no result); got:\n%s", got)
		}
	})

	t.Run("delta_still_hoists", func(t *testing.T) {
		// When the binding flips at this node, the existing hoist still wins
		// and shows the full state delta — the new fallback must not steal
		// that case.
		var buf bytes.Buffer
		node := statemachine.Node{
			ID:   "GATE_Y",
			Kind: statemachine.Gateway,
			Raw:  statemachine.RawNode{Binding: "y-enabled"},
			Fn: func(ctx *statemachine.Context) statemachine.Outcome {
				ctx.Set("y-enabled", false)
				return statemachine.Outcome{Bool: false}
			},
		}
		wrapped := wrap(node, Deps{Out: &buf}.withDefaults())
		wrapped(statemachine.NewContext()) // pre-state empty → delta exists

		got := buf.String()
		if !strings.Contains(got, "OK GATE_Y -> y-enabled=false") {
			t.Errorf("expected hoisted state delta to win over bool=false fallback; got:\n%s", got)
		}
	})
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

func TestStateDelta_IgnoresOverrideKeysAndSortsKeys(t *testing.T) {
	pre := map[string]string{"a": "1", "_override_extra": ""}
	post := map[string]string{"a": "2", "b": "3", "_override_extra": "hint"}

	got := stateDelta(pre, post)
	want := "a=2, b=3"
	if got != want {
		t.Errorf("stateDelta = %q, want %q", got, want)
	}
}

func TestStateDelta_LongValuesSortLast(t *testing.T) {
	// Short scalars render first, long blobs (e.g. phase-changed-files'
	// newline-joined path list) trail — so a multi-line value never
	// splits two short scalars on the OK line.
	pre := map[string]string{}
	post := map[string]string{
		"phase-changed-files":             "a/long/path/one.java\na/long/path/two.java",
		"external-driver-port-changed": "false",
		"system-driver-port-changed":   "true",
	}

	got := stateDelta(pre, post)
	want := "system-driver-port-changed=true, external-driver-port-changed=false, phase-changed-files=a/long/path/one.java\na/long/path/two.java"
	if got != want {
		t.Errorf("stateDelta = %q, want %q", got, want)
	}
}

func TestWrap_EndEventNameSurfacesAsExitDetail(t *testing.T) {
	// End-events and error-end-events have no Outcome and no state delta to
	// hoist; their YAML `name:` IS the meaningful exit signal. The trace
	// must surface that name in place of the misleading `(no result)`
	// placeholder so an operator scanning the exit line sees what reaching
	// the node actually meant ("Ticket Marked IN ACCEPTANCE") rather than a
	// content-free banner.
	prevNow := nowFn
	nowFn = fixedClock
	t.Cleanup(func() { nowFn = prevNow })

	cases := []struct {
		name string
		kind statemachine.NodeKind
		raw  statemachine.RawNode
		want string
	}{
		{
			name: "end-event with name",
			kind: statemachine.EndEvent,
			raw:  statemachine.RawNode{Name: "Ticket Marked IN ACCEPTANCE"},
			want: `OK IMPLEMENT_TICKET_END -> "Ticket Marked IN ACCEPTANCE"`,
		},
		{
			name: "error-end-event with name",
			kind: statemachine.ErrorEndEvent,
			raw:  statemachine.RawNode{Name: "Unknown Ticket Kind"},
			want: `OK UNKNOWN_TICKET_KIND -> "Unknown Ticket Kind"`,
		},
		{
			name: "end-event without name falls back to (no result)",
			kind: statemachine.EndEvent,
			raw:  statemachine.RawNode{},
			want: "OK NAMELESS_END -> (no result)",
		},
	}
	ids := []string{"IMPLEMENT_TICKET_END", "UNKNOWN_TICKET_KIND", "NAMELESS_END"}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			node := statemachine.Node{
				ID:   ids[i],
				Kind: tc.kind,
				Raw:  tc.raw,
				Fn:   func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} },
			}
			wrapped := wrap(node, Deps{Out: &buf}.withDefaults())
			wrapped(statemachine.NewContext())

			got := buf.String()
			if !strings.Contains(got, tc.want) {
				t.Errorf("trace output missing %q\nfull output:\n%s", tc.want, got)
			}
		})
	}
}

func TestWrap_CallActivityVerdictChipFromTestState(t *testing.T) {
	// Call-activity exit lines lead with a derived `verdict=` chip that
	// reads the sub-process's terminal test state and classifies it
	// against the call-site's expectation. The chip surfaces the
	// "did the cycle reach its expected state" signal that command-succeeded
	// (= exit==0) does not convey on its own. Non-test sub-processes
	// (neither key set) emit no chip so the trace stays terse.
	prevNow := nowFn
	nowFn = fixedClock
	t.Cleanup(func() { nowFn = prevNow })

	cases := []struct {
		name       string
		setup      func(*statemachine.Context)
		wantSub    string
		wantNotSub string
	}{
		{
			name: "green-as-expected",
			setup: func(ctx *statemachine.Context) {
				ctx.Set("expected-test-result", "success")
				ctx.Set("test-outcome", "pass")
			},
			wantSub: "-> verdict=green-as-expected ",
		},
		{
			name: "red-as-expected",
			setup: func(ctx *statemachine.Context) {
				ctx.Set("expected-test-result", "failure")
				ctx.Set("test-outcome", "fail")
			},
			wantSub: "-> verdict=red-as-expected ",
		},
		{
			name: "unexpected-fail",
			setup: func(ctx *statemachine.Context) {
				ctx.Set("expected-test-result", "success")
				ctx.Set("test-outcome", "fail")
			},
			wantSub: "-> verdict=unexpected-fail ",
		},
		{
			name: "unexpected-pass",
			setup: func(ctx *statemachine.Context) {
				ctx.Set("expected-test-result", "failure")
				ctx.Set("test-outcome", "pass")
			},
			wantSub: "-> verdict=unexpected-pass ",
		},
		{
			name: "infra short-circuits expectation comparison",
			setup: func(ctx *statemachine.Context) {
				ctx.Set("expected-test-result", "success")
				ctx.Set("test-outcome", "infra")
			},
			wantSub: "-> verdict=infra ",
		},
		{
			name:       "non-test phase emits no chip",
			setup:      func(ctx *statemachine.Context) {},
			wantNotSub: "verdict=",
		},
		{
			name: "only one field set emits no chip",
			setup: func(ctx *statemachine.Context) {
				ctx.Set("expected-test-result", "failure")
			},
			wantNotSub: "verdict=",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			node := statemachine.Node{
				ID:   "IMPLEMENT_TICKET",
				Kind: statemachine.CallActivity,
				Raw:  statemachine.RawNode{Process: "implement-ticket"},
				Fn:   func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} },
			}
			wrapped := wrap(node, Deps{Out: &buf}.withDefaults())
			ctx := statemachine.NewContext()
			tc.setup(ctx)
			wrapped(ctx)

			got := buf.String()
			if tc.wantSub != "" && !strings.Contains(got, tc.wantSub) {
				t.Errorf("trace output missing %q\nfull output:\n%s", tc.wantSub, got)
			}
			if tc.wantNotSub != "" && strings.Contains(got, tc.wantNotSub) {
				t.Errorf("trace output should not contain %q\nfull output:\n%s", tc.wantNotSub, got)
			}
		})
	}
}

func TestWrap_CallActivityVerdictPrependsToStateDelta(t *testing.T) {
	// When the call-activity exits with a state delta hoisted onto the OK
	// line, the verdict chip leads while the delta keys follow — operator
	// scans the line left-to-right and sees intent (verdict) before
	// mechanics (state keys).
	prevNow := nowFn
	nowFn = fixedClock
	t.Cleanup(func() { nowFn = prevNow })

	var buf bytes.Buffer
	node := statemachine.Node{
		ID:   "IMPLEMENT_TICKET",
		Kind: statemachine.CallActivity,
		Raw:  statemachine.RawNode{Process: "implement-ticket"},
		Fn: func(ctx *statemachine.Context) statemachine.Outcome {
			ctx.Set("expected-test-result", "failure")
			ctx.Set("test-outcome", "fail")
			ctx.Set("command-succeeded", false)
			return statemachine.Outcome{}
		},
	}
	wrapped := wrap(node, Deps{Out: &buf}.withDefaults())
	wrapped(statemachine.NewContext())

	got := buf.String()
	// verdict must precede the state-delta tail.
	wantPrefix := "OK IMPLEMENT_TICKET -> verdict=red-as-expected, "
	if !strings.Contains(got, wantPrefix) {
		t.Errorf("trace output should prepend verdict before state delta; want substring %q\nfull output:\n%s", wantPrefix, got)
	}
}

func TestWrap_EnterBannerShowsInheritedScopeChips(t *testing.T) {
	// channel / external-system-name are not node fields — they are
	// call-activity scope params bound by the channel / external-system
	// unrolls and pushed into ctx.Params on sub-process entry, so every node
	// *inside* the unrolled sub-process inherits them in ev.Params. The live
	// banner must surface them as inherited-scope chips; before this it showed
	// only the per-kind selector and dropped the discriminator.
	prevNow := nowFn
	nowFn = fixedClock
	t.Cleanup(func() { nowFn = prevNow })

	t.Run("user-task inherits channel", func(t *testing.T) {
		var buf bytes.Buffer
		node := statemachine.Node{
			ID:   "ASK_HUMAN",
			Kind: statemachine.UserTask,
			Raw:  statemachine.RawNode{Agent: "human"},
			Fn:   func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} },
		}
		wrapped := wrap(node, Deps{Out: &buf}.withDefaults())
		ctx := statemachine.NewContext()
		ctx.Params["channel"] = "api"
		wrapped(ctx)

		if got := buf.String(); !strings.Contains(got, "> ASK_HUMAN  kind=user-task agent=human channel=api") {
			t.Errorf("banner should carry inherited channel chip; got:\n%s", got)
		}
	})

	t.Run("gateway inherits channel", func(t *testing.T) {
		var buf bytes.Buffer
		node := statemachine.Node{
			ID:   "GATE_API",
			Kind: statemachine.Gateway,
			Raw:  statemachine.RawNode{Binding: "some-binding"},
			Fn:   func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{Value: "yes"} },
		}
		wrapped := wrap(node, Deps{Out: &buf}.withDefaults())
		ctx := statemachine.NewContext()
		ctx.Params["channel"] = "api"
		wrapped(ctx)

		if got := buf.String(); !strings.Contains(got, "> GATE_API  kind=gateway binding=some-binding channel=api") {
			t.Errorf("banner should carry inherited channel chip; got:\n%s", got)
		}
	})

	t.Run("no channel in scope emits no chip", func(t *testing.T) {
		var buf bytes.Buffer
		node := statemachine.Node{
			ID:   "ASK_HUMAN",
			Kind: statemachine.UserTask,
			Raw:  statemachine.RawNode{Agent: "human"},
			Fn:   func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} },
		}
		wrapped := wrap(node, Deps{Out: &buf}.withDefaults())
		wrapped(statemachine.NewContext())

		if got := buf.String(); strings.Contains(got, "channel=") {
			t.Errorf("node with no channel in scope must emit no channel chip; got:\n%s", got)
		}
	})

	t.Run("unrolling call-activity shows params= but no duplicate chip", func(t *testing.T) {
		// The unrolling call-activity pushes channel via Raw.Params — it shows
		// `channel=api` inside its `params=` chip. The inherited-scope chip must
		// not also fire (it would duplicate channel on the same line). Here the
		// param is in both Raw.Params (→ CallParams) and ctx.Params to prove the
		// CallParams guard suppresses the separate chip even when both are set.
		var buf bytes.Buffer
		node := statemachine.Node{
			ID:   "IMPLEMENT_AND_VERIFY_SYSTEM_API",
			Kind: statemachine.CallActivity,
			Raw: statemachine.RawNode{
				Process: "implement-and-verify-system",
				Params:  map[string]string{"channel": "api"},
			},
			Fn: func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} },
		}
		wrapped := wrap(node, Deps{Out: &buf}.withDefaults())
		ctx := statemachine.NewContext()
		ctx.Params["channel"] = "api"
		wrapped(ctx)

		got := buf.String()
		if !strings.Contains(got, "params=channel=api") {
			t.Errorf("call-activity should show channel via params= chip; got:\n%s", got)
		}
		if strings.Contains(got, "params=channel=api channel=api") {
			t.Errorf("inherited chip must not duplicate the params= channel on a call-activity; got:\n%s", got)
		}
	})

	t.Run("external-system-name chip mirrors channel", func(t *testing.T) {
		var buf bytes.Buffer
		node := statemachine.Node{
			ID:   "ASK_HUMAN",
			Kind: statemachine.UserTask,
			Raw:  statemachine.RawNode{Agent: "human"},
			Fn:   func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} },
		}
		wrapped := wrap(node, Deps{Out: &buf}.withDefaults())
		ctx := statemachine.NewContext()
		ctx.Params["external-system-name"] = "stripe"
		wrapped(ctx)

		if got := buf.String(); !strings.Contains(got, "external-system-name=stripe") {
			t.Errorf("banner should carry inherited external-system-name chip; got:\n%s", got)
		}
	})

	t.Run("channel headlines before external-system-name", func(t *testing.T) {
		var buf bytes.Buffer
		node := statemachine.Node{
			ID:   "ASK_HUMAN",
			Kind: statemachine.UserTask,
			Raw:  statemachine.RawNode{Agent: "human"},
			Fn:   func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} },
		}
		wrapped := wrap(node, Deps{Out: &buf}.withDefaults())
		ctx := statemachine.NewContext()
		ctx.Params["channel"] = "api"
		ctx.Params["external-system-name"] = "stripe"
		wrapped(ctx)

		if got := buf.String(); !strings.Contains(got, "channel=api external-system-name=stripe") {
			t.Errorf("channel should headline before external-system-name; got:\n%s", got)
		}
	})
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
