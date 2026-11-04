package spec

import (
	"slices"
	"strings"
	"testing"
)

const storyBody = `Feature: Checkout

  @UC-001.1
  Scenario: place a valid order
    Given a product
    When I order it
    Then the order is placed

  # covers UC-001.2
  Scenario: reject a zero quantity
    Given a product
    When I order 0 of it
    Then the order is rejected

  Rule: Large orders ship free mentioning UC-009.9

    Scenario: free shipping
      Given a large cart
      When I check out
      Then shipping is free
`

func TestStoryScenarios(t *testing.T) {
	got := StoryScenarios(storyBody)
	if len(got) != 3 {
		t.Fatalf("scenarios = %+v", got)
	}
	if got[0].Title != "place a valid order" || !slices.Equal(got[0].Refs, []string{"UC-001.1"}) {
		t.Errorf("first = %+v", got[0])
	}
	if !slices.Equal(got[1].Refs, []string{"UC-001.2"}) {
		t.Errorf("a comment line above the header belongs to its scenario: %+v", got[1])
	}
	if len(got[2].Refs) != 0 {
		t.Errorf("a Rule line must not leak its ids into the previous or next block: %+v", got[2])
	}
}

func mergeUseCases(t *testing.T) []UseCase {
	t.Helper()
	return []UseCase{Parse("UC-001.md", "---\nid: UC-001\ntitle: Place Order\n---\n- UC-001.1: a\n- UC-001.2: b\n")}
}

func TestStoryAlreadyMerged(t *testing.T) {
	ucs := mergeUseCases(t)
	two := StoryScenarios(strings.Join(strings.Split(storyBody, "  Rule:")[:1], ""))
	ok, ids := StoryAlreadyMerged(two, ucs)
	if !ok || !slices.Equal(ids, []string{"UC-001.1", "UC-001.2"}) {
		t.Fatalf("all scenarios name existing criteria: ok=%v ids=%v", ok, ids)
	}
	if ok, _ := StoryAlreadyMerged(StoryScenarios(storyBody), ucs); ok {
		t.Fatal("a scenario naming no criterion needs a merge")
	}
	unknown := []StoryScenario{{Title: "x", Refs: []string{"UC-777.1"}}}
	if ok, _ := StoryAlreadyMerged(unknown, ucs); ok {
		t.Fatal("a reference to a criterion that does not exist needs a merge")
	}
	if ok, _ := StoryAlreadyMerged(nil, ucs); ok {
		t.Fatal("a story with no scenarios is never a no-op")
	}
}

func TestParseScenarioMap(t *testing.T) {
	m, err := ParseScenarioMap([]string{"place a valid order => UC-001.1", "reject  zero => UC-001.2, UC-001.1"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(m["reject zero"], []string{"UC-001.1", "UC-001.2"}) {
		t.Fatalf("map = %v", m)
	}
	for _, bad := range []string{"no arrow", " => UC-001.1", "title => no id"} {
		if _, err := ParseScenarioMap([]string{bad}); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
}

func TestCheckStoryMerge(t *testing.T) {
	ucs := mergeUseCases(t)
	scs := []StoryScenario{{Title: "place a valid order"}, {Title: "reject a zero quantity"}}
	problems, covered := CheckStoryMerge(scs, map[string][]string{
		"place a valid order":    {"UC-001.1"},
		"reject a zero quantity": {"UC-001.2"},
	}, ucs)
	if len(problems) != 0 || !slices.Equal(covered, []string{"UC-001.1", "UC-001.2"}) {
		t.Fatalf("complete merge: problems=%v covered=%v", problems, covered)
	}

	problems, _ = CheckStoryMerge(scs, map[string][]string{"place a valid order": {"UC-001.1"}}, ucs)
	if len(problems) != 1 || !strings.Contains(problems[0], "reject a zero quantity") {
		t.Fatalf("unmapped scenario must be a gap: %v", problems)
	}

	problems, _ = CheckStoryMerge(scs[:1], map[string][]string{"place a valid order": {"UC-001.9"}}, ucs)
	if len(problems) != 1 || !strings.Contains(problems[0], "UC-001.9") {
		t.Fatalf("mapping to a missing criterion must be a gap: %v", problems)
	}

	dup := append(slices.Clone(ucs), Parse("UC-001b.md", "---\nid: UC-001\ntitle: Dup\n---\n- UC-001.1: again\n"))
	problems, _ = CheckStoryMerge(scs[:1], map[string][]string{"place a valid order": {"UC-001.1"}}, dup)
	if len(problems) == 0 {
		t.Fatal("validation errors in the merged use cases must fail the check")
	}

	if problems, _ := CheckStoryMerge(nil, nil, ucs); len(problems) == 0 {
		t.Fatal("an empty story must fail the check")
	}
}
