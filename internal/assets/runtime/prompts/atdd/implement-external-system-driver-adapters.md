---
# Translation work (fill TODO markers under the Real driver). Opus medium for per-implementation reasoning.
model: opus
effort: medium
---
The implement-external-system-driver-adapters task fills in real adapter logic for the External System Driver port — the Real driver that talks to the live external service plus the Stub driver(s) used in test runs. Replace each `TODO: External System Driver` prototype with actual logic.

Architecture: ${architecture}

Allowed write roots:
${allowed_roots}

Edit ONLY files under the "Allowed write roots" listed at the top of this prompt. External-system roots, when listed, are write-eligible only when the ticket explicitly calls for stub or simulator changes — otherwise read-only context.

## Steps

1. Implement the External System Driver adapters for real — replace each `TODO: External System Driver` prototype with actual logic. Do NOT read external-system source code to figure out behavior; rely on the contract tests and the published external API contract. If your previous WRITE didn't compile, instead fix the broken/missing piece in your prior edits (forgotten Driver stub, signature mismatch, typo) and fix it minimally.
2. **External driver-port guardrail.** Do NOT modify any file under `${external-system-driver-port}/` casually. If an interface change is unavoidable, STOP and present to the user: the method(s) you want to change, why the Real/Stub adapters alone cannot absorb the change, the proposed new signature(s), and the explicit warning that this WILL require contract-test updates. Wait for explicit user approval before editing any `${external-system-driver-port}/` file.
3. Do not modify acceptance tests, contract tests, DSL, Gherkin, or any code outside the external-system driver layer. `${system-test-path}/.../Legacy/` is read-only.
