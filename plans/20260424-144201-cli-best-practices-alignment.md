# CLI best-practices alignment for `gh optivem init`

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

## Fix — high priority

### 1. `-v` means two different things

[main.go:58](../main.go#L58) treats `-v` as `--version` before the subcommand, but [config.go:434](../internal/config/config.go#L434) registers `-v` as short for `--verbose`. So `gh optivem -v` prints version while `gh optivem init -v` enables verbose. Confusing and accidental.

- **How to apply:** pick one of these:
  - (a) Drop the `-v` alias for `--version`; keep `-v` for `--verbose` only. `--version` stays as the only way to print version.
  - (b) Use `-V` for version, `-v` for verbose (POSIX-ish convention).
- **Recommended:** (a). Simpler, matches how most CLIs behave (`git --version` has no short form either).

### 2. Top-level `--help` / `-h` is not handled

`gh optivem --help` hits the switch in [main.go:71](../main.go#L71) and prints `Unknown command: --help`. Stdlib `flag` only wires `-h`/`--help` after `flag.Parse` on a real subcommand.

- **How to apply:** in `main()`, before the subcommand switch, match `--help`/`-h` and call `printUsage()` with exit 0 (not 1). Also handle `help` as a bare word if desired.

## Fix — medium priority

### 3. Stdlib `flag` is a ceiling

Hand-rolled subcommand dispatch, short/long pairs, and usage. Fine for one subcommand; painful once there's a second.

- **How to apply:** migrate to [spf13/cobra](https://github.com/spf13/cobra) (what `gh` itself uses). Buys: nested subcommands, `-h`/`--help` on every level, auto-generated `completion bash|zsh|fish|pwsh`, consistent help output, env-var binding via viper, man-page generation.
- **When:** defer until a second subcommand is actually needed. Don't do it speculatively.

### 4. Flag-name inconsistency

Mixed negation and verb choices on booleans:

- `--skip-local-tests` vs `--exclude-legacy` (skip vs exclude — same intent, two words)
- `--no-commit-on-failure`, `--no-auto-upgrade` (use `no-`) vs `--keep-local`, `--dry-run` (don't)
- `--report-bug` (verb-noun) vs `--keep-local` (verb-adj)

- **How to apply:** pick one convention and rewrite. Common rule: default is the positive/safe behavior, `--no-X` flips it off. So:
  - `--exclude-legacy` → keep; `--skip-local-tests` → `--no-local-tests` (or `--exclude-local-tests` for parallelism with `--exclude-legacy`).
  - `--keep-local` → `--no-cleanup-local` (or keep it as-is and deprecate other `--no-*` flags; trade-off).
- **Recommended:** unify on `--no-<thing>` for "turn off default behavior" and verb-noun for opt-in actions. Do this as a single breaking-change pass with old-flag aliases printing a deprecation warning for one release.

### 5. No `--yes`/`--force` for unattended runs

[confirmRepoExists](../internal/config/config.go#L685) prompts `[y/N]` and aborts on non-TTY. Running against a pre-existing repo in CI is impossible without stdin tricks.

- **How to apply:** add `--yes`/`-y` that skips all interactive confirms (including the `--report-bug` confirmation). Document that `--yes` on CI is the expected pattern.

### 6. Auto-upgrade mid-run is unusual (design call)

[checkForUpdate](../main.go#L501-L548) silently upgrades and re-execs with the user's original args. Clever but surprising; most CLIs (including `gh` itself for extensions) notify and let the user decide.

- **How to apply:** flip the default to "notify only". Keep auto-upgrade behind an opt-in flag (e.g. `--auto-upgrade`) instead of opt-out (`--no-auto-upgrade`).
- **Trade-off:** if the UX goal is "always run the latest so bug reports are actionable", keep the current behavior — but at least print a one-line notice before the upgrade starts ("Upgrading to X.Y.Z... re-run with --no-auto-upgrade to skip").

## Fix — low priority

### 7. `printUsage` is thin

[main.go:83-89](../main.go#L83-L89) lists `init` with no example and no hint that `init --help` shows flags.

- **How to apply:** add one concrete example and a "Run `gh optivem <command> --help` for command-specific flags." footer.

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
4. Item 4 (flag-name pass). Do as one batch with deprecation warnings; ship in a dedicated release.
5. Item 3 (Cobra migration) only when a second subcommand is actually needed — at that point items 7, 8, 10 come along for free.
6. Item 9 (`--json`) only if a real consumer needs it.
