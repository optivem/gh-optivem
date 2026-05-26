# Retry post-create `git push` on GitHub ref-store replica lag

## Origin / intent

Acceptance run **26456900412**, job **77901798586** (matrix leg
`Run (monolith, multirepo, java, typescript)`) failed at
`internal/steps/finalize.go:173` (`commitAndPushRepo`) when pushing the
freshly-created sibling system repo:

```
remote: cannot lock ref 'refs/heads/main': reference already exists
 ! [remote rejected] main -> main
error: failed to push some refs to '.../test-app-...-system.git'
FATAL: git push failed in valentinajemuovic/test-app-...-system:
       command failed: git push -u origin main: exit status 1
```

The root repo's identical push at the same call site succeeded in the
same run — the asymmetry is a textbook signature of probabilistic
GitHub git-ref-store replica lag on a newly-created repository, not a
logic bug in this codebase.

The current `git push -u origin main` at `finalize.go:173` has no
retry. The accompanying comment at `finalize.go:166-172` argues retry
is unnecessary because:

> by Phase 5 the repo has been touched by clone + several gh api
> calls, so every replica has caught up.

Run 26456900412 falsifies that assumption. The push site exhibits the
same replica-lag class that `shell.MustRunPostCreate` already
compensates for on the *read* side (`waitForRepoVisible` + the
permissive post-create classifier in `internal/shell/retry.go:102-131`).

This plan adds the same compensating retry on the *write* side, as a
first-class, named, tested architectural element — not an inline ad-hoc
retry.

## Why this is the long-term fix (not a workaround)

Considered alternatives, rejected:

1. **REST `Git Data` API for the initial commit (no `git push` of a
   ref-creation).** Forks the scaffold flow into two code paths (REST
   for the first commit, git for everything else). The subsequent clone
   still hits a possibly-lagged replica, so the race resurfaces one
   step later. Net cost: complexity; net benefit: zero.

2. **`gh repo create --add-readme` so `main` exists at creation.**
   Reintroduces `initRepo()` which commit `b0d5c7c` (Remove initRepo to
   eliminate new-repo push race) deleted precisely because it produced
   a different variant of the same race ("is at X but expected Y"
   rejections, run 25207926785). Net wash.

3. **Probe replica consistency before pushing (e.g. poll
   `git ls-remote` until stable).** Not implementable reliably:
   GitHub's load balancer routes each call to a different replica;
   there is no exposed "all replicas agree" probe. Polling is guessing,
   not verifying.

4. **Narrowly-classified retry at the post-create push site (this
   plan).** Canonical client-side compensating action for an
   eventually-consistent platform. Same architectural shape as the
   existing `MustRunPostCreate` for the read side. Bounded budget,
   pinned classifier — failures outside the lag class still fail fast.

The retry is the long-term fix because the root cause (GitHub ref-store
replica lag on freshly-created repos) is a platform property we cannot
fix upstream. The job of our code is to compensate deterministically,
which is what option 4 does.

## What makes it durable (not a temp fix in spirit)

These four properties are non-negotiable — without them the retry
*would* be a band-aid:

1. **Named helper**, not inline retry — so the pattern is discoverable
   and reusable. Sibling concept to `MustRunPostCreate`.
2. **Pinned classifier with unit test** — the two server messages
   (`cannot lock ref` and `reference already exists`) are recorded as
   test fixtures. If GitHub changes the wording, the test fails and we
   notice instead of silently bypassing.
3. **Bounded retry budget** — reuses the existing 4-attempt /
   5s→15s→45s schedule from `defaultRetryAttempts` /
   `defaultRetryDelays`. A genuinely-broken repo still aborts after
   ~65s instead of looping forever.
4. **Comment at `finalize.go:166-172` rewritten** to record what run
   26456900412 taught us. Otherwise the next person reading it will
   repeat the "no retry needed at push time" reasoning that produced
   this regression.

## Scope boundaries

- Only the **post-create** push at `commitAndPushRepo` is changed.
  Other `git push` sites (none currently in `internal/steps/`, but
  future ones) keep plain `shell.Run` semantics.
- The retry is gated on `!preExisted` (the `preExisted` flag already
  passed into `commitAndPushRepo`). Reasoning: on a pre-existing repo,
  the push is fast-forward against a settled `main`; replica-lag of
  the ref-creation class cannot apply. Pre-existing repos keep
  fail-fast semantics so a genuine non-fast-forward / permission /
  branch-protection failure surfaces immediately.
- The classifier matches **only** the two documented replica-lag
  messages. The existing `RetryHardFail` regex (at
  `internal/shell/retry.go:36-44`) lists
  `! \[remote rejected\]` as a hard-fail token — that classification
  is correct for plain `MustRunWithRetry` callers and is **not
  changed** by this plan. The new helper deliberately overrides it for
  the narrow post-create window.

## Items

### Item 4 — Acceptance smoke trace (manual verify)

Re-trigger the acceptance workflow (`workflow_dispatch` on
`gh-acceptance-stage`) and confirm the previously-failing
`Run (monolith, multirepo, java, typescript)` matrix leg now passes.

If the same matrix leg still fails with a *different* error class
(non-fast-forward, permission), that is the intended outcome of the
narrow classifier: investigate as a separate bug. The classifier
deliberately does not paper over those.

## Files touched

- `internal/shell/retry.go` — add `MustRunPostCreatePush` (Item 1).
- `internal/shell/retry_test.go` — add classifier test (Item 3).
- `internal/steps/finalize.go` — wire helper at the push site +
  rewrite comment block (Item 2).
