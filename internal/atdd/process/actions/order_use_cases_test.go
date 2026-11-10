package actions

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// dependencyDiagram renders a dependencies.md over UC-001..UC-006 with the
// given edges ("UC-A>UC-B": B needs data A creates).
func dependencyDiagram(edges ...string) string {
	var b strings.Builder
	b.WriteString("# Use case dependencies\n\n```mermaid\nflowchart LR\n")
	for _, n := range []string{"UC-001", "UC-002", "UC-003", "UC-004", "UC-005", "UC-006"} {
		b.WriteString("    " + n + "[\"Use case " + n + "\"]\n")
	}
	for _, e := range edges {
		from, to, _ := strings.Cut(e, ">")
		b.WriteString("    " + from + " -->|\"" + to + " needs data " + from + " creates\"| " + to + "\n")
	}
	b.WriteString("```\n")
	return b.String()
}

// orderRepo holds the dependency diagram; each id in done gets a tagged
// acceptance test and no use case file (its criteria deleted), so it is done.
// UC-001's file is the criteria-free useCaseWithoutESCC and UC-002's the one
// with a criterion: UC-001 is done once tagged, UC-002 never is.
func orderRepo(t *testing.T, diagram string, done ...string) string {
	t.Helper()
	root := useCaseRepo(t)
	writeRepoFile(t, root, "docs/use-cases/dependencies.md", diagram)
	for _, id := range done {
		writeRepoFile(t, root, "system-test/at/"+strings.ReplaceAll(id, "-", "")+"Test.java",
			taggedJavaTest(strings.ReplaceAll(id, "-", "")+"Test", map[string]string{"should" + strings.ReplaceAll(id, "-", ""): id + ".1"}))
	}
	return root
}

// Default graph: UC-003 needs UC-001 and UC-002, UC-004 needs UC-003, UC-005
// needs UC-006.
var defaultEdges = []string{"UC-002>UC-003", "UC-001>UC-003", "UC-003>UC-004", "UC-006>UC-005"}

func TestOrderPilotUseCases_ChosenAndTransitiveDependencies(t *testing.T) {
	a := loopActions(orderRepo(t, dependencyDiagram(defaultEdges...)))
	ctx := statemachine.NewContext()
	ctx.Set(CtxKeyProposedUseCase, "UC-004")
	if out := a.orderPilotUseCases(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	// UC-001 and UC-002 tie: the lower id first; then UC-003, then UC-004.
	if got := ctx.GetString(CtxKeyUseCaseOrder); got != "UC-001, UC-002, UC-003, UC-004" {
		t.Errorf("use-case-order = %q", got)
	}
	if got := ctx.GetString(CtxKeyPilotUseCases); got != "UC-001, UC-002, UC-003, UC-004" {
		t.Errorf("pilot-use-cases = %q", got)
	}
	// select-next-use-case's `use-cases` param takes the list as is.
	if got := splitUseCaseIDs(ctx.GetString(CtxKeyUseCaseOrder)); len(got) != 4 || got[0] != "UC-001" {
		t.Errorf("split = %v", got)
	}

	// A dependency with a higher id still goes first.
	ctx.Set(CtxKeyProposedUseCase, "UC-005")
	if out := a.orderUseCasesIn(ctx, OrderModePilot); out.Err != nil {
		t.Fatal(out.Err)
	}
	if got := ctx.GetString(CtxKeyUseCaseOrder); got != "UC-006, UC-005" {
		t.Errorf("use-case-order = %q", got)
	}
}

func TestOrderPilotUseCases_LeavesDoneOut(t *testing.T) {
	// UC-001 is done (tagged test, no criteria); UC-002 has a tagged test but
	// still has a criterion in its file, so it is not done.
	a := loopActions(orderRepo(t, dependencyDiagram(defaultEdges...), "UC-001", "UC-002"))
	ctx := statemachine.NewContext()
	ctx.Set(CtxKeyProposedUseCase, "UC-004")
	if out := a.orderPilotUseCases(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	if got := ctx.GetString(CtxKeyUseCaseOrder); got != "UC-002, UC-003, UC-004" {
		t.Errorf("use-case-order = %q", got)
	}
	if got := ctx.GetString(CtxKeyPilotUseCases); got != "UC-001, UC-002, UC-003, UC-004" {
		t.Errorf("pilot-use-cases = %q (done ones stay in the pilot set)", got)
	}
}

func TestOrderPilotUseCases_Errors(t *testing.T) {
	a := loopActions(orderRepo(t, dependencyDiagram(defaultEdges...)))
	if out := a.orderPilotUseCases(statemachine.NewContext()); out.Err == nil || !strings.Contains(out.Err.Error(), CtxKeyProposedUseCase) {
		t.Errorf("no proposed use case: err = %v", out.Err)
	}
	ctx := statemachine.NewContext()
	ctx.Set(CtxKeyProposedUseCase, "UC-042")
	if out := a.orderPilotUseCases(ctx); out.Err == nil || !strings.Contains(out.Err.Error(), "UC-042 is not in the dependency diagram") {
		t.Errorf("unknown proposed use case: err = %v", out.Err)
	}
	if out := a.orderUseCasesIn(ctx, "bulk"); out.Err == nil || !strings.Contains(out.Err.Error(), "unknown mode") {
		t.Errorf("unknown mode: err = %v", out.Err)
	}
	// The param-driven action reads order-mode.
	ctx.Set(CtxKeyProposedUseCase, "UC-003")
	ctx.Params["order-mode"] = OrderModePilot
	if out := a.orderUseCases(ctx); out.Err != nil || ctx.GetString(CtxKeyUseCaseOrder) != "UC-001, UC-002, UC-003" {
		t.Errorf("order-mode param: err = %v, order = %q", out.Err, ctx.GetString(CtxKeyUseCaseOrder))
	}
}

func TestOrderUseCases_DiagramErrors(t *testing.T) {
	ctx := func() *statemachine.Context {
		c := statemachine.NewContext()
		c.Set(CtxKeyProposedUseCase, "UC-001")
		return c
	}
	// A cycle anywhere in the diagram halts, naming it, even when the chosen
	// use case is not on it.
	a := loopActions(orderRepo(t, dependencyDiagram("UC-005>UC-006", "UC-006>UC-005")))
	if out := a.orderPilotUseCases(ctx()); out.Err == nil || !strings.Contains(out.Err.Error(), "cycle: UC-005 --> UC-006 --> UC-005") {
		t.Errorf("cycle: err = %v", out.Err)
	}
	// An edge naming an undeclared use case.
	a = loopActions(orderRepo(t, dependencyDiagram("UC-001>UC-099")))
	if out := a.orderPilotUseCases(ctx()); out.Err == nil || !strings.Contains(out.Err.Error(), "unknown use case UC-099") {
		t.Errorf("unknown edge id: err = %v", out.Err)
	}
	// A use case file the diagram does not declare.
	a = loopActions(orderRepo(t, "```mermaid\nflowchart LR\nUC-001[\"Browse\"]\n```\n"))
	if out := a.orderPilotUseCases(ctx()); out.Err == nil || !strings.Contains(out.Err.Error(), "UC-002") {
		t.Errorf("undeclared use case file: err = %v", out.Err)
	}
	// No diagram at all.
	a = loopActions(useCaseRepo(t))
	if out := a.orderPilotUseCases(ctx()); out.Err == nil || !strings.Contains(out.Err.Error(), "not found") {
		t.Errorf("missing diagram: err = %v", out.Err)
	}
}

func TestOrderScalingUseCases(t *testing.T) {
	// Pilot UC-001 (done); UC-002 is never done (it keeps its criterion), so a
	// pilot holding it has not finished.
	a := loopActions(orderRepo(t, dependencyDiagram(defaultEdges...), "UC-001"))
	ctx := statemachine.NewContext()
	ctx.Set(CtxKeyPilotUseCases, "UC-001")
	if out := a.orderScalingUseCases(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	// The rest: UC-002, UC-003 (its other dependency is done), UC-004, then
	// UC-006 ahead of UC-005, which needs it.
	if got := ctx.GetString(CtxKeyUseCaseOrder); got != "UC-002, UC-003, UC-004, UC-006, UC-005" {
		t.Errorf("use-case-order = %q", got)
	}

	ctx.Set(CtxKeyPilotUseCases, "UC-001, UC-002")
	if out := a.orderScalingUseCases(ctx); out.Err == nil || !strings.Contains(out.Err.Error(), "UC-002 are not done") {
		t.Errorf("unfinished pilot: err = %v", out.Err)
	}
	if out := a.orderScalingUseCases(statemachine.NewContext()); out.Err == nil || !strings.Contains(out.Err.Error(), CtxKeyPilotUseCases) {
		t.Errorf("no pilot: err = %v", out.Err)
	}
}

func TestReorderUseCases_NewDependencyDefersCurrent(t *testing.T) {
	// The scaling order was UC-003, UC-004, UC-005 with UC-001/UC-002 done.
	// UC-003's narrative added UC-006 --> UC-003, and UC-006 is not done:
	// UC-003 goes back behind it.
	root := orderRepo(t, dependencyDiagram(append(defaultEdges, "UC-006>UC-003")...), "UC-001")
	// UC-002 done too: drop its criteria file.
	writeRepoFile(t, root, "docs/use-cases/UC-002-place-order/acceptance-criteria.md", "---\nid: UC-002\ntitle: Place Order\n---\n\n## Narrative\n\nA customer places an order.\n")
	writeRepoFile(t, root, "system-test/at/UC002Test.java", taggedJavaTest("UC002Test", map[string]string{"shouldUC002": "UC-002.1"}))
	a := loopActions(root)

	list := "UC-006,UC-003,UC-004,UC-005"
	ctx := statemachine.NewContext()
	ctx.Set(CtxKeyUseCaseID, "UC-003")
	ctx.Set(CtxKeyUseCaseOrder, "UC-003, UC-004, UC-005")
	ctx.Set(CtxKeyUseCaseCursors, `{"characterize-use-case|`+list+`":2,"characterize-use-case|UC-003,UC-004,UC-005":1}`)
	if out := a.reorderUseCases(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	if v, _ := ctx.Get(CtxKeyUseCaseDeferred).(bool); !v {
		t.Error("use-case-deferred = false, want true")
	}
	if got := ctx.GetString(CtxKeyUseCaseOrder); got != "UC-006, UC-003, UC-004, UC-005" {
		t.Errorf("use-case-order = %q", got)
	}
	cursors := map[string]int{}
	if err := json.Unmarshal([]byte(ctx.GetString(CtxKeyUseCaseCursors)), &cursors); err != nil {
		t.Fatal(err)
	}
	if _, stale := cursors["characterize-use-case|"+list]; stale {
		t.Error("the new list's cursor must be reset")
	}
	if cursors["characterize-use-case|UC-003,UC-004,UC-005"] != 1 {
		t.Errorf("other cursors must stay: %v", cursors)
	}
}

func TestReorderUseCases_DependenciesDoneKeepsOrder(t *testing.T) {
	root := orderRepo(t, dependencyDiagram(defaultEdges...), "UC-001")
	a := loopActions(root)
	ctx := statemachine.NewContext()
	ctx.Set(CtxKeyUseCaseID, "UC-002")
	ctx.Set(CtxKeyUseCaseOrder, "UC-002, UC-003")
	if out := a.reorderUseCases(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	if v, ok := ctx.Get(CtxKeyUseCaseDeferred).(bool); !ok || v {
		t.Errorf("use-case-deferred = %v, want false", ctx.Get(CtxKeyUseCaseDeferred))
	}
	if got := ctx.GetString(CtxKeyUseCaseOrder); got != "UC-002, UC-003" {
		t.Errorf("use-case-order = %q, want unchanged", got)
	}

	if out := a.reorderUseCases(statemachine.NewContext()); out.Err == nil {
		t.Error("no current use case must error")
	}
	noOrder := statemachine.NewContext()
	noOrder.Set(CtxKeyUseCaseID, "UC-002")
	if out := a.reorderUseCases(noOrder); out.Err == nil || !strings.Contains(out.Err.Error(), CtxKeyUseCaseOrder) {
		t.Errorf("no order: err = %v", out.Err)
	}
}
