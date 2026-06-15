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

## Candidate modules (to be confirmed by the inventory)

- **Process** — engine + process definition + agent set + scaffolding. *(Child 1 — see below.)*
- **Dev-workflow** — `commit`, `sync`, `actions status` across the academy repos.
- **Architecture / diagrams** — `architecture show`, process-flow rendering.
- **CLI / surface** — the Cobra command layer that composes the modules.

> These are hypotheses. The Step-1 inventory confirms, merges, or splits them before any child plan beyond Child 1 is written.

## Steps (parent-level)

- [ ] Step 1: **Inventory the ball of mud** — map every package/command/asset under `internal/**` and `main.go` to a candidate module; record cross-module coupling. This output defines the children.
- [ ] Step 2: **Confirm the module map & dependency rules** — finalize the bounded contexts, naming, and allowed dependency edges from the inventory.
- [ ] Step 3: **Execute Child 1** (process/engine — already drafted).
- [ ] Step 4: **Spawn remaining child plans from the inventory** — one per confirmed module, in dependency order.
- [ ] Step 5: **Keep parent and children in sync** — use `/coordinate-plans` as children land; update the module map as reality is discovered.

## Child plans

- **Child 1 — Process module: engine ↔ process definition ↔ agents ↔ scaffolding** → `20260615-0549-modularize-gh-optivem-engine-process.md` *(drafted; the hardest and most valuable module)*
- *Child 2+ — dev-workflow, architecture/diagrams, CLI surface — written after the Step-1 inventory.*

## Open questions

- **Module granularity** — is "process" one module or does the engine/definition/agent/scaffolding split warrant being modules in their own right under the parent? (Child 1 treats them as layers within the process module; the inventory may argue otherwise.)
- **Sequencing** — after Child 1, which module is the next cleanest cut? (Inventory decides.)
