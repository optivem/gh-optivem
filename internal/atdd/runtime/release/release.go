// Package release owns the end-of-cycle release mechanics for the ATDD
// pipeline driver: commit and close the GitHub issue.
//
// The package is deliberately mechanical and small. It exposes two
// primitives:
//
//   - Commit shells out to `git add -A` + `git commit -m <msg>`. It REQUIRES
//     a non-nil Confirmer — there is no way to silently commit. Callers
//     that want non-interactive use must pass a Confirmer that auto-returns
//     true; the explicit handshake makes "skip the gate" visible at the
//     call site rather than buried in a flag.
//   - CloseIssue shells out to `gh issue close <N>`.
//
// Note: this package used to also expose `RemoveDisabledMarkers` for
// stripping `@Disabled` markers from test files at the end of the AT cycle.
// The disable/enable apparatus (deterministic, then the test-disabler /
// test-enabler agents) was removed entirely once WIP acceptance tests
// gained a permanent env-var gate (GH_OPTIVEM_RUN_WIP_TESTS) the
// orchestrator lifts only at verify time — no marker is ever applied or
// stripped. See clauderun.renderGateMarkerExample and actions.runCommand.
package release

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/optivem/gh-optivem/internal/approval"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/outlog"
	"github.com/optivem/gh-optivem/internal/promptio"
)

// Confirmer asks the user to approve a destructive action. Implementations
// return (true, nil) to proceed, (false, nil) to abort cleanly, or
// (false, err) on I/O failure. A nil Confirmer is rejected by Commit; the
// "ask before every commit" gate is firm user policy and the only way to
// opt out is to pass a Confirmer that auto-returns true.
type Confirmer func(prompt string) (bool, error)

// CommitOptions controls a Commit call.
type CommitOptions struct {
	Message   string
	Confirm   Confirmer // mandatory; nil → error.
	GitRunner GitRunner // optional injection point for tests; nil → real exec.
	// Stdout is the back-compat fallback for callers that pre-date the
	// level architecture. Out, when non-nil, takes precedence — Commit
	// writes its staged-changes summary and message echo at Phase level
	// (operator-facing). nil → os.Stdout.
	Stdout io.Writer
	Out    *outlog.Out
}

// GitRunner is the test seam for `git` invocations. Tests pass a fake;
// production callers pass nil and Commit uses the real `exec` runner.
type GitRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// GhRunner is the test seam for `gh` invocations.
type GhRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// ErrCommitDeclined is returned by Commit when the Confirmer returns false.
var ErrCommitDeclined = errors.New("release: commit declined by user")

// ErrConfirmerRequired is returned by Commit when CommitOptions.Confirm is
// nil. This guards the "ask before commit" policy at the type-system level.
var ErrConfirmerRequired = errors.New("release: Commit requires a Confirmer (no silent commits)")

// -------------------------------------------------------------------------
// Commit
// -------------------------------------------------------------------------

// Commit stages all working-tree changes (`git add -A`) and creates a
// commit with the given message — but only after the user explicitly
// approves via the supplied Confirmer.
//
// The "ask before every commit" gate is firm user policy across the
// pipeline; nil Confirm returns ErrConfirmerRequired so a forgetful caller
// can't accidentally bypass it.
func Commit(ctx context.Context, opts CommitOptions) error {
	if opts.Confirm == nil {
		return ErrConfirmerRequired
	}
	if strings.TrimSpace(opts.Message) == "" {
		return errors.New("release: Commit requires a non-empty Message")
	}
	runner := opts.GitRunner
	if runner == nil {
		runner = realGit{}
	}
	// Operator-facing commit banner is Phase level. Out, when supplied
	// by the driver, routes the writes through every sink that accepts
	// Phase (terminal + --log-file). Fall back through opts.Stdout for
	// callers that haven't migrated.
	var stdout io.Writer
	switch {
	case opts.Out != nil:
		stdout = opts.Out.Phase
	case opts.Stdout != nil:
		stdout = opts.Stdout
	default:
		stdout = os.Stdout
	}

	// Stage everything. We deliberately use `git add -A` (full working-tree
	// stage) — the pipeline convention forbids slicing commits with --paths,
	// see CLAUDE.md / the "no --paths flag on commit script" memory.
	if _, err := runner.Run(ctx, "add", "-A"); err != nil {
		return fmt.Errorf("release: git add -A: %w", err)
	}

	// Show the user what they're about to commit. `git status --short`
	// gives a one-line-per-file summary that fits the Confirmer prompt.
	statusOut, err := runner.Run(ctx, "status", "--short")
	if err != nil {
		return fmt.Errorf("release: git status: %w", err)
	}
	fmt.Fprintln(stdout, "Staged changes:")
	if len(bytes.TrimSpace(statusOut)) == 0 {
		fmt.Fprintln(stdout, "  (none)")
	} else {
		fmt.Fprintln(stdout, string(statusOut))
	}
	fmt.Fprintf(stdout, "Commit message: %s\n", opts.Message)

	ok, err := opts.Confirm("Commit these changes?")
	if err != nil {
		return fmt.Errorf("release: confirmer: %w", err)
	}
	if !ok {
		return ErrCommitDeclined
	}

	if _, err := runner.Run(ctx, "commit", "-m", opts.Message); err != nil {
		return fmt.Errorf("release: git commit: %w", err)
	}
	return nil
}

// CloseIssue shells out to `gh issue close <N>`. The Confirmer policy is
// applied at the Commit boundary, not here — by convention closing an
// issue happens immediately after an already-approved final commit.
func CloseIssue(ctx context.Context, issueNum int, gh GhRunner) error {
	if issueNum <= 0 {
		return fmt.Errorf("release: CloseIssue requires a positive issue number, got %d", issueNum)
	}
	if gh == nil {
		gh = realGh{}
	}
	if _, err := gh.Run(ctx, "issue", "close", fmt.Sprintf("%d", issueNum)); err != nil {
		return fmt.Errorf("release: gh issue close %d: %w", issueNum, err)
	}
	return nil
}

// -------------------------------------------------------------------------
// Confirmer helper
// -------------------------------------------------------------------------

// InteractiveConfirmer returns a Confirmer backed by the shared promptio
// helper: the prompt is suffixed with " [y/n]: ", input is case-insensitive,
// unrecognised answers (including bare Enter) re-prompt, EOF returns false.
// See internal/promptio for the canonical semantics — every human y/n
// decision in the CLI funnels through that package.
func InteractiveConfirmer(stdin io.Reader, stdout io.Writer) Confirmer {
	if stdin == nil {
		stdin = os.Stdin
	}
	if stdout == nil {
		stdout = os.Stdout
	}
	return func(prompt string) (bool, error) {
		return promptio.ConfirmYN(stdin, stdout, prompt)
	}
}

// ApprovalConfirmer returns a Confirmer that routes through the global
// auto-approve policy with CategoryHuman — release is always-prompt under
// the new tier ladder (the non-implement reclassification is deferred to a
// follow-up plan).
//
// Released as a sibling to InteractiveConfirmer so the driver layer can
// pick the right Confirmer at wire-up time based on whether a Resolved
// policy is in play. Callers without a Resolved (tests, scripts) keep
// using InteractiveConfirmer; the driver passes ApprovalConfirmer when
// it threads Options.Approval into the release.Commit call.
func ApprovalConfirmer(r approval.Resolved, stdin io.Reader, stdout io.Writer) Confirmer {
	if stdin == nil {
		stdin = os.Stdin
	}
	if stdout == nil {
		stdout = os.Stdout
	}
	return func(prompt string) (bool, error) {
		// TODO(non-implement-tiering): placeholder; proper tier assignment
		// deferred to the follow-up plan. See plan
		// 20260528-0930-approval-tier-ladder.md §D5.
		return approval.Confirm(r, approval.CategoryHuman, stdin, stdout, prompt)
	}
}

// -------------------------------------------------------------------------
// Real exec runners
// -------------------------------------------------------------------------

type realGit struct{}

func (realGit) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("git %s: %w (stderr: %s)", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

type realGh struct{}

func (realGh) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("gh %s: %w (stderr: %s)", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}
