package diagram

import (
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
)

func TestRender_AllFlowsAppearAsHeadings(t *testing.T) {
	eng, err := statemachine.LoadDefault()
	if err != nil {
		t.Fatalf("load default YAML: %v", err)
	}
	got := Render(eng)

	// Every flow ID in the loaded engine must appear as either an aliased
	// heading or its raw ID. Catches "added a flow but forgot to update
	// flowAlias / flowOrder" — the renderer falls back to raw ID, so the
	// section still appears, but this also asserts we produce headings.
	for name := range eng.Flows {
		heading := flowAlias[name]
		if heading == "" {
			heading = name
		}
		want := "## " + heading + "\n"
		if !strings.Contains(got, want) {
			t.Errorf("missing heading for flow %q: want %q in output", name, want)
		}
	}
}

func TestRender_CallActivityLinksToTargetFlow(t *testing.T) {
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
