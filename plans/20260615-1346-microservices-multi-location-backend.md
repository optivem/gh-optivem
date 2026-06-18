# 2026-06-15 13:46:00 UTC — Support microservices: a backend that spans multiple locations

🤖 **Picked up by agent** — `Valentina_Desk` at `2026-06-18T07:07:16Z`

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

**Steps 1–2 (config model + validation) and the config/validation unit tests are committed.** The next executable unit is **Step 3: Scaffolding** — make `internal/scaffolding/**` iterate the YAML-declared `backend-services` map and scaffold one backend per service (per D7, YAML-authored, not flags), and wire the init-side arch switches (`internal/config/yaml_input.go`, `optivemyaml/optivemyaml.go`, `configinit/prompt.go`) to recognize the microservices shape. Steps 4 (driver routing) and 5 (diagram/docs) are independent of Step 3 once the config model is in place and can run in parallel; the remaining Step 6 tests (scaffolding produces N backends; driver routing for >1 backend) ride along with Steps 3–4.

## Decisions (resolved 2026-06-16)

These settle Open questions 1–5; the config shape is now pinned.

- **D1 — new `Arch` value `microservices`** (not extending `multitier`'s backend into a list). Exclusivity (`Validate` Rule 5), Sonar (Rule 18), scaffolding, and driver routing all switch on `architecture` already; a new value keeps `monolith` and single-backend `multitier` untouched. Overloading `system.backend` to be either a `TierSpec` or a list can't bind cleanly in yaml.v3 and would force every existing reader to handle both shapes.
- **D2 — heterogeneous services.** Each service carries its own `lang` (a `TierSpec` already has one); homogeneous is the degenerate case where they match. Enables one `java` + one `dotnet` service.
- **D3 — reuse the existing `repo-strategy` axis.** Each service has its own `repo:` (already on `TierSpec`): `mono-repo` ⇒ shared repo, distinct paths; `multi-repo` ⇒ one repo per service. No new concept — identical to how `multitier` backend/frontend work.
- **D4 — `system.backend-services` is a name-keyed map of `TierSpec`**, mirroring `external-systems:` exactly. The map key is the service identity and the driver-routing handle (grounds Open question 4: per-service base location keyed by service name). Deterministic iteration via a `BackendServiceNames()` helper modelled on `ExternalSystemNames()`. Reuses `TierSpec` verbatim, so `requireFullTier` already enforces per-service completeness — no new validation primitive.
- **D5 — single frontend.** `system.frontend` stays one `TierSpec`, exactly like `multitier`; microservices multiplicity is backend-only. Frontend pluralization (micro-frontends) is explicitly out of scope (a later plan).
- **D6 — terminology:** unit = "service", arch label = "microservices".
- **D7 — microservices is YAML-authored only** (executor decision, Step 1 CLI fork). The `--backend-*` flag family is **not** extended to N services: per-service flags can't bind arbitrary service names in Cobra, and this mirrors `external-systems:`, which `gh optivem init` also does not scaffold (operators add entries by hand). The `projectconfig` YAML schema is the SSoT that accepts `architecture: microservices` + `backend-services:`; the init/`config init` flag path (`ValidateArch`, `RawFlags`, `resolveLangs*`, `resolvePathFlagsForYAML`) stays monolith/multitier, with `ValidateArch`'s message pointing microservices operators at YAML authoring. **Step 3 consequence:** scaffolding iterates the YAML-declared `backend-services` map, not flags. The init-side arch switches (`internal/config/yaml_input.go`, `optivemyaml.go`, `configinit/prompt.go`) are reached only by the scaffold flow and are part of Step 3, not the config-model seam.

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

- [ ] Step 3: **Scaffolding.** Make `internal/scaffolding/**` iterate over the declared service locations and scaffold one backend per service, instead of the single `BackendPath`. Confirm per-service templating, repo dir, and content replacements. Per D7, the service list comes from the YAML `backend-services` map (not flags); wire the init-side arch switches that are currently monolith/multitier-only — `internal/config/yaml_input.go` (`ArchMonolith`/`ArchMultitier` → CLI string mapping), `internal/config/optivemyaml/optivemyaml.go`, and `internal/config/configinit/prompt.go` — to recognize the loaded microservices shape so `gh optivem init` can scaffold an existing microservices YAML.
- [ ] Step 4: **Driver routing.** Extend the system-test driver (`internal/atdd/runtime/driver/driver.go`) so each backend service is addressable on its own route/base-location keyed by service name (the `backend-services` map key, per D4), generalizing the single `Backend Route`.
- [ ] Step 5: **Architecture diagram + docs.** Add a microservices node family to the canonical YAML (`internal/diagrams/architecture/architecture.yaml`) and sync the `docs/atdd/architecture` prose. Content change via `diagram-content-editor`; **no local diagram regeneration** (CI rebuilds the generated `docs/*-diagram.md`/SVGs from YAML).
- [ ] Step 6: **Tests (remaining).** The config-shape + validation unit tests are **done** (`internal/kernel/projectconfig/microservices_test.go`: single vs. multi backend, duplicate-location rejection, arch/shape mismatch, per-service sonar, `Repos`/`BackendServiceNames`). Still owed alongside Steps 3–4: a scaffolding test that N service locations produce N backends; a driver routing test for >1 backend. Scope `go test` per-package (no unbounded `go test ./...` on Windows).

## Open questions

Questions 1–6 are all resolved.

6. **Relationship to the external-systems multiplicity plan — RESOLVED (2026-06-18).** The external-systems multiplicity work is already merged into the code (`ExternalSystems map[string]ExternalSystem`, `ExternalSystemNames()`, `validateExternalSystems()` in `config.go`); the referenced plan file no longer exists. `backend-services:` lives on a **separate `System` field** with its own `validateBackendServices()` path, so there is no config-surface collision — confirmed during the Step 1–2 implementation. The diagram-surface distinctness (external-system nodes vs. owned-service nodes) remains a Step 5 concern: keep them as separate node families.
