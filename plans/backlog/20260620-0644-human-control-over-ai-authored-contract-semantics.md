# 2026-06-20 06:44 UTC — Human control over AI-authored contract-test semantics (held in reserve)

## TL;DR

**Why:** #65 halted because the `contract-test-writer` agent *guessed* an external-system behavior it couldn't read from the repo (is the ERP product listing isolated to what the test created?) and guessed wrong (`hasCount(2)` vs a seeded, non-reset simulator returning 9). The shipped fix — the "Contract-test assertion invariants" checklist in `contract-test-writer.md` — removes the need for that decision in the common case. This note captures the **stronger control levers** deliberately *not* built, so a recurrence has somewhere to land instead of being re-derived.
**End result:** A decision record. Nothing is built. If the invariant rule proves insufficient, pull rung 2; only escalate to rung 3 if 2 also fails.

## Context — the rungs (cheapest first)

The governing question: contract-test *code* is fine for the agent to author (it's a mechanical translation of the DSL surface + the sibling test). But the contract *semantics the test depends on* — anything about the real external system's ambient/stateful behavior — is **not derivable from the codebase**, so the agent guesses. The levers differ by how much human control they buy and what they cost per ticket.

- **Rung 1 — invariant rule (SHIPPED).** `contract-test-writer.md` forbids asserting on the external system's whole-collection/ambient state; assert only self-created entities by key. Kills the whole class with zero per-ticket cost and no human decision. This is the 80/20.

- **Rung 2 — declare-and-halt on unobservable external semantics (NOT BUILT).** Agent-layer instruction: when the contract depends on an external-system behavior the agent **cannot read from the repo** (isolation, seed inventory, ordering guarantees, error semantics), it must **surface the assumption and stop** rather than guess. Targeted control exactly at the blind spot; cost is one halt only on genuinely irreducible cases. Build this *if* a case slips past rung 1.
  - Likely home: `contract-test-writer.md` (an explicit "if external behavior is unobservable, emit a blocked/assumption output" instruction), aligned with the existing `blocked` / scope-exception envelope pattern.

- **Rung 3 — ticket-authored contract semantics / child task (NOT BUILT).** Make a human pin the external-system semantics in the user-story ticket (a field, or a child sub-task) before the agent translates. Hard human gate on every external boundary. Heavyweight — pays a fixed cost on all stories to govern the rare ones; risks over-engineering. Only worth it if rungs 1+2 prove insufficient and a hard, always-on gate is genuinely wanted.
  - Likely home: ticket-parse / acceptance layer (a step that requires external-system semantics to be specified before contract-test authoring).

## Outcomes

- The two un-built control levers (rung 2, rung 3) are recorded with their rationale, cost, and likely implementation home, so a future postmortem can promote one without re-deriving the analysis.

## ▶ Next executable step (resume here)

None — this is a held-in-reserve decision record, not active work. Promote rung 2 to a real plan (via `/create-plan`) only if a contract-test failure recurs where the agent guessed an unobservable external behaviour despite the rung-1 invariant rule.

## Trigger to revisit

A future ATDD rehearsal halts because a contract test asserted something about real external-system behavior the agent could not have known from the repo — i.e. the rung-1 invariant rule did not cover it. That is the signal to build rung 2.
