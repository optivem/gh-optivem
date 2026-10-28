<!-- gh-optivem-spec -->
### Spec coverage

Spec coverage: 2 of 14 acceptance criteria verified (14.3%)

**Gate passed** (mode lenient, threshold 100.0%)

| State | Criteria |
|---|---:|
| Verified | 2 |
| In progress | 0 |
| Failing | 0 |
| Planned | 12 |
| Waived | 0 |

<details><summary>Criteria not yet verified</summary>

- `UC-001.3` Planned: Coupon discount applies; no coupon means no discount; subtotal = base - discount.
- `UC-001.4` Planned: Tax rate follows country; total = subtotal + tax.
- `UC-001.5` Planned: Coupon usage count increments after use.
- `UC-001.6` Planned: Placement timestamp is recorded.
- `UC-001.7` Planned: Promotion price rules (full price without promotion, discount when active).
- `UC-001.8` Planned: Non-positive / non-integer / empty / null quantity is rejected (narrative 4a).
- `UC-001.9` Planned: Unknown SKU or empty / null SKU is rejected (narrative 5b).
- `UC-001.10` Planned: Empty / null / invalid country is rejected.
- `UC-001.11` Planned: Non-existent, expired or over-used coupon is rejected.
- `UC-001.12` Planned: Admin cannot place an order.
- `UC-001.13` Planned: Order rejected at year end.
- `UC-001.14` Planned: ERP unavailable message (narrative 5a).

</details>
