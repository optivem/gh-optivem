# 2026-05-20 11:55 UTC — Cleanup dangling `phase_doc:` references in `process-flow.yaml`

**Status:** STUB — run `/refine-plan` on this file before `/execute-plan`.

**Origin:** Surfaced by `plans/20260520-1415-verify-deleted-2026-05-20-plans.md` (audit walk; not committed). The `system-implementation-refactoring-rename` plan (deleted in `83c7b6a`) flagged the dangling `phase_doc:` reference at `process-flow.yaml:1327` as "drop the field or repoint" and asked for the redesign-cycle siblings at lines 1291 + 1300 to be reviewed too — but the decision was kicked down the road. The wider problem (7 more dangling refs under `docs/atdd/process/change/behavior/*.md`) is the parent plan's Item 4 residual from `plans/20260519-1537-post-meta-bpmn-topics.md`.

---

## The gap

`docs/atdd/process/` no longer exists — every process doc was inlined into its corresponding prompt during the sweep that landed in commit `4b44722` ("inline process docs into prompt readers"). Yet `internal/atdd/runtime/statemachine/process-flow.yaml` still carries 10 hard-coded `phase_doc:` paths into that vanished tree:

```
process-flow.yaml:381  phase_doc: docs/atdd/process/change/behavior/at-red-test.md
process-flow.yaml:398  phase_doc: docs/atdd/process/change/behavior/at-red-dsl.md
process-flow.yaml:435  phase_doc: docs/atdd/process/change/behavior/at-red-system-driver.md
process-flow.yaml:594  phase_doc: docs/atdd/process/change/behavior/ct-red-test.md
process-flow.yaml:612  phase_doc: docs/atdd/process/change/behavior/ct-red-dsl.md
process-flow.yaml:632  phase_doc: docs/atdd/process/change/behavior/ct-red-external-system-driver.md
process-flow.yaml:652  phase_doc: docs/atdd/process/change/behavior/ct-green-external-system-stub.md
process-flow.yaml:1291 phase_doc: docs/atdd/process/change/structure/system-interface-redesign.md
process-flow.yaml:1300 phase_doc: docs/atdd/process/change/structure/external-system-interface-redesign.md
process-flow.yaml:1327 phase_doc: docs/atdd/process/change/structure/system-implementation-refactoring.md
```

The two AT_GREEN/AT_REFACTOR-shape entries at lines 498 + 549 already carry `phase_doc: ""` (post-collapse decision); the parameterised `phase_doc: ${phase_doc}` lines at 780, 795, 837, 936, 971, 1098, 1124, 1213 are templates that resolve to the above hard-coded values via call_activity.

## Open question (refinement)

**Drop or repoint?**

- **Drop:** delete the `phase_doc:` field on every writing-agent node. The merged prompts under `internal/assets/runtime/prompts/atdd/` are self-contained — agents don't need a separate doc path. Aligns with memory `feedback_drop_dont_relocate.md` (upstream mechanism = inlined prompt; drop the dead weight).
- **Repoint:** rewrite each `phase_doc:` to point at the corresponding prompt path under `internal/assets/runtime/prompts/atdd/*.md`. Keeps a one-to-one mapping from BPMN node → readable doc, which may be useful for diagram generation or human walk-throughs.

Refinement should also verify whether anything in the runtime *reads* `phase_doc:` today (grep `internal/atdd/runtime/` for `phase_doc`). If nothing reads it, "drop" is the obvious answer.

## Non-goals

- Restructuring `process-flow.yaml` shape beyond the `phase_doc:` field.
- Re-creating `docs/atdd/process/`. The inlining decision stands.
- Auditing other `${...}` placeholders in `process-flow.yaml` for similar drift. Scope is `phase_doc:` only.
