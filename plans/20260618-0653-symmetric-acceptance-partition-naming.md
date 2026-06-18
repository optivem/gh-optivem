# Symmetric acceptance-partition naming: `acceptance-<ch>` lies about its scope

> Follow-up to `plans/20260617-1651-per-channel-verify-covers-isolated-suite.md`.
> That plan fixes the **call sites** so the per-channel verify runs both partitions.
> This plan fixes the **names**, so the convention can no longer mislead a reader
> (or a future call site) into thinking `acceptance-<ch>` is the whole channel.

## TL;DR

**Why:** The acceptance-suite pair is named asymmetrically — `acceptance-<ch>` (non-isolated) and `acceptance-isolated-<ch>` (isolated). The unmarked id `acceptance-<ch>` reads as "**all** acceptance tests for the channel" but is really only the *non-isolated half*. That lie is exactly what produced rehearsal #76's `TESTS_INFRA_HALT`: the per-channel unroll bound `acceptance-<ch>` believing it was complete. The 1651 plan patches the call sites, but the name still claims the whole category while delivering a subset, so the trap stays armed for the next reader and the next call site.
**End result:** The two partitions are named symmetrically — `acceptance-parallel-<ch>` and `acceptance-isolated-<ch>` — so neither id pretends to be the whole. The bare `acceptance-<ch>` is promoted from a concrete suite to a **per-channel group alias** = `[acceptance-parallel-<ch>, acceptance-isolated-<ch>]`. Per-channel verify call sites bind `acceptance-<ch>` and it expands to both partitions **by construction**, retiring the hand-written comma-join from 1651 and making the coverage hole structurally impossible to reintroduce. Student-facing scaffolded `tests.yaml` reads honestly.

## ▶ Next executable step (resume here)

Confirm the **naming decision** below with the user (student-facing ids), then start at Item 1 (the SSoT resolver `internal/atdd/runtime/testselect/suite.go`). No pickup marker; `git status` clean on `main` at authoring.

## Naming decision (RESOLVED — pending user confirm)

The partition axis is *parallel execution* vs *serial isolation* (`maxParallelForks>1` vs `=1`; the isolated suite runs serially so a clock-mutating `@TimeDependent` test can't clobber parallel tests). The names should describe that axis symmetrically.

**Chosen:** `acceptance-parallel-<ch>` + `acceptance-isolated-<ch>`, with `acceptance-<ch>` as a per-channel group of both.

**Why `parallel`:** It's already the codebase's own word for the non-isolated suite — `testselect/suite.go:5` calls it "the parallel non-isolated suite (acceptance-<ch>)". It's the true execution opposite of *isolated/serial*, and it's plain enough for students to read. Rejected alternatives: `shared` (vague — shared what?), `concurrent` (synonym, longer), `default`/`normal` (says nothing about *why* it's separate), leaving `acceptance-<ch>` as-is (keeps the lie — the whole point).

**Why promote `acceptance-<ch>` to a group (not just rename one suite):** A flat rename to `acceptance-parallel-<ch>` removes the lie but still leaves call sites to *manually* pair the two partitions (the 1651 comma-join). Making `acceptance-<ch>` a group that fans out to both means a single binding `acceptance-<ch>` is always complete — the same correctness-by-construction the top-level `acceptance` group already gives for all channels, now at per-channel granularity. It also lets the generic engine keep its `"acceptance-"+ch` literal (no `testselect` import) and be *correct* again, because the string is now a group the CLI expands.

> One decision worth a thumbs-up before execution: these ids appear verbatim in
> every scaffolded student `tests.yaml` and in `--suite=` examples students read.
> If you'd prefer a different word than `parallel`, say so now — it's a one-token
> swap in this plan but a wide rename once executed.

## Problem

`internal/kernel/projectconfig/channels.go:9-13` states the convention outright: "an `acceptance-<token>` suite … `acceptance-${channel}` → acceptance-api / acceptance-ui." So `acceptance-api` is *defined* as "the acceptance suite for the api channel" — yet the scaffolded `tests.yaml` binds that id to `-DexcludeTags=isolated`, i.e. only the non-isolated subset. The name and the behaviour disagree.

Consequences:
- **Human:** a reader running `--suite=acceptance-api` reasonably believes they ran every API acceptance test; they silently skipped the isolated ones.
- **Code:** the per-channel unroll (`channels.go:73/126`) hardcoded `"acceptance-"+ch` on that same false belief → rehearsal #76 halt. 1651 patches it, but any *new* per-channel binding can fall into the identical trap because the name still invites it.

This is a teaching repo (memory: env/suite names leak into code students read), so an honest, symmetric convention has outsized value.

## Scope of the rename (blast radius)

Grounded by grep at authoring; executor re-confirms before editing.

| Site | Role | Change |
| --- | --- | --- |
| `internal/atdd/runtime/testselect/suite.go` | SSoT resolver `AcceptanceSuites` + `defaultSuiteGroups` | emit `acceptance-parallel-<ch>` + `acceptance-isolated-<ch>`; add per-channel `acceptance-<ch>` group alias to the default registry |
| `internal/kernel/projectconfig/channels.go:9-13` | doc comment asserting the `acceptance-<token>` convention | reword: `acceptance-<token>` is the per-channel **group**; concrete suites are `parallel`/`isolated` |
| `internal/atdd/runtime/preflight/preflight.go:~334` | existence sweep (expects `acceptance-<ch>`+`acceptance-isolated-<ch>`) | follow the new ids; sweep still routes through `ExpandSuiteGroups` so it stays auto-consistent |
| `internal/engine/statemachine/channels.go:73,126` | per-channel verify binding | with `acceptance-<ch>` now a group, the literal `"acceptance-"+ch` becomes correct-by-expansion — **revert the 1651 comma-join** here; update doc blocks |
| `internal/atdd/runtime/driver/scoped.go:153` | scoped-resume binding | same; bind `acceptance-<ch>` (group) — supersedes 1651's `testselect.AcceptanceSuites` join |
| **Scaffolded `tests.yaml` template** (emitted by `gh optivem init`, all 3 langs) | the suite `id:`s + `suiteGroups:` block | rename the two suite ids; add per-channel `acceptance-<ch>` group entries |
| Tests: `testselect/suite_test.go`, `preflight_test.go`, `channels_test.go`, `scoped_test.go`, `actions/bindings_test.go`, `clauderun_test.go`, `verify_classify_test.go`, `config_test.go` | assertions pinned to old ids | update to new ids; add an assertion that `acceptance-<ch>` expands to both partitions |

**Find the scaffold template first.** No `tests.yaml` lives in this repo; `gh optivem init` writes it from a template/embedded asset (or copies a per-language testkit). Locate the emission point (search the `init`/`copySystemTests` path and embedded assets) before editing — the student-facing ids are the *primary* artifact this plan changes; the Go-side default in `suite.go` is only the fallback.

## Items

1. **`testselect/suite.go`** — change `AcceptanceSuites` to emit `acceptance-parallel-<ch>` + `acceptance-isolated-<ch>`. Add a per-channel group `acceptance-<ch> → [those two]` to `defaultSuiteGroups` (so the bare id resolves even when a project omits `suiteGroups`). Keep the top-level `acceptance` group = all channels' both-partitions (now composed from the per-channel groups). Update the doc block.

2. **Scaffold template** — in the `gh optivem init`-emitted `tests.yaml` (Java + .NET + TypeScript): rename the two suite `id:`s to `acceptance-parallel-<ch>` / keep `acceptance-isolated-<ch>`; add a `suiteGroups:` block giving each `acceptance-<ch>` and the all-channels `acceptance`. Suite `command:` strings (the `-D…Tags=isolated` flags) are unchanged — only ids move.

3. **`channels.go:73,126` + `scoped.go:153`** — revert the 1651 per-channel comma-join: bind plain `acceptance-<ch>` again, now correct because it's a group the CLI expands. Update the doc blocks to say the per-channel id is a group covering both partitions, cross-referencing `testselect`. (Net diff vs pre-1651: identical literal, but now *meaningfully* complete.)

4. **`preflight.go`** — update the expected-id construction to the new convention; it already funnels through `ExpandSuiteGroups`, so confirm it validates `acceptance-parallel-<ch>` + `acceptance-isolated-<ch>` per `cfg.Channels`.

5. **Tests** — update every assertion pinned to the old ids (table above). Add: (a) `suite_test.go` asserts `acceptance-<ch>` expands to exactly the two partitions; (b) a binding test asserts the per-channel verify, given the group, requests both partitions — the correctness-by-construction guard that makes the #76 hole unreintroducible.

## Sequencing vs the 1651 plan

- **1651 ships first** (minimal halt fix; deterministic; unblocks rehearsal #76 today).
- **This plan supersedes 1651's call-site mechanism**: once `acceptance-<ch>` is a group, the comma-join is redundant and Item 3 reverts it. Executing this plan *before* 1651 lands is fine too (it subsumes the fix); if both are in flight, coordinate so the call sites aren't edited twice. Cross-check `git log`/`git status` for 1651 commits before starting Item 3 (memory: concurrent-agent collision).

## Verification

- `scripts/test.sh` (or `-p 2`) green for `internal/atdd/runtime/testselect`, `internal/atdd/runtime/preflight`, `internal/engine/statemachine`, `internal/atdd/process/actions`, `internal/atdd/runtime/driver`, `internal/build/runner`. Never unbounded `go test ./...` on Windows (memory).
- `gh optivem init` a throwaway project; confirm the generated `tests.yaml` shows `acceptance-parallel-api` / `acceptance-isolated-api` and the `acceptance-api` group. *(Operator step.)*
- Re-run rehearsal #76 end-to-end → clean `result: ok`; trace shows the per-channel verify expanded `acceptance-api` to both partitions. *(Operator step — rehearsal harness is user-driven.)*

## Out of scope

- The acceptance-test-writer non-determinism / `@TimeDependent ⇒ @isolated` corpus question — already carved out as a separate concern by the 1651 plan; not touched here.
- No new channels, no change to the parallel-vs-isolated gradle tag mechanism — this is a **rename + group-promotion** only, no behavioural change to what any suite executes.
