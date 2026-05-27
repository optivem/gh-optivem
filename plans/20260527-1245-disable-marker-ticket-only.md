# Plan: Drop phase/loop encoding from disable markers (ticket-identity only)

## Context

Test-disabler currently emits `@Disabled("<TICKET-ID> - AT - <LOOP> - <CYCLE-PHASE>")` (e.g. `71 - AT - RED - DSL`). Test-enabler matches via a dispatcher-composed `startsWith` filter on the same four-segment prefix.

Observed failure mode (rehearsal `20260527-120931`): the SYSTEM-DRIVER-phase enabler was dispatched against a test bearing a `71 - AT - RED - DSL` marker. The startsWith prefix mismatched, the enabler silently no-op'd, then ran `gh optivem compile` (guaranteed to pass because nothing changed) and reported "Compilation successful". Downstream phases inherit a still-skipped test from an apparently-green phase.

Root issue: the marker's `<LOOP>` and `<CYCLE-PHASE>` segments encode *state* the dispatcher already owns. Duplicating state into source-tree strings creates a drift surface where the dispatcher's belief and the marker's belief can diverge — and the divergence manifests as a silent no-op rather than a loud failure.

Goal: collapse the marker to ticket *identity* only — `@Disabled("#${ticket_id} ${issue_title}")` — matching the `#<id> <title>` shape that `process-flow.yaml:1842` already uses for the BPMN commit message. State lives in the dispatcher; source carries identity.

## Decisions resolved upfront

**Marker shape — raw issue title, not slug.** Adopt `#${ticket_id} ${issue_title}` (e.g. `#71 Gift-wrap an order`). Rationale:

- Symmetric with the BPMN commit-message binding at `process-flow.yaml:1842` (`#${ticket_id} ${issue_title}`).
- `${issue_title}` is already in scope: `clauderun.go:60` exposes `Options.IssueTitle`, and `clauderun.go:667` already binds `"issue-title"` for placeholder substitution.
- No new slugify rule to maintain — per `[[feedback_paths_deterministic_no_guessing]]`, derivations must be pinned, and *zero* derivation is the strongest pinning available.
- Embedded inside source-code string literals (Java `@Disabled("...")`, C# `Skip = "..."`, TS `// ...` comment) — no shell, no URL, no path → no constraint that would favor a kebab-cased slug.

Alternative considered: `#71 gift-wrap-an-order` (kebab-case slug). Rejected — requires a slugify function (one more thing to keep deterministic across Go and any future renderer), and diverges from the commit-message format already in use.

**Match rule — operate on `${test-names}`, ignore marker text.** The enabler scopes by method name (the writing-agent-emitted `${test-names}` list) and strips the `@Disabled` annotation on each named method without inspecting its reason. The marker text becomes purely informational (git blame, IDE preview, code review). Safety guard: only strip annotations whose reason starts with `#` — leaves legacy non-ticket markers like `@Disabled("flaky on CI")` untouched. Hard-fail if a named method has zero or multiple `#`-prefixed annotations (the silent no-op is the original bug — make divergence loud). Cross-ticket overlap is hypothetical under the current single-ticket rehearsal workflow; if it materialises later, file a separate plan to re-introduce id-aware disambiguation.

**No dual-format / backward-compat layer.** Per `[[feedback_teaching_repo_no_legacy]]` — single-format swap. Existing rehearsal repos carrying old-format markers are disposable; operators discard and re-run.

## Items

### Item 1 — Simplify `renderDisableMarkerExample` (test-disabler renderer)

**File:** `internal/atdd/runtime/clauderun/clauderun.go` (around line 920)

- Change signature: `renderDisableMarkerExample(lang, ticketID, issueTitle string) string`. Drop `loop` and `cyclePhase`.
- Early-return `""` when any of `lang`, `ticketID`, or `issueTitle` is empty (same fail-fast contract as today).
- Compose reason as `fmt.Sprintf("#%s %s", ticketID, issueTitle)`.
- Update the three language branches' format strings — only the reason payload changes; surrounding Java/C#/TS syntax is identical.
- Update the function-level doc comment (lines 904–919) to drop the `<LOOP>`/`<CYCLE-PHASE>` framing.

### Item 2 — Simplify `renderDisableMarkerRemovalExample` (test-enabler renderer)

**File:** `internal/atdd/runtime/clauderun/clauderun.go` (around line 942)

- Change signature: `renderDisableMarkerRemovalExample(lang string) string`. Drop both `ticketID` and `prevPhase` — the enabler no longer matches by marker text, so neither value enters the rendered instruction.
- Early-return `""` when `lang` is empty or unrecognised (same fail-fast contract).
- Update the three language branches' instruction strings to describe the new behaviour:
  > "For the named method, strip the `@Disabled` annotation (Java: delete the line; C#: rewrite `[Fact(Skip = "...")]` back to `[Fact]`; TS: delete the `// <reason>` comment and change `test.skip(` back to `test(`). Only strip annotations whose reason starts with `#` (leave legacy non-ticket markers like `@Disabled(\"flaky on CI\")` untouched). Hard-fail if the named method has zero or multiple `#`-prefixed annotations. Java only: if no `@Disabled` annotations remain in the file after stripping, also delete `import org.junit.jupiter.api.Disabled;`."
- Update the function-level doc comment (lines 936–941) to drop the startsWith / prevPhase / ticketID framing.

### Item 3 — Update the dispatcher call sites in `clauderun.go`

**File:** `internal/atdd/runtime/clauderun/clauderun.go:715–720`

- Line 715: replace `opts.TicketID, opts.NodeParams["loop"], opts.NodeParams["cycle-phase"]` with `opts.TicketID, opts.IssueTitle`.
- Line 718: simplify to `renderDisableMarkerRemovalExample(opts.Language)` — drop both `opts.TicketID` and `opts.NodeParams["prev-phase"]`.
- Both placeholders remain conditionally registered (`if ex := ...; ex != ""`) so an empty input still surfaces via `findUnfilledPlaceholders` rather than silently substituting.

### Item 4 — Update `test-disabler.md` prompt

**File:** `internal/assets/runtime/agents/atdd/test-disabler.md`

- Drop `loop` and `cycle-phase` parameter docs (lines 18–19).
- Replace the "Disable marker to emit" body (lines 26–32) with:
  > The dispatcher has composed the per-language marker with the reason string fully resolved. Emit exactly this shape:
  >
  > `${disable-marker-example}`
  >
  > The reason string is `#<TICKET-ID> <ISSUE-TITLE>`. The downstream `enable-tests` agent scopes by method name (the `${test-names}` list) and strips the annotation without inspecting the reason text, so the reason is purely informational (git blame, IDE preview, code review). The leading `#` is load-bearing as a safety prefix — the enabler refuses to strip annotations whose reason does not start with `#`, which protects legacy `@Disabled("flaky on CI")`-shape markers; do not drop the `#`.

### Item 5 — Update `test-enabler.md` prompt

**File:** `internal/assets/runtime/agents/atdd/test-enabler.md`

- Drop `prev-phase` parameter doc (line 18). Drop `ticket-id` parameter doc too (line 17) — no longer needed; scope comes from `${test-names}`.
- Replace the preamble (line 6) with:
  > You are the Test-Enabling Agent. For each method named in `${test-names}`, strip the per-language `@Disabled` annotation so the test runs again. Scope is the names list; do not inspect the annotation's reason text to decide whether to strip.
- Replace the "Removal transform" body (lines 25–34) with:
  > The dispatcher has composed the per-language strip transform for this dispatch:
  >
  > `${disable-marker-removal-example}`
  >
  > **Safety prefix.** Only strip annotations whose reason starts with `#`. Leave non-ticket markers like `@Disabled("flaky on CI")` untouched — those are legacy coverage that the upstream selection should have already filtered, but the prefix guard is defense in depth.
  >
  > **Hard-fail on ambiguity.** If a named method has zero `#`-prefixed `@Disabled` annotations, or more than one, fail loudly with a clear message — do not guess, do not silently no-op. The original silent no-op (an enable that did nothing then reported success) is the bug this whole plan removes; make divergence loud.
- Rewrite Step 1 to drop the prefix-match instruction: locate each method, apply the removal transform, fail loudly per the rules above.
- Rewrite Step 3 around the safety-prefix guard and the hard-fail-on-ambiguity rule (replaces the prior cross-ticket safety wording).

### Item 6 — Update BPMN bindings in `process-flow.yaml`

**File:** `internal/atdd/runtime/statemachine/process-flow.yaml`

- Drop `loop: RED` and `cycle-phase: TEST` from `DISABLE_ACCEPTANCE_TESTS` (lines 786–787).
- Drop `loop: RED` and `cycle-phase: ${cycle-phase}` from `DISABLE_TESTS` inside `implement-test-layer` (lines 1179–1180).
- Drop `prev-phase: ${cycle-phase}` from `ENABLE_TESTS` inside `implement-test-layer` (line 1138).
- Drop the upstream `cycle-phase:` pushes from the three `implement-and-verify-*` parents that feed `implement-test-layer` (lines 830, 852, 874) and the explanatory comment at line 829 — they become orphaned once the downstream `ENABLE_TESTS` / `DISABLE_TESTS` consumers above stop reading the value.
- `issue-title` needs **no** new bindings. It is registered globally at `clauderun.go:667` from `opts.IssueTitle`, not via BPMN binding — proved by the commit binding at line 1842 (`#${ticket-id} ${issue-title}`) which uses it with no upstream BPMN binding chain.

### Item 7 — Add Go renderer tests

**File:** `internal/atdd/runtime/clauderun/clauderun_test.go`

There are no existing tests for `renderDisableMarkerExample` / `renderDisableMarkerRemovalExample` today (confirmed by grep). Add fresh cases:

- `renderDisableMarkerExample("java", "71", "Gift-wrap an order")` renders a `@Disabled("#71 Gift-wrap an order")` snippet. Repeat for `csharp` and `typescript`.
- `renderDisableMarkerExample` returns `""` when any of `lang` / `ticketID` / `issueTitle` is empty, or when `lang` is unrecognised.
- `renderDisableMarkerRemovalExample("java")` renders per-language instruction text that mentions the `#`-prefix safety guard and the hard-fail-on-ambiguity rule. Repeat for `csharp` and `typescript`.
- `renderDisableMarkerRemovalExample` returns `""` when `lang` is empty or unrecognised.
- Regression assertion: the rendered enabler instruction must **not** mention any ticket-id substring matching (guards against drift back to the substring rule we explicitly rejected).

### Item 8 — Statemachine fixtures (executor's discretion)

**File:** `internal/atdd/runtime/statemachine/run_test.go`

Confirmed by grep: no current fixture asserts the param sets dispatched to disable/enable call activities, and no fixture hard-codes the `"71 - AT - RED - DSL"`-shape literal. No mandatory work.

If the executor judges the change merits new coverage (per `[[feedback_test_coverage_executor_discretion]]`), a worthwhile addition would be a fixture asserting that `DISABLE_TESTS` dispatches carry `issue-title` and do NOT carry `loop`/`cycle-phase`/`prev-phase`. Otherwise skip.

### Item 9 — Update language-equivalents reference docs

**File:** `internal/assets/runtime/references/code/language-equivalents/java.md` (and the parallel `csharp.md` / `typescript.md` rows if they exist)

- Replace any `@Disabled("<id> - AT - RED - <phase>")`-shape examples with `@Disabled("#<id> <title>")`.
- The reference doc and the dispatcher must agree on the emitted shape; verify by re-grepping after the edit that no `- AT - RED -` literal survives in this file.

## Cross-plan consideration

`plans/upcoming/20260527-1108-bpmn-commit-phase-suffix.md` proposes reshaping commit messages from `#${ticket_id} ${issue_title}` to `[${ticket_id}] ${issue_title} - <suffix>`. If it lands first, the commit-message format diverges from this plan's marker format — acceptable, because commits target `git log` readability (phase suffix matters) while markers target programmatic strip-by-name (marker text is now informational only, per the Decisions above). This plan picks `#${ticket_id} ${issue_title}` for the marker regardless. Re-alignment, if desired, would be a separate small plan.

## Out of scope

- **Shop template (`optivem/shop`).** Verified zero pre-existing markers in both `gh-optivem` and `optivem/shop` on 2026-05-27; disables remain runtime-emitted only. Cleanup of any future markers in `optivem/shop` is operator work in a separate repo.
- **TODO marker format.** Grep across `internal/assets/**` found zero `// TODO: <id> - AT - …`-shape markers today. If introduced later, follow the same `#<id> <title>` convention; no proactive work here.
- **Dual-format reader for backward compatibility.** Per `[[feedback_teaching_repo_no_legacy]]` — single-format swap, no migration code, no old-format-tolerant matchers.
- **The upstream marker-drift bug.** The original silent no-op surfaced because the dispatcher's expected prefix and the marker's actual prefix diverged. Under the unified format, divergence is structurally impossible (no phase encoding to drift), so the underlying bug no longer manifests in this rehearsal failure. If a different drift surface appears later, file a separate plan.
- **Misleading "Compilation successful" framing on no-op.** Symptom of the silent-strip-then-compile path. With the unified format the no-op path is structurally eliminated; the misleading framing disappears with it. No separate fix needed.

## Verification

- `go test ./internal/atdd/... -p 2` passes (per `[[feedback_go_test_windows]]`).
- `gh optivem compile` on a freshly scaffolded repo passes.
- Fresh rehearsal run (`bash scripts/atdd-rehearsal.sh <ticket-id> --config gh-optivem-monolith-java.yaml`) reaches GREEN with zero skipped tests in the change-cycle suite and no `silent no-op` traces in the agent transcripts.
- Repeat the rehearsal against `gh-optivem-monolith-csharp.yaml` and `gh-optivem-monolith-typescript.yaml` to confirm all three language renderers behave identically.
- Grep `internal/atdd/runtime/clauderun/clauderun.go`, `internal/atdd/runtime/statemachine/process-flow.yaml`, `internal/assets/runtime/agents/atdd/test-{disabler,enabler}.md`, and `internal/assets/runtime/references/code/language-equivalents/*.md` for `- AT - RED -` / `- AT - GREEN -` literals; expect zero hits. Do NOT grep `internal/atdd/runtime/verify/**` — that package legitimately enforces commit-message phase suffixes, unrelated to disable markers.
