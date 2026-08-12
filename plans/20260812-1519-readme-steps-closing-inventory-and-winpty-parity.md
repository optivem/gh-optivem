# 2026-08-12 15:19 CEST — readme-steps: closing inventory, and winpty parity on the GitHub sign-in

## TL;DR

**Why:** `scripts/readme-steps.sh` opens with an inventory that prints `NOT installed` for every tool the guest lacks, and never prints a matching "here is what it looks like now" block — so a reader is left with a list of absences and no resolution. Separately, `ensure_claude_signed_in` wraps its TUI in `winpty` and `ensure_gh_signed_in` does not, so `gh auth login` has no console to draw on when the script is run straight from Git Bash/mintty.
**End result:** the log carries a matched before/after pair of inventory blocks around everything that installs, and both interactive sign-ins get the same console treatment.

## Not what this fixes

The run in `.vm-logs/20260812-151634-readme-steps.log` ended with:

```
readme-steps: INTERRUPTED by SIGINT during [machine setup: GitHub CLI sign-in [interactive, off-log]]. Nothing after that step ran.
```

That was **not** a script defect and nothing in this plan addresses it. The trap at `scripts/readme-steps.sh:85` sets `signal=INT` only on a real SIGINT, and the log has no `machine setup: gh auth login exited N` line (`scripts/readme-steps.sh:392`) — so bash took the signal while `gh auth login` was still the live foreground child, which is a console Ctrl+C reaching the whole process group. The verdict machinery reported it correctly. Item 2 below touches the same function, but it is a latent bug for a different launch path; it would not have prevented the interrupt.

## Outcomes

- A run's log shows the guest's tool state **twice** — once before anything is installed, once after everything that installs has run — so every `NOT installed` line in the opening block is answered by a version string in the closing one.
- The two inventory passes are distinguishable in the log and in the `readme-steps: [...]` step banners, so neither can be mistaken for the other.
- `probe_tool` stays the single probe implementation; the closing pass cannot disagree with the opening one or with the check that gates an install.
- The closing pass reports only — it never exits, exactly like the opening one.
- `gh auth login` draws its prompts when `scripts/readme-steps.sh` is run directly from Git Bash under mintty, not only when launched through `readme-setup.ps1` in a Windows console.
- The winpty explanation lives in one place rather than being duplicated across both sign-in functions.

## ▶ Next executable step (resume here)

Step 1: in `scripts/readme-steps.sh`, change `inventory()` (currently `scripts/readme-steps.sh:309-323`) to take a label argument that drives both the `step` banner and the header line, then add the second call in the Run block immediately after `readme_local_environment_setup` (`scripts/readme-steps.sh:688`). Keep the `|| true` on every probe in both passes. Gate: `bash -n scripts/readme-steps.sh` passes.

## Steps

- [ ] Step 1: Give `inventory()` a label parameter. `inventory 'before'` / `inventory 'after'` (or equivalent wording) must drive both the `step` banner and the `readme-steps: what this guest ...` header line, so the two blocks are told apart in the log. Keep the body calling `probe_tool` — do not reimplement probing — and keep `|| true` on every probe so neither pass can exit.
- [ ] Step 2: Add the closing call in the Run block at the bottom of the file, immediately **after** `readme_local_environment_setup` (`scripts/readme-steps.sh:688`) — that is the last function in the file that installs anything. Update the existing opening call to pass the `before` label.
- [ ] Step 3: Update the inventory block comment (`scripts/readme-steps.sh:293-307`) so it describes both passes rather than only the opening one. It should state why the closing pass sits where it does (after the last installer, before the README verification steps) and keep the existing "nothing here exits" note applying to both.
- [ ] Step 4: Update the Run block comment (`scripts/readme-steps.sh:670-673`). It currently says the list is README's table of contents "plus the one line that has no README counterpart" — there are now two such lines.
- [ ] Step 5: Wrap `gh auth login` in `winpty` when available, in `ensure_gh_signed_in` (`scripts/readme-steps.sh:391`), mirroring `ensure_claude_signed_in` (`scripts/readme-steps.sh:432-436`). Preserve the existing `>&3 2>&4` redirection, the `login_rc` capture, the `machine setup: gh auth login exited $login_rc` log line, and the non-zero exit.
- [ ] Step 6: Factor the winpty explanation to one place rather than repeating the prose in both sign-in functions — a shared helper carrying the comment, or one comment the second site points at. Say in the comment that `require_tty` (`scripts/readme-steps.sh:165`) does not catch this, because `-t 0` is true under mintty.
- [ ] Step 7: `bash -n scripts/readme-steps.sh` passes.

## Notes

- **A run that stops inside `readme_local_environment_setup` gets no closing pass.** `require_tool` exits on a missing Go, Docker, Java, .NET or Node, and that is before the closing call. This is accepted, not worked around: the exit message already names the tool and the download page it wants, and probing from the EXIT trap would put a slow eleven-tool sweep in front of every failure verdict. Do not add trap-based probing.
- **Scope is `scripts/readme-steps.sh` only.** One file, bash only. No parallel implementation exists in another language.
- Per-tool "installed" lines already exist and are not being added — `probe_tool` prints them (`scripts/readme-steps.sh:200`) and `winget_install` (`:248`), `ensure_claude_installed` (`:407`) and `ensure_actionlint_installed` (`:504`) all re-probe through it. What this plan adds is the single comparable before/after pair, not the per-tool reporting.

## Verification

- `bash -n scripts/readme-steps.sh` passes.
- Operator: a guest re-run from a restored clean baseline — `scripts/vm-checkpoint-restore.ps1` → `scripts/vm-scripts-copy.ps1` → `powershell -ExecutionPolicy Bypass -File C:\Users\Public\readme-setup.ps1 -Run` — whose log shows the opening inventory, the installs, and a closing inventory in which the tools installed during the run now report a version rather than `NOT installed`.
- Operator: run `scripts/readme-steps.sh` directly from a Git Bash (mintty) window and confirm `gh auth login` draws its prompts.
