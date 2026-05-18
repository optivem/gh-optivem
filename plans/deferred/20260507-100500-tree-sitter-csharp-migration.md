# Tree-sitter migration: C#

## Status

**Deferred — pick up when someone needs C# tree-sitter parity for
a real shape the regex misses.** The TypeScript slice has been on
`main` since 2026-05-05 without regression, so it's no longer the
gating concern; the binding decision and extension points are
established. Java and C# slices are independent — either order works.
Move this plan back to `plans/` when picked up.

## Goal

Replace the regex-based method/caller/class recognition in
`internal/atdd/runtime/testselect/` with tree-sitter queries, **for C#
only** in this slice. The `MethodIndexer` / `CallerFinder` /
`ClassExtractor` extension points and the binding decision were both
established by the TypeScript slice; this plan only needs to add the
C# tree-sitter implementation and swap the wiring in `dotnetLayout()`.

## Why now (when picked up)

The TypeScript slice has shipped successfully. The C# regex inherits
the same blind spots as the Java one — multi-line attribute stacks,
generics with line breaks, expression-bodied members, properties with
arrow expressions — plus C#-specific shapes (records, primary
constructors, `init`-only setters, file-scoped namespaces). Once the
binding cost is paid (TS slice), C# is incremental: one new module
dependency (`tree-sitter-c-sharp`), one new file mirroring
`treesitter_typescript.go`, and a wiring swap in `dotnetLayout()`.

## Binding decision

Same as the TS slice — `github.com/tree-sitter/go-tree-sitter` plus
`github.com/tree-sitter/tree-sitter-c-sharp`, pinned to versions
compatible with the `go-tree-sitter v0.24` line. Full rationale and
the rejected alternatives (Wazero via `malivvan`, roll-your-own Wazero
wrapper, `smacker/go-tree-sitter`, `odvcencio/gotreesitter`) are in
`plans/20260505-230000-tree-sitter-typescript-migration.md`. Not
relitigated here.

## What's in scope (C# only)

Replace these symbols' C# code paths:

1. `dotnetLayout().MethodIndexer` (`layout.go:168`, currently
   `regexMethodIndexer(sig)` with the regex defined at
   `layout.go:131-136`) — switch to a tree-sitter query matching
   `method_declaration`, `constructor_declaration`,
   `local_function_statement`, plus property accessors
   (`property_declaration` → `accessor_list` → `accessor_declaration`).
   Capture the identifier node and the body span. Method shapes the
   regex misses: multi-line attribute stacks (`[Fact]\n[Trait("x","y")]
   \npublic async Task ...`), generics with line breaks,
   expression-bodied methods (`public T Foo() => expr;`), partial
   methods with separate signature/body declarations.
2. `dotnetLayout().CallerFinder` (`layout.go:169`, currently
   `regexCallerFinder` shared with Java at `layout.go:238-246`) —
   switch to a tree-sitter `invocation_expression` query matching the
   target method name. The shared regex false-positives on string
   literals (especially common in C# with `nameof()` and
   `[Description("Foo()")]`-style attributes) and comments; the
   tree-sitter version skips those by construction.
3. `dotnetLayout().ClassExtractor` (`layout.go:170`, currently
   `regexClassExtractor(classDeclRE)` with the regex at
   `layout.go:137`) — tree-sitter query for `class_declaration`,
   `interface_declaration`, `record_declaration`, `struct_declaration`,
   plus `base_list` for parent type names.

Out of scope in this slice:

- TypeScript and Java slices — TS already done; Java in
  `plans/deferred/20260507-100400-tree-sitter-java-migration.md`.
- `IsTestAnnotation` (`layout.go:171-177`) — line-shape attribute
  matcher; tree-sitter doesn't help meaningfully.
- `ChannelAnnotationRE` (`layout.go:178`) — same, line-shape over an
  attribute argument list.

## Migration shape: single-backend swap with parity gate

Same shape as the TS slice. One backend at a time. No env flag, no
parallel paths.

1. Add `github.com/tree-sitter/tree-sitter-c-sharp` to `go.mod`. Pin
   to the version compatible with `go-tree-sitter v0.24` (check
   upstream release notes for the matching pair before bumping).
2. New file `internal/atdd/runtime/testselect/treesitter_csharp.go`
   implementing the same `MethodIndexer` / `CallerFinder` /
   `ClassExtractor` surface as `treesitter_typescript.go`.
3. Swap `dotnetLayout()` to wire to the tree-sitter functions. Drop
   the `sig` and `classDeclRE` regex closures from `dotnetLayout()`
   in the same change. If the Java slice has also landed by this
   point, `regexCallerFinder` and `regexMethodIndexer` /
   `regexClassExtractor` become dead code and should be deleted in
   the same PR; if Java hasn't landed, they stay.
4. Hard parity gate: every existing C# testselect / tracer test must
   pass under tree-sitter without modification. Failures here mean
   the tree-sitter implementation has diverged from regex semantics
   on a shape the regex *did* handle correctly — fix until parity.
5. Pre-merge validation: build via `scripts/install.sh` (now requires
   zig per the TS slice), run a real shop WRITE cycle on a C# shop,
   confirm the tracer succeeds. If a regression surfaces post-merge,
   revert the PR — no flag to clean up.

## Test strategy

Hard rule: every existing C# testselect / tracer test must pass under
tree-sitter without modification.

Add new fixture tests for shapes the regex couldn't reliably parse:

1. Multi-line attribute stack on a method
   (`[Fact]\n    [Trait("x","y")]\n    public async Task Foo() ...`).
2. Generic method with line break in the type parameters
   (`public T Foo<T,\n        U>(...)` where `T` and `U` differ).
3. Expression-bodied method (`public int Foo() => 42;`).
4. Method invocation where the same identifier appears in a string
   literal, `nameof()`, or comment in the same file (regex
   false-positive case).
5. Record declaration with primary constructor
   (`public record Foo(int X, string Y);`) — the parent class shape.
6. File-scoped namespace (`namespace Foo;` rather than
   `namespace Foo { ... }`) — verify class extraction still works.

Add a benchmark `BenchmarkTreeSitterIndex_CSharp` over a fixture
resembling a small C# shop's tree (~50 files), mirroring the TS
benchmark.

## Out of scope

- **Java tree-sitter migration.** Separate plan
  (`plans/deferred/20260507-100400-tree-sitter-java-migration.md`).
- **Type-aware AST tooling** (Roslyn) — v3 escape hatch, only if a
  named case forces cross-file type resolution that tree-sitter
  heuristics can't fake.
- **Coverage-based fallback** — separate v2 escape hatch, unchanged by
  this work.

## Order of operations (when picked up)

1. Add `tree-sitter-c-sharp` dep + new file.
2. Swap `dotnetLayout()` wiring; drop the now-unused regex closures.
3. Run existing C# tests; iterate until parity.
4. Add the new shape-coverage fixture tests + benchmark.
5. Build via `scripts/install.sh`; run a real C# shop WRITE cycle.
6. Merge. If Java slice has also landed, delete dead regex helpers in
   the same PR.
