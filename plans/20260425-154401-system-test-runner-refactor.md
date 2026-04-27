# Plan: Absorb Run-SystemTests orchestration into gh-optivem (PR 2 — shop cutover)

> 🤖 **Picked up by agent** — `Valentina_Desk` at `2026-04-27T07:13:03Z`

## Status

Merged to `main` and released as gh-optivem `v1.3.10` on 2026-04-25; shop cutover commit `e900fbff` on `main`. All three per-lang prerelease pipelines green since 2026-04-25T21:01Z.

## Known follow-ups (not addressed in PR 2)

### Port-clash with other scaffolded projects

The runner's `IsAnyURLUp` probe is necessarily port-based: when another local project ("page-turner" was the case during testing) is bound to shop's ports (3311/8311/9311/3312/8312/9312 for typescript), the runner skips its own restart and may run tests against the wrong stack. Tests still passed against page-turner's stub-mode endpoints because the smoke is just "did the URL respond". This isn't a new problem — same probe pattern existed in the PS1 — but worth documenting.

