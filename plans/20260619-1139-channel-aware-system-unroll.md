# 2026-06-19 11:39:00 UTC — Channel-aware system unroll: derive implement/verify channels from the tests, not static config

## TL;DR

**Why:** The ATDD orchestration unrolls one implement-and-verify-system dispatch per *configured* channel (`channels: api, ui`), blind to which channels each ticket's acceptance tests actually registered for. When a ticket's tests are channel-specific (e.g. API-only negative/error-message tests), the orchestration still dispatches a system-implementer for the absent channel **and** verifies that channel against a suite that cannot contain the test — wasting compute and then hard-halting on `TESTS_INFRA_HALT`.
**End result:** A ticket whose acceptance tests target only a subset of configured channels runs and verifies *only* those channels — no wasted per-channel implementer, no infra halt — with a command-level safety net so any residual test/channel mismatch degrades to a clean skip instead of a crash.

## Outcomes

What we get out of this — the goals and deliverables:

- The per-ticket channel set for `change-system-behavior` is **derived from the acceptance tests' own `forChannels(...)` registration** (intersected with configured channels), not read straight from static config.
- An **API-only ticket unrolls only the API channel**: no UI `system-implementer` dispatch (no wasted ~12 min / ~$1.50 like rehearsal #76), no UI verify, no `TESTS_INFRA_HALT`.
- A **UI+API ticket still unrolls both** channels — no regression for multi-channel tickets.
- The **API-only convention for negative/error-message acceptance tests keeps working untouched** — the test-writer is *not* forced to register tests for channels they don't belong to.
- **Safety net:** `gh optivem test run --test=X --suite=<s>` exits cleanly (skip) when `X` matches zero tests in `<s>` **but exists in another channel/suite**, instead of exit 1 / infra-halt. A genuinely non-existent / misspelled test (exists nowhere) still **fails loud**.
- A regression test reproducing the #76 shape (API-only new test + `channels: api, ui`) that goes green with the fix.

## ▶ Next executable step (resume here)

**Design/spike first — this is not yet a mechanical edit.** Before writing code, pin down the discovery seam by answering Open Question 1: confirm whether `internal/atdd/runtime/testselect/` (or the test runner) can already enumerate, for the ticket's acceptance tests, which channels each registers for (`forChannels(...)`) **without running them** — and if so, what command/API surfaces it. Read `internal/atdd/runtime/testselect/`, `internal/engine/statemachine/channels.go` (esp. `UnrollSystemChannels`, ~line 68), and the `change-system-behavior` wiring in `internal/atdd/process/process-flow.yaml` to locate where the channel list is sourced and passed into the unroll. Output: a short note in this plan's Open questions resolving how channels are discovered (static parse vs. a `gh optivem test` query vs. dry-run listing) — then Steps 1–N become concrete. Use `/refine-plan` to fold the findings in.

## Steps

- [ ] Step 1 (spike): Establish the **channel-discovery mechanism**. Determine how to enumerate, for a ticket's new/relevant acceptance tests, the set of channels they register for — reusing `internal/atdd/runtime/testselect/` if it already models channel↔test/suite membership, or adding a thin listing path on `gh optivem test`. Decide static-parse vs. runtime-list (see Open questions). Pin the exact source of truth.
- [ ] Step 2 (core): Compute the **derived channel set** = (configured `channels:`) ∩ (channels the ticket's acceptance tests actually register for). Define behavior when the intersection is empty or the discovery is inconclusive (fail loud — never silently unroll zero channels; see Open questions).
- [ ] Step 3 (core): Feed the derived set into the unroll. `UnrollSystemChannels(channels []string)` (`internal/engine/statemachine/channels.go:68`) currently runs at load time from config, decoupled from test metadata. Introduce the seam that passes the *derived* list instead of the raw config list — and ensure the per-channel verify's `--suite=acceptance-<ch>` / `--test=<...>` construction follows the same derived set. Preserve the existing `common`-on-first-channel and `layer-suffix` semantics.
- [ ] Step 4 (safety net): In `gh optivem test run` (the relevant `*_commands.go` + the runner), when `--test=X` matches **zero** tests in the selected suite but `X` **is registered in another channel/suite**, exit 0 with a clear "skipped — not registered for this channel" signal instead of the infra-halt error. Guard strictly: `X` existing **nowhere** still errors (exit 1) so typos fail loud. Make sure the verdict classification upstream (`verify`/`test-outcome`) reads this as a clean skip, not `infra`.
- [ ] Step 5 (tests): Add a regression test at the unroll/engine level reproducing the #76 shape (API-only acceptance test, `channels: api, ui`) → asserts only the API channel is unrolled/verified. Add a command-level test for the safety-net skip-vs-fail-loud branch.
- [ ] Step 6: Update any affected docs / BPMN doc-blocks (the `UnrollSystemChannels` comment and the `change-system-behavior` notes in `process-flow.yaml`) so the derived-channel behavior is documented where the static-config assumption used to be.

## Open questions

1. **Discovery mechanism (blocks Steps 1–3).** Can channel membership be read *statically* (parsing `forChannels(...)` in the spec files) or does it need a runtime/dry-run list from the test runner? Does `internal/atdd/runtime/testselect/` already expose channel↔test mapping we can reuse? — *Resolve in the Step 1 spike.*
2. **Where the derivation runs.** The idea says "after `write-acceptance-tests`." Is that a new BPMN node feeding a param into the load-time unroll, or does the unroll itself gain access to discovered metadata? Reconcile with the constraint that `UnrollSystemChannels` is a load-time in-memory rewrite. — *Resolve in spike.*
3. **Empty-intersection / inconclusive behavior.** If discovery finds no channels (or fails), should the run fail loud (preferred, per the repo's fail-loud rule) rather than unroll nothing or fall back to all configured channels? Confirm the desired failure mode.
4. **Core + net independence.** Should Step 4 (safety net) ship independently of Steps 1–3 (it's a smaller, lower-risk change that alone would have turned #76's halt into a skip), or land together? Sequencing/PR-split decision.
5. **Other unroll sites.** `UnrollSystemDriverAdapterChannels` (`channels.go:125`) and any other per-channel unroll — do they share the same blind-to-test-registration assumption, and are they in scope for the same fix or explicitly deferred?
