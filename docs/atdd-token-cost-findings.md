# ATDD workflow token-cost findings

Measured from existing headless-dispatch logs (`.gh-optivem/runs/<ts>/NNN-<agent>.events.jsonl`, terminal `result` event carries `usage`, `total_cost_usd`, `num_turns`, `duration_ms`; agent is in the file name, model in the `system/init` event). Sample: 52 dispatches across 10 rehearsal runs (2026-06-29 to 2026-07-08), total USD 58.98. Per-dispatch fields are sufficient; no logging change is needed.

## Results

| Agent | Dispatches | Cost share | USD / dispatch | Turns / dispatch |
|---|---|---|---|---|
| system-implementer | 12 | 41% | 2.02 | 36 |
| dsl-implementer | 10 | 25% | 1.46 | 44 |
| acceptance-test-writer | 9 | 14% | 0.92 | 27 |
| system-driver-adapter-implementer | 10 | 10% | 0.59 | 19 |
| contract-test-writer | 4 | 5% | 0.79 | 27 |
| external-system-driver-adapter-implementer | 3 | 3% | 0.64 | 25 |
| stub-fidelity-test-writer | 4 | 1% | 0.19 | 3 |

- Cost is driven by turns per dispatch (cache-read of the growing context), not by dispatch count: cache hit rate is 95-98% for every agent. Cold-start cache creation is small next to cache reads (about 0.4M tokens created vs 14-30M read per agent type).
- 12 of 52 dispatches are repeats of the same agent within a run (fixer/retry paths), costing USD 10.40 (about 18%).
- Tool mix (1452 calls): Read 512, Bash 505, Edit 263, Grep 82, Glob 39. Zero `gh issue view` calls; 21 foreground build/test shell calls, none backgrounded.
- Many-substeps structure is not itself the problem: the cheap agents (stub-fidelity, adapters) are small; spend concentrates in `system-implementer` and `dsl-implementer`.

## Decisions

- Pass issue body into the prompt (old plan item 4): rejected, agents do not fetch the issue (0 calls observed).
- Per-agent model retiering (old item 5): rejected as a plan; model is already chosen per dispatch via `resolveDispatchModel`, and the agents named in the old table no longer exist.
- Backgrounding long shell commands (old item 7): rejected; wall time in a foreground command costs no tokens, and polling adds turns.
- `--no-update-check` on dispatch: invalid, Claude CLI rejects the flag under `-p`.
- Worth measuring further if cost matters: turn count of `system-implementer` and `dsl-implementer` (exploration reads, fixer repeats) and static gates that avoid repeat dispatches.
