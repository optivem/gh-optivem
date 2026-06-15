// Package configcheck holds the one project-config validation rule that
// needs engine knowledge: the `task-prompts:` keys must name MID tasks the
// embedded process-flow actually declares. That rule legitimately depends on
// both the engine (statemachine — "what are the valid task names?") and the
// schema (projectconfig — "what did the operator write?"), so it lives here,
// in the process/runtime layer that already imports both, rather than inside
// projectconfig (a near-kernel leaf that has no business reaching up into the
// engine).
//
// projectconfig keeps the *value* path-validation of task-prompts (its own
// shape concern); only the engine-derived known-name check moved here.
//
// Because there is no single config-load chokepoint, the package also exposes
// load-wrappers (Load / LoadFromPath) = the projectconfig load + the
// known-name check. Runtime entry points that must enforce the rule call
// configcheck.Load/LoadFromPath; sites that only generate config (scaffolding)
// keep calling projectconfig directly, so the enforce/skip decision is
// explicit in who imports what.
package configcheck

import (
	"fmt"
	"sort"
	"strings"

	"github.com/optivem/gh-optivem/internal/atdd/process"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// LoadFromPath reads and parses the config at path (via
// projectconfig.LoadFromPath), then enforces the task-prompts known-name
// rule. Use this at runtime entry points that consume task-prompts; a typo'd
// key fails the load with the same error it always did.
func LoadFromPath(path string) (*projectconfig.Config, error) {
	cfg, err := projectconfig.LoadFromPath(path)
	if err != nil {
		return nil, err
	}
	if err := ValidateTaskPrompts(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Load reads <repoPath>/gh-optivem.yaml (via projectconfig.Load), then
// enforces the task-prompts known-name rule. A missing file returns
// (nil, nil) exactly as projectconfig.Load does — ValidateTaskPrompts is a
// no-op on a nil config.
func Load(repoPath string) (*projectconfig.Config, error) {
	cfg, err := projectconfig.Load(repoPath)
	if err != nil {
		return nil, err
	}
	if err := ValidateTaskPrompts(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// ValidateTaskPrompts rejects task-prompts: keys that are not known embedded
// MID task names, so typos surface at config-load rather than deep inside the
// pipeline. The value path-validation lives in projectconfig.Validate (run at
// parse time); this is purely the engine-backed name check. A nil config or
// empty task-prompts is accepted. Keys are iterated in sorted order so errors
// are deterministic.
func ValidateTaskPrompts(cfg *projectconfig.Config) error {
	if cfg == nil || len(cfg.TaskPrompts) == 0 {
		return nil
	}
	known, err := knownTaskNames()
	if err != nil {
		return fmt.Errorf("config: task-prompts: enumerate MID task names: %w", err)
	}
	names := make([]string, 0, len(cfg.TaskPrompts))
	for name := range cfg.TaskPrompts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !known[name] {
			return fmt.Errorf("config: task-prompts: %q is not a known embedded MID task", name)
		}
	}
	return nil
}

// knownTaskNames returns the set of MID task-name verbs declared on every
// writing-agent EXECUTE_AGENT call-activity in the embedded process-flow
// YAML. Post-plan-1701 split, task-names (verbs) and agent names (nouns)
// diverged — the schema field is keyed by task-name, so this is the right
// source. Templated task-names (e.g. "fix-${failure-kind}" on the `fix` LOW
// process) are skipped: they resolve at runtime to a concrete MID that already
// appears in this set via its own entry.
func knownTaskNames() (map[string]bool, error) {
	eng, err := process.Load()
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, proc := range eng.Processes {
		for _, node := range proc.Nodes {
			if node.Kind != statemachine.CallActivity || node.Raw.Process != "execute-agent" {
				continue
			}
			name := node.Raw.Params["task-name"]
			if name == "" || strings.Contains(name, "${") {
				continue
			}
			out[name] = true
		}
	}
	return out, nil
}
