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

*(none — plan is ready to execute)*

## Out of scope

- Targeted compile (`compile_targeted` / `compile-targeted.sh`) — see
  Step 5.
- `disable-test.sh` / `enable-test.sh` — same shell-script pattern but
  different concern.
- Reading any docker-orchestration config from `gh-optivem.yaml` —
  unrelated; that conversation belongs elsewhere.
