package agents

import (
	"fmt"
	"sort"
	"strings"

	"github.com/optivem/gh-optivem/internal/assets"
	"gopkg.in/yaml.v3"
)

const (
	promptsDir     = "runtime/prompts/atdd"
	preamblePath   = "runtime/shared/preamble.md"
	scopePath      = "runtime/shared/scope.md"
	sessionEndPath = "runtime/shared/session-end.md"
)

// sharedPreamble is the universal ticket-vars + don't-commit/summarise block
// prepended to every agent prompt. sharedScope is the universal phase-scope
// doctrine inserted between preamble and body — every agent must honour the
// scope-exception contract, so the rule belongs in argv rather than a Read
// directive resolved per dispatch. sharedSessionEnd is the universal "end
// your reply with /exit cue" rule appended. All three load once at init so a
// missing asset fails the binary at startup rather than at first dispatch.
var (
	sharedPreamble   = mustReadAsset(preamblePath)
	sharedScope      = mustReadAsset(scopePath)
	sharedSessionEnd = mustReadAsset(sessionEndPath)
)

func mustReadAsset(path string) string {
	data, err := assets.FS.ReadFile(path)
	if err != nil {
		panic("agents: read embedded " + path + ": " + err.Error())
	}
	return strings.TrimRight(string(data), "\n")
}

// Tuning is the per-agent claude-CLI tuning declared in the prompt's
// YAML frontmatter. Both fields are mandatory — every embedded agent
// must declare its model and effort explicitly so an operator can see
// at a glance what the dispatch will cost, and a careless edit can't
// silently fall back to whatever the operator's session default
// happens to be.
//
// Allowed values mirror the `claude` CLI flags:
//   - model:  "sonnet" | "haiku" | "opus" (or a full id like
//     "claude-sonnet-4-6")
//   - effort: "low" | "medium" | "high" | "xhigh" | "max"
//
// Frontmatter shape:
//
//	---
//	model: sonnet
//	effort: medium
//	---
//	You are the Test Agent. ...
//
// Validation happens at LoadTuning — a missing field is a hard error
// the driver propagates, halting dispatch before claude is invoked.
type Tuning struct {
	Model  string `yaml:"model"`
	Effort string `yaml:"effort"`
}

// Prompt returns the embedded prompt body for the given agent name,
// with the shared preamble prepended and the shared session-end rule
// appended. YAML frontmatter (if any) is stripped — fetch it via Tuning.
// Returns an error if no prompt is embedded under that name.
// The returned content uses ${name} substitution placeholders matching
// the YAML's ExpandParams dialect — callers run statemachine.ExpandParams
// against the live ticket context before passing the result to `claude -p`.
func Prompt(name string) (string, error) {
	data, err := assets.FS.ReadFile(promptsDir + "/" + name + ".md")
	if err != nil {
		return "", fmt.Errorf("agents: no embedded prompt for %q", name)
	}
	_, body := splitFrontmatter(string(data))
	body = strings.TrimRight(body, "\n")
	return sharedPreamble + "\n\n" +
		sharedScope + "\n\n" +
		body + "\n\n---\n\n" +
		sharedSessionEnd + "\n", nil
}

// HasNoneScope reports whether the named agent's prompt frontmatter
// declares `scope: none`. A `none` declaration is a doctrinal exemption
// from `internal/atdd/phase-scopes.yaml` — the agent mutates only
// inter-phase artifacts or external systems, never the repo working
// tree (see runtime/shared/scope.md "scope: none" section). The
// frontmatter is the SSoT for the exemption; no sibling Go allowlist.
//
// The frontmatter's `scope:` value is a small sum-type: scalar `"none"`
// for the exemption, or a map (today always empty `{}`) for layer-pinned
// phases whose real scope lives in phase-scopes.yaml. We decode into
// yaml.Node so both shapes parse without errors, then discriminate by
// kind/value.
//
// Returns an error if the prompt is missing or the frontmatter fails to
// parse — never silently false on parse error, since the reverse-FK
// drift guard depends on this answer being authoritative.
func HasNoneScope(name string) (bool, error) {
	data, err := assets.FS.ReadFile(promptsDir + "/" + name + ".md")
	if err != nil {
		return false, fmt.Errorf("agents: no embedded prompt for %q", name)
	}
	fm, _ := splitFrontmatter(string(data))
	if fm == "" {
		return false, nil
	}
	var probe struct {
		Scope yaml.Node `yaml:"scope"`
	}
	if err := yaml.Unmarshal([]byte(fm), &probe); err != nil {
		return false, fmt.Errorf("agents: %q: parse scope frontmatter: %w", name, err)
	}
	return probe.Scope.Kind == yaml.ScalarNode && probe.Scope.Value == "none", nil
}

// LoadTuning returns the model/effort tuning declared in the named
// agent's prompt frontmatter. Every error path is fatal to dispatch:
//
//   - Missing prompt file → error.
//   - No frontmatter at all → error (every agent must declare tuning).
//   - Frontmatter present but missing `model` or `effort` → error.
//   - Frontmatter that fails to parse → error.
//
// Hard-failing on missing frontmatter (rather than silently inheriting
// the operator's session default) is intentional: the operator's
// session default is typically Opus + max, and a forgotten frontmatter
// on a mechanical-scaffolding agent is exactly the cost-spike class
// this whole mechanism was added to prevent.
func LoadTuning(name string) (Tuning, error) {
	data, err := assets.FS.ReadFile(promptsDir + "/" + name + ".md")
	if err != nil {
		return Tuning{}, fmt.Errorf("agents: no embedded prompt for %q", name)
	}
	fm, _ := splitFrontmatter(string(data))
	t, err := parseTuningFrontmatter(fm)
	if err != nil {
		return Tuning{}, fmt.Errorf("agents: %q: %w", name, err)
	}
	return t, nil
}

// parseTuningFrontmatter validates a frontmatter block and returns the
// parsed Tuning. Empty frontmatter, parse failure, or a missing
// required field are all errors. Exposed (lowercase, package-internal)
// so the validator can be tested without round-tripping through the
// embedded asset filesystem.
func parseTuningFrontmatter(fm string) (Tuning, error) {
	if fm == "" {
		return Tuning{}, fmt.Errorf("no frontmatter — every agent must declare `model:` and `effort:`")
	}
	var t Tuning
	if err := yaml.Unmarshal([]byte(fm), &t); err != nil {
		return Tuning{}, fmt.Errorf("parse frontmatter: %w", err)
	}
	if t.Model == "" {
		return Tuning{}, fmt.Errorf("frontmatter missing required `model:` field")
	}
	if t.Effort == "" {
		return Tuning{}, fmt.Errorf("frontmatter missing required `effort:` field")
	}
	return t, nil
}

// splitFrontmatter peels a leading `---\n...\n---\n` YAML block off s.
// Returns (frontmatter-yaml-without-fences, remaining-body). A file
// with no frontmatter returns ("", original-content). Tolerant of
// CRLF: callers may have edited the prompt on Windows.
func splitFrontmatter(s string) (string, string) {
	const marker = "---"
	// Must start with the opening marker on its own line.
	first, rest, ok := cutLine(s)
	if !ok || strings.TrimRight(first, "\r") != marker {
		return "", s
	}
	// Find the closing marker line.
	end := indexLine(rest, marker)
	if end < 0 {
		return "", s
	}
	fm := rest[:end]
	body := rest[end:]
	// Drop the closing marker line itself.
	_, body, _ = cutLine(body)
	return fm, body
}

// cutLine splits s at the first '\n' and returns (line-without-newline,
// remainder, found). When no '\n' exists, returns (s, "", false).
func cutLine(s string) (string, string, bool) {
	i := strings.IndexByte(s, '\n')
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+1:], true
}

// indexLine returns the byte offset of the line whose content (after
// trimming a trailing CR) equals target, or -1 if no such line exists.
func indexLine(s, target string) int {
	pos := 0
	for pos < len(s) {
		nl := strings.IndexByte(s[pos:], '\n')
		var line string
		if nl < 0 {
			line = s[pos:]
		} else {
			line = s[pos : pos+nl]
		}
		if strings.TrimRight(line, "\r") == target {
			return pos
		}
		if nl < 0 {
			return -1
		}
		pos += nl + 1
	}
	return -1
}

// Names returns every embedded agent name, sorted. The driver uses this to
// register a dispatcher per embedded prompt at startup, replacing the v1
// hand-maintained agentNames slice. Adding a new agent is now: drop the
// prompt under internal/assets/runtime/prompts/atdd/, recompile.
func Names() []string {
	entries, err := assets.FS.ReadDir(promptsDir)
	if err != nil {
		// assets.FS is built from a //go:embed directive; ReadDir on a
		// declared subtree cannot fail in a built binary. Panic surfaces a
		// build/embed-config bug rather than letting an empty registry
		// silently bind a YAML referencing valid agents.
		panic("agents: read embedded " + promptsDir + ": " + err.Error())
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		if name == e.Name() {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
