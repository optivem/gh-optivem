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

Refinement is a loop (Align): you draft, the Three Amigos (PO, developer,
tester) review your draft at the approval that follows this task, and if
they do not agree you run again with their feedback. The loop ends when the
PO agrees.

${refinement-feedback-section}

## Outputs

- Mutates `${parsed-concepts}` in place — edits to existing ACs, new ACs
  for additional scenarios, Gherkin normalization throughout.
- Sets flag: `Refinement Changed: yes|no` — `yes` if any edit or addition
  occurred; `no` if the AC set was already complete and Gherkin-correct.
- Ends with a **refinement report** in your final message, for the Three
  Amigos to review before they agree:
  - per business rule: whether a scenario with concrete numbers illustrates
    it, or which example you proposed;
  - each contradiction with an existing use case;
  - each optional step you moved out of a main success scenario.
- `open-questions` (optional): one line per question only the Three Amigos
  can settle (a proposed example the PO must confirm, a contradiction to
  resolve). Omit it when nothing is open.

${expected-outputs}

## Steps

1. Read `${parsed-concepts}`. On a refine round (a "Feedback from the
   previous round" section is shown above), it already holds your previous
   draft: apply the feedback and settle the open questions it lists, and
   leave everything else as it is.
2. For each acceptance criterion, evaluate coverage against the rubric
   in Additional Notes below; propose edits to existing ACs and add new
   ACs to cover any gaps.
3. Check that every business rule has a concrete example (see "Concrete
   example per business rule" below).
4. Check the criteria against the use cases in the repo (see "Contradictions
   with existing use cases" below).
5. Check that each main success scenario is the shortest path (see "Main
   success scenario is the shortest path" below).
6. Enforce Gherkin GIVEN-WHEN-THEN form on every scenario.
7. Preserve any author-written `Rule:` grouping (see Additional Notes).
8. Mutate `${parsed-concepts}` in place; set the `Refinement Changed`
   flag if any change occurred. Write the refinement report and emit
   `open-questions` when anything is open.

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

### Concrete example per business rule

A rule stated only in general terms ("the fee applies per order item")
can be read two ways until an example with numbers pins it down. A
business rule is a `Rule:` block, or a rule the ticket states in prose.

- For each rule, find at least one scenario that illustrates it with
  concrete values: quantities, amounts, dates, statuses (e.g. "Given 3
  items at $10, Then the fee is $1.50"), not "a valid order" or "some
  items".
- If a rule has none, propose one: add a scenario with concrete numbers
  that shows your reading of the rule, and add an `open-questions` line
  asking the PO to confirm the example.

### Contradictions with existing use cases

The use cases are the current spec of the system's behaviour. Find their
directory: `specDir` in `spec.yaml` at the repo root, else `docs/use-cases`.
Read the use case files there (`UC-NNN-<slug>.md`: a narrative plus
`UC-NNN.M` criteria). If there are none, skip this check.

- Compare every scenario with the use cases. A contradiction is the same
  condition with a different outcome, where the ticket does not say it
  intends to change that behaviour.
- For each contradiction, report the criterion id (`UC-NNN.M`), the
  narrative step it comes from, and the tests that verify it: search the
  test code for the criterion id; tests are tagged with the criterion ids
  they verify. Those tests would break if the story were implemented as
  written.
- Do not edit the use cases and do not resolve the contradiction yourself.
  Add an `open-questions` line for each one, naming the scenario and the
  criterion id.

### Main success scenario is the shortest path

The main success scenario is the shortest path to the goal. A step that a
user may or may not take (an optional coupon, a gift note) does not belong
in it.

- If a main success scenario includes an optional step, move that step
  into its own alternative scenario and keep the main scenario on the
  shortest path. Report each move in the refinement report.

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
