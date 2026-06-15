Wrap the scenarios in an `@isolated` describe block configured for serial mode — the `mode: 'serial'` is what runs them one at a time:

```typescript
test.describe('@isolated', () => {
    test.describe.configure({ mode: 'serial' });
    forChannels(ChannelType.UI, ChannelType.API)(() => {
        test('shouldXxx', async ({ scenario }) => { ... });
    });
});
```
