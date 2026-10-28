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

Step 6 is deferred until a gh-optivem release contains `gh optivem spec` (v1.6.100 does not; check with `gh optivem spec --help` after `gh extension upgrade optivem`). Then: in shop add a lenient `gh optivem spec check --mode lenient` step to `.github/workflows/monolith-java-acceptance-stage.yml` after the suite steps (the XML exists only there; the commit stage runs no system tests).

## Steps

- [ ] Step 6: wire the gate into shop's pipeline — ⏳ Deferred: needs a gh-optivem release containing `spec` (otherwise the acceptance stage breaks on `unknown command`). Design points to settle: (a) lenient check goes in the acceptance stage, not the commit stage (no system-test XML there); (b) strict at release needs the acceptance stage's JUnit XML uploaded as an artifact and downloaded in the release stage; (c) only the Java monolith carries tags and the tag-injection `doLast`, so other variants must stay out until tagged.

## Open questions

None: all resolved before execution (2026-10-28).

