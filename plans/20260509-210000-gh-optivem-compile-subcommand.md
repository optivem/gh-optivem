# `gh optivem compile system` / `compile system-tests` — replace `compile-all.sh` shell-out

## Motivation

The structural cycle's `compile_in_scope` action shells out to
`./compile-all.sh` in the scaffolded academy repo
(`internal/atdd/runtime/actions/bindings.go:703-714`). That script
compiles **every** language stack across **every** scope
(monolith/multitier/system-test × dotnet/java/typescript), regardless
of which stack the developer actually picked.

Today's rehearsal failure (`rehearsal-20260509-203144`) is the symptom:
TypeScript projects fail with

```
This is not the tsc command you are looking for
```

because `tsc` is invoked without a prior `npm install`. The dev had
chosen dotnet — those TypeScript stacks are out of scope and shouldn't
have been compiled at all.

The structural-cycle comment already calls this out as v2 work:

> `internal/atdd/runtime/actions/bindings.go:699-702`
> `// v1 calls compile-all.sh from the repo root unconditionally;`
> `// per-language scoping is a v2 nicety (would require knowing the`
> `// in-scope languages, which the structural cycle does not yet expose).`

The in-scope language **is** known — it's in `gh-optivem.yaml`. We just
haven't wired compile to read it.

## Why `gh-optivem.yaml` is enough

`internal/projectconfig/config.go:67-109` already encodes everything
needed:

- **Monolith**: `System.Path` + `System.Lang`
- **Multitier**: `System.Backend.{Path,Lang}` + `System.Frontend.{Path,Lang}`
- **System tests**: `SystemTest.{Path,Lang}`
- `Lang ∈ { java, dotnet, typescript }` (lines 54-58)

No schema extension required for v1.

`system.json` / `tests.json` are deliberately **not** the right config:
they describe runtime orchestration (docker compose, per-suite shell
commands) and don't carry a language field — language is implicit in
each suite's `Command` string. `compile` would be the first runner-side
command to read `gh-optivem.yaml`.

## Naming

`gh optivem build system` is already taken — it means
`docker compose build` (`runner_commands.go:99-110`). So:

- New verb: `compile` (source-level build)
- Existing verb: `build` (docker image build)

These are distinct enough to coexist; we just must not overload `build`.

## Plan

### Step 4 — Multi-config fan-out in the shop template ✅ Done (2026-05-11)

`compile-all.sh` is a **maintainer** tool, not a student-facing artifact
(scaffolding never copies it). Its real job is "compile every variant of
the template at once" — a cross-variant preflight before a shop commit.

A 2-line `gh optivem compile` wrapper would have lost that, because
`gh optivem compile` only compiles the one variant in `gh-optivem.yaml`.
Instead we encoded the variant matrix as six named YAMLs in shop:

- `gh-optivem-monolith-{dotnet,java,typescript}.yaml`
- `gh-optivem-multitier-{dotnet,java,typescript}.yaml`

`compile-all.sh` now loops over `gh-optivem-*.yaml`, invoking
`gh optivem compile -c <yaml>` per variant. Adding a variant = drop a
YAML; no script edits. The unparameterized `gh-optivem.yaml` was
**deleted** to prevent drift — every invocation in shop must name a
variant explicitly. Same pattern can extend to `test-all.sh` later.

### Step 5 — `compile-targeted.sh` (deferred)

The state machine has a parallel `compileTargeted` action
(`internal/atdd/runtime/actions/bindings.go:743-759`) that shells out
to `./compile-targeted.sh <scope>`. It's currently **unwired** — no
YAML node calls it (see comment at lines 717-723). Out of scope for
this PR. Revisit when the AT/CT creative/mechanical split refactor
(`plans/20260505-230100-at-ct-cycle-creative-mechanical-split.md`)
needs targeted compile.

## Open questions

*All resolved 2026-05-09:*

- ~~Per-language compile commands~~ — **dotnet**: `dotnet build`,
  **java**: `./gradlew compileJava`, **typescript**:
  `npm ci && npx tsc --noEmit`. Codified in Step 1.
- ~~Step 4 A vs B~~ — **B**: keep `compile-all.sh` as a 2-line
  wrapper. Codified in Step 4.
- ~~Convenience verb~~ — **yes**: `gh optivem compile` (no
  subcommand) runs `system + system-tests` in sequence. Codified in
  Step 2; Step 3 simplified accordingly.

- ~~Multitier ordering~~ — **sequential** Backend → Frontend, halt on
  first failure. Codified in Step 2.
- ~~Quiet flags~~ — **verbose**, no quiet flags. Codified in Step 1.

### Resolved: unify with `gh optivem init`'s `VerifyCompilation` ✅ Done (2026-05-11)

Approach **A** — full unify. `internal/steps/verify.go`'s `buildCommands`
was deleted; `compileComponent` now calls `compiler.CompileIn(lang, cwd)`,
normalizing `react` → `typescript` at the callsite (`cfg.FrontendLang`
stays `"react"` everywhere else — display strings, path resolution —
because flattening upstream would break those callers).

The "does init's java compile actually need `compileTestJava`?" question
turned out to have a stronger answer than the plan anticipated: yes, and
so does the runtime path. `system/monolith/java/src/test/java` and
`multitier/backend-java/src/test/java` both hold real JUnit unit tests
(e.g. `MyShopApplicationTests.java`). The structural cycle was silently
skipping their typecheck. So `compiler.commandsFor` now runs
`compileJava compileTestJava` for java in **both** init and runtime;
dotnet and typescript already covered tests via `.sln` and `tsconfig`
include globs respectively. No `CompileOptions{IncludeTests bool}` flag
was needed — tests are always in scope.

React's `npm run build` (full Next.js bundle) is gone from init. The
tradeoff (loses bundle-time errors that `tsc --noEmit` misses) was
accepted in exchange for one source of truth.

## Out of scope

- Targeted compile (`compile_targeted` / `compile-targeted.sh`) — see
  Step 5.
- `disable-test.sh` / `enable-test.sh` — same shell-script pattern but
  different concern.
- Reading any docker-orchestration config from `gh-optivem.yaml` —
  unrelated; that conversation belongs elsewhere.
