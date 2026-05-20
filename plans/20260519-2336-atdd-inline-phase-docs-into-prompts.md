# Plan: inline `change/behavior/*` + `change/structure/*` phase docs into their prompts

**Date:** 2026-05-19

## Context

`internal/assets/global/docs/atdd/process/` carries per-phase doctrine docs (under `change/behavior/`, `change/structure/`, `analysis/`, plus top-level `path-keys.md`) that exist as standalone files **and** are also `Read` by exactly one prompt each in `internal/assets/runtime/prompts/atdd/`. Each pairing is therefore a single-reader/single-writer relationship dressed up as two coupled files — a drift surface with no benefit. The phase doc's `## Scope` section additionally duplicates information already held by the prompt's frontmatter `scope:`, by `internal/atdd/runtime/architecture/phase-scopes.yaml`, and by the `gh optivem process scope <PHASE>` CLI command (a four-place duplication audited in `plans/20260519-1537-post-meta-bpmn-topics.md`).

This plan folds the per-phase doc content into its sole prompt reader and deletes the standalone phase doc. After this plan lands, each prompt is self-contained for its phase-specific procedure; the only doctrinal docs remaining as standalone files are (a) `shared/*` (genuine N>1 readers) and (b) docs referenced by Go-runtime code rather than prompts (`disable-tests.md`, `enable-tests.md`, `path-keys.md`).

**Sibling / coordinated plans:**

- [post-meta-bpmn-topics (20260519-1537)](20260519-1537-post-meta-bpmn-topics.md) — surfaced this work as Items 1 and 5 of the doctrine-vs-prompts audit. Item 1 (the `## Scope` rename to `## Frozen layers`) is **absorbed** into this plan: the path-list half is dropped (duplicates frontmatter); the behavioral-framing half rides along into the prompt body where applicable.
- **Follow-up plan (not yet authored): scope-authority shift.** Today's prompts carry `scope: {}` with a comment pointing to the CLI; the layer keys live in `phase-scopes.yaml`. A separate follow-up plan will move authority into prompt frontmatter (e.g. `scope: { write: [at_test, dsl_port, dsl_core] }`) and drop `phase-scopes.yaml`. This inlining plan intentionally **does not** touch the `scope:` field — see "What this plan does NOT do" below.

## Bucket map

Every doc in `internal/assets/global/docs/atdd/process/` (excluding `shared/`) falls into exactly one of these buckets:

### Bucket A — N=1, clean inline (10 docs)

Each is read by exactly one prompt. Action: fold doc content into that prompt, delete the standalone doc.

| Doc | Single prompt reader |
|---|---|
| `change/behavior/at-red-test.md` | `at-red-test.md` |
| `change/behavior/at-red-dsl.md` | `at-red-dsl.md` |
| `change/behavior/at-red-system-driver.md` | `at-red-system-driver.md` |
| `change/behavior/ct-red-test.md` | `ct-red-test.md` |
| `change/behavior/ct-red-dsl.md` | `ct-red-dsl.md` |
| `change/behavior/ct-red-external-system-driver.md` | `ct-red-external-system-driver.md` |
| `change/behavior/ct-green-external-system-stub.md` | `ct-green-external-system-stub.md` |
| `change/structure/system-interface-redesign.md` | `task-system-interface-redesign.md` |
| `change/structure/system-implementation-change.md` | `chore.md` |
| `change/structure/external-system-interface-redesign.md` | `task-external-system-interface-redesign.md` |

### Bucket B — N=2, becomes N=1 after prompt merge (1 doc)

`change/behavior/at-green-system.md` is read by **both** `at-green-system-backend.md` and `at-green-system-frontend.md` prompts. The two prompts differ only in one comment line and one word ("Backend"/"Frontend"). Action: merge the prompts into a single `at-green-system.md` prompt, then inline the phase doc into the merged prompt. After merge, this becomes a Bucket A pattern.

The BPMN flow (`process-flow.yaml:432-456`) currently dispatches two call_activities (AT_GREEN_BACKEND, AT_GREEN_FRONTEND) with different `agent:` params (`at-green-system-backend`, `at-green-system-frontend`). After this plan, both call_activities point at the same merged `at-green-system` agent.

A TODO marker is added to the merged prompt's frontmatter noting a possible future `at-green-component` variant — a structural decision deferred, not made by this plan.

### Bucket C — referenced by Go code, untouched (3 docs)

These docs are not read by any prompt, but they ARE cited as the spec/contract by Go runtime code or `projectconfig`. Deleting them would orphan Go-comment pointers to the §Conventions specification.

| Doc | Referenced from |
|---|---|
| `change/behavior/disable-tests.md` | `internal/atdd/runtime/actions/bindings.go:200, 835, 839, 871` (§Conventions disable-reason format) |
| `change/behavior/enable-tests.md` | BPMN service_task `enable_change_driven` (`process-flow.yaml:425-430`); doc is the contract for the action |
| `path-keys.md` | `internal/projectconfig/config.go`, `paths_defaults.go`, `paths_defaults_test.go` |

Action: untouched. These belong to the deterministic-action and config-loading machinery, not to the prompt-loading machinery.

### Bucket D — genuinely N=0, marked DRAFT (2 docs)

These have zero readers anywhere (no prompt, no Go code, no other docs). They look like pre-prompt specifications — documented but not yet wired. Action: mark each doc's title with a `(DRAFT)` suffix as a visible signal that the content is not currently in service. Do not delete (preserves the forward-looking intent); do not introduce a new frontmatter convention.

| Doc | Current title | New title |
|---|---|---|
| `analysis/acceptance-criteria-refinement.md` | `# ACCEPTANCE CRITERIA ANALYSIS` | `# ACCEPTANCE CRITERIA ANALYSIS (DRAFT)` |
| `change/behavior/at-refactor.md` | `# AT - REFACTOR` | `# AT - REFACTOR (DRAFT)` |

## Inlining recipe (applied to every Bucket A doc)

For each `(doc, prompt)` pair:

1. **Drop the path-list half of `## Scope`.** It duplicates the prompt's frontmatter and the `gh optivem process scope <PHASE>` CLI. The `See [the scope rule](../../shared/scope.md).` pointer is also dropped — the prompt already has `Read ${docs_root}/atdd/process/shared/scope.md.` on a separate line.
2. **Preserve the behavioral-framing half of `## Scope` (if present).** For phase docs whose `## Scope` section contains non-path content — e.g., `at-green-system.md`'s "tests/DSL/drivers are frozen during GREEN; needing to touch a frozen layer signals an earlier RED phase was wrong" rule — lift that prose into the prompt body as plain instructional text. Drop the `## Scope` heading either way; the surviving text becomes part of the prompt's instructional flow, not a labelled subsection.
3. **Fold the `## Steps` section into the prompt body** as a `## Steps` subsection or equivalent. Preserve numbering and the exact wording — this is the meat the prompt is supposed to follow.
4. **Drop the doc's top-level `# <TITLE>` heading.** Redundant with the prompt's filename and frontmatter identifier.
5. **Drop the doc's opening sentence if it's an echo of the prompt's role description.** E.g., `at-red-test.md`'s "Write acceptance tests; add `"TODO: DSL"` prototypes so the result compiles." is implicit in the prompt's "You are the Test Agent. ... write tests for them directly."
6. **Drop the `Read ${docs_root}/atdd/process/<bucket>/<doc>.md.` line from the prompt.** The doc is now inlined; the Read line points at a file about to be deleted.
7. **Drop sentences in the prompt that are now redundant** — most commonly "Follow the phase referenced below." which makes no sense once the phase IS the prompt.
8. **Delete the standalone phase doc.**

## Items

- [ ] **Item 5: Post-inline verification sweep — ⏳ Deferred: dispatch-time placeholder regression.**

  Sweep partially done; remaining work blocked on a runtime regression that needs a scope decision.

  - ✅ Grep for stale phase-doc references. Outside of `plans/` and `reports/`, only `phase_doc:` strings in `process-flow.yaml` and the test fixtures that pin them remain (intentional per the "What this plan does NOT do" non-goal — BPMN fixtures + tests stay untouched).
  - ❌ `go test -p 2 ./internal/atdd/runtime/... ./internal/projectconfig/...` produces 3 failures:
    - `clauderun.TestDispatch_PreparedPromptBannerReflectsOptions`
    - `driver.TestClaudeRunDispatch_ExpandsTemplatedNodeFields`
    - `driver.TestEndToEnd_SubstitutionAndPromptLog`

    All three fail with `prompt has unfilled placeholders after substitution: ${sut_namespace}, ${driver_adapter}, ${driver_port}, ${system_test_path}`.

    **Root cause.** These placeholders previously lived only in the standalone phase docs, where `clauderun.Dispatch` resolved them via `assetsync.MaterializeProject` against `ProjectConfig.PlaceholderMap()` at materialization time (see `clauderun.go:201-211`). The dispatch-time `findUnfilledPlaceholders` ran only over the prompt body — which was thin and never carried these path placeholders. After this plan inlined the phase docs into the prompts, those placeholders are now in the prompt body itself, but `driver.seedScopeState` (`driver/driver.go:474`) only seeds 5 keys (`repo_strategy`, `repos`, `architecture`, `language`, `allowed_roots`) and does not iterate `PlaceholderMap()`. So the substitution leaves them unfilled and the dispatcher fails.

    This is a real runtime regression, not a fixture artifact — production dispatch will fail on first AT/CT structural cycle for any project where these paths apply.

    **Fix candidates (need decision):**
    1. **(Cleanest)** Extend `driver.seedScopeState` to also seed every entry of `cfg.PlaceholderMap()` into `sCtx`. ~5 lines of Go. Makes prompts genuinely self-contained at dispatch time. Tests pass naturally because the existing rigs all build a Config.
    2. Escape the 3 affected prompts' placeholders (e.g., backtick-quote `${driver_port}` so the substitution regex skips them). Preserves prior behavior where the agent saw literal `${...}` and was expected to resolve from project context. Hacky; agents shouldn't see unfilled placeholders.
    3. Leave the regression in place and document; defer the fix to its own plan.

    Option 1 is the cleanest and most in-line with the inlining intent, but it is Go-runtime code, which the plan's "What this plan does NOT do" explicitly excludes. Author decision required to expand scope (Option 1) or write a follow-up plan (Option 3).
  - ⏳ Spot-check rendered prompt — not started; cheap once the placeholder situation is decided.

## Hand-off dependencies

- **No dependency on other plans** for this sweep itself. The inlining is a self-contained text/file refactor.
- **Coordinate with the scope-authority shift follow-up plan** when that gets written: the inlined prompts created by this plan will be the surface that the scope-authority shift updates. Land this first (smaller blast radius); the follow-up then edits the prompt frontmatter in place.
- **Coordinate ordering with [20260519-0911 ESIR WRITE phase doc](20260519-0911-author-esir-write-phase-doc.md)** if that plan has not yet landed: 0911 authors `change/structure/external-system-interface-redesign.md` (currently a stub or missing); this plan inlines whatever exists at the time the inlining commit lands. Land 0911 first if you want the inlined prompt to carry the full ESIR phase content rather than a placeholder.

## What this plan does NOT do

- **Does NOT change the `scope:` field of any prompt.** Frontmatter stays at `scope: {}` with the existing comment. The scope-authority shift is a separate follow-up plan.
- **Does NOT touch `internal/atdd/runtime/architecture/phase-scopes.yaml`.** That file's elimination is the scope-authority shift's job, not this plan's.
- **Does NOT touch `internal/assets/global/docs/atdd/process/shared/*`.** Those docs have N>1 readers by construction; they stay as the single source of truth for shared rules.
- **Does NOT touch the Bucket C docs.** `disable-tests.md`, `enable-tests.md`, and `path-keys.md` remain as cited specs for Go-runtime code; deleting them would orphan Go-comment pointers and break the §Conventions specification chain.
- **Does NOT delete the Bucket D docs.** The DRAFT marker preserves the forward-looking intent. A future plan can decide whether to write the missing prompt readers or to delete the docs outright.
- **Does NOT introduce new frontmatter conventions.** No `status:` field, no `phase_doc:` field; DRAFT is a textual title suffix only.
- **Does NOT decide the future `at-green-component` variant.** Item 1's TODO marker flags the question without answering it.
- **Does NOT modify any tests, BPMN fixtures, or runtime Go code** beyond the two `process-flow.yaml` `agent:` param updates in Item 1.
