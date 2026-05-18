package projectconfig

import "path"

// DefaultPaths returns the canonical Family B `paths:` entries for the
// given system-test language and root. The seven keys match the doctrine
// in `internal/assets/global/docs/atdd/process/placeholders.md`:
// driver_port, driver_adapter, external_driver_port, external_driver_adapter,
// at_test, dsl_port, dsl_core.
//
// Returns nil when testLang is unsupported or systemTestRoot is empty —
// the scaffolder leaves `paths:` absent for partial configs (no
// architecture chosen yet) and `Validate` accepts that shape.
//
// Per-language path stems mirror the post-scaffold tree (the
// `system-test/{lang}/` subdir is flattened by `copySystemTests`):
//
//   - typescript: <root>/src/testkit/{driver|external|dsl}/{port|adapter|core},
//     <root>/src/test
//   - java:       <root>/src/main/java/testkit/{driver|external|dsl}/{port|adapter|core},
//     <root>/src/test/java
//   - dotnet:     <root>/Testkit.{Driver|External|Dsl}.{Port|Adapter|Core},
//     <root>/Tests
//
// Users own subsequent edits — the scaffolder writes these defaults
// once and the migrate path back-fills only canonical keys that are
// absent. Anything beyond the canonical seven set by the user is
// preserved across migrations.
func DefaultPaths(testLang, systemTestRoot string) map[string]string {
	if systemTestRoot == "" {
		return nil
	}
	keys := canonicalPathKeys()
	stems, ok := pathStems(testLang)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(keys))
	for i, key := range keys {
		out[key] = path.Join(systemTestRoot, stems[i])
	}
	return out
}

// canonicalPathKeys is the Family B key set in fixed order so DefaultPaths,
// the migrate back-fill, and any tests over either can iterate in the
// same order.
func canonicalPathKeys() []string {
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

// pathStems returns the per-language path tails (in canonicalPathKeys
// order) that DefaultPaths joins under systemTestRoot. Returns ok=false
// for unsupported languages so the caller can omit `paths:` rather than
// write a partial map.
func pathStems(testLang string) ([]string, bool) {
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
