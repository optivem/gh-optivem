# Plan: bring `docs/atdd-at-cycle.md` to parity with internal/assets — Part 1: Cycle architecture & §Conventions

**Date:** 2026-05-16 (split into Part 1 / Part 2 / Legacy on 2026-05-18 during refinement)
**Context:** The goal is to eventually delete `internal/assets/`. `docs/atdd-at-cycle.md` is intended to become the canonical home for the AT cycle process spec, replacing the four global process pages under `internal/assets/global/docs/atdd/process/at-{red,green}-*.md`.

This is **Part 1** of three sibling plans created during refinement on 2026-05-18:

- **Part 1 (this file):** Cycle architecture and §Conventions. Establishes the normative schemas (disable-reason, phase-output flags, phase scope policy) and the doc-side items (1–4b) that wire them into `atdd-at-cycle.md`. Self-contained; can execute first.
- **[Part 2 — per-phase content](20260518-1116-atdd-at-cycle-part2-per-phase-content.md):** Phases 2–6 of the original plan (per-phase rules, framing, examples, cross-refs, mechanical fixes). Independent of Part 1; can run in parallel or after. **Not yet refined.**
- **[Legacy coverage cycle plan](20260518-1116-legacy-coverage-cycle.md):** Defines `docs/legacy-coverage-cycle.md` (sibling top-level cycle to AT) and its AT-side updates (boundary statement + "failing legacy = STOP" guardrail). Defines the legacy marker convention that extends §Conventions. **Not yet refined.**

**Source:** Gap analysis in [reports/atdd-at-cycle-gap-analysis.md](../reports/atdd-at-cycle-gap-analysis.md).

## Conventions

Normative schemas introduced or relied on by this plan. Other items reference these by anchor; cross-plan references (e.g. from the BPMN orchestration plan and the sibling-cycle plans — see Phase 7) point here too.

**Scope of these conventions:** designed to apply to **every cycle and sub-cycle** (AT, CT, Legacy, Structural), not just AT. This plan happens to introduce them while bringing `atdd-at-cycle.md` to parity, but sibling-cycle plans will reference the same schemas. If a future plan needs to hoist §Conventions to a shared cross-cycle location, that's a follow-up — not in scope here.

### Disable-reason convention

Change-driven tests are disabled between RED sub-phases with the following annotation reason:

```
@Disabled("<TICKET-ID> - AT - <LOOP> - <PHASE>")
```

- **Separator:** ` - ` (space-hyphen-space) between every segment.
- **`<TICKET-ID>`:** verbatim from the tracker (e.g. `OPV-123`, `#42`, `SHOP-7`). Leads so the re-enable step can filter `startsWith("<TICKET-ID> - ")` and ignore tests belonging to other tickets.
- **`AT`:** the cycle (Acceptance Test). Reserves the slot for `CT` (Contract Test) under the same convention later.
- **`<LOOP>`:** `RED` | `GREEN`. Currently only `RED` uses disable; the slot is reserved for schema regularity.
- **`<PHASE>`:** `TEST` | `DSL` | `SYSTEM DRIVER` (uppercase; internal space allowed).

Examples:
- `@Disabled("OPV-123 - AT - RED - TEST")`
- `@Disabled("OPV-123 - AT - RED - DSL")`
- `@Disabled("OPV-123 - AT - RED - SYSTEM DRIVER")`

Re-enable filter (used by the BPMN re-enable step at the start of the next phase):

```
startsWith("<CURRENT-TICKET-ID> - AT - RED - <PREV-PHASE>")
```

Never strip annotations whose prefix belongs to a different ticket.

> **Cross-plan note:** the [legacy coverage cycle plan](20260518-1116-legacy-coverage-cycle.md) tightens this convention with an explicit "applies only to change-driven scenarios; never to legacy" domain restriction and adds a sibling marker convention for legacy tests.

### Phase-output flags

After RED-DSL, the work-agent MUST set both flags below. They are read by the BPMN gateway downstream of RED-DSL to branch onto the right next phase; the gateway treats *unset* as an error (no implicit default).

| Flag name | Domain | Read by | Meaning when `yes` |
|---|---|---|---|
| `System Driver Interface Changed` | `yes` \| `no` | BPMN gateway after RED-DSL | RED-SYSTEM-DRIVER phase must run (new System Driver methods need real impls) |
| `External System Driver Interface Changed` | `yes` \| `no` | BPMN gateway after RED-DSL | Hand off to the CT cycle (external driver belongs to the CT sub-process) |

### Phase scope policy

**Every phase agent operates within a declared scope — no exceptions.** The table below is the complete source of truth: every phase has a row, and every agent's prompt is constructed with its row's allowed paths injected automatically (see Phase 7 BPMN bullet for the prompt-construction step). An agent without a declared scope is a configuration bug, not a default-allow.

Two layers enforce the policy; both converge on the same user-facing prompt — they differ only in who noticed the out-of-scope edit first.

- **Layer 1 — agent-triggered (in-agent recognition, BPMN-handled prompt):** the work-agent's prompt names the allowed paths for its phase. When the agent recognises it needs to edit out of scope, it does **not** wait inline for approval (per the Phase 7 "no approval inside the agent" rule). Instead, it exits with a structured *scope-exception-requested* signal naming the intended out-of-scope file(s) and the reason. BPMN sees the signal and runs the same human-task prompt as Layer 2.
- **Layer 2 — BPMN post-phase scope check (catches what Layer 1 missed):** after each phase agent finishes normally, BPMN runs a scripted step that diffs the modified files (`git diff --name-only` vs the pre-phase ref) against the allowed-path policy. On violation, BPMN halts and runs the same human-task prompt.

In both cases, BPMN never auto-allows and never auto-reverts — the user always decides. Options:

- **Accept (continue from current phase)** — the agent's out-of-scope change is judged correct (e.g. RED-SYSTEM-DRIVER discovered the DSL or driver-port interface was wrong; GREEN discovered the test was wrong). Record the exception and continue from the current phase.
- **Rewind to upstream phase** — accept the out-of-scope change, then restart the cycle from the phase whose output was wrong (e.g. accept a DSL edit made during RED-SYSTEM-DRIVER, then rerun RED-DSL to re-validate the corrected DSL, then continue). This is the most principled response when the violation reveals an upstream bug — it preserves the per-phase RED guarantee instead of carrying an unvalidated upstream change forward.
- **Revert + rerun** — discard the out-of-scope changes and rerun the current phase agent.
- **Abort** — stop the cycle, escalate to human review.

Allowed-path policy by phase:

| Phase | Allowed paths |
|---|---|
| RED-TEST | acceptance test files; DSL prototype stubs (interface + `"TODO: DSL"` throw) |
| RED-DSL | DSL Core impls; driver-port interface declarations |
| RED-SYSTEM-DRIVER | `${driver_port}/${sut_namespace}/` and `${driver_adapter}/${sut_namespace}/<channel>` |
| GREEN | production system code only; tests/DSL/drivers are frozen (see item 3) |
| CT-RED-TEST / CT-RED-DSL / CT-RED-EXTERNAL-DRIVER / CT-GREEN-STUBS | `external/**` only |

This table is the source of truth for the policy schema; the BPMN scope-check step loads it at runtime.

> **Refined 2026-05-18:** Added §Conventions as a standalone normative section, now with three sub-conventions: disable-reason, phase-output flags, phase scope policy. **Why:** all three schemas are referenced by items in this plan AND will be referenced by the separate BPMN plan (Phase 7 bullet) — they deserve a stable named anchor rather than living inside items that may be split/renumbered. The scope policy formalises a defence-in-depth guardrail (agent-side hint + BPMN-side check + user escalation on violation) so a misbehaving agent can't silently break the freeze or file-scope rules; the "accept" path acknowledges legitimate cases like "GREEN discovered the test was wrong".

## Phase 1 — Critical mechanics (load-bearing; the cycle breaks without them)

Without these, an agent following `atdd-at-cycle.md` literally would produce wrong output.

1. **Document the disable/re-enable convention; do not embed it in phase steps.** `atdd-at-cycle.md` mentions, once, that change-driven tests are disabled between RED sub-phases using the schema defined in §Conventions → *Disable-reason convention*, and re-enabled at the start of the next phase. It points to the BPMN process diagram for the mechanism. **This is BPMN orchestration plumbing** — handled by a dedicated, cheap (scripted / Haiku) step around the commit, not inside the phase agents. **Precondition:** the work-agent must have observed the test fail (RED proof) before the disable step runs. **Dependency:** Phase 7 bullet on the BPMN-side work (separate plan file).

   > **Refined 2026-05-18:** Reframed from "add disable/re-enable mechanism to each phase" → "document the convention once; mechanism lives in BPMN as a separate cheap/scripted step". Schema details hoisted to §Conventions. **Why:** the motivation is committing — so commits don't fail the pipeline — which is BPMN's concern, not the phase agents'. Disabling is mechanical bookkeeping done by orchestration plumbing, not phase work. Hoisting the schema keeps item 1 focused on the doc change and gives the BPMN plan a stable anchor to reference.

2. **Make the RED-DSL output flags explicit + gated.** The RED-DSL agent MUST set both flags defined in §Conventions → *Phase-output flags* (`System Driver Interface Changed`, `External System Driver Interface Changed`) before completing the phase. The BPMN gateway downstream of RED-DSL validates both flags are set and branches on their values; an unset flag is treated as a hard error, not a default `no`. `atdd-at-cycle.md` states the requirement plainly ("MUST be set; unset is a bug") and points to §Conventions for the schema and to the BPMN diagram for the gateway behaviour.

   > **Refined 2026-05-18:** Layering resolved: the work-agent owns *setting* the flags (it has the local information about what changed); BPMN owns *validating they were set* and *branching on values*. Schema hoisted to §Conventions for cross-plan reference (the BPMN plan needs it for the validation gate). **Why:** asking BPMN to derive these flags would force it to re-inspect the agent's diff, duplicating work the agent already did. But the gating semantics (error-if-unset, what each value triggers) belong to orchestration. Same split as item 1: doc names the schema; mechanism lives in BPMN.

3. **Add "tests/DSL/Drivers frozen in GREEN" rule** + escalation: if GREEN can't pass without touching them, ask the user (don't patch around).

   > **Refined 2026-05-18:** Kept as-is. **Why:** confirmed in walk-through as genuine GREEN-phase behavioural guidance — not orchestration plumbing like items 1–2. The freeze blocks the agent's escape hatch of loosening tests/DSL/Drivers to make GREEN pass; the escalation is the safety valve signalling "real bug in an earlier RED phase". Belongs in `atdd-at-cycle.md` as actual GREEN-section content, not hoisted to §Conventions or the BPMN plan.

4. ~~**Add file-scope constraint to RED-SYSTEM-DRIVER.**~~ — **removed.**

   > **Refined 2026-05-18:** Deleted. **Why:** with the universal scope policy in §Conventions (every phase has a row), the BPMN scope-check (allowlist-based, halts on anything outside the row), and the escalation flow (Accept / Rewind / Revert+rerun / Abort) all in place, RED-SYSTEM-DRIVER no longer needs its own per-phase item. The original wording included anti-targets (`external/`, `system/`) as "don't touch" callouts; we dropped those because (a) they're redundant with the allowlist (anything not allowed is forbidden by construction), (b) enumerating anti-targets per phase is a guessing game — you always miss the next plausible-wrong-place an agent drifts to, and (c) the source material (internal/assets) used denylist phrasing only because it didn't have the allowlist + check + escalation pattern. The underlying "doc must mention scope somewhere" need is promoted to item 4b below.

4b. **Add a top-level scope-policy mention to `atdd-at-cycle.md`.** Single intro line (or short paragraph) near the top of the doc: *"Every phase agent operates within an allowed-path scope; see §Conventions → Phase scope policy for the per-phase table, and the BPMN process diagram for how it's enforced."* No per-phase repetition needed.

   > **Refined 2026-05-18:** New item, promoted from the deleted item 4. **Why:** without an explicit doc-side item, the universal scope-policy principle (now in the plan's §Conventions) would never actually land in `atdd-at-cycle.md` — it would stay implicit and an executor of this plan might miss it. One top-of-doc mention is enough; per-phase repetition would just be noise once readers know to consult §Conventions.

## Phase 7 — NOT in this file (flagged as related work)

- **BPMN orchestration work** (dependency for items 1, 2, 4b — and items in [Legacy plan](20260518-1116-legacy-coverage-cycle.md)): the BPMN ATDD process needs new pieces that the doc reframes assume exist:
  - **Disable/enable steps around the commit** (item 1) — mark change-driven tests `@Disabled` after each RED sub-phase per §Conventions → *Disable-reason convention*, and re-enable them by ticket-prefix at the start of the next phase. Cheap implementation (scripted, or Haiku at most).
  - **Post-RED-DSL gateway** (item 2) — validate both flags from §Conventions → *Phase-output flags* are set (error if unset), then branch onto RED-SYSTEM-DRIVER and/or the CT cycle based on their values.
  - **Post-phase scope check** (item 4b — and any phase with a scope rule) — after each phase agent finishes, diff modified files against §Conventions → *Phase scope policy*; on violation, halt and prompt the user with the four options (Accept / Rewind to upstream phase / Revert + rerun / Abort). Pure scripted check (no LLM); user prompt is BPMN's standard human-task pattern.
  - **Failing-legacy detector** (defined in [Legacy plan](20260518-1116-legacy-coverage-cycle.md)) — same shared sub-process pattern as the scope check.
  - **Shared "Run Phase Agent" call activity** wrapping all of the above so every phase reuses the same envelope (load scope → inject into prompt → run agent → handle scope-exception signal → post-phase scope check → post-phase legacy check → escalation).

  Per the standing "new plan, never extend an existing one" rule, this is tracked in a separate plan file: `plans/<YYYYMMDD>-<HHMM>-atdd-bpmn-orchestration.md` (to be drafted), cross-referenced from here.

  > **Refined 2026-05-18:** Bullet added (item 1), then extended for the post-RED-DSL gateway (item 2), then again for the post-phase scope check, then again for the shared call-activity envelope. **Why:** each doc reframe introduced a real BPMN dependency this plan would otherwise leave invisible; grouping all the orchestration work under one BPMN plan keeps it coherent. The scope check is the enforcement layer for §Conventions → *Phase scope policy*; the Rewind-to-upstream-phase escalation preserves the per-phase RED guarantee when a downstream phase reveals an upstream bug; the shared call activity means adding a new phase = adding a §Conventions row + one BPMN call, with consistency by construction.

- **Sibling top-level cycles** (router-dispatched alongside AT; each gets its own plan file):
  - **[Legacy Coverage Cycle](20260518-1116-legacy-coverage-cycle.md)** — backfills retroactive acceptance tests (and external-system contract tests) driven by **legacy acceptance criteria** in the ticket. **Inverted RED-GREEN shape:** tests should pass on first run (the behaviour already exists); if they don't, the test is probably wrong → revise. Plan file created; not yet refined.
  - **Structural Cycle** — refactor / restructure work with no behavioural change. "Behaviour preserved" is the gate; no fail-first RED. Plan file: `plans/<YYYYMMDD>-<HHMM>-structural-cycle.md` (to be drafted).
  - **Cycle Router / Dispatcher** — upstream BPMN step that reads the ticket's acceptance criteria and dispatches to the appropriate top-level cycle(s). A single ticket may route to multiple cycles concurrently or sequentially (e.g. legacy AC + change-driven AC). Plan file: `plans/<YYYYMMDD>-<HHMM>-cycle-router.md` (to be drafted).

  > **Refined 2026-05-18:** New bundle. **Why:** the AT cycle is one of N top-level cycles, not the only one. The router dispatches by acceptance-criteria type (change-driven → AT, legacy → Legacy, refactor → Structural; CT remains a sub-cycle of AT, invoked from AT-RED-EXTERNAL-SYSTEM-DRIVER). Each sibling cycle deserves its own plan; signposting them here prevents their existence from being forgotten as we focus on AT.

- **CT-cycle parity work** (sub-cycle of AT, invoked from AT-RED-EXTERNAL-SYSTEM-DRIVER): `atdd-ct-cycle.md` likely has the same gaps vs its four internal CT pages (ct-red-test, ct-red-dsl, ct-red-external-driver, ct-green-stubs). Worth a parallel gap analysis.

- **Runtime prompt content** (compile-fix retry policy, batch-edits hint, "no approval inside agent", model/effort): these are agent-operational, not process-spec. They belong in the prompt files. The prompt files themselves are a separate migration concern — if `internal/assets/runtime/` is going away, those need a new generation mechanism, not relocation into `docs/`.

- **Supporting docs migration** (architecture/, language-equivalents/, glossary.md, testkit-*, placeholders.md, cycles.md, task-and-chore-cycles.md, system-interface-redesign.md, diagram-phase-details.md): 22 files in `internal/assets/global/docs/atdd/` that aren't process pages. Each needs a `docs/` home decided before internal/assets can be deleted.
