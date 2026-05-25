# BPMN MID/HIGH/CYCLE/TOP name-consistency audit

> **Parent plans:**
> - `plans/20260525-1057-bpmn-refactor-design.md` — design archive.
> - `plans/20260525-1645-bpmn-scope-doctrine.md` — scope-doctrine plan whose **Q45 C** spawned this follow-up.
>
> **Trigger.** Q45 in the scope-doctrine plan resolved that HIGH orchestrations carry no Scopes (scope lives only on MID tasks), but flagged a separate concern: cross-file naming and contract-shape drift between the brainstorm files. Example surfaced during the Q45 walk: HIGH has `implement-and-verify-external-system-driver-adapter-contract-tests` (singular `adapter`) while the MID task it ultimately wraps is `implement-external-system-driver-adapters` (plural). Per `feedback_new_plan_not_extend`, that audit lives here, not appended to the scope-doctrine plan.

## In scope

The four brainstorm files:

- `plans/ideas/2-bpmn-refactor-mid-level.md` (MID)
- `plans/ideas/3-bpmn-refactor-high-level.md` (HIGH)
- `plans/ideas/4-bpmn-refactor-cycle-level.md` (CYCLE)
- `plans/ideas/5-bpmn-refactor-top-level.md` (TOP)

Audit dimensions:

1. **Task-name consistency across layers.** Each layer's task names referencing the same domain noun should agree on singular vs plural, hyphenation, and stem shape. Example known drift: `adapter` (HIGH) vs `adapters` (MID) for the external-system driver-adapter task.
2. **`Inputs:` / `Outputs:` block drift.** A HIGH orchestration that wraps a MID task should declare Inputs/Outputs at least as specific as what its MID callee declares. Look for HIGH blocks missing an Input the MID requires, or HIGH declaring an Output its MID callee doesn't produce.
3. **`Steps:` shape consistency.** Steps numbered 1/2/3, `execute-agent` / `execute-command` / `execute-orchestration` verb usage uniform across layers.
4. **Cross-file invocation references.** Where HIGH/CYCLE/TOP names a MID task by name, the spelling must match the MID heading exactly (any post-Q-new-6 / post-rename references in particular).

## Out of scope

- **Scopes blocks.** Q45 already ruled HIGH/CYCLE/TOP carry no Scopes; Q43/Q44 pinned the MID Scopes shape. Re-litigating Scopes is not this audit's job.
- **The Q-new-6 acceptance-test HIGH rename.** Tracked separately in `plans/20260525-1659-bpmn-acceptance-test-rename.md`. This audit assumes that rename has landed; sequence-order is: Q-new-6 → this audit → Phase C YAML lock. If this audit picks up before Q-new-6 commits, re-baseline against the post-rename HIGH file.
- **Q40 (snake → kebab canonical-key rename).** Bigger, tracked in `plans/20260525-1645-bpmn-scope-doctrine.md`. This audit's findings should be authored in whichever case is current at execution time; if Q40 lands first, drift findings will use kebab; if not, snake.

## Items

_Item authoring deferred to a `/refine-plan` walk._ Estimated item shapes:

- **Pass 1 — Inventory.** Read all four brainstorm files; build a table of every task heading per layer + every cross-file invocation reference. One read each, no edits.
- **Pass 2 — Diff.** Spot pairs/triples that disagree (singular/plural, hyphenation, missing Inputs/Outputs propagation). Record findings as candidate edit items.
- **Pass 3 — Resolve.** Per finding, decide which side renames (usually the layer with the more specific/local name wins; canonical noun lives in MID; HIGH/CYCLE/TOP align to MID).
- **Pass 4 — Apply.** Mechanical edits per resolution.

## Prerequisites before execution

1. `plans/20260525-1659-bpmn-acceptance-test-rename.md` (Q-new-6) has committed; HIGH file is in its post-rename state.
2. `plans/20260525-1645-bpmn-scope-doctrine.md` is either complete (Q40 landed) or still pending — both states are fine, this audit doesn't depend on Q40.

## Cross-references

- Parent scope-doctrine plan: `plans/20260525-1645-bpmn-scope-doctrine.md` (Q45 C).
- Sibling rename plan: `plans/20260525-1659-bpmn-acceptance-test-rename.md` (Q-new-6).
- Phase C YAML/diagrams plan: `plans/20260525-1517-bpmn-refactor-yaml-and-diagrams.md` (this audit must precede Phase C lock).
- Memory `feedback_new_plan_not_extend` — rationale for spawning this as a fresh plan rather than appending to the scope-doctrine plan.
- Memory `feedback_no_layer_coding_in_names` — guides resolution direction when picking canonical singular/plural form.
