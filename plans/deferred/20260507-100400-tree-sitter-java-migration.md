# Tree-sitter migration: Java

## Status

**Deferred — pick up only after the TypeScript slice has been verified
in a real shop WRITE cycle and approved.** TS verification is tracked
in `plans/20260505-230000-tree-sitter-typescript-migration.md` (steps 1
and 2 of its `## Order of operations`). Once that gate passes, move
this plan back to `plans/`.

## Goal

Replace the regex-based method/caller/class recognition in
`internal/atdd/runtime/testselect/` with tree-sitter queries, **for Java
only** in this slice. The `MethodIndexer` / `CallerFinder` /
`ClassExtractor` extension points and the binding decision were both
established by the TypeScript slice; this plan only needs to add the
Java tree-sitter implementation and swap the wiring in `javaLayout()`.

## Why now (when picked up)

The TypeScript slice has shipped successfully. The Java regex carries
the same class of blind spots the TS one did — multi-line annotation
stacks, generics with line breaks, varargs, throws clauses split across
lines. Each new shape the regex misses today would otherwise force
another fixture-and-extend cycle. Once the binding cost is paid (TS
slice), Java is incremental: one new module dependency
(`tree-sitter-java`), one new file mirroring `treesitter_typescript.go`,
and a wiring swap in `javaLayout()`.

## Binding decision

Same as the TS slice — `github.com/tree-sitter/go-tree-sitter` plus
`github.com/tree-sitter/tree-sitter-java`, pinned to versions
compatible with the `go-tree-sitter v0.24` line. Full rationale and
the rejected alternatives (Wazero via `malivvan`, roll-your-own Wazero
wrapper, `smacker/go-tree-sitter`, `odvcencio/gotreesitter`) are in
`plans/20260505-230000-tree-sitter-typescript-migration.md`. Not
relitigated here.

## What's in scope (Java only)

Replace these symbols' Java code paths:

1. `javaLayout().MethodIndexer` (`layout.go:114`, currently
   `regexMethodIndexer(sig)` with the regex defined at
   `layout.go:78-83`) — switch to a tree-sitter query matching
   `method_declaration` and `constructor_declaration`. Capture the
   identifier node and the body span. Java method shapes the regex
   misses or risks mishandling: multi-line annotation stacks
   (`@Override\n    @Nullable\n    public ...`), generics with line
   breaks, varargs `String... args`, throws clauses split across
   lines.
2. `javaLayout().CallerFinder` (`layout.go:115`, currently
   `regexCallerFinder` shared with C# at `layout.go:238-246`) — switch
   to a tree-sitter `method_invocation` query matching the `name:
   identifier` whose text equals the target method name. The shared
   regex false-positives on string literals and comments containing
   `methodName(` patterns; the tree-sitter version skips those
   nodes by construction.
3. `javaLayout().ClassExtractor` (`layout.go:116`, currently
   `regexClassExtractor(classDeclRE)` with the regex at `layout.go:84`)
   — tree-sitter query for `class_declaration`,
   `interface_declaration`, and `record_declaration`, plus
   `superclass` and `super_interfaces` clauses for parents.

Out of scope in this slice:

- TypeScript and C# slices — TS already done; C# in
  `plans/deferred/20260507-100500-tree-sitter-csharp-migration.md`.
- `IsTestAnnotation` (`layout.go:117-123`) — line-shape annotation
  matcher; tree-sitter doesn't help meaningfully.
- `ChannelAnnotationRE` (`layout.go:124`) — same, line-shape over an
  annotation argument list.

## Migration shape: single-backend swap with parity gate

Same shape as the TS slice. One backend at a time. No env flag, no
parallel paths.

1. Add `github.com/tree-sitter/tree-sitter-java` to `go.mod`. Pin to
   the version compatible with `go-tree-sitter v0.24` (check upstream
   release notes for the matching pair before bumping).
2. New file `internal/atdd/runtime/testselect/treesitter_java.go`
   implementing the same `MethodIndexer` / `CallerFinder` /
   `ClassExtractor` surface as `treesitter_typescript.go`.
3. Swap `javaLayout()` to wire to the tree-sitter functions. Drop the
   `sig` and `classDeclRE` regex closures from `javaLayout()` in the
   same change. Keep `regexCallerFinder` itself — it's still used by
   `dotnetLayout()` until the C# slice lands.
4. Hard parity gate: every existing Java testselect / tracer test
   must pass under tree-sitter without modification. Failures here
   mean the tree-sitter implementation has diverged from regex
   semantics on a shape the regex *did* handle correctly — fix until
   parity.
5. Pre-merge validation: build via `scripts/install.sh` (now requires
   zig per the TS slice), run a real shop WRITE cycle on a Java shop,
   confirm the tracer succeeds. If a regression surfaces post-merge,
   revert the PR — no flag to clean up.

## Test strategy

Hard rule: every existing Java testselect / tracer test must pass
under tree-sitter without modification.

Add new fixture tests for shapes the regex couldn't reliably parse:

1. Multi-line annotation stack
   (`@Override\n    @Nullable\n    public T foo()`).
2. Generic method with line break in the type parameters
   (`public <T extends X,\n        U> Map<T,U> foo()`).
3. Varargs (`public void log(String... args)`).
4. Method invocation where the same identifier appears in a string
   literal or comment in the same file (regex false-positive case).
5. Throws clause split across lines
   (`public void foo()\n        throws IOException, SQLException`).

Add a benchmark `BenchmarkTreeSitterIndex_Java` over a fixture
resembling a small Java shop's tree (~50 files), mirroring the
TS benchmark.

## Out of scope

- **C# tree-sitter migration.** Separate plan
  (`plans/deferred/20260507-100500-tree-sitter-csharp-migration.md`).
- **Type-aware AST tooling** (JavaSymbolSolver / JavaParser) — v3
  escape hatch, only if a named case forces cross-file type resolution
  that tree-sitter heuristics can't fake.
- **Coverage-based fallback** — separate v2 escape hatch, unchanged by
  this work.

## Order of operations (when picked up)

1. Add `tree-sitter-java` dep + new file.
2. Swap `javaLayout()` wiring; drop the now-unused regex closures.
3. Run existing Java tests; iterate until parity.
4. Add the new shape-coverage fixture tests + benchmark.
5. Build via `scripts/install.sh`; run a real Java shop WRITE cycle.
6. Merge.
