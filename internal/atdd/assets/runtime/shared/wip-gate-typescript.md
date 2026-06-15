Add the gate as the first statement in each Acceptance Test body, keeping the surrounding `forChannels(...)` wrapper and the `test('shouldXxx', async ({ scenario }) => { ... })` declaration exactly as written:

```typescript
forChannels('ui', 'api')(() => {
  test('shouldXxx', async ({ scenario }) => {
    test.skip(process.env.GH_OPTIVEM_RUN_WIP_TESTS !== "1", "Work-in-progress test; set GH_OPTIVEM_RUN_WIP_TESTS=1 to run");
    ...
  });
});
```

This is Playwright's runtime `test.skip(condition, description)` overload — different from the definition-time `test.skip(title, body)` overload. No import change. Do not remove or replace the `forChannels(...)` wrapper — the gate is purely additive.
