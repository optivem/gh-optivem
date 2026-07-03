# 2026-07-03 10:03:00 UTC — dsl-implementer must add optional/backward-compatible DTO fields (rehearsal #72)

## Target state

**End result:** `scripts/atdd-rehearsal.sh` deletes the exercised stack's legacy test tree from the throwaway rehearsal worktree immediately after worktree creation, before `implement` ever runs — with no restore step, since the whole worktree is already disposed of at cleanup regardless of outcome (existing behavior). Every rehearsal therefore behaves as if legacy never existed: no phase, including `dsl-implementer`, can ever hit a `STOP_SCOPE_VIOLATION` triggered by a legacy call site breaking on a DTO change. `dsl-implementer.md` is **not** touched — no new authoring rule is added, and DTO fields are designed purely on their own merit. Ticket #72's `weight` field is simply added as `required` on `ReturnsProductRequest` (and `ExtCreateProductRequest`), with no optional/default workaround.

**What's observably different:** running `bash scripts/atdd-rehearsal.sh 72 ...` no longer halts with `STOP_SCOPE_VIOLATION` — the legacy dir is simply absent from the worktree the whole run operates in.

**What's unchanged:** the shop repo's actual `main`/feature branches — legacy specs stay fully intact, compiled, and tested for real everywhere else (course material, other CI); only the disposable per-rehearsal worktree copy has them removed, and that copy is discarded at cleanup either way. Also unchanged (per prior decision, not reopened): BPMN self-healing/auto-approval for non-ESCC scope exceptions at `STOP_SCOPE_VIOLATION` (deferred by plan `20260622-2002-escc-misroute-and-dsl-authoring-rules.md`), and `internal/atdd/process/actions/scope_exception.go` categorization logic (confirmed working as designed for this incident). No `go build ./...` rebuild is needed — this plan no longer touches any embedded prompt.

## Resolved decisions

- **Root-cause fix location:** quarantine legacy at the rehearsal-harness level (`scripts/atdd-rehearsal.sh`), not a DTO-design rule in `dsl-implementer.md`. Rationale: the prompt-rule approach requires the agent to prove a negative (enumerate every writer of a DTO and confirm none are out of scope) every time it touches a shared DTO — expensive, error-prone, and re-litigated per field. Deleting legacy from the disposable rehearsal worktree removes the failure class entirely, with no per-field judgment call ever required.
- **Scope of exclusion:** legacy is excluded only from the throwaway rehearsal worktree, not from the shop repo's real build/CI — legacy specs keep compiling and running everywhere else. The rehearsal is meant to simulate a legacy-free ("new") project for the duration of the run.
- **Restore:** not needed. `atdd-rehearsal.sh` already discards the entire worktree at cleanup (delete-by-default, `REHEARSAL_CLEANUP` env override) regardless of pass/fail, so a deletion inside it needs no undo step.

## ▶ Next executable step (resume here)

Step 1: edit `scripts/atdd-rehearsal.sh` to delete the exercised stack's legacy test tree from the worktree right after worktree creation (see Steps below for exact location and per-stack paths).

## Steps

- [ ] Step 1: In `scripts/atdd-rehearsal.sh`, add a step right after `git worktree add` succeeds (~line 367) and before `implement` is dispatched (~line 404) that deletes the legacy test tree for the exercised stack from `$WORKTREE_PATH`, selected by `$CONFIG`:
  - TypeScript: `system-test/typescript/tests/legacy`
  - Java: `system-test/java/src/test/java/com/mycompany/myshop/systemtest/legacy`
  - .NET: `system-test/dotnet/SystemTests/Legacy`
  No restore step — the worktree (and this deletion inside it) is discarded at cleanup either way.
- [ ] Step 2: Ticket #72's actual DTO change: keep/add `weight: string;` (required, no `?`, no default) on `ReturnsProductRequest.ts` and `ExtCreateProductRequest.ts` — no optional/default workaround needed.
- [ ] Step 3: Re-run the rehearsal — `bash scripts/atdd-rehearsal.sh 72 ...` — and confirm: the legacy dir is absent from the created worktree, dsl-implementer's DTO change compiles cleanly, and no `STOP_SCOPE_VIOLATION` halt occurs.
