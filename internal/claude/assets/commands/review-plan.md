Review a plan file against the current state of the codebase and revise it: remove items already done, remove items that are now obsolete, update items whose details have drifted, and keep the rest. The output is a revised plan file — no code changes are made.

## Input

The plan file is provided as `$ARGUMENTS`. If no argument is given, use the currently open file in the editor (from the IDE selection context). If no file is open either, ask the user which plan file to review.

The plan file path can be:
- A filename in the current repo (e.g. `MIGRATION.md`)
- A relative path from the academy workspace root (e.g. `actions/plans/20260420-foo.md`)
- An absolute path

Resolve the academy workspace root dynamically:
```bash
ACADEMY_ROOT="$(cd "$(git rev-parse --show-toplevel)/.." && pwd)"
```

## Plan format expectations

The plan file contains numbered or bulleted items (e.g. `## Step 1:`, `### 1.`, `- [ ] Step 1`, or similar). Parse whatever numbering/format is used. Preserve the file's existing structure (phases, headings, principle/target-state sections) — only revise the item list itself, not the surrounding narrative, unless the narrative itself is clearly invalidated.

## Phase 1: Audit

For each unchecked item in the plan:

1. **Read the item carefully** — what change does it describe, which files/actions/consumers does it name?
2. **Verify the item against current state.** Use Grep/Glob/Read to check:
   - If the item names a file, action, workflow, function, or flag — does it still exist? At the path/name stated?
   - If the item lists call sites or counts (e.g. "1 call site in `cleanup.yml`") — are those counts still accurate?
   - If the item describes a refactor — has the refactor already happened (target state already present)?
   - If the item references a related phase/item — is that dependency still relevant?
3. **Classify the item** into one of:
   - **Done** — the target state described by the item already exists in the code. Evidence required (file content, grep result).
   - **Obsolete** — the item no longer makes sense: the thing it refactors has been deleted, the approach was superseded by a different decision, the surrounding architecture moved. Evidence required.
   - **Needs revision** — still valid in principle, but specific details (paths, counts, consumer lists, scope) have drifted. Note what specifically needs to change.
   - **Still valid** — applies as written; no change needed.

Do not classify based on a quick read. For `Done` and `Obsolete` in particular, cite the concrete evidence you found (file path + line, grep hit count, missing file, etc.).

## Phase 2: Present findings

Present a structured summary to the user **before** editing the plan file:

```
## Review of <plan file>

### Done (proposed deletion)
- Item "<short title>" — <evidence, e.g. "create-release/action.yml:12-18 already requires an existing tag; implicit tag creation removed">

### Obsolete (proposed deletion)
- Item "<short title>" — <evidence, e.g. "find-release-by-run was deleted in commit abc123; no remaining callers">

### Needs revision (proposed edits)
- Item "<short title>" — current text says "1 call site in cleanup.yml" but grep shows 3 call sites in cleanup.yml and cleanup-nightly.yml. Propose: update consumer list.

### Still valid (no change)
- Item "<short title>"
- Item "<short title>"
```

Then ask: **"Apply these revisions? (yes / modify / skip)"**
- `yes` → proceed to Phase 3
- `modify` → take user's corrections and re-present
- `skip` → exit without touching the plan file

## Phase 3: Apply revisions

Only after approval:

1. Edit the plan file:
   - **Delete** all items classified as `Done` or `Obsolete`.
   - **Update** items classified as `Needs revision` with the specific edits listed in Phase 2.
   - **Leave** `Still valid` items untouched.
2. Preserve surrounding structure (principle, target state, phase headings). If a phase ends up with zero items after deletions, leave the heading with a one-line note like `_All items complete._` rather than deleting the phase — it preserves the historical shape of the plan.
3. If the plan file ends up empty (no items left in any phase), ask the user whether to delete the file entirely (per the Plan Processing rules in `courses/docs/rules/00-shared.md`).
4. **Do not commit.** Leave the revised plan file as an uncommitted change so the user can review the diff. Mention that `/commit` will pick it up.

## Rules

- **Evidence, not guesses.** An item is only `Done` or `Obsolete` if you verified it against the code. "Probably done" is not good enough — re-classify as `Needs revision` or `Still valid` until you can cite evidence.
- **Never silently delete.** Every deletion must appear in the Phase 2 summary with its evidence, and be approved, before being removed from the file.
- **Don't execute plan items.** This command revises the plan only; it never applies the changes the plan describes. Use `/execute-plan` for that.
- **Preserve author decisions.** If an item has an inline author comment (`VJ:`, `AUTHOR:`, recommended-option notes), keep that annotation when revising. Do not rewrite the author's voice.
- **Edit, never re-`Write`.** Apply the Phase 3 revisions as targeted `Edit`s to the affected lines; never re-`Write` the whole plan file for a handful of changes (`Edit` sends only the diff; a full re-`Write` resends the file — ~50× the tokens). Same plan-file write policy as `/create-plan` and `/refine-plan`.
- **One plan file per invocation.** If the user wants to review several, run the command once per file.
