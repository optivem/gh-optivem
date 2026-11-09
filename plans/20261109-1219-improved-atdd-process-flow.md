# 2026-11-09 11:19:28 UTC — Improved ATDD process flow

## TL;DR

**Why:** Since early November the process has been described publicly in four places:
- the SDD-for-QA talk (`substack/meetups/2026-11-05-spec-driven-development-for-qa.html`)
- the 2026-11-06 article batch (`substack/articles/{drafts/next,published}/2026-11-06-*`)
- the Circle session of 2026-10-30 (`substack/circle/2026-10-30/`)

Together they describe a sharper process than `internal/atdd/process/process-flow.yaml` runs today. The biggest differences:
- **Roles per step.** The articles split every step into executor, reviewer and approver. The BPMN only has agents plus approve gates.
- **Reviewer agents.** The articles have an adversarial reviewer in Red and review-first in Refactor. The BPMN has neither.
- **Align.** The articles describe an Align loop that checks the story against the use cases.
- **Acceptance criteria after green.** The articles delete the acceptance criteria once their tests pass. The BPMN keeps them and fingerprints them.
- **Refactor verification.** The articles run only the acceptance tests for the touched use case locally and leave the full suite to the pipeline. Parked for a later discussion (Q5).

**End result:** `gh optivem` runs one coherent flow, **Align → Update Use Case → Red → Green → Refactor & Review → Done**, with an explicit executor/reviewer/approver at every step. Students who read the articles or saw the talk recognise the same process when they run the tool.

## Outcomes

- **The target flow is written down once.** A single design section (below, then promoted to docs) is the reference for the BPMN, the agents and the articles.
- **Every step names its executor, reviewer and approver**, following the articles' rule: put human approval where nothing else can check the work.
- **Red is reviewed by an adversarial agent before the human approves.** It covers the Acceptance Test, the DSL and the drivers.
- **Green runs without review or approval.** The Acceptance Tests are the check.
- **Refactor is review-first.** The reviewer produces findings and a plan, a human approves, then the agent executes. Issues a human finds feed back into the guidelines.
- **Acceptance criteria don't drift from the tests.** Policy is set by Q4.
- **Statuses stay as today.** gh-optivem ends at IN ACCEPTANCE, and humans (manual QA review) move a story to DONE (Q9).

## Proposed process flow

### How to read these diagrams

Each diagram below is an **existing process from `docs/process-diagram.md`**, redrawn with the **same node IDs** and the proposed changes on top. Processes not shown are unchanged. Examples: `verify-tests-pass`, `verify-tests-fail`, the external-system / contract-test cascade, `redesign-*`, `cover-system-behavior`, `commit`, `approve`.

```mermaid
flowchart LR
    U[Unchanged: exists today, as is]
    C[Changed: exists today, behaviour changes]
    N[New: added by this plan]
    R[Removed: dropped by this plan]
    classDef unchanged fill:#eeeeee,stroke:#9e9e9e,color:#000
    classDef changed fill:#ffe0b2,stroke:#ef6c00,stroke-width:2px,color:#000
    classDef new fill:#c8e6c9,stroke:#2e7d32,stroke-width:2px,color:#000
    classDef removed fill:#ffcdd2,stroke:#c62828,stroke-width:2px,stroke-dasharray:5 3,color:#000
    class U unchanged
    class C changed
    class N new
    class R removed
```

### Overview: where the changes land

```mermaid
flowchart TD
    REFINE_TICKET[Refine Ticket<br/>→ Align loop]
    IMPLEMENT_TICKET[Implement Ticket]
    MERGE[Merge Story Into Use Cases]
    CSB[Change System Behavior]
    RED[Write Failing Acceptance Tests<br/>AT → DSL → drivers per channel]
    GREEN[Implement System per channel]
    AC_POLICY[Acceptance-criteria policy<br/>replaces spec fingerprints]
    REFACTOR[Refactor &amp; Review]
    EXEC_AGENT[execute-agent<br/>+ optional reviewer step]
    PIPE[Pipeline: full AT suite<br/>· manual QA quick functional test<br/>→ human sets DONE]
    OTHER[Bug / Task cycles<br/>cover · redesign · refactor]

    REFINE_TICKET --> IMPLEMENT_TICKET
    IMPLEMENT_TICKET -- Story --> MERGE --> CSB
    IMPLEMENT_TICKET -- Bug --> CSB
    IMPLEMENT_TICKET -- Task --> OTHER
    CSB --> RED --> GREEN --> AC_POLICY --> REFACTOR
    RED -. uses .-> EXEC_AGENT
    REFACTOR --> PIPE

    classDef unchanged fill:#eeeeee,stroke:#9e9e9e,color:#000
    classDef changed fill:#ffe0b2,stroke:#ef6c00,stroke-width:2px,color:#000
    classDef new fill:#c8e6c9,stroke:#2e7d32,stroke-width:2px,color:#000
    class IMPLEMENT_TICKET,CSB,OTHER unchanged
    class REFINE_TICKET,MERGE,RED,GREEN,REFACTOR,EXEC_AGENT,AC_POLICY changed
    class PIPE unchanged
```

### Refine Backlog Item (Align): single pass → loop until the PO agrees

Today: `REFINE_ACCEPTANCE_CRITERIA` runs the `acceptance-criteria-refiner` once (human tier), then ends.

```mermaid
flowchart TD
    REFINE_ACCEPTANCE_CRITERIA["Refine Acceptance Criteria<br/>refiner now also: concrete numbers per rule ·<br/>contradictions vs use cases in repo ·<br/>main success = shortest path"]
    THREE_AMIGOS_REVIEW["Three Amigos review draft<br/>(human)"]
    GATE_AC_AGREED{PO: Agreed?}
    REFINE_BACKLOG_ITEM_END(( ))

    REFINE_ACCEPTANCE_CRITERIA --> THREE_AMIGOS_REVIEW
    THREE_AMIGOS_REVIEW --> GATE_AC_AGREED
    GATE_AC_AGREED -- No: feedback --> REFINE_ACCEPTANCE_CRITERIA
    GATE_AC_AGREED -- Yes --> REFINE_BACKLOG_ITEM_END

    classDef unchanged fill:#eeeeee,stroke:#9e9e9e,color:#000
    classDef changed fill:#ffe0b2,stroke:#ef6c00,stroke-width:2px,color:#000
    classDef new fill:#c8e6c9,stroke:#2e7d32,stroke-width:2px,color:#000
    class REFINE_BACKLOG_ITEM_END unchanged
    class REFINE_ACCEPTANCE_CRITERIA changed
    class THREE_AMIGOS_REVIEW,GATE_AC_AGREED new
```

`Refine Ticket` (`MARK_IN_REFINEMENT → REFINE_BACKLOG_ITEM → MARK_READY`) is unchanged around it.

### Merge Story Into Use Cases: richer use case format + story ↔ criterion link

```mermaid
flowchart TD
    CHECK_STORY_MERGE_NEEDED[[Story Needs Merge?]]
    GATE_STORY_MERGE_NEEDED{Merge Needed?}
    MERGE_USE_CASES["Merge Into Use Cases<br/>+ business rules → docs/business-rules/<br/>+ Gherkin tagged @UC-NNN.M @narrative-Xy"]
    VERIFY_STORY_MERGE[[Verify Story Merge]]
    LINK_STORY_CRITERIA[["Record story ↔ criterion IDs<br/>on the ticket"]]
    MERGE_STORY_END(( ))

    CHECK_STORY_MERGE_NEEDED --> GATE_STORY_MERGE_NEEDED
    GATE_STORY_MERGE_NEEDED -- Yes --> MERGE_USE_CASES
    GATE_STORY_MERGE_NEEDED -- No --> LINK_STORY_CRITERIA
    MERGE_USE_CASES --> VERIFY_STORY_MERGE
    VERIFY_STORY_MERGE --> LINK_STORY_CRITERIA
    LINK_STORY_CRITERIA --> MERGE_STORY_END

    classDef unchanged fill:#eeeeee,stroke:#9e9e9e,color:#000
    classDef changed fill:#ffe0b2,stroke:#ef6c00,stroke-width:2px,color:#000
    classDef new fill:#c8e6c9,stroke:#2e7d32,stroke-width:2px,color:#000
    class CHECK_STORY_MERGE_NEEDED,GATE_STORY_MERGE_NEEDED,VERIFY_STORY_MERGE,MERGE_STORY_END unchanged
    class MERGE_USE_CASES changed
    class LINK_STORY_CRITERIA new
```

### Change System Behavior: spec fingerprints → acceptance-criteria policy; refactor → Refactor & Review

```mermaid
flowchart TD
    IMPLEMENT_EXTERNAL_DRIVERS_IF_NEEDED[Implement External Drivers If Needed]
    WRITE_ACCEPTANCE_TESTS_AND_SYSTEM_ADAPTERS["Write Failing Acceptance Tests<br/>(Red: reviewer on, WIP gate as today)"]
    IMPLEMENT_AND_VERIFY_SYSTEM["Implement System<br/>(Green: API before UI;<br/>WIP gate removed when green)"]
    GATE_STORY_SPEC_RECORD{Story?}
    RECORD_SPEC_FINGERPRINTS[Record Spec Fingerprints]
    COMMIT_SPEC_FINGERPRINTS[Commit Spec Fingerprints]
    APPLY_AC_POLICY["Apply AC policy (Q4):<br/>delete green criteria from use case"]
    COMMIT_AC_POLICY["Commit Use Case<br/>category = test-commit"]
    REFACTOR_OPPORTUNISTICALLY["Refactor &amp; Review (Loopable)<br/>— see § refactor"]
    CHANGE_SYSTEM_BEHAVIOR_END(( ))

    IMPLEMENT_EXTERNAL_DRIVERS_IF_NEEDED --> WRITE_ACCEPTANCE_TESTS_AND_SYSTEM_ADAPTERS
    WRITE_ACCEPTANCE_TESTS_AND_SYSTEM_ADAPTERS --> IMPLEMENT_AND_VERIFY_SYSTEM
    IMPLEMENT_AND_VERIFY_SYSTEM --> GATE_STORY_SPEC_RECORD
    GATE_STORY_SPEC_RECORD -- Story --> APPLY_AC_POLICY
    GATE_STORY_SPEC_RECORD --> REFACTOR_OPPORTUNISTICALLY
    APPLY_AC_POLICY --> COMMIT_AC_POLICY
    COMMIT_AC_POLICY --> REFACTOR_OPPORTUNISTICALLY
    RECORD_SPEC_FINGERPRINTS -.-> COMMIT_SPEC_FINGERPRINTS
    REFACTOR_OPPORTUNISTICALLY --> CHANGE_SYSTEM_BEHAVIOR_END

    classDef unchanged fill:#eeeeee,stroke:#9e9e9e,color:#000
    classDef changed fill:#ffe0b2,stroke:#ef6c00,stroke-width:2px,color:#000
    classDef new fill:#c8e6c9,stroke:#2e7d32,stroke-width:2px,color:#000
    classDef removed fill:#ffcdd2,stroke:#c62828,stroke-width:2px,stroke-dasharray:5 3,color:#000
    class IMPLEMENT_EXTERNAL_DRIVERS_IF_NEEDED,GATE_STORY_SPEC_RECORD,CHANGE_SYSTEM_BEHAVIOR_END unchanged
    class WRITE_ACCEPTANCE_TESTS_AND_SYSTEM_ADAPTERS,IMPLEMENT_AND_VERIFY_SYSTEM,REFACTOR_OPPORTUNISTICALLY changed
    class APPLY_AC_POLICY,COMMIT_AC_POLICY new
    class RECORD_SPEC_FINGERPRINTS,COMMIT_SPEC_FINGERPRINTS removed
```

If Q4 resolves to "keep fingerprints", the two red nodes stay and the two green ones drop.

### Execute Agent: optional adversarial reviewer before the human approval

This is the single place the Red reviewer plugs in. The new `reviewer` param is set only at the Red call sites (`write-acceptance-tests`, `implement-dsl`, `implement-system-driver-adapters`), so every other agent dispatch behaves as today.

```mermaid
flowchart TD
    APPROVE_PRE[Request Approval]
    GATE_APPROVED_PRE{Approval Outcome?}
    SNAPSHOT_WORKING_TREE[[Snapshot working tree]]
    RUN_AGENT["Run agent ${agent}"]
    VALIDATE_OUTPUTS_AND_SCOPES[[Validate outputs &amp; scopes]]
    GATE_OUTPUTS_AND_SCOPES_VALID{Outputs and Scopes Valid?}
    FIX[Fix the Failure]
    GATE_REVIEWER_CONFIGURED{Reviewer configured?}
    RUN_REVIEWER["Run reviewer ${reviewer}<br/>(adversarial, read-only)<br/>AT vs criterion · DSL vocabulary ·<br/>port shape · API design"]
    GATE_REVIEW_FINDINGS{Blocking findings?}
    REVIEW_FIX_EXHAUSTED{Review fix passes &lt; cap?}
    APPROVE_POST["Confirm Approval<br/>shows reviewer findings +<br/>approver role (QA and/or Devs · Devs)"]
    GATE_APPROVED_POST{Approval Outcome?}
    EXECUTE_AGENT_END(( ))
    EXECUTE_AGENT_OUTPUT_REJECTED_END((⚡))

    APPROVE_PRE --> GATE_APPROVED_PRE
    GATE_APPROVED_PRE -- Approved --> SNAPSHOT_WORKING_TREE
    SNAPSHOT_WORKING_TREE --> RUN_AGENT
    RUN_AGENT --> VALIDATE_OUTPUTS_AND_SCOPES
    VALIDATE_OUTPUTS_AND_SCOPES --> GATE_OUTPUTS_AND_SCOPES_VALID
    GATE_OUTPUTS_AND_SCOPES_VALID -- No --> FIX
    FIX --> RUN_AGENT
    GATE_OUTPUTS_AND_SCOPES_VALID -- Yes --> GATE_REVIEWER_CONFIGURED
    GATE_REVIEWER_CONFIGURED -- No --> APPROVE_POST
    GATE_REVIEWER_CONFIGURED -- Yes --> RUN_REVIEWER
    RUN_REVIEWER --> GATE_REVIEW_FINDINGS
    GATE_REVIEW_FINDINGS -- No --> APPROVE_POST
    GATE_REVIEW_FINDINGS -- Yes --> REVIEW_FIX_EXHAUSTED
    REVIEW_FIX_EXHAUSTED -- Yes: findings to writer --> RUN_AGENT
    REVIEW_FIX_EXHAUSTED -- No: hand to human --> APPROVE_POST
    APPROVE_POST --> GATE_APPROVED_POST
    GATE_APPROVED_POST -- Approved --> EXECUTE_AGENT_END
    GATE_APPROVED_POST -- Rejected --> EXECUTE_AGENT_OUTPUT_REJECTED_END

    classDef unchanged fill:#eeeeee,stroke:#9e9e9e,color:#000
    classDef changed fill:#ffe0b2,stroke:#ef6c00,stroke-width:2px,color:#000
    classDef new fill:#c8e6c9,stroke:#2e7d32,stroke-width:2px,color:#000
    class APPROVE_PRE,GATE_APPROVED_PRE,SNAPSHOT_WORKING_TREE,RUN_AGENT,VALIDATE_OUTPUTS_AND_SCOPES,GATE_OUTPUTS_AND_SCOPES_VALID,FIX,GATE_APPROVED_POST,EXECUTE_AGENT_END,EXECUTE_AGENT_OUTPUT_REJECTED_END unchanged
    class APPROVE_POST changed
    class GATE_REVIEWER_CONFIGURED,RUN_REVIEWER,GATE_REVIEW_FINDINGS,REVIEW_FIX_EXHAUSTED new
```

The scope-exception branch (`GATE_SCOPE_EXCEPTION_REQUESTED` → ESCC / contradictory-tests / scope-violation halts) and the PRE-rejected end are omitted for readability and are unchanged.

### Write and Verify Acceptance Test Code: reviewer only (WIP gate already exists)

```mermaid
flowchart TD
    WRITE_ACCEPTANCE_TESTS["Write Acceptance Tests<br/>(reviewer = matching *-reviewer;<br/>adds WIP gate as today)"]
    COMPILE_TESTS[Compile Tests]
    START_SYSTEM[Start System]
    GATE_EXPECTED_TEST_RESULT{Expected Test Result?}
    VERIFY_TESTS_PASS_ACCEPTANCE[Verify Acceptance Tests Pass]
    VERIFY_TESTS_FAIL_ACCEPTANCE[Verify Acceptance Tests Fail]
    COMMIT_TEST_CODE[Commit Test Code]
    WAV_AT_CODE_END(( ))

    WRITE_ACCEPTANCE_TESTS --> COMPILE_TESTS
    COMPILE_TESTS --> START_SYSTEM
    START_SYSTEM --> GATE_EXPECTED_TEST_RESULT
    GATE_EXPECTED_TEST_RESULT -- Success --> VERIFY_TESTS_PASS_ACCEPTANCE
    GATE_EXPECTED_TEST_RESULT -- Failure --> VERIFY_TESTS_FAIL_ACCEPTANCE
    VERIFY_TESTS_PASS_ACCEPTANCE --> COMMIT_TEST_CODE
    VERIFY_TESTS_FAIL_ACCEPTANCE --> COMMIT_TEST_CODE
    COMMIT_TEST_CODE --> WAV_AT_CODE_END

    classDef unchanged fill:#eeeeee,stroke:#9e9e9e,color:#000
    classDef changed fill:#ffe0b2,stroke:#ef6c00,stroke-width:2px,color:#000
    classDef new fill:#c8e6c9,stroke:#2e7d32,stroke-width:2px,color:#000
    class COMPILE_TESTS,START_SYSTEM,GATE_EXPECTED_TEST_RESULT,VERIFY_TESTS_PASS_ACCEPTANCE,VERIFY_TESTS_FAIL_ACCEPTANCE,COMMIT_TEST_CODE,WAV_AT_CODE_END unchanged
    class WRITE_ACCEPTANCE_TESTS changed
```

`implement-test-layer` (DSL, system driver adapters) gets the same change: its `RUN_ACTION` passes the matching `*-reviewer`. Red tests already stay out of regular CI through the existing permanent WIP gate (`@EnabledIfEnvironmentVariable(named = "GH_OPTIVEM_RUN_WIP_TESTS", ...)`, lifted only by the orchestrator's own verify runs; see `internal/atdd/process/actions/command.go`). Nothing new is needed in Red.

### Implement and Verify System (Green): API before UI; WIP gate removed once green

```mermaid
flowchart TD
    RESOLVE_CHANNEL[["Resolve Channel<br/>order: API, then UI"]]
    GATE_CHANNEL_TOUCHED{Channel Touched?}
    RESOLVE_INNER_LOOP[[Resolve Inner Loop]]
    CHANNEL_SKIPPED(( ))
    RUN_ACTION["Run the Configured Agent — see § ${action}"]
    BUILD_SYSTEM[Build the System]
    START_SYSTEM[Start the System]
    VERIFY_TESTS_PASS[Verify Tests Pass]
    GATE_INNER_LOOP{Inner Loop Mode?}
    VERIFY_INNER_LOOP[Verify Inner Loop]
    COMMIT_SYSTEM[Commit System Changes]
    IMPL_AND_VERIFY_SYSTEM_END(( ))

    RESOLVE_CHANNEL --> GATE_CHANNEL_TOUCHED
    GATE_CHANNEL_TOUCHED -- Yes --> RESOLVE_INNER_LOOP
    GATE_CHANNEL_TOUCHED -- No --> CHANNEL_SKIPPED
    RESOLVE_INNER_LOOP --> RUN_ACTION
    RUN_ACTION --> BUILD_SYSTEM
    BUILD_SYSTEM --> START_SYSTEM
    START_SYSTEM --> VERIFY_TESTS_PASS
    VERIFY_TESTS_PASS --> GATE_INNER_LOOP
    GATE_INNER_LOOP -- None --> COMMIT_SYSTEM
    GATE_INNER_LOOP -- Test After --> VERIFY_INNER_LOOP
    VERIFY_INNER_LOOP --> COMMIT_SYSTEM
    COMMIT_SYSTEM --> GATE_LAST_CHANNEL
    GATE_LAST_CHANNEL{Last touched channel?}
    GATE_LAST_CHANNEL -- No --> IMPL_AND_VERIFY_SYSTEM_END
    GATE_LAST_CHANNEL -- Yes --> REMOVE_WIP_GATE[["Remove WIP gate line from<br/>the story's now-green ATs"]]
    REMOVE_WIP_GATE --> COMMIT_WIP_GATE_REMOVAL["Commit<br/>category = test-commit"]
    COMMIT_WIP_GATE_REMOVAL --> IMPL_AND_VERIFY_SYSTEM_END

    classDef unchanged fill:#eeeeee,stroke:#9e9e9e,color:#000
    classDef changed fill:#ffe0b2,stroke:#ef6c00,stroke-width:2px,color:#000
    classDef new fill:#c8e6c9,stroke:#2e7d32,stroke-width:2px,color:#000
    class GATE_CHANNEL_TOUCHED,RESOLVE_INNER_LOOP,CHANNEL_SKIPPED,BUILD_SYSTEM,START_SYSTEM,GATE_INNER_LOOP,VERIFY_INNER_LOOP,COMMIT_SYSTEM,IMPL_AND_VERIFY_SYSTEM_END unchanged
    class RUN_ACTION,VERIFY_TESTS_PASS unchanged
    class RESOLVE_CHANNEL changed
    class GATE_LAST_CHANNEL,REMOVE_WIP_GATE,COMMIT_WIP_GATE_REMOVAL new
```

The `TDD → INNER_LOOP_TDD_UNSUPPORTED` halt is omitted and unchanged.

### Refactor (Refactor & Review): two new review-first options

```mermaid
flowchart TD
    GATE_REFACTOR_TYPE_CHOICE{Refactor Type?}
    REVIEW_CODE_AND_ARCHITECTURE[Review Code &amp; Architecture]
    REVIEW_UX_UI[Review UX / UI]
    REFACTOR_SYSTEM_STRUCTURE[Refactor System Structure]
    REFACTOR_TEST_STRUCTURE[Refactor Test Structure]
    REDESIGN_SYSTEM_STRUCTURE[Redesign System Structure]
    REDESIGN_EXTERNAL_SYSTEM_STRUCTURE[Redesign External System Structure]
    REFACTOR_TOP_END(( ))

    GATE_REFACTOR_TYPE_CHOICE -- Code &amp; Architecture Review --> REVIEW_CODE_AND_ARCHITECTURE
    GATE_REFACTOR_TYPE_CHOICE -- UX / UI Review --> REVIEW_UX_UI
    GATE_REFACTOR_TYPE_CHOICE -- Refactor System Structure --> REFACTOR_SYSTEM_STRUCTURE
    GATE_REFACTOR_TYPE_CHOICE -- Refactor Test Structure --> REFACTOR_TEST_STRUCTURE
    GATE_REFACTOR_TYPE_CHOICE -- Redesign System Structure --> REDESIGN_SYSTEM_STRUCTURE
    GATE_REFACTOR_TYPE_CHOICE -- Redesign External System Structure --> REDESIGN_EXTERNAL_SYSTEM_STRUCTURE
    GATE_REFACTOR_TYPE_CHOICE -- None --> REFACTOR_TOP_END
    REVIEW_CODE_AND_ARCHITECTURE --> GATE_REFACTOR_TYPE_CHOICE
    REVIEW_UX_UI --> GATE_REFACTOR_TYPE_CHOICE
    REFACTOR_SYSTEM_STRUCTURE --> GATE_REFACTOR_TYPE_CHOICE
    REFACTOR_TEST_STRUCTURE --> GATE_REFACTOR_TYPE_CHOICE
    REDESIGN_SYSTEM_STRUCTURE --> GATE_REFACTOR_TYPE_CHOICE
    REDESIGN_EXTERNAL_SYSTEM_STRUCTURE --> GATE_REFACTOR_TYPE_CHOICE

    classDef unchanged fill:#eeeeee,stroke:#9e9e9e,color:#000
    classDef changed fill:#ffe0b2,stroke:#ef6c00,stroke-width:2px,color:#000
    classDef new fill:#c8e6c9,stroke:#2e7d32,stroke-width:2px,color:#000
    class REFACTOR_SYSTEM_STRUCTURE,REDESIGN_SYSTEM_STRUCTURE,REDESIGN_EXTERNAL_SYSTEM_STRUCTURE,REFACTOR_TOP_END unchanged
    class REFACTOR_TEST_STRUCTURE unchanged
    class GATE_REFACTOR_TYPE_CHOICE changed
    class REVIEW_CODE_AND_ARCHITECTURE,REVIEW_UX_UI new
```

Refactor verification keeps running the whole suite, as today (Q5). Narrowing it is parked in a follow-up plan.

### Review Code & Architecture (new process)

```mermaid
flowchart TD
    RUN_REVIEWER["Reviewer agent: findings + plan<br/>(read-only, reads guidelines)"]
    APPROVE_PLAN{Human approves plan?}
    EXECUTE_PLAN[Run system-refactorer / test-refactorer on the plan]
    COMPILE_AND_START[Build, start system]
    VERIFY_TOUCHED["Verify: unit tests +<br/>full acceptance suite (as today)"]
    COMMIT_REFACTOR[Commit refactoring]
    HUMAN_REVIEW{Human review: issues<br/>the reviewer missed?}
    UPDATE_GUIDELINES["Agent: fix issue · update guidelines ·<br/>fix other occurrences"]
    REVIEW_END(( ))

    RUN_REVIEWER --> APPROVE_PLAN
    APPROVE_PLAN -- Rejected --> REVIEW_END
    APPROVE_PLAN -- Approved --> EXECUTE_PLAN
    EXECUTE_PLAN --> COMPILE_AND_START --> VERIFY_TOUCHED --> COMMIT_REFACTOR --> HUMAN_REVIEW
    HUMAN_REVIEW -- None --> REVIEW_END
    HUMAN_REVIEW -- Yes --> UPDATE_GUIDELINES
    UPDATE_GUIDELINES --> COMPILE_AND_START

    classDef new fill:#c8e6c9,stroke:#2e7d32,stroke-width:2px,color:#000
    class RUN_REVIEWER,APPROVE_PLAN,EXECUTE_PLAN,COMPILE_AND_START,VERIFY_TOUCHED,COMMIT_REFACTOR,HUMAN_REVIEW,UPDATE_GUIDELINES,REVIEW_END new
```

### Review UX / UI (new process; deferred to its own plan, Q6)

```mermaid
flowchart TD
    START_APP[Start system]
    RUN_UI_REVIEWER["Reviewer agent: UI consistency ·<br/>naming consistency (findings, no changes)"]
    HUMAN_UI_REVIEW["Human review in the running app<br/>→ one consolidated plan"]
    EXECUTE_UI_PLAN[Agent executes plan]
    RESTART_APP[Restart system]
    VISUAL_CHECK{Human visual check OK?}
    UPDATE_UI_GUIDELINES["Agent: update guidelines ·<br/>fix other occurrences"]
    COMMIT_UI[Commit]
    UI_REVIEW_END(( ))

    START_APP --> RUN_UI_REVIEWER --> HUMAN_UI_REVIEW --> EXECUTE_UI_PLAN --> RESTART_APP --> VISUAL_CHECK
    VISUAL_CHECK -- No --> UPDATE_UI_GUIDELINES --> EXECUTE_UI_PLAN
    VISUAL_CHECK -- Yes --> COMMIT_UI --> UI_REVIEW_END

    classDef new fill:#c8e6c9,stroke:#2e7d32,stroke-width:2px,color:#000
    class START_APP,RUN_UI_REVIEWER,HUMAN_UI_REVIEW,EXECUTE_UI_PLAN,RESTART_APP,VISUAL_CHECK,UPDATE_UI_GUIDELINES,COMMIT_UI,UI_REVIEW_END new
```

### Implement Ticket: status after the run

The CLI flow is unchanged and still ends at `MARK_IN_ACCEPTANCE`. The pipeline runs the full AT suite, and a human (manual QA review) moves the story to **DONE** (Q9: no automatic status transition).

### Not changed by this plan

Kept as they are today:
- `main`, `implement-ticket` routing (story/bug/task + subtypes)
- `cover-system-behavior`, `redesign-*`
- the external-driver / contract-test cascade (`implement-external-drivers-if-needed` and below)
- `verify-tests-pass` / `verify-tests-fail` and their fix loops
- `commit`, `approve`, `execute-command`, `fix`, `setup-tests`, `build-system`, `start-system`

The legacy **Baseline** (Q-Step 10) is not drawn yet. Whether it extends `cover-system-behavior` or becomes a kickoff mode is still open.

### Roles per step

| Step | Executor | Reviewer | Approver | Today in BPMN |
|---|---|---|---|---|
| 0. Baseline (legacy) | Agent | Domain experts + the running system | Human | `cover-system-behavior` (per ticket, no diagram/narrative generation) |
| 1. Align | Agent (`acceptance-criteria-refiner`) | Three Amigos | PO (human) | `refine-ticket`, single agent pass, no loop |
| 2. Update use case | Agent (`use-case-merger`) | Deterministic `VERIFY_STORY_MERGE` | Human (POST approval) | `merge-story-into-use-cases`. Matches. |
| 3. Red: AT | Agent (`acceptance-test-writer`) | **Adversarial agent** (AT vs criterion) | QA and/or Developers | Writer + `execute-agent` PRE/POST approval, **no reviewer**; approval prompt doesn't name a role. Commit order (AT + DSL port stubs → DSL → adapters per channel → system per channel) already matches the talk. |
| 3. Red: DSL | Agent (`dsl-implementer`) | **Adversarial agent** (business vocabulary) | Developers | Same, no reviewer |
| 3. Red: drivers | Agent (`system-driver-adapter-implementer`) | **Adversarial agent** (port shape, API design) | Developers | Same, no reviewer |
| 4. Green | Agent (`system-implementer`) | The Acceptance Tests | The Acceptance Tests | Matches; approvals kept as today (Q3). **Gap:** the WIP gate is never removed, so the tests skip in regular CI forever |
| 5a. Refactor: code/arch | Agent | **Adversarial agent, then human** | Human approves the plan, then reviews the result | `refactor` runs the refactorer directly, **no review-first** |
| 5b. Refactor: UX/UI | Agent | **Adversarial agent and human, app running** | Human | **Missing** |
| 6. Done | Pipeline runs full suite | Manual QA | Human | Matches. CLI ends at IN ACCEPTANCE; humans set DONE (Q9). |

### Step details

**0. Baseline (legacy, once).** Source: the coverage article.
1. Use case diagram.
2. Core use case: narrative + business rules + Gherkin acceptance criteria.
3. Acceptance test architecture.
4. The core use case's tests pass against the existing system. They are characterization tests: on failure, fix the test, criterion and narrative, never the system, and never weaken an assertion.
5. Repeat for the other use cases, one use case per commit.

**1. Align.**
- The agent drafts the acceptance criteria. Every business rule gets at least one example with concrete numbers ("a rule in general terms always leaves room for a second reading").
- The agent checks for contradictions **inside the story** and **against the use cases in the repo**, pointing at the narrative step, the criterion ID and the tests that would turn red.
- The agent checks that the main success scenario is the shortest path; optional steps become alternatives.
- Three Amigos refine with the agent. A loop gateway asks "Agreed?".
- The PO approves, and the status moves to READY. **Changes:** a single pass becomes a loop, plus the use-case contradiction check.

**2. Update use case.** The story goes into the use case **before** any test is written (narrative step, business rule, Gherkin criterion tagged `@UC-NNN.M @narrative-Xy`). Business rules live in `docs/business-rules/UC-NNN-*.md`. The criterion IDs the story introduced or changed are recorded on the ticket (story ↔ criterion link). **Changes:** business-rules file, `@narrative` tags, story↔criterion link. Coordinate with shop `plans/20261106-1533-use-cases-match-substack-articles.md`.

**3. Red.** Commit shape, from the talk's gift-wrap demo:

| Commit | Content | Approver |
|---|---|---|
| 1 | Acceptance Test + DSL signatures stubbed `TODO: DSL` | QA and/or Developers |
| 2 | DSL | Developers |
| 3…n | Drivers, one commit per channel | Developers |

After each writer, before the human gate, that layer's **reviewer agent** (`acceptance-test-reviewer`, `dsl-reviewer`, `system-driver-adapter-reviewer`) runs in adversarial mode. They check:
- **AT:** every Given/When/Then has a line in the test, every value matches, the tag points at the right criterion, and nothing is in the test that isn't in the criterion (e.g. a dropped `shouldSucceed()`).
- **DSL:** words are in the business language and match the use case ("coupon", not "voucher").
- **Driver port:** one method per use case, request/response shape, no loose parameters.
- **API driver:** REST design is consistent (plural collections, ID in the path, verb for the action).

Findings go to the writer for a fix pass (capped), then to the human approver with the findings attached. The human chooses review depth by complexity and risk, and driver/API design is always a human decision. When the human finds something the reviewer missed, the same guidelines loop as in 5 applies: fix it, update the guidelines, and find other occurrences (Circle 2026-10-30).

Circle 2026-10-30 additions to Red:
- **Only write red tests for the scenarios about to be implemented.** Don't start Red unless Green follows immediately.
- **Red commits keep the pipeline green.** Already true today through the permanent WIP gate (env var `GH_OPTIVEM_RUN_WIP_TESTS`). **Gap:** the gate is never removed, so after Green the pipeline still skips those tests. **Decision:** Green removes the gate line from the story's tests once they pass (Q7).
- **A changed test** is edited in place and gated like a new one until Green (Q8).
- **Feature flags only for months-long epics** (old and new ATs both active). Never one flag per story.

**4. Green.** System, one commit per channel, API before UI. No reviewer agent; the Acceptance Tests are the check. Human approvals stay as today (Q3). **Changes:** per-channel order pinned (API first), and once the story's tests are green the WIP-gate line is removed from them so the pipeline runs them (Q7, Step 9b).

**5. Refactor & Review (optional, loopable, explicit skip).**
- **5a. Code/architecture review:**
  1. The reviewer agent writes findings and a plan, with no changes, reading the guidelines files.
  2. The human approves the plan.
  3. The agent executes the plan.
  4. Run unit tests and the full acceptance suite (as today, Q5).
  5. The human reviews the result.
- **5b. UX/UI review:**
  1. Start the app.
  2. The reviewer agent checks UI and naming consistency.
  3. The human reviews; the two reviews merge into one plan.
  4. The agent executes the plan.
  5. Restart the app and do a visual check.
- **Guidelines feedback loop:** when the human finds an issue the reviewer missed, the agent (1) fixes it, (2) updates the guidelines file, and (3) finds and fixes other occurrences across the codebase. Guideline files are kept concrete and pruned.

**6. Done.**
- The pipeline runs the full acceptance suite.
- Manual QA does a quick functional test of the story in IN ACCEPTANCE, then a human moves it to DONE (Q9, as today).
- Acceptance criteria are deleted, generated or kept after green, per Q4.

## ▶ Next executable step (resume here)

All open questions are decided (Q1–Q10), and the work is split into per-stage plans (see the table under `## Steps`). Next: review each stage plan with `/refine-plan <file>`. The only step executable right now without review is Step 1 of `plans/20261109-1300-remove-wip-gate-at-green.md` (read-only: confirm the gap). Step 1 here (docs) can run via `/execute-plan`.

## Steps

This file is the umbrella design (Q1). The work is split into one plan per stage. Review and execute each one separately:

| Stage | Plan | State |
|---|---|---|
| Docs | Step 1 below (this file) | ready |
| 1. Align | ~~`plans/20261109-1255-align-refinement-loop.md`~~ | **done** (this commit): refine-backlog-item loops until the Three Amigos agree. The refiner's POST approval is the verdict (no separate review node); a "no" asks what should change and re-runs the refiner with that feedback and its open questions (`${refinement-feedback-section}`); 5 rounds max, then AC_REFINEMENT_EXHAUSTED prints the unresolved questions. The refiner now checks a concrete example per business rule, contradictions with `docs/use-cases` (criterion id, narrative step, tests), and main success scenario = shortest path. Operator: refine a shop story with a rule stated only in general terms and confirm the refiner proposes a numeric example, then answer "no" once and confirm the next round uses the feedback. |
| 2. Use case format | `plans/20261109-1256-use-case-format-and-story-link.md` | to review (coordinate with shop plan 20261106-1533) |
| 3. Red reviewers | ~~`plans/20261109-1257-red-reviewer-agents.md`~~ | **done** (commits 7aad5a58, f62fbf69 and the Step 6/7 commit): three read-only `*-reviewer` agents check the Red writers' output before approval; blocking findings go back to the writer (2 passes max), the approval names the approver role and lists the findings; channels run API before UI. Operator: gift-wrap rehearsal on shop with a planted dropped `.then().shouldSucceed()` and confirm the AT reviewer flags it. |
| 4/6. AC policy | `plans/20261109-1258-ac-delete-and-generate.md` | to review (ordering vs. use-case format plan) |
| 5a. Code & Architecture Review | ~~`plans/20261109-1259-code-architecture-review.md`~~ | **done** (commits dcbf03b1, a00ec26a and the Step 4 commit): the `refactor` menu offers `review-code-and-architecture`: a read-only code-architecture-reviewer proposes findings plus a system/test refactoring plan, the human approves it, the refactorers execute it as their Checklist with the full suite, then the human lists remaining issues and the guidelines-updater fixes them, records a rule under `${guidelines-path}` (default `docs/guidelines`) and fixes other occurrences (3 rounds max, each a `GUIDELINES` commit). Operator: on shop, run the `refactor` menu -> `review-code-and-architecture`, approve the plan, list one issue at HUMAN_REVIEW, and confirm `docs/guidelines/<area>.md` gains a rule, other occurrences are fixed, and a `GUIDELINES` commit lands. |
| 4. Green: WIP gate removal | ~~`plans/20261109-1300-remove-wip-gate-at-green.md`~~ | **done** (commits 83c01a71, 95c8cc5f and the Step 4/5 commit): gate stripped after Green in change-system-behavior and cover-system-behavior. Operator: re-run a shop rehearsal (e.g. #71 gift-wrap) and confirm the final commit has no gate and the pipeline runs the tests. |
| 0. Legacy baseline | `plans/20261109-1301-legacy-baseline-mode.md` | discussion |
| 5b. UX/UI Review | `plans/20261109-1302-ux-ui-review.md` | deferred until 5a ships |
| Refactor test scope | `plans/20261109-1250-refactor-touched-use-case-tests.md` | discussion (parked Q5) |

- [ ] Step 1: Promote `## Proposed process flow` into the process docs as the target design. Use the location the BPMN doc-blocks reference, with a "today vs target" column. Do not regenerate `docs/process-diagram.md` (CI does it).
- [ ] Step 2: When every stage plan is executed or dropped, delete this umbrella plan.

## Verification

- Operator: gift-wrap rehearsal on shop. Commits and approve prompts match the talk's "Gift-Wrap Commits" slide, and the Red reviewer catches a planted dropped `shouldSucceed()`.

## Open questions

- ~~Q1~~ **Decided:** this file is the umbrella design, with one plan per stage spun out.
- ~~Q2~~ **Decided:** three reviewers (`acceptance-test-reviewer`, `dsl-reviewer`, `system-driver-adapter-reviewer`), one paired with each Red writer, sharing one `runtime/shared/` chunk for the adversarial stance. Each run loads only its own checks (fewer tokens), and focused prompts are more accurate.
- ~~Q3~~ **Decided:** Green keeps today's approvals (PRE/POST, skipped under `--auto`).
- ~~Q4~~ **Decided:** delete the acceptance criteria on green, and add a `generate` command for POs/customers. Spec fingerprints are retired.
- ~~Q5~~ **Decided:** keep running the whole acceptance suite after a refactoring for now. Running only the touched use case's tests is parked in a follow-up discussion plan (`plans/*-refactor-touched-use-case-tests.md`).
- ~~Q6~~ **Decided:** UX/UI review comes later, in its own plan, after the Code & Architecture Review ships. Its diagram stays here as the target design.
- ~~Q7~~ **Decided:** red tests are already gated today (permanent WIP gate, env var). New: Green removes the gate line from the story's tests once they pass, so the pipeline runs them from then on (Step 9b).
- ~~Q8~~ **Decided:** a story that changes an existing acceptance test edits it **in place**. Red adds the WIP gate to the edited test, and Green removes it, the same path as new tests. No duplicate "old + new" tests. This matches the "Use cases stay, user stories go" article. Accepted risk: if a run halts between Red and Green, that behaviour is unchecked until the story is finished.
- ~~Q9~~ **Decided:** keep statuses as today. gh-optivem ends at IN ACCEPTANCE; manual QA does a quick functional test, then a human moves the story to DONE. No pipeline-driven status transition.
- ~~Q10~~ **Decided:** exploratory and usability testing stay outside gh-optivem. They are human QA work at IN ACCEPTANCE, alongside the quick functional test.
- **Out of scope (pipeline, not process flow):** scheduled AT runs (hourly default config, other platform variants at lower frequency, conditional on the default passing). If wanted, it belongs to a shop/pipeline plan.
