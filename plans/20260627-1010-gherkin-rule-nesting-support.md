# 2026-06-27 10:10:18 UTC — Support official Gherkin `Rule:` nesting (Feature → Rule → Scenario) end-to-end in the ATDD pipeline

## TL;DR

**Why:** Today a ticket's Acceptance Criteria can only express a flat `Feature:` → `Scenario:` list. There's no first-class way to group the scenarios that all illustrate one business rule (e.g. "shipping = $0.10/kg/unit") under that rule. Gherkin's official `Rule:` keyword (Gherkin v6+, https://cucumber.io/docs/gherkin/reference/#rule) is built exactly for this, but two pipeline agents would silently break it: the `acceptance-criteria-refiner` is a free rewriter whose mental model is a flat scenario list (no `Rule:` awareness, so it may flatten author-written rules), and the `acceptance-test-writer` has no `Rule:` → code mapping (so the grouping never reaches the generated test files).

**End result:** A ticket author can write `Feature:` → `Rule:` → `Scenario:` in the Acceptance Criteria, and it survives end-to-end: `parse-ticket` passes it through verbatim (already does), the `acceptance-criteria-refiner` preserves the `Rule:` grouping while still applying its coverage rubric and `@isolated` tagging, and the `acceptance-test-writer` maps each `Rule:` to a defined per-language grouping convention in the generated acceptance tests. Flat (no-`Rule:`) tickets are unchanged — `Rule:` is purely additive and opt-in.

## Outcomes

What we get out of this:

- A ticket's `## Acceptance Criteria` can use official Gherkin `Rule:` blocks (`Feature:` → one-or-more `Rule:` → `Scenario:`s under each), and the business rule lives in the `Rule:` line/description — the canonical home for "a rule + its illustrating examples".
- The `acceptance-criteria-refiner` is `Rule:`-aware: it **preserves** author-written `Rule:` grouping (never flattens a rule into a bare scenario list), keeps each refined/added scenario under the correct `Rule:`, and applies its existing coverage rubric and `@isolated` tagging *within* rules.
- The `acceptance-test-writer` has a defined, documented **`Rule:` → grouping convention per language** (Java / .NET / TypeScript) and emits grouped acceptance tests accordingly, composing correctly with the existing channel-parameterization wrappers (Java `@TestTemplate`+`@Channel`, TS `forChannels(...)`).
- A canonical, pinned **AC/Rule format doc** (symmetric to `internal/atdd/assets/runtime/shared/escc-format.md`) describes the `Feature`/`Rule`/`Scenario` shape and the `Rule:` → code-grouping mapping, so authors and both agents share one source of truth.
- `docs/atdd/code/language-equivalents.md` (and any relevant `docs/atdd/architecture/*.md`) document the per-language grouping convention.
- Regression coverage: a parse/intake test proving a `Rule:`-containing AC body survives extraction **verbatim** (the parser stays dumb), and the clauderun render-matrix test still passes for the refiner + writer prompts (no unresolved placeholders introduced).
- **Backward compatible:** existing flat tickets and tests are untouched; `Rule:` is opt-in, and a ticket with no `Rule:` behaves exactly as today.

## ▶ Next executable step (resume here)

All design decisions are resolved (see Decisions below) — execution can start. Step 1: write the canonical `internal/atdd/assets/runtime/shared/ac-format.md` format doc pinning the `Feature` → `Rule` → `Scenario` vocabulary and the **v1 grouping convention: naming-prefix + a `// Rule: <name>` comment block** (no structural nesting), with one worked example per language (Java / .NET / TypeScript) showing how it composes with the channel wrapper. Everything else (refiner prompt, writer prompt, language-equivalents doc, tests) keys off that doc.

## Steps

- [ ] Step 1: Author the canonical `internal/atdd/assets/runtime/shared/ac-format.md` (symmetric to `escc-format.md`): pin the `Feature:` → `Rule:` → `Scenario:` shape, state that `Rule:` is opt-in/additive, that the rule statement (incl. any formula) lives in the `Rule:` name/description as human-readable narrative (never executed), and document the **v1 grouping convention: method-name prefix + a `// Rule: <name>` comment block above the group, no structural nesting** — with one worked example per language (Java `@TestTemplate`+`@Channel`, .NET, TS `forChannels(...)`) showing the prefix/comment composing with the channel wrapper. Note explicitly: `@isolated` stays scenario-scoped inside a rule; `Background:` under `Rule:` is unsupported in v1.
- [ ] Step 2: Update `internal/atdd/assets/runtime/agents/atdd/acceptance-criteria-refiner.md` to be `Rule:`-aware: the AC body may contain `Rule:` blocks; **preserve** their grouping when rewriting (v1 = preserve only, never invent new rules or auto-group existing flat scenarios); place every edited/new scenario under the correct `Rule:`; apply the coverage rubric and `@isolated` tagging within rules; never flatten a `Rule:` into bare scenarios. Reference `ac-format.md`.
- [ ] Step 3: Update `internal/atdd/assets/runtime/agents/atdd/acceptance-test-writer.md` with the v1 `Rule:` → grouping convention: each `Rule:` maps to a method-name prefix + `// Rule: <name>` comment block (no structural nesting), composing with the existing channel wrapper; scenarios under a rule become its grouped tests; flat ACs stay flat. Add a worked example. Reference `ac-format.md`.
- [ ] Step 4: Update `docs/atdd/code/language-equivalents.md` (and any relevant `docs/atdd/architecture/*.md`) to document the per-language grouping convention so the human-facing docs match the prompts.
- [ ] Step 5: Add regression coverage — an intake/parse test asserting a `Rule:`-containing AC body is extracted verbatim (parser unchanged, stays dumb), and confirm the clauderun render-matrix test (`TestRenderMatrix_NoUnfilledPlaceholders`) still passes for the edited refiner + writer prompts.
- [ ] Step 6: Decide whether the `.github/ISSUE_TEMPLATE/*.yml` Acceptance Criteria textarea needs a one-line hint pointing at `Rule:` support (likely a short help/placeholder note, not a structural change) — or leave templates untouched and rely on `ac-format.md`.

## Decisions (resolved 2026-06-27)

- **Grouping convention → naming-prefix for v1.** Scenarios under a `Rule:` get a shared method-name prefix + a `// Rule: <name>` comment block; **no** structural nesting. Chosen because it's the lowest-risk, language-uniform path that composes trivially with the custom channel wrappers (Java `@TestTemplate`+`@Channel`; TS `forChannels(...)(() => …)`) and matches the repo's current flat house style (tax/total are flat scenarios + `@DataSource`). Structural nesting (`@Nested` / nested class / native `describe`) is deferred to a possible **v2** once the convention proves out — each language's nesting must first be shown to compose with the channel wrapper.
- **Refiner scope → preserve only.** v1 preserves author-written `Rule:` blocks; it does **not** invent new rules or auto-group existing flat scenarios. Auto-grouping is deferred.
- **`@isolated` × `Rule:` → scenario-scoped, unchanged.** The tag attaches to an individual `Scenario:` inside a `Rule:`; the writer's isolation-mirroring is unaffected by grouping. `ac-format.md` states this explicitly.
- **`Background:` under `Rule:` → unsupported in v1.** Documented as explicitly out of scope in `ac-format.md` so authors don't rely on it silently.
