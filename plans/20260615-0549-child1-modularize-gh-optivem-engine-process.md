# 2026-06-15 05:49:35 UTC — Process module: engine ↔ process definition ↔ agents (CHILD #7 — last)

> **Child of** `20260615-0548-gh-optivem-modular-monolith-parent.md`. This is the **last and hardest** module in the modular-monolith decomposition: the generic process engine and its pluggable process definition and agent set. All other children (#1–#6) are done; this one ran last by design because it depends on them. **First pass DONE (2026-06-15):** the engine↔definition seam is broken — the generic engine now lives at `internal/engine/statemachine` (exposes only `LoadBytes`, embeds no process) and the ATDD BPMN lives at `internal/atdd/process` (`Load()` wrapping `LoadBytes`). Build + full `go test ./...` green. The remaining work (extract a swappable agent set, prove both swap axes, document the reuse path, deep-untangle `actions`/`gates`/`verify`/`driver`) is a **deferred follow-up** — see Steps below. Parent holds the cross-module vision, dependency rules, and the inventory that spawns the other children.

## ▶ Next executable step (resume here)

**This is now design/planning work, not a mechanical edit — switch to `/create-plan`.** The first-pass carve (engine↔definition seam) is complete and committed. What remains is a **follow-up child** covering: (Step 7) make the agent set a separately-bound, swappable layer — parameterise the hardcoded `runtime/agents/atdd` dir in `internal/atdd/runtime/agents/embed.go`; (Step 8) prove both swap axes end-to-end; (Step 9) document the reuse path; (Step 10 remainder) deep-untangle `actions`/`gates`/`verify`/`driver` out of `internal/atdd/runtime/` into a process-definition home. Draft that follow-up with `/create-plan` (or refine this file's Steps 7–10 into it). Nothing here is a ready-to-execute mechanical move — the deep-untangle needs an import-coupling analysis first (see Open questions).

## TL;DR

**Why:** `gh-optivem` has grown into a "big ball of mud" where the generic orchestration machinery, the one specific ATDD/BPMN process, its agents, and project scaffolding are all tangled together. A developer who wants to run a *different* process (different BPMN, different agents) can't reuse the engine without forking the whole thing.

**End result:** Within one repo, the codebase is split along clear seams — **engine**, **process definition**, **agent set**, **CLI** (scaffolding already lives in its own peer module) — with all dependencies pointing down to the engine's contracts. The engine knows nothing about the specific ATDD process; it loads a process definition from a well-defined contract, and resolves the agents that process references (by role) from a separately-bound agent set. Two independent swap axes: drop in a new BPMN process, **and/or** rebind its roles to a different agent set — both with no engine changes.

## Outcomes

What we get out of this — the goals and deliverables:

- **A named, documented layering** (one repo, internal package boundaries) with the concerns this child carves: **engine**, **process definition**, **agent set**, **CLI/surface** — dependencies all pointing down to the engine. (Scaffolding is already its own peer module under `internal/scaffolding/`.)
- **A generic process engine** that executes a BPMN/state-machine process without hardcoded knowledge of the specific ATDD process, its agents, or its prompts; it defines the contracts everything else plugs into.
- **The current ATDD process extracted into a self-contained "process definition"** — its `process-flow.yaml` (BPMN) + gateway/action/data-flow bindings — referencing agents **by role name**, not by concrete prompt.
- **Agents as their own pluggable layer** — concrete prompt bodies (agent set) that fulfil the roles a process references, bound to the process at load time so they can be swapped independently of the flow.
- **Two independent swap axes**: (a) *process axis* — replace the whole BPMN definition; (b) *agent axis* — keep the process, rebind its roles to a different agent set. Either works for a third party or for the existing ATDD process.
- **A defined engine↔definition contract** (how a process declares states, gateways, actions, agent-role references, and data flow) so a third party can author their own definition/agent set against the same interface.
- **A reuse story**: documented steps for "bring your own BPMN and/or agents, reuse the engine."
- **No behavior change** for the existing ATDD flow — the same process runs identically after the split (regression-safe refactor).

## Steps

**First pass — DONE (2026-06-15).** Steps 1–6 + the moved-scope regression-check are complete: the engine↔definition seam is broken and the layout is locked. Steps 1–6 are removed (git history is the record); what remains below is the deferred follow-up.

- [ ] Step 7: **Extract the ATDD agent set into its own bound layer** — ⏳ Deferred (follow-up). The role→agent binding *contract* already exists and is confirmed (YAML `agent:` name → `internal/assets/runtime/agents/atdd/<name>.md`, resolved by the `agents` registry; binding dir hardcoded in `internal/atdd/runtime/agents/embed.go`). What's deferred: making the agent set a *separately-bound, swappable* unit (parameterise the hardcoded `runtime/agents/atdd` dir; allow an alternate agent set at load time).
- [ ] Step 8: **Prove both swap axes** — ⏳ Deferred (follow-up). Demonstrate (or spec) loading a second minimal process via `statemachine.LoadBytes` *and* rebinding the existing process to an alternate agent set, validating the boundaries end-to-end.
- [ ] Step 9: **Document the reuse path** — ⏳ Deferred (follow-up). Short guide: "bring your own BPMN (`LoadBytes`) and/or agents (alternate agent set) and run them on `internal/engine/statemachine`."
- [ ] Step 10 (remainder): **Deep-untangle `atdd/runtime` internals** — ⏳ Deferred (follow-up). `actions`/`gates`/`verify`/`driver` still live under `internal/atdd/runtime/` and encode *this* process; the moved-scope regression (build + full `go test ./...`) is green, but physically relocating these into a process-definition home is the remaining carve. The open question is how tightly they bind to the engine core (see Open questions).

## Resolved decisions

- **Engine home (2026-06-15)** — the generic engine becomes a top-level **peer module** `internal/engine/statemachine` (matching `internal/kernel`, `internal/build`, `internal/config`, …). It exposes only the generic contract `LoadBytes([]byte) (*Engine, error)`; it no longer embeds any concrete process.
- **Process-definition home (2026-06-15)** — the ATDD BPMN (`process-flow.yaml`) + its embed binding move to `internal/atdd/process` (package `process`), which exposes `Load() (*statemachine.Engine, error)` (= `statemachine.LoadBytes(DefaultYAML)`) and `DefaultYAML`. This is the concrete engine↔definition seam: the engine stops bundling the ATDD flow.
- **Session scope (2026-06-15)** — seam + contracts only. Break the engine↔definition seam, document the already-existing role→agent contract, rewrite importers/call-sites, keep build+tests green. `actions`/`gates`/`verify`/`driver` stay where they are (only their `statemachine` import path updates) — the deep untangle is a follow-up child.
- **Repo strategy** — one repo for now, with internal package boundaries (engine still extractable as its own module later if desired).
- **Agents** — a separate pluggable layer; processes reference agents by role, concrete agent sets bind at load time, swappable independently of the flow.
- **Layer names** — engine / process definition / agent set / CLI (recommended; open to renaming). Scaffolding is a confirmed **separate peer module** (`internal/scaffolding/`, parent decision), not a layer inside this child.

## Open questions

- ✅ **Scope of this child — resolved: seams-first, contract-first.** This child establishes the contracts (engine↔definition, role→agent binding) + directory layout and moves the clean wins; deep untangling of the `atdd/runtime` internals is deferred to a follow-up. The parent's ▶ Next-step framing ("carve ... into a Process module with a clean public surface") describes the *eventual end-state* — this child is the first pass toward it, not the whole carve in one go.
- **Existing coupling (the follow-up's main unknown)** — the engine is now carved to `internal/engine/statemachine` (import-clean, embeds no process) and the ATDD definition to `internal/atdd/process`. Remaining entanglement is confined to the `atdd/runtime` internals (`driver`, `agents`, `actions`, `gates`, `verify`, etc.), which still import `internal/engine/statemachine` and encode *this* process. How tightly those bind to the engine core — and therefore the effort to relocate them into a process-definition home — is the open question the follow-up child must answer first (an import-coupling analysis before any move).

> The wider "is `gh optivem` doing too much / modular-monolith decomposition" question now lives in the **parent** plan.
