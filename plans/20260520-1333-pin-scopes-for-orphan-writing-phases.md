# Plan: pin scopes for 4 orphan writing-agent phases

**Date:** 2026-05-20 13:33 UTC

## Why

`TestPhaseScopes_ReverseFK_WritingAgentsScoped` (renamed by
[plan 20260520-1053](20260520-1053-remove-phases-deferred-by-plan.md))
flags four writing-agent phase ids that have no entry in
`internal/atdd/phase-scopes.yaml`:

- `BACKLOG_REFINEMENT`  — agent `refine-acc`     — added by commit `600cd1b`
- `UPDATE_TICKET`       — agent `update-ticket`  — added by commit `600cd1b`
- `ENABLE_TESTS`        — agent `enable-tests`   — added by commit `b3b5952`
- `DISABLE`             — agent `disable-tests`  — added by commit `b3b5952`

These predate plan 20260520-1053 — they were added in recent commits that
should have pinned scopes in the same change but did not. The reverse-FK
test was passing at the time because it allowed `PhasesDeferredByPlan`
allowlist entries as an out (per the now-removed mechanism); but these 4
phases weren't on the allowlist either, so the test must have been
silently broken since `600cd1b`/`b3b5952` landed.

Per `feedback_no_deferred_mechanism`: every writing-agent phase needs its
scope explicitly pinned — no allowlist, no deferral. This plan fills the
gap.

## Items to walk

Items are stubs — refine with `/refine-plan` before executing. Each item
needs an explicit user scope-doctrine decision (which layer partition
applies). The four phases are split into two natural pairs.

### Item 1 — Introduce `scope: none` category (covers `BACKLOG_REFINEMENT`)

**Resolution:** introduce explicit `scope: none` as a doctrinal category,
distinct from "scope omitted" or "empty layer list". Semantic
(primary-backend reading, per refine-plan walk decision):
> Under the canonical GitHub / Jira issue-tracker backends, the agent
> modifies NO file in the repo working tree — not under any canonical
> layer path, not anywhere else (no config, no docs, no scripts).
> Mutations target inter-phase artifacts (`${parsed_concepts}`) or the
> external tracker only.
>
> Markdown / file adapters are treated as escape hatches (per
> `feedback_naming_github_jira_first`); their repo writes are
> out-of-doctrine and do not invalidate the `scope: none` declaration.

A `scope: none` agent that writes to a working-tree file *outside* an
escape-hatch adapter is a contract violation, full stop — this is what
makes `none` crisper than the ambiguous empty `{}`.

Grounding: the BPMN node lives in the `backlog_refinement` sub-process
(`process-flow.yaml:282-314`), invoked at top-level from the orchestrator
(`process-flow.yaml:111-135`). The agent (`refine-acc`) mutates the
`${parsed_concepts}` artifact only — see
`internal/assets/runtime/prompts/atdd/refine-acc.md` prompt body ("no
code layer touched"). The parsed-concepts artifact is an inter-phase
scratch object, not a code path under any Family B or Family A key.

This is distinct from a plan-deferral allowlist (per
`feedback_no_deferred_mechanism`) — it is a principled doctrinal category
with a hard contract (no repo writes), not a "TBD later" hole.

**Scope of work:**

1. Define `scope: none` as a valid prompt-frontmatter value, with the
   contract above. Document it alongside the existing `scope:` doctrine
   (likely in `internal/assets/runtime/shared/scope.md` per the
   phase-scopes.yaml header reference — confirm at execute time).
2. Replace `scope: {}` with `scope: none` in
   `internal/assets/runtime/prompts/atdd/refine-acc.md` frontmatter (and
   in `update-ticket.md` — see Item 2).
3. Update `TestPhaseScopes_ReverseFK_WritingAgentsScoped`
   (`phase_scopes_test.go:99`) so writing-agent nodes whose agent prompt
   declares `scope: none` are skipped instead of demanded to appear in
   `phase-scopes.yaml`. SSoT for the exemption is the prompt frontmatter,
   not a sibling Go map.
4. Do NOT add a `BACKLOG_REFINEMENT:` entry to `phase-scopes.yaml`.

**Open questions (resolve at execute time):**

- **Loader shape.** YAML parses `none` as a string, so the prompt
  frontmatter loader needs a small sum-type discriminator (string
  `"none"` vs. map of layer keys). Trivial but flagged so it doesn't
  get blurred.
- **Runtime projection.** What does the runtime scope-projection emit
  for a `scope: none` agent? Today the projection joins per-phase layer
  keys with `gh-optivem.yaml` paths to produce the agent's runtime
  `scope:` block. For `none`, presumably emit a clear "no code paths —
  artifact-only" sentinel rather than an empty list.

### Item 2 — Apply `scope: none` to `UPDATE_TICKET`

**Resolution:** `scope: none` — same category introduced in Item 1.

Grounding: BPMN node `process-flow.yaml:301-308` (same sub-process as
Item 1). Agent `update-ticket` overwrites three named H2 sections of
`${ticket_source}` — see
`internal/assets/runtime/prompts/atdd/update-ticket.md` ("touches only
the three named H2 sections of `${ticket_source}`"). Under the canonical
GitHub / Jira tracker backends, `${ticket_source}` is the tracker (not a
repo file), so the agent makes zero working-tree writes. A markdown
ticket-source adapter would write to the repo, but per
`feedback_naming_github_jira_first` that path is an escape hatch and is
out-of-doctrine for the `scope: none` contract.

**Scope of work:**

1. Replace `scope: {}` with `scope: none` in
   `internal/assets/runtime/prompts/atdd/update-ticket.md` frontmatter.
2. Do NOT add an `UPDATE_TICKET:` entry to `phase-scopes.yaml`. The
   reverse-FK test update from Item 1 covers exemption.
3. No new doctrine — items 1 and 2 share the same doctrinal change
   (introduced in Item 1). Item 2 is a second consumer.

### Item 3 — Pin scope for `ENABLE_TESTS`

**Scope:** `[at_test, ct_test]`

Grounding: BPMN node `process-flow.yaml:483-485` in the `at_green_system`
sub-process — top-level AT-only invocation today. Agent `enable-tests`
(prompt: `internal/assets/runtime/prompts/atdd/enable-tests.md`) strips
per-language disable markers from test methods in `${disable_targets}`.

The agent edits real test source files (insert/remove `@Disabled` /
`[Fact(Skip=…)]` / `test.skip(...)` markers + sometimes a now-unused
import). So it does NOT fit `scope: none` (Item 1) — it modifies repo
working-tree files.

Today the prompt's reason-format hardcodes `AT` and accepts only AT
phase labels, so concretely only `at_test` files are touched. The
`[at_test, ct_test]` pin is forward-looking: when CT-side enable-tests
lands the scope is already correct without re-pinning. Two-layer scope
is doctrinally clean because both keys are canonical test-layer keys
that this same agent will eventually span.

**Scope of work:**

1. Add `ENABLE_TESTS: [at_test, ct_test]` to
   `internal/atdd/phase-scopes.yaml` under the AT cycle section.
2. Update the prompt's frontmatter — replace `scope: {}` with whatever
   shape the runtime projection expects for a real layer-pinned phase
   (verify the projection mechanism at execute time; the frontmatter
   may be wholly authored by the runtime today and need no manual
   change).
3. Verify `TestPhaseScopes_ReverseFK_WritingAgentsScoped` passes after
   the entry is added.

### Item 4 — Pin scope for `DISABLE`

**Scope:** `[at_test, ct_test]` — mirrors Item 3.

Grounding: BPMN node `process-flow.yaml:994-996` inside the shared
`red_phase_cycle` sub-process. Agent `disable-tests` (prompt:
`internal/assets/runtime/prompts/atdd/disable-tests.md`) annotates test
methods listed in `${disable_targets}` with per-language disable
markers (`@Disabled` / `[Fact(Skip=…)]` / `test.skip(…)`) so the runner
skips them until `enable-tests` re-enables them.

Symmetric counterpart of Item 3. Same target set (test methods from
`${disable_targets}`), same hardcoded `AT` in the reason format today,
same forward-looking case for `[at_test, ct_test]`. Keeps the
disable/enable pair doctrinally consistent — the shared
`${disable_targets}` flow can't be scoped inconsistently between the
two halves.

Does NOT fit `scope: none` (Item 1) for the same reason as Item 3 —
modifies repo working-tree files.

**Scope of work:**

1. Add `DISABLE: [at_test, ct_test]` to
   `internal/atdd/phase-scopes.yaml` under the AT cycle section.
2. Update the prompt's frontmatter — same caveat as Item 3 (verify
   projection mechanism; the frontmatter may need no manual change).
3. Verify `TestPhaseScopes_ReverseFK_WritingAgentsScoped` passes after
   the entry is added.

## Sequencing (vs other in-flight plans)

- **Downstream of:** [plan 20260520-1053](20260520-1053-remove-phases-deferred-by-plan.md) —
  this plan's items are only visible as test failures because that plan
  renamed the reverse-FK test from "ScopedOrAllowlisted" to "Scoped".
- **Possibly overlapping with:** [plan 20260518-1116](20260518-1116-legacy-coverage-cycle.md)
  (legacy coverage cycle) — that plan adds new BPMN phases and pins their
  scopes; check whether its work coincidentally pins any of these 4
  before starting this plan.
