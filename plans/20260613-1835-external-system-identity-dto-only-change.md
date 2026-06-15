# Fix: `IDENTIFY_EXTERNAL_SYSTEM` crashes on a DTO-only external-driver-port change

**Date:** 2026-06-13 (local)
**Trigger:** Rehearsal loop `#65 view-product-list` crashed (rc=1) — runtime error, not a test failure.
**Status:** Proposed — not yet picked up.

---

## Problem

The ATDD `implement-ticket` run for shop ticket **#65 "View product list"** crashed at
`IDENTIFY_EXTERNAL_SYSTEM` with:

```
identify-external-system: no external system identifiable from the changed
driver-adapter files under "system-test/java/.../testkit/driver/adapter/external"
— the implement step wrote no per-system files. Onboard the external system
(which declares its real-kind) and re-run
```

The crash bubbled all the way up:
`IDENTIFY_EXTERNAL_SYSTEM → IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS →
SHARED_CONTRACT → WRITE_AND_VERIFY_ACCEPTANCE_TESTS(_FAIL) →
CHANGE_SYSTEM_BEHAVIOR → IMPLEMENT_TICKET → main`, killing the whole ticket.

This is **not** a test/red-green failure and **not** a misconfigured shop. It is a
structural gap in the orchestration that will recur for **any** ticket whose only
external-system change is a **DTO field** (no new Port method).

## Root cause

The routing **into** the external-driver contract-test cycle is correct. The
dsl-implementer added a `name` field to `ReturnsProductRequest` under
`system-test/java/.../testkit/driver/port/external/erp/dtos/` and emitted
`external-driver-port-changed=true`. That verdict is right: it is **directory-keyed**
("any file under the external-driver-port path changed"), per
`feedback_port_changed_flags_directory_keyed.md`. A file under
`driver/port/external/erp/**` changed, so `GATE_EXTERNAL_DRIVER_PORTS_CHANGED` →
`true` and the cycle entered
`implement-and-verify-external-system-driver-adapters-contract-tests`. Correct.

The break is at the **identity** step. Inside that cycle:

1. `IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS` dispatches
   `external-system-driver-adapter-implementer`, which fills
   `TODO: External System Driver` **adapter** prototypes. The port change was
   **DTO-only** — *"added `name` to `ReturnsProductRequest` (no interface-method
   change)"* — so **no adapter prototype was scaffolded**, the existing adapter still
   compiles, and the implementer correctly **no-op'd** (it reported zero remaining
   `TODO: External System Driver` markers under `driver/adapter/external`).
2. `IDENTIFY_EXTERNAL_SYSTEM` (`actions.identifyExternalSystem`,
   `internal/atdd/runtime/actions/bindings.go:546`) resolves the target external
   system's `name` → `real-kind` by scanning **`phase-changed-files`** for paths under
   the **external-system-driver-adapter** root and taking the first path segment as
   the system name. But `phase-changed-files` only carries the *immediately preceding*
   phase's diff — the adapter phase, which wrote **nothing**. Zero names → hard error.

So identity is inferred from a **downstream artifact (adapter files)** that is
legitimately empty on a DTO-only change, even though the **upstream trigger** (the
external-driver-**port** change under `driver/port/external/erp/**`) carries the system
name `erp` unambiguously in the exact same `<root>/<name>/...` shape.

`erp` *is* registered in `gh-optivem.yaml` (`real-kind: simulator`, per
`project_bpmn_full_coverage_story_and_realkind_gap.md`), so had identity resolved, the
rest of the cycle (`PROBE_CONTRACT_REAL` → red → simulator branch) would have run
normally. The only defect is the identity source.

### Evidence (from `worktrees/rehearsal-20260613-180107-65-view-product-list.log`)

| Line | Fact |
|------|------|
| 1313 | dsl-implementer: *"External System Driver Port: added `name` to `ReturnsProductRequest` (no interface-method change)"* |
| 1317 | dsl-implementer emitted `external-driver-port-changed=true` |
| 1328 | changed file `.../driver/port/external/erp/dtos/ReturnsProductRequest.java` (DTO under the **erp** external port) |
| 2147 | `GATE_EXTERNAL_DRIVER_PORTS_CHANGED -> bool=true` (correctly routed in) |
| 2287 | implementer: *"there are no `TODO: External System Driver` prototypes remaining under .../driver/adapter/external"* → no-op |
| 2319 | `FAIL IDENTIFY_EXTERNAL_SYSTEM -> ... no external system identifiable ...` |

## Fix — resolve identity SOLELY from the external-driver-**port** change (incl. DTOs)

Identity is resolved from **one source only: the changed paths under the
external-driver-port root**, `…/driver/port/external/<name>/**` — which covers the driver
**interface** (port methods) **and its DTOs** (`…/<name>/dtos/…`). This is the
**directory-keyed** principle (`feedback_port_changed_flags_directory_keyed.md`): identity
keys on *any file under `port/external/<name>/**`*, not on "a method changed," so DTO-only
changes are covered with no special-casing.

**The adapter files are NOT consulted at all** — not as a fallback, not as a union member.
The original code's adapter scan was never a real second source; it was an accident of
IDENTIFY reading `phase-changed-files`, which at that point happened to hold the adapter
diff. The port change is the only source needed, because:

- The cycle's **sole entry** is `GATE_EXTERNAL_DRIVER_PORTS_CHANGED`
  (`external-driver-port-changed`). Entering the cycle therefore **guarantees** at least one
  changed file under `port/external/<name>/**`, so the preserved port-path set is **always
  non-empty and always carries `<name>`** when IDENTIFY runs.
- A **method** change lives *on the port interface*, so it is a port change too — the
  adapter is merely downstream of it. A **DTO** change is a port change by definition. An
  **adapter-only** change with no port change cannot enter the cycle (no routing path).
  → the adapter scan covers **zero unique cases**; port paths cover all of them.

The port change resolves identity for every shape, each via the same segment extraction:

1. **Port interface method** changed under `port/external/<name>/**` → `<name>`.
2. **Port DTO** changed under `port/external/<name>/dtos/**` → `<name>`.
3. **Method + DTO mix** (same or different systems) → the corresponding name set
   (one name collapses duplicates; two systems → the >1 error below).

**Mechanism (deterministic, no new agent contract).** Port + DTO changes happen in the
earlier AT-cascade DSL phase, so by IDENTIFY time `phase-changed-files` no longer carries
them — they must be **preserved**:

- **Preserve (Item 1).** `validate-outputs-and-scopes` already computes
  `phase-changed-files` and lands the agent's `external-driver-port-changed` verdict
  (`bindings.go` ~1244–1293). When it lands that verdict **true**, it also resolves the
  `external-system-driver-port` root (`ResolveLayerPaths`) and stashes the subset of
  `phase-changed-files` under that root into a durable shared-state key (working name
  `external-driver-port-changed-paths`), guarded to write only when non-empty.
- **Resolve (Item 2).** `identifyExternalSystem` builds the candidate `name` set from
  **that preserved port-path set alone**, via the existing `<root>/<name>/...` first-segment
  extraction. The 0 / 1 / >1 switch and the registry lookup (`cfg.ExternalSystems`, setting
  `external-system-name` + `real-kind`) are reused verbatim. The adapter scan of
  `phase-changed-files` is **removed**. The zero-names error remains the genuine "no external
  system touched at all" stop; the >1-names error now correctly fires when a ticket touches
  two systems' ports (e.g. a method on `erp` + a DTO on `clock`) instead of silently serving
  only the one that happened to get an adapter file.

**Why not the alternatives** (record the decision, don't reopen during execution):

- *Make IDENTIFY recompute via `git diff` against a cycle base.* The runtime's change
  tracking is **per-phase** (per-phase baseline fingerprint, `bindings.go:206`,
  `snapshotWorkingTree`); there is no cycle-wide base ref exposed to actions, and
  earlier phases commit as they go (committed paths drop out of `git status`). Adding a
  cycle-base diff is a larger, riskier mechanism than reusing the per-phase verdict.
- *Have the dsl-implementer emit `external-system-name` as an output.* Cleanest on
  paper but adds a new agent-output contract and depends on agent self-reporting, which
  `feedback_port_changed_flags_directory_keyed.md` shows is exactly what drifts.
  Path-derivation is deterministic and still validates against the registry.
- *Keep the adapter-file scan as a union member / fallback alongside the port paths.*
  Rejected — it covers **zero unique cases**. The cycle's sole entry gate
  (`GATE_EXTERNAL_DRIVER_PORTS_CHANGED`) is port-keyed, so entering the cycle guarantees a
  non-empty port-path set; method changes are themselves port changes; an adapter-only
  change cannot route in. Unioning the adapter scan back in would just re-import the exact
  accident that caused this bug (IDENTIFY reading `phase-changed-files`, empty on DTO-only
  changes) under the name "backup." Identity is resolved **solely** from the port paths.

### One decision to confirm before coding

**Namespacing of the preserved key.** `external-driver-port-changed` lands namespaced
as `at-…` / `ct-…` (`landingStateKey`, `namespacedLandingKeys`). The meaningful port
change here is the **AT-cascade** DSL phase; the nested contract excursion's own
(compile-only) DSL phase does not re-change the port. Decision: **stash the path list
only when the landed verdict is `true` and the resolved subset is non-empty**, so the
no-op CT DSL phase cannot clobber the AT phase's list. Whether the key itself is
cascade-namespaced (and IDENTIFY reads `at-…` then `ct-…`) or kept flat with the
non-empty guard is the implementer's call during Item 1 — both satisfy the guard;
prefer flat unless the `bindings_test.go` namespacing invariants force otherwise.

---

## Items

- [ ] **1. Preserve the external-driver-port changed-path subset on a true verdict.**
  In `validateOutputsAndScopes` (`internal/atdd/runtime/actions/bindings.go`), when the
  landed `external-driver-port-changed` verdict is `true`, resolve the
  `external-system-driver-port` root via `ResolveLayerPaths` and stash the subset of
  `phase-changed-files` under that root into shared state (working key
  `external-driver-port-changed-paths`), guarded to only write when non-empty. Add the
  doc-comment rationale (this plan) inline.

- [ ] **2. Resolve identity SOLELY from the preserved external-driver-port paths; REMOVE the adapter scan.**
  In `identifyExternalSystem` (`bindings.go:546`), build the candidate `name` set from the
  preserved port-path set (Item 1) **alone**, via the existing `<root>/<name>/...`
  first-segment extraction. **Delete** the current scan of `phase-changed-files` against the
  `external-system-driver-adapter` root (`bindings.go:551–572`) — adapter files are no longer
  a source. This covers (a) port interface method, (b) port DTO, (c) method+DTO mix, all via
  the port change. Keep the 0 / 1 / >1 switch, the registry lookup, and the
  `external-system-name` + `real-kind` stamping unchanged. Rewrite the function's doc-comment
  (`bindings.go:525–545`) to state identity comes from the external-driver-**port** change
  (trigger, incl. DTOs) — explicitly NOT from the adapter files — and that the cycle's
  port-keyed entry gate guarantees that source is always present.

- [ ] **3. Update / add unit tests.**
  In `internal/atdd/runtime/actions/bindings_test.go`:
  - **DTO-only port change** — port file changed under `driver/port/external/erp/dtos/**`,
    **zero** adapter files → resolves `external-system-name=erp` + the registry `real-kind`.
  - **Method port change** — port interface file changed under `driver/port/external/erp/**`
    → resolves `erp` identically. (Adapter files, if any, are irrelevant — the test must
    prove resolution works **with no adapter files in `phase-changed-files`**, since the
    adapter scan is gone.)
  - **Two systems** — port files changed under both `port/external/erp/**` and
    `port/external/clock/**` → the **>1-system hard error** fires.
  - **No external system touched** — empty preserved port-path set → the **zero-names hard
    error** still fires.
  - Rework/remove any existing case that relied on the **adapter** scan as the identity
    source (it no longer exists). Confirm the `namespacedLandingKeys` /
    `external-driver-port-changed` namespacing invariants (lines ~1339–1394) still hold.

- [ ] **4. Doc-block / BPMN comment sync.**
  Update the `IDENTIFY_EXTERNAL_SYSTEM` node comment in
  `internal/atdd/runtime/statemachine/process-flow.yaml` (~1123–1135) so it states
  identity is resolved **solely** from the external-driver-**port** change (robust to
  DTO-only changes), not from "the driver-adapter files the step above just wrote." Note
  that identity no longer depends on the adapter-impl phase's output, so the old "must run
  AFTER the driver-adapter impl" ordering rationale (for identity) no longer applies —
  but **do not move the node**; leave its sequence position unchanged (other GREEN-shape
  steps still sequence around it). Content-only comment edit; no sequence-flow or
  node-shape change.

## Verification

- [ ] `scripts/test.sh` (or `go test -p 2 ./internal/atdd/runtime/actions/...` and
  `./internal/atdd/runtime/statemachine/...`) green. **Do not** run unbounded
  `go test ./...` on Windows (`feedback_go_test_windows.md`).
- [ ] Re-run the rehearsal for ticket #65 (`scripts/atdd-rehearsal-loop.sh`, or a
  single `#65 view-product-list` run) and confirm it advances **past**
  `IDENTIFY_EXTERNAL_SYSTEM` into `PROBE_CONTRACT_REAL` (resolving `erp` /
  `real-kind: simulator`) instead of crashing. *(User-driven — not an agent Item.)*
- [ ] Spot-check that a method-changing external story (e.g. a `#72`-class shop that
  adds an external Port method) still resolves identity unchanged. *(User-driven.)*

## Risks / notes

- **Shared-state threading is now the single source** (no adapter backup). The fix relies on
  `ctx.State` persisting the preserved port-path set from the AT DSL phase into the nested
  contract call-activity (it does — one `Context` per run; the `at-`/`ct-` namespacing exists
  *because* state is shared). The non-empty-only guard (Item 1) prevents the no-op CT DSL
  phase from clobbering the list. Because identity is resolved **solely** from this set,
  failure to thread it would re-surface as a zero-names error — but the port-keyed entry gate
  guarantees the set is populated whenever the cycle runs, so this is sound, not fragile.
- **No diagram regen step** — the regenerate-diagram workflow auto-rebuilds
  `docs/process-diagram.md` on push (`feedback_plans_no_diagram_regen.md`). Item 4 only
  edits the YAML node comment.
- **Scope discipline** — this is a black-box-preserving robustness fix to identity
  resolution; do **not** fold in unrelated port-changed or real-kind reshaping.
