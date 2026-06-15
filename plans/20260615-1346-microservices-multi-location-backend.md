# 2026-06-15 13:46:00 UTC — Support microservices: a backend that spans multiple locations

## TL;DR

**Why:** Today every supported architecture has **exactly one backend location** — `monolith` has one `SystemPath`, `multitier` has one `BackendPath` (single `BackendLang`, single `BackendRepo`). A microservices system has **N backend services**, each its own location/scaffold/route, which the current config and scaffolding cannot express.
**End result:** A project can declare a backend made of **multiple service locations**, and config validation, scaffolding, the system-test driver routing, and the architecture diagram/docs all treat "the backend" as a set of services rather than a single path.

## Outcomes

What we get out of this — the goals and deliverables:

- A project can declare a **microservices backend**: a list of backend services, each with its own repo-relative **location/path** (and, per open decision, its own language/repo), instead of the single `BackendPath`/`BackendLang`/`BackendRepo` triple.
- **Config validation** accepts and checks the multi-service shape (no two services share a path; each location is well-formed; the list is non-empty) and rejects it for the architectures that require a single backend.
- **Scaffolding** generates one backend scaffold **per declared service location** (N backends), instead of assuming a single backend root.
- The **system-test driver** can address more than one backend — each service is reachable at its own route/base-location, rather than a single `Backend Route`.
- The **architecture diagram + `docs/atdd` prose** depict the backend as multiple service nodes when the system is microservices (extending the existing `monolith` / `multitier` node families), kept in sync via the canonical YAML (no hand-edited generated diagrams).
- A **single-backend project (monolith / multitier) is completely unchanged** — the multi-location path is additive and only engages when the microservices shape is declared.

## ▶ Next executable step (resume here)

**Design/decision work, not a mechanical edit yet.** The fork that gates everything is *how* a multi-location backend is modelled (new `Arch` value `microservices` vs. extending `multitier`'s backend into a list — see Open question 1) and whether services are **homogeneous or heterogeneous** in language/repo (Open questions 2–3). Resolve the Open decisions below via `/refine-plan` **before** any code edit — the config field shape (`internal/config/config.go`, `internal/kernel/projectconfig/config.go`) cannot be written until decisions 1–3 land. Once they're settled, Step 1 (config model) becomes the first executable unit.

## Steps

- [ ] Step 1: **Config model.** Introduce the multi-service backend shape in `internal/config/config.go` (and `internal/kernel/projectconfig/config.go`): a list of backend services, each carrying its own location (+ language/repo per decisions). Decide the `Arch` value and CLI flags (`--arch`, the `--backend-*` family) that express it. Keep `monolith` / single-backend `multitier` untouched.
- [ ] Step 2: **Config validation.** Extend `ValidateArch` / backend-path / backend-lang validators (`internal/config/config.go`) to validate the service list: non-empty, no duplicate paths, each location well-formed; error clearly when a single-backend arch is given a multi-service list (and vice-versa).
- [ ] Step 3: **Scaffolding.** Make `internal/scaffolding/**` iterate over the declared service locations and scaffold one backend per service, instead of the single `BackendPath`. Confirm per-service templating, repo dir, and content replacements.
- [ ] Step 4: **Driver routing.** Extend the system-test driver (`internal/atdd/runtime/driver/driver.go`) so each backend service is addressable on its own route/base-location, generalizing the single `Backend Route`.
- [ ] Step 5: **Architecture diagram + docs.** Add a microservices node family to the canonical YAML (`internal/diagrams/architecture/architecture.yaml`) and sync the `docs/atdd/architecture` prose. Content change via `diagram-content-editor`; **no local diagram regeneration** (CI rebuilds the generated `docs/*-diagram.md`/SVGs from YAML).
- [ ] Step 6: **Tests.** Config-shape + validation unit tests (single vs. multi backend, duplicate-path rejection, arch/shape mismatch); a scaffolding test that N service locations produce N backends; driver routing test for >1 backend. Scope `go test` per-package (no unbounded `go test ./...` on Windows).

## Open questions

1. **Modelling fork — new `Arch` value or extend `multitier`?** Is microservices a third `Arch` (`"monolith" | "multitier" | "microservices"`) with its own backend-list field, **or** does `multitier`'s `BackendPath` simply become a list (1 = today's case, N = microservices)? This decides the entire config surface and every downstream switch.
2. **Homogeneous vs. heterogeneous services.** Do all backend services share one `BackendLang`, or can each service declare its own language (e.g. one `java`, one `dotnet`)? Affects scaffolding, per-service templates, and the diagram.
3. **Repo strategy.** One repo containing N service directories, vs. N repos (one per service, extending the existing `-backend` repo-naming). Affects `BackendRepo*` derivation and the multi-repo flow.
4. **Driver addressing.** How does a system test target a specific service — per-service base URL/route keyed by service name, service registry/gateway, or convention? Grounds Step 4.
5. **Terminology.** Pick the canonical word for a backend unit ("service") and the arch label ("microservices" vs "multiservice") so config, docs, and diagram agree.
6. **Relationship to the external-systems multiplicity plan.** `plans/20260615-0755-external-system-multi-system-support.md` makes *external* systems plural. Confirm this plan (plural *owned* backends) is independent and doesn't collide on config/diagram surfaces.
