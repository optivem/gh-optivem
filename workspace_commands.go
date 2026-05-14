// workspace_commands.go wires the `gh optivem workspace <verb>` subtree.
// The workspace noun spans operations that iterate every repo declared in
// the academy *.code-workspace file. The Go ports replace the bash scripts
// in academy/github-utils/scripts/ — see plans/20260514-0914-migrate-
// workspace-scripts-to-gh-optivem.md.
//
// Cascade for locating the workspace: --workspace > $GH_OPTIVEM_WORKSPACE >
// walk up from CWD. Resolution lives in internal/workspace.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/promptio"
	"github.com/optivem/gh-optivem/internal/shell"
	"github.com/optivem/gh-optivem/internal/workspace"
)

// workspaceFlagValue holds the --workspace persistent flag value. Read by
// every workspace subcommand and passed to workspace.Resolve as the
// highest-priority cascade input.
var workspaceFlagValue string

// commitCoAuthor is the trailer appended to every workspace-commit message.
// Kept verbatim from commit.sh for parity; flagged for review in a follow-up
// because gh-optivem itself is not a Claude session.
const commitCoAuthor = "Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"

const workspaceSeparator = "============================================"

// newWorkspaceCmd builds the `gh optivem workspace` parent. The parent has
// no Run, so invoking it without a subcommand prints help.
func newWorkspaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspace",
		Short: "Operate on every repo in the academy workspace",
	}
	cmd.PersistentFlags().StringVar(&workspaceFlagValue, "workspace", "",
		"Path to a directory containing a *.code-workspace file (default: $"+workspace.EnvVar+" or walk up from CWD)")
	cmd.AddCommand(
		newWorkspaceCommitCmd(),
		newWorkspaceSyncCmd(),
		newWorkspaceCheckActionsCmd(),
		newWorkspaceRateLimitCmd(),
	)
	return cmd
}

// ── commit ──────────────────────────────────────────────────────────────

// commitOptions captures the per-invocation flags for `workspace commit`.
type commitOptions struct {
	Repo             string
	Paths            string
	Yes              bool
	IncludeUntracked bool
}

func newWorkspaceCommitCmd() *cobra.Command {
	opts := commitOptions{}
	cmd := &cobra.Command{
		Use:   `commit [--repo <name>] [--paths "<paths>"] [--yes] [--include-untracked] "<message>"`,
		Short: "Commit, pull, and push every dirty repo in the workspace",
		Long: `Iterate every repo in the workspace. For each dirty repo, stage changes,
prompt for confirmation (y/N), and commit with the supplied message. After
the commit (or for already-clean repos), pull then push. Repos without a
remote tracking branch are skipped.

A commit message is required when any iterated repo has dirty changes.

` + "`--yes`" + ` skips the per-repo confirmation; required when stdin is not a TTY.
` + "`--yes`" + ` also refuses to stage untracked files unless ` + "`--include-untracked`" + `
is passed — the stray-file foot-gun is opt-in for scripted callers.`,
		Example: `  gh optivem workspace commit "Update settings"
  gh optivem workspace commit --repo myrepo "Fix bug"
  gh optivem workspace commit --repo myrepo --paths "system/monolith/java" "fix(monolith-java)"
  gh optivem workspace commit --yes "Sync .claude settings"`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			msg := ""
			if len(args) > 0 {
				msg = args[0]
			}
			if opts.Paths != "" && opts.Repo == "" {
				exitOnError(errors.New("--paths requires --repo (path semantics are repo-scoped)"))
			}
			exitOnError(runWorkspaceCommit(msg, opts))
		},
	}
	cmd.Flags().StringVar(&opts.Repo, "repo", "", "Only operate on the named repo")
	cmd.Flags().StringVar(&opts.Paths, "paths", "", `Stage only these space-separated paths (requires --repo)`)
	cmd.Flags().BoolVar(&opts.Yes, "yes", false, "Skip per-repo confirmation (required without a TTY)")
	cmd.Flags().BoolVar(&opts.IncludeUntracked, "include-untracked", false,
		"With --yes, also stage untracked (??) files. No effect interactively.")
	return cmd
}

func runWorkspaceCommit(msg string, opts commitOptions) error {
	_, folders, err := workspace.Resolve(workspaceFlagValue)
	if err != nil {
		return err
	}

	fmt.Println(workspaceSeparator)
	if opts.Repo != "" {
		fmt.Printf("  Commit Repo: %s\n", opts.Repo)
		if opts.Paths != "" {
			fmt.Printf("  Paths: %s\n", opts.Paths)
		}
	} else {
		fmt.Println("  Commit All Repos")
	}
	if msg == "" {
		fmt.Println("  Message: <none — will fail if dirty>")
	} else {
		fmt.Printf("  Message: %s\n", msg)
	}
	fmt.Println(workspaceSeparator)

	committed, synced, skipped := 0, 0, 0
	for _, repo := range folders {
		if opts.Repo != "" && repoBaseName(repo) != opts.Repo {
			continue
		}
		if !hasUpstream(repo) {
			skipped++
			continue
		}

		fmt.Println()
		fmt.Printf("--- %s ---\n", relOrSelf(repo))

		didCommit, err := commitOneRepo(repo, msg, opts)
		if err != nil {
			return err
		}
		if didCommit {
			committed++
		}

		if err := runGit(repo, "pull"); err != nil {
			return fmt.Errorf("git pull in %s: %w", repo, err)
		}
		if err := runGit(repo, "push"); err != nil {
			return fmt.Errorf("git push in %s: %w", repo, err)
		}
		synced++
		fmt.Println("  ✓ Pulled and pushed")
	}

	fmt.Println()
	fmt.Println(workspaceSeparator)
	fmt.Printf("  Done. %d committed, %d synced, %d skipped (no remote).\n", committed, synced, skipped)
	fmt.Println(workspaceSeparator)
	return nil
}

// commitOneRepo handles the stage/confirm/commit logic for one repo. Returns
// (true, nil) when a commit landed, (false, nil) when the repo was clean or
// the user declined.
func commitOneRepo(repo, msg string, opts commitOptions) (bool, error) {
	if opts.Paths != "" {
		pathArgs := splitPaths(opts.Paths)
		if err := runGit(repo, append([]string{"add", "--"}, pathArgs...)...); err != nil {
			return false, fmt.Errorf("git add in %s: %w", repo, err)
		}
		if cleanCached, _ := gitCachedClean(repo); cleanCached {
			fmt.Printf("  (no staged changes under: %s)\n", opts.Paths)
			return false, nil
		}
		if msg == "" {
			return false, errMissingMsg()
		}
		out, err := captureGit(repo, "diff", "--cached", "--name-only")
		if err != nil {
			return false, fmt.Errorf("git diff in %s: %w", repo, err)
		}
		for line := range strings.SplitSeq(strings.TrimRight(out, "\n"), "\n") {
			if line != "" {
				fmt.Printf("  %s\n", line)
			}
		}
		ok, err := confirmCommit(repo, opts)
		if err != nil {
			return false, err
		}
		if !ok {
			resetArgs := append([]string{"reset", "--quiet", "--"}, pathArgs...)
			_ = runGit(repo, resetArgs...)
			fmt.Println("  ✗ Skipped (unstaged)")
			return false, nil
		}
		if err := runGitCommit(repo, msg); err != nil {
			return false, err
		}
		fmt.Println("  ✓ Committed")
		return true, nil
	}

	status, err := captureGit(repo, "status", "--short")
	if err != nil {
		return false, fmt.Errorf("git status in %s: %w", repo, err)
	}
	if strings.TrimSpace(status) == "" {
		return false, nil
	}
	if msg == "" {
		return false, errMissingMsg()
	}
	fmt.Print(status)
	if !strings.HasSuffix(status, "\n") {
		fmt.Println()
	}
	if opts.Yes && !opts.IncludeUntracked {
		untracked := untrackedLines(status)
		if len(untracked) > 0 {
			var b strings.Builder
			b.WriteString("--yes refuses to stage untracked files. Either clean them up,\n")
			b.WriteString("       commit them via --paths, or pass --include-untracked to opt in.\n")
			b.WriteString("       Untracked entries:\n")
			for _, line := range untracked {
				b.WriteString("         ")
				b.WriteString(line)
				b.WriteString("\n")
			}
			return false, errors.New(b.String())
		}
	}
	ok, err := confirmCommit(repo, opts)
	if err != nil {
		return false, err
	}
	if !ok {
		fmt.Println("  ✗ Skipped")
		return false, nil
	}
	if err := runGit(repo, "add", "-A"); err != nil {
		return false, fmt.Errorf("git add -A in %s: %w", repo, err)
	}
	if err := runGitCommit(repo, msg); err != nil {
		return false, err
	}
	fmt.Println("  ✓ Committed")
	return true, nil
}

// confirmCommit returns true when the operator wants the commit to land.
// With --yes, returns true unconditionally. Without a TTY and without --yes,
// returns an error matching commit.sh's wording. Otherwise asks via promptio.
func confirmCommit(repo string, opts commitOptions) (bool, error) {
	if opts.Yes {
		return true, nil
	}
	if !stdinIsTTY() {
		return false, errors.New("stdin is not a TTY and --yes was not set; refusing to commit unattended.\n       Re-run with --yes if this is a scripted invocation.")
	}
	return promptio.ConfirmYN(os.Stdin, os.Stdout, fmt.Sprintf("Commit these changes to %s?", relOrSelf(repo)))
}

func errMissingMsg() error {
	return errors.New("commit message is required (no default).\n       Pass it as the last positional argument, e.g.:\n         gh optivem workspace commit \"<message>\"\n         gh optivem workspace commit --repo myrepo \"<message>\"")
}

func runGitCommit(repo, msg string) error {
	full := msg + "\n\n" + commitCoAuthor
	return runGit(repo, "commit", "-m", full)
}

// ── sync ────────────────────────────────────────────────────────────────

func newWorkspaceSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "sync",
		Short:   "Pull and push every repo in the workspace (no commit)",
		Example: `  gh optivem workspace sync`,
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			exitOnError(runWorkspaceSync())
		},
	}
}

func runWorkspaceSync() error {
	_, folders, err := workspace.Resolve(workspaceFlagValue)
	if err != nil {
		return err
	}

	fmt.Println(workspaceSeparator)
	fmt.Println("  Sync All Repos")
	fmt.Println(workspaceSeparator)

	synced, skipped := 0, 0
	for _, repo := range folders {
		if !hasUpstream(repo) {
			skipped++
			continue
		}
		fmt.Println()
		fmt.Printf("--- %s ---\n", relOrSelf(repo))
		if err := runGit(repo, "pull"); err != nil {
			return fmt.Errorf("git pull in %s: %w", repo, err)
		}
		if err := runGit(repo, "push"); err != nil {
			return fmt.Errorf("git push in %s: %w", repo, err)
		}
		synced++
		fmt.Println("  ✓ Pulled and pushed")
	}

	fmt.Println()
	fmt.Println(workspaceSeparator)
	fmt.Printf("  Done. %d synced, %d skipped (no remote).\n", synced, skipped)
	fmt.Println(workspaceSeparator)
	return nil
}

// ── check-actions ───────────────────────────────────────────────────────

func newWorkspaceCheckActionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "check-actions",
		Short:   "Report the latest GitHub Actions run for every workflow in every workspace repo",
		Example: `  gh optivem workspace check-actions`,
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			exitOnError(runWorkspaceCheckActions())
		},
	}
}

// workflowFailure carries the data needed to render the failure-details
// footer for one failing workflow run.
type workflowFailure struct {
	Repo     string
	Workflow string
	Title    string
	RunID    string
	RunURL   string
	Errors   string
}

func runWorkspaceCheckActions() error {
	_, folders, err := workspace.Resolve(workspaceFlagValue)
	if err != nil {
		return err
	}

	fmt.Println(workspaceSeparator)
	fmt.Println("  GitHub Actions Status Check")
	fmt.Println(workspaceSeparator)

	totalPassing, totalFailing, noWorkflows := 0, 0, 0
	var failures []workflowFailure

	for _, repo := range folders {
		shell.CheckRateLimit()

		workflows, err := shell.RunWithRetry(`gh workflow list --all`, true, repo)
		if err != nil || strings.TrimSpace(workflows) == "" {
			noWorkflows++
			fmt.Printf("  ⏭  %s — no workflows\n", relOrSelf(repo))
			continue
		}

		nwo := ""
		if out, err := shell.RunWithRetry(`gh repo view --json nameWithOwner -q .nameWithOwner`, false, repo); err == nil {
			nwo = strings.TrimSpace(out)
		}

		repoPassing, repoFailing, repoInProgress := 0, 0, 0
		var repoFailures []workflowFailure

		for line := range strings.SplitSeq(strings.TrimRight(workflows, "\n"), "\n") {
			fields := strings.Split(line, "\t")
			if len(fields) < 3 {
				continue
			}
			wfName, wfID := fields[0], fields[2]
			if wfName == "" {
				continue
			}

			latest, err := shell.RunWithRetry(fmt.Sprintf(`gh run list --workflow %s --limit 1`, wfID), false, repo)
			if err != nil || strings.TrimSpace(latest) == "" {
				continue
			}
			firstLine := strings.SplitN(strings.TrimRight(latest, "\n"), "\n", 2)[0]
			cols := strings.Split(firstLine, "\t")
			// columns: status, conclusion, title, workflow, branch, event, id, elapsed, date
			if len(cols) < 7 {
				continue
			}
			status, conclusion, title, workflowCol, runID := cols[0], cols[1], cols[2], cols[3], cols[6]

			switch {
			case status == "in_progress" || status == "queued":
				repoInProgress++
			case conclusion == "failure":
				repoFailing++
				errSnippet := failureSnippet(repo, runID)
				runURL := ""
				if nwo != "" {
					runURL = fmt.Sprintf("https://github.com/%s/actions/runs/%s", nwo, runID)
				}
				repoFailures = append(repoFailures, workflowFailure{
					Repo:     relOrSelf(repo),
					Workflow: workflowCol,
					Title:    title,
					RunID:    runID,
					RunURL:   runURL,
					Errors:   errSnippet,
				})
			default:
				repoPassing++
			}
		}

		if repoFailing > 0 {
			totalFailing++
			parts := []string{}
			if repoPassing > 0 {
				parts = append(parts, fmt.Sprintf("%d passing", repoPassing))
			}
			parts = append(parts, fmt.Sprintf("%d failing", repoFailing))
			if repoInProgress > 0 {
				parts = append(parts, fmt.Sprintf("%d in progress", repoInProgress))
			}
			fmt.Printf("  ❌ %s — %s\n", relOrSelf(repo), strings.Join(parts, ", "))
			failures = append(failures, repoFailures...)
		} else {
			totalPassing++
			if repoInProgress > 0 {
				fmt.Printf("  ✅ %s — %d passing, %d in progress\n", relOrSelf(repo), repoPassing, repoInProgress)
			} else {
				fmt.Printf("  ✅ %s — %d passing\n", relOrSelf(repo), repoPassing)
			}
		}
	}

	fmt.Println()
	fmt.Println(workspaceSeparator)
	fmt.Printf("  Summary: %d passing, %d failing, %d no workflows\n", totalPassing, totalFailing, noWorkflows)
	fmt.Println(workspaceSeparator)

	if len(failures) > 0 {
		fmt.Println()
		fmt.Println("Failure details:")
		for _, f := range failures {
			fmt.Println()
			fmt.Printf("  ❌ %s / %s\n", f.Repo, f.Workflow)
			fmt.Printf("     Commit: %s\n", f.Title)
			if f.RunURL != "" {
				fmt.Printf("     Run:    %s\n", f.RunURL)
			}
			fmt.Println("     Errors:")
			fmt.Print(f.Errors)
			if !strings.HasSuffix(f.Errors, "\n") {
				fmt.Println()
			}
		}
	}
	return nil
}

// failureSnippet returns the last three `##[error]` lines from the failed-log
// of a workflow run, each prefixed with "  ". Falls back to a placeholder
// when no error lines are present. Matches the bash sed trim in
// check-actions-all.sh:75.
func failureSnippet(repo, runID string) string {
	out, err := shell.RunWithRetry(fmt.Sprintf(`gh run view %s --log-failed`, runID), false, repo)
	if err != nil && out == "" {
		return "  (no error details available)\n"
	}
	var hits []string
	for line := range strings.SplitSeq(out, "\n") {
		if _, tail, ok := strings.Cut(line, "##[error]"); ok {
			hits = append(hits, "  "+tail)
		}
	}
	if len(hits) == 0 {
		return "  (no error details available)\n"
	}
	if len(hits) > 3 {
		hits = hits[len(hits)-3:]
	}
	return strings.Join(hits, "\n") + "\n"
}

// ── rate-limit ──────────────────────────────────────────────────────────

func newWorkspaceRateLimitCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "rate-limit",
		Short:   "Show current GitHub API rate limits and reset times",
		Example: `  gh optivem workspace rate-limit`,
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			exitOnError(runWorkspaceRateLimit())
		},
	}
}

func runWorkspaceRateLimit() error {
	fmt.Println("==========================================")
	fmt.Println("  GitHub API Rate Limits")
	fmt.Println("==========================================")
	fmt.Println()
	const jq = `.resources | to_entries[] | "\(.key):\n  Used:      \(.value.used) / \(.value.limit)\n  Remaining: \(.value.remaining)\n  Resets at: \(.value.reset | todate)\n"`
	out, err := shell.RunCapture(`gh api rate_limit --jq `+shellQuote(jq), "")
	if err != nil {
		return fmt.Errorf("gh api rate_limit: %w", err)
	}
	fmt.Println(out)
	return nil
}

// ── helpers ─────────────────────────────────────────────────────────────

// runGit runs `git -C <repo> <args...>` with stdout/stderr passed through to
// the operator. Local git operations are not retried — a failure usually
// means a real condition the operator must resolve (merge conflict, no
// upstream, auth, etc.).
func runGit(repo string, args ...string) error {
	full := append([]string{"-C", repo}, args...)
	cmd := exec.Command("git", full...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// captureGit returns the stdout of `git -C <repo> <args...>`. Used for
// status/diff parsing; stderr is discarded so a "no upstream" warning
// does not pollute parsing.
func captureGit(repo string, args ...string) (string, error) {
	full := append([]string{"-C", repo}, args...)
	cmd := exec.Command("git", full...)
	out, err := cmd.Output()
	return string(out), err
}

// gitCachedClean returns true when `git diff --cached --quiet` exits zero —
// i.e. nothing is staged. Mirrors commit.sh:201.
func gitCachedClean(repo string) (bool, error) {
	full := []string{"-C", repo, "diff", "--cached", "--quiet"}
	cmd := exec.Command("git", full...)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

// hasUpstream reports whether the repo's current branch has a remote tracking
// branch. Repos without one are skipped — mirrors commit.sh:188 and sync.sh:30.
func hasUpstream(repo string) bool {
	cmd := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

// untrackedLines returns the lines from `git status --short` that begin with
// `??` (untracked).
func untrackedLines(status string) []string {
	var out []string
	sc := bufio.NewScanner(strings.NewReader(status))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "??") {
			out = append(out, line)
		}
	}
	return out
}

// splitPaths splits a whitespace-separated path list as the bash word-split
// would. Empty entries are dropped.
func splitPaths(s string) []string {
	fields := strings.Fields(s)
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// repoBaseName returns the last path segment of repo for --repo matching.
func repoBaseName(repo string) string {
	repo = strings.TrimRight(repo, `/\`)
	if i := strings.LastIndexAny(repo, `/\`); i >= 0 {
		return repo[i+1:]
	}
	return repo
}

// relOrSelf renders a repo path relative to the current working directory
// when feasible (cleaner output); falls back to the absolute path otherwise.
// Skips relativization when the result would escape upward more than two
// levels — the long `../../..` chain is less readable than the absolute path.
func relOrSelf(repo string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return repo
	}
	rel, err := filepath.Rel(cwd, repo)
	if err != nil {
		return repo
	}
	if strings.HasPrefix(rel, "..") {
		// Count leading `..` segments; bail out for deeply nested cases.
		parts := strings.Split(rel, string(filepath.Separator))
		ups := 0
		for _, p := range parts {
			if p == ".." {
				ups++
			} else {
				break
			}
		}
		if ups > 2 {
			return repo
		}
	}
	return rel
}

// stdinIsTTY mirrors main.go's isInteractive — duplicated here rather than
// exported to keep the private helper private. Both check the same thing
// (charDevice mode bit on stdin).
func stdinIsTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// shellQuote wraps s in single quotes, escaping any embedded single quotes
// using the standard POSIX `'\''` dance. Used when constructing commands
// passed to internal/shell.RunCapture (which itself splits on spaces).
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
