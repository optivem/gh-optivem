# 2026-11-03 13:35 UTC — Support `inner-loop` and `paths.component-test` on `TierSpec`

> Moved here from shop's `plans/move-to-gh-optivem/20261103-1335-readd-inner-loop-pilot-config.md` on 2026-11-12. The shop-side follow-up (re-adding the keys to shop's config) stays in shop as `plans/20261103-1335-readd-inner-loop-pilot-config.md` and waits for this plan's release.

## TL;DR

**Why:** shop's `cf2869ad` ("backend-java: PIT mutation suite and test-after inner-loop pilot config") added `inner-loop: {mode: test-after}` and `paths.component-test` to the backend tier of `gh-optivem-multitier-java.yaml`. The published gh-optivem (v1.6.105, 2026-10-31) rejects the key with `field inner-loop not found in type projectconfig.TierSpec`. That failed every shop prerelease pipeline at `check` (e.g. `meta-prerelease-stage` run 41637810423), so shop `3f658c5a` removed the two keys again. As of v1.6.108 (2026-11-12), `internal/projectconfig` still has no `inner-loop` field.
**End result:** a published gh-optivem release accepts `inner-loop` (`mode: test-after`, `mutation-threshold`) and `paths.component-test` on a tier, so shop can put the keys back.

## ▶ Next executable step (resume here)

Step 1. Add the fields to `TierSpec` in `internal/projectconfig`, wire `paths.component-test` into the path defaults and validation, and accept `inner-loop.mode` / `inner-loop.mutation-threshold`. If another gh-optivem plan already covers this work, follow that plan instead and delete this one.

## Steps

- [ ] Step 1: Add `inner-loop` (`mode: test-after`, `mutation-threshold: <int>`) and `paths.component-test` to `TierSpec`, with validation and unit tests. Use shop's `cf2869ad` shape as the fixture:
  ```yaml
          inner-loop:
              mode: test-after
              mutation-threshold: 50
          paths:
              component-test: system/multitier/backend-java/src/componentTest/java/com/mycompany/myshop/backend
  ```
- [ ] Step 2: Publish a release. Check that `optivem/actions/install-gh-optivem@v1` picks it up (latest vs a pinned version), then tell shop's follow-up plan the version.

## Notes

- **Mutation threshold 50 (set 2026-11-05, shop).** First full baseline from `multitier-backend-java-mutation-nightly` (run 41993634354): 307 mutants, 164 killed = **53%**. The threshold sits just below the baseline, so the changed-class gate starts green.
