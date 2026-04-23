# Plan: Add §2 to workflow-auditor — extractable bash → composite action

## Motivation

The [workflow-auditor agent](../.claude/agents/workflow-auditor.md) currently has only §1 (`gh` CLI repo inference). A second recurring class of finding is inline `run:` bash that duplicates, or should become, a composite action under `optivem/actions`.

Concrete trigger: on 2026-04-23 we extracted `validate-version-unreleased` to fix release-stage's "already-published" failure mode. [gh-acceptance-stage.yml:135-156](../.github/workflows/gh-acceptance-stage.yml#L135-L156) still holds an inline copy of the same check — a case F hit the auditor should have flagged.

## Items

- [ ] **Add `§2 — Extractable bash → composite action` to [workflow-auditor.md](../.claude/agents/workflow-auditor.md)**
  Insert between existing §1 and the "§2 reserved" placeholder. Use the same structure as §1: rule statement, category labels, anti-patterns, provenance.

  Proposed categories:
  - **D — Duplicated bash logic.** Same pattern (not line-for-line) in `run:` blocks across 2+ workflow files in the scanned repos. Recommendation: extract to `optivem/actions/<name>/action.yml`, replace each occurrence with `uses:`.
  - **E — Shared-domain step without composite.** Single `run:` block > 10 lines invoking `gh api` / `gh release` / `gh workflow` / `git tag` / `git ls-remote` / `git describe` for a cohesive domain operation (version resolution, tag-existence check, release lookup, workflow trigger). Recommendation: search `optivem/actions/` for a fit; suggest new action if none.
  - **F — Near-duplicate of existing action.** Inline logic that a known composite in `optivem/actions` already provides. Recommendation: swap inline for `uses: optivem/actions/<name>@v1`.

  De-prioritize (note in examined-and-rejected, don't flag):
  - `run:` blocks ≤ 5 lines with no domain command (echo/jq/derivation of inputs)
  - Blocks that only write to `$GITHUB_OUTPUT` / `$GITHUB_STEP_SUMMARY` / `$GITHUB_ENV`
  - Blocks tightly coupled to workflow-specific `${{ github.* }}` context that wouldn't parameterize cleanly

- [ ] **Extend `Process` section**
  Add a per-`run:`-block classification pass, reusing the same file enumeration as §1.

- [ ] **Extend `Output` section**
  Add `## §2 findings` with subsections D / E / F mirroring §1's structure. Update Summary line to include D/E/F counts. Update "Chat return" section so top-3 items can come from §1 or §2.

- [ ] **Write provenance**
  Cite the 2026-04-23 `validate-version-unreleased` extraction — release-stage's need for a fail-fast check that would otherwise let GoReleaser waste ~60s building unpublishable artifacts, plus the matching inline copy still in `gh-acceptance-stage.yml:135-156`.

## Questions to resolve during evaluation

- **Scope of similarity matching.** "Same pattern (not line-for-line)" is vague — should category D require an exact AST match, a normalized-whitespace match, or fuzzy similarity (e.g. same 3+ command tokens in same order)? Start with exact-token-sequence matching and tighten later.
- **Cross-repo scope.** §1 scans the current repo plus siblings. Should §2 find cross-repo duplicates (bash in `shop/` that also exists in `gh-optivem/`)? Yes, probably — consolidation value is highest there. But this doubles the scan cost.
- **Existing-action lookup.** Category F requires the auditor to know every `optivem/actions/` composite's purpose. Approach: scan `optivem/actions/*/action.yml` and index by (name, description). Match candidates via keyword overlap in the `run:` block. Accept some false negatives — it's a seed rule, not an optimizer.
- **False-positive risk.** Inline bash that looks domain-y but is actually workflow-specific (e.g. reads `${{ needs.X.outputs.Y }}` in non-obvious ways) should be rejected, not flagged. The de-prioritize heuristics cover the obvious cases; will need to iterate based on first report.

## Out of scope

- Auto-fixing the findings. Auditor is read-only and always will be.
- Grading extraction ROI — a finding is raw observation; prioritization lives in the human review of the report.
- Broadening §2 to cover PowerShell or Python `run:` blocks. Bash-only for v1; revisit if non-bash duplication actually shows up in findings.
