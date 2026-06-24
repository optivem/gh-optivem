Triage open GitHub issues in a repo — classify each as done, obsolete, duplicate/merge, or still-valid by comparing against current repo state. Returns a structured report; never closes or modifies issues.

## Input

`$ARGUMENTS` is the repo (e.g. `optivem/courses`) and optionally a subset of issue numbers or a date range. If no repo is given, ask.

## Process

1. **List issues** — `gh issue list --repo <repo> --state open --limit 200 --json number,title,body,labels,createdAt,updatedAt`. If there are more than ~60 issues, fetch in pages and note the total.

2. **For each issue**, read the title and body. Extract the claim being made (what the issue wants done). Be careful: titles are often shorthand — the body usually has the real scope.

3. **Compare against current repo state.** The relevant repos live under the workspace root (sibling dirs to the `claude` repo). Common ones:
   - `../courses/` — course content, lesson plans, rules
   - `../shop/` — shop templates
   - `../actions/`, `../github-utils/`, `../gh-optivem/` — tooling
   - `../hub/`, `../optivem-testing/` — test harnesses

   Use Glob/Grep/Read on the local copies. Do **not** use `gh api` to read files from external repos (per CLAUDE.md).

4. **Classify each issue** into exactly one bucket:
   - **done** — the change described has been made. Cite the file path + evidence.
   - **obsolete** — the premise no longer applies (feature removed, approach abandoned, superseded by different decision). Explain why.
   - **duplicate/merge** — overlaps with another issue. Name the other issue number and what to merge into what.
   - **still-valid** — work remains. One-line reason.
   - **unclear** — body is too vague to judge, or you'd need to ask the user. List the question.

5. **Be conservative with "done" and "obsolete".** A missing keyword is not proof of absence — check multiple spellings, synonyms, and related paths before concluding. If in doubt, mark **unclear**, not done.

6. **Duplicate detection.** After classifying individually, do one pass looking for overlaps across the full list — especially issues filed months apart that describe the same thing (e.g. #61 and #71 both about comprehension checks).

## Output

A single markdown report with these sections:

```
## Summary
- Total open: N
- Done: N   Obsolete: N   Duplicate: N   Still-valid: N   Unclear: N

## Done (safe to close)
- #NN — <title> — <evidence: file:line or brief explanation>

## Obsolete (safe to close)
- #NN — <title> — <why no longer relevant>

## Duplicate / Merge
- #NN → merge into #MM — <what overlaps>

## Still-valid (keep open)
- #NN — <title> — <one-line reason>

## Unclear (need user input)
- #NN — <title> — <question>
```

Keep each bullet to one line. Do not pad. The user will review and decide closures — you do not take action.

## Rules

- Read-only. Never run `gh issue close`, `gh issue comment`, or `gh issue edit`.
- Never use `gh api` to read external repo file contents. Use local clones under the workspace.
- Do not create plan files or intermediate docs. Return the report inline.

## GitHub rate-limit discipline (strict)

The authenticated REST limit is 5000 req/hr shared across all parallel agents. Respect it:

- **One batched call** for all issue bodies — `gh issue list --repo <repo> --state open --limit 200 --json number,title,body,labels,createdAt,updatedAt`. Never `gh issue view` per issue.
- **Zero `gh` calls** in the per-issue loop. All evidence comes from local files (Glob/Grep/Read on workspace sibling repos).
- If you genuinely need extra `gh` calls (e.g. checking if a referenced repo is archived), batch related queries and cap the total at ~5 per triage run. State the count in your report footer.
- Never call `gh api` for file contents. If you need a file from an external repo, stop and ask the caller to clone it locally.
