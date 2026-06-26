# 2026-06-26 06:11:00 UTC — Fix release-stage bump-patch-version reusable-workflow permission error

## TL;DR

**Why:** `gh-release-stage.yml` fails GitHub's workflow validation ("Invalid workflow file", run 28207902796): the `bump-patch-version` caller job inherits the workflow-level `contents: read` default but calls a reusable workflow whose `bump` job needs `contents: write`. GitHub forbids a called workflow from requesting more permission than its caller is granted.
**End result:** The `bump-patch-version` caller job carries its own `permissions: contents: write` override (mirroring the sibling `run` job), the workflow validates and dispatches cleanly, and the workflow-level `contents: read` default is preserved for every other job.

## Outcomes

What we get out of this — the goals and deliverables:

- `gh-release-stage` dispatches without the "Invalid workflow file … Line 165" validation error.
- The `bump-patch-version` job is reachable and runs after `check` + `run`, with write access to call `gh-bump-patch-version.yml`.
- Least-privilege preserved: only `bump-patch-version` is elevated to `contents: write`; the workflow default stays `contents: read` and the read-only `post-release-stage` caller job stays on the inherited `contents: read`.

## ▶ Next executable step (resume here)

Edit `.github/workflows/gh-release-stage.yml`: in the `bump-patch-version` job (currently at lines 165-169), insert a job-level `permissions:` block between `needs: [check, run]` and `uses: ./.github/workflows/gh-bump-patch-version.yml`:

```yaml
  bump-patch-version:
    name: Bump Patch Version
    needs: [check, run]
    permissions:
      contents: write
    uses: ./.github/workflows/gh-bump-patch-version.yml
    secrets: inherit
```

Do not touch the workflow-level `permissions:` at lines 12-13, and do not add permissions to `post-release-stage`. This is the only edit.

## Steps

- [ ] Step 1: In `.github/workflows/gh-release-stage.yml`, add `permissions:` with `contents: write` to the `bump-patch-version` job, between `needs: [check, run]` and `uses:`.
- [ ] Step 2: Confirm no other changes — workflow-level default stays `contents: read` (lines 12-13); `post-release-stage` job stays unmodified (read-only is correct for it).
- [ ] Step 3: Verify the YAML still parses (no "Invalid workflow file" error) on the next `workflow_dispatch` of `gh-release-stage`, and that the `bump-patch-version` job is reachable. Static validation only — no local build/test reproduction is possible.
