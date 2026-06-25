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
	// Restart, when true, recreates the changed services in place so a freshly
	// built image (and changed stubs/migrations) is picked up, while leaving the
	// persistent services (postgres + its data volume) running — an incremental
	// recreate rather than a full down/up. When false, Up first checks
	// IsAnyURLUp; if the system already responds, Up is a no-op.
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
//   - opts.Restart=true     → incremental recreate: rebuild and force-recreate
//     every non-persistent service (`up -d --build --force-recreate --no-deps`)
//     while leaving postgres (and its data volume) running, then health-wait. On
//     a cold stack (postgres not up yet) it falls back to a full `up -d --build`
//     so postgres + deps start first. No `down`, so the loop stays fast.
//   - opts.Restart=false    → if IsAnyURLUp returns true, skip the entry; else
//     `down` + `up -d` + health-wait. (Mirrors the PS1 runner's behavior so
//     local re-runs are fast when the stack is already healthy.)
//
// The `up` invocation is wrapped in a small retry loop for transient network
// errors.
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

// persistentServices are the compose services that a --restart must never
// recreate: they own durable state (postgres + its data volume), and recreating
// them would re-pay the healthcheck wait and risk the data. Hard-coded for v1 —
// matches every current shop stack. Upgrade path if a stack ever adds a
// differently-named stateful service: promote to an optional persistentServices
// field on SystemEntry, defaulting to {"postgres"}.
var persistentServices = map[string]bool{"postgres": true}

func upOne(s SystemEntry, cwd string, opts SystemOptions) error {
	upArgs, err := upComposeArgs(s, cwd, opts)
	if err != nil {
		return fmt.Errorf("up %s: %w", s.Label, err)
	}

	const maxAttempts = 3
	const retryDelay = 10 * time.Second
	timeout := opts.upTimeout()
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := runComposeCtx(cwd, timeout, upArgs...)
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

	printEndpointsForEntry(os.Stdout, s)
	return nil
}

// upComposeArgs decides how upOne brings one stack up and performs any required
// pre-up cleanup, returning the `docker compose` argument list for the retry
// loop.
//
//   - Non-restart (opts.Restart == false): only reached when the stack is not
//     already healthy. Tear it down first (clearing stray containers), then a
//     plain `up -d`. Unchanged legacy behaviour.
//   - Restart: query the stack's services and which are running, then delegate
//     to buildUpArgs for the incremental-vs-cold-start decision. No `down`.
func upComposeArgs(s SystemEntry, cwd string, opts SystemOptions) ([]string, error) {
	if !opts.Restart {
		if err := downOne(s, cwd); err != nil {
			// Down errors are not fatal — the system may already be down.
			fmt.Fprintf(os.Stderr, "warn: down %s: %v\n", s.Label, err)
		}
		return []string{"-f", s.ComposeFile, "up", "-d"}, nil
	}
	services, err := composeServices(s.ComposeFile, cwd)
	if err != nil {
		return nil, err
	}
	running, err := composeRunningServices(s.ComposeFile, cwd)
	if err != nil {
		return nil, err
	}
	return buildUpArgs(s.ComposeFile, services, running), nil
}

// buildUpArgs is the pure arg-selection core of the --restart path: given the
// stack's full service list and the set of currently-running services, it
// returns the `docker compose up` arguments. Split out from upComposeArgs so the
// incremental / cold-start / degenerate decisions are unit-testable without
// docker. Service order from `services` is preserved.
func buildUpArgs(composeFile string, services []string, running map[string]bool) []string {
	const buildFlag = "--build"
	base := []string{"-f", composeFile, "up", "-d"}
	var recreate, persistentInStack []string
	for _, svc := range services {
		if persistentServices[svc] {
			persistentInStack = append(persistentInStack, svc)
		} else {
			recreate = append(recreate, svc)
		}
	}
	// No persistent service to protect, or nothing else to recreate → just bring
	// the whole stack up with a fresh build.
	if len(persistentInStack) == 0 || len(recreate) == 0 {
		return append(base, buildFlag)
	}
	// Cold start: a persistent service isn't running yet, so bring the whole
	// stack up so postgres + its dependents start in dependency order. The next
	// --restart then takes the incremental path below.
	for _, p := range persistentInStack {
		if !running[p] {
			return append(base, buildFlag)
		}
	}
	// Incremental: rebuild and force-recreate only the non-persistent services.
	// --no-deps keeps postgres (already running) out of the recreate, so its
	// healthcheck is not re-waited and its data volume is preserved. --build
	// picks up new app/simulator code; --force-recreate re-runs Flyway (new
	// migrations) and reloads WireMock's mounted mappings (changed stubs).
	args := append(base, buildFlag, "--force-recreate", "--no-deps")
	return append(args, recreate...)
}

// composeServices returns every service defined in the compose file, via
// `docker compose config --services`.
func composeServices(composeFile, cwd string) ([]string, error) {
	out, err := dockerCaptureChecked(cwd, "compose", "-f", composeFile, "config", "--services")
	if err != nil {
		return nil, fmt.Errorf("enumerate services for %s: %w", composeFile, err)
	}
	return strings.Fields(out), nil
}

// composeRunningServices returns the set of services with at least one running
// container, via `docker compose ps --services --status running`. An empty set
// (project never created, or all stopped) is a normal result, not an error.
func composeRunningServices(composeFile, cwd string) (map[string]bool, error) {
	out, err := dockerCaptureChecked(cwd, "compose", "-f", composeFile, "ps", "--services", "--status", "running")
	if err != nil {
		return nil, fmt.Errorf("list running services for %s: %w", composeFile, err)
	}
	set := make(map[string]bool)
	for _, name := range strings.Fields(out) {
		set[name] = true
	}
	return set, nil
}

// PrintEndpoints writes the component and external-system endpoint URLs for
// every system in sys to w, one per line, two-space indented. Empty URLs are
// skipped (mirrors WaitForSystem's component-with-no-URL skip).
func PrintEndpoints(w io.Writer, sys *SystemConfig) {
	for _, s := range sys.Systems {
		printEndpointsForEntry(w, s)
	}
}

// printEndpointsForEntry writes one system's endpoint URLs to w. Empty URLs
// are skipped. Shared by upOne (single entry) and PrintEndpoints (all entries)
// so the format stays in lockstep.
func printEndpointsForEntry(w io.Writer, s SystemEntry) {
	for _, c := range s.Components {
		if c.URL == "" {
			continue
		}
		fmt.Fprintf(w, "  %s: %s\n", c.Name, c.URL)
	}
	for _, e := range s.ExternalSystems {
		if e.URL == "" {
			continue
		}
		fmt.Fprintf(w, "  %s: %s\n", e.Name, e.URL)
	}
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

// dockerCaptureChecked is dockerCapture for queries whose failure must surface:
// it captures stdout and, on a non-zero exit, folds the child's stderr into the
// returned error so the cause (bad compose file, daemon down) is self-contained
// rather than a bare "exit status N".
func dockerCaptureChecked(cwd string, args ...string) (string, error) {
	cmd := exec.Command("docker", args...)
	cmd.Dir = cwd
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("docker %s: %w\nstderr:\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return string(out), nil
}
