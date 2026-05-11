# Trim hidden `atdd debug` subtree

> ⚠️ **Needs explicit human approval before implementation. Discuss first.**
> This plan is a draft. Do not execute any step until the author signs off.

## Context

`atdd_commands.go` registers a hidden `atdd debug` parent
(`atdd_commands.go:329`) with five diagnostic children:

| Command | Purpose |
|---------|---------|
| `atdd debug pick-top-ready` | Print top Ready item without moving it |
| `atdd debug classify --issue N` | Run classify package's deterministic fast path |
| `atdd debug next-phase --node X --state k=v` | Print which outgoing edge engine picks |
| `atdd debug gate <binding>` | Evaluate one gateway binding standalone |
| `atdd debug release --issue N` | Run release primitives end-to-end |

The author confirmed (2026-05-11) these are never reached for during
debugging: "NEVER". The hidden flag was the original hedge — kept the
commands out of student-facing help but accessible if needed. With no
actual use, they are dead CLI surface.

Removing them reduces:

- ~200 lines of CLI wiring in `atdd_commands.go`.
- Whatever internal-package public exports exist *only* to feed these
  commands (audit in Step 3).
- Test surface for the same.

## Steps

### Step 1 — Remove the five debug subcommand functions

Delete from `atdd_commands.go`:

- `newAtddDebugCmd()` (lines 329-343) — the hidden parent.
- `newAtddDebugPickTopReadyCmd()` (lines 347-363).
- `newAtddDebugClassifyCmd()` (lines 369-389).
- `newAtddDebugNextPhaseCmd()` (lines 398-447) and its helper `loadDebugConfig()` (lines 453-463).
- `newAtddDebugGateCmd()` (lines 467-491).
- `newAtddDebugReleaseCmd()` (lines 496-549).

Remove `newAtddDebugCmd()` from `newAtddCmd`'s `AddCommand` block
(line 73).

### Step 2 — Remove the file-header reference

`atdd_commands.go:7-12` documents the hidden debug parent:

> A hidden `debug` parent groups the diagnostic helpers — pick-top-ready,
> classify, next-phase, gate, release — so each underlying runtime package
> can be exercised standalone without rerunning the whole pipeline. The
> hidden flag (Cobra's `Hidden: true`) keeps these out of the default help
> text; `gh optivem atdd debug --help` still works for anyone who knows
> they exist.

Delete this paragraph. The opening sentence ("Two public commands mirror
today's slash commands") is unchanged; just drop the debug paragraph.

### Step 3 — Audit internal-package exports

For each runtime package the deleted commands imported, check whether any
exported symbol is now used *only* internally and can be made package-private
(or deleted outright). Likely candidates:

| Package | Symbol | Other callers? |
|---------|--------|----------------|
| `internal/atdd/runtime/board` | `PickTopReady`, `Options` | Driver/`manage-project` uses `PickTopReady`. Keep. |
| `internal/atdd/runtime/classify` | `Classify`, `Options` | Driver uses inside pipeline. Keep. |
| `internal/atdd/runtime/gates` | `Registry.Lookup` | Driver uses internally? Check. |
| `internal/atdd/runtime/statemachine` | `Engine.NextEdge` | Driver loop uses. Keep. |
| `internal/atdd/runtime/release` | `RemoveDisabledMarkers`, `RemoveOptions`, `InteractiveConfirmer`, `Commit`, `CommitOptions`, `CloseIssue` | RELEASE node action uses some; `InteractiveConfirmer` may be debug-only. Check. |

For each symbol with no remaining callers after Step 1, either downgrade
to package-private (preferred — keeps the test surface) or delete (if
the test was also debug-specific).

### Step 4 — Remove debug-only tests

Search `atdd_commands_test.go` (or equivalent) for any test that targets
`newAtddDebug*` constructors or the `gh optivem atdd debug ...` invocation
strings; delete those test cases.

Run `go test ./...` (scoped per `feedback_go_test_windows.md` — `-p 2` or
`scripts/test.sh`, not unconstrained) and confirm no failures.

### Step 5 — Verify the user-visible surface

```bash
gh optivem atdd --help          # public surface: implement-ticket, manage-project, show
gh optivem atdd --help-all      # same (no hidden debug parent anymore)
gh optivem atdd debug --help    # error: unknown command "debug" for "gh optivem atdd"
```

## Out of scope

- **Renaming `atdd show diagram` to flatten the 3-level depth.** That
  question (whether the `show` parent should remain or commands flatten to
  `atdd diagram`) lives in
  [`20260511-2030-atdd-surface-restructure.md`](20260511-2030-atdd-surface-restructure.md)
  alongside the broader ATDD-parent restructure.
- **Restructuring `implement-ticket` / `manage-project`** — handled by
  the restructure plan above.
- **Removing the `atdd` parent itself** — handled by the restructure plan.

This plan is intentionally surgical: delete-only, no behavior changes
on remaining commands.

## Open questions

1. **`internal/atdd/runtime/preflight`, `internal/atdd/runtime/override`,
   `internal/atdd/runtime/diagram`** — do any of these have exported
   symbols only used by debug commands? Step 3's audit covers the most
   likely candidates; double-check during implementation.
2. **`gates.RegisterAll(reg, gates.Deps{})`** — used by `debug gate` to
   evaluate one binding in isolation. Confirm the driver itself uses
   `RegisterAll` (it almost certainly does to populate the registry at
   startup); if so, keep the export.
