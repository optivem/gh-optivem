# BPMN - CYCLE LEVEL

> **Design content only.** Open questions, doctrine choices, and decision rationale belong in the parent plan: `plans/20260525-1057-bpmn-refactor-design.md`. Do not record Q&A here. If you encounter a question while reading or editing this file, add it to the plan, not inline.

Per-ticket sub-processes selected by the classification gateway in `implement-ticket` (TOP).

## refine-backlog

**Inputs:**
- ticket

**Outputs:**
- refined-ticket (with acceptance criteria)

**Steps:**
1. `refine-acceptance-criteria` (ticket: `<input>`)

## onboard-external-system

**Inputs:**
- external-system-description

**Outputs:**
- documented-external-system
- reachable-external-system

**Steps:**
1. Identify external system
2. Document external system contract
3. Set up external system access (credentials, endpoints, sandbox)
4. Verify external system reachable

## change-system-behavior

**Inputs:**
- acceptance-criteria

**Outputs:**
- modified-system-and-tests

**Steps:**
1. `write-and-verify-tests-fail` (acceptance-criteria: `<input>`)
2. `implement-and-verify-system` (agent-action: implement-system)
3. Refactor (loopable — after the chosen CYCLE returns, ask again):
    - `refactor-system-structure` → opportunistic mode (no checklist)
    - `refactor-test-structure` → opportunistic mode (no checklist)
    - `redesign-system-structure` → opportunistic mode (no checklist)
    - **none** → exit the loop; cycle ends.

## cover-system-behavior

**Inputs:**
- acceptance-criteria

**Outputs:**
- new-tests

**Steps:**
1. `write-and-verify-tests-pass` (acceptance-criteria: `<input>`)

## redesign-system-structure

**Inputs:**
- redesign-checklist (optional — opportunistic mode supplies none)

**Outputs:**
- restructured-system

**Steps:**
1. Implement driver adapters:
    1. `implement-system-driver-adapters`
    2. `implement-external-system-driver-adapters`
2. `implement-and-verify-system` (agent-action: implement-system)

## refactor-system-structure

**Inputs:**
- refactor-checklist (optional — opportunistic mode supplies none)

**Outputs:**
- refactored-system

**Steps:**
1. `implement-and-verify-system` (agent-action: refactor-system)

## refactor-test-structure

**Inputs:**
- refactor-checklist (optional — opportunistic mode supplies none)

**Outputs:**
- refactored-tests

**Steps:**
1. `refactor-and-verify-tests`
