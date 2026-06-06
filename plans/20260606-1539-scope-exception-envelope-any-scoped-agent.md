# Plan: Seed the scope-exception envelope for any non-`none`-scope dispatch

> 🤖 **Picked up by agent** — `Valentina_Desk` at `2026-06-06T13:53:09Z`

## TL;DR

**Why:** The write-time output allow-list seeds the `scope-exception-*` envelope by `category == "prod-agent"`, so three MIDs with a real `write:` scope — `refactor-tests` and the two `fix-unexpected-*-tests` agents — can't emit the envelope; an out-of-scope need there fails validation → FIX → burns the 2-attempt cap and halts at `AGENT_FIX_EXHAUSTED` instead of the clean `STOP_SCOPE_VIOLATION`.
**End result:** the seeding gates on whether the dispatch has a non-`none` scope (via `Engine.IsScopeNone`), not on category, so every scoped agent can raise the honest Layer-1 halt; `scope: none` dispatches (`refine-acceptance-criteria`) stay exempt; the doctrine prose and tests match.

> **DECISION MADE (2026-06-06):** the universal `scope-exception-*` envelope is seeded for **every
> dispatch whose MID has a non-`none` scope**, not only `category: prod-agent`. This lets
> `refactor-tests` and the two `fix-unexpected-*-tests` agents raise the honest Layer-1 scope-violation
> halt instead of churning the FIX loop to exhaustion. `scope: none` dispatches
> (`refine-acceptance-criteria`) stay exempt. Settled in review discussion.
>
> Review finding §3a. Independent plan from the `process-flow.yaml` review; no dependency on the other
> review plans.

## Why

The scope-exception envelope (`scope-exception-files` / `scope-exception-reason`,
`statemachine.EnvelopeOutputSpecs`) is the *honest* response to a too-narrow `write:` scope: the agent
refuses, emits the envelope, and the run **halts cleanly** at `STOP_SCOPE_VIOLATION` for a human to
widen the scope. Without the envelope, an out-of-scope need instead fails validation → routes to FIX →
the fixer can't widen a YAML scope → burns the 2-attempt cap → halts at `AGENT_FIX_EXHAUSTED` with a
murkier message and wasted dispatches.

The envelope is seeded into the write-time allow-list (`GH_OPTIVEM_OUTPUT_KEYS`) by the dispatch
switch in `driver.go:1146-1166`:

- `len(outs) > 0` → the MID's declared outputs (the 3 writers + `implement-dsl`; all list the envelope
  explicitly).
- `nodeParams["category"] == "prod-agent"` → envelope-only (covers `implement/update-system`, the four
  driver-adapter MIDs, `implement-external-system-stubs`, `refactor-system`).
- else → unwired.

So three MIDs with a **real write scope** fall into the `else` and cannot raise the clean halt:
`refactor-tests` (`category: test-agent`) and `fix-unexpected-passing-tests` /
`fix-unexpected-failing-tests` (`category: human`). The seeding keys on **category** when it should key
on **whether the dispatch has a scope to violate**. The envelope is opt-in (an agent emits it only when
it genuinely refuses) and always halts for human review — so granting it more broadly changes nothing
unless an agent actually hits the wall, and it can never silently widen a scope.

## Design (chosen)

Replace the `category == "prod-agent"` seeding gate with a **non-`none`-scope** gate, and always merge
`EnvelopeOutputSpecs()` onto whatever the MID declares (deduped) for non-`none`-scope dispatches. Use
the existing `Engine.IsScopeNone(scopeKey)` helper (the same `scope: none` doctrine
`refine-acceptance-criteria` relies on). Net effect:

| Dispatch | Allow-list seeded |
|---|---|
| declares outputs, non-`none` scope (3 writers, `implement-dsl`) | declared outputs (already include envelope) |
| no declared outputs, non-`none` scope (all prod-agent MIDs **+ `refactor-tests` + the 2 fix agents**) | envelope-only |
| `scope: none` (`refine-acceptance-criteria`) | unwired (unchanged) |

Single backend — remove the category-based branch, do not leave a category path beside a scope path
per `[[feedback_testselect_parsing_escalation]]`.

## Items

1. **`internal/atdd/runtime/driver/driver.go` (≈1146-1166).** Rewrite the seeding switch so the
   channel is wired when `outputFilePath != ""` AND (`len(outs) > 0` OR
   `!eng.IsScopeNone(scopeKey)`); the allow-list = declared `outs` plus `EnvelopeOutputSpecs()` merged
   in (skip duplicates so the 3 envelope-declaring writers don't double-list). Keep the `else` unwire
   for `scope: none` / no-MID dispatches intact (it still must clear `output-file-path` to avoid the
   stale-JSONL clobber documented at the original `default`). Confirm `Engine.IsScopeNone` exists and
   takes the same `scopeKey` used for `eng.Outputs`.
2. **Update the doc-block** (`driver.go:1124-1135`). Replace "every prod-agent dispatch must be able to
   emit the scope-exception envelope" with the non-`none`-scope rule, naming `refactor-tests` and the
   fix agents as now-included.
3. **`internal/assets/runtime/shared/scope.md`.** Update the doctrine statement so the prose SSoT
   matches: any agent with a non-`none` scope may emit the envelope (not just production-code agents).
   Keep it tight per `[[feedback_flag_non_token_efficient]]`.
4. **Tests.** Update the seeding tests (the prod-agent-gets-envelope / non-prod-doesn't assertions in
   `driver/*_test.go` and the envelope test in `actions/bindings_test.go` ≈1536): assert
   `refactor-tests`, `fix-unexpected-passing-tests`, and `fix-unexpected-failing-tests` dispatches now
   seed the envelope keys into the allow-list, and `refine-acceptance-criteria` (`scope: none`) still
   does not. Scope `go test` per `[[feedback_go_test_windows.md]]`.

## Verification

- Dispatch a `refactor-tests` run that attempts an out-of-scope write and confirm it halts at
  `STOP_SCOPE_VIOLATION` (clean) rather than `AGENT_FIX_EXHAUSTED` (churned).
- Confirm a `refine-acceptance-criteria` run is unaffected (no `GH_OPTIVEM_OUTPUT_*` exported).
