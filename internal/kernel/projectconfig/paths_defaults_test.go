package projectconfig

import (
	"reflect"
	"strings"
	"testing"
)

// The three *FlatScaffold tests reproduce the shop template's checked-in
// `gh-optivem-monolith-<lang>.yaml` `paths:` block exactly (verified on disk,
// plan 20260526-1430), modulo the `system-test/<lang>/` root which the scaffold
// flattens to `system-test/`. Java carries its source package as a MIDDLE
// segment (`com/mycompany/myshop` here — resolved from owner+system casings at
// the real call site); TS/dotnet ignore the package arg and carry no namespace.
// Externals nest under the driver layer (`driver/{port,adapter}/external`).

func TestDefaultPaths_TypescriptFlatScaffold(t *testing.T) {
	t.Parallel()
	got := DefaultPaths(LangTypescript, "system-test", "ignored")
	want := map[string]string{
		"system-driver-port":             "system-test/src/testkit/driver/port",
		"system-driver-adapter":          "system-test/src/testkit/driver/adapter",
		"external-system-driver-port":    "system-test/src/testkit/driver/port/external",
		"external-system-driver-adapter": "system-test/src/testkit/driver/adapter/external",
		"at-test":                        "system-test/tests/latest/acceptance",
		"dsl-port":                       "system-test/src/testkit/dsl/port",
		"dsl-core":                       "system-test/src/testkit/dsl/core",
		"ct-test":                        "system-test/tests/latest/contract",
		"system-driver-adapter-shared":   "system-test/src/testkit/driver/adapter/shared",
		"common":                 "system-test/src/testkit/common",
		"domain-value-types":     "system-test/src/testkit/domainvaluetypes",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("typescript: got %v, want %v", got, want)
	}
}

func TestDefaultPaths_JavaFlatScaffold(t *testing.T) {
	t.Parallel()
	got := DefaultPaths(LangJava, "system-test", "com/mycompany/myshop")
	want := map[string]string{
		"system-driver-port":             "system-test/src/main/java/com/mycompany/myshop/testkit/driver/port",
		"system-driver-adapter":          "system-test/src/main/java/com/mycompany/myshop/testkit/driver/adapter",
		"external-system-driver-port":    "system-test/src/main/java/com/mycompany/myshop/testkit/driver/port/external",
		"external-system-driver-adapter": "system-test/src/main/java/com/mycompany/myshop/testkit/driver/adapter/external",
		"at-test":                        "system-test/src/test/java/com/mycompany/myshop/systemtest/latest/acceptance",
		"dsl-port":                       "system-test/src/main/java/com/mycompany/myshop/testkit/dsl/port",
		"dsl-core":                       "system-test/src/main/java/com/mycompany/myshop/testkit/dsl/core",
		"ct-test":                        "system-test/src/test/java/com/mycompany/myshop/systemtest/latest/contract",
		"system-driver-adapter-shared":   "system-test/src/main/java/com/mycompany/myshop/testkit/driver/adapter/shared",
		"common":                 "system-test/src/main/java/com/mycompany/myshop/testkit/common",
		"domain-value-types":     "system-test/src/main/java/com/mycompany/myshop/testkit/domainvaluetypes",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("java: got %v, want %v", got, want)
	}
}

func TestDefaultPaths_DotnetFlatScaffold(t *testing.T) {
	t.Parallel()
	got := DefaultPaths(LangDotnet, "system-test", "ignored")
	want := map[string]string{
		"system-driver-port":             "system-test/Driver.Port",
		"system-driver-adapter":          "system-test/Driver.Adapter",
		"external-system-driver-port":    "system-test/Driver.Port/External",
		"external-system-driver-adapter": "system-test/Driver.Adapter/External",
		"at-test":                        "system-test/SystemTests/Latest/AcceptanceTests",
		"dsl-port":                       "system-test/Dsl.Port",
		"dsl-core":                       "system-test/Dsl.Core",
		"ct-test":                        "system-test/SystemTests/Latest/ExternalSystemContractTests",
		"system-driver-adapter-shared":   "system-test/Driver.Adapter/Shared",
		"common":                 "system-test/Common",
		"domain-value-types":     "system-test/DomainValueTypes",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("dotnet: got %v, want %v", got, want)
	}
}

// TestDefaultSystemDriverAdapterChannels pins the per-language member shape:
// each member is the system-driver-adapter root with the channel appended as a
// subfolder, cased per language (TS/Java lowercase, .NET PascalCase). The
// members derive from the same DefaultPaths adapter value so they track the
// root.
func TestDefaultSystemDriverAdapterChannels(t *testing.T) {
	t.Parallel()
	channels := []string{"api", "ui"}
	cases := []struct {
		lang string
		want map[string]string
	}{
		{LangTypescript, map[string]string{
			"api": "system-test/src/testkit/driver/adapter/api",
			"ui":  "system-test/src/testkit/driver/adapter/ui",
		}},
		{LangJava, map[string]string{
			"api": "system-test/src/main/java/com/mycompany/myshop/testkit/driver/adapter/api",
			"ui":  "system-test/src/main/java/com/mycompany/myshop/testkit/driver/adapter/ui",
		}},
		{LangDotnet, map[string]string{
			"api": "system-test/Driver.Adapter/Api",
			"ui":  "system-test/Driver.Adapter/Ui",
		}},
	}
	for _, tc := range cases {
		pkg := "ignored"
		if tc.lang == LangJava {
			pkg = "com/mycompany/myshop"
		}
		got := DefaultSystemDriverAdapterChannels(tc.lang, "system-test", pkg, channels)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.lang, got, tc.want)
		}
	}
}

// TestDefaultSystemDriverAdapterChannels_NarrowChannelSet — an API-only project
// gets exactly one member; the subset is honoured.
func TestDefaultSystemDriverAdapterChannels_NarrowChannelSet(t *testing.T) {
	t.Parallel()
	got := DefaultSystemDriverAdapterChannels(LangTypescript, "system-test", "ignored", []string{"api"})
	want := map[string]string{"api": "system-test/src/testkit/driver/adapter/api"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("api-only: got %v, want %v", got, want)
	}
}

// TestDefaultSystemDriverAdapterChannels_EmptyCases — partial configs (no lang,
// no root) and an empty channel set yield nil, mirroring DefaultPaths.
func TestDefaultSystemDriverAdapterChannels_EmptyCases(t *testing.T) {
	t.Parallel()
	if got := DefaultSystemDriverAdapterChannels("", "system-test", "shop", []string{"api"}); got != nil {
		t.Errorf("unknown lang: got %v, want nil", got)
	}
	if got := DefaultSystemDriverAdapterChannels(LangTypescript, "", "shop", []string{"api"}); got != nil {
		t.Errorf("empty root: got %v, want nil", got)
	}
	if got := DefaultSystemDriverAdapterChannels(LangTypescript, "system-test", "shop", nil); got != nil {
		t.Errorf("no channels: got %v, want nil", got)
	}
}

// TestDefaultSystemDriverAdapterChannels_MembersUnderAdapterRoot pins the
// invariant that every member sits under the configured system-driver-adapter
// root — so a future reconcile of the root carries the members with it.
func TestDefaultSystemDriverAdapterChannels_MembersUnderAdapterRoot(t *testing.T) {
	t.Parallel()
	for _, lang := range []string{LangTypescript, LangJava, LangDotnet} {
		root := DefaultPaths(lang, "system-test", "shop")["system-driver-adapter"]
		members := DefaultSystemDriverAdapterChannels(lang, "system-test", "shop", []string{"api", "ui"})
		for ch, m := range members {
			if !strings.HasPrefix(m, root+"/") {
				t.Errorf("%s member %q = %q is not under adapter root %q", lang, ch, m, root)
			}
		}
	}
}

// TestDefaultPaths_EmptyJavaPackage — an empty javaPackage collapses the Java
// package middle segment so partial/legacy configs still get a valid path. TS
// ignores the arg entirely (no namespace anywhere), so empty is a no-op there.
func TestDefaultPaths_EmptyJavaPackage(t *testing.T) {
	t.Parallel()
	got := DefaultPaths(LangTypescript, "system-test", "")
	if got["system-driver-port"] != "system-test/src/testkit/driver/port" {
		t.Errorf("ts system-driver-port with empty javaPackage: got %q", got["system-driver-port"])
	}
	if got["at-test"] != "system-test/tests/latest/acceptance" {
		t.Errorf("ts at-test with empty javaPackage: got %q", got["at-test"])
	}
	if got["ct-test"] != "system-test/tests/latest/contract" {
		t.Errorf("ts ct-test with empty javaPackage: got %q", got["ct-test"])
	}

	// Java with an empty package collapses just the package segment; the
	// `testkit` / `systemtest` structural segments stay. SSoT-aware callers
	// pass the real `com/<org>/<sut>` package.
	gotJava := DefaultPaths(LangJava, "system-test", "")
	if gotJava["system-driver-port"] != "system-test/src/main/java/testkit/driver/port" {
		t.Errorf("java system-driver-port with empty javaPackage: got %q (want collapsed-package shape)", gotJava["system-driver-port"])
	}
	if gotJava["at-test"] != "system-test/src/test/java/systemtest/latest/acceptance" {
		t.Errorf("java at-test with empty javaPackage: got %q (want collapsed-package shape)", gotJava["at-test"])
	}
	if gotJava["ct-test"] != "system-test/src/test/java/systemtest/latest/contract" {
		t.Errorf("java ct-test with empty javaPackage: got %q", gotJava["ct-test"])
	}
}

// TestDefaultPaths_CustomSystemTestRoot — users who override
// --system-test-path get paths rooted under their chosen directory.
func TestDefaultPaths_CustomSystemTestRoot(t *testing.T) {
	t.Parallel()
	got := DefaultPaths(LangTypescript, "tests", "ignored")
	if got["system-driver-port"] != "tests/src/testkit/driver/port" {
		t.Errorf("system-driver-port: got %q, want under custom root", got["system-driver-port"])
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
// Validate's lang enum check fires and rejects the language cleanly.
func TestDefaultPaths_EmptyForUnknownLanguage(t *testing.T) {
	t.Parallel()
	if got := DefaultPaths("python", "system-test", "shop"); got != nil {
		t.Errorf("unknown lang: got %v, want nil", got)
	}
}

// TestDefaultPaths_KeysMatchPlaceholderDoctrine — the canonical key set
// must stay aligned with the Family B doctrine. A drift here would
// mean newly-scaffolded projects ship a paths: block that references
// unknown keys (or omits keys the phase docs reference), and the
// phase-doc placeholder substitution at dispatch would fail on those keys.
//
// See `internal/projectconfig/path-keys.md` for the
// canonical-key vocabulary doc.
func TestDefaultPaths_KeysMatchPlaceholderDoctrine(t *testing.T) {
	t.Parallel()
	got := DefaultPaths(LangTypescript, "system-test", "shop")
	want := []string{
		"system-driver-port",
		"system-driver-adapter",
		"external-system-driver-port",
		"external-system-driver-adapter",
		"at-test",
		"dsl-port",
		"dsl-core",
		"ct-test",
		"system-driver-adapter-shared",
		"common",
		"domain-value-types",
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

// TestDefaultDbMigrationPath_MatchesShopTemplate pins the doctrinal
// value for `system.db-migration-path` against the shop template's
// existing on-disk tree (`shop/system/db/migrations/`). A drift here
// would mean newly-scaffolded projects ship a value that the shop's
// own Flyway sidecar expectation doesn't recognise.
func TestDefaultDbMigrationPath_MatchesShopTemplate(t *testing.T) {
	t.Parallel()
	if DefaultDbMigrationPath != "system/db/migrations" {
		t.Errorf("DefaultDbMigrationPath: got %q, want %q (shop template's canonical migration tree)",
			DefaultDbMigrationPath, "system/db/migrations")
	}
}
