# Eliminate `initRepo` to remove the new-repo push race

## Motivation

Acceptance run [25207926785](https://github.com/optivem/gh-optivem/actions/runs/25207926785) failed in `Run (multitier, monorepo, dotnet, java)` with:

```
! [remote rejected] main -> main (cannot lock ref 'refs/heads/main':
  is at 0ec1e6a390fab1e9b6e4496480feaf27945084cd
  but expected 4d304a2564c2d780f6ee6ec44e900d4b20906ccd)
```

This rejection format requires **both `is at X` and `expected Y` to be real commits** on the remote — i.e. the local clone observed `Y` as `origin/main` (recorded at clone time) and then sent a ref-update saying "advance from `Y` to <new>", which the server rejected because its current value is `X`.

The only way `main` ends up with two distinct commits in this pipeline is the existence of **two writers**:

1. `initRepo()` (`internal/shell/github.go:380`) — clones the empty repo, writes a placeholder README, commits "Initial commit", and pushes. This commit becomes the first state of `main`.
2. `CommitAndPush` in Phase 5 (`internal/steps/finalize.go:119`) — applies the template on the Phase 3 clone, commits, pushes.

Between (1) and Phase 3's clone, GitHub's git replicas can be at different points relative to (1)'s push. If Phase 3's clone hits a replica with a different observed state than the replica Phase 5's push hits, the push's "expected old" sha doesn't match the server's current sha and the push is rejected. With two writers there's always a race window, regardless of how robust the retry around any individual operation is.

## Goal

Eliminate the race by collapsing to **one writer**: `CommitAndPush` in Phase 5. The remote's `main` is unborn until Phase 5 pushes, at which point the push is `0000…0000 → <new-sha>` (ref creation, not advance) — a code path that cannot produce the observed error class.

## Non-goal

A full local-first pipeline reshuffle (`gh repo create --source=. --push`, with environments/secrets/SonarCloud moved after the create+push). That pattern is genuinely best-practice for greenfield-only scaffolders, but here it would force pipeline reordering for no extra correctness — the unified-clone flow below is already race-free for the bug we saw, and it preserves the existing-repo path unchanged.

## Approach: unified flow for new and existing repos

Same five steps regardless of whether the repo is new or pre-existing. Only step 1 short-circuits when the repo already exists.

1. **Ensure repo exists** — `gh repo view`; if missing, `gh repo create --public` (no auto-init, no `initRepo`).
2. **Clone** — `gh repo clone <repo> <dir>`. Verified empirically (gh 2.80.0): on an empty repo this exits 0, emits a warning, and produces a `.git` dir with `HEAD = refs/heads/main` already (GitHub sets the default branch on repo creation; the clone honors it). On populated repos: normal clone.
3. **Apply template** — every existing step in Phase 3 unchanged.
4. **Commit** — `git add -A`, `git commit` (skip if clean tree, as today; `commitAndPushRepo` already handles this for pre-existing repos at `finalize.go:157–166`).
5. **Push** — `git push -u origin main`. For empty remote: creates `main` (ref-creation push, verified to work end-to-end during the Q1 probe). For existing remote: fast-forward.

We considered adding `git symbolic-ref HEAD refs/heads/main` after the clone as defensive insurance against a user with a non-`main` GitHub account default branch. Skipped: the empirical probe showed HEAD is already at `refs/heads/main` for the realistic case, and the edge case (account configured with a different default) is rare and would surface as a clear, traceable push error if it ever occurred. Adding defensive code for an unobserved scenario isn't justified.

## File-by-file changes

### `internal/shell/github.go`

**Delete `initRepo()` and its call.**

- Line 337: remove `g.initRepo()` from `CreateRepo()`.
- Lines 376–412: delete `initRepo` function and its doc comment.
- Imports: `os`, `path/filepath` may no longer be needed in this file — verify and trim.

The `waitForRepoVisible` call stays; it still closes the GraphQL-vs-create race for downstream `gh api` calls (environments, secrets).

### `internal/steps/finalize.go`

**Change push to handle ref-creation. Keep plain `shell.Run` (no retry hardening).**

Line 172 today:

```go
if out, err := shell.Run("git push", false, true, repoDir); err != nil {
    log.Fatalf("git push failed in %s: %v\n%s", fullRepo, err, out)
}
```

Change to:

```go
// Plain Run, no retry: post-create replica lag is documented at clone time
// (see MustRunPostCreate) but not at push time -- by Phase 5 the repo has
// been touched by clone + several gh api calls, so every replica that
// matters has caught up. If push-time lag ever surfaces, design a targeted
// retry around the actual error string then; don't speculate now.
if out, err := shell.Run("git push -u origin main", false, true, repoDir); err != nil {
    log.Fatalf("git push failed in %s: %v\n%s", fullRepo, err, out)
}
```

`-u origin main` works for both cases: ref-creation on empty remote (the new-repo path; verified end-to-end during the Q1 probe), normal upstream-already-set push on existing repo (the `-u` re-applies the same upstream that the clone already set, harmless).

No retry classifier is added. Auth failures, permission errors, and branch-protection rejections are all permanent and should fail fast. The clone uses `MustRunPostCreate` because post-create clone lag was observed in production; push-time lag has not been observed and adding a retry on speculation would mask future real bugs.

### Tests

- `internal/config/config_system_test.go` — search for assertions on the "Initial commit" README being present in scaffolded repos. The README is now written by `steps.UpdateReadme` (already a Phase 3 step) and committed as part of the template apply, so the *content* is still there but the commit history loses the separate "Initial commit" filler. Tests asserting commit count or commit-message text need to be updated.
- Any unit test that exercises `CreateRepo()` and expects an `initRepo` side-effect — delete or rewrite.
- Add: a unit test that `Clone` on an empty repo produces a working tree with `HEAD → refs/heads/main` and no commits, and that a subsequent commit + push creates `main` on the remote. (May require a local git fixture rather than network.)

### Follow-on

- `internal/shell/github.go:341–344` — the comment block on `waitForRepoVisible` references the race that `MustRunPostCreate` mitigates. Wording stays accurate; no change needed.
- The `PreExistingRepos` branch in `CreateRepo()` (line 324–327) still does the right thing (skip create, log warning). No change.

## Validation

1. Local: run `go build ./...` and `go vet ./...`.
2. Unit tests: `go test ./internal/...`.
3. Acceptance smoke: trigger `gh-acceptance-stage` for a single matrix combination first (e.g. `multitier, monorepo, dotnet, java` — the one that failed) before the full matrix.
4. Confirm orphaned test repos from prior failed runs are cleaned up (the test from run 25207926785 left `valentinajemuovic/test-app-b897668f-469ad5cb9f5772fd`).

## Open questions

1. ~~**Empty-repo clone exit code**~~ — Resolved by empirical probe (gh 2.80.0): `gh repo clone` of an empty repo exits 0, emits a warning to stderr, and produces a working `.git` dir with `HEAD = refs/heads/main`. CI uses gh 2.89.0 — re-confirm on first acceptance run, but no behaviour change is expected across this minor-version bump.
2. ~~**`git symbolic-ref` placement**~~ — Resolved: not adding the call. The empirical probe showed it would be a no-op in the realistic case, and the edge-case scenario (non-`main` GitHub account default branch) is unobserved.
3. ~~**Retry classifier for push**~~ — Resolved: no retry. Best-practice retry policy ties retries to *observed* transient failure modes; push-time replica lag is unobserved, and adding speculative retry hardening would mask future real failures (auth, permission, branch protection). If push-time lag ever surfaces, target the retry around the actual error string then.
