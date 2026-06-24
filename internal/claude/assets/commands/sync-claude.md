Run the sync-all-claude-settings script to distribute Claude settings across all workspace repos.

Execute the following command and report the output:

```bash
bash "$(git rev-parse --show-toplevel)/scripts/sync-all-claude-settings.sh"
```

Report which repos were updated and whether settings were already in sync.
