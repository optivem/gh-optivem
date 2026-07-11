---
# Real DSL logic = system-semantics reasoning, but it's test infrastructure and
# hard-gated by the acceptance/contract suites. Trialling sonnet down from opus
# 2026-06-17; medium effort, scope per dispatch bounded to one DSL surface.
model: sonnet
effort: medium
---

Replace every `TODO: DSL` prototype in the DSL Core (`${dsl-core}`) with real logic. If new port-side behaviour is needed, you may add methods to the driver ports or add/change fields on the DTOs they carry.

This dispatch implements DSL for the **`${test-category}`** test layer.

> **CT-path guard.** If you are implementing CT-side DSL (`test-category=contract`), the System Driver port (`${system-driver-port}`) will **not** legitimately need new methods — do **not** add System Driver prototypes there and do **not** emit `system-driver-port-changed=true`. Contract tests stimulate the External-System Driver only (`${external-system-driver-port}`). On the AT path (`test-category=acceptance`) System Driver prototypes are fine.

> **Distinct default identity per instance.** When the DSL auto-assigns a *default* identity to an entity seeded in a `given()` step (the scenario named the entity but not its identifier), each instance of the **same** entity within one scenario must receive a **distinct** default identity — never one shared constant across all instances. The real, id-enforcing external simulator rejects a duplicate id, so two same-entity instances sharing one default identity fail contract-real on the first verify (a stub would silently tolerate the collision, hiding the defect until the real driver runs). This holds for **every** domain entity — products (SKU), orders (order-number), coupons (coupon-code), and any future entity — not just products. State it at the requirement level: stay faithful to the shop reference testkit DSL and mirror the reference's concrete scheme for distinct defaults (e.g. a per-entity counter, or a value derived from the entity's name) — do **not** invent a bespoke scheme.

> **No load-bearing defaults for newly-formula-relevant fields.** When a field on a shared entity builder is becoming load-bearing for the first time in this ticket — a previously-inert attribute that a production formula will now newly derive cost/output from (e.g. a `weight` field no formula read before, now feeding a shipping calculation) — do not invent a nonzero or otherwise load-bearing default for it if the ticket's own new scenarios already supply the real value explicitly in their `given()` steps. A silently-added default changes what pre-existing scenarios compute once the formula lands, breaking assertions in tests this ticket never touched. Pick a neutral default (e.g. zero, or whatever value leaves the new formula term inert) instead — the ticket's own scenarios don't need the default to be realistic, since they set the value themselves.

## Inputs

### Scope

${scope-block}

No substituted input — discover `TODO: DSL` prototypes by reading the files under `${dsl-core}`.

## Steps

1. Implement the DSL Core (`${dsl-core}`) for real — replace each `TODO: DSL` prototype with actual logic.
2. If you need to add additional driver methods, add the method signature to the port (interface) and a throwing stub body to the corresponding adapter (impl class) so compilation works. Interfaces hold signatures only; the throwing-TODO body lives in the adapter:
   (a) Signature in the System Driver Port (`${system-driver-port}`); throwing `"TODO: System Driver"` stub in the System Driver Adapter (`${system-driver-adapter}`).
   (b) Signature in the External System Driver Port (`${external-system-driver-port}`); throwing `"TODO: External System Driver"` stub in the External System Driver Adapter (`${external-system-driver-adapter}`).
3. Before emitting outputs, set each `*-port-changed` flag per the rules in the Notes section below.
4. Emit the phase-output flag(s) listed under `## Outputs` for this invocation. Every flag listed for this invocation **MUST** be emitted before completing — unset is a bug, not a default `false`. The downstream gateway picks the next task from the flag values.

## Outputs

${expected-outputs}

Notes:

- Both required keys MUST be emitted — the downstream gateway treats *unset* as an error (no implicit default).
- Each `*-port-changed` flag is keyed on the port **directory**. List every file you wrote and set the flag `true` if any file sits under the flag's port directory (interface, DTO, enum — anything). `system-driver-port-changed` covers `${system-driver-port}/**`; `external-driver-port-changed` covers `${external-system-driver-port}/**`. The dispatcher's `validate-outputs-and-scopes` re-derives directory keying from `${changed-files}`, so an incorrect value mis-routes the cycle.
- `scope-exception-files` / `scope-exception-reason` are the envelope from the prepended `scope.md`. Emit only when you read or wrote outside scope.

## Additional Notes

- Import hygiene (see the preamble's compile rule): when a DSL method that would have used an import — e.g. a `Map`-based builder — isn't emitted for this scenario, delete the import it left dangling. An unused import fails the terminal quality gate even though it compiles.
- ${re-entry-policy} For this agent the "broken/missing piece" is typically a forgotten driver stub in the System Driver port (`${system-driver-port}`) or External System Driver port (`${external-system-driver-port}`). Do not change DSL Core (`${dsl-core}`) semantics in the fix.
