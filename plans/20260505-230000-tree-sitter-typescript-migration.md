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

## Binding decision: WASM via Wazero (not CGo)

Two viable Go bindings:

| Option | Build cost | Runtime cost | Distribution |
|---|---|---|---|
| `github.com/tree-sitter/go-tree-sitter` (CGo) | C toolchain on every build host (MinGW/MSVC on Windows, gcc on Linux); cross-compilation needs `CC=...` per target | Native parser speed | Static binary still possible but build complexity grows |
| `github.com/wasilibs/go-tree-sitter` (WASM via Wazero) | Pure Go; `go build` on stock Windows or Linux | ~3–5× slower per parse than native | Single static binary, no toolchain assumptions |

**Recommendation: WASM.** The "single-binary, stock-Go build" property
was load-bearing in the Tier 1 vs Tier 2 decision — going CGo here
quietly forfeits the main reason tree-sitter beat per-language AST
helpers. Parser speed is not the bottleneck: the bridge runs once per
verify cycle on a small file set (low hundreds of TS files in a typical
shop), and Wazero parses on the order of MB/s. Even at 3–5× slower
than native, this is sub-second work.

**Verification before committing:** confirm `wasilibs/go-tree-sitter`
ships a TypeScript grammar (or that a compatible TS grammar binding
exists in the wasilibs ecosystem) before sinking implementation effort.
If the TS grammar isn't available there, fall back to the CGo binding
and accept the build complexity — surface the trade-off and ask before
flipping.

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
   resembling a small shop's TS tree (~50 files). Confirms the WASM
   backend stays sub-second; if it doesn't, flag before proceeding.

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

1. Verify `wasilibs/go-tree-sitter` (or compatible) ships a TypeScript
   grammar. If not, surface CGo trade-off and pause for decision.
2. Refactor: add `MethodIndexer` / `CallerFinder` function-pointer
   fields to the layout struct, wire all three layouts (TS, Java, C#)
   to their existing regex implementations. Confirm all existing tests
   still pass — pure refactor, no behaviour change.
3. Implement `treesitter_typescript.go`: parse, query, emit
   `methodRegion` and call-site offsets compatible with the existing
   pipeline.
4. Swap the TS layout's wiring to use the tree-sitter functions; delete
   the TS regex closures in the same change. Java and C# layouts
   untouched. Run the full test suite — every existing TS test must
   pass without modification (parity gate).
5. Add new fixture tests for shapes the regex couldn't parse
   (arrow-property class field, multi-line signature, getter/setter,
   decorated method). Confirm they pass under tree-sitter.
6. Add the WASM backend benchmark. Confirm sub-second on a 50-file
   fixture.
7. Build the branch via `scripts/install.sh`. Run a real shop WRITE
   cycle that previously hit the `inputSku` failure with
   `ATDD_VERIFY_VERBOSE=1`. Confirm the tracer succeeds with one
   selection per channel, no `WARNING: tracer could not stage`.
8. Merge. Java and C# slices unblocked at this point — open separate
   plans.
