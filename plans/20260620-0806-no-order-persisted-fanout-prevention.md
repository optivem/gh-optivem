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

Resume at **Step 2 (Layer 1 — `shop` reference example)**: this is a *design + multi-language implementation* item in the `academy/shop` repo, not a mechanical edit here. The full design (response-level primitive, current-behaviour constraints, per-language Items 1–3, fan-out-complete requirement, alternatives) is now folded directly into Step 2 below — `/execute-plan` against `shop`. One open question remains (reference scope: order place/cancel vs. broader absence class).

## Steps

### Short-term (unblock the corpus)

- [x] **Step 1 — Drop the absence criterion from #68 (temporary).** ✅ 2026-06-20 — removed `And no order is persisted` from the "Zero quantity is rejected" scenario in `optivem/shop#68` (validation-rejection assertion kept). Note: deferred plan `20260617-1437` explicitly rejected this as a *permanent* fix (silently drops a real criterion, lowering fidelity) — so this is strictly a temporary unblock, tracked by Step 6.

### Long-term Layer 1 — `shop` reference example (root / class-killing fix; supersedes & absorbs 20260617-1437)

> Lives in the **`academy/shop` repo**, not in any gh-optivem BPMN/commands/agents layer — this is the reference the agents pattern-match against.
>
> **This section absorbs the now-deleted plan `20260617-1437`.** Its full Java/.NET/TS design is folded in below; the only thing layered on top is the #68 lesson that the older plan predates — the shared port method must be **fan-out-complete** (present on *every* conformant `*Failure` core class, not just the one in the current scenario).

- [ ] **Step 2 — Add the canonical, always-on, fan-out-complete absence example, mirrored ×3.** Add a first-class (not WIP-gated, not `@EnabledIfEnvironmentVariable`) acceptance test extending an **existing passing negative scenario** (e.g. `cannotPlaceOrderWithNonExistentCoupon`) with the absence tail, plus the backing DSL primitive in `system-test/{java,dotnet,typescript}`. Reuse a passing scenario so the example exercises *only* the new DSL primitive and needs **no new system behaviour**; do not resurrect #69's quantity<100 constraint. Green on monolith **and** multitier per language (`-p 2` / `scripts/test.sh`; never unbounded `go test`/Gradle-all per the Windows hazard).

  **Design decision — response-level absence assertion (resolved; carried from 20260617-1437).** Use a **response-level** primitive: the absence assertion reads the **failed operation's outcome directly** (the rejection guarantees nothing was persisted) — it does **not** route through the eager-fetch `.order()` path and does **not** call `getResultValue` on the `FAILED` alias. Deciding reasons:
  1. **Faithful to how the system rejects.** Validation short-circuits *before* any persistence (the #69 `@Max` rejection never reaches the service/DB), so "no order persisted" is guaranteed by the rejection itself.
  2. **Dodges the structural alias bug without new plumbing.** Reading the failure outcome never calls `getResultValue` on the `FAILED` alias, so the `IllegalStateException` cannot occur.
  3. **Uses data already on hand.** No new `BrowseOrders` use case, no new driver method, no system-side query — so it mirrors cleanly ×3.
  4. **Right altitude for a teaching reference.** Demonstrates the pattern ("a rejected command persists nothing") with minimum machinery.

  **Current behaviour the design must respect (verified against shop `main` + the #69 worktree):**
  - `system-test/{java,dotnet,typescript}` are three mirrored testkits; the DSL lives at `…/testkit/dsl/` (Java: `…/dsl/core/scenario/then/steps/` + `…/dsl/port/then/steps/`).
  - The Java DSL exposes `ViewOrder` (lookup **by order number**) and `BrowseCoupons` — **no browse/list-orders use case.** A validation-rejected order is never assigned a number, so there is no key to look it up by.
  - `ThenOrderImpl` resolves its order **eagerly in the constructor** via `viewOrder(...).shouldSucceed()` — incompatible with any after-a-failure assertion.
  - `UseCaseContext.getResultValue(alias)` throws `IllegalStateException` when the alias is in `FAILED:` state (`UseCaseContext.java:76`).
  - TypeScript currently ships only `…/dsl/port/then/steps` (the interface); the `core` impl + a `ThenOrder` step may need creating. .NET's `ThenOrder` equivalent was not found by name — locate or create it.

  **Per-language work (folded from 20260617-1437 Items 1–3):**
  1. **Java** — make the after-failure absence assertion reachable without the eager `viewOrder(...).shouldSucceed()` fetch (a dedicated terminal step off `ThenFailureImpl`, or short-circuit `ThenOrderImpl` when the prior operation failed — whichever, `getResultValue` must **not** be called on a `FAILED` alias). The assertion must assert the **real** invariant (the failed operation yielded no persisted order), not that an alias string is null. Pick a name that reads correctly on the failure path (rename `doesNotExist` if misleading). Add the first-class example.
  2. **.NET** — locate or create the `ThenOrder`/failure-path equivalent; same response-level assertion + same first-class example.
  3. **TypeScript** — create the `core` impl + `ThenOrder` step as needed; same response-level assertion + first-class example.

  **Fan-out-complete requirement (#68 lesson, layered on top):** the shared port method must be present (stubbed or implemented) on **every** core `*Failure` class that conforms to the port — place-order **and** cancel-order (and any others) — so the copied reference compiles and the agent copies a *complete* primitive, never a half one.

- [x] **Step 3 — Reconcile `20260617-1437`.** ✅ Folded into Step 2 (design decision, current-behaviour notes, per-language Items 1–3, alternatives) and the standalone plan deleted, per the delete-completed-plans convention.

  **Alternatives considered (carried from 20260617-1437):**
  - *Persistence-level absence assertion (deferred — stronger but disproportionate).* Issue a fresh browse/list-orders query and assert the order is genuinely absent. Strongest fidelity, but shop has no list/browse-orders use case, so it needs a new use case + driver adapter ×3 (plus possibly a system query endpoint), and for validation-rejection criteria the order is never created by construction. Revisit under the rule of three if a non-validation side-effect-absence class of tickets appears.
  - *Java-only fix now, mirror later (rejected).* Leaves .NET/TS agents reinventing the broken primitive — the mirror invariant is the whole point of an example.
  - *Drop the `.and().order().doesNotExist()` tail from acceptance tests (rejected as a permanent fix).* Silently drops a real criterion, lowering fidelity — opposite of the "accuracy over speed" gate. (The #68 drop in Step 1 is strictly temporary, tracked by Step 6.)

### Long-term Layer 2 — agent guardrail (gh-optivem agents)

- [ ] **Step 4 — Add a shared-port fan-out rule to `acceptance-test-writer`.** In `internal/atdd/assets/runtime/agents/atdd/acceptance-test-writer.md` (and/or the shared chunks it concatenates), instruct: when widening a **shared** port/DSL interface (adding a method to a shared `Then*`/`When*` port type), implement-or-stub that method on **every** core class that implements that port — not only the one in the current scenario — and run `gh optivem test compile` before declaring done. Tradeoff (record in the plan, not the prompt): cheap/prompt-only and catches the class, but stays agent-discipline-dependent and gives nothing concrete to copy — Layer 1 is what actually supplies the reference.

### Long-term Layer 3 — fixer self-heal (gh-optivem; verify before changing)

- [ ] **Step 5a — VERIFY the compile-tests fixer scope.** Determine whether the `command-failed-fixer`'s `${scope-block}` at the `WRITE_AND_VERIFY_ACCEPTANCE_TEST_CODE → COMPILE_TESTS` path (`internal/atdd/process/process-flow.yaml`, ~line 1025) already includes the testkit DSL (`system-test/**/testkit/dsl/**`). Do **not** assume — read the scope wiring first.
- [ ] **Step 5b — If out of scope, widen it (defense in depth).** Only if Step 5a shows the cancel-order DSL file is *outside* the fixer's scope: propose the minimal scope widening so a half-implemented shared port self-heals during FIX (fixer adds the missing stub) instead of halting. If already in scope, record that the fixer would already recover this in a non-rehearsal run and close Layer 3.

### Restore-gate

- [ ] **Step 6 — Reinstate #68's criterion.** Once Layer 1 (Step 2) lands and an absence-shaped ticket goes green by copying the `shop` reference, restore `And no order is persisted` to `optivem/shop#68` (undo Step 1). The criterion must not be permanently lost.

## Open questions

- **Scope of the reference:** just `noOrderPersisted` (place-order + cancel-order), or the broader "absence / non-persistence" class across all use cases? (Inferred: start with order place/cancel per the failing ticket; broaden under the rule of three.)
- ~~**Supersede vs. fold:** mark `20260617-1437` superseded-in-place, or fold its detailed Java/.NET/TS design into Step 2 and delete it?~~ **Resolved (2026-06-22)** — folded into Step 2 and the standalone plan deleted (Step 3 done).
- **Layer 3 necessity:** if Step 5a shows the DSL is already in the fixer's scope, Layer 3 may collapse to a one-line "already covered" note rather than a code change. Confirm whether you still want it tracked.
- ~~**Short-term edit ownership:** me via `gh` vs. you edit the issue.~~ **Resolved** — done via `gh` (Step 1, 2026-06-20).
