# Trunk-Based Development

This repo and the projects it scaffolds use **trunk-based development (TBD)**: everyone integrates small changes into `main` frequently, history stays linear, and long-lived branches don't exist.

This doc covers two ways to practice TBD — committing straight to `main`, or merging via short-lived PRs — and the discipline that makes both work.

## Core rules (both approaches)

1. **Never force-push `main`.** Ever. Rewriting published trunk history breaks every other committer and every bot.
2. **`pull --rebase` is the default.** Linear trunk requires that pulls rebase your local commits, never create merge commits.
3. **Keep changes small.** Hours, not days. Small commits rebase trivially; large commits never do.
4. **Linear trunk history.** No "Merge branch 'main' of …" commits on `main`.
5. **Only rebase commits that haven't been pushed.** Rebasing local-only commits is safe. Rebasing pushed commits requires a force-push, which is forbidden on `main`.

## One-time git setup

Run once per machine:

```bash
git config --global pull.rebase true        # pull always rebases, never merges
git config --global rebase.autoStash true   # auto-stash dirty files during rebase
git config --global rerere.enabled true     # remember conflict resolutions
```

After this, `git pull` does the right thing for TBD by default.

## TBD — Commit straight to `main` (no PRs)

**When it fits**

- You have no pre-merge code review gate, or you have one supplied by something *other than* a PR — e.g., pair/mob programming where two people already saw the change before it landed, or an external review tool (Gerrit, Phabricator-style) that gates commits without using git branches.
- You have no pre-merge CI gate either, or you accept that broken commits will occasionally land on `main` and be reverted quickly.
- Learning repos, prototypes, and trusted small teams typically fall here because pre-merge review and CI aren't worth the overhead yet.

**Workflow**

```bash
git pull                  # rebase by default
# edit, run tests locally
git add -p
git commit -m "..."
git pull                  # rebase again — someone (or a bot) may have raced you
git push                  # if rejected, pull + push again
```

If the push is rejected because something landed on `origin/main` in the meantime, run `git pull` (which rebases) and retry `git push`. Repeat until it lands.

**Tradeoffs**

- Fastest workflow, zero PR overhead.
- Forces small commits — you feel every commit immediately.
- No CI gate before main; broken commits land directly on the trunk (unless you've added a pre-commit gate via other tooling).
- No second pair of eyes — unless review is happening another way (pairing, external tool).

## Scaled TBD — Short-lived PRs

**When it fits**

- You need a pre-merge code review gate and/or a pre-merge CI gate, and the PR is the natural mechanism to deliver them on GitHub.
- Most real teams doing TBD on GitHub end up here, because GitHub's review and required-status-check tooling is fundamentally branch-and-PR-shaped.
- Branches live **hours**, not days. If a PR is older than ~24h, you've drifted from TBD.

**Workflow**

```bash
git checkout main
git pull
git checkout -b my-work
# edit, commit (often multiple small commits)
git push -u origin my-work
# open PR, CI runs, get review (or self-merge if policy allows)
```

If `main` moved while the PR was open:

```bash
git fetch origin
git rebase origin/main
git push --force-with-lease   # safe on YOUR branch — refuses if someone else pushed to it
# then merge the PR
```

When merging the PR, use **Squash** or **Rebase and merge** — never "Create a merge commit." Merge commits break linear trunk history.

**What each command does**

Starting the branch:

- `git checkout main` + `git pull` — switch to `main` and fetch+rebase the latest commits. This makes sure your branch starts from current trunk, not yesterday's. Skipping this means extra rebase work later for no reason.
- `git checkout -b my-work` — create branch `my-work` and switch to it in one step. The branch starts at whatever commit `main` is at now. It exists only locally until you push.
- `git push -u origin my-work` — push the branch to the remote for the first time. `-u` (short for `--set-upstream`) tells git that from now on, `git push` and `git pull` on this branch default to `origin/my-work`. Without `-u` you'd have to spell out `git push origin my-work` every time.

Reacting to `main` moving:

- `git fetch origin` — download the remote's latest state without changing any of your working files. `origin/main` in your local repo now reflects what's actually on the server. `fetch` is the "look, don't touch" command; always safe.
- `git rebase origin/main` — replay your branch's commits on top of the new `origin/main`. Mechanically: find the common ancestor of `my-work` and `origin/main`, set aside every commit on `my-work` since that ancestor, move the base of `my-work` to the tip of `origin/main`, and replay your commits on top one at a time. Your commits get new hashes (their parent changed) but their content stays the same. If any commit conflicts, the rebase pauses for you to resolve — edit, `git add`, `git rebase --continue`.
- `git push --force-with-lease` — your branch on the remote still has the **old** commit hashes from before the rebase; your local has **new** hashes. A normal `git push` is rejected because git refuses to overwrite remote commits by default. So you need a force-push.

**Plain `--force` vs `--force-with-lease`**

Plain `--force` overwrites whatever is on the remote, even if someone else pushed to your branch in the meantime — their commits silently disappear. `--force-with-lease` is the safer version: *force-push, but only if the remote tip is still where I last saw it.* The "lease" is git's record of what `origin/my-work` was when you last fetched.

- No one else touched your branch → lease matches → force-push succeeds.
- Someone else pushed to your branch → lease doesn't match → push is rejected. You fetch, look at their work, and decide what to do (usually rebase on top of theirs too).

This is why force-pushing a **feature branch** is safe and force-pushing **`main`** is not. A feature branch has at most a few collaborators and no bots; `--force-with-lease` catches the rare overwrite case. `main` has every committer, every bot, every CI run watching it — rewriting it strands everyone downstream.

**Why the dance exists**

Two facts collide:

1. You want pre-merge CI to test what will actually be on `main` after merge.
2. `main` changed since you opened the PR.

You can satisfy (1) by either merging `main` into your branch (adds a merge commit, breaks linear history, easy and lossy) or rebasing your branch onto `main` (rewrites your branch's commits with new parents, requires force-push, keeps linear history). TBD picks rebase. The force-push is the price; `--force-with-lease` is the safety net.

**Tradeoffs**

- CI gate before main; broken commits never land.
- Review (even self-review) catches issues.
- Force-push is safe because you only ever force-push your own branch, never `main`.
- Scales to teams.
- More steps; PR overhead per change.
- Easy to slip into multi-day PRs and lose the TBD discipline.

## Side-by-side

| Concern | TBD | Scaled TBD |
|---|---|---|
| Where you commit | `main` directly | Short-lived branch, then merge to `main` |
| `pull --rebase` default | Yes, on `main` | Yes, on `main` and your branch |
| Force-push trunk | **Never** | **Never** |
| Force-push branch | N/A | Yes, with `--force-with-lease` |
| Pre-merge CI gate | Only if supplied by other tooling | Yes, via PR status checks |
| Pre-merge review gate | Only if supplied by pairing or an external tool | Yes, via PR review |
| Merge style | N/A | Squash or rebase-merge (never "create merge commit") |

## The real axis is the review gate, not team size

It is tempting to say "no-PR TBD is for small teams, PR-based TBD is for big teams." That framing is wrong. The right axis is **whether you have a pre-merge code review gate, and how it's delivered.**

- **No review gate at all** — pure commit-straight-to-trunk with nothing in front of it. This works for solo work, prototypes, and very small trusted teams. It doesn't scale, not because of team size per se, but because at any meaningful size you eventually want *somebody else's eyes* on changes before they land.
- **Review gate delivered by a PR** — Scaled TBD in this doc. The PR is GitHub's first-class mechanism for pre-merge review and required status checks. This is what most teams on GitHub end up using because GitHub's tooling is shaped that way.
- **Review gate delivered some other way** — pair/mob programming where the pair is the review, or an external review tool like Gerrit or Phabricator's Differential that gates commits without using git branches. Google's Piper + Critique and Facebook/Meta's Phabricator origins are the canonical large-scale examples: thousands of engineers, single trunk, no git branches, but very much a pre-commit review gate. They do TBD without PRs — but **not** without review.

So the real question is not "how big is the team" but "where does my review gate come from?" If the answer is "GitHub PR," use Scaled TBD. If the answer is "the pair, or a non-git review tool," plain TBD works at any size. If the answer is "I don't have one yet," plain TBD is fine for now — just understand that's the constraint, not the team headcount.

The same logic applies to the pre-merge CI gate. PRs deliver it via required status checks; in a no-PR world you need another mechanism (a pre-receive hook, a merge queue, a commit-time CI bot) to get the equivalent guarantee.

## The version-bump bot

This repo (and every repo scaffolded by `gh optivem init`) runs [`gh-bump-patch-version.yml`](../.github/workflows/gh-bump-patch-version.yml), which commits a `VERSION` bump directly to `main` after each release via the GitHub Contents API.

That means **`main` has more than one committer**, even in a solo repo: you and the bot.

**Nothing about this is bot-specific.** It is exactly the same situation you'd be in if a human teammate pushed to `main` while you were working on it. The bot is just another committer — a predictable, mechanical one, but otherwise indistinguishable from a colleague who happened to land a commit between your `git fetch` and your `git push`. Everything in this doc about races, rebasing, and pulling applies identically whether the other committer is a bot, a teammate, or your future self on a different machine.

The bot makes the multi-committer reality visible in what would otherwise feel like a solo repo. That's useful: it forces you to internalize the TBD `pull --rebase` discipline now, on a low-stakes race, instead of discovering it the hard way when you join a team. The bot's race window is small — only right after a release — but it's real, and the resolution is identical to the teammate case.

**With plain TBD**, you'll occasionally see `git push` rejected because the bot landed a `Bump VERSION ...` commit. Resolution:

```bash
git pull        # rebases your local commits on top of the bot's bump
git push
```

**With Scaled TBD**, the bot doesn't affect you while the PR is open (you're on your branch). Before merging, rebase your branch on the latest `main` (which now includes any bot commits), force-push-with-lease, then merge.

The bot's commits stay in trunk history as their own commits. That's deliberate — releases are real events worth recording.

## Conflict resolution during rebase

If `git pull` (or `git rebase`) hits a conflict:

```bash
# See conflicted files
git status

# Edit each file; resolve the <<<<<<< / ======= / >>>>>>> markers; save
git add <file>

# Continue the rebase
git rebase --continue

# Or bail out entirely and try a different approach
git rebase --abort
```

If a rebase leaves you in a weird state, **stop and read `git status`**. Don't run more commands to "fix" it blindly. `git rebase --abort` always gets you back to where you started.

## Beyond Scaled TBD — tooling for very large engineering orgs

Plain TBD and Scaled TBD cover almost every project this repo will produce. Once an organization grows to hundreds or thousands of committers on one trunk, the same philosophy holds but additional tooling is needed on top:

- **Merge queues** (GitHub Merge Queue, Bors, Aviator) serialize merges. They solve two problems naive `pull --rebase` + `push` cannot solve at very high contention:

  1. *You lose every race.* `pull --rebase` doesn't lose commits — your work stays on your local branch the whole time — but with many committers, the loop "fetch → rebase → push → rejected → repeat" can fail to converge. Someone else always lands between your fetch and your push.
  2. *Stale-base CI.* You rebased onto `origin/main` at time T1, CI went green at T2, but by T2 `origin/main` has moved on. Your code lands on a `main` that CI never tested. Result: green PR, broken trunk.

  Merge queues fix both by serializing merges and re-running CI against the actual merge order, only landing if CI passes against the final base.
- **Feature flags** become non-optional — incomplete work merges to trunk behind a flag, then gets flipped later.
- **Pre-merge CI gates** are mandatory.
- **Revert-first culture** — if your commit broke main, someone reverts immediately; you re-land with the fix.
- **Affected-test / build-graph tooling** (Bazel, Nx, Turborepo) bounds CI cost.

The discipline you build with plain TBD or Scaled TBD (small commits, `pull --rebase`, never force-push trunk, linear history) is the foundation that carries over. Nothing in this list contradicts what's above — it adds layers on top.

## Merge vs rebase — and why TBD picks rebase

The merge-vs-rebase debate is often framed as a matter of taste. It isn't — at least not when you've already picked a branching model. **The choice between merge and rebase is downstream of the choice between trunk-based and feature-branching.** Pick a branching model first, and the integration strategy follows.

### What each does, mechanically

**Merge:**

```
A---B---C---M  (main, after merge)
     \     /
      D---E    (your branch)
```

Creates a merge commit (`M`) joining the two histories. Preserves the original commits (`D`, `E`) exactly — same hashes, same parents. Nothing is rewritten, nothing needs force-push. Conflicts resolved once, at the merge point. The merge commit itself records "this is when these two streams joined."

**Rebase:**

```
A---B---C---D'---E'  (your branch, after rebase)
```

Replays your commits (`D`, `E`) on top of the new base. They get **new** hashes (`D'`, `E'`) because their parents changed. No merge commit. History is linear. Destructive in the sense that the old commits are gone — if they were already pushed, you need a force-push to update the remote. Conflicts resolved per commit during the replay.

### Side-by-side

| Concern | Merge | Rebase |
|---|---|---|
| History shape | Branching DAG with merge commits | Linear |
| Commit hashes | Preserved | Rewritten |
| Force-push needed? | No | Yes (if commits were pushed) |
| Conflict resolution | Once, at merge | Per commit, during replay |
| Safe on pushed commits? | Yes | No |
| Preserves "integration point" fact? | Yes | No |
| Reads like a story | No (parallel streams) | Yes (sequential) |
| `git bisect` clarity | Worse | Better — every commit is a real point in time |

### Trunk-based vs feature-branching — and what each demands

**Feature-branching (GitFlow and friends) is merge-shaped.** Long-lived branches exist as a feature: `develop`, `release/*`, `hotfix/*`, and feature branches that may live for days or weeks. The whole *point* of these branches is to preserve their identity until they're integrated. Rebasing a long-lived shared branch is hostile to collaborators — it rewrites commits they've built on top of. So feature-branching models lean on merge commits: they record when each branch was integrated, they're safe with multiple authors per branch, and they don't require force-pushes. The cost is a non-linear history full of merge commits, which is *what feature-branching wants*.

**Trunk-based development is rebase-shaped.** TBD's promise is a single trunk with linear history and no long-lived branches. Merge commits directly contradict that promise — every merge commit on `main` is a divergence point that breaks the "one stream of small commits" model. So TBD leans on rebase: your local commits get replayed on top of new trunk before each push, your short-lived feature branch gets rebased onto `main` before merging, and the PR itself is squash- or rebase-merged so the merge to trunk is linear. The cost is force-pushes (on your own short-lived branch only, never on `main`), which is *what TBD accepts in exchange for a clean trunk*.

So when someone asks "should I merge or rebase," the honest answer is: which branching model are you running? **Feature-branching → merge. Trunk-based → rebase.** Mixing them ("we do TBD but with merge commits on main") gives you the worst of both: TBD's small-batch discipline without TBD's history benefits.

### Why does TBD want a linear trunk in the first place?

The previous section says "TBD wants linear history, merge breaks linear history, therefore TBD uses rebase." That's a syllogism, not an argument. Here's what linear trunk history actually buys you, and why each benefit matters more under TBD than under feature-branching:

- **`git bisect` works cleanly.** Bisect's job is to binary-search history to find the commit that introduced a regression. On a linear trunk every commit is a real, testable point in time — check it out, run the suite, decide good or bad. On a history full of merge commits bisect has to either descend into merges (slow, confusing) or skip them (lossy). Under TBD you're shipping continuously, so bisect is a frequent tool, not a once-a-quarter recovery move.

- **`git log` is a real record, not noise.** `git log --oneline` on a TBD trunk reads as a literal sequence of what changed. On a merge-heavy trunk roughly half the commits are "Merge branch 'main' of github.com:foo/bar" — a string that carries zero information about what changed. TBD relies on the log being the canonical record of integration; if half the entries are noise, the log stops being useful.

- **`git revert` is precise.** On a linear trunk, `git revert <sha>` undoes exactly one focused change. Reverting a merge commit is awkward (you have to specify a parent with `-m`) and ambiguous (are you undoing the merge, the underlying changes, or both?). TBD's promise of fast revert culture — "broke main, get it reverted, re-land with the fix" — depends on revert being trivial. Merge commits make it not trivial.

- **The mental model matches the engineering reality.** TBD's claim is *"we are all working on one stream and integrating constantly."* A linear history shows exactly that — one stream, one commit at a time. A branching DAG with merge commits visually suggests parallel streams of work that joined later. That's the feature-branching model; depicting TBD that way actively misleads readers of the history.

- **CI signal is unambiguous.** Every commit on a linear trunk is a real point where CI could (and ideally did) run. Under TBD this lets you say "main is green at commit X" with a clear meaning. With merge commits, the CI run on a merge commit and the CI runs on its parents can disagree, and reasoning about "is main green" gets murky.

- **Squash-merging hides nothing valuable.** A common objection to squashing is "but we lose the granular history of the feature." Under TBD the granular history is *minutes of activity*, not weeks. There's nothing of long-term value to preserve. Under feature-branching that objection has teeth; under TBD it doesn't.

- **Force-push prohibition on `main` is enforceable.** Because the only rebases that happen in TBD are on local-only commits or your own short-lived branch, the *only* force-pushes are on your branch — never on `main`. Merge-based workflows can still drift into needing force-pushes on shared lines, which is the actually-dangerous case.

Pick rebase because each of these benefits is something TBD specifically depends on. Under feature-branching, where bisect runs once a quarter and the log is mostly read by hand during code archaeology, the calculus is different — and that's why feature-branching teams happily accept merge commits.

### Where merge still appears inside TBD

TBD is rebase-first, not merge-banned. Two places where merge remains correct under TBD:

- **The final PR merge into `main`** — but only via *squash-merge* or *rebase-merge* in the GitHub UI, which produces a single linear commit on `main`. Not "create a merge commit."
- **Already-pushed commits on a shared branch.** If you and a teammate are both committing to a single short-lived branch, rebasing it under each other will cause pain. Either keep branches single-author (the TBD-preferred answer) or accept merge commits on that branch and squash them out at PR merge time.

### Quick test for any situation

Ask: *will I have to force-push to make this work?*

- **No, I won't need to force-push** → either you're doing a true merge, or you're rebasing local-only commits. Both safe.
- **Yes, and the branch is mine alone** → force-push-with-lease. Standard TBD.
- **Yes, and the branch is shared with others** → don't rebase. Either merge, or stop sharing the branch.
- **Yes, and the branch is `main`** → stop. There is no situation in TBD where this is the right answer.

## The rule that doesn't change

In every flavor of TBD, in every team size:

1. **Never force-push `main`.**
2. **Commits/branches stay small and short-lived.** Hours, not days.
3. **`pull --rebase` is the default** wherever you're pulling.
4. **Linear trunk history** — no merge commits on `main`.
