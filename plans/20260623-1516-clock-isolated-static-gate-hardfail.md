# 2026-06-23 15:16:00 UTC — Static gate: a clock-driving acceptance test must be `@isolated` (hard-fail at compile)

> **Supersedes** `plans/deferred/20260618-1601-clock-driving-scenario-isolated-gate.md`.
> That plan's decision gate said *"build it once a real near-miss occurs."* Rehearsal run
> `20260623-100107` (#76) is that occurrence — an un-isolated clock-driving test flaked on the
> UI channel and halted the run. Decision gate is now **GO**. Once this plan is committed,
> delete the deferred file (its content is folded in below).

## TL;DR

**Why:** A clock-driving acceptance test that is **not** marked `@isolated` runs in parallel, races on the singleton clock, and flakes — the #76 bug. The invariant ("clock-driving ⇒ `@isolated`") exists only as a review/authoring convention: the `acceptance-criteria-refiner` that would tag it doesn't run in the implement/rehearsal flow, and the `acceptance-test-writer` is deliberately a mechanical tag-mirror that won't add a missing tag. So an un-isolated clock-driving test reaches verify with nothing stopping it.

**End result:** A static check, invoked inside the ATDD flow at compile time, **hard-fails the run loudly and early** (seconds in, before any implement work) when a test calls the clock-control DSL but lacks its language isolation marker — naming the offending test. The missing tag can never again surface as a non-deterministic UI-channel flake 30 minutes later that dies at a human-approval gate.

## Outcomes

What we get out of this:

- A clock-driving acceptance test with no `@isolated` marker **fails the ATDD run at compile time** (~`compile-tests` step, seconds in), not 30 minutes later at UI verify.
- The failure is **loud and specific**: a non-zero exit with a message naming the test, e.g. `clock-driving test shouldRejectCancellationAt2245OnDecember31st is missing @isolated`, halting at a clean dedicated terminal — no silent normalization, no auto-added tag.
- The check covers **all three languages** (TypeScript / Java / .NET), keyed on each clock-control DSL entrypoint and requiring each language's isolation marker.
- A **regression fixture / unit test** proves the check rejects an un-isolated clock-driving test and passes an isolated one.
- The #76 clock-isolation flake class is closed at the **earliest, cheapest point** — replacing a ~$3.66 / 30m41s confusing halt with a ~5s actionable one.
- The deferred plan `20260618-1601` is resolved and removed.

## ▶ Next executable step (resume here)

**Design/decide first, then build** — start by resolving the two Open Questions below (where the check is invoked from, and the exact per-language clock-DSL identifiers in the *current* shop DSL), since the implementation shape depends on them. Concretely: grep the shop `system-test/**` tree for the clock-control entrypoint per language (TS `.clock().withTime(`, Java `.clock().withTime(`, .NET `.Clock().WithTime(`) and confirm the isolation marker shape each test carries, then pick the invocation point (recommended: a shop-side static check invoked by the ATDD `compile-tests` step so it hard-fails the run early). Once those are settled, Step 2 onward is mechanical. If only the decision remains, refine this plan via `/refine-plan` before `/execute-plan`.

## Steps

- [ ] **Step 1 — Confirm identifiers & marker shapes (per language).** In the shop repo, grep `system-test/**/acceptance/**` for the clock-control DSL entrypoint and isolation marker per language and pin the exact literals the check will key on:
  - **TypeScript:** clock call `.clock().withTime(`; marker = `test.describe('@isolated', …)` serial wrapper (a test included under Playwright `--grep-invert '@isolated'` is, by definition, NOT isolated).
  - **Java:** clock call `.clock().withTime(`; marker = class-level `@Isolated(` (carries `@Tag("isolated")`).
  - **.NET:** clock call `.Clock().WithTime(`; marker = class-level `[Collection("Isolated")]` **and** `[Isolated(`.
- [ ] **Step 2 — Write the static check (one per language).** A source-text/AST scan over `system-test/**/acceptance/**` that flags any test source which calls the clock DSL but lacks the isolation marker. Keep it simple and language-local; emit a message naming each offending test/file. Non-zero exit on any violation.
- [ ] **Step 3 — Invoke it inside the ATDD flow at compile time.** Wire the check so the `compile-tests` step (or an adjacent lint sub-step it calls) runs it and a violation makes the run hard-fail at a clean dedicated terminal — *before* `start-system` / implement. (See Open Question 1 for the exact home.)
- [ ] **Step 4 — Regression fixture / unit test.** Prove the check rejects a deliberately-untagged clock-driving test and passes an isolated one, per language (or at minimum a unit test of the check logic).
- [ ] **Step 5 — (If shop-side) wire into shop CI too** so a violation also fails the shop build independently of the ATDD flow.
- [ ] **Step 6 — Resolve the deferred plan.** Delete `plans/deferred/20260618-1601-clock-driving-scenario-isolated-gate.md` (content folded here).

## Decisions (resolved)

- **Hard-fail, not auto-fix.** A missing `@isolated` on a clock-driving test is a deterministic *authoring* defect, not a transient condition. The gate hard-fails loud; it does **not** auto-add the tag. Auto-tagging would be silent normalization (against the repo's fail-loud rule) and would reintroduce the isolation *judgement* the `acceptance-test-writer` was deliberately stripped of (it only mirrors).
- **Early, at compile time.** Fire at/around `compile-tests` (seconds in), not at verify — so the run dies before the ~30m / $3.66 of implement work.
- **Layer scope = static-check only.** No BPMN changes (no channel-unroll change, no AC-isolation-pass node) and no agent-prompt changes (the writer does not learn to self-judge isolation). Selected deliberately during the postmortem.

## Open questions

1. **Where does the check live / get invoked?** Two ends to reconcile:
   - The DSL identifiers live **shop-side**, so the check itself naturally lives there (deferred plan's recommended Option A: a shop-side static check / lint / contract test over `system-test/**`).
   - The prevention goal requires it to fire **inside the ATDD flow** so it hard-fails the rehearsal before implement.
   - *Recommendation:* a shop-side static check that the ATDD `compile-tests` step invokes — gets both (early hard-fail in the flow **and** an independent shop-CI failure via Step 5). Confirm `gh optivem test compile` is the right hook vs. a new dedicated lint step.
2. **Exact clock-DSL identifier in the *current* shop DSL** per language — confirm against today's source (Step 1), in case the entrypoint has changed since the parent isolation plan.
3. **Marker-detection robustness.** Is a literal source-text scan sufficient, or is a light AST/structural check needed (e.g. a TS test nested inside a `describe('@isolated')` several lines up, or a Java annotation on the enclosing class vs. method)? Start with source-text; escalate to AST only if false negatives appear.
