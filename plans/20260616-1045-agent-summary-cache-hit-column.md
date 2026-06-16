# 2026-06-16 10:45 UTC — Surface cache-hit % in the agent-summary table

## TL;DR

**Why:** The agent-summary table's `in` column sums fresh input **plus cache-creation plus cache reads** into one number (`driver.go:1916`, mirrored in the live exit banner `clauderun.go:1566`). On a normal run that number is ~95% cache reads — e.g. run `20260616-083529` agent #8 showed `1422.6k in` of which only **7.0k was fresh** input and **1,355.6k was cache reads**. A reader sees "1.4M in" and reasonably over-reacts, treating a cheap cache-served prefix as if it were expensive fresh context. The column hides the single most important efficiency signal of a multi-turn agent run: how much of the input was cache hits.

**End result:** The single conflated `in` column is **split into `fresh` and `cached`** so the real shape of a run is visible directly — e.g. that agent #8 row reads `67.0k fresh / 1355.6k cached` instead of `1422.6k in`. `fresh = input_tokens + cache_creation` (everything billed at ≥ full rate this turn); `cached = cache_read` (the cheap reuse). No data/schema change — `summary.jsonl` already stores `cache_read`/`cache_creation` separately, so `gh optivem run summary` replay of old runs gets the split for free.

## Outcomes

What we get out of this — the goals and deliverables:

- The `=== Agent summary ===` table (live banner totals, the `summary.md` digest, and `gh optivem run summary` replay — all three route through `renderAgentSummary`) shows `fresh` and `cached` columns per agent and on the totals row, replacing the single conflated `in`.
- The reader sees paid-for vs reused input directly (`67.0k fresh / 1355.6k cached`) with no mental math, killing the over-reaction the old `1422.6k in` invited.
- The per-dispatch live exit banner (`clauderun.go` `formatUsageTail` / `writeExitBanner`) shows the same `fresh`/`cached` split, honoring the existing "the two views read the same" design contract (`formatSummaryTokens` doc comment).
- Old runs replayed via `gh optivem run summary [ts]` render the split from their existing sidecar — no migration, no re-run.
- Single source of truth for the fresh/cached bucketing (one helper) so the table and the banner can't drift.

## ▶ Next executable step (resume here)

Mechanical, ready to start. **Open question 1 is resolved: split `in` into `fresh` + `cached`** (drop the conflated `in`). Remaining decisions (OQ 2–4) are cosmetic and can be settled inline during Step 1. Step 1 (the shared helper) is the first edit.

## Steps

- [ ] Step 1: **Shared bucketing helper.** Add one helper (e.g. `splitInputTokens(u *clauderun.TokenUsage) (fresh, cached int)`) returning `fresh = input_tokens + cache_creation_input_tokens` and `cached = cache_read_input_tokens`. Place it where both render sites can reach it without a new import cycle (driver imports clauderun, so `clauderun` is the natural home; if that pulls an unwanted dep, duplicate the one-liner as `formatSummaryTokens` already is, with a doc note). Rationale for the bucketing: `cache_creation` is billed at ≥ full rate this turn (a write), so it belongs with `fresh`, not `cached` — `cached` is strictly the cheap reuse.
- [ ] Step 2: **Table columns (`renderAgentSummary`, `driver.go:1860`).** Replace the single `in` column with `fresh` and `cached` in the header, the per-row `Fprintf`, and the totals row. Drop the `totalIn` accumulator in favor of `totalFresh` + `totalCached`. Keep the existing `formatSummaryTokens` rendering for both. Render `—` when `r.usage == nil`, consistent with the existing `out`/`cost` `—` handling. Mind the fenced-table width in `summary.md` (digest wraps the table in a ``` block — two compact token columns keep it GitHub-renderable; verify the rendered width).
- [ ] Step 3: **Live exit banner (`clauderun.go:1539` `writeExitBanner` / its `formatUsageTail` at ~1566/1571).** Replace the `, X in / Y out, $Z` tail with the split, e.g. `, 67.0k fresh / 1355.6k cached / 16.5k out, $1.72`, using the Step 1 helper so the per-dispatch banner and the summary table agree.
- [ ] Step 4: **Tests.** Update the golden-output assertions in `internal/atdd/runtime/driver/driver_test.go` (table render), `internal/atdd/runtime/driver/summary_sidecar_test.go` (replay render), and `internal/atdd/process/clauderun/clauderun_test.go` (exit-banner tail — see existing fixtures at lines ~1081, ~1346 carrying `cache_read_input_tokens`). Add cases for: a high-cache row (mostly cached), a zero-cache row (cache_read=0 → `0` cached), and a no-usage row (`—`). Scope `go test` per-package (`./internal/atdd/runtime/driver/...`, `./internal/atdd/process/clauderun/...`) — avoid unbounded `go test ./...` on Windows.
- [ ] Step 5: **Docs touch-up.** If `README.md` / `CONTRIBUTING.md` show a sample agent-summary table, refresh it to include the new column so docs don't drift from output.

## Open questions

1. ~~One `cache%` column vs. split `in` into `fresh` + `cached`?~~ **Resolved: split into `fresh` + `cached`**, drop the conflated `in`. Most explicit, no mental math, and removes the column that caused the original over-reaction.
2. **Bucketing of `cache_creation`.** Plan puts `cache_creation` in `fresh` (it's billed at ≥ full rate this turn). Alternative: a third `created` column to distinguish first-write from genuinely-fresh input — likely over-detailed; fold into `fresh` unless someone wants per-bucket visibility.
3. **Header labels.** `fresh` / `cached` (chosen). Alternatives considered: `new`/`reused`, `paid`/`cached`. Keep both ≤6 chars so the digest's fenced table stays within GitHub's render width.
4. **Banner verbosity (Step 3).** Is the `fresh / cached` split on every per-dispatch banner welcome signal or noise? If noise, scope the change to the summary table only and drop Step 3 (the table is where cross-agent comparison actually happens).
