# CT - GREEN - STUBS

## Purpose

Bring the dockerized External System stub into contract-compatibility with the real Test Instance, so the stub-suite contract tests pass. This is the only GREEN phase in the CT sub-process — it closes the loop opened by CT - RED - TEST.

## What it produces

- After WRITE: stub server changes (routes, fixtures) and the re-enabled contract tests exist in the working tree.
- Tests in state: contract tests enabled and PASSING against the dockerized stub.

## Conventions

- The dockerized stub follows the json-server pattern — see [`external-systems/simulators`](../../../external-systems/simulators) for the canonical reference (`mock-server.js`, `Dockerfile`).
- Stub data must reflect the real Test Instance's contract — same shapes, same status codes, same error semantics. Drift between stub and real instance breaks the CT cycle.
- `@Disabled` removal syntax per language: see [language-equivalents/](../code/language-equivalents/README.md).

## Example

A new stub route added to `mock-server.js` to honor a contract method that previously returned 404.

```javascript
// Promotion endpoint - returns default no-promotion state
server.get('/erp/api/promotion', (req, res) => {
  res.status(200).json({
    promotionActive: false,
    discount: 1.0
  });
});
```

## CT - GREEN - STUBS - WRITE

1. Enable the tests marked disabled with reason `"CT - RED - EXTERNAL DRIVER"`.
2. Implement the dockerized External System stub changes — add or update routes, fixtures, or middleware so the stub honors the new contract.

## Anti-patterns

- Forgetting to remove the `@Disabled` (reason `"CT - RED - EXTERNAL DRIVER"`) — the tests look passing locally but are silently skipped in CI.
- Modifying the real Test Instance instead of the stub — the real instance is owned by the external system, not by this repo.
- Letting stub data drift from the real-instance contract — if the stub returns shapes the real instance would not, the contract tests stop being meaningful. Mirror the real instance.
- Adding "test-only" branches to the stub that the real instance does not honor — same drift problem, harder to spot.
