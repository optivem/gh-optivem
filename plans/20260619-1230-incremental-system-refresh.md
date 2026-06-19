# Plan: make `system start --restart` incremental — stop full down/up on every GREEN-shape loop ({20260619-1230})

## TL;DR

**Why:** Every ATDD GREEN-shape loop spent ~31s in `gh optivem system start --restart`, which blindly tore down and rebuilt both 4-container stacks from scratch — postgres healthcheck wait and full db-migrate included — even though usually one service changed.
**End result:** `--restart` now keeps postgres running and incrementally recreates only the non-persistent services (`up -d --build --force-recreate --no-deps`), so app/stub/migration changes still take effect but the loop is much faster. Human-typed commands and their meanings are unchanged. **Code is landed (items 1–5); only the live verify (item 6) remains, deferred to the operator.**

## ▶ Next executable step (resume here)

Item 6 (deferred, operator-run): rehearse one full RED→GREEN cycle locally against
the real shop stack and confirm the incremental `--restart` is both **correct** and
**fast**. This needs Docker running and a checked-out shop — not a mechanical edit.
If you want the agent to attempt it, say so; otherwise run it yourself per the
checklist below. Once verified, **delete this plan file** (git history is the record).

## What was implemented (for verify context)

The `opts.Restart` branch of `runner.upOne` (`internal/build/runner/system.go`) no
longer does `down` + `up -d`. It now:

- enumerates the stack's services (`docker compose -f <file> config --services`),
  excluding the hard-coded persistent set `{postgres}` (`persistentServices`);
- if postgres is **already running** → `up -d --build --force-recreate --no-deps
  <non-persistent services>` (incremental: postgres + data volume untouched,
  `--build` picks up app/simulator code, `--force-recreate` re-runs Flyway for new
  migrations and reloads WireMock's mounted stub mappings);
- if postgres is **not running** (cold stack) → falls back to a down-free
  `up -d --build` of the whole stack so postgres + deps start in order; the next
  `--restart` then takes the incremental path.

The non-restart (`opts.Restart == false`) "skip if healthy" path is unchanged.
Help text (`--restart` flag) and the `start-system-restart` process comment were
updated to describe the incremental behavior. The BPMN step set did **not** change,
so no diagram regeneration was needed.

The editing/deleting-an-existing-migration case (rewriting history) still requires
the explicit `gh optivem system clean` (`down -v --rmi local`) — by design it must
not run on every loop.

## Items

- [ ] **Item 6: Verify (deferred — operator-run).** ⏳ Deferred: needs Docker + a
  checked-out shop; heavy, environment-dependent. Rehearse one full RED→GREEN cycle
  locally and confirm each of:
  - (a) an **app source** change takes effect after `start --restart` (no full down/up);
  - (b) a **stub-mapping** change is reloaded after `start --restart`;
  - (c) a **new migration** is applied after `start --restart`.
  Then time the new `--restart` against today's ~31s and record the delta. Confirm a
  multitier stack's built `frontend` service falls into the recreate set (it should —
  it is non-persistent).

## Risks / watch-items (for verify)

- **db-migrate re-run cost**: a no-op Flyway run still pays JVM startup + connect
  (~seconds). Correct and acceptable; a later optimization could skip db-migrate when
  `system/db/migrations` is unchanged — out of scope here.
- **Cold start / postgres not up**: the first `--restart` after a cold machine takes
  the down-free `up -d --build` fallback (brings up postgres + deps); confirm this.
- **`depends_on` under `--no-deps`**: naming every non-postgres service explicitly
  preserves ordering among them; postgres must already be running+healthy between
  loops (it is).
- **Multitier stacks** add a built `frontend` service — confirm it recreates.
