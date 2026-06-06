# Plan: Decouple the verify `suite` from the `tests` path-discriminator (fix bare `--suite=contract`)

## Why

A rehearsal of user story #72 ("Charge shipping based on product weight") halted in the
contract cycle with:

```
gh optivem test run --suite=contract --test=shouldBeAbleToGetProductWeight
ERROR: suite(s) not found: contract.
Available: … contract-stub, contract-stub-isolated, contract-real …
```

This is **not** a test-code regression (the WRITE-phase weight DSL chain compiles). It is a
runtime-config defect in `process-flow.yaml`.

### Root cause

`implement-test-layer` derives its verify suite straight from the path discriminator:

- `process-flow.yaml` `VERIFY_TESTS_PASS_FILTERED` → `suite: ${tests}` (and its
  `VERIFY_TESTS_FAIL_FILTERED` twin).

`tests` is **overloaded**: it is both
1. the **AT/CT path discriminator** read by the CT-path System-Driver fence
   (`internal/atdd/runtime/actions/bindings.go`, `if ctx.Params["tests"] == "contract"`), and
2. the **test-runner suite selector** (via `suite: ${tests}`).

The AT path survives this only by luck: `tests: acceptance` happens to be a registered
suite-group alias (`internal/atdd/runtime/testselect/suite.go` → `acceptance-api`,
`acceptance-ui`). The CT path feeds `tests: contract`, but `contract` is **not** a
registered alias, so it passes through verbatim and the runner rejects it.

Introduced when the CT cycle was nested under AT (commit `497771a`). Story #72 is the first
to drive the nested-CT DSL verify, so the first to trip it. It will hit **every** contract
path where the DSL port changed.

### Exact emission point

```
implement-and-verify-external-system-driver-adapters-contract-tests  (CT-HIGH)
└─ IMPLEMENT_AND_VERIFY_DSL            params: tests: contract        ← origin
   (only via GATE_DSL_PORT_CHANGED == true)
   └─ implement-and-verify-dsl
      └─ IMPLEMENT_TEST_LAYER          tests: ${tests} → contract
         └─ implement-test-layer
            └─ VERIFY_TESTS_PASS_FILTERED  suite: ${tests} → suite: contract   ★ tests→suite copy
               └─ … run-tests → EXECUTE_COMMAND → "--suite=contract"           ★ flag emitted
```

## Decision (locked in discussion 2026-06-06)

Chosen fix = **Option B: decouple `suite` from `tests`**, NOT the one-line alias-registration
(Option A: `contract → [contract-real]`). Rationale: Option A papers over the conflation and
leaves a misleadingly-named alias (`contract` that means only `contract-real`); Option B
restores the real distinction — `tests` stays a pure path discriminator, `suite` becomes an
explicit, caller-supplied test-runner selector. No new alias is registered; `contract-real`
is already a real suite id needing no expansion.

- **CT DSL-step suite = `contract-real`.** Same "real passes after DSL + DTO wiring" intent
  as the immediately-following `VERIFY_TESTS_PASS_CONTRACT_REAL` node. It must NOT be the full
  contract group — `contract-stub` is not implemented until later in the cascade, so a union
  verify would demand the stub pass before it exists.
- **AT-path suites stay `acceptance`.** Behaviour-preserving: the two AT leaves keep emitting
  `suite: acceptance` (now explicit instead of implicit-via-`tests`). The `acceptance` alias
  in `suite.go` is **retained** — see the channel note below.

## Channel note — is the `acceptance` alias still needed? (answers the side question)

Yes, it is load-bearing today. Per `internal/atdd/runtime/statemachine/channels.go`:

- `UnrollSystemChannels` rewrites the **system-impl GREEN** step (`change-system-behavior`'s
  `implement-and-verify-system`) to `suite: acceptance-<channel>` per channel — that step is
  genuinely per-channel and does NOT use the alias after unroll.
- `UnrollSystemDriverAdapterChannels` overrides **only `channel`** per node and **keeps the
  inherited suite** — so the per-channel adapter verify still runs the union `acceptance`.
- The **DSL-layer AT verify** also runs the union `acceptance`.
- The **absent-`channels:` fallback** keeps the single static `suite: acceptance`.

So implementation work is per-channel (system impl + adapter folders), but acceptance-test
*verification* at the DSL/adapter layers currently runs both channels.

For **this** fix the alias is **retained as-is** — the change is strictly behaviour-preserving
for AT. But its necessity is **not settled**: those DSL/adapter verifies are RED proofs of
channel-agnostic behaviour, so the union (both-channel) run is arguably overkill — only the
per-channel GREEN system-impl step clearly earns per-channel coverage. That question (slim the
RED-layer verify to a single channel; re-evaluate whether the `acceptance` alias /
`ExpandSuiteGroups` still earns its slot) is carried by a **separate follow-up plan**
(`plans/20260606-1116-slim-red-layer-acceptance-verify.md`) and is out of scope here.

## Items

1. **`implement-test-layer`: read suite from an explicit param.**
   In `internal/atdd/runtime/statemachine/process-flow.yaml`, change both verify nodes
   `VERIFY_TESTS_PASS_FILTERED` and `VERIFY_TESTS_FAIL_FILTERED` from `suite: ${tests}` to
   `suite: ${suite}`. (`tests` is still inherited for the fence; only the suite source moves.)

2. **Thread `suite` through the two intermediate processes.**
   - `implement-and-verify-dsl` → its `IMPLEMENT_TEST_LAYER` node `params:` gains `suite: ${suite}`.
   - `implement-and-verify-system-driver-adapters` → its `IMPLEMENT_TEST_LAYER` node `params:`
     gains `suite: ${suite}`.

3. **Bind `suite` explicitly at all three leaf callers** (strict-mode `ExpandParams` rejects an
   unresolved `${suite}`, so none may be left unbound):
   - AT/DSL caller in `write-and-verify-acceptance-test-code` (`tests: acceptance`) → add `suite: acceptance`.
   - AT/adapter caller in `write-and-verify-acceptance-tests` (`tests: acceptance`) → add `suite: acceptance`.
   - CT/DSL caller in `implement-and-verify-external-system-driver-adapters-contract-tests`
     (`tests: contract`) → add `suite: contract-real`.

4. **Comment hygiene.** Update the `UnrollSystemDriverAdapterChannels` doc comment in
   `channels.go` that says the adapter verify suite "stays the inherited `tests:` value" — it
   now stays the inherited explicit `suite:` value (`acceptance`). Behaviour identical; wording
   only.

5. **Regression test.** In `internal/atdd/runtime/statemachine/transitions_test.go`, add an
   assertion that the CT-HIGH `IMPLEMENT_AND_VERIFY_DSL` node resolves its filtered verify to
   `suite: contract-real` (guards against the bare-`contract` regression). Keep the existing
   `contract-real` / `contract-stub` node assertions.

## Verification

- `go test ./internal/atdd/runtime/statemachine/ ./internal/atdd/runtime/testselect/ ./internal/runner/ -p 2`
  (scoped; never unbounded `go test ./...` on Windows). No new sequence-flow edges are added,
  so the statemachine loop/RAM hazard does not apply.
- Re-run the #72 rehearsal (or a contract-path slice that changes the DSL port) and confirm the
  DSL-step verify emits `--suite=contract-real` and the cycle proceeds past the former halt.
- Rebuild + reinstall the binary the rehearsal harness uses
  (`go install ./...` → `~/go/bin/gh-optivem`) before re-running.

## Out of scope

- Per-channel refinement of the DSL/adapter union acceptance verifies — carried by
  `plans/20260606-1116-slim-red-layer-acceptance-verify.md`.
- Any change to `suite.go`'s alias registry (the `acceptance` alias is retained as-is here; no
  `contract` alias is added). Whether the alias survives long-term is the follow-up's call.
