# Plan: shop has zero retry-policy scripts — retry consumed via `uses:`

> **Supersedes the A/B/C/D options in
> [`deferred/20260514-2200-retry-helpers-canonical-home.md`](deferred/20260514-2200-retry-helpers-canonical-home.md).**
> That plan asked "where should canonical bash live?" and missed that
> `shop`'s vendored copies are a **student-facing teaching surface**, not
> just an internal sync concern. A student forking shop (or receiving a
> scaffolded per-language repo) inherits whatever is in
> `.github/workflows/scripts/` — including retry helpers — and treats them
> as locally editable. That's the wrong teaching surface for cross-cutting
> policy.

## The principle

**Cross-cutting policy is controlled centrally. Shop-specific operations
stay in shop.**

| Concern | Where it lives | Why |
|---|---|---|
| Retry policy (HTTP 4xx/5xx classification, backoff, idempotency rules) | `optivem/actions/shared/retry.sh` canonical, exposed via `optivem/actions/retry@main` composite | Cross-cutting — uniform across all workspace repos; not shop's concern to define |
| Shop-pipeline operations (find-flavor-rcs, verify-tag-doesnt-exist, create-meta-release-tag, dispatch-sibling-bumpers, …) | shop itself — either inline in meta workflows or as `shop/.github/actions/<op>/` local composites | Specific to multi-language shop; used nowhere else; not scaffolded to students anyway |
| Generic workspace tooling (scaffolding, ATDD rehearsal, test orchestration) | `gh-optivem` CLI | Already its job |

The earlier framing — "shop has zero scripts of any kind" — was over-reach.
The real concern is **policy control**, not script presence.

## The asymmetry: per-language vs meta workflows

Shop's workflows fall into two classes, and the student-control concern
applies to one of them:

| Class | Examples | Scaffolded to students? | Retry pattern |
|---|---|---|---|
| **Per-language** | `monolith-typescript-commit-stage.yml`, `multitier-backend-java-commit-stage.yml`, … (~18 files) | **Yes** — `apply_template.go` copies these into student repos | **Mandatory:** `uses: optivem/actions/retry@main` (no local scripts) |
| **Meta** | `meta-release-stage.yml`, `drift.yml`, `bump-patch-version-multirepo.yml`, `cross-lang-system-verification.yml`, … (~6 files) | **No** — stay in shop only | Same pattern preferred for consistency, but no hard constraint |

**Per-language workflows are the constraint driver.** Meta workflows
benefit from the same pattern for cleanliness, not for student control.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│ Layer 3 — Workflows (shop & student forks)                          │
│   Reads as a sequence of named operations                           │
└──────────────────────────┬──────────────────────────────────────────┘
                           │ uses: ./.github/actions/<op>     (shop local)
                           │ uses: optivem/actions/retry@main (Layer 1)
┌──────────────────────────▼──────────────────────────────────────────┐
│ Layer 2 — Shop's local pipeline operations (shop-only)              │
│   shop/.github/actions/find-flavor-rcs/                             │
│   shop/.github/actions/verify-tag-does-not-exist/                   │
│   shop/.github/actions/create-meta-release-tag/                     │
│   ... only as needed; meta-workflow ops can also stay inline        │
│                                                                      │
│   Each one's substantive logic lives locally; any retry calls go    │
│   through Layer 1 via `uses: optivem/actions/retry@main`            │
└──────────────────────────┬──────────────────────────────────────────┘
                           │ uses: optivem/actions/retry@main
┌──────────────────────────▼──────────────────────────────────────────┐
│ Layer 1 — Cross-cutting retry policy (centralized control)          │
│   optivem/actions/retry/action.yml                                  │
│   optivem/actions/shared/retry.sh    (unified bash helper)          │
│                                                                      │
│   Composite — takes `command:` + `working-directory:` inputs.       │
│   Single unified retry policy: 4xx fail-fast, 5xx + network-blip    │
│   retry. No per-tool `policy:` parameter — regex covers all tools.  │
└─────────────────────────────────────────────────────────────────────┘
```

## Layer 1 — `optivem/actions/retry@main`

```yaml
# optivem/actions/retry/action.yml
name: 'Run command with retry'
description: |
  Wrap a shell command in retry semantics. 4 attempts with 5s/15s/45s
  backoff. Retries on HTTP 5xx, network blips, and known transient
  phrases across gh/docker/sonar/git tools. Fails fast on HTTP 4xx and
  known hard-fail patterns (manifest unknown, project not found, auth
  errors).
inputs:
  command:
    description: 'Shell command to run with retry'
    required: true
  working-directory:
    description: 'Directory to cd into before running command'
    required: false
    default: '.'
runs:
  using: composite
  steps:
    - shell: bash
      env:
        CMD: ${{ inputs.command }}
        WD: ${{ inputs.working-directory }}
      run: |
        set -euo pipefail
        source "$GITHUB_ACTION_PATH/../shared/retry.sh"
        cd "$WD"
        # shellcheck disable=SC2086
        eval "retry_with_policy $CMD"
```

### Input shape — Wandalen-inspired, forward-compatible

| Input | Notes |
|---|---|
| `command:` | The shell command to retry. Required. |
| `working-directory:` | Optional cd target. Defaults to repo root. |

Future evolution: if this composite is ever replaced by a JavaScript
action (to support `uses:`-style action wrapping like Wandalen), the name
stays the same and `command:` continues to work. A new `action:` input
would be added without breaking existing call-sites.

### Why the name is `retry` and not `retry-cmd`

Forward-compatibility. If a future JS-action variant ships supporting
both `command:` and `action:`, the existing `retry@main` reference doesn't
need to change. Matches Wandalen's single-name convention
(`Wandalen/wretry.action@v3`).

### Wandalen retained for `uses:`-style retry

Composite actions cannot dynamically dispatch on `uses:` based on an input
(GitHub Actions doesn't resolve `uses: ${{ inputs.action }}`). So:

- **`optivem/actions/retry@main` (our composite)** handles all `run:`
  shell command retry. Domain-aware 4xx/5xx classification is the
  value-add.
- **`Wandalen/wretry.action@v3` (third-party, retained)** handles
  `uses:` action retry. Generic retry semantics. Our 4xx/5xx
  classification doesn't apply here anyway because intercepting another
  action's stderr is brittle.

This split **revises** the premise of the deferred plan
[`deferred/20260515-0549-wrap-gcloud-actions-retry.md`](deferred/20260515-0549-wrap-gcloud-actions-retry.md),
which assumed Wandalen would be removed entirely. See §"Follow-ups".

### Single unified retry policy — no `policy:` parameter

The four current bash helpers (`gh-retry.sh`, `docker-retry.sh`,
`sonar-retry.sh`, `git-retry.sh`) differ in **specific phrasings**, not
in **concepts**. All four encode:
- Transient: HTTP 5xx + network blips (timeout, connection reset,
  TLS, DNS) + tool-specific phrasings of those same categories
- Hard-fail: HTTP 4xx + tool-specific auth/config error messages

A unified regex is the **union** of all four — strictly broader, no
collisions (e.g. sonar output never contains `manifest unknown`, docker
output never contains `Project key does not exist`). Verified for docker
vs sonar; gh and git still to diff (see §"Open verifications").

End state: **one** `actions/shared/retry.sh`, **one** Go port in
`gh-optivem/internal/shell/retry.go`. Drift surface drops from
"2 ports × 4 policies = 8" to "2 ports × 1 policy = 2".

## Layer 2 — Shop's local pipeline operations

Substantive shop-specific operations that benefit from being named can
live as local composites in `shop/.github/actions/<op>/`. These are
shop-only (never scaffolded), so students never see them.

**Examples that warrant Layer 2 extraction:**
- `find-flavor-rcs` — parses a JSON manifest embedded in a tag annotation
- `verify-tag-does-not-exist` — probes with custom 404 classification
- `create-meta-release-tag` — git push + idempotency-aware gh release create

**Examples that should stay inline** (no substantive logic to hide):
- One-liner `gh extension install optivem/gh-optivem` → just calls
  `optivem/actions/retry@main` directly in the workflow

Judgment per call-site. Default to inline; extract only when there's
real logic worth naming.

## Class 1 — Per-language workflow code samples (mandatory pattern)

### Before — `monolith-typescript-commit-stage.yml` (today)

```yaml
- name: Run sonarscanner
  env:
    SONAR_TOKEN: ${{ secrets.SONAR_TOKEN }}
  run: |
    set -euo pipefail
    source "$GITHUB_WORKSPACE/.github/workflows/scripts/sonar-retry.sh"
    cd system/monolith/typescript
    sonar_retry bash ./run-sonar.sh

- name: Pre-pull base images
  shell: bash
  run: |
    set -euo pipefail
    source "$GITHUB_WORKSPACE/.github/workflows/scripts/docker-retry.sh"
    for image in node:22-alpine; do
      docker_retry pull "$image"
    done
```

### After — `optivem/actions/retry@main` used directly

```yaml
- name: Run sonarscanner
  uses: optivem/actions/retry@main
  env:
    SONAR_TOKEN: ${{ secrets.SONAR_TOKEN }}
  with:
    working-directory: system/monolith/typescript
    command: bash ./run-sonar.sh

- name: Pre-pull base images
  uses: optivem/actions/retry@main
  with:
    command: docker pull node:22-alpine
```

No `source` lines. No `.github/workflows/scripts/` directory in
scaffolded student repos. Retry policy is pulled cross-repo from
`optivem/actions/` at runtime.

## Class 2 — Meta workflow code samples (preferred but not mandatory)

### Substantive operation → extract to Layer 2

**Before** — inline in `meta-release-stage.yml`:

```yaml
- name: Find flavor RCs to promote
  id: flavor-rcs
  env:
    GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
    RC_TAG: ${{ steps.rc.outputs.rc_tag }}
  run: |
    set -euo pipefail
    source "$GITHUB_WORKSPACE/.github/workflows/scripts/gh-retry.sh"
    tag_object=$(git cat-file -p "${RC_TAG}")
    # ... 20 lines of JSON manifest parsing + output setting ...
```

**After** — local Layer 2 composite:

```yaml
- name: Find flavor RCs to promote
  id: flavor-rcs
  uses: ./.github/actions/find-flavor-rcs
  with:
    rc-tag: ${{ steps.rc.outputs.rc_tag }}
  env:
    GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

The local composite at `shop/.github/actions/find-flavor-rcs/action.yml`
owns the parsing logic; any gh calls inside it go through
`uses: optivem/actions/retry@main`.

### Simple one-liner → stay inline, just call Layer 1

**Before**:
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

**After**:
```yaml
- name: Install gh-optivem CLI extension
  uses: optivem/actions/retry@main
  env:
    GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
  with:
    command: gh extension install optivem/gh-optivem
```

## What disappears from shop

```diff
 shop/.github/workflows/scripts/
-    retry-core.sh
-    gh-retry.sh
-    docker-retry.sh
-    sonar-retry.sh
-    git-retry.sh
```

The entire `shop/.github/workflows/scripts/` directory is deleted. No
retry-policy code remains anywhere in shop.

## What disappears from `actions/shared/`

```diff
 optivem/actions/shared/
-    gh-retry.sh
-    docker-retry.sh
-    sonar-retry.sh
-    git-retry.sh
-    _test-gh-retry.sh
-    _test-docker-retry.sh
-    _test-sonar-retry.sh
-    _test-git-retry.sh
+    retry.sh
+    _test-retry.sh
     retry-core.sh    # engine, unchanged
```

Four tool-specific helpers collapse into one unified helper.

## What disappears from gh-optivem

```diff
 gh-optivem/internal/shell/
-    ghretry.go
-    ghretry_test.go
-    sonarretry.go
+    retry.go
+    retry_test.go
     retrycore.go    # engine, unchanged
```

gh-optivem's `.github/scripts/` keeps its vendored copies for now —
internal tool, not student-facing, separate concern.

## Execution plan

**Approach:** Q3b — incremental per workflow. Don't attempt all 24
workflows at once.

### Step 1 — Add `optivem/actions/retry@main`

1. Create `optivem/actions/retry/action.yml` (composite, shape above).
2. Create `optivem/actions/shared/retry.sh` (unified helper, regex from
   union of the four current helpers).
3. Smoke-test against a known-transient mock and a known-hard-fail mock.

### Step 2 — Migrate per-language workflows (one at a time)

For each of the ~18 per-language workflows in shop:

1. Replace each `source ... /*-retry.sh` + `*_retry CMD` pattern with
   `uses: optivem/actions/retry@main` + `with: { command: CMD }`.
2. Commit per workflow.
3. After all per-language workflows migrate, verify a scaffolded student
   repo has no broken references.

### Step 3 — Migrate meta workflows (preferred, not mandatory)

For each of the ~6 meta workflows:

1. Same pattern as Step 2 for simple one-liners.
2. For substantive operations (find-flavor-rcs, etc.), extract to
   `shop/.github/actions/<op>/` and call Layer 1 from inside.

### Step 4 — Delete shop's vendored helpers

Once no shop workflow references `.github/workflows/scripts/*-retry.sh`,
delete:
- `shop/.github/workflows/scripts/retry-core.sh`
- `shop/.github/workflows/scripts/gh-retry.sh`
- `shop/.github/workflows/scripts/docker-retry.sh`
- `shop/.github/workflows/scripts/sonar-retry.sh`
- `shop/.github/workflows/scripts/git-retry.sh`

Verify with `grep -r "workflows/scripts/.*-retry" shop/` returning zero
matches.

### Step 5 — Update sync direction

`optivem/actions/scripts/sync-shared.sh`:
- Remove `shop/.github/workflows/scripts` from TARGETS list
- Replace the four `*-retry.sh` entries with one `retry.sh`

Keep gh-optivem in TARGETS (internal tool, separate concern).

### Step 6 — Collapse `actions/shared/` helpers

After all consumers (composite + gh-optivem vendored copies + Go port)
point at the unified helper:
- Delete `actions/shared/{gh,docker,sonar,git}-retry.sh`
- Delete corresponding `_test-*.sh`
- Add `_test-retry.sh` for the unified helper

### Step 7 — Collapse Go port

- Replace `gh-optivem/internal/shell/{ghretry,sonarretry}.go` with one
  `retry.go`
- Update callers
- Tests follow

## Follow-ups (separate plans)

### Amend the deferred gcloud-actions plan

[`deferred/20260515-0549-wrap-gcloud-actions-retry.md`](deferred/20260515-0549-wrap-gcloud-actions-retry.md)
currently assumes Wandalen will be removed from the codebase entirely.
That premise needs revising to reflect today's decision:

> Wandalen is removed for **shell command retry** (replaced by
> `optivem/actions/retry@main`). Wandalen is **retained for action-wrapping
> retry** because (a) composite actions can't dynamically dispatch on
> `uses:`, (b) generic exit-code retry is the best available behaviour
> at the action layer anyway, and (c) building a JS action mirroring
> Wandalen with our classification regex would be a much larger
> investment with limited payoff.

A small header amendment to that deferred plan would prevent anyone
picking it up later from acting on the obsolete premise.

### Future: JS-action variant of `optivem/actions/retry`

If at some point the workspace genuinely needs the `uses:`-wrapping
behaviour with our own classification (rather than Wandalen's generic),
the composite can be upgraded to a JS action without renaming —
existing `command:`-style call-sites stay working; new `action:`-style
becomes available. Not in scope here.

## Out of scope

- Moving canonical bash from `actions/shared/` to anywhere else. Stays
  in `actions/`.
- gh-optivem's own `.github/scripts/` vendored copies. Keep as-is —
  internal tool plumbing, not student-facing.
- The TypeScript port deferred by the 1945 plan — still deferred.
- Anything outside the shop/actions/gh-optivem triangle.

## Status

- Architecture: decided
- Q1 (composite shape): single generic `retry@main`, unified regex
- Q2 (discovery): migrate as we go
- Q3 (migration order): incremental per workflow
- Q4 (teaching visibility): moot — retry never appears as a named concept
  in workflows; just `uses: optivem/actions/retry@main`
- Wandalen split: keep for `uses:` retry, remove for `run:` retry
