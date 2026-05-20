# Session End Rule

A shared rule for every ATDD agent. The Claude CLI session doesn't terminate when the model finishes its turn — only `/exit` (or the harness) closes it, so without an explicit cue the operator can't tell whether the agent is thinking, waiting, or done.

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

Do **not** paraphrase, reorder, drop the bold labels, replace with a plain "Done"/"/exit" line, or add extra options. The wording is fixed because the operator (and downstream tooling) keys off it. Don't narrate "exiting" or "done" without the block — silence and "done" look identical from the operator's seat; the literal `/exit` in Option 1 is what closes the loop.

The footer is harmless when the agent is invoked via `claude -p` (the process is already terminating); in interactive mode it tells the operator the session is theirs to close or to continue with feedback.
