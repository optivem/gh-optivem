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

## Fix — resolve identity from the external-driver-**port** change, not the adapter files

Identity must come from the **cycle trigger** — the external-driver-**port** change set
— which is present and unambiguous on both method-changing and DTO-only changes. The
adapter-file scan stays only as a secondary source so the existing method-changing path
is unaffected.

**Recommended mechanism (deterministic, no new agent contract):** preserve the
external-driver-port changed-path list at the moment the port-change verdict is landed,
then have `IDENTIFY_EXTERNAL_SYSTEM` resolve the name from it (port first, adapter
fallback).

- `validate-outputs-and-scopes` already computes `phase-changed-files` and lands the
  agent's `external-driver-port-changed` verdict (`bindings.go` ~1244–1293). When it
  lands that verdict **true**, it also resolves the `external-system-driver-port` root
  (`ResolveLayerPaths`) and stashes the subset of `phase-changed-files` under that root
  into a durable shared-state key (working name `external-driver-port-changed-paths`).
- `identifyExternalSystem` derives `name` from the union of: that preserved
  port-path list **and** the current adapter files under the
  `external-system-driver-adapter` root. The existing `<root>/<name>/...` first-segment
  extraction, the 0 / 1 / >1 switch, and the registry lookup (`cfg.ExternalSystems`,
  setting `external-system-name` + `real-kind`) are reused verbatim. The
  zero-names error remains as the genuine "no external system touched at all" stop.

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

- [ ] **2. Resolve identity from port paths first, adapter files as fallback.**
  In `identifyExternalSystem` (`bindings.go:546`), build the candidate `name` set from
  the union of the preserved port-path list (Item 1) and the existing
  `external-system-driver-adapter` scan of `phase-changed-files`. Keep the
  `<root>/<name>/...` segment extraction, the 0 / 1 / >1 switch, the registry lookup,
  and the `external-system-name` + `real-kind` stamping unchanged. Refresh the
  function's doc-comment (`bindings.go:533–545`) to state identity comes from the
  port change (trigger), not the adapter artifact.

- [ ] **3. Update / add unit tests.**
  In `internal/atdd/runtime/actions/bindings_test.go`: add a case proving a **DTO-only**
  external-driver-port change (port file changed under `driver/port/external/erp/**`,
  **zero** adapter files) resolves `external-system-name=erp` + the registry
  `real-kind`. Keep an existing case proving the method-changing path (adapter files
  present) still resolves identically. Add a case proving the genuine
  "no external system touched" path still yields the zero-names hard error, and that the
  >1-system error still fires. Confirm the `namespacedLandingKeys` /
  `external-driver-port-changed` namespacing invariants (lines ~1339–1394) still hold.

- [ ] **4. Doc-block / BPMN comment sync.**
  Update the `IDENTIFY_EXTERNAL_SYSTEM` node comment in
  `internal/atdd/runtime/statemachine/process-flow.yaml` (~1123–1135) so it states
  identity is resolved from the external-driver-**port** change (robust to DTO-only
  changes), not from "the driver-adapter files the step above just wrote." No
  sequence-flow or node-shape change — content only.

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

- **Shared-state threading.** The fix relies on `ctx.State` persisting from the AT DSL
  phase into the nested contract call-activity (it does — one `Context` per run; the
  `at-`/`ct-` namespacing exists *because* state is shared). The non-empty-only guard
  (Item 1) is what prevents the no-op CT DSL phase from clobbering the list.
- **No diagram regen step** — the regenerate-diagram workflow auto-rebuilds
  `docs/process-diagram.md` on push (`feedback_plans_no_diagram_regen.md`). Item 4 only
  edits the YAML node comment.
- **Scope discipline** — this is a black-box-preserving robustness fix to identity
  resolution; do **not** fold in unrelated port-changed or real-kind reshaping.
