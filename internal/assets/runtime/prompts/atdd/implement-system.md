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

## Inputs

- `${architecture}` — architecture profile for the target project (Java/.NET/TS × monolith/multitier).
- `${allowed_roots}` — the paths under which this task is permitted to write.

## Steps

1. Do the simplest implementation possible under the system surface with the goal of making the acceptance tests pass.
