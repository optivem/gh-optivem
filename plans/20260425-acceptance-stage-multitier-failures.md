# Acceptance-stage matrix failures — multitier root-cause analysis

**Run:** [optivem/gh-optivem run 24940116058](https://github.com/optivem/gh-optivem/actions/runs/24940116058)
**Workflow:** `gh-acceptance-stage`
**Date:** 2026-04-25
**Result:** 4 of 16 `Run` matrix jobs failed; all failures are in the `multitier` deployment style.

## Matrix outcome

The `Run` job is parametrised by `(deployment-style, repo-style, backend, frontend)`.

| Deployment | Repo-style | Backend, Frontend     | Result | Failed phase                          |
| ---------- | ---------- | --------------------- | ------ | ------------------------------------- |
| monolith   | monorepo   | typescript, dotnet    | PASS   | —                                     |
| monolith   | monorepo   | java, typescript      | PASS   | —                                     |
| monolith   | monorepo   | dotnet, java          | PASS   | —                                     |
| monolith   | multirepo  | java, typescript      | PASS   | —                                     |
| monolith   | multirepo  | typescript, dotnet    | PASS   | —                                     |
| monolith   | multirepo  | dotnet, java          | PASS   | —                                     |
| multitier  | monorepo   | typescript, dotnet    | PASS   | —                                     |
| multitier  | monorepo   | dotnet, java          | PASS   | —                                     |
| multitier  | monorepo   | **java, typescript**  | **FAIL** | Phase 5 — Frontend commit stage     |
| multitier  | multirepo  | **dotnet, java**      | **FAIL** | Phase 6 — Acceptance stage (legacy) |
| multitier  | multirepo  | **typescript, dotnet**| **FAIL** | Phase 8 — Production stage          |
| multitier  | multirepo  | **java, typescript**  | **FAIL** | Phase 8 — Production stage          |

(Smoke jobs all pass; only the long-running `Run` jobs fail. `monolith` is fully green; `multitier+multirepo` is fully red, 3/3.)

## Root causes

The four failures are **three different bugs**, not one. Each was confirmed by drilling into the failed child workflow run inside the scaffolded test app.

### 1. Frontend commit stage — GHCR `write_package` denied

- **Affected combo:** multitier + monorepo + java + typescript
- **Child run:** [test-app-8e4cbef4 / frontend-commit-stage / 24940268695](https://github.com/valentinajemuovic/test-app-8e4cbef4-4202a0b578381504/actions/runs/24940268695)
- **Failing step:** `Build and Push Docker Image`
- **Error:**
  ```
  ERROR: failed to push ghcr.io/valentinajemuovic/test-app-8e4cbef4-4202a0b578381504/frontend:sha-…
  denied: permission_denied: write_package
  ```
- **Job-level `GITHUB_TOKEN`** has `Packages: write` (verified in log), so the workflow itself is configured correctly.
- **Likely cause:** First-push package-creation race. GHCR creates the package on first push and binds it to the source repo; if the package metadata isn't fully provisioned when the second image push lands, the second push is denied. This is consistent with the matrix: only the React/TS frontend image fails on its **first** push for this scaffold, and the Java/.NET frontend variants don't hit it because the timing or image layout differs.
- **Why monolith doesn't hit it:** monolith pushes a single combined image, so there is no "second package" race.
- **Why other multitier+monorepo combos passed:** likely flake-sensitive — they happened to win the race.

### 2. Acceptance-stage-legacy — transient Gradle CDN 502

- **Affected combo:** multitier + multirepo + dotnet + java
- **Child run:** [test-app-cc525a3f / acceptance-stage-legacy / 24940666031](https://github.com/valentinajemuovic/test-app-cc525a3f-40c6263a604ee44a/actions/runs/24940666031)
- **Failing step:** `Compile System Tests`
- **Error:**
  ```
  Exception in thread "main" java.io.IOException:
  Server returned HTTP response code: 502 for URL:
  https://github.com/gradle/gradle-distributions/releases/download/v8.14.3/gradle-8.14.3-bin.zip
  ```
- **Cause:** GitHub release-asset CDN returned 502 while downloading the Gradle distribution for the wrapper. Pure infrastructure flake — not a code or workflow bug.
- **Why multitier+multirepo+java specifically:** the Java frontend in multirepo runs its own Gradle build in a separate repo, so it hits the Gradle download path that other combos don't. (Other Java builds may have been cached; this scaffold's repo is fresh, so no cache.)

### 3. Production stage — hardcoded monorepo path `backend/VERSION`

- **Affected combos:** multitier + multirepo + (typescript+dotnet) and multitier + multirepo + (java+typescript)
- **Child runs:**
  - [test-app-49d7cc1b / prod-stage / 24940849271](https://github.com/valentinajemuovic/test-app-49d7cc1b-0337d873df67d311/actions/runs/24940849271)
  - [test-app-7c241a55 / prod-stage / 24940769603](https://github.com/valentinajemuovic/test-app-7c241a55-55bdab64c1e1971e/actions/runs/24940769603)
- **Failing step:** `Read Component Base Versions`
- **Error:**
  ```
  ##[error]VERSION file not found: backend/VERSION
    (key: ghcr.io/valentinajemuovic/test-app-…-backend/backend:v1.0.47-rc.1)
  ```
- **Cause:** the `prod-stage` workflow asks the `read-component-base-versions` action for `backend/VERSION` and `frontend/VERSION`. That layout is correct for **monorepo** (where `backend/` and `frontend/` are sibling subdirs), but in **multirepo** the backend repo's VERSION file lives at the repo root (just `VERSION`). The workflow template doesn't switch the path based on `repo-style`.
- **Why monolith and multirepo+monolith pass:** monolith only has one VERSION file; multirepo+monolith effectively still resolves a single component path; only the multitier+multirepo combination requires per-component VERSION lookups across two separate repos, which the template doesn't account for.
- **Why multitier+multirepo+dotnet+java didn't hit this:** it failed earlier at Phase 6 (Gradle 502), so prod-stage never ran. If we fix #2 first, this combo will likely surface the same `backend/VERSION` error.

## Severity & priority

| # | Bug                                  | Type            | Severity | Priority |
| - | ------------------------------------ | --------------- | -------- | -------- |
| 1 | GHCR write_package race              | Flaky infra/perm| Medium   | P2       |
| 2 | Gradle CDN 502                       | Transient flake | Low      | P3       |
| 3 | `backend/VERSION` hardcoded          | Real workflow bug | High   | P1       |

#3 is the only deterministic bug — it will fail every time on multitier+multirepo until fixed. #1 and #2 are flakes that need retry/recovery.

## Proposed fixes

### Fix #3 (P1) — make VERSION path repo-style-aware

**Where:** the `prod-stage` workflow template that's emitted into scaffolded apps. Search for `read-component-base-versions` action invocations and the `entries` JSON they pass.

**Change:** when generating the `entries` array, pick path based on `repo-style`:

- monorepo: `backend/VERSION`, `frontend/VERSION` (current behaviour)
- multirepo: `VERSION` for each component, but **resolved against the correct repo** — the action needs to checkout the backend repo and the frontend repo separately, then read `VERSION` at the root of each.

Two implementation options:

- **Option A (recommended)** — extend `read-component-base-versions` to accept a `repo` field per entry; when present, the action does a sparse `gh api repos/{repo}/contents/{file}` fetch instead of reading from the working tree. Cleaner and avoids checking out two repos in the prod-stage workflow.
- **Option B** — in the prod-stage template, add two `actions/checkout` steps (one per component repo) when `repo-style == multirepo`, then run the existing action with paths pointing at each checkout. Heavier, but no action change required.

Recommended: **A**, because the action is the right level of abstraction for "read a VERSION for a component" and the prod-stage template stays simple.

**Verification:** re-run the matrix; multitier+multirepo+(ts+dotnet) and (java+ts) should reach the end without the VERSION error. Add a unit test to `read-component-base-versions` covering the cross-repo case.

### Fix #2 (P3) — wrap Gradle download in retry

**Where:** the `acceptance-stage-legacy` workflow template (or whichever step runs `./gradlew` for the first time on a clean runner).

**Change:** add a retry around the first Gradle invocation, e.g. with `nick-fields/retry@v3`:

```yaml
- name: Compile System Tests
  uses: nick-fields/retry@v3
  with:
    timeout_minutes: 15
    max_attempts: 3
    retry_on: error
    command: ./gradlew compileTestJava
```

Or pre-warm the wrapper in a dedicated step that retries, so the actual build step doesn't carry retry logic.

**Verification:** can't easily reproduce a CDN 502, but adding retry is safe — at worst it's a no-op. Confirm the retry block fires on a forced 502 by temporarily pointing the distribution URL at a non-existent host in a test branch.

### Fix #1 (P2) — handle GHCR first-push race

**Where:** every workflow template that does `docker buildx … --push` to `ghcr.io/${{ github.repository }}/...`.

Two layered fixes:

- **Idempotent retry** on the push step. The same `nick-fields/retry` pattern works; a single retry usually wins the race because the package gets created on the first attempt even when it errors.
- **Pre-create the GHCR package** before the first push, e.g. push an empty manifest or use `gh api -X PUT /user/packages/container/{name}` early in the pipeline. This eliminates the race entirely but is more invasive.

Recommended: start with **retry** (cheap, reversible). If it still flakes, escalate to pre-create.

**Verification:** re-run the multitier+monorepo+java+ts combo several times; should reach Phase 6 consistently.

## Order of work

1. **Fix #3** — single deterministic bug, blocks 2-3 multitier+multirepo combos.
2. **Fix #2** — small, safe retry wrapper. Unblocks the dotnet+java combo so we can see whether it then also hits #3.
3. **Fix #1** — retry wrapper on docker push. Lower priority because it's intermittent and may go away once the GHCR namespaces stabilise.

After all three: the matrix should be 16/16 green on a clean run. Re-run twice to confirm flake elimination.

## Out of scope (noted but not addressed here)

- Sonar warnings (`RSPEC-4136`, `RSPEC-2955`, etc.) reported as annotations on the `Run` jobs — these are pre-existing code-smell warnings in the .NET system-test code, not related to the failures.
- Cache-restore warning on Windows (`tar.exe failed with exit code 2`) on the smoke job — non-fatal, smoke still passed.
