# Fold `phase-scopes.yaml` into `process-flow.yaml`

## Origin / intent

Spun off from `plans/20260526-1448-agent-prompt-fixes.md` Item 2a
during refinement (2026-05-26). The parent plan is scoped to
prompt-body fixes under `internal/assets/runtime/prompts/atdd/*.md`;
this work is a runtime/SSoT refactor that touches Go loaders, BPMN
YAML, and the `phase_scopes_test.go` build-time guard — beyond the
prompt-files scope.

This plan is a **precondition** for Items 3, 4, and 9 of the parent
plan (which all key on per-phase scope data). The parent plan
cross-references this one as a precondition; the two refinements
can proceed in parallel.

## Why fold instead of remap

`phase-scopes.yaml` is a sidecar SSoT keyed by phase id, joined at
runtime with `process-flow.yaml`'s node ids through a foreign-key
relationship. That FK is **currently broken and the build-time
guard skipped**:
`TestPhaseScopes_ForwardFK_PhasesExistInBPMN`
(`internal/atdd/phase_scopes_test.go:85`) carries `t.Skip(...)`
because the five-level BPMN refactor
(`plans/20260525-1517-bpmn-refactor-yaml-and-diagrams.md` Item 3)
renamed every phase id in `process-flow.yaml`; `phase-scopes.yaml`
still references the pre-refactor IDs (`AT_RED_TEST`,
`CT_RED_DSL`, `LEGACY_*`, etc.). The skipped FK is a live drift
incident.

The original Phase-D plan was to remap the keys. During refinement
the question surfaced: **why does the sidecar exist at all?** The
scope of a phase is a property of the phase node, and
`process-flow.yaml` already carries every other per-node field
(`type`, `agent`, `params`, …). Folding scope into the node
eliminates the FK entirely — there's nothing to remap, nothing to
join, nothing to drift.

Trade-off captured in
`feedback_question_second_file_ssots.md`: two-file SSoT joins are
a drift source; fold sidecars into the primary file when entries
are 1:1 properties of entities already defined there.

## Scope

1. Extend every **writing-agent node** in
   `internal/atdd/runtime/statemachine/process-flow.yaml` to carry
   inline `read:` and `write:` lists. The target set is the same one
   `phase_scopes_test.go::writingAgentNodeIDs` already computes —
   every node whose `agent:` (UserTask) or `params.agent`
   (CallActivity) resolves to a concrete, non-`human`,
   non-templated writing agent. Templated dispatchers
   (`${agent}`) carry scope on the parent call-activity, not on the
   inner node, so the test's existing filter is the authoritative
   definition of "writing-agent node" here too.
   - Always-explicit, no flat shorthand: every writing-agent node
     declares both keys even when they match. Duplication accepted
     per `feedback_explicit_lists_no_subset_constraint.md`.
   - No subset constraint between `read:` and `write:` — both
     directions of asymmetry occur in practice (placeholder writes
     give `read ⊊ write`; driver-port guardrails give `read ⊋
     write`).
2. Delete `internal/atdd/phase-scopes.yaml`.
3. Rewrite `internal/atdd/phase_scopes.go` (the loader). Drop the
   `//go:embed phase-scopes.yaml` directive, the `phaseScopesYAML`
   byte slice, the `yaml.Unmarshal` path, and the `PhaseScopes`
   type entirely. Expose scope as a method on `statemachine.Engine`
   (or a small accessor function in `internal/atdd/runtime/
   statemachine`) with the shape
   `Scope(nodeID string) (read, write []string, ok bool)`. All
   four current consumers (`bindings.go`,
   `bindings_test.go`, `process_commands.go`, `embed.go`) read
   scope by phase id — none iterate the whole map — so the
   accessor matches every actual use site and removes a parallel
   data structure that would otherwise have to stay in sync with
   the engine. Keep `NonWritingAgents` and `FamilyAPathKeysInScope`
   in `phase_scopes.go` (they're still consumed by the test
   guards in this package). The file can be renamed if the type
   removal makes its current name misleading — settle that during
   execution, not now.
4. Update consumers.

   **Code consumers (loader calls — need rewrite to the new
   `engine.Scope(nodeID)` accessor):**
   - `internal/atdd/runtime/actions/bindings.go` and
     `bindings_test.go`.
   - `process_commands.go` (the `gh optivem process scope <phase>`
     CLI) — single-node lookup instead of a join.
   - `process_commands_test.go` — currently calls
     `atdd.LoadPhaseScopes()` directly and is `t.Skip`'d pending
     the same Phase-D remap. Re-author against the new accessor;
     drop the skip.

   **Comment-only updates (no code uses the loader; only stale
   `phase-scopes.yaml` mentions in code comments):**
   - `internal/atdd/runtime/gates/bindings.go:228` — comment
     references `phase-scopes.yaml` as the SSoT; reword to point
     at `process-flow.yaml` node scope.
   - `internal/atdd/runtime/agents/embed.go:89,96` — same shape.
     Cross-link parent-plan Item 4 (`## Scope` block rendering)
     in case the rendering work lands here.
5. Delete dead `LEGACY_*` entries during the fold; legacy tests
   live in the same paths as change-cycle tests per
   `feedback_legacy_tests_no_marker.md`, so no separate node /
   scope is needed in `process-flow.yaml` either.
6. Rewrite `internal/atdd/phase_scopes_test.go`. Each existing
   guard gets a decision; one new guard added.
   - `TestPhaseScopes_ForwardFK_PhasesExistInBPMN` — **drop
     entirely**. The FK does not exist any more (scope lives on
     the node), so neither can the test. Includes deleting the
     `allNodeIDs(eng)` helper if no other guard consumes it.
   - `TestPhaseScopes_ReverseFK_WritingAgentsScoped` — **keep**,
     retargeted. Today it iterates `writingAgentNodeIDs(eng)` and
     checks `ps.Phases[nodeID]` or the `scope: none` frontmatter
     fallback. After the fold it iterates the same set and checks
     `engine.Scope(nodeID)` for `ok == true` (or the same
     frontmatter fallback). Semantics unchanged — every
     writing-agent node has scope or declares `scope: none`.
   - `TestPhaseScopes_LayersAreCanonical` — **keep**, retargeted.
     Iterate `engine.Scope(nodeID)` over every writing-agent
     node; assert every layer in `read:` and every layer in
     `write:` resolves through `canonicalPathKeys()` or equals
     `system-path`.
   - `TestPhaseScopes_NoDuplicateLayersWithinPhase` — **keep**,
     retargeted. Check duplicates *within* `read:` and *within*
     `write:` separately. Identical entries across `read:` and
     `write:` are not duplicates — they're the symmetric case the
     explicit-lists rule accepts.
   - `TestPhaseScopes_NonEmptyLayerLists` — **keep**, retargeted.
     Assert both `read:` and `write:` are non-empty per
     writing-agent node. (Pinning the per-list semantics now:
     even placeholder-writing phases like `AT_RED_TEST` read
     `at-test` + `dsl-port` while writing `dsl-core`
     placeholders, so neither list is ever legitimately empty.)
   - **New:** `TestPhaseScopes_ReadWriteShape` — assert every
     writing-agent node carries both keys (no flat shorthand, no
     missing key). Anchors the schema decision from Item 1.
   - The test file may be renamed alongside `phase_scopes.go` if
     Item 3 renames the production file — settle the file name
     during execution, not now.

## Out of scope (handled by the parent plan)

- The actual `read:` / `write:` data per phase (parent plan
  Item 3 — applied to the folded node scope once this lands).
- The frontmatter SSoT decision for prompt files (parent plan
  Item 4).
- Universal `scope:` declaration in every prompt frontmatter
  (parent plan Item 9).
- Prompt-body edits (parent plan Items 1, 5, 6, 7, 8, 10, 11).

## Memory invalidation

`feedback_no_deferred_mechanism.md` in the user's memory
references `phase-scopes.yaml` directly ("every writing-agent
phase must have its scope pinned in `phase-scopes.yaml`"). After
this plan lands the wording must be updated to point at
`process-flow.yaml` node scope. Update the memory file as part
of this plan's execution.

## Acceptance

- `internal/atdd/phase-scopes.yaml` deleted; the `//go:embed`
  directive and `phaseScopesYAML` byte slice in
  `phase_scopes.go` are gone with it.
- Every writing-agent node in `process-flow.yaml` (the set
  `writingAgentNodeIDs` computes) carries inline `read:` and
  `write:` lists, both always present, no flat shorthand.
- Phase-scope access goes through an
  `engine.Scope(nodeID) (read, write []string, ok bool)`
  accessor on `statemachine.Engine` (Item 3). The `PhaseScopes`
  type is removed; all four code consumers (`actions/bindings.go`,
  `actions/bindings_test.go`, `process_commands.go`,
  `process_commands_test.go`) use the accessor.
- `gh optivem process scope <phase>` works against the new
  single-file SSoT and returns the same data shape it does today
  (test parity check before/after — the CLI output format should
  be unchanged so any downstream consumer doesn't break).
- The five existing guards in `phase_scopes_test.go` land in
  their Item-6 decided shape: Forward FK deleted; Reverse FK,
  Layer-resolution, NoDuplicates (per list), and NonEmpty (per
  list) kept and retargeted at `engine.Scope`; new
  ReadWriteShape guard added.
- `process_commands_test.go` no longer carries the Phase-D
  `t.Skip` — the skip is dropped along with the FK migration it
  was waiting on.
- Stale comment references to `phase-scopes.yaml` in
  `gates/bindings.go:228` and `agents/embed.go:89,96` are
  updated to point at `process-flow.yaml` node scope.
- Updated `feedback_no_deferred_mechanism.md` memory wording.
- Downstream test suite green; no behavioural drift in scope
  enforcement (i.e. the same phases scope to the same path sets
  they did before the fold — only the SSoT location moved).
- Parent plan items 3, 4, 9 unblocked.
