# Persist run artifacts beyond rehearsal-worktree teardown

## Motivation

Every ATDD run writes its analytics artifacts under the **root of the repo it
executed in** — `<repoPath>/.gh-optivem/runs/<timestamp>/`. For a normal
`gh optivem implement` against a real repo that is durable. But **rehearsal
runs execute inside a throwaway git worktree** (`<workspace>/worktrees/rehearsal-<...>/`)
created and later pruned by the rehearsal harness. When that worktree is
removed, its `.gh-optivem/runs/` goes with it — so the machine-readable
analytics surfaces (`steps.jsonl`, `summary.jsonl`) and the human digest
(`summary.md`, `flow.txt`) are **lost the moment the rehearsal is cleaned up.**

Concretely: the rehearsal digest at the top of the conversation that motivated
this plan —
`worktrees/rehearsal-20260617-084210-72-charge-shipping-based-on-product-weight/.gh-optivem/runs/20260617-064227/summary.md`
— **no longer exists on disk**; the worktree was already pruned. A scan of the
surviving worktrees shows only a handful of run dirs, all inside worktrees that
happen not to have been cleaned yet. There is no run history that outlives the
worktrees.

This is fine for one-off debugging (look at the run, then throw it away) but it
defeats **analytics over time**: cost-per-run trends, effort-tuning experiments
(e.g. the `system-implementer` `high`→`medium` trial), agent-vs-command
wall-clock splits, per-channel timing — none of it can be aggregated across
runs because the data does not survive the run.

## Current behaviour (verified against code)

- The per-run artifact dir is rooted at `repoPath`:
  - `runDir := filepath.Join(repoPath, ".gh-optivem", "runs", runState.runTimestamp)` — `internal/atdd/runtime/driver/driver.go:418`
  - step sidecar: `stepsPath = filepath.Join(rs.repoPath, ".gh-optivem", "runs", rs.runTimestamp, "steps.jsonl")` — `step_summary.go` (`stepsPath`)
  - agent sidecar `summary.jsonl` and digest `summary.md` written to the same dir.
- For rehearsals, `repoPath` **is the transient worktree**, so all of the above
  land inside it.
- In-repo pruning already exists but is scoped to the same repo dir:
  `pruneOldRuns(filepath.Join(repoPath, ".gh-optivem", "runs"), opts.KeepRuns)` —
  `driver.go:482`, governed by `--keep-runs` (default 10, `implement_commands.go:214`).
  It does nothing to help durability — it only caps how many run dirs accumulate
  *within* a repo.
- A **durable, worktree-independent location already exists and is already used**:
  the gh-optivem user-state dir, `<userstate.Dir()>/runs/<runTimestamp>-<pid>/`,
  via `resolvePidRunDir` — `driver.go:1800-1806`. Today it holds only PID marker
  files, not the analytics artifacts. This is the obvious home for a durable copy.

## Proposed approach

At end-of-run (after `summary.md` / sidecars are finalized), **mirror the
analytics artifacts to a durable, worktree-independent location** so they
survive worktree teardown. Reuse the existing `userstate.Dir()` runs convention
rather than inventing a new path. Keep the in-worktree copy as-is (locality for
live debugging); add a durable copy that analytics reads from.

## Items

1. **Decide the durable root and layout** (resolve Open Question A first). Add a
   helper that resolves `<durable-root>/runs/<runTimestamp>/` independent of
   `repoPath`, alongside the existing `resolvePidRunDir`.

2. **Mirror the analytics artifacts on completion.** After the run's
   `summary.md`, `summary.jsonl`, and `steps.jsonl` are written, copy them (plus
   `flow.txt`) to the durable dir. Drive it from the same deferred tail that
   prints the summaries (`printStepSummary` / `printAgentSummary` site in
   `driver.go`) so it fires on success *and* error paths. Per-agent event/prompt
   files are large — see Open Question B on whether to include them.

3. **Make it best-effort, never fatal.** A failed durable-copy warns to stderr
   and never blocks or fails the run — mirror the existing sidecar stance
   (`recordStep` / `appendSummaryLine` "best-effort" contract).

4. **Pruning for the durable store.** The durable dir now accumulates across all
   runs and all worktrees. Add retention (resolve Open Question C) — reuse
   `pruneOldRuns` against the durable root, with its own cap (likely larger than
   the per-repo `--keep-runs` default of 10, since this is the analytics archive).

5. **Tests.** Round-trip test: run with a temp `repoPath` + temp durable root,
   assert the sidecars exist in *both* after completion; assert a write failure
   on the durable side is a warning, not a run failure; assert durable pruning
   honours its cap.

6. **Document the location.** Update the relevant help text / README so operators
   know where persistent run history lives and that it is the surface analytics
   should read (the `.jsonl` sidecars), not the transient in-worktree copy.

## Open questions

- **A. Durable root.** **(Recommended)** Reuse `<userstate.Dir()>/runs/<ts>/` —
  already exists, already worktree-independent, already the PID-marker home; one
  helper to add. Alternative: a workspace-level `<workspace>/.gh-optivem-runs/`
  (more discoverable in the IDE, but introduces a new path and a
  workspace-root-resolution dependency). Recommend user-state dir.

- **B. Per-agent files (`NNN-<agent>.events.jsonl` / `.prompt.md` / `.outputs.jsonl`).**
  **(Recommended)** Mirror only the small aggregate artifacts (`summary.md`,
  `summary.jsonl`, `steps.jsonl`, `flow.txt`) — that is the analytics surface,
  and the per-agent event streams are large (100k–230k each in the sampled run).
  Alternative: mirror everything (full forensic replay, much more disk). Recommend
  aggregate-only, revisit if forensic replay of pruned worktrees is ever needed.

- **C. Durable retention.** **(Recommended)** Keep more than the per-repo default —
  e.g. last 100 runs (or a size cap) — since the whole point is history. Make it a
  flag/env with a sensible default. Alternative: never prune (unbounded growth).
  Recommend a generous default cap.

- **D. Scope: all runs or rehearsals only?** **(Recommended)** Do it for **all**
  runs — a durable archive is harmless for real-repo runs (they already have the
  in-repo copy; the durable copy is just additive) and avoids a rehearsal-vs-real
  branch. Alternative: only when running inside a worktree. Recommend all runs,
  uniform path.

## Out of scope

- Any analytics tooling/dashboards that *consume* the durable JSONL — this plan
  only guarantees the data survives. Querying/aggregation is a separate effort.
- Changes to the in-worktree artifact layout or the rehearsal harness's
  worktree lifecycle itself.
