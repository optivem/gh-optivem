package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const deleteHeader = "---\nid: UC-001\ntitle: Place Order\n---\n\n# UC-001 Place Order\n\nNarrative stays.\n\n"

func TestDeleteCriteria_GherkinScenarios(t *testing.T) {
	in := deleteHeader + "```gherkin\nFeature: Place Order\n\n" +
		"@UC-001.1\nScenario: Placed\n  When the customer places an order\n  Then the order is placed\n\n" +
		"# outline for the same path\n@UC-001.2 @narrative-4a\nScenario Outline: Priced\n  Given price <p>\n  Then total <t>\n\n  Examples:\n    | p | t |\n    | 1 | 1 |\n\n" +
		"@UC-001.2\nScenario: Priced again\n  Then total 2\n\n" +
		"@UC-001.3\nScenario: Discounted\n  Then discount 1\n```\n"
	out, err := DeleteCriteria("a.md", in, []string{"UC-001.2"})
	if err != nil {
		t.Fatal(err)
	}
	want := deleteHeader + "```gherkin\nFeature: Place Order\n\n" +
		"@UC-001.1\nScenario: Placed\n  When the customer places an order\n  Then the order is placed\n\n" +
		"@UC-001.3\nScenario: Discounted\n  Then discount 1\n```\n"
	if out != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestDeleteCriteria_LastScenarioLeavesNoDoubleBlank(t *testing.T) {
	in := deleteHeader + "```gherkin\nFeature: F\n\n@UC-001.1\nScenario: A\n  Then a\n\n@UC-001.2\nScenario: B\n  Then b\n\n```\n"
	out, err := DeleteCriteria("a.md", in, []string{"UC-001.2"})
	if err != nil {
		t.Fatal(err)
	}
	want := deleteHeader + "```gherkin\nFeature: F\n\n@UC-001.1\nScenario: A\n  Then a\n\n```\n"
	if out != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out, want)
	}
}

func TestDeleteCriteria_EveryCriterionLeavesTheProse(t *testing.T) {
	in := deleteHeader + "```gherkin\nFeature: F\n\n@UC-001.1\nScenario: A\n  Then a\n```\n"
	out, err := DeleteCriteria("a.md", in, []string{"UC-001.1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Narrative stays.") || strings.Contains(out, "Scenario") {
		t.Fatalf("got:\n%s", out)
	}
	if uc := Parse("a.md", out); len(uc.Criteria) != 0 || uc.FrontErr != "" || len(Validate([]UseCase{uc})) != 0 {
		t.Fatalf("emptied use case must parse and validate: %+v", uc)
	}
}

func TestDeleteCriteria_Bullets(t *testing.T) {
	in := deleteHeader + "## Rules\n\n- UC-001.1: Placed.\n- UC-001.2 [flag: dark]: Priced,\n  continued here.\n- UC-001.3: Discounted.\n\nTrailing prose.\n"
	out, err := DeleteCriteria("a.md", in, []string{"UC-001.2", "UC-001.1"})
	if err != nil {
		t.Fatal(err)
	}
	want := deleteHeader + "## Rules\n\n- UC-001.3: Discounted.\n\nTrailing prose.\n"
	if out != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestDeleteCriteria_KeepsCRLF(t *testing.T) {
	in := strings.ReplaceAll(deleteHeader+"- UC-001.1: a\n- UC-001.2: b\n", "\n", "\r\n")
	out, err := DeleteCriteria("a.md", in, []string{"UC-001.1"})
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.ReplaceAll(deleteHeader+"- UC-001.2: b\n", "\n", "\r\n"); out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

func TestDeleteCriteria_UnknownIDFails(t *testing.T) {
	in := deleteHeader + "- UC-001.1: a\n"
	if _, err := DeleteCriteria("a.md", in, []string{"UC-001.1", "UC-001.9"}); err == nil || !strings.Contains(err.Error(), "UC-001.9") {
		t.Fatalf("err = %v, want one naming UC-001.9", err)
	}
	if _, err := DeleteCriteria("a.md", in, nil); err == nil {
		t.Fatal("no ids must be an error")
	}
}

func TestDeleteCriteria_SampleFile(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "gherkin", "UC-001-acceptance-criteria.md"))
	if err != nil {
		t.Fatal(err)
	}
	data := string(raw)
	uc := Parse("UC-001-acceptance-criteria.md", data)
	var ids []string
	for _, c := range uc.Criteria {
		ids = append(ids, c.ID)
	}
	out, err := DeleteCriteria(uc.File, data, ids[:1])
	if err != nil {
		t.Fatal(err)
	}
	if got := len(Parse(uc.File, out).Criteria); got != len(ids)-1 {
		t.Fatalf("criteria left = %d, want %d", got, len(ids)-1)
	}
}
