# Plan: Wire the task-subtype binding so task-kind tickets dispatch (Phase D)

> **CONTEXT (2026-06-06):** surfaced by the read-only `process-flow.yaml` drift audit (the sweep
> that produced the §1–§4 review plans). Not a fresh defect — it is the **known, intentionally
> deferred Phase-D wiring** for the two-level ticket gateway. The two-axis YAML structure + a
> `task-subtype` stub binding landed in commit `642fa78` ("split flat ticket-kind gateway into type +
> subtype (Item 11)"); the real binding was deferred. The originating plan `20260526-0832` (Item 11
> Q11.2) has since been retired — the deferral decision now lives only in the code comments
> (`gates/bindings.go:175-181`, `:549-559`) and git history.
>
> Independent of the in-flight §1a cover-path plan (`1518`): this is the `implement-ticket` TOP, a
> different process from the AT cover cascade — no file overlap with that work.

## Why

`implement-ticket` classifies tickets with a **two-level gateway** (`process-flow.yaml:271-340`):

- `GATE_TICKET_KIND` (binding `ticket-kind`) routes `story`/`bug` → `change-system-behavior`,
  `task` → `GATE_TASK_SUBTYPE`, else → `UNKNOWN_TICKET_KIND`.
- `GATE_TASK_SUBTYPE` (binding `task-subtype`) routes the 5 subtypes (`legacy-coverage`,
  `system-redesign`, `external-system-redesign`, `system-refactor`, `test-refactor`) → their cycles,
  else → `UNKNOWN_TASK_SUBTYPE`.

But the **bindings never caught up to that structure**:

1. `ticketKind` (`gates/bindings.go:510-547`) emits `story` | `bug` | **`task/<subtype>`** — it
   resolves the subtype *itself* (`Tracker.Subtypes`, validates exactly one in the closed set) and
   returns the **composite** value. It **never emits bare `task`**.
2. `taskSubtype` (`:554-559`) is a **stub**: it returns the preseeded `ctx.State["task-subtype"]` if
   present, else `Err`.

Consequences in a real run:

- The `ticket-kind == task` edge (`:333`) is **dead** — the binding never emits bare `task`.
- The composite `task/legacy-coverage` (etc.) the binding *does* emit matches **no** `GATE_TICKET_KIND`
  edge, so it falls straight to the `UNKNOWN_TICKET_KIND` catch-all.
- **Net: every task-kind ticket dead-ends at `UNKNOWN_TICKET_KIND`.** Story/bug dispatch end-to-end;
  none of the 5 task subtypes can dispatch at all. (And even if one reached `GATE_TASK_SUBTYPE`, the
  stub `taskSubtype` errors unless hand-preseeded.)

The YAML is the *correct, intended* side; the bindings are the lagging half of the Item-11 split.

## Design (chosen) — Option A: each gateway resolves its own axis

Make the bindings match the YAML the split already drew. The subtype-resolution logic **moves down
one axis** — out of `ticketKind`, into `taskSubtype` — verbatim:

- **`ticketKind`**: when `Tracker.Classify` says the native type is a task, emit **bare `task`**
  (stop calling `Tracker.Subtypes`, stop composing `task/<sub>`). `story`/`bug`/alias/`Classify`
  paths unchanged.
- **`taskSubtype`**: promote the stub to real. Keep the preseed short-circuit (tests / hand-debug);
  otherwise `issueFromContext(ctx)` → `Tracker.Subtypes` → exactly-one-in-set validation → emit the
  bare subtype (`legacy-coverage`, …). The "0 labels / 2+ labels / unrecognised label" `Err`
  messages move here too, re-pointed to the task-subtype axis.

This makes `taskSubtype` a structural mirror of `ticketKind`'s old task branch — minimal, symmetric,
and it preserves today's rich operator error messages. **No YAML topology change**: the two gateways
and all their edges are already drawn for exactly these emitted values; only the now-stale "Phase D
stub / deferred" prose is corrected.

**Rejected — Option B (collapse to one flat gateway on the composite `task/<sub>` value).** Would
delete `GATE_TASK_SUBTYPE` + the `taskSubtype` binding and revert the YAML to a single gateway
matching the binding's current output. Rejected: it discards the two-axis model that the YAML, the
operator-conceptual-model comment (`:229-236`), and Item 11 deliberately landed. The binding is the
side that lagged — fix the binding, not the design.

## Items

1. **`gates/bindings.go` — `ticketKind`.** In the `case "task"` branch (`:531-543`), return
   `Outcome{Value: "task"}`; remove the `Tracker.Subtypes` call, the exactly-one validation, and the
   `"task/" + sub` composition. Leave `story`/`bug`, the `feature→story` alias, `Classify`, and the
   no-native-type error intact. Update the godoc lookup table (`:497-509`) so task rows show
   `task → task` (subtype resolved on the downstream axis).
2. **`gates/bindings.go` — `taskSubtype`.** Replace the stub body (`:554-559`) with the real
   resolution: preseed short-circuit retained; else `issueFromContext(ctx)` → `Tracker.Subtypes` →
   exactly-one + set-membership check → `Outcome{Value: <subtype>}`. Move the `Err` cases (no native
   type is N/A here; 0 / 2+ / unrecognised subtype labels) down from `ticketKind`, with messages
   phrased against the task-subtype axis. Move ownership of `ticketKindTaskSubtypes` /
   `ticketKindTaskSubtypeSet` to this binding (rename to drop the now-wrong `ticketKind` prefix —
   names describe the subtype axis they now serve, per `[[feedback_no_layer_coding_in_names]]`).
   Update the godoc.
3. **`gates/bindings.go` — registration comment (`:175-181`).** Drop the "Stub binding for now /
   Phase D wires …" language; describe the live two-axis split (`ticketKind` → kind, `taskSubtype`
   → subtype).
4. **`process-flow.yaml` doc (`:229-236`).** Drop "The binding-side split lands in Phase D; this plan
   ships the YAML structure plus a `task-subtype` stub binding … End-to-end execution for task-kind
   tickets requires the Phase D wiring." Replace with present-tense prose: both axes are live and
   resolve from the tracker. **No node/edge change.** Keep it tight per `[[feedback_flag_non_token_efficient]]`.
5. **Tests** (`gates/bindings_test.go`, `statemachine/transitions_test.go`). Scope `go test` per
   `[[feedback_go_test_windows]]`.
   - `ticketKind` table (`:739-743`): task rows now `want: "task"` (subtype-independent); the preseed
     passthrough test (`:718-724`) fixture becomes `task`.
   - New `taskSubtype` table: the 5 subtypes resolve from a fake `Tracker.Subtypes`; the malformed
     cases (0 / 2+ / unknown label) return the expected `Err`; preseed short-circuit covered.
   - `transitions_test.go`: assert a task ticket now *routes past* `GATE_TICKET_KIND` to
     `GATE_TASK_SUBTYPE` and on to its cycle (e.g. `legacy-coverage` → `COVER_SYSTEM_BEHAVIOR`),
     **not** `UNKNOWN_TICKET_KIND`. Prefer `wantEdge`/routing assertions over a full walk
     (`[[feedback_statemachine_test_loop_hazard]]`).
   - Confirm the diagram humanizer test (`diagram/diagram_test.go:71,90`, the `task/cover-legacy`
     label cases) still holds — those exercise generic slash-label prettifying and are driven by YAML
     `when:` predicates (already `task` / bare subtypes), so should be unaffected; verify, don't assume.

## Verification

- Run a real task ticket of each subtype (or the closest the fixtures allow) and confirm it dispatches
  to its cycle instead of halting at `UNKNOWN_TICKET_KIND`.
- Confirm `story` / `bug` tickets still dispatch unchanged.
- Confirm a task ticket with **zero**, **multiple**, or an **unknown** `subtype:*` label surfaces a
  clear operator error from the task-subtype axis (re-labels and re-runs).

## Notes / out of scope

- **Catch-all reachability is *not* in scope.** Because both bindings `Err` on malformed input rather
  than emitting an out-of-set value, the `UNKNOWN_TICKET_KIND` / `UNKNOWN_TASK_SUBTYPE` error-end
  catch-alls are arguably belt-and-suspenders (the same property the §3b `GATE_REFACTOR_TYPE_CHOICE`
  plan documented). That predates this change for `UNKNOWN_TICKET_KIND`; do **not** add/remove those
  nodes here. If worth addressing, it is a separate §3b-style finding (`[[feedback_materialize_dont_expand]]`).
- `Tracker.Subtypes` already exists and is exercised by `ticketKind` today — `taskSubtype` reuses the
  same call; **no `Tracker` interface change**.
- No diagram-regeneration step (the regenerate-diagram workflow handles `docs/` on push;
  `[[feedback_plans_no_diagram_regen]]`). No YAML topology change means the diagram is unaffected anyway.
