# Plan (deferred): Add `smoke_test` as a Family B canonical key

**Date filed:** 2026-05-18

**Filed from:** [SSoT phase-scope plan (20260518-1530)](../20260518-1530-atdd-phase-scope-ssot.md), item 1's γ-extension refinement discussion.

## Why deferred

During SSoT phase-scope refinement, the user described the actual layout under `system_test/`:

> `system_test_path` is like the root, and inside it we have `acceptance_test`, `smoke_test`, `dsl_port`, `dsl_core`, etc.

`acceptance_test` is covered by the `at_test` Family B key (added by [predecessor plan 20260518-1500](../20260518-1500-atdd-phase-scope-placeholders.md) item 1). `dsl_port` / `dsl_core` are likewise covered. **`smoke_test` is not in the canonical Family B key list.**

The SSoT plan deliberately keeps the canonical-key contract owned by its predecessor and treats new keys as that predecessor's territory. Adding `smoke_test` here would be scope creep and would risk an inconsistent half-add (key but no per-language stems, key but no `placeholders.md` example block update, etc.). The predecessor's pattern of "add to `canonicalPathKeys()` + `pathStems()` + `placeholders.md` example block" should be repeated for `smoke_test` in its own small plan.

## Work this plan would do

Mirror the predecessor's items 1–3 for `smoke_test`:

1. **`internal/projectconfig/paths_defaults.go:canonicalPathKeys()`** — append `"smoke_test"` to the canonical slice (after `dsl_core`, preserving the append-at-end ordering invariant).
2. **`internal/projectconfig/paths_defaults.go:pathStems()`** — add per-language smoke-test path defaults. Values MUST be **deterministically constructed** per `[[feedback_paths_deterministic_no_guessing]]`:
   - Pin against the `shop` template's actual smoke-test layout if it has one (read at execute time and use the literal stems).
   - If the `shop` template does NOT yet have smoke tests, do not invent layout. Block this plan on adding canonical smoke-test directories to the `shop` template first — file a sibling plan that creates the `shop` reference layout, then resume this plan.
   - No `TODO: layout` markers, no (a)/(b)/(c) ladder. Either the layout is pinned by a real reference (template), or the plan blocks on creating that reference.
3. **`internal/assets/global/docs/atdd/process/placeholders.md`** — extend the Family B example block to include `smoke_test`.
4. **Update the `DefaultPaths` docstring** at `paths_defaults.go` lines 5–24 — bump "seven" to "eight" and include `smoke_test` in the comma-separated key list. (The predecessor plan made this a sub-step; carry forward.)
5. **Optionally add `smoke_test` to a phase in `phase-scopes.yaml`** — only if there's a real BPMN phase that writes smoke tests. As of refinement, no such phase appears in `process-flow.yaml`, so this is likely a vocabulary addition only.

## Open design questions to resolve at pickup

1. **Is `smoke_test` ATDD-cycle-relevant?** Smoke tests typically run post-deploy, not during AT/CT cycles. If no AT/CT phase writes smoke tests, this is purely vocabulary (so that `gh-optivem.yaml paths:` can declare the path, but no `check_phase_scope` invocation will use it). Confirm with the user.
2. **Per-language layout.** Read the `shop` template at pickup to discover the actual smoke-test layout. If the template does not yet have smoke tests, file a sibling plan to add canonical smoke-test directories to `shop` first — this plan blocks on that. Do not invent layout values (per `[[feedback_paths_deterministic_no_guessing]]`).
3. **Smoke-test agent.** If a smoke-test phase ever joins the BPMN process flow, it'd need a `phase-scopes.yaml` entry referencing `smoke_test`. Out of scope for this plan unless the user wants both at once.

## Pre-requisites

- Predecessor plan ([20260518-1500](../20260518-1500-atdd-phase-scope-placeholders.md)) must have landed (it locks the canonical-key contract this plan extends).
- SSoT plan ([20260518-1530](../20260518-1530-atdd-phase-scope-ssot.md)) does **not** need to have landed first — these are independent additions.

## Out of scope

- Adding BPMN phases that exercise smoke tests (separate concern; smoke testing's place in the ATDD cycle is undecided).
- Smoke-test runner integration with `verify` actions (separate from path vocabulary).
- Renaming any existing Family B keys.

## Hand-off

Pick up when: someone needs to declare a smoke-test path in `gh-optivem.yaml`, OR when an explicit smoke-test phase is added to `process-flow.yaml`.
