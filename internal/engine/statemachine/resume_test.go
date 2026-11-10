package statemachine_test

import (
	"bytes"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

const resumeFlowYAML = `
processes:
  root:
    name: "Root"
    start: FIRST
    nodes:
      - id: FIRST
        type: call-activity
        process: unit
        name: "First"
      - id: SECOND
        type: call-activity
        process: unit
        name: "Second"
      - id: EXTERNAL
        type: call-activity
        process: unit
        name: "External"
        params:
          external-system-name: erp
      - id: AFTER
        type: service-task
        action: after
        name: "After"
      - id: ROOT_END
        type: end-event
        name: "End"
    sequence-flows:
      - { from: FIRST, to: SECOND }
      - { from: SECOND, to: EXTERNAL }
      - { from: EXTERNAL, to: AFTER }
      - { from: AFTER, to: ROOT_END }
  unit:
    name: "Unit"
    start: WORK
    outputs:
      - key: test-names
        type: string-list
      - key: dsl-port-changed
        type: bool
      - key: real-kind
        type: string
      - key: scope-exception-reason
        type: string
        optional: true
    nodes:
      - id: WORK
        type: service-task
        action: work
        name: "Work"
      - id: COMMIT
        type: service-task
        action: commit
        name: "Commit"
      - id: UNIT_END
        type: end-event
        name: "End"
    sequence-flows:
      - { from: WORK, to: COMMIT }
      - { from: COMMIT, to: UNIT_END }
`

type fakeProbe struct {
	commits map[string]statemachine.UnitCommit // key: ticket|unit
	clean   bool
}

func (f *fakeProbe) FindUnitCommit(ticket, unit string) (statemachine.UnitCommit, bool, error) {
	c, ok := f.commits[ticket+"|"+unit]
	return c, ok, nil
}

func (f *fakeProbe) WorkingTreeClean() (bool, error) { return f.clean, nil }

type resumeRig struct {
	eng      *statemachine.Engine
	probe    *fakeProbe
	log      *bytes.Buffer
	worked   []string // units whose work ran, by ctx.Unit
	trailers map[string][]string
}

func newResumeRig(t *testing.T, clean bool) *resumeRig {
	t.Helper()
	eng, err := statemachine.LoadBytes([]byte(resumeFlowYAML))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	rig := &resumeRig{
		eng:      eng,
		probe:    &fakeProbe{commits: map[string]statemachine.UnitCommit{}, clean: clean},
		log:      &bytes.Buffer{},
		trailers: map[string][]string{},
	}
	eng.ActionFn = func(name string) statemachine.NodeFn {
		return func(ctx *statemachine.Context) statemachine.Outcome {
			switch name {
			case "work":
				rig.worked = append(rig.worked, ctx.Unit)
				ctx.Set("test-names", []string{"shouldA", "shouldB"})
				ctx.Set("dsl-port-changed", true)
				ctx.Set("real-kind", "simulator")
			case "commit":
				tr, err := eng.CommitTrailers(ctx)
				if err != nil {
					return statemachine.Outcome{Err: err}
				}
				rig.trailers[ctx.Unit] = tr
			}
			return statemachine.Outcome{}
		}
	}
	eng.GateFn = func(string) statemachine.NodeFn { return nil }
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	eng.Resume = &statemachine.ResumeConfig{
		Units:             map[string]bool{"unit": true},
		ExcludeWhenParams: []string{"external-system-name"},
		Probe:             rig.probe,
		Log:               rig.log,
	}
	return rig
}

func newResumeCtx() *statemachine.Context {
	ctx := statemachine.NewContext()
	ctx.Set("ticket-id", "65")
	return ctx
}

func trailerValue(trailers []string, key string) string {
	for _, t := range trailers {
		if v, ok := strings.CutPrefix(t, key+": "); ok {
			return v
		}
	}
	return ""
}

// commitFor runs a fresh rig to completion and returns the UnitCommit the
// unit FIRST would have left behind.
func commitFor(t *testing.T, sha string) statemachine.UnitCommit {
	t.Helper()
	rig := newResumeRig(t, true)
	if err := rig.eng.RunProcess("root", newResumeCtx()); err != nil {
		t.Fatalf("RunProcess: %v", err)
	}
	return statemachine.UnitCommit{SHA: sha, State: trailerValue(rig.trailers["root@FIRST"], statemachine.TrailerState)}
}

func TestResume_NoTrailerRunsAndMarksCommit(t *testing.T) {
	rig := newResumeRig(t, true)
	if err := rig.eng.RunProcess("root", newResumeCtx()); err != nil {
		t.Fatalf("RunProcess: %v", err)
	}
	if want := []string{"root@FIRST", "root@SECOND", ""}; !reflect.DeepEqual(rig.worked, want) {
		t.Fatalf("worked = %q, want %q (external call runs unmarked)", rig.worked, want)
	}
	tr := rig.trailers["root@FIRST"]
	if got := trailerValue(tr, statemachine.TrailerTicket); got != "65" {
		t.Errorf("ticket trailer = %q, want 65", got)
	}
	if got := trailerValue(tr, statemachine.TrailerUnit); got != "root@FIRST" {
		t.Errorf("unit trailer = %q, want root@FIRST", got)
	}
	if got := trailerValue(tr, statemachine.TrailerState); got != `{"dsl-port-changed":true,"real-kind":"simulator","test-names":["shouldA","shouldB"]}` {
		t.Errorf("state trailer = %q", got)
	}
	if len(rig.trailers[""]) != 0 {
		t.Errorf("external-system call must not be marked, got %v", rig.trailers[""])
	}
}

func TestResume_TrailerPresentAndTreeCleanSkipsAndRestoresState(t *testing.T) {
	proof := commitFor(t, "abc1234567890def")
	rig := newResumeRig(t, true)
	rig.probe.commits["65|root@FIRST"] = proof

	ctx := newResumeCtx()
	if err := rig.eng.RunProcess("root", ctx); err != nil {
		t.Fatalf("RunProcess: %v", err)
	}
	for _, u := range rig.worked {
		if u == "root@FIRST" {
			t.Fatalf("FIRST must be skipped, worked = %q", rig.worked)
		}
	}
	if got, _ := ctx.State["test-names"].([]string); !reflect.DeepEqual(got, []string{"shouldA", "shouldB"}) {
		t.Errorf("test-names = %#v, want typed []string restored", ctx.State["test-names"])
	}
	if got, _ := ctx.State["dsl-port-changed"].(bool); !got {
		t.Errorf("dsl-port-changed = %#v, want true", ctx.State["dsl-port-changed"])
	}
	if got := ctx.GetString("real-kind"); got != "simulator" {
		t.Errorf("real-kind = %q, want simulator", got)
	}
	logged := rig.log.String()
	if !strings.Contains(logged, "root@FIRST") || !strings.Contains(logged, "abc1234567890def") {
		t.Errorf("skip must log unit id and proving SHA, got %q", logged)
	}
}

func TestResume_DirtyTreeReRuns(t *testing.T) {
	proof := commitFor(t, "abc1234")
	rig := newResumeRig(t, false)
	rig.probe.commits["65|root@FIRST"] = proof

	if err := rig.eng.RunProcess("root", newResumeCtx()); err != nil {
		t.Fatalf("RunProcess: %v", err)
	}
	if len(rig.worked) == 0 || rig.worked[0] != "root@FIRST" {
		t.Fatalf("dirty tree must re-run FIRST, worked = %q", rig.worked)
	}
	if !strings.Contains(rig.log.String(), "uncommitted") {
		t.Errorf("re-run reason must be logged, got %q", rig.log.String())
	}
}

func TestResume_PartiallyCommittedRunSkipsOnlyProvenUnits(t *testing.T) {
	proof := commitFor(t, "abc1234")
	rig := newResumeRig(t, true)
	rig.probe.commits["65|root@FIRST"] = proof // SECOND has no marker

	if err := rig.eng.RunProcess("root", newResumeCtx()); err != nil {
		t.Fatalf("RunProcess: %v", err)
	}
	if want := []string{"root@SECOND", ""}; !reflect.DeepEqual(rig.worked, want) {
		t.Fatalf("worked = %q, want %q", rig.worked, want)
	}
}

func TestResume_OtherTicketMarkerNeverCounts(t *testing.T) {
	proof := commitFor(t, "abc1234")
	rig := newResumeRig(t, true)
	rig.probe.commits["12|root@FIRST"] = proof

	if err := rig.eng.RunProcess("root", newResumeCtx()); err != nil {
		t.Fatalf("RunProcess: %v", err)
	}
	if rig.worked[0] != "root@FIRST" {
		t.Fatalf("another ticket's marker must not skip FIRST, worked = %q", rig.worked)
	}
}

func TestResume_DisabledWithoutConfig(t *testing.T) {
	rig := newResumeRig(t, true)
	rig.eng.Resume = nil
	if err := rig.eng.RunProcess("root", newResumeCtx()); err != nil {
		t.Fatalf("RunProcess: %v", err)
	}
	if len(rig.worked) != 3 {
		t.Fatalf("worked = %q, want all three calls run", rig.worked)
	}
	if len(rig.trailers[""]) != 0 {
		t.Errorf("no resume config means no trailers, got %v", rig.trailers[""])
	}
}

func TestResume_CorruptStateFailsLoud(t *testing.T) {
	cases := map[string]string{
		"not json":        `{`,
		"unknown key":     `{"nope":"x"}`,
		"mistyped bool":   `{"dsl-port-changed":"yes"}`,
		"mistyped list":   `{"test-names":"a,b"}`,
		"envelope key":    `{"scope-exception-reason":"x"}`,
		"list non-string": `{"test-names":[1]}`,
	}
	for name, state := range cases {
		t.Run(name, func(t *testing.T) {
			rig := newResumeRig(t, true)
			rig.probe.commits["65|root@FIRST"] = statemachine.UnitCommit{SHA: "deadbeef", State: state}
			err := rig.eng.RunProcess("root", newResumeCtx())
			if err == nil {
				t.Fatalf("corrupt %s trailer must fail the run, not be skipped or ignored", statemachine.TrailerState)
			}
			if len(rig.worked) != 0 {
				t.Errorf("nothing may run after a corrupt marker, worked = %q", rig.worked)
			}
		})
	}
}

func TestResume_MissingTicketFailsLoud(t *testing.T) {
	rig := newResumeRig(t, true)
	if err := rig.eng.RunProcess("root", statemachine.NewContext()); err == nil {
		t.Fatal("a resumable unit without ticket-id must fail loud")
	}
}

func TestParseTrailerLines(t *testing.T) {
	msg := "#65 Add thing - DSL\n\nAtdd-Ticket: 65\nAtdd-Unit: a@B\nAtdd-State: {\"k\":\"v: w\"}\nCo-Authored-By: X <x@y>\n"
	if got := statemachine.ParseTrailerLines(msg, statemachine.TrailerState); !reflect.DeepEqual(got, []string{`{"k":"v: w"}`}) {
		t.Errorf("state = %q", got)
	}
	if got := statemachine.ParseTrailerLines(msg, "Missing"); len(got) != 0 {
		t.Errorf("missing key = %q, want none", got)
	}
}

// TestResume_UnitKeyStateScopesUnitPerItem: a unit entered once per item
// (characterize's layers, once per use case) is marked and proven per item, so
// an earlier item's marker never skips the unit for a later item.
func TestResume_UnitKeyStateScopesUnitPerItem(t *testing.T) {
	proof := commitFor(t, "abc1234")
	newRig := func() (*resumeRig, *statemachine.Context) {
		rig := newResumeRig(t, true)
		rig.eng.Resume.UnitKeyState = []string{"use-case-id"}
		ctx := newResumeCtx()
		ctx.Set("use-case-id", "UC-002")
		return rig, ctx
	}

	rig, ctx := newRig()
	rig.probe.commits["65|root@FIRST@UC-001"] = proof
	if err := rig.eng.RunProcess("root", ctx); err != nil {
		t.Fatalf("RunProcess: %v", err)
	}
	if rig.worked[0] != "root@FIRST@UC-002" {
		t.Fatalf("another use case's marker must not skip FIRST, worked = %q", rig.worked)
	}
	if got := trailerValue(rig.trailers["root@FIRST@UC-002"], statemachine.TrailerUnit); got != "root@FIRST@UC-002" {
		t.Errorf("unit trailer = %q, want root@FIRST@UC-002", got)
	}

	rig, ctx = newRig()
	rig.probe.commits["65|root@FIRST@UC-002"] = proof
	if err := rig.eng.RunProcess("root", ctx); err != nil {
		t.Fatalf("RunProcess: %v", err)
	}
	if slices.Contains(rig.worked, "root@FIRST@UC-002") {
		t.Fatalf("the same use case's marker must skip FIRST, worked = %q", rig.worked)
	}
}
