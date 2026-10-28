# 2026-10-28 16:43:19 UTC — Spec gate: real-run validation on shop

## TL;DR

**Why:** The spec-coverage gate (`gh optivem spec`) was only trialled on a copy of shop with synthetic JUnit XML, so it is unproven that stock Gradle writes JUnit 5 `@Tag` values as `<property>` entries, which the whole tag-to-criterion link relies on.
**End result:** The gate has run on real Gradle test output from shop's Place Order tests, the tag mechanism is confirmed (or fixed and documented), and the lenient and strict results are recorded; optionally shop's pipeline runs the gate by stage.

## Outcomes

- Known, from real output, whether Gradle JUnit XML carries `@Tag` IDs; if not, a working fix (for example a `TestExecutionListener`) exists and `docs/spec-gate/convention.md` matches reality.
- Place Order's top-layer tests in shop carry `@Tag("UC-001.n")`, delivered on a short-lived branch (never `main`).
- `gh optivem spec check` results on real XML are recorded for lenient and strict modes (expected: 13 of 14 criteria Verified, UC-001.14 Planned).
- Any bugs or friction found are fixed or logged in `BACKLOG.md`; the BACKLOG TODO "prove tags-as-properties" is closed.
- Optionally, shop's pipeline runs the gate: lenient at the commit and acceptance stages, strict at release.

## ▶ Next executable step (resume here)

Step 1: in `../shop` (resolve dynamically), create a short-lived branch, add `@Tag("UC-001.n")` to the Place Order top-layer Java tests using the mapping in `docs/spec-gate/trial/mapping.md`, and confirm Docker and Keycloak are available for the system tests. Gate: the tagged tests compile. Unblocks Step 2 (the real Gradle run).

## Steps

- [ ] Step 1: In shop, create a short-lived branch and add `@Tag("UC-001.n")` to the Place Order top-layer tests (mapping in `docs/spec-gate/trial/mapping.md`); confirm Docker and Keycloak are available. Never touch shop `main`.
- [ ] Step 2: Run the real Gradle tests for those tests and inspect the produced JUnit XML: are the tags present as `<property>` entries, and how do `@TestTemplate` channel results appear?
- [ ] Step 3: If tags are absent, add a fix (a `TestExecutionListener` or report configuration, in shop or as a documented snippet) and re-run; update the tag/XML section of `docs/spec-gate/convention.md` to match what was observed.
- [ ] Step 4: Add `spec.yaml` and the Place Order use case markdown to shop on the same branch; run `gh optivem spec check` in lenient and strict modes on the real XML; record results and friction in `docs/spec-gate/trial/gh-optivem-run.md`; fix gate bugs found, with tests.
- [ ] Step 5: Close the BACKLOG TODO "prove tags-as-properties on a real run"; log any new findings there.
- [ ] Step 6 (optional, see open questions): wire the gate into shop's pipeline: lenient at commit and acceptance stages, strict at release; merge the shop branch only after the user approves.

## Open questions

- Is wiring the gate into shop's pipeline (Step 6) in scope now, or only after Steps 1–5 succeed? Recommended: decide after Step 4, because pipeline changes are visible to everyone and depend on the findings.
- Should the shop changes be merged to `main` or kept as a branch only? Recommended: keep as a branch until the results are reviewed, then merge on the user's approval.
- Does Docker/Keycloak run on this machine? If not, Step 2 may need CI (a workflow run on the branch) instead; inferred, not confirmed.
