# 2026-08-13 08:50:00 UTC — Fix `gh optivem init` silently corrupting repos on repeated runs

## TL;DR

**Why:** GitHub issue #60 — `gh optivem init` was run twice against the same target repo (`jcupac/book-shop`), and the second run silently produced a corrupted Java `system-test` module (duplicate `testkit` classes, ~50 javac "duplicate class" errors) instead of failing. Root cause: `CreateRepo()` skips creation (not failing) when the repo already exists, and a directory-rename step later swallows an `os.Rename` error instead of failing loud, so a stale un-renamed tree from run 1 collides with the freshly-copied template from run 2.
**End result:** Re-running `gh optivem init` with the same flags against an already-scaffolded repo fails loudly and immediately at the `CreateRepo` guard, naming the repo and requiring explicit opt-in to proceed. Any remaining directory-rename failure in the scaffolding pipeline also fails loud instead of being silently discarded, so a corrupted partial scaffold can never be pushed silently again.

## Outcomes

- `gh optivem init` refuses to scaffold into a target GitHub repo that already exists, with a clear, actionable error naming the repo — no silent "skipping creation" fallthrough.
- Re-scaffolding an existing repo requires an explicit opt-in (e.g. `--force`) if that capability is wanted at all; default behavior is to fail.
- `renameDirs()` in the scaffolding file-ops layer no longer discards `os.Rename` errors — a destination-collision or any other rename failure aborts the scaffold with a specific file-path error instead of letting corrupted output continue downstream.
- A regression test exists proving that a second `init` run against a non-empty/already-scaffolded repo fails fast rather than producing duplicate-class output.
- Confirmed whether the same copy → placeholder-rename → replace-in-tree pipeline (`apply_template.go` / `replacements.go`) affects other language combinations (e.g. backend-lang=java, other test-lang values) — if so, the fix covers all of them since the bug is in the shared pipeline, not Java-specific.

## ▶ Next executable step (resume here)

Step 1: In `internal/kernel/shell/github.go`, change `GitHub.CreateRepo()` (~line 427-438) so that when `exists` is true, it fails loud (return an error / `log.Fatalf`) with a message naming the repo, instead of `log.Warnf("Repository %s already exists -- skipping creation")` + silent return. Decide and implement whether an explicit `--force` opt-in should be supported to allow intentional re-scaffolding, or whether re-scaffolding an existing repo is simply unsupported for now (recommended: unsupported for now — smallest safe fix; add `--force` later only if a real use case shows up).

## Steps

- [ ] Step 1: Fix `CreateRepo()` in `internal/kernel/shell/github.go` (~line 427-438) to fail loud when the target repo already exists, instead of silently skipping creation. Message must name the repo and the fact that re-scaffolding an existing repo is not supported (or requires `--force`, if implemented).
- [ ] Step 2: Fix `renameDirs()` in `internal/scaffolding/files/files.go` (~line 164-188, the swallowed error at line 183) to propagate/fail loud on any `os.Rename` error (e.g. destination-already-exists collision), naming source and destination paths, instead of `if err == nil { count++ }` silently continuing.
- [ ] Step 3: Trace callers of `renameDirs()` (`internal/scaffolding/steps/replacements.go` ~line 120-141, `applyPlaceholderPairs`) to confirm the propagated error surfaces as a scaffold-abort (not swallowed again one level up).
- [ ] Step 4: Check whether `internal/scaffolding/steps/apply_template.go` (`copySystemTests`, ~line 141-161) and `internal/scaffolding/files/files.go` (`CopyDir`, ~line 211-238) are used for other language combinations (backend-lang=java, other test-lang values) beyond the Java test-lang path that reproduced this bug — if the same copy+rename+replace pipeline is shared, no additional per-language fix should be needed once Steps 1-2 land, but confirm via a quick read/test rather than assuming.
- [ ] Step 5: Add/update a Go test covering `CreateRepo()`'s existing-repo behavior (fails loud, correct error message) in the `internal/kernel/shell` test suite.
- [ ] Step 6: Add/update a Go test covering `renameDirs()`'s error-propagation behavior (destination collision surfaces as an error, not swallowed) in the `internal/scaffolding/files` test suite.
- [ ] Step 7: Manually verify: run `gh optivem init` with the same flags twice in a row against the same fresh target repo (a throwaway/test repo, not `jcupac/book-shop`) and confirm the second run fails loudly at the `CreateRepo` guard rather than producing corrupted output.
- [ ] Step 8: Run the full Go test suite to confirm no regressions.

## Open questions

- Should `--force` (explicit re-scaffold opt-in) be implemented now, or is failing outright sufficient until a real need for re-scaffolding surfaces? (Recommended: fail outright for now — smallest safe fix, avoids designing a cleanup/reset semantics no one has asked for yet.)
- Does `jcupac/book-shop` (the corrupted GitHub repo from the original bug report) need manual cleanup/deletion now that the root cause is understood? Out of scope for this plan's code fix, but worth flagging to the user separately.
