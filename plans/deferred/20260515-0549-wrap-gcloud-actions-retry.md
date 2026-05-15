# Plan: wrap google-github-actions/* with retry semantics

Date: 2026-05-15
Source: follow-up from Item 19 of `plans/20260514-fix-shop-workflow-retry-gaps.md`. The Item 19 verification step found that `google-github-actions/auth@v3`, `google-github-actions/setup-gcloud@v3`, and `google-github-actions/deploy-cloudrun@v3` have **NO retry documented** in their upstream READMEs (checked 2026-05-15). Per the parent plan's Item 19 prescription, this opens a new fix item to wrap them.
Scope: 18 `*-stage-cloud.yml` files in `shop/.github/workflows/`, 153 `google-github-actions/*@v3` call sites total.
Status: design — needs decisions before implementation.

---

## Why a separate plan

The parent plan's Item 19 said: *"If they do not [document retry], open a new fix item to wrap with `nick-fields/retry@v4`."* The naive prescription has a gap: `nick-fields/retry@v4` wraps **shell commands** (`run:`), not GitHub Actions (`uses:`). To wrap a `uses:` step with retry, the only widely-used tool is `Wandalen/wretry.action@v3` — which the parent plan's Item 13 explicitly removes from the codebase. So the design has a choice to make before any swap can begin.

## Design options

### Option A — Keep `Wandalen/wretry.action@v3` for the gcloud sites only

After Item 13 lands, `Wandalen/wretry.action@v3` is removed from the docker-login call sites but would reappear at the 153 gcloud sites. This contradicts the spirit of moving to a single canonical retry engine, but it's the only retry wrapper that handles `uses:` directly.

Pros: minimal new code, mechanical swap, no per-action composite to author.
Cons: re-introduces wretry as a dependency right after Item 13 removed it.

### Option B — Author three composite actions (`optivem/actions/gcp-auth`, `optivem/actions/gcp-setup`, `optivem/actions/gcp-deploy-cloudrun`)

Each composite re-implements the gcloud action's behaviour as `run:` shell commands wrapped with a new `gcloud-retry.sh` engine modelled on `docker-retry.sh`. The composites would shell out to `gcloud auth`, `gcloud components install`, and `gcloud run deploy` directly.

Pros: stays on the canonical engine pattern; no third-party retry wrapper.
Cons: large authoring cost; re-implements upstream work; risks divergence from `google-github-actions/*` features (e.g. workload identity, JSON output parsing).

### Option C — Hybrid: small `gcp-call` composite that takes the underlying gcloud invocation as input and runs it under `gcloud-retry.sh`

Replace the three `uses: google-github-actions/*` steps at each site with one or two `run:` steps that source `gcloud-retry.sh` and call `gcloud` directly. Authenticate via `GOOGLE_APPLICATION_CREDENTIALS` env var (which `google-github-actions/auth` ultimately sets anyway).

Pros: smallest engine extension (one new `gcloud-retry.sh`); no per-action composite.
Cons: workload-identity-federation flows for `auth@v3` are non-trivial to re-implement.

## Open questions (require user input before execution)

1. Which option (A / B / C)?
2. If Option A: should the parent plan's Item 13 commit be merged before this plan starts, to keep the wretry-removal cleanly bisectable, or interleaved?
3. If Option B or C: confirm whether shop workflows rely on workload-identity-federation (`workload_identity_provider:` input on `auth@v3`) — if yes, re-implementation must preserve that flow.

## Tier 1 — implementation (gated on option choice)

### Item G-1 — Author chosen retry mechanism

Depending on Option A/B/C, either: (A) confirm `Wandalen/wretry.action@v3` is acceptable for gcloud sites only and document the exception in the engine docs, or (B) author `optivem/actions/gcp-auth`, `gcp-setup`, `gcp-deploy-cloudrun`, or (C) author `actions/shared/gcloud-retry.sh` plus one `optivem/actions/gcp-call` composite.

### Item G-2 — Bulk-swap the 153 sites

Across 18 `*-stage-cloud.yml` files, replace each `uses: google-github-actions/auth@v3`, `setup-gcloud@v3`, `deploy-cloudrun@v3` step with the chosen retry-wrapped form. Representative file: `monolith-dotnet-acceptance-stage-cloud.yml` lines 139, 145, 160, 188, 194, 209, 233, 239, 254 (9 sites — typical density for an acceptance-stage-cloud).

### Item G-3 — Green run

Trigger one acceptance-stage-cloud and one prod-stage-cloud run to confirm the wrapped path executes end-to-end at least once.

---

## Dependencies

```
[parent plan Item 13] (wretry removal from docker-login sites)
  ↘ (only if Option A) Item G-1
                       ↘ Item G-2 → G-3
```

Option B / C have no dependency on Item 13.

---

## Acceptance criteria

1. All 153 `google-github-actions/*@v3` call sites are wrapped with retry semantics consistent with the chosen option.
2. The audit's gcloud-retry gap is closed when `workflow-auditor` is re-run against `shop/.github/workflows/*-stage-cloud.yml`.
3. A successful acceptance-stage-cloud and prod-stage-cloud run exercises the wrapped path.
