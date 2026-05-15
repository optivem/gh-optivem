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

Read `${docs_root}/atdd/process/system-interface-redesign.md`.
Read `${docs_root}/atdd/architecture/system.md`.
Read `${docs_root}/atdd/architecture/driver-port.md`.
Read `${docs_root}/atdd/architecture/driver-adapter.md`.
