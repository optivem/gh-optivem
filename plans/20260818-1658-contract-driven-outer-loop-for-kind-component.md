# 2026-08-18 16:58 CEST — Contract-driven outer loop for `kind: component`

> ⏬ **LOWER PRIORITY — DISCUSS BEFORE EXECUTING.** This is a design plan, not an
> edit list. Nothing here is executable yet: the Open Questions below are
> genuinely open, and answering them wrong would bake a wrong process into the
> BPMN flow that every component team then walks. Do **not** run
> `/execute-plan` on this file — its pre-flight gate will stop on the open
> questions anyway. The next move is a conversation, then `/refine-plan`.
>
> Nothing is broken while this sits. `kind: component` ships today with
> `implement` refusing it outright, which is an honest, stable end state — a
> component team gets a fully working inner loop and no half-applicable outer
> loop. This plan is about giving them an outer loop, not about repairing one.

## TL;DR

**Why:** `kind: component` (landed 2026-08-18, `f3f003d0`) lets a team declare a
project that is one unit of code with no system under test. Their inner loop —
unit → component → integration — works unchanged. But ATDD's **outer loop *is*
the system loop**: acceptance tests drive a booted SUT from the outside. A team
that cannot see the system cannot write them. So `gh optivem implement` refuses
`kind: component`, and a component team has no guided outer loop at all.

**End result:** a component team drives their work from a **contract** the way a
system team drives it from an acceptance test — consumer-driven contract testing
is precisely the "I cannot see the system, but I know what I owe it" mechanism.
`gh optivem implement` accepts `kind: component` and walks a flow whose outer
loop is contract-shaped: a failing contract test is the RED that opens the loop,
and a verified contract is the GREEN that closes it.

## Why this is its own plan

The `kind:` discriminator plan (`20260818-1351`, executed and deleted) stated the
position and deliberately stopped:

> ATDD's outer loop **is** the system loop. A team that cannot see the system
> cannot write acceptance tests against it. So `kind: component` is not merely a
> smaller config — it describes a different development process: the inner loop
> is unchanged; the outer loop is replaced by a **contract**. `kind: component`
> therefore means *inner loop + contract boundary, no SUT*.
>
> **This plan states the position and stops there.** Designing a contract-driven
> outer loop as an actual process flow is its own plan — it needs a design pass
> on what the flow looks like, not a step tacked onto a config change.

That is still true. This file exists so the position does not have to be
re-derived from scratch next time, and so the open questions are written down
where they can be argued about rather than guessed at during execution.

## Background — what already exists to build on

Gathered so the design conversation starts from facts, not recollection.

**The refusal is deliberate and permanent-until-this-plan.** `implement` is
guarded by `requireSystemKind` in `kind_guards.go`; the message says the outer
loop is the system loop and points the operator at `component-test run`.
Removing that guard is the *last* step of this plan, not the first — it must not
open until there is a flow behind it.

**Contract testing is not new here.** `shop`'s `backend-java` already runs four
contract-shaped suites, and they map onto two genuinely different counterparty
situations — which is the crux of the design:

- `provider-verification` — the counterparty (the frontend) *does* run our
  verification, so agreement is pinned by a real pact file under
  `shop/contracts/`.
- `external-contract` / `external-contract-real` — the counterparties (ERP, tax,
  clock) will *not* run our verification, so agreement is pinned by stub-vs-real
  **parity pairs** plus stub-consumability tests.

A component team is on one side or the other of this line depending on whether
their counterparty cooperates, and the flow probably differs between the two.

**Contract distribution is file-based today, not a broker.** Mono-repo shares a
`contracts/` directory; multi-repo pushes from consumer CI. A broker-as-default
migration is planned separately in `shop/plans/20260625-0913`. Whether the
component outer loop *requires* a broker is Open Question 4 — it should not be
assumed either way.

**The BPMN flow is data, not code.** `internal/atdd/process/process-flow.yaml`
drives everything; per-phase write scopes live in `phase-scopes.yaml`, and there
is no "deferred by plan" escape hatch — every writing-agent phase must have its
scope pinned. So a component flow means real new nodes with real pinned scopes,
which is a large part of why this needs design before edits.

## Open questions — ALL UNRESOLVED, this is the discussion

1. **Is the contract the RED, or is it an input to the RED?** Two readings.
   (a) The contract test *is* the outer-loop test: it goes red, the inner loop
   drives it green, done — a clean structural analogue of the acceptance test.
   (b) The contract is a *given*, handed down by the system team, and the
   component team's outer loop is really just "satisfy this spec" — in which
   case the loop is thinner and looks more like ticket → inner loop → verify
   contract. **My lean: (a)**, because it preserves the double-loop shape the
   whole methodology teaches, and because a component team that cannot author
   its own contract test has no way to express intent before coding. But (b) is
   the honest description of a team handed a fixed contract they do not own, and
   that is a real org shape. This question probably decides the whole flow.

2. **Who authors the contract, and can the component team change it?** If the
   component is a *provider*, the consumer owns the contract and the component
   team may not unilaterally change it — so "the test is red" may mean "you are
   not done" *or* "the contract is wrong and needs a negotiation with a team you
   cannot see". The flow needs a branch for the second case, and that branch is a
   human hand-off, not an agent step. If the component is a *consumer*, it owns
   its own pact and the loop is self-contained. **Does `kind: component` need to
   declare which side it is on?** That would be a schema change (a
   `contract:` block naming role and counterparty) on top of the four fields
   `kind: component` currently allows.

3. **What does "the system team" look like from the component side?** The
   motivating scenario is a team with no access to and no knowledge of the wider
   system. Does the contract arrive as a file in their repo, a pulled artifact, a
   broker fetch, or a ticket description? This determines whether the flow starts
   with a fetch step and whether that step can fail in a way an agent can act on.

4. **Does this require the Pact broker migration to land first?** Related to 3.
   File-based distribution works inside one repo; a component team in a *separate*
   repo from the system may have no file-based path at all. If so, this plan is
   sequenced behind `shop/plans/20260625-0913`. **Check that plan's state before
   answering** — do not assume.

5. **Does `implement --target` gain component slices?** The system flow has
   `test` / `driver-adapter` / `system` slices for team handoff. A component flow
   might need none (it is already one team, one unit) or might want
   `contract` / `implementation`. Cheapest answer is probably none at first.

6. **What about a component with no counterparty at all?** A leaf library with no
   consumer contract has an inner loop and genuinely nothing outside it. Is that
   a supported shape (`implement` still refuses, and that is fine), or does it
   get a degenerate flow? **My lean: keep refusing** — an honest refusal beats a
   ceremonial loop with nothing in it.

7. **Does the double-loop teaching material need a component chapter?** This is
   a teaching repo. If the answer to Q1 is (a), the "outer loop = acceptance
   test" framing in `docs/atdd/**` becomes "outer loop = the test at your
   boundary, which is an acceptance test when you own the system and a contract
   test when you do not." That is a docs change with real pedagogical weight,
   not a footnote.

## Sketch — NOT a step list

Deliberately not `- [ ]` items: turning these into steps before the questions
above are answered is exactly the mistake this plan exists to prevent. Recorded
only so the shape of the work is visible when estimating.

- Decide the loop shape (Q1/Q2), and whether `kind: component` needs a
  `contract:` block to express role + counterparty.
- Design the BPMN sub-flow: nodes, gateways, which are agent-writing vs human,
  and the pinned `phase-scopes.yaml` entry for every writing node.
- Decide how a contract enters the component's working tree (Q3/Q4).
- Build it behind the existing refusal; the guard comes off last.
- Rehearse on a real component. `shop`'s `backend-clean-java` is the obvious
  candidate — it is already `kind: component` and already has contract suites.
- Docs: the component chapter in the ATDD material (Q7), and replace the
  "deliberately out of scope" paragraph in `docs/cli-reference.md`'s
  *Where this sits in the process* section.

## ▶ Next executable step (resume here)

**None — this plan is not executable, by design.** The next move is a
conversation about the Open Questions above, starting with Q1 (is the contract
test the outer-loop RED, or an input to it?), since it decides the shape of
everything else. Q4 needs a fact check first: read
`shop/plans/20260625-0913` and report whether the broker migration has landed,
because it may sequence this whole plan.

After that conversation, run `/refine-plan` on this file to write the decisions
in and convert the Sketch into real steps. Do not run `/execute-plan` before
then.
