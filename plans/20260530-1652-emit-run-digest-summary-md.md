# Emit a human-readable run digest (`summary.md`) alongside the existing machine sidecar

**Status:** proposed
**Created:** 2026-05-30 16:52 CEDT

## Problem

`gh optivem implement` already emits a rich set of run artifacts, but the only
*human digest* is the `=== Agent summary ===` table, and the only durable
narrative is the long forensic log
(`rehearsal-69-…-160041.log`). An operator who wants the one-screen answer —
*which ticket, did it pass, what ran, what did it cost* — has to either scroll
the long log or mentally stitch the table to the ticket context.

We want a short, GitHub-renderable digest emitted per run that fronts the
existing summary table with ticket context and an overall verdict.

## What already exists (reuse, do not reinvent)

- `internal/atdd/runtime/driver/summary_sidecar.go` — owns
  `.gh-optivem/runs/<ts>/summary.jsonl` (one JSON row per **agent dispatch**:
  agent, model, effort, elapsed, usage, error). `appendSummaryLine` is
  best-effort, written as each dispatch completes (crash-survivable).
- `renderAgentSummary` (`driver.go:1539`) — single source of truth for the
  table shape; both the live banner (`printAgentSummary`) and the replay
  (`PrintSummaryFile` → `gh optivem run summary [ts]`) route through it.
- `dispatchRecord` (`driver.go:1386`) — already carries the per-dispatch
  `err`, so failed rows already render with a `✗` prefix.
- Ticket context is already in `ctx.State` at run time: `issue-num`,
  `issue-title`, `issue-url`, plus the parsed body fields the parse-ticket
  action writes (`description`, `acceptance-criteria`, `checklist`).
- `run_dir` is already seeded into the context (`driver.go:354`) and is the
  same `.gh-optivem/runs/<ts>/` the sidecar writes into.

## What is genuinely missing

1. **Overall verdict.** `Run`'s final error is returned to the cobra layer but
   never captured into `runState`, so no digest can stamp
   `Result: ✅ succeeded` / `❌ failed`.
2. **A human digest renderer + file.** No `summary.md` is written today.
3. **Ticket context in the digest.** The sidecar is per-dispatch only; it has
   no ticket title/body fields.

## Decisions resolved (so execution doesn't stall)

- **D1 — "tasks triggered" scope: agent dispatches only.** The digest lists
  the agent dispatches already captured in `dispatchRecord` (the "what
  actually ran + what it cost" view). The exhaustive node-by-node BPMN trace
  (service tasks, gates, human STOPs) stays in the long `--log-file`, where it
  belongs. Rationale: those records are already in hand for free; widening to
  full node trace would require the trace decorator to also append records — a
  separate, larger change. Cross-ref: if full-trace digest is wanted later,
  write a *fresh* plan.
- **D2 — ticket body excerpt: title + description + acceptance-criteria only.**
  Not the full raw body, and not the checklist (which can be long). Keeps
  "short" actually short and bounded. The full body is one click away via the
  emitted `issue-url`.
- **D3 — format: Markdown.** GitHub renders it inline; matches the repo's
  no-Pages, markdown-first doctrine in `CLAUDE.md`.
- **D4 — location: `.gh-optivem/runs/<ts>/summary.md`,** beside
  `summary.jsonl`. Not the rehearsal-wrapper's domain — the binary owns it so
  every `implement` run (not just rehearsals) gets it.
- **D5 — one renderer, two callers.** A new `renderRunDigest` is called by the
  run-end path AND exposed via `gh optivem run summary --markdown` so the live
  emission and the replay never drift (same contract `renderAgentSummary`
  already honours).
- **D6 — emission is best-effort,** mirroring `appendSummaryLine`: a write
  failure logs a `driver: warning:` to stderr and never blocks/fails the run.

## Items

### 1. Capture the overall verdict into `runState`

- Add a `result error` field (+ mutex-guarded setter, matching
  `appendRecord`) to `runState` in `driver.go`.
- In `Run` (`driver.go:201`), convert the single `return eng.RunProcess(...)`
  into a named-return + `defer` that stashes the final error into
  `runState.result` **before** the existing
  `defer runState.printAgentSummary(...)` fires. Watch LIFO ordering: the
  digest-writing defer (Item 3) must run after the result is stashed and
  before `logClose()`.
- Files: `internal/atdd/runtime/driver/driver.go`.
- Test: a `Run`-level test asserting `runState.result` is non-nil on a forced
  `RunProcess` failure and nil on success.

### 2. Add the digest renderer (`renderRunDigest`)

- New function (co-located with `renderAgentSummary`, or in
  `summary_sidecar.go` — executor's discretion) that takes:
  ticket title/num/url, description, acceptance-criteria, the
  `[]dispatchRecord`, and the verdict error; writes Markdown to an
  `io.Writer`.
- Layout (draft — refine at encoding time):
  - `# Run digest — #<num> <title>`
  - `**Result:** ✅ succeeded` / `❌ failed: <err>`
  - `**Ticket:** <url>`
  - `## Description` / `## Acceptance criteria` blockquotes (D2)
  - `## Agents dispatched` — the table, reusing `renderAgentSummary` output
    inside a fenced block so columns stay aligned in rendered Markdown.
- Reuse `renderAgentSummary` for the table body rather than re-implementing
  column logic.
- Files: `internal/atdd/runtime/driver/` (the file chosen above).
- Test: golden-style assertion on the rendered Markdown for (a) a passing run
  with usage, (b) a failed run, (c) a run with no dispatches.

### 3. Write `summary.md` at run end (best-effort)

- Add a `summaryMarkdownPath()` method on `runState` (sibling to
  `summaryPath()`), returning
  `<repoPath>/.gh-optivem/runs/<ts>/summary.md`; empty when `rs == nil`.
- Add a `writeRunDigest` best-effort writer (truncating create, like the
  log-file mirror) and call it from the run-end defer (Item 1), pulling the
  ticket fields from `sCtx` and the verdict from `runState.result`.
- Decide how the run-end defer reaches `sCtx`: capture it in the closure
  (sCtx is in scope at the defer site). Confirm during encoding.
- Files: `internal/atdd/runtime/driver/driver.go` (+ sidecar file for the
  path/writer helpers).
- Test: end-to-end driver test asserting `summary.md` exists, contains the
  ticket title and the verdict line, after a fake-dispatch run.

### 4. Surface the digest path to the operator

- After the run, echo the digest path so the operator can find it (mirrors how
  the long log path is surfaced). Confirm whether this belongs in the binary's
  run-end banner or the rehearsal wrapper — the **binary** is correct here
  since every `implement` run now emits it (cf. `printConfig`'s rationale that
  script-specific values stay in the wrapper, but this is binary-owned).
- Files: `internal/atdd/runtime/driver/driver.go`.

### 5. Add `--markdown` to `gh optivem run summary`

- New `--markdown` bool flag on `newRunSummaryCmd` (`run_commands.go:46`). When
  set, load the sidecar AND read the sibling `summary.md` (or re-render from
  records if we choose render-on-read — executor's discretion, but prefer
  reading the emitted file so replay == live). Default (unset) keeps today's
  table-only behaviour byte-for-byte.
- Files: `run_commands.go`, and an exported `driver.PrintRunDigestFile` (or
  reuse `renderRunDigest`) parallel to `PrintSummaryFile`.
- Test: cobra-level test for the `--markdown` path.

## Verification

- `scripts/test.sh` (or `go test -p 2 ./internal/atdd/runtime/driver/...` and
  the root package for `run_commands`) — never unbounded `go test ./...` on
  Windows.
- Manual: run a rehearsal, confirm `.gh-optivem/runs/<ts>/summary.md` renders
  cleanly on github.com and the verdict line matches the actual run outcome.
- Confirm `gh optivem run summary` (no flag) output is unchanged vs. `main`.

## Non-goals / explicitly out of scope

- Full node-by-node BPMN trace in the digest (D1 — fresh plan if wanted).
- Any change to the long `--log-file` shape or the existing `summary.jsonl`
  schema (additive only — do not break existing sidecar readers).
- GitHub Pages / any rendered-docs scaffolding (repo doctrine).
