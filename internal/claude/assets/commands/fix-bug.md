Take a failure — an error log, a stack trace, or a link to a failing GitHub Actions run — root-cause it, propose a fix, then hand off to `/create-plan` to draft the remediation plan. No code changes: this command produces a *diagnosis* and a *plan*, not the fix itself. Use `/execute-plan` afterward to implement.

## When to use which command

- `/fix-bug` (this one) — you have a **failure to diagnose**: a CI/workflow run that failed, a pasted error log, or a stack trace. It finds the root cause (pinned to `file:line`), proposes a fix, and hands off to `/create-plan`. General-purpose — any repo, any language.
- `/atdd-postmortem` — the same shape but scoped to **ATDD rehearsal / orchestration halts** (`TESTS_INFRA_HALT`, scope-diff halts, wrong-artifact agents). Reach for it when the failure is a `gh optivem` rehearsal-loop run, not a plain CI/build failure.
- `/create-plan` — `/fix-bug` calls this for you; invoke it directly only when you already know the fix and just want a plan.
- `/execute-plan` — implements the plan `/fix-bug` produced.

If the failure is an ATDD rehearsal halt, say so and point at `/atdd-postmortem` instead.

## Input

The failure is provided as `$ARGUMENTS`. Accepted shapes, in order of preference:

- **A failing-run link or id** (preferred): a GitHub Actions run URL (`https://github.com/<owner>/<repo>/actions/runs/<id>`), a bare run id, or a job URL. The command fetches the logs itself with `gh`.
- **A path to a log file**: read it directly.
- **Pasted error text**: a stack trace, build error, or test-failure block copied from the terminal or CI. Parse it and follow any `file:line` references back to the source.

If `$ARGUMENTS` is empty, ask the user for a link, a path, or pasted failure text in one line, then proceed.

## Phase 0 — Gather the evidence

If given a **GitHub Actions link or run id**, fetch the failure with `gh` (never `git`, per CLI conventions). Extract `<owner>/<repo>` from the URL if present; otherwise default to the current repo.

```bash
# Failed steps only — the cheapest useful view.
gh run view <run-id> --repo <owner>/<repo> --log-failed
# If that's empty or you need the surrounding context, get the job summary first:
gh run view <run-id> --repo <owner>/<repo>
```

Never swallow stderr from `gh` (per conventions) — if the fetch fails (auth, run not found, rate limit), surface the real error and stop; do not guess at the cause from the URL alone.

If given a **path**, read the file. If given **pasted text**, parse it as-is. In all cases, isolate the *first* genuine error — the earliest failing assertion, the compile error, the unhandled exception — not the cascade of downstream noise it triggered.

## Phase 1 — Root-cause (pin to file:line)

1. From the evidence, identify the precise failure: the failing command, the error type/message, the test or build target, and the `stderr`/stack tail.
2. **Read the actual source the failure points at** in the repo — the failing test, the implementation under it, the config or workflow step. **Pin the cause to a concrete `file:line`.** Never propose a fix from the log alone (per the repo's fail-loud rule: surface the *true* cause, not a plausible-looking one).
3. **Reproduce locally first** when the repo supports it (per the shop's *Fixing Failing Workflows* convention): run the failing build/test locally with the appropriate flags before concluding anything, and **report whether the failure reproduced**. A failure that won't reproduce locally is itself a finding (flake, environment-specific, or CI-config bug) — say so.
4. **Check all parallel implementations.** In multi-language repos (e.g. shop's .NET / Java / TypeScript), a bug in one language usually has a twin in the others. Check the equivalent code in every language and note which are affected — the plan should fix all of them, not just the one that failed.
5. Classify the defect so the plan targets the right layer: genuine product bug, test-authoring error, CI/workflow misconfiguration, environment/infra issue, or flake. If it's a real flake, prevention may belong in retry/timeout config rather than the code — say so.

## Phase 2 — Propose a fix

State, concisely:

- **Root cause** — one or two sentences, pinned to `file:line`.
- **Proposed fix** — the concrete change(s), each with the file it lives in. If the cause spans languages, list the change per language. If there's more than one reasonable approach, lead with a **recommended** one and say why in a sentence (per the recommend-and-proceed convention); don't bury the user in an options matrix.
- **Scope** — every file/language the fix touches, and any verification the plan should end with (e.g. `compile-all.sh` + `--sample` tests).

## Phase 3 — Draft and commit the plan, then hand back the execute command

Once Phase 2 has shown the user the root cause and proposed fix, **do not pause for confirmation** — go straight to drafting and committing the plan. (The user has opted out of the confirm gate for this command specifically; this overrides the general "ask before committing" default, and *only* for the plan file produced here.)

1. Invoke the **`create-plan`** skill (via the Skill tool) with a synthesized idea string built from the diagnosis. Give it enough grounding that its draft is concrete and needs no re-derivation:
   - the failure (the run/test/command that failed, and the error),
   - the root cause pinned to `file:line`,
   - the proposed change(s), with the file(s) and language(s) each touches,
   - the verification the plan should end with.
2. Commit the plan **without asking** — skip `/create-plan`'s confirm-before-commit gate and finalize the plan file directly.
3. Report back the plan path and the exact command to execute it:

   > Plan committed: `plans/YYYYMMDD-HHMM-<slug>.md`. Run it with:
   > `/execute-plan plans/YYYYMMDD-HHMM-<slug>.md`

`/create-plan` owns the plan file (it writes `plans/YYYYMMDD-HHMM-<slug>.md` in the current repo). `/fix-bug` stops once the plan is committed — report the plan path and let the user execute via the printed command.

## Rules

- **No code changes.** Output is a diagnosis + a plan. If the fix is obvious, capture it as plan steps — don't implement it here. Use `/execute-plan` afterward.
- **Pin the root cause to `file:line` before proposing anything.** Never propose a fix from the log/trace alone.
- **Reproduce locally before concluding** when the repo allows it, and report the result. Inability to reproduce is a finding, not a dead end.
- **Fix all languages.** In multi-language repos, check every parallel implementation and fold the missing ones into the plan.
- **Use `gh`, never `git`, for run logs**, and never swallow its stderr — on auth/not-found/rate-limit failure, surface the error and stop.
- **Recommend, don't enumerate.** When multiple fixes are viable, lead with one recommendation and a one-line why; minimize the choices handed to the user.
- **One failure per invocation.** For several unrelated failures, run the command once each.
