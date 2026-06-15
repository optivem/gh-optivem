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
// given system-test language, root, and Java source package. The nine keys
// match the doctrine referenced by the inline `read:` / `write:` scope
// on writing-agent MID nodes in
// `internal/atdd/process/process-flow.yaml`: system-driver-port,
// system-driver-adapter, external-system-driver-port,
// external-system-driver-adapter, at-test, dsl-port, dsl-core, ct-test,
// system-driver-adapter-shared.
//
// Returns nil when testLang is unsupported or systemTestRoot is empty —
// the scaffolder leaves `paths:` absent for partial configs (no
// architecture chosen yet) and `Validate` accepts that shape.
//
// The values reproduce the shop template's checked-in
// `gh-optivem-<arch>-<lang>.yaml` `paths:` block exactly — the structural
// SSoT verified on disk (plan 20260526-1430). There is NO sut-namespace
// trailing segment: TypeScript and dotnet carry no namespace anywhere, and
// Java carries its source package (`com/<org>/<sut>`) as a MIDDLE segment, not
// a trailing leaf. The `javaPackage` argument (e.g. `com/mycompany/myshop`,
// resolved at the call site from the owner + system-name casings) is consumed
// only by the Java branch; TypeScript and dotnet ignore it. An empty
// `javaPackage` collapses the Java package segment (partial/legacy shape).
//
// Externals nest UNDER the driver layer (`driver/{port,adapter}/external`,
// `Driver.{Port,Adapter}/External`), not as a sibling `external/*` dir.
//
// Per-language path stems mirror the post-scaffold tree (the
// `system-test/{lang}/` subdir is flattened by `copySystemTests`) and are
// pinned against the shop template. `latest` / `Latest` is doctrine literal,
// not project-customizable:
//
//   - typescript: <root>/src/testkit/{driver|dsl}/{port|adapter|core}
//     [+ driver/{port,adapter}/external], <root>/tests/latest/{acceptance,contract}
//   - java:       <root>/src/main/java/<javaPackage>/testkit/{driver|dsl}/…
//     [+ driver/{port,adapter}/external],
//     <root>/src/test/java/<javaPackage>/systemtest/latest/{acceptance,contract}
//   - dotnet:     <root>/{Driver|Dsl}.{Port|Adapter|Core} [+ Driver.{Port,Adapter}/External],
//     <root>/SystemTests/Latest/{AcceptanceTests,ExternalSystemContractTests}
//
// The dotnet contract-test stem (`ExternalSystemContractTests`) is
// asymmetric vs the acceptance/etc. `<TestType>Tests` naming. It is
// pinned as a literal here against the shop template, not derived by
// rule.
//
// Users own subsequent edits — the scaffolder writes these defaults
// once and the migrate path back-fills only canonical keys that are
// absent. Anything beyond the canonical nine set by the user is
// preserved across migrations.
//
// See `internal/projectconfig/path-keys.md` for the
// canonical-key vocabulary doc consumed by `gh-optivem.yaml paths:` and
// by the inline per-phase scope on writing-agent MID nodes in
// `internal/atdd/process/process-flow.yaml`.
func DefaultPaths(testLang, systemTestRoot, javaPackage string) map[string]string {
	if systemTestRoot == "" {
		return nil
	}
	keys := CanonicalPathKeys()
	stems, ok := pathStems(testLang, javaPackage)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(keys))
	for i, key := range keys {
		out[key] = path.Join(systemTestRoot, stems[i])
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
func DefaultSystemDriverAdapterChannels(testLang, systemTestRoot, javaPackage string, channels []string) map[string]string {
	if len(channels) == 0 {
		return nil
	}
	adapter := DefaultPaths(testLang, systemTestRoot, javaPackage)["system-driver-adapter"]
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
// `internal/atdd/process/process-flow.yaml` (the inline
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
		"system-driver-adapter-shared",
	}
}

// pathStems returns the per-language path tails (in CanonicalPathKeys
// order) that DefaultPaths joins under systemTestRoot. Returns ok=false
// for unsupported languages so the caller can omit `paths:` rather than
// write a partial map.
//
// The stems reproduce the shop template's checked-in `paths:` block (verified
// on disk, plan 20260526-1430). Externals nest under the driver layer
// (`driver/{port,adapter}/external`). The javaPackage argument is interpolated
// as a middle segment in every Java stem — under `src/main/java/` for testkit
// keys and under `src/test/java/.../systemtest/` for at-test / ct-test; an
// empty javaPackage collapses that segment (partial/legacy shape). TS and
// dotnet structure nothing by namespace and ignore javaPackage.
func pathStems(testLang, javaPackage string) ([]string, bool) {
	switch testLang {
	case LangTypescript:
		return []string{
			"src/testkit/driver/port",
			"src/testkit/driver/adapter",
			"src/testkit/driver/port/external",
			"src/testkit/driver/adapter/external",
			"tests/latest/acceptance",
			"src/testkit/dsl/port",
			"src/testkit/dsl/core",
			"tests/latest/contract",
			"src/testkit/driver/adapter/shared",
		}, true
	case LangJava:
		main := path.Join("src/main/java", javaPackage, "testkit")
		test := path.Join("src/test/java", javaPackage, "systemtest")
		return []string{
			path.Join(main, "driver/port"),
			path.Join(main, "driver/adapter"),
			path.Join(main, "driver/port/external"),
			path.Join(main, "driver/adapter/external"),
			path.Join(test, "latest/acceptance"),
			path.Join(main, "dsl/port"),
			path.Join(main, "dsl/core"),
			path.Join(test, "latest/contract"),
			path.Join(main, "driver/adapter/shared"),
		}, true
	case LangDotnet:
		return []string{
			"Driver.Port",
			"Driver.Adapter",
			"Driver.Port/External",
			"Driver.Adapter/External",
			"SystemTests/Latest/AcceptanceTests",
			"Dsl.Port",
			"Dsl.Core",
			"SystemTests/Latest/ExternalSystemContractTests",
			"Driver.Adapter/Shared",
		}, true
	default:
		return nil, false
	}
}
