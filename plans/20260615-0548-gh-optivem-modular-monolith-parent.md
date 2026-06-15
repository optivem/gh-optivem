# 2026-06-15 05:48:00 UTC — gh optivem modular-monolith decomposition (PARENT)

> **Parent plan.** Holds the vision, module map, dependency rules, and naming for decomposing `gh optivem`. Concrete execution lives in **child plans** (listed below). This file is the stable reference; children are independently executable and may be reordered, deferred, or dropped.

## TL;DR

**Why:** `gh optivem` has grown into a "big ball of mud" — a generic process engine, one specific ATDD/BPMN process, its agents, project scaffolding, dev-workflow tooling (commit/sync/actions), and architecture/diagram tooling are all tangled in one binary. Concerns can't be reused, swapped, or reasoned about independently.

**End result:** One binary, organized as a **modular monolith** — several bounded-context modules with explicit public surfaces and a strict dependency direction (everything points down to shared/engine contracts; nothing reaches into another module's internals). The first and hardest module — the process engine and its pluggable process/agents — is carved out first; the rest follow once an inventory reveals the real seams.

## Outcomes

- **A documented module map** for `gh optivem`: the bounded contexts, each module's public surface, and the dependency rules between them.
- **A modular monolith, not microservices** — one binary, hard internal boundaries, modules communicating through interfaces rather than each other's internals.
- **A strict dependency direction** — modules point down to shared/engine contracts; no module reaches into another's internals; no cycles.
- **A naming convention** for modules and their packages, applied consistently.
- **A factual inventory** of what lives where today, which is what *defines* the child plans (no child is written ahead of the facts).
- **A set of independently-executable child plans**, one per module, each regression-safe (no behavior change to the existing CLI).

## Modular-monolith principles (the stable contract)

- **One binary.** No microservices, no extra repos for now. Engine remains extractable as its own module later if desired.
- **Bounded contexts as modules.** Each module owns a cohesive concern and exposes a small public surface; internals are private.
- **Dependencies point down.** Modules depend on shared/engine contracts, never on each other's internals. No cycles.
- **CLI composes.** The CLI/surface layer wires modules together; modules don't know about the CLI.
- **Regression-safe.** Every child split preserves existing CLI behavior.

## Module map (from Step-1 inventory)

*Grounded in actual Go imports traced across every `*.go` (root command files + `internal/**`). Module path: `github.com/optivem/gh-optivem`.*

- **Shared kernel** (infrastructure, NOT a bounded context) — `log`, `shell`, `pathx`, `spinner`, `promptio`, `approval`, `cmdctx`. Imported broadly; everything may depend on these. `shell → log, pathx, spinner`; `approval → promptio`.
- **Engine core** — `atdd/runtime/statemachine` is the center of gravity: nearly every `atdd/runtime/*` package imports it. This is the generic process model Child 1 extracts.
- **Process module (ATDD)** — the bulk of `internal/`: `atdd`, `atdd/runtime/{agents, gates, actions, verify, diagram, override, repolocator, trace, driver, clauderun, release, preflight, tracker/**, intake, outlog, testselect}`, plus `expand`, `assets`, `userstate`. `driver` is the orchestrator that pulls the rest together. Commands: `process`, `run`, `implement`, `test`.
- **Build/run helpers** — `runner` (`→ spinner, pathx`) and `compiler` (`→ projectconfig, shell`). Used by *both* Process (`preflight → runner`) and Scaffolding (`steps → compiler, runner`). Likely belongs to the engine/process side or a small shared "build" module — TBD.
- **Scaffolding** — `steps`, `templates`, `files`. Command: `environment`. `steps → config, projectconfig, templates, files, shell, log, compiler, runner`.
- **Config** — `config`, `configinit`, `projectconfig`. Commands: `config`, `compile`. `config → approval, cmdctx, log, projectconfig, version, shell`; `configinit → approval, config, files, projectconfig, steps`.
- **Dev-workflow / GitHub** — `ghbulk`, `sonar`, `workspace`. Commands: `cross_repo`, `cleanup`. **Lowest coupling** (`ghbulk`/`sonar → shell, log` only; `workspace → projectconfig`).
- **Architecture / diagrams** — `diagrams/architecture` (command `architecture`) and `diagrams/diagram` (`→ statemachine`, used by `process`). Read-only renderers over the engine model. *(moved out of `atdd/runtime/` — done.)*
- **Diagnostics / misc** — commands `doctor`, `system`, `cleanup`, `hooks`; package `version`. `doctor → promptio, userstate`.
- **CLI / surface** — `main.go` + all `*_commands.go`; composes the modules.

### Cross-module seams (the hard cuts)

1. **`projectconfig → atdd/runtime/statemachine`** — Config reaches *up* into the Engine. Backwards dependency; the biggest surprise. Must be inverted (engine should not be a dependency of config).
2. **`configinit → steps`** — Config reaches into Scaffolding.
3. **`steps → compiler, runner`** — Scaffolding reaches into build/run helpers (Process side).
4. **`preflight → runner`** — Process reaches into build/run helpers (expected if `runner` is engine-side).
5. **`projectconfig` is imported by almost everything** — it's a near-kernel domain type, yet it also imports the engine (see #1), so it can't simply be demoted to the kernel until #1 is resolved.

### Cut difficulty

- **Dev-workflow** — *easy*. Only kernel + `projectconfig`. Best next child after Child 1.
- **Architecture/diagrams** — *medium*. Read-only over `statemachine`; needs the engine to expose a stable public model.
- **Diagnostics/misc** — *medium*. Small, but `doctor` touches `userstate`/`promptio`.
- **Config** — *hard*. Seams #1 and #2 must be inverted first.
- **Scaffolding** — *hard*. Seam #3 couples it to build/run helpers.
- **Process (Child 1)** — *hard/largest*. The whole engine + definition + agents; resolving seam #1 is a prerequisite.

### Decisions (Step 2 — confirmed)

- **Module map & shared-kernel set accepted** as written above.
- **Seam #1 gets its own child** ("invert `projectconfig ↔ engine`") that runs **before Child 1** — Child 1's clean engine extraction depends on it.
- **`runner` + `compiler` become a small shared "build" module** that both Process and Scaffolding depend on (not folded into the engine).
- **`scaffolding` (`steps`/`templates`/`files`) is its own module**, not a layer inside Process.
- **Child ordering is difficulty-first**: Dev-workflow next; Process (Child 1) last.

**Confirmed child order:** Dev-workflow → Architecture/diagrams → Diagnostics → Config → Scaffolding → *invert seam #1* → Process (Child 1).

## Steps (parent-level)

- [ ] Step 3: **Spawn + execute children in the confirmed order** — one `/create-plan` per module, then `/execute-plan`, following *Confirmed child order* above. Dev-workflow first; the seam-#1 invert child before Child 1; Child 1 last.
- [ ] Step 4: **Keep parent and children in sync** — use `/coordinate-plans` as children land; update the module map as reality is discovered.

## Child plans

Listed in execution order (only Child 1 is drafted; the rest are written just-in-time per Step 3):

1. **Dev-workflow** (`ghbulk`, `sonar`, `workspace`) — ✅ **done** → moved to `internal/devworkflow/`; see `20260615-0706-module-devworkflow.md`.
2. **Architecture / diagrams** — ✅ **done** → moved to `internal/diagrams/{architecture,diagram}` + updated CI workflows, agent defs, docs prose; see `20260615-0722-module-diagrams.md`. *(2 emitted renderer headers left at old path — see record.)*
3. **Diagnostics / misc** (`doctor`, `system`, `version`). *(not drafted)*
4. **Config** (`config`, `configinit`, `projectconfig`). *(not drafted)*
5. **Scaffolding** (`steps`, `templates`, `files`) + the shared **build** module (`runner`, `compiler`). *(not drafted)*
6. **Invert seam #1** — untangle `projectconfig → statemachine`. Prerequisite for Child 1. *(not drafted)*
7. **Process module: engine ↔ process definition ↔ agents** → `20260615-0549-child1-modularize-gh-optivem-engine-process.md` *(drafted; runs last — hardest, depends on #6)*

## Open questions

*All Step-2 questions resolved — see "Decisions (Step 2 — confirmed)" above. Child 1's draft still describes scaffolding as a layer inside the Process module; reconcile it with the decision that scaffolding is its own module when Child 1 is next revised.*
