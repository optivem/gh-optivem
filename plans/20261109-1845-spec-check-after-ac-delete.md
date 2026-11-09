# 2026-11-09 18:45 local — `spec check` after criteria are deleted on green

## TL;DR

**Why:** Plan 1258 deletes a use case's acceptance criteria once they pass, so the tests become the spec. `spec check` / `spec status` still assume every criterion lives in the use case. A delivered criterion's ID is now "unknown", so the report gets noisier with every story. Worse, a failing test of a delivered criterion is no longer reported as Failing, and strict coverage (Verified ÷ criteria in the spec) is measured only over undelivered criteria, so it sits below the threshold forever.
**End result:** `spec check` treats a test ID `UC-NNN.M` whose use case `UC-NNN` exists as a **delivered** criterion: Verified when its tests pass, Failing when any fails (Failing blocks, as today). Unknown IDs mean only a typo or a deleted use case again. Strict coverage counts delivered criteria, so the threshold means something.

Parent: `plans/20261109-1219-improved-atdd-process-flow.md` (Q4). Follows `plans/20261109-1258-ac-delete-and-generate.md`, which kept `spec check`/`status` because shop's `spec.yaml` (`results`, `threshold`) must still parse.

## Outcomes

- A new criterion source in the report: **delivered** = an ID found on tests, not in the use case text, whose `UC-NNN` prefix names an existing use case. It appears in the report under its use case, flagged "delivered", with no criterion text (the text, if wanted, comes from `gh optivem spec generate`).
- Delivered criteria get Verified / Failing from their tests like any other criterion; Failing blocks in both modes.
- `UnknownID` is reported only for IDs whose use case does not exist (typo, deleted use case). Same for `Orphan` "no known criterion ID".
- Strict coverage = Verified ÷ (in-scope spec criteria + delivered criteria).
- Golden report files, `docs/spec-gate/convention.md` and `docs/cli-reference.md` describe the delivered state.

## ▶ Next executable step (resume here)

User to OK the decisions below. Then Step 1: in `internal/spec/report.go` `BuildReport` / `linkTests`, add delivered criteria for test IDs whose `UC-NNN` is a known use case, and stop listing them as unknown.

## Steps

- [ ] Step 1: `internal/spec/report.go`: collect test IDs not in the spec; for each whose `UC-NNN` prefix names a parsed use case, add a delivered criterion to that use case (`Delivered: true`, no text) and link its tests. Keep `UnknownID` / `Orphan` for the rest.
- [ ] Step 2: `internal/spec/gate.go`: delivered criteria take part in `gateCriteria` (Failing blocks; strict counts them in numerator and denominator). Waivers/flags apply by ID as they do for spec criteria.
- [ ] Step 3: Renderers (`render.go` text/HTML/markdown): show delivered criteria under their use case, marked "delivered", sorted by ID with the rest. Update golden files.
- [ ] Step 4: Docs: `docs/spec-gate/convention.md` (lifecycle: a criterion is delivered once deleted on green), `docs/cli-reference.md`, and the Green row in `docs/bpmn-process-design.md` if it mentions `spec check`.
- [ ] Step 5: Tests: unit tests for delivered / unknown / failing-delivered / strict coverage; run `go test -p 2 -timeout 300s ./internal/spec/... .`.

## Decisions (proposed 2026-11-09, pending user OK — override any line)

- **Keep the coverage gate, make it delivery-aware** rather than retiring it: it is the only report that ties tests to use cases for a PO, and the fix is local to `internal/spec`.
- **"Delivered" is inferred from test tags plus use-case existence**, not from a recorded list: IDs are never renumbered or reused (plan 1256), so a `UC-NNN.M` tag on a test of an existing use case can only be a delivered criterion. No new state file (fingerprints were just removed).
- **No criterion text in the report for delivered criteria.** Pulling it from `spec generate` would couple the gate to the DSL parser; the report links to the generate command instead.

## Verification

- Operator, once shop is free: run `gh optivem spec check --config ../shop/spec.yaml` after a story's criteria were deleted on green, and confirm its tests show as delivered/Verified, not unknown; break one of them and confirm the check fails.
