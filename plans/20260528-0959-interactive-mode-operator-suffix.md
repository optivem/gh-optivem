# 20260528-0959 — Interactive-mode operator suffix

Spinoff from [20260527-2214-runtime-prompts-audit.md](./20260527-2214-runtime-prompts-audit.md) — the prompt-audit plan compressed existing prose; this plan adds a small new feature (operator-facing terminal hint) without touching the audit's compression work.

## Background

Agents are dispatched in one of two modes (`internal/atdd/runtime/clauderun/clauderun.go:281` `Headless bool`):

- **Headless** (`claude -p <prompt>`) — one-shot. The process exits when the agent stops emitting. No human waiting on a session prompt.
- **Interactive** — the operator runs Claude Code with the rendered prompt and watches the agent work. When the agent finishes, the Claude Code REPL stays open. New operators don't always know how to close cleanly (`/exit`) or how to redirect (just type feedback into the prompt).

The current shared `preamble.md` says "When the work is done, exit cleanly" — but that's an instruction to the *agent*, not a hint to the *operator*. There's nothing telling the human what to do when the agent stops talking.

## Goal

When a dispatch is interactive, append a short operator-facing block at the end of the prompt explaining the two terminal options:

```
---

**Operator:** when this agent's work is complete and you approve, type `/exit` to close the session and continue the cycle. To redirect, type feedback into the prompt and the agent will incorporate it.
```

When headless, no change — the suffix would be a no-op (no REPL) and just wastes tokens on every headless dispatch.

## Items

### 1. Add `internal/assets/runtime/shared/interactive-suffix.md`

Body is the operator-facing block above. Mirror the `preamble.md` / `scope.md` shape (no frontmatter; plain markdown).

### 2. Wire conditional concatenation in `internal/atdd/runtime/agents/embed.go::Prompt`

The current concatenation (line ~78) is:

```go
return sharedPreamble + "\n\n" +
    sharedScope + "\n\n" +
    body, nil
```

The new contract needs to know whether the dispatch is headless. Two shapes:

- **Option A — add a `headless bool` arg to `Prompt`.** Callers in `clauderun.go` already know `opts.Headless`; pass it through. Append `interactive-suffix.md` only when `!headless`.
- **Option B — keep `Prompt` signature, append suffix dispatcher-side.** In `clauderun.go::renderPrompt`, after calling `agents.Prompt(...)`, if `!opts.Headless`, append the suffix string. Keeps `Prompt` pure (no mode flag).

Recommend **Option B** — `embed.Prompt` is a pure markdown-concatenation helper today; adding a `headless` flag couples it to dispatcher semantics. Option B keeps the concern in the dispatcher.

### 3. Load the suffix at init via the existing `mustReadAsset` path

`embed.go` already does `mustReadAsset(preamblePath)` / `mustReadAsset(scopePath)` at package init. Add a parallel `interactiveSuffix = mustReadAsset(interactiveSuffixPath)` and export it via a small helper (e.g. `agents.InteractiveSuffix() string`) so `clauderun.go` can append it without re-reading the embedded asset on every render.

### 4. Test coverage

- Unit test in `clauderun_test.go` that `renderPrompt` with `Headless: false` ends with the suffix text (e.g. `mustContain(got, "type \`/exit\` to close the session")`).
- Unit test with `Headless: true` does NOT contain the suffix.
- Existing tests should be unaffected since they default to whichever `Headless` value `newOpts()` sets.

### 5. Audit downstream consumers

`embed.Prompt` is called from `clauderun.go::renderPrompt` (around line ~700 — confirm during execution). Verify no other call site bypasses it; if any does, decide whether they need the suffix too (`gh optivem prompt show <task>`-style dump commands shouldn't add it, since they're not a real interactive dispatch).

## Verification

- After implementation, run an interactive dispatch (e.g. `gh optivem atdd run <ticket>` without `--headless`, depending on the actual CLI flag) and visually confirm the suffix appears at the bottom of the prompt.
- Run a headless dispatch and confirm the suffix is absent.
- `go test ./...` clean.
