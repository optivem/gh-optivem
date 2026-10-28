<!-- gh-optivem-spec -->
### Spec coverage

Spec coverage: 0 of 14 acceptance criteria verified (0.0%)

**Gate failed** (mode strict, threshold 80.0%)

| State | Criteria |
|---|---:|
| Verified | 0 |
| In progress | 1 |
| Failing | 2 |
| Planned | 10 |
| Waived | 1 |

<details><summary>Criteria not yet verified</summary>

- `UC-001.1` Failing: A valid order is placed and gets status PLACED with an ORD- prefixed number.
- `UC-001.2` Failing: Base price = unit price x quantity.
- `UC-001.3` In progress: Coupon discount applies; no coupon means no discount; subtotal = base - discount.
- `UC-001.4` Planned: Tax rate follows country; total = subtotal + tax.
- `UC-001.5` Waived: Coupon usage count increments after use. _(waived until 2026-12-31 by vj: Payment &lt;sandbox&gt; down | later)_
- `UC-001.6` Planned: Placement timestamp is recorded.
- `UC-001.7` Planned: Promotion price rules (full price without promotion, discount when active).
- `UC-001.8` Planned: Non-positive / non-integer / empty / null quantity is rejected (narrative 4a).
- `UC-001.9` Planned: Unknown SKU or empty / null SKU is rejected (narrative 5b).
- `UC-001.10` Planned: Empty / null / invalid country is rejected.
- `UC-001.11` Planned: Non-existent, expired or over-used coupon is rejected.
- `UC-001.12` Planned: Admin cannot place an order.
- `UC-001.13` Planned: Order rejected at year end.
- `UC-001.14` Planned: ERP unavailable message (narrative 5a). _(baselined)_

</details>

### Gate reasons

- validation error (duplicate-id): UC-001.9 is defined twice
- UC-001.1 is Failing
- UC-001.2 is Failing
- UC-001.3 is In progress (strict requires Verified)
- UC-001.4 is Planned (strict requires Verified)
- UC-001.6 is Planned (strict requires Verified)
- UC-001.7 is Planned (strict requires Verified)
- UC-001.8 is Planned (strict requires Verified)
- UC-001.9 is Planned (strict requires Verified)
- UC-001.10 is Planned (strict requires Verified)
- UC-001.11 is Planned (strict requires Verified)
- UC-001.12 is Planned (strict requires Verified)
- UC-001.13 is Planned (strict requires Verified)
- coverage 0.0% is below threshold 80.0%

### Spec validation errors

- `duplicate-id`: UC-001.9 is defined twice

### Orphan tests

- `com.shop.PlaceOrderTest.shouldHaveNoTag`: no criterion ID
- `com.shop.PlaceOrderTest.shouldDoGhost`: no known criterion ID

### Tests referencing unknown criterion IDs

- `UC-999.1` in `com.shop.PlaceOrderTest.shouldDoGhost`
