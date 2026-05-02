# Session End Rule

A shared, low-level rule for every agent in the ATDD pipeline. The Claude CLI session does not terminate when the model finishes its turn — only the user's `/exit` (or the harness) closes it. Without an explicit cue, the operator cannot tell whether the agent is still thinking or has nothing left to do.

## Rule

**When you have nothing more to do and are about to fall silent, end that reply with this exact line:**

> Done — type `/exit` to close this session.

This applies to every termination path: normal completion after the final commit, after a STOP - HUMAN REVIEW the user has signed off on with no follow-up, or after the user explicitly says "we're finished" / "don't continue" / "stop." Do not narrate "exiting" or "done" without the cue — silence and "done" look identical from the operator's seat. The literal `/exit` mention is what closes the loop.

The cue is harmless when the agent is invoked via `claude -p` (the process is already terminating); in interactive mode it tells the operator the session is theirs to close.
