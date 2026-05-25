# BPMN acceptance-test HIGH rename (Q-new-6)

> **Parent plan:** `plans/20260525-1057-bpmn-refactor-design.md` — decision recorded as **Q-new-6** in the Cross-file connectedness section.
> **Trigger:** review of `plans/ideas/3-bpmn-refactor-high-level.md` surfaced that three HIGH tasks on the acceptance-test side share a near-identical name stem (`write-and-verify-...`) while each carries a different scope — outer (pinned param), middle (full flow with cascading DSL/adapter impl), inner (test-code-only primitive). The duplication-feel is a naming smell, not a structural one; this plan executes the rename that pushes scope into each layer's name.

## Inputs (read these first)

- **Decision:** `plans/20260525-1057-bpmn-refactor-design.md` — Q-new-6 (full rationale, alternatives considered, rename map). Do NOT re-litigate the doctrine here; this plan is execution-only.
- **Structural shape unchanged:** Q31 Option D (thin wrappers + parameterized core + inner primitive) stays. Only the names change.

## Scope

- **In scope:** rename 3 HIGH task headings, all internal call sites + invocation references across the 4 affected files. No structural changes, no behaviour changes, no semantic changes.
- **Out of scope:** DSL / driver / contract-test HIGH names — they have no 3-layer stack and no name-stem duplication (rationale in Q-new-6 point 4).
- **Out of scope:** any change to `process-flow.yaml` — it has not yet been authored (Phase C). Q-new-6 lands BEFORE YAML encoding so the YAML is written with the new names from the start.

## Rename map

| Layer | Before | After |
|---|---|---|
| Outer (cycle entry, expected=Failure) | `write-and-verify-tests-fail` | `write-and-verify-acceptance-tests-fail` |
| Outer (cycle entry, expected=Success) | `write-and-verify-tests-pass` | `write-and-verify-acceptance-tests-pass` |
| Middle (canonical full flow) | `write-and-verify-tests` | `write-and-verify-acceptance-tests` |
| Inner (test-code primitive) | `write-and-verify-acceptance-tests` | `write-and-verify-acceptance-test-code` |

**Critical ordering note for find/replace:** the middle's new name (`write-and-verify-acceptance-tests`) is the SAME string as the inner's old name. A naive global find/replace will collide. Rename order MUST be:

1. **First** rename the inner: `write-and-verify-acceptance-tests` → `write-and-verify-acceptance-test-code`.
2. **Then** rename the middle: `write-and-verify-tests` → `write-and-verify-acceptance-tests`.
3. **Then** rename the outer wrappers: `write-and-verify-tests-fail` → `write-and-verify-acceptance-tests-fail`; `write-and-verify-tests-pass` → `write-and-verify-acceptance-tests-pass`.

Steps 2 and 3 can technically be one pass (the wrappers contain the middle's old name in their bodies AND in their own names, so a single `write-and-verify-tests` → `write-and-verify-acceptance-tests` global replace catches both heading and call sites). But run them as separate steps to keep the diff readable per layer.

## Items

### Item 1 — Rename in `plans/ideas/3-bpmn-refactor-high-level.md`

Three heading renames + internal call-site references inside the file. Order matters (see above).

1. Inner rename:
   - Heading `## write-and-verify-acceptance-tests` → `## write-and-verify-acceptance-test-code`.
   - Internal call site inside the middle (currently `write-and-verify-tests` step 1): `write-and-verify-acceptance-tests` → `write-and-verify-acceptance-test-code`.
2. Middle rename:
   - Heading `## write-and-verify-tests` → `## write-and-verify-acceptance-tests`.
   - Internal call sites inside the two outer wrappers: `write-and-verify-tests` → `write-and-verify-acceptance-tests`.
3. Outer rename:
   - Heading `## write-and-verify-tests-fail` → `## write-and-verify-acceptance-tests-fail`.
   - Heading `## write-and-verify-tests-pass` → `## write-and-verify-acceptance-tests-pass`.

**Verify before commit:** `grep "write-and-verify-tests\b" plans/ideas/3-bpmn-refactor-high-level.md` returns no matches; `grep "write-and-verify-acceptance-tests\b"` matches only the new middle heading + its call sites in the outer wrappers.

### Item 2 — Rename invocation sites in `plans/ideas/4-bpmn-refactor-cycle-level.md`

Two cycle-level call sites currently invoke the outer wrappers:

- `change-system-behavior` step 1: `write-and-verify-tests-fail` → `write-and-verify-acceptance-tests-fail`.
- `cover-system-behavior` step 1: `write-and-verify-tests-pass` → `write-and-verify-acceptance-tests-pass`.

**Verify before commit:** `grep "write-and-verify-tests" plans/ideas/4-bpmn-refactor-cycle-level.md` returns no matches.

### Item 3 — Update wording in `plans/20260525-1057-bpmn-refactor-design.md`

Q-new-6 itself is already authored. Other entries reference the old names in *historical/rationale* prose; update only the cells that read as live state, not the ones that read as decision history.

- **Cross-check inventory rows (lines ~32, ~41):** diagram #6 (`at-cycle`) and #20 (`red-phase-cycle`) absorption-target cells mention `write-and-verify-tests-fail`. Update to `write-and-verify-acceptance-tests-fail`.
- **Q31 entry (line ~446–451):** "Final shape" enumeration lists `write-and-verify-tests`, `write-and-verify-tests-fail`, `write-and-verify-tests-pass`. Update the names in the live-shape enumeration; **leave the Q-new-1 / fac98ea historical references untouched** (they describe a prior state).
- **Q31.b entry (line ~455):** wrapper names mentioned in prose. Update.
- **Q6.a entry (line ~443):** mentions parameterized core + wrappers. Update names.
- **Q-new-1 entry (line ~511):** describes the Q-new-1 rename (`write-and-verify-red-tests` → `write-and-verify-tests`). The arrow's TARGET (`write-and-verify-tests`) is now stale, but Q-new-1 is a historical decision record. **Leave Q-new-1 unchanged** — its target represents the Q-new-1 result, which Q-new-6 then supersedes (the supersession is already recorded in Q-new-6's "Supersedes" line).

**Verify before commit:** after edits, `grep "write-and-verify-tests\b\|write-and-verify-tests-fail\|write-and-verify-tests-pass" plans/20260525-1057-bpmn-refactor-design.md` should match ONLY inside Q-new-1's prose (historical) and inside Q-new-6's "Before" column of the rename table.

### Item 4 — Update wording in `plans/20260525-1517-bpmn-refactor-yaml-and-diagrams.md`

The "HIGH orchestrations (Q31 = D)" section (lines ~46–51) reads as live state and references the old names. Update:

- Line ~47: `write-and-verify-tests` → `write-and-verify-acceptance-tests` (parameterized core).
- Line ~48: `write-and-verify-tests-fail` → `write-and-verify-acceptance-tests-fail`; `write-and-verify-tests-pass` → `write-and-verify-acceptance-tests-pass`.

Add a reference to Q-new-6 next to the existing "(Q31 = D)" label so future readers can find the naming refinement.

**Verify before commit:** `grep "write-and-verify-tests" plans/20260525-1517-bpmn-refactor-yaml-and-diagrams.md` returns no matches outside Q-new-6 reference.

### Item 5 — Repo-wide sanity sweep

Final guard against stragglers in surfaces the prior items might have missed.

1. `grep -rn "write-and-verify-tests\b\|write-and-verify-tests-fail\|write-and-verify-tests-pass" .` from repo root.
2. Allow-list expected matches:
   - Q-new-1 prose in `plans/20260525-1057-bpmn-refactor-design.md` (historical decision record).
   - "Before" column of Q-new-6's rename table in the same file (rename-map artifact).
   - "Before" column of this plan's rename map (rename-map artifact).
3. Any other match is a straggler — fix in this item, do not defer.

**Note:** as of plan authoring, `process-flow.yaml` has not yet been authored (Phase C); no YAML matches are expected. If Phase C runs before this plan completes, those YAML files become additional touch-points and this item picks them up.

### Item 6 — Commit

Single commit covering all five items. Suggested message:

```
plans/bpmn-acceptance-test-rename: apply Q-new-6 naming refinement

Push scope into the name at each layer of the 3-tier acceptance-test HIGH
hierarchy. Structural shape unchanged (Q31 Option D stays); names now
make outer/middle/inner scope visible per Q-new-6.
```

## Open questions

None — Q-new-6 in the parent plan settled the doctrine. If new questions surface during execution, add them as `### Q60+` entries here and pause for resolution before continuing.
