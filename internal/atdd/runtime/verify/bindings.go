// Bindings — per-node pre/post-condition checks.
//
// The verify package wraps every NodeFn in the engine through middleware.go's
// Wrap. This file supplies the table of which checks apply to which node.
// Today the checks are commit-message HEAD-prefix guards, anchored to the
// at-cycle-conventions.md commit-message format (`<Ticket> | <Phase>`).
//
// Each check is best-effort: a missing git binary or a fresh repo with no
// HEAD commit returns "skip" rather than an error so transitions tests and
// dry-runs still walk the process. Hard authoring bugs (HEAD points at a
// different phase than the predecessor would have produced) surface as Err.
package verify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
)

// GitRunner is the seam for shell-outs to `git`. The default implementation
// runs the real binary; tests inject a fake.
type GitRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// Deps bundles the verifier's collaborators.
type Deps struct {
	Git GitRunner
}

func (d Deps) withDefaults() Deps {
	if d.Git == nil {
		d.Git = realGit{}
	}
	return d
}

// Bindings returns the canonical Pre/Post check map. Keys are YAML node IDs;
// values point at the closures the engine should wrap around the node's Fn.
//
// A nil PreFn / PostFn means "no check for this side" — Wrap already
// handles nil hooks transparently.
func Bindings(deps Deps) map[string]struct {
	Pre  PreFn
	Post PostFn
} {
	deps = deps.withDefaults()
	requireHead := func(phase string) PreFn {
		return func(nodeID string, ctx *statemachine.Context) error {
			return requireHeadMatches(deps, phase, nodeID)
		}
	}
	return map[string]struct {
		Pre  PreFn
		Post PostFn
	}{
		// AT cycle — each WRITE must be preceded by the matching commit.
		"AT_RED_DSL":             {Pre: requireHead("AT - RED - TEST")},
		"CT_RED_DSL":             {Pre: requireHead("CT - RED - TEST")},
		"CT_RED_EXTERNAL_SYSTEM_DRIVER": {Pre: requireHead("CT - RED - DSL")},
	}
}

// WrapAll applies every binding from Bindings to the matching node in every
// process of the supplied engine. Call this once during driver startup, after
// engine.Bind. Nodes without a binding pass through unchanged.
//
// We mutate the engine's process nodes in place so the engine's RunProcess loop
// sees the wrapped Fn the next time it dispatches the node.
func WrapAll(eng *statemachine.Engine, deps Deps) {
	bindings := Bindings(deps)
	for _, process := range eng.Processes {
		for id, node := range process.Nodes {
			b, ok := bindings[id]
			if !ok {
				continue
			}
			node.Fn = Wrap(node.Fn, id, b.Pre, b.Post)
			process.Nodes[id] = node
		}
	}
}

// requireHeadMatches reads the latest commit message via `git log -1
// --pretty=%B` and confirms it contains the expected phase suffix. The
// match is `… | <phase>` (with optional trailing whitespace) so a
// `#42 | Register Customer | AT - RED - TEST` message is accepted.
//
// A repo with no commits, or a missing git binary, is treated as "skip"
// (return nil) — the user is most likely running transitions tests or a
// dry-run, and we do not want to crash the engine on infrastructure that
// isn't there.
func requireHeadMatches(deps Deps, phase, nodeID string) error {
	out, err := deps.Git.Run(context.Background(), "log", "-1", "--pretty=%B")
	if err != nil {
		// Look for the canonical "fatal: your current branch … does not
		// have any commits yet" / "not a git repository" diagnostics. We
		// soft-skip those — the verify check is informational, not
		// load-bearing for a fresh sandbox.
		msg := err.Error()
		if strings.Contains(msg, "does not have any commits") ||
			strings.Contains(msg, "not a git repository") ||
			strings.Contains(msg, "executable file not found") {
			return nil
		}
		return fmt.Errorf("verify[%s]: read HEAD commit message: %w", nodeID, err)
	}
	headMsg := strings.TrimSpace(string(out))
	if headMsg == "" {
		// Empty repo / no HEAD — skip rather than fail.
		return nil
	}
	if !phaseSuffixRe(phase).MatchString(headMsg) {
		return fmt.Errorf("verify[%s]: HEAD commit message %q does not end with %q phase suffix",
			nodeID, firstLine(headMsg), phase)
	}
	return nil
}

// phaseSuffixRe builds a regex that matches `… | <phase>` (case-sensitive,
// any leading content, trailing whitespace allowed). We anchor on `|` to
// mirror the at-cycle-conventions.md format and avoid matching `<phase>`
// substrings inside the body.
func phaseSuffixRe(phase string) *regexp.Regexp {
	// Escape regex meta in `phase` defensively. The known phases are
	// pure-ASCII with hyphens and spaces, but escaping keeps the contract
	// safe for future additions.
	return regexp.MustCompile(`(?m)\|\s*` + regexp.QuoteMeta(phase) + `\s*$`)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// ErrSkip is returned by helpers that want to opt out of a check (e.g.
// "no HEAD commit yet"); not currently used externally but kept here so
// future check authors have a clear pattern.
var ErrSkip = errors.New("verify: check skipped")

// realGit is the default GitRunner.
type realGit struct{}

func (realGit) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out, fmt.Errorf("git %s: %w (stderr: %s)",
				strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
		}
		return out, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}
