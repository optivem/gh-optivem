package diagram

import (
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
)

func TestRender_AllProcessesAppearAsHeadings(t *testing.T) {
	eng, err := statemachine.LoadDefault()
	if err != nil {
		t.Fatalf("load default YAML: %v", err)
	}
	got := Render(eng)

	// Every process ID in the loaded engine must appear as either an aliased
	// heading or its raw ID. Catches "added a process but forgot to update
	// processAlias / processOrder" — the renderer falls back to raw ID, so the
	// section still appears, but this also asserts we produce headings.
	for name := range eng.Processes {
		heading := processAlias[name]
		if heading == "" {
			heading = name
		}
		want := "## " + heading + "\n"
		if !strings.Contains(got, want) {
			t.Errorf("missing heading for process %q: want %q in output", name, want)
		}
	}
}

func TestRender_CallActivityLinksToTargetProcess(t *testing.T) {
	eng, err := statemachine.LoadDefault()
	if err != nil {
		t.Fatalf("load default YAML: %v", err)
	}
	got := Render(eng)

	// AT_CYCLE in main → call_activity → at_cycle. The label must include
	// the aliased "see § AT Cycle" suffix so the link is followable when
	// rendered on github.com.
	want := "see § AT Cycle"
	if !strings.Contains(got, want) {
		t.Errorf("expected call_activity link suffix %q in output", want)
	}
}

func TestRender_OutputIsDeterministic(t *testing.T) {
	// Render twice and assert byte-equal output. Ordering bugs (map
	// iteration leaking through) would surface as a random diff between
	// the two runs.
	eng, err := statemachine.LoadDefault()
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
		{"ticket_type == story", "story"},
		{"ticket_type in [story, bug]", "story / bug"},
		{"structural_test_mode in [compile, full]", "compile / full"},
	}
	for _, tc := range cases {
		if got := edgeLabel(tc.in); got != tc.want {
			t.Errorf("edgeLabel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRender_ProcessWithOutputsEmitsDataObjectAndProducesEdge(t *testing.T) {
	yaml := []byte(`
processes:
  sample_flow:
    start: WORK
    outputs:
      - alpha
      - beta
    nodes:
      - id: WORK
        type: service_task
        action: do_work
        documentation: "Do work"
      - id: SAMPLE_END
        type: end_event
    sequence_flows:
      - {from: WORK, to: SAMPLE_END}
`)
	eng, err := statemachine.LoadBytes(yaml)
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	got := Render(eng)

	for _, want := range []string{
		"SAMPLE_FLOW_OUTPUTS[/alpha, beta/]",
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
    start: WORK
    nodes:
      - id: WORK
        type: service_task
        action: do_work
        documentation: "Do work"
      - id: SAMPLE_END
        type: end_event
    sequence_flows:
      - {from: WORK, to: SAMPLE_END}
`)
	eng, err := statemachine.LoadBytes(yaml)
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	got := Render(eng)
	for _, banned := range []string{
		"_OUTPUTS",
		"produces",
		"outputNode",
	} {
		if strings.Contains(got, banned) {
			t.Errorf("unexpected %q in rendered output for process with no outputs:\n%s", banned, got)
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
