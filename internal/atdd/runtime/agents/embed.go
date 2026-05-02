package agents

import (
	"embed"
	"fmt"
	"sort"
	"strings"
)

//go:embed prompts/*.md shared/*.md
var promptFS embed.FS

// sharedSessionEnd is the universal "end your reply with /exit cue" rule
// appended to every agent prompt. Loaded once at init so a missing file
// fails the binary at startup rather than at first dispatch.
var sharedSessionEnd = mustReadShared("shared/session-end.md")

// sharedLegacyCommitBlock is the pre-CLI-commits "Commit Confirmation Rule"
// reference that used to be inlined into every committing leaf prompt.
// Re-injected at render time when --cli-commits is off, replacing the
// LegacyCommitConfirmationMarker placeholder.
var sharedLegacyCommitBlock = mustReadShared("shared/legacy-commit-confirmation.md")

func mustReadShared(path string) string {
	data, err := promptFS.ReadFile(path)
	if err != nil {
		panic("agents: read embedded " + path + ": " + err.Error())
	}
	return strings.TrimRight(string(data), "\n")
}

// Prompt returns the embedded prompt template for the given agent name,
// with the shared session-end rule appended. Returns an error if no
// prompt is embedded under that name. The returned content uses ${name}
// substitution placeholders matching the YAML's ExpandParams dialect —
// callers run statemachine.ExpandParams against the live ticket context
// before passing the result to `claude -p`.
func Prompt(name string) (string, error) {
	data, err := promptFS.ReadFile("prompts/" + name + ".md")
	if err != nil {
		return "", fmt.Errorf("agents: no embedded prompt for %q", name)
	}
	body := strings.TrimRight(string(data), "\n")
	return body + "\n\n---\n\n" + sharedSessionEnd + "\n", nil
}

// LegacyCommitBlock returns the pre-CLI-commits commit-confirmation rule
// reference. Used by clauderun.renderPrompt to swap back the legacy
// commit-gating language when --cli-commits is off.
func LegacyCommitBlock() string {
	return sharedLegacyCommitBlock
}

// Names returns every embedded agent name, sorted. The driver uses this to
// register a dispatcher per embedded prompt at startup, replacing the v1
// hand-maintained agentNames slice. Adding a new agent is now: drop the
// prompt under prompts/, recompile.
func Names() []string {
	entries, err := promptFS.ReadDir("prompts")
	if err != nil {
		// promptFS is built from a //go:embed directive; ReadDir on the
		// declared root cannot fail in a built binary. Panic surfaces a
		// build/embed-config bug rather than letting an empty registry
		// silently bind a YAML referencing valid agents.
		panic("agents: read embedded prompts/: " + err.Error())
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
