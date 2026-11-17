# 2026-11-17 14:47:43 UTC — Greenfield use case modelling: from a product idea to a first use case model

> Sibling of `20261117-1547-local-html-visualizations.md` (the docs site there renders what this produces). Like that plan, sequence this after `characterize` is proven end-to-end, since this reuses its agents' output format and approval pattern.

## TL;DR

**Why:** gh-optivem has an entry point for a legacy system (`characterize` derives the use case model from the code) and for a story on an existing model (`implement` + `use-case-merger`). It has none for a new product. Someone with an idea like "a scheduling system for hotels" has to write the use case model by hand before `implement` can start.
**End result:** a command takes a one-line product idea, asks the operator clarifying questions, and writes an approved use case diagram, narratives for the first few core use cases, and optionally the first story tickets, all in the same `docs/use-cases` format as `characterize`. `implement` can then start straight away.

## Outcomes

- A new command (name open, e.g. `envision`) takes a product idea as free text.
- The agent asks clarifying questions, because the operator is the source of truth rather than the code ("Do guests book themselves or only staff? Is there payment?").
- **Use case diagram**: the full product model with actors, use cases (`UC-NNN`) and external systems as secondary actors, plus `dependencies.md`, in the same format as `legacy-use-case-diagram-writer`. One approval gate, where a "no" asks what should change and redoes the step, as in `characterize`.
- **UX/UI mockups before the narratives**: after the diagram is approved, the agent builds a clickable static HTML mockup of the main screens, one flow per use case for each human actor. It uses sample data, with no backend and no Docker. An approval gate follows, in the same approve-or-redo shape as the diagram. Showing screens first catches misunderstandings while they are still cheap to fix, because the operator reacts to something concrete instead of reading prose. The approved mockups are kept in the repo, e.g. under `${use-cases-path}/mockups/`. The narratives are then written against them, so a narrative step such as "Staff choose Check out" names a screen that exists, and the `ui` channel's acceptance tests and drivers later have a reference for the intended screens.
- **All narratives up front.** They are text only, with no Docker and no builds, so writing them all is cheap. Seeing the whole model helps get the dependency order right, and rewriting a narrative later is cheap when an agent does it. The operator approves the diagram and skims the narratives.
- **Shaped like `characterize`, minimising total development time**:
  - Pilot the most important use case, and the ones it depends on, with full approvals. It sets the test architecture.
  - Implement the rest in one unattended batch session, in dependency order.
  - Write criteria just before each use case's tests.
- **Rebuild cost**: unlike legacy, every Green attempt changes system code, so a rebuild per Green attempt is unavoidable. Batching saves everything around it:
  - Rebuild only the system containers, keeping the database, stubs and simulators and Keycloak warm. This is the counterpart of `system start --restart --external-only`.
  - Rebuild only when system code changed: not after the test, DSL or driver steps.
  - Iterate inside Green on the Docker-free inner loop (see `../20261103-1335-tierspec-inner-loop-component-test.md`), with one acceptance rebuild at the end.
  - Hot reload is an option only (fragile, differs per language).
- Optionally, the first story tickets are created instead of running the batch, for operators who want story-by-story.
- The output feeds `implement` and the docs site unchanged. From here on there is no difference between a greenfield and a legacy project.

## ▶ Next executable step (resume here)

Design only. Run `/refine-plan` on this file to settle the open questions, then execute Step 1.

## Steps

- [ ] Step 0: Measure before designing. From the `steps.jsonl` / `summary.jsonl` of past `implement` runs (e.g. in `shop/.gh-optivem/runs/`), work out the share of a story's elapsed time spent on stack start and stop, system rebuilds, agent work and fix loops. This decides whether a system-only restart, the inner loop or batching is the main lever.
- [ ] Step 1: Compare with the legacy agents (`legacy-use-case-diagram-writer.md`, `legacy-use-case-narrative-writer.md`, `legacy-use-case-criteria-writer.md`). Decide per agent whether to generalise it (source = code or operator answers) or add a greenfield sibling, keeping one output format.
- [ ] Step 2: Design the clarifying-question loop: how the agent asks, how answers are recorded, and when it stops asking.
- [ ] Step 2b: Design the UX/UI mockup step:
  - the mockup agent's prompt and output format: static HTML, one folder per use case, shared styles
  - where the mockups live in the repo
  - how the narrative writer and later the `ui` channel test writer read them
  - the approval-and-redo gate
  - Decide whether a project with no human actors (API-only) skips this step.
- [ ] Step 3: Add the BPMN flow to `process-flow.yaml`: questions, diagram, approval, UX/UI mockups, approval, narratives, approval, optional story tickets. Reuse the characterize approval-and-redo pattern.
- [ ] Step 4: Wire the command and its flags, and refuse on `kind: component` if appropriate.
- [ ] Step 5: Add tests and document the command in `docs/cli-reference.md` and `--help`.

## Open questions

- UX/UI mockups: one mockup per use case, or one per actor journey across several use cases? Recommended: one per actor journey, since that is how the operator thinks about screens. Each screen is tagged with the `UC-NNN` it serves.
- Should the mockups stay in sync as the real UI is built (and if so, who updates them), or are they frozen as the design record? Recommended: frozen as the design record, with the real UI and its `ui` acceptance tests taking over as the source of truth once built.

- Command name: `envision`, `model`, `discover`, or a mode of an existing command?
- Is a system-only restart a new flag (e.g. `--system-only`) on `system start --restart`, or should the batch driver decide what to restart on its own?
- Should story tickets be created through the tracker abstraction (GitHub and Jira), and should that step be optional?
- Does this run before `init` (shaping the scaffold, e.g. external systems) or after it, on an already-scaffolded project?
