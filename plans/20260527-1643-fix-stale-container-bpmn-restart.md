# Fix stale-container bug: BPMN `start-system` must restart after `build-system`

## Context

During rehearsal `worktrees/rehearsal-20260527-155753` (issue #71 "Gift-wrap an
order"), GREEN's `implement-and-verify-system` rebuilt the system Docker image
with the new gift-wrap code but the verify step then ran the two acceptance
tests against the **stale** running container — neither test exercised the
freshly-built code (gift-wrap field unmarshalled into nothing, total stayed at
200.00, gift-wrapped flag stayed false). The orchestrator then dispatched
`fix-unexpected-failing-tests` against tests that had no source-side bug.

### Root cause — two layers

1. **BPMN layer** (`internal/atdd/runtime/statemachine/process-flow.yaml`): the
   `start-system` MID at lines 1822-1838 dispatches `gh optivem system start`
   with no `--restart` flag. The three `build-system` → `start-system`
   sequences in the BPMN therefore rely entirely on the runner to notice that
   the image changed:
   - `implement-and-verify-system` (lines 1035-1042) — the main GREEN HIGH used
     by every `change-system-behavior` / redesign / refactor CYCLE.
   - `implement-and-verify-external-system-driver-adapters-contract-tests`
     post-driver-impl pair (lines 939-947).
   - Same process, post-stubs-impl pair (lines 981-989).

2. **Runner layer** (`internal/runner/system.go::Up` at lines 128-140): when
   called without `--restart`, `Up` checks `IsAnyURLUp(s, opts.Health)` and
   short-circuits with `"System is already running, skipping restart"` —
   meaning the freshly-built image is never instantiated as long as the old
   container is still healthy.

Either layer alone would prevent the bug, but neither catches it today, and a
healthy stale container at GREEN-start is the orchestrator's normal state (RED
brought the stack up earlier in the same cycle).

### Other `start-system` callers — correct as-is

The remaining `start-system` callsites are pre-verify safety calls or RED-side
calls that run *before* any build, so the idempotent skip is the right
behaviour for them:

- `write-and-verify-acceptance-test-code` line 770 (RED, no build).
- `implement-and-verify-external-system-driver-adapters-contract-tests` line
  962 (pre-stub-fail-verify safety call, no preceding build).
- `refactor-and-verify-tests` line 1107 (RED, no build).
- `implement-test-layer` line 1171 (test-layer impl, no system build).

These must continue to short-circuit when healthy or RED becomes needlessly
slow (down + up + health-wait on every test-write phase).

### Open questions resolved up-front

1. **Fix at the BPMN layer only, or also add a runner-side image-staleness
   guard?**

   **BPMN-only.** The bug surfaces at three named, enumerable call sites — the
   fix is "pass `--restart` from exactly the three places that have just
   rebuilt the image". A runner-side guard (compare the running container's
   image-id against the latest-built image-id and force a restart on
   mismatch) would also work, and would belt-and-braces this class of bug for
   future call sites and for local-dev users running `system build && system
   start` directly. But:

   - It moves the "should we restart?" decision into the runner where the
     BPMN can no longer express intent — the YAML stops being the source of
     truth.
   - It adds docker-coupling to `Up` (image-id introspection via
     `docker compose ps` or `docker inspect`) for a problem that already has
     a well-placed fix at the BPMN layer.
   - The failure mode without the guard is loud and immediate (tests fail
     against stale code) — not silent. Future call sites that omit
     `--restart` will surface the same way and be caught at the same review
     gate that this one was.

   Deferred to a follow-up plan **only if** the BPMN fix proves insufficient
   (e.g. local-dev users outside the orchestrator routinely hit the same
   problem and the documentation fix below is not enough). Recorded as a
   non-goal here.

2. **Split `start-system` into two MIDs (`start-system` + `restart-system`),
   or parameterise the existing MID?**

   **Parameterise.** The difference between the two callsite shapes is one
   CLI flag. A second MID would duplicate the entire process block to vary
   one literal. Adding a `${start-flags}` placeholder to the existing
   `command:` template mirrors the placeholder patterns already used by
   `execute-command` (`command: ${command}` at line 2124), `fix`
   (`task-name: "fix-${failure-kind}"` at line 2201), and `commit`
   (`message: "#${ticket-id} ${issue-title}"` at line 1862). The
   `ExpandParams` state-fallback path that resolves those placeholders also
   resolves this one.

3. **What's the default when a caller omits `start-flags`?**

   **Empty string, declared explicitly in `start-system`'s `params:`
   block.** `ExpandParams` state-fallback returns "" for a missing key in
   practice, but declaring the default in-place means a future change to the
   fallback contract cannot silently leak a literal `${start-flags}` into
   the dispatched command.

## Items

### 1. Add `${start-flags}` placeholder to the `start-system` MID

**Files touched:**

- `internal/atdd/runtime/statemachine/process-flow.yaml` (the `start-system:`
  block at lines 1822-1838)

**Change:** replace the existing `params:` block under `EXECUTE_COMMAND` with
the templated form:

```yaml
  start-system:
    name: "Start System"
    start: EXECUTE_COMMAND
    nodes:
      - id: EXECUTE_COMMAND
        type: call-activity
        process: execute-command
        name: "Dispatch the Command"
        params:
          command: "gh optivem system start ${start-flags}"
          start-flags: ""
```

The `start-flags: ""` default sits in the MID's own `params:` so that callers
who do not override it get a deterministic empty fragment (resolved by
`ExpandParams`'s params-first lookup) rather than relying on the state-fallback
contract for a missing key.

Add a block comment immediately above `start-system:` documenting the
restart-required contract:

> `${start-flags}` defaults to empty — the dispatched command is then
> `gh optivem system start`, and `runner.Up` short-circuits with the
> "already running, skipping restart" optimisation when the system is
> healthy (`internal/runner/system.go:131`). This is correct for callers
> that run *before* a system build (RED-side test-write phases, pre-verify
> safety calls).
>
> Callers that run *after* `build-system` MUST pass
> `start-flags: "--restart"` to force `runner.Up` to tear down the stale
> container and bring up the freshly-built image. Without it, `Up` skips
> the restart, the new image is never instantiated, and the verify step
> runs against the stale container — every test then exercises pre-build
> code and the orchestrator mis-routes the resulting failures to
> `fix-unexpected-failing-tests`.
>
> Current restart-required callers (Items 2-4 of plan
> `plans/20260527-1643-fix-stale-container-bpmn-restart.md`):
>
>   - `implement-and-verify-system` — `START_SYSTEM` after `BUILD_SYSTEM`.
>   - `implement-and-verify-external-system-driver-adapters-contract-tests`
>     — `START_SYSTEM_AFTER_DRIVER` after `BUILD_SYSTEM_AFTER_DRIVER`.
>   - Same process — `START_SYSTEM_AFTER_STUBS` after `BUILD_SYSTEM_AFTER_STUBS`.

### 2. Pass `start-flags: "--restart"` from `implement-and-verify-system`

**Files touched:**

- `internal/atdd/runtime/statemachine/process-flow.yaml` (the `START_SYSTEM`
  call-activity inside `implement-and-verify-system` at lines 1040-1043)

**Change:** add a `params:` block to the `START_SYSTEM` call-activity:

```yaml
      - id: START_SYSTEM
        type: call-activity
        process: start-system
        name: "Start the System"
        params:
          start-flags: "--restart"
```

This is the main GREEN HIGH used by every story / bug / redesign / refactor
ticket — it is the call site that bit the gift-wrap rehearsal and the one with
the broadest blast radius.

### 3. Pass `start-flags: "--restart"` from the CT driver-impl post-build start

**Files touched:**

- `internal/atdd/runtime/statemachine/process-flow.yaml` (the
  `START_SYSTEM_AFTER_DRIVER` call-activity inside
  `implement-and-verify-external-system-driver-adapters-contract-tests` at
  lines 944-947)

**Change:** add a `params:` block:

```yaml
      - id: START_SYSTEM_AFTER_DRIVER
        type: call-activity
        process: start-system
        name: "Start System"
        params:
          start-flags: "--restart"
```

### 4. Pass `start-flags: "--restart"` from the CT stubs-impl post-build start

**Files touched:**

- `internal/atdd/runtime/statemachine/process-flow.yaml` (the
  `START_SYSTEM_AFTER_STUBS` call-activity inside
  `implement-and-verify-external-system-driver-adapters-contract-tests` at
  lines 986-989)

**Change:** add a `params:` block:

```yaml
      - id: START_SYSTEM_AFTER_STUBS
        type: call-activity
        process: start-system
        name: "Start System"
        params:
          start-flags: "--restart"
```

### 5. Confirm `runCommand` tolerates the empty-flag trailing space

**Files touched (audit; edit only if test fails):**

- `internal/atdd/runtime/statemachine/run.go` (the `runCommand` action that
  dispatches `command:` strings — read only)
- `internal/atdd/runtime/statemachine/run_test.go` (add a focused test only if
  no existing test covers a `start-system`-shaped command)

**Change:** verify the `start-flags: ""` default produces the literal
`"gh optivem system start "` (trailing space) and that `runCommand` tokenises
this correctly — i.e. the trailing space does not become an extra empty
argv element that confuses Cobra. Existing patterns in the file already template
trailing placeholders (e.g. `run-tests`'s `${suite-flag}` / `${test-flag}` plumbing),
so this should already work; this item exists as a defensive read-through, not a
speculative edit.

If `runCommand` does mishandle the trailing space, normalise by trimming the
expanded command (e.g. `strings.TrimSpace`) or by tokenising with
`strings.Fields` before invoking the underlying exec call. Prefer
`strings.Fields` because it also collapses any double-space if a caller
passes a non-empty `start-flags` with a leading or trailing space — generalises
beyond this MID without expanding scope.

## Verification

For the operator after Items 1-4 land:

- Resume the in-flight rehearsal (`worktrees/rehearsal-20260527-155753`) by
  cancelling the current `fix-unexpected-failing-tests` dispatch and re-running
  the GREEN step. Confirm via the trace and the run logs under
  `.gh-optivem/runs/<ts>/NNN-execute-command.prompt.md` that the dispatched
  command is now `gh optivem system start --restart`, and that the verify step
  runs against a freshly-recreated container.
- Re-run the two gift-wrap tests (`placingAnOrderAsAGiftMarksItAsGiftWrapped`,
  `placingAnOrderAsAGiftAddsA5DollarPackagingFee`) under
  `gh optivem test run --suite acceptance-api --test
  placingAnOrderAsAGiftMarksItAsGiftWrapped,placingAnOrderAsAGiftAddsA5DollarPackagingFee`
  and confirm both pass — the WRITE-phase edits to `OrderService.java`,
  `Order.java`, `PlaceOrderRequest.java`, and `ViewOrderDetailsResponse.java`
  are now exercised because the rebuilt image is the one running.
- Re-run the next available CT-side rehearsal (or contrive a `task/external-
  system-redesign` ticket) and confirm both
  `implement-and-verify-external-system-driver-adapters-contract-tests` start
  points now dispatch `--restart`.

## Non-goals

- **Runner-side image-staleness guard in `internal/runner/system.go::Up`.**
  Deferred per resolved Q1 — fix at the BPMN layer first; escalate only if
  the BPMN fix proves insufficient (e.g. local-dev users hit the same bug
  outside the orchestrator).
- **Splitting `start-system` into `start-system` + `restart-system`.**
  Deferred per resolved Q2 — a one-line difference does not justify a second
  MID. The `${start-flags}` placeholder expresses the variation at the call
  site where it semantically belongs.
- **Changing `runner.Up`'s `IsAnyURLUp` short-circuit.** That optimisation is
  correct for local-dev re-runs and for the RED-side `start-system` callers
  that have not built anything. Touching it would regress those flows for the
  benefit of fixing a problem the BPMN-layer fix already closes.
- **Adding `--restart` to the other four `start-system` callsites** (`write-
  and-verify-acceptance-test-code` line 770, the pre-stub-fail safety call at
  962, `refactor-and-verify-tests` line 1107, `implement-test-layer` line
  1171). These run before any build; the idempotent skip is the correct
  behaviour and forcing a restart would slow every RED phase for no benefit.
- **Auditing or editing `README.md` / `docs/atdd/` / `.github/workflows/` for
  documented `system build && system start` sequences.** The bug observed in
  rehearsal manifested via the orchestrator; the CLI surfaces this with a
  loud test failure rather than a silent skip if a human runs it. If
  follow-up evidence shows local-dev users hit this, open a separate
  documentation plan.
