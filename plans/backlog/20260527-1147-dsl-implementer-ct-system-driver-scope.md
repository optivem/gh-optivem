# dsl-implementer on the CT path — System Driver scope concern

> **Verdict (2026-06-04 review): KEEP — live deferred concern.** Verified the CT-side HIGH process still exists in `process-flow.yaml` and is still the documented orphan, so the misbehaviour can't manifest yet — confirming the "defer until the CT call site lands" recommendation. The concern remains unaddressed in code; revisit when the CT call site lands and apply Option A+D.
>
> **Does the current process involve CT? No — the CT path is defined but not wired into the live call graph.** The CT-side HIGH `implement-and-verify-external-system-driver-adapters-contract-tests` exists (process-flow.yaml:965, full contract-real / contract-stub verify split at 1007–1076) but its header comment (952–964) still declares it an intentional orphan — *"No other process calls this HIGH today."* The live cascade is AT-only: `change-system-behavior` → `write-and-verify-acceptance-tests` → `implement-and-verify-external-system-driver-adapters` (the AT variant, no `-contract-tests` suffix). The intended CT wiring (Q31.a, Option A nested) is unresolved and deferred to Phase D. So the bug's precondition — a live CT call site reaching `dsl-implementer` — does not exist today.

## TL;DR

**Why:** An emergency fix stripped the `${touches-system-driver}` parameter from `dsl-implementer.md` to stop a placeholder leak, but that leaves the agent with no signal of which call path it's on. On the CT (contract-test) path it could now wrongly add System Driver prototype methods, emit `system-driver-port-changed: true`, and trigger a downstream adapter cycle the CT-path BPMN never contemplates.
**End result:** A decision (and, when warranted, an implementation) that prevents CT-path `dsl-implementer` from touching the System Driver port — the recommendation being to defer until the CT-side call site lands, then apply Option A+D (prompt guidance + a runtime output-flag invariant check) as the lowest-friction backstop.

## Context

Earlier today (2026-05-27 ~11:43) an ATDD rehearsal dispatch of
`dsl-implementer` failed with:

```
clauderun: prompt has unfilled placeholders after substitution: ${touches-system-driver}
```

The prompt body of `dsl-implementer.md` referenced
`${touches-system-driver}` as a switchable "is the System Driver port
in scope for this invocation?" parameter, but no caller in
`process-flow.yaml` ever bound it. The parameter existed to gate
behaviour on two call paths:

- **AT path** — `write-and-verify-acceptance-tests` → `implement-and-verify-dsl`.
  The AT side legitimately needs new System Driver prototypes to
  compile new ATs, so the parameter would be `true`.
- **CT path** — `implement-and-verify-external-system-driver-adapters-contract-tests`
  → `implement-and-verify-dsl`. Contract tests stimulate the
  *External*-System Driver, not the System Driver — the System Driver
  port is conceptually out of scope on this path.

The immediate fix (committed in this session) was to **strip the
parameter entirely** from `dsl-implementer.md` (Parameters section,
the conditional in step 2(a), and the table-cell footnote) on the
basis that `dsl-implementer` should always be allowed to add System
Driver prototype methods when the DSL it implements legitimately
needs them. With the parameter gone, the placeholder leak is resolved
and the rehearsal can proceed.

That fix leaves an open question on the CT path. This plan owns it.

## The open question

`implement-dsl`'s scope (process-flow.yaml:1385) declares
`read: [dsl-core, driver-port, external-system-driver-port]` and
`write: [dsl-core, driver-port, external-system-driver-port]`. The
scope is identical on both call paths. After the strip, the agent
prompt no longer signals which side it is running on, so on the CT
path the agent can:

1. See `driver-port` in its write scope.
2. Conclude it may add System Driver prototype methods.
3. Emit `system-driver-port-changed: true`.
4. Trigger the downstream `implement-system-driver-adapters` cycle
   from within a contract-test flow — which the CT-path BPMN
   (`implement-and-verify-external-system-driver-adapters-contract-tests`,
   process-flow.yaml:901) does not contemplate.

Whether this actually happens in practice depends on whether the
agent's other contextual signals (the `${tests}: contract` param,
`${cycle_phase}` if it propagates, the read scope it's editing
against) are strong enough that it *won't* venture into the System
Driver port without a System-Driver-shaped reason. Today we don't
know; the AT path is what the rehearsal exercises.

## Options

### A. Trust the agent's contextual reasoning (no change)

The CT path already passes `tests: contract` through
`implement-and-verify-dsl` and `implement-test-layer`. The
`dsl-implementer` body could be enriched with a one-line note: "if
you are implementing CT-side DSL (tests=contract), the System Driver
port will not legitimately need new methods — do not add them." No
runtime gate, no scope split. Cheapest. Risk: the agent ignores the
guidance and the misbehaviour only surfaces when the downstream
adapter cycle fails or a scope-exception envelope flags it.

### B. Narrow the scope per call path

Move the scope declaration off the `implement-dsl` process and onto
each caller, so the CT path can declare a narrower write set
(`[dsl-core, external-system-driver-port]` — no `driver-port`). The
scope machinery already supports per-node scope (plan
20260526-1448 Item 4), and `findUnfilledPlaceholders` /
`validateOutputsAndScopes` would then enforce it at runtime: an
agent that writes to `driver-port` on the CT path triggers a
scope-violation FIX cycle, exactly as designed.

Drawback: the `implement-dsl` process becomes a parameterised shell
of itself — every caller must repeat the scope block. The current
single-source pattern (scope co-located with the leaf process) is
nicer to read.

### C. A CT-specific agent

Split `dsl-implementer` into `dsl-implementer-at` and
`dsl-implementer-ct` (or use the `task-prompts:` override knob to
swap a CT-specific prompt body at the CT call site). The CT body
omits step 2(a) (System Driver prototype injection) entirely.

Drawback: prompt duplication. Two bodies that share 90% of their
text drift the moment one is edited.

### D. Output-flag invariant check

Add a runtime assertion: after `dsl-implementer` runs on the CT path,
if `system-driver-port-changed=true`, fail the dispatch with a
diagnostic ("CT-path dsl-implementer must not touch the System Driver
port"). The check is a few lines in `actions/bindings.go` next to the
existing output validation. Catches the bug after the fact rather
than preventing it.

Combine with A for belt-and-braces: prompt guidance plus runtime
enforcement.

## Recommendation

Defer until the CT path actually exercises this code. The CT-side
HIGH process is documented as an orphan in the structural call graph
(process-flow.yaml:888-898, "No other process calls this HIGH
today"), so the misbehaviour cannot manifest in any current
rehearsal. When the CT-side call-site lands (Phase D per the
brainstorm), revisit this plan and pick Option A+D as the
lowest-friction pairing — guidance in the prompt, runtime check as
backstop.

## Out of scope

- The AT-side fix that already shipped (Parameters section removal).
- Any change to the scope-exception envelope mechanism itself.

## References

- `internal/assets/runtime/agents/atdd/dsl-implementer.md` — current
  prompt body (post-strip).
- `internal/atdd/runtime/statemachine/process-flow.yaml`:
  - line 678 — AT call site
  - line 901-998 — CT-side HIGH (orphan today)
  - line 1363-1393 — `implement-dsl` leaf with its read/write scope
- Memory `feedback_schema_fields_earn_slot` — the rationale for
  removing the parameter rather than wiring an always-`true` binding.
