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

Same six steps regardless of whether the repo is new or pre-existing. Only step 1 short-circuits when the repo already exists.

1. **Ensure repo exists** — `gh repo view`; if missing, `gh repo create --public` (no auto-init, no `initRepo`).
2. **Clone** — `gh repo clone <repo> <dir>`. Works on empty repos (emits a warning, produces a `.git` dir with an unborn `main`) and on populated repos (normal clone).
3. **Pin local branch to `main`** — `git -C <dir> symbolic-ref HEAD refs/heads/main`. No-op for existing repos. For empty repos, this guarantees the first commit lands on `main` regardless of the user's `init.defaultBranch` config (could be `master` on older git installs).
4. **Apply template** — every existing step in Phase 3 unchanged.
5. **Commit** — `git add -A`, `git commit` (skip if clean tree, as today; `commitAndPushRepo` already handles this for pre-existing repos at `finalize.go:157–166`).
6. **Push** — `git push -u origin main`. For empty remote: creates `main` (ref-creation push). For existing remote: fast-forward.

## File-by-file changes

### `internal/shell/github.go`

**Delete `initRepo()` and its call.**

- Line 337: remove `g.initRepo()` from `CreateRepo()`.
- Lines 376–412: delete `initRepo` function and its doc comment.
- Imports: `os`, `path/filepath` may no longer be needed in this file — verify and trim.

The `waitForRepoVisible` call stays; it still closes the GraphQL-vs-create race for downstream `gh api` calls (environments, secrets).

### `internal/shell/github.go` (`Clone` method)

**Pin HEAD to `main` after clone.**

In `Clone(dest)` (line 441), after the clone succeeds and the `.git` check passes, run:

```go
MustRun("git symbolic-ref HEAD refs/heads/main", g.DryRun, dest)
```

This is idempotent: for a populated clone HEAD is already `refs/heads/main`, the call is a no-op write. For an empty clone, it pins the unborn branch name.

Question to confirm during implementation: does `gh repo clone` of an empty repo exit non-zero or just emit a warning? If non-zero, `MustRunWithRetry` at line 442 will treat it as a failure and abort. If so, switch to a non-failing check or filter on the specific empty-repo warning.

### `internal/steps/finalize.go`

**Change push to handle ref-creation and add post-create-style retry tolerance.**

Line 172 today:

```go
if out, err := shell.Run("git push", false, true, repoDir); err != nil {
    log.Fatalf("git push failed in %s: %v\n%s", fullRepo, err, out)
}
```

Change to:

```go
if out, err := shell.RunPostCreate("git push -u origin main", repoDir); err != nil {
    log.Fatalf("git push failed in %s: %v\n%s", fullRepo, err, out)
}
```

`-u origin main` works for both cases: ref-creation on empty remote, normal upstream-already-set push on existing repo (the `-u` re-applies the same upstream that the clone already set, harmless).

The retry tolerance handles the adjacent replica-lag class: an empty repo created in Phase 2 may not be visible to every git replica at Phase 5 push time. Today the push is plain `shell.Run`; we want the same "retry on any non-rate-limit error in the post-create window" behaviour that `MustRunPostCreate` provides for the clone. Either reuse `MustRunPostCreate` directly (it `Fatalf`s, which matches the current behaviour) or add a `RunPostCreate` non-Fatal sibling and keep the explicit `log.Fatalf` here for the formatted message.

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

1. **Empty-repo clone exit code** — does `gh repo clone` of a no-commit repo exit 0 with a warning, or non-zero? Affects whether `MustRunWithRetry` in `Clone()` swallows it correctly. Verify before editing `Clone`.
2. **`git symbolic-ref` placement** — better in `Clone()` (one line, minimal blast radius) or in a new `steps.PrepareLocalRepo` step (more visible in the pipeline)? Leaning toward `Clone()` for minimal churn.
3. **Retry classifier for push** — `MustRunPostCreate` retries on *any* non-rate-limit error. For the push, that's broad: a real auth failure or a malformed ref would also retry. Acceptable in the post-create window (≤65s of retries before abort), but worth confirming against historical push failures we'd want to fail fast on.
