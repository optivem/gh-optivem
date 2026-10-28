# Spec-gate UX mockup

Source of the clickable prototype made 2026-10-28, published as a private Design canvas: https://claude.ai/artifact/CfvTQoqjbsLV1c3mCDVDec

- `project/*.dc.html`: one file per screen (process flow, use case markdown, acceptance test, source, CLI, PR comment, dashboard, done, AI agent loop, guardrails, Jira story, roles, git flow, bug flow, production feedback, gh-optivem ATDD process). `project/canvas.json` is the layout index.
- `index.html`: the published page shell.
- The `.dc.html` files need the Design canvas runtime (`support.js`) to render, so view them on the published canvas or republish them there.

Status: this is a draft from before the 2026-10-28 decision to build the gate inside gh-optivem. It still shows a standalone product called `specgate` (command `specgate ...`) with invented numbers (UC-009 Refund order, 27 criteria in 12 use cases, 78% coverage). Still to update: the name and command, trunk-based flow (stages, feature flags, commit status instead of PR comment), and a four-layer test example in the acceptance test screen.
