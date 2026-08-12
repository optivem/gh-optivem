# 2026-08-12 17:12:00 CEST — Assert `gh` token scopes in preflight, and grant them at login

## TL;DR

**Why:** [Issue #58](https://github.com/optivem/gh-optivem/issues/58) — `gh optivem init` created `jcupac/book-shop` on GitHub, then died one step later with `project board: list projects: … missing required scopes [read:project]`, leaving a half-scaffolded repo behind. The preflight that exists precisely to catch this (`VerifyEnvironment`, called at `internal/config/config.go:1232` "before any mutation") missed it, because `verifyGhAuth` reads only the *exit code* of `gh auth status` — which is 0 for any authenticated token, whatever its scopes. The README compounds it: Prerequisites tells the user to run bare `gh auth login`, which never requests `project`, and the `gh auth refresh -s project` fix is buried 200 lines later in "Implement a ticket".

**End result:** A token missing `project` (or `repo` / `workflow` / `read:org`) fails preflight *before* anything is created on GitHub, naming the missing scopes and the exact `gh auth refresh` command. And the README's Prerequisites block hands the user `gh auth login -s project` up front, so the common case never reaches that failure at all.

## Outcomes

What we get out of this — the goals and deliverables:

- A scope-deficient token is rejected during `Verifying environment…`, with zero GitHub side effects — no orphaned repo to clean up, no partial-scaffold commit.
- The failure message names which scopes are missing and gives the one command that fixes it, instead of gh's `[read:project]` (which is not even the scope name you grant — you grant `project`).
- A student following the README's Prerequisites verbatim ends up with a token that carries `project` from the very first `gh auth login`, so the failure is never reached.
- `gh optivem environment verify` reports the same verdict standalone, so the user can check before running `init`.
- A fine-grained PAT or `GH_TOKEN`/`GITHUB_TOKEN` env token — where the classic OAuth-scope model does not apply — is not falsely rejected; it warns and continues.

## Context

### Why the preflight let it through

`verifyGhAuth` (`internal/config/tool_checks.go:55-72`) runs `gh auth status`, retries once on a non-zero exit, and returns nil if the second attempt succeeds. It captures `CombinedOutput()` at lines 60 and 64 but uses it only to build the *failure* string — on success the output is discarded. That output contains exactly the answer that was needed:

```
  - Token scopes: 'admin:org', 'delete:packages', 'delete_repo', 'gist', 'project', 'read:packages', 'repo', 'workflow'
```

### Why the blast radius was a created repo

`init`'s step list (`main.go:397-407`) runs `Create repositories` (`main.go:403`) as step 1/5 of the Setup repository phase, and `Ensure project board` (`main.go:404`) as step 2/5. The first call that touches a project API is `findOrCreateProject`'s `gh project list` at `internal/scaffolding/steps/project.go:273` — one step too late to be harmless. Preflight is the only place that can fail before a side effect exists.

### Why `project` is unconditional

Not gated on `--no-project`: `implement`'s board tracker (`internal/atdd/runtime/tracker/github`) needs `project` too — it already emits `gh auth refresh -h github.com -s project` as its own repair hint at `github.go:845`. So the scope is a prerequisite of the tool, not of one init step, and the vocabulary is already established.

### Coordination note

`plans/20260812-1705-environment-setup-command.md` (PENDING, not approved) proposes replacing `README.md:11-104` wholesale with a `gh optivem environment setup` command, and touches `tool_checks.go` as well. This plan is deliberately small and lands first; if that plan later proceeds, its rewrite subsumes the README edit here, and the scope check added here becomes one more thing `environment setup` can act on.

## ▶ Next executable step (resume here)

Step 1: add scope assertion to `internal/config/tool_checks.go`. Extract the `Token scopes: 'a', 'b', …` line from the `gh auth status` output already captured in `verifyGhAuth` (lines 60/64), compare against the required set `{repo, workflow, project, read:org}`, and return an error naming the missing scopes plus `gh auth refresh -s <missing,…>`. Keep it inside the existing `{"gh CLI auth", verifyGhAuth}` check (`token_auth.go:463`) rather than adding a new check entry, so the parallel fan-out and its success/failure line stay as they are. Stop after the Go change compiles; README and tests follow in Steps 3–5.

## Steps

- [ ] **Step 1 — Parse and assert scopes in `verifyGhAuth`** (`internal/config/tool_checks.go`). Add a `ghTokenScopes(output string) ([]string, bool)` helper that finds the `Token scopes:` line and splits the quoted, comma-separated names; the bool reports whether the line was present. Add `requiredGhScopes = []string{"repo", "workflow", "project", "read:org"}` at package level. In `verifyGhAuth`, after the existing success path, compare and return an error listing every missing scope with a single actionable fix line (`gh auth refresh -s repo,workflow,project`). Comparison is exact-name; do not try to model scope implication (`repo` ⊃ `repo:status` etc.) — gh reports the granted names verbatim and the required set has no implied members.
- [ ] **Step 2 — Indeterminate case warns, does not block.** When the `Token scopes:` line is absent or parses to an empty set (fine-grained PAT, or a `GH_TOKEN`/`GITHUB_TOKEN` env token, where classic scopes do not apply), emit `log.Warnf` naming the scopes gh-optivem needs and return nil. Document the reasoning in the doc comment in the surrounding style, citing `verifyClaude` (`tool_checks.go:109-132`) as the in-file precedent for "report what can be proven, put the rest in the message" — blocking here would be a false negative against a valid fine-grained PAT, and the project step downstream still fails loudly if the permission really is absent.
- [ ] **Step 3 — README Prerequisites grants the scope up front** (`README.md:20-24`). Change the fresh-shell block to `gh auth login -s project` (keeping `gh --version` and `gh auth status` around it), plus one line saying why: `gh auth login` does not request `project` by default, and both `init`'s project-board step and `implement`'s board tracker need it.
- [ ] **Step 4 — Demote the late note** (`README.md:229-233`). Rewrite the `gh auth refresh -s project` paragraph in "Implement a ticket" as a short back-reference/fallback for users who already logged in without the scope, instead of introducing the scope there for the first time.
- [ ] **Step 5 — Tests** (`internal/config/verify_environment_tools_test.go`). Using the existing stub-`gh` harness (`internal/config/testutil_stub_binary_test.go`) and `stubGhAuthRetrySleep` (`verify_environment_tools_test.go:15`), cover: (a) all required scopes present → pass; (b) `project` absent from the scopes line → fail, message naming `project` and the refresh command; (c) no `Token scopes:` line at all → pass. Add direct table tests for `ghTokenScopes` covering the real gh line shape, a `Token scopes: none` variant, and a missing line.

## Verification

- `go build ./...`
- `go test ./internal/config/ -p 2` — scoped to the one package; never run unbounded `go test ./...` on Windows.
- `gh optivem environment verify` against the operator's real token (which does carry `project`) — the `gh CLI auth: valid` line must still appear, with no new warning.
