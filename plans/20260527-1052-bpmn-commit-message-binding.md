# Plan: BPMN commit subprocess must pass a commit message to `gh optivem commit` — remaining work

> 🤖 **Picked up by agent** — `Valentina_Desk` at `2026-05-27T09:19:45Z`

Items 1–5 landed (commit pending operator approval). Only the end-to-end rehearsal re-run remains.

## Items

### Item 6 — Re-run the rehearsal end-to-end

After Items 1-5 land, re-run `bash ../gh-optivem/scripts/atdd-rehearsal.sh 69 --config gh-optivem-monolith-typescript.yaml` and walk it past the first commit gate. Confirms the four call sites all reach `gh optivem commit "<message>"` successfully and that the BPMN does not regress on any other action.

## Verification

- `bash scripts/atdd-rehearsal.sh 69 --config gh-optivem-monolith-typescript.yaml` walks past `COMMIT_TEST_CODE` (and ideally further) without `commit message is required` errors.
- `git log` on the rehearsal branch shows the per-site message literals (`[69] Add product search`).
