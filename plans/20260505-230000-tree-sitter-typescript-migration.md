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

## Binding decision: WASM via Wazero (`malivvan/tree-sitter`)

We use [`github.com/malivvan/tree-sitter`](https://github.com/malivvan/tree-sitter)
— it wraps a WASM build of upstream
[tree-sitter](https://github.com/tree-sitter/tree-sitter) and runs it
via [Wazero](https://wazero.io/), a zero-dependency, pure-Go
WebAssembly runtime. No CGo, no C toolchain. Preserves the project's
`CGO_ENABLED=0` build property in `.goreleaser.yml`.

The parser core is the **same upstream C tree-sitter source**, just
compiled to WASM (via wasi-sdk in upstream's modern build pipeline)
instead of compiled to native via CGo. Parser correctness is
equivalent to `smacker/go-tree-sitter`'s. The maturity gap is in the
Go-side wrapper code (memory management, query API, lifetimes), not
in the parser.

**Why WASM-via-Wazero over CGo + `smacker/go-tree-sitter`:**

The smacker route is more mature at the Go-wrapper layer — years of
production use, batteries-included grammars. Four factors dominate
the trade-off at our scale:

1. **CGo cost is permanent; wrapper risk is bounded.** Dropping
   `CGO_ENABLED=0` is an ongoing tax on every release: per-target C
   toolchains in CI (MinGW for Windows, Xcode CLT or osxcross for
   Darwin), more brittle goreleaser config, `scripts/install.sh`
   rework, and friction for every future OS/arch addition. The
   malivvan wrapper risk, if realized, costs a fork or a swap to a
   different binding — bounded, recoverable, and localised to one
   file (`treesitter_typescript.go`).

2. **Our distribution model is the dominant constraint.** The tool
   ships as `gh extension install` across six OS/arch targets
   (linux/darwin/windows × amd64/arm64) cross-compiled from a single
   Linux runner. That is exactly the scenario CGo punishes most. A
   teaching tool whose install story breaks on one platform is worse
   than a teaching tool whose parser has an edge-case bug.

3. **The wrapper-maturity gap is smaller than it looks at our
   scale.** Educational shop code is small, simple, short-lived. The
   bug classes an immature Go wrapper produces (memory leaks over
   long sessions, goroutine races at high concurrency, API
   papercuts) do not get exercised by a CLI that parses a handful of
   files per verify cycle and exits. The bug classes that *do*
   surface (correctness on weird shapes) live in the parser, and the
   parser is upstream-equivalent.

4. **Reversibility favors WASM.** If `malivvan/tree-sitter` rots,
   the escape hatches are: fork it (it is small), swap to a
   roll-your-own thin Wazero wrapper around `tree-sitter.wasm` (~few
   hundred lines, feasible), or revisit CGo *then* with concrete
   evidence. If we go CGo now and regret the CI cost, undoing it is
   harder — CI assumptions accrete around per-OS toolchains.

**Pure-Go reimplementation rejected.**
[`odvcencio/gotreesitter`](https://github.com/odvcencio/gotreesitter)
(GLR reimplementation in Go, embedded grammars) was considered. Two
problems: single author with AI-generated authorship concerns flagged
on HN, and a separate grammar pipeline that can drift from upstream.
The malivvan path keeps the canonical upstream parser; gotreesitter
forks it.

**Roll-our-own thin Wazero wrapper.** Considered as a fallback if
malivvan stalls. Feasible (~few hundred lines around
`tree-sitter.wasm` and the query API), but writing infrastructure
that should arguably be a community library, for a tool of this
scope, is hard to justify *now*.

**Operational guardrails:**

- **Pin `malivvan/tree-sitter` tightly** in `go.mod`. Pre-release
  software with low star count deserves a pinned version and an
  explicit upgrade ritual, not `@latest`. Record the chosen version
  in this plan when the dependency is added.
- **Verify the bundled WASM blob's upstream lineage** at integration
  time — confirm malivvan's WASM is built from a recent upstream
  tree-sitter release, not a long-stale fork. If the lag is large,
  the roll-our-own option becomes more attractive.
- **Fix the stale package reference in the saved escalation-policy
  memory.** The memory currently says `github.com/wasilibs/...`; the
  actual library is `github.com/malivvan/tree-sitter`. (`wasilibs`
  ships related Wazero helpers but no tree-sitter wrapper.)

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
   resembling a small shop's TS tree (~50 files). WASM via Wazero is
   roughly 3–5× slower per parse than native CGo, but on shop-sized
   inputs verify-cycle latency is dominated by test runs, not parsing
   — sub-second is the expectation. Sanity-check coverage, not a
   load-bearing gate.

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

1. Add `github.com/malivvan/tree-sitter` and its TypeScript grammar
   subpackage as dependencies. Pin to a specific version in `go.mod`
   (no `@latest`); record the chosen version in this plan. **No CI
   changes needed** — `CGO_ENABLED=0` stays, single-runner
   cross-compile to all six targets stays, `scripts/install.sh`
   stays.
2. Verify upstream lineage of the bundled WASM blob: inspect
   `malivvan/tree-sitter`'s build provenance and confirm the WASM is
   built from a recent upstream tree-sitter release, not a long-stale
   fork. If the lag is large, escalate to the roll-your-own Wazero
   wrapper option before sinking implementation work into the
   malivvan path.
3. Implement `treesitter_typescript.go`: parse via malivvan, query
   for method declarations and call sites, emit `methodRegion` and
   call-site offsets compatible with the existing pipeline.
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
   plans.
