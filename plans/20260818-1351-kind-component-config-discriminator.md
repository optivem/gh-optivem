# 2026-08-18 13:51:57 UTC — `kind:` discriminator: let a project be a component, not a system

## TL;DR

**Why:** `system.architecture` silently doubles as the "is this a whole SUT?" discriminator — `Validate()`'s own comment calls it *"scope"*. Because that flag lives **inside** the `system:` block, a project that has no system has nowhere to say so. Setting `architecture` triggers a cascade demanding `system-test.{path,repo,lang}`, the canonical `system-test.paths.*` keys, `system.db-migration-path`, a driver-adapter path per channel, `sonar.organization` and per-tier `sonar-project`; leaving it unset makes `gh optivem system compile` hard-fail with *"has no system.architecture set"*. A component-only project is unexpressible in either direction.

**End result:** `gh-optivem.yaml` carries a top-level `kind:` (`system` | `component`, defaulting to `system` when absent, so every existing config keeps working untouched). `kind: component` declares one unit of code — name, path, repo, lang — and nothing else, and the commands that need a booted SUT refuse it with a specific message instead of a cascade of irrelevant validation errors. A team that owns only their component, with no access to or knowledge of the wider system, can use the tool.

## Outcomes

What we get out of this — the goals and deliverables:

- A single-component team can run `gh optivem compile`, `gh optivem component-test compile`, and `gh optivem component-test run` against a repo that contains one backend and no system, no docker stack, no system-test project, and no frontend.
- The discriminator is explicit and top-level. `kind:` answers "what is this project?"; `system.architecture` goes back to answering only "what shape is the system?" — one field, one job.
- Commands that genuinely require a SUT (`system start/stop/build/compile`, `system-test setup/run`) refuse a `kind: component` project with a message naming the kind and the reason, rather than today's "system-test.paths.* required" on a project that has no system tests.
- Zero migration cost: absent `kind:` means `system`. All twelve `shop` configs, and every repo already scaffolded from the template, are unaffected with no edit.
- `system/multitier/backend-clean-java` becomes expressible as an honest config. It is already a component-only project in everything but the config — its commit-stage workflow runs `test`, `componentTest`, `integrationTest`, `contractTest`, and `checkstyleAll` with no system tier at all — and it becomes the first real user of `kind: component`.
- The gap that motivated this is closed by construction rather than by a second registration mechanism: `shop`'s `compile-all.sh` picks the new config up through its existing `gh-optivem-*.yaml` glob, so the script's stated contract ("drop a new yaml — no changes to this script") stays true. See `shop`'s `plans/20260818-1326-compile-all-misses-backend-clean-java.md`.
- A stated position on what ATDD means for a team that cannot see the system: the inner loop is unchanged, and the contract replaces the outer loop.

## Background — what is actually coupled, and what is not

Evidence gathered from the source, so execution does not have to re-derive it:

**The two coupling points.**

1. `compile_commands.go:160` — `compileSystem` dispatches on `cfg.System.Architecture` and has `case "": return fmt.Errorf("compile system: %s has no system.architecture set", ...)`. No architecture, no compile.
2. `config.go:812` — Rule 7: `if c.System.Architecture != "" && c.SystemTest.IsEmpty()` → error. This is the head of the cascade; Rules 17/18 (`sonar.organization`, per-tier `sonar-project`), Rule 22b (`system-test.paths.*`), the `db-migration-path` rule, and the channel adapter rule all key off the same `Architecture != ""` condition.

Together these are a deadlock: to compile you must set `architecture`; to set `architecture` you must describe a whole SUT you do not have.

**What is already component-shaped and needs no change.**

- `Config.Validate()` is lenient by design — `requireFullTier` (`config.go:1249`) returns `nil` for an entirely empty tier and only rejects a *partially* filled one. A backend-only config already validates, as long as `architecture` is unset.
- `discoverComponents` (`component_commands.go:165`) already tolerates a backend with no frontend: it appends the frontend only `if sys.Frontend.Path != ""`. The component-test tier is already component-scoped.
- `componenttest.Component{Name, Path, Lang}` is exactly the shape a `kind: component` config needs to express. The type exists; this plan surfaces it in the schema rather than inventing a new one.

**What does need changing beyond the two coupling points.**

- `compileSystem`'s multitier branch compiles Backend **then Frontend unconditionally** — no skip for an absent frontend.
- `newCompileCmd`'s Run walks system → component-test → system-test in sequence and would need a kind-aware walk.
- `compileSystemTests` errors when `SystemTest.IsEmpty()`, which is the normal state for a component.

## Proposed shape

```yaml
kind: component
project:
  provider: github
component:
  name: backend-clean-java
  path: system/multitier/backend-clean-java
  repo: optivem/shop
  lang: java
```

versus today's config, which gains one optional line and changes in no other way:

```yaml
kind: system      # optional; this is the default when absent
system:
  architecture: multitier
  ...
```

**Command surface by kind:**

| Command | `kind: system` | `kind: component` |
|---|---|---|
| `compile` | all tiers | the one component |
| `component-test compile` / `run` | ✅ | ✅ |
| `system compile` / `start` / `stop` / `build` | ✅ | ❌ refuse — no stack to boot |
| `system-test setup` / `run` | ✅ | ❌ refuse — no SUT to drive |
| `test` (aggregate) | all tiers | component-test only |
| `config` / `doctor` / `commit` / `sync` / `actions` / `branch` / `pr` | ✅ | ✅ — kind-agnostic |
| `config init` / `config migrate` | ✅ | ✅ — prompts for / writes `kind` |
| `init` (scaffold) | scaffolds a SUT | ❌ refuse — SUT-only by definition |
| `implement` | ✅ | ❌ refuse — drives the outer loop |

## The process question this raises

ATDD's outer loop **is** the system loop. A team that cannot see the system cannot write acceptance tests against it. So `kind: component` is not merely a smaller config — it describes a different development process:

- **inner loop** — unit → component → integration tests. Fully available, unchanged.
- **outer loop** — replaced by a **contract**. Consumer-driven contract testing is precisely the "I cannot see the system, but I know what I owe it" mechanism.

`kind: component` therefore means *inner loop + contract boundary, no SUT*.

**This plan states the position and stops there.** Designing a contract-driven outer loop as an actual process flow is its own plan — it needs a design pass on what the flow looks like, not a step tacked onto a config change. Here, `implement` simply refuses `kind: component`: the ATDD driver walks a flow whose outer loop does not exist for a component project, and refusing is honest where running a half-applicable flow is not.

## ▶ Next executable step (resume here)

Open questions are all resolved — this plan is ready to execute as a config/CLI change, with the process-flow work explicitly out of scope.

Start at Step 1: add the `Kind` string field (`yaml:"kind,omitempty"`) and the `Component` block to `Config` in `internal/kernel/projectconfig/config.go`, with enum validation (`system` | `component`), a `KindOrDefault()` accessor so no call site compares against `""`, and a unit test asserting that an existing `shop` config with no `kind:` validates exactly as it does today. That unblocks Steps 2–6, which all dispatch on the new field.

## Steps

- [ ] Step 1: Add `Kind string \`yaml:"kind,omitempty"\`` and a `Component` block to `Config` (`internal/kernel/projectconfig/config.go`). Enum-validate `kind` against `system` | `component`; absent means `system`. Add a `KindOrDefault()` accessor so no call site compares against `""`.
- [ ] Step 2: Split `Validate()` by kind. Move the existing `Architecture != ""` cascade (Rules 7, 17, 18, 22b, db-migration, channel adapters) under `kind: system`. Add the `kind: component` rules: `component.{name,path,repo,lang}` all required; `system`, `system-test`, `channels`, and `external-systems` rejected with a message saying they belong to `kind: system`.
- [ ] Step 3: Make `compileSystem` kind-aware (`compile_commands.go`). For `kind: component`, compile the single component via `compiler.Compile`. Keep the `case "":` architecture error for `kind: system` only. Separately, make the multitier branch skip an empty frontend rather than compiling `""` — that is a latent bug independent of this plan.
- [ ] Step 4: Make the `compile` aggregate walk kind-aware — for `kind: component`, run component-test compile and skip the system-test phase instead of erroring on `SystemTest.IsEmpty()`. Mirror the same for the `test` aggregate.
- [ ] Step 5: Make `discoverComponents` return the single declared component for `kind: component` (it already handles backend-only for `kind: system`; this adds the new branch).
- [ ] Step 6: Add kind guards to the SUT-only commands — `system compile/start/stop/build`, `system-test setup/run`, `init` (the scaffold verb), and `implement` — failing with a specific message naming the kind, the command, and why it needs a system. One shared helper, not a guard copy-pasted per command. `init` and `implement` are permanent refusals, not stopgaps: `init` scaffolds a whole SUT from the template, and `implement` drives a process flow whose outer loop does not exist for a component.
- [ ] Step 7: Update `gh optivem config init` and `config migrate` — note these are the *config* verbs, distinct from the `init` scaffold verb guarded in Step 6. `config init` prompts for kind (or takes `--kind`) and can author a `kind: component` config; `config migrate` writes `kind: system` explicitly into existing configs so the default stops being implicit over time.
- [ ] Step 8: Update `doctor` to report the project's kind and to skip system-tier checks for `kind: component`.
- [ ] Step 9: Docs — the config reference, the `kind` semantics, the command-by-kind matrix, and the "contract replaces the outer loop" position from the section above.
- [ ] Step 10: Tests — an existing shop config validates unchanged with no `kind:`; a `kind: component` config validates, compiles, and runs component tests; every SUT-only command refuses `kind: component` with the expected message; a `kind: component` config that declares `system:` or `system-test:` is rejected.
- [ ] Step 11: Prove it end-to-end against a real project — add `gh-optivem-multitier-clean-java.yaml` to `shop` as `kind: component` pointing at `system/multitier/backend-clean-java`, and confirm `shop`'s existing `compile-all.sh` glob picks it up with no script change. (Executed in the `shop` repo; commit separately there.)

## Decisions taken (resolved before execution)

1. **`init` for `kind: component`** — hard error. Scaffolding a component is not a deferred feature: `init` creates a whole SUT from the template and is `kind: system` only, permanently. A component config is authored by `config init` or by hand.
2. **Process flow** — separate plan. This plan states the position (contract replaces the outer loop) and refuses `implement` for `kind: component`; designing a contract-driven flow is its own design pass.
3. **Components per config** — one. Mirrors `kind: system` being one config, one system. Multi-component is `repos:` territory or a config per component.
4. **Field name** — `kind`, matching the Kubernetes/CRD idiom; `type` collides with `lang` and with the existing `external-systems.real-kind`.
