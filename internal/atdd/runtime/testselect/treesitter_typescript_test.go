package testselect

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// Coverage for TypeScript declaration shapes the regex layer couldn't
// parse. Each test exercises the tree-sitter implementation directly so
// failures point at the parser/query, not the upstream Select pipeline.

func TestTreesitterMethodIndexer_ArrowPropertyClassField(t *testing.T) {
	// The original `inputSku` failure: a class field whose value is an
	// arrow function. Regex's anchored signature shape (`name(...)`)
	// could not match `name = (...) => {...}`.
	const body = `export class NewOrderPage {
  inputSku = async (sku: string): Promise<void> => {
    await this.page.locator('[aria-label="SKU"]').fill(sku);
  };
}
`
	regions := treesitterMethodIndexer(body)
	if !hasMethodNamed(regions, "inputSku") {
		t.Fatalf("expected method 'inputSku', got %+v", regions)
	}
}

func TestTreesitterMethodIndexer_MultilineSignature(t *testing.T) {
	// Open paren on a continuation line — regex anchored on the same
	// line as the name failed to match.
	const body = `export class OrderService {
  async placeOrder
  (
    request: PlaceOrderRequest,
    options: PlaceOrderOptions,
  ): Promise<OrderResult> {
    return this.driver.send(request);
  }
}
`
	regions := treesitterMethodIndexer(body)
	if !hasMethodNamed(regions, "placeOrder") {
		t.Fatalf("expected method 'placeOrder', got %+v", regions)
	}
}

func TestTreesitterMethodIndexer_GetterAndSetter(t *testing.T) {
	const body = `export class Session {
  get currentUser(): User { return this._user; }
  set timeout(ms: number) { this._timeout = ms; }
}
`
	regions := treesitterMethodIndexer(body)
	if !hasMethodNamed(regions, "currentUser") {
		t.Errorf("expected getter 'currentUser', got %+v", regions)
	}
	if !hasMethodNamed(regions, "timeout") {
		t.Errorf("expected setter 'timeout', got %+v", regions)
	}
}

func TestTreesitterMethodIndexer_DecoratedMethod(t *testing.T) {
	// Stage-3 decorator preceding a method declaration. The regex layer
	// could mis-anchor on the decorator line; tree-sitter parses the
	// decorator + method_definition as a single declaration.
	const body = `export class OrderService {
  @traced
  @retry(3)
  async placeOrder(request: PlaceOrderRequest): Promise<void> {
    await this.driver.send(request);
  }
}
`
	regions := treesitterMethodIndexer(body)
	if !hasMethodNamed(regions, "placeOrder") {
		t.Fatalf("expected method 'placeOrder', got %+v", regions)
	}
}

func TestTreesitterCallerFinder_ArrowPropertyAndDotted(t *testing.T) {
	// Arrow-property field invoked via member-expression: the callee
	// `inputSku` lives inside `newOrderPage.inputSku(...)`.
	const body = `async function run() {
  const newOrderPage = new NewOrderPage();
  await newOrderPage.inputSku("S1");
  inputSku("free-call");
}
`
	offs := treesitterCallerFinder(body, "inputSku")
	if len(offs) != 2 {
		t.Fatalf("expected 2 call sites for 'inputSku' (member + free), got %d (offsets %v)", len(offs), offs)
	}
	for _, off := range offs {
		if off < 0 || off >= len(body) {
			t.Errorf("offset %d out of range", off)
		}
	}
}

func TestTreesitterClassExtractor_ExtendsAndImplements(t *testing.T) {
	const body = `export interface MyShopDriver {}
export class BasePage {}
export class NewOrderPage extends BasePage implements MyShopDriver {
  inputSku = async (sku: string): Promise<void> => {};
}
`
	declared, parents := treesitterClassExtractor(body)

	gotDeclared := append([]string{}, declared...)
	sort.Strings(gotDeclared)
	wantDeclared := []string{"BasePage", "MyShopDriver", "NewOrderPage"}
	if !reflect.DeepEqual(gotDeclared, wantDeclared) {
		t.Errorf("declared: got %v want %v", gotDeclared, wantDeclared)
	}

	gotParents := append([]string{}, parents...)
	sort.Strings(gotParents)
	wantParents := []string{"BasePage", "MyShopDriver"}
	if !reflect.DeepEqual(gotParents, wantParents) {
		t.Errorf("parents: got %v want %v", gotParents, wantParents)
	}
}

func hasMethodNamed(regions []methodRegion, name string) bool {
	for _, r := range regions {
		if r.name == name {
			return true
		}
	}
	return false
}

// BenchmarkTreeSitterIndex_TypeScript indexes ~50 synthesised TS files
// resembling a small shop's testkit tree. Sanity check, not a
// load-bearing gate: on a real verify cycle the runtime is dominated by
// test execution, not parsing.
func BenchmarkTreeSitterIndex_TypeScript(b *testing.B) {
	files := makeShopFixtureFiles(50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, body := range files {
			_ = treesitterMethodIndexer(body)
		}
	}
}

func makeShopFixtureFiles(n int) []string {
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, fmt.Sprintf(shopFixtureTemplate, i, i, i, i, strings.Repeat("    await this.helper();\n", 4)))
	}
	return out
}

const shopFixtureTemplate = `import { BasePage } from './BasePage.js';

export class Page%d extends BasePage {
  inputField%d = async (value: string): Promise<void> => {
    await this.page.locator('[aria-label="Field"]').fill(value);
  };

  async clickButton%d(): Promise<void> {
%s
  }

  get currentValue(): string { return this._value; }
  set currentValue(v: string) { this._value = v; }
}

export interface Port%d {
  doIt(arg: string): Promise<void>;
}
`

