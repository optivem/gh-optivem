// Package agents resolves the agent layer of a process: the per-role prompt
// bodies a user-task dispatches, their model/effort tuning, plus the registry
// that maps YAML `agent:` names to NodeFn dispatchers.
//
// # Bring your own agents (the agent-set swap point)
//
// An AgentSet binds a directory of `<name>.md` prompt files within some
// filesystem. The bodies, their mandatory model/effort frontmatter, and the
// agent roster all resolve against that binding:
//
//   - DefaultAgentSet() — the built-in ATDD set, rooted at "runtime/agents/atdd"
//     within the embedded assets.FS. The zero-config default.
//   - NewAgentSet(root) — an alternate root within that same assets.FS.
//   - NewAgentSetFS(fsys, root) — an alternate root in *any* fs.FS: a reusing
//     caller's own embed.FS of prompts, or a test fixture FS — without shipping
//     those prompts in gh-optivem's asset tree.
//
// Prompt, LoadTuning and Names are methods on the set, so two sets coexist side
// by side. The set is threaded through clauderun.Options.AgentSet and
// driver.Options.AgentSet (nil → DefaultAgentSet), so binding an alternate set
// rebinds the whole run's agent layer with no change to the process YAML.
//
// The five shared dispatch chunks (preamble, scope, fixer-preamble, and the
// interactive/headless suffixes) stay package-global — they are mode and
// doctrine concerns every set must honour, not per-set content — so a swapped
// set replaces only the per-agent body, not the universal framing.
// InteractiveSuffix/HeadlessSuffix are methods only for a uniform set-owned API.
//
// internal/atdd/runtime/driver/swappable_agentset_test.go is the worked
// example: the unchanged embedded process-flow.yaml rebound to an in-memory
// stub set (via NewAgentSetFS), asserting the dispatched prompt comes from the
// stub body while the shared chunks remain.
//
// # The two swap points
//
// This package owns the *agent-set* swap point (bring your own agents). The
// companion *process* swap point — bring your own BPMN, run it on the generic
// engine — lives in internal/engine/statemachine via LoadBytes.
//
// # Registry
//
// Registry maps agent names from the YAML to NodeFn dispatchers, with a
// built-in `human` STOP dispatcher for `agent: human` nodes that block on
// stdin without dispatching anything. The driver registers one dispatcher per
// embedded agent at startup (see driver.registerAgentDispatchers), so adding an
// agent is: drop a `<name>.md` under the bound set's root and recompile.
package agents
