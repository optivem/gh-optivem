---
# Serves two callers: change-system-behavior (translation work — Sonnet medium)
# and redesign-system-structure (architectural adapter absorption — needs more
# reasoning). Opus + medium covers the reshape branch; the no-Checklist branch
# under-uses the budget but staying on one model keeps dispatch simple.
model: opus
effort: medium
---
The implement-system-driver-adapters task fills in real adapter logic for the System Driver port. It serves two callers:

- **change-system-behavior CYCLE** — called via the `implement-and-verify-system-driver-adapters` HIGH, only when the prior `implement-dsl` task set `System Driver Interface Changed: yes`. Replace each `TODO: System Driver` prototype with real logic. No Checklist is supplied.
- **redesign-system-structure CYCLE** — called as step 1a of the structural reshape, in parallel with `implement-external-system-driver-adapters` and ahead of `implement-system`. A Checklist parsed from the ticket body is supplied; this task absorbs the surface reshape inside the adapter layer so DSL, Gherkin, and tests stay untouched.

Architecture: ${architecture}

Allowed write roots:
${allowed_roots}

## Checklist

${checklist}

Edit ONLY files under the "Allowed write roots" listed at the top of this prompt.

## Steps

1. **Branch on Checklist.**
   (a) If the Checklist section above is empty or absent, you are running under **change-system-behavior**: implement the System Driver adapters for real — replace each `TODO: System Driver` prototype with actual logic. If your previous WRITE didn't compile, instead fix the broken/missing piece in your prior edits (forgotten Driver stub, signature mismatch, typo) and fix it minimally.
   (b) If the Checklist is non-empty, you are running under **redesign-system-structure**: update the matching System Driver adapter(s) under `${driver-adapter}/<channel>` to absorb the change described in the Checklist. Prefer adapter-only changes — keep behaviour observable through the **existing** driver interface. Apply across all parallel implementations (Java/.NET/TS × monolith/multitier).
2. **Driver-port guardrail.** Do NOT modify any file under `${driver-port}/` casually. If an interface change is unavoidable, STOP and present to the user: the method(s) you want to change, why the adapter alone cannot absorb the change, the proposed new signature(s). Wait for explicit user approval before editing any `${driver-port}/` file.
3. Do not modify acceptance tests, DSL, Gherkin, or the system surface from this task. The redesign caller invokes `implement-system` separately for the surface change; the change-system-behavior caller has tests/DSL/system already in place.
