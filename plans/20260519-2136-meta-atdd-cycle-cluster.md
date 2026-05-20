# Meta-plan: sequence the ATDD AT-cycle + doctrine-vs-prompts cluster

**Date:** 2026-05-19 21:36 UTC (status refreshed 2026-05-20)
**Type:** Coordination meta-plan (audit + sequencing only). No new code or doc edits in this plan; it tells the user *which plans to refine/execute in what order*.

## Status snapshot (2026-05-20)

The architectural conflict this meta-plan was created to mediate has been resolved:

- **1537 drained** — refined and removed; items promoted to `20260520-1145-runtime-references-tree-rename.md`, `20260520-1145-system-implementation-refactoring-rename.md`, `20260520-1213-collapse-at-green-backend-frontend.md`, `20260520-1109-ac-refinement-and-at-refactor-agent-steps.md` (commit `bf77476`).
- **1701 (Part 1) deleted** — was a zombie stub.
- **1116-part2 deleted** — premise obsoleted by inline-phase-docs sweep (`4b44722`); per-phase rules content recoverable from git if ever needed.
- **1144 moved to `plans/deferred/`** — items 2-6 executed (`d680e74`); only item 7 (failing-legacy detector) remains, blocked on the legacy plan.
- **0911 deleted** — Item 1 landed (`90f6fdc`); Items 2 + 4 obsoleted by the inline sweep (the standalone ESIR phase doc was inlined into its prompt, making the "verify references resolve" checks moot).

**What's still live in this cluster:**

- `plans/20260518-1116-legacy-coverage-cycle.md` — needs refinement, then settles the legacy-marker convention that unblocks 1144 item 7.

Everything else cited below describes the historical sequencing decision that produced today's state. Kept for archival reference; not load-bearing for new work.

## Why this existed

`/coordinate-plans` was invoked across the 6 plans then in `plans/`. On audit, **only 3 of the 6 were real coordination input**; the others were stubs, closed, or scratchpads. More importantly, there was a **latent architectural conflict** between two of the plans that had to be resolved before either side moved — refining downstream plans first would have entrenched an assumption that was about to be overturned.

## In-scope plans (audit)

| Plan | Real status | Disposition |
|---|---|---|
| `20260516-1701-atdd-at-cycle-absorb-internal-assets.md` | Zombie stub — body is signposts only; the "items 1–4b" it claims to contain were never written into this file (schemas ended up as Snapshots B/C/D inside `20260518-1144`). | **Decide delete vs flesh-out** (chore, anytime). |
| `20260518-1116-atdd-at-cycle-part2-per-phase-content.md` | Live, 🛑 NOT YET REFINED. Per-phase rules / framing / examples / cross-refs for `docs/atdd/process/at-cycle.md`. Item 7 depends on legacy plan; item 17 depends on supporting-docs migration. | **Refine, but only after 1537.** |
| `20260518-1116-legacy-coverage-cycle.md` | Live, ⚠️ NOT YET REFINED. Defines the legacy-cycle sibling + marker convention. Stale pickup marker dropped 2026-05-19 (`dc0967e`). | **Refine, after 1537.** Key dependency for 1144 item 7 + part-2 item 7. |
| `20260518-1144-atdd-bpmn-orchestration.md` | ✅ Executed 2026-05-19 (`d680e74`): items 2, 3, 4, 5, 6 landed. Items 8, 9 obsolete (superseded by SSoT plan). | **Only item 7 remains**, blocked on legacy marker. Not coordination material on its own. |
| `20260519-0911-author-esir-write-phase-doc.md` | Item 1 landed (`90f6fdc`). Items 2 + 4 deferred. Item 4 may be unblocked now that plan 0922 has executed. | **Verify Item 4 manually** (5-min chore). Item 2 stays parked behind fixture precondition. |
| `20260519-1537-post-meta-bpmn-topics.md` | Scratchpad, 12 discussion seeds. Header parks it on `0929` landing — and `0929` was discharged on 2026-05-19 (`55dfb18`), so **the park has effectively lifted**. | **Refine first.** Headlines change the substrate the other plans are written against. |

## The architectural conflict

Two visions of `internal/assets/global/` are currently in flight:

- **1701 Part 1 + 1116 Part 2:** "Eventually delete `internal/assets/`." `docs/atdd/process/at-cycle.md` becomes the canonical home; the four global process pages disappear.
- **1537 Items 2 + 5:** "Rename `internal/assets/global/` → `runtime/doctrine/atdd/`. Inline N=1-reader docs into their prompts; keep N≥2 shared docs (`scope.md`, `conventions.md`, `path-keys.md`, `at-green-system.md`) in the renamed tree."

Both can't be right. Refining the legacy plan or Part 2 before 1537's headlines are decided risks writing against a substrate about to be reshaped.

## Recommended sequence

1. **Refine `20260519-1537-post-meta-bpmn-topics.md`** — walk **only the headlines (items 1, 5, 6)**. Per 1537's own framing note: items 2, 3, 4 collapse out of item 5; items 11, 12 collapse out of item 6; items 7, 8, 9, 10 are orphan/drift cleanup that needs the tree shape first. Promote each accepted headline to its own dated plan file. Defer 7–12 for now.

2. **Refine `20260518-1116-legacy-coverage-cycle.md`** — now informed by 1537's decisions about the doctrine tree. Settles the legacy marker convention.

3. **Refine `20260518-1116-atdd-at-cycle-part2-per-phase-content.md`** — now informed by both 1537 (substrate) and the legacy plan (item 7's ordering rule). Several items may dissolve if Item 5 of 1537 inlines per-phase docs into prompts.

4. **Execute `20260518-1144` item 7** — failing-legacy detector. Blocks on step 2 settling the marker.

5. **Execute** the refined per-phase content plan (step 3 output) and any plans spun out of step 1.

## Chores (do anytime, independent)

- **1701 zombie cleanup:** investigate whether items 1–4b have a hidden home or were silently absorbed by 1144's Snapshots B/C/D. Most likely answer: delete `20260516-1701-...` outright (its content is now redundant).
- **0911 Item 4 verification:** plan 0922 has landed (`46ef833`, `be12d7b`, `470c67a`). Check that `process-flow.yaml:1108` and `task-external-system-interface-redesign.md:21` resolve to the new ESIR phase doc cleanly. If they do, close 0911.

## Why this is a meta-plan, not a coordination wave-plan

After 1537 is refined, the dependency chain across the remaining work is trivial:

```
1537 headlines (refine + promote)
  → legacy-coverage-cycle (refine)
    → 1144 item 7 (execute)
    → at-cycle-part2 (refine, then execute)
```

There are no parallel-safe batches that benefit from running concurrent agent sessions — each step needs its predecessor's decisions. So the meta-plan is just this sequencing note; no wave-plan or per-batch file-list is needed.

## Hand-off

Next concrete step: `/refine-plan plans/20260519-1537-post-meta-bpmn-topics.md`, walking items 1, 5, 6 only.
