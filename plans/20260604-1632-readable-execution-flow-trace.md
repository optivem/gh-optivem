# Readable execution-flow trace

## TL;DR

**Why:** No artifact shows what the BPMN engine actually executed in one run, nested by sub-process, with each step's input/output and the agent/command behind it — operators must eyeball a flat colored stream or open N per-agent transcripts to answer "where did this run halt / what did each step receive and return".
**End result:** Each run writes a live, human-readable `flow.txt` execution tree under its run dir — every step in dispatch order, nested by call-activity, showing `in`/`out` params, `↻ retry N` on loop-backs, and pointers to the agent prompt-log or classified command. It renders from the same per-dispatch `Event` record that drives the existing live colored stream, so the two cannot drift.

**Status:** Draft — decisions locked (this conversation), awaiting execution
**Created:** 2026-06-04 16:32 CEDT

> Supersedes **Direction A** of the discussion doc
> `plans/backlog/20260525-1418-structured-bpmn-execution-trace.md` (per
> `feedback_new_plan_not_extend` — fresh plan, the backlog doc is retired once
> this lands). That doc settled on a machine-parseable **JSONL** artifact whose
> consumer was `jq` + a future invariant-helper layer (Direction B). This plan
> deliberately **re-aims A at a human-readable execution tree** instead: the
> operator's actual need is "trace what got executed in the flow, with in/out
> params, and the agent/command behind each step." Parseability is **deferred**
> until a real consumer (Direction B file-mode) shows up — the structured record
> built here is the seam that keeps that option open without paying for it now.

## Problem

There is no artifact that shows **what the BPMN engine actually executed in one
run**, in order, with the nesting that makes the double-loop structure legible.
Two channels exist and neither fills the gap:

| Channel | What it is | Why it's not this |
|---|---|---|
| Live trace stream (`internal/atdd/runtime/trace/trace.go`) | Per-node colored `[trace HH:MM:SS]` banners, mirrored to `--log-file` | **Flat** — call-activities are only painted as visual markers; you can't see the inner contract/unit loop nested inside the acceptance loop. Tuned for watching live, not reading after. Service-/user-tasks show no **input** params. |
| Per-agent `*.events.jsonl` / `*.events.log` (`driver.go` `dispatchPaths`) | One Claude Code session's tool-call transcript, per dispatch | **Per-agent**, not per-run. Covers what *one agent* did, not the flow that dispatched it. |

So an operator asking "why did this run take 21 minutes / where did it halt /
what did each step receive and return" has to eyeball a flat colored stream or
open N separate agent transcripts.

## Goal

A single, human-readable **execution tree** per run — every BPMN step in
dispatch order, **nested by sub-process**, each step showing its **input
params** and **output** (outcome + state delta), and for the leaf nodes a
**pointer** to the agent (name + prompt-log) or the **command** (line + classified
result). Written **live** so it survives a halt. Rendered from the **same
per-dispatch record** that drives the existing live stream, so the two cannot
drift.

Sample of the intended artifact is captured in the conversation that produced
this plan; the shape is the indented tree with `in`/`out` lines per step, `↻
retry N` on loop-back re-dispatches, and an at-a-glance footer (result,
wall-clock, commit SHA, counts).

## Decisions (locked)

- **D1 — Readable tree, not JSONL (for now).** The artifact is human-first. The
  JSONL/`jq` artifact from backlog-doc Direction A is **not built** in this plan.
- **D2 — One source of truth.** Extract a per-dispatch `Event` record that
  `wrap()` populates once; **both** the existing live stream **and** the new tree
  renderer consume it. No second hand-written formatter. (This honors the
  backlog doc's own locked sub-decision: "single source of truth for event
  shape.")
- **D3 — Live to a file, not post-run render.** The tree is written
  incrementally as the run executes, into the per-run dir, so a hung/halted run
  still leaves the partial tree on disk. No separate `trace render` command.
- **D4 — Result + pointers only (depth).** Each step shows outcome + state
  delta + the command line + agent name/prompt-log + files touched. It does
  **not** inline command stdout or agent response text — those live in
  `--log-file` and `*.prompt.md` / `*.events.log`, which the footer points to.
  The **one** exception is the existing infra-halt stderr tail
  (`writeInfraHaltBanner`), retained because that is the moment output is needed
  inline.
- **D5 — Artifact location + name.** `<repoPath>/.gh-optivem/runs/<run-ts>/flow.txt`
  (one per run, alongside the per-dispatch `NNN-*.prompt.md`). **Not** named
  `*.events.*` — that suffix already means the per-agent transcript and reusing
  it would conflate the two.
- **D6 — Direction B stays on spy fixtures.** No file-based invariant helpers in
  this plan. The `Event` seam (D2) is what keeps B's file-mode buildable later;
  building it now is out of scope (no consumer).

## Concurrency note

Active uncommitted work on `plans/20260527-1147-dsl-implementer-ct-system-driver-scope.md`
touches `internal/atdd/runtime/statemachine/process-flow.yaml`,
`statemachine/transitions_test.go`, `actions/bindings.go`, `diagram/diagram.go`.
**Item 2 (sub-process depth)** below is the only one that reaches into the
statemachine engine — coordinate / rebase against that work before starting
Item 2, and re-check `git log` before committing (per
`feedback_concurrent_agent_collision`). Items 1, 3, 4 are confined to the
`trace` package + `driver` wiring and should not collide.

## Items

_All items landed (commit pending) — see git history for the implementation.
The `Event` record (D2), sub-process depth threading (D2), tree renderer +
live writer (D3/D4/D5), and driver wiring (D3/D5) are all in
`internal/atdd/runtime/trace/{trace.go,tree.go}` and
`internal/atdd/runtime/driver/driver.go`. Only the operator verification below
remains._

## Verification (operator, after the items land)
- Run `gh optivem implement` on a slice that exercises a loop-back (a contract
  cycle that needs ≥2 implement passes) and confirm `flow.txt`:
  shows the inner cycle **nested** under the outer call-activity; shows `in`/`out`
  on service-/user-tasks; tags the re-dispatch `↻ retry 2`; and the footer counts
  match `printAgentSummary`.
- Induce an infra halt (e.g. break the test command) and confirm the **partial**
  `flow.txt` ends at the failing step with the stderr tail and a `halted (infra)`
  footer.
- Diff the live stream before/after Item 1's refactor on the same run to confirm
  **no regression** in the existing colored output.

## Out of scope / explicitly not doing
- JSONL / `jq`-parseable artifact (backlog-doc Direction A's original form) —
  deferred until a consumer exists (D1, D6).
- File-based invariant helpers (backlog-doc Direction B) — stays on spy fixtures
  (D6).
- Spy state-capture knob (backlog-doc Direction C) — rejected there, not revived.
- Inlining full command stdout or agent response text (D4) — pointers only.
- Any change to the live colored stream's format beyond the Item-1 refactor that
  must leave it byte-stable.
- Diagram regeneration steps (the regenerate-diagram workflow owns
  `docs/process-diagram.md` + `docs/images/*.svg`; per
  `feedback_plans_no_diagram_regen` this plan adds none).
