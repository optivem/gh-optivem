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

Once this task completes, a human confirms the refined ACs before
downstream consumption.

## Inputs

- `${parsed_concepts}` — the parsed-concepts artifact produced upstream
  during ticket intake (the parsing work that backs the ticket-kind
  classification). Contains the structured ACs (legacy + newly-derived)
  ready to refine. The raw ticket source is not re-read.

## Outputs

- Mutates `${parsed_concepts}` in place — edits to existing ACs, new ACs
  for additional scenarios, Gherkin normalization throughout.
- Sets flag: `Refinement Changed: yes|no` — `yes` if any edit or addition
  occurred; `no` if the AC set was already complete and Gherkin-correct.

## Steps

1. Read `${parsed_concepts}`.
2. For each acceptance criterion, evaluate coverage against the rubric
   in Additional Notes below; propose edits to existing ACs and add new
   ACs to cover any gaps.
3. Enforce Gherkin GIVEN-WHEN-THEN form on every scenario.
4. Mutate `${parsed_concepts}` in place; set the `Refinement Changed`
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
