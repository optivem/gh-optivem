# Plan: per-node implementer (human or agent), configurable in the BPMN + `gh-optivem.yaml`

> 🤖 **Picked up by agent (refine)** — `Valentina_Desk` at `2026-05-18T11:20:54Z`

**Date:** 2026-05-16
**Status:** Draft — open questions below need answers before code lands.

## Goal

Make the **implementer** of every implement-role `user_task` in
`internal/atdd/runtime/statemachine/process-flow.yaml` selectable at the
start of a run. Today each such node hard-codes one of two implementers:

- `agent: <agent-name>` — dispatch the named embedded agent via `clauderun`
  (e.g. `at-red-test`, `at-green-system-backend`, `chore`, `fix-verify`).
- `agent: human` (with `role: implement`) — STOP and wait for the operator
  to do the work manually (today: external_system_onboarding's
  `PROVISION` / `DEFINE_IFACE` / `IMPL_DRIVER` / `WRITE_SMOKE`).

The operator currently has only one global lever — `--manual-agents` —
which flips **every** agent-dispatch node into the v1 "pause + launch in a
second window" fallback. This plan introduces a finer-grained,
per-implementer choice: declare in `gh-optivem.yaml` which implement-role
nodes run as agent vs human, with an optional run-start recap so the
operator can audit the picks before any node fires.

Scope is **implement** nodes only. Review STOPs (`role: review` —
`APPROVE_CHANGE`, `STOP_RED_REVIEW`, `APPROVE_COMMIT`, gate-fail
review prompts) are out of scope; those are governance gates by design
and stay human.

## Phase 1 — BPMN shape: introduce `implementer:` as the implement-role axis

The current `agent:` field on `user_task` conflates two concerns:

1. **Who implements** (human or some agent).
2. **Which embedded prompt** to dispatch (when an agent does it).

Split them on implement-role nodes only:

1. **Add `implementer:` to the `user_task` schema** for implement-role
   nodes. Allowed values: `human` (block on stdin, no dispatch) or
   `agent` (dispatch the agent named in `agent:`).
2. **Keep `agent:` as the "which prompt"** field. On implement-role
   nodes, `agent:` MUST name a real embedded agent (member of
   `agents.Names()`) — even when `implementer: human`, so flipping back
   to agent later is one config edit, not a re-discovery exercise.
3. **Today's `agent: human` on implement nodes** (external_system_onboarding's
   four implement steps) becomes `implementer: human` with an
   *as-yet-unnamed* `agent:` value. This blocks landing this phase
   until those nodes have a named onboarding agent — see open question
   below.
4. **Review STOPs are unchanged**: they keep `agent: human, role: review`
   and have no `implementer:` field. The grammar split is what makes
   "human review STOP" and "human implementer STOP" two distinct things
   in the YAML.

### YAML shape (proposal)

```yaml
- id: WRITE
  type: user_task
  role: implement
  agent: ${agent}          # which prompt would be dispatched
  implementer: agent       # default; can be overridden to "human"
  phase_doc: ${phase_doc}
  documentation: "${phase_label} - WRITE"
```

The `implementer:` default in the YAML is the **process author's
recommendation**. The override channel is `gh-optivem.yaml` (Phase 2).

## Phase 2 — `gh-optivem.yaml`: declare per-run implementer overrides

Add a new top-level field to `projectconfig.Config`:

```yaml
implementers:
  # By embedded agent name — applies to every implement-role node that
  # dispatches that agent (covers shared sub-processes like
  # red_phase_cycle.WRITE where the agent is supplied via params:).
  at-red-test: human
  at-green-system-frontend: agent
  fix-verify: agent

  # Optional per-node-id form for surgical overrides that the per-agent
  # form can't express (e.g. the four onboarding implement steps share
  # `agent:` but the operator wants only PROVISION human).
  # Node IDs are validated against the loaded process flow at startup,
  # same way node_extras / node_replacements are.
  PROVISION: human
  DEFINE_IFACE: agent
```

Resolution precedence (most → least specific):

1. `--implementer <node-id>=<human|agent>` CLI flag (repeatable; for
   one-off run-time overrides without editing config).
2. `gh-optivem.yaml` per-node-id entry.
3. `gh-optivem.yaml` per-agent entry.
4. BPMN `implementer:` default on the node.

Validation rules (in `projectconfig.Validate`):

- Keys in the per-agent block must be members of `agents.Names()` (same
  validation as `agent_prompts:`).
- Per-node-id keys are validated at driver startup against the loaded
  process flow (same as `node_extras:` / `node_replacements:`).
- Values must be `human` or `agent` (literal strings, not booleans —
  matches the existing schema's lowercase-string convention).

## Phase 3 — Run-start recap

Before the engine fires the first node, print a one-screen recap of the
resolved implementer for every implement-role node the process actually
reaches, so the operator can abort and edit before any agent dispatch
or human STOP fires.

Two open knobs:

- **Static vs reachable**: do we list every implement-role node in
  every `processes:` entry, or only the ones the chosen entry process
  (default: `main`) can reach? Reachable is more useful but requires
  walking call_activity edges with the gateway predicates unresolved
  (so "can reach via at least one branch" is the right semantics).
- **Confirm vs print-only**: `gh optivem implement` is already verbose
  at startup (`printConfig`); a non-interactive recap that the operator
  reads-and-Ctrl-Cs is probably less friction than an extra `y/n`.
  Confirmation should be off by default and opt-in via
  `--confirm-implementers`.

## Phase 4 — Driver wiring

1. **`statemachine` schema bump**: extend `RawNode` (in
   `internal/atdd/runtime/statemachine/types.go`) with `Implementer
   string`. Validate at `LoadFile` / `LoadDefault`: only on
   implement-role `user_task` nodes; absent → defaults to `agent` if
   `agent:` names a real agent, else `human`.
2. **`agents.Registry`**: register a second built-in next to `human`
   called `human-implementer` whose dispatcher prints the rendered
   prompt body (so the human can see the same instructions the agent
   would have got) and blocks on stdin until the operator commits and
   types "done". Distinct from today's `human` (approve-style) STOP so
   we don't conflate "approve this" with "now go implement this."
3. **Driver dispatcher choice**: in `driver.go`'s
   `wrapAgentDispatchers`, resolve the implementer for each
   implement-role node by walking the precedence chain above. When
   `implementer == human`, route the dispatch to
   `human-implementer` regardless of what `agent:` names. When
   `implementer == agent`, dispatch the named agent as today.
4. **Backwards compat**: nodes with no `implementer:` and `agent: human`
   continue to dispatch the existing `human` STOP unchanged (this is
   the review path).

## Phase 5 — Interaction with existing knobs

- `--manual-agents` (v1 fallback) stays as a separate, coarser lever.
  When set, it wins over per-node `implementer: agent` and routes the
  dispatch to the manual-pause flow — that's the whole point of the
  flag, and we don't want this plan to subtly change its meaning.
  When `implementer: human`, `--manual-agents` is a no-op for that
  node (already human).
- `agent_prompts:` overrides apply to the named agent's embedded
  prompt; orthogonal to implementer choice. A node with
  `implementer: human` still renders the prompt body (so the human
  reads the same instructions) — the agent prompt override flows
  through there too.
- `node_replacements:` short-circuits the dispatcher entirely (the
  documented escape hatch); per-node `implementer:` is irrelevant when
  a replacement is in effect.

## Phase 6 — Documentation

- Update the BPMN-schema doc comment at the top of `process-flow.yaml`
  to describe `implementer:` alongside `agent:` and `role:`.
- Update `docs/how-it-works.md` (or wherever the operator-facing
  "what flags do what" section lives) with the resolution precedence.
- Cross-link from `docs/atdd-at-cycle.md` once that file lands the
  Phase 1 mechanics from `plans/20260516-1701-atdd-at-cycle-absorb-internal-assets.md`
  — the AT cycle's WRITE nodes are exactly the ones an operator will
  want to flip first ("let me hand-write this scenario before I trust
  the agent with it").

## Open questions (block implementation)

1. **Onboarding agent name** — `external_system_onboarding`'s four
   implement steps are `agent: human` today because no `atdd-onboarding`
   agent exists. Phase 1 requires naming one (even if the prompt is a
   stub) so `implementer: human` can carry an `agent:` value for later
   re-flipping. Acceptable to defer those four nodes to a follow-up,
   leaving them as legacy `agent: human` until the onboarding agent
   exists?
2. **Per-agent vs per-node-id config shape** — both, or only
   per-agent? Per-node-id is strictly more expressive but adds a
   second resolution layer and a second validation path. Real
   academy use cases probably only need per-agent (a student wants
   "let me hand-write all RED-TEST scenarios myself" = `at-red-test:
   human`, full stop).
3. **`human-implementer` STOP semantics** — does it require the
   operator to type "done" explicitly, or just press Enter? Today's
   `human` review STOP requires y/n (via `promptio.ConfirmYN`); the
   implement STOP wants a different verb because "approve" isn't
   what's happening. Proposal: print the rendered prompt + "press
   Enter once you've committed your work to the current branch", then
   let the existing HEAD-diff verify decorator confirm a commit
   landed (so the human gets the same post-condition check the agent
   would have).
4. **Scope creep guard** — should the run-start recap (Phase 3) also
   show which agents have `agent_prompts:` overrides applied? It's
   one more line per node and it'd save the operator one
   `gh optivem config show` lookup. But it widens this plan from
   "implementer choice" to "implement-role node audit," which is a
   different feature. Probably defer.

## Out of scope

- Mid-run implementer flipping (e.g. "agent failed three times, let me
  finish this WRITE by hand"). The natural recovery today is Ctrl-C,
  edit config, re-run; that workflow is fine for now.
- Auto-selection heuristics (e.g. "use human for the first WRITE in a
  new ticket, agent for the rest"). Out of scope; the BPMN default +
  static config is enough.
- Replacing `--manual-agents` with this mechanism. They serve
  different audiences — `--manual-agents` is a debugging tool, this is
  a per-project policy.
