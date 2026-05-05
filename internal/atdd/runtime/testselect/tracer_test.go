package testselect

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// SelectTracer — pick rule
// ---------------------------------------------------------------------------

// Adapter / port that both the WHEN-vs-THEN test and the alphabetical
// tie-break test reuse. PlaceOrderPort has a single port method
// `placeOrder`; the DSLs that exercise it differ per fixture.
const tracerAdapter = `package com.mycompany.myshop.testkit.driver.adapter.myshop.api;
public class OrderApiAdapter implements PlaceOrderPort {
  @Override
  public void placeOrder(String id) {
  }
}
`

const tracerPort = `package com.mycompany.myshop.testkit.driver.port.myshop;
public interface PlaceOrderPort {
  void placeOrder(String id);
}
`

const tracerAdapterPath = "system-test/java/src/main/java/com/mycompany/myshop/testkit/driver/adapter/myshop/api/OrderApiAdapter.java"
const tracerPortPath = "system-test/java/src/main/java/com/mycompany/myshop/testkit/driver/port/myshop/PlaceOrderPort.java"

func TestSelectTracer_WhenPreferredOverThen(t *testing.T) {
	const dsl = `package com.mycompany.myshop.testkit.dsl.core.usecase.myshop;
public class OrderDsl {
  private final PlaceOrderPort port;
  public void whenPlacingOrder(String id) {
    port.placeOrder(id);
  }
  public void thenAssertOrderPlaced(String id) {
    port.placeOrder(id);
  }
}
`
	const testWhen = `package com.mycompany.myshop.systemtest.latest.acceptance;
public class PlaceOrderWhenTest {
  @TestTemplate
  @Channel({ChannelType.API})
  public void shouldPlaceOrderViaWhen() {
    dsl.whenPlacingOrder("o-1");
  }
}
`
	const testThen = `package com.mycompany.myshop.systemtest.latest.acceptance;
public class PlaceOrderThenTest {
  @TestTemplate
  @Channel({ChannelType.API})
  public void shouldAssertViaThen() {
    dsl.thenAssertOrderPlaced("o-2");
  }
}
`
	files := map[string]string{
		tracerAdapterPath: tracerAdapter,
		tracerPortPath:    tracerPort,
		"system-test/java/src/main/java/com/mycompany/myshop/testkit/dsl/core/usecase/myshop/OrderDsl.java":           dsl,
		"system-test/java/src/test/java/com/mycompany/myshop/systemtest/latest/acceptance/PlaceOrderWhenTest.java":    testWhen,
		"system-test/java/src/test/java/com/mycompany/myshop/systemtest/latest/acceptance/PlaceOrderThenTest.java":    testThen,
	}
	diff := diffBlock(tracerAdapterPath, "@@ -4,1 +4,1 @@")

	res, err := SelectTracerWithDeps("repo", "HEAD", makeDeps(diff, files))
	if err != nil {
		t.Fatalf("SelectTracer: %v", err)
	}
	if len(res.Unmapped) != 0 {
		t.Fatalf("unmapped: %v", res.Unmapped)
	}
	if len(res.Selections) != 1 {
		t.Fatalf("selections: %+v", res.Selections)
	}
	got := res.Selections[0]
	if got.Stage != "when" {
		t.Errorf("stage: got %q want when", got.Stage)
	}
	if got.DSLMethod != "whenPlacingOrder" {
		t.Errorf("DSLMethod: got %q want whenPlacingOrder", got.DSLMethod)
	}
	if got.Test != "PlaceOrderWhenTest.shouldPlaceOrderViaWhen" {
		t.Errorf("test: got %q", got.Test)
	}
	if got.Suite != "acceptance-api" {
		t.Errorf("suite: got %q", got.Suite)
	}
	if got.PortMethod != "placeOrder" {
		t.Errorf("port: got %q", got.PortMethod)
	}
}

func TestSelectTracer_GivenFallbackWhenNoWhen(t *testing.T) {
	const dsl = `package com.mycompany.myshop.testkit.dsl.core.usecase.myshop;
public class OrderDsl {
  private final PlaceOrderPort port;
  public void givenAnExistingOrder(String id) {
    port.placeOrder(id);
  }
  public void thenAssertOrderPlaced(String id) {
    port.placeOrder(id);
  }
}
`
	const testGiven = `package com.mycompany.myshop.systemtest.latest.acceptance;
public class PlaceOrderGivenTest {
  @TestTemplate
  @Channel({ChannelType.API})
  public void shouldUseGivenSetup() {
    dsl.givenAnExistingOrder("o-1");
  }
}
`
	const testThen = `package com.mycompany.myshop.systemtest.latest.acceptance;
public class PlaceOrderThenTest {
  @TestTemplate
  @Channel({ChannelType.API})
  public void shouldAssertViaThen() {
    dsl.thenAssertOrderPlaced("o-2");
  }
}
`
	files := map[string]string{
		tracerAdapterPath: tracerAdapter,
		tracerPortPath:    tracerPort,
		"system-test/java/src/main/java/com/mycompany/myshop/testkit/dsl/core/usecase/myshop/OrderDsl.java":           dsl,
		"system-test/java/src/test/java/com/mycompany/myshop/systemtest/latest/acceptance/PlaceOrderGivenTest.java":   testGiven,
		"system-test/java/src/test/java/com/mycompany/myshop/systemtest/latest/acceptance/PlaceOrderThenTest.java":    testThen,
	}
	diff := diffBlock(tracerAdapterPath, "@@ -4,1 +4,1 @@")

	res, err := SelectTracerWithDeps("repo", "HEAD", makeDeps(diff, files))
	if err != nil {
		t.Fatalf("SelectTracer: %v", err)
	}
	if len(res.Selections) != 1 {
		t.Fatalf("selections: %+v", res.Selections)
	}
	got := res.Selections[0]
	if got.Stage != "given" {
		t.Errorf("stage: got %q want given", got.Stage)
	}
	if got.DSLMethod != "givenAnExistingOrder" {
		t.Errorf("DSL: got %q", got.DSLMethod)
	}
}

func TestSelectTracer_AlphabeticalTieBreakWithinStage(t *testing.T) {
	const dsl = `package com.mycompany.myshop.testkit.dsl.core.usecase.myshop;
public class OrderDsl {
  private final PlaceOrderPort port;
  public void whenPlacingOrderBeta(String id) {
    port.placeOrder(id);
  }
  public void whenPlacingOrderAlpha(String id) {
    port.placeOrder(id);
  }
}
`
	const testAlpha = `package com.mycompany.myshop.systemtest.latest.acceptance;
public class PlaceOrderAlphaTest {
  @TestTemplate
  @Channel({ChannelType.API})
  public void shouldUseAlpha() {
    dsl.whenPlacingOrderAlpha("o-a");
  }
}
`
	const testBeta = `package com.mycompany.myshop.systemtest.latest.acceptance;
public class PlaceOrderBetaTest {
  @TestTemplate
  @Channel({ChannelType.API})
  public void shouldUseBeta() {
    dsl.whenPlacingOrderBeta("o-b");
  }
}
`
	files := map[string]string{
		tracerAdapterPath: tracerAdapter,
		tracerPortPath:    tracerPort,
		"system-test/java/src/main/java/com/mycompany/myshop/testkit/dsl/core/usecase/myshop/OrderDsl.java":           dsl,
		"system-test/java/src/test/java/com/mycompany/myshop/systemtest/latest/acceptance/PlaceOrderAlphaTest.java":   testAlpha,
		"system-test/java/src/test/java/com/mycompany/myshop/systemtest/latest/acceptance/PlaceOrderBetaTest.java":    testBeta,
	}
	diff := diffBlock(tracerAdapterPath, "@@ -4,1 +4,1 @@")

	res, err := SelectTracerWithDeps("repo", "HEAD", makeDeps(diff, files))
	if err != nil {
		t.Fatalf("SelectTracer: %v", err)
	}
	if len(res.Selections) != 1 {
		t.Fatalf("selections: %+v", res.Selections)
	}
	got := res.Selections[0]
	if got.DSLMethod != "whenPlacingOrderAlpha" {
		t.Errorf("DSL: got %q want whenPlacingOrderAlpha (alphabetical first)", got.DSLMethod)
	}
	if got.Test != "PlaceOrderAlphaTest.shouldUseAlpha" {
		t.Errorf("test: got %q", got.Test)
	}
}

func TestTracerChannelForPath(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"system-test/typescript/src/testkit/driver/adapter/myShop/ui/client/pages/NewOrderPage.ts", "acceptance-ui"},
		{"system-test/typescript/src/testkit/driver/adapter/myShop/api/MyShopApiDriver.ts", "acceptance-api"},
		{"system-test/java/src/main/java/com/mycompany/myshop/testkit/driver/adapter/myshop/ui/X.java", "acceptance-ui"},
		{"system-test/java/src/main/java/com/mycompany/myshop/testkit/driver/adapter/myshop/api/X.java", "acceptance-api"},
		{"system-test/java/src/main/java/com/mycompany/myshop/testkit/driver/adapter/external/erp/X.java", "contract-stub"},
		{"system-test/dotnet/Driver.Adapter/External/Erp/X.cs", "contract-stub"},
		{"system-test/dotnet/Driver.Adapter/MyShop/Ui/X.cs", "acceptance-ui"},
		{"system-test/dotnet/Driver.Adapter/MyShop/Api/X.cs", "acceptance-api"},
		// No /ui/, /api/, or /external/ — unmapped.
		{"system-test/java/src/main/java/com/mycompany/myshop/testkit/driver/adapter/myshop/X.java", ""},
	}
	for _, c := range cases {
		got := tracerChannelForPath(c.path)
		if got != c.want {
			t.Errorf("tracerChannelForPath(%q)=%q want %q", c.path, got, c.want)
		}
	}
}

func TestSelectTracer_UnmappedChannel_NoSegment(t *testing.T) {
	// Adapter sits directly under testkit/driver/adapter/myshop/ with no
	// /ui/, /api/, or /external/ segment — channel inference fails and the
	// change goes into Unmapped.
	const adapter = `package com.mycompany.myshop.testkit.driver.adapter.myshop;
public class OrphanAdapter implements OrphanPort {
  public void doThing(String id) {
  }
}
`
	const port = `package com.mycompany.myshop.testkit.driver.port.myshop;
public interface OrphanPort {
  void doThing(String id);
}
`
	const dsl = `package com.mycompany.myshop.testkit.dsl.core.usecase.myshop;
public class OrphanDsl {
  private final OrphanPort port;
  public void whenDoingThing(String id) {
    port.doThing(id);
  }
}
`
	const test = `package com.mycompany.myshop.systemtest.latest.acceptance;
public class OrphanTest {
  @TestTemplate
  @Channel({ChannelType.API})
  public void shouldDoThing() {
    dsl.whenDoingThing("x");
  }
}
`
	const adapterPath = "system-test/java/src/main/java/com/mycompany/myshop/testkit/driver/adapter/myshop/OrphanAdapter.java"
	files := map[string]string{
		adapterPath: adapter,
		"system-test/java/src/main/java/com/mycompany/myshop/testkit/driver/port/myshop/OrphanPort.java":             port,
		"system-test/java/src/main/java/com/mycompany/myshop/testkit/dsl/core/usecase/myshop/OrphanDsl.java":         dsl,
		"system-test/java/src/test/java/com/mycompany/myshop/systemtest/latest/acceptance/OrphanTest.java":           test,
	}
	diff := diffBlock(adapterPath, "@@ -3,1 +3,1 @@")

	res, err := SelectTracerWithDeps("repo", "HEAD", makeDeps(diff, files))
	if err != nil {
		t.Fatalf("SelectTracer: %v", err)
	}
	if len(res.Selections) != 0 {
		t.Errorf("expected no selections, got %+v", res.Selections)
	}
	if len(res.Unmapped) != 1 || res.Unmapped[0].Method != "doThing" {
		t.Errorf("expected 1 unmapped doThing, got %v", res.Unmapped)
	}
}

func TestSelectTracer_UnmappedNoDSLCaller(t *testing.T) {
	// Channel infers fine, port bridges fine, but no DSL calls the port
	// method at all → unmapped.
	const dsl = `package com.mycompany.myshop.testkit.dsl.core.usecase.myshop;
public class OrderDsl {
  // No port caller — DSL only contains unrelated helpers.
  public void someUnrelatedHelper() {
  }
}
`
	files := map[string]string{
		tracerAdapterPath: tracerAdapter,
		tracerPortPath:    tracerPort,
		"system-test/java/src/main/java/com/mycompany/myshop/testkit/dsl/core/usecase/myshop/OrderDsl.java": dsl,
	}
	diff := diffBlock(tracerAdapterPath, "@@ -4,1 +4,1 @@")

	res, err := SelectTracerWithDeps("repo", "HEAD", makeDeps(diff, files))
	if err != nil {
		t.Fatalf("SelectTracer: %v", err)
	}
	if len(res.Selections) != 0 {
		t.Errorf("expected no selections, got %+v", res.Selections)
	}
	if len(res.Unmapped) != 1 {
		t.Errorf("expected 1 unmapped, got %v", res.Unmapped)
	}
}

func TestSelectTracer_TypeScriptPageObjectChain(t *testing.T) {
	// Real-world worked example: inputSku → MyShopUiDriver.placeOrder →
	// port placeOrder → whenPlacingOrder DSL → place-order test.
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
  async thenAssertOrderPlaced(request: PlaceOrderRequest): Promise<void> {
    return this.port.placeOrder(request);
  }
}
`
	// One it() per fixture file — TS test-region resolution in the existing
	// callersOfTest is file-granular, so multi-it() fixtures aren't useful
	// to assert tracer disambiguation. Java fixtures (above) carry that
	// load.
	const tsTestWhen = `// @channel(UI)
describe("Place Order", () => {
  it("should place order", async () => {
    await dsl.whenPlacingOrder({sku: "S1"});
  });
});
`
	const tsTestThen = `// @channel(UI)
describe("Assert Order", () => {
  it("should assert order placed", async () => {
    await dsl.thenAssertOrderPlaced({sku: "S2"});
  });
});
`
	const pagePath = "system-test/typescript/src/testkit/driver/adapter/myShop/ui/client/pages/NewOrderPage.ts"
	const driverPath = "system-test/typescript/src/testkit/driver/adapter/myShop/ui/my-shop-ui-driver.ts"
	const portPath = "system-test/typescript/src/testkit/driver/port/myShop/my-shop-driver.ts"
	const dslPath = "system-test/typescript/src/testkit/dsl/core/usecase/myShop/order-dsl.ts"
	files := map[string]string{
		pagePath:   tsPageObject,
		driverPath: tsAdapter,
		portPath:   tsPort,
		dslPath:    tsDsl,
		"system-test/typescript/src/tests/place-order.spec.ts": tsTestWhen,
		"system-test/typescript/src/tests/assert-order.spec.ts": tsTestThen,
	}
	diff := diffBlock(pagePath, "@@ -4,1 +4,1 @@")

	res, err := SelectTracerWithDeps("repo", "HEAD", makeDeps(diff, files))
	if err != nil {
		t.Fatalf("SelectTracer: %v", err)
	}
	if len(res.Unmapped) != 0 {
		t.Errorf("unmapped: %v; diag: %v", res.Unmapped, res.Diagnostics)
	}
	if len(res.Selections) != 1 {
		t.Fatalf("expected 1 selection, got %+v; diag: %v", res.Selections, res.Diagnostics)
	}
	got := res.Selections[0]
	if got.Suite != "acceptance-ui" {
		t.Errorf("suite: %q", got.Suite)
	}
	if got.Stage != "when" {
		t.Errorf("stage: %q want when", got.Stage)
	}
	if got.DSLMethod != "whenPlacingOrder" {
		t.Errorf("DSLMethod: %q", got.DSLMethod)
	}
	if got.Test != "Place Order.should place order" {
		t.Errorf("test: %q", got.Test)
	}
	if got.AdapterMethod != "inputSku" {
		t.Errorf("adapter method: %q", got.AdapterMethod)
	}
}

func TestSelectTracer_ExternalLayerFallbackNoStage(t *testing.T) {
	// Contract-stub adapters under external/ may have a DSL caller that
	// doesn't follow when/given/then naming. Tracer should fall back to
	// alphabetical-first when no stage matches.
	const adapter = `package com.mycompany.myshop.testkit.driver.adapter.external.erp;
public class ErpAdapter implements ErpPort {
  public void postInvoice(String id) {
  }
}
`
	const port = `package com.mycompany.myshop.testkit.driver.port.external.erp;
public interface ErpPort {
  void postInvoice(String id);
}
`
	const dsl = `package com.mycompany.myshop.testkit.dsl.core.usecase.external;
public class ErpDsl {
  private final ErpPort port;
  public void postInvoiceFromOrder(String id) {
    port.postInvoice(id);
  }
}
`
	const test = `package com.mycompany.myshop.systemtest.latest.contract;
public class ErpContractTest {
  @TestTemplate
  public void shouldPostInvoice() {
    dsl.postInvoiceFromOrder("o-1");
  }
}
`
	const adapterPath = "system-test/java/src/main/java/com/mycompany/myshop/testkit/driver/adapter/external/erp/ErpAdapter.java"
	files := map[string]string{
		adapterPath: adapter,
		"system-test/java/src/main/java/com/mycompany/myshop/testkit/driver/port/external/erp/ErpPort.java":      port,
		"system-test/java/src/main/java/com/mycompany/myshop/testkit/dsl/core/usecase/external/ErpDsl.java":      dsl,
		"system-test/java/src/test/java/com/mycompany/myshop/systemtest/latest/contract/ErpContractTest.java":   test,
	}
	diff := diffBlock(adapterPath, "@@ -3,1 +3,1 @@")

	res, err := SelectTracerWithDeps("repo", "HEAD", makeDeps(diff, files))
	if err != nil {
		t.Fatalf("SelectTracer: %v", err)
	}
	if len(res.Unmapped) != 0 {
		t.Errorf("unmapped: %v; diag: %v", res.Unmapped, res.Diagnostics)
	}
	if len(res.Selections) != 1 {
		t.Fatalf("expected 1 selection, got %+v", res.Selections)
	}
	got := res.Selections[0]
	if got.Suite != "contract-stub" {
		t.Errorf("suite: %q", got.Suite)
	}
	if got.Stage != "" {
		t.Errorf("stage: %q want empty (no stage in name)", got.Stage)
	}
	if got.DSLMethod != "postInvoiceFromOrder" {
		t.Errorf("DSL: %q", got.DSLMethod)
	}
}

func TestStageOfDSLPath(t *testing.T) {
	cases := []struct {
		file   string
		method string
		want   string
	}{
		{"system-test/.../when/WhenPlaceOrder.java", "placeOrder", "when"},
		{"system-test/.../given/GivenSetup.java", "setup", "given"},
		{"system-test/.../then/ThenAssert.java", "assertX", "then"},
		// Method-name prefix.
		{"system-test/.../OrderDsl.java", "whenPlacingOrder", "when"},
		{"system-test/.../OrderDsl.java", "givenSomething", "given"},
		{"system-test/.../OrderDsl.java", "thenAssert", "then"},
		// File-basename prefix.
		{"system-test/.../WhenSomething.java", "placeOrder", "when"},
		{"system-test/.../GivenSomething.java", "placeOrder", "given"},
		{"system-test/.../ThenSomething.java", "placeOrder", "then"},
		// No stage signal.
		{"system-test/.../OrderDsl.java", "placeOrder", ""},
	}
	for _, c := range cases {
		got := stageOfDSLPath(c.file, c.method)
		if got != c.want {
			t.Errorf("stageOfDSLPath(%q, %q)=%q want %q", c.file, c.method, got, c.want)
		}
	}
}

func TestSelectTracer_DiagnosticsRecordChain(t *testing.T) {
	// Validate that the tracer's diag trail mentions both the bridge
	// (when adapter method != port name) and the DSL/test pick. A regression
	// in the diag shape is easy to miss otherwise — the verbose UI relies
	// on these strings.
	const dsl = `package com.mycompany.myshop.testkit.dsl.core.usecase.myshop;
public class OrderDsl {
  private final PlaceOrderPort port;
  public void whenPlacingOrder(String id) {
    port.placeOrder(id);
  }
}
`
	const test = `package com.mycompany.myshop.systemtest.latest.acceptance;
public class PlaceOrderTest {
  @TestTemplate
  @Channel({ChannelType.API})
  public void shouldPlaceOrder() {
    dsl.whenPlacingOrder("o-1");
  }
}
`
	files := map[string]string{
		tracerAdapterPath: tracerAdapter,
		tracerPortPath:    tracerPort,
		"system-test/java/src/main/java/com/mycompany/myshop/testkit/dsl/core/usecase/myshop/OrderDsl.java":     dsl,
		"system-test/java/src/test/java/com/mycompany/myshop/systemtest/latest/acceptance/PlaceOrderTest.java": test,
	}
	diff := diffBlock(tracerAdapterPath, "@@ -4,1 +4,1 @@")

	res, err := SelectTracerWithDeps("repo", "HEAD", makeDeps(diff, files))
	if err != nil {
		t.Fatalf("SelectTracer: %v", err)
	}
	if len(res.Selections) != 1 {
		t.Fatalf("selections: %+v", res.Selections)
	}
	joined := strings.Join(res.Diagnostics, "\n")
	for _, want := range []string{"port \"placeOrder\"", "DSL \"whenPlacingOrder\"", "test \"PlaceOrderTest.shouldPlaceOrder\"", "suite acceptance-api"} {
		if !strings.Contains(joined, want) {
			t.Errorf("diagnostics missing %q\nfull:\n%s", want, joined)
		}
	}
}

func TestSelectTracer_NoChanges(t *testing.T) {
	res, err := SelectTracerWithDeps("repo", "HEAD", makeDeps("", map[string]string{}))
	if err != nil {
		t.Fatalf("SelectTracer: %v", err)
	}
	if len(res.Selections) != 0 || len(res.Unmapped) != 0 {
		t.Errorf("expected empty result, got %+v", res)
	}
}

func TestSelectTracer_OrderingStable(t *testing.T) {
	// Two adapters edited in the same diff (one ui, one api). Selections
	// should sort by suite first.
	const adapterUI = `package com.mycompany.myshop.testkit.driver.adapter.myshop.ui;
public class OrderUiAdapter implements PlaceOrderUiPort {
  @Override
  public void placeOrderUi(String id) {
  }
}
`
	const adapterAPI = `package com.mycompany.myshop.testkit.driver.adapter.myshop.api;
public class OrderApiAdapter implements PlaceOrderApiPort {
  @Override
  public void placeOrderApi(String id) {
  }
}
`
	const portUI = `package com.mycompany.myshop.testkit.driver.port.myshop;
public interface PlaceOrderUiPort {
  void placeOrderUi(String id);
}
`
	const portAPI = `package com.mycompany.myshop.testkit.driver.port.myshop;
public interface PlaceOrderApiPort {
  void placeOrderApi(String id);
}
`
	const dsl = `package com.mycompany.myshop.testkit.dsl.core.usecase.myshop;
public class OrderDsl {
  private final PlaceOrderUiPort uiPort;
  private final PlaceOrderApiPort apiPort;
  public void whenPlacingOrderViaUi(String id) {
    uiPort.placeOrderUi(id);
  }
  public void whenPlacingOrderViaApi(String id) {
    apiPort.placeOrderApi(id);
  }
}
`
	const testUI = `package com.mycompany.myshop.systemtest.latest.acceptance;
public class PlaceOrderUiTest {
  @TestTemplate
  @Channel({ChannelType.UI})
  public void shouldPlaceOrderUi() {
    dsl.whenPlacingOrderViaUi("u");
  }
}
`
	const testAPI = `package com.mycompany.myshop.systemtest.latest.acceptance;
public class PlaceOrderApiTest {
  @TestTemplate
  @Channel({ChannelType.API})
  public void shouldPlaceOrderApi() {
    dsl.whenPlacingOrderViaApi("a");
  }
}
`
	const adapterUIPath = "system-test/java/src/main/java/com/mycompany/myshop/testkit/driver/adapter/myshop/ui/OrderUiAdapter.java"
	const adapterAPIPath = "system-test/java/src/main/java/com/mycompany/myshop/testkit/driver/adapter/myshop/api/OrderApiAdapter.java"

	files := map[string]string{
		adapterUIPath:  adapterUI,
		adapterAPIPath: adapterAPI,
		"system-test/java/src/main/java/com/mycompany/myshop/testkit/driver/port/myshop/PlaceOrderUiPort.java":  portUI,
		"system-test/java/src/main/java/com/mycompany/myshop/testkit/driver/port/myshop/PlaceOrderApiPort.java": portAPI,
		"system-test/java/src/main/java/com/mycompany/myshop/testkit/dsl/core/usecase/myshop/OrderDsl.java":     dsl,
		"system-test/java/src/test/java/com/mycompany/myshop/systemtest/latest/acceptance/PlaceOrderUiTest.java":  testUI,
		"system-test/java/src/test/java/com/mycompany/myshop/systemtest/latest/acceptance/PlaceOrderApiTest.java": testAPI,
	}
	diff := diffBlock(adapterUIPath, "@@ -4,1 +4,1 @@") +
		diffBlock(adapterAPIPath, "@@ -4,1 +4,1 @@")

	res, err := SelectTracerWithDeps("repo", "HEAD", makeDeps(diff, files))
	if err != nil {
		t.Fatalf("SelectTracer: %v", err)
	}
	if len(res.Selections) != 2 {
		t.Fatalf("expected 2 selections, got %+v", res.Selections)
	}
	gotSuites := []string{res.Selections[0].Suite, res.Selections[1].Suite}
	wantSuites := []string{"acceptance-api", "acceptance-ui"}
	if !reflect.DeepEqual(gotSuites, wantSuites) {
		// Only assert the sorted order — the test is about determinism.
		sorted := append([]string(nil), gotSuites...)
		sort.Strings(sorted)
		if !reflect.DeepEqual(sorted, wantSuites) {
			t.Errorf("suites: got %v want sorted contents %v", gotSuites, wantSuites)
		}
		if gotSuites[0] != "acceptance-api" {
			t.Errorf("expected acceptance-api first (sorted), got %v", gotSuites)
		}
	}
}
