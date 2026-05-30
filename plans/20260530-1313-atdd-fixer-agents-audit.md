# Audit & improve the ATDD `-fixer` agents

Audit of the five `-fixer` agent prompts under
`internal/assets/runtime/agents/atdd/`, scored on two axes the request
named: **are they good enough at fixing**, and **are they token
efficient**. This plan captures the findings and the prompt-side work to
act on them. Harness-side wiring gaps are cross-referenced, not
re-planned.

## Scope of the audit

The five agents and their dispatch wiring:

| Agent file | MID / process | Model · effort | Job |
|---|---|---|---|
| `command-failed-fixer.md` | `fix-command-failed` (β-convention) | opus · high | A shell command (build/lint/`gh optivem system build`) exited non-zero |
| `missing-output-fixer.md` | `fix-missing-output` (β-convention) | opus · high | A writing-agent skipped a required `gh optivem output write` key |
| `scope-diff-fixer.md` | `fix-scope-diff` (β-convention) | opus · high | A writing-agent's diff fell outside its `scopes:` whitelist |
| `unexpected-failing-tests-fixer.md` | `fix-unexpected-failing-tests` | opus · high | Verify went red after a behaviour-preserving WRITE |
| `unexpected-passing-tests-fixer.md` | `fix-unexpected-passing-tests` | opus · high | A just-authored must-fail test passed |

Each rendered prompt = `runtime/shared/preamble.md` + `runtime/shared/scope.md`
+ the agent body (+ `interactive-suffix.md` only in interactive mode),
assembled in `internal/atdd/runtime/agents/embed.go::Prompt`. The
`${scope-block}`, `${changed-files}`, `${verify-results}`,
`${command-*}`, `${missing-outputs}`, `${violating-paths}` placeholders
are substituted in `clauderun.go::renderPrompt`.

## Findings

### What's already good (keep — do not regress)

- **One-shot, no self-retry.** Every fixer is told "one attempt only, do
  not re-run verify/command, the caller re-validates after you exit."
  This is the right contract — it avoids each fixer burning a second
  Opus pass re-running the thing the orchestrator will re-run anyway.
- **Diagnose-then-fix, pick-the-side-and-surface-reasoning.** The three
  judgment-heavy fixers (`scope-diff`, `unexpected-failing`,
  `unexpected-passing`) make the agent state which side it's fixing and
  why, so the caller's re-validate can catch a wrong pick. Good.
- **`unexpected-passing-tests-fixer` defaults to suspecting the test.**
  Correct bias: a just-authored must-fail test that's green-on-arrival
  is far more likely mis-asserted than the SUT being wrong. The
  anti-pattern note nails the inverse-of-red→green reasoning.
- **Inputs are pre-substituted, not re-fetched.** `${changed-files}`,
  `${command-stderr-tail}`, `${verify-results}` arrive in the prompt, and
  the bodies correctly forbid re-running `git status` / `gh issue view`.
  `verify_results_text` is tail-bounded by `lastNLines(...)` in
  `runCommand`, so there's no prompt-size blowup from a chatty runner.

### Quality findings ("good enough at fixing?")

- **Q1 — The scope-exception escape hatch every fixer instructs is not
  consumed by any gateway.** All five bodies tell the agent to "emit the
  scope-exception envelope and stop" when the fix needs an out-of-scope
  edit. But the `scope-exception-requested` gate binding
  (`internal/atdd/runtime/gates/bindings.go:202`) has **no gateway node
  routing on it** — an out-of-scope signal falls through
  `outputs-and-scopes-valid == false` → `GATE_FIX_ON_FAILURE` → `FIX`,
  the exact loop the envelope was meant to bypass. This is a
  prompt-promises-what-the-harness-ignores mismatch. **Owned by the
  parked plan [[20260528-1230-wire-scope-exception-gateway-in-execute-agent]]**
  (currently `NEEDS DISCUSSION` — VJ's position is that a scope
  exception should route to `FIX`, not a new halt). This plan does not
  re-solve it; Item 5 only aligns the prompt wording to whatever that
  plan resolves.

- **Q2 — The verify→fix→verify loop is unbounded; no circuit breaker.**
  `verify-tests-pass` / `verify-tests-fail` route
  `FIX_UNEXPECTED_*_TESTS → RUN_TESTS` with no max-iteration guard
  (`process-flow.yaml:1256`, `:1300`). A fixer that repeatedly mis-picks
  the side loops forever at **opus · high** — the most expensive tuning
  in the tree — and (per [[feedback_statemachine_test_loop_hazard]]) an
  unbounded fix loop is exactly the shape that has burned 20GB+ RAM
  before. The fix is harness-side (a loop counter / escalation-to-human
  after N attempts), so it is **out of scope for the prompt edits here**
  but recorded as the highest-severity risk; see Item 6 (spin-off
  pointer).

- **Q3 — `scope-diff-fixer`'s revert path has a blind safety net.** The
  agent may run `git checkout HEAD -- <path>` to revert a "Mode B/C"
  violating edit. If it wrongly reverts a *legitimate* edit (Mode A,
  "scopes too narrow"), the caller's re-validate **passes** (the working
  tree is now scope-clean) while real work was silently dropped — the
  safety net cannot catch a wrong revert, only a wrong widen. The
  existing anti-pattern note warns against this but the structural
  asymmetry isn't surfaced in the decision step. Prompt-side mitigation
  in Item 4.

### Token-efficiency findings

- **T1 — ~5 lines of identical boilerplate duplicated across all five
  bodies.** Each body opens its `## Steps` with the same two paragraphs,
  near-verbatim:
  - "Per the preamble carve-out for `fix-*` tasks, you MAY run `git
    diff`, `git diff HEAD`, and `git show HEAD:<path>` … No other
    `git`/`gh` calls."
  - "One attempt only — do not retry … Approval upstream of you already
    gated this dispatch. Stay inside `${scope-block}` — emit the
    scope-exception envelope if you need to widen."

  That's ~5 lines × 5 agents ≈ **25 duplicated lines shipped on every
  fixer dispatch**, and a five-place edit cost whenever the contract
  shifts. `scope-diff-fixer` legitimately extends the git carve-out
  (`git checkout HEAD -- <path>`), so the shared chunk must be the
  *base* the one outlier appends to — not a hard replacement.

- **T2 — The scope-exception rule is stated three times per prompt.**
  `scope.md` (prepended) already carries the full envelope contract;
  then the body's intro restates "Stay inside `${scope-block}` — emit the
  scope-exception envelope," and then Step 4 restates it a third time
  ("If the fix would require editing a path outside `${scope-block}`,
  emit the scope-exception envelope and stop"). Collapse to one
  in-body reference.

- **T3 — "The caller's verify re-runs … it is the safety net" is stated
  twice per prompt** (intro + Step 4) in four of the five. Once is
  enough.

- **T4 — The git-carve-out sentence duplicates `preamble.md`.**
  `preamble.md` already says "The `fix-*` tasks' `git diff` / `git show
  HEAD:<path>` carve-out applies only to those tasks." The per-body
  repeat is redundant with the prepended preamble.

- **T5 — Model/effort is uniformly opus · high; two fixers may not need
  it.** `missing-output-fixer`'s contract is mechanically uniform
  ("redo + emit, idempotency handles the already-done case" — it is
  explicitly forbidden from branching on diff inspection), and
  `command-failed-fixer` is largely classify-stderr-then-minimal-edit.
  The three judgment-heavy fixers justify opus. This is a **tuning
  experiment, not a blind downgrade** — recorded as Item 7, gated on
  rehearsal evidence, never merged on a hunch.

## Items

> All prompt-body edits below are **content changes** — list-and-gate per
> [[feedback_renames_autonomous_content_gated]]. Present the rewritten
> bodies for review before committing. No diagram regeneration steps
> (these files don't feed the diagram); no commit without approval
> ([[feedback_no_commit_without_approval]]).

- [ ] **Item 1: Add a shared `fixer-preamble.md` chunk and prepend it to
  fixer agents (T1).**
  Create `internal/assets/runtime/shared/fixer-preamble.md` carrying the
  two shared paragraphs (git read-carve-out base + the one-attempt /
  approval-already-gated / stay-in-scope contract). Wire it in
  `internal/atdd/runtime/agents/embed.go` so it is prepended **only to
  the five `*-fixer` agents** (gate on the `-fixer` name suffix, or an
  explicit set — mirror how `sharedPreamble` / `sharedScope` load once at
  init via `mustReadAsset`). Add an embed-presence test alongside the
  existing `embed_test.go` cases. **Decision to confirm at execution:**
  suffix-match vs. explicit allow-list — prefer the explicit set if the
  `-fixer` suffix isn't already a load-bearing convention elsewhere.

- [ ] **Item 2: Strip the now-shared boilerplate from the five bodies
  (T1, T2, T3, T4).**
  In each of `command-failed-fixer.md`, `missing-output-fixer.md`,
  `scope-diff-fixer.md`, `unexpected-failing-tests-fixer.md`,
  `unexpected-passing-tests-fixer.md`: delete the two duplicated `##
  Steps` opening paragraphs (now in Item 1's chunk), the second
  scope-exception restatement, and the duplicate "caller's verify is the
  safety net" sentence. Keep exactly one in-body pointer where a step
  genuinely depends on the contract. **`scope-diff-fixer` exception:**
  its body keeps the one extra sentence granting `git checkout HEAD --
  <path>` (the revert action the shared base doesn't cover), phrased as
  an addition to the shared carve-out.

- [ ] **Item 3: Re-read each trimmed body end-to-end for standalone
  coherence.**
  After Items 1–2 the bodies must still read as complete prompts when
  concatenated with `preamble.md` + `scope.md` + `fixer-preamble.md`.
  Confirm no step references a sentence that was moved out, and that the
  Inputs / Steps / Anti-patterns structure still flows. Dispatch one
  rendered prompt per fixer via the `RenderPrompt` test seam (or `gh
  optivem … --show-prompt`) and eyeball the assembled output.

- [ ] **Item 4: Surface the wrong-revert asymmetry in
  `scope-diff-fixer.md` (Q3).**
  In the classification step, add one sentence making the safety-net
  asymmetry explicit: a wrong *widen* is caught by the caller's
  re-validate, but a wrong *revert* passes validation while dropping real
  work — so when a violating path is genuinely ambiguous between "Mode A
  widen" and "Mode B/C revert," **prefer widening and surface the
  uncertainty** rather than reverting. This tightens an existing
  anti-pattern into the decision step; it is not new behaviour.

- [ ] **Item 5: Align all five bodies' scope-exception wording to the
  resolved gateway semantics (Q1) — blocked.**
  ⏳ Blocked on [[20260528-1230-wire-scope-exception-gateway-in-execute-agent]].
  Once that plan resolves whether the envelope routes to a
  `STOP_SCOPE_VIOLATION` halt or falls through to `FIX`, update the five
  bodies' "emit the envelope and stop" sentence to describe the actual
  downstream behaviour (today the prompts imply a clean stop that does
  not happen). Do not touch the prompts for this until the routing
  decision lands — otherwise the wording drifts again.

## Verification

- After Items 1–3: run the clauderun render tests
  (`go test ./internal/atdd/runtime/clauderun/... -run RenderPrompt -p 2`)
  and the agents embed tests
  (`go test ./internal/atdd/runtime/agents/... -p 2`). Use `-p 2` /
  scoped packages only — never unbounded `go test ./...`
  ([[feedback_go_test_windows]]).
- Operator spot-check: render one prompt per fixer and confirm the
  assembled output is shorter than the pre-audit baseline with no
  contract sentence lost (a quick before/after line count on the five
  rendered prompts).
- Live-dispatch confirmation of fix quality (Q3, and that the trimmed
  prompts still fix correctly) can only come from a real ATDD rehearsal
  that trips each failure-kind — defer to the next rehearsal rather than
  blocking this plan.

## Out of scope / spin-offs

- **Item 6 (spin-off): bound the verify→fix loop (Q2).** A
  max-iteration guard / escalate-to-human after N failed fix attempts in
  `verify-tests-pass` / `verify-tests-fail`. Harness-side
  (`process-flow.yaml` + engine), not a prompt edit. Highest-severity
  finding; flagged here, not solved here. **Fresh plan written:**
  [[20260530-1339-bound-verify-fix-loop]] (skeleton — refine before
  executing).
- **Item 7 (spin-off): model/effort tuning experiment (T5).** Trial
  `missing-output-fixer` and `command-failed-fixer` at a cheaper tier
  (e.g. sonnet · high or opus · medium) and measure fix success across a
  rehearsal batch before changing the frontmatter. Gated on evidence;
  the three judgment-heavy fixers stay opus · high.
- The `scope_exception_requested` gateway wiring itself — owned by
  [[20260528-1230-wire-scope-exception-gateway-in-execute-agent]].
