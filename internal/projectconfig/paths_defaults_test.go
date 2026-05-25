package projectconfig

import (
	"reflect"
	"testing"
)

func TestDefaultPaths_TypescriptFlatScaffold(t *testing.T) {
	t.Parallel()
	got := DefaultPaths(LangTypescript, "system-test", "shop")
	want := map[string]string{
		"driver-port":                    "system-test/src/testkit/driver/port/shop",
		"driver-adapter":                 "system-test/src/testkit/driver/adapter/shop",
		"external-system-driver-port":    "system-test/src/testkit/external/port/shop",
		"external-system-driver-adapter": "system-test/src/testkit/external/adapter/shop",
		"at-test":                        "system-test/tests/latest/acceptance",
		"dsl-port":                       "system-test/src/testkit/dsl/port/shop",
		"dsl-core":                       "system-test/src/testkit/dsl/core/shop",
		"ct-test":                        "system-test/tests/latest/contract",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("typescript: got %v, want %v", got, want)
	}
}

func TestDefaultPaths_JavaFlatScaffold(t *testing.T) {
	t.Parallel()
	got := DefaultPaths(LangJava, "system-test", "shop")
	want := map[string]string{
		"driver-port":                    "system-test/src/main/java/testkit/driver/port/shop",
		"driver-adapter":                 "system-test/src/main/java/testkit/driver/adapter/shop",
		"external-system-driver-port":    "system-test/src/main/java/testkit/external/port/shop",
		"external-system-driver-adapter": "system-test/src/main/java/testkit/external/adapter/shop",
		"at-test":                        "system-test/src/test/java/shop/latest/acceptance",
		"dsl-port":                       "system-test/src/main/java/testkit/dsl/port/shop",
		"dsl-core":                       "system-test/src/main/java/testkit/dsl/core/shop",
		"ct-test":                        "system-test/src/test/java/shop/latest/contract",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("java: got %v, want %v", got, want)
	}
}

func TestDefaultPaths_DotnetFlatScaffold(t *testing.T) {
	t.Parallel()
	got := DefaultPaths(LangDotnet, "system-test", "shop")
	want := map[string]string{
		"driver-port":                    "system-test/Testkit.Driver.Port/shop",
		"driver-adapter":                 "system-test/Testkit.Driver.Adapter/shop",
		"external-system-driver-port":    "system-test/Testkit.External.Port/shop",
		"external-system-driver-adapter": "system-test/Testkit.External.Adapter/shop",
		"at-test":                        "system-test/SystemTests/Latest/AcceptanceTests",
		"dsl-port":                       "system-test/Testkit.Dsl.Port/shop",
		"dsl-core":                       "system-test/Testkit.Dsl.Core/shop",
		"ct-test":                        "system-test/SystemTests/Latest/ExternalSystemContractTests",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("dotnet: got %v, want %v", got, want)
	}
}

// TestDefaultPaths_EmptySutNamespace — the pre-SSoT shape (no suffix) is
// what `runConfigMigrate`'s gap-fill back-fill uses for legacy configs
// until the SSoT join step (plan 20260518-1530 item 6) runs. Tests both
// a testkit key (sut_namespace-free trailing append) and at-test (Java
// stem with the package segment collapsed) plus ct-test.
func TestDefaultPaths_EmptySutNamespace(t *testing.T) {
	t.Parallel()
	got := DefaultPaths(LangTypescript, "system-test", "")
	if got["driver-port"] != "system-test/src/testkit/driver/port" {
		t.Errorf("ts driver-port with empty sutNamespace: got %q, want pre-SSoT shape", got["driver-port"])
	}
	if got["at-test"] != "system-test/tests/latest/acceptance" {
		t.Errorf("ts at-test with empty sutNamespace: got %q", got["at-test"])
	}
	if got["ct-test"] != "system-test/tests/latest/contract" {
		t.Errorf("ts ct-test with empty sutNamespace: got %q", got["ct-test"])
	}

	// Java with empty sutNamespace collapses the package segment so the
	// pre-SSoT shape stays a valid path. SSoT-aware callers pass the
	// real sutNamespace; this test pins the gap-fill fallback shape.
	gotJava := DefaultPaths(LangJava, "system-test", "")
	if gotJava["at-test"] != "system-test/src/test/java/latest/acceptance" {
		t.Errorf("java at-test with empty sutNamespace: got %q (want collapsed-package shape)", gotJava["at-test"])
	}
	if gotJava["ct-test"] != "system-test/src/test/java/latest/contract" {
		t.Errorf("java ct-test with empty sutNamespace: got %q", gotJava["ct-test"])
	}
}

// TestDefaultPaths_CustomSystemTestRoot — users who override
// --system-test-path get paths rooted under their chosen directory.
func TestDefaultPaths_CustomSystemTestRoot(t *testing.T) {
	t.Parallel()
	got := DefaultPaths(LangTypescript, "tests", "shop")
	if got["driver-port"] != "tests/src/testkit/driver/port/shop" {
		t.Errorf("driver-port: got %q, want under custom root", got["driver-port"])
	}
	if got["at-test"] != "tests/tests/latest/acceptance" {
		t.Errorf("at-test: got %q, want under custom root", got["at-test"])
	}
}

// TestDefaultPaths_EmptyForPartialConfig — partial configs (no
// architecture chosen yet) carry no test lang; the scaffolder must
// leave `paths:` absent rather than write a partial map.
func TestDefaultPaths_EmptyForPartialConfig(t *testing.T) {
	t.Parallel()
	if got := DefaultPaths("", "system-test", "shop"); got != nil {
		t.Errorf("empty testLang: got %v, want nil", got)
	}
	if got := DefaultPaths(LangJava, "", "shop"); got != nil {
		t.Errorf("empty systemTestRoot: got %v, want nil", got)
	}
}

// TestDefaultPaths_EmptyForUnknownLanguage — unsupported test languages
// (e.g. an early-stage --test-lang=python) fall through to nil so
// Validate's lang enum check fires before MaterializeProject would.
func TestDefaultPaths_EmptyForUnknownLanguage(t *testing.T) {
	t.Parallel()
	if got := DefaultPaths("python", "system-test", "shop"); got != nil {
		t.Errorf("unknown lang: got %v, want nil", got)
	}
}

// TestDefaultPaths_KeysMatchPlaceholderDoctrine — the canonical key set
// must stay aligned with the Family B doctrine. A drift here would
// mean newly-scaffolded projects ship a paths: block that references
// unknown keys (or omits keys the phase docs reference), and
// MaterializeProject would error on first run.
//
// See `internal/projectconfig/path-keys.md` for the
// canonical-key vocabulary doc.
func TestDefaultPaths_KeysMatchPlaceholderDoctrine(t *testing.T) {
	t.Parallel()
	got := DefaultPaths(LangTypescript, "system-test", "shop")
	want := []string{
		"driver-port",
		"driver-adapter",
		"external-system-driver-port",
		"external-system-driver-adapter",
		"at-test",
		"dsl-port",
		"dsl-core",
		"ct-test",
	}
	for _, key := range want {
		if _, ok := got[key]; !ok {
			t.Errorf("missing canonical key %q", key)
		}
	}
	if len(got) != len(want) {
		t.Errorf("key count: got %d (%v), want %d (%v)", len(got), got, len(want), want)
	}
}

// TestDefaultPaths_RoundTripsThroughValidate — the emitted defaults must
// not shadow Family A names (which Validate rejects). This is the safety
// net for any future addition to the canonical key set. Embeds the
// minimum non-paths fields Validate insists on so the test fails only
// when the paths: shape itself is the problem.
func TestDefaultPaths_RoundTripsThroughValidate(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Project: Project{
			Provider: ProviderGitHub,
			URL:      "https://github.com/orgs/x/projects/1",
		},
		SystemTest: TierSpec{
			Paths: DefaultPaths(LangTypescript, "system-test", "shop"),
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("default paths fail Validate: %v", err)
	}
}
