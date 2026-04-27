# Plan: Absorb Run-SystemTests orchestration into gh-optivem (PR 2 — shop cutover)

> 🤖 **Picked up by agent** — `Valentina_Desk` at `2026-04-27T07:13:03Z`

## Status

Merged to `main` and released as gh-optivem `v1.3.10` on 2026-04-25; shop cutover commit `e900fbff` on `main`. All three per-lang prerelease pipelines green since 2026-04-25T21:01Z.

## Items still open

### Verification still owed

1. ⏳ **Diff the suite results table** before/after on all 3 langs, to confirm no behavior drift.

## Known follow-ups (not addressed in PR 2)

### Port-clash with other scaffolded projects

The runner's `IsAnyURLUp` probe is necessarily port-based: when another local project ("page-turner" was the case during testing) is bound to shop's ports (3311/8311/9311/3312/8312/9312 for typescript), the runner skips its own restart and may run tests against the wrong stack. Tests still passed against page-turner's stub-mode endpoints because the smoke is just "did the URL respond". This isn't a new problem — same probe pattern existed in the PS1 — but worth documenting.

## Open question — relocate `system.json` + compose files out of `system-test/`?

VJ raised: should the `system.json` config and the `docker-compose.local.*.yml` files live under `shop/docker/` (or similar) rather than `shop/system-test/<lang>/<arch>/`? The reasoning: a docker-compose stack describes the SUT, not the test runner — `system-test/` should hold tests, not infrastructure.

Trade-offs to decide later:
- **Pro relocate:** clearer separation. `system-test/` becomes purely test code + `tests-*.json`. Reduces duplication if all 3 langs share one set of compose files (today they're per-lang because of build contexts and ports differing — but that's solvable with port files / compose `extends`).
- **Pro stay:** the runner resolves compose paths relative to `system.json`'s directory. Moving means cross-tree references. Each lang currently has its own ports (3111/3211/3311), so per-lang compose files are not strictly redundant.
- **Affects scaffolder:** `templates.SelectDockerCompose` flattens `system-test/<lang>/<arch>/` into `system-test/`. If the source lives elsewhere, `apply_template.go`'s copy logic changes.

Not addressed in this PR. File a follow-up issue if pursuing.
