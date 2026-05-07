# Tree-sitter migration: TypeScript first

## Goal

Replace the regex-based method/caller recognition in
`internal/atdd/runtime/testselect/` with tree-sitter queries, **for
TypeScript only** in this slice. Java and C# regexes remain untouched
until this slice ships, runs successfully against a real shop WRITE
cycle, and is approved.

Tree-sitter is shape-aware (not type-aware), in-process, and parses
every well-formed declaration shape correctly by construction —
arrow-property class fields, multi-line signatures, getters/setters,
decorated methods, generics with line breaks. The regex layer's blind
spots — the most recent of which surfaced as `NewOrderPage.inputSku`
failing to bridge to `MyShopUiDriver.placeOrder` — disappear as a
category once this lands.

## Why now

The regex layer worked well enough for the first pass, but the recent
production failure (`NewOrderPage.inputSku` not bridging through
`MyShopUiDriver.placeOrder`) is the second time a TS shape has slipped
through. Each new shape requires a fresh regex extension, fixture, and
review. Tree-sitter pays a one-time migration cost in exchange for
"every TS method shape just works." Long-term direction discussed and
agreed: **regex now → tree-sitter when bugs accumulate → type-aware AST
tooling (Roslyn / ts-morph / JavaSymbolSolver) only if a specific case
forces cross-file type resolution.** TypeScript has accumulated enough
to graduate to step 2.

## Binding decision: official CGo binding (`tree-sitter/go-tree-sitter`)

We use [`github.com/tree-sitter/go-tree-sitter`](https://github.com/tree-sitter/go-tree-sitter)
— the official Go binding maintained under the tree-sitter org, with
each grammar shipped as a separate Go module
(`github.com/tree-sitter/tree-sitter-typescript`,
`-tree-sitter-java`, `-tree-sitter-c-sharp`). Drops `CGO_ENABLED=0`
in `.goreleaser.yml` in exchange for a battle-tested wrapper, current
upstream grammars, and zero ongoing wrapper-maintenance burden.

**Wazero path (`malivvan/tree-sitter`) rejected after lineage check.**
Initial preference was a no-CGo Wazero wrapper to preserve
`CGO_ENABLED=0`. Inspection of `malivvan/tree-sitter` v0.0.1 (only
released version) revealed the bundled `lib/ts.wasm` does **not
include the TypeScript grammar** — the `Makefile` zig-cc invocation
omits `src/typescript/...`, the WASM linker exports no
`tree_sitter_typescript` symbol, and the Go-side `_languages`
registry confirms typescript/tsx are absent. The repo has been
dormant since 2025-01-25 (3 stars, 1 fork, single tag). No public API
exists for loading additional grammars at runtime. As-shipped, the
library cannot parse TypeScript at all.

**Roll-your-own Wazero wrapper rejected.** Feasible (~few hundred
lines around `tree-sitter.wasm` and the query API) and was the named
fallback in the original plan. Rejected on cost-of-ownership grounds:
we'd own a WASM build pipeline (zig + wasi-sdk + brotli), a
hand-rolled Wazero wrapper, and ongoing tracking of upstream
tree-sitter ABI changes — forever, for a teaching tool that parses a
handful of files per verify cycle. Maintenance burden outsized for
the use case.

**`smacker/go-tree-sitter` rejected.** 559 stars and years of
production use, but last push 2024-08-27, maintainer effectively
stepped back, grammar snapshots embedded in the repo are aging.
Author has indicated the official binding should be preferred going
forward. Picking smacker now means betting on a binding that isn't
shipping.

**Pure-Go reimplementation
([`odvcencio/gotreesitter`](https://github.com/odvcencio/gotreesitter))
rejected.** Single author with AI-generated authorship concerns
flagged on HN, separate grammar pipeline that can drift from upstream.

**Why the CGo cost is acceptable here:**

The original plan over-weighted CGo's release-pipeline cost. Concrete
audit of the project's actual setup:

1. **Cross-compile pain dissolves under `zig cc`.** The plan's CGo
   nightmare ("per-target C toolchains in CI: MinGW for Windows,
   Xcode CLT or osxcross for Darwin") describes the world before
   `zig cc`. A single zig install handles all six targets
   (linux/darwin/windows × amd64/arm64) from one Linux runner.
   Goreleaser supports this pattern out of the box. The change to
   `.goreleaser.yml` is `CGO_ENABLED=1` plus per-target `CC=zig cc
   -target ...` env entries; the change to the workflow is `apt
   install zig` — single-runner cross-compile property preserved.

2. **End-user install is unchanged.** Students run `gh extension
   install optivem/gh-optivem` and download the goreleaser binary.
   They never compile. Self-contained binaries — `tree-sitter-*`
   grammar modules embed the C source directly, no external
   `libtree-sitter.so` at runtime.

3. **Dev workstation needs a C compiler.** One-time onboarding step
   for whoever rebuilds locally (e.g., `scripts/install.sh`). Real
   but bounded: install zig once.

4. **Wrapper maturity matters more at our scale than the original
   plan thought.** The implicit assumption was "wrapper bugs don't
   bite a small CLI." But owning a hand-rolled Wazero wrapper
   forever is a much larger ongoing tax than tracking an active,
   community-maintained binding — particularly given Java and C#
   slices will follow.

**Operational guardrails:**

- **Pin both modules tightly in `go.mod`.** Chosen versions:
  `github.com/tree-sitter/go-tree-sitter v0.24.0` (matches what
  `tree-sitter-typescript` declares as its tested binding version;
  v0.25 has a redesigned query API the grammar wasn't tested
  against) and
  `github.com/tree-sitter/tree-sitter-typescript v0.23.2`. Record
  bumps in this plan or a follow-up.
- **CI gets a `zig` install step.** Add to the goreleaser
  composite action ahead of the build step.
- **Update the saved escalation-policy memory.** The memory still
  references `github.com/wasilibs/...` (stale). It should now read:
  `regex → tree-sitter via official CGo binding
  (tree-sitter/go-tree-sitter) → AST only if forced`. The
  Wazero-via-malivvan and roll-your-own-Wazero options were both
  evaluated and rejected with reasons documented in this plan;
  re-litigating them later requires those reasons to no longer
  hold.

## What's in scope (TS only)

Replace these symbols' TS code paths:

1. `MethodSignatureRE` (`layout.go:195-199`) — captures method
   declaration name + body region. New: tree-sitter query matching
   `class_declaration` → `method_definition`, `public_field_definition`
   with arrow-function value, `function_declaration`, `function_signature`,
   `method_signature`, getter/setter accessors. Capture name node,
   parameter list span, body span (handle the case of no body for
   abstract / interface signatures).
2. `CallerREFor` (`layout.go:234-236`) — finds call sites of a named
   method. New: tree-sitter query matching `call_expression` whose
   `function` node ends in `property_identifier` matching the target
   name (covers `obj.name(...)`, `this.name(...)`) plus bare
   `identifier` (covers free-function calls). Returns byte offsets that
   the existing `byteOffsetToLine` plumbing already consumes.
3. `ClassDeclRE` (`layout.go`, similar pattern) — class header parsing
   for the implements/extends list used by `classQualifyPortCandidates`.
   New: tree-sitter query for `class_heritage` → `extends_clause` and
   `implements_clause`, capturing parent type names.
4. `extractMethodRegions` (`methods.go`) — currently a regex line walk.
   New: parse the file once, walk the tree once, emit the same
   `methodRegion` structs the rest of the pipeline expects. The struct
   shape and downstream consumers stay identical so the change is local.

Out of scope in this slice:
- Java regexes (`layout.go:117`, `layout.go:175`) — unchanged.
- C# regexes (`layout.go:233`) — unchanged.
- `IsTestAnnotation`, `ChannelAnnotationRE` — these are line-shape
  matchers over annotations / decorators, not method bodies. Tree-sitter
  doesn't help meaningfully; leave alone.
- The `callersOfTest` TS-specific branch (`grep.go:79-88`) — already
  works via fixture coverage; revisit only if tree-sitter migration
  surfaces a regression.

## Migration shape: single-backend swap with parity gate

One backend at a time, per project convention. Tree-sitter replaces
regex for TypeScript in one PR. No env flag, no parallel paths, no
dual code path at any point.

1. Refactor pass: introduce per-language indexer/caller-finder
   extension points on the `layout` struct (`MethodIndexer`,
   `CallerFinder`, function-typed). Wire all three layouts (TS, Java,
   C#) to their existing regex implementations. Existing call sites in
   `methods.go`, `grep.go`, `testselect.go`, `tracer.go` route through
   the layout's function pointers instead of inlining regex calls.
   Confirm all existing tests still pass — pure refactor, no behaviour
   change. The point of this step is to localise the swap to one
   layout's wiring.
2. New file `internal/atdd/runtime/testselect/treesitter_typescript.go`
   implementing the same `MethodIndexer` / `CallerFinder` surface using
   tree-sitter queries.
3. Swap the TS layout's wiring to point at the tree-sitter functions.
   Regex closures for TS are deleted in the same change. Java and C#
   layouts keep their regex closures untouched.
4. Hard parity gate: every existing TS testselect / tracer test must
   pass under tree-sitter without modification. Failures here mean
   the tree-sitter implementation has diverged from regex semantics
   on a shape the regex *did* handle correctly — fix until parity.
5. Pre-merge validation: build the branch via `scripts/install.sh`,
   run a real shop WRITE cycle that previously hit the `inputSku`
   failure, confirm the tracer succeeds where it failed before. If a
   regression surfaces post-merge, revert the PR — no flag to clean
   up.

## Test strategy

**Hard rule: every existing testselect / tracer test must pass under
tree-sitter without modification.** That's the parity gate — it
demonstrates the swap didn't change semantics, only parsing.

Concretely:

1. Run the existing TS test set after step 3 of the migration (TS
   wiring swapped to tree-sitter). Any failure is a parity bug in the
   tree-sitter implementation; fix until green.
2. Add new fixture tests for shapes the regex couldn't parse, which
   the tree-sitter backend must handle:
   - Arrow-property class field: `inputSku = async (sku: string) => {...}`
   - Multi-line method signature with open paren on a continuation line.
   - Getter / setter (`get currentUser()`, `set timeout(ms)`).
   - Decorated method (`@traced async placeOrder(...)`) — TS
     decorators are stage-3 but shop code may use them; the
     tree-sitter query should not be tripped by them.
3. Add a benchmark `BenchmarkTreeSitterIndex_TypeScript` over a fixture
   resembling a small shop's TS tree (~50 files). Native CGo parser
   speed; on shop-sized inputs verify-cycle latency is dominated by
   test runs, not parsing — sub-second is the expectation.
   Sanity-check coverage, not a load-bearing gate.

## Out of scope

- **Java and C# tree-sitter migration.** Blocked on TS variant being
  approved after a real shop WRITE cycle. Once approved, those slices
  follow the same shape (separate plan files; reuse the `MethodIndexer`
  / `CallerFinder` extension points introduced in step 1).
- **Type-aware AST tooling** (Roslyn / ts-morph / JavaSymbolSolver) —
  v3 escape hatch, only if a named case forces cross-file type
  resolution that tree-sitter heuristics can't fake.
- **Coverage-based fallback** — separate v2 escape hatch, unchanged by
  this work.
- **The cwd bug** — sibling plan
  `20260505-220100-verify-runs-from-wrong-cwd.md`. Independent.

## Order of operations

The refactor pass (per-language `MethodIndexer` / `CallerFinder` /
`ClassExtractor` extension points on the layout struct, all three
languages wired to their existing regex implementations) has landed.
Remaining steps:

1. Add the dependencies, pinned: `github.com/tree-sitter/go-tree-sitter v0.24.0`
   and `github.com/tree-sitter/tree-sitter-typescript v0.23.2`. Run
   `go mod tidy`, confirm the module graph resolves cleanly. No
   compilation in this step — pure module-graph operation, deferred
   smoke-test validation to step 2 where the toolchain is in place.
3. Implement `treesitter_typescript.go`: parse via
   `tree-sitter/go-tree-sitter` with the TypeScript language module,
   author tree-sitter queries for method declarations / call sites /
   class heritage, emit `methodRegion` and call-site offsets
   compatible with the existing pipeline.
4. Swap the TS layout's wiring to use the tree-sitter functions;
   delete the TS regex closures in the same change. Java and C#
   layouts untouched. Run the full test suite — every existing TS
   test must pass without modification (parity gate).
5. Add new fixture tests for shapes the regex couldn't parse
   (arrow-property class field, multi-line signature, getter/setter,
   decorated method). Confirm they pass under tree-sitter.
6. Add the tree-sitter index benchmark. Sanity check.
7. Build the branch via `scripts/install.sh`. Run a real shop WRITE
   cycle that previously hit the `inputSku` failure with
   `ATDD_VERIFY_VERBOSE=1`. Confirm the tracer succeeds with one
   selection per channel, no `WARNING: tracer could not stage`.
8. Merge. Java and C# slices unblocked at this point — open separate
   plans (reuse the same binding, add `tree-sitter-java` and
   `tree-sitter-c-sharp` as additional deps).
