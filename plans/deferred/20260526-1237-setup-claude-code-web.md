# Set up Claude Code on the web

Goal: be able to drive Claude Code from the phone (https://claude.ai/code) so
work can continue away from the desktop. Already covered by Pro/Max
subscription — no extra cost, but usage shares the same monthly quota as the
desktop CLI.

Docs:
- https://code.claude.com/docs/en/web-quickstart.md
- https://code.claude.com/docs/en/claude-code-on-the-web.md

## Items

1. Sign in to https://claude.ai/code with the existing Anthropic account.

2. Install the Claude GitHub App and grant access to 1-2 optivem repos to
   start with (pick small/safe ones — can broaden later). Read/write needed
   so it can create branches + PRs.

3. Create a cloud environment in the web UI:
   - Network access: **Trusted** (default — covers npm, PyPI, GitHub, Docker
     Hub, AWS/GCP/Azure SDKs).
   - No setup script unless a specific extra tool is needed.
   - Leave env vars empty for now. **The env-var store is NOT a secrets
     vault** — anyone with edit access on the environment can read it. Don't
     paste API keys / tokens there. GitHub auth goes through Anthropic's
     proxy so the GitHub token never enters the VM.

4. Kick off a tiny first task in **Plan mode** on one of the connected repos
   (e.g. a typo fix or a one-line README tweak). Confirm the round-trip
   works: branch created, PR opened, diff visible in the web UI.

5. Install the Claude mobile app (iOS or Android), sign in with the same
   account, open the **Code** tab, and confirm the session from item 4
   appears there. Verify you can leave a follow-up comment from the phone.

6. (Optional) Add `claude.ai/code` to the phone home screen as a PWA
   (Safari/Chrome → "Add to Home Screen") for one-tap access.

7. Decide a default workflow for phone-driven work — e.g. plan locally in
   VS Code → commit the `plans/*.md` file → trigger cloud run from phone
   that executes the plan. Note the chosen pattern wherever it makes sense
   (or just remember it — no doc required).
