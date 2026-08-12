# 2026-08-12 15:39:00 CEST — Put the Claude Code bin directory on PATH, in the script and in the README

## TL;DR

**Why:** The Windows Claude Code installer (`irm https://claude.ai/install.ps1 | iex`) drops `claude.exe` in `%USERPROFILE%\.local\bin` and does **not** add that directory to `PATH` — it prints a warning telling you to add it by hand. `scripts/readme-steps.sh` assumes the opposite: it calls `refresh_path`, which re-reads `PATH` from the registry, and then fails the run because `claude --version` still does not resolve. `README.md:35` makes the same wrong assumption in prose, so a real Windows reader hits the identical wall.
**End result:** `readme-steps.sh` walks past the Claude Code install on a clean Windows guest, and the README tells a Windows reader the truth about where the binary lands and that they have to put it on `PATH` themselves — the same shape the README already uses for `actionlint`.

## Outcomes

What we get out of this — the goals and deliverables:

- A clean-room guest run of `scripts/readme-steps.sh` gets past `[machine setup: Claude Code install]` and reaches the sign-in step, instead of exiting 1 on a tool it just successfully installed.
- A Windows reader following `README.md` on their own machine knows that `install.ps1` does not touch `PATH`, where the binary actually is, and what to do about it — before they run `claude --version` and see it fail.
- The script handles the Claude Code bin directory the same way it already handles the Go bin directory, so "what does this file do about PATH for tool X" has one answer shape rather than two.
- When the Claude Code probe does fail after this change, the error names the directory that was searched, so the next diagnosis starts from a location rather than from "it does not run".

## ▶ Next executable step (resume here)

Step 1 — edit `README.md` around line 35. The Windows install block currently ends with "Then, from a **fresh** shell — the installer writes `PATH` and the shell you ran it in keeps the copy it started with". That sentence is true for the macOS/Linux `install.sh` block below it and false for the Windows `install.ps1` block it is attached to. Split it: keep the fresh-shell note with the macOS/Linux block, and give the Windows block a note in the shape of the existing `actionlint` bullet at `README.md:80` — `install.ps1` puts `claude.exe` in `%USERPROFILE%\.local\bin` and never edits your `PATH`; add that directory to `PATH` yourself. No code runs; this is a prose edit. It unblocks Steps 2–4, which make the script mirror what the README now says.

## Steps

- [ ] Step 1: `README.md` — correct the Windows Claude Code bullet (around line 35). State that `install.ps1` installs to `%USERPROFILE%\.local\bin` and does not add it to `PATH`, and that the reader must add it themselves. Match the wording shape of the `actionlint` bullet at `README.md:80` ("… never edits your `PATH` … Add that directory to `PATH` yourself if the check comes back not found"). Keep the "from a **fresh** shell" note attached to the macOS/Linux `install.sh` block only, where it is accurate.

- [ ] Step 2: `scripts/readme-steps.sh` — add `add_claude_bin_to_path()` in the Helpers section next to `add_go_bin_to_path()` (currently lines 308–316), following its shape: append to `PATH`, `export`, `hash -r`. Build the directory deterministically as `"$(cygpath -u "$USERPROFILE")/.local/bin"`. Do **not** derive it from `$HOME`: the installer installs relative to the Windows user profile, and `$HOME` in Git Bash is a separate setting that can point elsewhere. Write the comment at the prose density the rest of the file uses, and make it carry the *why* — `install.ps1` does not write the registry `PATH`, so `refresh_path` alone can never see this directory, which is the exact failure this function exists to prevent.

- [ ] Step 3: `scripts/readme-steps.sh` — in `ensure_claude_installed`, call `add_claude_bin_to_path` between the `refresh_path` on line 458 and the re-probe on line 459. `refresh_path` stays: it is still what picks up anything the installer *did* write to the registry.

- [ ] Step 4: `scripts/readme-steps.sh:460` — rewrite the failure message so it names the directory that was searched, mirroring the `actionlint` failure message at line 549 ("actionlint was installed but is not on PATH. Add `$(go env GOPATH)/bin` to PATH and re-run."). The current text, "Claude Code was installed but 'claude --version' still does not run", sends the reader nowhere.

- [ ] Step 5: `bash -n scripts/readme-steps.sh` on the host — syntax check only, since the behaviour cannot be exercised here (see Notes).

- [ ] Step 6: Re-read `README.md`'s Prerequisites section against `scripts/readme-steps.sh`'s `machine_setup` block and confirm they still tell the same story. The script's own header states the rule: the README is its table of contents, and a claim in one that the other contradicts is a bug in this file.

## Notes

**Why the in-shell append, and not a registry write.** Persisting `%USERPROFILE%\.local\bin` into the user's registry `PATH` would also make the README's "fresh shell" instruction true for Windows. It is not the choice here: `add_go_bin_to_path` already set the precedent of an in-shell append for exactly this situation, and keeping `refresh_path` as the only thing in the file that touches the registry means there is one direction of data flow to reason about. If a later run shows that operators genuinely need `claude` on `PATH` in shells this script did not start, that is a separate change with its own plan.

**This does not reproduce on the dev host, and that is expected.** The host's `claude` is an npm install at `AppData/Roaming/npm/claude`, and `~/.local/bin` does not exist there at all. The bug is specific to a clean Windows guest that installed via `install.ps1`. Step 5 is therefore a syntax check, not a behavioural one — the real proof is the clean-room re-run under `## Verification`.

**No parallel implementations to fix.** `scripts/readme-setup.ps1` installs Git Bash and never touches Claude Code, and nothing else in the repo references `.local/bin`.

## Verification

- Operator: restore the VM checkpoint, run `scripts/vm-scripts-copy.ps1`, then `bash scripts/readme-steps.sh` in the guest. Confirm the log shows `machine setup: Claude Code installed - <version>` after the install and moves on to `[machine setup: Claude Code sign-in [interactive, off-log]]`, rather than exiting 1 as in `.vm-logs/20260812-153052-readme-steps.log`.
- Operator: on that same run, confirm the closing `[inventory: after]` block reports a version for Claude Code where the opening `[inventory: before]` block said `NOT installed`.
