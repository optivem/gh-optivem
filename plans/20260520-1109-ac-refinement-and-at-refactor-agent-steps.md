# Plan: turn acceptance-criteria-refinement and at-refactor docs into agent steps

## Purpose

Two ATDD phase docs are currently DRAFT stubs with bare prose steps and no
agent wiring. This plan is a discussion scaffold for how to convert them
into proper agent steps (prompt readers, agent prompts, gate plumbing).
The discussion happens against the contents pasted below; items will be
added as we walk through the conversion.

Source files (unchanged for now):

- `internal/assets/global/docs/atdd/process/analysis/acceptance-criteria-refinement.md`
- `internal/assets/global/docs/atdd/process/change/behavior/at-refactor.md`

---

## Source 1 — acceptance-criteria-refinement.md (current contents)

```
# ACCEPTANCE CRITERIA ANALYSIS (DRAFT)

1. Analyze Acceptance Criteria, is it written with Gherkin GIVEN-WHEN-THEN.
2. Does it have adequate positive and negative scenarios.
```

Observations / open questions (to discuss):

- Title says "ANALYSIS" but filename says "REFINEMENT" — which is canonical?
- Step 1 is a yes/no check on Gherkin form. Is the agent supposed to
  *rewrite* the AC into Gherkin if it isn't, or just flag the gap?
- Step 2 is a coverage check ("positive and negative scenarios"). Is
  "adequate" defined anywhere, or do we need a heuristic / rubric?
- No `## Scope` block, no link to `shared/scope.md`, no "propose first,
  then implement" framing — unlike the at-refactor doc.
- No inputs/outputs declared: what artifact does the agent read, and what
  does it write back?

---

## Source 2 — at-refactor.md (current contents)

```
# AT - REFACTOR (DRAFT)

Refactor the System if any improvements are seen — propose first, then implement.

## Scope

This phase touches the `system_path` layer (bare layer name; resolved
physical path lives in `gh-optivem.yaml system.path`). Production
system code only.

See [the scope rule](../../shared/scope.md).

## Steps

1. Refactor the System (if any improvements are seen) - propose first, then implement
```

Observations / open questions (to discuss):

- Scope is declared (good) — `system_path` layer, production code only.
- Single step that's effectively "look for improvements, propose, then
  implement". This is the same shape as other refactor phases — should we
  model it after an existing refactor agent (e.g. the post-green refactor
  step) or keep it standalone?
- "If any improvements are seen" implies a no-op exit path; does the gate
  treat "no refactor needed" as a discharge signal, and how?
- Does this phase have its own gate fixture? If so, where does it sit in
  `process-flow.yaml`?

---

## Items (to be filled in during the walk)

> Each item below will be expanded into concrete edits — prompt reader,
> agent prompt file, gate wiring, inline phase doc, language-equivalents
> update, etc. — once we've discussed it. Leaving as stubs deliberately.

5. **Reuse the existing phase-cycle template** used by `at-red` and
   `at-green` for `at-refactor`. The agent emits production-code changes
   only; the surrounding template handles run-tests + commit downstream
   (same wrapper that AT_GREEN_BACKEND → AT_GREEN_FRONTEND → COMMIT uses
   in `process-flow.yaml`).
   - **Only structural difference:** no `ENABLE_TESTS` / `DISABLE_TESTS`
     step. Refactor doesn't flip test enablement — it operates on
     production code with the test set already enabled from the
     preceding GREEN.
   - Concretely: define `at_refactor_system` (mirroring
     `at_green_system`) with the same COMMIT terminal but no
     enable/disable wrapper.
   - Agent prompt sits at `internal/assets/runtime/prompts/atdd/`
     alongside `at-green-system.md` (filename TBD during execution —
     `at-refactor-system.md` is the obvious choice).
6. Wire at-refactor into `process-flow.yaml`.

   **6b. at-refactor wiring**
   - New sub-process `at_refactor_system` mirroring `at_green_system`
     (line 424 of `process-flow.yaml`) — same COMMIT terminal, but
     **no** `ENABLE_TESTS` / `DISABLE_TESTS` wrapper.
   - States:
     - `AT_REFACTOR_SYSTEM` — runs the refactor agent on production
       code; emits a `refactor_changed` flag (or just discharges as
       no-op if no improvements were seen).
     - `COMMIT` — reuses the existing COMMIT terminal (see line 996
       comment in `process-flow.yaml`).
   - Transitions:
     - `AT_REFACTOR_SYSTEM` → `COMMIT` when `refactor_changed == true`
     - `AT_REFACTOR_SYSTEM` → `AT_END` (or wherever the ATDD cycle
       exits) when `refactor_changed == false` (no-op discharge)
     - `COMMIT` → `AT_END`
   - Slot the new sub-process after `at_green_system` completes, before
     the cycle exits.
7. Inline the at-refactor phase doc into its prompt reader, mirroring
   the pattern from commit `4b44722`.
   - `at-refactor.md` → `runtime/prompts/atdd/at-refactor-system.md`
     (per item 5).
8. Update `docs/atdd/code/language-equivalents.md` **only if at-refactor
   introduces language-specific behavior**. backlog-refinement is
   language-agnostic (it operates on tickets + parsed-concepts artifact,
   not source code) — no update needed for that phase. For at-refactor,
   add content only if refactor heuristics differ meaningfully across
   Java / .NET / TypeScript (e.g. idiomatic equivalents of a refactor
   pattern). If the agent is generic enough that the same prompt works
   for all three languages without per-language guidance, skip this
   item.
9. Drop the `(DRAFT)` suffix from the at-refactor phase doc H1 title
   once items 5–8 land:
   - `at-refactor.md`.

---

## Pickup

Items 1–4 landed in session at 2026-05-20T10:13:47Z (see commit log).
Slice B (item 6a + refine-acc/update-ticket parts of items 7 and 9)
landed at 2026-05-20T10:49:48Z (see commit log).
Items 5, 6b, 7 (at-refactor), 8, 9 (at-refactor) deferred to fresh
sessions.
