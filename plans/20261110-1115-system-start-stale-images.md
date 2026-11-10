# 2026-11-10 11:15 — `system start` silently reuses stale Docker images

## TL;DR

**Why:** Plain `gh optivem system start` runs `docker compose up -d` without `--build`, so it starts whatever images are already there, however old. In the shop's storefront Step 4 (2026-11-10), 5-week-old `my-shop-real-frontend` / `my-shop-stub-frontend` images caused false multitier test failures. They only went away after a manual `docker rmi -f`.
**End result:** Plain `system start` always runs `docker compose up -d --build`, so it never runs an image older than its source. Docker's layer cache keeps an unchanged rebuild down to a few seconds.
- **Stack down / unhealthy:** `down`, then `up -d --build` (previously plain `up -d`).
- **Stack healthy:** no more "System is already running, skipping restart". It runs `up -d --build` with no `down`, and Compose recreates only the services whose image changed. Up-to-date stacks stay fast; stale ones get fixed.
- **CI:** behaves the same. Pipeline compose files pull the system under test via `image:` only, so `--build` touches only the simulators, which a clean runner builds anyway. There is no local/pipeline gating.
- **Unchanged:** `--restart` (incremental force-recreate, postgres kept up; its help text is reworded to say what it adds over plain `start`), `--restart --external-only`, and `system build --rebuild`.

## ▶ Next executable step (resume here)

Only the deferred shop follow-up remains: after a gh-optivem release includes this fix, edit `shop/plans/20261107-1705-meta-storefront-specs-build.md` (drop the `docker rmi -f` workaround, Open question #1) and `shop/CONTRIBUTING.md` (`--restart` bullet).

## Current behaviour (verified 2026-11-10)

| Command | Behaviour | Code |
|---|---|---|
| `system start` | If any URL is healthy, skip ("System is already running, skipping restart"). Otherwise `down`, then `up -d` with **no `--build`**, so existing images are reused. | `internal/build/runner/system.go` `Up` / `upComposeArgs` (non-restart branch) |
| `system start --restart` | `up -d --build`, plus `--force-recreate --no-deps` on the non-postgres services when postgres is already up. | `buildUpArgs` |
| `system build --rebuild` | `docker compose build --no-cache`. | `Build` |

There are two gaps:
1. **Cold start (stack down):** old images are reused with no `--build`. This is the incident above.
2. **Healthy stack:** `start` skips entirely, even when the running containers come from images built before the latest source change. This is "running" but stale.

## Resolved decisions

1. **Cold start: always `--build`** (not staleness detection). The non-restart `up -d` becomes `up -d --build`. Docker's layer cache is the authoritative "has anything changed?" check; mtime heuristics miss changes (git checkout timestamps, files outside the context, `.dockerignore`), and an unchanged rebuild costs only a few seconds of cache checks per service.
2. **Healthy stack: never skip, and no `down`.** Remove the "System is already running, skipping restart" shortcut from `Up`. A healthy stack gets `up -d --build` only (no `--force-recreate`, no preceding `down`), so Compose recreates just the services whose image changed: an up-to-date stack stays fast and a stale one gets fixed. A stack that is not healthy keeps today's `down` (clears stray containers) and then runs `up -d --build`. The `IsAnyURLUp` probe stays, but it now picks between "down first" and "no down" instead of deciding whether to skip.
3. **`--restart` stays; reword its help text.** Its behaviour is unchanged. Since plain `start` now also builds, the help text (and the `SystemOptions.Restart` / `Up` doc comments) must say what `--restart` adds:
   - `--force-recreate` of every non-persistent service, which reloads bind-mounted WireMock stub mappings and re-runs Flyway even when no image changed.
   - postgres and its data volume stay running.
   - it is the prerequisite for `--external-only`.
4. **CI: uniform `--build`, no local/pipeline gating.** Verified 2026-11-10: in the shop's `docker/*/*/docker-compose.pipeline.{real,stub}.yml`, the system under test (frontend/backend) is pulled through `image: ${SYSTEM_IMAGE_*:-ghcr.io/...}` and has **no** `build:` section. The only service with `build:` is `external-system-simulators` (in `pipeline.real`), which a clean runner builds anyway. `--build` only affects services that have a `build:` section, so pulled RC images are never rebuilt. Add a short code comment recording this, so a future reader doesn't add gating.

## Implementation

Gh-optivem side done (plain `start` → `up -d --build`, healthy-skip removed, `plainStartArgs` + `TestPlainStartArgs`, `--restart` help + `docs/cli-reference.md` reworded).

- [ ] Shop follow-up: once a gh-optivem release ships with this fix, (a) drop the manual `docker rmi -f my-shop-*-frontend` workaround from `shop/plans/20261107-1705-meta-storefront-specs-build.md` (its Open question #1), and (b) reword `shop/CONTRIBUTING.md`'s `--restart` bullet, which still says plain `system start` reuses possibly stale images. — ⏳ Deferred: blocked on a gh-optivem release containing this fix.

## Context

- Found in the shop's storefront meta-plan, Step 4 (`shop/plans/20261107-1705-meta-storefront-specs-build.md`, Open question #1).
- Never run in parallel with another plan editing `internal/build/runner/system.go`.
