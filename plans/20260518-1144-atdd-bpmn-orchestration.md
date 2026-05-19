# Plan: ATDD BPMN orchestration

> ✅ **REFINED 2026-05-18** — every item walked one-by-one; the plan now extends the existing Go BPMN runtime (`internal/atdd/runtime/statemachine/process-flow.yaml` + `gates/` / `actions/` / `agents/`) rather than introducing a new orchestration tool. Items 1 and 10 are strikethrough stubs (deleted with rationale); item 9 Part B (CT_GREEN_STUBS) is ⏳ Deferred pending stubs-ownership. No pre-execute blockers remain — the cross-cutting §Conventions `<channel>` edit landed in commit `82bb983`.

**Date:** 2026-05-18
**Context:** The BPMN-side orchestration work that the AT-cycle doc reframes (Part 1 items 1, 2, 4b) assume exists. Those doc items reference "the BPMN process diagram", a "post-RED-DSL gateway", a "post-phase scope check", and a "shared call activity". The BPMN orchestration **itself already exists** in this repo as a Go runtime: `internal/atdd/runtime/statemachine/process-flow.yaml` (BPMN-shaped spec with start_event / end_event / service_task / user_task / gateway / call_activity nodes + sequence_flows + predicates), executed by the Go state machine in `internal/atdd/runtime/statemachine/` with pluggable `gates/`, `actions/`, `agents/` registries, rendered as Mermaid by `internal/atdd/runtime/diagram/` (per [`docs/process-diagram.md`](../docs/process-diagram.md)). **This plan extends that existing runtime** with the missing gates, actions, and call_activity wiring — it does not introduce a new orchestration tool or artefact form.

**Sibling plans referenced:**
- [Part 1 — AT-cycle architecture & §Conventions](20260516-1701-atdd-at-cycle-absorb-internal-assets.md) — defines the §Conventions schemas this plan implements gates and steps against.
- [Part 2 — `atdd-at-cycle.md` per-phase content](20260518-1116-atdd-at-cycle-part2-per-phase-content.md) — independent doc-content work; no orchestration dependency.
- [Legacy coverage cycle](20260518-1116-legacy-coverage-cycle.md) — supplies the legacy marker convention needed by the failing-legacy detector below.
- [Phase-scope placeholders substrate (20260518-1500)](20260518-1500-atdd-phase-scope-placeholders.md) — defines the `at_test`, `dsl_port`, `dsl_core` Family B keys. Substrate prerequisite for the SSoT successor plan (which retires Snapshot A and rewires `check_phase_scope`).

**Source:** Phase 7 of Part 1 (the four BPMN bullets), plus the cross-plan reference in the legacy-coverage plan.

## Open questions (residual, after structural reframe)

> **Refined 2026-05-18:** Q1 (artefact form) and Q2 (runtime location) removed. **Why:** "follow exactly how we've done bpmn stuff up to now, it's in go" (user). The artefact is `internal/atdd/runtime/statemachine/process-flow.yaml` (BPMN-shaped YAML); the runtime is the Go state machine in the same package. No new tool, no new artefact form.

Residual questions — answered per item during the walk, not pre-resolved:

1. **What is the shared envelope's interface contract** with each phase agent?
   - How are allowed paths injected into the agent's prompt (env var? prepended block? template variable?).
   - How does the agent emit a *scope-exception-requested* signal back (exit code? structured JSON to stdout? a marker file?).
   - How does the agent report the two phase-output flags from RED-DSL (same channel as the scope-exception signal? separate?).
2. **Where do the human-task prompts live?** Each escalation prompt (scope violation, failing legacy, flag-unset, GREEN-can't-pass-without-touching-frozen-layer) needs a concrete UX — re-uses the existing `user_task: agent: human` STOP mechanism in the state machine, but the prompt content + option set still need defining.

## §Conventions snapshot (inlined to survive the upcoming split)

These are the §Conventions excerpts this plan depends on, snapshotted from `docs/atdd/process/shared/conventions.md` as of 2026-05-18. The user has flagged that `conventions.md` will be split; the snapshot below is the data items 3, 4, 5, 8, 9 consume — the plan is no longer load-bearing on the upstream file path.

**The executor's first task is to reconcile this snapshot with whatever §Conventions looks like at execute time.** Drift between snapshot and upstream is a normal first-execute-phase finding, not a blocker — adjust the snapshot, re-confirm, then proceed.

### Snapshot A — Phase scope policy (allowed-path table) — RETIRED

> Per-phase allowed-paths assignment is defined in `internal/atdd/phase-scopes.yaml` (per the SSoT phase-scope plan, [20260518-1530](20260518-1530-atdd-phase-scope-ssot.md)). This plan no longer owns that assignment. `check_phase_scope` (item 5 below) reads it directly from the embedded yaml at runtime, keyed by BPMN node id. Git history preserves the prior shape of this snapshot.

### Snapshot B — Disable-reason convention

Consumed by item 3. Annotation format (cycle slot hard-coded `AT` today; `CT` slot reserved for symmetry but not in use):

```
@Disabled("<TICKET-ID> - AT - <LOOP> - <PHASE>")
```

- `<LOOP>` ∈ {RED, GREEN}. Only RED uses disable today.
- `<PHASE>` ∈ {TEST, DSL, SYSTEM DRIVER} (uppercase; internal space allowed).
- Re-enable filter: `startsWith("<CURRENT-TICKET-ID> - AT - RED - <PREV-PHASE>")`. Never strip annotations whose prefix belongs to a different ticket.

### Snapshot C — Phase-output flags (RED-DSL only)

Consumed by item 4. **Two flags** emitted by `at-red-dsl`; the post-RED-DSL gateway treats unset as an error.

| Flag name | Domain | Meaning when `yes` |
|---|---|---|
| `System Driver Interface Changed` | yes \| no | RED-SYSTEM-DRIVER must run |
| `External System Driver Interface Changed` | yes \| no | Hand off to CT cycle |

Note: `dsl_interface_changed` is **not** a RED-DSL phase-output flag — it's emitted by RED-TEST and gates entry to RED-DSL (see `at_cycle` `GATE_DSL_AT` line 384). Item 4's validation gateway covers only the two flags above.

### Snapshot D — Layer 2 mechanics + STOP_SCOPE_VIOLATION options

Consumed by items 2 (STOP_SCOPE_VIOLATION definition) and 5 (Layer 2 action). Moved here from `conventions.md` on 2026-05-18 — these are BPMN-runtime specifics that phase agents don't read.

**What stays in §Conventions** (agent-facing, not moved):
- The principle ("every phase agent operates within a declared scope").
- **Layer 1 — agent-triggered** (the agent contract: prompt names allowed paths; on out-of-scope need, agent emits `scope_exception` signal and exits without inline approval). Item 6 implements the runtime side of Layer 1 — see item 6 for binding/wiring detail — but the *behavioural contract* the agent must follow remains in §Conventions because the agent reads it.
- The per-phase allowed-paths table (≡ Snapshot A).

**Layer 2 — post-phase scope check** (catches what Layer 1 missed; implemented by item 5): after each phase agent finishes normally, a scripted check diffs the modified files (`git diff --name-only` vs the pre-phase ref) against the allowed-path policy. On violation, the cycle halts and routes to `STOP_SCOPE_VIOLATION` — the same human-task prompt Layer 1 routes to.

The cycle never auto-allows and never auto-reverts — the user always decides. **`STOP_SCOPE_VIOLATION` options** (item 2):

- **Accept (continue from current phase)** — the agent's out-of-scope change is judged correct (e.g. RED-SYSTEM-DRIVER discovered the DSL or driver-port interface was wrong; GREEN discovered the test was wrong). Record the exception and continue from the current phase.
- **Rewind to upstream phase** — accept the out-of-scope change, then restart the cycle from the phase whose output was wrong (e.g. accept a DSL edit made during RED-SYSTEM-DRIVER, then rerun RED-DSL to re-validate the corrected DSL, then continue). The most principled response when the violation reveals an upstream bug — preserves the per-phase RED guarantee instead of carrying an unvalidated upstream change forward.
- **Revert + rerun** — discard the out-of-scope changes and rerun the current phase agent.
- **Abort** — stop the cycle, escalate to human review.

## Items

> **Refined 2026-05-18 (applies to items 2–9):** All items below extend the existing Go BPMN runtime — new nodes in `internal/atdd/runtime/statemachine/process-flow.yaml`, new bindings in `internal/atdd/runtime/gates/` / `actions/` / `agents/`, rendered via the existing `internal/atdd/runtime/diagram/`. No new orchestration tool, no new artefact form.

### ~~1. Choose and document the orchestration form (decision blocker)~~ — removed

> **Refined 2026-05-18:** Deleted. **Why:** The artefact + runtime already exist (`process-flow.yaml` + Go state machine in `internal/atdd/runtime/statemachine/`, Mermaid via `internal/atdd/runtime/diagram/`). "Follow exactly how we've done bpmn stuff up to now, it's in go" (user). The choice this item proposed was a non-decision.

### 2. Extend `red_phase_cycle` + `green_phase_cycle` with scope + legacy enforcement

The shared per-phase wrapper already exists as two sub-processes in `internal/atdd/runtime/statemachine/process-flow.yaml`: `red_phase_cycle` (line 815) and `green_phase_cycle` (line 936). Every AT/CT phase already invokes them via `call_activity` with per-phase `params:` (`agent`, `phase_doc`, `phase_label`, `change_type`/`suite`, `compile_action`). This item extends those wrappers with the scope + legacy enforcement they currently lack.

**Additions to each wrapper:**

a. **New `allowed_paths` param** threaded from the parent's call site (in `at_cycle`, `at_green_system`, `ct_subprocess`, …) through to the `WRITE` user_task. The agent reads it from its prompt template. (Resolves the agent-prompt half of Open Q3.)

b. **Post-WRITE scope-exception gateway (Layer 1)** — new `gateway` node immediately after `WRITE` (or after the human-review STOP), with a binding that reads the agent's scope-exception signal. On `signal == true`, branch to a new `STOP_SCOPE_VIOLATION` human task (shared across both wrappers). Detail of the signal channel is the second half of Open Q3 — see item 6.

c. **Pre-COMMIT scope check (Layer 2)** — new `service_task` between `DISABLE` and `COMMIT` (red) / between the verify-passes gate and the parent's COMMIT (green), running the post-phase scope check action. On hit, branch to the same `STOP_SCOPE_VIOLATION`. Detail in item 5.

d. **Pre-COMMIT failing-legacy check** — new `service_task` right before `COMMIT`, running the failing-legacy detector. On hit, branch to a new `STOP_LEGACY_FAILED` human task. Detail in item 7.

**Two new human-task STOPs**, defined once and reused across the two wrappers:

- `STOP_SCOPE_VIOLATION` — context: violating paths + allowed paths + (when Layer-1-triggered) the agent's reason. Options: Accept / Rewind to upstream phase / Revert + rerun / Abort.
- `STOP_LEGACY_FAILED` — context: failing legacy test name + failure output. Options: Treat as real regression (rewind) / Mark legacy test as needing revision (escalate to legacy cycle) / Abort.

**Adding a new phase** stays the same as today: add a `call_activity` to the wrapper with per-phase `params:` (now also `allowed_paths`); no bespoke per-phase orchestration.

> **Refined 2026-05-18:** Reframed from "define a new envelope" → "extend the two existing wrappers". **Why:** `red_phase_cycle` + `green_phase_cycle` already are the shared envelope; this item adds the missing scope/legacy enforcement to them. The original "Refined later" note on disable/enable placement is also dropped — the existing code already settles it: `DISABLE` lives inside `red_phase_cycle` (line 876); `ENABLE_TESTS` lives in the parent (`at_green_system` line 409) between phases.

### 3. Update `disable_change_driven` + `enable_change_driven` actions to §Conventions disable-reason format

Both nodes already exist in `process-flow.yaml`: `DISABLE` (line 876 inside `red_phase_cycle`, `action: disable_change_driven`) and `ENABLE_TESTS` (line 409 inside `at_green_system`, `action: enable_change_driven`). This item updates the **action implementations** under `internal/atdd/runtime/actions/` — no new nodes in `process-flow.yaml`.

- **`disable_change_driven`** (runs at end of phase, before COMMIT — already wired): grep the project for test files, annotate change-driven tests per **Snapshot B** (above). Cycle slot is hard-coded `AT` today; `<LOOP>` ∈ {RED, GREEN}; `<PHASE>` ∈ {TEST, DSL, SYSTEM DRIVER}. **Precondition:** RED proof has been observed (test ran, failed at runtime). Skip legacy tests entirely (per the legacy-coverage plan's domain restriction).
- **`enable_change_driven`** (runs at start of next phase — already wired): grep for `@Disabled` annotations matching the **Snapshot B** re-enable filter (`startsWith("<CURRENT-TICKET-ID> - AT - RED - <PREV-PHASE>")`) and remove them. Never strip annotations for other tickets; never strip legacy markers.

Inputs (ticket ID, cycle, loop, phase) come from the action's context — extend the action signatures / context if not all four are currently threaded through.

Language-specific syntax for `@Disabled` (Java) / `@pytest.mark.skip` (Python) / `[Ignore]` (.NET) / etc. is delegated to the existing `language-equivalents/` material.

> **Refined 2026-05-18:** Reframed from "add disable/enable steps" → "update existing actions". **Why:** the nodes are already in the YAML and wired into the wrappers — only the action bodies need to follow the new §Conventions disable-reason format and the legacy-skip rule.

### 4. Add post-RED-DSL flag-presence validation gateway

The branching this item describes is **already wired** in `at_cycle` (line 316): `GATE_DSL_AT` (binding `dsl_interface_changed`), `GATE_EXT_AT` (binding `external_system_driver_interface_changed`), `GATE_SYS_AT` (binding `system_driver_interface_changed`) with sequence_flows wiring exactly the three branches (route to RED-SYSTEM-DRIVER / CT_SUBPROCESS / AT_GREEN_SYSTEM). The current gateways consume flag *values* but do not verify the agent emitted them.

This item **adds the missing validation**:

- New `gateway` node `GATE_DSL_FLAGS_PRESENT` placed between `AT_RED_DSL` (the `red_phase_cycle` call_activity) and the existing `GATE_EXT_AT` in `at_cycle`.
- New binding `dsl_flags_present` in `internal/atdd/runtime/gates/` that reads the **two** RED-DSL phase-output flags per **Snapshot C** (`System Driver Interface Changed`, `External System Driver Interface Changed`) and returns `true` only if both are explicitly set.
- New `user_task: agent: human` STOP `STOP_FLAG_UNSET` — "STOP - HUMAN REVIEW — AT - RED - DSL phase-output flags missing; re-run with reminder". Loopback to `AT_RED_DSL`.
- Sequence_flows: `AT_RED_DSL → GATE_DSL_FLAGS_PRESENT`; `GATE_DSL_FLAGS_PRESENT → GATE_EXT_AT when present == true`; `GATE_DSL_FLAGS_PRESENT → STOP_FLAG_UNSET when present == false`; `STOP_FLAG_UNSET → AT_RED_DSL`.

> **Corrected 2026-05-18:** Earlier draft of this item said the validation checks "all three flags including `dsl_interface_changed`". **Why wrong:** `dsl_interface_changed` is emitted by RED-TEST (it gates entry to RED-DSL via the existing `GATE_DSL_AT` line 384), not by RED-DSL. Per Snapshot C, RED-DSL emits two flags, not three. The validation gateway sits between `AT_RED_DSL` and `GATE_EXT_AT` (not `GATE_DSL_AT`).

One validation gateway covers all three flags (they're emitted together by the same RED-DSL phase). The existing `GATE_DSL_AT` / `GATE_EXT_AT` / `GATE_SYS_AT` then consume validated values without change.

> **Refined 2026-05-18:** Reframed from "add a post-RED-DSL gateway (validation + branching)" → "add only the flag-presence-validation gateway". **Why:** The three branching gateways are already in `at_cycle`; only the flag-presence check is new. Sequential-vs-parallel between RED-SYSTEM-DRIVER and CT sub-cycle was already settled in the existing wiring (`CT_SUBPROCESS` runs first, then `GATE_SYS_AT`, then `AT_RED_SYSTEM_DRIVER`) — that's a pre-existing decision, not something this plan re-litigates.

### 5. Implement the post-phase scope check action

Concrete implementation of item 2's bullet (c). Pure scripted (no LLM). Lives as:

- **Action** `check_phase_scope` in `internal/atdd/runtime/actions/` — reads `internal/atdd/phase-scopes.yaml` (embedded in the binary via the loader at `internal/atdd/phase_scopes.go:LoadPhaseScopes`) for the per-phase layer list, keyed by the current BPMN node's `id:` (e.g. `AT_RED_TEST`). Resolves layer names against `gh-optivem.yaml paths:` (in the user's project) — literal values, no `${...}` substitution (substitution retired per SSoT plan locked δ). Runs `git diff --name-only <pre-phase-ref> HEAD` and `git status --porcelain` for un-staged modifications. Writes a structured result to context.
- **Gate binding** `phase_scope_clean` in `internal/atdd/runtime/gates/` — reads the structured result; returns `true` if all modified paths fall within the allowed set, else `false`.
- **Wired position** (already pinned in item 2): between `DISABLE` and `COMMIT` in `red_phase_cycle`; before the parent's COMMIT in green flows.

**Path-match semantics — directory-aware prefix.** A diff path matches an allowed path `P` iff `diffPath == P || strings.HasPrefix(diffPath, P+"/")`. Raw `strings.HasPrefix(diffPath, P)` is wrong — it would let `.../shop` match `.../shop2/...`. This contract is shared with the `gh optivem process scope` CLI projection and is the resolution of open question λ (SSoT plan, resolved 2026-05-18).

**Layer-name → resolved-path join.** The action looks up the current node's layer list in `phase-scopes.yaml`, then resolves each layer against the loaded `*projectconfig.Config`:

- Family B layers (e.g. `driver_port`, `at_test`, `ct_test`) → `cfg.Paths[layer]`.
- Family A path-shaped layers — only `system_path` today — → `cfg.System.Path` (per `FamilyAPathKeysInScope` in `internal/atdd/phase_scopes.go`).

Both surfaces are fully resolved at scaffold/migrate time (per SSoT plan item 3), so the action's join is a lookup, not a substitution.

**Pre-phase ref:** captured by a small upstream service_task at WRITE-time (or read from the most recent COMMIT baseline) — pick one before execute; the choice affects whether the check sees the working-tree state alone or the full set of changes since the phase started.

**Allowlist phases (no-op + log).** Phases on the deferred allowlist in `internal/atdd/phase_scopes.go:PhasesDeferredByPlan` (`AT_GREEN_BACKEND`, `AT_GREEN_FRONTEND`, the three structural-cycle entries) currently have no `phase-scopes.yaml` entry pending their respective follow-up plans. For these the action returns success without checking, and logs a single line citing the deferred plan — the absence of enforcement is intentional and visible. The action panics on a missing-key for any phase that is neither in `phase-scopes.yaml` nor on the allowlist (build-time test `TestPhaseScopes_ReverseFK_WritingAgentsScopedOrAllowlisted` makes that condition unreachable in practice).

STOP options (Accept / Rewind / Revert + rerun / Abort) live on `STOP_SCOPE_VIOLATION` (defined in item 2), not duplicated here.

On out-of-scope diff the error message points readers at `internal/assets/global/docs/atdd/process/shared/scope.md` (the user-facing scope rule, owned by SSoT plan item 2).

> **Refined 2026-05-18:** Reframed from "post-phase scope check" → "implement the action + gate binding behind item 2 bullet (c)". **Why:** Item 2 already pins placement and STOP; this item is the action-body detail.
>
> **Re-refined 2026-05-19 (SSoT plan item 8):** Action's read surface rewired: it now reads `internal/atdd/phase-scopes.yaml` directly by node id, not `allowed_paths` from per-node `params:`. **Why:** under SSoT, `phase-scopes.yaml` is the single doctrinal source of per-phase scope (plan 20260518-1530 item 1); threading `allowed_paths` strings through `process-flow.yaml` is redundant. `Config.PlaceholderMap()`-based substitution retired (SSoT locked δ); the action does layer-name → resolved-path lookup, not `${...}` substitution. `<channel>` separately dropped 2026-05-18.

### 6. Implement the Layer 1 scope-exception signal contract

Concrete implementation of item 2's bullet (b) — the agent-side escape hatch per §Conventions → Phase scope policy Layer 1 (see **Snapshot A** above for the per-phase allowed paths; §Conventions itself currently lives at `docs/atdd/process/shared/conventions.md` pending the upcoming split).

**Signal channel:** reuse the existing agent→runtime channel. Phase agents already emit a structured COMMIT output that the state machine consumes (per `process-flow.yaml` header: "what dispatches next, gated by which flag"). The Layer 1 signal becomes a structured field in that output payload:

```
scope_exception:
  files: [path/to/out-of-scope.go, ...]
  reason: "<one-line rationale>"
```

When absent, the phase ran within scope. No new IPC mechanism (no exit codes, marker files, or extra stdout channels).

**Gate binding:** new `scope_exception_requested` in `internal/atdd/runtime/gates/` reads the agent's output and returns `true` if `scope_exception` is non-empty.

**Wiring (already pinned in item 2 bullet (b)):** the new gateway sits immediately after `WRITE`; on `true`, branch to `STOP_SCOPE_VIOLATION` (skipping the post-phase scope check from item 5 and bypassing `DISABLE` / `COMMIT`); on `false`, continue down the normal path.

**Agent prompt template update:** the per-phase agent prompts (currently under `internal/assets/runtime/prompts/atdd/` or wherever they end up after the Part 1 Phase 7 prompt-slimming work) need a section instructing the agent to:
1. Edit only within `allowed_paths`.
2. If unavoidably blocked, emit the `scope_exception` field and exit.
3. **Never** ask inline for approval. Matches the runtime-prompt rule "no approval inside the agent".

> **Refined 2026-05-18:** Reframed from "define the signal format + envelope branch + behavioural rule" → "implement the channel + binding + prompt instruction". **Why:** The wiring is now in item 2 (b); this item is the contract detail. Open Q3 closed: signal channel = structured field in existing agent output (not a new IPC mechanism).

### 7. Implement the failing-legacy detector action

Concrete implementation of item 2's bullet (d). Lives as:

- **Action** `detect_failing_legacy` in `internal/atdd/runtime/actions/` — runs the test suite filtered by the legacy marker, writes structured result to context.
- **Gate binding** `failing_legacy_present` in `internal/atdd/runtime/gates/` — `true` if any legacy test failed.
- **Wired position** (already pinned in item 2 bullet (d)): right before COMMIT in both wrappers.
- **STOP options** (Treat as real regression / Mark for revision / Abort) live on `STOP_LEGACY_FAILED` (defined in item 2).

**Hard dependency (execute blocker):** the legacy marker convention — what "legacy marker" concretely means in the test suite — is owned by the [legacy-coverage-cycle plan](20260518-1116-legacy-coverage-cycle.md) (its §Conventions tightening + its Open Question on annotation/naming/directory). The action body cannot be written until that plan settles the marker. This plan consumes the marker as a typed dependency — it does not co-own the design.

**Behavioural guardrail (already enforced elsewhere):** "never `@Disabled` a failing legacy test" is enforced by (i) the `disable_change_driven` action's legacy-skip rule (item 3) and (ii) `STOP_LEGACY_FAILED`'s option set deliberately omitting `@Disabled`. Not restated here.

> **Refined 2026-05-18:** Reframed from "cross-plan reference + marker design alternatives" → "thin consumer of the legacy plan's marker convention". **Why:** user clarified the marker convention is fully owned by the legacy-coverage-cycle plan; this item just consumes it. Dropped the "annotation/naming/directory" restatement so the design call lives in one place.

### ~~8. Thread `allowed_paths` param into every AT-phase invocation~~ — OBSOLETE

> **Obsolete 2026-05-19** — superseded by SSoT plan ([20260518-1530](20260518-1530-atdd-phase-scope-ssot.md)) item 8 (c): per-node `allowed_paths` params removed from `process-flow.yaml`; `check_phase_scope` (item 5 above) now reads `internal/atdd/phase-scopes.yaml` directly by node id. No `params:` threading needed in this plan. Skip this item.

### ~~9. Thread `allowed_paths` into CT phases — split A (RED phases) + B (CT_GREEN_STUBS deferred)~~ — OBSOLETE

> **Obsolete 2026-05-19** — superseded by SSoT plan ([20260518-1530](20260518-1530-atdd-phase-scope-ssot.md)) item 8 (c): per-node `allowed_paths` params removed from `process-flow.yaml`; `check_phase_scope` (item 5 above) now reads `internal/atdd/phase-scopes.yaml` directly by node id. Part A is therefore obsolete in the same way as item 8.
>
> Part B's `CT_GREEN_STUBS` rewiring concern (bare `user_task` vs `call_activity → green_phase_cycle`) remains a real follow-up and is tracked separately in `plans/deferred/20260518-1530-multitier-green-scope.md` / the CT_GREEN_STUBS allowlist entry in `internal/atdd/phase_scopes.go:PhasesDeferredByPlan`. Skip this item.

### ~~10. Document the orchestration~~ — removed

> **Refined 2026-05-18:** Deleted. **Why:** No new doc. The existing `docs/process-diagram.md` + the header docstring of `internal/atdd/runtime/statemachine/process-flow.yaml` already document the orchestration vocabulary and rendering. Where `docs/atdd/at-cycle.md` / `ct-cycle.md` reference "the BPMN process diagram", that pointer goes to `docs/process-diagram.md` (no new artefact needed). Any new node-type vocabulary introduced by items 2–7 gets a one-line update in `process-flow.yaml`'s header docstring as part of that item — not as a separate doc-writing step.

## Out of scope

- Per-language `@Disabled` syntax — delegated to `language-equivalents/`.
- Legacy marker convention design — owned by the [legacy-coverage-cycle plan](20260518-1116-legacy-coverage-cycle.md).
- Structural cycle and cycle router orchestration — separate plans (signposted in [Part 1 Phase 7](20260516-1701-atdd-at-cycle-absorb-internal-assets.md)).
- Migration of supporting docs (architecture, language-equivalents, glossary, etc.) out of `internal/assets/` — separate concern.

## Hand-off

Before executing this plan, the residual Open Questions (Q1 contract details — agent output `scope_exception` channel is settled in item 6; Q2 human-task UX reuses the existing `user_task: agent: human` STOP pattern) must be answered as they're walked. The artefact + runtime choice is settled (extends the existing Go BPMN runtime — see header). Placeholder resolution is settled (`Config.PlaceholderMap()` already covers every placeholder this plan needs — see item 5).

**Cross-cutting Part 1 edit — ✅ done 2026-05-18 (commit `82bb983`):**

- **§Conventions → Phase scope policy → RED-SYSTEM-DRIVER row:** `<channel>` removed. Row now reads `${driver_port}/${sut_namespace}/` and `${driver_adapter}/${sut_namespace}/`. **Why:** the channel boundary is better enforced by ticket scope + the existing human review STOP than by a per-call path regex, and `<channel>` had no source in `projectconfig.PlaceholderMap()` or in node `params:`.
- **Location at edit time:** `docs/atdd/process/shared/conventions.md:67`. The user has flagged that `conventions.md` will be split — when the split lands, the row may move; Snapshot A in this plan reflects the post-edit content and is the authoritative copy for execute purposes.
