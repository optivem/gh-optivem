# 2026-07-01 09:28:00 UTC — Document the `.env` file mechanism in CONTRIBUTING's manual-test section

## TL;DR

**Why:** Running `CONTRIBUTING.md`'s "End-to-end manual test" (`scripts/manual-test.sh`) hit `FATAL: Verification failed ... GHCR_TOKEN: GitHub rejected GHCR_TOKEN (HTTP 401 — token expired or revoked)`. The fix was a routine credential refresh, but the CONTRIBUTING section for the manual test never mentions that `gh-optivem` already supports a portable `.env` file (`internal/config/env_file.go`, loaded at binary startup, documented in `README.md`'s "Environment Variables" section: `%AppData%\gh-optivem\.env` on Windows / `~/.config/gh-optivem/.env` on Linux-mac, or `GH_OPTIVEM_ENV_FILE` override) that lets credentials be edited without restarting a terminal. `manual-test.sh` itself never touches these tokens directly — it defers entirely to the `gh optivem` binary's own `config init`/`init`, which already reads that file — so the mechanism fully covers this exact scenario today; it's just undiscoverable from the manual-test instructions. The "single system test" section further down CONTRIBUTING.md already links it, but the manual-test section (read first, and the one that actually failed) does not.
**End result:** `CONTRIBUTING.md`'s "End-to-end manual test" section links to the existing `.env` mechanism (README's "Environment Variables" section), so the next person hitting an expired/missing credential FATAL during the manual test discovers the no-restart-needed fix immediately instead of re-deriving it.

## Outcomes

- `CONTRIBUTING.md`'s "End-to-end manual test" section (and/or the "Quick smoke test" intro right above it, wherever the credentials are first needed) includes a pointer to the `.env` file mechanism, matching the pattern already used in the "single system test" section (`## Tests`).
- No behavior change — this is a documentation-only fix; the `.env` mechanism itself is already fully implemented and already covers `manual-test.sh` (which defers entirely to the `gh optivem` binary for credential handling).

## ▶ Next executable step (resume here)

Step 1 below: edit `CONTRIBUTING.md`'s "End-to-end manual test" section (around the `bash scripts/manual-test.sh ...` example, currently line ~243) to add a one-line pointer to the `.env` file mechanism documented in `README.md#environment-variables`.

## Steps

- [ ] Step 1: In `CONTRIBUTING.md`, near the "End-to-end manual test" section's `bash scripts/manual-test.sh ...` example, add a short note that the required credentials (`DOCKERHUB_USERNAME`, `DOCKERHUB_TOKEN`, `SONAR_TOKEN`, `GHCR_TOKEN`, `WORKFLOW_TOKEN`, `REPO_TOKEN`) can be supplied via the portable `.env` file instead of OS environment variables — link to `README.md#environment-variables` rather than duplicating the full explanation, mirroring how the "single system test" section under `## Tests` already does this.
- [ ] Step 2: Check whether the top-of-file "Most-used commands" section's "End-to-end manual test" snippet (lines ~11-18, the very first mention in the file) should carry the same pointer, since it's likely the first thing a reader tries — decide whether one link (in the detailed section) is sufficient or both spots need it, to avoid the reader missing it if they only skim the top.
- [ ] Step 3: Re-read the edited section once done to confirm the note doesn't duplicate/contradict the existing "single system test" `.env` explanation, and that both consistently point at the same canonical source (`README.md#environment-variables`) rather than each explaining it slightly differently.
- [ ] Step 4: Commit and push via the `/commit` skill, scoped to the `gh-optivem` repo (per repo convention — never raw `git`).

## Open questions

- None — this is a straightforward documentation cross-reference; no code or behavior changes are involved.
