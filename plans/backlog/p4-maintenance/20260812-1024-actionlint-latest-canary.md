# 2026-08-12 10:24:00 CEST — Track the latest actionlint without unpinning the merge gate

## TL;DR

**Why:** The three `actionlint` install sites are pinned to commit `914e7df` (= tag v1.7.12) and nothing in the repo ever tells us when a newer actionlint would flag our workflows. The pin is correct and should stay — it protects the merge gate — but it also means new upstream rules are invisible until someone bumps the SHA by hand, which has not happened since the pin landed on 2026-07-01.
**End result:** A separate, non-blocking canary workflow lints this repo's workflows with `actionlint@latest` on a schedule. When upstream adds a rule that fires on our files, the canary goes red, we read the finding, and we bump the three pinned sites deliberately. The merge gate stays deterministic — no upstream release can turn `main` red on its own.

## Outcomes

What we get out of this — the goals and deliverables:

- We learn about new actionlint findings on our own schedule instead of discovering them the next time somebody bumps the pin.
- `gh-commit-stage` and the acceptance pipeline keep their reproducible, SHA-pinned lint — a new actionlint release still cannot fail a PR that would otherwise pass.
- The "is our workflow OK against latest?" question gets a standing answer in the Actions tab rather than needing an ad-hoc local run.
- Exactly one file in the repo carries a mutable tool reference, and it is the one file whose entire purpose is to track a mutable reference. Everything else stays pinned.
- `githubactions:S8545` keeps protecting every other workflow — the suppression is scoped to the canary file, not the rule.

## Context

### The three pinned sites and what each is for

| Site | Runs actionlint? | Purpose |
|---|---|---|
| `.github/workflows/gh-commit-stage.yml:38` (lint at `:56`) | Yes | Lints this repo's own workflows. Runs on push + PR to `main`. |
| `.github/actions/acceptance-test/action.yml:100` | Yes | `gh optivem init` shells out to it over the *scaffolded* workflows. |
| `.github/workflows/_gh-acceptance-pipeline.yml:161` | **No** | Only satisfies `verifyActionlint`'s PATH check so `go run . environment verify` (`:178`) can run as the token gate. |

All three carry the same SHA so the set is greppable as one unit.

### Why the pin exists (both reasons are real)

1. **Lockstep, learned the hard way.** Per the comment at `acceptance-test/action.yml:95-99`, a floating `@v1` once let the install sites drift apart — shop went green while scaffold verify went red on the same files, because the `if-cond` rule landed post-1.7.7. A linter is not a library: a patch release adds *rules*, so "pick up patches automatically" means "the CI verdict changes with no commit of ours."
2. **SonarCloud `githubactions:S8545`.** Commit `34e35c0a` (2026-07-01, "resolve 19 SonarCloud issues") replaced `@v1.7.12` with the commit SHA at all three sites; its message lists `githubactions:S8545 x3`. The rule objects to *mutable* references, so `@v1` would fail it harder than the patch tag that originally tripped it.

Note that reason 1 does not apply to `_gh-acceptance-pipeline.yml:161`, which never invokes the linter. Reason 2 applies to all three.

### How Sonar is configured here

There is no `sonar-project.properties`. All scanner configuration is inline `-D` flags at `gh-commit-stage.yml:78-84`, and the scan step is gated `if: github.ref == 'refs/heads/main' && env.SONAR_TOKEN != ''` (`:69`) — so **Sonar only runs on pushes to `main`, never on PRs.** `-Dsonar.sources=.` means `.github/` is in scope.

Two consequences for this plan:
- A Sonar-ignore change cannot be validated on a branch. It is only observable after merge to `main`.
- The canary workflow itself will be scanned once merged.

### Assumption that needs confirming, not asserting

S8545 demonstrably fired on `@v1.7.12`, a pinned patch tag. It is a near-certain inference that a floating `@latest` also fires — but this has **not** been observed. If it turns out not to fire, Step 3 is unnecessary and should be dropped rather than added speculatively.

### Considered and not chosen

- **Unpin the three sites to `@v1` or `@latest`.** Directly re-opens the S8545 issues closed by `34e35c0a`, and re-introduces the drift failure the lockstep comment documents. It fuses two different jobs — "tell me about new rules" and "let new rules block my merges" — that this plan deliberately separates.
- **Disable `githubactions:S8545` in the SonarCloud Quality Profile.** Drops supply-chain protection across every workflow to solve a one-line problem.
- **Resolve the latest tag into a variable at runtime** (`LATEST=$(gh release view …); go install …@"$LATEST"`) so no literal mutable ref appears. This would likely dodge S8545 without a suppression, but it evades the rule instead of stating intent. An explicit, commented, file-scoped ignore is honest about what is happening and why.
- **Renovate with a `customManagers` regex rule** to auto-PR the SHA bump. Genuinely attractive: the PR carries a *new SHA* rather than a mutable ref, so it needs no Sonar suppression at all, and the existing CI on that PR answers "does latest pass?" with full signal. Not chosen for now only because neither Renovate nor Dependabot is configured on this repo (no `renovate.json`, no `.github/dependabot.yml`), making it an app-installation and config task rather than a single self-contained workflow file. **Worth revisiting** — see Open question 4.

## Resolved decisions

1. **Canary trigger.** Scheduled weekly + `workflow_dispatch`. A scheduled run is non-blocking by construction and produces no per-PR noise; `continue-on-error` on PRs would add a green-but-actually-failed step people learn to ignore. Existing precedent for the shape is `gh-cleanup-prereleases.yml:4-6`.

2. **What the canary lints.** This repo's own workflows only. Linting scaffolded output would require a full `gh optivem init` run per canary execution — the expensive part of the acceptance pipeline — and scaffolded workflows are largely copied from `shop`, so drift there is `shop`'s canary to own.

3. **How a red canary is surfaced.** Failed job only, to start. GitHub notifies on scheduled-workflow failure and the Actions tab shows it. Issue-filing can be added later only if a red canary is demonstrably missed.

4. **Canary now vs. Renovate.** Canary now. It is one file and answers the question this week, whereas Renovate is an app install plus config plus a regex manager for a non-standard `go install` line. If Renovate later lands for other reasons, the canary becomes redundant and should be deleted — not kept alongside.

## Steps

All agent steps shipped (canary workflow, cross-references at the three pinned sites, reworded `_gh-acceptance-pipeline.yml` comment; `actionlint` clean, no Go/scaffolding files touched).

Step 3 (scoped Sonar ignore) was **not** applied: S8545 firing on `@latest` is unobserved, and the plan says drop rather than add speculatively. It moves to Verification below as a conditional follow-up.

## Verification

- After merge to `main`, check whether the Sonar scan reports `githubactions:S8545` on `gh-actionlint-canary.yml`. If it does, add a file-scoped ignore to the inline `-D` flags at `gh-commit-stage.yml` (with a YAML comment explaining the one-file exemption; move to `sonar-project.properties` if one is ever introduced):
  ```
  -Dsonar.issue.ignore.multicriteria=e1
  -Dsonar.issue.ignore.multicriteria.e1.ruleKey=githubactions:S8545
  -Dsonar.issue.ignore.multicriteria.e1.resourceKey=.github/workflows/gh-actionlint-canary.yml
  ```
  and confirm the rule stays active on every other workflow. If it does not fire, nothing to do.
- Trigger the canary manually via `workflow_dispatch` once and confirm it installs a version *newer* than 1.7.12 and reports a real lint result (pass or fail) rather than erroring on setup.
- Confirm a failing canary does not affect any required check on an open PR.
