package actions

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// ---------------------------------------------------------------------------
// Adapter shims (different runner interfaces across packages must not leak)
// ---------------------------------------------------------------------------

// ghAdapter exists because each underlying package (tracker) defines its
// own GhRunner interface — Go's structural typing means we can wrap once
// instead of teaching every package to depend on a shared runner type.
// The wrapper is zero-cost.
type ghAdapter struct{ inner GhRunner }

func (g ghAdapter) Run(ctx context.Context, args ...string) ([]byte, error) {
	return g.inner.Run(ctx, args...)
}

// ---------------------------------------------------------------------------
// Default exec runners + stdin prompter
// ---------------------------------------------------------------------------

type realGh struct{}

func (realGh) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out, fmt.Errorf("gh %s: %w (stderr: %s)",
				strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
		}
		return out, fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

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

// realShell teas child-process stdio to two writers: the live operator-
// facing sink (Detail level by default, populated by withDefaults from
// Deps.Out.Detail) and an in-memory buffer the action body parses for
// ShellResult.Stdout / .Stderr. Both writers default to os.Stdout / Stderr
// when constructed directly (test paths that skip Deps.withDefaults).
type realShell struct {
	stdout, stderr io.Writer
}

func (r realShell) Run(ctx context.Context, commandLine string) (ShellResult, error) {
	// We deliberately route through the user's shell so command lines like
	// `./test-all.sh --sample` and `bash -lc compile-all.sh` work uniformly.
	// On Windows, gh-optivem ships against bash via the Git Bash shim; if
	// that is missing the user gets a clear "executable file not found"
	// from os/exec.
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "bash"
	}
	cmd := exec.CommandContext(ctx, shell, "-c", commandLine)
	// Tee the child's stdio: stream live to the operator's terminal so
	// long-running commands (docker compose build, gradle, etc.) show
	// progress instead of looking hung, and capture into buffers so the
	// returned ShellResult still carries stdout for callers that parse it
	// (e.g. `gh optivem test run --list`) and stderr is still inlined
	// into the error message on failure.
	//
	// The live sinks are Detail-level by default (the operator's terminal
	// only sees them with --verbose; --log-file always gets them) — a
	// shift from the pre-outlog behaviour where realShell wrote straight
	// to os.Stdout, bypassing the log-file mirror entirely. Zero-value
	// writers fall back to os.Stdout / os.Stderr for direct-construction
	// callers.
	stdoutSink, stderrSink := r.stdout, r.stderr
	if stdoutSink == nil {
		stdoutSink = os.Stdout
	}
	if stderrSink == nil {
		stderrSink = os.Stderr
	}
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(stdoutSink, &stdoutBuf)
	cmd.Stderr = io.MultiWriter(stderrSink, &stderrBuf)
	err := cmd.Run()
	result := ShellResult{Stdout: stdoutBuf.Bytes(), Stderr: stderrBuf.Bytes()}
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			result.ExitCode = ee.ExitCode()
			return result, fmt.Errorf("shell %q: %w (stderr: %s)",
				commandLine, err, strings.TrimSpace(stderrBuf.String()))
		}
		// Process never started (binary not found, etc.) — exec.ExitError
		// is not in the chain, so we leave ExitCode at its zero value;
		// callers that surface command-exit-code into state still get a
		// stable int, just one that signals "no exit code observed".
		return result, fmt.Errorf("shell %q: %w", commandLine, err)
	}
	return result, nil
}

type stdinPrompter struct{}

func (stdinPrompter) Ask(prompt string) (string, error) {
	if _, err := fmt.Fprint(os.Stderr, prompt); err != nil {
		return "", err
	}
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return line, nil
}
