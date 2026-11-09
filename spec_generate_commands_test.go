package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/spec"
)

// specGenerateFixture writes spec.yaml, one use case and one Java test file
// whose body is body, and returns the spec.yaml path and the test dir.
func specGenerateFixture(t *testing.T, body string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"spec.yaml":                 "specDir: specs\n",
		"specs/UC-001-place.md":     "---\nid: UC-001\ntitle: Place Order\n---\n",
		"tests/PlaceOrderTest.java": "class PlaceOrderTest {\n    @Tag(\"UC-001.1\")\n    @Test\n    void shouldPlaceOrder() {\n        " + body + "\n    }\n}\n",
	}
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, "spec.yaml"), filepath.Join(dir, "tests")
}

func TestSpecGenerate_PrintsGherkin(t *testing.T) {
	cfg, tests := specGenerateFixture(t, `scenario.when().placeOrder().withQuantity(5).then().shouldSucceed();`)
	var out, errOut bytes.Buffer
	code, err := runSpecGenerate(specGenerateOptions{Config: cfg, Tests: []string{tests}}, &out, &errOut)
	if err != nil || code != spec.ExitPass {
		t.Fatalf("code=%d err=%v stderr=%s", code, err, errOut.String())
	}
	for _, want := range []string{"Feature: UC-001 Place Order", "@UC-001.1", "When place order with quantity 5"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output lacks %q:\n%s", want, out.String())
		}
	}
}

func TestSpecGenerate_UnparseableWritesNothing(t *testing.T) {
	cfg, tests := specGenerateFixture(t, `scenario.when().placeOrder();`)
	outFile := filepath.Join(t.TempDir(), "criteria.feature")
	var out, errOut bytes.Buffer
	code, err := runSpecGenerate(specGenerateOptions{Config: cfg, Tests: []string{tests}, Out: outFile}, &out, &errOut)
	if err != nil || code != spec.ExitGateFailed {
		t.Fatalf("code=%d err=%v, want exit 1", code, err)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", out.String())
	}
	if _, statErr := os.Stat(outFile); !os.IsNotExist(statErr) {
		t.Fatalf("--out file was written")
	}
	if !strings.Contains(errOut.String(), "PlaceOrderTest.java:4 shouldPlaceOrder") {
		t.Fatalf("stderr lacks file:line and test name: %s", errOut.String())
	}
}

func TestSpecGenerate_UsageErrors(t *testing.T) {
	cfg, _ := specGenerateFixture(t, `scenario.when().placeOrder().then().shouldSucceed();`)
	var out, errOut bytes.Buffer
	if _, err := runSpecGenerate(specGenerateOptions{Config: cfg, Tests: []string{"x"}, UseCases: []string{"UC-001.1"}}, &out, &errOut); err == nil {
		t.Fatal("want an error for a criterion ID passed as --use-case")
	}
	if _, err := runSpecGenerate(specGenerateOptions{Config: cfg}, &out, &errOut); err == nil || !strings.Contains(err.Error(), "at-test") {
		t.Fatalf("err = %v, want missing gh-optivem.yaml at-test error", err)
	}
}
