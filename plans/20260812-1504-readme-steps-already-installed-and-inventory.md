# 2026-08-12 15:04:00 CEST — Stop the README walkthrough failing on tools that are already there, and say up front what the guest has

## TL;DR

**Why:** The clean-room walkthrough died with `FAILED (exit 43) during [machine setup: GitHub CLI install]` on a guest where the GitHub CLI was already installed and working. Two defects had to line up: the probe asked a stale `PATH` and reported `NOT installed`, and winget's "already installed, no upgrade applicable" (`0x8A15002B`, low byte 43) was then treated as a fatal error by `set -euo pipefail`. The log ends up contradicting itself two lines apart — `GitHub CLI NOT installed` immediately above winget's `Found an existing package already installed`.
**End result:** The walkthrough asks the machine's real `PATH` before it probes anything, cross-checks the winget package database so "installed but not on this shell's PATH" is reported as exactly that, and lets the post-install probe — never winget's exit code — be the verdict. It also opens every run with a per-tool inventory block saying what the guest already has and at what version, so a run on a guest that was supposed to be clean is self-evident from the first screen of the log.

## Outcomes

What we get out of this — the goals and deliverables:

- A re-run on a guest that already has the GitHub CLI continues through the install step instead of exiting 43.
- The log never again says `NOT installed` on one line and `already installed` on the next: a tool that is present but missing from this shell's `PATH` is named as that case.
- Every run opens with a one-block inventory — one line per tool the walkthrough deals with, installed-with-version or not-installed — printed *before* anything is installed. On a guest that is meant to be clean, that block is the proof the checkpoint was restored.
- Genuine install failures still fail loudly. Success stays asserted by the probe, matching the rule the script already states about its own exit verdict.
- The PowerShell half (`readme-setup.ps1`) gets the same two fixes, so a Git that is present-but-off-PATH cannot reproduce this failure one layer up.

## Evidence

- `.vm-logs/20260812-145922-readme-steps.log` — the failure, and the two contradicting lines.
- `.vm-logs/20260812-145904-readme-setup.log` — PowerShell's own `Get-Command gh.exe` missed gh in the same run, which is what pins the cause to a stale inherited `PATH` rather than anything in bash.
- `.vm-logs/20260812-143430-readme-steps.log` — the 14:34 run that installed gh 2.97.0 in the first place.
- Verified read-only on the host: `winget list --id GitHub.cli -e` exits `0` when the package is installed; `winget list --id Nonexistent.Pkg -e` exits `20` (`APPINSTALLER_CLI_ERROR_NO_APPLICATIONS_FOUND`). That is a `PATH`-independent installed/absent answer.
- Not reproduced in a guest — it needs the clean-room VM. The exit-code identity was confirmed by arithmetic instead: `0x8A15002B` = `-1978335189`, and `& 0xFF` = `43`.

## Root cause

Two defects, both required:

1. **False-negative probe.** `probe_tool` (`scripts/readme-steps.sh:172`) asks `command -v`, which only sees this process's `PATH`. `refresh_path` (`scripts/readme-steps.sh:132`) exists for exactly this, but is called *only after* an install (`scripts/readme-steps.sh:201`) — never before the first probe. The run inherited a `PATH` snapshot from a PowerShell window opened at 14:31, before the 14:34 run installed gh, so gh was invisible. `scripts/readme-setup.ps1` has the same hole: it refreshes `$env:Path` only inside the Git-install branch, which was skipped ("Git Bash already installed"), so the stale `PATH` was handed straight to bash.
2. **Intolerant exit code.** With gh already installed, `winget install --id GitHub.cli` (`scripts/readme-steps.sh:199`) returns `APPINSTALLER_CLI_ERROR_UPDATE_NOT_APPLICABLE`. Under `set -euo pipefail` that aborts at line 199 — *before* the `refresh_path` + re-probe on lines 201–205 that would have found gh and made the whole step a no-op.

## ▶ Next executable step (resume here)

Step 1: in `scripts/readme-steps.sh`, call `refresh_path` once at startup — after the trap/logging preamble and before `machine_setup` runs — so every probe in the file asks the machine's real `PATH` rather than the parent shell's snapshot. Keep the existing post-install `refresh_path` calls as they are. This alone turns the failing run into a no-op run; Steps 2–4 make the log honest about why. Gate: `bash -n scripts/readme-steps.sh` passes and the startup refresh is visible in the run log before the first `machine setup:` line.

## Steps

- [ ] Step 1: `scripts/readme-steps.sh` — call `refresh_path` once at startup, before `machine_setup`, so the first probe of every tool sees the registry `PATH` and not the inherited snapshot. Keep the post-install refreshes. Note in the comment why the pre-probe refresh matters (a parent shell opened before the last install), so the call is not later read as redundant with the post-install one.
- [ ] Step 2: `scripts/readme-steps.sh` — add a `package_installed <winget-id>` helper wrapping `winget list --id <id> -e` (exit 0 = installed, non-zero = absent).
- [ ] Step 3: `scripts/readme-steps.sh` — use `package_installed` in `winget_install` (`:190`–`:206`): when the PATH probe misses but the package database says the package is installed, report that case in its own words ("installed but not on this shell's PATH") and skip the install rather than launching one that cannot succeed.
- [ ] Step 4: `scripts/readme-steps.sh` — in `winget_install`, capture winget's status (`local rc=0; winget ... || rc=$?`) instead of letting `set -e` kill the run, echo the status into the log, and let the post-install re-probe be the verdict. On re-probe failure, include the winget exit code in the existing error message. This is the file's own "Success is ASSERTED, never inferred" rule applied to installs.
- [ ] Step 5: `scripts/readme-steps.sh` — add a read-only inventory pass that runs before anything is installed and prints one line per tool the walkthrough deals with (git, winget, gh, claude, bash, go, actionlint, docker, java, dotnet, npm): installed-with-version, or not installed. Reuse `probe_tool` so there is one probe implementation in the file. The pass never exits on a miss — enforcement stays with the existing `ensure_*` / `require_*` functions. Add it to the "Run" block at the bottom so the call list still reads as the table of contents.
- [ ] Step 6: `scripts/readme-setup.ps1` — refresh `$env:Path` from Machine + User registry unconditionally near the top, not only inside the Git-install branch, so the handover to bash always passes a current `PATH`.
- [ ] Step 7: `scripts/readme-setup.ps1` — mirror Step 4 at the `winget install --id Git.Git` site: let the `Find-Bash` re-probe be the verdict instead of the hard `if ($LASTEXITCODE -ne 0) { Write-Error }`, keeping the exit code in the error text when the re-probe genuinely fails.
- [ ] Step 8: Verify — `bash -n scripts/readme-steps.sh`, and a Windows PowerShell **5.1** `ParseFile` check on `scripts/readme-setup.ps1` (the guest has 5.1 and no pwsh; the file must stay pure ASCII and 5.1-compatible — no `??`, no ternary, no em-dashes).
- [ ] Step 9: Verify — run the inventory pass on the host, where several of the tools are genuinely absent, and confirm it prints a line per tool and exits 0 without installing anything.

## Verification

Operator, in the VM — not agent work:

- Restore the `clean-baseline` checkpoint and run the full loop end to end.
- Confirm a second run on a guest that already has gh reports "already installed" and continues, rather than exiting 43.
- Confirm the inventory block appears at the top of the log, before the first install.
- Confirm the walkthrough still fails loudly when an install genuinely does not take (e.g. by pointing `winget_install` at a bogus package id once, by hand).

## Notes

- Scope is `scripts/` only — `readme-steps.sh` and `readme-setup.ps1`. No change to `README.md`: the README is the specification here, and it is not what was wrong.
- `scripts/readme-steps.sh` currently has uncommitted working-tree changes (a comment/structure reshuffle). Execute on top of them rather than reverting.
