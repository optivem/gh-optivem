# Disable change-driven tests

Annotation reason format:

```
@Disabled("<TICKET-ID> - AT - <LOOP> - <PHASE>")
```

- **Separator:** ` - ` (space-hyphen-space) between every segment.
- **`<TICKET-ID>`:** verbatim from the tracker (e.g. `OPV-123`, `#42`, `SHOP-7`). Leads so the re-enable step can filter `startsWith("<TICKET-ID> - ")` and ignore tests belonging to other tickets.
- **`AT`:** the cycle (Acceptance Test). Reserves the slot for `CT` (Contract Test) under the same convention later.
- **`<LOOP>`:** `RED` | `GREEN`. Currently only `RED` uses disable; the slot is reserved for schema regularity.
- **`<PHASE>`:** `TEST` | `DSL` | `SYSTEM DRIVER` (uppercase; internal space allowed).

Examples:

- `@Disabled("OPV-123 - AT - RED - TEST")`
- `@Disabled("OPV-123 - AT - RED - DSL")`
- `@Disabled("OPV-123 - AT - RED - SYSTEM DRIVER")`

**Phase agents must not annotate `@Disabled` themselves.** This bookkeeping is handled outside the phase agent by the runtime's `disable_change_driven` action, which runs between phases.
