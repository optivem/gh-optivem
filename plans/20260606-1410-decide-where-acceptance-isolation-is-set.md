# Plan: Express acceptance-test isolation upstream (Gherkin tag), writer mirrors

> **DECISION MADE (2026-06-06):** isolation is decided **upstream** (ticket / acceptance-criteria
> refiner), not by the `acceptance-test-writer`. The writer stays a mechanical 1:1 translator. This
> plan now describes the resulting design. Spun out of
> `plans/20260606-1356-run-isolated-acceptance-suites.md` (the *run* side). Fully settled and ready
> to execute once its run-side dependency lands (see Dependencies).

## Why

The scaffold ships plain and `@Isolated` (`com.optivem.testing.Isolated`) acceptance tests.
`@Isolated` marks a test that controls **process-global state shared across concurrently-running
tests** and so must run serially (`-DincludeTags=isolated`, `maxParallelForks=1`). In the shop demo
the only such state is the clock and promotion; other domains have their own (a feature flag, global
config) or none.

Today there is **no guidance anywhere** on when a new acceptance test should be `@Isolated` — the
choice is whatever sibling the writer happens to copy. Failure modes: **under-isolation** (should be
isolated, written plain → flakes under parallel forks — the dangerous, silent one) and
**over-isolation** (plain test tagged isolated → just slower).

## Hard constraint (shaped the decision)

"Tell the agent to isolate clock/promotion tests" is **wrong**: those are shop-domain builders, and
hardcoding them into a generic agent prompt is the scaffold→agent coupling
`[[feedback_no_scaffold_repo_coupling]]` forbids. The solution must be domain-agnostic. Asking the
*writer* to infer isolation from a concept rule was rejected because it puts a silent
domain-judgement (with the dangerous failure mode) inside the mechanical translator — against
`[[feedback_agents_dont_validate_inputs]]` (judgement belongs upstream, not in the agent body).

## Design (chosen: upstream, via a Gherkin tag)

Isolation is expressed **per scenario as a Gherkin `@isolated` tag** in the Acceptance Criteria
text, and the writer maps it 1:1 to the test annotation.

**Why a Gherkin tag fits with zero plumbing:** the Acceptance Criteria section is extracted as
**freeform text** (`internal/atdd/runtime/intake/parse.go` → `ExtractSection(...).Body`; ACs are
not parsed into structured scenarios). So a `@isolated` tag simply lives in the AC body text —
idiomatic Gherkin, human-readable in the ticket, survives intake untouched, and needs **no
parser/schema/`parsed-concepts` change**. The change is entirely in the two agent prompts (+ docs).

**Flow:**
- **Author / refiner sets the tag.** A human ticket author may write `@isolated` above a scenario.
  The `acceptance-criteria-refiner` (which already enforces Gherkin GIVEN-WHEN-THEN and whose output
  is **human-confirmed before downstream consumption** — `acceptance-criteria-refiner.md:16`) may
  add `@isolated` to a scenario that sets shared-global state. Because refinement is human-confirmed,
  no isolation choice reaches the writer un-reviewed — this keeps the agent out of the *silent* path.
- **Writer mirrors, never judges.** `acceptance-test-writer` translates a scenario tagged
  `@isolated` into a test carrying the language `@Isolated` annotation (alongside the existing WIP
  gate + channel annotations); untagged → plain. No concept rule, no builder names — it reads the
  tag and applies the annotation. This stays inside its "translate 1:1, don't classify" boundary.

**Domain-agnostic by construction:** the `@isolated` tag carries no domain vocabulary; a domain with
no shared-global scenarios simply never uses it.

## Items

1. **`acceptance-criteria-refiner.md`** — add to the rubric / Additional Notes: a scenario that sets
   **process-global state shared across concurrently-running tests** (a singleton like a clock or a
   global toggle — *examples, stated as examples, never as the rule*) gets a `@isolated` Gherkin tag;
   ordinary per-scenario data does not. Keep it short per `[[feedback_flag_non_token_efficient]]`.
   Note it is subject to the existing human-confirm gate.
2. **`acceptance-test-writer.md`** Step 1 — when the source scenario carries `@isolated`, emit the
   test with the language `@Isolated` annotation; otherwise plain. Provide the per-language annotation
   shape the same way `${gate-marker-example}` is provided (Java `@Isolated` /
   `com.optivem.testing.Isolated`, and the .NET / TypeScript equivalents — confirm the exact symbol
   per language against the shop testkit). This is mechanical mirroring, explicitly *not* a
   classification rule.
3. **Docs** — if a DSL / ATDD reference under `docs/atdd/` documents acceptance-test conventions, add
   one line on the `@isolated` tag → `@Isolated` annotation mapping. Do not author a new doc just for
   this (executor's discretion).
4. **Ticket template (optional)** — consider a one-line note in the Acceptance Criteria field help
   (`.github/ISSUE_TEMPLATE/*.yml`) that a scenario needing serial/global-state isolation can be
   tagged `@isolated`. Gate: only if it doesn't bloat the form; executor's call.

## Settled: refiner suggests, human confirms

Decided (2026-06-06): the refiner **may suggest** `@isolated` on a scenario that sets shared-global
state — it is a suggestion, not an authority, because the existing human-confirm-after-refine gate
(`acceptance-criteria-refiner.md:16`) reviews it before the writer ever sees it. The human author can
also set or remove the tag. The writer only mirrors whatever tag survives confirmation.

## Verification

- Manual: a ticket with one `@isolated` scenario and one plain scenario → writer emits one
  `@Isolated` test and one plain test; the isolated one runs under the isolated suite (now wired by
  `plans/20260606-1356-run-isolated-acceptance-suites.md`).
- No Go unit test (prompt/prose changes).

## Dependencies / not in this plan

- The run-side fold that makes isolated suites actually execute:
  `plans/20260606-1356-run-isolated-acceptance-suites.md`. Without it, a correctly-tagged `@Isolated`
  test is still never run — execute/land that first (it is itself blocked on the `1345` preflight
  plan).
- No change to the AC parser or `parsed-concepts` schema (the tag rides in freeform text).
