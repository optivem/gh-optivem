# CLI best-practices alignment for `gh optivem init`

🤖 **Picked up by agent** — `Valentina_Desk` at `2026-04-27T06:12:50Z`

Review of `main.go`, `internal/config/config.go`, and `internal/log/log.go` against standard CLI conventions (clig.dev, POSIX/GNU flag norms, typical `gh` extension shape).

Verdict: mostly aligned. Output layer (colors, streams, levels) is solid. Flag layer has real rough edges that will surface as the command surface grows.

## Already aligned (no action)

- Colors via `fatih/color` honor `NO_COLOR`/`FORCE_COLOR`, TTY-detect, Windows-safe.
- Stream discipline: errors/warnings to stderr, info/success to stdout.
- Standard flags present: `--version`, `--dry-run`, `--verbose`/`-v`, `--quiet`/`-q`, `--log-file`.
- Fail-fast validation in phases (format → network → resolve) before any mutation.
- `--verbose` and `--quiet` are mutually exclusive.
- `--report-bug` is opt-in with explicit confirmation — good consent model.
- TTY detection in `confirmBugReport` ([main.go:430-436](../main.go#L430-L436)).
- Banner shows `(default)` vs user-supplied via `flag.Visit`.

## Fix — low priority

### 9. No `--json` machine-readable output

Less critical for a scaffold-once tool, but the final "System / Repository / Actions / …" summary is useful as structured output for CI chains that invoke this tool.

- **How to apply:** add `--json` that emits the summary (and maybe the phase/step results) as JSON on stdout instead of the human table. Keep text output on stderr so logs still work.

## Recommended order of execution

1. Item 1 (`-v` ambiguity) + item 2 (top-level `--help`). Tiny, user-visible, no breaking change.
2. Item 5 (`--yes`). Small, unblocks CI use.
3. Item 6 (auto-upgrade default). One-line change + a notice; biggest surprise to remove.
4. Item 3 (Cobra migration). The deferral trigger has fired — six subcommands now duplicate dispatch logic. Items 7, 8, and 10 fall out of this work, so do them in the same migration rather than separately.
5. Item 4 (flag-name pass). Do as one batch with deprecation warnings; ship in a dedicated release. Easier post-Cobra (uniform flag definitions).
6. Item 9 (`--json`) only if a real consumer needs it.
