package agents

import (
	"fmt"
	"sort"
	"strings"

	"github.com/optivem/gh-optivem/internal/assets"
)

const (
	promptsDir     = "runtime/prompts/atdd"
	preamblePath   = "runtime/shared/preamble.md"
	sessionEndPath = "runtime/shared/session-end.md"
)

// sharedPreamble is the universal ticket-vars + don't-commit/summarise block
// prepended to every agent prompt. sharedSessionEnd is the universal "end
// your reply with /exit cue" rule appended. Both load once at init so a
// missing asset fails the binary at startup rather than at first dispatch.
var (
	sharedPreamble   = mustReadAsset(preamblePath)
	sharedSessionEnd = mustReadAsset(sessionEndPath)
)

func mustReadAsset(path string) string {
	data, err := assets.FS.ReadFile(path)
	if err != nil {
		panic("agents: read embedded " + path + ": " + err.Error())
	}
	return strings.TrimRight(string(data), "\n")
}

// Prompt returns the embedded prompt template for the given agent name,
// with the shared preamble prepended and the shared session-end rule
// appended. Returns an error if no prompt is embedded under that name.
// The returned content uses ${name} substitution placeholders matching
// the YAML's ExpandParams dialect — callers run statemachine.ExpandParams
// against the live ticket context before passing the result to `claude -p`.
func Prompt(name string) (string, error) {
	data, err := assets.FS.ReadFile(promptsDir + "/" + name + ".md")
	if err != nil {
		return "", fmt.Errorf("agents: no embedded prompt for %q", name)
	}
	body := strings.TrimRight(string(data), "\n")
	return sharedPreamble + "\n\n" + body + "\n\n---\n\n" + sharedSessionEnd + "\n", nil
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
