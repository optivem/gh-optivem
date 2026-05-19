# Glossary

## Behavioral Change

A **behavioral change** is a change defined by **change-driven acceptance criteria** — the new (or restored) behavior IS specified by the AC scenarios produced from the ticket. Stories (new behavior) and Bugs (restored behavior) are behavioral; their change-driven AC route to the **AT Cycle** (test-first / ATDD). The unit of work in the AT Cycle is the **ticket** — all change-driven scenarios for the ticket are batched through each phase together, with no per-scenario inner loop.

Note: a behavioral-change ticket may *also* include a Legacy Coverage section; that's orthogonal — see [Legacy Coverage](#legacy-coverage) below.

## Structural Change

All structural cycles are governed by the rule **existing AC must stay green** locally before the final ticket commit. CI's **Acceptance Stage** is the post-commit verifier but it is **human-watched, not agent-watched**. Agents are CI-unaware and never advance a ticket past **TICKET STATUS - IN ACCEPTANCE**.

A **structural change** is a change that produces **no change-driven acceptance criteria**. Task tickets carry one of three `subtype:*` labels — `subtype:system-interface-redesign`, `subtype:external-system-interface-redesign`, or `subtype:system-implementation-change` — each routing to a structural cycle. The structural change still flows through a cycle — `subtype:system-interface-redesign` and `subtype:external-system-interface-redesign` enter the **Driver Adapter Cycle** (`da_cycle`), and `subtype:system-implementation-change` enters the **System Under Test Cycle** (`sut_cycle`) — but each cycle has no RED/GREEN per scenario. After the structural change is implemented, the BPMN orchestrator handles the rest. The `subtype:external-system-interface-redesign` path routes through the Contract Test Sub-Process.

Note: a structural-change ticket may *also* include a Legacy Coverage section; that's orthogonal — see [Legacy Coverage](#legacy-coverage) below.

## Legacy Coverage

**Legacy Coverage** is orthogonal to behavioral/structural classification. It is a **section in the ticket schema**, optional on any ticket type (Story / Bug / Task — and on Task tickets, regardless of `subtype:*` label). The section lists retroactive AC scenarios for previously uncovered functionality the change touches.

Legacy Coverage uses the **test-last** approach: tests are written retroactively for already-built behavior, and they should pass on first run because the behavior already exists. **This is NOT ATDD** — there is no Red → Green per scenario. A ticket whose schema carries a Legacy Coverage section routes through the **Legacy Coverage Cycle**, regardless of ticket type.

When a ticket carries both a change-driven payload (Story / Bug AC, or a structural change from any of the three Task subtypes) *and* a Legacy Coverage section, the Legacy Coverage Cycle runs first, then the AT Cycle (if applicable) — fill the coverage gap before piling new behavior on top.

## Interface Change

An **interface change** is any modification to a public contract between layers. This includes:

- Adding, removing, or renaming interface methods
- Changing method signatures (parameters, return types)
- Adding, removing, or renaming fields in request or response DTOs associated with those methods

This definition applies uniformly to DSL port interfaces, Driver port interfaces, and external system interfaces.

In the intake-classification sense, an **interface change** is specifically a change at the **system boundary** — a system-side channel (API, UI, mobile, CLI, admin, …) or an external-system API. The two interface-change `subtype:*` labels (`subtype:system-interface-redesign`, `subtype:external-system-interface-redesign`) cover the system-side and external-system cases respectively. Driver *implementations* update to match the new interface; driver *interfaces* stay the same so existing acceptance tests still pass through them. The system-side subtype is channel-agnostic — the WRITE agent reads the ticket's Checklist plus the system tree to figure out which driver(s) to modify (no pre-classified channel field).

**Why it matters for the ATDD pipeline:**
- A DSL interface change → update DSL port and implementation
- A Driver interface change → update driver port and adapters
- An external system interface change (any change under `driver-port/.../external/`) → triggers the contract test subprocess (see the `ct-*.md` per-phase docs)
- A system-side interface change labelled `subtype:system-interface-redesign` → routes through the Driver Adapter Cycle's `structural_cycle` path: update the relevant System Driver(s); then the BPMN orchestrator handles the rest.
- An external-system interface change labelled `subtype:external-system-interface-redesign` → routes through the Driver Adapter Cycle's Contract Test Sub-Process path (per-phase RED/GREEN inside CT).

For both interface-change subtypes, driver bodies adapt to the new boundary interface (see [Structural Change](#structural-change) above for the existing-AC / Acceptance-Stage rule). If the ticket additionally carries a Legacy Coverage section, the Legacy Coverage Cycle runs first.

## Internal-only Change

An **internal-only change** is a change inside the system that does not modify any boundary — no system-side channel change, no external-system API change. Examples: refactor a class, rename, dependency upgrade. Drivers are untouched. Internal-only changes are labelled `subtype:system-implementation-change`; they route to the **System Under Test Cycle** (`sut_cycle`): implement the change, then the BPMN orchestrator handles the rest. See [Structural Change](#structural-change) above for the existing-AC rule. If the ticket additionally carries a Legacy Coverage section, the Legacy Coverage Cycle runs first.

## Legacy Coverage Cycle

The **Legacy Coverage Cycle** is the **test-last retroactive-AC cycle**. It is reachable from any ticket (Story / Bug / Task) whose body carries a [Legacy Coverage](#legacy-coverage) section. Because the behavior already exists, the retroactive acceptance tests written in this cycle should pass on first run; this is **not ATDD** (no Red → Green per scenario).

Task tickets enter the matching structural cycle by `subtype:*` label — `subtype:system-interface-redesign` or `subtype:external-system-interface-redesign` → **Driver Adapter Cycle** (`da_cycle`); `subtype:system-implementation-change` → **System Under Test Cycle** (`sut_cycle`). All structural paths have no RED/GREEN per scenario (see [Structural Change](#structural-change) above for the existing-AC rule). All cycle phases are now defined; see `cycles.md` for the prose and the rendered [process-flow diagram](https://github.com/optivem/gh-optivem/blob/main/docs/process-flow-diagram.md) (hosted in `gh-optivem`) for the full flows. The Legacy Coverage Cycle's own internal phases are TBD.

## Ticket Status - In Acceptance

The maximum ticket status any agent ever sets. After the final commit of a ticket, the BPMN orchestrator ticks any checklist items completed and moves the ticket to **IN ACCEPTANCE**. The agent is then done. Pipeline-watching, fix-loops on red CI, and the move from IN ACCEPTANCE to DONE are human responsibilities — agents are CI-unaware.

## `shop/` Package vs `shop` Repository

ATDD content uses the word **shop** in two distinct ways. They look similar but mean different things:

- **`shop/` (with trailing slash)** — a package/folder convention inside the testkit's driver layer (e.g. `driver-port/.../shop/api`, `driver-adapter/.../shop/ui`). This is the SUT-internal driver namespace, paired with `external/` (drivers for external systems). The name is part of ATDD doctrine and is **not** the repo name. Do not rename it.
- **`shop` (no slash, used in repo context)** — the repository name of the system under test.

The two uses are kept textually distinct (slash vs. no-slash) so they can be reasoned about independently.

In code paths the literal folder name (`shop/`, `myShop/`, etc.) comes from the `${sut_namespace}` placeholder, sourced from `system.sut_namespace` in `gh-optivem.yaml` (defaulting to the last path segment of `system.repo`). The driver layout fragments around it — `${driver_port}`, `${driver_adapter}`, `${external_driver_port}`, `${external_driver_adapter}` — come from named-location placeholders under the top-level `paths:` block in the same config file. Phase docs use these placeholders so the example paths in this glossary stay literal (they teach the convention) while every other phase doc renders concrete project paths at sync time.
