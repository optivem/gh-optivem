# Fix `validate-outputs-and-scopes` cross-phase false positives

## Problem

`validate-outputs-and-scopes` (the post-`RUN_AGENT` validator in the LOW
`execute-agent` primitive) compares the working tree against `HEAD`. The
ATDD pipeline runs many phases back-to-back without committing between
them, so every phase after the first sees the *previous* phases'
uncommitted edits in the diff baseline. The validator then attributes
those paths to whichever phase happens to be running now and reports
out-of-scope violations against files the current agent never touched.

Reproduced on 2026-05-26 during a rehearsal of issue #61 (TypeScript
monolith). Phase order:

1. `implement-system-driver-adapters` legitimately edited
   `system-test/typescript/.../NewOrderPage.ts`. Approved.
2. `implement-external-system-driver-adapters` made no edits (correctly —
   the ticket had no external-system work).
3. `validate-outputs-and-scopes` for Phase 2 still saw Phase 1's
   uncommitted `NewOrderPage.ts` in `git diff --name-only HEAD`, joined
   against Phase 2's `scopes:` (`external-system-driver-port`,
   `external-system-driver-adapter`), and flagged the file as
   out-of-scope. The `scope-diff` `fix-*` recovery was then dispatched
   with no real failure to fix.

Root cause: `actions.modifiedPathsSinceHead`
(`internal/atdd/runtime/actions/bindings.go:560`) unions
`git diff --name-only HEAD` + `git status --porcelain`. Both are
HEAD-relative, not phase-relative.

`checkPhaseScope` (`bindings.go:460`, registered as `check-phase-scope`)
uses the same helper and has the same latent bug, though it is not
currently wired into `process-flow.yaml`. The helper is the single point
to fix.

## Design

Switch the per-phase baseline from `HEAD` to a **fingerprint of the
working tree captured immediately before `RUN_AGENT`**. The fingerprint
is `map[path]contentHash` for every modified tracked path + every
untracked path (the same set `modifiedPathsSinceHead` enumerates today,
but as a snapshot, not a diff). After `RUN_AGENT`, "paths modified by
this phase" = the symmetric set difference between the post-state
fingerprint and the snapshot:

- present in snapshot, absent now → deleted by this phase
- absent in snapshot, present now → added by this phase
- present in both, hash differs → modified by this phase
- present in both, hash matches → untouched (i.e. an upstream phase's
  edit that this phase did not change; correctly excluded)

Snapshot lives in `ctx.State` under a stable key
(`pre-agent-fingerprint`). It is a transient per-phase value — the
next phase overwrites it on its own snapshot step.

### Resolved questions

- **Scope of the snapshot**: tracked-modified + untracked + ignored?
  Track tracked-modified + untracked (everything `git status --porcelain`
  reports). Don't fingerprint clean tracked files — too expensive on
  large repos and unnecessary, because a file that was clean at snapshot
  time and is dirty now will appear in the post-state `git status`.
- **Hash function**: SHA-256 of file bytes via `crypto/sha256`. Cheap,
  collision-resistant, no external dependency. Renames are not tracked
  specially — they show up as one delete + one add, which is what the
  validator already accepts.
- **Where to wire the snapshot task**: as a new service-task node
  `SNAPSHOT_WORKING_TREE` in the `execute-agent` subprocess of
  `process-flow.yaml`, between `APPROVE_PRE` and `RUN_AGENT`. Action
  name: `snapshot-working-tree`. The fix-recovery subprocess (`fix`)
  re-enters `execute-agent`, so the fix agent gets its own snapshot
  step automatically.
- **`check-phase-scope`**: out of scope as a *consumer* (not currently
  wired in `process-flow.yaml`), but it shares the
  `modifiedPathsSinceHead` helper. The plan removes the helper and
  rewires `checkPhaseScope` to the same snapshot baseline, so the
  dormant code stays correct if/when it gets re-wired.
- **`fix-scope-diff` prompt copy**: the prompt
  (`internal/assets/runtime/prompts/atdd/fix-scope-diff.md`) says the
  validator "enumerated the working-tree changes since HEAD". One-line
  reword to "since the pre-agent snapshot". No behavioural change to
  the fix agent itself.
- **`${changed_files}` placeholder**: today this is the full
  `git status` dump. After the fix, change it to the **delta against
  the snapshot** so the fix agent doesn't reason about upstream-phase
  edits that are irrelevant to its diagnosis. Bake this into the
  validator's state-write so the prompt template gets a narrower list.
- **Backwards-compatibility shims**: none. `modifiedPathsSinceHead` is
  removed entirely; both consumers move to the new helper in the same
  change. Per the gh-optivem teaching-repo policy (no `Legacy*`
  scaffolding), no transitional alias.

## Items

### Item 1 — Snapshot + delta helpers

In `internal/atdd/runtime/actions/bindings.go`:

- Add `type WorkingTreeFingerprint map[string]string` (path → hex SHA-256).
- Add `(a actions) captureWorkingTreeFingerprint(ctx context.Context) (WorkingTreeFingerprint, error)`:
  enumerates dirty paths via the same `git status --porcelain`
  parsing as today (tracked-modified + untracked + rename endpoints),
  reads each file from disk via the action's repo-relative path, and
  hashes the bytes. Files that disappear between enumeration and read
  (race) get an empty hash entry — equivalent to "deleted".
- Add `(a actions) modifiedPathsSinceFingerprint(ctx context.Context, base WorkingTreeFingerprint) ([]string, error)`:
  re-runs the enumeration, hashes the current state, and returns the
  sorted union of (paths in `base` but not now), (paths now but not
  in `base`), and (paths in both with differing hashes).
- Delete `modifiedPathsSinceHead`. No callers will remain after Items
  2 and 3.

### Item 2 — Snapshot service-task action + registration

In `internal/atdd/runtime/actions/bindings.go`:

- Add `(a actions) snapshotWorkingTree(ctx *statemachine.Context) statemachine.Outcome`:
  calls `captureWorkingTreeFingerprint`, stashes the result in
  `ctx.State["pre-agent-fingerprint"]`. Returns
  `statemachine.Outcome{Err: ...}` on `git status` failure (genuine
  wiring problem, same shape as the existing `validate-outputs-and-scopes`
  hard-error path).
- Register under `r.Register("snapshot-working-tree", a.snapshotWorkingTree)`
  next to the existing `validate-outputs-and-scopes` registration
  (`bindings.go:179`).

### Item 3 — Rewire validators to consume the snapshot

In `internal/atdd/runtime/actions/bindings.go`:

- `validateOutputsAndScopes` (line 762): replace the
  `modifiedPathsSinceHead` call (line 796) with
  `modifiedPathsSinceFingerprint(ctx.Context, snapshot)` where
  `snapshot` is read from `ctx.State["pre-agent-fingerprint"]`. If
  the snapshot key is missing, hard-error (`"validate-outputs-and-scopes: pre-agent-fingerprint not set — execute-agent must run snapshot-working-tree before RUN_AGENT"`)
  — this is a wiring bug, not an agent-output problem.
- Also update the `ctx.Set("scope-violating-paths", ...)` call to
  emit the snapshot-delta list (which is what `violating` already
  iterates over) — no behavioural change here, the list just no
  longer contains upstream-phase paths.
- Write a new state key `phase-changed-files` (sorted, newline-joined)
  carrying the full snapshot delta (in-scope + out-of-scope), so the
  `fix-scope-diff` template's `${changed_files}` substitution can
  point at it instead of the current full-`git status` dump.
- `checkPhaseScope` (line 460): same baseline switch. If
  `ctx.State["pre-agent-fingerprint"]` is missing (no upstream
  snapshot), fall back to `HEAD` *only here* — `check-phase-scope` is
  the dormant cycle-boundary checker and may be reused in a different
  context where no per-phase snapshot exists. Document the fallback
  in the godoc; emit a debug line via `a.deps.Stderr` so re-wiring
  is loud.

### Item 4 — Process-flow wiring

In `internal/atdd/runtime/statemachine/process-flow.yaml`, inside the
`execute-agent` subprocess (line 1658):

- Add a new service-task node between `APPROVE_PRE` and `RUN_AGENT`:

  ```yaml
  - id: SNAPSHOT_WORKING_TREE
    type: service-task
    action: snapshot-working-tree
  ```

- Replace the sequence-flow
  `{from: APPROVE_PRE, to: RUN_AGENT}` (line 1701) with two edges:
  `APPROVE_PRE → SNAPSHOT_WORKING_TREE → RUN_AGENT`.
- Update the file-header comment block (lines ~1646–1656) describing
  `execute-agent` to mention the snapshot step and why it exists
  (cross-phase baseline correctness).

### Item 5 — `fix-scope-diff` prompt copy

In `internal/assets/runtime/prompts/atdd/fix-scope-diff.md`:

- Line 9: replace "enumerated the working-tree changes since HEAD"
  with "enumerated the working-tree changes since the pre-agent
  snapshot".
- Line 21: clarify that `${changed_files}` is the snapshot-delta
  listing, not the full `git status` dump.
- No behavioural change to the fix agent's diagnostic playbook.

### Item 6 — Tests

In `internal/atdd/runtime/actions/bindings_test.go`:

- New table-driven test on `captureWorkingTreeFingerprint`: clean
  repo (empty map), one modified tracked file (one entry, hash matches
  on disk), one untracked file (entry present), rename (both endpoints
  fingerprinted).
- New test on `modifiedPathsSinceFingerprint`: given a fingerprint and
  a sequence of post-state working trees, returns the expected delta
  in each case (add / delete / modify / no-op).
- Update the existing `validateOutputsAndScopes` tests that seed
  `git diff` / `git status` stubs to also pre-seed
  `ctx.State["pre-agent-fingerprint"]`. The "missing snapshot key →
  hard error" case is its own test.
- New scenario test: simulate the bug. Pre-seed a snapshot that
  already contains `path-A` (an upstream-phase edit), run the
  validator with only `path-A` in the post-state (no new edits), and
  assert `outputs-and-scopes-valid=true` with no violating paths.

In `internal/atdd/runtime/driver/embedded_smoke_test.go` (or the
nearest equivalent end-to-end smoke):

- Add a two-phase smoke covering the rehearsal-#61 shape: Phase 1
  edits a file in scope-A, approved. Phase 2 has scope-B and makes no
  edits. Assert no `fix-scope-diff` dispatch.

### Item 7 — Docs

- `internal/atdd/runtime/actions/bindings.go` godocs already exist on
  `validateOutputsAndScopes` and `checkPhaseScope`; update both to
  describe the snapshot baseline instead of the HEAD baseline.
- Skim `docs/atdd/process/*.md` and `docs/atdd/architecture/*.md` for
  any prose that says "diffs against HEAD" in the scope-check
  context, and rewrite. Likely small (`grep -rni "since HEAD" docs/atdd`).

## Out of scope

- Wiring `check-phase-scope` into `process-flow.yaml`. It stays
  dormant for now — that decision belongs to the cycle-boundary
  enforcement plans listed in `plans/deferred/`.
- Persisting the snapshot to disk for cross-run survival. The
  snapshot is intentionally in-memory only; if the run crashes
  between snapshot and validation, the operator restarts the run from
  the call-site, which re-snapshots.
- A `gh optivem` CLI projection of the snapshot delta. Operators
  already get the violating-paths list in the validator output;
  nothing else consumes the snapshot today.

## Validation

After implementation:

- `go test ./internal/atdd/runtime/actions/...` — covers helpers + validator.
- `go test ./internal/atdd/runtime/driver/...` — covers the smoke.
- Manual: rerun the issue-#61 rehearsal
  (`bash ../gh-optivem/scripts/atdd-rehearsal.sh 61 --config gh-optivem-monolith-typescript.yaml`)
  and confirm Phase 2 (`implement-external-system-driver-adapters`)
  passes validation with no `fix-scope-diff` prompt when the agent
  legitimately makes no changes.
