# Backlog conflict triage against `1725-scoped-implement-by-layer-channel`

## TL;DR

**Why:** With `plans/20260530-1725-scoped-implement-by-layer-channel.md` fully
landed, the remaining `plans/backlog/*` items need a single, authoritative ruling
on which can be picked up **independently** and which **contend the surfaces 1725
just restructured** — so the next agent/operator doesn't reopen a stale
`process-flow.yaml` line range or re-anchor a flag against the wrong
`implement_commands.go`.
**End result:** a tiered triage (🟢 safe / 🟡 one-shared-file, mechanical / 🔴
contends a 1725 seam) over all 12 backlog plans, plus the re-anchoring work each
🔴 plan needs before it can be executed.

**Status:** ANALYSIS-ONLY meta-plan — no code, no agent prompts. This is a
coordination index, not an execution ticket. Each row points at its own backlog
plan, which stays the unit of execution.
**Created:** 2026-06-04 10:24 CEST

> **This is a meta-plan, not an extension of 1725.** It does not edit 1725 or any
> backlog plan; it cross-references them. Per house rule "new plan, never extend":
> when a 🔴 plan is later re-anchored, the re-anchoring is recorded **in that
> plan's own file**, and this index is updated to point at it.

## Why this exists

`1725-scoped-implement-by-layer-channel` is **fully landed** (Items 0–7
committed; only operator acceptance remains). So "conflict with 1725" is no longer
a *concurrent-execution* question — it is two narrower ones:

1. **Textual overlap** — does the backlog plan edit a file 1725 rewrote, so its
   line references / patch anchors are now stale?
2. **Semantic dependency** — does it rely on a structure 1725 changed?

### 1725's owned surfaces (the collision set)

Any backlog plan touching one of these must be checked against the **post-1725**
state, not the state its author wrote it against:

- `internal/atdd/runtime/statemachine/process-flow.yaml` — the
  `write-and-verify-acceptance-tests` cascade is now **split by the
  `shared-contract` slice** (truncated before `GATE_SYSTEM_DRIVER_PORTS_CHANGED`)
  and **re-gated on `dsl-port-changed`** before the channel-unrolled adapter tail.
  **Any line range into that cascade region is stale.**
- `internal/atdd/runtime/driver/{target,scoped,driver,channels}.go` — the
  `--target`/`--channel` selector, the git-state resume/footprint detector, and
  the `driver.Run` routing.
- `implement_commands.go` — `--target` / `--channel` flags + positional `issue`.
- `internal/assets/runtime/agents/atdd/system-driver-adapter-implementer.md` —
  now channel-aware via a `${channel}` param.
- `internal/projectconfig` `PlaceholderMap` path resolution — the footprint
  detector resolves write-scope keys (incl. the `<driver-adapter>/<ch>` channel
  subtree) through it.

## Triage

### 🟢 Safe — disjoint surfaces, pick up in any order

| Backlog plan | Surfaces | Why clear |
| --- | --- | --- |
| `20260502-103000-fix-unfiltered-workflow-badges-academy-wide.md` | `README.md`, `internal/steps/readme.go` | No overlap. **Already carries a picked-up-by-agent marker — check it's not in flight before starting.** |
| `20260511-1702-shop-frontend-react-to-typescript-rename.md` | `internal/steps/names.go`, `apply_template.go`, `MAPPING.md`, `atdd-task.md` | Scaffold/template surfaces; none touched by 1725. |
| `20260525-1418-structured-bpmn-execution-trace.md` | `trace/trace.go`, `dispatch_{spy,expect}_test.go`, new invariants module | Trace + test infra, disjoint from the cascade and the driver package. |
| `20260518-1530-smoke-test-family-b-key.md` | `paths_defaults.go::canonicalPathKeys/pathStems`, docs | Purely **additive** new path key; 1725's footprint detector reads `PlaceholderMap` generically, so a new key cannot break it. |

### 🟡 Coexists — one shared file, mechanically separable

Re-anchor against current state before patching; no semantic clash.

| Backlog plan | Shared file | Note |
| --- | --- | --- |
| `20260430-150508-minimize-tokens-and-latency-in-clauderun-dispatch.md` | `driver/driver.go` | 1725 wired `driver.Run` here; this plan threads issue body into prompts. Different concerns, same file — mechanical merge. `clauderun.go`/`prompt.tmpl` disjoint. |
| `20260528-1302-suppress-subprocess-stderr-non-verbose.md` | `implement_commands.go` (help/verbose region) | Distinct from 1725's flag-parsing additions, but same file. `bindings.go` + `trace.go` disjoint. |
| `20260515-0950-ticket-type-feature-rename-and-config.md` | `process-flow.yaml` (ticket-type gates / `intake/parse.go`) | Touches a **different region** of the YAML than the cascade slice — separable, but large and process-flow-touching, so re-verify against the post-1725 file. |
| `20260526-1430-reconcile-defaultpaths-with-shop-template-layout.md` | `internal/projectconfig/paths_defaults.go` | **Semantic coupling, not textual.** 1725's resume detector narrows to the `<driver-adapter>/<ch>` subtree via `PlaceholderMap`; changing emitted path layout shifts what it resolves. **Coordinate with `plans/20260604-0955-configurable-per-channel-adapter-folders.md`** (the 1725 spin-off about exactly these per-channel folders). |

### 🔴 Contends a 1725 seam — re-derive before executing

Do **not** treat these as independent. Each needs its anchors re-derived against
the landed `shared-contract` slice / flag surface first.

| Backlog plan | Collision | Re-anchor needed |
| --- | --- | --- |
| `20260527-1147-dsl-implementer-ct-system-driver-scope.md` | Targets `process-flow.yaml` **lines 901–998** — the cascade region 1725 split into `shared-contract`. | Line ranges are stale; re-locate the CT-path `dsl-implementer` dispatch + its gates in the post-1725 YAML. |
| `20260525-1753-implement-pre-refine-check-and-post-refactor-offer.md` | Hits **both** `process-flow.yaml` **and** `implement_commands.go` — the two surfaces 1725 rewrote most. | Re-anchor the pre-run gate against the new flag surface (positional issue + `--target`); confirm the post-run block doesn't fight the per-slice commit/handoff model. |
| `20260526-1746-rebuild-onboard-external-system.md` | Re-inserts a call-activity node + edges into `process-flow.yaml` + `transitions_test.go`. | 1725 D-external folded external-system into the `--target test` slice — reconcile re-insertion with that decision before re-adding a separate node. |
| `20260526-1754-rebuild-checklist-progress-tracking.md` | Re-inserts CYCLE resume-gates into `process-flow.yaml` + `bindings.go`. | 1725 deliberately made resume **git-state-derived, no status cursor** (see its Resume-mechanism Non-goal). Reconcile the checklist auto-tick/resume gates with that model, not the pre-2026-05-26 one. |

## Recommended sequencing

1. **Drain 🟢 first** — they unblock with zero coordination. (Confirm the
   workflow-badges pickup marker is stale/abandoned before claiming it.)
2. **🟡 next, one at a time** — re-anchor, patch, commit; the single shared file
   makes serial execution cheap. Run `reconcile-defaultpaths` **with**
   `20260604-0955-configurable-per-channel-adapter-folders.md`, not against it.
3. **🔴 last, and only after a re-anchor pass** — for each, the first execution
   step is "re-derive line ranges / node insertion points against the current
   `process-flow.yaml`," recorded in that plan's own file (not here).

## Do NOT

- **Do not edit 1725 or any backlog plan from this file.** This is an index;
  re-anchoring is recorded in the target plan per "new plan, never extend."
- **Do not trust any `process-flow.yaml` line number written before 2026-06-04**
  in the 🔴 / `ticket-type` / `dsl-implementer-ct` plans — the cascade was
  restructured by 1725 Item 2a.
- **Do not re-add a resume status file / cursor** when executing the checklist
  plan — 1725's git-state resume model is authoritative; the checklist gates must
  conform to it.
- **Do not double-claim the workflow-badges plan** without checking its existing
  pickup marker and `git status` first.

## Related

- `plans/20260530-1725-scoped-implement-by-layer-channel.md` — the landed plan
  this triage is measured against.
- `plans/20260530-1702-channels-field-channel-by-channel.md` — 1725's dependency;
  shares the channel axis + common-layer ownership.
- `plans/20260604-0955-configurable-per-channel-adapter-folders.md` — 1725
  spin-off; the semantic partner of the `reconcile-defaultpaths` backlog plan.
