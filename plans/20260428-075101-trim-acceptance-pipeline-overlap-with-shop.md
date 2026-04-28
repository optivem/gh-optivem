# Plan — Trim gh-optivem acceptance pipeline given shop's cross-lang coverage

**Date:** 2026-04-28
**Status:** proposed
**Owner:** unassigned
**Trigger:** [shop cross-lang verification workflow](../../shop/.github/workflows/cross-lang-system-verification.yml) shipped (Phase 2 complete) and is now invoked from `meta-prerelease-stage.yml` in parallel with `run`, gating `tag-meta-rc`. That workflow tests cross-language behavior parity directly on shop's images. `_gh-acceptance-pipeline.yml`'s 36-combo full matrix duplicates that signal at higher cost.

## Goal

Reduce gh-optivem's acceptance pipeline wall time and runner usage by removing coverage that shop's cross-lang verification now provides cheaper, while preserving the unique-to-gh-optivem signal: **does scaffolding from templates produce a working project?**

## Problem

`_gh-acceptance-pipeline.yml` matrix sizes (current `acceptance-mode` values):
- `single` — 1 combo
- `minimal` — 8 combos (Latin-square, lang × test-lang sampled)
- `condensed` — partial
- `full` — **36 combos** = 2 arch × 2 repo-strategy × 3 lang × 3 test-lang

Each combo: scaffolds a project (~2-3 min), starts SUT, runs system tests (~5-10 min). Full mode = ~6-10 hours of cumulative runner time.

Of those 36 combos, the `lang ≠ test-lang` rows (the cross-language ones — 24 of 36) are now **redundant** with shop's cross-lang. If a backend behavior diverges between languages, shop's cross-lang catches it directly on shop's own images. By the time gh-optivem's matrix runs, the regression is either already known (shop's daily cron) or already gating meta-rc tagging (shop's meta-prerelease).

## What gh-optivem still uniquely covers

- **Scaffolding correctness:** template substitution, README rendering, file layout, generated config, CI wiring of the scaffolded repo. This is gh-optivem's core value-add and only its own pipeline can verify it.
- **Same-lang round-trip:** scaffold + run the scaffolded project's own test suite (`lang == test-lang`). This proves "a freshly-scaffolded project works end-to-end" — the minimum claim gh-optivem must defend.

These 12 combos (2 arch × 2 repo-strategy × 3 lang, with `test-lang == lang`) cannot be moved to shop. They stay.

## Proposed changes

- [ ] **Drop the cross-language axis from the `full` matrix.** New `full` = 12 combos (`2 arch × 2 repo-strategy × 3 lang`, `test-lang == lang`). Rationale: shop's cross-lang now owns the cross-language signal.
- [ ] **Default `acceptance-mode` to `minimal` for non-release runs.** Release runs (`verify-level: release`, `is-release-run: true`) keep `full`. Other runs trade coverage for latency.
- [ ] **Keep `single` and `minimal`** as escape hatches for fast feedback during development.
- [ ] **Update gh-optivem's CLAUDE.md / README** to document the split: gh-optivem owns scaffolding correctness, shop owns cross-language behavior parity.

## Out of scope

- Smoke matrix (`smoke-mode: full|single|none`) — already minimal coverage; not duplicating shop's signal.
- Removing `test-lang` from the matrix axis entirely — keep it so `single`/`minimal` modes can still target specific cross-lang combos when needed for debugging gh-optivem itself (rare but useful).

## Risks

- **Scaffolded-cross-lang divergence undetected.** If gh-optivem's templates introduce a cross-lang bug that shop doesn't have (e.g. broken template substitution that produces incorrect ports for cross-lang config), the trimmed gh-optivem matrix won't catch it. Mitigation: shop's templates and shop are kept in sync — divergence is a flagged abnormality. Periodic full runs (e.g. weekly heartbeat) catch slow drift.
- **Coordination across repos.** This plan presumes shop's cross-lang stays green and reliable. If shop's cross-lang becomes flaky, gh-optivem loses its safety net. Mitigation: shop's cross-lang has its own daily cron + meta-prerelease gate; flakiness should surface there first.

## Verification

- [ ] **Measure baseline:** record current full-matrix wall time (last 3 release runs).
- [ ] **Apply the trim and re-measure:** target 12 combos × ~10 min = ~2h cumulative (vs. ~6-10h today). `acceptance-mode: minimal` runs should drop from ~80 min to ~20 min on average.
- [ ] **Audit one quarter post-change:** check whether any cross-lang regression slipped through — if shop's cross-lang missed something gh-optivem would have caught, revisit the trim.

## Pointers

- Full matrix definition: [_gh-acceptance-pipeline.yml](../.github/workflows/_gh-acceptance-pipeline.yml) (search for `full = all 36 combos`).
- Shop's cross-lang reference: [shop/.github/workflows/cross-lang-system-verification.yml](../../shop/.github/workflows/cross-lang-system-verification.yml).
- Shop's meta-prerelease integration: [shop/.github/workflows/meta-prerelease-stage.yml](../../shop/.github/workflows/meta-prerelease-stage.yml) — see `cross-lang` and `tag-meta-rc` jobs.
