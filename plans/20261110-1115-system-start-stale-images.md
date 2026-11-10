# 2026-11-10 11:15 — `system start` silently reuses stale Docker images

## TL;DR

**Why:** Plain `gh optivem system start` runs `docker compose up -d` without `--build`, so it starts whatever images are already there, however old. In the shop's storefront Step 4 (2026-11-10), 5-week-old `my-shop-real-frontend` / `my-shop-stub-frontend` images caused false multitier test failures. They only went away after a manual `docker rmi -f`.
**End result:** Plain `system start` never runs an image older than its source without saying so. It either rebuilds (Docker's layer cache makes an unchanged rebuild cheap) or at least warns loudly and names the fix. `--restart` and `system build --rebuild` stay as they are.

## ▶ Next executable step (resume here)

Design/decision only: run `/refine-plan plans/20261110-1115-system-start-stale-images.md` to resolve the open questions, then execute the implementation items.

## Current behaviour (verified 2026-11-10)

| Command | Behaviour | Code |
|---|---|---|
| `system start` | If any URL is healthy, skip ("System is already running, skipping restart"). Otherwise `down`, then `up -d` with **no `--build`**, so existing images are reused. | `internal/build/runner/system.go` `Up` / `upComposeArgs` (non-restart branch) |
| `system start --restart` | `up -d --build`, plus `--force-recreate --no-deps` on the non-postgres services when postgres is already up. | `buildUpArgs` |
| `system build --rebuild` | `docker compose build --no-cache`. | `Build` |

There are two gaps:
1. **Cold start (stack down):** old images are reused with no `--build`. This is the incident above.
2. **Healthy stack:** `start` skips entirely, even when the running containers come from images built before the latest source change. This is "running" but stale.

## Open questions

1. **Cold start: always build, or detect staleness?**
   - (a) Add `--build` to the non-restart `up -d`. Compose only rebuilds changed layers, so an unchanged tree costs a few seconds of cache checks per service.
   - (b) Detect staleness: compare each built service's image `Created` time (`docker image inspect`) with the newest mtime under its build context. Rebuild (or warn) only when the image is older.
   - **Recommended: (a).** It is simple and correct by construction. Docker's cache already does the "has anything changed?" check reliably, and mtime heuristics can miss changes (git checkout timestamps, files outside the context, `.dockerignore`).
2. **Healthy stack: keep skipping?**
   - (a) Keep the skip, but run the staleness check from Q1(b) and print `::warning`-style text: "service X runs an image built <date>, but its source changed since. Run `gh optivem system start --restart`."
   - (b) Never skip: always go through `--build`, recreating only the services whose image changed (`up -d --build` without `--force-recreate` already does this).
   - **Recommended: (b).** `up -d --build` leaves unchanged containers alone, so a healthy, up-to-date stack stays fast, and a stale one is fixed instead of just reported.
3. **Does `--restart` keep any purpose after Q1/Q2?** If plain `start` always builds, `--restart` differs only in `--force-recreate` (which picks up stub mappings and other bind-mounted or non-image changes) and in keeping postgres running. **Recommended:** keep it, and reword its help text to say what it adds over plain `start`.
4. **CI impact:** CI starts from a clean runner, so `--build` there either builds anyway or rebuilds images pulled from the registry. Check whether any workflow relies on `system start` running *pulled* (not locally built) images, e.g. acceptance stages running RC images. **Recommended:** if one does, keep `--build` local-only behind the existing local/pipeline mode switch rather than adding a new flag.

## Implementation (after the open questions are resolved)

- [ ] Change the non-restart branch of `upComposeArgs` to the chosen `up` args (and remove or keep the healthy-skip in `Up`, per Q2).
- [ ] Unit-test the arg selection the same way `buildUpArgs` is tested.
- [ ] Update `--restart` help text and any docs/README that describe `system start` (grep `system start` in `docs/` and the shop's `CLAUDE.md` / `CONTRIBUTING.md`).
- [ ] Shop follow-up: drop the manual `docker rmi -f my-shop-*-frontend` workaround from `shop/plans/20261107-1705-meta-storefront-specs-build.md` (its Open question #1) once a release ships with this fix.

## Context

- Found in the shop's storefront meta-plan, Step 4 (`shop/plans/20261107-1705-meta-storefront-specs-build.md`, Open question #1).
- Never run in parallel with another plan editing `internal/build/runner/system.go`.
