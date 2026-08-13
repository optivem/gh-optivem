# 2026-08-13 08:50:00 UTC — Fix `gh optivem init` silently corrupting repos on repeated runs

## TL;DR

**Why:** GitHub issue #60 — `gh optivem init` was run twice against the same target repo (`jcupac/book-shop`), and the second run silently produced a corrupted Java `system-test` module (duplicate `testkit` classes, ~50 javac "duplicate class" errors) instead of failing. Root cause: `CreateRepo()` skips creation (not failing) when the repo already exists, and a directory-rename step later swallows an `os.Rename` error instead of failing loud, so a stale un-renamed tree from run 1 collides with the freshly-copied template from run 2.
**End result:** Re-running `gh optivem init` with the same flags against an already-scaffolded repo fails loudly and immediately at the `CreateRepo` guard, naming the repo. There is no override — re-scaffolding an existing repo is unsupported outright, since the scaffolding pipeline can corrupt output in ways beyond the one reproduced here and there's no reliable way to detect "safe to re-run" short of a full clean/reset nobody has asked for. Any remaining directory-rename failure in the scaffolding pipeline also fails loud instead of being silently discarded, so a corrupted partial scaffold can never be pushed silently again.

## Outcomes

- `gh optivem init` refuses to scaffold into a target GitHub repo that already exists, full stop — with a clear, actionable error naming the repo. No silent "skipping creation" fallthrough, and no `--force`/override flag: the existence check (`RepoExists()`, `github.go:412-425`) already gives a definitive, reliable answer, so there's nothing to "evaluate" beyond that — the fix is purely to fail instead of skip on `exists == true`.
- `renameDirs()` in the scaffolding file-ops layer no longer discards `os.Rename` errors — a destination-collision or any other rename failure aborts the scaffold with a specific file-path error instead of letting corrupted output continue downstream.
- A regression test exists proving that a second `init` run against a non-empty/already-scaffolded repo fails fast rather than producing duplicate-class output.
- Confirmed whether the same copy → placeholder-rename → replace-in-tree pipeline (`apply_template.go` / `replacements.go`) affects other language combinations (e.g. backend-lang=java, other test-lang values) — if so, the fix covers all of them since the bug is in the shared pipeline, not Java-specific.

## ▶ Next executable step (resume here)

All code, tests, and the full suite run are done and were approved for commit. Only Step 7 remains, and it is deferred (see below) — optionally run it manually against a throwaway GitHub repo (not `jcupac/book-shop`) to end-to-end confirm the second `init` run fails loudly at the `CreateRepo` guard. No further code changes are expected; this plan file is otherwise ready to be deleted once Step 7 is either run or accepted as permanently deferred.

## Steps

- [ ] Step 7: Manually verify: run `gh optivem init` with the same flags twice in a row against the same fresh target repo (a throwaway/test repo, not `jcupac/book-shop`) and confirm the second run fails loudly at the `CreateRepo` guard rather than producing corrupted output. — ⏳ Deferred: skipped to avoid creating real GitHub side effects (repo/CI) in this session; `TestCreateRepo_ExistingRepoFailsLoud` already covers the fail-loud behavior at the unit level. Run manually against a throwaway repo when convenient.

## Open questions

- Does `jcupac/book-shop` (the corrupted GitHub repo from the original bug report) need manual cleanup/deletion now that the root cause is understood? Out of scope for this plan's code fix, but worth flagging to the user separately.
