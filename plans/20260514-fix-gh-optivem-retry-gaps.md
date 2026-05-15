# Plan: fix gh-optivem Go retry-coverage gaps

🤖 **Picked up by agent** — `Valentina_Desk` at `2026-05-15T06:28:11Z`

Date: 2026-05-14
Driven by: [`audits/20260514-external-call-retry-coverage.md`](../audits/20260514-external-call-retry-coverage.md)
Phase 6 input for: [`plans/20260514-1945-retry-mechanism-end-to-end.md`](20260514-1945-retry-mechanism-end-to-end.md)
Coordination (Lane B): [`plans/20260515-0900-retry-parallel-coordination.md`](20260515-0900-retry-parallel-coordination.md) — read first, stamp pickup marker before Item 1.

## Status (2026-05-15)

**Items 1-8 shipped.** Item 9 paused on the open `MustRunStdinWithRetry` vs new-`RunStdinWithRetry` decision. Item 10 stays deferred per its own "Lowest priority" footer.

## Goal

Eliminate every `R-MISSING` finding from the retry-coverage audit. After this
plan lands, every external-I/O call site in `gh-optivem`'s Go code is either:

- Wrapped in `shell.RetryWithPolicy` / `shell.RunWithRetry` /
  `shell.RunCaptureWithRetry` / `shell.MustRunWithRetry` /
  `shell.MustRunStdinWithRetry` / `shell.MustRunPostCreate`, OR
- Explicitly documented `R-DOC-OK` (local-only, probe-by-design, or
  intentional fail-silent for offline cases).

No new retry engine is introduced — every item routes through the existing
`internal/shell/retrycore.go` / `internal/shell/ghretry.go`.

## Constraints

- Order matches the audit's "Recommended order of fixes" (incident-correlated
  first, then leverage, then consistency).
- Each item is independently shippable. Items 1 and 2 share a transient
  regex; define it once in `internal/shell/sonarretry.go` (new file) and
  reuse.
- Tests: every wrap that changes a function's failure semantics needs at
  least one new unit test that asserts the retry loop fires on the chosen
  transient pattern and passes through on the chosen hard-fail pattern.
  Mirror the `internal/shell/github_test.go` pattern (table-driven, sleepFn
  stubbed to no-op).

---

## Items

### 9. Swap `internal/steps/project.go`'s `projectRunStdin` and `projectRun` seams

**Audit ref:** L4, L5.

- **L5 (`projectRun` for `gh project link`):** rename and rewire to
  `shell.RunWithRetry`. The seam at `internal/steps/project.go:50` is
  test-only — the change is a one-line edit in the seam plus updates to
  any test that asserts the exact wrapper used.
- **L4 (`projectRunStdin` for the GraphQL `updateProjectV2Field` mutation):**
  swap to `shell.MustRunStdinWithRetry` if abort-on-fail is acceptable
  semantically (current site is followed by `log.Fatalf` on error anyway),
  OR add a new non-`Must` `shell.RunStdinWithRetry` if we want the error
  to propagate to the caller. **Decision needed before execution.**

**Tests:** the existing `internal/steps/project_test.go` table tests will
need their seam replacements updated; the failure-mode tests (transient →
retry → success) can be added once.

---

### 10. Add retry to `internal/config/config.go`'s direct `exec.Command` probes

**Audit ref:** L8, L9.
**Functions:** `realCheckOwnerExists` (line 841), `realCheckProjectExists`
(line 873), `confirmReposExist` (line 936), `CloneShop` (line 1180) — the
`gh api ...` call only —, `latestMetaRelease` (line 1213).

**Change shape:** each currently builds a raw `exec.Command("gh", "api", ...)`
and calls `cmd.Run()` / `cmd.CombinedOutput()`. The minimal-touch fix is to
route each through `shell.RunWithRetry` (or `RunCaptureWithRetry` where
stdout is parsed), reinterpreting the existing "non-zero exit means not
found" classifier into a stderr / output pattern check.

**Caveat:** `internal/config/config.go` currently uses `exec.Command`
directly to suppress stderr on the expected-404 case (line 843, "Stderr is
suppressed so the first 404 doesn't leak when we fall back"). Migrating to
`shell.RunWithRetry` means stderr always lands in the returned error string
— callers need to start matching on the IS-NOT-FOUND wording in the error
instead of the cmd's exit code. This is a behaviour change worth landing in
its own commit, not a one-line swap.

**Lowest priority** in the audit. Defer until after items 1-9 ship and the
failure log has had a few weeks to surface real init-time 5xx incidents
against `gh api users/...` / `gh api orgs/...` / `gh api repos/...`.

---

## Out of scope

- The `gh repo clone` call at `internal/config/config.go:1196`. Audit
  classified as Low; clone has its own protocol-level retries inside the
  git client.
- The `git checkout` call at `internal/config/config.go:1202`. Local-only.
- The `runtime/...` packages. Their gh calls are dispatched from
  long-running agent contexts where retry semantics belong to the agent's
  budget, not to the per-call wrapper. Audit classifies these as
  R-DOC-OK; if a future incident says otherwise, revisit in a separate plan.
- `internal/runner/system.go`'s docker-compose retry (`upOne`'s
  `transientNetRE` + 3-attempt loop). Already R-OK; bash parity lives in
  the shop-side `docker-retry.sh`.

## Verification

After each item lands:

1. `go build ./...` clean.
2. `go test ./...` passes (no test should depend on a wrapper *not* being
   retry-capable).
3. Audit re-run (manual grep) confirms the corresponding R-MISSING entry
   no longer matches. When all items in this plan land, the audit's
   R-MISSING count drops from 16 to 0 and the audit can be re-issued with
   a closing note (mirroring the "2026-05-14: H1-H5 fixed" footer in
   `audits/20260514-silent-external-call-failures.md`).

---

## Cross-reference

- Companion audit: [`audits/20260514-external-call-retry-coverage.md`](../audits/20260514-external-call-retry-coverage.md)
- Parent program: [`plans/20260514-1945-retry-mechanism-end-to-end.md`](20260514-1945-retry-mechanism-end-to-end.md) (Phase 6)
- Sibling audit (silent errors, not retries): [`audits/20260514-silent-external-call-failures.md`](../audits/20260514-silent-external-call-failures.md)
- Engine sources: [`internal/shell/retrycore.go`](../internal/shell/retrycore.go), [`internal/shell/ghretry.go`](../internal/shell/ghretry.go)
