package generate

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/spec"
)

var update = flag.Bool("update", false, "rewrite golden files")

func golden(t *testing.T, path, got string) {
	t.Helper()
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create)", err)
	}
	if strings.ReplaceAll(string(want), "\r\n", "\n") != got {
		t.Fatalf("%s differs from golden (run with -update to refresh)\n--- got ---\n%s", path, got)
	}
}

func loadSpecs(t *testing.T) []spec.UseCase {
	t.Helper()
	ucs, err := spec.LoadDir(filepath.Join("testdata", "specs"))
	if err != nil {
		t.Fatal(err)
	}
	return ucs
}

// goldenInputs are real shop acceptance tests (see each file's provenance
// header) and the criterion IDs each one's output must carry.
var goldenInputs = map[string][]string{
	"java/PlaceOrderPositiveTest.java":                      {"UC-001.1", "UC-001.2", "UC-001.3", "UC-001.4", "UC-001.5", "UC-005.4"},
	"java/PlaceOrderNegativeTest.java":                      {"UC-001.8", "UC-001.9", "UC-001.10", "UC-001.11", "UC-001.12"},
	"java/PlaceOrderPositiveIsolatedTest.java":              {"UC-001.6", "UC-001.7"},
	"java/GiftWrapOrderPositiveTest.java":                   {},
	"dotnet/PlaceOrderPositiveTest.cs":                      {"UC-001.1", "UC-001.2", "UC-001.3", "UC-001.4", "UC-001.5", "UC-005.4"},
	"dotnet/PlaceOrderNegativeTest.cs":                      {"UC-001.8", "UC-001.9", "UC-001.10", "UC-001.11", "UC-001.12"},
	"dotnet/PlaceOrderPositiveIsolatedTest.cs":              {"UC-001.6", "UC-001.7"},
	"typescript/place-order-positive-test.spec.ts":          {"UC-001.1", "UC-001.2", "UC-001.3", "UC-001.4", "UC-001.5", "UC-005.4"},
	"typescript/place-order-negative-test.spec.ts":          {"UC-001.8", "UC-001.9", "UC-001.10", "UC-001.11", "UC-001.12"},
	"typescript/place-order-positive-isolated-test.spec.ts": {"UC-001.6", "UC-001.7"},
	"typescript/cancel-order-negative-test.spec.ts":         {"UC-003.6", "UC-003.7"},
}

func generateFile(t *testing.T, rel string) Result {
	t.Helper()
	res, err := Generate(Options{TestDirs: []string{"testdata/" + rel}, UseCases: loadSpecs(t)})
	if err != nil {
		t.Fatalf("Generate(%s): %v", rel, err)
	}
	return res
}

func TestGolden(t *testing.T) {
	for rel := range goldenInputs {
		t.Run(rel, func(t *testing.T) {
			res := generateFile(t, rel)
			golden(t, filepath.Join("testdata", rel+".feature.golden"), res.Gherkin)
		})
	}
}

// TestRoundTrip feeds each generated document to the spec gate's Gherkin
// parser: it must parse without errors and yield exactly the expected
// criterion IDs (docs/spec-gate/convention.md §3).
func TestRoundTrip(t *testing.T) {
	for rel, want := range goldenInputs {
		t.Run(rel, func(t *testing.T) {
			res := generateFile(t, rel)
			md := "---\nid: UC-001\ntitle: Round trip\n---\n\n```gherkin\n" + res.Gherkin + "```\n"
			uc := spec.Parse("round-trip.md", md)
			if uc.FrontErr != "" || len(uc.ParseErrs) > 0 {
				t.Fatalf("parse errors: %q %v", uc.FrontErr, uc.ParseErrs)
			}
			var got []string
			for _, c := range uc.Criteria {
				got = append(got, c.ID)
			}
			sort.Strings(got)
			w := slices.Clone(want)
			sort.Strings(w)
			if !slices.Equal(got, w) {
				t.Fatalf("criterion IDs = %v, want %v", got, w)
			}
		})
	}
}

func TestUntaggedAreReported(t *testing.T) {
	res := generateFile(t, "java/GiftWrapOrderPositiveTest.java")
	if len(res.Untagged) != 2 {
		t.Fatalf("untagged = %v, want 2", res.Untagged)
	}
	if !strings.Contains(res.Gherkin, "Feature: "+UntaggedFeature) || !strings.Contains(res.Gherkin, "@wip") {
		t.Fatalf("untagged wip tests not rendered:\n%s", res.Gherkin)
	}
}

func TestUseCaseFilter(t *testing.T) {
	res, err := Generate(Options{TestDirs: []string{"testdata/java"}, UseCases: loadSpecs(t), Only: []string{"UC-005"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Gherkin, "UC-001") || !strings.Contains(res.Gherkin, "@UC-005.4") || strings.Contains(res.Gherkin, UntaggedFeature) {
		t.Fatalf("filter not applied:\n%s", res.Gherkin)
	}
}

const javaHeader = "class T {\n    @Tag(\"UC-001.1\")\n    @TestTemplate\n    void broken() {\n"

// TestFailLoud checks that each unsupported shape fails with file:line and
// the test name instead of being skipped.
func TestFailLoud(t *testing.T) {
	cases := map[string]struct{ file, src, reason string }{
		"lambda argument":    {"T.java", javaHeader + "scenario.when().placeOrder().withSku(() -> \"x\").then().shouldSucceed();\n}}", "lambda"},
		"new argument":       {"T.java", javaHeader + "scenario.when().placeOrder().withSku(new Sku(\"x\")).then().shouldSucceed();\n}}", "`new`"},
		"second statement":   {"T.java", javaHeader + "scenario.when().placeOrder().then().shouldSucceed();\nfoo.bar();\n}}", "second statement"},
		"no then":            {"T.java", javaHeader + "scenario.when().placeOrder();\n}}", "no .then()"},
		"unknown keyword":    {"T.java", javaHeader + "scenario.expect().placeOrder().then().shouldSucceed();\n}}", "unknown section keyword"},
		"unbalanced parens":  {"T.java", javaHeader + "scenario.when().placeOrder(.then().shouldSucceed();\n}}", "unbalanced"},
		"csharp lambda":      {"T.cs", "class T {\n[Fact]\n[Trait(\"tag\", \"UC-001.1\")]\npublic async Task Broken() {\nawait Scenario().When().PlaceOrder().WithSku(x => x).Then().ShouldSucceed();\n}}", "lambda"},
		"typescript no then": {"t.spec.ts", "test('broken', criteria('UC-001.1'), async ({ scenario }) => {\nawait scenario.when().placeOrder();\n});", "no .then()"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, errs, _ := ParseFile(c.file, []byte(c.src))
			if len(errs) != 1 {
				t.Fatalf("errors = %v, want 1", errs)
			}
			msg := errs[0].Error()
			if !strings.HasPrefix(msg, c.file+":") || !strings.Contains(msg, "roken") || !strings.Contains(msg, c.reason) {
				t.Fatalf("error %q lacks file:line, test name or %q", msg, c.reason)
			}
		})
	}
}

func TestUnknownUseCaseFails(t *testing.T) {
	dir := t.TempDir()
	src := "class T {\n    @Tag(\"UC-099.1\")\n    @Test\n    void orphan() {\n        scenario.when().placeOrder().then().shouldSucceed();\n    }\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "T.java"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Generate(Options{TestDirs: []string{dir}, UseCases: loadSpecs(t)})
	if err == nil || !strings.Contains(err.Error(), "T.java:4 orphan") || !strings.Contains(err.Error(), "UC-099") {
		t.Fatalf("err = %v, want unknown use case UC-099 at T.java:4 orphan", err)
	}
}

func TestAllOrNothing(t *testing.T) {
	dir := t.TempDir()
	good := "class A {\n @Tag(\"UC-001.1\")\n @Test\n void good() { scenario.when().placeOrder().then().shouldSucceed(); }\n}\n"
	bad := "class B {\n @Tag(\"UC-001.1\")\n @Test\n void bad() { scenario.when().placeOrder(); }\n}\n"
	for n, s := range map[string]string{"A.java": good, "B.java": bad} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	res, err := Generate(Options{TestDirs: []string{dir}, UseCases: loadSpecs(t)})
	if err == nil || res.Gherkin != "" {
		t.Fatalf("want an error and no output, got err=%v output=%q", err, res.Gherkin)
	}
}

func TestHumanize(t *testing.T) {
	for in, want := range map[string]string{
		"shouldRejectOrderWithZeroQuantity":              "should reject order with zero quantity",
		"orderPrefixShouldBeORD":                         "order prefix should be ORD",
		"cannotCancelAnOrderOn31stDecBetween2200And2230": "cannot cancel an order on 31st dec between 2200 and 2230",
		"ShouldBeAbleToPlaceOrder":                       "should be able to place order",
		"withSKUCode":                                    "with SKU code",
	} {
		if got := humanize(in); got != want {
			t.Errorf("humanize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStripTitleParams(t *testing.T) {
	for in, want := range map[string]string{
		"shouldPlaceOrder_unitPrice=$unitPrice_quantity=$quantity": "shouldPlaceOrder",
		`shouldRejectOrderWithEmptySku_"$sku"`:                     "shouldRejectOrderWithEmptySku",
		`shouldRejectCouponWithBlankCode_"${code}"`:                "shouldRejectCouponWithBlankCode",
		"plainTitle": "plainTitle",
	} {
		if got := stripTitleParams(in); got != want {
			t.Errorf("stripTitleParams(%q) = %q, want %q", in, got, want)
		}
	}
}
