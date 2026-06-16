// Tests for the step-execution summary: the renderer's table shape +
// totals, the steps.jsonl round-trip, the cobra-facing PrintStepSummaryFile
// entry point, and wrapStepRecorders' command-capture wrap. Agent steps are
// recorded at dispatch time (covered by the dispatch-integration tests); these
// focus on the step-summary machinery itself.
package driver

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// TestRenderStepSummary_Table asserts one row per step in execution order,
// the kind labels, the ✗ marker on a failed row, the totals row carrying the
// sum, and the wall-clock headline line.
func TestRenderStepSummary_Table(t *testing.T) {
	steps := []stepRecord{
		{name: "write-acceptance-tests", kind: stepKindAgent, elapsed: 30 * time.Second},
		{name: "gh optivem test compile", kind: stepKindCommand, elapsed: 5 * time.Second},
		{name: "gh optivem test run", kind: stepKindCommand, elapsed: 12 * time.Second, err: errors.New("exit 1")},
	}
	var buf bytes.Buffer
	renderStepSummary(&buf, steps, 90*time.Second)
	out := buf.String()

	for _, want := range []string{
		"=== Step summary ===",
		"write-acceptance-tests",
		"agent",
		"gh optivem test compile",
		"command",
		"✗ gh optivem test run", // failed row marked
		"totals",
		"Total execution time (wall-clock): 1m30s",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("step summary missing %q\n---\n%s", want, out)
		}
	}

	// Sum of step times = 30+5+12 = 47s, shown on the totals row.
	if !strings.Contains(out, "47s") {
		t.Errorf("step summary missing sum-of-steps total 47s\n---\n%s", out)
	}

	// Execution order preserved: agent step appears before the compile step.
	if strings.Index(out, "write-acceptance-tests") > strings.Index(out, "gh optivem test compile") {
		t.Errorf("rows out of execution order\n---\n%s", out)
	}
}

// TestRenderStepSummary_Empty: no rows → no output (matches renderAgentSummary).
func TestRenderStepSummary_Empty(t *testing.T) {
	var buf bytes.Buffer
	renderStepSummary(&buf, nil, 0)
	if buf.Len() != 0 {
		t.Errorf("expected no output for empty steps, got %q", buf.String())
	}
}

// TestRenderStepSummary_NoWallClock: when wallClock is zero (replay path), the
// headline line is omitted but the totals row still prints.
func TestRenderStepSummary_NoWallClock(t *testing.T) {
	var buf bytes.Buffer
	renderStepSummary(&buf, []stepRecord{{name: "x", kind: stepKindCommand, elapsed: time.Second}}, 0)
	out := buf.String()
	if strings.Contains(out, "wall-clock") {
		t.Errorf("expected no wall-clock line when wallClock==0\n---\n%s", out)
	}
	if !strings.Contains(out, "totals") {
		t.Errorf("expected totals row\n---\n%s", out)
	}
}

// TestStepSidecar_RoundTrip writes mixed records (ok + failed) via
// appendStepLine and reads them back via loadSteps, asserting every field
// survives. The error case checks the message only (errors.New has no identity
// across encode/decode), mirroring the agent sidecar test.
func TestStepSidecar_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "steps.jsonl")
	in := []stepRecord{
		{name: "write-acceptance-tests", kind: stepKindAgent, elapsed: 30 * time.Second},
		{name: "gh optivem test run", kind: stepKindCommand, elapsed: 12 * time.Second, err: errors.New("exit 1")},
	}
	for _, s := range in {
		if err := appendStepLine(path, s); err != nil {
			t.Fatalf("appendStepLine: %v", err)
		}
	}
	got, err := loadSteps(path)
	if err != nil {
		t.Fatalf("loadSteps: %v", err)
	}
	if len(got) != len(in) {
		t.Fatalf("round-trip count: got %d want %d", len(got), len(in))
	}
	for i := range in {
		if got[i].name != in[i].name || got[i].kind != in[i].kind || got[i].elapsed != in[i].elapsed {
			t.Errorf("row %d mismatch: got %+v want %+v", i, got[i], in[i])
		}
	}
	if got[1].err == nil || got[1].err.Error() != "exit 1" {
		t.Errorf("row 1 error not preserved: %+v", got[1].err)
	}
	if got[0].err != nil {
		t.Errorf("row 0 should have no error: %v", got[0].err)
	}
}

// TestAppendStepLine_EmptyPath: empty path is a no-op (rs was nil), no error.
func TestAppendStepLine_EmptyPath(t *testing.T) {
	if err := appendStepLine("", stepRecord{name: "x"}); err != nil {
		t.Errorf("empty path should no-op, got %v", err)
	}
}

// TestPrintStepSummaryFile renders the on-disk sidecar through the same
// renderer the live banner uses, so the replay view stays in sync.
func TestPrintStepSummaryFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "steps.jsonl")
	if err := appendStepLine(path, stepRecord{name: "compile-tests", kind: stepKindCommand, elapsed: 3 * time.Second}); err != nil {
		t.Fatalf("appendStepLine: %v", err)
	}
	var buf bytes.Buffer
	if err := PrintStepSummaryFile(&buf, path); err != nil {
		t.Fatalf("PrintStepSummaryFile: %v", err)
	}
	if !strings.Contains(buf.String(), "compile-tests") {
		t.Errorf("replay missing step row\n---\n%s", buf.String())
	}
}

// TestRunState_StepsNilSafe: the append/snapshot/wallClock helpers tolerate a
// nil runState, mirroring appendRecord — test fixtures rely on it.
func TestRunState_StepsNilSafe(t *testing.T) {
	var rs *runState
	rs.appendStep(stepRecord{name: "x"}) // must not panic
	if got := rs.snapshotSteps(); got != nil {
		t.Errorf("nil runState snapshotSteps: got %v want nil", got)
	}
	if got := rs.wallClock(); got != 0 {
		t.Errorf("nil runState wallClock: got %v want 0", got)
	}
	if got := rs.stepsPath(); got != "" {
		t.Errorf("nil runState stepsPath: got %q want empty", got)
	}
}

// TestRunState_WallClock pins nowFn so the elapsed-since-started math is
// deterministic.
func TestRunState_WallClock(t *testing.T) {
	orig := nowFn
	defer func() { nowFn = orig }()
	base := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	rs := &runState{started: base}
	nowFn = func() time.Time { return base.Add(90 * time.Second) }
	if got := rs.wallClock(); got != 90*time.Second {
		t.Errorf("wallClock: got %v want 90s", got)
	}
	// Zero started → zero wall-clock (test fixtures that skip the field).
	if got := (&runState{}).wallClock(); got != 0 {
		t.Errorf("zero-started wallClock: got %v want 0", got)
	}
}

// TestCommandStepName_Fallbacks: prefer the command line, then task-name, then
// the generic label.
func TestCommandStepName_Fallbacks(t *testing.T) {
	cases := []struct {
		params map[string]string
		want   string
	}{
		{map[string]string{"command": "gh optivem test compile"}, "gh optivem test compile"},
		{map[string]string{"task-name": "compile-tests"}, "compile-tests"},
		{map[string]string{}, "command"},
	}
	for _, c := range cases {
		ctx := &statemachine.Context{Params: c.params, State: map[string]any{}}
		if got := commandStepName(ctx); got != c.want {
			t.Errorf("commandStepName(%v): got %q want %q", c.params, got, c.want)
		}
	}
}

// TestWrapStepRecorders captures a run-command service task: the wrapper times
// the inner NodeFn and records exactly one command step, named by the command
// param, carrying the inner Outcome's error.
func TestWrapStepRecorders(t *testing.T) {
	orig := nowFn
	defer func() { nowFn = orig }()
	base := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	calls := 0
	nowFn = func() time.Time {
		// t0 then end: advance 7s across the single wrapped call.
		calls++
		return base.Add(time.Duration(calls-1) * 7 * time.Second)
	}

	wantErr := errors.New("boom")
	eng := &statemachine.Engine{Processes: map[string]*statemachine.Process{
		"compile-tests": {
			ID: "compile-tests",
			Nodes: map[string]statemachine.Node{
				"EXECUTE_COMMAND": {
					ID:   "EXECUTE_COMMAND",
					Kind: statemachine.ServiceTask,
					Raw:  statemachine.RawNode{ID: "EXECUTE_COMMAND", Action: "run-command"},
					Fn: func(*statemachine.Context) statemachine.Outcome {
						return statemachine.Outcome{Err: wantErr}
					},
				},
				// A non-run-command service task must be left untouched.
				"PARSE": {
					ID:   "PARSE",
					Kind: statemachine.ServiceTask,
					Raw:  statemachine.RawNode{ID: "PARSE", Action: "parse-ticket"},
					Fn:   func(*statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} },
				},
			},
		},
	}}

	rs := &runState{started: base}
	wrapStepRecorders(eng, rs, nil)

	ctx := &statemachine.Context{Params: map[string]string{"command": "gh optivem test compile"}, State: map[string]any{}}
	out := eng.Processes["compile-tests"].Nodes["EXECUTE_COMMAND"].Fn(ctx)
	if !errors.Is(out.Err, wantErr) {
		t.Fatalf("wrapper dropped inner error: %v", out.Err)
	}

	steps := rs.snapshotSteps()
	if len(steps) != 1 {
		t.Fatalf("expected 1 recorded step, got %d", len(steps))
	}
	s := steps[0]
	if s.name != "gh optivem test compile" {
		t.Errorf("step name: got %q", s.name)
	}
	if s.kind != stepKindCommand {
		t.Errorf("step kind: got %q want command", s.kind)
	}
	if s.elapsed != 7*time.Second {
		t.Errorf("step elapsed: got %v want 7s", s.elapsed)
	}
	if s.err == nil {
		t.Errorf("step error not recorded")
	}

	// Fire the non-run-command task: it must NOT add a step.
	eng.Processes["compile-tests"].Nodes["PARSE"].Fn(ctx)
	if got := len(rs.snapshotSteps()); got != 1 {
		t.Errorf("non-run-command task recorded a step: now %d steps", got)
	}
}
