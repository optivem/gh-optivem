# 2026-08-12 10:24:00 CEST — Track the latest actionlint without unpinning the merge gate

> **⏸ PENDING — TO BE DISCUSSED. Do not execute.**
> This plan is a proposal, not an approved work item. The `## Open questions` section
> below has four unresolved decisions, each with a recommendation. Discuss and resolve
> them first; only then does `## Steps` become executable.

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

## Open questions

Each has a recommendation; none are settled.

1. **Canary trigger — scheduled, or `continue-on-error` on every PR?**
   *Recommended: scheduled weekly + `workflow_dispatch`.* A scheduled run is non-blocking by construction and produces no per-PR noise; `continue-on-error` on PRs adds a green-but-actually-failed step that people learn to ignore. Existing precedent for the shape is `gh-cleanup-prereleases.yml:4-6`.

2. **What the canary lints — this repo's workflows only, or also scaffolded output?**
   *Recommended: this repo's own workflows only.* Linting scaffolded output would require a full `gh optivem init` run per canary execution, which is the expensive part of the acceptance pipeline; the marginal signal does not justify it. Scaffolded workflows are largely copied from `shop`, so drift there is `shop`'s canary to own.

3. **How a red canary is surfaced — failed job only, or auto-filed issue?**
   *Recommended: failed job only, to start.* GitHub notifies on scheduled-workflow failure and the Actions tab shows it. Add issue-filing later only if a red canary is demonstrably missed — building the notification machinery before knowing it is needed is speculative.

4. **Canary now, or set up Renovate instead?**
   *Recommended: canary now.* It is one file and answers the question this week, whereas Renovate is an app install plus config plus a regex manager for a non-standard `go install` line. If Renovate later lands for other reasons, the canary becomes redundant and should be deleted — not kept alongside.

## Steps

*(Blocked on the Open questions above. Written against the recommended answers.)*

- [ ] **Step 1 — Add the canary workflow.**
  New file: `.github/workflows/gh-actionlint-canary.yml`.
  `on: schedule` (weekly) + `workflow_dispatch:`, mirroring the trigger shape of `gh-cleanup-prereleases.yml:4-6`. Checkout, `actions/setup-go@v6` with `go-version-file: go.mod` and `cache: false` (matching `gh-commit-stage.yml:22-26`), then `go install github.com/rhysd/actionlint/cmd/actionlint@latest`, `actionlint --version`, and bare `actionlint`.
  Header comment must state: this workflow is non-blocking and deliberately unpinned; it exists so a new actionlint release is discovered here rather than at the merge gate; when it goes red, review the finding and bump the SHA at the three pinned sites.

- [ ] **Step 2 — Cross-reference the canary from the three pinned sites.**
  Files: `.github/workflows/gh-commit-stage.yml:36-38`, `.github/actions/acceptance-test/action.yml:98-100`, `.github/workflows/_gh-acceptance-pipeline.yml:159-161`.
  Add one line to each existing comment naming `gh-actionlint-canary.yml` as where latest-tracking lives, so a reader who wonders "are we stuck on 1.7.12 forever?" finds the answer at the pin.

- [ ] **Step 3 — Scope a Sonar ignore to the canary file.** *(Conditional — only if S8545 actually fires on the canary; see the assumption above.)*
  File: `.github/workflows/gh-commit-stage.yml:78-84`.
  Add to the inline `-D` flags:
  ```
  -Dsonar.issue.ignore.multicriteria=e1
  -Dsonar.issue.ignore.multicriteria.e1.ruleKey=githubactions:S8545
  -Dsonar.issue.ignore.multicriteria.e1.resourceKey=.github/workflows/gh-actionlint-canary.yml
  ```
  Scoped to the single file by `resourceKey` — the rule must keep firing on every other workflow. Add a YAML comment stating why this one file is exempt.
  If a `sonar-project.properties` is ever introduced, this belongs there instead; note that in the comment.

- [ ] **Step 4 — Fix the over-claiming comment at the pipeline site.**
  File: `.github/workflows/_gh-acceptance-pipeline.yml:159`.
  The line reads `# Pinned to match shop's lint-workflows.yml — see acceptance-test/action.yml`, invoking the lockstep rationale. That rationale does not apply here: this site never invokes the linter (confirmed — lines 158-161 are the only `actionlint` mentions in the file). Reword to say the binary is installed only to satisfy `environment verify`'s PATH check and is never run, with the SHA kept identical to the two real lint sites so the pin set stays greppable.

- [ ] **Step 5 — Lint and confirm blast radius.**
  Run `actionlint` at the repo root; it must pass. `git diff --stat` must show only the four files above plus the one new workflow — no Go files, no scaffolding templates.

## Verification

- After merge to `main`, confirm the Sonar scan on that push reports no new `githubactions:S8545` issue for `gh-actionlint-canary.yml` (Step 3's ignore working), and still reports the rule as active elsewhere. This cannot be checked pre-merge — Sonar is gated to `main` pushes only.
- Trigger the canary manually via `workflow_dispatch` once and confirm it installs a version *newer* than 1.7.12 and reports a real lint result (pass or fail) rather than erroring on setup.
- Confirm a failing canary does not affect any required check on an open PR.
