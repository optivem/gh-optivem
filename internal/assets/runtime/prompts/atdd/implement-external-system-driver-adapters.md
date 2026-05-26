---
# Serves two callers: cover-system-behavior / change-system-behavior
# (translation work) and redesign-system-structure (Real/Stub adapter +
# Ext* DTO reshape — needs more reasoning). Opus + medium covers the
# reshape branch.
model: opus
effort: medium
---
The implement-external-system-driver-adapters task fills in real adapter logic for the External System Driver port — the Real driver that talks to the live external service plus the Stub driver(s) used in test runs. It serves two callers:

- **change-system-behavior / cover-system-behavior CYCLEs** — called when the prior `implement-dsl` task set `External System Driver Interface Changed: yes`. Replace each `TODO: External System Driver` prototype with real logic. No Checklist is supplied.
- **redesign-system-structure CYCLE** — called as step 1b of the structural reshape, in parallel with `implement-system-driver-adapters`. A Checklist parsed from the ticket body is supplied; this task reshapes the external-system driver layer (Ext* DTOs, Real driver, Stub driver(s)) to match the new external API so DSL, Gherkin, and tests stay untouched.

Architecture: ${architecture}

Allowed write roots:
${allowed_roots}

## Checklist

${checklist}

Edit ONLY files under the "Allowed write roots" listed at the top of this prompt. External-system roots, when listed, are write-eligible only when the ticket explicitly calls for stub or simulator changes — otherwise read-only context.

## Steps

1. **Branch on Checklist.**
   (a) If the Checklist section above is empty or absent, you are running under **change-system-behavior** or **cover-system-behavior**: implement the External System Driver adapters for real — replace each `TODO: External System Driver` prototype with actual logic. Do NOT read external-system source code to figure out behavior; rely on the contract tests and the published external API contract. If your previous WRITE didn't compile, instead fix the broken/missing piece in your prior edits (forgotten Driver stub, signature mismatch, typo) and fix it minimally.
   (b) If the Checklist is non-empty, you are running under **redesign-system-structure**: identify the external system from the Checklist and locate its driver components under `${external-system-driver-adapter}/<external-system>/` (`XyzRealDriver`, `XyzStubDriver` per stub variant, `BaseXyzClient`, `Ext*` DTOs). Then execute steps 2–4 below.
2. Update the `Ext*` DTOs to match the new external surface (fields, types, encoding).
3. Update the Real driver impl (`${external-system-driver-adapter}/<external-system>/XyzRealDriver`) to consume the new surface. Apply across all parallel implementations (Java/.NET/TS × monolith/multitier — see [architecture/driver-adapter.md](../../../architecture/driver-adapter.md)).
4. Update the Stub driver impl(s) (`${external-system-driver-adapter}/<external-system>/XyzStubDriver`) to mirror the new surface so stubs stay consistent with reality.
5. **External driver-port guardrail.** Do NOT modify any file under `${external-system-driver-port}/` casually. If an interface change is unavoidable, STOP and present to the user: the method(s) you want to change, why the Real/Stub adapters alone cannot absorb the change, the proposed new signature(s), and the explicit warning that this WILL require contract-test updates (the calling CYCLE invokes the contract-test-update flow for affected scenarios). Wait for explicit user approval before editing any `${external-system-driver-port}/` file.
6. Do not modify acceptance tests, contract tests, DSL, Gherkin, or any code outside the external-system driver layer. `${system-test-path}/.../Legacy/` is read-only.
