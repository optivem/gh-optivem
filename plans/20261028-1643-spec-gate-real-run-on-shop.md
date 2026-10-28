# 2026-10-28 16:43:19 UTC — Spec gate: real-run validation on shop
> 🤖 **Picked up by agent** — ValentinaLaptop at 2026-10-28T16:56:17Z

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

Step 6 (optional): ask the user whether to wire the gate into shop's pipeline, given the findings (the gate needs the tag-injection `doLast` step, already on shop `main`, and `gh optivem spec check` per stage). If no, delete this plan.

## Steps

- [ ] Step 6 (optional; decided: ask the user after Step 4 whether to do it, based on the findings): wire the gate into shop's pipeline: lenient at commit and acceptance stages, strict at release.

## Open questions

None: all resolved before execution (2026-10-28).

