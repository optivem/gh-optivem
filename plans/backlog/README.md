# gh-optivem plan backlog

The backlog is grouped by priority. The open items, attended-verification checklist, live conflicts and wave order live in `plans/20261029-2142-meta-backlog-consolidation.md`.

| Folder | Theme | Plans |
|---|---|---|
| `p1-correctness/` | False halts, silent passes, broken shipped commands | Empty (all shipped; remaining verification is in the meta-plan). Git does not track empty dirs, so the folder is absent. |
| `p2-atdd-quality/` | ATDD output quality and pipeline robustness | failure-capture-head-window, view-product-list-erp-list-gap, no-order-persisted-fanout-prevention, atdd-dispatch-retry-budget-and-resume, environment-setup-command, optional-inner-tdd-loop-per-component |
| `p3-tooling-cost/` | Token, latency and tooling cost | rehearsal-corpus-testinstance-and-refine, unattended-plan-execution |
| `p4-maintenance/` | Docs, Sonar debt, shop-first renames | shop-frontend-react-to-typescript-rename (blocked until shop renames `frontend-react`), sonar-deferred (test-file findings need a human Won't Fix, then delete the plan) |
| `parked/` | Decision records, archive specs, undecided direction | Not scheduled. Revisit only on a trigger noted in each plan |

## Shop dependency

Shop dependency cuts across priorities, so it is tracked here rather than as a separate folder:

- **shop-first** (shop must change and merge first): `p4-maintenance/20260511-1702-shop-frontend-react-to-typescript-rename.md`, `parked/20260618-1046-microservices-scaffold-pipeline.md`, Slice 3 of `p2-atdd-quality/20261023-0959-optional-inner-tdd-loop-per-component.md`.
- **Edits shop:** `p2-atdd-quality/20260613-1950-view-product-list-erp-list-gap.md`, `p3-tooling-cost/20260617-1001-rehearsal-corpus-testinstance-and-refine.md`, `p2-atdd-quality/20260620-0806-no-order-persisted-fanout-prevention.md`, Step 4 of the optional-inner-tdd-loop plan. Commit with `--repo shop`, and check the active shop cloud-deploy plan before pushing to shop `main`.
- **Needs a synced `../shop` clone:** `parked/20261015-2108-spec-driven-positioning-and-branding.md`.
- **Verify-only rehearsal on shop:** `p2-atdd-quality/20260608-2109-failure-capture-head-window.md`, `p2-atdd-quality/20260620-0806-no-order-persisted-fanout-prevention.md`, `parked/20260818-1658-contract-driven-outer-loop-for-kind-component.md`. Only one session at a time may run a rehearsal or `system start`, because both use fixed docker ports.
