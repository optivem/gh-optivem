---
# GREEN-stage production code to make failing acceptance tests pass. Opus medium covers the common-layer + adapter implementation on the first channel (trialling medium down from high — see if GREEN still passes at lower cost).
# model is the first-channel (common:true) tier: that dispatch builds the shared common layer + forward-only migration, so it stays on opus. model-later-channel is the tier for every later channel (common:false), which does only the per-channel adapter delta hard-gated by acceptance-${channel} — routed to sonnet (resolveDispatchModel, driver.go). To disable the downgrade, delete the model-later-channel line; to retune it, change its value. No rebuild of routing logic needed.
model: opus
effort: medium
model-later-channel: sonnet
---
The implement-system task writes production code under the system surface (`${system-surface}`) to make the failing acceptance tests pass for one delivery channel — the `${channel}` channel.

Architecture: ${architecture}

## Inputs

### Scope

${scope-block}

### Parameters

- `architecture` — architecture profile for the target project (Java/.NET/TS × monolith/multitier).
- `channel` — the delivery channel this dispatch must green (e.g. `api`, `ui`). The acceptance run is scoped to this channel's suite (`acceptance-${channel}`), so you only need to satisfy the `${channel}` slice of the failing test.
- `common` — whether this is the first channel of the cycle. `true` → build the channel-agnostic **common** layer plus the `${channel}` adapter; `false` → the `${channel}` adapter delta only (the common layer already landed in an earlier channel's dispatch).

## Steps

1. Read the failing Acceptance Test (`${at-test}`) to see the required behaviour, then trace through the DSL Port (`${dsl-port}`) and DSL Core (`${dsl-core}`) to the System Driver port/adapter pair (`${system-driver-port}`, `${system-driver-adapter}`) to see how the test reaches the production system. If the test stages stub external interactions, also read the External System Driver port/adapter pair (`${external-system-driver-port}`, `${external-system-driver-adapter}`) and the Contract Tests (`${ct-test}`) to see the stub contract the implementation must satisfy.
2. Do the simplest implementation possible under the system surface (`${system-surface}`) that greens the `${channel}` acceptance slice. Scope the work by the `common` flag:
   - **`common: true`** (first channel of the cycle): implement the channel-agnostic **common** layer — the DTO / entity / service logic the behaviour needs, shared across every channel — **and** the `${channel}` adapter that exposes it through this channel.
   - **`common: false`** (a later channel): implement **only** the `${channel}` adapter delta that wires this channel to the already-built common layer. Do not re-touch the common layer or its migration — they landed in the first channel's dispatch and are verified by their own commit.
3. When the AT asserts persisted state (a column read/written, an audit-log entry, a soft-delete tombstone, etc.) **and** `common: true`, also add a schema migration under the shared migration set (`${system-db-migration-path}`) — a single timestamped SQL file in the Flyway naming convention (`V{YYYYMMDDHHMMSS}__{description}.sql`, forward-only, no undo). The migration belongs to the common layer, so it is authored once on the first channel and not repeated on later channels. Read the existing migrations first to see the current schema; do not redeclare columns that already exist. The migration set is shared across every SUT (3 languages × 2 architectures); your one file is consumed by all of them.
4. **Self-verify by running your `${channel}` acceptance slice — the run is the gate, not your own judgment.** Compile-green is not done; a passing `acceptance-${channel}` run is. After implementing, converge your slice to green against a real test run:

   1. Build, (re)start the system, then run **only** your channel's slice:
      - `gh optivem system build`
      - `gh optivem system start --restart`
      - `gh optivem test run --suite=acceptance-${channel}`
   2. Read the outcome and act on its kind — distinguish an **infra** failure from a **test** failure:
      - **Build or start failure (infra):** the system did not compile or did not come up (a bad migration, a broken wiring, a startup error). This is an error in your own production code or migration, **not** a test verdict — fix the cause and re-run from the build step.
      - **Test failure:** the slice ran and one or more scenarios are red (e.g. `Order line for SKU 'DELL-XPS' not found`). Read the failure, form a hypothesis about which production code path under the system surface (`${system-surface}`) is wrong or missing, fix it, and re-run the trio.
      - **All green:** the `${channel}` slice passes — you are done; exit cleanly.
   3. Repeat until the slice is green or the stop rule below fires.

   **Completeness hint (a heuristic to guide your first implementation — no longer the gate):** while implementing, it helps to walk each scenario in the Acceptance Test (`${at-test}`) and name the concrete production code path that produces every asserted value (a computed rate, a subtotal, an independent per-line result, a persisted column), and to remember that a schema migration greens nothing on its own — a column needs a **write** path that populates it, a **read** path that surfaces it, and the **business rule** that derives its value. Use this to avoid stubbed, hard-coded, or merely-compiling paths that will not survive the run. But the done-signal is the acceptance run, not this checklist.

   **Stop rule (bound the loop).** Iterate only while you are making progress. Stop and exit — reporting the still-red slice and your best diagnosis of the remaining failure — when either condition holds:
   - your dispatch budget is nearly spent (budget-primary), or
   - the slice has stalled: two consecutive runs produce no newly-passing scenario and no reduction in the failing set (no progress) — the same signal the orchestration's `fix-loop-progressing` guard watches for.

   Do not spin past these. An honest not-green exit is correct: a genuinely stuck slice is handed to the bounded outer verdict, which re-confirms independently and escalates to the human-gated fixer. The `clauderun` standard dispatch ceiling (token/time) is the hard backstop — you do not need, and must not invent, a bespoke per-run cap.
