# Bound the `fix`-dispatch loops: reject = halt + capped retries (`execute-command`, `execute-agent`)

> **Working style: token-efficient.** Execute in the cheapest form that still produces a quality result; if a costlier workflow is proposed where a cheaper one suffices, surface the cheaper option (memory: `feedback_flag_non_token_efficient`).

> **No diagram-regen step here.** Editing `process-flow.yaml` triggers the regenerate-diagram GitHub Actions workflow on push to `main`; a local regen step races it (memory: `feedback_plans_no_diagram_regen`). This plan stops at YAML + tests.

## Context

Triggered by rehearsal `rehearsal-71-gift-wrap-an-order-20260530-122333`, which sat in an endless loop re-asking *"Do you approve fix to attempt remediation for command-failed?"*. A `gh optivem test compile` failure (100 errors, all in `testkit/driver/**` + `systemtest/configuration/**` — the classic signature of Lombok annotation processing not running, and outside the `command-failed-fixer`'s scope) could not be fixed in-scope, so every cycle re-ran the identical compile and re-prompted. Neither `y` nor `n` escaped: `execute-command` routes `{from: FIX, to: RUN_COMMAND}` regardless of how `fix` ended.

### Loopback audit of `process-flow.yaml`

Five back-edges. Three are properly bounded; two are not:

| Process | Back-edge | Bound today |
|---|---|---|
| `verify-tests-pass` | `FIX_UNEXPECTED_FAILING_TESTS → RUN_TESTS` | `max-visits: 2` → `FIX_LOOP_EXHAUSTED` ✅ |
| `verify-tests-fail` | `FIX_UNEXPECTED_PASSING_TESTS → RUN_TESTS` | `max-visits: 2` → `FIX_LOOP_EXHAUSTED` ✅ |
| `refactor` | 4× `…STRUCTURE → GATE_REFACTOR_TYPE_CHOICE` | human exit edge (`choice == none`) ✅ |
| `execute-command` | `FIX → RUN_COMMAND` | **none** — global backstop only |
| `execute-agent` | `FIX → RUN_AGENT` | **none** — global backstop only |

### How retries are controlled today (full control surface)

1. **`fix-on-failure` flag** — on failure, `true` dispatches `fix`; the fix dispatch re-enters `execute-agent` with `fix-on-failure: false`, so a fix never recurses into a nested fix. Bounds *recursion depth*, not *loop iterations*.
2. **`max-visits` / `on-max-visits`** (`load.go`/`run.go`) — per-node semantic cap; on the (N+1)th arrival, route to `on-max-visits` without executing the body. Currently only on the two verify-loop FIX nodes (`max-visits: 2`). The graceful bound.
3. **`maxDispatchesPerProcess = 10000`** (`run.go:200`, non-configurable const) — per-`runProcess` backstop; errors out, ungraceful.
4. **Approval gates** — `CategoryHuman` is the top tier and the default `--auto` floor, so `human < floor` is always false → **human gates always prompt, even under `--auto`** (`approval.go:229`). The whole `fix` flow is `category: human`.

### Why today's bounds are insufficient

- **Backstop is unreachable in practice.** The `execute-command` loop body is ~4 dispatches → ~2,500 cycles to trip 10000. Interactively the human `ASK_HUMAN` rate-limits and the operator gives up first (rehearsal-71: ~7 cycles over 3+ hours, never self-terminated). A control that fires only after 2,500 wasted cycles is not a control.
- **Autonomous mode (`--auto`, no TTY) is the real hazard.** The human-tier `fix` prompt reads EOF and `ConfirmYN` returns `(false, nil)` — **auto-reject** (`promptio.go`). Then:
  - `execute-command`: fix auto-rejected → `FIX → RUN_COMMAND` → re-run the deterministic command → spins on compile/test cycles.
  - `execute-agent`: the back-edge re-enters `RUN_AGENT` **with no approval gate on re-entry** (`APPROVE_PRE` guards first entry only) → **re-dispatches the opus·high writing agent every cycle**. This is the "goes back to agent work and burns too much" token-burn mode.
- The `STOP_SCOPE_VIOLATION` halt (`4b87177`) only covers the agent *voluntarily* emitting a scope-exception envelope — not the reject re-loop, nor a fixer that produces valid-but-non-resolving output and re-fails identically.

## Design principles (industry grounding)

- **Unbounded retry is an anti-pattern** → retry budgets / capped attempts. *Google SRE* ("Addressing Cascading Failures"): retries need an aggregate cap or they amplify into retry storms. Durable-workflow engines make this first-class (Temporal `MaximumAttempts`, Step Functions `Retry.MaxAttempts`); queues do too (SQS `maxReceiveCount` → DLQ). → **the cap.**
- **Only retry transient faults; fail fast on permanent ones** → *Azure Retry pattern*. A deterministic compile error is *permanent* — re-running is pure waste. A stochastic LLM dispatch is *transient-like* — a small bounded re-roll is legitimate. → why the command loop leans on reject=halt and the agent loop keeps a real (capped) retry budget.
- **Fail fast, fail loud; open the circuit** → *Nygard, Release It!* (Fail Fast + Circuit Breaker). Don't busy-wait on a known failure. → **reject = halt** (the operator, or auto-reject, opens the breaker).
- **Escalate to human after automated recovery is exhausted** → poison-message → dead-letter + alert. → `on-max-visits` routes to a named error-end-event for human adjudication.
- **A throttle is not a bound** → safety must come from a control that *guarantees* termination, not one that merely slows it (a watching human isn't a guarantee, and isn't present under `--auto`). Layered defense (Swiss-cheese): cap + reject=halt cover disjoint paths; the 10000 backstop stays a true last line, not the primary control.
- **Low primitive reports; caller routes; deliberate stops are error events** → modelling the halt as a BPMN `error-end-event` is the spec mechanism for "this path can't complete — propagate an error," keeping routing upstream.

## Decisions (resolved upfront)

- **D1 — `max-visits: 2` on the `FIX` node in `execute-command` and `execute-agent`.** Caps the *approve/retry* path. `2` already means **3 total attempts at the operation + 2 fix passes** — not stingy. Rationale for 2 over 3: by attempt 3 two independent high-effort passes have failed, so the fault is almost certainly *permanent* (diminishing returns; the verify-loop authors profiled the 3rd opus·high pass as "rarely resolves"); the cost asymmetry favors lower (over-cap → a human cheaply re-triggers; over-spend → unrecoverable tokens, the exact autonomous burn we're guarding); and it stays consistent with the verify loops. Revisit to 3 only on telemetry showing attempt-3 recoveries are common — not on guess.
- **D2 — Reject (`n`) = hard halt.** Change `fix`'s `FIX_REJECTED_END` from `end-event` to `error-end-event`. Because both `execute-command` and `execute-agent` call the **same `fix` process**, this single edit gives both loops a clean operator exit *and* makes `--auto` halt immediately on EOF-reject (zero retry burn). Routes/halts like `STOP_SCOPE_VIOLATION`.
- **D3 — `fix`'s PRE-reject is the deliberate exception to the documented "PRE-rejection = soft skip" philosophy.** That philosophy holds because a rejected writing agent lets "downstream fail naturally on the missing artifact." It does **not** hold for `fix`, whose downstream is a *re-run of the step that just failed* — so fix-reject must halt, not soft-continue. Leave `execute-agent`'s own `APPROVE_PRE` → `EXECUTE_AGENT_REJECTED_END` soft-skip unchanged (that's the writing-agent case the philosophy was written for).
- **D4 — Distinct error-end-events per process:** `COMMAND_FIX_EXHAUSTED`, `AGENT_FIX_EXHAUSTED` (mirrors the existing `FIX_LOOP_EXHAUSTED`); each names its own halt verdict for the diagram label and the adjudicating human.
- **D5 — Verify loops stay as-is.** They call `execute-agent` directly (no `fix` wrapper), are already capped at 2, and their auto-rejected fixer never dispatches under `--auto` (the re-run node `RUN_TESTS` is cheap) — so they carry no endless-loop or token-burn risk. Extending reject=halt to them would require splitting the *shared* `EXECUTE_AGENT_REJECTED_END` soft-skip terminal — disproportionate machinery for a loop that's already safe (memory: `feedback_drop_dont_relocate`).
- **D6 — No new config knob.** Cap stays a hardcoded `max-visits:` in YAML; `maxDispatchesPerProcess` stays a const backstop (memory: `feedback_schema_fields_earn_slot`).
- **D7 — Autonomous "fixers never run" is intentional, not in scope.** Fixes are human-gated by the ladder model; under `--auto` the fixer auto-rejects and the run is *meant* to halt for a human — which D1+D2 now deliver cleanly.

## Steps

### Step 1 — Reject = halt (covers both loops)
In `process-flow.yaml`, `fix` process: change node `FIX_REJECTED_END` from `type: end-event` to `type: error-end-event`; rename to a meaningful verdict (e.g. *"Fix Declined — Run Halted"*). Add a comment citing D2/D3 (PRE-reject exception). Leave the `{from: GATE_APPROVED_PRE, to: FIX_REJECTED_END, when: rejected}` edge unchanged.

### Step 2 — Cap `execute-command`'s FIX loop
On the `FIX` node: add `max-visits: 2`, `on-max-visits: COMMAND_FIX_EXHAUSTED` (comment mirrors the verify-loop cap, cite this plan). Add `error-end-event` `COMMAND_FIX_EXHAUSTED` (name e.g. *"Command Fix Exhausted — `${command}` still failing after 2 fix attempts (likely out of fixer scope)"*). Leave `{from: FIX, to: RUN_COMMAND}` unchanged.

### Step 3 — Cap `execute-agent`'s FIX loop
On the `FIX` node: add `max-visits: 2`, `on-max-visits: AGENT_FIX_EXHAUSTED`; comment that the back-edge re-enters `RUN_AGENT` with no re-approval, so this cap is the only graceful guard against repeated writing-agent re-dispatch under `--auto`. Add `error-end-event` `AGENT_FIX_EXHAUSTED` (name e.g. *"Agent Fix Exhausted — `${task-name}` output still invalid after 2 fix attempts (widen scope or adjudicate)"*). Leave `{from: FIX, to: RUN_AGENT}` unchanged.

### Step 4 — Update statemachine tests
`internal/atdd/runtime/statemachine/transitions_test.go` (+ any process-shape/golden test): assert the `max-visits`/`on-max-visits` on both `FIX` nodes, the two new error-end-events, and `FIX_REJECTED_END` now being an error-end-event; add a transition test that the 3rd `FIX` arrival routes to the exhausted terminal without dispatching, and that fix-reject halts. Model on existing `FIX_LOOP_EXHAUSTED` / `STOP_SCOPE_VIOLATION` coverage. Confirm `load.go` `buildProcess` validation (MaxVisits>0 ⇒ OnMaxVisits required + target exists) is satisfied.

### Step 5 — Build + targeted test
`go build ./...`; `go test ./internal/atdd/runtime/statemachine/...` (scoped — never unbounded `go test ./...` on Windows, memory: `feedback_go_test_windows`; watch for the statemachine loop/RAM hazard, `feedback_statemachine_test_loop_hazard` — audit gate fixtures first, kill on memory climb).

## Verification

- Re-run the rehearsal (`scripts/atdd-rehearsal.sh`, issue 71) off a fresh build. Confirm: replying `n` at the fix prompt **halts** (Fix Declined) instead of re-prompting; replying `y` repeatedly halts at `COMMAND_FIX_EXHAUSTED` after 2 fix attempts.
- (Optional) confirm an `--auto` run halts on the first compile failure rather than spinning.
- Diagram regeneration is handled by the regenerate-diagram CI workflow on push to `main` — do not regenerate locally.

## References

- Audit + diagnosis of rehearsal-71: this conversation (2026-05-30).
- Verify-loop cap precedent: `verify-tests-pass`/`verify-tests-fail` (`max-visits: 2` → `FIX_LOOP_EXHAUSTED`), commit `56fe4b9`.
- Scope-exception halt: commit `4b87177`, `STOP_SCOPE_VIOLATION`.
- Control surfaces: `internal/approval/approval.go`, `internal/promptio/promptio.go`, `internal/atdd/runtime/statemachine/run.go`.
- Principles: Google SRE Book (cascading failures / retries); Nygard, *Release It!* (Fail Fast, Circuit Breaker); Azure Architecture Center (Retry pattern — transient vs. permanent); BPMN error-end-event semantics.
