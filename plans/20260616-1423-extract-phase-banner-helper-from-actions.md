# 2026-06-16 14:23:00 UTC — Extract the phase-banner presentation helper out of the `actions` binding package

## TL;DR

**Why:** `internal/atdd/process/actions/banner.go` (`WritePhaseBoundary`) is **pure terminal presentation** — it formats a coloured `[phase] start/end …` banner — yet it lives in the `actions` package, whose job is the BPMN binding layer (YAML `action:` → registered `NodeFn` that performs process logic). `WritePhaseBoundary` is not registered, takes no `*statemachine.Context`, returns no `statemachine.Outcome`, and is called only by the driver. It is a *helper*, not *true custom logic* (contrast `validateExternalSystemsRegistered`), and it sits in the wrong layer.
**End result:** `WritePhaseBoundary` lives in the presentation layer (`internal/atdd/runtime/outlog`, next to the `Out` writers it conceptually belongs with). The `actions` package holds only bound logic + its computation helpers. Behaviour and output are byte-for-byte identical.

## Outcomes

- The phase-banner formatter moves to `outlog` — the package that already owns operator-facing output (`Out.Phase` is the "top-level banner" channel per its doc comment).
- `actions/banner.go` is deleted; the `actions` package no longer carries any presentation-only helper.
- The driver's two call sites (`driver.go:1602`, `driver.go:1605`) call `outlog.WritePhaseBoundary` instead of `actions.WritePhaseBoundary`. The driver already imports `outlog`, so no new dependency edge is created.
- No behavioural change: same signature, same colour, same blank-line bracketing, same second-rounding.

## ▶ Next executable step (resume here)

Step 1 — mechanical move. This is a straight relocation with one caller; no design decision is gated. Start by creating `internal/atdd/runtime/outlog/banner.go`.

## Steps

- [ ] **Step 1: Create `internal/atdd/runtime/outlog/banner.go`.** Move `WritePhaseBoundary` verbatim into `package outlog`, keeping the identical signature `WritePhaseBoundary(w io.Writer, edge, phaseName string, elapsed time.Duration)` and the full doc comment. Add the file's own imports (`fmt`, `io`, `time`, `github.com/fatih/color`). Keep the `nil`-writer no-op and the `start`/`end` switch unchanged.
- [ ] **Step 2: Repoint the driver.** In `internal/atdd/runtime/driver/driver.go` (inside `wrapPhaseBoundaries`, lines ~1602 and ~1605) change `actions.WritePhaseBoundary(...)` → `outlog.WritePhaseBoundary(...)`. `outlog` is already imported (`driver.go:46`); leave the `actions` import (still used elsewhere in the file).
- [ ] **Step 3: Delete `internal/atdd/process/actions/banner.go`.** Confirm no other reference remains: `grep -rn "actions.WritePhaseBoundary" .` returns nothing, and the `bindings_test.go` matches on "banner" are unrelated (they assert stderr scope-violation text, not this helper).
- [ ] **Step 4: Build + targeted tests.** Per-package, no unbounded `go test ./...` on Windows:
  `go build ./...` then `go test ./internal/atdd/runtime/outlog/ ./internal/atdd/runtime/driver/ ./internal/atdd/process/actions/`.
- [ ] **Step 5 (optional): Cover the moved helper.** `outlog` has no banner test today. Add a small `outlog/banner_test.go` asserting the `start` (leading blank line + `[phase]  start  <name>`) and `end` (`[phase]  end    <name>  <elapsed>` + trailing blank line) shapes and the `nil`-writer no-op. Cheap regression guard now that the helper has a home of its own.

## Notes / decisions

- **Free function vs. method on `*Out`.** Keep it a free function taking `io.Writer` (zero risk, identical call shape). A later refinement could make it `(*Out).WritePhaseBoundary` routed at `Phase` level, but the driver currently passes a bare `w` into `wrapPhaseBoundaries`, so the free function is the faithful move. Defer the method form.
- **Scope is deliberately just this one helper.** The narration `Fprintln`s *inside* registered actions (e.g. `moveToInProgress` → `Out.Phase`, `bindings.go:346`) are an action narrating its own effect — defensible, left alone. `WritePhaseBoundary` is the only thing in `actions` that is presentation-only *and* in the wrong layer, so it is the clean first move.

## Deferred (separate plan if wanted)

- Full logic-vs-helper audit of the `actions` package: classify each symbol as **bound logic** (registered `NodeFn`), **computation helper** (`ResolveLayerPaths`, `pathInScope`, `shellEscape`, `dirtyTreePaths`, fingerprint fns), or **presentation**, and decide whether the computation helpers should be lifted into their own `actions/helpers.go` (or sub-package) once `bindings.go` (~1690 lines) is split. Not required for this change.
