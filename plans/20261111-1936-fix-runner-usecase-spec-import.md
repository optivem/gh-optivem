# 2026-11-11 18:36:33 UTC — Fix runner/usecase.go import-guard break (drop internal/spec import)

## TL;DR

**Why:** Commit 1a9edb95 (`system-test run --use-case`) made `internal/build/runner/usecase.go` import `internal/spec`. That breaks `TestBuildImportsKernelOnly`, because `internal/build/**` is a shared bottom-layer module and may import only `internal/kernel/**` or `internal/build/**`. As a result gh-acceptance-stage run 43075882701 went red at Commit Stage › Unit tests. The failure reproduces locally.
**End result:** `runner.ResolveUseCaseSelection` takes plain `map[string][]string` data instead of `[]spec.UseCase`. The runner no longer imports `internal/spec`. The caller in `test_commands.go` converts the spec use cases into the map. The import guard is green and `--use-case` behaves exactly as before.

## Outcomes

- `TestBuildImportsKernelOnly` passes. `internal/build/**` again imports only kernel/build packages.
- `--use-case` behaves exactly as before: unknown use cases are still rejected, and Java criterion expansion still combines the spec's criteria with the tags found in the test sources.
- The Commit Stage of gh-acceptance-stage goes green again.

## ▶ Next executable step (resume here)

Step 1: in `internal/build/runner/usecase.go`, change the signature at line 94 to `ResolveUseCaseSelection(raw []string, lang string, declared map[string][]string, testsDir string)`. `declared` maps a use-case ID to its spec criterion IDs. Replace the `known` map built at about line 104 with lookups into `declared`: membership check for unknown use cases, and `declared[id]` for the Java criterion set in place of `known[id].Criteria`. Remove the `internal/spec` import (line 13) and update the doc comment. Then do Steps 2–3 and verify (Step 4).

## Steps

- [ ] Step 1: `internal/build/runner/usecase.go`: switch `ResolveUseCaseSelection` to take `declared map[string][]string` (use-case ID → spec criterion IDs). Drop the `spec` import and update the doc comment (lines 84–93).
- [ ] Step 2: `test_commands.go` `resolveUseCaseSelection` (about lines 200–204): convert the `[]spec.UseCase` from `loadSpecUseCases(".")` into the map, skipping blank IDs as the old loop did, before calling `runner.ResolveUseCaseSelection`.
- [ ] Step 3: `internal/build/runner/usecase_test.go`: change the `specUseCases()` helper (line 42) to return `map[string][]string` and drop the `spec` import. Leave the call sites unchanged except where they relied on the slice type.
- [ ] Step 4: Verify with `go test -p 2 ./internal/build/...` (includes `TestBuildImportsKernelOnly`) and `go test -p 2 -run 'UseCase' .` for the root package. Never run unbounded `go test ./...` on Windows.
