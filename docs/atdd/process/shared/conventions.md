# Conventions

Normative schemas that apply to **every cycle and sub-cycle** (AT, CT, Legacy, Structural). The AT cycle is the first to reference them, but sibling-cycle docs point here too.

Every phase agent operates within a declared allowed-path scope; see [Phase scope policy](#phase-scope-policy) for the per-phase table and how violations are handled.

## Phase scope policy

**Every phase agent operates within a declared scope — no exceptions.** The table below is the complete source of truth: every phase has a row, and every agent's prompt is constructed with its row's allowed paths injected automatically. An agent without a declared scope is a configuration bug, not a default-allow.

Two layers enforce the policy; both converge on the same user-facing prompt — they differ only in who noticed the out-of-scope edit first.

- **Layer 1 — agent-triggered:** the work-agent's prompt names the allowed paths for its phase. When the agent recognises it needs to edit out of scope, it does **not** wait inline for approval. Instead, it exits with a structured *scope-exception-requested* signal naming the intended out-of-scope file(s) and the reason. The signal triggers the same human-task prompt as Layer 2.
- **Layer 2 — post-phase scope check (catches what Layer 1 missed):** after each phase agent finishes normally, a scripted check diffs the modified files (`git diff --name-only` vs the pre-phase ref) against the allowed-path policy. On violation, the cycle halts and runs the same human-task prompt.

In both cases, the cycle never auto-allows and never auto-reverts — the user always decides. Options:

- **Accept (continue from current phase)** — the agent's out-of-scope change is judged correct (e.g. RED-SYSTEM-DRIVER discovered the DSL or driver-port interface was wrong; GREEN discovered the test was wrong). Record the exception and continue from the current phase.
- **Rewind to upstream phase** — accept the out-of-scope change, then restart the cycle from the phase whose output was wrong (e.g. accept a DSL edit made during RED-SYSTEM-DRIVER, then rerun RED-DSL to re-validate the corrected DSL, then continue). This is the most principled response when the violation reveals an upstream bug — it preserves the per-phase RED guarantee instead of carrying an unvalidated upstream change forward.
- **Revert + rerun** — discard the out-of-scope changes and rerun the current phase agent.
- **Abort** — stop the cycle, escalate to human review.

Allowed-path policy by phase:

| Phase | Allowed paths |
|---|---|
| RED-TEST | acceptance test files; DSL prototype stubs (interface + `"TODO: DSL"` throw) |
| RED-DSL | DSL Core impls; driver-port interface declarations |
| RED-SYSTEM-DRIVER | `${driver_port}/${sut_namespace}/` and `${driver_adapter}/${sut_namespace}/` |
| GREEN | production system code only; tests/DSL/drivers are frozen (see GREEN section) |
| CT-RED-TEST / CT-RED-DSL / CT-RED-EXTERNAL-DRIVER / CT-GREEN-STUBS | `external/**` only |

This table is the source of truth for the policy schema; the post-phase scope-check step loads it at runtime.
