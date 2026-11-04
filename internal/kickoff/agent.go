package kickoff

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/optivem/gh-optivem/internal/atdd/assets"
	"github.com/optivem/gh-optivem/internal/atdd/process/clauderun"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/agents"
	"github.com/optivem/gh-optivem/internal/expand"
)

// agentsDir holds the kickoff agent prompts inside the embedded assets. They
// are kept apart from the ATDD agents: they run outside a ticket, so they
// carry no ticket preamble and no phase scope.
const agentsDir = "runtime/agents/kickoff"

// Agent names, matching the prompt files under agentsDir.
const (
	AgentPrototyper    = "ux-prototyper"
	AgentDiagramWriter = "use-case-diagram-writer"
	AgentUseCaseWriter = "use-case-writer"
	AgentStoryWriter   = "story-writer"
)

// AgentNames lists every kickoff agent.
var AgentNames = []string{AgentPrototyper, AgentDiagramWriter, AgentUseCaseWriter, AgentStoryWriter}

// agentWriteScope is where each agent may write, as repo-relative slash-form
// path prefixes. A run that changes anything else fails: the prompt states the
// scope, and this check enforces it.
var agentWriteScope = map[string][]string{
	AgentPrototyper:    {PrototypeDir + "/"},
	AgentDiagramWriter: {DiagramFile},
	AgentUseCaseWriter: {UseCasesDir + "/"},
	AgentStoryWriter:   {StoriesDraft + "/"},
}

// RenderAgent returns the prompt for the named agent with vars substituted,
// plus its model/effort tuning. A placeholder left unfilled is an error.
func RenderAgent(name string, vars map[string]string) (string, agents.Tuning, error) {
	set := agents.NewAgentSet(agentsDir)
	tuning, err := set.LoadTuning(name)
	if err != nil {
		return "", agents.Tuning{}, err
	}
	data, err := fs.ReadFile(assets.FS, agentsDir+"/"+name+".md")
	if err != nil {
		return "", agents.Tuning{}, fmt.Errorf("kickoff: no prompt for agent %q", name)
	}
	body := stripFrontmatter(string(data))
	merged := map[string]string{}
	for k, v := range vars {
		merged[k] = v
	}
	if name == AgentStoryWriter {
		for key, asset := range map[string]string{"ac-format": "runtime/shared/ac-format.md", "escc-format": "runtime/shared/escc-format.md"} {
			chunk, err := fs.ReadFile(assets.FS, asset)
			if err != nil {
				return "", agents.Tuning{}, fmt.Errorf("kickoff: read %s: %w", asset, err)
			}
			merged[key] = strings.TrimRight(string(chunk), "\n")
		}
	}
	// Substitute the shared chunks last so a chunk's own text is never re-expanded.
	chunks := map[string]string{}
	for _, k := range []string{"ac-format", "escc-format"} {
		if v, ok := merged[k]; ok {
			chunks[k] = v
			delete(merged, k)
		}
	}
	rendered := expand.Apply(body, merged)
	rendered = expand.Apply(rendered, chunks)
	if unfilled := expand.FindUnfilled(rendered); len(unfilled) > 0 {
		return "", agents.Tuning{}, fmt.Errorf("kickoff: agent %s has unfilled placeholders: %s", name, strings.Join(unfilled, ", "))
	}
	return strings.TrimRight(rendered, "\n") + "\n", tuning, nil
}

func stripFrontmatter(s string) string {
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		return s
	}
	rest := s[strings.Index(s, "\n")+1:]
	for _, marker := range []string{"\n---\n", "\n---\r\n"} {
		if i := strings.Index("\n"+rest, marker); i >= 0 {
			return ("\n" + rest)[i+len(marker):]
		}
	}
	return s
}

// AgentRunner runs kickoff agents headless through the claude CLI and
// enforces each agent's write scope.
type AgentRunner struct {
	Layout Layout
	Claude clauderun.ClaudeRunner
	Stdout io.Writer
	Stderr io.Writer
	// Snapshot overrides the working-tree snapshot used for the scope check
	// (tests only).
	Snapshot func(root string) (map[string]string, error)
}

// Run renders the agent, runs it, and fails when it changed files outside its
// write scope.
func (r AgentRunner) Run(ctx context.Context, name string, vars map[string]string) error {
	prompt, tuning, err := RenderAgent(name, vars)
	if err != nil {
		return err
	}
	snapshot := r.Snapshot
	if snapshot == nil {
		snapshot = snapshotTree
	}
	before, err := snapshot(r.Layout.Root)
	if err != nil {
		return fmt.Errorf("snapshot before %s: %w", name, err)
	}
	if _, err := r.Claude.Run(ctx, clauderun.RunOpts{
		Prompt:   prompt,
		Headless: true,
		Model:    tuning.Model,
		Effort:   tuning.Effort,
		Dir:      r.Layout.Root,
		Stdin:    os.Stdin,
		Stdout:   r.Stdout,
		Stderr:   r.Stderr,
	}); err != nil {
		return fmt.Errorf("agent %s failed: %w", name, err)
	}
	after, err := snapshot(r.Layout.Root)
	if err != nil {
		return fmt.Errorf("snapshot after %s: %w", name, err)
	}
	if out := outOfScope(before, after, agentWriteScope[name]); len(out) > 0 {
		return fmt.Errorf("agent %s changed files outside its scope (%s): %s", name, strings.Join(agentWriteScope[name], ", "), strings.Join(out, ", "))
	}
	return nil
}

// outOfScope returns the paths that differ between two snapshots and sit
// outside every allowed prefix.
func outOfScope(before, after map[string]string, allowed []string) []string {
	var bad []string
	seen := map[string]bool{}
	check := func(p string) {
		if seen[p] || before[p] == after[p] {
			return
		}
		seen[p] = true
		for _, a := range allowed {
			if p == a || (strings.HasSuffix(a, "/") && strings.HasPrefix(p, a)) {
				return
			}
		}
		bad = append(bad, p)
	}
	for p := range before {
		check(p)
	}
	for p := range after {
		check(p)
	}
	sort.Strings(bad)
	return bad
}

// snapshotTree maps every dirty or untracked path in the git working tree to a
// hash of its content ("" for a deleted file), so an agent editing an already
// dirty file is still detected.
func snapshotTree(root string) (map[string]string, error) {
	cmd := exec.Command("git", "-C", root, "status", "--porcelain", "-z", "-uall")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	snap := map[string]string{}
	entries := strings.Split(string(out), "\x00")
	for i := 0; i < len(entries); i++ {
		e := entries[i]
		if len(e) < 4 {
			continue
		}
		status, path := e[:2], e[3:]
		if status[0] == 'R' || status[0] == 'C' {
			i++ // the original path follows as its own entry
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			snap[path] = ""
			continue
		}
		sum := sha256.Sum256(data)
		snap[path] = fmt.Sprintf("%x", sum)
	}
	return snap, nil
}
