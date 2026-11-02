# gh-optivem plan backlog

The backlog is grouped by priority. Within each folder, the order to work in and the cross-plan dependencies live in `plans/20261029-2142-meta-backlog-consolidation.md`: its execution units, waves and conflicts tables.

| Folder | Theme | Notes |
|---|---|---|
| `p1-correctness/` | False halts, silent passes, broken shipped commands | Do these first. #22 → #27+#28 share `acceptance-test-writer.md` |
| `p2-atdd-quality/` | ATDD output quality and pipeline robustness | #24 L1 + #31 share the retry-engine change (do them in one session) |
| `p3-tooling-cost/` | Token, latency and tooling cost; run-artifact persistence | Measure with #43 before optimising (#1 remainder) |
| `p4-maintenance/` | Badges, docs, Sonar debt, shop-first renames | #3 is blocked until shop renames `frontend-react` |
| `parked/` | Decision records, archive specs, undecided direction | Not scheduled. Revisit only on a trigger noted in each plan |

## Shop dependency

Shop dependency cuts across priorities, so it is tracked here rather than as a separate folder:

- **shop-first** (shop must change and merge first): #3, #18, #42 Slice 3.
- **Edits shop:** #13, #16, #21, #32, #44, #42 Step 4. Commit with `--repo shop`, and check the active shop cloud-deploy plan before pushing to shop `main`.
- **Needs a synced `../shop` clone:** #45, #36, #41.
- **Verify-only rehearsal on shop:** #6, #9, #10, #12, #15, #22, #27, #28, #35, #43. Only one session at a time may run a rehearsal or `system start`, because both use fixed docker ports.

The `#N` numbers refer to the per-plan table in the meta-plan.
