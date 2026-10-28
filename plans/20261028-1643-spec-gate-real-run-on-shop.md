# 2026-10-28 16:43:19 UTC — Spec gate: real-run validation on shop

## TL;DR

**Why:** The spec-coverage gate (`gh optivem spec`) was only trialled on a copy of shop with synthetic JUnit XML, so it is unproven that stock Gradle writes JUnit 5 `@Tag` values as `<property>` entries, which the whole tag-to-criterion link relies on.
**End result:** The gate has run on real Gradle test output from shop's Place Order tests, the tag mechanism is confirmed (or fixed and documented), and the lenient and strict results are recorded; optionally shop's pipeline runs the gate by stage.

## Outcomes

- Known, from real output, whether Gradle JUnit XML carries `@Tag` IDs; if not, a working fix (for example a `TestExecutionListener`) exists and `docs/spec-gate/convention.md` matches reality.
- Place Order's top-layer tests in shop carry `@Tag("UC-001.n")`, developed on a short-lived branch and merged to shop `main` once the real run is green (13 of 14 Verified, UC-001.14 Planned).
- `gh optivem spec check` results on real XML are recorded for lenient and strict modes (expected: 13 of 14 criteria Verified, UC-001.14 Planned).
- Any bugs or friction found are fixed or logged in `BACKLOG.md`; the BACKLOG TODO "prove tags-as-properties" is closed.
- Optionally, shop's pipeline runs the gate: lenient at the commit and acceptance stages, strict at release.

## ▶ Next executable step (resume here)

Step 1: in `../shop` (resolve dynamically), create a short-lived branch, add `@Tag("UC-001.n")` to the Place Order top-layer Java tests using the mapping in `docs/spec-gate/trial/mapping.md`, and check whether Docker and Keycloak work locally (if not, Step 2 uses a CI run on the branch). Gate: the tagged tests compile. Unblocks Step 2 (the real Gradle run).

## Steps

- [ ] Step 1: In shop, create a short-lived branch and add `@Tag("UC-001.n")` to the Place Order top-layer tests (mapping in `docs/spec-gate/trial/mapping.md`); check whether Docker and Keycloak work locally (decided: check locally first; if not, fall back to a CI run on the shop branch for Step 2). Do all work on the branch; nothing lands on shop `main` until the real run is green (see Step 5).
- [ ] Step 2: Run the real Gradle tests for those tests and inspect the produced JUnit XML: are the tags present as `<property>` entries, and how do `@TestTemplate` channel results appear?
- [ ] Step 3: If tags are absent, add a fix (a `TestExecutionListener` or report configuration, in shop or as a documented snippet) and re-run; update the tag/XML section of `docs/spec-gate/convention.md` to match what was observed.
- [ ] Step 4: Add `spec.yaml` and the Place Order use case markdown to shop on the same branch; run `gh optivem spec check` in lenient and strict modes on the real XML; record results and friction in `docs/spec-gate/trial/gh-optivem-run.md`; fix gate bugs found, with tests.
- [ ] Step 5: Close the BACKLOG TODO "prove tags-as-properties on a real run"; log any new findings there. If the real run is green (13 of 14 Verified, UC-001.14 Planned), merge the shop branch to shop `main` (decided: merge when green); if not green, leave it on the branch and report.
- [ ] Step 6 (optional; decided: ask the user after Step 4 whether to do it, based on the findings): wire the gate into shop's pipeline: lenient at commit and acceptance stages, strict at release.

## Open questions

None: all resolved before execution (2026-10-28).

