package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRender_AllSectionsAppearAsHeadings(t *testing.T) {
	doc, err := LoadDefault()
	if err != nil {
		t.Fatalf("load default YAML: %v", err)
	}
	got := Render(doc)
	for _, sec := range doc.Sections {
		want := "## " + sec.Heading + "\n"
		if !strings.Contains(got, want) {
			t.Errorf("missing heading %q in rendered output", want)
		}
	}
}

func TestRender_RefSuffixUsesTargetSectionHeading(t *testing.T) {
	doc, err := LoadDefault()
	if err != nil {
		t.Fatalf("load default YAML: %v", err)
	}
	got := Render(doc)
	// In Overview, DSL_PORT carries ref: dsl_port → label must include
	// the resolved heading suffix "see § DSL Port", proving the slug
	// lookup hit the dsl_port section's heading rather than the slug
	// itself.
	want := "DSL_PORT[DSL Port — see § DSL Port]"
	if !strings.Contains(got, want) {
		t.Errorf("expected ref-suffixed label %q in output", want)
	}
}

func TestRender_EdgeStylesEmitCorrectMermaid(t *testing.T) {
	yaml := []byte(`
sections:
  - name: probe
    heading: Probe
    nodes:
      - {id: A, label: alpha}
      - {id: B, label: beta}
    edges:
      - {from: A, to: B}                          # -->
      - {from: A, to: B, label: hop}              # -->|hop|
      - {from: A, to: B, style: undirected}       # ---
`)
	doc, err := Parse(yaml)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := Render(doc)
	for _, want := range []string{
		"    A --> B\n",
		"    A -->|hop| B\n",
		"    A --- B\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing edge form %q in rendered output:\n%s", want, got)
		}
	}
}

func TestRender_DirectionDefaultsToTD(t *testing.T) {
	yaml := []byte(`
sections:
  - name: probe
    heading: Probe
    nodes:
      - {id: A, label: alpha}
    edges: []
`)
	doc, err := Parse(yaml)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := Render(doc)
	if !strings.Contains(got, "flowchart TD\n") {
		t.Errorf("expected default direction TD; got:\n%s", got)
	}
}

func TestRender_OutputIsDeterministic(t *testing.T) {
	doc, err := LoadDefault()
	if err != nil {
		t.Fatalf("load default YAML: %v", err)
	}
	a := Render(doc)
	b := Render(doc)
	if a != b {
		t.Errorf("Render output not deterministic across two calls")
	}
}

// TestRender_MatchesCommittedGolden asserts the renderer output is
// byte-identical to the committed `docs/architecture-diagram.md`. When
// this fires after a YAML or renderer change, run
// `go run . architecture show > docs/architecture-diagram.md` and commit
// the diff — the same step the regenerate workflow runs on push.
func TestRender_MatchesCommittedGolden(t *testing.T) {
	doc, err := LoadDefault()
	if err != nil {
		t.Fatalf("load default YAML: %v", err)
	}
	got := Render(doc)

	// Walk up to repo root: this file lives at
	// internal/atdd/runtime/architecture/, four levels deep.
	root := filepath.Join("..", "..", "..", "..")
	goldenPath := filepath.Join(root, "docs", "architecture-diagram.md")
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenPath, err)
	}
	if got != string(want) {
		t.Errorf("rendered output drifts from %s; regenerate with `go run . architecture show > docs/architecture-diagram.md`", goldenPath)
	}
}
