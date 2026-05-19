# Disable change-driven tests

Annotation reason format:

```
@Disabled("<TICKET-ID> - AT - <LOOP> - <PHASE>")
```

- **Separator:** ` - ` (space-hyphen-space) between every segment.
- **`<TICKET-ID>`:** verbatim from the tracker (e.g. `OPV-123`, `#42`, `SHOP-7`).
- **`AT`:** the cycle (Acceptance Test).
- **`<LOOP>`:** `RED` | `GREEN`.
- **`<PHASE>`:** `TEST` | `DSL` | `SYSTEM DRIVER` (uppercase; internal space allowed).

Examples:

- `@Disabled("OPV-123 - AT - RED - TEST")`
- `@Disabled("OPV-123 - AT - RED - DSL")`
- `@Disabled("OPV-123 - AT - RED - SYSTEM DRIVER")`
