---
# Serves two callers: change-system-behavior (GREEN-implementation, narrow)
# and redesign-system-structure (architectural surface reshape). Opus + high
# covers both safely.
model: opus
effort: high
---
The implement-system task makes the system's surface match its specification. It serves two callers:

- **change-system-behavior CYCLE** — invoked when failing acceptance tests need production code to pass. No Checklist is supplied.
- **redesign-system-structure CYCLE** — invoked as step 2 of the structural reshape. A Checklist parsed from the ticket body is supplied; this task executes it against the system surface.

Architecture: ${architecture}

Allowed write roots:
${allowed_roots}

## Checklist

${checklist}

Edit ONLY files under the "Allowed write roots" listed at the top of this prompt. External-system roots, when listed, are write-eligible only when the ticket explicitly calls for stub or simulator changes — otherwise read-only context.

Do not present or wait for approval inside the agent.

## Steps

1. **Branch on Checklist.**
   (a) If the Checklist section above is empty or absent, you are running under **change-system-behavior**: do the simplest implementation possible with the goal of making the acceptance tests pass. Production system code only — acceptance tests, DSL, and Driver Adapters are frozen.
   (b) If the Checklist is non-empty, you are running under **redesign-system-structure**: execute the Checklist on the system surface. Read the Checklist and the system tree to decide which channel(s) the ticket targets — do NOT pre-classify; let the Checklist + system layout pick it. Examples: `api`, `ui`, `mobile`, `cli`, `admin`. Update the system surface under `system/` to match:
   - **API**: controllers, request/response DTOs, routes, status codes, error format.
   - **UI**: page structure, form fields, navigation, copy, selectors.
   - **Other**: channel-specific equivalents (commands/flags for CLI, screens for mobile, admin pages, …).

   Apply across **all parallel implementations** (Java/.NET/TS × monolith/multitier — see [architecture/system.md](../../../architecture/system.md)). After editing the source of truth, grep the system tree for residual references (e.g. the old URL string) before moving on.
2. **Driver-port guardrail.** Do NOT modify any file under `${driver-port}/` casually. If a driver-interface change is unavoidable, STOP and present to the user: the method(s) you want to change, why the adapter alone cannot absorb the change, the proposed new signature(s). Wait for explicit user approval before editing any `${driver-port}/` file. The matching adapter absorption is handled by the `implement-system-driver-adapters` task — not here.
3. **Escalation when no Checklist is supplied.** If you cannot make the tests pass without touching acceptance tests, DSL, Driver interfaces, or Driver adapters, **stop and ask the user** — do not patch around it. Needing to touch a frozen layer signals that an earlier task was wrong; the user decides whether to rewind.
4. `${system-test-path}/.../Legacy/` is read-only in both modes.

Read `${references_root}/atdd/architecture/system.md`.
Read `${references_root}/atdd/architecture/driver-port.md`.
Read `${references_root}/atdd/architecture/driver-adapter.md`.
