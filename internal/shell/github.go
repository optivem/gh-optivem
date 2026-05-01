// Package shell provides GitHub CLI wrapper and subprocess helpers.
package shell

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/log"
	"github.com/optivem/gh-optivem/internal/pathx"
	"github.com/optivem/gh-optivem/internal/spinner"
)

const (
	rateLimitThreshold = 50
	pollMaxDuration    = 60 * time.Minute

	errMsgInvalidCommand = "invalid command %q: %w"
	errMsgEmptyCommand   = "empty command"
)

// RateLimitExceeded is returned when a gh command fails due to rate limiting.
type RateLimitExceeded struct {
	Msg string
}

func (e *RateLimitExceeded) Error() string { return e.Msg }

// Run executes a shell command. In dry-run mode, just prints it.
// When check=false, non-rate-limit failures are logged (not swallowed silently)
// but still return a nil error so callers can continue. Rate-limit errors are
// always returned regardless of check so callers can back off.
func Run(cmdStr string, dryRun, check bool, cwd string) (string, error) {
	if dryRun {
		log.Infof("[DRY RUN] %s", cmdStr)
		return "", nil
	}

	parts, err := splitCommand(cmdStr)
	if err != nil {
		return "", fmt.Errorf(errMsgInvalidCommand, cmdStr, err)
	}
	if len(parts) == 0 {
		return "", errors.New(errMsgEmptyCommand)
	}
	cmd := exec.Command(pathx.NormalizeExe(parts[0]), parts[1:]...)
	if cwd != "" {
		cmd.Dir = cwd
	}

	out, err := cmd.CombinedOutput()
	output := string(out)

	if err != nil {
		lower := strings.ToLower(output)
		if strings.Contains(lower, "rate limit") || strings.Contains(lower, "api rate limit exceeded") {
			return output, &RateLimitExceeded{Msg: fmt.Sprintf("GitHub API rate limit exceeded. Command: %s\n%s", cmdStr, output)}
		}
		if check {
			return output, fmt.Errorf("command failed: %s: %w\n%s", cmdStr, err, output)
		}
		log.Warnf("command failed (check=false, continuing): %s: %v\n%s", cmdStr, err, output)
	}
	return output, nil
}

// RunStdin is like Run but pipes stdin to the process. Use when an argument
// would contain a secret (token, password) that must not appear in the
// command line — passing it via stdin keeps it out of error messages, retry
// logging, and process lists (argv is readable by other local users on most
// systems, and any error surfaces the full cmdStr). cmdStr is the command
// as it should appear in logs; it never includes the stdin value.
func RunStdin(cmdStr, stdin string, dryRun, check bool, cwd string) (string, error) {
	if dryRun {
		log.Infof("[DRY RUN] %s (stdin: ***)", cmdStr)
		return "", nil
	}

	parts, err := splitCommand(cmdStr)
	if err != nil {
		return "", fmt.Errorf(errMsgInvalidCommand, cmdStr, err)
	}
	if len(parts) == 0 {
		return "", errors.New(errMsgEmptyCommand)
	}
	cmd := exec.Command(pathx.NormalizeExe(parts[0]), parts[1:]...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Stdin = strings.NewReader(stdin)

	out, err := cmd.CombinedOutput()
	output := string(out)

	if err != nil {
		lower := strings.ToLower(output)
		if strings.Contains(lower, "rate limit") || strings.Contains(lower, "api rate limit exceeded") {
			return output, &RateLimitExceeded{Msg: fmt.Sprintf("GitHub API rate limit exceeded. Command: %s\n%s", cmdStr, output)}
		}
		if check {
			return output, fmt.Errorf("command failed: %s: %w\n%s", cmdStr, err, output)
		}
		log.Warnf("command failed (check=false, continuing): %s: %v\n%s", cmdStr, err, output)
	}
	return output, nil
}

// MustRun executes an external command and aborts the program on failure.
// Use for external system calls (gh, git, etc.) where partial failure must
// not be silently swallowed. Honors dry-run semantics.
func MustRun(cmdStr string, dryRun bool, cwd string) string {
	out, err := Run(cmdStr, dryRun, true, cwd)
	if err != nil {
		log.Fatalf("%v", err)
	}
	return out
}

// RunCapture runs a command and captures stdout separately. On failure, stderr
// is captured and included in the returned error so the caller has context.
func RunCapture(cmdStr, cwd string) (string, error) {
	parts, err := splitCommand(cmdStr)
	if err != nil {
		return "", fmt.Errorf(errMsgInvalidCommand, cmdStr, err)
	}
	if len(parts) == 0 {
		return "", errors.New(errMsgEmptyCommand)
	}
	cmd := exec.Command(pathx.NormalizeExe(parts[0]), parts[1:]...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	stdoutStr := strings.TrimSpace(string(out))
	if err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return stdoutStr, fmt.Errorf("%w: %s", err, stderrStr)
		}
		return stdoutStr, err
	}
	return stdoutStr, nil
}

// RunPassthrough runs a command with stdout/stderr passed through to the terminal.
func RunPassthrough(cmdStr, cwd string) error {
	parts, err := splitCommand(cmdStr)
	if err != nil {
		return fmt.Errorf(errMsgInvalidCommand, cmdStr, err)
	}
	if len(parts) == 0 {
		return errors.New(errMsgEmptyCommand)
	}
	cmd := exec.Command(pathx.NormalizeExe(parts[0]), parts[1:]...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// splitCommand splits a command string into parts, respecting quotes.
// Inside double quotes, \" and \\ are honored as escape sequences so that
// callers using fmt.Sprintf("%q", s) — which emits Go-style escaping — do
// not have an embedded \" prematurely terminate the quoted run. Inside
// single quotes, content is fully literal (POSIX semantics).
// Returns an error if the input has an unterminated quote.
func splitCommand(s string) ([]string, error) {
	var parts []string
	var current strings.Builder
	inQuote := false
	quoteChar := byte(0)

	flush := func() {
		if current.Len() > 0 {
			parts = append(parts, current.String())
			current.Reset()
		}
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		if consumed := tryEscape(s, i, inQuote, quoteChar, &current); consumed {
			i++
			continue
		}
		switch {
		case inQuote && c == quoteChar:
			inQuote = false
		case inQuote:
			current.WriteByte(c)
		case c == '"' || c == '\'':
			inQuote = true
			quoteChar = c
		case c == ' ' || c == '\t':
			flush()
		default:
			current.WriteByte(c)
		}
	}
	if inQuote {
		return nil, fmt.Errorf("unterminated %c quote", quoteChar)
	}
	flush()
	return parts, nil
}

// tryEscape handles \" and \\ inside a double-quoted run. Returns true and
// writes the unescaped byte when an escape was consumed; the caller must then
// advance past the trailing byte. Returns false in all other cases.
func tryEscape(s string, i int, inQuote bool, quoteChar byte, out *strings.Builder) bool {
	if !inQuote || quoteChar != '"' || s[i] != '\\' || i+1 >= len(s) {
		return false
	}
	next := s[i+1]
	if next != '"' && next != '\\' {
		return false
	}
	out.WriteByte(next)
	return true
}

// CheckRateLimit checks the GitHub API rate limit and waits if low.
func CheckRateLimit() {
	out, err := RunCapture("gh api rate_limit --jq .resources.core", "")
	if err != nil {
		log.Warnf("rate limit check failed (continuing without wait): %v", err)
		return
	}

	var data struct {
		Remaining int   `json:"remaining"`
		Reset     int64 `json:"reset"`
	}
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		log.Warnf("rate limit response unparseable (continuing without wait): %v; raw=%q", err, out)
		return
	}

	if data.Remaining < rateLimitThreshold {
		waitSecs := data.Reset - time.Now().Unix() + 5
		if waitSecs > 0 {
			waitForRateLimitReset(data.Remaining, waitSecs)
		} else {
			log.Infof("Rate limit low (%d remaining) but reset is imminent.", data.Remaining)
		}
	}
}

// waitForRateLimitReset sleeps for waitSecs while showing a live spinner so
// the user can see the process is alive (waits can be tens of minutes). The
// status text counts down remaining seconds, updated each second.
func waitForRateLimitReset(remaining int, waitSecs int64) {
	msg := fmt.Sprintf("Rate limit low (%d remaining) — waiting for reset", remaining)
	s := spinner.Start(msg)
	defer s.Stop()

	deadline := time.Now().Add(time.Duration(waitSecs) * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		left := time.Until(deadline).Round(time.Second)
		if left <= 0 {
			return
		}
		s.Update(fmt.Sprintf("%s remaining", left))
		<-ticker.C
	}
}

// GitHub wraps gh CLI calls for a specific repo.
type GitHub struct {
	Repo   string
	DryRun bool
}

func NewGitHub(cfg *config.Config) *GitHub {
	return &GitHub{Repo: cfg.FullRepo, DryRun: cfg.DryRun}
}

func (g *GitHub) ForRepo(fullRepo string) *GitHub {
	return &GitHub{Repo: fullRepo, DryRun: g.DryRun}
}

func (g *GitHub) run(cmd string) (string, error) {
	return RunWithRetry(fmt.Sprintf("gh %s --repo %s", cmd, g.Repo), g.DryRun, true, "")
}

// mustRun is the GitHub-struct companion to MustRun. Auto-prepends `gh` and
// `--repo g.Repo`. Aborts the program on failure.
func (g *GitHub) mustRun(cmd string) string {
	out, err := g.run(cmd)
	if err != nil {
		log.Fatalf("%v", err)
	}
	return out
}

// isRepoNotFound reports whether a gh repo view failure means the repo doesn't
// exist (vs a transient failure like network / auth / rate limit).
func isRepoNotFound(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "could not resolve to a repository") ||
		strings.Contains(lower, "not found") ||
		strings.Contains(lower, "404")
}

func (g *GitHub) CreateRepo() {
	if g.DryRun {
		log.Infof("[DRY RUN] Would check and create repo %s if missing", g.Repo)
		return
	}
	out, err := Run(fmt.Sprintf("gh repo view %s --json name", g.Repo), false, true, "")
	if err == nil {
		log.Warnf("Repository %s already exists -- skipping creation", g.Repo)
		return
	}
	var rle *RateLimitExceeded
	if errors.As(err, &rle) {
		log.Fatalf("rate limit hit while checking if %s exists: %v", g.Repo, err)
	}
	if !isRepoNotFound(out) {
		log.Fatalf("failed to check if repository %s exists: %v\n%s", g.Repo, err, out)
	}
	MustRunWithRetry("gh repo create "+g.Repo+" --public", false, "")
	g.waitForRepoVisible()
}

// waitForRepoVisible polls gh repo view until the repo resolves via GraphQL.
// Closes a race between gh repo create (REST, returns on primary write) and
// gh repo clone's GraphQL resolve step — the GraphQL index can lag behind the
// primary by a few seconds, causing clone to fail with "Could not resolve to
// a Repository" for a repo that was just created successfully.
func (g *GitHub) waitForRepoVisible() {
	if g.DryRun {
		return
	}
	const maxAttempts = 15
	const pollDelay = 1 * time.Second
	viewCmd := fmt.Sprintf("gh repo view %s --json name", g.Repo)

	sp := spinner.Start(fmt.Sprintf("Waiting for %s to become visible via GraphQL", g.Repo))
	defer sp.Stop()

	var out string
	var err error
	for i := 1; i <= maxAttempts; i++ {
		out, err = Run(viewCmd, false, true, "")
		if err == nil {
			return
		}
		var rle *RateLimitExceeded
		if errors.As(err, &rle) {
			log.Fatalf("rate limit hit while waiting for %s to become visible: %v", g.Repo, err)
		}
		sp.Update(fmt.Sprintf("attempt %d/%d", i, maxAttempts))
		if i < maxAttempts {
			sleepFn(pollDelay)
		}
	}
	log.Fatalf("repository %s did not become visible within %ds after creation: %v\n%s",
		g.Repo, maxAttempts, err, out)
}

func (g *GitHub) CreateEnvironment(name string) {
	MustRunWithRetry(fmt.Sprintf("gh api repos/%s/environments/%s -X PUT", g.Repo, name), g.DryRun, "")
}

func (g *GitHub) SecretSet(name, value string) {
	if g.DryRun {
		log.Infof("[DRY RUN] gh secret set %s --repo %s (stdin: ***)", name, g.Repo)
		return
	}
	// Pass the secret via stdin (gh reads stdin when --body is omitted) so it
	// never appears in the command line — argv is readable by other local users
	// on most systems, and any error from `gh secret set` would surface the full
	// cmdStr into logs, retry chatter, and the auto-filed bug report body.
	// Note: `--body -` does NOT mean stdin — gh would store the literal "-".
	MustRunStdinWithRetry(
		fmt.Sprintf("gh secret set %s --repo %s", name, g.Repo),
		value, false, "")
}

func (g *GitHub) VariableSet(name, value string) {
	if g.DryRun {
		log.Infof("[DRY RUN] gh variable set %s --body \"%s\" --repo %s", name, value, g.Repo)
		return
	}
	MustRunWithRetry(fmt.Sprintf("gh variable set %s --body %s --repo %s", name, value, g.Repo), false, "")
}

func (g *GitHub) Clone(dest string) {
	MustRunWithRetry(fmt.Sprintf("gh repo clone %s %s", g.Repo, dest), false, "")
	if _, err := os.Stat(filepath.Join(dest, ".git")); err != nil {
		log.Fatalf("clone of %s to %s produced no .git directory: %v", g.Repo, dest, err)
	}
}

func (g *GitHub) WorkflowRun(workflow string, fields map[string]string) {
	// Sort keys so the constructed command is deterministic — helps logs,
	// tests, and retry-idempotency reasoning.
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var fieldArgs string
	for _, k := range keys {
		fieldArgs += fmt.Sprintf(" -f %s=%s", k, fields[k])
	}
	g.mustRun(fmt.Sprintf("workflow run %s%s", workflow, fieldArgs))
}

// RunWatchWorkflow watches the latest run for a specific workflow name.
// If workflow is empty, watches the overall latest run.
// Retries up to 12 times (60s total) waiting for the run to appear.
// intervalSecs controls the polling frequency for gh run watch.
// If gh run watch hits a rate limit mid-stream, falls back to manual polling.
func (g *GitHub) RunWatchWorkflow(workflow string, intervalSecs int) error {
	var listCmd string
	if workflow != "" {
		listCmd = fmt.Sprintf("gh run list --repo %s --workflow %s --limit 1 --json databaseId --jq .[0].databaseId", g.Repo, workflow)
	} else {
		listCmd = fmt.Sprintf("gh run list --repo %s --limit 1 --json databaseId --jq .[0].databaseId", g.Repo)
	}

	const appearPollSecs = 10
	const appearAttempts = 6
	sp := spinner.Start(fmt.Sprintf("Waiting for workflow run to appear (%s, polling every %ds)", workflow, appearPollSecs))
	var out string
	var err error
	for attempt := 1; attempt <= appearAttempts; attempt++ {
		out, err = RunCapture(listCmd, "")
		if err == nil && strings.TrimSpace(out) != "" {
			break
		}
		sp.Update(fmt.Sprintf("attempt %d/%d", attempt, appearAttempts))
		if attempt < appearAttempts {
			time.Sleep(appearPollSecs * time.Second)
		}
	}
	sp.Stop()
	if err != nil || strings.TrimSpace(out) == "" {
		return fmt.Errorf("no workflow runs found for %s (workflow: %s) after %d attempts", g.Repo, workflow, appearAttempts)
	}

	runID := strings.TrimSpace(out)
	log.Successf("Watching workflow run (polling every %ds): https://github.com/%s/actions/runs/%s", intervalSecs, g.Repo, runID)
	_, err = Run(fmt.Sprintf("gh run watch %s --repo %s --exit-status --interval %d", runID, g.Repo, intervalSecs), false, true, "")
	if err == nil {
		return nil
	}

	// If gh run watch failed due to rate limiting, fall back to polling run status.
	var rle *RateLimitExceeded
	if !errors.As(err, &rle) {
		return err
	}

	log.Infof("Rate limit hit while watching run %s — switching to polling mode...", runID)
	return g.pollRunUntilComplete(runID)
}

// pollRunUntilComplete polls gh run view until the run is no longer in_progress/queued.
// Waits for rate limit reset between polls. Returns nil if the run succeeded, error otherwise.
// Bounded by pollMaxDuration so a stuck run / permanently malformed output cannot loop forever.
// Uses Run (combined output) so rate-limit classification in Run triggers the wait-and-retry path.
func (g *GitHub) pollRunUntilComplete(runID string) error {
	deadline := time.Now().Add(pollMaxDuration)
	viewCmd := fmt.Sprintf("gh run view %s --repo %s --json status,conclusion --jq '[.status,.conclusion] | join(\",\")'", runID, g.Repo)

	sp := spinner.Start(fmt.Sprintf("Polling workflow run %s (every 60s)", runID))
	defer sp.Stop()

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("polling run %s timed out after %s", runID, pollMaxDuration)
		}

		CheckRateLimit()

		statusOut, err := Run(viewCmd, false, true, "")
		if err != nil {
			var rle *RateLimitExceeded
			if errors.As(err, &rle) {
				sp.Update("rate limit hit, retrying in 60s")
				time.Sleep(60 * time.Second)
				continue
			}
			return fmt.Errorf("failed to poll run %s: %w", runID, err)
		}

		parts := strings.SplitN(strings.TrimSpace(statusOut), ",", 2)
		if len(parts) < 2 {
			log.Warnf("Unexpected run view output for %s: %q — retrying in 30s", runID, statusOut)
			time.Sleep(30 * time.Second)
			continue
		}
		status, conclusion := parts[0], parts[1]

		if status == "completed" {
			switch conclusion {
			case "success":
				return nil
			case "":
				return fmt.Errorf("run %s completed with empty conclusion field", runID)
			default:
				return fmt.Errorf("run %s completed with conclusion: %s", runID, conclusion)
			}
		}

		sp.Update(fmt.Sprintf("status: %s", status))
		time.Sleep(60 * time.Second)
	}
}

// Delete is best-effort cleanup; teardown happens after the main work has
// either succeeded or already failed, so we log failures but don't abort.
func (g *GitHub) Delete() {
	out, err := Run(fmt.Sprintf("gh repo delete %s --yes", g.Repo), g.DryRun, true, "")
	if err != nil {
		log.Warnf("Delete of %s failed (best-effort, continuing): %v\n%s", g.Repo, err, out)
	}
}
