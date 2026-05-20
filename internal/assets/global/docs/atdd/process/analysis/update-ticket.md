# UPDATE TICKET (DRAFT)

Write the refined acceptance criteria back to the ticket source.
Mechanical overwrite — no judgment, no creative authoring.

## Role in the flow

Runs after the refine-acc cycle when the user has confirmed the refined
ACs **and** the `Refinement Changed` flag is `yes`. Skipped (no-op
discharge) when refinement produced no changes.

Once this step discharges, the execution cycles (AT behavioral /
structural) proceed.

## Inputs

- `${parsed_concepts}` — the refined parsed-concepts artifact produced
  by `refine-acc`. Read-only here.
- `${ticket_source}` — the ticket source file whose three named sections
  are about to be overwritten.

## Outputs

- Overwrites three sections in `${ticket_source}`:
  - `Description`
  - `Legacy Acceptance Criteria`
  - `Acceptance Criteria`
- All other sections in the ticket source are left byte-for-byte
  unchanged.

## Steps

1. Read the refined parsed-concepts artifact at `${parsed_concepts}`.
2. Locate the three named sections in `${ticket_source}` by their H2
   headers.
3. Overwrite each section's body with the corresponding content derived
   from the parsed-concepts artifact.
4. Leave every other section in the ticket source unchanged.
