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

### Step 1 — Per-language compile dispatcher

New package `internal/compiler` with one entry point:

```go
func Compile(tier projectconfig.TierSpec, repoRoot string) error
```

Internal switch on `tier.Lang`:

| Lang         | Commands (cwd = `repoRoot/tier.Path`)        |
|--------------|----------------------------------------------|
| `dotnet`     | `dotnet build`                               |
| `java`       | `./gradlew compileJava`                      |
| `typescript` | `npm ci && npx tsc --noEmit`                 |

No quiet flags — match the verbosity of today's `compile-all.sh` so
warnings (Sonar, nullability, etc.) stay visible and failure output
stays informative.

Notes:

- The `npm ci` is the bug fix for today's failure — bare `npx tsc`
  prints "This is not the tsc command you are looking for" because no
  install ever ran.
- Stream stdout/stderr through; return first non-zero exit.
- Fake `Shell` for tests, mirroring `internal/runner/tests.go` pattern.

### Step 2 — Wire `compile system` and `compile system-tests` Cobra commands

New file `compile_commands.go`, mirroring `runner_commands.go` shape:

```
gh optivem compile             (shortcut: runs system + system-tests)
gh optivem compile system
gh optivem compile system-tests
```

The bare `gh optivem compile` is the "compile everything in scope"
shortcut for the structural cycle — it runs `compile system` then
`compile system-tests` in sequence, stopping at the first failure.
Subcommands stay available for callers (or humans) who want to be
explicit. *(Note: this is a deliberate departure from
`build`/`run`/`test`, which all require an explicit subcommand. The
asymmetry is justified because compile is the only one of the four
where "do both" is the dominant use case — the structural cycle.)*

Both load `gh-optivem.yaml` via `projectconfig.Load()` from the user's
cwd (consistent with the existing `Path = "gh-optivem.yaml"` constant
at `internal/projectconfig/config.go:39`).

`compile system` dispatch:

- `cfg.System.Architecture == ArchMonolith` → `Compile(asTierSpec(cfg.System), ".")`
- `cfg.System.Architecture == ArchMultitier` → `Compile(cfg.System.Backend, ".")` **then** `Compile(cfg.System.Frontend, ".")` — sequential, halt on first failure (matches today's `compile-all.sh`)

`compile system-tests` dispatch:

- `Compile(cfg.SystemTest, ".")`

Register both under root in `main.go` (next to existing
`newBuildCmd`, `newRunCmd`, `newTestCmd`).

Error handling: same `exitOnError` pattern as `runner_commands.go:52`.

### Step 3 — Swap the state-machine call site

`internal/atdd/runtime/actions/bindings.go:703-714` `compileInScope`:

```go
// before
cmdLine := "./compile-all.sh"

// after
cmdLine := "gh optivem compile"
```

The shape of the function (run command, stream output, return Outcome)
stays identical — only the command string changes. The bare
`gh optivem compile` does the system + system-tests sequence
internally (Step 2), so the state-machine call site stays a single
shell-out.

This matches the `gh optivem test system` shell-out pattern already
used at `bindings.go:819` and `:1017`.

Update tests in `internal/atdd/runtime/actions/bindings_test.go` (the
`compile-all.sh` string appears there too — `Grep` confirmed).

### Step 4 — Reduce `compile-all.sh` to a wrapper in the shop template

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
