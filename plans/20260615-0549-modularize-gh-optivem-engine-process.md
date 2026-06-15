# 2026-06-15 05:49:35 UTC — Process module: engine ↔ process definition ↔ agents ↔ scaffolding (CHILD 1)

> **Child of** `20260615-0548-gh-optivem-modular-monolith-parent.md`. This is the first and hardest module in the modular-monolith decomposition: the generic process engine and its pluggable process definition, agent set, and scaffolding. Parent holds the cross-module vision, dependency rules, and the inventory that spawns the other children.

## TL;DR

**Why:** `gh-optivem` has grown into a "big ball of mud" where the generic orchestration machinery, the one specific ATDD/BPMN process, its agents, and project scaffolding are all tangled together. A developer who wants to run a *different* process (different BPMN, different agents) can't reuse the engine without forking the whole thing.

**End result:** Within one repo, the codebase is split along clear seams — **engine**, **process definition**, **agent set**, **scaffolding**, **CLI** — with all dependencies pointing down to the engine's contracts. The engine knows nothing about the specific ATDD process; it loads a process definition from a well-defined contract, and resolves the agents that process references (by role) from a separately-bound agent set. Two independent swap axes: drop in a new BPMN process, **and/or** rebind its roles to a different agent set — both with no engine changes.

## Outcomes

What we get out of this — the goals and deliverables:

- **A named, documented layering** (one repo, internal package boundaries) with five concerns: **engine**, **process definition**, **agent set**, **scaffolding**, **CLI/surface** — dependencies all pointing down to the engine.
- **A generic process engine** that executes a BPMN/state-machine process without hardcoded knowledge of the specific ATDD process, its agents, or its prompts; it defines the contracts everything else plugs into.
- **The current ATDD process extracted into a self-contained "process definition"** — its `process-flow.yaml` (BPMN) + gateway/action/data-flow bindings — referencing agents **by role name**, not by concrete prompt.
- **Agents as their own pluggable layer** — concrete prompt bodies (agent set) that fulfil the roles a process references, bound to the process at load time so they can be swapped independently of the flow.
- **Two independent swap axes**: (a) *process axis* — replace the whole BPMN definition; (b) *agent axis* — keep the process, rebind its roles to a different agent set. Either works for a third party or for the existing ATDD process.
- **A defined engine↔definition contract** (how a process declares states, gateways, actions, agent-role references, and data flow) so a third party can author their own definition/agent set against the same interface.
- **A reuse story**: documented steps for "bring your own BPMN and/or agents, reuse the engine."
- **No behavior change** for the existing ATDD flow — the same process runs identically after the split (regression-safe refactor).

## Steps

- [ ] Step 1: **Map the current ball of mud** — inventory what lives where today (`internal/atdd/runtime/**`, `internal/assets/runtime/agents/atdd/**`, scaffolding/generation code, CLI commands) and tag each piece as engine / process-definition / agent-set / scaffolding / CLI.
- [ ] Step 2: **Name the layers and agree the seams** — settle the five-layer terminology and what each layer may depend on (dependency direction: everything points down to the engine; the engine points to nothing concrete).
- [ ] Step 3: **Define the engine↔definition contract** — what a process definition must declare (BPMN/state-machine, gateways, actions, **agent-role references**, data-flow/State-vs-Params) and how the engine discovers/loads it.
- [ ] Step 4: **Define the role→agent binding** — how a process references an agent by role and how a concrete agent set is bound at load time (the agent swap axis).
- [ ] Step 5: **Carve out the process engine** — move generic execution machinery into an engine package with no `atdd`-specific imports; the engine compiles and runs without any concrete process baked in.
- [ ] Step 6: **Extract the ATDD process definition** — gather `process-flow.yaml` + bindings into one self-contained definition unit (referencing agents by role) that the engine loads via the contract.
- [ ] Step 7: **Extract the ATDD agent set** — pull the concrete agent prompts into a separately-bound agent-set unit that fulfils the process's roles.
- [ ] Step 8: **Separate scaffolding** — isolate project-generation/template concerns from the engine so they evolve independently.
- [ ] Step 9: **Prove both swap axes** — demonstrate (or at least spec) loading a second minimal process *and* rebinding the existing process to an alternate agent set, validating the boundaries.
- [ ] Step 10: **Document the reuse path** — a short guide: "bring your own BPMN and/or agents and run them on the engine."
- [ ] Step 11: **Regression-check** — confirm the existing ATDD flow runs identically after the restructure.

## Resolved decisions

- **Repo strategy** — one repo for now, with internal package boundaries (engine still extractable as its own module later if desired).
- **Agents** — a separate pluggable layer; processes reference agents by role, concrete agent sets bind at load time, swappable independently of the flow.
- **Layer names** — engine / process definition / agent set / scaffolding / CLI (recommended; open to renaming).

## Open questions

- **Scope of this child** — full physical carve-out now, or first pass that establishes the seams (contracts + directory layout, move the clean wins) and defers deep untangling? *Recommendation: seams-first, contract-first.*
- **Existing coupling** — how entangled are the engine and the ATDD specifics today? (Informed by the parent's Step-1 inventory; affects effort estimate.)

> The wider "is `gh optivem` doing too much / modular-monolith decomposition" question now lives in the **parent** plan.
