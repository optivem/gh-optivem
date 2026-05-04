# Session End Rule

A shared, low-level rule for every agent in the ATDD pipeline. The Claude CLI session does not terminate when the model finishes its turn — only the user's `/exit` (or the harness) closes it. Without an explicit cue, the operator cannot tell whether the agent is still thinking, waiting for input, or has nothing left to do.

## Rule

**When you have nothing more to do and are about to fall silent, end that reply with this exact block, verbatim — same wording, same order, same Markdown:**

> **Next steps.** Please review the response above and choose one of the following:
>
> - **Option 1.** If you approve, no further action is required — type `/exit` to close this session.
> - **Option 2.** If you want something changed, reply with feedback describing what to change.

This is the **common footer for every ATDD agent**. It applies to every termination path:

- Normal completion after the final commit.
- A STOP - HUMAN REVIEW gate where you have presented work and are waiting for approval.
- The user has explicitly said "we're finished" / "don't continue" / "stop."
- You cannot proceed and are reporting why.

Do **not** paraphrase, shorten, expand, or reorder the block. Do **not** drop the bold labels. Do **not** replace it with a plain "Done" or "/exit" line. Do **not** add extra options. The wording is fixed because the operator (and any downstream tooling) keys off it to know the turn is over.

Do **not** narrate "exiting" or "done" without the block — silence and "done" look identical from the operator's seat. The literal `/exit` mention inside Option 1 is what closes the loop.

The footer is harmless when the agent is invoked via `claude -p` (the process is already terminating); in interactive mode it tells the operator the session is theirs to close or to continue with feedback.
