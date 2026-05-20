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

## Inputs

- `${parsed_concepts}` — the parsed-concepts artifact emitted by the
  upstream parse-ticket / concepts phase. Contains the structured ACs
  (legacy + newly-derived) ready to refine. The raw ticket source is not
  re-read.

## Outputs

- Mutates `${parsed_concepts}` in place — edits to existing ACs, new ACs
  for additional scenarios, Gherkin normalization throughout.
- Sets flag: `Refinement Changed: yes|no` — `yes` if any edit or addition
  occurred; `no` if the AC set was already complete and Gherkin-correct.
  The downstream `UPDATE_TICKET` step runs only when `yes`.

## Scope

This phase mutates only the parsed-concepts artifact passed via the
`${parsed_concepts}` input — no code layer is modified. The ticket
source file, production system code, and tests are out of scope.

## Steps

1. Read `${parsed_concepts}`.
2. For each acceptance criterion, evaluate coverage and propose edits or
   new ACs as needed.
3. Enforce Gherkin GIVEN-WHEN-THEN form on every scenario.
4. Mutate `${parsed_concepts}` in place; set the `Refinement Changed`
   flag if any change occurred.
