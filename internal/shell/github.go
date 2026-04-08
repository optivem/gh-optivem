// Package shell provides GitHub CLI wrapper and subprocess helpers.
package shell

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/log"
)

const rateLimitThreshold = 50

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
			return output, fmt.Errorf("command failed: %s\n%s", cmdStr, output)
		}
	}
	return output, nil
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
		return
	}

	var data struct {
		Remaining int   `json:"remaining"`
		Reset     int64 `json:"reset"`
	}
	if json.Unmarshal([]byte(out), &data) != nil {
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
	return Run(fmt.Sprintf("gh %s --repo %s", cmd, g.Repo), g.DryRun, true, "")
}

func (g *GitHub) CreateRepo() {
	out, err := RunCapture(fmt.Sprintf("gh repo view %s --json name", g.Repo), "")
	if err == nil && out != "" {
		log.Warnf("Repository %s already exists -- skipping creation", g.Repo)
		return
	}
	Run(fmt.Sprintf("gh repo create %s --public --add-readme --license %s", g.Repo, g.License), false, true, "")
}

func (g *GitHub) EnablePages() {
	Run(fmt.Sprintf("gh api repos/%s/pages -X POST -f source[branch]=main -f source[path]=/docs", g.Repo), g.DryRun, true, "")
}

func (g *GitHub) CreateEnvironment(name string) {
	Run(fmt.Sprintf("gh api repos/%s/environments/%s -X PUT", g.Repo, name), g.DryRun, true, "")
}

func (g *GitHub) SecretSet(name, value string) {
	if g.DryRun {
		log.Logf("[DRY RUN] gh secret set %s --body *** --repo %s", name, g.Repo)
		return
	}
	Run(fmt.Sprintf("gh secret set %s --body %s --repo %s", name, value, g.Repo), false, true, "")
}

func (g *GitHub) VariableSet(name, value string) {
	if g.DryRun {
		log.Logf("[DRY RUN] gh variable set %s --body \"%s\" --repo %s", name, value, g.Repo)
		return
	}
	Run(fmt.Sprintf("gh variable set %s --body %s --repo %s", name, value, g.Repo), false, true, "")
}

func (g *GitHub) Clone(dest string) {
	Run(fmt.Sprintf("gh repo clone %s %s", g.Repo, dest), false, true, "")
}

func (g *GitHub) WorkflowRun(workflow string, fields map[string]string) {
	var fieldArgs string
	for k, v := range fields {
		fieldArgs += fmt.Sprintf(" -f %s=%s", k, v)
	}
	g.run(fmt.Sprintf("workflow run %s%s", workflow, fieldArgs))
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
func (g *GitHub) pollRunUntilComplete(runID string) error {
	for {
		CheckRateLimit()

		statusOut, err := RunCapture(
			fmt.Sprintf("gh run view %s --repo %s --json status,conclusion --jq '[.status,.conclusion] | join(\",\")", runID, g.Repo),
			"",
		)
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

func (g *GitHub) Delete() {
	Run(fmt.Sprintf("gh repo delete %s --yes", g.Repo), false, false, "")
}
