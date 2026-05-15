# Plan: fix shop workflow external-I/O retry gaps

Date: 2026-05-14
Source audit: `gh-optivem/audits/20260514-shop-workflow-retry-coverage.md`
Scope: shop workflows + optivem/actions composites. Tier-ordered (composite-level cascade fixes first; leaf workflow swaps last).
Status: queued — items are discrete fix sites the audit found, one per N-A / N-B / N-C call site. Adopt-in-order or cherry-pick.

---

## Tier 1 — composite-level cascade fixes

These resolve multiple consumer-workflow findings at one source-of-truth change. Land them first.

### Item 9 — `shop/.github/workflows/monolith-typescript-commit-stage.yml :: run :: step "Run Code Analysis" line 130`

⏳ **Deferred (2026-05-15)**: upstream-retry check done — `sonarqube-scan-action@v7` has no documented retry. But the plan's prescription calls `sonar-scanner` directly, and the binary is not installed on the runner (the upstream action ships and runs it internally). Need a decision on how to install/invoke it: (a) `npx sonarqube-scanner`, (b) download the sonar-scanner-cli tarball + add to PATH, (c) add a per-project `run-sonar.sh` like the acceptance-stage uses, or (d) wrap the existing `SonarSource/sonarqube-scan-action@v7` step with `nick-fields/retry@v4` (mirrors Item 18's pattern; loses sonar-retry.sh's transient-vs-hard-fail policy but no install needed). Re-attempt after the user picks one.

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

⏳ **Deferred (2026-05-15)**: blocked by Item 9 decision.

Same swap as Item 9, projectKey `optivem_shop-multitier-backend-typescript`. (N-C.4)

### Item 11 — `shop/.github/workflows/multitier-frontend-react-commit-stage.yml :: run :: step "Run Code Analysis" line 131`

⏳ **Deferred (2026-05-15)**: blocked by Item 9 decision.

Same swap as Item 9, projectKey `optivem_shop-multitier-frontend-react`. (N-C.4)

---

## Tier 4 — deferred / verify-only

### Item 20 — Defer `npm ci` / `./gradlew build` / `dotnet restore` registry-fetch retry to a follow-up §4 pass

The `npm ci` / `./gradlew build` (which transitively triggers dependency resolution against Maven Central + Gradle plugin portal) / `dotnet restore` calls all DO hit external registries, but are backed by `actions/setup-*` cache paths and lockfile pins. The marginal failure rate is much lower than the docker / sonar incidents that drove §4 to exist. Defer to a second §4 pass with its own report — most-exposed files are `monolith-typescript-acceptance-stage-cloud.yml` (~12 inline `npm ci`), `multitier-typescript-acceptance-stage-cloud.yml` (~12 inline `npm ci`), and the 7 commit-stage files (`./gradlew build` × 2, `npm ci` × 3, `dotnet build` × 2 — only the dependency-resolving subset is in scope). (N-B.7, deferred.)

---

## Dependencies between items

Items 9, 10, 11 (typescript Sonar) — ⏳ deferred on install-step decision (see Item 9).
Item 20 — ⏳ deferred to follow-up §4 pass.

---

## Acceptance criteria

For each item, the fix is complete when:

1. The matching call site sources the shared engine (`gh-retry.sh` / `docker-retry.sh` / `sonar-retry.sh` / new `git-retry.sh`).
2. The call is wrapped with the matching `<tool>_retry` function.
3. A green CI run exercises the wrapped path at least once (a full meta-prerelease cycle for the tier-1 git items; one commit-stage run for tier-2 docker items; one acceptance-stage run for the sonar items).
4. The audit's matching N-A / N-B / N-C bullet no longer matches when the workflow-auditor agent is re-run against the same scope.

The §4 anti-pattern AP.5 ("broken-wrapper-by-undefined-function") proposed in the audit should be considered for promotion to the `workflow-auditor.md` rubric so future invocations catch the failure mode that produced Item 6.
