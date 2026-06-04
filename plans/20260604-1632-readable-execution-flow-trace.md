# Readable execution-flow trace

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

### Item 1 — Extract the shared per-dispatch `Event` record (D2)
- [ ] In `internal/atdd/runtime/trace/`, introduce an `Event` struct holding what
      `wrap()` already gathers: node ID, kind, the selector (action / agent /
      binding / process), **input params** (`ctx.Params` snapshot — new for
      service-/user-tasks; today only call-activity prints `params=`), outcome
      (value/bool/err), state delta (`pre`→`post`), elapsed, user-task files
      delta, and the call-activity verdict.
- [ ] Refactor `writeEnter`/`writeExit` so the **live stream renders from
      `Event`** rather than reading node/outcome directly — proving the record is
      sufficient for the existing output (no visible change to the live stream).
- [ ] Snapshot `ctx.Params` in `wrap()` for **all** node kinds so the `in` line
      can be populated for service-/user-tasks, not just call-activities.

### Item 2 — Thread sub-process depth into the record (D2; engine seam)
- [ ] Establish a run-scoped **depth** signal so the tree renderer can indent a
      call-activity's children under it. Preferred: a depth counter the
      call-activity wrapper increments before `inner()` and decrements after —
      **verify first** that the engine runs a sub-process synchronously inside the
      call-activity `NodeFn` (so children dispatch during the parent's `inner()`
      and the bracketing is correct). If sub-process execution is not synchronous
      within the node, fall back to having the engine expose current depth on
      `statemachine.Context`.
- [ ] Record `depth` (and the enclosing scope/process-instance id) on each
      `Event` so the renderer can both indent and detect loop-back
      re-dispatches.

### Item 3 — Tree renderer + live writer (D3, D4, D5)
- [ ] Add a tree renderer in the `trace` package that formats an `Event` into the
      indented tree line(s): `<glyph> NODE_ID  kind  selector` + `in` / `out` /
      `agent` / `cmd` / `files` sub-lines, indented by `depth`.
- [ ] Annotate **loop-back re-dispatches** (`↻ retry N`) by counting prior
      occurrences of the same node id within the same scope/process-instance.
- [ ] Surface the **command line** on every service-task `out`/`cmd` line — it's
      already in `ctx.State` as `command-line` (today only shown on infra-halt);
      classify the result (`PASS`/`RED`/`INFRA`) from the existing outcome/state.
- [ ] Wire a live writer: open `<run-ts>/flow.txt` at driver startup, write each
      `Event` as it completes (flush per step so a halt leaves a usable partial
      file). Plain text, **no ANSI** (this file is the decolored, readable
      sibling of the colored `--log-file` stream).
- [ ] Emit the header (run metadata) at start and the footer (result, wall-clock,
      commit SHA, counts, pointers to `*.prompt.md` + `--log-file`) at end. Derive
      footer counts from the recorded `Event`s; reconcile with the existing
      `printAgentSummary` / `summary_sidecar.go` so counts agree (do not invent a
      second, divergent tally).

### Item 4 — Driver wiring + run-dir plumbing (D3, D5)
- [ ] Resolve `<run-ts>/flow.txt` from the existing `runState` run directory
      (`rs.runTimestamp`, `.gh-optivem/runs/<run-ts>/`) and pass the writer into
      `trace.WrapAll` via `trace.Deps`.
- [ ] Ensure the file rides the existing `--keep-runs` pruning (it lives under the
      pruned run dir, so it's covered for free — confirm, don't add new pruning).
- [ ] Fail-soft: if the file can't be opened, warn to stderr and continue (same
      policy as `openEventsLog` / PID markers) — the trace is informational, never
      load-bearing for the run.

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
