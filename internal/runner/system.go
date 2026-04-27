package runner

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

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
}

// BuildOptions tunes behavior of Build. Zero-values are safe defaults.
type BuildOptions struct {
	// NoCache, when true, passes --no-cache to `docker compose build` so
	// every layer is rebuilt from scratch. Analog of dotnet's
	// `build --no-incremental` and gradle's `--rerun-tasks`.
	NoCache bool
}

func (o SystemOptions) logLines() int {
	if o.LogLines <= 0 {
		return 50
	}
	return o.LogLines
}

// transientNetRE matches docker-compose pull/build errors that are usually
// resolved by retrying — DNS hiccups, Docker Hub blips, ECONNRESET. Mirrors
// the bash/PS1 retry pattern.
var transientNetRE = regexp.MustCompile(
	`ECONNRESET|ETIMEDOUT|ECONNREFUSED|EAI_AGAIN|ENOTFOUND|i/o timeout|TLS handshake|Bad Gateway|Service Unavailable|Gateway Timeout`)

// Build runs `docker compose -f <composeFile> build` for every entry in sys.
// cwd is the working directory the compose-file paths are resolved against
// (typically the system-test directory holding system.json). When
// opts.NoCache is true, `--no-cache` is appended so every layer is rebuilt.
func Build(sys *SystemConfig, cwd string, opts BuildOptions) error {
	for _, s := range sys.Systems {
		fmt.Fprintf(os.Stdout, "\n=== Build %s (%s) ===\n", s.Label, s.ComposeFile)
		args := []string{"-f", s.ComposeFile, "build"}
		if opts.NoCache {
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
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := runCompose(cwd, "-f", s.ComposeFile, "up", "-d")
		if err == nil {
			lastErr = nil
			break
		}
		lastErr = err
		if attempt < maxAttempts && transientNetRE.MatchString(err.Error()) {
			fmt.Fprintf(os.Stderr, "warn: transient network error on attempt %d/%d, retrying in %s...\n",
				attempt, maxAttempts, retryDelay)
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
// streamed to os.Stdout/os.Stderr so the user sees live progress.
func runCompose(cwd string, args ...string) error {
	full := append([]string{"compose"}, args...)
	cmd := exec.Command("docker", full...)
	cmd.Dir = cwd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runDocker executes `docker <args...>` with output streamed to the user.
func runDocker(cwd string, args ...string) error {
	cmd := exec.Command("docker", args...)
	cmd.Dir = cwd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// dockerCapture runs `docker <args...>` and returns its stdout. Used for
// machine-readable docker queries (e.g. `docker ps -aq`).
func dockerCapture(cwd string, args ...string) (string, error) {
	cmd := exec.Command("docker", args...)
	cmd.Dir = cwd
	out, err := cmd.Output()
	return string(out), err
}
