# Switch Mermaid `flowchart` → `graph` workspace-wide

## TL;DR

**Problem:** Mermaid diagrams declared with `flowchart TD` / `flowchart LR`
fail to render in VS Code when the Mermaid preview plugin is downgraded
(*"No diagram type detected matching given configuration for text:"*). The
`flowchart` keyword is a newer alias (Mermaid 8.3+); older bundled Mermaid only
understands the original `graph` keyword. GitHub ships modern Mermaid so it
renders fine there — the breakage is VS-Code-only.

**Fix:** Replace `flowchart <DIR>` with `graph <DIR>` everywhere. The two are
exact aliases (identical rendering) and `graph` works in **every** Mermaid
version *and* on GitHub — strictly more portable. Going forward, default to
`graph`.

**Done already:** `substack/articles/jc-draft/PAID-ha-db.md` (6 blocks,
committed `7156a1b`).

**Scope of this plan:** the remaining sites — hand-written course/agent docs
(direct edit) and the gh-optivem Go generators + their generated docs + the one
test assertion.

---

## Part A — Hand-written docs (direct `flowchart TD` → `graph TD`)

Plain Markdown, no generator behind them. One occurrence each unless noted.

- [ ] `courses/workshops/introducing-atdd-to-a-team.md` (line 10)
- [ ] `courses/02-atdd/accelerator/course/09-architecture-external-stubs/00-overview.md` (line 33)
- [ ] `courses/02-atdd/accelerator/course/08-architecture-scenario-dsl/00-overview.md` (line 35)
- [ ] `courses/02-atdd/accelerator/course/06-architecture-channels/00-overview.md` (line 40)
- [ ] `courses/02-atdd/accelerator/course/05-architecture-drivers/00-overview.md` (line 34)
- [ ] `courses/02-atdd/accelerator/course/04-architecture-clients/00-overview.md` (line 37)
- [ ] `courses/.claude/agents/README.md` (line 10)

Note: `07-architecture-usecase-dsl/00-overview.md` already uses `graph TD` — no change.

## Part B — gh-optivem generators (edit Go, then regenerate)

Editing the generated `.md` directly would be reverted on the next
`gh optivem ... show`. Change the emitter, the test, then regenerate.

- [ ] `internal/atdd/runtime/architecture/architecture.go:102` — `"```mermaid\nflowchart %s\n"` → `"```mermaid\ngraph %s\n"`
- [ ] `internal/atdd/runtime/diagram/diagram.go:142` — `"```mermaid\nflowchart LR\n"` → `"```mermaid\ngraph LR\n"`
- [ ] `internal/atdd/runtime/diagram/diagram.go:237` — `"```mermaid\nflowchart TD\n"` → `"```mermaid\ngraph TD\n"`
- [ ] `internal/atdd/runtime/diagram/diagram.go:540` — comment `Mermaid's flowchart TD treats…` → `graph TD treats…` (keep comment in sync)
- [ ] `internal/atdd/runtime/architecture/architecture_test.go:83` — assertion `strings.Contains(got, "flowchart TD\n")` → `"graph TD\n"`
- [ ] Run `go test ./internal/atdd/runtime/...` — confirm green
- [ ] Regenerate `docs/architecture-diagram.md` via `gh optivem architecture show > docs/architecture-diagram.md`
- [ ] Regenerate `docs/process-diagram.md` via `gh optivem process show > docs/process-diagram.md`
- [ ] Verify the two regenerated docs now emit `graph TD` / `graph LR` (no stray `flowchart`)

## Part C — sweep / verify

- [ ] `grep -rn "flowchart" courses gh-optivem` returns only intentional prose (if any), no diagram declarations
- [ ] Commit via `/commit` (touches `courses` + `gh-optivem`)

## Out of scope

- `worktrees/**` and `.tmp/**` — scratch / rehearsal copies, regenerated; do not edit.
- `shop/docs/design/**` already use `graph` — no change.
