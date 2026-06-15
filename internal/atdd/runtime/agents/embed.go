package agents

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/optivem/gh-optivem/internal/assets"
	"gopkg.in/yaml.v3"
)

const (
	defaultAgentsDir      = "runtime/agents/atdd"
	preamblePath          = "runtime/shared/preamble.md"
	scopePath             = "runtime/shared/scope.md"
	fixerPreamblePath     = "runtime/shared/fixer-preamble.md"
	interactiveSuffixPath = "runtime/shared/interactive-suffix.md"
	headlessSuffixPath    = "runtime/shared/headless-no-ask.md"
)

// sharedPreamble is the universal ticket-vars + don't-commit/summarise block
// prepended to every agent prompt. sharedScope is the universal phase-scope
// doctrine inserted between preamble and body — every agent must honour the
// scope-exception contract, so the rule belongs in argv rather than a Read
// directive resolved per dispatch. fixerPreamble is the failure-kind fixers'
// shared contract — the git read-carve-out + one-attempt/approval-gated/
// stay-in-scope block — inserted between scope and the body for `*-fixer`
// agents only (see isFixer), so the contract lives in one place instead of
// being copied into all five bodies. interactiveSuffix is the operator-facing
// /exit + redirect hint appended only when the dispatcher invokes the agent
// interactively (see InteractiveSuffix). headlessSuffix is the symmetric
// counterpart appended only when the dispatcher invokes the agent headless
// (see HeadlessSuffix) — the no-`AskUserQuestion` clause, since a headless
// run has no operator to answer. All load once at init so a missing asset
// fails the binary at startup rather than at first dispatch.
var (
	sharedPreamble    = mustReadAsset(preamblePath)
	sharedScope       = mustReadAsset(scopePath)
	fixerPreamble     = mustReadAsset(fixerPreamblePath)
	interactiveSuffix = mustReadAsset(interactiveSuffixPath)
	headlessSuffix    = mustReadAsset(headlessSuffixPath)
)

func mustReadAsset(path string) string {
	data, err := assets.FS.ReadFile(path)
	if err != nil {
		panic("agents: read embedded " + path + ": " + err.Error())
	}
	return strings.TrimRight(string(data), "\n")
}

// AgentSet binds a concrete agent-prompt directory so an alternate set of
// prompts can be supplied at load time instead of being fixed at package
// init. Two pieces of instance state — the filesystem (fsys) and the root
// directory within it holding the per-agent `<name>.md` prompt files — let
// two sets coexist side by side rather than one global root. Prompt,
// LoadTuning and Names resolve against (fsys, root); the five shared chunks
// (preamble/scope/fixer-preamble/the two suffixes) stay package-global
// because they are dispatch-level doctrine and mode concerns every set must
// honour, not per-set content — so InteractiveSuffix/HeadlessSuffix are
// methods only for a uniform set-owned API, and return the global chunks
// regardless of binding.
//
// The default set reads from the built-in assets.FS; a third party bringing
// their own agents (or a test binding a stub set) supplies their own fs.FS
// via NewAgentSetFS — the "bring your own agents" swap point.
type AgentSet struct {
	fsys fs.FS
	root string
}

// NewAgentSet binds an alternate agent set rooted at the given directory
// within the built-in assets.FS (a path holding `<name>.md` prompt files).
// To bind a set from a different filesystem — a third party's own embed.FS,
// or a test fixture FS — use NewAgentSetFS.
func NewAgentSet(root string) *AgentSet {
	return NewAgentSetFS(assets.FS, root)
}

// NewAgentSetFS binds an agent set rooted at `root` within an arbitrary
// fs.FS. This is the filesystem swap point: a reusing process supplies its
// own embed.FS of `<name>.md` prompts (each with the mandatory model/effort
// frontmatter), and tests supply a stub/fixture FS, without shipping those
// prompts in gh-optivem's own assets tree. The shared dispatch chunks still
// come from the global assets.FS regardless.
func NewAgentSetFS(fsys fs.FS, root string) *AgentSet {
	return &AgentSet{fsys: fsys, root: root}
}

// DefaultAgentSet returns the built-in ATDD agent set — the zero-config
// default rooted at "runtime/agents/atdd" within assets.FS. Production
// callers use this; only tests (and future alternate processes) bind a
// different set.
func DefaultAgentSet() *AgentSet {
	return NewAgentSet(defaultAgentsDir)
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
// with the shared preamble + scope rule prepended. YAML frontmatter
// (if any) is stripped — fetch it via Tuning. Returns an error if no
// prompt is embedded under that name. The returned content uses
// ${name} substitution placeholders matching the YAML's ExpandParams
// dialect — callers run statemachine.ExpandParams against the live
// ticket context before passing the result to `claude -p`.
func (s *AgentSet) Prompt(name string) (string, error) {
	data, err := fs.ReadFile(s.fsys, s.root+"/"+name+".md")
	if err != nil {
		return "", fmt.Errorf("agents: no embedded prompt for %q", name)
	}
	_, body := splitFrontmatter(string(data))
	body = strings.TrimRight(body, "\n")
	prompt := sharedPreamble + "\n\n" + sharedScope + "\n\n"
	if isFixer(name) {
		prompt += fixerPreamble + "\n\n"
	}
	return prompt + body + "\n", nil
}

// isFixer reports whether name is one of the failure-kind fixer agents.
// process-flow.yaml's `fix` MID dispatches `${failure-kind}-fixer`, so the
// `-fixer` suffix is the same load-bearing dispatch convention
// TestFixKindAgentsExist pins. These agents — and only these — get
// fixerPreamble prepended between scope.md and the body.
func isFixer(name string) bool {
	return strings.HasSuffix(name, "-fixer")
}

// InteractiveSuffix returns the operator-facing block the dispatcher
// appends to interactive (non-headless) prompts — explaining how to
// close the session (`/exit`) and how to redirect the agent. Kept in
// the agents package so the embedded asset has a single owner; the
// dispatcher decides per-dispatch whether to append it based on
// Options.Headless.
func (s *AgentSet) InteractiveSuffix() string { return interactiveSuffix }

// HeadlessSuffix returns the no-`AskUserQuestion` block the dispatcher
// appends to headless (`claude -p`) prompts — a headless run has no
// operator to answer, so the agent must resolve ambiguity itself and
// proceed rather than ask. Symmetric counterpart to InteractiveSuffix;
// the dispatcher decides per-dispatch which to append based on
// Options.Headless.
func (s *AgentSet) HeadlessSuffix() string { return headlessSuffix }

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
func (s *AgentSet) LoadTuning(name string) (Tuning, error) {
	data, err := fs.ReadFile(s.fsys, s.root+"/"+name+".md")
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
// agent definition under internal/assets/runtime/agents/atdd/, recompile.
func (s *AgentSet) Names() []string {
	entries, err := fs.ReadDir(s.fsys, s.root)
	if err != nil {
		// The default set's assets.FS is built from a //go:embed directive;
		// ReadDir on a declared subtree cannot fail in a built binary. Panic
		// surfaces a build/embed-config bug (or a misbound alternate set)
		// rather than letting an empty registry silently bind a YAML
		// referencing valid agents.
		panic("agents: read embedded " + s.root + ": " + err.Error())
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
