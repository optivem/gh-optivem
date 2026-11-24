# 2026-11-24 10:17 UTC — `spec status` strips nested-bracket test names and ignores not-applicable-only entries

> Comes from shop's `plans/20261110-1207-system-test-reliability-followups.md`, idea #71 (added 2026-11-24 from shop meta-plan X1). Shop never edits gh-optivem; the shop-side follow-up below waits for a gh-optivem release.

## TL;DR

**Why:** On 2026-11-17 Java `spec status` showed `UC-009.2` as **Planned** although all its tests passed. The UI runs of a parameterised test were named with an argument whose `toString()` has brackets (`... [Channel: UI, address: Address[firstName=...], ...]`). `templateSuffixRe` (`\s*\[[^\]]*\]\s*$`, `internal/spec/junit.go`) can't strip a nested bracket group, so each UI run became its own test entry. The API-suite skip (`... [Skipped: API channel was not specified]`) normalised to the bare name; its not-applicable result is dropped (`notApplicableMarker` in `addCase`), but the bare-name entry is still created with no results, and an entry with no results reads as skipped, so the criterion turned Planned. Shop worked around it (`db087f76`: `Address.toString()` has no brackets), but any other record argument hits the same trap.
**End result:** a test name whose trailing bracket group nests brackets normalises to the bare method name, and a test entry whose only results are not-applicable skips is dropped, so a green test never reads as unbuilt.

## ▶ Next executable step (resume here)

Step 1.

## Steps

- [ ] Step 1: **Strip a nested trailing bracket group.** In `NormalizeTestName` (`internal/spec/junit.go`), replace `templateSuffixRe` with a small scanner: if the trimmed name ends with `]`, walk back counting bracket depth to the matching `[` and cut there (repeat while the name still ends with `]`, so two trailing groups both go). An unbalanced name is left as it is. Then apply `paramSuffixRe` as today.
- [ ] Step 2: **Drop entries with only not-applicable results.** In `aggregator.tests()` (or when an entry is created in `addCase`), skip a `Test` that has no `Results` because every case it saw was a not-applicable skip. Keep tags/IDs merging unchanged for entries that do have results.
- [ ] Step 3: **Tests** in `internal/spec/junit_test.go`: `m() [Channel: UI, address: Address[firstName=A, lastName=B], country: US]` → `m`; three such UI passes plus one `m() [Skipped: API channel was not specified]` → one entry `m` with three passed results and the criterion Verified; a test that only has the not-applicable skip → no entry.
- [ ] Step 4: Shop follow-up after release: delete shop idea #71 (shop's `Address.toString()` workaround can stay).
