# 2026-06-18 10:46 (CEST) — Microservices: end-to-end scaffold pipeline

> Quick-capture follow-up. Refine with `/refine-plan` before executing — the
> `## Steps` below are a gap inventory, not yet investigated per-step.

Follow-up to the completed **20260615-1346-microservices-multi-location-backend**
(config model + validation + per-service backend scaffolding + driver routing +
diagram). That plan stopped at "N backends + init-side arch switches + tests";
this one closes the gap to a **working end-to-end `gh optivem init` for a
microservices YAML**.

## TL;DR

**Why:** `gh optivem init` on a `microservices` YAML today scaffolds one backend
code copy per declared service (reusing the multitier per-language backend
template) and the single frontend — but none of the downstream pipeline a
runnable project needs. The microservices arm of the scaffolder is a backend-copy
loop, not a full project generator.
**End result:** `gh optivem init` against a `microservices` YAML produces a
complete, CI-runnable project (per-service workflows, docker-compose, sonar keys,
license/finalize, system-test wiring) the same way monolith/multitier do today.

## Problem — the known gaps (from the 20260615 Step 3 executor)

- **No shop-side `system/microservices/` template.** Each service currently
  copies from `system/multitier/backend-<lang>` (the only per-language backend
  template the shop ref carries). A microservices system likely needs its own
  shop reference (per-service layout, inter-service wiring, compose topology).
- **Downstream pipeline steps branch on `monolith`/`multitier` only** and fall
  through for microservices:
  - per-service pipeline + commit-stage **workflows** (today only mono/multi
    `*PipelineWorkflows` / commit-stage maps exist);
  - **docker-compose** topology for N services + frontend;
  - **sonar key** fixups per service (`*SonarKeyReplacements` are mono/multi);
  - **content-replacement** helpers + `ValidateNoLeftoverTemplateRefs`
    (`forbiddenTemplateRefs` has no microservices arm);
  - **`finalize` / `WriteLicense`** would target an empty `SystemRepoDir` for
    multi-repo microservices (guarded to no-op today, but not actually wired);
  - **`WriteOptivemYAML`** multi-repo fan-out across N service repos.

## Steps (gap inventory — refine before executing)

- [ ] Step 1: Decide + add the shop-side `system/microservices/` template (or
      confirm per-service reuse of `system/multitier/backend-<lang>` is the
      intended long-term shape). Gate the rest on this.
- [ ] Step 2: Per-service workflows — generalize `pipelineWorkflows` /
      commit-stage maps to emit one set per service (sorted by `BackendServiceNames()`).
- [ ] Step 3: docker-compose generation for N services + the single frontend.
- [ ] Step 4: Per-service sonar key + content-replacement helpers; add a
      microservices arm to `forbiddenTemplateRefs` / `ValidateNoLeftoverTemplateRefs`.
- [ ] Step 5: `finalize` / `WriteLicense` / `WriteOptivemYAML` multi-repo
      fan-out across N service repos (resolve the empty-`SystemRepoDir` path).
- [ ] Step 6: End-to-end test — `gh optivem init` against a microservices YAML
      (mono-repo + multi-repo) produces a complete project with no leftover
      template refs. Scope `go test` per-package (no unbounded `./...` on Windows).

## Open questions

1. Shop template: dedicated `system/microservices/` reference vs. per-service
   reuse of the multitier backend template? (Blocks Step 1; everything else
   depends on the answer.)
2. Inter-service topology in docker-compose — are services independently
   deployed (own compose service each) or is there shared infra (gateway, shared
   db)? Needs a reference architecture decision.
3. Multi-repo microservices: one repo per service + a root orchestration repo,
   or a different repo layout? Drives `WriteOptivemYAML` fan-out and the
   `__SIBLING_REPOS__` wiring.
