# gh-optivem Repo Guidelines

## Documentation: No GitHub Pages

Never add GitHub Pages scaffolding to this tool or to the repos it creates. Scaffolded repos expose docs as plain markdown under `docs/` and link to them from the README using relative paths (e.g. `[Architecture](docs/architecture.md)`). GitHub renders markdown and Mermaid natively on github.com, which is sufficient for the student audience and avoids the Pages workflow, `id-token: write` permissions, and deploy-time failures.

Concretely, do not reintroduce:

- An `EnablePages` step or shell wrapper calling `gh api repos/{owner}/{repo}/pages`.
- A `pages.yml` workflow in scaffolded repos or in the `shop` template.
- A `Docs:` URL pointing to `{owner}.github.io/{repo}/` in the summary or project-registration output.
- A `build_type=workflow` Pages API call.

If you find yourself proposing any of the above, stop and reconsider — the answer is always README + `docs/*.md` links.
