# 2026-06-25 06:48 UTC — Multitier multirepo: consumer-CI-push contract transport

> Follow-up to shop plan `20260624-0814-multirepo-contract-transport-dockerized-broker.md`
> (Steps 2 and 6, which were deferred pending multi-repo scaffolding analysis).

## TL;DR

**Why:** `applyMultitierMultirepo` already creates separate frontend and backend repos,
but it pre-seeds both with a copy of `contracts/` — the manual-duplicate pattern that
`contracts/README.md` explicitly labels the false-green fallback. The frontend's
commit-stage has no mechanism to push the freshly generated pact to the backend repo, so
the backend verifies a stale copy. The frontend repo also has no cross-repo write token.

**End result:** The multitier multirepo scaffold produces a frontend commit stage that
pushes the generated pact to the backend repo immediately after `npm run test:pact`, so
the backend always verifies the current contract. `REPO_TOKEN` is wired onto the frontend
component repo for the push. The `contracts/README.md` forward-looking note is retired
(replaced by a "how it works" note).

## Resolved decisions

- **Push step location → `frontend-commit-stage.yml` (new job step, not a separate workflow).**
  After the Pact consumer tests pass and write the contract, a commit-and-push step targets
  the backend repo's `contracts/` path. One workflow file is simpler to teach than two.
- **Token → `REPO_TOKEN` repurposed for write.** `REPO_TOKEN` already carries `repo` scope
  (includes Contents:write). Wire it onto the frontend component repo in
  `SetupVariablesAndSecrets`. Update the doc comment from "Contents:read" to
  "Contents:read+write; also used for consumer-CI-push in multirepo multitier." A
  dedicated fine-grained PAT is a worthwhile hardening follow-on but out of scope here.
- **Backend pre-seeded copy → keep it.** The initial seeded copy lets the backend
  provider verification work from day one, before the first consumer CI run. After the
  first push from frontend CI it becomes auto-maintained. Removing it would break
  first-run provider verification.
- **Frontend pre-seeded copy → keep it.** Consumer runs locally against `../contracts`
  and regenerates it in place. The seed gives a starting point for local runs.

## Outcomes

- `system/multitier/frontend-react/.github/workflows/frontend-commit-stage.yml` gains a
  new job step: "Push contract to backend repo" — runs after Pact tests, commits and
  pushes the updated pact to `<project>-backend/contracts/` via `REPO_TOKEN`.
- `internal/scaffolding/steps/github_setup.go` → `SetupVariablesAndSecrets`: `REPO_TOKEN`
  added to the frontend component repo (currently omitted from the component-repo loop).
- `internal/config/config.go` → `RepoToken` doc comment updated to mention write use.
- `contracts/README.md` → forward-looking note ("this mechanism is forward-looking")
  replaced with a concrete "how it works in the scaffold" note.

## Steps

- [ ] **Step 1 — Add the push step to the frontend commit-stage workflow in shop.**
  File: `system/multitier/frontend-react/.github/workflows/` — locate the
  `*-frontend-react-*-commit-stage.yml` template. After the Pact consumer test step,
  add a step that:
  1. Checks out the backend repo using `REPO_TOKEN`.
  2. Copies the freshly generated `contracts/frontend-backend.json` into the backend
     checkout's `contracts/`.
  3. Commits (if changed) and pushes directly to the backend repo's default branch.
  Check the existing workflow to confirm: the Pact step name, the job/step ordering,
  and whether `REPO_TOKEN` is already referenced elsewhere in that workflow.

- [ ] **Step 2 — Wire `REPO_TOKEN` onto the frontend component repo.**
  File: `internal/scaffolding/steps/github_setup.go` → `SetupVariablesAndSecrets`.
  The component-repo loop currently sets DOCKERHUB_TOKEN, SONAR_TOKEN, GHCR_TOKEN on
  both frontend and backend repos — but not `REPO_TOKEN`. Add `REPO_TOKEN` to the
  frontend repo only (the backend doesn't need to push cross-repo).
  Update `internal/config/config.go` RepoToken doc comment.

- [ ] **Step 3 — Wire the new push workflow in `applyMultitierMultirepo`.**
  File: `internal/scaffolding/steps/apply_template.go` → `applyMultitierMultirepo`.
  The `frontendWfMap` currently copies `MultitierFrontendCommitStageWf`,
  `MultitierFrontendBumpPatchWf`, and `CleanupWf`. If Step 1 added the push step
  inline in `frontend-commit-stage.yml`, no extra wiring is needed here — it rides
  along with the existing workflow copy. Verify this is the case; if Step 1 used a
  separate workflow file instead, add it to `frontendWfMap`.

- [ ] **Step 4 — Retire the forward-looking note in `contracts/README.md`.**
  File: `shop/contracts/README.md`. Replace the
  `> **Note:** this mechanism is **forward-looking**...` blockquote with a one-line
  note: "In the multitier multirepo scaffold, this step is wired by `gh optivem init`."

- [ ] **Step 5 — Compile and run affected tests.**
  From `gh-optivem/`: `go build ./...` then `go test ./internal/scaffolding/...`.
  From `shop/`: no compilation required (markdown + workflow template only).

## Out of scope

- Step 3 of the shop plan (Dockerized Pact Broker spike) — separate opt-in lesson.
- Fine-grained PAT / GitHub App hardening for `REPO_TOKEN` — follow-on.
- Multi-repo scaffolding for monolith (already handled separately, different token flow).
- Consumer-side `can-i-deploy` gate — broker lesson only.

## Open questions

_(none — all resolved above)_
