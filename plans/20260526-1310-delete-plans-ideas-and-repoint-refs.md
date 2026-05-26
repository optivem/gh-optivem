# Delete `plans/ideas/` brainstorm files and repoint references

## Origin / intent

Conversation with user (2026-05-26) flagged `plans/ideas/` as no longer
relevant. Audit found the folder is **not** abandoned scratch — it holds
five BPMN-refactor brainstorm files that are explicitly referenced from
two load-bearing places:

- `internal/atdd/runtime/statemachine/process-flow.yaml` (the runtime's
  process catalog) — header comment names them as the canonical source
  of process names, and the per-cycle commentary cites them as the
  cascade definition.
- `plans/20260525-1057-bpmn-refactor-design.md` (the design plan that
  drove the YAML) — enumerates all five files and cross-references them
  from individual Q-decisions.

The brainstorm files' content has since been **fully absorbed** into the
YAML (which is now the canonical, runtime-executed process catalog) and
into the design plan (which records every decision inline). The
brainstorm files are working drafts that no downstream artifact needs
anymore.

User decision (2026-05-26): **delete the 5 files + repoint references**.
Process-flow.yaml itself becomes the canonical source for process
names and the per-cycle cascade; the design plan stops cross-referencing
deleted artifacts.

Adjacent observation: `plans/ideas/2-bpmn-refactor-mid-level.md` is the
file that introduced the obsolete `gh optivem build-system` /
`compile-system` / `run-tests` command names that this morning's
process-flow.yaml fix corrected (sibling work, same session). Removing
the file removes the last surviving copy of those wrong names.

## Design decisions (resolved 2026-05-26)

1. **Process-flow.yaml becomes the canonical source for kebab-case
   process names.** The header comment that today defers to the
   brainstorm files is rewritten to state that the YAML below is itself
   canonical. No external pointer survives the deletion.
2. **The cascade commentary at process-flow.yaml:807-809 drops the
   brainstorm-file citation.** The cascade is fully defined by the YAML
   process bodies (`change-system-behavior`, `write-and-verify-acceptance-tests`,
   etc.); the comment is reworded to point at those processes by name,
   not at the deleted markdown.
3. **`plans/20260525-1057-bpmn-refactor-design.md` loses its
   brainstorm-file enumeration block but retains its Q-decisions
   verbatim.** Each Q-decision's path reference becomes a parenthetical
   note that the brainstorm artifact is no longer kept; the decision's
   substance is unchanged. The design plan is a historical decision
   record — its Q-rows are not rewritten, only the dead path strings are
   removed/annotated.
4. **No content from the brainstorm files is rescued.** They are
   declared fully absorbed. If a reviewer disputes this for any
   specific file, the path forward is to copy the missing content into
   the YAML or design plan before deleting — not to keep the brainstorm
   folder around.
5. **Plan execution is autonomous through Item 1 and gated for Item 3.**
   Item 1 (file deletions) and Item 2 (yaml header rewrite + cascade
   comment rewrite — surgical, mechanical) are pure deletion/rename
   work, no body rewrites. Item 3 (design-plan annotations) touches
   roughly seven Q-rows of historical prose and lists every line; the
   executor lists the proposed edits and waits for approval before
   applying.

## Scope

In scope:

- Delete the five files:
  - `plans/ideas/1-bpmn-refactor-low-level.md`
  - `plans/ideas/2-bpmn-refactor-mid-level.md`
  - `plans/ideas/3-bpmn-refactor-high-level.md`
  - `plans/ideas/4-bpmn-refactor-cycle-level.md`
  - `plans/ideas/5-bpmn-refactor-top-level.md`
- Remove the now-empty `plans/ideas/` directory.
- Rewrite `internal/atdd/runtime/statemachine/process-flow.yaml`
  header comment (around line 46-48) to drop the `plans/ideas/*` pointer.
- Rewrite the cascade commentary inside the same file (around lines
  807-809) to drop the `plans/ideas/3-...` / `plans/ideas/4-...`
  citation.
- Edit `plans/20260525-1057-bpmn-refactor-design.md` to drop the
  five-bullet brainstorm-file enumeration (~lines 69-75) and annotate
  the path references inside individual Q-decisions (lines 264, 385,
  414, 459, 535-536, 574, 588) so they don't point at deleted files.

Out of scope:

- Any rewrite of the design plan's Q-decisions themselves. Their
  substance stays verbatim; only dead path strings are touched.
- Any new content authored from the brainstorm files. The design
  decision in this plan is that the YAML + design plan already capture
  everything; if that turns out to be wrong for a specific file, the
  plan stops and content is rescued before deletion proceeds.
- Anything under `plans/deferred/` or `plans/upcoming/` — those are
  separate working folders.

## Items

### Item 1 — Delete the five brainstorm files + the now-empty folder

**Files touched:**

- `plans/ideas/1-bpmn-refactor-low-level.md` (delete)
- `plans/ideas/2-bpmn-refactor-mid-level.md` (delete)
- `plans/ideas/3-bpmn-refactor-high-level.md` (delete)
- `plans/ideas/4-bpmn-refactor-cycle-level.md` (delete)
- `plans/ideas/5-bpmn-refactor-top-level.md` (delete)
- `plans/ideas/` (remove directory)

**Steps:**

1. Confirm no untracked or staged changes inside `plans/ideas/` via
   `git status -- plans/ideas/`. If any exist, halt and surface them —
   another agent may have in-flight work in the folder.
2. `git rm plans/ideas/*.md`.
3. Verify the directory is empty and remove it (`Remove-Item` on
   Windows). Git tracks file deletions, not empty directories, so no
   additional staging step is needed.

**Autonomous?** Yes — pure deletion. No body rewrites, no renames in
referenced code.

**Verification:** `git status` shows the five files staged for deletion;
no other working-tree changes appear from this item.

### Item 2 — Repoint `process-flow.yaml` header + cascade commentary

**File touched:** `internal/atdd/runtime/statemachine/process-flow.yaml`

**Current state (header, around line 46-48):**

```
#   - Process names match the canonical kebab-case task names from
#     plans/ideas/{1..5}-bpmn-refactor-*-level.md. Cross-file connectedness
#     (Q-new-3 vocabulary, Q-new-6 acceptance-test naming) is applied.
```

**Target state:**

```
#   - Process names are kebab-case canonical. This YAML is itself the
#     source of truth — historical brainstorm files that named these
#     processes have been retired.
```

**Current state (cascade comment, around lines 807-810):**

```
  # other process calls this HIGH today: the brainstorm files
  # (plans/ideas/3-bpmn-refactor-high-level.md +
  # plans/ideas/4-bpmn-refactor-cycle-level.md) define the cascade
  # `change-system-behavior` → `write-and-verify-acceptance-tests` →
```

**Target state:**

```
  # other process calls this HIGH today: the cascade
  # `change-system-behavior` → `write-and-verify-acceptance-tests` →
```

(Drop the two intermediate "the brainstorm files (…) define" lines; the
sentence resumes with the existing cascade chain.)

**Steps:**

1. Edit the two regions with targeted `Edit` calls (each `old_string` is
   unique in the file).
2. Re-grep for `plans/ideas` inside the YAML to confirm zero remaining
   matches.

**Autonomous?** Yes — surgical comment edit, no semantic change.

**Verification:** `go build ./internal/atdd/runtime/statemachine/...` is
clean (comments don't compile, but this catches any accidental code
brush). `grep -n 'plans/ideas' internal/atdd/runtime/statemachine/process-flow.yaml`
returns nothing.

### Item 3 — Annotate dead path references in the design plan

**File touched:** `plans/20260525-1057-bpmn-refactor-design.md`

**Reference sites (verified via `Grep` 2026-05-26):**

- Lines ~69-75: the five-bullet "brainstorm files" enumeration in the
  intro.
- Line ~264: "Items 2–5 'refine the LOW/MID/HIGH/PEAK brainstorm' by
  applying the resolved Q-decisions to `plans/ideas/1-4-*.md`."
- Line ~385: "Items 2–5 only apply Q-decisions to `plans/ideas/1-4-*.md`."
- Line ~414: "Q-late-2 — input-name / placeholder refinements in
  brainstorm. ✓ Applied directly to `plans/ideas/1-bpmn-refactor-low-level.md`."
- Line ~459: "Full table (lives in `plans/ideas/5-bpmn-refactor-top-level.md`)."
- Lines ~535-536: two bullets citing `plans/ideas/3-...` and
  `plans/ideas/4-...` as touch-points.
- Line ~574: "Strip from all five `plans/ideas/*.md` files: …"
- Line ~588: "brainstorm now lives in `plans/ideas/1-5-*.md`."

**Approach:**

- **Intro block (~69-75):** delete the five-bullet enumeration outright.
  Replace with a one-sentence note: "Brainstorm content has been fully
  absorbed into this design plan and into
  `internal/atdd/runtime/statemachine/process-flow.yaml`; the original
  `plans/ideas/` working files were retired 2026-05-26."
- **In-Q-row path references:** strip the path itself, keep the
  surrounding decision text verbatim. Where the path is the only
  meaningful content of a sub-clause (e.g. line 459's "lives in …"),
  the executor proposes the rewording and pauses for approval before
  applying — these are not pure path deletes.

**Steps:**

1. Read the design plan sections around each cited line to confirm the
   line numbers are still accurate (the file may have shifted).
2. For each site, draft the precise `Edit` (old_string → new_string)
   and list them all in a single message to the user.
3. **Gate:** wait for explicit approval before applying the design-plan
   edits. They touch historical decision prose; one of them might
   warrant a rewording the executor didn't anticipate.
4. After approval, apply the edits.
5. Re-grep for `plans/ideas` repo-wide to confirm zero matches.

**Autonomous?** No — content-edit gate. The intro deletion and the
mid-Q rewordings need user sign-off before commit.

**Verification:** `grep -rn 'plans/ideas' .` returns zero matches across
the repo (after Items 1–3 are all applied).

### Item 4 — Repo-wide sweep + commit

**Steps:**

1. Final repo-wide grep for `plans/ideas` — confirm zero matches.
2. `go build ./...` to catch any incidental breakage (none expected;
   nothing in Go references markdown plans).
3. List files staged for deletion + modification.
4. **Gate:** present the full diff summary to the user and ask for
   approval before commit (per
   [feedback_no_commit_without_approval]).
5. On approval, commit with a concise message naming the deletion and
   the two repoint sites.

**Verification:** `git log -1 --stat` shows the five deletions + the
two file modifications; no other surprises.

## Cross-references

- Sibling work this session: `process-flow.yaml` command-string rename
  from `gh optivem run-tests` to `gh optivem test run` (and four other
  noun-verb fixes). That edit also touched `bindings.go`, `gates/bindings.go`,
  and `bindings_test.go`. The brainstorm file `2-bpmn-refactor-mid-level.md`
  was the last surviving copy of the wrong names; deleting it here
  finalises that cleanup.
- The design plan `plans/20260525-1057-bpmn-refactor-design.md` is **not**
  itself deleted by this plan. It stays as the historical record of the
  refactor decisions.
