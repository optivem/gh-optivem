# 2026-06-15 07:22 UTC — Module extraction: architecture / diagrams

**Child of** `20260615-0548-gh-optivem-modular-monolith-parent.md`. Second module cut. Higher ripple than dev-workflow: the packages were nested in the `atdd/runtime/` tree and their old paths were hardcoded in CI, agent definitions, and docs.

## What changed

Moved both renderer packages out of `atdd/runtime/` into a new `internal/diagrams/` module (pure move + reference updates; no logic changes):

- `internal/atdd/runtime/architecture` → `internal/diagrams/architecture`
- `internal/atdd/runtime/diagram` → `internal/diagrams/diagram`

References updated:

- **Go imports:** `architecture_commands.go`, `process_commands.go`.
- **Golden test:** `architecture_test.go` repo-root walk-up `..×4` → `..×3` (package moved one level shallower) + comment.
- **CI workflows:** `regenerate-architecture-diagram.yml`, `regenerate-diagram.yml` path filters.
- **Agent definitions:** `architecture-sync.md`, `bpmn-logic-audit.md`, `diagram-content-editor.md`, `diagram-tweaker.md`, `token-usage-audit.md`.
- **Docs prose:** `CONTRIBUTING.md`, `plans/backlog/20260526-1746-rebuild-onboard-external-system.md`, parent plan.

`diagram → statemachine` import is unchanged (statemachine did not move; downward dep to the engine core is allowed).

## Deliberately left at the OLD path (flagged)

Two **emitted** renderer header strings that get written into the generated diagrams:

- `internal/diagrams/architecture/architecture.go:82` (+ a code comment at line 6)
- `internal/diagrams/diagram/diagram.go:122`

Reason: `architecture.go` has a golden test asserting byte-identical output against the committed `docs/architecture-diagram.md`. Changing the emitted path would require regenerating that golden doc, which conflicts with the standing rule *"never edit generated diagram files / don't regenerate locally — let CI regenerate."* Leaving the emitted headers (and the two generated `docs/*-diagram.md`) at the old path keeps everything self-consistent and the golden test green. **Open decision:** update the headers + regenerate (overriding the local-regen rule), or route the header change through a CI-driven regeneration step later.

## Verification

- `go build ./...` ✓
- `go test ./...` ✓ (all green, incl. `internal/diagrams/architecture` golden + render tests)

## Status

Done, with the two emitted-header strings flagged above pending a regeneration decision.
