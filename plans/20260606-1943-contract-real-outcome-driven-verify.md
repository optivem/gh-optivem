# Make contract-real (and contract-stub) verification outcome-driven

## TL;DR

**Why:** Contract-real/-stub verification statically predicts test polarity from `real-kind`, but red/green is a runtime property of the external system's state; the echo simulator makes additive-field tests arrive GREEN where RED is expected (this halted issue #72), and a missing-plumbing real instance burns fix-loop passes trying to fix code it can't.
**End result:** Both contract legs run the test and branch on the observed `test-outcome` (a `PROBE_*` step over `run-tests`) instead of asserting a predicted polarity — GREEN proceeds, RED+simulator implements the simulator, RED+test-instance halts with an upstream-gap message; statemachine tests/fixtures and prose docs match the new topology.

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

## Verification (after agent work)

- Re-run the #72 rehearsal (`rehearsal-…-72-charge-shipping-based-on-product-weight`) and confirm
  contract-real now goes GREEN → straight to the stub leg → no `fix-unexpected-passing-tests` halt.
- Confirm a hand-constructed **new-endpoint** contract change still gets a real RED → simulator impl →
  green (the path we must not regress). A quick way: temporarily point a contract test at an unmapped
  route and confirm `PROBE_CONTRACT_REAL` → fail → simulator branch.
- (User) Decide whether the eventual `real-kind == test-instance` shop coverage gap is worth closing
  now or left as a known gap — unchanged by this plan, but the test-instance halt path is newly
  exercised by it.
