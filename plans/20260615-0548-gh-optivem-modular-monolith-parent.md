# 2026-06-15 05:48:00 UTC — gh optivem modular-monolith decomposition (PARENT)

> **Parent plan.** Holds the vision, module map, dependency rules, and naming for decomposing `gh optivem`. Concrete execution lives in **child plans** (listed below). This file is the stable reference; children are independently executable and may be reordered, deferred, or dropped.

> **Resume contract.** This plan is meant to be resumed by running `/clear` then `/execute-plan plans/20260615-0548-gh-optivem-modular-monolith-parent.md` — **no custom prompt needed.** The **▶ Next executable step** block below always names the single next concrete, executable unit of work, fully grounded so a fresh agent can act without re-deriving it. **Whenever that unit is completed, replace the block with the next one.** If only design/planning remains (not a mechanical move), the block must say so explicitly and point at the child plan to draft/refine — so the agent knows to switch to `/create-plan` or `/refine-plan` rather than hunt for edits.

## ▶ Next executable step (resume here)

**Modular-monolith decomposition complete — no children remain.** As of 2026-06-15 all eight children (#1–#8) are ✅ **done and committed**. The final one, child #5, formalized the already-moved Scaffolding and shared `build` packages as documented modules: `internal/build/doc.go` (surface `compiler`/`runner`, kernel-only dependency rule) backed by the walking cycle-guard `internal/build/import_guard_test.go`, and `internal/scaffolding/doc.go` (Scaffolding boundary). Both still-open seams are now resolved — **#3 `steps → compiler, runner`** and **#4 `preflight → runner`** are legal downward edges onto the shared `internal/build` module, kept cycle-free by the guard. `go build ./...` and `go test ./...` are green. **No next executable step — this parent plan is a finished reference; nothing left to resume.** Still un-moved at root by design (a deliberate end-state, not pending work): `expand`, `userstate`, `assets`, and the generic `atdd/runtime/*` packages that stayed (`agents`, `intake`, `outlog`, `override`, `trace`, `release`, `repolocator`, `testselect`, `tracker/**`, `preflight`, `driver`).

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

- **Shared kernel** (infrastructure, NOT a bounded context) — `log`, `shell`, `pathx`, `spinner`, `promptio`, `approval`, `cmdctx`, `version`. Imported broadly; everything may depend on these. `shell → log, pathx, spinner`; `approval → promptio`. *(`version` folded in via child #3.)*
- **Engine core** — ✅ **carved (child #7, first pass)** to `internal/engine/statemachine` (peer module). It is the center of gravity (nearly every `atdd/runtime/*` package imports it) and now exposes only the generic `LoadBytes` contract — it embeds no concrete process. The ATDD BPMN it used to bundle now lives in the process module at `internal/atdd/process`.
- **Process module (ATDD)** — the bulk of `internal/`. The process-definition core lives under `atdd/process/`: the BPMN (`process-flow.yaml` + `Load()`) plus the ATDD-specific `{actions, gates, verify, clauderun, configcheck}` (relocated in child #8). The generic/orchestration packages stay at `atdd/runtime/{agents, override, repolocator, trace, driver, release, preflight, tracker/**, intake, outlog, testselect}` — `driver` is the orchestrator that pulls the rest together. Plus `expand`, `assets`, `userstate` (still at root). Commands: `process`, `run`, `implement`, `test`. *(`diagram` moved out to `internal/diagrams/diagram`.)*
- **Build/run helpers** — `runner` (`→ spinner, pathx`) and `compiler` (`→ projectconfig, shell`). Used by *both* Process (`preflight → runner`) and Scaffolding (`steps → compiler, runner`). Likely belongs to the engine/process side or a small shared "build" module — TBD.
- **Scaffolding** — `steps`, `templates`, `files`. Command: `environment`. `steps → config, projectconfig, templates, files, shell, log, compiler, runner`.
- **Config** — `internal/config/**` (`config` at `internal/config`; `configinit` at `internal/config/configinit`; `optivemyaml` at `internal/config/optivemyaml`) + `projectconfig` demoted to the shared kernel at `internal/kernel/projectconfig`. ✅ **done (child #4)**. Commands: `config`, `compile`.
- **Dev-workflow / GitHub** — `ghbulk`, `sonar`, `workspace`. Commands: `cross_repo`, `cleanup`. **Lowest coupling** (`ghbulk`/`sonar → shell, log` only; `workspace → projectconfig`).
- **Architecture / diagrams** — `diagrams/architecture` (command `architecture`) and `diagrams/diagram` (`→ statemachine`, used by `process`). Read-only renderers over the engine model. *(moved out of `atdd/runtime/` — done.)*
- **Diagnostics / misc** — *not a module* (resolved, child #3): commands `doctor`, `system`, `cleanup`, `hooks` stay at the root CLI surface; the lone package `version` was folded into the shared kernel (`internal/kernel/version`). `doctor → promptio, userstate`.
- **CLI / surface** — `main.go` + all `*_commands.go`; composes the modules.

### Cross-module seams (the hard cuts)

1. **`projectconfig → atdd/runtime/statemachine`** — ✅ **resolved** (child #6, `20260615-0749`). The one engine-backed rule (task-prompts known-name check) was relocated to `internal/atdd/process/configcheck`; `projectconfig` now imports nothing from `internal/atdd/**` (build-level guard test in `import_guard_test.go` keeps it that way). Config no longer reaches up into the Engine.
2. **`configinit → steps, files`** — ✅ **resolved**. The optivem.yaml builder (`BuildOptivemYAML` + the two `WriteOptivemYAMLToFilePath*` wrappers) moved to the config-side leaf `internal/config/optivemyaml`; the generic `.gitignore` helper (`EnsureGitignoreLine`, shared with the Process driver) moved to the kernel as `internal/kernel/gitignore.EnsureLine` rather than the config side, to avoid a Process→Config edge. `configinit`/`config` now import nothing from `internal/scaffolding/**` — build-level guards in `internal/configinit/import_guard_test.go` and `internal/config/import_guard_test.go` keep it that way.
3. **`steps → compiler, runner`** — ✅ **resolved** (child #5, `20260615-1310`). A legal downward edge: Scaffolding (`steps`) depends on the shared `internal/build` module, which sits above the kernel and below both Scaffolding and Process. The module's boundary is documented (`internal/build/doc.go`, `internal/scaffolding/doc.go`) and the edge stays legal because `internal/build/import_guard_test.go` forbids `build` from importing anything outside `internal/kernel/**` — so `build` can never reach back up into Scaffolding and form a cycle.
4. **`preflight → runner`** — ✅ **resolved** (child #5, `20260615-1310`). A legal downward edge: Process (`preflight`) depends on the shared `internal/build` module on the same terms as seam #3. Kept legal by the same backstop — `internal/build/import_guard_test.go` ensures `build` depends only downward (kernel), so the shared dependency can never become a cycle.
5. **`projectconfig` is imported by almost everything** — it's a near-kernel domain type. ✅ **resolved** (child #4): demoted to the shared kernel at `internal/kernel/projectconfig`. Its ~50 cross-module importers are now legal downward edges; its `import_guard_test.go` moved with it and still asserts no `internal/atdd/**` import.

### Cut difficulty

- **Dev-workflow** — *easy*. Only kernel + `projectconfig`. Best next child after Child 1.
- **Architecture/diagrams** — *medium*. Read-only over `statemachine`; needs the engine to expose a stable public model.
- **Diagnostics/misc** — *medium*. Small, but `doctor` touches `userstate`/`promptio`.
- **Config** — *hard*. ✅ **done (child #4)**. Seams #1 and #2 inverted; `projectconfig` demoted to kernel; `configinit` nested under `internal/config/`.
- **Scaffolding** — *hard*. Seam #3 couples it to build/run helpers.
- **Process (Child 1)** — *hard/largest*. The whole engine + definition + agents; resolving seam #1 is a prerequisite.

### Decisions (Step 2 — confirmed)

- **Module map & shared-kernel set accepted** as written above.
- **Seam #1 gets its own child** ("invert `projectconfig ↔ engine`") that runs **before Child 1** — Child 1's clean engine extraction depends on it.
- **`runner` + `compiler` become a small shared "build" module** that both Process and Scaffolding depend on (not folded into the engine).
- **`scaffolding` (`steps`/`templates`/`files`) is its own module**, not a layer inside Process.
- **Child ordering is difficulty-first**: Dev-workflow next; Process (Child 1) last.

**Confirmed child order:** ~~Dev-workflow~~ ✅ → ~~Architecture/diagrams~~ ✅ → ~~Diagnostics~~ ✅ → ~~Config~~ ✅ → ~~Scaffolding~~ ✅ → ~~*invert seam #1*~~ ✅ → ~~Process (Child 1) first pass~~ ✅ → ~~Process follow-up (child #8)~~ ✅ → ~~Scaffolding + shared `build` module (child #5)~~ ✅ (done 2026-06-15). **All children complete — modular-monolith decomposition done.**

### Resume notes (for the fresh session — read first)

- **Use an isolated subagent per move.** Each module move (relocate packages + update import paths + `go build`/`go test`) should run in its own subagent so the heavy file-editing stays out of the main context; the subagent returns only a summary and the orchestrator commits via the commit skill. *(Subagents 529'd twice on 2026-06-15 — retry; fall back to inline only if they keep failing.)*
- **Physical nesting** (`internal/<module>/`) is the agreed shape; pure moves only.
- **Where we paused (mechanical moves done):** ✅ dev-workflow, ✅ architecture/diagrams, ✅ kernel (→ `internal/kernel/`: log, shell, pathx, spinner, promptio, approval, cmdctx, **version**, **projectconfig**), ✅ build (→ `internal/build/`: runner, compiler), ✅ scaffolding (→ `internal/scaffolding/`: steps, templates, files), ✅ diagnostics child #3 (`version` folded into kernel; commands stay at root), ✅ config child #4 (`projectconfig` → kernel; `configinit` → `internal/config/configinit`; `config` unchanged). Remaining work is design, not a pure move: only the engine/process carve-out (#7) is left, and it needs deliberate planning in a fresh session. Still un-moved at root by design pending that: `expand`, `userstate`, `assets`, and the whole `atdd/` engine+process tree.
- **Emitted-header exception (architecture/diagrams):** the 2 renderer header strings (`internal/diagrams/architecture/architecture.go:82`, `internal/diagrams/diagram/diagram.go:122`) are intentionally left at the old path — user decision, leave them; do not regenerate locally.

## Steps (parent-level)

- [ ] Step 3: **Spawn + execute children in the confirmed order** — following *Confirmed child order* above, **one isolated subagent per move** (see Resume notes). The single next concrete unit is always spelled out in the **▶ Next executable step** block at the top — execute that. The seam-invert children and the engine carve-out are design work, not moves — pause for planning before them.
- [ ] Step 4: **Keep parent and children in sync** — use `/coordinate-plans` as children land; update the module map as reality is discovered.

## Child plans

Listed in execution order (only Child 1 is drafted; the rest are written just-in-time per Step 3):

1. **Dev-workflow** (`ghbulk`, `sonar`, `workspace`) — ✅ **done** → moved to `internal/devworkflow/`; see `20260615-0706-module-devworkflow.md`.
2. **Architecture / diagrams** — ✅ **done** → moved to `internal/diagrams/{architecture,diagram}` + updated CI workflows, agent defs, docs prose; see `20260615-0722-module-diagrams.md`. *(2 emitted renderer headers left at old path — see record.)*
3. **Diagnostics / misc** (`doctor`, `system`, `version`) — ✅ **done**: confirmed *not a module*. Commands stay at the root CLI surface; `version` folded into the shared kernel (`internal/version` → `internal/kernel/version`, importers + `.goreleaser.yml` + `scripts/install.sh` ldflags paths updated). `go build ./...` + kernel/config tests green.
4. **Config** (`config`, `configinit`, `projectconfig`) — ✅ **done**: `projectconfig` demoted to the shared kernel (`internal/kernel/projectconfig`), `configinit` nested at `internal/config/configinit`, `config` unchanged at `internal/config`. `go build ./...` + `go test ./...` green. See `20260615-0933-module-config.md`.
5. **Scaffolding** (`steps`, `templates`, `files`) + the shared **build** module (`runner`, `compiler`) — ✅ **done (2026-06-15)**: the already-moved packages are now formalized as modules. `internal/build/doc.go` documents the shared `build` module (surface `compiler`/`runner`, kernel-only dependency rule) with a walking cycle-guard (`internal/build/import_guard_test.go`); `internal/scaffolding/doc.go` documents the Scaffolding boundary. Seams #3 and #4 resolved as legal downward edges onto `build`. `go build ./...` + `go test ./...` green. See `20260615-1310-module-scaffolding-build.md`.
6. **Invert seam #1** — untangle `projectconfig → statemachine`. ✅ **done** → rule relocated to `internal/atdd/process/configcheck`; `projectconfig` is now a leaf and kernel-eligible (seam #5 unblocked); see `20260615-0749-invert-seam1-projectconfig-engine.md`.
7. **Process module: engine ↔ process definition ↔ agents** — ✅ **done (2026-06-15)**: engine carved to `internal/engine/statemachine` (`LoadBytes` only), ATDD BPMN to `internal/atdd/process` (`Load()`); build + `go test ./...` green. Plan file removed (fully complete; git history is the record).
8. **Process follow-up: swappable agent set + deep-untangle** — ✅ **done (2026-06-15)**: swappable `AgentSet` binding (`NewAgentSetFS`, injectable `fs.FS`); both swap axes proven as tests; reuse path documented in `doc.go`; `actions`/`gates`/`verify`/`clauderun`/`configcheck` relocated to `internal/atdd/process/`. Build + `go vet` + full `go test ./...` green. Plan file removed (fully complete).

## Open questions

*All Step-2 questions resolved — see "Decisions (Step 2 — confirmed)" above. Child 1's draft still describes scaffolding as a layer inside the Process module; reconcile it with the decision that scaffolding is its own module when Child 1 is next revised.*
