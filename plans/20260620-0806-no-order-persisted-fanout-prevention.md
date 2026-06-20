# 2026-06-20 08:06:00 UTC — Prevent the "no order is persisted" absence-assertion fan-out failure (defense in depth)

## TL;DR

**Why:** Any acceptance criterion of the shape *"And no order is persisted"* keeps breaking ATDD runs because the `shop` reference repo ships **zero** absence-assertion examples — so the agent reinvents the primitive every run and gets it wrong. Rehearsal #68 is the latest recurrence: the `acceptance-test-writer` widened the shared TS port `ThenFailure` but only stubbed it on one of two conformant core classes → `tsc` TS2416 at `COMPILE_TESTS`. This is the same disease as deferred plan `20260617-1437` (which was never executed), with a new compile-time surface.
**End result:** A short-term unblock (drop the criterion from #68 *temporarily*), plus three layered long-term defenses — a canonical fan-out-complete reference example in `shop` (the root fix), an `acceptance-test-writer` guardrail against half-implementing shared ports, and a verification that the compile-tests fixer can self-heal a half-implemented port — so absence-shaped tickets never again reinvent the primitive, leave a port half-implemented, or burn a fix-loop round-trip on a mechanical omission.

## Outcomes

What we get out of this:

- **#68 rehearsal unblocked now** — its "And no order is persisted" criterion is temporarily removed (with a restore-gate tracking it), so the rehearsal corpus moves while the real fix lands.
- **`shop` ships a canonical, always-on, fan-out-complete "no order persisted" reference** — a committed acceptance test + backing DSL primitive mirrored across Java / .NET / TypeScript, green on monolith **and** multitier, demonstrating the shared port method present on **every** conformant `*Failure` core class. The agent copies a complete primitive instead of inventing a half one.
- **Deferred plan `20260617-1437` is reconciled** — folded into / superseded by this plan, no longer a dangling unexecuted plan.
- **`acceptance-test-writer` has an explicit fan-out guardrail** — when it widens a shared `Then*`/`When*` port, it stubs the new method on every conformant core class and compiles the DSL before declaring done.
- **The compile-tests fix path is known to self-heal** (or is documented as not) — we verify whether `command-failed-fixer`'s scope reaches the testkit DSL so a half-implemented port is repaired during FIX rather than halting.
- **The criterion is never silently lost** — the short-term drop has an explicit restore step gated on Layer 1 landing.

## ▶ Next executable step (resume here)

**Step 1 done (2026-06-20).** Short-term unblock applied: `And no order is persisted` removed from the "Zero quantity is rejected" scenario in `optivem/shop#68` (validation-rejection assertion kept). Step 6 restore-gate tracks reinstatement.

Resume at **Step 2 (Layer 1 — `shop` reference example)**: this is a *design + multi-language implementation* item in the `academy/shop` repo, not a mechanical edit here — switch to `/refine-plan` to settle the open questions (reference scope; supersede-vs-fold) first, then `/execute-plan` against `shop`. Carry forward the deferred plan's response-level primitive design and the fan-out-complete requirement (shared port method present on every conformant `*Failure` core class).

## Steps

### Short-term (unblock the corpus)

- [x] **Step 1 — Drop the absence criterion from #68 (temporary).** ✅ 2026-06-20 — removed `And no order is persisted` from the "Zero quantity is rejected" scenario in `optivem/shop#68` (validation-rejection assertion kept). Note: deferred plan `20260617-1437` explicitly rejected this as a *permanent* fix (silently drops a real criterion, lowering fidelity) — so this is strictly a temporary unblock, tracked by Step 6.

### Long-term Layer 1 — `shop` reference example (root / class-killing fix; supersedes 20260617-1437)

> Lives in the **`academy/shop` repo**, not in any gh-optivem BPMN/commands/agents layer — this is the reference the agents pattern-match against.

- [ ] **Step 2 — Add the canonical, always-on, fan-out-complete absence example, mirrored ×3.** Carry forward the deferred plan's design decision: a **response-level** primitive that reads the failure outcome directly (does **not** route through the eager-fetch `.order()` path, does **not** call `getResultValue` on the `FAILED` alias). Add a first-class (not WIP-gated) acceptance test extending an existing passing negative scenario with the absence tail, plus the backing DSL primitive in `system-test/{java,dotnet,typescript}`. **Critically:** the shared port method must be present (stubbed or implemented) on **every** core `*Failure` class that conforms to the port — place-order **and** cancel-order (and any others) — so the copied reference is fan-out-complete. Green on monolith **and** multitier per language (`-p 2` / `scripts/test.sh`; never unbounded `go test`/Gradle-all per the Windows hazard).
- [ ] **Step 3 — Reconcile `20260617-1437`.** Mark the deferred plan superseded by this one (or fold its Items 1–3 in and delete it per the "delete completed plans" convention once Step 2 is done).

### Long-term Layer 2 — agent guardrail (gh-optivem agents)

- [ ] **Step 4 — Add a shared-port fan-out rule to `acceptance-test-writer`.** In `internal/atdd/assets/runtime/agents/atdd/acceptance-test-writer.md` (and/or the shared chunks it concatenates), instruct: when widening a **shared** port/DSL interface (adding a method to a shared `Then*`/`When*` port type), implement-or-stub that method on **every** core class that implements that port — not only the one in the current scenario — and run `gh optivem test compile` before declaring done. Tradeoff (record in the plan, not the prompt): cheap/prompt-only and catches the class, but stays agent-discipline-dependent and gives nothing concrete to copy — Layer 1 is what actually supplies the reference.

### Long-term Layer 3 — fixer self-heal (gh-optivem; verify before changing)

- [ ] **Step 5a — VERIFY the compile-tests fixer scope.** Determine whether the `command-failed-fixer`'s `${scope-block}` at the `WRITE_AND_VERIFY_ACCEPTANCE_TEST_CODE → COMPILE_TESTS` path (`internal/atdd/process/process-flow.yaml`, ~line 1025) already includes the testkit DSL (`system-test/**/testkit/dsl/**`). Do **not** assume — read the scope wiring first.
- [ ] **Step 5b — If out of scope, widen it (defense in depth).** Only if Step 5a shows the cancel-order DSL file is *outside* the fixer's scope: propose the minimal scope widening so a half-implemented shared port self-heals during FIX (fixer adds the missing stub) instead of halting. If already in scope, record that the fixer would already recover this in a non-rehearsal run and close Layer 3.

### Restore-gate

- [ ] **Step 6 — Reinstate #68's criterion.** Once Layer 1 (Step 2) lands and an absence-shaped ticket goes green by copying the `shop` reference, restore `And no order is persisted` to `optivem/shop#68` (undo Step 1). The criterion must not be permanently lost.

## Open questions

- **Scope of the reference:** just `noOrderPersisted` (place-order + cancel-order), or the broader "absence / non-persistence" class across all use cases? (Inferred: start with order place/cancel per the failing ticket; broaden under the rule of three.)
- **Supersede vs. fold:** mark `20260617-1437` superseded-in-place, or fold its detailed Java/.NET/TS design into Step 2 and delete it? (Inferred: fold + delete once Step 2 is authored, per the delete-completed-plans convention.)
- **Layer 3 necessity:** if Step 5a shows the DSL is already in the fixer's scope, Layer 3 may collapse to a one-line "already covered" note rather than a code change. Confirm whether you still want it tracked.
- ~~**Short-term edit ownership:** me via `gh` vs. you edit the issue.~~ **Resolved** — done via `gh` (Step 1, 2026-06-20).
