# 2026-11-07 13:00 UTC — gh-optivem `spec` reads `@UC-NNN.M`-tagged Gherkin scenarios as criteria

> Moved from shop `plans/move-to-gh-optivem/` on 2026-11-09 (the shop-side copy still needs deleting from a shop session; this session didn't touch shop because the user's shop session is busy). The shop follow-up is Track D of shop `plans/20261106-1533-use-cases-match-substack-articles.md`; it waits for this plan's release.

## TL;DR

**Why:** Shop's use-case docs (`docs/shared/specs/use-cases/UC-001-place-order/acceptance-criteria.md`) now write acceptance criteria as Gherkin scenarios tagged `@UC-001.N`, to match the Substack articles. gh-optivem's parser (`internal/spec/usecase.go`, `criterionPattern`) reads only bullet lines (`- UC-001.N: text`), so it finds zero criteria and `gh optivem spec validate` passes silently with `OK`, exit 0. Shop has turned its `spec-gate` step off until this is fixed.
Shop is also moving to a folder per use case (see below), which `LoadDir` can't see today: it reads only `.md` files directly inside `specDir`.
**End result:** A published gh-optivem release reads tagged Gherkin scenarios (and bullets) as criteria, in both the flat layout and the folder-per-use-case layout. It warns when a use-case file has no criteria, and finds all 14 criteria in shop's UC-001.

## ▶ Next executable step (resume here)

Only Step 5 (release) is left. Steps 1–4 were done on 2026-11-09. The release is cut by gh-optivem's acceptance pipeline after a push to `main`; wait for the user's go.

Checked against the live shop checkout (2026-11-09, read-only): `gh optivem spec validate` with the new parser prints `OK: 12 use case(s) valid, 23 criteria gated (10 planned use case(s) not gated)`. That's UC-001 with 14 criteria plus UC-004 with 9; the other 10 use cases are `status: planned`.

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

- [ ] Step 5 — **Release** a new gh-optivem version, and confirm `gh optivem spec validate` in a shop checkout reports 14 criteria. Use whichever layout shop has by then: flat `docs/use-cases/` or `docs/shared/specs/use-cases/UC-001-place-order/acceptance-criteria.md`.

## Hand-off back to shop

What the new parser does (2026-11-09):
- **Criteria from Gherkin:** inside a fenced `gherkin` block, each distinct `@UC-NNN.M` tag is one criterion. Several scenarios may share a tag (UC-001.1 and UC-001.8 do); their titles are joined with `; ` into the criterion text, so editing any of them flags the criterion as Changed. The group is the enclosing `Rule:` title, or the markdown heading above the block. Bullets still work. Scenarios outside a `gherkin` fence are ignored.
- **Errors (fail `spec validate` and the gate):** a scenario with two `@UC-…` tags (`criteria`), the same ID as both a bullet and a scenario or in two files (`duplicate-id`), an implemented use case with no criteria (`no-criteria`, the old silent pass), a front-matter `status` other than `implemented`/`planned`.
- **Warnings (printed, don't fail):** a scenario in a `gherkin` block with no `@UC-…` tag; a `UC-NNN-*` folder whose front-matter `id` differs.
- **Layout:** `LoadDir` reads top-level `*.md` except `README.md`, plus `<specDir>/*/acceptance-criteria.md`. Other files in use-case folders are skipped.
- **Planned use cases:** `status: planned` use cases are validated and listed ("UC-006 … planned, N criteria, not gated", JSON `plannedUseCases`) but left out of the totals, coverage and threshold. A test tagged with a planned criterion is not reported as an unknown ID.
- **Pooled results (Step 4 finding):** results from all `results:` globs are pooled. Tests merge by `class#method` (languages have different class names, so they don't merge), and a criterion's state comes from all tests linked to its ID: any failing test anywhere → Failing; Verified needs every linked test to pass. **A language with no test for a criterion doesn't lower its state**, so a Java-only test makes the criterion Verified even if .NET and TypeScript have none. For Track D2, if per-language coverage matters, run `spec status` once per language (a `spec.yaml` per language, or a per-language `results:` override) rather than listing all three globs in one file; gh-optivem has no per-glob report today.


Once released, run Track D of `plans/20261106-1533-use-cases-match-substack-articles.md` in shop. Then delete this file from shop's `plans/move-to-gh-optivem/`.
