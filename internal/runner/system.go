package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// defaultUpTimeout caps a single `docker compose up -d` attempt. Picked to be
// 3x the observed legitimate worst-case (~90s for a multi-service stack with
// fresh image pulls), so a stuck Docker Hub pull surfaces as a retryable
// failure instead of consuming the entire test budget.
const defaultUpTimeout = 5 * time.Minute

// SystemOptions tunes behavior of Up/Down. Zero-values are safe defaults.
type SystemOptions struct {
	// LogLines controls how many trailing lines of compose logs are dumped on
	// a health-probe failure during Up. Zero defaults to 50.
	LogLines int
	// Restart, when true, makes Up tear down the system before starting it
	// again. When false, Up first checks IsAnyURLUp; if the system already
	// responds, Up is a no-op.
	Restart bool
	// Health overrides default polling parameters.
	Health HealthOptions
	// UpTimeout caps a single `docker compose up -d` attempt. Zero uses
	// defaultUpTimeout. The Up retry loop treats a timeout as transient and
	// retries, so the effective ceiling is roughly UpTimeout * maxAttempts.
	UpTimeout time.Duration
}

// BuildOptions tunes behavior of Build. Zero-values are safe defaults.
type BuildOptions struct {
	// Rebuild, when true, forces a full rebuild from scratch (every layer
	// rebuilt, no cache reuse). Maps to `docker compose build --no-cache`
	// under the hood. Analog of dotnet's `build --no-incremental` and
	// gradle's `--rerun-tasks` — outcome-oriented ("rebuild") rather than
	// mechanism-oriented ("skip cache").
	Rebuild bool
}

func (o SystemOptions) logLines() int {
	if o.LogLines <= 0 {
		return 50
	}
	return o.LogLines
}

func (o SystemOptions) upTimeout() time.Duration {
	if o.UpTimeout <= 0 {
		return defaultUpTimeout
	}
	return o.UpTimeout
}

// transientNetRE matches docker-compose pull/build errors that are usually
// resolved by retrying — DNS hiccups, Docker Hub blips, ECONNRESET, plus
// registry-side flakes (manifest 403/429, buildx metadata-resolve failures).
// Mirrors the bash/PS1 retry pattern.
var transientNetRE = regexp.MustCompile(
	`ECONNRESET|ETIMEDOUT|ECONNREFUSED|EAI_AGAIN|ENOTFOUND|i/o timeout|TLS handshake|` +
		`Bad Gateway|Service Unavailable|Gateway Timeout|` +
		// Registry-side flakes: docker buildx / compose pull failures that surface
		// as transient HTTP errors against image registries (Docker Hub, MCR, GHCR).
		// MCR returns intermittent 403 on public manifests during cache hiccups,
		// not as a real auth signal.
		`unexpected status from HEAD request|failed to resolve source metadata|` +
		`manifest unknown|429 Too Many Requests|403 Forbidden|toomanyrequests`)

// tailWriter is an io.Writer that retains only the last cap bytes written to
// it. Used to capture a bounded tail of streamed subprocess output for
// error-pattern matching, without buffering the full stream.
type tailWriter struct {
	mu  sync.Mutex
	cap int
	buf []byte
}

func (t *tailWriter) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.cap {
		t.buf = t.buf[len(t.buf)-t.cap:]
	}
	return len(p), nil
}

func (t *tailWriter) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

// Build runs `docker compose -f <composeFile> build` for every entry in sys.
// cwd is the working directory the compose-file paths are resolved against
// (typically the system-test directory holding systems.json). When
// opts.Rebuild is true, `--no-cache` is appended so every layer is rebuilt.
func Build(sys *SystemConfig, cwd string, opts BuildOptions) error {
	for _, s := range sys.Systems {
		fmt.Fprintf(os.Stdout, "\n=== Build %s (%s) ===\n", s.Label, s.ComposeFile)
		args := []string{"-f", s.ComposeFile, "build"}
		if opts.Rebuild {
			args = append(args, "--no-cache")
		}
		if err := runCompose(cwd, args...); err != nil {
			return fmt.Errorf("build %s: %w", s.Label, err)
		}
	}
	return nil
}

// Up brings up every system in sys. For each entry, behavior depends on opts:
//
//   - opts.Restart=true     → unconditional `down` then `up -d`, then health-wait.
//   - opts.Restart=false    → if IsAnyURLUp returns true, skip the entry; else
//     `down` + `up -d` + health-wait. (Mirrors the PS1 runner's behavior so
//     local re-runs are fast when the stack is already healthy.)
//
// `up -d` is wrapped in a small retry loop for transient network errors.
func Up(sys *SystemConfig, cwd string, opts SystemOptions) error {
	for _, s := range sys.Systems {
		fmt.Fprintf(os.Stdout, "\n=== System: %s ===\n", s.Label)
		if !opts.Restart && IsAnyURLUp(s, opts.Health) {
			fmt.Fprintln(os.Stdout, "System is already running, skipping restart")
			continue
		}
		if err := upOne(s, cwd, opts); err != nil {
			return err
		}
	}
	return nil
}

func upOne(s SystemEntry, cwd string, opts SystemOptions) error {
	if err := downOne(s, cwd); err != nil {
		// Down errors during a restart are not fatal — the system may already
		// be down. Log and continue to the up step.
		fmt.Fprintf(os.Stderr, "warn: down %s: %v\n", s.Label, err)
	}

	const maxAttempts = 3
	const retryDelay = 10 * time.Second
	timeout := opts.upTimeout()
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := runComposeCtx(cwd, timeout, "-f", s.ComposeFile, "up", "-d")
		if err == nil {
			lastErr = nil
			break
		}
		lastErr = err
		retriable := errors.Is(err, context.DeadlineExceeded) || transientNetRE.MatchString(err.Error())
		if attempt < maxAttempts && retriable {
			reason := "transient network error"
			if errors.Is(err, context.DeadlineExceeded) {
				reason = fmt.Sprintf("compose up exceeded %s", timeout)
			}
			fmt.Fprintf(os.Stderr, "warn: %s on attempt %d/%d, retrying in %s...\n",
				reason, attempt, maxAttempts, retryDelay)
			time.Sleep(retryDelay)
			continue
		}
		break
	}
	if lastErr != nil {
		return fmt.Errorf("up %s: %w", s.Label, lastErr)
	}

	if err := WaitForSystem(s, opts.Health); err != nil {
		// Dump compose logs for the failed container so the user sees what
		// happened, then return the error.
		_ = runCompose(cwd, "-f", s.ComposeFile, "logs", "--tail", fmt.Sprintf("%d", opts.logLines()))
		return fmt.Errorf("system %s health check: %w", s.Label, err)
	}

	for _, c := range s.Components {
		fmt.Fprintf(os.Stdout, "  %s: %s\n", c.Name, c.URL)
	}
	for _, e := range s.ExternalSystems {
		fmt.Fprintf(os.Stdout, "  %s: %s\n", e.Name, e.URL)
	}
	return nil
}

// Down brings down every system in sys (compose down + container cleanup).
// Errors per-system are reported but do not short-circuit the loop, so a
// partial cleanup still proceeds.
func Down(sys *SystemConfig, cwd string) error {
	var firstErr error
	for _, s := range sys.Systems {
		fmt.Fprintf(os.Stdout, "\n=== Stop %s ===\n", s.Label)
		if err := downOne(s, cwd); err != nil {
			fmt.Fprintf(os.Stderr, "warn: down %s: %v\n", s.Label, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// Clean tears down every system and removes its named volumes plus locally-
// built images (`docker compose down -v --rmi local`). External images pulled
// from registries are left alone — same scope as `./gradlew clean`, which
// deletes build outputs but not the dependency cache.
//
// Like Down, errors are logged per-system but do not short-circuit the loop.
func Clean(sys *SystemConfig, cwd string) error {
	var firstErr error
	for _, s := range sys.Systems {
		fmt.Fprintf(os.Stdout, "\n=== Clean %s ===\n", s.Label)
		if err := runCompose(cwd, "-f", s.ComposeFile, "down", "-v", "--rmi", "local"); err != nil {
			fmt.Fprintf(os.Stderr, "warn: clean %s: %v\n", s.Label, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func downOne(s SystemEntry, cwd string) error {
	// `down` ignores its own exit code in PS1 (`2>$null`); we surface it here
	// because the caller (Up) decides what to do with it.
	if err := runCompose(cwd, "-f", s.ComposeFile, "down"); err != nil {
		return err
	}
	if s.ContainerName == "" {
		return nil
	}
	// Force-remove any stray containers matching the project name. Filter
	// "name=" is a substring match in docker, so this catches sibling/orphan
	// containers from previous runs.
	out, err := dockerCapture(cwd, "ps", "-aq", "--filter", "name="+s.ContainerName)
	if err != nil {
		return nil // probe-only; don't fail Down on a missing daemon
	}
	ids := strings.Fields(out)
	if len(ids) == 0 {
		return nil
	}
	args := append([]string{"rm", "-f"}, ids...)
	_ = runDocker(cwd, args...)
	return nil
}

// runCompose executes `docker compose <args...>` from cwd. stdout+stderr are
// streamed to os.Stdout/os.Stderr so the user sees live progress; the last
// 16 KB are also mirrored into the returned error message so a failure's
// FATAL line is self-contained — the live stream may have scrolled off or
// been redirected to a log file the user does not look at.
func runCompose(cwd string, args ...string) error {
	full := append([]string{"compose"}, args...)
	cmd := exec.Command("docker", full...)
	cmd.Dir = cwd
	tail := &tailWriter{cap: 16 * 1024}
	cmd.Stdout = io.MultiWriter(os.Stdout, tail)
	cmd.Stderr = io.MultiWriter(os.Stderr, tail)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose %s: %w\nstderr tail:\n%s",
			strings.Join(args, " "), err, tail.String())
	}
	return nil
}

// runComposeCtx is runCompose with a hard deadline. If the deadline elapses,
// the process is killed and the returned error wraps context.DeadlineExceeded
// so the caller's retry loop can recognise it as transient.
//
// Compose streams build/pull errors (registry 403, ECONNRESET, etc.) to its
// stdio, not to its exit code. To make those visible to the caller's retry
// regex, the last 16KB of stdio are mirrored into the returned error message.
func runComposeCtx(cwd string, timeout time.Duration, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	full := append([]string{"compose"}, args...)
	cmd := exec.CommandContext(ctx, "docker", full...)
	cmd.Dir = cwd
	tail := &tailWriter{cap: 16 * 1024}
	cmd.Stdout = io.MultiWriter(os.Stdout, tail)
	cmd.Stderr = io.MultiWriter(os.Stderr, tail)
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("docker compose %s: %w", strings.Join(args, " "), context.DeadlineExceeded)
	}
	if err != nil {
		return fmt.Errorf("docker compose %s: %w\nstderr tail:\n%s",
			strings.Join(args, " "), err, tail.String())
	}
	return nil
}

// runDocker executes `docker <args...>` with output streamed to the user
// and the last 16 KB mirrored into the returned error.
func runDocker(cwd string, args ...string) error {
	cmd := exec.Command("docker", args...)
	cmd.Dir = cwd
	tail := &tailWriter{cap: 16 * 1024}
	cmd.Stdout = io.MultiWriter(os.Stdout, tail)
	cmd.Stderr = io.MultiWriter(os.Stderr, tail)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker %s: %w\nstderr tail:\n%s",
			strings.Join(args, " "), err, tail.String())
	}
	return nil
}

// dockerCapture runs `docker <args...>` and returns its stdout. Used for
// machine-readable docker queries (e.g. `docker ps -aq`).
func dockerCapture(cwd string, args ...string) (string, error) {
	cmd := exec.Command("docker", args...)
	cmd.Dir = cwd
	out, err := cmd.Output()
	return string(out), err
}
