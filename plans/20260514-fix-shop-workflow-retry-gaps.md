# Plan: fix shop workflow external-I/O retry gaps

Date: 2026-05-14
Source audit: `gh-optivem/audits/20260514-shop-workflow-retry-coverage.md`
Scope: shop workflows + optivem/actions composites. Tier-ordered (composite-level cascade fixes first; leaf workflow swaps last).
Status: queued — items are discrete fix sites the audit found, one per N-A / N-B / N-C call site. Adopt-in-order or cherry-pick.

---

## Tier 1 — composite-level cascade fixes

These resolve multiple consumer-workflow findings at one source-of-truth change. Land them first.

### Item 1 — Build `optivem/actions/shared/git-retry.sh`

The seed dependency for items 2–5. Mirror the shape of `gh-retry.sh` / `docker-retry.sh` / `sonar-retry.sh`:

- Source `retry-core.sh`.
- `_GIT_RETRY_ATTEMPTS=4`, `_GIT_RETRY_DELAYS=(5 15 45)`.
- Retryable regex (network/transient): `Could not resolve host|Connection reset by peer|RPC failed.*HTTP 5\d\d|Operation timed out|unable to access|\bEOF\b|TLS handshake|tls:.*handshake|temporary failure in name resolution|no such host|HTTP 502|HTTP 503|HTTP 504|server certificate verification failed`.
- Hard-fail regex (auth / policy / permission): `Permission denied|HTTP 401|HTTP 403|! \[remote rejected\]|pre-receive hook declined|repository .* not found|fatal: protocol|fatal: bad refspec`.
- Wrappers: `git_push_retry "$@"` → `retry_with_policy "$_GIT_RETRY_RETRYABLE" "$_GIT_RETRY_HARD_FAIL" git-push -- git push "$@"`; `git_fetch_retry "$@"` → same shape against `git fetch "$@"`.
- Add a `_test-git-retry.sh` alongside the existing `_test-*-retry.sh` shells.
- Vendor into `shop/.github/workflows/scripts/` via the existing `actions/scripts/sync-shared.sh` flow.

After this lands, items 2–5 are mechanical wrapper-swaps.

### Item 2 — `actions/publish-tag/tag.sh :: tag-job :: line 36`

Replace `if git push "$push_target" "$TAG"; then ...` with `if git_push_retry "$push_target" "$TAG"; then ...`. Source `$GITHUB_ACTION_PATH/../shared/git-retry.sh` at the top of `tag.sh`. Preserve the post-failure concurrent-push-race recovery at lines 41-46 unchanged — the retry wrapper handles transients, the existing recovery handles the legitimate "another runner won the race" case. (N-B.1)

### Item 3 — `actions/check-sha-on-branch/check.sh :: check-step :: line 14`

Source `$GITHUB_ACTION_PATH/../shared/git-retry.sh` and replace `git fetch origin "$BASE_BRANCH" --quiet` with `git_fetch_retry origin "$BASE_BRANCH" --quiet`. Note the wrapper will need to handle `--quiet` redirecting stderr — confirm the regex still matches on the buffered stderr from the wrapper, not on what `--quiet` suppresses. (N-B.2)

### Item 4 — `actions/cleanup-prereleases/action.yml :: "Fetch all tags" :: line 43`

Step body `git fetch --tags --force`. Replace with an inline `run: bash` that sources `$GITHUB_ACTION_PATH/shared/git-retry.sh` (or rebases the step into the underlying `cleanup-prereleases.sh` script which already sources `gh-retry.sh`). Either way, end with `git_fetch_retry --tags --force`. (N-B.3)

### Item 5 — `shop/.github/workflows/meta-prerelease-stage.yml :: tag-meta-rc :: step "Tag meta-rc" line 234`

Add `source "$GITHUB_WORKSPACE/.github/workflows/scripts/git-retry.sh"` near the top of the `run:` body. Replace `git push origin "$TAG"` with `git_push_retry origin "$TAG"`. (N-B.4)

### Item 6 — `shop/.github/workflows/meta-release-stage.yml :: tag-meta-release :: step "Create meta release tag and GitHub release" line 334`

Currently calls `git_push_retry origin "$RELEASE_TAG"` with no source providing the function. Add `source "$GITHUB_WORKSPACE/.github/workflows/scripts/git-retry.sh"` alongside the existing `gh-retry.sh` source on line 329. After Item 1 lands, this becomes a one-line addition — and resolves the latent "command not found" failure that would otherwise fire on the next meta-release execution. **Priority within Tier 1: highest** — currently broken. (N-B.5)

### Item 7 — `shop/.github/workflows/monolith-dotnet-acceptance-stage.yml :: sonar :: step "Run Sonar Analysis" line 388`

Replace:

```yaml
- name: Run Sonar Analysis
  uses: Wandalen/wretry.action@v3
  with:
    attempt_limit: 3
    attempt_delay: 30000
    command: cd system-test/dotnet && ./run-sonar.sh
  env:
    SONAR_TOKEN: ${{ secrets.SONAR_TOKEN }}
```

with:

```yaml
- name: Run Sonar Analysis
  shell: bash
  env:
    SONAR_TOKEN: ${{ secrets.SONAR_TOKEN }}
  run: |
    set -euo pipefail
    source "$GITHUB_WORKSPACE/.github/workflows/scripts/sonar-retry.sh"
    cd system-test/dotnet
    sonar_retry bash ./run-sonar.sh
```

Note: if `run-sonar.sh` itself only invokes one sonarscanner command, the cleaner fix is to source `sonar-retry.sh` inside `run-sonar.sh` and `sonar_retry`-wrap the scanner call there. Either works; the inline approach above is the smallest correct swap. (N-C.2, incident-driven from acceptance run 25865827466.)

### Item 8 — Apply Item 7's pattern to the 5 sibling acceptance-stage workflows

- `shop/.github/workflows/monolith-java-acceptance-stage.yml :: sonar :: step "Run Sonar Analysis"`
- `shop/.github/workflows/monolith-typescript-acceptance-stage.yml :: sonar :: step "Run Sonar Analysis"`
- `shop/.github/workflows/multitier-dotnet-acceptance-stage.yml :: sonar :: step "Run Sonar Analysis"`
- `shop/.github/workflows/multitier-java-acceptance-stage.yml :: sonar :: step "Run Sonar Analysis"`
- `shop/.github/workflows/multitier-typescript-acceptance-stage.yml :: sonar :: step "Run Sonar Analysis"`

Same swap as Item 7 in each — file-by-file. Line numbers within ~10 of 388 in each (mechanical to locate). (N-C.2, completion of incident fix.)

### Item 9 — `shop/.github/workflows/monolith-typescript-commit-stage.yml :: run :: step "Run Code Analysis" line 130`

Replace the `SonarSource/sonarqube-scan-action@v7` step with an inline `run:` that sources `sonar-retry.sh` and invokes the scanner. **Precondition:** decide first whether `sonarqube-scan-action@v7` already has internal retry semantics — check upstream changelog. If yes, mark R-DOC-OK with a comment at the call site and **skip Item 9** (and Items 10, 11). If no, proceed with:

```yaml
- name: Run Code Analysis
  if: steps.verify-main.outputs.on-branch == 'true' && env.SONAR_TOKEN != ''
  shell: bash
  env:
    SONAR_TOKEN: ${{ secrets.SONAR_TOKEN }}
  working-directory: system/monolith/typescript
  run: |
    set -euo pipefail
    source "$GITHUB_WORKSPACE/.github/workflows/scripts/sonar-retry.sh"
    sonar_retry sonar-scanner \
      -Dsonar.projectKey=optivem_shop-monolith-typescript \
      -Dsonar.projectName=shop-monolith-typescript \
      -Dsonar.organization=optivem \
      -Dsonar.sources=src
```

(N-C.4, conditional on upstream-retry verification.)

### Item 10 — `shop/.github/workflows/multitier-backend-typescript-commit-stage.yml :: run :: step "Run Code Analysis" line 130`

Same swap as Item 9, projectKey `optivem_shop-multitier-backend-typescript`. (N-C.4)

### Item 11 — `shop/.github/workflows/multitier-frontend-react-commit-stage.yml :: run :: step "Run Code Analysis" line 131`

Same swap as Item 9, projectKey `optivem_shop-multitier-frontend-react`. (N-C.4)

---

## Tier 2 — workflow-level batch swaps

These are bulk changes against duplicated patterns. After Tier 1 the engine has full coverage; Tier 2 migrates the long tail of inline / 3rd-party wrappers to the canonical engine.

### Item 12 — Create `optivem/actions/docker-login@v1` composite

Wraps `docker/login-action@v4` in a `run:` that sources `docker-retry.sh` and invokes the login via `docker_retry login` — or, if cleaner, wraps the `docker/login-action@v4` step itself in a `nick-fields/retry@v4` configured at `max_attempts: 4, retry_wait_seconds: 5, polling_interval_seconds: 0` with the engine's exponential schedule. Inputs: `registry`, `username`, `password`. The composite's existence is the prerequisite for Item 13.

### Item 13 — Bulk-replace `Wandalen/wretry.action@v3 + docker/login-action@v4` (167 sites)

Across 51 shop workflow files, replace each occurrence with `uses: optivem/actions/docker-login@v1` (from Item 12). One PR per workflow archetype (commit-stage / acceptance-stage / acceptance-stage-cloud / qa-stage / qa-stage-cloud / prod-stage / prod-stage-cloud / acceptance-stage-legacy) keeps blast radius reviewable. (N-C.1)

Representative starting points (others identical in shape):

- `shop/.github/workflows/monolith-dotnet-commit-stage.yml :: build-push :: line 80, 92`
- `shop/.github/workflows/monolith-dotnet-acceptance-stage.yml :: check / run :: lines 93, 103, 176, 186`
- `shop/.github/workflows/monolith-dotnet-acceptance-stage-cloud.yml :: check / deploy-app / deploy-external-real / deploy-external-stub :: lines 77, 87, 148, 197, 242`
- `shop/.github/workflows/monolith-dotnet-prod-stage.yml :: run :: lines 125, 135`
- `shop/.github/workflows/monolith-dotnet-prod-stage-cloud.yml :: promote / deploy :: lines 107, 134, 197`
- `shop/.github/workflows/drift.yml :: drift :: lines 111, 121`
- `shop/.github/workflows/cross-lang-system-verification.yml :: cross-lang :: lines 147, 157`

(Full 51-file list reproducible by `rg -nU '^\s+uses: Wandalen/wretry\.action@v3$' shop/.github/workflows`.)

### Item 14 — `monolith-dotnet-commit-stage.yml :: build-push :: step "Pre-pull base images" line 173-190`

Replace the inline `for attempt in 1 2 3; do if docker pull "$image"; then break; fi; ... sleep $((attempt * 15)); done` retry loop with:

```yaml
- name: Pre-pull base images
  if: steps.verify-main.outputs.on-branch == 'true'
  shell: bash
  run: |
    set -euo pipefail
    source "$GITHUB_WORKSPACE/.github/workflows/scripts/docker-retry.sh"
    for image in mcr.microsoft.com/dotnet/sdk:8.0 mcr.microsoft.com/dotnet/aspnet:8.0; do
      docker_retry pull "$image"
    done
```

Same swap, same image list pattern, in each sibling. (N-C.3)

### Item 15 — Apply Item 14's pattern to the 6 sibling commit-stage workflows

- `shop/.github/workflows/monolith-java-commit-stage.yml :: build-push :: step "Pre-pull base images" line 179-189`
- `shop/.github/workflows/monolith-typescript-commit-stage.yml :: build-push :: step "Pre-pull base images" line 177-187`
- `shop/.github/workflows/multitier-backend-dotnet-commit-stage.yml :: build-push :: step "Pre-pull base images" line 170-180`
- `shop/.github/workflows/multitier-backend-java-commit-stage.yml :: build-push :: step "Pre-pull base images" line 179-189`
- `shop/.github/workflows/multitier-backend-typescript-commit-stage.yml :: build-push :: step "Pre-pull base images" line 177-187`
- `shop/.github/workflows/multitier-frontend-react-commit-stage.yml :: build-push :: step "Pre-pull base images" line 178-188`

(N-C.3, completion.)

---

## Tier 3 — leaf workflow fixes

Discrete single-line swaps. Lowest blast radius per fix, but each is genuinely missing retry coverage.

### Item 16 — Bulk-replace `gh extension install optivem/gh-optivem` (16 sites)

Replace each unwrapped invocation with an inline `run: bash` block that sources `gh-retry.sh` and invokes `gh_retry extension install optivem/gh-optivem`. Sites:

- `shop/.github/workflows/_prerelease-pipeline.yml :: smoke :: step "Install gh-optivem CLI extension" line 194`
- `shop/.github/workflows/cross-lang-system-verification.yml :: cross-lang :: step "Install gh-optivem CLI extension" line 144`
- `shop/.github/workflows/drift.yml :: drift / drift-typescript :: steps "Install gh-optivem CLI extension" lines 108, 295`
- `shop/.github/workflows/monolith-dotnet-acceptance-stage.yml :: run :: step "Install gh-optivem CLI extension" line 280`
- `shop/.github/workflows/monolith-dotnet-acceptance-stage-legacy.yml :: run :: step "Install gh-optivem CLI extension" line 244`
- `shop/.github/workflows/monolith-java-acceptance-stage.yml :: run :: step "Install gh-optivem CLI extension" line 277`
- `shop/.github/workflows/monolith-java-acceptance-stage-legacy.yml :: run :: step "Install gh-optivem CLI extension" line 244`
- `shop/.github/workflows/monolith-typescript-acceptance-stage.yml :: run :: step "Install gh-optivem CLI extension" line 266`
- `shop/.github/workflows/monolith-typescript-acceptance-stage-legacy.yml :: run :: step "Install gh-optivem CLI extension" line 235`
- `shop/.github/workflows/multitier-dotnet-acceptance-stage.yml :: run :: step "Install gh-optivem CLI extension" line 285`
- `shop/.github/workflows/multitier-dotnet-acceptance-stage-legacy.yml :: run :: step "Install gh-optivem CLI extension" line 250`
- `shop/.github/workflows/multitier-java-acceptance-stage.yml :: run :: step "Install gh-optivem CLI extension" line 282`
- `shop/.github/workflows/multitier-java-acceptance-stage-legacy.yml :: run :: step "Install gh-optivem CLI extension" line 250`
- `shop/.github/workflows/multitier-typescript-acceptance-stage.yml :: run :: step "Install gh-optivem CLI extension" line 271`
- `shop/.github/workflows/multitier-typescript-acceptance-stage-legacy.yml :: run :: step "Install gh-optivem CLI extension" line 241`

Replacement shape (per site):

```yaml
- name: Install gh-optivem CLI extension
  shell: bash
  env:
    GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
  run: |
    set -euo pipefail
    source "$GITHUB_WORKSPACE/.github/workflows/scripts/gh-retry.sh"
    gh_retry extension install optivem/gh-optivem
```

Sufficiently mechanical that a single PR with `sed`-driven changes is reasonable. (N-A.1)

### Item 17 — `shop/.github/workflows/bump-patch-version-multirepo.yml :: dispatch :: step "Dispatch sibling bumpers" line 43`

Replace `gh workflow run bump-patch-version.yml --repo "$repo"` (inside the `for repo in $siblings; do ... done` loop) with `gh_retry workflow run bump-patch-version.yml --repo "$repo"`. Source `gh-retry.sh` at the top of the `run:` body. The intentional cross-repo `--repo` flag stays (it's not a §1-B finding — the dispatch is per-sibling-repo, not against `${{ github.repository }}`). (N-A.2)

### Item 18 — `shop/.github/workflows/lint-workflows.yml :: actionlint :: step "Install actionlint" line 28-32`

Wrap the `curl` install in a retry. The smallest correct change is to switch to `nick-fields/retry@v4`:

```yaml
- name: Install actionlint
  uses: nick-fields/retry@v4
  with:
    max_attempts: 4
    timeout_minutes: 1
    retry_wait_seconds: 5
    polling_interval_seconds: 0
    command: |
      bash <(curl -fsSL https://raw.githubusercontent.com/rhysd/actionlint/v1.7.7/scripts/download-actionlint.bash) 1.7.7
      ./actionlint --version
```

The 4×{5,15,45} schedule isn't directly expressible in `nick-fields/retry@v4` (it uses constant `retry_wait_seconds`), so a flat 5s × 4 is the closest practical match — acceptable for a non-release-path step. Long-term, see if a future `optivem/actions/curl-retry@v1` is worth introducing. (N-B.6)

---

## Tier 4 — deferred / verify-only

### Item 19 — Verify `google-github-actions/*@v3` internal retry semantics

Composite. Check the upstream changelog for `google-github-actions/auth@v3`, `google-github-actions/setup-gcloud@v3`, `google-github-actions/deploy-cloudrun@v3`. If they document internal retry, leave the existing call sites and add a one-line comment at one representative call site marking R-DOC-OK so the next audit pass records that decision. If they do not, open a new fix item to wrap with `nick-fields/retry@v4`. Sites are in the 18 `*-stage-cloud.yml` files. (Examined-and-rejected → R-OK with documentation, or future fix.)

### Item 20 — Defer `npm ci` / `./gradlew build` / `dotnet restore` registry-fetch retry to a follow-up §4 pass

The `npm ci` / `./gradlew build` (which transitively triggers dependency resolution against Maven Central + Gradle plugin portal) / `dotnet restore` calls all DO hit external registries, but are backed by `actions/setup-*` cache paths and lockfile pins. The marginal failure rate is much lower than the docker / sonar incidents that drove §4 to exist. Defer to a second §4 pass with its own report — most-exposed files are `monolith-typescript-acceptance-stage-cloud.yml` (~12 inline `npm ci`), `multitier-typescript-acceptance-stage-cloud.yml` (~12 inline `npm ci`), and the 7 commit-stage files (`./gradlew build` × 2, `npm ci` × 3, `dotnet build` × 2 — only the dependency-resolving subset is in scope). (N-B.7, deferred.)

---

## Dependencies between items

```
Item 1 (git-retry.sh)
  → Item 2 (publish-tag/tag.sh)
  → Item 3 (check-sha-on-branch/check.sh)
  → Item 4 (cleanup-prereleases/action.yml)
  → Item 5 (meta-prerelease-stage.yml)
  → Item 6 (meta-release-stage.yml — fixes latent runtime bug)

Item 7 (one acceptance-stage Sonar) — independent; sonar-retry.sh already exists
  → Item 8 (5 sibling acceptance-stages — same swap)

Items 9, 10, 11 (typescript Sonar) — gated on upstream-retry verification of `sonarqube-scan-action@v7`

Item 12 (docker-login@v1 composite)
  → Item 13 (51 workflow files migrated)

Item 14 (one commit-stage docker pull loop) — independent; docker-retry.sh exists
  → Item 15 (6 sibling commit-stages — same swap)

Items 16, 17, 18 — independent leaf-workflow fixes; can land in parallel
```

Items 1, 7, 12, 14, 16, 17, 18 can begin in parallel. Items 2-6 are sequential after Item 1. Items 8 and 15 are mechanical follow-ups to 7 and 14 respectively. Item 13 is gated on Item 12. Items 9-11 are gated on upstream verification.

---

## Acceptance criteria

For each item, the fix is complete when:

1. The matching call site sources the shared engine (`gh-retry.sh` / `docker-retry.sh` / `sonar-retry.sh` / new `git-retry.sh`).
2. The call is wrapped with the matching `<tool>_retry` function.
3. A green CI run exercises the wrapped path at least once (a full meta-prerelease cycle for the tier-1 git items; one commit-stage run for tier-2 docker items; one acceptance-stage run for the sonar items).
4. The audit's matching N-A / N-B / N-C bullet no longer matches when the workflow-auditor agent is re-run against the same scope.

The §4 anti-pattern AP.5 ("broken-wrapper-by-undefined-function") proposed in the audit should be considered for promotion to the `workflow-auditor.md` rubric so future invocations catch the failure mode that produced Item 6.
