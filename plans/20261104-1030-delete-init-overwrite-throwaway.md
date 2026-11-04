# 2026-11-04 10:30 — Delete the `init-overwrite-throwaway` repo and its SonarCloud projects

## TL;DR

**Why:** The Step 10 attended check for init-into-existing-repo left a private throwaway repo and two SonarCloud projects behind. The repo's hourly scheduled `acceptance-stage*` workflows keep failing (no prior successful run for their `Get Last Workflow Run` check) and burn Actions minutes.
**End result:** `valentinajemuovic/init-overwrite-throwaway` and its two SonarCloud projects no longer exist, and the init-into-existing-repo plan is closed out.

## Next step (resume here)

Run the dry-runs in Step 1, then the real deletions in Steps 2-3. Existing tooling covers both, so no new script is needed: `gh optivem cleanup repos` and `gh optivem cleanup sonar-projects` (both support `--dry-run`).

## Steps

- [ ] Step 1: Dry-run both deletions and read the target list.
  - `gh optivem cleanup repos valentinajemuovic init-overwrite-throwaway --dry-run`
  - `SONAR_TOKEN=<token> gh optivem cleanup sonar-projects valentinajemuovic valentinajemuovic_init-overwrite-throwaway-system valentinajemuovic_init-overwrite-throwaway-system-test --dry-run`
- [ ] Step 2: Delete the repo. Needs the `delete_repo` scope, which the current token lacks: run `gh auth refresh -h github.com -s delete_repo` first (interactive, so the user types `! gh auth refresh -h github.com -s delete_repo`). Then run the Step 1 repos command without `--dry-run`. User approval required (irreversible).
- [ ] Step 3: Delete the two SonarCloud projects: the Step 1 sonar command without `--dry-run`. Needs `SONAR_TOKEN` in the environment (generate at https://sonarcloud.io/account/security). User approval required (irreversible).
- [ ] Step 4: Verify both are gone (`gh repo view valentinajemuovic/init-overwrite-throwaway` returns not found; the Sonar project keys 404), then delete `plans/20261103-1100-init-into-existing-repo.md` (its only remaining step is this cleanup) and this file.
- [ ] Optional: drop the `delete_repo` scope afterwards (`gh auth refresh -h github.com -r delete_repo`).

## Decisions

- Reuse `gh optivem cleanup` rather than adding a script; it already has `--dry-run` and explicit-name targeting.
