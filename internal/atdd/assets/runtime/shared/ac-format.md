# Acceptance Criteria (AC) format — `Feature` / `Rule` / `Scenario`

The `## Acceptance Criteria` ticket section is the spec for the **outer /
acceptance loop**, symmetric to how `## External System Contract Criteria` spec
the inner / contract loop (see `escc-format.md`). It states user-observable,
end-to-end behaviour in Gherkin `Given` / `When` / `Then` form.

This file is the canonical, pinned vocabulary. Authors write it directly; the
`acceptance-criteria-refiner` (rewriter) and the `acceptance-test-writer`
(translator) are the only interpreters. `parse-ticket` stays dumb — it reads
only the section's *presence* and passes the body through verbatim; it does not
interpret the `Feature` / `Rule` / `Scenario` structure (it only validates the
body's Gherkin *syntax* at intake — see `gherkin.go`).

## Shape

ACs are authored as bare `Scenario:` blocks — no `Feature:` header is required.
Optionally, scenarios that all illustrate **one business rule** may be grouped
under an official Gherkin `Rule:` block (Gherkin v6+,
https://cucumber.io/docs/gherkin/reference/#rule):

```
Feature: Checkout
  Rule: Shipping is charged at $0.10 per kg per unit

    Scenario: shipping for a single item
      Given a product Apple weighing 2kg
      When I check out 1 Apple
      Then the shipping charge is $0.20

    Scenario: shipping scales with quantity
      Given a product Apple weighing 2kg
      When I check out 3 Apple
      Then the shipping charge is $0.60

  Rule: Orders over $50 ship free

    Scenario: free shipping above the threshold
      Given a cart totalling $60
      When I check out
      Then the shipping charge is $0.00
```

- **`Rule:` is opt-in and purely additive.** A ticket with no `Rule:` is a flat
  `Scenario:` list and behaves exactly as before — `Rule:` changes nothing for
  flat ACs.
- When any `Rule:` is used, a single `Feature:` header is conventional (Gherkin
  allows `Rule:` directly under `Feature:`). A flat AC needs no `Feature:` line.
- The **rule statement itself — including any formula** (e.g. "$0.10/kg/unit") —
  lives in the `Rule:` name and/or its description lines as **human-readable
  narrative**. It is documentation: it is **never parsed, executed, or asserted
  on**. Only the `Scenario:` `Given`/`When`/`Then` steps drive tests.

## Grouping convention — `Rule:` → code (v1)

A `Rule:` maps to a **lightweight in-file grouping**, not to a nested language
construct. Two parts, both language-uniform:

1. A **`// Rule: <name>` comment block** above the group of tests that came from
   that rule.
2. A **shared method-name (or test-title) prefix** on every test under the rule,
   so the grouping is visible in test output and the file reads top-to-bottom as
   "rule, then its examples".

There is **no structural nesting** in v1 — no Java `@Nested`, no nested C# class,
no native TS `describe(...)`. This is deliberate: the naming-prefix convention
composes trivially with the existing channel-parameterization wrappers and
matches the repo's flat house style. Structural nesting is a possible **v2**,
gated on first proving each language's nesting composes with the channel wrapper.

The grouping is additive: it sits *alongside* the test's existing
channel-parameterization annotations/wrapper and the permanent WIP gate — it
never replaces them.

### Java (`@TestTemplate` + `@Channel`)

```java
// Rule: Shipping is charged at $0.10 per kg per unit
@TestTemplate
@Channel({ChannelType.UI, ChannelType.API})
void shippingPerKgPerUnit_singleItem() { ... }

@TestTemplate
@Channel({ChannelType.UI, ChannelType.API})
void shippingPerKgPerUnit_scalesWithQuantity() { ... }
```

### C# / .NET (`[Theory]` + `[ChannelData]`)

```csharp
// Rule: Shipping is charged at $0.10 per kg per unit
[Theory]
[ChannelData(ChannelType.UI, ChannelType.API)]
public async Task ShippingPerKgPerUnit_SingleItem(Channel channel) { ... }

[Theory]
[ChannelData(ChannelType.UI, ChannelType.API)]
public async Task ShippingPerKgPerUnit_ScalesWithQuantity(Channel channel) { ... }
```

### TypeScript (`forChannels(...)`)

```typescript
// Rule: Shipping is charged at $0.10 per kg per unit
forChannels(ChannelType.UI, ChannelType.API)(() => {
    test('shippingPerKgPerUnit — single item', async ({ scenario }) => { ... });
    test('shippingPerKgPerUnit — scales with quantity', async ({ scenario }) => { ... });
});
```

## Interaction with other AC features

- **`@isolated` stays scenario-scoped.** The `@isolated` tag attaches to an
  individual `Scenario:` inside a `Rule:`, exactly as it does for a flat
  scenario. Grouping does not change how the writer mirrors isolation onto the
  test — the tag's per-scenario semantics are unaffected by the enclosing
  `Rule:`.
- **`Background:` under `Rule:` is unsupported in v1.** A rule-scoped
  `Background:` block is explicitly out of scope; do not author one and do not
  rely on it. Per-scenario `Given` steps are the only supported setup.

## Refiner ↔ writer contract

- **`acceptance-criteria-refiner`** — **preserves** author-written `Rule:`
  grouping. It never flattens a rule into a bare scenario list, places every
  edited/added scenario under the correct `Rule:`, and applies its coverage
  rubric and `@isolated` tagging *within* rules. In v1 it does **not** invent
  new rules or auto-group existing flat scenarios.
- **`acceptance-test-writer`** — translates each `Rule:` into the grouping
  convention above (comment block + name prefix), composing with the channel
  wrapper. Scenarios under a rule become its grouped tests; flat ACs stay flat.
