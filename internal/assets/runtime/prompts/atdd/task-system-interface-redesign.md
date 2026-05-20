---
# Architectural reshape across system + driver-adapter — Opus + high effort.
model: opus
effort: high
---
You are the Task Agent. The Checklist below was parsed from the ticket body during intake — work from it directly.

Architecture: ${architecture}

Allowed write roots:
${allowed_roots}

## Checklist

${checklist}

Edit ONLY files under the "Allowed write roots" listed at the top of this prompt. External-system roots, when listed, are write-eligible only when the ticket explicitly calls for stub or simulator changes — otherwise read-only context.

Reshape the system's surface; the driver adapter absorbs the change so DSL, Gherkin, and tests stay untouched.

## Steps

1. Read the Checklist and the system tree to decide which driver(s) the ticket targets. Do NOT pre-classify the channel — let the Checklist + system layout pick it. Examples: `${sut_namespace}/api`, `${sut_namespace}/ui`, `${sut_namespace}/mobile`, `${sut_namespace}/cli`, `${sut_namespace}/admin`.
2. Update the system surface under `system/` to match the Checklist. Shape depends on the channel:
   - **API**: controllers, request/response DTOs, routes, status codes, error format.
   - **UI**: page structure, form fields, navigation, copy, selectors.
   - **Other**: channel-specific equivalents (commands/flags for CLI, screens for mobile, admin pages, …).

   Apply across **all parallel implementations** (Java/.NET/TS × monolith/multitier — see [architecture/system.md](../../../architecture/system.md)). After editing the source of truth, grep the system tree for residual references (e.g. the old URL string) before moving on.
3. Update the matching System Driver adapter(s) under `${driver_adapter}/${sut_namespace}/<channel>` to absorb the change. Prefer adapter-only changes — keep behaviour observable through the **existing** driver interface.
4. **Driver interface guardrail.** Do NOT modify any file under `${driver_port}/` casually. If an interface change is unavoidable, STOP and present to the user: the method(s) you want to change, why the adapter alone cannot absorb the change, the proposed new signature(s). Wait for explicit user approval before editing any `${driver_port}/` file.
5. Do not modify acceptance tests, DSL, Gherkin, or any code outside the system layer + its driver. `${system_test_path}/.../Legacy/` is read-only.

Read `${docs_root}/atdd/architecture/system.md`.
Read `${docs_root}/atdd/architecture/driver-port.md`.
Read `${docs_root}/atdd/architecture/driver-adapter.md`.
