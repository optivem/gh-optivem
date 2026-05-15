# Plan: evaluate moving the canonical retry helpers into `gh-optivem`

> ⚠️ **Status (2026-05-15): decision still open — no work executed.**
> Recommendation in §"Recommendation" is **Option A (status quo, no work)**.
> This plan has not been actioned and should not be picked up by an agent
> without explicit human approval of one of the four options.

> ⚠️ **Requires human decision before any execution.** This plan does not
> pick a direction on its own. It lays out four options (A/B/C/D), gives a
> recommendation with reasoning, and lists the triggers that should flip the
> call. A human must read §"Options" + §"Recommendation" and explicitly
> approve **Option A** (status quo, no work) or **Option B** (move
> canonical, execute the §"If Option B is chosen" sketch) — or choose C/D —
> before any files are moved. Do not let an agent execute this plan
> autonomously.

> **Type: proposal / evaluation.** This plan asks a question — *where should
> `retry-core.sh`, `gh-retry.sh`, `docker-retry.sh`, `sonar-retry.sh` live?* —
> and lays out the options with tradeoffs and a recommendation. It does **not**
> commit to executing the move. If accepted, it would amend the sync direction
> of Phase 1 in [`20260514-1945-retry-mechanism-end-to-end.md`](20260514-1945-retry-mechanism-end-to-end.md).

## Question

The 1945 end-to-end plan picks `optivem/actions/shared/` as the canonical
home for the four bash retry helpers, with vendored copies synced into
`shop/.github/workflows/scripts/` and `gh-optivem/.github/scripts/`. Should
the canonical home instead be **`gh-optivem`** (with `actions` and `shop`
receiving vendored copies)?

The trigger for the question: `gh-optivem` is the only workspace repo that
needs *both* a bash port and a Go port of the same retry engine (the Go
side already lives at `internal/shell/retrycore.go` + `ghretry.go`). Keeping
the two ports in the same repo would make policy edits a single-repo PR
instead of a cross-repo one.

## Current state (2026-05-14)

### Canonical source

- `optivem/actions/shared/retry-core.sh` — generic engine (~80 LOC)
- `optivem/actions/shared/gh-retry.sh` — gh wrapper (~15 LOC)
- `optivem/actions/shared/docker-retry.sh` — docker wrapper (~15 LOC)
- `optivem/actions/shared/sonar-retry.sh` — sonarscanner wrapper (~15 LOC)
- `optivem/actions/shared/_test-*.sh` — smoke tests for each
- `optivem/actions/scripts/sync-shared.sh` — vendoring script

### Consumers

1. **`optivem/actions/*/` composite-action shell scripts** — ~15 files
   `source "$GITHUB_ACTION_PATH/../shared/gh-retry.sh"` (and similarly for
   `docker-retry.sh`). Path is **relative within the actions repo** and only
   resolves when the composite is consumed via `uses: optivem/actions/...`
   at workflow runtime. Examples:
   - `commit-files/commit.sh`
   - `create-commit-status/create.sh`
   - `wait-for-workflow/wait.sh`
   - `cleanup-prereleases/cleanup-prereleases.sh`
   - `cleanup-deployments/cleanup-deployments.sh`
   - `cleanup-ghcr-orphan-manifests/cleanup-ghcr-orphan-manifests.sh`
   - `deploy-docker-compose/start.sh`
   - `resolve-docker-image-digests/resolve-docker-image-digests.sh`
   - `resolve-project-status-field/resolve-project-status-field.sh`
   - `resolve-latest-deployed-prerelease/resolve.sh`
   - `resolve-latest-prerelease-with-status/resolve.sh`
   - `check-commit-status-exists/check.sh`
   - `get-commit-status/read.sh`
   - `get-last-workflow-run/get.sh`
   - `create-deployment/create.sh`
   - `bulk-update-project-item-status/bulk-update-project-item-status.sh`
2. **`optivem/shop/.github/workflows/scripts/`** — vendored copies, sourced
   from `run:` blocks via `$GITHUB_WORKSPACE/.github/workflows/scripts/...`.
3. **`optivem/gh-optivem/.github/scripts/`** — vendored copies for the
   acceptance-stage workflow.
4. **`optivem/gh-optivem/internal/shell/retrycore.go`** + `ghretry.go` — Go
   port. Same policy (4 attempts, 5s/15s/45s backoff, hard-fail pass-through,
   transient regex) re-implemented in Go for the CLI binary.

### Why `actions/shared/` is canonical today

The four scripts originated alongside the composite actions that consume
them. The composite actions are the **highest-fanout** consumer surface (15
files across the actions repo, used by ~88 workflow files in `shop`). They
source the helpers directly from the actions repo's own working tree at
runtime via `$GITHUB_ACTION_PATH/../shared/...`, which only works if the
canonical copy lives **inside** the actions repo. So today there is one
"native" consumer (actions composites) and two "vendored" consumers (shop
workflow scripts and gh-optivem workflow scripts), with the Go port as a
parallel codebase.

## Options

### Option A — Status quo: keep canonical in `optivem/actions/shared/`

The 1945 plan as written.

**Pros:**
- 15 composite-action scripts in `actions/` already source the helpers via
  the in-repo relative path. **Zero churn** on the highest-fanout consumer
  surface.
- Conceptually, the helpers were born to support CI composites; living next
  to those composites is co-locating "library + first caller".
- `actions/` is already the published-via-`uses:` surface — vendoring out
  to `shop` and `gh-optivem` is one-directional.

**Cons:**
- Retry policy changes require edits to *two* repos: bash in `actions/` +
  Go in `gh-optivem`. Two PRs, two reviewers, two merge events.
- The Go port is a parallel codebase that has to be kept in semantic sync
  with the bash port by code review, not by tooling.
- A future Go-side regex update can land without the bash side and silently
  diverge (or vice versa) — no shared file forces lockstep.

### Option B — Move canonical to `optivem/gh-optivem`

Make `gh-optivem` the canonical home for both bash and Go retry ports. Sync
to `actions` and `shop`.

**End-state layout:**
| Repo | Path | Role |
|---|---|---|
| `gh-optivem` | `internal/shell/retrycore.go`, `ghretry.go`, etc. | Canonical Go port (already here) |
| `gh-optivem` | `shared/retry/retry-core.sh`, `gh-retry.sh`, `docker-retry.sh`, `sonar-retry.sh` | **NEW** canonical bash port |
| `gh-optivem` | `shared/retry/_test-*.sh` | **MOVED** smoke tests |
| `gh-optivem` | `scripts/sync-shared.sh` | **MOVED** vendoring script (reverses direction) |
| `gh-optivem` | `.github/scripts/{retry-core,gh-retry,docker-retry,sonar-retry}.sh` | Vendored copy for own workflows |
| `actions` | `shared/{retry-core,gh-retry,docker-retry,sonar-retry}.sh` | **Vendored** from gh-optivem |
| `shop` | `.github/workflows/scripts/{retry-core,gh-retry,docker-retry,sonar-retry}.sh` | Vendored from gh-optivem |

**Pros:**
- **Bash port and Go port live in the same repo.** A retry policy change
  (e.g. new transient pattern, new hard-fail pattern, backoff tweak) becomes
  one PR touching both `retrycore.go` and `shared/retry/retry-core.sh`.
- Single test surface — `go test ./internal/shell/...` for the Go side and
  `bash shared/retry/_test-*.sh` for the bash side, both runnable from a
  single repo checkout.
- The 1945 plan's "Recommended order of fixes" already calls for adding a
  `workflow-auditor.md` rubric section on retry coverage *inside gh-optivem*
  — the rubric and the helpers it audits would then live in the same repo.

**Cons:**
- **Significant churn on `optivem/actions/`.** The 15 composite scripts
  today do `source "$GITHUB_ACTION_PATH/../shared/gh-retry.sh"` — that
  relative path resolves only because `shared/` is colocated with the
  composites at runtime. Two sub-options:
  - **B1**: Vendor into `actions/shared/` and leave the composites' source
    paths untouched. Vendored files just appear in `actions/` instead of
    being authored there. Composite scripts don't change at all.
  - **B2**: Repoint composite scripts at the vendored gh-optivem copy via
    some other path. Doesn't work cleanly — `gh-optivem` isn't checked out
    in the runner when `optivem/actions/foo@main` is used.
  B1 is the only sane variant — vendor into `actions/shared/` keeps every
  existing `source` path working.
- **Sync direction inverts.** Today: `actions → {shop, gh-optivem}`. After:
  `gh-optivem → {actions, shop}`. Three vendoring targets instead of two,
  one of which is the repo that *used to be* canonical. Requires moving
  `sync-shared.sh` from `actions/scripts/` to `gh-optivem/scripts/` and
  adding `actions/shared/` to its target list.
- **Cognitive load on `actions/`.** Anyone editing a composite that uses
  retry will see `actions/shared/<helper>.sh` and naturally edit it — but
  that file is now a vendored copy. The banner says "GENERATED — DO NOT
  EDIT" with a `Source: gh-optivem/shared/retry/<helper>.sh` pointer.
  Workable, but a footgun.
- **Tests and CI live across repos.** The bash smoke tests
  (`_test-retry-core.sh`, etc.) would move out of `actions/`. Any
  actions-side lint job that runs them (`lint-gh-usage.yml`) would need to
  either be moved or shelled into the vendored copy. The 1945 plan's
  Phase 1 names canonical-side tests as gating; those gates would now run
  in gh-optivem CI, not actions CI.
- **Discoverability cost.** The helpers' "first caller" pattern is in
  `actions/` composites. A reader who finds `gh_retry` invoked in
  `commit-files/commit.sh` and follows the `source` line will find a
  vendored file with a "look elsewhere" banner instead of the canonical
  source. One extra hop.

### Option C — Hybrid: keep bash canonical in `actions/`, treat Go as authoritative for policy decisions, and add a CI lint that cross-checks

Keep the bash canonical where it is (Option A's layout) but add a CI lint
in `gh-optivem` that diff-checks the policy constants between the Go side
and the vendored bash copy — failing the build if they drift.

**Pros:**
- No file moves. Zero churn on actions composites.
- The drift problem (Cons #1 of Option A) becomes a build-time alarm rather
  than a coordination cost.

**Cons:**
- Adds CI tooling that didn't exist before — a regex-extracting comparator
  that knows how to parse both Go consts and bash variable assignments and
  decide they encode the "same" policy. Non-trivial; failure modes
  (false-positives on syntactic but-not-semantic differences) are
  annoying.
- Doesn't address the "two PRs to change policy" cost — only flags it.

### Option D — Move only the Go port to live next to wherever bash canonical is

The mirror of Option B: put both ports in `optivem/actions/`. Bash stays
where it is; Go moves to a new `actions/lib/retry/` directory.

**Rejected on a sniff test.** `actions/` is a composite-actions repo, not
a Go library repo. `gh-optivem`'s Go binary genuinely needs the retry code
to compile in — vendoring Go source from `actions/` into `gh-optivem` at
build time is doable (go modules can point at a sibling repo) but adds a
module dependency and a release-coordination burden that the current setup
deliberately avoids. The 1945 plan kept the Go port in `gh-optivem` for
exactly this reason. Not pursuing.

## Recommendation

**Option A (status quo).** Reasoning:

1. **The highest-fanout consumer surface — 15 composite scripts in
   `actions/` — sources the helpers via an in-repo relative path.** Option
   B's sub-variant B1 (vendor into `actions/shared/` so the path keeps
   working) does make the change non-disruptive at runtime, but the
   reader-experience footgun ("looks like canonical, actually generated")
   is real. It's the kind of trap that surfaces a year later when someone
   "fixes a typo in `gh-retry.sh`" inside `actions/` and the fix evaporates
   on the next sync.
2. **The actual pain Option B solves — bash/Go lockstep edits — is
   moderate, not severe.** The 1945 plan's `_test-gh-retry.sh` and the Go
   `ghretry_test.go` are independent test suites that both must pass when
   policy changes; a developer changing policy already has to land changes
   in both files. The repo boundary doesn't add much beyond a second PR.
3. **The Go port's regex constants are unlikely to drift far.** Both the
   bash and Go sides encode the same conceptual rules (`HTTP 5xx`,
   `timeout`, `connection reset`, etc.). A lightweight cross-check (Option
   C) could pick off the residual drift risk without a repo move.

**Conditions that would flip the recommendation toward Option B:**

- A future requirement to share **non-retry** library code across
  bash+Go+composite-action consumers (e.g. a shared error-classification
  helper). At that point the question becomes "where does shared library
  code live for the workspace?" and `gh-optivem` is the natural answer.
- Observable drift between bash and Go retry policies that causes a CI
  incident — i.e. Option C's gap gets exercised. The right response then
  is the colocation in Option B, not a more elaborate lint in Option C.
- A decision to retire `optivem/actions/` as a separate repo (e.g. fold
  composites into `gh-optivem` itself). The "highest-fanout consumer is
  inside actions" objection vanishes.

## If Option B is chosen — execution sketch

Not committed; sketch only, for reviewers comparing cost.

1. **Move canonical files.**
   - `actions/shared/retry-core.sh` → `gh-optivem/shared/retry/retry-core.sh`
     (and matching `gh-retry`, `docker-retry`, `sonar-retry`).
   - `actions/shared/_test-*.sh` → `gh-optivem/shared/retry/_test-*.sh`.
2. **Move and repoint sync script.**
   - `actions/scripts/sync-shared.sh` → `gh-optivem/scripts/sync-shared.sh`.
   - Update its `TARGETS` list to: `../actions/shared`,
     `../shop/.github/workflows/scripts`, `./.github/scripts` (gh-optivem's
     own workflow copy — needed because `.github/scripts/` and
     `shared/retry/` are different paths for different consumers).
3. **Replace `actions/shared/<helper>.sh` with vendored copies** carrying
   the `GENERATED — Source: gh-optivem/shared/retry/<helper>.sh @ <sha>`
   banner. Composite-script `source` paths inside `actions/` stay byte-
   identical — the vendored file is at the same location it always was.
4. **Move CI gating.** Any bash-test job in `actions/` that runs
   `_test-*-retry.sh` against the canonical source moves to `gh-optivem`'s
   CI. The actions-side workflow that runs only against vendored copies
   becomes a smoke test ("vendored copy is syntactically valid + sources
   retry-core") rather than the policy test.
5. **Cross-repo push order on the cutover commit:** gh-optivem (introduces
   canonical) → actions (replaces files with vendored copies) → shop
   (re-vendored if the sync ran). Each repo committed via `/commit`.
6. **Update `20260514-1945-retry-mechanism-end-to-end.md`** end-state
   architecture table and the "Workspace files at end-state" section to
   reflect the new layout; supersede the old Phase 1 sync direction.

Estimated cost: 1 PR per repo (3 PRs), ~half a day end-to-end including
test execution. No runtime behaviour change. Reversible (sync script can
flip direction again later if the call turns out to be wrong).

## Out of scope

- Moving the Go port (`internal/shell/retrycore.go`, `ghretry.go`) — it
  stays in `gh-optivem` in all options.
- Anything in `optivem/shop` beyond receiving the vendored copies. Shop
  doesn't author retry policy in any option.
- The TypeScript port deferred by the 1945 plan — still deferred. If it
  lands, it follows whichever canonical home this plan settles on.

## Decision

**Open.** Pending review against the recommendation above. If you'd like
to flip to Option B I'll execute the sketch in §"If Option B is chosen";
if you accept the recommendation this plan can be archived as "evaluated,
status quo retained" with the decision logged near the 1945 plan's
architecture section.
