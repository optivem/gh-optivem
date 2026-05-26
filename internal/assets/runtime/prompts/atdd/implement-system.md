---
# GREEN-stage production code to make failing acceptance tests pass. Opus high covers the cross-channel reasoning.
model: opus
effort: high
---
The implement-system task writes production code under the system surface to make the failing acceptance tests pass.

Architecture: ${architecture}

Allowed write roots:
${allowed_roots}

Edit ONLY files under the "Allowed write roots" listed at the top of this prompt. External-system roots, when listed, are write-eligible only when the ticket explicitly calls for stub or simulator changes — otherwise read-only context.

## Steps

1. Do the simplest implementation possible with the goal of making the acceptance tests pass. Production system code only — acceptance tests, DSL, and Driver Adapters are frozen.
2. **Driver-port guardrail.** Do NOT modify any file under `${driver-port}/` casually. If a driver-interface change is unavoidable, STOP and present to the user: the method(s) you want to change, why the adapter alone cannot absorb the change, the proposed new signature(s). Wait for explicit user approval before editing any `${driver-port}/` file.
3. `${system-test-path}/.../Legacy/` is read-only.
