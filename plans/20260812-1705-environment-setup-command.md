# 2026-08-12 17:05:00 CEST — Replace the README's install prose with `gh optivem environment setup`

> **⏸ PENDING — TO BE DISCUSSED. Do not execute.**
> This plan is a proposal, not an approved work item. The `## Open questions` section
> below has five unresolved decisions, each with a recommendation. Discuss and resolve
> them first; only then does `## Steps` become executable.

## TL;DR

**Why:** README.md carries ~50 lines of "install this, then check it ran, then fix your PATH yourself" prose (`README.md:11-104`) for tools that `gh optivem` already knows how to detect. Every one of those checks exists in `internal/config/tool_checks.go`, each already carrying the exact install command in its failure string — but the tool only ever tells you what is missing, never fixes it. Meanwhile `scripts/readme-steps.sh` (774 lines) re-encodes the same setup as executable steps for the clean-room VM loop, so the same knowledge lives in three places: prose, check functions, and a test script.

**End result:** One command — `gh optivem environment setup` — installs everything installable, edits PATH where the upstream installers refuse to, and then prints the short list of things only a human can do (Docker Desktop, Claude sign-in, six credentials). The README's setup section collapses to: install `gh`, install the extension, run setup, do the three human steps. `readme-steps.sh` becomes a test *of that command* rather than a parallel reimplementation of the prose.

## Outcomes

What we get out of this — the goals and deliverables:

- A student on a fresh machine runs one command instead of following a checklist of eight installs, each with its own version-check incantation.
- The two PATH landmines the README currently apologises for — Claude Code's `%USERPROFILE%\.local\bin` and actionlint's `$(go env GOPATH)/bin`, both of which their installers explicitly decline to add — get handled by the tool instead of pushed onto the reader.
- `environment verify` stays exactly what it is: a read-only probe. The acceptance pipeline (`_gh-acceptance-pipeline.yml:178`) and the VM loop both depend on it never mutating the machine.
- Install knowledge stops being triplicated. The install command for each tool lives next to its check function, and both the README and `readme-steps.sh` defer to it.
- The clean-room VM loop gets shorter and starts proving the product rather than proving a script that shadows the product.

## Context

### What `environment verify` checks today

`VerifyEnvironment` (`internal/config/token_auth.go:431`, surfaced at `environment_commands.go:98`) fans out over:

| Check | Source | Installable? |
|---|---|---|
| gh CLI auth | `verifyGhAuth`, `tool_checks.go:55` | Install yes, `gh auth login` no |
| actionlint | `verifyActionlint`, `tool_checks.go:79` | Yes — `go install …@v1` |
| bash | `verifyBash`, `tool_checks.go:100` | Yes — Git for Windows |
| docker | `verifyDocker`, `tool_checks.go:231` | **No** — GUI licence accept, WSL2, reboot |
| claude | `verifyClaude`, `tool_checks.go:125` | Install yes, sign-in no |
| java / dotnet / npm | `compilerChecksFor`, gated on `--lang` | Yes |
| 5 token checks | `token_auth.go:490-497` | **No** — human at a browser |

Two gaps worth noting up front. **Go is not checked at all** — `README.md:76` lists it as a prerequisite purely because actionlint needs it, but nothing verifies it. And `verifyClaude` deliberately checks presence only (`tool_checks.go:125-136`): `claude` exposes no non-interactive auth-status command, so a signed-out machine is indistinguishable from a working one. That constraint carries straight into setup — it can install Claude Code, it cannot confirm sign-in.

### Every check already knows its own fix

The failure strings in `tool_checks.go` are install instructions:

- `"Install: https://cli.github.com/"`
- `"Install: go install github.com/rhysd/actionlint/cmd/actionlint@v1"`
- `"Install Git Bash (Windows): https://git-scm.com/download/win"`

So the data needed to drive an installer is already there, just in prose form inside error strings. The work is promoting it to a structured table, not discovering it.

### The prior art is already written, in the wrong place

`scripts/readme-steps.sh` has a fully-developed winget installer at `:242` (`winget_install`) with a PATH-free presence probe at `:237` (`package_installed`). Its comments record real failures from real guest runs:

- winget "cannot succeed" when the package is already present, so a naive install-then-check reports a false failure (`:249-253`).
- Several winget non-zero exit codes still leave the package installed, so exit status is logged but never acted on — the re-probe decides (`:279-282`).
- A freshly installed tool stays "command not found" in the shell that installed it, so PATH must be refreshed mid-run (`:144`, `:265`).

That is hard-won knowledge sitting in a test harness. Porting it into the product is most of Step 2.

### The three tiers, which is what bounds the whole plan

1. **Auto-installable, unattended** — Go, actionlint, Git Bash, Node, Java, .NET. A command does these end to end.
2. **Installable, human finishes** — gh CLI (then `gh auth login`), Claude Code (then run `claude` once and sign in).
3. **Human-only** — Docker Desktop and all six credentials.

Tier 3 is why this plan does not delete the README's setup section — it shortens it. Any framing of "the command replaces the README" is wrong and should be resisted during discussion.

### Considered and not chosen

- **Make `verify` self-healing (`verify --fix`).** One entry point is superficially tidy, but a probe that sometimes mutates is a worse contract than two commands with clear halves. `_gh-acceptance-pipeline.yml` and the VM loop both call `verify` expecting it to change nothing; a `--fix` flag puts a mutating path one typo away from CI.
- **A `curl … | bash` bootstrap script instead of a subcommand.** Would mean a third encoding of the install knowledge, hosted separately, versioned separately from the extension that depends on it. The whole point is to stop having parallel encodings.
- **Install Docker Desktop.** Silent install exists but needs a reboot, WSL2 enablement, and licence acceptance the user is legally the one accepting. Detect and instruct.
- **Do nothing; keep the long README.** Defensible if the audience is small and technical. It is not chosen because the clean-room loop keeps finding setup failures (`0101ad82` — Claude bin not on PATH) that are precisely the class of thing prose cannot fix and code can.

## Open questions

Each has a recommendation; none are settled.

1. **New `environment setup` subcommand, or a flag on `verify`?**
   *Recommended: new subcommand.* Keeps `verify` read-only for the pipeline and the VM loop, and matches the existing `gh optivem claude setup` / `gh optivem claude check` pairing — same verb split, same repo, already familiar.

2. **Which platforms in the first cut — winget only, or winget + brew + apt?**
   *Recommended: winget first, behind a platform-dispatch seam.* Windows is the only path with a clean-room harness proving it, and `readme-steps.sh` already contains a debugged winget implementation to port. macOS/Linux should return an honest "not automated on this platform yet, here are the commands" rather than an untested `brew` path nobody has run. Structure the table so adding a backend is a data change.

3. **Does setup edit PATH?**
   *Recommended: yes, for the two known cases, loudly.* Claude Code's `~/.local/bin` and Go's `$(go env GOPATH)/bin` are exactly where the README currently gives up and tells the reader to do it themselves — that is the highest-value thing the command does. It must print the before/after and the file or registry key it touched. Whether that needs a `--no-path` opt-out is a sub-question; lean no until someone asks.

4. **Does setup take `--lang`, or install every toolchain?**
   *Recommended: `--lang`, same flag and same parsing as `verify`.* Installing Java, .NET *and* Node on a machine that needs one of them is a multi-gigabyte surprise. Reusing `verify`'s flag also satisfies the house rule that interactive and flag validation stay in parity — see `internal/config/verify_flags.go`.

5. **Does `readme-steps.sh` get collapsed in this plan, or a follow-up?**
   *Recommended: in this plan, as the last step, after one green guest run.* The duplication is half the justification; leaving it un-collapsed banks the cost without the payoff. But it must not be collapsed before the new command has actually passed in a guest, or the loop loses its only working setup path.

## Steps

*(Blocked on the Open questions above. Written against the recommended answers.)*

- [ ] **Step 1 — Promote the install hints to a structured table.**
  New file: `internal/config/tool_install.go`, alongside `tool_checks.go`.
  One entry per tier-1 and tier-2 tool: probe command, human label, per-platform install spec (winget id for Windows; a documentation URL elsewhere), and an optional post-install PATH directory. Populate the install strings from the existing failure messages in `tool_checks.go:55-240` so the two agree by construction. Add Go, which `verify` does not currently check at all — setup needs it as actionlint's prerequisite, and it should be installed before actionlint is attempted.

- [ ] **Step 2 — Port the winget backend from `readme-steps.sh`.**
  Same file. Reproduce the three behaviours the script's comments document as load-bearing: the PATH-free `winget list --id <id> -e` presence probe (`readme-steps.sh:237`), treating winget's exit code as advisory and letting a re-probe decide the outcome (`:279-282`), and refreshing the process environment's PATH after an install before re-probing (`:265`). Each of those comments records a real guest-run failure; carry the *reasoning* across, not just the code.

- [ ] **Step 3 — Handle the two PATH cases explicitly.**
  Same file. After installing Claude Code, ensure `%USERPROFILE%\.local\bin` is on the user's PATH; after installing Go, ensure `$(go env GOPATH)/bin` is, before attempting the actionlint `go install`. Persist to `HKCU:\Environment` on Windows *and* update the current process env so the rest of the run sees it. Print exactly what was changed and where. Do not enumerate or log other values under that key.

- [ ] **Step 4 — Add the `environment setup` command.**
  File: `environment_commands.go`, sibling to `verify` at `:98`.
  Takes `--lang`, parsed by the same helper `verify` uses (`internal/config/verify_flags.go`). Order: install tier 1, then tier 2, then run the existing `VerifyEnvironment` to report the true remaining state. Finish with a "what's left for you" block covering only what is genuinely outstanding — Docker if absent, `gh auth login` if unauthenticated, `claude` sign-in (always, since it is unprovable — say so), and any missing credentials from `requiredEnvVars()`.

- [ ] **Step 5 — Cut the README down.**
  File: `README.md:58-104` ("Local environment setup"), with knock-on edits to `:11-47` ("Prerequisites").
  Prerequisites keeps `gh` only — it is the one thing needed before the extension exists. The setup section becomes: run `gh optivem environment setup --lang …`, then the tier-3 human steps. Delete the per-tool install blocks and both PATH apologies (`:29-31`, `:79-83`). Keep `environment verify` documented as the re-check.

- [ ] **Step 6 — Update the help text.**
  Files: `environment_commands.go` `Short`/`Long`/`Example` strings.
  `verify` and `setup` must each state plainly that one only reads and the other writes — that distinction is the entire design and it should be visible from `--help`.

- [ ] **Step 7 — Tests.**
  New: `environment_setup_test.go` (command wiring, `--lang` parity with `verify`) and `internal/config/tool_install_test.go` (table completeness; every tier-1/2 entry has a probe and a Windows spec; the already-installed path is a no-op; a non-zero winget exit with a passing re-probe counts as success).
  Run scoped — `go test ./internal/config/` — never an unbounded `go test ./...`.

- [ ] **Step 8 — Collapse `scripts/readme-steps.sh` onto the new command.** *(Only after a green guest run — see Verification.)*
  Replace the `ensure_<tool>_installed` family and `winget_install` with a call to `gh optivem environment setup`, keeping the script's logging and its closing inventory so a guest run still reports what ended up installed. The script's job becomes proving the command works on a clean machine, not performing the setup itself.

## Verification

- Run the clean-room VM loop end to end on a fresh guest against Steps 1-7, before Step 8 is attempted. The pass condition is that `gh optivem environment setup` leaves a machine where `gh optivem environment verify` reports only the tier-3 items (Docker, credentials) and the unprovable Claude sign-in.
- Confirm on that guest that `claude` and `actionlint` both run from a **newly opened** shell — that is the specific regression `0101ad82` fixed and the one Step 3 is claiming to make impossible.
- Re-run the loop after Step 8 and confirm the collapsed script reaches the same closing inventory as before.
- Confirm `environment verify` is still byte-for-byte non-mutating: run it twice on a guest with a deliberately missing tool and check nothing was installed and PATH is unchanged.
