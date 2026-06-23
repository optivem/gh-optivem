# 2026-06-23 13:46:00 UTC — system-implementer: first-class "contradictory-tests" halt verdict

## TL;DR

**Why:** When ticket #76's acceptance suite was internally contradictory (a new test needed blackoutEnd ≥22:45, a pre-existing `@isolated` test needed 22:30), the system-implementers — scoped to system code, unable to edit tests — could not satisfy both. The API implementer *correctly diagnosed the contradiction in prose* and committed its fix anyway; that diagnosis was discarded, and the next channel's implementer silently **reverted** the committed fix to green the other side, ending in a 30-min wandering halt.
**End result:** A system-implementer that determines no single system change can green all acceptance tests for a rule emits a first-class, clearly-labeled halt (reusing the scope-exception envelope — the reconciling edit is an out-of-scope test edit) with its diagnosis, makes **no** edit that greens one side at the other's expense, and **never** reverts a system change an earlier same-ticket dispatch committed. The run stops loudly with the diagnosis instead of oscillating.

## Outcomes

What we get out of this — the goals and deliverables (this is the fail-loud safety net for when Plan A's test-reconciliation is missed):

- A structurally-contradictory acceptance suite **fails loudly once**, with the implementer's diagnosis surfaced — never a silent revert + 30-min human-gate halt.
- The system-implementer is explicitly forbidden from "guessing a side": it must not green a subset of tests by reverting or working against a system change an earlier same-ticket dispatch already committed.
- The implementer's existing prose-only diagnosis (which #76's API implementer already produced verbatim) becomes a routed, run-halting verdict — no silent swallow (matches the repo's fail-loud doctrine).
- The contradiction is caught as early as the dispatch that *reads* the contradicting test, rather than only after a later channel wastes a full implement pass.

## ▶ Next executable step (resume here)

Edit `internal/atdd/assets/runtime/agents/atdd/system-implementer.md` to add a "contradictory tests — halt, don't pick a side" stop condition in Step 4 (alongside the existing stop rule), instructing the implementer to emit the **scope-exception envelope** (the chosen halt mechanism — see Decisions) with its diagnosis and make no edit. Then confirm in `internal/atdd/runtime/driver/driver.go` that the scope-exception envelope already surfaces the diagnosis and halts the run; add light wiring in `implement_commands.go` only if the halt isn't already labeled. Stop for user review before committing.

## Steps

- [ ] Step 1: Re-read how the **scope-exception envelope** halts a run (`scope.md`, `preamble.md`, `driver.go` envelope wiring near `withEnvelopeSpecs`) to confirm it surfaces the agent's diagnosis and stops the run. (Decision: reuse this envelope — see Decisions; do **not** add a new outcome unless this re-read shows the trace label is genuinely ambiguous.)
- [ ] Step 2: Edit `internal/atdd/assets/runtime/agents/atdd/system-implementer.md` Step 4: add a **contradictory-tests stop condition** — when the implementer determines no single change under the system surface can make all acceptance tests for the rule pass (a pre-existing test asserts the old rule and contradicts the new test / the ticket), it must emit the chosen halt envelope with its diagnosis and exit, making **no** edit. Explicitly state: **never revert or work against a system change an earlier same-ticket dispatch committed** to green a subset; do **not** guess a side. Mirror the existing "honest not-green exit is correct" framing and the `unexpected-failing-tests-fixer` "make no edits, write your diagnosis, exit" doctrine.
- [ ] Step 3: Confirm routing so the verdict halts loudly with the diagnosis attached — the scope-exception envelope path through `internal/atdd/runtime/driver/driver.go` and the exit-banner path in `implement_commands.go` (cf. `ExitCodePendingHuman`). Add light wiring only if the halt isn't already surfaced/labeled.
- [ ] Step 4: Add the **read-time detection** instruction (per Decisions, Q2): system-implementer must act on a contradiction it spots while reading sibling tests in Step 1 (incl. `@isolated`) — not change the self-verify suite. This is how #76's API implementer already spotted it; the change is making it halt instead of writing prose.
- [ ] Step 5: Add/adjust a test in `implement_commands_test.go` (or the driver tests) asserting the contradictory-tests verdict produces the labeled halt and does not fall through to the generic unexpected-failing-tests human gate. Re-read system-implementer.md for consistency with Plan A's behavior-change clause, and stop for user review.

## Decisions

_(open questions resolved — folded into the steps)_

- **Q1 — halt mechanism → reuse the scope-exception envelope.** Far less plumbing and *correct*: the implementer genuinely needs an out-of-scope test edit it cannot make — exactly what the envelope is for, and what #76's API implementer named ("test files are outside my write scope"). Add a dedicated `CONTRADICTORY_TESTS_HALT` outcome only if Step 1's re-read shows the trace label can't distinguish "contradictory tests" from "needs wider scope."
- **Q2 — self-verify scope → rely on read-time detection, don't widen self-verify.** The parallel slice's `--grep-invert '@isolated'` exclusion is intentional; the implementer already reads sibling tests (incl. `@isolated`) in Step 1, which is how #76's API implementer spotted the contradiction. Instruct it to act there. Revisit only if read-time detection proves unreliable.
- **Q3 — Plan A boundary → implementer halts, never edits tests.** Reconciling obsolete tests is the writer's job (Plan A); the implementer's only move on a contradiction is the halt. Keep the two plans' wording aligned on this.
