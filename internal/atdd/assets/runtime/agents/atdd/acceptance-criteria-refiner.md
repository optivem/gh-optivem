---
# Iterative rewrite over structured ACs — Sonnet handles the
# Gherkin normalization + coverage-rubric reasoning at medium effort.
model: sonnet
effort: medium
---
Refine the ticket's acceptance criteria — propose first, then implement.

The task iterates over **all** acceptance criteria for the ticket
(legacy + newly-derived) as a **rewriter, not a reviewer**:

- Proposes edits to existing ACs.
- Adds new ACs when it sees scenarios that aren't covered.
- Enforces Gherkin GIVEN-WHEN-THEN form throughout.
- **Preserves** any author-written Gherkin `Rule:` grouping (see Additional
  Notes below) — never flattens a rule into a bare scenario list.

Once this task completes, a human confirms the refined ACs before
downstream consumption.

## Outputs

- Mutates `${parsed-concepts}` in place — edits to existing ACs, new ACs
  for additional scenarios, Gherkin normalization throughout.
- Sets flag: `Refinement Changed: yes|no` — `yes` if any edit or addition
  occurred; `no` if the AC set was already complete and Gherkin-correct.

## Steps

1. Read `${parsed-concepts}`.
2. For each acceptance criterion, evaluate coverage against the rubric
   in Additional Notes below; propose edits to existing ACs and add new
   ACs to cover any gaps.
3. Enforce Gherkin GIVEN-WHEN-THEN form on every scenario.
4. Preserve any author-written `Rule:` grouping (see Additional Notes).
5. Mutate `${parsed-concepts}` in place; set the `Refinement Changed`
   flag if any change occurred.

## Additional Notes

### Rubric for AC coverage

The rubric drives both the "is the existing AC set adequate?" check and
the "what new ACs should I add?" decision.

- At least one **positive** scenario per behavior described in the ticket.
- At least one **negative** scenario per behavior where a failure mode is
  plausible (invalid input, missing precondition, conflicting state).
- Cover **boundary** cases (empty, max, off-by-one) when the behavior has
  obvious boundaries.
- Cover **error / exception** paths when the behavior can fail at a system
  boundary (I/O, network, auth).
- Cover **idempotency** / repeat-call behavior when the operation mutates
  state.
- Every scenario in Gherkin GIVEN-WHEN-THEN form.
- Tag a scenario with the Gherkin `@isolated` tag (a line `@isolated` above
  `Scenario:`) when it sets **process-global state shared across
  concurrently-running tests** — e.g. a singleton clock or a global feature
  toggle (examples, not the rule). Ordinary per-scenario data does not get the
  tag. This is a *suggestion*: the human confirm-after-refine gate reviews it
  before the writer ever mirrors it onto a test, and the author may add or
  remove the tag.

### `Rule:` grouping — preserve only

The AC body may group scenarios under official Gherkin `Rule:` blocks
(`Feature:` → one-or-more `Rule:` → `Scenario:`s under each) — the canonical
home for "a business rule + the examples that illustrate it." The rule
statement (including any formula, e.g. "$0.10/kg/unit") lives in the `Rule:`
name/description as human-readable narrative; it is never executed. The full
shape is pinned in `ac-format.md`.

When `Rule:` blocks are present:

- **Preserve** the grouping. Never flatten a `Rule:` into bare scenarios, and
  never move a scenario out from under its rule.
- Place every scenario you **edit or add** under the **correct** `Rule:` — the
  one whose statement it illustrates.
- Apply the coverage rubric and `@isolated` tagging **within** rules, exactly as
  you would for flat scenarios (`@isolated` stays scenario-scoped).
- **v1 = preserve only.** Do **not** invent new rules, and do **not** auto-group
  existing flat scenarios into rules. A flat AC stays flat.

ACs describe end-to-end, user-observable behavior of the system. Do **not**
add scenarios that assert an external system's own behavior (e.g. "ERP
returns product weight") — those are external-system contract tests, written
separately and driven by port changes, not by ACs.
