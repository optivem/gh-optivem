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

Step 10 (remaining): the two-line registration in `spec_commands.go`. Check `git status spec_commands.go` first; if another session still has uncommitted edits there, wait. Everything else is done:
- the package `internal/spec/generate`
- the command in `spec_generate_commands.go`, with its tests
- the docs in `docs/spec-gate/convention.md` §12

## Steps

- [ ] Step 10 (remaining): register the command in `spec_commands.go` — ⏳ Deferred: spec_commands.go has another session's uncommitted edits (2026-11-09). `newSpecGenerateCmd` already exists in `spec_generate_commands.go`, with tests in `spec_generate_commands_test.go`. The wiring is two edits in `newSpecCmd()`:
  - `cmd.AddCommand(newSpecCheckCmd(), newSpecStatusCmd(), newSpecValidateCmd(), newSpecAdoptCmd(), newSpecRecordCmd())` → append `, newSpecGenerateCmd()` inside the call.
  - In its `Example` string, add the line `  gh optivem spec generate --use-case UC-001` after the line `  gh optivem spec record`. The closing backtick of the raw string moves to the new line.

  Then run `go build . && go vet . && go test -p 2 -timeout 300s -run 'TestSpecGenerate' .`.

## Survey results (Step 6, 2026-11-09)

All three front-ends use the regex/tokenizer approach. None needed tree-sitter. Shop `main` @ `ac04be12`, `latest` acceptance dirs, read-only:

| Language | Tests found | Not parsed |
|---|---|---|
| Java | 86 | 0 |
| .NET | 86 | 0 |
| TypeScript | 86 | 0 |
| rehearsal-71 `GiftWrapOrderPositiveTest.java` (`9056c76b`) | 2 (untagged, WIP-gated) | 0 |

The first run failed on 10 tests: 4 Java, 4 .NET, 2 TS. Every failure was an argument expression, not the chain structure. Each was fixed with a general grammar rule (see Decisions, "Value expressions"), not a per-file case. Tree-sitter would not have helped, because the open question was how to render these expressions, not how to parse them.

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

- **When-line voice: plain action (`When place order with quantity 5`).** Accepted by the user on 2026-11-09. A fixed subject with conjugated verbs would need a vocabulary, which can come later as a separate plan.
- **Given article: "a"/"an" only for a one-word subject.** `Given an order with status PLACED`, but `And logged in as customer "A"`, not "a logged in as customer". This is deterministic: a one-word subject is a noun (product, order, coupon, clock), and a multi-word subject is a state.
- **Acronyms keep their case in prose.** A run of two or more capitals stays as written (`orderPrefixShouldBeORD` → "Order prefix should be ORD"). Everything else is lowercased.
- **Feature name: `Feature: UC-NNN <title>`.** The title comes from the use case's front matter.
- **Value expressions (found by the Step 6 survey).** These argument forms only restate a value, so each renders as that value:
  - C# null-forgiving `null!`
  - a cast `(decimal) x`
  - parentheses
  - a standard conversion call (`OrderStatus.valueOf(s)`, `Number.parseFloat(s)`, `decimal.Parse(s, …)`; the list is in `conversionCalls`)
  - string concatenation `"Order " + X + " does not exist."`, rendered as one template `"Order ${X} does not exist."`, which matches how a TS template literal renders

  `new`, lambdas and any other call still fail loud.
- **Body grammar.** Besides the single chain, a body may hold:
  - local constants bound to a literal (`const code = 'SUMMER2025';`, used in shop TS), which are inlined into the chain
  - the WIP gate statement (`test.skip(…GH_OPTIVEM_RUN_WIP_TESTS…)` or `Skip.IfNot(…)`), which marks the test wip

  Any other statement is the "second statement" error.
- **TS `xs.forEach((x) => { test(…) })` is an outline over `x`.** Its rows come from `xs` if that is an inline or same-file const array, otherwise it is a named source (`emptyArguments`). C# `[ChannelClassData]` / `[ChannelMemberData]` and Java `@MethodSource` are named sources as well.
- **Exclude globs from spec.yaml apply to test paths too.** `**` matches any number of directories, the same as the results globs.

## Open questions

None.

## Verification

- After the wiring, run `gh optivem spec generate --config ../shop/spec.yaml --tests <lang at-test dir>` on shop and read the output. Shop has no plain `gh-optivem.yaml`, only the per-variant files, so `--tests` is required there. An in-process run on 2026-11-09 gave the same result for each of Java, .NET and TS: exit 0, 96 scenarios, 7 features, and 11 untagged tests reported.
- Judge whether the plain When voice and the Given article rule read well to a PO. If they do not, write a separate vocabulary plan.
