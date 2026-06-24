package diagram

import (
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/process"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

func TestRender_AllProcessesAppearAsHeadings(t *testing.T) {
	eng, err := process.Load()
	if err != nil {
		t.Fatalf("load default YAML: %v", err)
	}
	got := Render(eng)

	// Every process's explicit `name:` must appear as a heading. Catches
	// "added a process but forgot to update processOrder" — the renderer
	// emits headings in process-order then lexical, so the section still
	// appears, but this asserts every name is present.
	for id, process := range eng.Processes {
		want := "## " + process.Name + "\n"
		if !strings.Contains(got, want) {
			t.Errorf("missing heading for process %q: want %q in output", id, want)
		}
	}
}

func TestRender_CallActivityLinksToTargetProcess(t *testing.T) {
	eng, err := process.Load()
	if err != nil {
		t.Fatalf("load default YAML: %v", err)
	}
	got := Render(eng)

	// Every MID's EXECUTE_AGENT call-activity targets `execute-agent`
	// with documentation "Dispatch the Agent" — distinct from the
	// target heading "Execute Agent", so the collapse rule keeps the
	// "see § …" suffix. The assertion locks in that the renderer
	// still emits the link suffix for non-redundant labels so cross-
	// section navigation works on github.com.
	want := "see § execute-agent"
	if !strings.Contains(got, want) {
		t.Errorf("expected call-activity link suffix %q in output", want)
	}
}

func TestRender_OutputIsDeterministic(t *testing.T) {
	// Render twice and assert byte-equal output. Ordering bugs (map
	// iteration leaking through) would surface as a random diff between
	// the two runs.
	eng, err := process.Load()
	if err != nil {
		t.Fatalf("load default YAML: %v", err)
	}
	a := Render(eng)
	b := Render(eng)
	if a != b {
		t.Errorf("Render output not deterministic across two calls")
	}
}

func TestEdgeLabel(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"x == true", "Yes"},
		{"x == false", "No"},
		{"ticket_type == story", "Story"},
		{"ticket_type == refactor-system-structure", "Refactor System Structure"},
		{"ticket_type == task/cover-legacy", "Task / Cover Legacy"},
		{"ticket_type in [story, bug]", "Story / Bug"},
		{"structural_test_mode in [compile, full]", "Compile / Full"},
	}
	for _, tc := range cases {
		if got := edgeLabel(tc.in); got != tc.want {
			t.Errorf("edgeLabel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTitleCaseFromKebab(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"refine-ticket", "Refine Ticket"},
		{"implement-and-verify-system", "Implement and Verify System"},
		{"implement-dsl", "Implement DSL"},
		{"implement-and-verify-dsl", "Implement and Verify DSL"},
		{"task/cover-legacy", "Task / Cover Legacy"},
		{"story", "Story"},
		{"main", "Main"},
		{"write-and-verify-acceptance-test-code", "Write and Verify Acceptance Test Code"},
	}
	for _, tc := range cases {
		if got := titleCaseFromKebab(tc.in); got != tc.want {
			t.Errorf("titleCaseFromKebab(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRender_ProcessWithOutputsEmitsDataObjectAndProducesEdge(t *testing.T) {
	yaml := []byte(`
processes:
  sample_flow:
    name: "Sample Flow"
    start: WORK
    outputs:
      - key: alpha
        type: bool
      - key: beta
        type: string-list
        optional: true
    nodes:
      - id: WORK
        type: service-task
        action: do_work
        name: "Do work"
      - id: SAMPLE_END
        type: end-event
        name: "Synthetic Test Event"
    sequence-flows:
      - {from: WORK, to: SAMPLE_END}
`)
	eng, err := statemachine.LoadBytes(yaml)
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	got := Render(eng)

	for _, want := range []string{
		// Renderer formats each spec as "key[?]: type", one per line via
		// <br/>; the <br/> forces mermaidLabel to quote the whole label.
		`SAMPLE_FLOW_OUTPUTS[/"alpha: bool<br/>beta?: string-list"/]`,
		"SAMPLE_END -. produces .-> SAMPLE_FLOW_OUTPUTS",
		"classDef outputNode",
		"class SAMPLE_FLOW_OUTPUTS outputNode",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in rendered output:\n%s", want, got)
		}
	}
}

func TestRender_ProcessWithoutOutputsHasNoDataObject(t *testing.T) {
	yaml := []byte(`
processes:
  sample_flow:
    name: "Sample Flow"
    start: WORK
    nodes:
      - id: WORK
        type: service-task
        action: do_work
        name: "Do work"
      - id: SAMPLE_END
        type: end-event
        name: "Synthetic Test Event"
    sequence-flows:
      - {from: WORK, to: SAMPLE_END}
`)
	eng, err := statemachine.LoadBytes(yaml)
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	got := Render(eng)
	// `outputNode` is intentionally emitted once by the top-level Legend
	// section regardless of process outputs, so it isn't a useful ban
	// here — the per-process `_OUTPUTS` node ID and `produces` edge verb
	// are sufficient to verify the data-object block was skipped.
	for _, banned := range []string{
		"_OUTPUTS",
		"produces",
	} {
		if strings.Contains(got, banned) {
			t.Errorf("unexpected %q in rendered output for process with no outputs:\n%s", banned, got)
		}
	}
}

func TestRenderExpanded_TopLevelSectionsPresent(t *testing.T) {
	eng, err := process.Load()
	if err != nil {
		t.Fatalf("load default YAML: %v", err)
	}
	got := RenderExpanded(eng)

	// Every process marked diagram-section-order > 0 must appear as a ## heading.
	for _, proc := range sectionRoots(eng) {
		want := "## " + proc.Name + "\n"
		if !strings.Contains(got, want) {
			t.Errorf("missing heading for section process %q: want %q in output", proc.ID, want)
		}
	}
}

func TestRenderExpanded_OutputIsDeterministic(t *testing.T) {
	eng, err := process.Load()
	if err != nil {
		t.Fatalf("load default YAML: %v", err)
	}
	a := RenderExpanded(eng)
	b := RenderExpanded(eng)
	if a != b {
		t.Errorf("RenderExpanded output not deterministic across two calls")
	}
}

func TestRenderExpanded_SubgraphAppearsForCallActivity(t *testing.T) {
	// Use process ID "main" with diagram-section-order: 1 so RenderExpanded
	// renders it as a section.
	yaml := []byte(`
processes:
  main:
    name: "Main"
    diagram-section-order: 1
    start: START
    nodes:
      - id: START
        type: start-event
        name: "Start"
      - id: CALL_SUB
        type: call-activity
        name: "Run Sub"
        process: sub
      - id: END
        type: end-event
        name: "End"
    sequence-flows:
      - {from: START, to: CALL_SUB}
      - {from: CALL_SUB, to: END}
  sub:
    name: "Sub Process"
    start: SUB_START
    nodes:
      - id: SUB_START
        type: start-event
        name: "Sub Start"
      - id: SUB_WORK
        type: service-task
        action: do_work
        name: "Do Work"
      - id: SUB_END
        type: end-event
        name: "Sub End"
    sequence-flows:
      - {from: SUB_START, to: SUB_WORK}
      - {from: SUB_WORK, to: SUB_END}
`)
	eng, err := statemachine.LoadBytes(yaml)
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	got := RenderExpanded(eng)

	for _, want := range []string{
		"subgraph CALL_SUB[Sub Process]",
		"CALL_SUB__SUB_WORK[[Do Work]]",
		"START --> CALL_SUB__SUB_START",
		"CALL_SUB__SUB_END --> END",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in expanded output:\n%s", want, got)
		}
	}
}

func TestMermaidLabel_QuotesReservedChars(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"plain label", "plain label"},
		{"COMMIT: Ticket | AT - RED - TEST", `"COMMIT: Ticket | AT - RED - TEST"`},
		{"label with (parens)", `"label with (parens)"`},
	}
	for _, tc := range cases {
		if got := mermaidLabel(tc.in); got != tc.want {
			t.Errorf("mermaidLabel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
