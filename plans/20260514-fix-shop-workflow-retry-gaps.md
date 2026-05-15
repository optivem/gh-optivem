# Plan: fix shop workflow external-I/O retry gaps

Date: 2026-05-14
Source audit: `gh-optivem/audits/20260514-shop-workflow-retry-coverage.md`
Scope: shop workflows + optivem/actions composites. Tier-ordered (composite-level cascade fixes first; leaf workflow swaps last).
Status: queued — items are discrete fix sites the audit found, one per N-A / N-B / N-C call site. Adopt-in-order or cherry-pick.

---

## Tier 4 — deferred / verify-only

### Item 20 — Defer `npm ci` / `./gradlew build` / `dotnet restore` registry-fetch retry to a follow-up §4 pass

The `npm ci` / `./gradlew build` (which transitively triggers dependency resolution against Maven Central + Gradle plugin portal) / `dotnet restore` calls all DO hit external registries, but are backed by `actions/setup-*` cache paths and lockfile pins. The marginal failure rate is much lower than the docker / sonar incidents that drove §4 to exist. Defer to a second §4 pass with its own report — most-exposed files are `monolith-typescript-acceptance-stage-cloud.yml` (~12 inline `npm ci`), `multitier-typescript-acceptance-stage-cloud.yml` (~12 inline `npm ci`), and the 7 commit-stage files (`./gradlew build` × 2, `npm ci` × 3, `dotnet build` × 2 — only the dependency-resolving subset is in scope). (N-B.7, deferred.)

---

## Dependencies between items

Item 20 — ⏳ deferred to follow-up §4 pass.

---

## Acceptance criteria

For each item, the fix is complete when:

1. The matching call site sources the shared engine (`gh-retry.sh` / `docker-retry.sh` / `sonar-retry.sh` / new `git-retry.sh`).
2. The call is wrapped with the matching `<tool>_retry` function.
3. A green CI run exercises the wrapped path at least once (a full meta-prerelease cycle for the tier-1 git items; one commit-stage run for tier-2 docker items; one acceptance-stage run for the sonar items).
4. The audit's matching N-A / N-B / N-C bullet no longer matches when the workflow-auditor agent is re-run against the same scope.

The §4 anti-pattern AP.5 ("broken-wrapper-by-undefined-function") proposed in the audit should be considered for promotion to the `workflow-auditor.md` rubric so future invocations catch the failure mode that produced Item 6.
