# 2026-10-23 09:59 CEST — Optional inner TDD loop, decided per component

## TL;DR

**Why:** Today the ATDD flow is outer-loop only: acceptance tests are the sole
gate on `IMPLEMENT_AND_VERIFY_SYSTEM`. For components with a real domain core
(e.g. shop `backend-clean-java`), that leaves design quality and unit-level
regression protection unmeasured. Böckeler's experiment ("TDD in the agent
loop", martinfowler.com) is the reason this is opt-in rather than default:
agent-internal TDD there cost 3–8.5× more tokens, with no measurable quality or
mutation-score gain on small greenfield tasks. It also tended to lock the design
into whatever shape the first test set. She did not test legacy or long-lived
code.

**End result:** Each component (tier) of a `kind: system` project declares an
inner-loop mode: `none` (default, today's behaviour), `unit-tests`, or `tdd`.
`backend-clean-java` runs the `unit-tests` mode end-to-end. In that mode,
domain code is written with unit tests, and green is gated on a mutation-score
threshold over the domain core. A `tdd` mode, with a separate unit-test-writer
agent, exists for teaching.

## Outcomes

- A component can opt in to an inner loop without affecting any other component
  in the same system. Thin backends and `frontend-react` stay `none` at zero
  extra cost.
- In `unit-tests` mode, "green" means more than "the acceptance test passes". It
  also means:
  - unit tests on the domain core pass;
  - a mutation-score threshold holds (PIT for Java).
  This turns test quality into an outcome sensor rather than a process ritual.
- In `tdd` mode, the red step is confirmed by a *different* agent from the one
  writing production code, mirroring the existing AT-writer / implementer split.
  It is a demonstrable double loop for course material.
- Inner-loop writes are confined to the domain core by a pinned `${...}`
  placeholder in `phase-scopes.yaml`. Adapters stay covered by AT/CT only.
- The shop pilot (`system/multitier/backend-clean-java`) gives a first data
  point on Böckeler's open question: does the inner loop pay back on long-lived
  code?

## ▶ Next executable step (resume here)

Design only. Nothing mechanical is ready yet. Resolve the Open questions below
with `/refine-plan` on this file before running `/execute-plan`. The questions
that most need answers first are Q1 (monolith placement), Q3 (gate strictness)
and Q4 (`tdd` iteration shape), because Step 1's schema shape and Step 3's BPMN
shape depend on them.

## Steps

### Slice 1 — schema + `unit-tests` mode, Java only, pilot on backend-clean-java

- [ ] Step 1: Add the inner-loop mode field to `TierSpec`
  (`internal/kernel/projectconfig/config.go`). Use an enum `none|unit-tests|tdd`,
  default `none`. Add Validate rules and init/config prompts that run the same
  validator as the flag.
- [ ] Step 2: Add a Family B path placeholder for the component's domain core and
  its unit-test path. Pin it in `phase-scopes.yaml` for the new inner-loop phase.
  Paths must be constructed deterministically (pinned against the shop
  template), with no fallback ladder.
- [ ] Step 3: In `implement-and-verify-system`, add a gateway on the component's
  mode. For `unit-tests`, the implementer prompt gains a "design upfront, then
  domain code + unit tests through the domain API/ports, no mocking inside the
  domain" section. A new verify step runs the unit suite plus mutation testing,
  scoped to the domain core.
- [ ] Step 4: Wire PIT into shop `backend-clean-java` (domain-core target only).
  Add a `gh optivem` runner command/suite for it that local users run the same
  way CI does.
- [ ] Step 5: Tests:
  - statemachine/transition tests for the new gateway (audit loopback edges for
    the deadlock hazard first);
  - render-matrix coverage for the new placeholder.

### Slice 2 — `tdd` mode

- [ ] Step 6: Add a unit-test-writer agent and a red → green (→ refactor) inner
  sequence in BPMN, with `tdd-stage` styling. Reuse
  `expected-test-result: failure` for the unit-level red.

### Slice 3 — other languages and tiers

- [ ] Step 7: Stryker.NET for `backend-dotnet` and StrykerJS for
  `backend-typescript` / `frontend-react`, if any tier there opts in.
- [ ] Step 8: Docs. Update `docs/atdd/process/**` for the double loop as an
  optional inner loop, and explain when to choose each mode (domain core vs
  CRUD/plumbing).

## Background and rationale

### Source

Birgitta Böckeler, *TDD inside the agent loop – theater or actual value?*,
martinfowler.com:
https://martinfowler.com/articles/exploring-gen-ai/tdd-in-the-agent-loop.html

### What the article found

- Sonnet ran the tasks with and without TDD instructions, and Opus judged the
  results blind. There was no clearly discernible quality difference. The
  non-TDD runs sometimes ranked slightly higher on design and test quality.
  Mutation scores showed no meaningful difference.
- TDD runs used roughly 8.5× (small task), 3× (medium) and 4.9× (large) more
  tokens. Cheap cache reads were counted the same as fresh tokens, so the
  dollar cost is likely overstated, but the direction is clear.
- Non-TDD runs designed upfront: data types, edge cases, contracts. TDD runs
  grew the design from many locally minimal decisions, rarely revisited it, and
  locked into whatever shape the first test set. Behaviour nobody thought to
  test simply didn't get implemented.
- A red test only proves something if someone checks *why* it is red. When the
  same agent writes the test and confirms the failure, it only proves the agent
  saw a failure.
- Her suggested alternatives:
  - **Approved Scenarios:** functional scenarios that a human confirms and
    freezes, and that must be re-approved whenever they are violated. This is
    close kin to ATDD.
  - **Outcome sensors rather than process instructions:** mutation testing,
    static analysis and modularity reviews.

### Where the article leaves room

- The tasks were small, greenfield and pure business logic, with a very small
  sample. Legacy and long-lived code were not tested. That is exactly the
  "feature 50, not feature 1" question.
- She tested agent-internal TDD only, with the agent writing both the tests and
  the code. The mode where a human owns the scenarios and the agent implements
  them (= our ATDD outer loop) is not argued against; the article supports it.
- The "quality" judgement came mostly from an LLM judge, which is a proxy rather
  than ground truth.

### Our position (Claude + Valentina, 2026-10-23)

- **ATDD stays the human-owned outer loop.** The article strengthens this: it is
  the checkpoint that agent-internal TDD lacks.
- **The inner loop is opt-in per component, not the default.** The fixed token
  cost only pays back where there is decision-heavy logic (a domain core).
  CRUD, adapters and glue stay `none`, because there the acceptance test
  already pinpoints the typical wiring failures.
- **Prefer outcome sensors over policing the ritual.** That is why the
  `unit-tests` mode gates on mutation score instead of enforcing red-green
  ordering.
- **Where we keep real TDD (`tdd` mode), make the red independent.** A separate
  unit-test-writer agent writes the failing test, mirroring the existing
  AT-writer / implementer split. This answers the "agent confirms its own red"
  critique, though it is still not a human checkpoint.
- **Earlier claims we downgraded after reading the article:**
  - "TDD gives design pressure": that holds for humans, but agents don't feel
    the friction that creates it.
  - "TDD pays back through fewer debugging loops": this is a hypothesis
    reasoned from human TDD, not evidence about agents.
- **What the pilot is meant to test:** does the inner loop pay back on
  long-lived code (backend-clean-java over many tickets)? Track three numbers:
  - tokens per ticket;
  - fix-loop iterations;
  - files touched per change over time.

## Related

- `plans/20260818-1658-contract-driven-outer-loop-for-kind-component.md`: that
  plan covers `kind: component` projects, where `implement` is refused today.
  This plan covers components *inside* a `kind: system` (tiers). The two plans
  are kept separate on purpose. If that plan lands, its flow could reuse this
  inner-loop mode as-is.

## Open questions

1. **Where does the monolith's setting live?** Monoliths use System-level
   `path/repo/lang`, not `TierSpec`. Options:
   - a sibling field on `System`;
   - a per-path list (domain-core dirs, each with a mode).
   Lean: a sibling field on `System`, mirroring the TierSpec field.
2. **Do the `redesign-*` cycles get the inner loop?** They reuse
   `IMPLEMENT_AND_VERIFY_SYSTEM`. Lean: yes in `unit-tests` mode, since the
   mutation gate is cheap and protects the redesign. Unclear in `tdd` mode.
3. **Mutation threshold: what value, and is it a hard failure?** Lean: the
   threshold is configured per component, and the gate fails hard (it routes
   into the existing fix loop). Starting value to be set from a baseline run on
   backend-clean-java.
4. **Shape of the `tdd` mode:** per-behaviour BPMN iteration (writer → verify
   red → implementer → verify green, looped), or a single agent-internal loop?
   The BPMN loop gives a real independent red at 3–8× the token cost. The
   agent-internal loop is cheaper but becomes the "theater" Böckeler criticises.
5. **Does `implement --target` gain a `unit-test` slice** for team handoff?
   Lean: no in Slice 1.
6. **Is the field name `inner-loop`?** It must avoid "ATDD"/internal jargon,
   because the name leaks into configs that students read.
