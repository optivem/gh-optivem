# gh-optivem Repo Guidelines

## Documentation: No GitHub Pages

Never add GitHub Pages scaffolding to this tool or to the repos it creates. Scaffolded repos expose docs as plain markdown under `docs/` and link to them from the README using relative paths (e.g. `[Architecture](docs/architecture.md)`). GitHub renders markdown and Mermaid natively on github.com, which is sufficient for the student audience and avoids the Pages workflow, `id-token: write` permissions, and deploy-time failures.

Concretely, do not reintroduce:

- An `EnablePages` step or shell wrapper calling `gh api repos/{owner}/{repo}/pages`.
- A `pages.yml` workflow in scaffolded repos or in the `shop` template.
- A `Docs:` URL pointing to `{owner}.github.io/{repo}/` in the summary or project-registration output.
- A `build_type=workflow` Pages API call.

If you find yourself proposing any of the above, stop and reconsider — the answer is always README + `docs/*.md` links.

## `system_test.paths:` — scaffold-authoritative, not "default"

`gh optivem init` writes `system_test.paths:` from `projectconfig.DefaultPaths` (called from `internal/steps/optivem_yaml.go::BuildOptivemYAML`). This is the **only** place the binary writes a `paths:` block, and it is correct: the scaffolder owns both the YAML and the directory tree the YAML points at, so the values are authoritative initial values matching the just-created tree — not runtime defaults.

After `init`, the block is operator-owned everywhere else:

- `projectconfig.Validate` Rule 22a hard-errors on missing/non-canonical `system_test.paths:` keys instead of back-filling defaults.
- `gh optivem config migrate` no longer back-fills a `paths:` block.

Do not "fix" the apparent inconsistency by ripping out the `DefaultPaths` call in `BuildOptivemYAML`, and do not generalise `DefaultPaths` into a validate-time or migrate-time fallback. See `internal/projectconfig/path-keys.md` ("Ownership: scaffold-authoritative at `init`, operator-owned afterwards") for the full doctrine.
