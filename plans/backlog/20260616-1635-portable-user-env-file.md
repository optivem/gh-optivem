# Portable user-level .env file + committed sample template

## TL;DR

**Why:** Credentials (`SONAR_TOKEN`, `DOCKERHUB_*`, `GHCR_TOKEN`,
`WORKFLOW_TOKEN`, `REPO_TOKEN`) live only in the OS environment, read via
`os.Getenv` (`internal/config/config.go` `readEnvTokens`). On Windows this
forces the operator to set six User env vars and **fully restart the
terminal / VS Code** before any already-open shell sees them (a snapshot is
taken at process launch). There is no portable way to carry the set across
machines, and the only `.env` loader in the tree is test-only
(`internal/config/config_system_test.go` `loadEnvFile`).

**End result:** `gh optivem` loads a user-level `.env` file at startup
(values never override a real exported env var). One file, copy-pasteable
across computers, shell-agnostic — no terminal restart needed after editing
it. A committed `.env.example` documents every required var as a template.

## Background

- `internal/config/config.go` `readEnvTokens()` + the new
  `requiredEnvVars()` / `MissingRequiredEnvVars()` (added 2026-06-16) are the
  single source of which credentials are required.
- The full-credential presence check now folds into `preflight.Run` via
  `opts.MissingEnvVars` (`preflight_helpers.go`), so once a `.env` is loaded
  at startup, preflight automatically sees the loaded values.
- The test loader `loadEnvFile` already encodes the right precedence:
  **existing env wins**, file fills gaps only. Reuse that semantics.
- Startup hook point: `main.main()` after the `--version` short-circuit and
  before `newRootCmd().Execute()` (`main.go:78-100`).

## Open decisions (resolve before executing)

1. **File location.** Recommendation: `GH_OPTIVEM_ENV_FILE` override (an
   explicit absolute path, ideal for a synced folder) → else
   `os.UserConfigDir()/gh-optivem/.env` (on Windows `%AppData%\gh-optivem\.env`,
   on Linux/mac `~/.config/gh-optivem/.env`). Stable logical path on every
   machine; copy the file there per machine, or point the override at
   Dropbox/OneDrive.
   - Alternative: home dotfile `~/.gh-optivem.env`.
2. **Should a project-local `./.env` also be auto-loaded** (lower priority
   than real env, higher/lower than the user file)? Risk: a `.env` committed
   or left in a repo silently injecting creds. Recommendation: **no** — only
   the user-level file + the explicit override, to keep secrets out of repos.
3. **`.env.example` location + scope.** Recommendation: repo root
   `.env.example` listing all six vars with placeholder values and a one-line
   comment each (mirror the hints in `missingEnvHint`). Confirm it must NOT
   shadow the real `.env` already gitignored for tests.

## Steps

1. **Add a production env-file loader.** Promote the test-only `loadEnvFile`
   semantics into the `config` package (e.g. `config.LoadEnvFile(path)
   (loaded int, err error)`): skip blank/`#` lines, optional `export `
   prefix, `key=value` via `strings.Cut`, trim surrounding quotes, `os.Setenv`
   only when `os.Getenv(k) == ""`. Unit-test precedence + quote/comment
   handling.

2. **Add path resolution.** `config.UserEnvFilePath()`:
   `GH_OPTIVEM_ENV_FILE` if set, else `os.UserConfigDir()/gh-optivem/.env`.
   Missing file is a silent no-op (not every operator uses one).

3. **Hook into startup.** Call the loader in `main.main()` right after the
   `--version` block, before `Execute()`. Keep it side-effect-free on
   `--version` (already returns early). Emit nothing on the happy path; a
   malformed line is at most a `log.Warn` (log isn't initialised this early —
   consider deferring the notice or writing to stderr directly).

4. **Commit `.env.example`.** Root-level template with all six required vars,
   placeholder values, and a comment per var pointing at where to get the
   token (reuse `missingEnvHint` wording). Verify `.gitignore` ignores
   `.env` but NOT `.env.example`.

5. **Docs.** README + CONTRIBUTING: document the user-level `.env`, the
   `GH_OPTIVEM_ENV_FILE` override, the "real env wins" precedence, and the
   "copy `.env.example` → fill in → drop at the user path, no restart needed"
   workflow. Cross-reference the rehearsal scripts (`scripts/atdd-rehearsal*.sh`)
   which currently rely on the ambient shell environment.

6. **Update help text** for any command whose `--help` mentions credential
   setup, if applicable (run the `help-text-updater` pass).

## Verification

- New `config` loader unit tests (precedence, quotes, comments, missing file).
- Manual: drop a `.env` at the resolved path with `SONAR_TOKEN` only, run
  `gh optivem config preflight` in a shell that has none of the vars exported
  → preflight should see SONAR_TOKEN as present and report only the other
  five as missing (proves load + precedence + the 2026-06-16 aggregated check
  interoperate).
- Manual: export `SONAR_TOKEN=real` in the shell, put a different value in the
  `.env` → `gh optivem config show-environment` (or equivalent) confirms the
  exported value wins.
