# Create a ticket

Before running `gh optivem implement`, create the GitHub issue it will work from. See the [README](../README.md) for the actual day-to-day workflow — creating a project and running the ATDD loop on it.

Create a ticket in your repository. It needs a description and Acceptance Criteria written as Gherkin scenarios — copy [optivem/shop#72](https://github.com/optivem/shop/issues/72) as a worked example of the shape the agents expect.

Then **add it to the GitHub Project board `init` created**, or `implement` fails with `issue #N not found on project`. That board's URL is in your repo's `gh-optivem.yaml` under the top-level `project:` key, shaped `https://github.com/users/<login>/projects/<number>`, or `/orgs/<org>/projects/<number>` for an organization.

Add the issue to it:

```bash
gh project item-add <number> --owner <login|org> --url https://github.com/<owner>/<repo>/issues/<issue_number>
```

If that comes back *missing required scopes*:

```bash
gh auth refresh -s project
```

Once the ticket is created and on the board, head back to the [README](../README.md) to run `gh optivem implement`.
