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

## Fix — medium priority

### 3. Stdlib `flag` is a ceiling

Hand-rolled subcommand dispatch, short/long pairs, and usage. Fine for one subcommand; painful once there's a second — and there are now six (`init`, `build`, `run`, `test`, `stop`, `clean` at [main.go:71-90](../main.go#L71-L90)), each routed through its own `dispatchX` function.

- **How to apply:** migrate to [spf13/cobra](https://github.com/spf13/cobra) (what `gh` itself uses). Buys: nested subcommands, `-h`/`--help` on every level, auto-generated `completion bash|zsh|fish|pwsh`, consistent help output, env-var binding via viper, man-page generation.
- **When:** the deferral trigger has fired — six subcommands now duplicate dispatch boilerplate. Migration is now the right next move.

### 4. Flag-name inconsistency

Mixed negation and verb choices on booleans:

- `--skip-local-tests` vs `--exclude-legacy` (skip vs exclude — same intent, two words)
- `--no-commit-on-failure` (use `no-`) vs `--keep-local`, `--dry-run` (don't)
- `--report-bug` (verb-noun) vs `--keep-local` (verb-adj)

- **How to apply:** pick one convention and rewrite. Common rule: default is the positive/safe behavior, `--no-X` flips it off. So:
  - `--exclude-legacy` → keep; `--skip-local-tests` → `--no-local-tests` (or `--exclude-local-tests` for parallelism with `--exclude-legacy`).
  - `--keep-local` → `--no-cleanup-local` (or keep it as-is and deprecate other `--no-*` flags; trade-off).
- **Recommended:** unify on `--no-<thing>` for "turn off default behavior" and verb-noun for opt-in actions. Do this as a single breaking-change pass with old-flag aliases printing a deprecation warning for one release.

## Fix — low priority

### 7. `printUsage` is thin

[main.go:93-104](../main.go#L93-L104) now lists six commands (`init`, `build system`, `run system`, `test system`, `stop system`, `clean system`) with no example for any of them and no hint that `<command> --help` shows flags.

- **How to apply:** add one concrete example per command (or at least for `init`) and a "Run `gh optivem <command> --help` for command-specific flags." footer.

### 8. No shell completion

Not provided. `gh` users expect `gh optivem completion bash|zsh|fish|pwsh`.

- **How to apply:** falls out of the Cobra migration (item 3). Don't hand-roll it separately.

### 9. No `--json` machine-readable output

Less critical for a scaffold-once tool, but the final "System / Repository / Actions / …" summary is useful as structured output for CI chains that invoke this tool.

- **How to apply:** add `--json` that emits the summary (and maybe the phase/step results) as JSON on stdout instead of the human table. Keep text output on stderr so logs still work.

### 10. No positional arg for the obvious one (design call)

Five required flags is a lot. `gh repo create <name>` takes name positionally — this command probably should too.

- **How to apply:** make `<repo>` a positional: `gh optivem init page-turner --owner acme --system-name "Page Turner" --arch monolith --repo-strategy monorepo --monolith-lang java`. `--repo` stays as a deprecated alias for one release.

## Recommended order of execution

1. Item 1 (`-v` ambiguity) + item 2 (top-level `--help`). Tiny, user-visible, no breaking change.
2. Item 5 (`--yes`). Small, unblocks CI use.
3. Item 6 (auto-upgrade default). One-line change + a notice; biggest surprise to remove.
4. Item 3 (Cobra migration). The deferral trigger has fired — six subcommands now duplicate dispatch logic. Items 7, 8, and 10 fall out of this work, so do them in the same migration rather than separately.
5. Item 4 (flag-name pass). Do as one batch with deprecation warnings; ship in a dedicated release. Easier post-Cobra (uniform flag definitions).
6. Item 9 (`--json`) only if a real consumer needs it.
