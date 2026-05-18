# Enable change-driven tests

At the start of each RED sub-phase, `@Disabled` annotations from the previous phase are stripped using this filter:

```
startsWith("<CURRENT-TICKET-ID> - AT - RED - <PREV-PHASE>")
```

**Never strip annotations whose prefix belongs to a different ticket.** The leading `<TICKET-ID>` segment exists precisely so concurrent tickets don't disturb each other.

The annotation schema being matched is defined in [disable-tests](disable-tests.md).

**Phase agents must not strip `@Disabled` themselves.** This bookkeeping is handled outside the phase agent by the runtime's `enable_change_driven` action, which runs between phases.
