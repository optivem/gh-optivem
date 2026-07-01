# 2026-07-01 05:59:00 UTC — Close the GHCR_TOKEN verification gap in `gh optivem environment verify`

## TL;DR

**Why:** The scheduled `acceptance-stage-legacy` run on `valentinajemuovic/test-app-ea272ffe-cda491f6817cfd4b` (run [28495856291](https://github.com/valentinajemuovic/test-app-ea272ffe-cda491f6817cfd4b/actions/runs/28495856291)) failed at `preflight > Check Container Packages Exist` with "Failed to obtain GHCR registry token ... Token missing or invalid." The action itself (`optivem/actions/check-ghcr-packages-exist@v1`) is correctly failing loud per the no-silent-swallow convention. The real gap: `gh optivem environment verify`'s `verifyGHCRToken` (`internal/config/token_auth.go:264-296`) only checks the token against GitHub's REST API (`api.github.com/user`) and — by explicit design noted in its own comment — never performs the actual `ghcr.io` OCI bearer-token exchange the generated pipelines depend on, and never surfaces a classic PAT's expiration date. So a token that passes scaffold-time verification can still fail in production days later with zero warning, exactly as happened here (secret set 2026-06-27T15:01:11Z, pipeline failing by 2026-07-01).
**End result:** `gh optivem environment verify` performs the same `ghcr.io` OCI token exchange the runtime pipelines use, fails loud (mirroring `check.sh`) when that exchange doesn't return a bearer, and warns the user with the token's expiration date when GitHub reports one — so a bad or soon-to-expire `GHCR_TOKEN` is caught before scaffolding, not discovered later via a failed scheduled pipeline. README guidance is updated to recommend non-expiring PATs for these cron-backed pipelines.

## Outcomes

- `gh optivem environment verify` (and scaffold-time preflight) exercises the real `ghcr.io` OCI token-exchange endpoint for `GHCR_TOKEN`, not just a GitHub REST API proxy for it — so a token that would fail the runtime pipeline's auth flow is caught locally, before scaffolding.
- Classic PATs (`GHCR_TOKEN`, `WORKFLOW_TOKEN`, `REPO_TOKEN` when classic) that carry an expiration date surface that date as a warning during verification, with a louder warning when expiry is within 7 days.
- README.md's token-creation guidance tells users to select "No expiration" (or explicitly accepts the tradeoff) for these classic PATs, since they back cron-scheduled pipelines that run indefinitely.
- `internal/config/token_auth.go`'s test coverage (`token_auth_test.go` and friends) exercises the new OCI-exchange check and the new expiration-header parsing using the existing `fakeRoundTripper` pattern.

## ▶ Next executable step (resume here)

Step 1: In `internal/config/token_auth.go`, add a helper that performs the `ghcr.io` OCI token exchange (`https://ghcr.io/token?service=ghcr.io&scope=repository:{owner}/{repo}:pull` with HTTP Basic auth `x-access-token:{token}`, mirroring `academy/actions/shared/ghcr-probe.sh:38-49`), wire it into `verifyGHCRToken` (line 272 area) so it runs after the existing REST-API scope check, and fail with an actionable error (mirroring `check.sh:36-39`'s wording) when no `.token` comes back. Ground truth for the exact request shape lives in `academy/actions/shared/ghcr-probe.sh` (sibling repo, read-only reference — do not edit it).

## Steps

- [ ] Step 1: Add the real `ghcr.io` OCI token-exchange check to `verifyGHCRToken` in `internal/config/token_auth.go`, matching the flow in `academy/actions/shared/ghcr-probe.sh:38-49`. Fail loud with an actionable message (no silent pass) when the exchange returns no `.token`.
- [ ] Step 2: Add expiration-header surfacing: after any successful classic-PAT check via `githubUserAuthCheck` (`token_auth.go:169`, used by `verifyGHCRToken` and the shared scope-check helper around lines 225-262 for `WORKFLOW_TOKEN`/`REPO_TOKEN`), read the `github-authentication-token-expiration` response header when present and log it as a warning; escalate to a more prominent warning when expiry is within 7 days.
- [ ] Step 3: Extend `token_auth_test.go` (and/or add a new `*_test.go` alongside `verify_environment_token_rejected_test.go`) with cases for: the new OCI-exchange failure path (empty `.token`), the new expiration-header warning (present/absent/near-term), using the existing `fakeRoundTripper` pattern.
- [ ] Step 4: Update `README.md` (~lines 130-135) to recommend "No expiration" for `GHCR_TOKEN` / `WORKFLOW_TOKEN` / `REPO_TOKEN` classic PATs, noting they back cron-scheduled pipelines (e.g. `acceptance-stage-legacy.yml` runs hourly) that will silently start failing once a PAT with an expiration lapses.
- [ ] Step 5: Run `go test ./internal/config/...` and `go build ./...`; fix any failures.
- [ ] Step 6: Manually run `gh optivem environment verify` against a real classic PAT with a known expiration and confirm the new warning prints (best-effort manual check, not automatable in CI).

## Operational callout (outside this plan's scope)

The actual `GHCR_TOKEN` secret on `valentinajemuovic/test-app-ea272ffe-cda491f6817cfd4b` needs to be rotated by the user — `gh secret set GHCR_TOKEN --repo valentinajemuovic/test-app-ea272ffe-cda491f6817cfd4b` — ideally with a non-expiring token, before the scheduled `acceptance-stage-legacy` workflow will go green again. This is a one-off credential fix, not a code change, and isn't tracked as a plan step.

## Open questions

- Should the new OCI-exchange check in `verifyGHCRToken` use a real-but-unpublished scope path (e.g. `{owner}/probe`) since the target package may not exist yet at verify time, or does `ghcr.io`'s token endpoint issue a bearer regardless of whether the scoped repository exists? (Needs a quick manual check against the live endpoint before implementing Step 1 — the token exchange itself, unlike the manifest HEAD request, may not require the repo to exist.)
- Should near-term expiry (Step 2) be a hard failure (blocks scaffolding) or a warning-only? Recommendation: warning-only, since a token expiring in 5 days is still valid today and blocking would be disruptive — but flag this for confirmation during execution.
