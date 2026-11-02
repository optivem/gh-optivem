# 2026-06-16 09:22:00 CEST — Scope-drift guard for the verify-tests-pass fix loop

> ⏸️ **DEFERRED — needs discussion before any build.** Moved to `plans/deferred/` on 2026-06-16.
>
> **Open meta-question (supersedes Q1–Q4 below): should this guard exist at all?** During refine it surfaced that the scope-drift guard is a *third* overlapping backstop (count cap + no-progress already halt the loop), that it requires inventing a new "system-under-test layer" concept that doesn't exist in `gh-optivem.yaml` and already has a multitier hole (`system-path` is monolith-only), and that its own Problem section flags an asymmetric too-narrow/too-broad risk in the most collision-prone file in the repo.
>
> **Leaning (not yet decided — discuss first):** *drop the auto-halting guard, keep only the diagnostic.* I.e. don't have the orchestrator make a relevance judgment; instead enrich the existing count-cap / no-progress `_EXHAUSTED` halt message with the layers/files the fixer edited across its passes (reuse `phase-changed-files`), so a human arriving at the halt immediately sees "only edited tests/DTOs, never the system" — the #69 value — with no new SUT-scope concept and no false-halt risk. The fuller Q1–Q4 guard design below is the alternative if discussion decides the auto-halt is worth its cost.

> Spun off from `plans/20260615-1845-fix-flow-interactive-stall-and-frame-mismatch.md` (Step 4). That plan's **no-progress** guard landed (commit on the same session); this plan carries the **scope-drift** half, which needs a design decision before it can be built. Cross-references the parent's Conclusion / Q3 and the now-shipped `check-fix-progress` / `GATE_FIX_PROGRESSING` wiring.

## TL;DR

**Why:** The `unexpected-failing-tests-fixer` is dispatched with a write scope that is the **union of every layer** (`process-flow.yaml` `fix-unexpected-failing-tests` read/write lists). That is deliberate — the fixer doesn't know a-priori which layer is wrong — but it means a fixer can "fix" a red acceptance test by editing a driver-port DTO or a test file instead of the production code under test, drift that the no-progress guard won't catch (the failure *does* change) and the count cap won't catch (it bounds attempts, not relevance). Run #69's fixer edited driver-port + core DTOs and broke test compilation; a scope-drift guard is the orchestrator-side backstop for that shape.

**End result:** When a fix pass's only edits fall **outside the system-under-test scope of the red test**, the verify-tests-pass loop halts for a human (a sibling terminal to `FIX_LOOP_NO_PROGRESS_EXHAUSTED`) instead of letting the fixer keep editing the wrong layer. Built on the existing `validate-outputs-and-scopes` / `phase-changed-files` / `ResolveLayerPaths` machinery, layered under the no-progress guard and the count cap.

## Problem — why this needs design, not just code

"Halt when the fixer's only edits fall outside the SUT scope of the red test" has **no executable meaning yet**, because:

- The fixer's *legal* `write:` scope is already every layer, so a check against the fixer's own declared scope is a no-op — it never fires.
- There is **no existing notion of "the SUT scope of the red test."** `Engine.Scope(taskName)` returns a writing-agent MID's read/write layers; there is no suite→SUT-layer mapping and no "production code only" layer set in `gh-optivem.yaml` today.

So the guard requires a **new, narrower definition** of the expected fix scope, and the consequences of getting it wrong are asymmetric and expensive: too narrow → halts legitimate fixes (e.g. a fix that correctly reshapes a coupled test surface); too broad → never fires (dead code in the most collision-prone file in the repo).

## Building blocks already in place (reuse, don't rebuild)

- `validate-outputs-and-scopes` stamps `ctx.State["phase-changed-files"]` on **every** dispatch — the newline-joined snapshot delta of what the WRITE phase (here, the fixer) actually edited (`internal/atdd/process/actions/bindings.go`). This is the fixer's edit set.
- `ResolveLayerPaths(layerKeys, cfg)` resolves Family-B layer keys → concrete path roots; `pathInScope(path, allowed)` is the directory-aware membership test. Both are already used by the scope check.
- The no-progress guard's wiring is the template: a `service-task` (`check-fix-progress`) + boolean `gateway` (`GATE_FIX_PROGRESSING`) on the fail branch, halting at an `_EXHAUSTED`-suffixed error-end terminal. The scope-drift guard slots in beside it on the same fail branch.

## Open questions — RESOLVE FIRST (via /refine-plan)

1. **What *is* "the SUT scope of the red test"?** Candidate definitions, pick one:
   - (a) **Production code only** — the union of all *non-test, non-DSL-port* layers (system-path + driver/adapter layers + core). A fixer whose edits touch **only** `at-test` / `ct-test` / `dsl-port` while a behaviour is red is drifting. Simple, language-agnostic, matches the #69 shape (fixer edited DTOs/tests, not the system). Risk: legitimately reshaping a test coupled to a renamed surface looks like drift.
   - (b) **Suite-derived layer set** — map the verify's `suite` param (acceptance | contract | …) to the layers that suite legitimately exercises. More precise, but needs a new suite→layers table that doesn't exist and must be kept in sync.
   - (c) **Exclude-list only** — flag drift only when **100%** of edits are in a small hard "never the sole fix" set (e.g. driver-port DTOs alone). Most conservative; fires rarely but never wrongly.
   - *Recommendation to evaluate first: (a), with the "only edits" framing (drift = the edit set is non-empty AND entirely outside the SUT scope), so a fix that touches production code at all is never flagged.*
2. **Where does the guard run, and does it share `CHECK_FIX_PROGRESS` or get its own node?** Fold the scope check into the existing `check-fix-progress` action (one fail-branch service-task stamping two verdicts) vs. a second service-task + gateway in series. Folding is less YAML churn; separate nodes keep each halt terminal's cause crisp in the trace/diagram.
3. **Interaction ordering with no-progress.** If a pass both makes no progress AND drifts, which halt wins / which message surfaces? (Likely: scope-drift is the more actionable diagnosis, so check it first — but confirm.)
4. **First-pass behaviour.** Scope-drift can be judged on the *first* fixer pass (we have `phase-changed-files` immediately), unlike no-progress which needs two passes. Confirm we want it to fire as early as the first pass.

## Steps (draft — finalise after Q1–Q4)

- [ ] **Step 1 — Resolve Q1–Q4** (/refine-plan), write the chosen SUT-scope definition into this plan.
- [ ] **Step 2 — Scope-drift verdict.** Extend `check-fix-progress` (or add `check-fix-scope`) to read `phase-changed-files`, resolve the SUT scope via `ResolveLayerPaths`, and stamp a `fix-scope-drifted` (or fold into `fix-loop-progressing`) verdict. Reuse `pathInScope`.
- [ ] **Step 3 — Wire the halt** in `verify-tests-pass`: gateway branch → a new `FIX_SCOPE_DRIFT_EXHAUSTED` error-end terminal (or reuse the progress gateway with a 3-way route). Keep the no-progress + count-cap layering intact.
- [ ] **Step 4 — Tests.** Unit: edits entirely outside SUT scope → drift verdict; edits touching production code → no drift; empty edit set → no drift (fails safe). Transitions: the new edge + terminal kind. Regression: a normal headless fix flow that edits production code is unchanged. Scope `go test` per-package (no unbounded `./...` on Windows).

## Verification

- Confirm by eye on a story where the fixer is induced to edit only a test/DTO while an acceptance test is red: the loop halts at the scope-drift terminal with an operator-actionable message, not after burning the full count cap.

## Notes / constraints carried from the parent plan

- Layer **under** `max-visits: 2` (count) and beside the no-progress guard — this is the *relevance* backstop the count cap can't provide.
- `process-flow.yaml` + `actions/bindings.go` are collision-prone: re-confirm a clean tree + grep `plans/*.md` for live pickup markers before executing.
- Do **not** add a local diagram-regeneration step — the GH Actions workflow regenerates `docs/process-diagram.md` + SVGs on push to main.
