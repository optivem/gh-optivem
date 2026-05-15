package projectconfig

import "path"

// DefaultPaths returns the canonical Family B `paths:` entries for the
// given system-test language and root. The four keys match the doctrine
// in `internal/assets/global/docs/atdd/process/placeholders.md`:
// driver_port, driver_adapter, external_driver_port, external_driver_adapter.
//
// Returns nil when testLang is unsupported or systemTestRoot is empty —
// the scaffolder leaves `paths:` absent for partial configs (no
// architecture chosen yet) and `Validate` accepts that shape.
//
// Per-language path stems mirror the post-scaffold tree (the
// `system-test/{lang}/` subdir is flattened by `copySystemTests`):
//
//   - typescript: <root>/src/testkit/{driver|external}/{port|adapter}
//   - java:       <root>/src/main/java/testkit/{driver|external}/{port|adapter}
//   - dotnet:     <root>/Testkit.{Driver|External}.{Port|Adapter}
//
// Users own subsequent edits — the scaffolder writes these defaults
// once and the migrate path back-fills only canonical keys that are
// absent. Anything beyond the canonical four set by the user is
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
		}, true
	case LangJava:
		return []string{
			"src/main/java/testkit/driver/port",
			"src/main/java/testkit/driver/adapter",
			"src/main/java/testkit/external/port",
			"src/main/java/testkit/external/adapter",
		}, true
	case LangDotnet:
		return []string{
			"Testkit.Driver.Port",
			"Testkit.Driver.Adapter",
			"Testkit.External.Port",
			"Testkit.External.Adapter",
		}, true
	default:
		return nil, false
	}
}
