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

### Step 4 — Reduce `compile-all.sh` to a wrapper in the shop template ⏳ Deferred: separate session, runs against the shop template repo (different repo, non-blocking per the plan note)

Replace the existing `compile-all.sh` in the shop template with a
2-line wrapper:

```bash
#!/usr/bin/env bash
set -e
gh optivem compile
```

Students still see a readable script at the repo root; the actual
per-language logic lives in the Go binary. This change happens in the
shop template repo, not in `gh-optivem` — it's non-blocking and can
follow Step 3 in a separate PR.

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

### Remaining questions

#### Unify with `gh optivem init`'s `VerifyCompilation`?

**Context (raised post-implementation, 2026-05-09):** Steps 1–3 introduced
`internal/compiler.commandsFor` as the per-language dispatch table for the
runtime compile path. There is a parallel dispatcher at scaffold-time in
`internal/steps/verify.go:58-74` (`buildCommands`), called by
`VerifyCompilation` (`:30-45`) → `compileComponent` (`:47-56`) during the
"Verify local compilation" step of `gh optivem init`. Two real divergences
between the two dispatchers:

1. **Java**: init runs `compileJava compileTestJava` (covers test code);
   runtime `internal/compiler` runs `compileJava` only. The plan resolved
   the runtime case to `compileJava` — and the structural cycle compiles
   test code separately via `compile system-tests`, so that's correct *for
   the runtime path*. Init's `compileTestJava` is needed because init
   compiles `system/` and `system-tests/` as separate verifies, and
   `system/`'s gradle project may include test sources that have to
   typecheck.
2. **react**: init handles a `react` lang case as `npm ci && npm run build`;
   `internal/compiler` doesn't (no `LangReact` enum). React isn't an
   architecture-tier language in `gh-optivem.yaml` today — frontend
   `lang: typescript` covers it — so this only matters at init-time when
   `--frontend-lang react` is passed.

**Possible approaches:**

- **A. Move both onto `internal/compiler`, parameterize java for test
  compile.** Add a `CompileOptions{IncludeTests bool}` (or a second
  entry-point `CompileWithTests`) so init's verify pass can opt in. Drop
  the `react` case from init by treating it as typescript-with-build (or
  add `LangReact` to `projectconfig`). One source of truth, one place to
  edit when a language's tooling changes.
- **B. Leave them separate.** Init's compile is a "does it build at all"
  check on a freshly-scaffolded local clone, before any `gh-optivem.yaml`
  is read into the runner; runtime's compile is driven by the YAML the
  user can edit. Different inputs, different timing — keeping them apart
  avoids overcoupling.
- **C. Half-step: extract just `commandsFor`.** Move the `lang → []string`
  table to a shared internal helper, but keep the call sites (which differ
  in cwd resolution, error semantics, and the test-code variant) separate.

**Open call:** A is the right answer eventually, but blocked on a small
design call — does init's java compile actually need `compileTestJava`,
or is that a habit from before `compile system-tests` existed? If the tests
live under a separate gradle project rooted at `system-test/`, init could
compile them via a second `Compile(systemTest, ...)` call and drop
`compileTestJava` from the system-tier compile. That would make A
trivially clean. C is the safe interim if the gradle layout question
isn't worth digging into right now.

## Out of scope

- Targeted compile (`compile_targeted` / `compile-targeted.sh`) — see
  Step 5.
- `disable-test.sh` / `enable-test.sh` — same shell-script pattern but
  different concern.
- Reading any docker-orchestration config from `gh-optivem.yaml` —
  unrelated; that conversation belongs elsewhere.
