This is a one-shot dispatch. Investigate, do the work, and exit.

Ticket: #${issue_num} "${issue_title}"
Phase: ${phase}
Phase doc: ${phase_doc}

When the work is done, do not summarise and do not commit — exit cleanly. The orchestrator drives compile, test runs, disabling, and commits as separate service tasks; the agent must never run `git commit`, `git add`, `gh issue close`, the compile commands, or the test commands.

---
