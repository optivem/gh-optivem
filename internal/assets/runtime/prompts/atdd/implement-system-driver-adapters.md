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

## Inputs

- `${architecture}` — the target architecture for the System Driver adapters.
- `${allowed_roots}` — the paths the agent may write to.

## Steps

1. Implement the System Driver adapters for real — replace each `TODO: System Driver` prototype with actual logic. If your previous WRITE didn't compile, instead fix the broken/missing piece in your prior edits (forgotten Driver stub, signature mismatch, typo) and fix it minimally.
