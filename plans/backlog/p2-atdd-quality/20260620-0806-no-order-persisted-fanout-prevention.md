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

**Steps 1-5 shipped (2026-11-03); the shop change is on `optivem/shop` main** (`noOrderPersisted` / `NoOrderPersisted` present in Java, .NET, TypeScript tests and DSL). Q-7 resolved GO, scoped to order place/cancel only.

**Runtime verification of Step 2: done (2026-11-05).** `cannotPlaceOrderWithNonExistentCoupon` (ending in `noOrderPersisted`) is green on API and UI channels on all 6 combos (monolith and multitier × Java / .NET / TypeScript), via `gh optivem system start` + `gh optivem system-test run --suite acceptance --test <name>` + `system stop`. No shop changes needed. Re-verified the same day as meta-plan unit C2a on a fresh clone of shop `main` (`dd481b4`): green on all 6 combos. A fresh Linux dev container needs JDK 21, the .NET 8 SDK, the Chromium shared libraries (`libglib2.0-0t64`, `libnss3`, ...; even the API channel launches a browser) and `npm ci` in `system-test/typescript`, otherwise the tests fail on env, not on code. Side finding (gh-optivem, not shop): for .NET, `--test cannotPlace...` (camelCase) runs and passes the test but the runner's post-run name check (`nameExecuted` in `internal/build/runner/tests.go`, case-sensitive `strings.Contains`) reports "requested test(s) never executed" and exits 1; with the PascalCase name it exits 0.

Remaining (both need ATDD agent runs):
1. **Fixer rehearsal check** - rehearse the Layer 3 `scope-task-name: ${task-name}` compile-tests fixer change. Blocked 2026-11-05: `implement` preflight fails on the `gh` token's missing `project` scope. Also needs an attended run: `fix-*` dispatches are `category: human`, so an `--auto --headless` run pauses (exit 2) at the fixer's approval rather than dispatching it.
2. **Step 6** - reinstate #68's criterion once an absence-shaped ticket goes green by copying the shop reference. Gate not met yet: no absence-shaped ticket is in the rehearsal corpus (#68's criterion is dropped, #69 has none).

## Steps

### Short-term (unblock the corpus)

- [x] **Step 1 — Drop the absence criterion from #68 (temporary).** ✅ 2026-06-20 — removed `And no order is persisted` from the "Zero quantity is rejected" scenario in `optivem/shop#68` (validation-rejection assertion kept). Note: deferred plan `20260617-1437` explicitly rejected this as a *permanent* fix (silently drops a real criterion, lowering fidelity) — so this is strictly a temporary unblock, tracked by Step 6.

### Long-term Layer 1 — `shop` reference example (root / class-killing fix; supersedes & absorbs 20260617-1437)

> Lives in the **`academy/shop` repo**, not in any gh-optivem BPMN/commands/agents layer — this is the reference the agents pattern-match against.
>
> **This section absorbs the now-deleted plan `20260617-1437`.** Its full Java/.NET/TS design is folded in below; the only thing layered on top is the #68 lesson that the older plan predates — the shared port method must be **fan-out-complete** (present on *every* conformant `*Failure` core class, not just the one in the current scenario).

- [x] **Step 3 — Reconcile `20260617-1437`.** ✅ Folded into Step 2 (design decision, current-behaviour notes, per-language Items 1–3, alternatives) and the standalone plan deleted, per the delete-completed-plans convention.

  **Alternatives considered (carried from 20260617-1437):**
  - *Persistence-level absence assertion (deferred — stronger but disproportionate).* Issue a fresh browse/list-orders query and assert the order is genuinely absent. Strongest fidelity, but shop has no list/browse-orders use case, so it needs a new use case + driver adapter ×3 (plus possibly a system query endpoint), and for validation-rejection criteria the order is never created by construction. Revisit under the rule of three if a non-validation side-effect-absence class of tickets appears.
  - *Java-only fix now, mirror later (rejected).* Leaves .NET/TS agents reinventing the broken primitive — the mirror invariant is the whole point of an example.
  - *Drop the `.and().order().doesNotExist()` tail from acceptance tests (rejected as a permanent fix).* Silently drops a real criterion, lowering fidelity — opposite of the "accuracy over speed" gate. (The #68 drop in Step 1 is strictly temporary, tracked by Step 6.)

### Layers 2 and 3 - done (2026-11-03)

- Layer 2: fan-out guardrail added to `acceptance-test-writer.md` (shared-port widening must be stubbed on every conformant core class, then `gh optivem system-test compile`). Tradeoff: prompt-only and agent-discipline-dependent; Layer 1 supplies the concrete reference.
- Layer 3: verified the compile-tests fixer had **no** scope - `compile-tests` re-pins `task-name`, shadowing the writer MID, so `originating-task-name` resolved to a non-writing process. Fixed by threading `scope-task-name: ${task-name}` on the compile-tests command call; the dispatcher and `phaseTaskName` prefer it, so the fixer inherits the writer's read/write scope (including the testkit DSL). Other command MIDs (`build-system`, `start-system`, ...) share the same shadowing but are out of scope here.

### Restore-gate

- [ ] **Step 6 — Reinstate #68's criterion.** Once Layer 1 (Step 2) lands and an absence-shaped ticket goes green by copying the `shop` reference, restore `And no order is persisted` to `optivem/shop#68` (undo Step 1). The criterion must not be permanently lost.

## Open questions

- ~~**Scope of the reference:** place/cancel only vs. broader absence class?~~ **Resolved (Q-7, 2026-11-03)** - GO, scoped to order place/cancel only; no broader absence class.
- ~~**Supersede vs. fold:** mark `20260617-1437` superseded-in-place, or fold its detailed Java/.NET/TS design into Step 2 and delete it?~~ **Resolved (2026-06-22)** — folded into Step 2 and the standalone plan deleted (Step 3 done).
- ~~**Layer 3 necessity**~~ **Resolved (2026-11-03)** - verified the fixer had no scope on the compile path; fixed (see Layers 2 and 3).
- ~~**Short-term edit ownership:** me via `gh` vs. you edit the issue.~~ **Resolved** — done via `gh` (Step 1, 2026-06-20).
