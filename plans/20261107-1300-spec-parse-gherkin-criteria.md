# 2026-11-07 13:00 UTC — gh-optivem `spec` reads `@UC-NNN.M`-tagged Gherkin scenarios as criteria

> ⚠️ **gh-optivem work.** Moved here from shop's `plans/move-to-gh-optivem/` on 2026-11-11. Shop never edits gh-optivem. The shop follow-up is Track D of `plans/20261106-1533-use-cases-match-substack-articles.md`; it waits for this plan's release.

## TL;DR

**Why:** Shop's use-case docs (`docs/shared/specs/use-cases/UC-001-place-order/acceptance-criteria.md`) now write acceptance criteria as Gherkin scenarios tagged `@UC-001.N`, to match the Substack articles. gh-optivem's parser (`internal/spec/usecase.go`, `criterionPattern`) reads only bullet lines (`- UC-001.N: text`), so it finds zero criteria and `gh optivem spec validate` passes silently with `OK`, exit 0. Shop has turned its `spec-gate` step off until this is fixed.
Shop is also moving to a folder per use case (see below), which `LoadDir` can't see today: it reads only `.md` files directly inside `specDir`.
**End result:** A published gh-optivem release reads tagged Gherkin scenarios (and bullets) as criteria, in both the flat layout and the folder-per-use-case layout. It warns when a use-case file has no criteria, and finds all 14 criteria in shop's UC-001.

## ▶ Next executable step (resume here)

Step 5 (C2 in shop meta-plan `20261111-0733-meta-storefront-reliability-cutover.md`): release. Steps 1, 2, 2b and 2c (C1, C1b, C1c) shipped in `7399d6f2`, verified 2026-11-11 (`go test ./...` green; local build in a shop checkout: `OK: 12 use case(s) valid, 35 criteria gated (9 planned use case(s) not gated)`, UC-001 now has 16 criteria). `7399d6f2` is already in tag `v1.6.108`, and the installed `gh optivem` v1.6.108 prints the same result, so Step 5 may only need confirming. Steps 3 and 4 are still open (see notes on each).

## What shop has now (input format)

Criteria live in fenced ```` ```gherkin ```` blocks in `docs/shared/specs/use-cases/UC-001-place-order/acceptance-criteria.md`. Each `Scenario:` carries an `@UC-001.N` tag, and extension scenarios also carry a narrative tag (e.g. `@narrative-4a`). There are 14 criteria, UC-001.1 to UC-001.14. Copy the shop file as a test fixture.

**Layout change (decided 2026-11-07, shop plan `plans/20261107-1638-storefront-use-cases-rules-acceptance.md`):** shop is moving its specs to one folder per use case, with `spec.yaml` `specDir: docs/shared/specs/use-cases`:

```
docs/shared/specs/use-cases/
  README.md                     ← index (diagram + table), NO front matter
  UC-001-place-order/
    use-case-narrative.md                 ← narrative, NO front matter
    business-rules.md           ← optional, NO front matter
    acceptance-criteria.md      ← front matter (id, title) + the tagged Gherkin blocks
  UC-002-view-order/
    ...
```

Only `acceptance-criteria.md` is a spec file for gh-optivem. The other files are for people.

Tests already report criteria in the shape `internal/spec/junit.go` reads: per-testcase `<property name="tag" value="UC-001.N"/>`, in Java, .NET and TypeScript JUnit XML. No change is needed there.

## Steps

- [ ] Step 3 — **No silent pass:** make `spec validate` warn (or fail) when a use-case file has zero criteria. That silent pass is what hid this problem. *(Note 2026-11-11: `7399d6f2` added this, but `d751983d` deliberately made a use case with no criteria valid, because criteria are deleted once their tests are green. Decide whether this step is obsolete.)*
- [ ] Step 4 — **Check pooled results:** shop's Track D will list Java, .NET and TypeScript result globs in `spec.yaml` `results:`. Check how `spec status` merges results across files. If it pools them, one language's passing test hides another language's gap. If so, consider a per-language report (e.g. per-glob coverage) and record the behaviour in this plan's hand-off note so shop can decide.
- [ ] Step 5 — **Release** a new gh-optivem version, and confirm `gh optivem spec validate` in a shop checkout reports 14 criteria. Use whichever layout shop has by then: flat `docs/use-cases/` or `docs/shared/specs/use-cases/UC-001-place-order/acceptance-criteria.md`.

## Hand-off back to shop

Once released, run Track D of `plans/20261106-1533-use-cases-match-substack-articles.md` in shop. Then delete this file from shop's `plans/move-to-gh-optivem/`.
