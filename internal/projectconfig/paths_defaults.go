package projectconfig

import "path"

// DefaultPaths returns the canonical Family B `paths:` entries for the
// given system-test language, root, and sut_namespace. The eight keys
// match the doctrine in `internal/atdd/phase-scopes.yaml`'s referenced
// vocabulary: driver_port, driver_adapter, external_driver_port,
// external_driver_adapter, at_test, dsl_port, dsl_core, ct_test.
//
// Returns nil when testLang is unsupported or systemTestRoot is empty —
// the scaffolder leaves `paths:` absent for partial configs (no
// architecture chosen yet) and `Validate` accepts that shape.
//
// Per-SSoT (plan 20260518-1530 item 3), the returned values are fully
// resolved: testkit keys (driver_*, external_driver_*, dsl_*) take
// `sutNamespace` as a trailing directory segment. at_test and ct_test
// are sut_namespace-free at the DefaultPaths trailing-append layer —
// Java's stems already incorporate sutNamespace as a middle (package)
// segment per plan 20260518-1742 items 3a/3b; TypeScript and dotnet
// stems don't structure tests by namespace. A non-empty `sutNamespace`
// is the SSoT shape; `sutNamespace == ""` reproduces the pre-SSoT
// shape (no suffix, Java tests with the package segment collapsed) and
// is what `runConfigMigrate`'s gap-fill back-fill uses for pre-SSoT
// configs until the SSoT join step (plan 1530 item 6) runs.
//
// Per-language path stems mirror the post-scaffold tree (the
// `system-test/{lang}/` subdir is flattened by `copySystemTests`) and
// are pinned against the shop template's `latest/` form, per plan
// 20260518-1742 items 3a/3b. `latest` / `Latest` is doctrine literal,
// not project-customizable:
//
//   - typescript: <root>/src/testkit/{driver|external|dsl}/{port|adapter|core}[/sutNamespace],
//     <root>/tests/latest/acceptance,
//     <root>/tests/latest/contract
//   - java:       <root>/src/main/java/testkit/{driver|external|dsl}/{port|adapter|core}[/sutNamespace],
//     <root>/src/test/java/<sutNamespace>/latest/acceptance,
//     <root>/src/test/java/<sutNamespace>/latest/contract
//   - dotnet:     <root>/Testkit.{Driver|External|Dsl}.{Port|Adapter|Core}[/sutNamespace],
//     <root>/SystemTests/Latest/AcceptanceTests,
//     <root>/SystemTests/Latest/ExternalSystemContractTests
//
// The dotnet contract-test stem (`ExternalSystemContractTests`) is
// asymmetric vs the acceptance/etc. `<TestType>Tests` naming. It is
// pinned as a literal here against the shop template, not derived by
// rule.
//
// Users own subsequent edits — the scaffolder writes these defaults
// once and the migrate path back-fills only canonical keys that are
// absent. Anything beyond the canonical eight set by the user is
// preserved across migrations.
//
// See `internal/assets/global/docs/atdd/process/path-keys.md` for the
// canonical-key vocabulary doc consumed by `gh-optivem.yaml paths:`
// and `internal/atdd/phase-scopes.yaml`.
func DefaultPaths(testLang, systemTestRoot, sutNamespace string) map[string]string {
	if systemTestRoot == "" {
		return nil
	}
	keys := CanonicalPathKeys()
	stems, ok := pathStems(testLang, sutNamespace)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(keys))
	for i, key := range keys {
		stem := stems[i]
		// Testkit keys (driver/external/dsl) get sutNamespace as a trailing
		// directory segment when present. at_test and ct_test are
		// sut_namespace-free at this layer — Java's stems already
		// incorporate sutNamespace as a middle (package) segment via
		// pathStems; TS and dotnet stems don't structure tests by
		// namespace.
		if key != "at_test" && key != "ct_test" && sutNamespace != "" {
			stem = path.Join(stem, sutNamespace)
		}
		out[key] = path.Join(systemTestRoot, stem)
	}
	return out
}

// CanonicalPathKeys is the Family B key set in fixed order so DefaultPaths,
// the migrate back-fill, and any tests over either can iterate in the
// same order.
//
// See `internal/assets/global/docs/atdd/process/path-keys.md` for the
// vocabulary doc and `internal/atdd/phase-scopes.yaml` for the per-phase
// scope assignment that consumes these keys.
func CanonicalPathKeys() []string {
	return []string{
		"driver_port",
		"driver_adapter",
		"external_driver_port",
		"external_driver_adapter",
		"at_test",
		"dsl_port",
		"dsl_core",
		"ct_test",
	}
}

// pathStems returns the per-language path tails (in CanonicalPathKeys
// order) that DefaultPaths joins under systemTestRoot. Returns ok=false
// for unsupported languages so the caller can omit `paths:` rather than
// write a partial map.
//
// The sutNamespace parameter is consumed by the Java branch to
// interpolate the `<sutNamespace>` package segment in at_test and
// ct_test stems (per plan 20260518-1742 items 3a/3b — Java structures
// tests by package, TS and dotnet don't). The TS and dotnet branches
// ignore it; DefaultPaths handles the testkit-key trailing-segment
// append uniformly for all three languages.
func pathStems(testLang, sutNamespace string) ([]string, bool) {
	switch testLang {
	case LangTypescript:
		return []string{
			"src/testkit/driver/port",
			"src/testkit/driver/adapter",
			"src/testkit/external/port",
			"src/testkit/external/adapter",
			"tests/latest/acceptance",
			"src/testkit/dsl/port",
			"src/testkit/dsl/core",
			"tests/latest/contract",
		}, true
	case LangJava:
		atTest := path.Join("src/test/java", sutNamespace, "latest/acceptance")
		ctTest := path.Join("src/test/java", sutNamespace, "latest/contract")
		return []string{
			"src/main/java/testkit/driver/port",
			"src/main/java/testkit/driver/adapter",
			"src/main/java/testkit/external/port",
			"src/main/java/testkit/external/adapter",
			atTest,
			"src/main/java/testkit/dsl/port",
			"src/main/java/testkit/dsl/core",
			ctTest,
		}, true
	case LangDotnet:
		return []string{
			"Testkit.Driver.Port",
			"Testkit.Driver.Adapter",
			"Testkit.External.Port",
			"Testkit.External.Adapter",
			"SystemTests/Latest/AcceptanceTests",
			"Testkit.Dsl.Port",
			"Testkit.Dsl.Core",
			"SystemTests/Latest/ExternalSystemContractTests",
		}, true
	default:
		return nil, false
	}
}
