# Headless agents must not call `AskUserQuestion` — inject a mode-conditional no-ask clause

**Status:** proposed
**Created:** 2026-05-30 17:01 CEDT

## Problem

A headless dispatch (`claude -p`, no stdin, no operator at a REPL) can call
`AskUserQuestion`, which can only ever be **auto-rejected** — there is no human
to answer. Every such call burns turns (and wall-clock) for nothing, and the
agent typically loops on "you rejected my call, how should I proceed?" before
eventually self-correcting.

Observed live in the rehearsal-71 `system-implementer` dispatch
(`004-system-implementer.events.jsonl`): **2 `AskUserQuestion` calls**, ~3–4
minutes of a >12-minute run wasted, triggered by a prompt-framing ambiguity
("Getting Started says begin at Phase 1 (ANALYSIS), but the git log shows phases
3–5 already committed"). The agent eventually resolved it correctly on its own —
*"The prompt is clear — no contradiction… my job is to write production code"* —
but only after the detour. The right behaviour is to skip the ask entirely and
go straight to the best-supported interpretation.

This is the single highest-value fix for headless runtime/token waste, because
the headless harness has **no other guardrail**: `runHeadless`
(`internal/atdd/runtime/clauderun/clauderun.go`) invokes
`claude -p … --output-format stream-json --verbose [--model] [--effort]` with
**no `--max-turns`, no context deadline, and no token budget** — nothing stops a
loop but the operator.

## What already exists (reuse, do not reinvent)

- `clauderun.renderPrompt` (`clauderun.go` ~L844) already has the exact
  mode-conditional render seam we need:
  ```go
  if !opts.Headless {
      rendered = strings.TrimRight(rendered, "\n") + "\n\n" + agents.InteractiveSuffix() + "\n"
  }
  ```
  The interactive REPL hint is appended **only** in interactive mode. The
  headless no-ask clause is the symmetric counterpart on the `opts.Headless`
  branch.
- `agents.InteractiveSuffix()` is the established pattern for a shared,
  mode-specific prompt tail sourced from the embedded asset set
  (`internal/assets/runtime/shared/`), kept as prose rather than a Go literal.
- `opts.Headless` is already threaded through `Options` → `RunOpts` and is the
  single switch the runner uses to choose `claude -p` vs interactive.

## What is genuinely missing

1. A headless-only prompt clause instructing the agent **not** to call
   `AskUserQuestion`, and what to do instead (resolve + proceed).
2. The wiring on the `opts.Headless` branch of `renderPrompt` to append it.

## Decisions resolved (so execution doesn't stall)

- **D1 — headless-only, interactive unchanged.** Interactive mode keeps
  `AskUserQuestion`: there is a human at the REPL, asking is the whole point,
  and `InteractiveSuffix()` already invites redirection. The clause is appended
  *only* when `opts.Headless` is true.
- **D2 — clause content = "don't ask; resolve and proceed".** It must (a)
  state the agent is running headless with no operator, (b) forbid
  `AskUserQuestion`, (c) tell it to pick the best-supported interpretation,
  **state the assumption explicitly** in its output, and proceed, and (d) give a
  last-resort escape: if genuinely blocked, emit a structured blocked output and
  stop — never spin.
- **D3 — clause lives as a shared asset**, mirroring `InteractiveSuffix()`:
  add `agents.HeadlessSuffix()` backed by
  `internal/assets/runtime/shared/headless-no-ask.md`, so it is editable as
  prose and reusable across every headless agent (not system-implementer-only).

## Items

1. **Add the shared asset.** Create
   `internal/assets/runtime/shared/headless-no-ask.md` with the D2 clause body.
   Keep it short — it is appended to every headless prompt.
2. **Add `agents.HeadlessSuffix()`** in `internal/atdd/runtime/agents/`
   (sibling to `InteractiveSuffix()`), reading the new asset via the same
   embed path.
3. **Wire it into `renderPrompt`** (`clauderun.go`): add the symmetric
   `if opts.Headless { rendered = … + agents.HeadlessSuffix() + … }` append,
   adjacent to the existing `!opts.Headless` interactive append.
4. **Tests** in `clauderun_test.go`: mirror the existing `InteractiveSuffix`
   render assertions — a headless render contains the no-ask clause and an
   interactive render does **not** (and vice versa for the interactive suffix).

## Out of scope (cross-reference, don't fold in)

- `--max-turns` cap and `context.WithTimeout` wall-clock deadline were
  considered as additional headless backstops but **not** chosen for this plan.
  If wanted later, write a *fresh* plan — they are independent of the prompt
  clause and have their own design (where the cap value comes from, how a
  timeout surfaces as an actionable error).
- Sibling plan from the same investigation:
  `plans/20260530-1702-channels-field-channel-by-channel.md` (bounding the
  system-implementer dispatch's blast radius per channel).

## Verification (operator)

- Re-run a headless rehearsal dispatch and confirm the run's
  `*.events.jsonl` contains **zero** `AskUserQuestion` tool calls, and that an
  ambiguous prompt is resolved inline with a stated assumption rather than an
  ask-then-reject detour.
