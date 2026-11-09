# 2026-11-09 16:36 local — Acceptance-criteria generator: Gherkin from tagged acceptance tests

## TL;DR

**Why:** Plan `plans/20261109-1258-ac-delete-and-generate.md` deletes a story's acceptance criteria once its tests are green. After that, the tagged acceptance tests are the only copy of the criteria. A PO or customer still needs to read them, and nobody can do that without a generator. 1258 says to ship the generator first.
**End result:** `gh optivem spec generate` reads the acceptance-test sources (Java, .NET, TypeScript), turns each tagged test's DSL chain into a Gherkin scenario, and prints one `Feature` per use case, grouped by `@UC-NNN.M`. Output goes to stdout or `--out <file>` and is never committed. A chain it cannot parse fails the run and names the test. It never skips a test silently.

Parent: `plans/20261109-1219-improved-atdd-process-flow.md` (Q4, "Change System Behavior"). Requested by: `plans/20261109-1258-ac-delete-and-generate.md` Step 3 and its Decisions.

## Outcomes

- `gh optivem spec generate [--tests <dir>]... [--use-case UC-NNN]... [--out <file>]` exists in the `spec` family (`spec_commands.go`).
- One language-neutral model (`TestCase` → sections → calls → args) with one Gherkin renderer. Three small regex front-ends (Java, C#, TypeScript) fill it.
- Every shop `latest` acceptance test on `main`, plus the rehearsal-71 gift-wrap tests, parses with no per-file special cases. That is the evidence that regex is enough. If it is not, the escalation trigger below applies.
- Golden-file tests built from real shop test files copied into `testdata/`. A round-trip test proves the output parses with the existing `internal/spec` Gherkin parser.

## ▶ Next executable step (resume here)

Step 1. Before starting, check `git status` on `internal/spec/`, because another session had uncommitted edits there on 2026-11-09. This plan adds only new files under `internal/spec/generate/` and one `AddCommand` line in `spec_commands.go`. If `spec_commands.go` is dirty, wait.

## Steps

- [ ] Step 1: Create package `internal/spec/generate` with the shared model: `TestCase{File, Line, Name, Tags, Wip, Params, Examples, Sections}`, `Section{Keyword: given|when|then, Continuation bool (from .and()), Subject Call, Modifiers []Call}`, `Call{Name (lowerCamel), Args []Arg}`, `Arg{Kind: string|number|bool|null|enum|identifier|param|template, Text}`. No language logic in this file.
- [ ] Step 2: Write the renderer from the model to Gherkin, following the mapping rules in Decisions. Output is deterministic: use cases sorted by number, criteria by number, then file path and line. Every output starts with a header comment: generated, not committed, source dirs.
- [ ] Step 3: Write the Java front-end (`java.go`). It finds test methods (`@Test` / `@TestTemplate` / `@ParameterizedTest`), collects the `@Tag` values (`UC-N.M`, `wip`), treats an `@EnabledIfEnvironmentVariable(named = "GH_OPTIVEM_RUN_WIP_TESTS"…)` gate as wip, takes inline `@DataSource(...)` / `@ValueSource` rows as Examples and an `@ArgumentsSource(X.class)` as a named source, and then tokenizes the method body's single `scenario….;` statement into the model. Parsing works on a tokenizer that respects strings and parentheses, with regex for the annotation and method headers. It is not one giant regex.
- [ ] Step 4: Write the .NET front-end (`csharp.go`). It reads `[Fact]` / `[Theory]`, `[Trait("tag", "UC-N.M")]` (also `"criterion"`, the same names `internal/spec/junit.go` accepts), takes `[InlineData]` / `[ChannelInlineData]` rows as Examples, handles the body `await Scenario(channel)….;`, turns PascalCase calls into lowerCamel, and drops numeric suffixes (`20.00m`, `5L`).
- [ ] Step 5: Write the TypeScript front-end (`typescript.go`). It reads `test('name', criteria('UC-N.M', …), async ({ scenario… }) => { await scenario….; })` and `test.each(cases)` / `test.eachAlsoFirstRow(cases)`, which take Examples from a same-file `const cases = [{…}]` array of object literals. It strips the `_$param` suffix from titles, treats single-quoted strings as strings, and treats a template literal with `${…}` as `template`.
- [ ] Step 6: Survey run, which is the escalation checkpoint. Run all three front-ends over shop `system-test/{java,dotnet,typescript}` `latest` acceptance dirs on shop's current checkout (read-only) and over `git -C ../shop show rehearsal/71-gift-wrap-an-order:<path>` for the gift-wrap tests. Record in this plan the number of tests per language and the number that do not parse. If any real test cannot be parsed without a per-file special case, stop and apply the escalation decision. Do not add workarounds.
- [ ] Step 7: Golden tests. Copy these into `internal/spec/generate/testdata/{java,dotnet,typescript}/`, unchanged except for a header comment naming the shop source path and commit:
  - shop `PlaceOrderPositiveTest` (plain chains, `@DataSource` / `ChannelInlineData` / `test.eachAlsoFirstRow` outlines, enum args, `null` args)
  - `PlaceOrderNegativeTest` (`shouldFail` + `errorMessage` / `fieldErrorMessage`, `@ArgumentsSource`, imported constants)
  - `PlaceOrderPositiveIsolatedTest` (one-line chained sections, clock)
  - TS `cancel-order-negative-test.spec.ts` (`test.each` with a top-level const array, template literal)
  - rehearsal-71 `GiftWrapOrderPositiveTest.java` (untagged, WIP-gated)

  Each input gets a `<name>.feature.golden`, using the same `-update` flag pattern as `internal/spec/render_test.go`.
- [ ] Step 8: Round-trip test. Feed each golden output into `spec.Parse` (inside a ```` ```gherkin ```` fence with front matter) and assert no `ParseErrs`, plus the expected set of criterion IDs. This keeps the generator's output valid under `docs/spec-gate/convention.md` §3.
- [ ] Step 9: Fail-loud tests with small hand-written fixtures, each expecting an error that names `file:line` and the test:
  - a lambda or `new` argument
  - a second statement in the body
  - a chain without `.then()`
  - an unknown section keyword
  - unbalanced parentheses
  - a tag whose `UC-NNN` has no use case in `specDir`
- [ ] Step 10: Add the command `newSpecGenerateCmd` in a new file `spec_generate_commands.go`, plus one `AddCommand` entry and an `Example` line in `spec_commands.go`. Flags:
  - `--config`: the existing spec.yaml flag, for `specDir`, `exclude` and use-case titles
  - `--tests` (repeatable; default `system-test.paths.at-test` from the project's `gh-optivem.yaml`)
  - `--use-case` (repeatable filter)
  - `--out`

  Exit codes follow the spec family: 0 ok, 1 tests that cannot be parsed or have unknown IDs, 2 usage or config error. On exit 1, nothing is written to stdout or `--out`.
- [ ] Step 11: Docs. Add a short "Generating readable criteria" section to `docs/spec-gate/convention.md` with the command, the mapping rules and one before/after example. Update the `--help` text.
- [ ] Step 12: Run `go test -p 2 ./internal/spec/generate/... .` (never unbounded `go test ./...` on Windows) and `go vet`.

## Decisions

- **Command: `gh optivem spec generate`.** It fits the existing `spec` family (`check`, `status`, `validate`, `adopt`, `record`), which already owns the criterion convention and spec.yaml.
- **Input: test sources, not JUnit XML or the use-case markdown.** The XML has tags but no steps, and after 1258 the markdown no longer has criteria. Only the source has both the `@UC-NNN.M` tag and the DSL chain.
- **Test dirs default to `system-test.paths.at-test` in `gh-optivem.yaml`, and `--tests` overrides it.** That path is already pinned per language in every shop config, so the generator needs no new config key. The language comes from the file extension (`.java`, `.cs`, `.ts`), so there is no `--lang` flag.
- **Use-case titles come from `spec.LoadDir(specDir)`.** The use case's front matter `title` becomes the `Feature:` name. A tag whose `UC-NNN` is not in specDir is an error, matching the gate's unknown-ID handling.
- **One shared model and renderer, three regex front-ends.** The DSL chain has the same shape in all three languages. Only syntax differs: casing, `await`, `Scenario(channel)` vs a `scenario` field, `m` suffixes. Normalising in the front-end means one set of mapping rules and one set of renderer tests.
- **Regex/tokenizer first. Tree-sitter only if Step 6 finds a real shop test that regex cannot parse, and then as a single backend swap for that language's front-end, with no flag and no dual path.** This follows memory `feedback_testselect_parsing_escalation` and 1258's decision. The model boundary makes the swap local.
- **Nothing to reuse from `internal/atdd/runtime/testselect`.** It only resolves suite IDs and never parses test source, so this is new parsing code.
- **Mapping rules (deterministic, no vocabulary file in v1):**
  - **Sections.** `.given()` / `.when()` / `.then()` start a `Given` / `When` / `Then` line. `.and()` inside a section starts an `And` line. Each section's first call is its subject.
  - **Prose.** Identifiers are split on case boundaries and lowercased (`placeOrder` → "place order", `withUnitPrice` → "with unit price").
  - **Given lines.** `Given a <subject>` followed by its `with…` modifiers, comma-joined: `Given a product with sku "BOOK-123", with unit price "20.00"`.
  - **When lines.** `When <action>` followed by its modifiers the same way: `When place order with quantity 5`.
  - **Then lines.** `Then <subject>` (`Then should succeed`, `Then should fail`). Every assertion call becomes its own `And` line. A call directly on `shouldFail()` gives `And error message "…"`. A call on an entity gives `And order has base price "100.00"` or `And order is gift wrapped`.
  - **Scenario title.** The test name, humanized and sentence-cased (`shouldRejectOrderWithZeroQuantity` → "Should reject order with zero quantity"), which matches shop's hand-written Gherkin.
- **Literal arguments are rendered as written, per language.** This keeps the generated text traceable to the source.
  - Strings appear in double quotes.
  - Numbers appear without type suffixes.
  - `true` / `false` are kept.
  - `null` becomes `none`.
  - Enums keep only their last segment (`OrderStatus.PLACED` → `PLACED`).
  - Other identifiers (imported constants such as `PERMISSION_DENIED`) appear bare.
  - Template literals keep their text.
  - Values are not normalised across languages (Java `"20.00"` vs .NET `20.00m` → `20.00`).
- **Parameterised tests become `Scenario Outline`.** A method parameter appears as `<name>`. Inline rows (`@DataSource`, `@ValueSource`, `[InlineData]`, `[ChannelInlineData]`, a same-file TS `const` array) become `Examples:`. A provider class (`@ArgumentsSource(EmptyArgumentsProvider.class)`) becomes a visible `# Examples: supplied by EmptyArgumentsProvider` comment, not a silent omission.
- **Grouping.** There is one `Feature` per use case, and each scenario carries exactly one `@UC-NNN.M` tag. A test that carries several IDs is emitted once under each ID. This keeps the output valid under convention §3, where two IDs on one scenario is an error.
- **Channels and `@Isolated` are not rendered.** Criteria are channel-agnostic. The channel matrix is an execution concern.
- **Wip tests are rendered with an extra `@wip` tag.** A test is wip if it has `@Tag("wip")` or the `GH_OPTIVEM_RUN_WIP_TESTS` gate. Readers should see in-progress behaviour, and leaving it out would be a silent skip.
- **Untagged tests are rendered too.** They go in a trailing `Feature: Untagged acceptance tests` block, and a stderr note gives their count and paths. The exit code stays 0. This is visible without being a gate. The spec gate itself decides whether an untagged test is acceptable.
- **Fail loud, all or nothing.** Any chain the grammar rejects is collected with `file:line test-name: reason`. All such errors are printed to stderr and the run exits 1 with no partial output. A partial document would read as the complete criteria, which is a lie.
- **Output goes to stdout or `--out`, never committed, and starts with a "generated, do not commit" header.** This is decided in 1258: committing it would recreate the second copy.
- **Golden tests use real shop files copied with a provenance header.** Tests must not read `../shop` at run time: shop changes under other sessions, and CI has no sibling checkout.

## Open questions

- **When-line voice: plain action (`When place order with quantity 5`) or a fixed subject (`When the customer places an order with quantity 5`)?** **Recommended: plain action in v1.** Turning arbitrary method names into conjugated verbs needs a vocabulary, which can be added later as a separate plan if readers find the plain form too terse. The plain form is deterministic and correct for every DSL verb.
