# Plan: add retry around `docker compose pull` in `optivem/actions/deploy-docker-compose`

Mirrors / extends [20260513-1410-add-docker-retry-shared.md](20260513-1410-add-docker-retry-shared.md). The shared `docker_retry` helper and its first two consumers (`tag-docker-images`, `resolve-docker-image-digests`) are already in place under `academy/actions/`; this plan adds the third consumer that was out of scope in the original.

## Context

On 2026-05-13, the `manual-test-7c8a1d588639b8d6` repo's manual-test pipeline ([run 25806629081](https://github.com/valentinajemuovic/manual-test-7c8a1d588639b8d6/actions/runs/25806629081/job/75811167937)) failed at **Simulate Deployment (QA Environment)**. The job uses `optivem/actions/deploy-docker-compose@v1`, whose `start.sh` runs `docker compose -f "$COMPOSE_FILE" up -d` — which implicitly pulls images. The frontend pull from ghcr.io timed out with:

```
frontend Error Get "https://ghcr.io/v2/": net/http: request canceled (Client.Timeout exceeded while awaiting headers)
Error response from daemon: Get "https://ghcr.io/v2/": net/http: request canceled (Client.Timeout exceeded while awaiting headers)
##[error]Process completed with exit code 1.
```

This is the exact class of transient GHCR failure the existing `docker_retry` helper was built to absorb. `deploy-docker-compose` was simply not on the migration list in the original PR. Without this fix, every transient GHCR blip during a manual-test or shop deploy surfaces as a hard pipeline failure that must be manually re-run.

The fix mirrors the existing consumers: source `docker-retry.sh`, do an explicit `docker compose pull` wrapped in `docker_retry` **before** `docker compose up -d`. The `up -d` step then operates on already-local images and performs no registry I/O.

## Critical files

- `actions/deploy-docker-compose/start.sh` — **edit**, source the helper, add an explicit retry-wrapped `compose pull` before the existing `up -d`

(No edits to `action.yml` — there is no `docker/login-action` step in this composite action; auth is handled by the caller workflow's earlier login step.)

## Reuse references

- `actions/shared/docker-retry.sh` — retry helper, already in place
- `actions/resolve-docker-image-digests/resolve-docker-image-digests.sh` lines 1–10, 106 — sourcing idiom and `docker_retry pull` consumer shape
- `actions/tag-docker-images/tag-docker-images.sh` — second consumer, same source pattern
- `actions/deploy-docker-compose/start.sh` lines 17–24 — current `docker compose ... up -d` block being wrapped

## Steps

### 1. Edit `actions/deploy-docker-compose/start.sh`

- Near the top (after `set -euo pipefail`), add:
  ```bash
  # shellcheck source=../shared/docker-retry.sh
  source "$(dirname "${BASH_SOURCE[0]}")/../shared/docker-retry.sh"
  ```
- Between the "📦 Images:" block (lines 7–15) and the "🐳 Running docker compose up..." line, insert an explicit retry-wrapped pull:
  ```bash
  echo "📥 Pulling images (with retry)..."
  if [[ -n "$COMPOSE_FILE" ]]; then
    docker_retry compose -f "$COMPOSE_FILE" pull
  else
    docker_retry compose pull
  fi
  echo ""
  ```
- Leave the existing `docker compose ... up -d` lines (19–24) unchanged. Without `--pull always`, `up -d` reuses the already-local images and performs no registry I/O — so the only network-touching call is the pre-pull, which is now retried.

## Out of scope

- Wrapping `docker compose up -d` itself with retry. `up` performs container creation/start as side effects; a blind retry could leave partial state. The pull is the only registry-touching operation, so wrapping just that is sufficient.
- `wait-for-endpoints` and other downstream actions — no registry I/O.
- Lint-enforcing the wrapper repo-wide via `actions/shared/_lint/check-no-raw-docker.sh` — same out-of-scope note as the original plan; tracked separately.
- Touching consumer workflows or the gh-optivem scaffold — the `@v1` retag of `optivem/actions` is the rollout vehicle.

## Verification

1. **Unit / smoke**: existing `actions/shared/_test-docker-retry.sh` continues to pass (no helper changes — only a new consumer).

2. **Re-run the failing manual-test job** once `optivem/actions` releases the new `v1`:
   ```bash
   gh run rerun 25806629081 --failed --repo valentinajemuovic/manual-test-7c8a1d588639b8d6
   ```
   - Healthy network: end-to-end success; logs show `📥 Pulling images (with retry)...` before `🐳 Running docker compose up...`.
   - GHCR blips: logs show `::notice::[docker-retry] attempt N failed … retrying in Ms`, then pull succeeds and `up -d` proceeds against local images.

3. **Manual disable check**: setting `DOCKER_RETRY_DISABLE=1` in the workflow `env:` block bypasses the loop; underlying `docker compose pull` exit code is returned unchanged on failure.

4. **Cross-stack smoke**: scaffold a fresh `manual-test-*` repo through gh-optivem (default-path scaffold) and let it run end-to-end. The deploy step should succeed on first attempt, with the new pull line visible in the QA deploy logs.
