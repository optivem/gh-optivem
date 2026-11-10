package spec

import (
	"strings"
	"testing"
)

const depsDiagram = "# Use case dependencies\n\nPlain prose is ignored, also UC-009 --> UC-010.\n\n" +
	"```mermaid\nflowchart LR\n" +
	"    %% nodes\n" +
	"    UC-001[\"Register Product\"]\n" +
	"    UC-002[\"Register Customer\"]\n" +
	"    UC-003[\"Place Order\"]\n" +
	"    UC-010[\"Cancel Order\"]\n" +
	"\n" +
	"    UC-002 -->|\"orders need a customer\"| UC-003\n" +
	"    UC-001 -->|orders need a product| UC-003;\n" +
	"    UC-003 -->|\"only placed orders can be cancelled\"| UC-010\n" +
	"```\n"

func TestParseDependencies(t *testing.T) {
	d, err := ParseDependencies(strings.ReplaceAll(depsDiagram, "\n", "\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(d.UseCases, ","); got != "UC-001,UC-002,UC-003,UC-010" {
		t.Errorf("UseCases = %s", got)
	}
	if d.Titles["UC-003"] != "Place Order" {
		t.Errorf("title = %q", d.Titles["UC-003"])
	}
	want := []DependencyEdge{
		{"UC-002", "UC-003", "orders need a customer"},
		{"UC-001", "UC-003", "orders need a product"},
		{"UC-003", "UC-010", "only placed orders can be cancelled"},
	}
	if len(d.Edges) != len(want) {
		t.Fatalf("edges = %+v", d.Edges)
	}
	for i := range want {
		if d.Edges[i] != want[i] {
			t.Errorf("edge %d = %+v, want %+v", i, d.Edges[i], want[i])
		}
	}
}

func TestParseDependencies_Errors(t *testing.T) {
	block := func(lines ...string) string {
		return "```mermaid\nflowchart LR\n" + strings.Join(lines, "\n") + "\n```\n"
	}
	cases := map[string]struct{ content, want string }{
		"no block":         {"# Use case dependencies\n", "no mermaid block"},
		"two blocks":       {block(`UC-001["A"]`) + block(`UC-002["B"]`), "more than one mermaid block"},
		"unclosed":         {"```mermaid\nflowchart LR\nUC-001[\"A\"]\n", "not closed"},
		"no header":        {"```mermaid\nUC-001[\"A\"]\n```\n", "flowchart"},
		"unknown id":       {block(`UC-001["A"]`, `UC-001 -->|"x"| UC-002`), "unknown use case UC-002"},
		"no reason":        {block(`UC-001["A"]`, `UC-002["B"]`, `UC-001 --> UC-002`), "write an edge as"},
		"empty reason":     {block(`UC-001["A"]`, `UC-002["B"]`, `UC-001 -->|""| UC-002`), "has no reason"},
		"duplicate node":   {block(`UC-001["A"]`, `UC-001["B"]`), "declared twice"},
		"stray line":       {block(`UC-001["A"]`, `style UC-001 fill:#f00`), "not a use case node"},
		"labelled on edge": {block(`UC-001["A"]`, `UC-002["B"]`, `UC-001["A"] -->|"x"| UC-002`), "write an edge as"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseDependencies(tc.content)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func mustDeps(t *testing.T, nodes []string, edges ...[2]string) Dependencies {
	t.Helper()
	var b strings.Builder
	b.WriteString("```mermaid\nflowchart LR\n")
	for _, n := range nodes {
		b.WriteString(n + "[\"" + n + "\"]\n")
	}
	for _, e := range edges {
		b.WriteString(e[0] + " -->|\"needs it\"| " + e[1] + "\n")
	}
	b.WriteString("```\n")
	d, err := ParseDependencies(b.String())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDependencies_Order(t *testing.T) {
	// UC-010 depends on UC-003 (through UC-009, outside the ordered set),
	// UC-003 on UC-002; UC-001 and UC-002 tie and go by number.
	d := mustDeps(t, []string{"UC-001", "UC-002", "UC-003", "UC-009", "UC-010"},
		[2]string{"UC-002", "UC-003"}, [2]string{"UC-003", "UC-009"}, [2]string{"UC-009", "UC-010"})
	got, err := d.Order([]string{"UC-010", "UC-003", "UC-002", "UC-001", "UC-003"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "UC-001,UC-002,UC-003,UC-010" {
		t.Errorf("order = %v", got)
	}
	// A dependency goes first even when its number is higher.
	d = mustDeps(t, []string{"UC-002", "UC-010"}, [2]string{"UC-010", "UC-002"})
	if got, _ := d.Order(d.UseCases); strings.Join(got, ",") != "UC-010,UC-002" {
		t.Errorf("order = %v", got)
	}
	if p := d.Prerequisites()["UC-002"]; !p["UC-010"] || len(p) != 1 {
		t.Errorf("prerequisites = %v", p)
	}
}

func TestDependencies_Cycle(t *testing.T) {
	d := mustDeps(t, []string{"UC-001", "UC-002", "UC-003", "UC-004"},
		[2]string{"UC-004", "UC-001"}, [2]string{"UC-001", "UC-002"}, [2]string{"UC-002", "UC-003"}, [2]string{"UC-003", "UC-001"})
	_, err := d.Order([]string{"UC-004"})
	if err == nil || !strings.Contains(err.Error(), "UC-001 --> UC-002 --> UC-003 --> UC-001") {
		t.Errorf("err = %v", err)
	}
	self := mustDeps(t, []string{"UC-001"}, [2]string{"UC-001", "UC-001"})
	if c := self.Cycle(); strings.Join(c, ",") != "UC-001,UC-001" {
		t.Errorf("self cycle = %v", c)
	}
	if c := mustDeps(t, []string{"UC-001", "UC-002"}, [2]string{"UC-001", "UC-002"}).Cycle(); c != nil {
		t.Errorf("acyclic: cycle = %v", c)
	}
}

func TestCompareUseCaseIDs(t *testing.T) {
	if CompareUseCaseIDs("UC-9", "UC-10") >= 0 || CompareUseCaseIDs("UC-010", "UC-002") <= 0 || CompareUseCaseIDs("UC-001", "UC-001") != 0 {
		t.Error("UC ids must compare by number")
	}
}
