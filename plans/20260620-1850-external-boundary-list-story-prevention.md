# 2026-06-20 16:50:27 UTC — Prevent list-shaped external-boundary stories from halting at STOP_SCOPE_VIOLATION

## TL;DR

**Why:** A list/collection acceptance story backed by an external system (e.g. #65 "View product list", which proxies the catalog from ERP) gets staged on the per-SKU stub primitive, so the external-contract/stub room is gate-skipped, the list stub is never built, and `system-implementer` correctly refuses (scope-exception → `STOP_SCOPE_VIOLATION`). The whole rehearsal halts.
**End result:** A ticket carries an **External System Contract Criteria** section (alongside Acceptance Criteria) that names each external system exactly and states its boundary behaviour in Given/Then form, with two registers — **shared** (stub + real, containment) and **stub-only** (fidelity: exact-set + empty). `parse-ticket` reads only its **presence + exact system name(s)** (it stays dumb — the Given/Then bodies pass through verbatim); that presence opens the contract/stub room (overruling the chain-of-checks if that would have skipped it). The two registers are authored by **two writers** — the existing `contract-test-writer` (shared register, by-key invariant unchanged, both drivers) and a new `stub-fidelity-test-writer` (stub-only register, exact-set + empty, stub only) — so both registers + the stub get built and verified green **before** `system-implementer` runs. The boundary is proven faithful, the production read-path has a stub to talk to, and the run never halts on this class of scope-violation. Guard **B** (a loud ticket-level hard fail) catches any story that forgot to declare External System Contract Criteria.

**How it lands (refined 2026-06-20, "long-term cleanest"):** the existing external contract/stub room — *already* a sub-section of `shared-contract`, before system implementation — is kept **in place** (no phase reorder, `channels.go` untouched). Only its **entry** changes: one room, one explicit signal — enter when `ticket-has-escc` (with the brittle `at-external-driver-port-changed` proxy demoted to a fallback for non-ESCC tickets). The new `stub-fidelity-test-writer` rides **inside the per-system callee** that the existing unroll already clones; `external.go` resolves the touched-system set by **precedence** (`escc-systems` when declared, else port-change paths). Slice B = 4 files (`process-flow.yaml`, `external.go`, `gates/bindings.go`, `contract-test-writer.md`).

## Outcomes

What we get out of this — the goals and deliverables:

- A list-shaped, externally-backed story (the #65 class) walks past `IMPLEMENT_AND_VERIFY_SYSTEM_API` without a `STOP_SCOPE_VIOLATION` — the contract test and list stub for the new external operation exist by the time production code is written.
- Tickets gain an **External System Contract Criteria** section: per-external-system (named exactly, e.g. `External System: ERP`), Given/Then boundary behaviour with two registers — **shared** (stub + real, weak/containment) and **stub-only** (strong fidelity: exact-set + empty) — giving the inner/contract loop its own spec, symmetric to how Acceptance Criteria spec the outer/acceptance loop.
- The decision "this story touches a new external-system boundary" is made from an **explicit, reviewable declaration** (the presence of an External System Contract Criteria block), not inferred from the brittle `external-driver-port-changed` file-change proxy that silently skips the contract/stub room. The exact system name routes to the right boundary and feeds the existing `VALIDATE_EXTERNAL_SYSTEMS_REGISTERED` gate.
- Red **contract tests**, written *from* the External System Contract Criteria by two writers — `contract-test-writer` (shared/containment, both drivers) and the new `stub-fidelity-test-writer` (stub-only/exact-set + empty) — drive the stub into existence (proper ATDD double-loop), so the stub is never an after-the-fact patch.
- **Boundary verified first, with clean failure localization.** The stub is proven *faithful to what we stage* (stub-only fidelity register) before any system/acceptance code relies on it — so an acceptance failure always means "the system is wrong," never "the stub silently lied." Stub-broken, stub/real-disagree, and system-proxy-broken become three distinct signals.
- Friction stays on external-boundary stories only: no External System Contract Criteria → the current chain runs unchanged. A story that forgets its External System Contract Criteria is still caught by guard **B** (a loud ticket-level hard fail with an actionable "add ESCC" message).
- The fix is systemic (catches the class for every future external-boundary story), not a one-off hand-edit of the #65 corpus story.

## ▶ Next executable step (resume here)

**The #65 halt is fixed.** DONE so far: Slice A (intake + `parse-ticket`) and, this session, **Step 2** (ESCC-first room-entry routing + `ticket-has-escc` binding + `external.go` case-insensitive precedence) and the **`contract-test-writer` half of Step 3** (consumes the verbatim ESCC Shared register). With ESCC on the #65 ticket, the room now opens, the containment contract test drives the list stub into existence, and `system-implementer` no longer halts.

**Next move = execute the already-drafted follow-up plan** for the two deferred pieces. That plan — **`plans/20260620-2348-stub-fidelity-writer-and-guard-b.md`** — is written with both blocking design decisions pre-resolved in its `## Open questions` (separate stub-only test-names key + reuse `contract-stub` suite; Guard-B path families = union of the contract/stub writers' write-scopes). It covers:
1. **`stub-fidelity-test-writer`** (Stub-only / exact-set + empty fidelity register) — Slice 1 there. ✅ **DONE & committed 2026-06-22** (the new writer + `write-stub-fidelity-tests` process + stub-only probe/verify leg + `ct-isolated-test-names` key + `stub-fidelity-tests-present` presence gate all shipped; scoped tests green). **Do not resume this parent until the child plan is fully done** — its remaining work IS this parent's deferral.
2. **Guard B** (loud "contract needed but undeclared" halt) — Slice 2 there. ⏳ remaining.

Then the operator items: **Step 5** (add `## External System Contract Criteria` to GitHub issue #65 in the `shop` repo) and **Step 6** (re-run the #65 rehearsal, multitier-java) — also carried in the follow-up plan's operator hand-off.

Re-enter with `/clear` then `/execute-plan plans/20260620-2348-stub-fidelity-writer-and-guard-b.md` (confirm its 4 pre-filled answers first, or `/refine-plan` to adjust them), or run Step 5/6 directly.

## Root cause (pinned)

Halt: `change-system-behavior → IMPLEMENT_AND_VERIFY_SYSTEM_API → EXECUTE_AGENT → STOP_SCOPE_VIOLATION` (`internal/atdd/process/process-flow.yaml:2815 → 2793`).

1. AC is list-shaped ("the response contains 3 products"); MyShop owns no products table, so production proxies the catalog: `ProductService.java:17` → `ErpGateway.getProducts()` → `GET /erp/api/products` (a **list** endpoint).
2. `dsl-implementer` staged `Given … products` on the **existing per-SKU** primitive — `GivenProductImpl.java:38` (`app.erp().returnsProduct()`) backed by `ErpStubClient.java:21-25` (`configureGetProduct` registers only `GET /erp/api/products/{sku}`). It added **no new external-driver port method** → emitted `at-external-driver-port-changed=false`.
3. `GATE_EXTERNAL_DRIVER_PORTS_CHANGED == false` (`process-flow.yaml:911`) jumps to `SHARED_CONTRACT_END`, **skipping the entire external contract/stub room** — both `write-contract-tests` / `contract-test-writer` (`process-flow.yaml:1894-1916`) and `external-system-stub-implementer` (`process-flow.yaml:2177`). So no list stub and no list contract test are ever built.
4. `system-implementer` needs the list stub, is read-only on the external stub adapter (its scope block), emits the scope-exception → halt. The refusal is **correct**; the scope-exception → halt is **deliberate** (`process-flow.yaml:2710-2718`). The defect is upstream: the room got skipped.

The linchpin is that the gate keys on "did a port **file** change," but a genuinely-new external **endpoint** (list, reusing the existing DTO shape + path prefix) is needed without any port-file change. (Same hazard as `[[feedback_port_changed_flags_directory_keyed]]`.)

Prior art: `plans/deferred/20260613-1950-view-product-list-erp-list-gap.md` diagnosed this on the **monolith** variant (2026-06-11) and flagged the multitier replication as pending — that pending case is what just halted. That plan's Option A is a `shop`-content fix; this plan is the systemic gh-optivem prevention (its Option B, generalized).

## Decided shape: A (primary) + B (loud backstop)

The recommendation is settled: **A** (External System Contract Criteria on the ticket) is the primary mechanism; **B** is the safety net — and B is a **fail-loud guard**, not silent agent recovery. **C** is optional defense-in-depth, deferred unless A+B prove insufficient. (There is **no clean command-layer fix** — `validate-outputs-and-scopes` surfaced the exception correctly; nothing is mis-classified.)

- **A — External System Contract Criteria ticket section (primary).** Tickets gain a `## External System Contract Criteria` section alongside `## Acceptance Criteria`. Per external system, named **exactly** (`External System: ERP`), in Given/Then form:
  ```
  ## External System Contract Criteria
  External System: ERP
    Shared (stub + real):
      Given products Apple (1.00), Bread (2.50)
      Then ERP has products Apple (1.00), Bread (2.50)        # containment
    Stub only:
      Given products Apple (1.00), Bread (2.50)
      Then ERP has exactly products Apple (1.00), Bread (2.50)
      Given no products
      Then ERP has no products
  ```
  This gives the **inner/contract loop its own spec**, symmetric to how Acceptance Criteria spec the outer/acceptance loop. The section's **presence** is the routing signal: `parse-ticket` reads it and **opens the contract/stub room** (overruling the `GATE_EXTERNAL_DRIVER_PORTS_CHANGED` cascade when that would have skipped it). The **exact system name** routes to the right boundary and feeds the existing `VALIDATE_EXTERNAL_SYSTEMS_REGISTERED` gate (`process-flow.yaml:912`). The `contract-test-writer` then writes the contract test **from** the External System Contract Criteria (not inferred). When **absent**, the current cascade runs unchanged — friction lands only on external-boundary stories. Aligns with `[[feedback_agents_dont_validate_inputs]]` (boundary decisions belong in ticket parsing / upstream gates) and `[[feedback_acceptance_tests_accuracy_over_speed]]` (the contract test gets a real source of truth). Files: ticket schema/parse + `parse-ticket` service-task; `process-flow.yaml` gating around `GATE_EXTERNAL_DRIVER_PORTS_CHANGED` (`:905-913`).

  **Scope of ESCC — two registers: shared (weak) + stub-only (strong/fidelity).** ESCC is **Given/Then only** (no `When` — the contract has no actor action; the `When` belongs to the AC), and carries up to two registers per external system:
  - **Shared (stub + real).** Weak/containment: "ERP *has* products A, B with id + price." Runs against both drivers, so it can only assert what holds for both (the real ERP's full catalog isn't controlled — `ErpRealDriver.returnsProduct` *creates* via POST; the real may hold other products and can never be made empty). Verifies the stub and real **agree** on the operation + shape.
  - **Stub-only (strong/fidelity).** "ERP has *exactly* A, B"; "ERP has *no* products." Runs against the stub only (fully controlled). Verifies the stub is **faithful to what we staged** — incl. exact-set and empty.

  Why the stub-only register is **not** redundant with the Acceptance Criteria: acceptance tests verify the **system's output**, and in general do **not** re-verify that the external state matches what the `Given` staged (e.g. `place order` asserts an order total, never "the ERP actually holds SKU-1"). #65 only *looks* like an exception because its feature echoes the catalog. So if the (non-trivial, aggregating) list stub does setup wrong, an acceptance test fails **without telling us why** — the precondition silently lied. The stub-only register closes that gap and gives clean **failure localization**: stub-only red → stub is broken; shared red → stub/real disagree; acceptance red with both green → the system's proxy/mapping is broken. The stub-only register is warranted when stub fidelity is **non-trivial and otherwise unverified** (collections, empty, exact-set — the #65 class); for a trivial per-SKU stub it is optional. The `Then` pins the **shape** the feature depends on (`id + price`), not the full external payload.

  **Ordering — boundary first.** The external boundary is a **foundation**: both registers must be authored and verified **green** (stub built + proven faithful, stub/real agreed) **before** the system code or acceptance loop relies on it. This is the inner/contract loop solidified beneath the outer/acceptance loop. The flow already places the contract/stub room ahead of system implementation, so the work is to **stop skipping it** (the A routing fix) and **add the stub-fidelity register** to it — not to reorder phases.
- **B — Fail-loud "contract needed but undeclared" guard (backstop).** A story that *should* carry External System Contract Criteria but doesn't must fail **loud and actionable**, not silently. When the flow reaches a point where a contract/stub is genuinely needed but **no External System Contract Criteria was declared** (the present-day signal for this is the `system-implementer` scope-exception naming external contract/stub files), the halt message becomes a clear, teaching error — *"this story needs a contract for `<ERP>` but declares no External System Contract Criteria; add a `## External System Contract Criteria` section and re-run"* — instead of the cryptic generic `STOP_SCOPE_VIOLATION`. This keeps the **acceptance loop simple** (it doesn't try to *infer* list-shaped stories; it just enforces "External System Contract Criteria present when a contract is needed") and is correctly **fail-loud on malformed input** rather than masking an incomplete ticket with auto-recovery. Files: the scope-exception halt routing/message (`process-flow.yaml:2786-2795` + `validate-outputs-and-scopes` categorization of the named files). B is an ordinary **ticket-level hard fail** (not a special soft-skip) — corpus continuation stays the rehearsal loop's job via its existing `--continue-on-failure` flag. Detection stays at the existing late scope-exception (no new early inference). Cost: a broken ticket still burns one `system-implementer` run before halting — acceptable (rare once tickets carry ESCC).
- **C — BPMN gate broadening (optional, deferred).** Stop `GATE_EXTERNAL_DRIVER_PORTS_CHANGED` (`process-flow.yaml:910-911`) from skipping the contract/stub room purely on "no port file changed," via a distinct signal independent of the ticket declaration. Only pursue if A+B leave a real gap. Files: `internal/atdd/process/process-flow.yaml` + the new signal's producer.

> **Note — superseded idea.** An earlier draft of B had the `dsl-implementer` agent *silently* build an enumerable source when it spotted a list-shaped AC. Dropped: silent recovery masks the incomplete ticket and relies on a fragile mid-flow inference. The fail-loud guard above is preferred — it pushes the fix back to the ticket (where A wants it) and teaches correct authoring.

## Flow — two sequential loops (not nested)

> **⚠ SUPERSEDED (refined 2026-06-20).** This section's "two sequential loops, Phase 1 commits before Phase 2" model was **dropped** as not the long-term-cleanest shape (see Resolved decisions → "Phase ordering"). The retained design keeps the **single existing contract room inside `shared-contract`** (already positioned before system implementation) and only fixes its **entry routing** (open on explicit ESCC, port-proxy as fallback). The "boundary proven before the system relies on it" guarantee is delivered by the **stub-only fidelity register inside that room** (greens before system impl), not by a separate committed phase. The diagram below is kept for rationale/history only — it does **not** describe the implementation.

The boundary is established as **its own loop that greens and commits before the acceptance test is written** — not nested under the acceptance loop.

```
PHASE 1 — contract loop (from ESCC)          PHASE 2 — acceptance loop (from AC)
┌─────────────────────────────────────┐      ┌─────────────────────────────────────┐
│ write ALL contract tests for the     │      │ write acceptance test        → RED   │
│ ticket's declared boundary:          │      │ implement DSL + system-driver        │
│   • shared register (stub + real)    │ then │   adapters + system                  │
│   • stub-only register (fidelity)    │ ───► │   (on the green, committed boundary) │
│                               → RED   │      │                                      │
│ build stub (+ real/simulator) → GREEN│      │ acceptance test              → GREEN │
│ commit: boundary proven              │      │ commit: feature done                 │
└─────────────────────────────────────┘      └─────────────────────────────────────┘
```

- **Scope of Phase 1** = the boundary capability **this ticket's ESCC declares** (not every contract test globally), greened on **both** drivers — incl. the real/simulator side (for #65 the simulator already serves the list, so no real-side work).
- **Why sequential, not nested:** ESCC gives the contract its own spec, so Phase 1 no longer needs the acceptance test to *discover* what to build (the only reason nesting exists). Payoff: unambiguous failure (a red AT in Phase 2 can only mean the system, never the stub), a reusable committed boundary, and two visible/teachable commits.
- **Caveat (discovery):** sequential assumes ESCC is complete. If Phase 2 reveals an undeclared boundary need (e.g. the feature needs product *name*, ESCC declared only `id + price`), revise the ESCC and re-run Phase 1 — the guard-B path. The price of going explicit; fine for a specified corpus.
- **Execution implication — this is a flow REORDER.** Today the acceptance test is authored first (`WRITE_AND_VERIFY_ACCEPTANCE_TESTS_FAIL` early in `implement-ticket`), with the contract/stub room interleaved after. The sequential model moves the contract loop **ahead** of acceptance-test authoring, with its own green + commit. That is a larger change to the top-level `implement-ticket` ordering than "stop skipping the room" — call it out for execution and confirm against `process-flow.yaml`.

### Logic

Top-level `implement-ticket`:

```
parse-ticket(ticket)                         # → Acceptance Criteria + ESCC (named systems)

# PHASE 1 — CONTRACT LOOP (only if ESCC declared)
if ticket.has_ESCC:
    validate-external-systems-registered(ESCC.systems)   # fail loud on unknown name
    for system S in ESCC.systems:
        write-contract-tests(S, from=ESCC)   # shared + stub-only registers → RED
        repeat until GREEN (cap N):
            implement-external-stub(S)        # + real/simulator if not present
            run-contract-suite(S)             # shared: stub & real agree;
                                              # stub-only: stub faithful to staging
        commit("boundary proven: " + S)       # Phase 1 closes GREEN + committed

# PHASE 2 — ACCEPTANCE LOOP
write-acceptance-tests(from=AC)              # → RED
repeat until GREEN (cap N):
    implement-dsl()
    for channel C in touched_channels:
        implement-system-driver-adapters(C)
        implement-system(C)                   # production code, on the GREEN boundary
    run-acceptance-suite()
commit("feature done")
```

Gateway change (the #65 fix — footprint cascade stops being the only entry):

```
# OLD — footprint-derived (skipped #65):
if at-external-driver-port-changed: enter contract room   else: skip   # ← #65 wrongly skipped

# NEW — ESCC-driven, cascade kept as fallback:
if ticket.has_ESCC:                 run Phase 1 contract loop   # override, cannot skip
elif at-external-driver-port-changed: enter contract room      # cascade still serves the 90%
else:                               skip                       # legitimately nothing external
```

Guard B (loud backstop, at the system step):

```
on system-implementer scope-exception(files, reason):
    if files under external contract/stub paths AND not ticket.has_ESCC:
        HARD FAIL: "needs External System Contract Criteria for <S> — add it and re-run"
    else:
        STOP_SCOPE_VIOLATION        # existing generic halt, unchanged
```

## Steps

> **Step 2 (A) — DONE (2026-06-20, this session).** Room entry restructured to `GATE_TICKET_HAS_ESCC` → `GATE_EXTERNAL_DRIVER_PORTS_CHANGED` (ESCC checked first, port-changed demoted to fallback; both true-branches → `VALIDATE_EXTERNAL_SYSTEMS_REGISTERED`) in `process-flow.yaml`; `ticket-has-escc` binding registered in `gates/bindings.go` (+ test want-list); `external.go` resolves the touched-system set by precedence (`escc-systems` when `ticket-has-escc`, else port-change paths) — **case-insensitively**, since tickets read `External System: ERP` while the registry key is lowercase `erp`. Source-aware `validate-external-systems-registered` error message. Tests green.
>
> **Step 3 (A) — `contract-test-writer` half DONE (2026-06-20, this session).** `contract-test-writer.md` gained an optional "External System Contract Criteria" input that authors the **Shared (stub + real)** containment register from the verbatim `${external-system-contract-criteria}` body (by-key invariant unchanged); the body is wired in via a `write-contract-tests` node param + seeded in the render-matrix tests. This alone removes the #65 halt: the room opens on ESCC and the containment contract test drives the list stub into existence before `system-implementer`.

> **Step 3 (A, remainder) — DONE (2026-06-22), via `plans/20260620-2348-…` Slice 1.** The new **`stub-fidelity-test-writer`** agent (Stub-only / exact-set + empty register, `<Sys>StubContractIsolatedTest`, `@Isolated`) + its `write-stub-fidelity-tests` process + in-callee stub-only probe/verify leg shipped. The test-names collision was resolved by giving the isolated register its own landing key: `isolated-test-names` → `ct-isolated-test-names` (`namespacedLandingKeys`), distinct from the shared `ct-test-names`, so the stub-only tests never reach `PROBE_CONTRACT_REAL`. Gated by a `stub-fidelity-tests-present` presence binding so the optional register doesn't crash strict `ExpandParams`.
> **Step 4 (B) — DONE (2026-06-22), via `plans/20260620-2348-…` Slice 2.** The "contract needed but undeclared" path now fails loud: `categorize-scope-exception` resolves the contract/stub Family-B path families (`ct-test`, `dsl-port`, `dsl-core`, `external-system-driver-adapter`; shared `common`/`system-driver-adapter-shared` excluded so unrelated scope-exceptions aren't mis-categorized) and stamps `scope-exception-needs-escc`; the new `GATE_SCOPE_EXCEPTION_NEEDS_ESCC` reroutes a contract/stub exception on an ESCC-less ticket to `ESCC_UNDECLARED_HALT` (system-named, actionable stderr guidance) instead of the generic `STOP_SCOPE_VIOLATION`. The child plan is deleted (all agent work done); Steps 5–6 below are the only remaining work.

- [ ] Step 5: Add `## External System Contract Criteria` to the #65 corpus ticket (ERP list) so the corpus exercises the new path. *(Operator-driven — edits GitHub issue #65 in the `shop` repo.)*
- [ ] Step 6: Re-run the #65 rehearsal (`scripts/atdd-rehearsal.sh 65 …`, multitier-java) and confirm it walks past `IMPLEMENT_AND_VERIFY_SYSTEM_API` without `STOP_SCOPE_VIOLATION`, the list contract test + stub exist, and the per-SKU stories still pass. *(Operator-driven verification.)*

## Resolved decisions

- **ESCC parse depth → presence + system names only (parse-ticket stays dumb).** `parse-ticket` detects the `## External System Contract Criteria` block, extracts each `External System: <name>`, emits the "contract-needed" signal, and feeds the named system(s) into `VALIDATE_EXTERNAL_SYSTEMS_REGISTERED`. It does **not** structurally parse Given/Then — the register bodies pass through **verbatim** to `contract-test-writer`, which is the sole interpreter of the criteria. Rationale: routing only needs presence + exact name; structural parsing in the parser would duplicate interpretation and couple the parser to the criteria grammar (`[[feedback_agents_dont_validate_inputs]]`). Implication for Step 2: the parse output is `{contract_needed: bool, systems: [name], escc_body: <verbatim text per system>}`; Step 3's `contract-test-writer` consumes `escc_body`.
- **Canonical register keywords → the plan's prose verbs (pinned).** `contract-test-writer` (the sole interpreter, per the parse-depth decision) keys on this fixed vocabulary; students author it directly:
  - Register sub-headers: `Shared (stub + real):` and `Stub only:`.
  - `Given products <A> (<price>), <B> (<price>)` — stage these products; `Given no products` — stage empty.
  - `Then <System> has products <…>` → **containment** assertion (shared register; weak).
  - `Then <System> has exactly products <…>` → **exact-set** assertion (stub-only register; fidelity).
  - `Then <System> has no products` → **empty** assertion (stub-only register; fidelity).
  The verb (`has` / `has exactly` / `has no`) is the assertion-kind signal; the sub-header is the driver-set signal. No machine tags/comments are required in the ticket — the inline `# containment` notes in the format block are illustrative, not parsed.
- **Register → test artifacts → two writers (not one).** The two registers carry **contradictory invariants** and **different run targets**, so they are authored by **two separate agents**:
  - **Shared register → existing `contract-test-writer`, invariant unchanged.** Runs against **both drivers** (stub + real); keeps its current absolute invariant intact — assert by key, containment only, *never* exact-set/empty/whole-collection (the rule that prevents fix-loop exhaustion against the shared, non-reset real system, `contract-test-writer.md:22-24`). It authors only the **Shared (stub + real)** register from ESCC.
  - **Stub-only register → new `stub-fidelity-test-writer`.** Runs against the **stub only** (fully controlled); its own clean absolute invariant — assert **exact-set** (`has exactly products`) and **empty** (`has no products`) — verifying the stub is faithful to what we staged. Authored only when the ticket declares a `Stub only:` register.
  - Rationale for splitting rather than making `contract-test-writer`'s invariant register-conditional: negating that load-bearing safety rule conditionally is the riskiest possible prompt edit and dilutes the protection on the 90% (non-list) path. Two agents = two coherent, non-conditional invariants — same narrow-agent pattern as `external-system-stub-implementer` / `external-system-real-simulator-implementer`. (`[[feedback_acceptance_tests_accuracy_over_speed]]`: fidelity over saving one agent.) Both writers read the same verbatim `escc_body`; the new agent needs its own EXECUTE_AGENT node, service-task, scope block, and `${expected-outputs}`. Name `stub-fidelity-test-writer` is pinned (describes scope: proves stub fidelity); executor may finalize if a clearly-better scope name surfaces, no layer-coding (`[[feedback_no_layer_coding_in_names]]`).
- **Hard fail vs. soft → hard fail (ticket level).** B is an ordinary **ticket-level hard fail** with an excellent message — not a special "soft skip." Corpus continuation is *not* B's concern: the rehearsal loop's existing `--continue-on-failure` flag already governs whether a halted ticket stops the loop or rolls on to the next. Building a soft-skip into B would duplicate that machinery and make B behave unlike every other failure.
- **B's detection point → reframe the existing late halt.** Keep the present detection point (the `system-implementer` scope-exception); do **not** add an earlier signal — early detection would require the same unreliable "is this externally-backed?" inference this plan removes. Cost: a broken ticket still burns one `system-implementer` run before halting; acceptable (rare once tickets carry ESCC).
- **Authoring → manual now.** Corpus tickets carry ESCC explicitly; the `acceptance-criteria-refiner` auto-surface hook is a future enhancement, out of scope.
- **ESCC has two registers → shared + stub-only.** Shared (stub + real): containment + shape (`id + price`). Stub-only (stub fidelity): exact-set + empty. The stub-only register is *not* redundant with the AC — acceptance tests verify system output, not external-state-setup fidelity — and gives clean failure localization. Stub-only is warranted when stub fidelity is non-trivial/otherwise-unverified (the list class); optional for trivial per-SKU stubs.
- **Boundary-first ordering.** Both registers must be authored and verified green (stub built + proven faithful) before system code / the acceptance loop relies on the boundary. Already the flow's order; the fix is to stop skipping the room and add the stub-fidelity register.

- **Human specifies; agents build → no hand-authored `shop` code.** The human's *only* authoring is the **External System Contract Criteria on the ticket** (declare the boundary). **Every line of code is agent-authored during the run:** `external-system-stub-implementer` → list stub; dsl/external-driver implementer → `returnsProducts` step; `contract-test-writer` → shared + stub-only contract tests; `system-implementer` → `ErpGateway.getProducts` + `ProductController`. The real ERP simulator already serves `GET /erp/api/products`, so the **shared** register greens against both drivers. So greening #65 needs **no** `shop`-content fix — only the ESCC on the #65 ticket (Step 5). Consequently the deferred plan `plans/deferred/20260613-1950-view-product-list-erp-list-gap.md` (Option A, manual hand-authoring) is **superseded** by this orchestration fix; archive it once #65 greens.

- **Phase ordering → no reorder; one room, one explicit entry (refined 2026-06-20, "long-term cleanest").** The "two sequential loops, Phase 1 commits before Phase 2" model is **dropped**. Grounding (read of `process-flow.yaml:727-918`): the external contract/stub room is *already* a sub-section of `shared-contract` (`GATE_EXTERNAL_DRIVER_PORTS_CHANGED :807` → unroll anchor `:850`), and `shared-contract` runs *before* system implementation (which lives later, in `change-system-behavior`). So the room is already ahead of the code that the #65 halt blamed; it skips only because of the gate, not the ordering. A true phase reorder would force the room to be reachable from **two** places (early-for-ESCC + nested-for-cascade, because `at-external-driver-port-changed` is *produced by* the DSL step *inside* the cascade) — duplication, dual entry points, and a `channels.go` re-anchor. The clean long-term shape is the opposite: **one room, one explicit entry signal** (ESCC presence, with the brittle port-changed proxy demoted to fallback and eventually retired via deferred Option C). The "boundary proven faithful before the system relies on it" guarantee is delivered by the **stub-only fidelity register greening inside the room** (Step 3), not by a separate committed phase. Supersedes the "Flow — two sequential loops" section and drops Step 3b.
- **Slice B Go scope (refined 2026-06-20) → 4 files, `channels.go` out.** Pinned after reading the engine:
  - **`channels.go` — untouched.** `UnrollExternalSystems` clones the callee process `implement-and-verify-external-system-driver-adapters-contract-tests` per external system, anchored on `shared-contract` + `IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS` *by name*. The new `stub-fidelity-test-writer` node is added **inside that callee** (alongside `WRITE_CONTRACT_TESTS`, `:1206`), so the existing unroll clones it per system for free — no re-anchor needed.
  - **`external.go` — precedence, not union.** `resolve-external-system` + `validate-external-systems-registered` derive the touched-system set as `escc-systems` **when `ticket-has-escc`**, else `externalSystemNamesFromChangedPaths(external-driver-port-changed-paths)`. The explicit declaration is authoritative when present; an undeclared port change is *not* silently merged in — it surfaces via guard B (which is exactly guard B's job). Both actions read `escc-systems` from State (already stamped by `parse-ticket`/`tracker.go:118-120`).
  - **`gates/bindings.go` — register `ticket-has-escc`.** A `boolStateGate(ctx, "ticket-has-escc")` reader (the state is already produced by Slice A's `parse-ticket`); add `"ticket-has-escc"` to `TestRegisterAll_AllBindingsRegistered`'s `want` list; land it *with* its YAML gateway so the registered-vs-referenced startup cross-check (`Engine.Bind`) stays green.
  - **`process-flow.yaml` + `contract-test-writer.md`** — the room entry-gate OR-condition, the new in-callee `stub-fidelity-test-writer` node (+ scope block + `${expected-outputs}`), guard-B message, and wiring `${external-system-contract-criteria}` + `escc-format.md` into both writer prompts.
