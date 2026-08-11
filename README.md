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
