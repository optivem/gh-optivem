package projectconfig

import (
	"path"
	"strings"
)

// DefaultDbMigrationPath is the doctrinal value for `system.db-migration-path`
// — the Family A path-shaped key naming the shared canonical Flyway-style
// migration set consumed by every SUT (3 languages × 2 architectures).
//
// Used by `gh optivem init` (BuildOptivemYAML writes this verbatim into the
// scaffolded gh-optivem.yaml when architecture is set) and by `gh optivem
// config migrate` (back-fills this exactly once for pre-this-plan configs).
// The Validate Rule 22b error message also names it, so missing-key errors
// quote the literal value an operator would `migrate` to add.
const DefaultDbMigrationPath = "system/db/migrations"

// DefaultPaths returns the canonical Family B `paths:` entries for the
// given system-test language, root, and sut-namespace. The eight keys
// match the doctrine referenced by the inline `read:` / `write:` scope
// on writing-agent MID nodes in
// `internal/atdd/runtime/statemachine/process-flow.yaml`: system-driver-port,
// system-driver-adapter, external-system-driver-port,
// external-system-driver-adapter, at-test, dsl-port, dsl-core, ct-test.
//
// Returns nil when testLang is unsupported or systemTestRoot is empty —
// the scaffolder leaves `paths:` absent for partial configs (no
// architecture chosen yet) and `Validate` accepts that shape.
//
// Per-SSoT (plan 20260518-1530 item 3), the returned values are fully
// resolved: testkit keys (system-driver-*, external-system-driver-*, dsl-*) take
// `sutNamespace` as a trailing directory segment. at-test and ct-test
// are sut-namespace-free at the DefaultPaths trailing-append layer —
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
// See `internal/projectconfig/path-keys.md` for the
// canonical-key vocabulary doc consumed by `gh-optivem.yaml paths:` and
// by the inline per-phase scope on writing-agent MID nodes in
// `internal/atdd/runtime/statemachine/process-flow.yaml`.
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
		// directory segment when present. at-test and ct-test are
		// sut-namespace-free at this layer — Java's stems already
		// incorporate sutNamespace as a middle (package) segment via
		// pathStems; TS and dotnet stems don't structure tests by
		// namespace.
		if key != "at-test" && key != "ct-test" && sutNamespace != "" {
			stem = path.Join(stem, sutNamespace)
		}
		out[key] = path.Join(systemTestRoot, stem)
	}
	return out
}

// DefaultSystemDriverAdapterChannels returns the scaffold-authoritative
// per-channel members for system-test.system-driver-adapter-channels: — one
// entry per channel, each the whole-layer system-driver-adapter path with the
// channel appended as a subfolder cased per language (TS/Java lowercase
// `.../api`, .NET PascalCase `.../Api`), matching the shop template's adapter
// channel split (driver/adapter/api, Driver.Adapter/Api).
//
// The members are DERIVED from the system-driver-adapter value (not pinned as
// independent literals) so they stay consistent with whatever that root
// resolves to: a future reconcile of the adapter root
// (reconcile-defaultpaths) moves the members with it rather than leaving them
// stranded. Only the per-language *casing* — the part that genuinely cannot be
// a single lowercase join — is language-specific here.
//
// Returns nil for an unsupported language, an empty systemTestRoot (no adapter
// root to anchor on), or an empty channel set — mirroring DefaultPaths /
// DefaultChannels, the scaffolder omits the block for partial configs.
func DefaultSystemDriverAdapterChannels(testLang, systemTestRoot, sutNamespace string, channels []string) map[string]string {
	if len(channels) == 0 {
		return nil
	}
	adapter := DefaultPaths(testLang, systemTestRoot, sutNamespace)["system-driver-adapter"]
	if adapter == "" {
		return nil
	}
	out := make(map[string]string, len(channels))
	for _, ch := range channels {
		seg, ok := channelPathSegment(testLang, ch)
		if !ok {
			return nil
		}
		out[ch] = path.Join(adapter, seg)
	}
	return out
}

// channelPathSegment returns the per-language directory segment for a channel
// under the system-driver-adapter root. TS and Java use the lowercase channel
// token verbatim (driver/adapter/api); .NET PascalCases it to match the
// project's subfolder casing (Driver.Adapter/Api). Returns ok=false for an
// unsupported language, mirroring pathStems.
func channelPathSegment(testLang, channel string) (string, bool) {
	switch testLang {
	case LangTypescript, LangJava:
		return channel, true
	case LangDotnet:
		return titleFirst(channel), true
	default:
		return "", false
	}
}

// titleFirst upper-cases the first byte of s and lower-cases the rest
// ("api" → "Api", "ui" → "Ui"). The channel tokens are lowercase canonical
// (validateChannels enforces it), so this yields the .NET subfolder casing.
func titleFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}

// CanonicalPathKeys is the Family B key set in fixed order so DefaultPaths,
// the migrate back-fill, and any tests over either can iterate in the
// same order.
//
// See `internal/projectconfig/path-keys.md` for the
// vocabulary doc and
// `internal/atdd/runtime/statemachine/process-flow.yaml` (the inline
// `read:` / `write:` lists on writing-agent MID nodes) for the per-phase
// scope assignment that consumes these keys.
func CanonicalPathKeys() []string {
	return []string{
		"system-driver-port",
		"system-driver-adapter",
		"external-system-driver-port",
		"external-system-driver-adapter",
		"at-test",
		"dsl-port",
		"dsl-core",
		"ct-test",
	}
}

// pathStems returns the per-language path tails (in CanonicalPathKeys
// order) that DefaultPaths joins under systemTestRoot. Returns ok=false
// for unsupported languages so the caller can omit `paths:` rather than
// write a partial map.
//
// The sutNamespace parameter is consumed by the Java branch to
// interpolate the `<sutNamespace>` package segment in at-test and
// ct-test stems (per plan 20260518-1742 items 3a/3b — Java structures
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
