# Test File Rules

## Positive vs Negative Test Classes

Each use case has two test files (see [language-equivalents/](../code/language-equivalents/README.md) for the file extension per language):

- **`<UseCase>PositiveTest`** — scenarios where `Then` asserts **success** (e.g. `shouldSucceed()`, resource is returned, state is correct).
- **`<UseCase>NegativeTest`** — scenarios where `Then` asserts **failure** (e.g. `shouldFailWith(...)`, error message returned).

When writing a first scenario and leaving the rest as `// TODO:` comments:
- If the first scenario is positive, put its `// TODO:` siblings in the **positive** file only if they are also positive; put negative `// TODO:` lines in the **negative** file.
- If new DSL is needed and only one test method is written, the remaining `// TODO:` lines must go into the correct file based on this rule — never mix positive and negative TODOs in the same file.

## Grouping Scenarios Under a `Rule:` (v1)

Acceptance Criteria may group scenarios under official Gherkin `Rule:` blocks
(`Feature:` → `Rule:` → `Scenario:`s) — a business rule plus the examples that
illustrate it (the AC format is pinned in the runtime `ac-format.md`). The
acceptance-test-writer translates each `Rule:` with a **lightweight naming
convention**, the same in every language:

- A **`// Rule: <name>` comment block** above the group of tests from that rule.
- A **shared method-name (or, in TypeScript, test-title) prefix** on every test
  under the rule.

There is **no structural nesting** in v1 — no Java `@Nested`, no nested C# class,
no native TypeScript `describe(...)`. The grouping is additive: it composes with
the test's existing channel-parameterization wrapper and the permanent WIP gate,
never replacing them. `@isolated` stays scenario-scoped inside a rule. A flat
(no-`Rule:`) AC stays flat. See
[language-equivalents/](../code/language-equivalents/README.md) for the per-language
naming/comment syntax.
