# Plan: add retry around `docker` registry operations in `optivem/actions`

## Context

On 2026-05-13, `optivem/shop` workflow run [25801842099](https://github.com/optivem/shop/actions/runs/25801842099) — the `meta-release-stage` promotion for `meta-v1.0.87-rc.306` — failed at the `promote-multitier-dotnet` job. The child workflow it dispatched ([run 25801865586](https://github.com/optivem/shop/actions/runs/25801865586), `multitier-dotnet-prod-stage.yml`) failed at the **"Tag Docker Images with Component Release Versions"** step with:

```
Error response from daemon: Get "https://ghcr.io/v2/": context deadline exceeded
(Client.Timeout exceeded while awaiting headers)
```

A one-shot GHCR network blip — the exact class of failure the existing `shared/gh-retry.sh` was built to absorb for `gh` CLI calls. There is no equivalent for `docker` registry operations, so any transient GHCR/DockerHub blip surfaces as a hard pipeline failure that must be manually re-run.

The fix mirrors the existing `gh-retry` pattern for `docker`: one shared helper, used by every action that shells out to the registry; plus `Wandalen/wretry.action@v3` around the in-action `docker/login-action@v4` step (a JS Action, so it can't go through a shell wrapper). User chose the "full convergence" scope — both docker-using actions migrate to the helper in this PR.

## Critical files

- `actions/shared/docker-retry.sh` — **new**, shell wrapper mirroring `actions/shared/gh-retry.sh`
- `actions/shared/_test-docker-retry.sh` — **new**, test harness mirroring `actions/shared/_test-gh-retry.sh`
- `actions/tag-docker-images/tag-docker-images.sh` — **edit**, swap `docker buildx imagetools create` for `docker_retry buildx imagetools create`
- `actions/tag-docker-images/action.yml` — **edit**, wrap `docker/login-action@v4` step with `Wandalen/wretry.action@v3`
- `actions/resolve-docker-image-digests/resolve-docker-image-digests.sh` — **edit**, replace inline retry loop around `docker pull` with `docker_retry pull`; wrap unprotected `docker inspect` with `docker_retry inspect`

## Reuse references

- `actions/shared/gh-retry.sh` — shape, error-classification grep pattern, backoff structure, `*_RETRY_DISABLE=1` escape hatch
- `actions/shared/_test-gh-retry.sh` — fake-binary-on-PATH test harness shape
- `actions/check-commit-status-exists/action.yml` (or any other consumer) — shows the `source "$GITHUB_ACTION_PATH/../shared/gh-retry.sh"` sourcing idiom inside the inline shell of a composite step
- `optivem/shop/.github/workflows/multitier-dotnet-prod-stage.yml` lines 126–142 — `Wandalen/wretry.action@v3` wrapping `docker/login-action@v4`

## Steps

### 1. Create `actions/shared/docker-retry.sh`

Mirror `gh-retry.sh` structure. Differences:

- Function name: `docker_retry`.
- Retryable regex tuned for the docker daemon's error vocabulary:
  - `context deadline exceeded`
  - `Client\.Timeout`
  - `i/o timeout`, `timed out`
  - `connection reset`, `connection refused`
  - `\bEOF\b`, `unexpected EOF`
  - `TLS handshake`, `tls:.*handshake`
  - `temporary failure in name resolution`, `no such host`
  - `HTTP 5[0-9][0-9]`, `Internal Server Error`, `Bad Gateway`, `Service Unavailable`, `Gateway Timeout`
  - `server error`
  - `received unexpected HTTP status: 5[0-9][0-9]`
- Hard-fail regex (never retry — pass through immediately):
  - `unauthorized`, `denied: permission`
  - `manifest unknown`, `name unknown`, `repository name not known`
  - `requested access to the resource is denied`
- Attempts: 4. Delays: 5s / 15s / 45s.
- Escape hatch: `DOCKER_RETRY_DISABLE=1`.
- Same stdout/stderr-buffered semantics so `out=$(docker_retry inspect …)` callers keep working.

### 2. Create `actions/shared/_test-docker-retry.sh`

Mirror `_test-gh-retry.sh`: fake `docker` on `PATH` reading scripted `(exit_code | stderr_text)` sequence from `DOCKER_FAKE_SEQ`. Test cases:

- success on first attempt → no retry
- transient on attempt 1, success on attempt 2 → exit 0, output preserved
- 4 transients → exit non-zero after exhausting attempts, warning emitted
- hard-fail (`unauthorized`) → exit immediately, no retry
- non-classified failure → pass through (no retry), exit code preserved
- `DOCKER_RETRY_DISABLE=1` → bypass loop entirely

Override `_DOCKER_RETRY_DELAYS=(0 0 0)` for fast tests.

### 3. Edit `actions/tag-docker-images/tag-docker-images.sh`

- Near the top (after the `: "${GITHUB_OUTPUT:?…}"` line), add:
  ```bash
  # shellcheck source=../shared/docker-retry.sh
  source "$(dirname "${BASH_SOURCE[0]}")/../shared/docker-retry.sh"
  ```
- In `retag_image()`, replace line 82:
  ```bash
  if ! docker buildx imagetools create --tag "$new_image_url" "$source_image_url"; then
  ```
  with:
  ```bash
  if ! docker_retry buildx imagetools create --tag "$new_image_url" "$source_image_url"; then
  ```

No behaviour change on the happy path; transient daemon errors get up to 3 retries before falling through to the existing `failed_images` tracking.

### 4. Edit `actions/tag-docker-images/action.yml`

Wrap the existing `Log in to Container Registry` step (lines 38–43) with `Wandalen/wretry.action@v3`:

```yaml
- name: Log in to Container Registry
  uses: Wandalen/wretry.action@v3
  with:
    action: docker/login-action@v4
    attempt_limit: 3
    attempt_delay: 10000
    with: |
      registry: ${{ inputs.registry }}
      username: ${{ inputs.registry-username }}
      password: ${{ inputs.token }}
```

(Same shape as the workflow-level logins already use in `optivem/shop`.)

### 5. Edit `actions/resolve-docker-image-digests/resolve-docker-image-digests.sh`

- Add the same `source "$(dirname "${BASH_SOURCE[0]}")/../shared/docker-retry.sh"` line near the top.
- Replace the inline retry loop (lines 103–120) with a single call:
  ```bash
  if ! docker_retry pull "$image_url"; then
    echo "::error::Failed to pull Docker image: $image_url"
    exit 1
  fi
  ```
- Replace the unprotected `docker inspect` (line 123):
  ```bash
  if ! inspect_json="$(docker_retry inspect "$image_url")"; then
  ```

## Out of scope (logical follow-ups, not in this PR)

- `actions/shared/_lint/check-no-raw-docker.sh` mirroring `check-no-raw-gh.sh` to lint-enforce the wrapper across the repo. Worth doing later but materially expands surface and isn't needed to fix the failing run.
- Touching any other repo's workflows — `optivem/shop`'s workflow-level logins are already wretry-wrapped.

## Verification

1. **Unit / smoke**:
   ```bash
   bash actions/shared/_test-docker-retry.sh
   ```
   All assertions pass (transient retry, hard-fail pass-through, exhaustion, escape hatch, stdout preservation).

2. **Re-run the failing promotion** once changes are pushed and `optivem/actions` releases the new `v1` (or whichever tag the consumer pins):
   ```bash
   gh run rerun 25801842099 --failed --repo optivem/shop
   ```
   - On a healthy network: succeeds end-to-end.
   - If GHCR blips again: the new `docker_retry` absorbs it; logs will show `::notice::[docker-retry] attempt N failed … retrying in Ms`.

3. **Cross-stack smoke** (independent of the failing one): trigger one of the other `*-prod-stage.yml` workflows (e.g. `multitier-java-prod-stage.yml`) end-to-end to confirm no regression in the happy path of `tag-docker-images` or `resolve-docker-image-digests`.

4. **Manual disable check**: confirm `DOCKER_RETRY_DISABLE=1 docker_retry pull <bad-image>` skips the loop and returns the underlying error/exit code unchanged.
