# REFINE ACC (DRAFT)

Refine the ticket's acceptance criteria — propose first, then implement.

## Role in the flow

This cycle runs **after** parse-ticket / concepts extraction and **before**
the execution cycles (AT behavioral / structural). It iterates over **all**
acceptance criteria for the ticket (legacy + newly-derived).

The refiner is a **rewriter, not a reviewer**:

- Proposes edits to existing ACs.
- Adds new ACs when it sees scenarios that aren't covered.
- Enforces Gherkin GIVEN-WHEN-THEN form throughout.

Once the refiner discharges, the **user confirms** the refined ACs (human
gate). If refinement produced changes, a downstream `UPDATE_TICKET` step
(separate phase doc) writes the refined content back to the ticket source.
If no changes, `UPDATE_TICKET` is skipped.

## Steps

1. Read the parsed-concepts artifact.
2. For each acceptance criterion, evaluate coverage and propose edits or
   new ACs as needed.
3. Enforce Gherkin GIVEN-WHEN-THEN form on every scenario.
4. Mutate the parsed-concepts artifact in place; set `refinement_changed`
   if any change occurred.
