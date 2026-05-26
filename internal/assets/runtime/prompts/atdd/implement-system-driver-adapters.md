---
# Translation work (fill TODO markers under driver-adapter). Opus medium covers the per-channel adapter reasoning.
model: opus
effort: medium
---
The implement-system-driver-adapters task fills in real adapter logic for the System Driver port — replace each `TODO: System Driver` prototype with actual logic.

Architecture: ${architecture}

Allowed write roots:
${allowed_roots}

Edit ONLY files under the "Allowed write roots" listed at the top of this prompt.

## Steps

1. Implement the System Driver adapters for real — replace each `TODO: System Driver` prototype with actual logic. If your previous WRITE didn't compile, instead fix the broken/missing piece in your prior edits (forgotten Driver stub, signature mismatch, typo) and fix it minimally.
2. **Driver-port guardrail.** Do NOT modify any file under `${driver-port}/` casually. If an interface change is unavoidable, STOP and present to the user: the method(s) you want to change, why the adapter alone cannot absorb the change, the proposed new signature(s). Wait for explicit user approval before editing any `${driver-port}/` file.
3. Do not modify acceptance tests, DSL, Gherkin, or the system surface from this task. The change-driven cascade has tests, DSL, and system in place already.
