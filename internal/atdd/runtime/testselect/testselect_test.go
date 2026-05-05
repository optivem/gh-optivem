package testselect

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// makeDeps wires fakeGit / fakeRead / fakeWalk over a virtual filesystem.
// The repoRoot string is opaque — paths in `files` are taken at face value.
func makeDeps(diff string, files map[string]string) *Deps {
	return &Deps{
		Git: func(ctx context.Context, repoRoot string, args ...string) ([]byte, error) {
			return []byte(diff), nil
		},
		Read: func(repoRoot, path string) ([]byte, error) {
			// Indexers pass empty repoRoot + absolute path, diff parser
			// passes the actual repoRoot + repo-relative path. Try both.
			if body, ok := files[path]; ok {
				return []byte(body), nil
			}
			// Strip the repoRoot prefix if present.
			rel := strings.TrimPrefix(path, repoRoot)
			rel = strings.TrimPrefix(rel, "/")
			rel = strings.TrimPrefix(rel, "\\")
			if body, ok := files[rel]; ok {
				return []byte(body), nil
			}
			return nil, &fileNotFoundError{path}
		},
		Walk: func(repoRoot string, roots []string, exts []string) ([]string, error) {
			var out []string
			extSet := map[string]bool{}
			for _, e := range exts {
				extSet[e] = true
			}
			normRoot := strings.ReplaceAll(repoRoot, "\\", "/")
			for path := range files {
				if !extSet[extOf(path)] {
					continue
				}
				for _, r := range roots {
					rel := strings.ReplaceAll(r, "\\", "/")
					rel = strings.TrimPrefix(rel, normRoot)
					rel = strings.TrimPrefix(rel, "/")
					if rel == "" || strings.HasPrefix(path, rel) {
						out = append(out, path)
						break
					}
				}
			}
			sort.Strings(out)
			return out, nil
		},
	}
}

type fileNotFoundError struct{ path string }

func (e *fileNotFoundError) Error() string { return "not found: " + e.path }

func extOf(p string) string {
	if i := strings.LastIndex(p, "."); i >= 0 {
		return p[i:]
	}
	return ""
}

// ----------------------------------------------------------------------------
// Diff helpers — produce minimal unified=0 diff blocks for the tests.
// ----------------------------------------------------------------------------

func diffBlock(path string, hunks ...string) string {
	var b strings.Builder
	b.WriteString("diff --git a/" + path + " b/" + path + "\n")
	b.WriteString("--- a/" + path + "\n")
	b.WriteString("+++ b/" + path + "\n")
	for _, h := range hunks {
		b.WriteString(h + "\n")
	}
	return b.String()
}

// ----------------------------------------------------------------------------
// Java fixtures — adapter / port / dsl / test files
// ----------------------------------------------------------------------------

const javaAdapter = `package com.mycompany.myshop.testkit.driver.adapter.myshop.api;
public class CustomerApiAdapter implements RegisterCustomerPort {
  @Override
  public void register(String email) {
    // implementation
  }
}
`

const javaPort = `package com.mycompany.myshop.testkit.driver.port.myshop;
public interface RegisterCustomerPort {
  void register(String email);
}
`

const javaDSL = `package com.mycompany.myshop.testkit.dsl.core.usecase.myshop;
public class RegisterCustomerDsl {
  private final RegisterCustomerPort port;
  public RegisterCustomerDsl(RegisterCustomerPort port) {
    this.port = port;
  }
  public void registerCustomer(String email) {
    port.register(email);
  }
  public void registerCustomerWithDefaults() {
    registerCustomer("default@example.com");
  }
}
`

const javaTest = `package com.mycompany.myshop.systemtest.latest.acceptance;
public class RegisterCustomerPositiveTest {
  @TestTemplate
  @Channel({ChannelType.API})
  public void shouldRegister() {
    dsl.registerCustomer("a@example.com");
  }
}
`

const javaTestNeg = `package com.mycompany.myshop.systemtest.latest.acceptance;
public class RegisterCustomerNegativeTest {
  @TestTemplate
  @Channel({ChannelType.API})
  public void shouldRejectMissingEmail() {
    dsl.registerCustomer("");
  }
}
`

const javaTestUntagged = `package com.mycompany.myshop.systemtest.latest.acceptance;
public class RegisterCustomerLooseTest {
  @TestTemplate
  public void shouldRunSomehow() {
    dsl.registerCustomer("x@example.com");
  }
}
`

const javaTestBoth = `package com.mycompany.myshop.systemtest.latest.acceptance;
public class RegisterCustomerBothTest {
  @TestTemplate
  @Channel({ChannelType.UI, ChannelType.API})
  public void shouldRunOnBothChannels() {
    dsl.registerCustomer("b@example.com");
  }
}
`

const javaAdapterExternal = `package com.mycompany.myshop.testkit.driver.adapter.external.erp;
public class ErpAdapter implements ErpPort {
  @Override
  public void postInvoice(String id) {
    // impl
  }
}
`

const javaPortExternal = `package com.mycompany.myshop.testkit.driver.port.external.erp;
public interface ErpPort {
  void postInvoice(String id);
}
`

const javaDSLExternal = `package com.mycompany.myshop.testkit.dsl.core.usecase.external;
public class ErpDsl {
  private final ErpPort port;
  public void postInvoiceFromOrder(String id) {
    port.postInvoice(id);
  }
}
`

const javaTestContract = `package com.mycompany.myshop.systemtest.latest.contract;
public class ErpContractTest {
  @TestTemplate
  public void shouldPostInvoice() {
    dsl.postInvoiceFromOrder("o-1");
  }
}
`

// Adapter with two methods changed; both share one DSL caller and two
// tests — ensures dedup.
const javaAdapterTwoMethods = `package com.mycompany.myshop.testkit.driver.adapter.myshop.api;
public class OrderApiAdapter implements PlaceOrderPort {
  public void placeOrder(String id) { }
  public void cancelOrder(String id) { }
}
`

const javaPortTwoMethods = `package com.mycompany.myshop.testkit.driver.port.myshop;
public interface PlaceOrderPort {
  void placeOrder(String id);
  void cancelOrder(String id);
}
`

const javaDSLBoth = `package com.mycompany.myshop.testkit.dsl.core.usecase.myshop;
public class OrderDsl {
  private final PlaceOrderPort port;
  public void roundTripOrder(String id) {
    port.placeOrder(id);
    port.cancelOrder(id);
  }
}
`

const javaTestRoundTrip = `package com.mycompany.myshop.systemtest.latest.acceptance;
public class OrderRoundTripTest {
  @TestTemplate
  @Channel({ChannelType.API})
  public void shouldRoundTrip() {
    dsl.roundTripOrder("o-9");
  }
}
`

// Adapter with internal helper method that has no port and shouldn't match.
const javaAdapterWithPrivate = `package com.mycompany.myshop.testkit.driver.adapter.myshop.api;
public class CustomerApiAdapter implements RegisterCustomerPort {
  @Override
  public void register(String email) {
    internalDebugDump();
  }
  private void internalDebugDump() {
    // helper
  }
}
`

// ----------------------------------------------------------------------------
// Tests
// ----------------------------------------------------------------------------

const adapterPath = "system-test/java/src/main/java/com/mycompany/myshop/testkit/driver/adapter/myshop/api/CustomerApiAdapter.java"
const portPath = "system-test/java/src/main/java/com/mycompany/myshop/testkit/driver/port/myshop/RegisterCustomerPort.java"
const dslPath = "system-test/java/src/main/java/com/mycompany/myshop/testkit/dsl/core/usecase/myshop/RegisterCustomerDsl.java"

func TestSelect_HappyPath_SingleMethodChanged(t *testing.T) {
	files := map[string]string{
		adapterPath: javaAdapter,
		portPath:    javaPort,
		dslPath:     javaDSL,
		"system-test/java/src/test/java/com/mycompany/myshop/systemtest/latest/acceptance/RegisterCustomerPositiveTest.java": javaTest,
		"system-test/java/src/test/java/com/mycompany/myshop/systemtest/latest/acceptance/RegisterCustomerNegativeTest.java": javaTestNeg,
	}
	// Diff hits the body of register() (line 4 of the file).
	diff := diffBlock(adapterPath, "@@ -4,1 +4,1 @@")

	res, err := SelectWithDeps("repo", "HEAD", makeDeps(diff, files))
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(res.Unmapped) != 0 {
		t.Fatalf("expected no unmapped, got %v", res.Unmapped)
	}
	if len(res.Selections) != 1 {
		t.Fatalf("expected 1 selection (acceptance-api), got %d: %+v", len(res.Selections), res.Selections)
	}
	got := res.Selections[0]
	if got.Suite != "acceptance-api" {
		t.Errorf("suite: got %q want acceptance-api", got.Suite)
	}
	want := []string{"RegisterCustomerNegativeTest.shouldRejectMissingEmail", "RegisterCustomerPositiveTest.shouldRegister"}
	if !reflect.DeepEqual(got.Tests, want) {
		t.Errorf("tests: got %v want %v", got.Tests, want)
	}
}

func TestSelect_DedupAcrossChangedMethods(t *testing.T) {
	files := map[string]string{
		"system-test/java/src/main/java/com/mycompany/myshop/testkit/driver/adapter/myshop/api/OrderApiAdapter.java":             javaAdapterTwoMethods,
		"system-test/java/src/main/java/com/mycompany/myshop/testkit/driver/port/myshop/PlaceOrderPort.java":                     javaPortTwoMethods,
		"system-test/java/src/main/java/com/mycompany/myshop/testkit/dsl/core/usecase/myshop/OrderDsl.java":                      javaDSLBoth,
		"system-test/java/src/test/java/com/mycompany/myshop/systemtest/latest/acceptance/OrderRoundTripTest.java":               javaTestRoundTrip,
	}
	// Diff hits both methods (lines 3 and 4 of the file).
	diff := diffBlock(
		"system-test/java/src/main/java/com/mycompany/myshop/testkit/driver/adapter/myshop/api/OrderApiAdapter.java",
		"@@ -3,2 +3,2 @@",
	)

	res, err := SelectWithDeps("repo", "HEAD", makeDeps(diff, files))
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(res.Unmapped) != 0 {
		t.Fatalf("unmapped: %v", res.Unmapped)
	}
	if len(res.Selections) != 1 {
		t.Fatalf("selections: %+v", res.Selections)
	}
	got := res.Selections[0]
	if !reflect.DeepEqual(got.Tests, []string{"OrderRoundTripTest.shouldRoundTrip"}) {
		t.Errorf("tests: %v", got.Tests)
	}
}

func TestSelect_TransitiveDSLHelper(t *testing.T) {
	files := map[string]string{
		adapterPath: javaAdapter,
		portPath:    javaPort,
		dslPath:     javaDSL, // RegisterCustomerDsl has both registerCustomer and registerCustomerWithDefaults
		"system-test/java/src/test/java/com/mycompany/myshop/systemtest/latest/acceptance/RegisterCustomerPositiveTest.java": javaTest,
		"system-test/java/src/test/java/com/mycompany/myshop/systemtest/latest/acceptance/RegisterCustomerNegativeTest.java": javaTestNeg,
		"system-test/java/src/test/java/com/mycompany/myshop/systemtest/latest/acceptance/RegisterCustomerWithDefaultsTest.java": `package com.mycompany.myshop.systemtest.latest.acceptance;
public class RegisterCustomerWithDefaultsTest {
  @TestTemplate
  @Channel({ChannelType.API})
  public void shouldUseDefault() {
    dsl.registerCustomerWithDefaults();
  }
}
`,
	}
	diff := diffBlock(adapterPath, "@@ -4,1 +4,1 @@")

	res, err := SelectWithDeps("repo", "HEAD", makeDeps(diff, files))
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(res.Selections) != 1 {
		t.Fatalf("selections: %+v", res.Selections)
	}
	want := []string{
		"RegisterCustomerNegativeTest.shouldRejectMissingEmail",
		"RegisterCustomerPositiveTest.shouldRegister",
		"RegisterCustomerWithDefaultsTest.shouldUseDefault",
	}
	if !reflect.DeepEqual(res.Selections[0].Tests, want) {
		t.Errorf("tests: got %v want %v", res.Selections[0].Tests, want)
	}
}

func TestSelect_PrivateHelperBridgesToPortMethod(t *testing.T) {
	// `internalDebugDump` has no port of its own, but `register()` (in the
	// same adapter file) calls it and *is* port-backed. The selector should
	// bridge through the adapter caller graph and treat the change as if
	// `register()` had changed — no Unmapped entry, same selection.
	files := map[string]string{
		adapterPath: javaAdapterWithPrivate,
		portPath:    javaPort,
		dslPath:     javaDSL,
		"system-test/java/src/test/java/com/mycompany/myshop/systemtest/latest/acceptance/RegisterCustomerPositiveTest.java": javaTest,
	}
	// Hunk covers both methods (lines 4 and 7).
	diff := diffBlock(adapterPath, "@@ -4,4 +4,4 @@")

	res, err := SelectWithDeps("repo", "HEAD", makeDeps(diff, files))
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(res.Unmapped) != 0 {
		t.Errorf("expected no unmapped (helper should bridge to register), got %v", res.Unmapped)
	}
	if len(res.Selections) != 1 {
		t.Fatalf("expected one selection for register(), got %+v", res.Selections)
	}
	if res.Selections[0].Suite != "acceptance-api" {
		t.Errorf("suite: %q", res.Selections[0].Suite)
	}
	if !reflect.DeepEqual(res.Selections[0].Tests, []string{"RegisterCustomerPositiveTest.shouldRegister"}) {
		t.Errorf("tests: %v", res.Selections[0].Tests)
	}
	// Diagnostic should record the bridge.
	hasBridge := false
	for _, d := range res.Diagnostics {
		if strings.Contains(d, "internalDebugDump") && strings.Contains(d, "bridged") {
			hasBridge = true
		}
	}
	if !hasBridge {
		t.Errorf("expected bridge diagnostic, got %v", res.Diagnostics)
	}
}

func TestSelect_TrulyUnmappedAdapterMethod_NoCallerNoPort(t *testing.T) {
	// An orphan adapter method: no port match, and no other adapter method
	// calls it. Bridging should fail and the method should land in Unmapped.
	const javaAdapterOrphan = `package com.mycompany.myshop.testkit.driver.adapter.myshop.api;
public class CustomerApiAdapter implements RegisterCustomerPort {
  @Override
  public void register(String email) {
  }
  public void orphanedHelper() {
  }
}
`
	files := map[string]string{
		adapterPath: javaAdapterOrphan,
		portPath:    javaPort,
		dslPath:     javaDSL,
		"system-test/java/src/test/java/com/mycompany/myshop/systemtest/latest/acceptance/RegisterCustomerPositiveTest.java": javaTest,
	}
	// Hunk targets only the orphan helper (line 6 of the file).
	diff := diffBlock(adapterPath, "@@ -6,1 +6,1 @@")

	res, err := SelectWithDeps("repo", "HEAD", makeDeps(diff, files))
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(res.Unmapped) != 1 || res.Unmapped[0].Method != "orphanedHelper" {
		t.Errorf("expected one unmapped entry for orphanedHelper, got %v", res.Unmapped)
	}
	if len(res.Selections) != 0 {
		t.Errorf("expected no selections, got %+v", res.Selections)
	}
}

func TestSelect_TypeScriptPageObjectBridgesToDriverMethod(t *testing.T) {
	// Mirrors the real shop layout: `inputSku` lives on a Page Object
	// helper class under `testkit/driver/adapter/.../pages/`. The port is
	// named `placeOrder`, not `inputSku`. The adapter driver
	// (`my-shop-ui-driver.ts`) implements `placeOrder` and calls
	// `newOrderPage.inputSku(...)` from inside it. A change to `inputSku`
	// should bridge up to `placeOrder` and select the corresponding test.
	const tsPageObject = `import { BasePage } from './BasePage.js';
export class NewOrderPage extends BasePage {
  async inputSku(sku: string): Promise<void> {
    await this.page.locator('[aria-label="SKU"]').fill(sku);
  }
}
`
	const tsAdapter = `import { NewOrderPage } from './client/pages/NewOrderPage.js';
export class MyShopUiDriver implements MyShopDriver {
  async placeOrder(request: PlaceOrderRequest): Promise<void> {
    const newOrderPage = new NewOrderPage(this.page);
    await newOrderPage.inputSku(request.sku);
  }
}
`
	const tsPort = `export interface MyShopDriver {
  placeOrder(request: PlaceOrderRequest): Promise<void>;
}
`
	const tsDsl = `export class OrderDsl {
  constructor(private port: MyShopDriver) {}
  async whenPlacingOrder(request: PlaceOrderRequest): Promise<void> {
    return this.port.placeOrder(request);
  }
}
`
	const tsTest = `// @channel(UI)
describe("Place Order", () => {
  it("should place order", async () => {
    await dsl.whenPlacingOrder({sku: "S1"});
  });
});
`
	const pagePath = "system-test/typescript/src/testkit/driver/adapter/myShop/ui/client/pages/NewOrderPage.ts"
	const driverPath = "system-test/typescript/src/testkit/driver/adapter/myShop/ui/my-shop-ui-driver.ts"
	const portPath = "system-test/typescript/src/testkit/driver/port/myShop/my-shop-driver.ts"
	const dslPath = "system-test/typescript/src/testkit/dsl/core/usecase/myShop/order-dsl.ts"
	const testPath = "system-test/typescript/src/tests/place-order.spec.ts"

	files := map[string]string{
		pagePath:   tsPageObject,
		driverPath: tsAdapter,
		portPath:   tsPort,
		dslPath:    tsDsl,
		testPath:   tsTest,
	}
	// Hunk targets `inputSku` body (line 4 of NewOrderPage.ts).
	diff := diffBlock(pagePath, "@@ -4,1 +4,1 @@")

	res, err := SelectWithDeps("repo", "HEAD", makeDeps(diff, files))
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(res.Unmapped) != 0 {
		t.Errorf("expected no unmapped (page-object should bridge to placeOrder), got %v; diag: %v",
			res.Unmapped, res.Diagnostics)
	}
	if len(res.Selections) == 0 {
		t.Fatalf("expected at least one selection, got none; diag: %v", res.Diagnostics)
	}
	// At least one selection must include the place-order test.
	wantTest := "Place Order.should place order"
	found := false
	for _, s := range res.Selections {
		for _, n := range s.Tests {
			if n == wantTest {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected test %q in selections, got %+v; diag: %v",
			wantTest, res.Selections, res.Diagnostics)
	}
	// Diagnostic should record the bridge from inputSku → placeOrder.
	hasBridge := false
	for _, d := range res.Diagnostics {
		if strings.Contains(d, "inputSku") && strings.Contains(d, "placeOrder") && strings.Contains(d, "bridged") {
			hasBridge = true
		}
	}
	if !hasBridge {
		t.Errorf("expected bridge diagnostic mentioning inputSku → placeOrder, got %v", res.Diagnostics)
	}
}

func TestSelect_UntaggedTest_FallsBackToBothChannels(t *testing.T) {
	files := map[string]string{
		adapterPath: javaAdapter,
		portPath:    javaPort,
		dslPath:     javaDSL,
		"system-test/java/src/test/java/com/mycompany/myshop/systemtest/latest/acceptance/RegisterCustomerLooseTest.java": javaTestUntagged,
	}
	diff := diffBlock(adapterPath, "@@ -4,1 +4,1 @@")

	res, err := SelectWithDeps("repo", "HEAD", makeDeps(diff, files))
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	// Untagged test → two suites both contain it.
	suites := map[string]bool{}
	for _, s := range res.Selections {
		suites[s.Suite] = true
		if !reflect.DeepEqual(s.Tests, []string{"RegisterCustomerLooseTest.shouldRunSomehow"}) {
			t.Errorf("tests in %s: %v", s.Suite, s.Tests)
		}
	}
	if !suites["acceptance-api"] || !suites["acceptance-ui"] {
		t.Errorf("expected both channels, got %v", suites)
	}
	// Diagnostic should mention the fallback.
	hasFallback := false
	for _, d := range res.Diagnostics {
		if strings.Contains(d, "no @Channel") {
			hasFallback = true
		}
	}
	if !hasFallback {
		t.Errorf("expected diagnostic about untagged test, got %v", res.Diagnostics)
	}
}

func TestSelect_BothChannels_OneTestTwoSuites(t *testing.T) {
	files := map[string]string{
		adapterPath: javaAdapter,
		portPath:    javaPort,
		dslPath:     javaDSL,
		"system-test/java/src/test/java/com/mycompany/myshop/systemtest/latest/acceptance/RegisterCustomerBothTest.java": javaTestBoth,
	}
	diff := diffBlock(adapterPath, "@@ -4,1 +4,1 @@")

	res, err := SelectWithDeps("repo", "HEAD", makeDeps(diff, files))
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(res.Selections) != 2 {
		t.Fatalf("expected 2 selections, got %d: %+v", len(res.Selections), res.Selections)
	}
	want := []string{"RegisterCustomerBothTest.shouldRunOnBothChannels"}
	for _, s := range res.Selections {
		if !reflect.DeepEqual(s.Tests, want) {
			t.Errorf("suite %s tests: %v", s.Suite, s.Tests)
		}
	}
}

func TestSelect_ContractLayer_StubOnly(t *testing.T) {
	files := map[string]string{
		"system-test/java/src/main/java/com/mycompany/myshop/testkit/driver/adapter/external/erp/ErpAdapter.java": javaAdapterExternal,
		"system-test/java/src/main/java/com/mycompany/myshop/testkit/driver/port/external/erp/ErpPort.java":       javaPortExternal,
		"system-test/java/src/main/java/com/mycompany/myshop/testkit/dsl/core/usecase/external/ErpDsl.java":       javaDSLExternal,
		"system-test/java/src/test/java/com/mycompany/myshop/systemtest/latest/contract/ErpContractTest.java":     javaTestContract,
	}
	diff := diffBlock(
		"system-test/java/src/main/java/com/mycompany/myshop/testkit/driver/adapter/external/erp/ErpAdapter.java",
		"@@ -4,1 +4,1 @@",
	)

	res, err := SelectWithDeps("repo", "HEAD", makeDeps(diff, files))
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(res.Selections) != 1 || res.Selections[0].Suite != "contract-stub" {
		t.Fatalf("expected one contract-stub selection, got %+v", res.Selections)
	}
}

func TestSelect_NoChanges(t *testing.T) {
	res, err := SelectWithDeps("repo", "HEAD", makeDeps("", map[string]string{}))
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(res.Selections) != 0 || len(res.Unmapped) != 0 {
		t.Errorf("expected empty result, got %+v", res)
	}
}

func TestSelect_NonAdapterChange_Ignored(t *testing.T) {
	// A diff that hits a non-adapter file should produce no selections.
	files := map[string]string{
		"system-test/java/src/main/java/com/mycompany/myshop/testkit/dsl/core/usecase/myshop/RegisterCustomerDsl.java": javaDSL,
	}
	diff := diffBlock(
		"system-test/java/src/main/java/com/mycompany/myshop/testkit/dsl/core/usecase/myshop/RegisterCustomerDsl.java",
		"@@ -7,1 +7,1 @@",
	)
	res, err := SelectWithDeps("repo", "HEAD", makeDeps(diff, files))
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(res.Selections) != 0 || len(res.Unmapped) != 0 {
		t.Errorf("expected empty result, got %+v", res)
	}
}

func TestParseHunkHeader(t *testing.T) {
	cases := []struct {
		in           string
		wantStart    int
		wantEnd      int
		wantOK       bool
	}{
		{"@@ -1,1 +1,1 @@", 1, 1, true},
		{"@@ -1,0 +5,3 @@", 5, 7, true},
		{"@@ -3 +5 @@", 5, 5, true},
		{"@@ -3,2 +5,0 @@ context", 5, 4, true}, // pure deletion
		{"not a hunk", 0, 0, false},
	}
	for _, c := range cases {
		r, ok := parseHunkHeader(c.in)
		if ok != c.wantOK {
			t.Errorf("%q: ok=%v want %v", c.in, ok, c.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if r.start != c.wantStart || r.end != c.wantEnd {
			t.Errorf("%q: got [%d,%d] want [%d,%d]", c.in, r.start, r.end, c.wantStart, c.wantEnd)
		}
	}
}

func TestExtractMethodRegions_Java(t *testing.T) {
	regions := extractMethodRegions(javaDSL, layouts["java"])
	names := map[string]bool{}
	for _, r := range regions {
		names[r.name] = true
	}
	for _, want := range []string{"registerCustomer", "registerCustomerWithDefaults"} {
		if !names[want] {
			t.Errorf("missing region %q (got %v)", want, names)
		}
	}
}

func TestParseChannelArgs(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"API", []string{"API"}},
		{"{ChannelType.UI, ChannelType.API}", []string{"UI", "API"}},
		{"ChannelType.API", []string{"API"}},
		{"  UI ", []string{"UI"}},
	}
	for _, c := range cases {
		got := parseChannelArgs(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("parseChannelArgs(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

func TestSelect_RenamedAdapterMethod_NewNameFlows(t *testing.T) {
	// Adapter method `register` was renamed to `signUp`. The port and the
	// DSL have already been updated to the new name (the rename is
	// atomic in the diff). The selector should pick up the new name and
	// flow through to tests calling the DSL.
	const adapter = `package com.mycompany.myshop.testkit.driver.adapter.myshop.api;
public class CustomerApiAdapter implements RegisterCustomerPort {
  @Override
  public void signUp(String email) {
  }
}
`
	const port = `package com.mycompany.myshop.testkit.driver.port.myshop;
public interface RegisterCustomerPort {
  void signUp(String email);
}
`
	const dsl = `package com.mycompany.myshop.testkit.dsl.core.usecase.myshop;
public class RegisterCustomerDsl {
  private final RegisterCustomerPort port;
  public void registerCustomer(String email) {
    port.signUp(email);
  }
}
`
	files := map[string]string{
		adapterPath: adapter,
		portPath:    port,
		dslPath:     dsl,
		"system-test/java/src/test/java/com/mycompany/myshop/systemtest/latest/acceptance/RegisterCustomerPositiveTest.java": javaTest,
	}
	diff := diffBlock(adapterPath, "@@ -4,1 +4,1 @@")

	res, err := SelectWithDeps("repo", "HEAD", makeDeps(diff, files))
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(res.Unmapped) != 0 {
		t.Errorf("unmapped: %v", res.Unmapped)
	}
	if len(res.Selections) != 1 {
		t.Fatalf("selections: %+v", res.Selections)
	}
	if !reflect.DeepEqual(res.Selections[0].Tests, []string{"RegisterCustomerPositiveTest.shouldRegister"}) {
		t.Errorf("tests: %v", res.Selections[0].Tests)
	}
}

func TestSelect_DotNet_HappyPath(t *testing.T) {
	files := map[string]string{
		"system-test/dotnet/Driver.Adapter/MyShop/Api/CustomerApiAdapter.cs": `namespace MyCompany.MyShop.Testkit.Driver.Adapter.MyShop.Api;
public class CustomerApiAdapter : IRegisterCustomerPort {
    public void Register(string email) {
    }
}
`,
		"system-test/dotnet/Driver.Port/MyShop/IRegisterCustomerPort.cs": `namespace MyCompany.MyShop.Testkit.Driver.Port.MyShop;
public interface IRegisterCustomerPort {
    void Register(string email);
}
`,
		"system-test/dotnet/Dsl.Core/Usecase/MyShop/RegisterCustomerDsl.cs": `namespace MyCompany.MyShop.Testkit.Dsl.Core.Usecase.MyShop;
public class RegisterCustomerDsl {
    public void RegisterCustomer(string email) {
        port.Register(email);
    }
}
`,
		"system-test/dotnet/Tests/RegisterCustomerPositiveTest.cs": `namespace MyCompany.MyShop.SystemTest.Latest.Acceptance;
public class RegisterCustomerPositiveTest {
    [Fact]
    [Channel(ChannelType.API)]
    public void ShouldRegister() {
        dsl.RegisterCustomer("a@example.com");
    }
}
`,
	}
	diff := diffBlock(
		"system-test/dotnet/Driver.Adapter/MyShop/Api/CustomerApiAdapter.cs",
		"@@ -3,1 +3,1 @@",
	)
	res, err := SelectWithDeps("repo", "HEAD", makeDeps(diff, files))
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(res.Selections) != 1 || res.Selections[0].Suite != "acceptance-api" {
		t.Fatalf("selections: %+v unmapped: %+v diag: %+v", res.Selections, res.Unmapped, res.Diagnostics)
	}
	if !reflect.DeepEqual(res.Selections[0].Tests, []string{"RegisterCustomerPositiveTest.ShouldRegister"}) {
		t.Errorf("tests: %v", res.Selections[0].Tests)
	}
}

func TestInferLayer(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"system-test/java/src/main/java/com/mycompany/myshop/testkit/driver/adapter/myshop/api/X.java", "shop"},
		{"system-test/java/src/main/java/com/mycompany/myshop/testkit/driver/adapter/external/erp/X.java", "external"},
		{"system-test/dotnet/Driver.Adapter/External/Erp/X.cs", "external"},
		{"system-test/dotnet/Driver.Adapter/MyShop/Api/X.cs", "shop"},
	}
	for _, c := range cases {
		got := inferLayer(c.path)
		if got != c.want {
			t.Errorf("inferLayer(%q)=%q want %q", c.path, got, c.want)
		}
	}
}
