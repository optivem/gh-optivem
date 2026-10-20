# 2026-10-20 11:37:52 UTC — Replace flaky softprops "Create GitHub Release" step in prod-stage workflows

## TL;DR

**Why:** gh-acceptance-stage run 39299269758's parent (https://github.com/optivem/gh-optivem/actions/runs/39288526604) failed in `Run (multitier, monorepo, dotnet, java)` because the downstream test-app `prod-stage` died at `Create GitHub Release`: `softprops/action-gh-release@v3` created a draft, then `error finalizing release: HttpError` x3 → `Too many retries`. The action swallows the real HTTP error and leaves a stray draft release; 15 sibling prod-stages with the identical step passed, so it is a transient GitHub API failure made opaque by the action.
**End result:** Every prod-stage/release workflow creates its GitHub Release with `gh release create` (single published call, real API error surfaced), so a transient failure is diagnosable and no draft is left behind.

## Outcomes

- Prod-stage failures at release creation show the true HTTP status/message instead of bare `HttpError`.
- No orphaned `draft: true` releases after a failed run; a re-run is idempotent.
- All 14 shop prod-stage workflows (+ optivem-testing `release-stage.yml`, actions hotfix example) share the same step body.
- gh-acceptance-stage on gh-optivem is green across the matrix with non-draft releases.
- No gh-optivem Go code change needed (no reference to `action-gh-release` in this repo).

## ▶ Next executable step (resume here)

In the sibling `shop` repo, edit `.github/workflows/multitier-dotnet-prod-stage.yml` (step `Create GitHub Release`, ~lines 238-246) replacing `softprops/action-gh-release@v3` with a `run:` step using `gh release create "$TAG" --verify-tag --title "$TITLE" --notes-file "$NOTES"` (env: `GH_TOKEN: ${{ secrets.WORKFLOW_TOKEN }}`, `--repo ${{ github.repository }}`, TAG=`steps.promote.outputs.version`, TITLE=`steps.release-notes.outputs.title`, NOTES=`steps.release-notes.outputs.notes-file`), with a fallback branch: if a release for the tag already exists (draft), `gh release edit "$TAG" --draft=false --title ... --notes-file ...`. Then propagate to the other files in Step 2.

## Steps

- [ ] Step 1: Write the new step in `shop/.github/workflows/multitier-dotnet-prod-stage.yml` (as described above); no shell retry loop (rely on `gh` native behaviour, per repo convention).
- [ ] Step 2: Apply the identical step to the other shop prod-stage workflows: `{monolith,multitier}-{dotnet,java,typescript}-prod-stage.yml` and their `-cloud` variants (14 files total). Adjust only the step-output names where a workflow differs (e.g. `steps.promote.outputs.version`).
- [ ] Step 3: Apply the same replacement to `optivem-testing/.github/workflows/release-stage.yml:106` and `actions/tag-docker-images/examples/hotfix-release.yml:37`.
- [ ] Step 4: Confirm gh-optivem needs no change: grep `internal/scaffolding/steps/apply_template.go` and templates for the step name / `action-gh-release` (initial grep found none).
- [ ] Step 5: Verify — run actionlint/YAML sanity on edited workflows; diff the step body across the 14 shop files to confirm they are identical.
- [ ] Step 6: Commit/push shop, optivem-testing, actions (via `/commit`), then trigger gh-acceptance-stage (`workflow_dispatch`) on gh-optivem and confirm Phase 10 prod-stage passes for all matrix cells and the created release is non-draft.

## Notes

- Failure not reproducible locally (needs real GitHub release API); classified as flake + opaque-error defect in a third-party action.
- The stray draft release `v1.0.221` in `valentinajemuovic/test-app-8ee6ebc7-0138f4eee942bdad` needs no cleanup (disposable test repo).
