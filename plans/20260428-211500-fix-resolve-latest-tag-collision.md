# Plan — Fix resolve-latest-tag-from-sha collision with promotion-state lookup

**Date:** 2026-04-28
**Status:** proposed
**Owner:** unassigned
**Trigger:** `resolve-latest-tag-from-sha` returns the release tag (`meta-v1.0.30`) instead of the originating RC tag (`meta-v1.0.30-rc.49`) once a release has been promoted, because both tags point at the same shop commit and `sort -V` ranks the release tag higher.

## Goal

Make the "what RC was this release promoted from?" lookup unambiguous, without breaking the existing identity-tag query at the acceptance-pipeline call site.

## Problem

Two callers of `optivem/actions/resolve-latest-tag-from-sha`:

1. [`_gh-acceptance-pipeline.yml:528`](../.github/workflows/_gh-acceptance-pipeline.yml) — pattern `'*'`, asks *"give me a meaningful tag for this shop SHA"*. **Identity question — fine.**
2. [`gh-release-stage.yml:115`](../.github/workflows/gh-release-stage.yml) — pattern `'meta-v*'`, asks *"give me the shop tag this release was promoted from"*. **State question — broken.**

After promotion, the same shop commit carries both `meta-v1.0.30-rc.49` and `meta-v1.0.30`. `sort -V | tail -1` picks `meta-v1.0.30`. The caller wanted the RC.

## Root cause

Git tags are doing two jobs:
- **Identity** — "this commit *is* meta-v1.0.30".
- **Promotion state** — "this commit *was promoted to* release".

When state is queried via tags-by-SHA, the two jobs collide. Tactical patches (stricter glob, exclude `-rc.*`, prefer-RC flag) keep the overload and just paper over one symptom.

## Decision: separate identity from state

Tags keep being tags (identity). Promotion state moves to a primitive built for state.

**Where state lives — by question:**

| Question | Primitive |
|---|---|
| Did this commit pass acceptance? | commit status `acceptance-stage` on shop SHA *(already in place)* |
| Was this commit released, and from which RC? | **new** commit status `released-from-rc` on shop SHA, description = RC tag |
| What's currently deployed? | floating Docker tag (out of scope) |
| Where did this image come from? | OCI annotations on the manifest (out of scope) |

Commit status — not image tag — because the lookup happens *during the release decision*, before the deploy primitive is meaningful. Image tags answer deploy-currency questions for downstream consumers; the release pipeline asking "what RC am I promoting?" is a gating-decision question, which is the commit-status job. (See conversation 2026-04-28.)

## Proposed changes

- [ ] **Add a `released-from-rc` commit status** at release time. Written by `gh-release-stage.yml` `run` job after the release tag is published, against the shop SHA, with `description` = the RC shop tag (e.g. `meta-v1.0.30-rc.49`). Use existing `optivem/actions/create-commit-status@v1`.
- [ ] **Replace the `resolve-latest-tag-from-sha` call at `gh-release-stage.yml:115`** with a `get-commit-status` lookup against the new `released-from-rc` context. Falls back to the current behaviour only on the *first* release pass (no status yet) — see Migration below.
- [ ] **Leave `_gh-acceptance-pipeline.yml:528` untouched.** Its question is genuinely identity-shaped and pattern `'*'` is correct. The collision bug doesn't affect it because at that point in the pipeline only the RC tag exists on the shop SHA.
- [ ] **Document the split in `gh-optivem/NAMING.md`** (or a new `STATE-VS-IDENTITY.md`): tags = identity, commit statuses = gating/promotion state, image tags = deploy currency.

## Migration

The new `released-from-rc` status doesn't exist on historical shop SHAs. Two options:

- **Lazy:** if `get-commit-status released-from-rc` returns empty, fall through to `resolve-latest-tag-from-sha` with pattern `'meta-v*-rc.*'` (note: `-rc.*` glob, not `*` — this *also* fixes the collision for legacy SHAs by excluding the release tag from the candidate set). Going forward, the status is always present.
- **Eager backfill:** one-shot script writing `released-from-rc` statuses for the last N release-stage runs by reading their workflow logs. Higher effort, cleaner state.

Recommend **lazy** — the legacy fallback is one extra `if` branch and decays naturally as new releases populate the status.

## Pragmatic compromise (D-lite, if the above is too big)

If we don't want to add a new commit-status context now: change just the glob at `gh-release-stage.yml:119` from `'meta-v*'` to `'meta-v*-rc.*'`. This excludes the release tag from the candidate set and resolves the collision today. **Cost:** keeps the tags-as-state overload; the next state question we ask will hit the same class of bug. **Benefit:** one-line fix.

Reach for D-lite only if the structural fix is blocked. Don't ship both — they're alternatives.

## Out of scope

- Floating Docker tags (`:production`, `:rc`) — separate concern, deploy-side, not blocking this fix.
- OCI annotations on release images for provenance — complementary, can land later.
- Migrating `_gh-acceptance-pipeline.yml`'s call site — its question is identity-shaped and works correctly today.
- Changing `optivem/actions/resolve-latest-tag-from-sha` itself — the action is correct for identity lookups; the bug is in *how it's used* for state lookups.

## Risks

- **`released-from-rc` status missing on a release SHA.** Lazy fallback covers historical SHAs. For new SHAs, the status write must succeed before the lookup runs — order it inside the `run` job, fail the job if the status write fails.
- **Multiple releases on the same shop SHA.** Theoretically possible if two release-stage runs target SHAs that collapse to the same shop commit. Commit status overwrites on `(sha, context)`, so the most recent wins — same semantics as the current tag-based lookup. No regression.
- **Confusion between commit-status contexts.** `acceptance-stage`, `release-stage`, `released-from-rc` start to proliferate. Mitigation: document them in one place (`STATE-VS-IDENTITY.md`).

## Verification

- [ ] **Reproduce the bug:** run gh-release-stage against an already-released RC version; confirm `shop-tag` resolves to the release tag instead of the RC tag.
- [ ] **Apply fix:** add `released-from-rc` write + lookup, swap the call site.
- [ ] **Re-run release-stage** and confirm `shop-tag` now resolves to the RC tag, including for a SHA that already has a release tag attached.
- [ ] **Test legacy fallback:** dispatch release-stage against an old release SHA that has no `released-from-rc` status; confirm the lazy fallback resolves correctly via the stricter glob.
- [ ] **Audit one quarter post-change:** confirm no caller reintroduces tags-by-SHA queries for state-shaped questions.

## Pointers

- Action: [`optivem/actions/resolve-latest-tag-from-sha/action.yml`](../../actions/resolve-latest-tag-from-sha/action.yml)
- Buggy call site: [`gh-release-stage.yml:115`](../.github/workflows/gh-release-stage.yml)
- Healthy call site (do not change): [`_gh-acceptance-pipeline.yml:528`](../.github/workflows/_gh-acceptance-pipeline.yml)
- Existing commit-status writer: [`optivem/actions/create-commit-status`](../../actions/create-commit-status)
- Existing commit-status reader: [`optivem/actions/get-commit-status`](../../actions/get-commit-status) — already used for `acceptance-stage` at `gh-release-stage.yml:101`.
