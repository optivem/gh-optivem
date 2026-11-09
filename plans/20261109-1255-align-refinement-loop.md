# 2026-11-09 11:55 UTC — Align: refinement loop until the PO agrees

## TL;DR

**Why:** Today `refine-backlog-item` → `refine-acceptance-criteria` dispatches the `acceptance-criteria-refiner` once (human tier) and ends. The articles and the Circle session (2026-10-30) describe Align as a loop:
1. The agent drafts the criteria with concrete numbers.
2. It checks for contradictions inside the story **and against the use cases in the repo**.
3. The Three Amigos refine.
4. The loop repeats until the PO agrees.

A rule written only in general terms ("the fee applies per order item") reads two ways until an example with numbers pins it down.
**End result:** Refinement loops until the PO says "agreed". Every business rule in a refined story has at least one example with concrete numbers, and contradictions with existing use cases are reported with the narrative step, criterion ID and tests they would break.

Parent: `plans/20261109-1219-improved-atdd-process-flow.md`. See its "Refine Backlog Item (Align)" diagram and roles table.

## Outcomes

- `refine-backlog-item` loops `REFINE_ACCEPTANCE_CRITERIA → THREE_AMIGOS_REVIEW → GATE_AC_AGREED` until the PO agrees; READY is set only after "Yes".
- The refiner reports, per rule, whether a concrete example exists. Rules without one get a proposed example.
- The refiner reads the use cases (`docs/use-cases/`) and reports contradictions with existing behaviour, citing `UC-NNN.M`, the narrative step and the test names.
- The refiner flags optional steps written into a main success scenario (it should be the shortest path; optional steps become alternatives).

## ▶ Next executable step (resume here)

Review this plan with the user (`/refine-plan plans/20261109-1255-align-refinement-loop.md`). After that, Step 1 can run via `/execute-plan`.

## Steps

- [ ] Step 1: Reshape `refine-backlog-item` in `internal/atdd/process/process-flow.yaml`:
  - add `THREE_AMIGOS_REVIEW` (human)
  - add the `GATE_AC_AGREED` gateway, reusing the `approve` primitive's outcome binding
  - add the loop-back edge to `REFINE_ACCEPTANCE_CRITERIA`, with a `max-visits` cap

  Audit the loop-back against statemachine tests for deadlocks before running them.
- [ ] Step 2: Pass the previous round's feedback into the refiner on loop-back (a new placeholder, e.g. `${refinement-feedback}`) and update `acceptance-criteria-refiner.md` for draft vs. refine rounds.
- [ ] Step 3: Extend `acceptance-criteria-refiner.md` with three checks:
  - a concrete example per business rule
  - contradictions vs. the use cases in the repo
  - main success scenario = shortest path

  Pin its `read:` scope in `phase-scopes.yaml` to include the use-case directory.
- [ ] Step 4: Update statemachine/process tests and render-matrix tests for the new nodes and placeholder. Run with `-p 2`.

## Open questions

- Does `THREE_AMIGOS_REVIEW` need its own node, or is the existing human-tier POST approval on the refiner enough, with "Agreed?" as its outcome? A separate node is clearer in the diagram; folding it in means fewer prompts per round.
- Loop cap: how many refinement rounds before the run halts (e.g. 5)?
