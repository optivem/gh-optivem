# Publish `install-gh-optivem` as a remote `optivem/actions` action

**Goal:** Stop scaffolding shop's `install-gh-optivem` composite action into student
repos. Move the action to the published `optivem/actions` repo, reference it remotely
everywhere, and delete both shop's local copy and the gh-optivem `copyGitHubActions`
scaffolder function.

**Why:** Scaffolded acceptance-stage workflows reference `uses: ./.github/actions/install-gh-optivem`
(a *local* path), so the scaffolder must copy that action dir into every student repo
(`internal/steps/apply_template.go` `copyGitHubActions`). That copy ships shop's
meta-prerelease *source-build* machinery (the `ref`-set clone+`go build` path) into
student repos that only ever need the default `gh extension install optivem/gh-optivem`
path. `optivem/actions` already hosts ~40 versioned reusable actions (including
`retry@v1`, which this action already depends on), so the action has a natural remote
home and the local copy becomes unnecessary.

## Context / facts established

- **17 references** to `./.github/actions/install-gh-optivem` in shop, all passing an
  identical `with:` block (`ref: ${{ inputs.gh-optivem-ref }}` + `github-token`). So the
  migration changes only the `uses:` line per reference; the `with:` block is untouched.
  - 15 student-facing / shop-CI: `*-acceptance-stage.yml`, `*-acceptance-stage-legacy.yml`
    (monolith+multitier × dotnet/java/typescript), `cross-lang-system-verification.yml`,
    `drift.yml` (×2).
  - 2 shop-internal: `_meta-prerelease-pipeline.yml`, `_prerelease-pipeline.yml`.
- The action is **fully relocatable**: it clones gh-optivem from the absolute URL
  `https://github.com/optivem/gh-optivem`, references `optivem/actions/retry@v1`
  absolutely, and has no relative-path file deps. Sibling actions in `optivem/actions`
  already cross-reference as `optivem/actions/<name>@v1` (e.g. `render-system-stage-summary`),
  so the in-repo `retry@v1` reference needs no change.
- `optivem/actions` versions via a floating `@v1` tag (latest fixed `v1.0.8`), advanced
  automatically by `.github/workflows/update-v1.yml` on push to `main`.
- The only consumer of the local action path outside shop's workflows is
  `gh-optivem/internal/steps/apply_template.go` (`copyGitHubActions` + its call site).
  No docs, scripts, tests, or golden fixtures reference it. `install-gh-optivem` appears
  in gh-optivem *only* in `apply_template.go`.
- All three repos (gh-optivem, shop, actions) are on `main`, clean except for the
  uncommitted `copyGitHubActions` addition in gh-optivem. No concurrent-agent markers on
  any target file.

## ⚠️ Ordering constraint (hard)

The `@v1` tag must already include `install-gh-optivem` **before** shop references
`optivem/actions/install-gh-optivem@v1`, or every workflow run 404s on the action.
Execute the repos strictly in order: **actions → shop → gh-optivem**. Do not open the
shop PR until `v1` has advanced (verify the tag points at a commit containing the new
action dir).

## Items

### Phase 1 — `optivem/actions` (publish the action)

1. [ ] Create `actions/install-gh-optivem/action.yml` by moving shop's
   `shop/.github/actions/install-gh-optivem/action.yml` verbatim (it is already
   self-contained and relocatable; the `optivem/actions/retry@v1` reference stays as-is).
2. [ ] Conform the action to `optivem/actions` repo conventions: confirm it passes
   `lint-gh-usage.yml` and `lint-shell-policy.yml` (read those workflows; fix any
   shell-policy violations in the inline `run:` steps). Add a `README`/usage note if the
   repo convention requires one per action (check a sibling like `retry/`).
3. [ ] Merge to `actions/main` and confirm `update-v1.yml` advances `@v1` to a commit
   that contains `install-gh-optivem/` (the verification gate for Phase 2).

### Phase 2 — `shop` (switch to remote, delete local copy)

4. [ ] In all 17 referencing workflows, change
   `uses: ./.github/actions/install-gh-optivem` →
   `uses: optivem/actions/install-gh-optivem@v1`. Leave each `with:` block unchanged.
5. [ ] Delete `shop/.github/actions/install-gh-optivem/` entirely.
6. [ ] Grep shop for any remaining `actions/install-gh-optivem` reference (docs, drift
   checks, lint allowlists); none expected — confirm clean.

### Phase 3 — `gh-optivem` (remove the scaffolder copy)

7. [ ] Delete `copyGitHubActions` (`internal/steps/apply_template.go:104-114`) and its
   call site + log line in `ApplyTemplate` (`apply_template.go:188-190`). This reverts the
   uncommitted change rather than committing it.
8. [ ] Confirm no other gh-optivem code, test, or golden fixture references
   `install-gh-optivem` or `copyGitHubActions` (already verified: only this file). Build:
   `go build ./...`.

## Verification (user-driven)

- After Phase 1, manually confirm in the GitHub UI that the `optivem/actions` `v1` tag
  includes `install-gh-optivem/` before merging shop.
- Run (or let the next push trigger) one shop acceptance-stage workflow on a real branch
  to confirm the remote action resolves and installs gh-optivem (default path, empty ref).
- Run shop's meta-prerelease dry-run once to confirm the `ref`-set source-build path still
  works through the remote action.
- Scaffold one student repo (`gh optivem ...`) and confirm its acceptance-stage workflow
  references `optivem/actions/install-gh-optivem@v1` and contains no
  `.github/actions/install-gh-optivem/` dir.

## Out of scope

- No change to the action's two-path (released vs source-build) behavior.
- No change to the `with:` wiring (`gh-optivem-ref`, `github-token`) in any workflow.
- No broader rework of how scaffolded workflows install other tools.
