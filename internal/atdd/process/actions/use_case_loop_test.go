package actions

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

const useCaseWithESCC = `---
id: UC-002
title: Place Order
---

# Place Order

## Narrative

A customer places an order.

## Business Rules

- The total includes tax.

## Acceptance Criteria

- UC-002.1: An order is placed.

## External System Contract Criteria

External System: ERP
  Shared (stub + real):
    Given products Apple (1.00)
    Then ERP has products Apple (1.00)
`

const useCaseWithoutESCC = `---
id: UC-001
title: Browse Products
---

## Narrative

A customer browses products.
`

// useCaseRepo is a git repo holding the two use case criteria files.
func useCaseRepo(t *testing.T) string {
	t.Helper()
	return gitRepo(t, map[string]string{
		"docs/use-cases/UC-001-browse-products/acceptance-criteria.md": useCaseWithoutESCC,
		"docs/use-cases/UC-002-place-order/acceptance-criteria.md":     useCaseWithESCC,
	})
}

// loopConfig is characterizeConfig with the driver port layers the
// port-changed derivation reads.
func loopConfig() *projectconfig.Config {
	cfg := characterizeConfig()
	cfg.SystemTest.Paths["system-driver-port"] = "system-test/driver/port/system"
	cfg.SystemTest.Paths["external-system-driver-port"] = "system-test/driver/port/external"
	return cfg
}

func loopActions(root string) actions {
	return newActions(Deps{Config: loopConfig(), RepoPath: root, Stdout: &bytes.Buffer{}})
}

// layerCtx is a characterize-layer call scope whose system-test start commit
// is the repo's current HEAD (record-start-commits' value).
func layerCtx(t *testing.T, root, useCases, layer string) *statemachine.Context {
	t.Helper()
	ctx := statemachine.NewContext()
	ctx.Set(CtxKeySystemTestStartCommit, strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD")))
	ctx.Params["use-cases"] = useCases
	ctx.Params["layer-process"] = layer
	return ctx
}

// taggedJavaTest is an acceptance test class with one test per name, each
// tagged with its criterion id.
func taggedJavaTest(class string, tests map[string]string) string {
	var b strings.Builder
	b.WriteString("class " + class + " {\n")
	for name, tag := range tests {
		b.WriteString("    @Test\n    @Tag(\"" + tag + "\")\n    void " + name + "() {\n        scenario\n                .when().placeOrder()\n                .then().shouldSucceed();\n    }\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func TestSelectNextUseCase_PopsInOrderThenNoneLeft(t *testing.T) {
	root := useCaseRepo(t)
	a := loopActions(root)
	ctx := layerCtx(t, root, "UC-002, UC-001, UC-003", "characterize-at-layer")

	for _, want := range []string{"UC-002", "UC-001", "UC-003"} {
		if out := a.selectNextUseCase(ctx); out.Err != nil {
			t.Fatal(out.Err)
		}
		if ctx.Get(CtxKeyUseCaseLeft) != true || ctx.GetString(CtxKeyUseCaseID) != want {
			t.Fatalf("got left=%v id=%q, want %s", ctx.Get(CtxKeyUseCaseLeft), ctx.GetString(CtxKeyUseCaseID), want)
		}
	}
	if out := a.selectNextUseCase(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	if ctx.Get(CtxKeyUseCaseLeft) != false {
		t.Fatal("want use-case-left=false after the last use case")
	}
	if ctx.GetString(CtxKeyUseCaseID) != "UC-003" {
		t.Errorf("the last use case must stay current for a re-run, got %q", ctx.GetString(CtxKeyUseCaseID))
	}
}

func TestSelectNextUseCase_CursorPerCallSite(t *testing.T) {
	root := useCaseRepo(t)
	a := loopActions(root)
	ctx := layerCtx(t, root, "UC-001", "characterize-at-layer")
	a.selectNextUseCase(ctx)
	a.selectNextUseCase(ctx)
	if ctx.Get(CtxKeyUseCaseLeft) != false {
		t.Fatal("AT layer core pass should be exhausted")
	}
	// The next layer (same use case list) starts from the beginning.
	ctx.Params["layer-process"] = "characterize-dsl-layer"
	if out := a.selectNextUseCase(ctx); out.Err != nil || ctx.Get(CtxKeyUseCaseLeft) != true || ctx.GetString(CtxKeyUseCaseID) != "UC-001" {
		t.Fatalf("DSL layer: err=%v left=%v id=%q", out.Err, ctx.Get(CtxKeyUseCaseLeft), ctx.GetString(CtxKeyUseCaseID))
	}
}

func TestSelectNextUseCase_StampsESCC(t *testing.T) {
	root := useCaseRepo(t)
	a := loopActions(root)
	ctx := layerCtx(t, root, "UC-002,UC-001", "characterize-adapter-layer")

	if out := a.selectNextUseCase(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	if ctx.Get("ticket-has-escc") != true {
		t.Fatalf("ticket-has-escc = %v, want true", ctx.Get("ticket-has-escc"))
	}
	if systems, _ := ctx.Get("escc-systems").([]string); len(systems) != 1 || systems[0] != "ERP" {
		t.Errorf("escc-systems = %v, want [ERP]", ctx.Get("escc-systems"))
	}
	if body := ctx.GetString("external-system-contract-criteria"); body == "" || body[:len("External System: ERP")] != "External System: ERP" {
		t.Errorf("external-system-contract-criteria = %q", body)
	}

	if out := a.selectNextUseCase(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	if ctx.Get("ticket-has-escc") != false || ctx.GetString("external-system-contract-criteria") != "" || ctx.Get("escc-systems") != nil && len(ctx.Get("escc-systems").([]string)) != 0 {
		t.Fatalf("UC-001 has no ESCC; stale keys: has=%v systems=%v", ctx.Get("ticket-has-escc"), ctx.Get("escc-systems"))
	}
}

func TestSelectNextUseCase_NoCriteriaFileYetHasNoESCC(t *testing.T) {
	root := useCaseRepo(t)
	a := loopActions(root)
	ctx := layerCtx(t, root, "UC-009", "write-use-case-criteria")
	if out := a.selectNextUseCase(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	if ctx.GetString(CtxKeyUseCaseID) != "UC-009" || ctx.Get("ticket-has-escc") != false {
		t.Fatalf("id=%q has-escc=%v", ctx.GetString(CtxKeyUseCaseID), ctx.Get("ticket-has-escc"))
	}
}

func TestSelectNextUseCase_SameUseCaseRestamps(t *testing.T) {
	root := useCaseRepo(t)
	a := loopActions(root)
	ctx := layerCtx(t, root, "UC-001,UC-002", "characterize-dsl-layer")
	if out := a.selectNextUseCase(ctx); out.Err != nil || ctx.Get("ticket-has-escc") != false {
		t.Fatalf("err=%v has-escc=%v", out.Err, ctx.Get("ticket-has-escc"))
	}
	// Late discovery: the criteria writer adds an ESCC section to UC-001.
	p := filepath.Join(root, "docs", "use-cases", "UC-001-browse-products", "acceptance-criteria.md")
	escc := "\n## External System Contract Criteria\n\nExternal System: Clock\n  Shared (stub + real):\n    Given the time is 10:00\n    Then Clock returns 10:00\n"
	if err := os.WriteFile(p, []byte(useCaseWithoutESCC+escc), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, restamp := range []func(*statemachine.Context) statemachine.Outcome{
		func(c *statemachine.Context) statemachine.Outcome {
			c.Params["same-use-case"] = "true"
			defer delete(c.Params, "same-use-case")
			return a.selectNextUseCase(c)
		},
		a.restampUseCase,
	} {
		if out := restamp(ctx); out.Err != nil {
			t.Fatal(out.Err)
		}
		if ctx.GetString(CtxKeyUseCaseID) != "UC-001" || ctx.Get("ticket-has-escc") != true || ctx.Get(CtxKeyUseCaseLeft) != true {
			t.Fatalf("restamp: id=%q has-escc=%v left=%v", ctx.GetString(CtxKeyUseCaseID), ctx.Get("ticket-has-escc"), ctx.Get(CtxKeyUseCaseLeft))
		}
	}
	// The cursor did not move: the next pop is UC-002.
	if out := a.selectNextUseCase(ctx); out.Err != nil || ctx.GetString(CtxKeyUseCaseID) != "UC-002" {
		t.Fatalf("err=%v id=%q, want UC-002", out.Err, ctx.GetString(CtxKeyUseCaseID))
	}
}

func TestSelectNextUseCase_Errors(t *testing.T) {
	root := useCaseRepo(t)
	a := loopActions(root)
	if out := a.restampUseCase(statemachine.NewContext()); out.Err == nil {
		t.Error("restamp without a current use case must error")
	}
	noStart := statemachine.NewContext()
	noStart.Params["use-cases"] = "UC-001"
	if out := a.selectNextUseCase(noStart); out.Err == nil || !strings.Contains(out.Err.Error(), CtxKeySystemTestStartCommit) {
		t.Errorf("select without a system-test start commit must error, got %v", out.Err)
	}
	if out := newActions(Deps{RepoPath: root, Stdout: &bytes.Buffer{}}).selectNextUseCase(layerCtx(t, root, "UC-001", "x")); out.Err == nil {
		t.Error("select without a config must error")
	}
}

// Gap 4: a system with only the core use case has an empty other-use-cases;
// the bulk passes skip, they do not fail.
func TestSelectNextUseCase_EmptyListHasNoneLeft(t *testing.T) {
	root := useCaseRepo(t)
	a := loopActions(root)
	for _, list := range []string{"", "[]", " , "} {
		ctx := layerCtx(t, root, list, "characterize-at-layer")
		ctx.Set(CtxKeyUseCaseID, "UC-001")
		if out := a.selectNextUseCase(ctx); out.Err != nil {
			t.Fatalf("use-cases %q: %v", list, out.Err)
		}
		if ctx.Get(CtxKeyUseCaseLeft) != false || ctx.GetString(CtxKeyUseCaseID) != "UC-001" {
			t.Fatalf("use-cases %q: left=%v id=%q, want false and the State untouched", list, ctx.Get(CtxKeyUseCaseLeft), ctx.GetString(CtxKeyUseCaseID))
		}
	}
}

// Gap 1: in the bulk layers each use case gets its own test-names and
// port-changed flags, derived from its tagged tests and its layer commits
// (directory-keyed, since the start commit), never the previous use case's.
func TestSelectNextUseCase_RederivesTestsAndPortChanges(t *testing.T) {
	root := useCaseRepo(t)
	// Before the start commit: an earlier run's UC-002 DSL-port change, which
	// must not count.
	writeRepoFile(t, root, "system-test/dsl/port/Old.java", "interface Old {}\n")
	commitAll(t, root, "#1 old - ACCEPTANCE TESTS (UC-002)")
	a := loopActions(root)
	ctx := layerCtx(t, root, "UC-001,UC-002", "characterize-dsl-layer")

	writeRepoFile(t, root, "system-test/at/BrowseTest.java", taggedJavaTest("BrowseTest", map[string]string{"shouldListProducts": "UC-001.1"}))
	writeRepoFile(t, root, "system-test/dsl/port/BrowseDsl.java", "interface BrowseDsl {}\n")
	commitAll(t, root, "#1 t - ACCEPTANCE TESTS (UC-001)")
	writeRepoFile(t, root, "system-test/at/OrderTest.java", taggedJavaTest("OrderTest", map[string]string{"shouldPlaceOrder": "UC-002.1"}))
	commitAll(t, root, "#1 t - ACCEPTANCE TESTS (UC-002)")
	writeRepoFile(t, root, "system-test/dsl/core/BrowseImpl.java", "class BrowseImpl {}\n")
	commitAll(t, root, "#1 t - DSL (UC-001)")
	writeRepoFile(t, root, "system-test/driver/port/system/dto/OrderRequest.java", "record OrderRequest() {}\n")
	writeRepoFile(t, root, "system-test/driver/port/external/erp/ErpDriver.java", "interface ErpDriver {}\n")
	commitAll(t, root, "#1 t - DSL (UC-002)")

	if out := a.selectNextUseCase(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	if names, _ := ctx.Get("test-names").([]string); len(names) != 1 || names[0] != "shouldListProducts" {
		t.Errorf("UC-001 test-names = %v", ctx.Get("test-names"))
	}
	if ctx.Get("dsl-port-changed") != true || ctx.Get("system-driver-port-changed") != false || ctx.Get("external-driver-port-changed") != false {
		t.Errorf("UC-001 flags dsl=%v sys=%v ext=%v, want true/false/false", ctx.Get("dsl-port-changed"), ctx.Get("system-driver-port-changed"), ctx.Get("external-driver-port-changed"))
	}
	if _, ok := ctx.State["external-driver-port-changed-paths"]; ok {
		t.Errorf("UC-001 has no external port change; paths = %q", ctx.GetString("external-driver-port-changed-paths"))
	}

	if out := a.selectNextUseCase(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	if names, _ := ctx.Get("test-names").([]string); len(names) != 1 || names[0] != "shouldPlaceOrder" {
		t.Errorf("UC-002 test-names = %v", ctx.Get("test-names"))
	}
	// A DTO-only system driver port change counts (directory-keyed).
	if ctx.Get("dsl-port-changed") != false || ctx.Get("system-driver-port-changed") != true || ctx.Get("external-driver-port-changed") != true {
		t.Errorf("UC-002 flags dsl=%v sys=%v ext=%v, want false/true/true", ctx.Get("dsl-port-changed"), ctx.Get("system-driver-port-changed"), ctx.Get("external-driver-port-changed"))
	}
	if got := ctx.GetString("external-driver-port-changed-paths"); got != "system-test/driver/port/external/erp/ErpDriver.java" {
		t.Errorf("UC-002 external-driver-port-changed-paths = %q", got)
	}
}

// Before a use case's acceptance test layer it has no tagged tests and no
// port change: test-names is unset (an empty list would run the whole suite)
// and the flags are false, whatever the previous use case left.
func TestSelectNextUseCase_FirstPassClearsPreviousUseCase(t *testing.T) {
	root := useCaseRepo(t)
	a := loopActions(root)
	ctx := layerCtx(t, root, "UC-001", "characterize-at-layer")
	ctx.Set("test-names", []string{"previousTest"})
	ctx.Set("dsl-port-changed", true)
	ctx.Set("system-driver-port-changed", true)
	ctx.Set("external-driver-port-changed-paths", "system-test/driver/port/external/clock/Clock.java")
	if out := a.selectNextUseCase(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	if _, ok := ctx.State["test-names"]; ok {
		t.Errorf("test-names = %v, want unset", ctx.Get("test-names"))
	}
	if _, ok := ctx.State["external-driver-port-changed-paths"]; ok {
		t.Error("external-driver-port-changed-paths must be unset")
	}
	if ctx.Get("dsl-port-changed") != false || ctx.Get("system-driver-port-changed") != false {
		t.Errorf("flags dsl=%v sys=%v, want false", ctx.Get("dsl-port-changed"), ctx.Get("system-driver-port-changed"))
	}
}

// A tagged test file the grammar rejects is an error for its use case, never
// a silently missing test; a rejected test in an unrelated file is ignored.
func TestSelectNextUseCase_UnparsableTaggedTestIsError(t *testing.T) {
	root := useCaseRepo(t)
	broken := "class Broken {\n    @Test\n    @Tag(\"UC-001.1\")\n    void broken() {\n        scenario.when(.then();\n    }\n}\n"
	writeRepoFile(t, root, "system-test/at/Unrelated.java", strings.ReplaceAll(broken, "UC-001.1", "UC-009.1"))
	a := loopActions(root)
	if out := a.selectNextUseCase(layerCtx(t, root, "UC-001", "characterize-dsl-layer")); out.Err != nil {
		t.Fatalf("an unrelated unparsable test must not fail UC-001: %v", out.Err)
	}
	writeRepoFile(t, root, "system-test/at/Broken.java", broken)
	if out := a.selectNextUseCase(layerCtx(t, root, "UC-001", "characterize-dsl-layer")); out.Err == nil {
		t.Fatal("an unparsable test tagged with UC-001 must fail")
	}
}

// Gap 2: the acceptance test layer gets the use case's criteria as
// acceptance-criteria and a traceability line per criterion.
func TestSelectNextUseCase_StampsCriteria(t *testing.T) {
	root := useCaseRepo(t)
	flat := "---\nid: UC-003\ntitle: Cancel Order\n---\n\n# UC-003 Cancel Order\n\n## Business rules\n- BR-1. Only open orders.\n\n## Acceptance criteria\n\n```gherkin\nFeature: Cancel Order\n\n  @UC-003.1 @narrative-1\n  Scenario: Cancel an open order\n    When the order is cancelled\n    Then it is cancelled\n```\n\n## External System Contract Criteria\nExternal System: ERP\n  Shared (stub + real):\n    Given an order\n    Then ERP has the order\n"
	writeRepoFile(t, root, "docs/use-cases/UC-003-cancel-order.md", flat)
	a := loopActions(root)
	ctx := layerCtx(t, root, "UC-003,UC-002,UC-009", "characterize-at-layer")

	if out := a.selectNextUseCase(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	ac := ctx.GetString("acceptance-criteria")
	if !strings.HasPrefix(ac, "```gherkin") || !strings.Contains(ac, "Scenario: Cancel an open order") || strings.Contains(ac, "BR-1") || strings.Contains(ac, "External System") {
		t.Errorf("flat layout acceptance-criteria = %q", ac)
	}
	if got := ctx.GetString(CtxKeyScenarioCriteria); got != "Cancel an open order => UC-003.1" {
		t.Errorf("scenario-criteria = %q", got)
	}

	if out := a.selectNextUseCase(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	if got := ctx.GetString("acceptance-criteria"); got != "- UC-002.1: An order is placed." {
		t.Errorf("UC-002 acceptance-criteria = %q", got)
	}
	if got := ctx.GetString(CtxKeyScenarioCriteria); got != "An order is placed. => UC-002.1" {
		t.Errorf("UC-002 scenario-criteria = %q", got)
	}

	// No criteria file yet: both keys unset, not the previous use case's.
	if out := a.selectNextUseCase(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	for _, k := range []string{"acceptance-criteria", CtxKeyScenarioCriteria} {
		if _, ok := ctx.State[k]; ok {
			t.Errorf("%s = %q, want unset", k, ctx.GetString(k))
		}
	}
}

// The folder layout's criteria file has no "Acceptance criteria" heading:
// the whole file is the criteria, without front matter and ESCC.
func TestStampUseCaseCriteria_FolderLayout(t *testing.T) {
	ctx := statemachine.NewContext()
	body := "---\nid: UC-004\ntitle: Pay\n---\n\n```gherkin\nFeature: Pay\n\n  @UC-004.1\n  Scenario: Pay an order\n    Then it is paid\n```\n\n## External System Contract Criteria\nExternal System: Bank\n"
	stampUseCaseCriteria(ctx, "UC-004-pay/acceptance-criteria.md", body)
	ac := ctx.GetString("acceptance-criteria")
	if !strings.HasPrefix(ac, "```gherkin") || strings.Contains(ac, "id: UC-004") || strings.Contains(ac, "Bank") {
		t.Errorf("acceptance-criteria = %q", ac)
	}
	if got := ctx.GetString(CtxKeyScenarioCriteria); got != "Pay an order => UC-004.1" {
		t.Errorf("scenario-criteria = %q", got)
	}
}

// Gap 3: the final remove-wip-gate gets every characterized use case's
// tagged tests, not the last use case's test-names.
func TestStampCharacterizedTestNames(t *testing.T) {
	root := useCaseRepo(t)
	writeRepoFile(t, root, "system-test/at/BrowseTest.java", taggedJavaTest("BrowseTest", map[string]string{"shouldListProducts": "UC-001.1"}))
	writeRepoFile(t, root, "system-test/at/OrderTest.java", taggedJavaTest("OrderTest", map[string]string{"shouldPlaceOrder": "UC-002.1"}))
	writeRepoFile(t, root, "system-test/at/OtherTest.java", taggedJavaTest("OtherTest", map[string]string{"shouldDoOther": "UC-007.1"}))
	a := loopActions(root)

	ctx := statemachine.NewContext()
	ctx.Set("proposed-use-case", "UC-001")
	ctx.Set("other-use-cases", []string{"UC-002"})
	ctx.Set("test-names", []string{"shouldPlaceOrder"})
	if out := a.stampCharacterizedTestNames(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	if names, _ := ctx.Get("test-names").([]string); strings.Join(names, ",") != "shouldListProducts,shouldPlaceOrder" {
		t.Errorf("test-names = %v, want both use cases' tests and not UC-007's", ctx.Get("test-names"))
	}

	// Only the core use case (other-use-cases empty).
	ctx.Set("other-use-cases", []string{})
	if out := a.stampCharacterizedTestNames(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	if names, _ := ctx.Get("test-names").([]string); strings.Join(names, ",") != "shouldListProducts" {
		t.Errorf("core only: test-names = %v", ctx.Get("test-names"))
	}

	none := statemachine.NewContext()
	none.Set("proposed-use-case", "UC-005")
	if out := a.stampCharacterizedTestNames(none); out.Err == nil {
		t.Error("no tagged test must error")
	}
	if out := a.stampCharacterizedTestNames(statemachine.NewContext()); out.Err == nil {
		t.Error("missing proposed-use-case must error")
	}
}
