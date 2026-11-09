# 2026-11-09 19:09 UTC — Plan coordination meta-plan: all active top-level plans

**Plans analysed:** 8 in-scope, 2 referenced-only (`20261104-1120-meta-backlog-consolidation.md`, left untouched on purpose; `move-to-shop/20261106-2048-setup-gcp-flavor-aware-and-domain-callers.md`, shop work).

This is the single place to see what is still open across the top-level plans and in what order to run it. The child plans stay where they are (per `[[feedback_new_plan_not_extend]]`); this file only coordinates them.

## Per-plan status snapshot

| Plan | Status | Open items | Touched files (primary, open work only) | Who can run it |
|---|---|---|---|---|
| `20261104-1121-switch-shop-to-own-domain` | ⏳ Deferred (user, 2026-11-05) | Step 1+5 (shop removal), Step 8 (removal tests + attended e2e) | `internal/kickoff/removal.go` (+ tests); shop "empty" template | Blocked: walking-skeleton design needs a discussion |
| `20261106-1305-scaffold-cloud-setup-guide` | Steps 1–4b done | guide unverified bits; Step 5 live GCP rehearsal | `docs/cloud-setup.md` | User only (costs money, their GCP/Neon accounts) |
| `20261107-1300-spec-parse-gherkin-criteria` | Steps 1–4 done | Step 5 release + shop check | none (release via pipeline after a push) | User go needed |
| `20261109-1219-improved-atdd-process-flow` | Umbrella; stages 1, 2, 3, 4, 4/6, 5a done | Step 2: delete when children are done | the plan itself | Agent, last |
| `20261109-1250-refactor-touched-use-case-tests` | Discussion | Step 1 decide; Step 2 write impl plan | none now; later `process-flow.yaml` (refactor processes `VERIFY_TESTS_PASS`) | User + `/refine-plan` |
| `20261109-1258-ac-delete-and-generate` | Done except Step 7 | Step 7: one stale comment | `internal/scaffolding/steps/apply_template.go` (~line 158) | Agent, now (file is clean in `git status`) |
| `20261109-1301-legacy-baseline-mode` | Discussion | Step 1 decide; Step 2 draw flow + impl plan | none now; later `process-flow.yaml` (`cover-system-behavior`) and/or `internal/kickoff/` | User + `/refine-plan` |
| `20261109-1302-ux-ui-review` | Unblocked; decisions proposed, pending user OK | Steps 1–3 | new `capture-ui-screens` service task (Go), new `ux-ui-reviewer` agent prompt, `process-flow.yaml` (`GATE_REFACTOR_TYPE_CHOICE` + new `review-ux-ui` process), tests | User OK on decisions, then agent |

## Dependency graph

```
1219 (umbrella) ◄── 1258 Step 7
                ◄── 1302 ux-ui-review ◄── user OK on proposed decisions
                ◄── 1250 refactor test scope (discussion)
                ◄── 1301 legacy baseline (discussion)

1300 Step 5 (release) ──► shop Track D (shop plan 20261106-1533)   [outside this repo]
1305 Step 5 (live GCP) ──► guide "unverified bits" are settled during Step 5
1121 (deferred) ──► waits on walking-skeleton decision (meta-backlog Phase B)
```

Edges are the plans' own words: 1219's table lists 1258/1302/1250/1301 as its stages and Step 2 deletes it "when every stage plan is executed or dropped"; 1305 says the unverified bits are settled during Step 5. No cycles.

## Conflicts

### 1. `internal/atdd/process/process-flow.yaml` — soft conflict (future)
- `1302` Steps 1–2 add a `review-ux-ui` process and a menu branch at `GATE_REFACTOR_TYPE_CHOICE`.
- `1250` Step 2 (if "narrow") and `1301` Step 2 (if shape (b)) will each produce an implementation plan that edits this file (refactor processes / `cover-system-behavior`).
- **Why soft:** different processes, but the meta-backlog's rule C7 forbids two parallel sessions on this file.
- **Resolution (recommended):** run `1302` execution alone on this file; the implementation plans that come out of `1250`/`1301` run after it, one at a time. Discussion steps (Step 1 of each) touch no files and can run any time.

### 2. `internal/kickoff/` — coordination conflict (possible)
- `1121` owns `internal/kickoff/removal.go`.
- `1301` option (a) is "a new kickoff mode", which would touch the same package.
- **Resolution (recommended):** no action now: `1121` is deferred and `1301` is undecided. When `1301` is refined, record whether it picked (a); if so, sequence its implementation plan after `1121`'s skeleton decision.

## Consolidation findings (decided)

### 1. `1219` umbrella + its child plans — keep separate, close umbrella last
- `1219` only tracks its children (`1258`, `1302`, `1250`, `1301`); its own content is the target design and decisions record.
- **Resolution (recommended):** keep all as written; `1219` is not an execution unit, only its Step 2 (delete) runs after the children are executed or dropped. **Why:** the children are already refined/split; merging would re-open finished decisions.
- **Alternative considered:** merge `1250` + `1301` into one discussion plan. Rejected: unrelated questions (refactor test scope vs. legacy on-ramp); one `/refine-plan` session can still walk both back to back (see Wave 1 Batch C).

### 2. `1258` Step 7 — fold into any small session
- One-line comment edit, already unblocked (no other session's edits in `apply_template.go` today).
- **Resolution (recommended):** do it directly with `Edit` (not via an agent, per token rule), then `1258` is fully done and can be deleted.

## Execution units (post-consolidation)

| Unit | Plans | Type | Touched files (primary) |
|---|---|---|---|
| U1 | `1258` Step 7 | standalone (direct edit) | `internal/scaffolding/steps/apply_template.go` |
| U2 | `1302` | standalone | `process-flow.yaml`, new service task + agent prompt, tests |
| U3 | `1250` Step 1 | discussion | none |
| U4 | `1301` Step 1 | discussion | none |
| U5 | `1300` Step 5 | user-gated release | none (push → pipeline release) |
| U6 | `1305` Step 5 + unverified bits | user-only live run | `docs/cloud-setup.md` |
| U7 | `1121` | ⏳ deferred | `internal/kickoff/removal.go`, shop template |
| U8 | `1219` Step 2 | closing step | the plan file |

## Needs-decision

### 1. OK the proposed decisions in `1302`?
- Its four decisions (screenshots + UI source, no CI, `APPROVE_VISUAL_CHECK` human gate, reuse the code-review loop) are "pending user OK". U2 can't start until then.
- **Recommended:** accept as proposed — they mirror the shipped Code & Architecture Review, so the least new machinery.

### 2. Release `1300` now?
- The parser is done and checked against shop; only the release (push to `main` → pipeline) is left, and shop's Track D waits on it.
- **Recommended:** yes, release soon — shop's `spec-gate` stays off until it ships.

## Execution waves

### Wave 1 — can start now

**Batch A (parallel-safe):**
- U1 `1258` Step 7 — `apply_template.go` comment — direct `Edit`, no agent; then delete `1258`.
- U5 `1300` Step 5 — after your go: `/commit` + let the pipeline release, then `gh optivem spec validate` in a shop checkout (expect 14 UC-001 criteria).

**Batch B (needs you, no files):**
- U3 + U4 — one `/refine-plan` session each (`1250`, `1301`), can be back to back. Output is decisions + (maybe) new implementation plans.
- Needs-decision #1 — OK `1302`'s decisions.

**Batch C (user-only, any time):**
- U6 `1305` Step 5 — live GCP run with a short repo name; fold gaps into `docs/cloud-setup.md`.

### Wave 2 — after your OK on `1302`
- U2 `1302` — one agent session via `/execute-plan plans/20261109-1302-ux-ui-review.md`. Sole owner of `process-flow.yaml` while it runs.

### Wave 3 — after Wave 2 lands
- Implementation plans created by U3/U4 (if any), one at a time on `process-flow.yaml`.
- U8 — delete `1219` once `1258`, `1302`, `1250`, `1301` are executed or dropped.

### Not scheduled
- U7 `1121` — waits on the walking-skeleton discussion (tracked in the meta-backlog plan, Phase B).

## Pre-execute checks (apply before any wave starts)

- `grep -l "PICKUP\|claimed by\|Picked up by agent" plans/*.md plans/*/*.md plans/*/*/*.md` (note: `in-flight` matches false positives in `1305` and the meta-backlog prose).
- `git status` clean on `apply_template.go` and `process-flow.yaml`.
- On Windows run Go tests with `scripts/test.sh` or `-p 2`.

## Out of scope of this meta-plan

- `20261104-1120-meta-backlog-consolidation.md` (kept separate by request) and everything under `plans/backlog/`, `plans/move-to-shop/`.
- Plan content correctness and execution.
