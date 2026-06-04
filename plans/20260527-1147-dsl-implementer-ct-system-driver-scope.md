# Wire CT as a nested subprocess of AT (+ fence the CT-path dsl-implementer)

> **Status (2026-06-04): ACTIVE — implementing.** Decision taken: the
> contract-test (CT) cycle becomes a **nested subprocess of the acceptance-
> test (AT) cycle**, fired on the `external-driver-port-changed` gate, rather
> than a standalone sibling run before AT. This plan now owns *both* halves of
> that change: (1) the **wiring** — repoint the AT-side external-driver gate at
> the CT-HIGH that is an orphan today; and (2) the **fence** — once that edge
> lands, the CT-path `dsl-implementer` goes live, so the System-Driver-scope
> hazard this plan originally tracked must be closed in the same change
> (Option A+D below).
>
> Supersedes the earlier "defer until the CT call site lands" verdict — we are
> now deliberately landing that call site, so the precondition the deferral
> waited on is being created here on purpose.

## TL;DR

**Why:** Today the CT-HIGH `implement-and-verify-external-system-driver-adapters-contract-tests`
(process-flow.yaml:965, full contract-real → contract-stub verify split at
1007–1076) is an **intentional orphan** — nothing in the live call graph
dispatches it (header comment 952–964: *"No other process calls this HIGH
today"*). Run #72 (`charge-shipping-based-on-product-weight`) proved the gap
concretely: its ticket carried a contract scenario ("ERP returns the weight")
but **no contract test was ever written** — the run walked the AT-only cascade
and silently dropped the CT half. We resolve Q31.a (Option A *nested*) by wiring
CT in.

**End result:**
- The AT-side external-driver gate (`GATE_EXTERNAL_DRIVER_PORTS_CHANGED` in
  `shared-contract`) routes its `true` branch into the **CT-HIGH** instead of
  today's thin AT-only `implement-and-verify-external-system-driver-adapters`
  step. The CT-HIGH is a **superset** — it implements the external adapter *and*
  writes+verifies the contract test (real, then stub) — so the thin variant is
  retired.
- The CT-path `dsl-implementer` (the `implement-and-verify-dsl` with
  `tests: contract` *inside* the CT-HIGH, line 981–988) is fenced so it cannot
  touch the System Driver port or emit `system-driver-port-changed: true` —
  closing the leak that would otherwise route control back into the AT cycle's
  system-driver-adapter gate. Fence = Option A (prompt guidance) + Option D
  (runtime output-flag invariant).

## Decision: nested, not sibling

The CT cycle could be wired two ways. We chose **nested**.

- **Nested (chosen).** The AT cycle drives; when the `dsl-implementer` raises
  `external-driver-port-changed`, the existing external-driver gate fires the
  CT-HIGH as a sub-step *between AT-red and AT-green*. One top-level walk per
  ticket. Keeps the outside-in double-loop story: the AT discovers the contract
  shape, and the contract test is what licenses the AT to trust its stub
  (contract-real ≡ contract-stub). Structurally enforces "stub certified before
  AT green."
- **Sibling-before (rejected).** A standalone top-level CT cycle run before AT.
  Cleaner failure attribution and de-risking, and it would sidestep the
  dsl-implementer scope hazard entirely — but it inverts contract *discovery*
  (you must pre-specify the contract before the AT exists) and teaches a
  non-double-loop, integration-first workflow. For a teaching repo whose every
  other cycle is outside-in, nested is the more coherent lesson.

Discriminator that settled it: in our intended flow the external-system
contract is **discovered by the feature**, not specified up front — which points
at nested. (If that ever flips to "contracts are specified up front / owned by a
platform team," revisit the sibling option.)

## Background — how the System-Driver-scope concern arose

On 2026-05-27 an ATDD rehearsal dispatch of `dsl-implementer` failed with:

```
clauderun: prompt has unfilled placeholders after substitution: ${touches-system-driver}
```

`dsl-implementer.md` referenced `${touches-system-driver}` as a switchable "is
the System Driver port in scope for this invocation?" parameter, but no caller
in `process-flow.yaml` ever bound it. It was meant to gate behaviour on two call
paths:

- **AT path** — `write-and-verify-acceptance-tests` → `implement-and-verify-dsl`.
  The AT side legitimately needs new System Driver prototypes to compile new
  ATs, so the parameter would be `true`.
- **CT path** — the CT-HIGH → `implement-and-verify-dsl`. Contract tests
  stimulate the *External*-System Driver, not the System Driver — the System
  Driver port is conceptually out of scope on this path.

The emergency fix **stripped the parameter entirely** (per memory
`feedback_schema_fields_earn_slot` — a field nothing branches on shouldn't be a
slot), on the basis that `dsl-implementer` should always be free to add System
Driver prototypes when the DSL it implements needs them. That resolved the leak
but left the agent with **no signal of which call path it is on**. While the
CT-HIGH stayed an orphan the hazard was dormant; wiring CT in (Change 1) makes
it live, which is why Change 2 ships in the same plan.

## Items

### 1. Repoint the AT external-driver gate at the CT-HIGH

`internal/atdd/runtime/statemachine/process-flow.yaml`, process `shared-contract`
(node `IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS`, ~715–722; gate
`GATE_EXTERNAL_DRIVER_PORTS_CHANGED`, ~710–713).

- Change that node's `process:` from
  `implement-and-verify-external-system-driver-adapters` →
  `implement-and-verify-external-system-driver-adapters-contract-tests`.
- **Reconcile call-site params.** Today the node forwards
  `task-name: implement-external-system-driver-adapters`,
  `expected-test-result: ${expected-test-result}`, `tests: acceptance`. The
  CT-HIGH starts at `WRITE_CONTRACT_TESTS` and pins its own expected results
  internally (write-contract-tests → success; contract-real pass; contract-stub
  fail-then-pass) and sources `${test-names}` from `write-contract-tests`'
  output (line 1446). It does **not** consume the caller's
  `expected-test-result`/`tests`. So **drop** those three params at the call
  site; pass only what the CT-HIGH actually reads. Verify against the CT-HIGH's
  `${...}` references during execution and bind exactly that set — no more.
- **Contract discovery is from the port diff, not the ticket.** Per the nested
  decision, `contract-test-writer` derives the contract from the changed
  external-system-driver-port; do **not** forward `acceptance-criteria` /
  ticket contract Gherkin. (Feeding ticket-specified contract scenarios in is a
  later enhancement — Out of scope.)
- Update the `shared-contract` header comment (658–675), which currently says
  the slice cascades into "external-system driver adapters," to say it cascades
  into the contract-test-first CT-HIGH.

### 2. Retire the thin AT-only external-adapter HIGH + de-orphan the CT-HIGH

`internal/atdd/runtime/statemachine/process-flow.yaml`.

- After Item 1, the HIGH `implement-and-verify-external-system-driver-adapters`
  (925–945) is unreferenced. Confirm with a grep for its name (expect only its
  own definition remaining), then remove it. **Keep** the leaf
  `implement-external-system-driver-adapters` — the CT-HIGH still calls it
  (line 992).
- Rewrite the CT-HIGH header comment (947–964): delete the "**Orphan in the
  structural call graph (intentional — known gap)** … No other process calls
  this HIGH today … left for Phase D" note and replace with "Called by
  `shared-contract`'s `GATE_EXTERNAL_DRIVER_PORTS_CHANGED` true-branch (Q31.a
  resolved: CT nested under AT)."
- Sweep the file for other stale "orphan"/"Phase D" references to this HIGH
  (e.g. the comment near line 1959 lists it as a restart-required caller — that
  one is fine; only the orphan framing changes).

### 3. Fence the CT-path dsl-implementer — Option A (prompt guidance)

`internal/assets/runtime/agents/atdd/dsl-implementer.md`.

- Add a CT-path guard note keyed on the contract-test context: *"If you are
  implementing CT-side DSL (dispatched with `tests=contract`), the System Driver
  port (`${system-driver-port}`) will not legitimately need new methods — do
  **not** add System Driver prototypes there and do **not** emit
  `system-driver-port-changed=true`. Contract tests stimulate the External-
  System Driver only (`${external-system-driver-port}`)."*
- Confirm the `tests` value actually reaches this prompt. The `implement-dsl`
  leaf forwards `tests` to `execute-agent`; verify `${tests}` is surfaced in the
  dsl-implementer prompt body (substituted, not just passed) so the guard has a
  signal to key on. If it isn't surfaced today, surface it. (Guidance only —
  hard enforcement is Item 4; per memory `feedback_agents_dont_validate_inputs`
  the agent body does not self-validate.)

### 4. Fence the CT-path dsl-implementer — Option D (runtime invariant)

`internal/atdd/runtime/actions/bindings.go` (beside the existing output
validation — pin the exact function during execution).

- When a `dsl-implementer` dispatch runs in CT context (`tests == contract`) and
  emits `system-driver-port-changed == true`, **fail the dispatch** with a
  diagnostic: *"CT-path dsl-implementer must not touch the System Driver port
  (`${system-driver-port}`); contract tests stimulate the External-System Driver
  only."*
- Source of the CT-path signal is the `tests` param in scope at validation time
  — pin where that is read; do not infer from agent output alone.
- AT context (`tests == acceptance`) emitting `system-driver-port-changed=true`
  stays **allowed** (it is correct there).

### 5. Tests

- **Statemachine fixture** proving the AT cascade, on
  `external-driver-port-changed == true`, now enters the CT-HIGH (a
  `write-contract-tests` / `contract-test-writer` dispatch appears) and walks the
  contract-real → contract-stub verify split. ⚠️ Per memory
  `feedback_statemachine_test_loop_hazard`, new edges in `process-flow.yaml` can
  deadlock the statemachine tests and consume 20GB+ RAM — audit the gate fixture
  for loopbacks before running, run scoped (`-p 2`, single package), and kill on
  any memory climb.
- **bindings.go unit test**: CT-path (`tests=contract`) `dsl-implementer`
  emitting `system-driver-port-changed=true` → dispatch fails with the
  diagnostic; AT-path (`tests=acceptance`) emitting the same → allowed.

## Verification (operator-driven, not agent steps)

- Re-run a contract-bearing rehearsal ticket (the #72 shape: AT scenario + a
  "Contract test — …" scenario). Confirm the run digest now shows a
  `contract-test-writer` dispatch, a contract test is actually written, and it
  is verified contract-real then contract-stub.
- Confirm **no** spurious `system-driver-adapter-implementer` cycle fires off
  the back of the CT subtree (i.e. the fence held — `system-driver-port-changed`
  did not leak up to the AT cycle's system-driver gate).

## Out of scope

- **Ticket-specified contract scenarios.** `contract-test-writer` deriving the
  contract from the port diff is the chosen model; feeding the ticket's contract
  Gherkin in is a separate enhancement.
- **The sibling-before alternative** (rejected — see Decision).
- **The AT-side `${touches-system-driver}` strip** that already shipped.
- **Any change to the scope-exception envelope mechanism itself.**
- **Diagram regeneration.** `docs/process-diagram.md` + `docs/images/*.svg` are
  auto-regenerated by the GH Actions workflow on push to `main`; do not add a
  local regen step (memory `feedback_plans_no_diagram_regen`).

## References

- `internal/atdd/runtime/statemachine/process-flow.yaml` (lines as of 2026-06-04):
  - `shared-contract` 676–735 — AT cascade; external-driver gate 710–713, the
    node to repoint 715–722.
  - `write-and-verify-acceptance-tests` 757–801 — AT cycle + the system-driver
    gate (799) the leaked flag would wrongly trigger.
  - `implement-and-verify-external-system-driver-adapters` 925–945 — thin AT
    variant to retire (Item 2).
  - CT-HIGH `…-contract-tests` 965–1076 — the orphan to de-orphan; inner
    `IMPLEMENT_AND_VERIFY_DSL` (`tests: contract`) 981–988 is the fenced path.
  - `write-contract-tests` 1440–1477 — `test-names` output source.
  - `implement-dsl` leaf 1479+ — read/write scope incl. `driver-port`.
- `internal/assets/runtime/agents/atdd/dsl-implementer.md` — prompt body (Item 3).
- `internal/atdd/runtime/actions/bindings.go` — output validation (Item 4).
- Memory: `feedback_schema_fields_earn_slot` (why the param was stripped),
  `feedback_statemachine_test_loop_hazard` (test-run safety),
  `feedback_agents_dont_validate_inputs` (fence belongs in runtime, not the
  agent body), `feedback_plans_no_diagram_regen`.
- Related plan: `20260604-1024-backlog-conflict-triage-vs-1725-scoped-implement.md`
  (notes the stale line ranges this plan now re-anchors).
