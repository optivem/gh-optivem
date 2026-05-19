package projectconfig

import "path"

// DefaultPaths returns the canonical Family B `paths:` entries for the
// given system-test language, root, and sut_namespace. The seven keys
// match the doctrine in `internal/atdd/phase-scopes.yaml`'s referenced
// vocabulary: driver_port, driver_adapter, external_driver_port,
// external_driver_adapter, at_test, dsl_port, dsl_core.
//
// Returns nil when testLang is unsupported or systemTestRoot is empty —
// the scaffolder leaves `paths:` absent for partial configs (no
// architecture chosen yet) and `Validate` accepts that shape.
//
// Per-SSoT (plan 20260518-1530 item 3), the returned values are fully
// resolved: testkit keys (driver_*, external_driver_*, dsl_*) take
// `sutNamespace` as a trailing directory segment; at_test is
// sut_namespace-free at the DefaultPaths layer (any language-specific
// structural sut_namespace usage lives in `pathStems`). A non-empty
// `sutNamespace` is the SSoT shape; `sutNamespace == ""` reproduces the
// pre-SSoT shape (no suffix) and is what `runConfigMigrate`'s
// back-fill uses until the SSoT join step (plan item 6) lands.
//
// Per-language path stems mirror the post-scaffold tree (the
// `system-test/{lang}/` subdir is flattened by `copySystemTests`):
//
//   - typescript: <root>/src/testkit/{driver|external|dsl}/{port|adapter|core}[/sutNamespace],
//     <root>/src/test
//   - java:       <root>/src/main/java/testkit/{driver|external|dsl}/{port|adapter|core}[/sutNamespace],
//     <root>/src/test/java
//   - dotnet:     <root>/Testkit.{Driver|External|Dsl}.{Port|Adapter|Core}[/sutNamespace],
//     <root>/Tests
//
// Users own subsequent edits — the scaffolder writes these defaults
// once and the migrate path back-fills only canonical keys that are
// absent. Anything beyond the canonical seven set by the user is
// preserved across migrations.
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
		// Testkit keys (everything except at_test) get sutNamespace as a
		// trailing directory segment when present. at_test's
		// sut_namespace handling is per-language and structural — owned
		// by pathStems (plan 20260518-1742 items 3a/3b).
		if key != "at_test" && sutNamespace != "" {
			stem = path.Join(stem, sutNamespace)
		}
		out[key] = path.Join(systemTestRoot, stem)
	}
	return out
}

// CanonicalPathKeys is the Family B key set in fixed order so DefaultPaths,
// the migrate back-fill, and any tests over either can iterate in the
// same order.
func CanonicalPathKeys() []string {
	return []string{
		"driver_port",
		"driver_adapter",
		"external_driver_port",
		"external_driver_adapter",
		"at_test",
		"dsl_port",
		"dsl_core",
	}
}

// pathStems returns the per-language path tails (in CanonicalPathKeys
// order) that DefaultPaths joins under systemTestRoot. Returns ok=false
// for unsupported languages so the caller can omit `paths:` rather than
// write a partial map.
//
// The sutNamespace parameter is reserved for per-language structural
// incorporation of sut_namespace (e.g. Java's at_test stem will embed
// `<sutNamespace>` as a package segment per plan 20260518-1742 item 3a).
// Today every branch ignores it; DefaultPaths handles the testkit-key
// trailing-segment append uniformly.
func pathStems(testLang, sutNamespace string) ([]string, bool) {
	_ = sutNamespace
	switch testLang {
	case LangTypescript:
		return []string{
			"src/testkit/driver/port",
			"src/testkit/driver/adapter",
			"src/testkit/external/port",
			"src/testkit/external/adapter",
			"src/test",
			"src/testkit/dsl/port",
			"src/testkit/dsl/core",
		}, true
	case LangJava:
		return []string{
			"src/main/java/testkit/driver/port",
			"src/main/java/testkit/driver/adapter",
			"src/main/java/testkit/external/port",
			"src/main/java/testkit/external/adapter",
			"src/test/java",
			"src/main/java/testkit/dsl/port",
			"src/main/java/testkit/dsl/core",
		}, true
	case LangDotnet:
		return []string{
			"Testkit.Driver.Port",
			"Testkit.Driver.Adapter",
			"Testkit.External.Port",
			"Testkit.External.Adapter",
			"Tests",
			"Testkit.Dsl.Port",
			"Testkit.Dsl.Core",
		}, true
	default:
		return nil, false
	}
}
