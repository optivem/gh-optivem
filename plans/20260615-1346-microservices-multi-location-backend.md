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

**The modelling fork is resolved (see Decisions below) — Step 1 is now the first executable unit.** Introduce `architecture: microservices` and the `backend-services` name-keyed map on `projectconfig.System`, leaving `monolith` / `multitier` byte-for-byte unchanged. Only Open question 6 (independence from the external-systems multiplicity plan) remains, and it's a confirm-no-collision check, not a blocker on Step 1.

## Decisions (resolved 2026-06-16)

These settle Open questions 1–5; the config shape is now pinned.

- **D1 — new `Arch` value `microservices`** (not extending `multitier`'s backend into a list). Exclusivity (`Validate` Rule 5), Sonar (Rule 18), scaffolding, and driver routing all switch on `architecture` already; a new value keeps `monolith` and single-backend `multitier` untouched. Overloading `system.backend` to be either a `TierSpec` or a list can't bind cleanly in yaml.v3 and would force every existing reader to handle both shapes.
- **D2 — heterogeneous services.** Each service carries its own `lang` (a `TierSpec` already has one); homogeneous is the degenerate case where they match. Enables one `java` + one `dotnet` service.
- **D3 — reuse the existing `repo-strategy` axis.** Each service has its own `repo:` (already on `TierSpec`): `mono-repo` ⇒ shared repo, distinct paths; `multi-repo` ⇒ one repo per service. No new concept — identical to how `multitier` backend/frontend work.
- **D4 — `system.backend-services` is a name-keyed map of `TierSpec`**, mirroring `external-systems:` exactly. The map key is the service identity and the driver-routing handle (grounds Open question 4: per-service base location keyed by service name). Deterministic iteration via a `BackendServiceNames()` helper modelled on `ExternalSystemNames()`. Reuses `TierSpec` verbatim, so `requireFullTier` already enforces per-service completeness — no new validation primitive.
- **D5 — single frontend.** `system.frontend` stays one `TierSpec`, exactly like `multitier`; microservices multiplicity is backend-only. Frontend pluralization (micro-frontends) is explicitly out of scope (a later plan).
- **D6 — terminology:** unit = "service", arch label = "microservices".

Concrete shape (mono-repo, heterogeneous):

```yaml
system:
  architecture: microservices
  backend-services:                 # NEW — name-keyed map of TierSpec, mirrors external-systems:
    orders:
      path: system/microservices/orders-java
      repo: optivem/shop
      lang: java
      sonar-project: optivem_shop-orders
    inventory:
      path: system/microservices/inventory-dotnet
      repo: optivem/shop
      lang: dotnet
      sonar-project: optivem_shop-inventory
  frontend:                         # unchanged — single TierSpec (D5)
    path: system/microservices/frontend-react
    repo: optivem/shop
    lang: typescript
    sonar-project: optivem_shop-frontend
  db-migration-path: system/db/migrations
```

Multi-repo differs only in `repo-strategy: multi-repo`, each service's `repo: optivem/shop-<name>`, and `path: .`.

## Steps

- [ ] Step 1: **Config model.** Add `BackendServices map[string]TierSpec` (`yaml:"backend-services,omitempty"`) to `projectconfig.System` and `ArchMicroservices = "microservices"` to the enum (`internal/kernel/projectconfig/config.go`); add a `BackendServiceNames()` helper modelled on `ExternalSystemNames()`. On the CLI side (`internal/config/config.go`) extend `ValidateArch`/`RawFlags` and decide the `--backend-*` flag family for expressing N services (per-service flags vs. only-via-YAML). Keep `monolith` / single-backend `multitier` untouched — these are purely additive.
- [ ] Step 2: **Config validation.** Add the `microservices` arm to `Validate` Rule 5 (requires ≥1 `backend-services` entry; forbids monolith's `path/repo/lang`; single frontend per D5), thread `BackendServices` into `Repos()`, the Rule 3/4 lang+path loops, and Rule 18 Sonar (per-service `sonar-project` required). `requireFullTier` already covers per-service completeness; map keys give duplicate-name rejection for free. Error clearly when a single-backend arch carries `backend-services` (and vice-versa).
- [ ] Step 3: **Scaffolding.** Make `internal/scaffolding/**` iterate over the declared service locations and scaffold one backend per service, instead of the single `BackendPath`. Confirm per-service templating, repo dir, and content replacements.
- [ ] Step 4: **Driver routing.** Extend the system-test driver (`internal/atdd/runtime/driver/driver.go`) so each backend service is addressable on its own route/base-location keyed by service name (the `backend-services` map key, per D4), generalizing the single `Backend Route`.
- [ ] Step 5: **Architecture diagram + docs.** Add a microservices node family to the canonical YAML (`internal/diagrams/architecture/architecture.yaml`) and sync the `docs/atdd/architecture` prose. Content change via `diagram-content-editor`; **no local diagram regeneration** (CI rebuilds the generated `docs/*-diagram.md`/SVGs from YAML).
- [ ] Step 6: **Tests.** Config-shape + validation unit tests (single vs. multi backend, duplicate-path rejection, arch/shape mismatch); a scaffolding test that N service locations produce N backends; driver routing test for >1 backend. Scope `go test` per-package (no unbounded `go test ./...` on Windows).

## Open questions

Questions 1–5 are resolved — see **Decisions** above (D1–D6). One remains:

6. **Relationship to the external-systems multiplicity plan.** `plans/20260615-0755-external-system-multi-system-support.md` makes *external* systems plural. This plan deliberately mirrors that plan's `external-systems:` name-keyed-map pattern for `backend-services:` but on a **separate `System` field**, so there's no config-surface collision; confirm the diagram surfaces (external-system nodes vs. owned-service nodes) also stay distinct before Step 5.
