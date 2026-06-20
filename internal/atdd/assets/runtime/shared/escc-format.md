# External System Contract Criteria (ESCC) format

The `## External System Contract Criteria` ticket section is the spec for the
**inner / contract loop**, symmetric to how `## Acceptance Criteria` spec the
outer / acceptance loop. It names each external system the story crosses and
states its boundary behaviour in `Given` / `Then` form (no `When` — a contract
has no actor action; the `When` belongs to the Acceptance Criteria).

This file is the canonical, pinned vocabulary. Authors write it directly; the
two contract-test writers (`contract-test-writer`, `stub-fidelity-test-writer`)
are the only interpreters. `parse-ticket` stays dumb — it reads only the
section's *presence* and each `External System: <name>` line, and passes the
register bodies through verbatim.

## Shape

```
## External System Contract Criteria
External System: ERP
  Shared (stub + real):
    Given products Apple (1.00), Bread (2.50)
    Then ERP has products Apple (1.00), Bread (2.50)
  Stub only:
    Given products Apple (1.00), Bread (2.50)
    Then ERP has exactly products Apple (1.00), Bread (2.50)
    Given no products
    Then ERP has no products
```

- **`External System: <name>`** — one line per external system, named
  **exactly** as it is registered in `gh-optivem.yaml` (`external-systems:`).
  This is the routing signal: its presence opens the contract/stub room and the
  name feeds `validate-external-systems-registered`.
- Each system carries up to **two registers**. Either may be omitted; declare a
  register only when the boundary needs it.

## The two registers

- **`Shared (stub + real):`** — runs against **both** drivers (stub + real), so
  it may assert only what holds for both. The real external system is shared,
  seeded, and never reset, so this register is **weak / containment** — it
  proves the stub and the real *agree* on the operation + shape, nothing about
  the whole collection.
- **`Stub only:`** — runs against the **stub only** (fully controlled). This is
  the **strong / fidelity** register: it proves the stub is faithful to exactly
  what we staged, including exact-set and empty. Warranted when stub fidelity is
  non-trivial and otherwise unverified (collections, empty, exact-set); optional
  for a trivial per-SKU stub.

## Keyword vocabulary (pinned)

The **verb** is the assertion-kind signal; the **sub-header** is the driver-set
signal. No machine tags or comments are required — any `#` notes in a ticket are
illustrative, not parsed.

| Line | Meaning |
|------|---------|
| `Given products <A> (<price>), <B> (<price>)` | stage these products |
| `Given no products` | stage empty |
| `Then <System> has products <…>` | **containment** assertion — Shared register (weak); each named entity is *present* |
| `Then <System> has exactly products <…>` | **exact-set** assertion — Stub-only register (fidelity) |
| `Then <System> has no products` | **empty** assertion — Stub-only register (fidelity) |

The `Then` pins the **shape** the feature depends on (e.g. `id + price`), not
the full external payload.

## Register → writer

- **Shared (stub + real)** → `contract-test-writer`. Containment, by key, both
  drivers. Its absolute by-key invariant (never exact-set / empty /
  whole-collection against the shared real system) is unchanged.
- **Stub only** → `stub-fidelity-test-writer`. Exact-set (`has exactly`) and
  empty (`has no`), stub driver only.

Two writers, two coherent non-conditional invariants — the by-key safety rule
that protects the 90% (non-list) path is never negated conditionally.
