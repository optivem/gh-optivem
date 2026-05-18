# Enable change-driven tests

Strip `@Disabled` annotations matching this filter:

```
startsWith("<CURRENT-TICKET-ID> - AT - RED - <PREV-PHASE>")
```

- **`<PREV-PHASE>`:** `TEST` | `DSL` | `SYSTEM DRIVER` (uppercase; internal space allowed).

**Never strip annotations whose prefix belongs to a different ticket.**
