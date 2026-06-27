# 2026-06-27 09:13:55 UTC — Add the ESCC section to shop #72 so it parses as external-contract work

## TL;DR

**Why:** Rehearsal #72 ("Charge shipping based on product weight from ERP") halted at `EXTERNAL_DRIVER_PORT_CHANGED_HALT` after ~10m55s / $1.91. The ticket genuinely needs an external ERP driver-port change, but it declared the contract requirement as a `### Contract test — ERP returns the weight` subsection under Acceptance Criteria instead of the canonical `## External System Contract Criteria` (ESCC) section. `parse-ticket` (intentionally dumb — only recognizes the literal heading, `internal/atdd/runtime/intake/sections.go:25`) therefore set `ticket-has-escc=false`, the external-drivers phase was skipped, and the `dsl-implementer` later touched the external ERP DTOs and tripped the (correct, by-design) gate.

**End result:** Shop issue #72's body declares a proper `## External System Contract Criteria` section naming `External System: erp`, so a fresh rehearsal parses `ticket-has-escc=true`, runs `IMPLEMENT_EXTERNAL_DRIVERS_IF_NEEDED` upfront, and exercises the full external-contract path end-to-end instead of being doomed at parse-time.

## Outcomes

What we get out of this:

- Shop issue #72 carries a canonical `## External System Contract Criteria` section (`External System: erp` + a `Shared (stub + real):` containment register for the `sku`/`price`/`weight` shape), matching the format in `internal/atdd/assets/runtime/shared/escc-format.md`.
- The informal `### Contract test — ERP returns the weight` subsection is removed/relocated so the contract requirement lives in exactly one place (no duplication).
- A re-run of the #72 rehearsal parses `ticket-has-escc=true` at `PARSE_TICKET`, runs the external-drivers phase, and advances past `write-acceptance-tests-and-dsl` without hitting `EXTERNAL_DRIVER_PORT_CHANGED_HALT`.
- #72 once again functions as the designated full-external-contract rehearsal story (all three `*-port-changed` gates reachable).
- **No gh-optivem code changes** — the BPMN gate, `parse-ticket`, and `dsl-implementer` are unchanged (the gate is a working-as-designed safety net; user scoped this fix to the ticket layer only).

## ▶ Next executable step (resume here)

Edit the body of shop GitHub issue #72 (`gh issue edit 72 --repo optivem/shop`): insert a `## External System Contract Criteria` section (drafted in Step 2 below) and delete the now-redundant `### Contract test — ERP returns the weight` subsection. This is a GitHub issue-body edit, not a code change in this repo. After editing, verify by re-running the #72 rehearsal and confirming `ticket-has-escc=true` at `PARSE_TICKET`.

## Steps

- [ ] Step 1: Confirm the exact registered external-system key and the matcher's case sensitivity. Config registers it lowercase as `erp` (`shop/gh-optivem-monolith-java.yaml:46`); the `escc-format.md` example shows `ERP`. Use the value `validate-external-systems-registered` actually compares against — default to the registered key `erp` unless the matcher is case-insensitive. (Resolves the Open question below.)
- [ ] Step 2: Draft the ESCC section text. Shape (per `escc-format.md`):
  ```
  ## External System Contract Criteria
  External System: erp
    Shared (stub + real):
      Given products SKU-123 (12.00, 1.5 kg)
      Then erp has product SKU-123 with price 12.00 and weight 1.5
  ```
  Derive the register from the existing "ERP returns the product weight" contract scenario: a `Shared (stub + real):` **containment** assertion that pins the `sku` + `price` + `weight` shape the feature depends on (not the full payload). No `Stub only:` register unless stub fidelity needs exact-set/empty (not required here — keep it minimal). Match the exact keyword vocabulary the contract writers expect (`Given products …`, `Then <System> has …`).
- [ ] Step 3: Apply the edit to issue #72 — insert the ESCC section (as its own top-level `##` section, sibling to `## Acceptance Criteria`) and delete the `### Contract test — ERP returns the weight` subsection. Use `gh issue edit 72 --repo optivem/shop --body-file <tempfile>` with the full revised body. Leave the rest of the ticket (user story, context, shipping rule, "Why this forces ERP DSL changes", file checklist, out-of-scope) intact.
- [ ] Step 4: Sanity-check the edited body renders the ESCC heading exactly as `## External System Contract Criteria` (the parser is case- and text-exact).

## Verification

- [ ] Re-run the #72 rehearsal (the same loop that produced run `20260627-082748`).
- [ ] Confirm `PARSE_TICKET` now emits `ticket-has-escc=true` and `escc-systems=[erp]` (was `escc-systems=[], ticket-has-escc=false`).
- [ ] Confirm the run enters `IMPLEMENT_EXTERNAL_DRIVERS_IF_NEEDED` (builds ERP contract tests + stubs + external driver adapters) and advances past `write-acceptance-tests-and-dsl` without `EXTERNAL_DRIVER_PORT_CHANGED_HALT`.

## Open questions

- **ESCC name casing:** registered key is `erp` (lowercase, `gh-optivem-monolith-java.yaml:46`) but `escc-format.md`'s example writes `ERP`. Resolved-by-default to `erp` (match the registered key); Step 1 confirms whether `validate-external-systems-registered` is case-sensitive before finalizing.
- **`Stub only:` register:** omitted by default (the feature only needs by-key containment of the weight shape). Add one only if a rehearsal shows stub fidelity (exact-set/empty) is needed for the weight path.
