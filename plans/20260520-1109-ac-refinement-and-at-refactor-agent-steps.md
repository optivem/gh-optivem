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

1. Rename `acceptance-criteria-refinement.md` → `backlog-refinement.md`
   and reposition it as a **backlog-refinement cycle** that runs after
   ticket-parse / concepts extraction and **before** the execution cycles
   (behavioral / structural). Lifecycle:
   1. Cycle iterates over **all** acceptance criteria for the ticket
      (legacy + newly-derived).
   2. Rewriter behavior (not reviewer): proposes edits to existing ACs,
      adds new ACs when it sees additional scenarios that aren't covered,
      enforces Gherkin GIVEN-WHEN-THEN format throughout.
   3. **User confirms** the refined concepts/ACs (human gate).
   4. If any changes occurred during refinement, the **`UPDATE_TICKET`**
      step runs (writer defined in item 2). If refinement produced no
      changes, `UPDATE_TICKET` is skipped (no-op discharge).
   5. Only then do the execution cycles (AT behavioral / structural)
      proceed.
   Confirm placement in `process-flow.yaml` (covered in item 6) and the
   final filename / dir (`process/analysis/` vs a new `process/backlog/`
   location).
2. Define inputs/outputs for the backlog-refinement cycle and its
   downstream `UPDATE_TICKET` step.
   - **Input (cycle):** the parsed-concepts artifact emitted by the
     upstream parse-ticket / concepts phase (structured ACs ready to
     refine). Raw ticket is not re-read.
   - **Working output (cycle):** mutates the parsed-concepts artifact
     in place — edits to existing ACs, new ACs for additional scenarios,
     Gherkin normalization. Tracks whether *any* change occurred (for
     the conditional `UPDATE_TICKET` below).
   - **`UPDATE_TICKET` step (new, downstream of the cycle):** runs only
     if refinement produced changes. Overwrites three sections of the
     ticket source: `Description`, `Legacy Acceptance Criteria`, and
     `Acceptance Criteria` — populated from the refined parsed-concepts
     artifact. Other ticket sections are left untouched.
   - **Skip path:** if refinement produced no changes, `UPDATE_TICKET`
     does not run; the cycle discharges silently into the execution
     cycles.
   - Open: where does `UPDATE_TICKET` live — its own phase doc / agent,
     or a sub-step inside `backlog-refinement.md`? Decide alongside
     item 6 (process-flow wiring).
3. Add a `## Scope` block to `backlog-refinement.md`. Unlike at-refactor
   (which scopes to a code layer), backlog-refinement scopes to the
   **ticket source's three named sections** — `Description`,
   `Legacy Acceptance Criteria`, `Acceptance Criteria` — plus the
   **parsed-concepts artifact** as the working store. Production system
   code and tests are explicitly out of scope. Link to `shared/scope.md`
   for the cross-phase scope rule, same as at-refactor.
4. Embed a short, opinionated **rubric for AC coverage** inline in
   `backlog-refinement.md`. Self-contained — no external pointer. The
   rubric drives both the "is the existing AC set adequate?" check and
   the "what new ACs should I add?" decision. Initial rubric content (to
   be tightened during execution):
   - At least one **positive** scenario per behavior described in the
     ticket.
   - At least one **negative** scenario per behavior where a failure mode
     is plausible (invalid input, missing precondition, conflicting
     state).
   - Cover **boundary** cases (empty, max, off-by-one) when the behavior
     has obvious boundaries.
   - Cover **error / exception** paths when the behavior can fail at a
     system boundary (I/O, network, auth).
   - Cover **idempotency** / repeat-call behavior when the operation
     mutates state.
   - Every scenario in Gherkin GIVEN-WHEN-THEN form.
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
6. Wire both phases into `process-flow.yaml`.

   **6a. backlog-refinement wiring**
   - New sub-process (e.g. `backlog_refinement`) slotted **after**
     parse-ticket / concepts and **before** the first execution cycle
     (at_red / at_green).
   - States:
     - `BACKLOG_REFINEMENT` — runs the refinement agent; mutates the
       parsed-concepts artifact and sets a `refinement_changed` flag.
     - `CONFIRM_REFINEMENT` — user-confirm gate (human approval).
     - `UPDATE_TICKET` — runs only when `refinement_changed == true`;
       overwrites the ticket's `Description`, `Legacy AC`, and `AC`
       sections.
   - Transitions:
     - `BACKLOG_REFINEMENT` → `CONFIRM_REFINEMENT`
     - `CONFIRM_REFINEMENT` → `UPDATE_TICKET` when
       `refinement_changed == true`
     - `CONFIRM_REFINEMENT` → `<execution-cycle-entry>` when
       `refinement_changed == false` (no-op discharge skips
       `UPDATE_TICKET`)
     - `UPDATE_TICKET` → `<execution-cycle-entry>`
   - Open: name of the execution-cycle entry state (likely
     `AT_RED_TEST` based on existing flow at line 318 of
     `process-flow.yaml`).

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
7. Inline the phase docs into their prompt readers, mirroring the
   pattern from commit `4b44722` (the 11 phases already inlined).
   Targets:
   - `backlog-refinement.md` (post-rename from
     `acceptance-criteria-refinement.md` — see item 1).
   - `at-refactor.md`.
   - **Conditional:** if item 2's open question resolves to
     `UPDATE_TICKET` getting its own phase doc (rather than a sub-step
     inside `backlog-refinement.md`), inline that doc too.
8. Update `docs/atdd/code/language-equivalents.md` **only if at-refactor
   introduces language-specific behavior**. backlog-refinement is
   language-agnostic (it operates on tickets + parsed-concepts artifact,
   not source code) — no update needed for that phase. For at-refactor,
   add content only if refactor heuristics differ meaningfully across
   Java / .NET / TypeScript (e.g. idiomatic equivalents of a refactor
   pattern). If the agent is generic enough that the same prompt works
   for all three languages without per-language guidance, skip this
   item.
9. Drop the `(DRAFT)` suffix from the two phase doc H1 titles once
   items 1–8 land:
   - `backlog-refinement.md` (post-rename from
     `acceptance-criteria-refinement.md` per item 1 — the rename and
     the DRAFT-drop can happen in the same edit pass).
   - `at-refactor.md`.

---

## Pickup

Not yet picked up. Discussion-first plan — items become concrete edits
as we walk them.
