# Tracer can't bridge Page Object helpers (e.g. `NewOrderPage.inputSku`)

## Symptom

Running an `AT - RED - SYSTEM DRIVER - WRITE` cycle that edits a Page Object
method:

```
system-test/typescript/src/testkit/driver/adapter/myShop/ui/client/pages/NewOrderPage.ts — inputSku
```

Choosing `[t]racer` from the verify prompt produces:

```
WARNING: tracer could not stage 1 adapter method(s) — running full suite for safety:
  - system-test/typescript/.../pages/NewOrderPage.ts::inputSku
```

The tracer falls back to the affected-set's full suites (which then hits a
separate cwd bug — see the sibling plan `20260505-220100-verify-runs-from-wrong-cwd.md`;
that is a distinct problem and out of scope here).

The bridge step is `resolveAdapterToPortBackedMethods` in
`internal/atdd/runtime/testselect/grep.go:111`. It is called from both
`SelectWithDeps` (`testselect.go:214`) and `SelectTracerWithDeps`
(`tracer.go:125`). For Page Object helpers like `inputSku`, the helper has no
matching port method — by design — and the bridge is supposed to walk
adapter callers transitively until it hits an adapter method that does
fulfil a port (e.g. `placeOrder`). For `inputSku`, the bridge returns
empty, so the change is unmapped.

## Why the affected-set didn't notice this

`Select` produced a non-empty selection list because *other* changed
methods in the same WRITE landed on port-backed adapters (the WRITE log
shows three changed files: a Page Object plus an app page plus a system
page). Those covered the suites; the tracer is per-method-per-channel, so
the one method that doesn't bridge surfaces as unmapped where the
affected-set silently swallows it.

## Diagnosis (do this first)

Rerun the failing cycle with `ATDD_VERIFY_VERBOSE=1` — the verify node
prints the diagnostics list, including which adapter-caller chain (if any)
the bridge attempted. Without that output it is guesswork; the plan below
keeps the fix minimal once one of the hypotheses is confirmed.

Three hypotheses, ranked by prior:

### H1. The caller's file isn't in `adapterFiles`

`resolveAdapterToPortBackedMethods` only searches `adapterFiles`, which
comes from `deps.Walk(repoRoot, []string{lay.PortRoot(repoRoot)},
lay.SourceExts)` filtered by `lay.AdapterMatch`. For TypeScript,
`AdapterMatch` filters on `testkit/driver/adapter/`.

If the method that calls `inputSku` lives in
`system-test/typescript/src/testkit/driver/adapter/myShop/ui/client/pages/NewOrderPage.ts`
itself (Page Object internal call) **or** in another file under
`testkit/driver/adapter/`, the file is in the set. So this hypothesis
fails unless the caller lives somewhere unexpected (e.g. a shared `pages/`
helper that isn't under `adapter/`). Confirm by `grep -rn 'inputSku\b'
system-test/typescript`.

### H2. The caller exists but the TS method-signature regex doesn't recognise its enclosing method

`resolveAdapterToPortBackedMethods` resolves the callers' enclosing method
via the per-language `MethodSignatureRE`. TypeScript class method bodies
are matched by the layout's regex (see `layout.go`). Common shapes that
the current regex may miss:

- **Arrow-property methods**: `inputSku = async (sku: string) => { ... }`
  declared as a class field. Not the body shape the layout's
  `MethodSignatureRE` was written for.
- **Public-async methods spanning multiple lines** before the open paren:
  ```
  async placeOrder(
    sku: string,
    qty: number,
  ): Promise<void> {
  ```
  The current TS sig regex is anchored to a single line ending in `(` —
  multi-line signatures will be missed and the file's `methodIndex`
  entries for that method will be absent, so the call site at
  `inputSku(...)` lands "outside any method region" and is dropped.
- **Static methods**, **getters/setters** (`get foo()`).

If the caller's enclosing method isn't in the index, the bridge sees zero
adapter callers and gives up. Verify by dumping `adapterMethods.byFile[<page-object-file>]`
— is `placeOrder` (or whichever method actually calls `inputSku`) listed?

### H3. The bridge stops at a method that itself is not port-backed and has no port-backed ancestor on its branch

The Page Object pattern is `Test → DSL → Adapter (driver method on
e.g. NewOrderDriverAdapter) → Page Object helper`. The driver-side
adapter (`NewOrderDriverAdapter.placeOrder`) implements a port
(`NewOrderDriverPort.placeOrder`). If that adapter exists and the regex
*does* find it as a caller, the bridge succeeds. If the chain instead
goes through an intermediate that does not implement a port (e.g. a
private helper between `placeOrder` and `inputSku`), the bridge returns
nothing and the plan is to make the bridge keep walking — but it
already does (BFS in `resolveAdapterToPortBackedMethods`). So this
hypothesis is only reachable if the BFS is misconfigured (e.g. only
goes one level deep). Inspect the BFS loop; it looks correct on a
read-through, but worth confirming.

## Fixes per hypothesis

### If H1: extend the adapter walk

Either widen `lay.AdapterMatch` to cover the actual caller's path, or —
preferred — keep `AdapterMatch` strict and instead make the helper file
itself part of the walk. The current setup already walks every TypeScript
file under `PortRoot()` filtered by `AdapterMatch`, so widening the
matcher (e.g. to also accept `testkit/driver/adapter/**/pages/`, which it
already does via the `testkit/driver/adapter/` substring) should be a
one-line change. Verify with a fixture test.

### If H2: extend the TS method-signature regex

Add two recognisers:

1. **Arrow-property methods**: `^\s*(?:public\s+|private\s+|protected\s+|readonly\s+|static\s+)*(\w+)\s*=\s*(?:async\s*)?\(`
   captures group 1 = method name. Treat the body as ending at the
   matching `}` of the arrow function — the existing depth-tracker for
   `{` / `}` handles this once the start line is known.
2. **Multi-line method signatures**: relax the anchor so the open paren
   may be on a continuation line. Replace `(\w+)\s*\(` with
   `(\w+)\s*(?:\([^)]*\))?\s*\(?` and post-validate by scanning forward
   for the open paren / open brace pair. Less elegant than a real parser
   but matches the project's "regex layer is good enough" stance from
   the original plan (see `20260504-130000-...md`).

Add fixture coverage in `internal/atdd/runtime/testselect/tracer_test.go`
that mirrors the failing case: a TS Page Object with `inputSku = async
(sku) => { ... }` (or whichever shape H2 confirmed), called from a
sibling adapter that satisfies a port. Assert tracer succeeds, no
unmapped.

### If H3: tighten the BFS

Confirm by adding a deliberately-three-level-deep fixture: helper →
intermediate → port-backed driver method. If the bridge fails on it,
patch the BFS frontier handling. Looks correct on inspection, so this
branch is unlikely.

## Out of scope

- **Coverage-based fallback** — if the regex bridge keeps producing
  unmapped on real shop edits, the v2 escape hatch is to instrument tests
  with `c8` / `nyc` and use coverage-of-changed-lines as the fallback.
  Not in v1; the warning + full-suite path is acceptable for now.
- **Java / .NET equivalents** — the same Page Object pattern in Java
  uses regular methods, not arrow properties; the regex there is
  unaffected. Worth running the same diagnosis if a Java WRITE fails the
  bridge in the future.
- **The cwd bug** — the full-suite fallback fails for an unrelated
  reason. Fixing that is the sibling plan
  `20260505-220100-verify-runs-from-wrong-cwd.md`. The two are
  independent: H2 is in `testselect/`, the cwd bug is in
  `actions/bindings.go`.

## Order of operations

1. Reproduce with `ATDD_VERIFY_VERBOSE=1` and capture the diagnostics.
2. `grep -rn 'inputSku\b' system-test/typescript` to confirm callers.
3. From those two pieces decide H1 / H2 / H3. Almost certainly H2.
4. Add a failing fixture test that mirrors the production shape.
5. Patch `MethodSignatureRE` (or `AdapterMatch`, or the BFS) to make the
   fixture pass.
6. Rerun the original WRITE cycle; observe `[t]racer` succeeds with one
   selection per channel.
