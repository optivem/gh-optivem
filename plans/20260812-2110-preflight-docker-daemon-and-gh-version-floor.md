# 2026-08-12 21:10:00 CEST — Preflight must prove Docker is reachable and `gh` is new enough

🤖 **Picked up by agent** — `ValentinaLaptop` at `2026-08-13T06:36:32Z`

## TL;DR

**Why:** Issue [#59](https://github.com/optivem/gh-optivem/issues/59) — a scaffold of `jcupac/book-shop` passed `environment verify` green, then died six phases later at `docker compose build` because Docker Desktop was installed but **not running**. By then `init` had already created a GitHub repo, a project board, 3 environments, subtype labels, 6 secrets/variables and 3 SonarCloud projects. The same run also ran `gh` 2.42.0, too old to have `gh project link` at all — and that failure was swallowed and printed as `OK Linked`.

**End result:** `environment verify` (and therefore `init`'s pre-mutation preflight) proves Docker is actually *usable* and `gh` is new enough *before* anything is created, and a failed project-link can never report success.

## Outcomes

What we get out of this — the goals and deliverables:

- A stopped/unreachable Docker daemon fails `gh optivem environment verify` and `gh optivem init` at preflight, **before any repo, board, environment, secret or SonarCloud project is created** — with a message that names the fix (start Docker Desktop / `sudo systemctl start docker`), not a raw npipe connect error 80 seconds in.
- A missing Docker **Compose v2 plugin** is proven rather than assumed (today `tool_checks.go:386-388` reasons "compose v2 is a docker sub-command, so the `docker` binary alone is sufficient" — that holds for the plugin only if it was actually installed).
- A `gh` CLI older than the floor gh-optivem depends on fails preflight with the upgrade command, instead of surfacing much later as `unknown flag: --owner`.
- **No scaffold step ever prints `OK` for a command that failed.** `linkRepoToProject` reports the truth; a repo that did not get linked to its board says so.
- Regression tests covering all three, runnable on Windows without an unbounded `go test ./...`.

## Context — the diagnosis (do not re-derive)

Pinned root causes from the #59 log:

| # | Root cause | Pin |
|---|---|---|
| 1 | `verifyDocker` is `exec.LookPath("docker")` and nothing else — binary presence, never daemon reachability. `init` calls it as its pre-mutation fail-fast preflight, so preflight went green and the run proceeded to mutate GitHub + SonarCloud before dying. | `internal/config/tool_checks.go:389`; registered at `internal/config/token_auth.go:466`; called from `internal/config/config.go:1232`; crash lands at `internal/scaffolding/steps/verify.go:504` |
| 2 | No `gh` CLI version floor exists anywhere in the codebase. `verifyGhAuth` checks presence + auth + scopes, never version. The reporter's `gh` 2.42.0 predates `gh project link` entirely. | `internal/config/tool_checks.go:201` |
| 3 | `linkRepoToProject` passes `check=false` to `projectRun`, which swallows the error, so control falls through to `log.Successf("Linked %s to project %s")`. The log shows `WARN command failed … unknown flag: --owner` immediately followed by `OK Linked jcupac/book-shop to project`. | `internal/scaffolding/steps/project.go:411-413` → `:421` |

Reproduced locally with `DOCKER_HOST` pointed at a nonexistent npipe:

| probe | daemon down | verdict |
|---|---|---|
| `command -v docker` (today's check) | **exit 0 — PASS** | blind to the failure |
| `docker info` | exit 1, `check if the path is correct and if the daemon is running` | **the discriminator** |
| `docker compose version` | **exit 0** | proves plugin presence only — cannot substitute |

`gh project link` shipped in **gh v2.45.0** (published 2024-03-04; cli/cli PR #8595, merged 2024-02-29, cited in that release's notes). That is the version floor.

Design constraint, per the repo's fail-loud rule: a non-zero exit *or* a timeout from `docker info` is a definitive "docker is not usable right now" verdict and must fail hard. Do not coerce an indeterminate result into a pass — this is unlike `verifyClaude` / the unreported-scopes branch of `verifyGhAuth`, which warn-and-pass precisely because those questions have no provable answer. Daemon reachability does.

## ▶ Next executable step (resume here)

**Step 1** — in `internal/config/tool_checks.go`, extend `verifyDocker` (line 389) beyond `exec.LookPath`:

1. Probe the daemon with `docker info` under a bounded `context.WithTimeout` (follow the `exec.CommandContext` + `bytes.Buffer` stderr pattern at `internal/atdd/runtime/preflight/tools.go:24-38`). Use a named constant, ~20s: a healthy daemon answers in under 2s, and a Docker Desktop still booting is legitimately "not usable now".
2. Non-zero exit **or** context deadline ⇒ return an error naming the fix: start Docker Desktop (macOS/Windows) or `sudo systemctl start docker` (Linux). Include a trimmed tail of the docker output.
3. Probe `docker compose version` for Compose v2 plugin presence, and update the now-stale comment at lines 386-388 that says the binary alone is sufficient.

**No production-code seam is needed.** `internal/config/testutil_stub_binary_test.go` already provides `writeStub` / `writeStubOSSpecific` + the emptied-`PATH` helper, which plant a fake executable on `PATH` that behaves however the test wants — so a `docker` stub whose `info` exits 1 exercises the real `exec.CommandContext` path end to end. Prefer that over a package-level func var; it tests the actual subprocess wiring rather than around it.

Stop after this step and run `go test -p 2 ./internal/config/...`. Unblocks Step 2 (same file, same harness).

## Steps

- [ ] **Step 1 — Docker daemon + Compose v2 probes.** `internal/config/tool_checks.go` `verifyDocker`: add the command seam, the bounded `docker info` reachability probe, the `docker compose version` plugin probe, and the actionable failure messages. Update the stale doc comment at lines 386-388.
- [ ] **Step 2 — `gh` version floor.** `internal/config/tool_checks.go`: parse `gh version` and assert a minimum of **v2.45.0** (the release that introduced `gh project link`). Register it either inside `verifyGhAuth` or as a sibling `check` in the list at `internal/config/token_auth.go:459-467` — prefer a sibling so a version failure and an auth failure are distinguishable in the aggregated output. Fail with the upgrade command (`gh extension upgrade` is wrong here — point at the platform's gh install/upgrade path). Document the floor's provenance (PR #8595 → v2.45.0) in the comment so a future bump is traceable.
- [ ] **Step 3 — Stop the project-link swallow.** `internal/scaffolding/steps/project.go` `linkRepoToProject` (lines 411-421): stop reporting success for a failed link. Either drop `check=false` or inspect `out`/`err` before the `log.Successf`. Keep the existing `isAlreadyLinkedOutput` tolerance (lines 424-432) — "already linked" stays a legitimate no-op.
- [ ] **Step 4 — Tests.** Extend `internal/config/verify_environment_tools_test.go` (already has `TestVerifyEnvironment_DockerMissing` at line 414 using an emptied `PATH`) with: daemon-unreachable, `docker info` timeout, missing-compose-plugin, and gh-version-below-floor cases — using the existing `writeStub` harness in `internal/config/testutil_stub_binary_test.go` to plant `docker` / `gh` stubs on `PATH`. Note `writeStub` enforces that the stub body parses under **both** `cmd.exe` and `/bin/sh`; for a body that can't be written portably, use `writeStubOSSpecific` and branch on `runtime.GOOS`. Extend `internal/scaffolding/steps/project_test.go` (already stubs `"project link"` at lines 305-387) with a link-fails-must-not-report-success case.

  ⚠️ **Coordination:** as of 2026-08-12 21:10 local, `internal/config/verify_environment_tools_test.go` and `internal/config/testutil_stub_binary_test.go` have **uncommitted modifications from a concurrent agent** working plan `20260812-2107-gh-stub-sh-syntax-ci-failure.md` (gh stub bodies breaking the commit stage — same `writeStub` harness). Re-check `git status` and `git log` before touching these files; land that plan first if it is still in flight.
- [ ] **Step 5 — Verify.** `go test -p 2 ./internal/config/... ./internal/scaffolding/steps/...` — **never** unbounded `go test ./...` on Windows.
- [ ] **Step 6 — Close the loop on #59.** Once verified, propose closing the issue with a short comment naming the three fixes.

## Verification

- `go test -p 2 ./internal/config/... ./internal/scaffolding/steps/...` passes.
- Manual: stop Docker Desktop, run `gh optivem environment verify` — it must fail at preflight with the start-the-daemon message, not reach Phase 6.
- Manual (optional): confirm the aggregated `environment verify` output still lists every failing check in one pass rather than aborting on the first.

## Open questions

- **Should the Docker daemon probe be skippable in CI**, the way `verifyClaude` is via `GH_OPTIVEM_SKIP_CLAUDE_CHECK` (`tool_checks.go:300`)? GitHub-hosted Linux runners *do* have a running daemon, so the check should pass there for free — the recommendation is **no new skip flag** unless a CI job actually breaks, since every skip switch is a way for a real failure to go unnoticed. Revisit only if the scaffolding matrix goes red.
- **Does anything else in the codebase depend on a `gh` feature newer than v2.45.0?** The floor is derived from `gh project link` alone. Worth a quick scan of `internal/shell/github.go` and `internal/scaffolding/steps/*.go` for newer subcommands/flags while implementing Step 2, so the floor is set once rather than bumped after the next report.
