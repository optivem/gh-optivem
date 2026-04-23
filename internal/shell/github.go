// Package shell provides GitHub CLI wrapper and subprocess helpers.
package shell

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/log"
)

const (
	rateLimitThreshold = 50
	pollMaxDuration    = 60 * time.Minute
)

// RateLimitExceeded is returned when a gh command fails due to rate limiting.
type RateLimitExceeded struct {
	Msg string
}

func (e *RateLimitExceeded) Error() string { return e.Msg }

// Run executes a shell command. In dry-run mode, just prints it.
func Run(cmdStr string, dryRun bool, check bool, cwd string) (string, error) {
	if dryRun {
		log.Logf("[DRY RUN] %s", cmdStr)
		return "", nil
	}

	parts := splitCommand(cmdStr)
	cmd := exec.Command(parts[0], parts[1:]...)
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

// RunCapture runs a command and captures stdout separately.
func RunCapture(cmdStr string, cwd string) (string, error) {
	parts := splitCommand(cmdStr)
	cmd := exec.Command(parts[0], parts[1:]...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// RunPassthrough runs a command with stdout/stderr passed through to the terminal.
func RunPassthrough(cmdStr string, cwd string) error {
	parts := splitCommand(cmdStr)
	cmd := exec.Command(parts[0], parts[1:]...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Stdout = nil // inherit
	cmd.Stderr = nil // inherit
	return cmd.Run()
}

// splitCommand splits a command string into parts, respecting quotes.
func splitCommand(s string) []string {
	var parts []string
	var current strings.Builder
	inQuote := false
	quoteChar := byte(0)

	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQuote {
			if c == quoteChar {
				inQuote = false
			} else {
				current.WriteByte(c)
			}
		} else if c == '"' || c == '\'' {
			inQuote = true
			quoteChar = c
		} else if c == ' ' || c == '\t' {
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		} else {
			current.WriteByte(c)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
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
			log.Logf("Rate limit low (%d remaining). Waiting %ds for reset...", data.Remaining, waitSecs)
			time.Sleep(time.Duration(waitSecs) * time.Second)
		} else {
			log.Logf("Rate limit low (%d remaining) but reset is imminent.", data.Remaining)
		}
	}
}

// GitHub wraps gh CLI calls for a specific repo.
type GitHub struct {
	Repo    string
	License string
	DryRun  bool
}

func NewGitHub(cfg *config.Config) *GitHub {
	return &GitHub{Repo: cfg.FullRepo, License: cfg.License, DryRun: cfg.DryRun}
}

func (g *GitHub) ForRepo(fullRepo string) *GitHub {
	return &GitHub{Repo: fullRepo, License: g.License, DryRun: g.DryRun}
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
	out, err := Run(fmt.Sprintf("gh repo view %s --json name", g.Repo), g.DryRun, true, "")
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
	g.initRepo()
}

// initRepo pushes the initial commit (README + LICENSE) so the default branch
// exists immediately without waiting for GitHub's async initialization.
func (g *GitHub) initRepo() {
	dir, err := os.MkdirTemp("", "repo-init-*")
	if err != nil {
		log.Fatalf("failed to create temp dir for repo init: %v", err)
	}
	defer os.RemoveAll(dir)

	// Clone the empty repo.
	MustRunWithRetry(fmt.Sprintf("gh repo clone %s %s", g.Repo, dir), false, "")

	// Write README.md.
	repoName := g.Repo
	if idx := strings.Index(repoName, "/"); idx >= 0 {
		repoName = repoName[idx+1:]
	}
	readmePath := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# "+repoName+"\n"), 0644); err != nil {
		log.Fatalf("failed to write README.md at %s: %v", readmePath, err)
	}

	// Write LICENSE if a license is configured.
	if g.License != "" {
		licenseBody, err := RunCapture(fmt.Sprintf("gh api licenses/%s --jq .body", g.License), "")
		if err == nil && licenseBody != "" {
			licensePath := filepath.Join(dir, "LICENSE")
			if werr := os.WriteFile(licensePath, []byte(licenseBody+"\n"), 0644); werr != nil {
				log.Warnf("Could not write LICENSE file at %s: %v -- continuing without LICENSE", licensePath, werr)
			}
		} else {
			log.Warnf("Could not fetch license template %q -- skipping LICENSE file", g.License)
		}
	}

	// Commit and push.
	MustRun("git add -A", false, dir)
	MustRun("git commit -m \"Initial commit\"", false, dir)
	MustRun("git push", false, dir)
}

func (g *GitHub) EnablePages() {
	MustRunWithRetry(fmt.Sprintf("gh api repos/%s/pages -X POST -f source[branch]=main -f source[path]=/docs", g.Repo), g.DryRun, "")
}

func (g *GitHub) CreateEnvironment(name string) {
	MustRunWithRetry(fmt.Sprintf("gh api repos/%s/environments/%s -X PUT", g.Repo, name), g.DryRun, "")
}

func (g *GitHub) SecretSet(name, value string) {
	if g.DryRun {
		log.Logf("[DRY RUN] gh secret set %s --body *** --repo %s", name, g.Repo)
		return
	}
	MustRunWithRetry(fmt.Sprintf("gh secret set %s --body %s --repo %s", name, value, g.Repo), false, "")
}

func (g *GitHub) VariableSet(name, value string) {
	if g.DryRun {
		log.Logf("[DRY RUN] gh variable set %s --body \"%s\" --repo %s", name, value, g.Repo)
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
	var fieldArgs string
	for k, v := range fields {
		fieldArgs += fmt.Sprintf(" -f %s=%s", k, v)
	}
	g.mustRun(fmt.Sprintf("workflow run %s%s", workflow, fieldArgs))
}

func (g *GitHub) RunWatch(intervalSecs int) error {
	return g.RunWatchWorkflow("", intervalSecs)
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

	var out string
	var err error
	for attempt := 1; attempt <= 12; attempt++ {
		out, err = RunCapture(listCmd, "")
		if err == nil && strings.TrimSpace(out) != "" {
			break
		}
		if attempt < 12 {
			log.Logf("Waiting for workflow run to appear (attempt %d/12)...", attempt)
			time.Sleep(5 * time.Second)
		}
	}
	if err != nil || strings.TrimSpace(out) == "" {
		return fmt.Errorf("no workflow runs found for %s (workflow: %s) after 12 attempts", g.Repo, workflow)
	}

	runID := strings.TrimSpace(out)
	_, err = Run(fmt.Sprintf("gh run watch %s --repo %s --exit-status --interval %d", runID, g.Repo, intervalSecs), false, true, "")
	if err == nil {
		return nil
	}

	// If gh run watch failed due to rate limiting, fall back to polling run status.
	var rle *RateLimitExceeded
	if !errors.As(err, &rle) {
		return err
	}

	log.Logf("Rate limit hit while watching run %s — switching to polling mode...", runID)
	return g.pollRunUntilComplete(runID)
}

// pollRunUntilComplete polls gh run view until the run is no longer in_progress/queued.
// Waits for rate limit reset between polls. Returns nil if the run succeeded, error otherwise.
// Bounded by pollMaxDuration so a stuck run / permanently malformed output cannot loop forever.
// Uses Run (combined output) so rate-limit classification in Run triggers the wait-and-retry path.
func (g *GitHub) pollRunUntilComplete(runID string) error {
	deadline := time.Now().Add(pollMaxDuration)
	viewCmd := fmt.Sprintf("gh run view %s --repo %s --json status,conclusion --jq '[.status,.conclusion] | join(\",\")'", runID, g.Repo)

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("polling run %s timed out after %s", runID, pollMaxDuration)
		}

		CheckRateLimit()

		statusOut, err := Run(viewCmd, false, true, "")
		if err != nil {
			var rle *RateLimitExceeded
			if errors.As(err, &rle) {
				log.Logf("Rate limit hit polling run %s — waiting 60s before retry...", runID)
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
			if conclusion == "success" {
				return nil
			}
			return fmt.Errorf("run %s completed with conclusion: %s", runID, conclusion)
		}

		log.Logf("Run %s status: %s — polling again in 60s...", runID, status)
		time.Sleep(60 * time.Second)
	}
}

// Delete is best-effort cleanup (check=false); failures are intentionally ignored
// because teardown happens after the main work has either succeeded or already failed.
func (g *GitHub) Delete() {
	Run(fmt.Sprintf("gh repo delete %s --yes", g.Repo), false, false, "")
}
