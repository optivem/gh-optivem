# Make contract-real (and contract-stub) verification outcome-driven

**Created:** 2026-06-06 19:43 (CEDT)
**Supersedes part of:** `plans/20260606-1356-*` (the `real-kind` gate that pins contract-real polarity). This plan does **not** edit that plan; it replaces the polarity-prediction half of it. See [[project_bpmn_full_coverage_story_and_realkind_gap]].

## Context — why

The contract sub-flow `implement-and-verify-external-system-driver-adapters-contract-tests`
(`internal/atdd/runtime/statemachine/process-flow.yaml`, ~lines 1039–1216) currently uses
`GATE_REAL_KIND` to **predict the polarity** of the contract-real test before running it:

- `real-kind == test-instance` → `VERIFY_TESTS_PASS_CONTRACT_REAL` (expects GREEN, line 1201)
- `real-kind == simulator` → `VERIFY_TESTS_FAIL_CONTRACT_REAL` (expects RED, line 1205) → implement simulator → green

Red/green of a contract-real test is a **runtime property of the external system's current state**
("is the plumbing there?"), not statically knowable from `real-kind` or change-type. Both branches
encode an only-sometimes-true assumption, and each traps on the opposite outcome:

- **simulator + additive field on an existing endpoint** → the ERP simulator is `json-server`
  (`external-systems/simulators/mock-server.js`), which is schemaless and **echoes any POSTed field**.
  So the test arrives GREEN, `verify-tests-fail` routes it to `fix-unexpected-passing-tests`, and the
  run halts. **This is exactly what issue #72 ("charge shipping based on product weight") hit** — `weight`
  is an additive field on `/erp/api/products`, so `IMPLEMENT_EXTERNAL_SYSTEM_REAL_SIMULATOR` was
  short-circuited and never ran.
- **test-instance + missing plumbing** → the vendor sandbox does not yet honor the contract → test is
  RED → `verify-tests-pass` routes to `fix-unexpected-failing-tests`, which burns 2 opus·high passes
  trying to "fix" code that **cannot** fix a vendor sandbox → `FIX_LOOP_EXHAUSTED` halt.

New endpoints / new capabilities work today only because `json-server` genuinely 404s on an unknown
route (real red) — the asymmetry is purely "new route vs. additive field on an existing route."

### Vocabulary — probe, not verify

A `verify-tests-pass` / `verify-tests-fail` process is an **assertion**: it pins an expected polarity
and embeds a fixer + retry-cap + halt for the mismatch. The outcome-driven step asserts nothing — it
runs and records the result for the caller to branch on. The primitive for that already exists: it is
`run-tests`, which runs the suite and stamps `test-outcome` (pass / fail / infra) with no assertion and
no fixer (it is literally what both `verify-tests-*` processes wrap). So the new step reuses `run-tests`
directly and the caller gates on `test-outcome`; do **not** reach for `verify-tests-pass`. There is no
expected polarity, so there is no `-pass` / `-fail` in the node name — the node is `PROBE_*` and the
result lives in `test-outcome`.

### Decision

Stop asserting an expected polarity for contract-real. **Run the test, branch on the observed outcome.**
`real-kind` decides *what to do about a red*, not *whether to expect red*:

- **GREEN** → external system already honors the contract → proceed. Legitimate terminal whether the
  cause is the echo simulator, pre-existing plumbing, or a vendor that already supports it.
- **RED + simulator** → we own it → implement the simulator → verify green (fix loop here is legit).
- **RED + test-instance** → we do **not** own it → halt with an "upstream/provider contract gap" message;
  do **not** route to the code-fixer.

**Accepted tradeoff:** we drop the test-first "must be RED first" discipline on contract-real. That
discipline is only sound when *we* write the code that goes red→green; for an external system we may
not control, or that incidentally already satisfies the contract, demanding red-first is theater. The
safety net is unchanged: the acceptance loop still drives the SUT end-to-end through the real ERP path
(green-verify), and `fix-unexpected-passing-tests` still guards vacuous tests wherever `verify-tests-fail`
legitimately remains (e.g. the acceptance RED cycle).

## Scope note — contract-stub is included (please veto if unwanted)

`contract-stub` has the **identical** disease: `ErpStubDriver.returnsProduct` configures WireMock by
echoing the request DTO, so `VERIFY_TESTS_FAIL_CONTRACT_STUB` (line 1154) also arrives GREEN for an
additive field and would trap on the next step after contract-real. Fixing only contract-real leaves
the pipeline broken one node later. Item 2 applies the same transform to the stub leg (simpler — stubs
are always owned, so no `real-kind` branch). If you want contract-stub left alone, drop Item 2.

## Items (agent work)

### Item 1 — contract-real: replace polarity-prediction with outcome probe

In `implement-and-verify-external-system-driver-adapters-contract-tests`:

- **Remove** `GATE_REAL_KIND`, `VERIFY_TESTS_PASS_CONTRACT_REAL`, `VERIFY_TESTS_FAIL_CONTRACT_REAL` and
  their edges (lines ~1100–1121, 1201–1206).
- **Keep** `IMPLEMENT_EXTERNAL_SYSTEM_REAL_SIMULATOR`, `BUILD_SYSTEM_AFTER_SIMULATOR`,
  `START_SYSTEM_AFTER_SIMULATOR`, `VERIFY_TESTS_PASS_CONTRACT_REAL_AFTER_SIMULATOR`.
- **Add** the probe + branch. Target topology:

  ```
  START_SYSTEM_AFTER_DRIVER → PROBE_CONTRACT_REAL
  PROBE_CONTRACT_REAL            (call-activity, process: run-tests, suite: contract-real, test-names: ${test-names})
  PROBE_CONTRACT_REAL → GATE_CONTRACT_REAL_OUTCOME       (gateway, binding: test-outcome)
    test-outcome == pass  → START_SYSTEM_BEFORE_STUB_PROBE   # already honors contract → stub side
    test-outcome == fail  → GATE_CONTRACT_REAL_RED_KIND
    test-outcome == infra → TESTS_INFRA_HALT                 (error-end-event)
    (default)             → UNKNOWN_TESTS_OUTCOME            (error-end-event)
  GATE_CONTRACT_REAL_RED_KIND   (gateway, binding: real-kind)
    real-kind == simulator     → IMPLEMENT_EXTERNAL_SYSTEM_REAL_SIMULATOR (→ build → start →
                                  VERIFY_TESTS_PASS_CONTRACT_REAL_AFTER_SIMULATOR → START_SYSTEM_BEFORE_STUB_PROBE)
    real-kind == test-instance → CONTRACT_REAL_UPSTREAM_GAP_HALT (error-end-event)
  ```

- `CONTRACT_REAL_UPSTREAM_GAP_HALT` name (doubles as BPMN label): something like
  *"Contract-real red on a real instance — upstream/provider has not yet shipped this contract; not fixable in this repo"*.
- Add `TESTS_INFRA_HALT` / `UNKNOWN_TESTS_OUTCOME` error-end-events to this sub-flow (mirror
  `verify-tests-pass`).
- Rewrite the comment block at lines ~1095–1099 to describe the outcome-driven model (delete the
  "test-instance ⇒ GREEN / simulator ⇒ RED" framing).

### Item 2 — contract-stub: same outcome probe (no real-kind branch)

- **Remove** `VERIFY_TESTS_FAIL_CONTRACT_STUB` (line 1154) and its edge.
- Rename `START_SYSTEM_BEFORE_STUB_FAIL` → `START_SYSTEM_BEFORE_STUB_PROBE`.
- Target topology:

  ```
  START_SYSTEM_BEFORE_STUB_PROBE → PROBE_CONTRACT_STUB
  PROBE_CONTRACT_STUB            (call-activity, process: run-tests, suite: contract-stub, test-names: ${test-names})
  PROBE_CONTRACT_STUB → GATE_CONTRACT_STUB_OUTCOME       (gateway, binding: test-outcome)
    test-outcome == pass  → IMPL_EXT_DRIVER_CT_END           # already honors → done
    test-outcome == fail  → IMPLEMENT_EXTERNAL_SYSTEM_STUBS  (→ build → start → VERIFY_TESTS_PASS_CONTRACT_STUB → END)
    test-outcome == infra → TESTS_INFRA_HALT
    (default)             → UNKNOWN_TESTS_OUTCOME
  ```

### Item 3 — update statemachine tests / fixtures for the new topology

- Find tests/golden fixtures asserting this sub-flow's node IDs and edges (grep for
  `GATE_REAL_KIND`, `VERIFY_TESTS_FAIL_CONTRACT_REAL`, `VERIFY_TESTS_PASS_CONTRACT_REAL`,
  `VERIFY_TESTS_FAIL_CONTRACT_STUB`, `START_SYSTEM_BEFORE_STUB_FAIL` across `internal/atdd/**`,
  `**/*_test.go`, and any `testdata`/golden trace fixtures). Update to the new node set.
- **Hazard:** per [[feedback_statemachine_test_loop_hazard]], audit any gate/loop fixtures before
  running statemachine tests and watch RAM — the new back-edge from
  `VERIFY_TESTS_PASS_CONTRACT_REAL_AFTER_SIMULATOR` already existed, but reconfirm no new self-loop was
  introduced. Run scoped (`-p 2` / single package), never unbounded `go test ./...`
  ([[feedback_go_test_windows]]).

### Item 4 — align prose docs

- Sweep `docs/atdd/**` and `docs/bpmn-process-design.md` for the "contract-real RED until we author the
  simulator" / "test-instance already honors → GREEN" narrative and rewrite to outcome-driven.
- Do **not** touch `docs/process-diagram.md` or `docs/images/process-diagram-19-*.svg` — the
  regenerate-diagram GH Action rebuilds these on push to main ([[feedback_plans_no_diagram_regen]]).

## Verification (after agent work)

- Re-run the #72 rehearsal (`rehearsal-…-72-charge-shipping-based-on-product-weight`) and confirm
  contract-real now goes GREEN → straight to the stub leg → no `fix-unexpected-passing-tests` halt.
- Confirm a hand-constructed **new-endpoint** contract change still gets a real RED → simulator impl →
  green (the path we must not regress). A quick way: temporarily point a contract test at an unmapped
  route and confirm `PROBE_CONTRACT_REAL` → fail → simulator branch.
- (User) Decide whether the eventual `real-kind == test-instance` shop coverage gap is worth closing
  now or left as a known gap — unchanged by this plan, but the test-instance halt path is newly
  exercised by it.
