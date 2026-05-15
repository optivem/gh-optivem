# Plan: coordinate parallel execution of retry Phase 6 (shop + gh-optivem)

## Status (2026-05-15)

| Lane | Plan | Agent | State |
|---|---|---|---|
| A — shop workflow retry gaps | [`20260514-fix-shop-workflow-retry-gaps.md`](20260514-fix-shop-workflow-retry-gaps.md) | `Valentina_Desk`, picked up 2026-05-15T05:45:21Z | in progress |
| B — gh-optivem Go retry gaps | [`20260514-fix-gh-optivem-retry-gaps.md`](20260514-fix-gh-optivem-retry-gaps.md) | pending pickup | not started |

Both lanes run on this machine, both commit directly to `main`, no separate worktrees.

## Scope

This file governs **how** the two lanes run side by side, not **what** each lane does. Each lane's items live in its own plan. This coordination plan is disposable — delete when both lanes finish.

## Lane file ownership (no overlap expected)

| Repo / path | Lane A writes | Lane B writes |
|---|---|---|
| `gh-optivem/plans/20260514-fix-shop-workflow-retry-gaps.md` | ✅ | — |
| `gh-optivem/plans/20260514-fix-gh-optivem-retry-gaps.md` | — | ✅ |
| `gh-optivem/internal/**/*.go`, `gh-optivem/main.go` | — | ✅ |
| `gh-optivem/audits/20260514-external-call-retry-coverage.md` (closing footer) | — | ✅ when Lane B done |
| `gh-optivem/audits/20260514-shop-workflow-retry-coverage.md` (closing footer) | ✅ when Lane A done | — |
| `shop/.github/workflows/*.yml` | ✅ | — |
| `optivem/actions/docker-login@v1` (new composite, Item 12) | ✅ | — |
| `optivem/actions/shared/*.sh` (canonical bash helpers) | — | — |
| `gh-optivem/.github/scripts/*.sh` (vendored helpers) | — | — |

Canonical and vendored bash helpers are stable for the duration; neither lane edits them.

## Coordination rules

### Pickup markers
Each lane stamps `🤖 Picked up by agent — <machine> at <ISO>` at the top of its plan file before starting work. Drop the marker when pausing for the day or marking the plan done. Lane A already has its marker; Lane B must add one when it starts.

### Git on a shared working tree
Both agents commit directly to `main` in the same `gh-optivem` checkout. Before each commit:

1. `git pull --rebase` to absorb the other lane's commits.
2. Commit the lane's change-set (specific files, never `-A`).
3. `git push` immediately so the other lane can pull.

File-level conflicts are unlikely given the ownership table above. If one occurs, resolve in the editing lane's favor — the other lane wasn't supposed to touch that file.

### If the shared retry policy needs to change mid-flight

If either lane discovers a new transient pattern that the engine doesn't yet catch:

1. **Pause the lane** — do not land the regex change inside an A/B item.
2. Edit canonical `optivem/actions/shared/<helper>.sh`.
3. Run `optivem/actions/scripts/sync-shared.sh` to vendor into `shop/.github/workflows/scripts/` and `gh-optivem/.github/scripts/`.
4. Mirror the regex change in `gh-optivem/internal/shell/ghretry.go` (or the appropriate Go wrapper). **Land this in the same session as step 3** so bash and Go ports don't drift.
5. Commit per repo (actions → gh-optivem → shop), push each.
6. Resume the paused lane.

### Done conditions

- **Lane A done** when every Tier 1–4 item in the shop plan has shipped or is explicitly marked deferred, and the shop-side audit (`audits/20260514-shop-workflow-retry-coverage.md`) gets a closing footer noting which categories cleared.
- **Lane B done** when all 10 Go items have shipped, `go test ./...` is green, and the gh-optivem audit (`audits/20260514-external-call-retry-coverage.md`) gets a closing footer mirroring the `audits/20260514-silent-external-call-failures.md` "H1–H5 fixed" pattern.
- **This coordination plan deleted** once both lanes are done.

## Out of scope (do not pick up while parallel work runs)

- [`20260514-2200-retry-helpers-canonical-home.md`](20260514-2200-retry-helpers-canonical-home.md) — decision-pending proposal. Executing it would invert the bash-helpers sync direction, which would conflict with both lanes' assumptions about where canonical helpers live. Defer until both A and B are done, then revisit with a human decision.
- [`20260514-1945-retry-mechanism-end-to-end.md`](20260514-1945-retry-mechanism-end-to-end.md) — status/reference doc only, no work prescribed.

## Risks

- **Concurrent push to `main` race.** Two agents push within seconds of each other; one push gets rejected by remote. Mitigation: the rebase-before-commit step above; if rejected, `git pull --rebase && git push` is the recovery.
- **Plan-file edits collide.** Each lane updates its own plan file with item-completion notes. If both edit at exactly the same line range (highly unlikely given they're separate files), resolve in the editing lane's favor.
- **Audit re-issue stomping.** If both lanes finish and both try to add a closing footer to the *same* audit file, the table above prevents that — each audit has a single owner.
- **A regex change lands in Lane B's Go port but not in canonical bash** (or vice versa). Mitigation: the "shared retry policy change" procedure above is the only sanctioned path; do not let either lane modify the regex inside an item swap.
