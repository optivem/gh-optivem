# 2026-11-09 12:01 UTC — Discuss: legacy baseline (use cases + characterization tests for an existing system)

## TL;DR

**Why:** The published article "Acceptance test coverage with AI" (`substack/articles/published/2026-11-06-PAID-ATDD-acceptance-test-coverage-with-ai.md`) describes a legacy on-ramp:
1. Generate the use case diagram (source code wins over the docs).
2. Do the core use case first: narrative, business rules and Gherkin criteria, then the acceptance test architecture, then tests that pass against the existing system. These are characterization tests: on failure, fix the test, criterion and narrative, never the system, and never weaken an assertion.
3. Then cover the other use cases, one per commit.
4. Then delete the criteria.

Today gh-optivem has `cover-system-behavior` per ticket (`task/legacy-coverage`). It writes passing tests for given criteria, but it does not generate the diagram, narratives or criteria.
**End result:** A decision on how gh-optivem supports the baseline (a kickoff mode, an extension of `cover-system-behavior`, or prompts only), followed by an implementation plan.

Parent: `plans/20261109-1219-improved-atdd-process-flow.md`, Step 10 (not yet drawn).

## Outcomes

- A recorded decision on the shape of the baseline.
- If built: the process diagram for it, drawn in the parent's colour legend.

## ▶ Next executable step (resume here)

Discussion only: `/refine-plan plans/20261109-1301-legacy-baseline-mode.md`.

## Steps

- [ ] Step 1: Settle the open questions with the user.
- [ ] Step 2: Draw the chosen flow (new/changed nodes coloured) and write the implementation plan.

## Open questions

- **Shape:** (a) a new kickoff mode that runs the whole baseline; (b) extend `cover-system-behavior` with optional "generate narrative + criteria" steps per use case ticket; (c) no new flow, just ship the article's prompts as docs.
- How does the existing kickoff (which already generates stories that name `UC-NNN.M` criteria) relate to this?
- The first test needs a human to "hand-hold" the agent through each layer (article). How does the flow represent that?
- Characterization rule "fix the test, never the system": does the existing `verify-mode: green-when-complete` fix loop already enforce it, or does it need a scope guard?
