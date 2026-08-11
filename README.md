[![gh Commit Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-commit-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-commit-stage.yml)
[![gh Acceptance Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-acceptance-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-acceptance-stage.yml)
[![gh Release Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-release-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-release-stage.yml)

# gh-optivem

A GitHub CLI extension that scaffolds a ready-to-go system test project with a GitHub Actions pipeline — a complete, working project from the first run.

You then use it to implement your GitHub tickets. Write a User Story with Acceptance Criteria as an issue, and `gh optivem` takes it from start to end: RED (acceptance tests, DSL and drivers) through GREEN (backend and frontend implementation).

## Install

```bash
gh extension install optivem/gh-optivem
```

Check that it worked:

```bash
gh optivem --version
```

## Environment variables

`gh optivem init` reads these credentials from your local environment and sets them as secrets and variables on the repos it creates, so the generated pipelines can pull base images, publish to GHCR, and scan with SonarCloud.

Set them on your machine the usual way, then restart your IDE / terminal — the environment snapshot is taken when the process launches, so a shell opened before you set them won't see them.

- `DOCKERHUB_USERNAME` — your Docker Hub username.
- `DOCKERHUB_TOKEN` — Docker Hub Personal Access Token (read-only scope is enough). Create at https://app.docker.com/settings/personal-access-tokens.
- `SONAR_TOKEN` — SonarCloud token. Create at https://sonarcloud.io/account/security.
- `GHCR_TOKEN` — GitHub PAT (classic) with `write:packages` + `read:packages`. Create at https://github.com/settings/tokens, selecting **No expiration**.
- `WORKFLOW_TOKEN` — GitHub PAT (classic) with `repo` + `workflow` scopes. Create at https://github.com/settings/tokens, selecting **No expiration**.
- `REPO_TOKEN` — GitHub PAT with `repo` scope (classic) or `Contents: Read` on each component repo (fine-grained). Create at https://github.com/settings/tokens or https://github.com/settings/personal-access-tokens, selecting **No expiration** for the classic form.

The three GitHub PATs (`GHCR_TOKEN`, `WORKFLOW_TOKEN`, `REPO_TOKEN`) back cron-scheduled pipelines that keep running indefinitely once scaffolded — a classic PAT with an expiration date will eventually lapse and start failing the schedule silently. "No expiration" avoids the rotation chore entirely for these specific tokens.

To confirm what your shell is actually exporting (token values masked):

```bash
gh optivem environment show
```

To live-check each token is also accepted by its provider before scaffolding:

```bash
gh optivem environment verify
```

## Generate your project

`gh optivem init` creates your GitHub repository, applies the project template, sets up the pipeline and waits for the pipeline to wait.

Run it with your project's values:

```bash
gh optivem init --owner <username|organization> --repo <repo_name> --system-name "<system_name>" --arch multitier --repo-strategy <monorepo|multirepo> --backend-lang <java|dotnet|typescript> --frontend-lang typescript --test-lang <java|dotnet|typescript>
```

For example:

```bash
gh optivem init --owner valentinajemuovic --repo book-shop --system-name "Book Shop" --arch multitier --repo-strategy monorepo --backend-lang dotnet --frontend-lang typescript --test-lang java
```

## Verify your setup

Once `init` completes, check the status badges in your new repository's README. All badges should be green.

Then, from your project's repo root, run one sample test per suite against a locally started system:

```bash
gh optivem system start
gh optivem system-test run --sample
gh optivem system stop
```

This confirms that your toolchain, Docker, and system startup are all working. It should work out of the box.

## Everything else

See the [full CLI reference](docs/cli-reference.md) — credentials, every `init` flag, `gh optivem implement` (the day-to-day verb that walks a ticket through the ATDD pipeline), the system and test runners, cross-repo ops, and cleanup.

## Upgrade

```bash
gh extension upgrade optivem
```

## Uninstall

```bash
gh extension remove optivem
```

## Support

Something went wrong? [Open an issue](https://github.com/optivem/gh-optivem/issues).

If `gh optivem init` itself fails, add `--report-bug` and it files the issue for you, with your scaffold config and log file attached:

```bash
gh optivem init --report-bug ...
```

Either way, a run always writes a plain-text log to `$TEMP/gh-optivem-<timestamp>.log` — attach it to the issue.

## Maintainer

Maintained by Valentina Jemuović at Optivem — [GitHub](https://github.com/valentinajemuovic) · [LinkedIn](https://www.linkedin.com/in/valentinajemuovic/).

## License

[MIT](LICENSE) © Optivem
