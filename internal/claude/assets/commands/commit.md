Run `gh optivem commit` to commit, pull, and push repos in the academy workspace.

Execute the following command and report the output:

```bash
gh optivem commit --yes $ARGUMENTS
```

`$ARGUMENTS` supports:
- `"message"`: commits all dirty repos with the given message
- `--repo <name> "message"`: commits only the named repo with the given message
- `--repo <name> --paths "<paths>" "message"`: stages only specific paths under the named repo

A commit message is required when any iterated repo has dirty changes. Add `--include-untracked` to also stage untracked files (otherwise the command refuses them under `--yes`).

Report which repos were committed, how many were synced, and how many were skipped.
