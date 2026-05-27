// cross_repo_commands.go wires the cross-repo verbs that live at the root
// of `gh optivem` — commit, sync, actions status, rate-limit, and the
// hidden TBD-discipline reports (lint-history, stale-branches).
//
// The verbs share the same scope-resolution cascade (workspace.Resolve in
// internal/workspace): when a *.code-workspace file is reachable, every
// declared folder is iterated; otherwise the scope shrinks to the CWD's
// git repo. The resolved scope is announced on every invocation via the
// "Mode: …" banner so the operator always sees what's about to happen.
//
// The --workspace flag is registered at root (main.go); its value lands in
// workspaceFlagValue below and is passed to workspace.Resolve as the
// highest-priority cascade input.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/approval"
	"github.com/optivem/gh-optivem/internal/cmdctx"
	"github.com/optivem/gh-optivem/internal/shell"
	"github.com/optivem/gh-optivem/internal/workspace"
)

// workspaceFlagValue holds the --workspace persistent flag value. Bound
// at root in main.go (newRootCmd); read by every cross-repo verb and
// passed to workspace.Resolve as the highest-priority cascade input.
var workspaceFlagValue string

// commitCoAuthor is the trailer appended to every commit message. Kept
// verbatim from commit.sh for parity; flagged for review in a follow-up
// because gh-optivem itself is not a Claude session.
const commitCoAuthor = "Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"

const workspaceSeparator = "============================================"

// scopeBannerLine returns the one-line "Mode: …" announcement printed at
// the top of every cross-repo verb's banner block. The line names what
// scope resolved so the operator can confirm at a glance whether the
// invocation will touch the whole workspace, the project's own repos,
// or just the cwd repo.
//
// Wording is fixed by the plan's decisions log (2026-05-15): workspace
// mode names the count + source file; project mode names the count and
// names gh-optivem.yaml as the source; single-repo mode names just the
// repo basename, no "no workspace file found" trailer.
func scopeBannerLine(scope workspace.Scope) string {
	switch scope.Mode {
	case workspace.ModeWorkspace:
		return fmt.Sprintf("Mode: workspace (%d repos from %s)", len(scope.Folders), filepath.Base(scope.SourceFile))
	case workspace.ModeProject:
		return fmt.Sprintf("Mode: project (%d repos from %s)", len(scope.Folders), filepath.Base(scope.SourceFile))
	case workspace.ModeSingleRepo:
		return fmt.Sprintf("Mode: single repo (%s)", filepath.Base(scope.Root))
	}
	return "Mode: unknown"
}

// ── commit ──────────────────────────────────────────────────────────────

// commitOptions captures the per-invocation flags for `commit`.
//
// Approval is the resolved global auto-approve policy (--auto + --confirm).
// The per-command Yes flag still wins ahead of it inside confirmCommit so
// scripts that pass `commit --yes "msg"` keep their existing semantics
// regardless of the global policy. When Yes is false the prompt routes
// through approval.Confirm with CategoryCommit, which by default still
// prompts under --auto because commit is in the default exclusion set.
type commitOptions struct {
	Repo             string
	Paths            string
	Yes              bool
	IncludeUntracked bool
	Approval         approval.Resolved
}

func newCommitCmd() *cobra.Command {
	opts := commitOptions{}
	cmd := &cobra.Command{
		Use:   `commit [--repo <name>] [--paths "<paths>"] [--yes] [--include-untracked] ["<message>"]`,
		Short: "Commit, pull, and push every dirty repo in scope",
		Long: `Iterate every repo in scope (workspace folders, or the cwd repo when no
*.code-workspace is reachable — see "Mode: …" banner). For each dirty repo,
stage changes, prompt for confirmation (y/N), and commit with the supplied
message. When the branch has an upstream, pull --rebase before staging and
push after. When the branch has no upstream (e.g. local-only rehearsal
branches), commit locally and skip the pull/push — the operator is told
"(local only — no upstream branch)". Clean repos with no upstream are
counted as skipped since neither commit nor sync can occur.

A commit message is required when any iterated repo has dirty changes.

` + "`--yes`" + ` skips the per-repo confirmation; required when stdin is not a TTY.
` + "`--yes`" + ` also refuses to stage untracked files unless ` + "`--include-untracked`" + `
is passed — the stray-file foot-gun is opt-in for scripted callers.`,
		Example: `  gh optivem commit "Update settings"
  gh optivem commit --repo myrepo "Fix bug"
  gh optivem commit --repo myrepo --paths "system/monolith/java" "fix(monolith-java)"
  gh optivem commit --yes "Sync .claude settings"`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			msg := ""
			if len(args) > 0 {
				msg = args[0]
			}
			if opts.Paths != "" && opts.Repo == "" {
				exitOnError(errors.New("--paths requires --repo (path semantics are repo-scoped)"))
			}
			opts.Approval = cmdctx.Approval(cmd)
			exitOnError(runCommit(msg, opts))
		},
	}
	cmd.Flags().StringVar(&opts.Repo, "repo", "", "Only operate on the named repo")
	cmd.Flags().StringVar(&opts.Paths, "paths", "", `Stage only these space-separated paths (requires --repo)`)
	cmd.Flags().BoolVar(&opts.Yes, "yes", false, "Skip per-repo confirmation (required without a TTY)")
	cmd.Flags().BoolVar(&opts.IncludeUntracked, "include-untracked", false,
		"With --yes, also stage untracked (??) files. No effect interactively.")
	return cmd
}

func runCommit(msg string, opts commitOptions) error {
	scope, err := workspace.Resolve(workspaceFlagValue)
	if err != nil {
		return err
	}

	fmt.Println(workspaceSeparator)
	fmt.Printf("  %s\n", scopeBannerLine(scope))
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

	committed, synced, localOnly, skipped := 0, 0, 0, 0
	for _, repo := range scope.Folders {
		if opts.Repo != "" && repoBaseName(repo) != opts.Repo {
			continue
		}

		hasUp := hasUpstream(repo)
		// Skip only when there's nothing to do: clean working tree AND no
		// upstream to sync with. A dirty repo without an upstream still
		// gets a local commit — see the "(local only)" path below.
		if !hasUp {
			clean, err := workingTreeClean(repo)
			if err != nil {
				return fmt.Errorf("git status in %s: %w", repo, err)
			}
			if clean {
				skipped++
				continue
			}
		}

		fmt.Println()
		fmt.Printf("--- %s ---\n", relOrSelf(repo))
		fmt.Printf("  %s\n", tbdModeBanner(repo))

		// Pull --rebase onto the current branch's upstream *before*
		// staging/committing so the new commit lands on top of the freshest
		// remote tip, not a stale local ref. Working tree is dirty by
		// definition here — stash unstaged + staged changes, rebase, then
		// pop. Emulates `rebase.autoStash` regardless of operator config.
		// No upstream → nothing to rebase onto, skip the pull.
		if hasUp {
			if err := pullWithAutoStash(repo); err != nil {
				return fmt.Errorf("pre-commit git pull --rebase in %s: %w", repo, err)
			}
		}

		didCommit, err := commitOneRepo(repo, msg, opts)
		if err != nil {
			return err
		}
		if didCommit {
			committed++
		}

		if hasUp {
			if err := pushWithRebaseRetry(repo); err != nil {
				return err
			}
			synced++
			fmt.Println("  ✓ Pulled and pushed")
		} else if didCommit {
			localOnly++
			fmt.Println("  ✓ Committed (local only — no upstream branch)")
		}
	}

	fmt.Println()
	fmt.Println(workspaceSeparator)
	fmt.Printf("  Done. %d committed (%d synced, %d local only), %d skipped (no upstream and clean).\n", committed, synced, localOnly, skipped)
	fmt.Println(workspaceSeparator)
	return nil
}

// workingTreeClean reports whether `git status --short` is empty in repo —
// i.e. no modified, staged, or untracked entries.
func workingTreeClean(repo string) (bool, error) {
	status, err := captureGit(repo, "status", "--short")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(status) == "", nil
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
// With --yes, returns true unconditionally — the per-command primitive
// preserves the documented `commit --yes "msg"` script contract regardless
// of the global --auto policy. Without --yes, the prompt routes through
// approval.Confirm(CategoryCommit) so --auto + --confirm=… can opt the
// commit gate in or out of auto-yes. Without a TTY and without either
// flag, returns an error matching commit.sh's wording.
func confirmCommit(repo string, opts commitOptions) (bool, error) {
	if opts.Yes {
		return true, nil
	}
	if !stdinIsTTY() {
		return false, errors.New("stdin is not a TTY and --yes was not set; refusing to commit unattended.\n       Re-run with --yes if this is a scripted invocation.")
	}
	return approval.Confirm(opts.Approval, approval.CategoryCommit, os.Stdin, os.Stdout, fmt.Sprintf("Commit these changes to %s?", relOrSelf(repo)))
}

func errMissingMsg() error {
	return errors.New("commit message is required (no default).\n       Pass it as the last positional argument, e.g.:\n         gh optivem commit \"<message>\"\n         gh optivem commit --repo myrepo \"<message>\"")
}

func runGitCommit(repo, msg string) error {
	full := msg + "\n\n" + commitCoAuthor
	return runGit(repo, "commit", "-m", full)
}

// ── sync ────────────────────────────────────────────────────────────────

func newSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "sync",
		Short:   "Pull and push every repo in scope (no commit)",
		Example: `  gh optivem sync`,
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			exitOnError(runSync())
		},
	}
}

func runSync() error {
	scope, err := workspace.Resolve(workspaceFlagValue)
	if err != nil {
		return err
	}

	fmt.Println(workspaceSeparator)
	fmt.Printf("  %s\n", scopeBannerLine(scope))
	fmt.Println("  Sync All Repos")
	fmt.Println(workspaceSeparator)

	synced, skipped := 0, 0
	for _, repo := range scope.Folders {
		if !hasUpstream(repo) {
			skipped++
			continue
		}
		fmt.Println()
		fmt.Printf("--- %s ---\n", relOrSelf(repo))
		fmt.Printf("  %s\n", tbdModeBanner(repo))
		if err := runGit(repo, "pull", "--rebase"); err != nil {
			return fmt.Errorf("git pull --rebase in %s: %w", repo, err)
		}
		if err := pushWithRebaseRetry(repo); err != nil {
			return err
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

// ── actions (noun-verb) ─────────────────────────────────────────────────

// newActionsCmd builds the `actions` noun parent. Query verbs that read
// CI state and present a summary live under it (matches `gh run list` /
// `gh workflow list` conventions). The noun is extensible — `actions
// list`, `actions rerun` could grow here later.
func newActionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "actions",
		Short: "Inspect GitHub Actions across repos in scope",
	}
	cmd.AddCommand(newActionsStatusCmd())
	return cmd
}

func newActionsStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "status",
		Short:   "Report the latest GitHub Actions run for every workflow in every repo in scope",
		Example: `  gh optivem actions status`,
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			exitOnError(runActionsStatus())
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

func runActionsStatus() error {
	scope, err := workspace.Resolve(workspaceFlagValue)
	if err != nil {
		return err
	}

	fmt.Println(workspaceSeparator)
	fmt.Printf("  %s\n", scopeBannerLine(scope))
	fmt.Println("  GitHub Actions Status Check")
	fmt.Println(workspaceSeparator)

	totalPassing, totalFailing, noWorkflows := 0, 0, 0
	var failures []workflowFailure

	for _, repo := range scope.Folders {
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

// newRateLimitCmd builds the bare `rate-limit` verb. Single GitHub API
// call — no scope cascade is needed (the rate limit is per-token, not
// per-repo), so this command does not call workspace.Resolve and does
// not print the scope banner.
func newRateLimitCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "rate-limit",
		Short:   "Show current GitHub API rate limits and reset times",
		Example: `  gh optivem rate-limit`,
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			exitOnError(runRateLimit())
		},
	}
}

func runRateLimit() error {
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

// ── lint-history (TBD-discipline drift detector) ────────────────────────

// NOTE: lint-history and stale-branches were added to enforce TBD
// discipline (docs/tbd.md) — they catch when someone has fallen off the
// rails. lint-history flags merge commits on main, which violate the
// "linear trunk" rule (#4). stale-branches flags local branches older
// than --age, which violate the "hours, not days" rule (#3 / line 61).
// Both are periodic-audit tools, not daily-workflow tools.
//
// TODO(placement): they're currently Hidden at root while the operator
// decides where they belong long-term. Options: keep at root + unhide,
// move under a new `tbd` / `audit` noun, or merge into `actions` if its
// scope broadens. Revisit per plans/20260515-0736-extend-test-system-
// with-universal-git-verbs.md "Deferred follow-ups".

// lintHistoryOptions captures the per-invocation flags for `lint-history`.
type lintHistoryOptions struct {
	Limit int
}

func newLintHistoryCmd() *cobra.Command {
	opts := lintHistoryOptions{}
	cmd := &cobra.Command{
		Use:    "lint-history [--limit N]",
		Short:  "Flag merge commits on main in every repo in scope (docs/tbd.md drift detector)",
		Hidden: true,
		Long: `For each repo in scope, scan the last N first-parent commits of main for
merge commits and flag any hits. docs/tbd.md mandates a linear trunk — any
merge commit on main is drift. Exits non-zero when drift is found.

Inspected ref is origin/main when present, falling back to local main. Repos
with neither are skipped. Run ` + "`gh optivem sync`" + ` first if you want the
freshest remote state — lint-history does not fetch.`,
		Example: `  gh optivem lint-history
  gh optivem lint-history --limit 500`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			exitOnError(runLintHistory(opts))
		},
	}
	cmd.Flags().IntVar(&opts.Limit, "limit", 100, "Number of first-parent commits on main to scan per repo")
	return cmd
}

func runLintHistory(opts lintHistoryOptions) error {
	if opts.Limit <= 0 {
		return fmt.Errorf("--limit must be a positive integer, got %d", opts.Limit)
	}
	scope, err := workspace.Resolve(workspaceFlagValue)
	if err != nil {
		return err
	}

	fmt.Println(workspaceSeparator)
	fmt.Printf("  %s\n", scopeBannerLine(scope))
	fmt.Printf("  Lint history — last %d commits on main per repo\n", opts.Limit)
	fmt.Println(workspaceSeparator)

	cleanRepos, dirtyRepos, skippedRepos := 0, 0, 0
	for _, repo := range scope.Folders {
		fmt.Println()
		fmt.Printf("--- %s ---\n", relOrSelf(repo))
		ref := mainLintRef(repo)
		if ref == "" {
			fmt.Println("  ⏭  no main / origin/main — skipped")
			skippedRepos++
			continue
		}
		merges, err := lintHistoryOneRepo(repo, ref, opts.Limit)
		if err != nil {
			return fmt.Errorf("git log in %s: %w", repo, err)
		}
		if len(merges) == 0 {
			fmt.Printf("  ✓ linear (%s)\n", ref)
			cleanRepos++
			continue
		}
		fmt.Printf("  ✗ %d merge commit(s) on %s:\n", len(merges), ref)
		for _, line := range merges {
			fmt.Printf("    %s\n", line)
		}
		dirtyRepos++
	}

	fmt.Println()
	fmt.Println(workspaceSeparator)
	fmt.Printf("  Done. %d linear, %d with merge commits, %d skipped.\n", cleanRepos, dirtyRepos, skippedRepos)
	fmt.Println(workspaceSeparator)
	if dirtyRepos > 0 {
		return fmt.Errorf("lint-history: %d repo(s) have merge commits on main (docs/tbd.md requires linear trunk)", dirtyRepos)
	}
	return nil
}

// lintHistoryOneRepo returns the merge commits in the last `limit` first-parent
// commits of `ref` in repo. Each line is "<short-sha> <subject>". Returns
// (nil, nil) for a clean linear window.
func lintHistoryOneRepo(repo, ref string, limit int) ([]string, error) {
	out, err := captureGit(repo, "log", "--merges", "--first-parent", ref,
		fmt.Sprintf("-%d", limit), "--pretty=format:%h %s")
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimRight(out, "\n")
	if trimmed == "" {
		return nil, nil
	}
	return strings.Split(trimmed, "\n"), nil
}

// mainLintRef returns "origin/main" when that ref exists, falling back to
// "main" when only the local branch is present, or "" when neither is. The
// remote-tracking ref is preferred so lint-history reports against the
// authoritative trunk, not a stale local branch.
func mainLintRef(repo string) string {
	if exec.Command("git", "-C", repo, "rev-parse", "--verify", "--quiet", "origin/main").Run() == nil {
		return "origin/main"
	}
	if exec.Command("git", "-C", repo, "rev-parse", "--verify", "--quiet", "main").Run() == nil {
		return "main"
	}
	return ""
}

// ── stale-branches (TBD-discipline drift detector) ──────────────────────
//
// See the NOTE above lint-history. stale-branches enforces the same
// docs/tbd.md discipline from a different angle: branches that have
// drifted past the Scaled-TBD "hours, not days" threshold.

// staleBranchesOptions captures the per-invocation flags for `stale-branches`.
type staleBranchesOptions struct {
	Age time.Duration
}

// staleBranch is one local branch that's older than the threshold, with the
// committer-date of its tip.
type staleBranch struct {
	Name string
	Tip  time.Time
}

func newStaleBranchesCmd() *cobra.Command {
	opts := staleBranchesOptions{Age: 24 * time.Hour}
	cmd := &cobra.Command{
		Use:    "stale-branches [--age <duration>]",
		Short:  "List local branches in each repo in scope whose tip is older than the threshold",
		Hidden: true,
		Long: `For each repo in scope, list local branches (excluding main) whose tip
commit was authored more than --age ago. docs/tbd.md:62 sets the "hours, not
days" expectation for Scaled-TBD branch lifetime; this command helps
Scaled-TBD teams notice when a branch has drifted from that discipline.

Reports the branch name and the age of the tip commit. Exits zero whether or
not stale branches were found — the output is informational, not enforcement.`,
		Example: `  gh optivem stale-branches
  gh optivem stale-branches --age 4h
  gh optivem stale-branches --age 72h`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			exitOnError(runStaleBranches(opts))
		},
	}
	cmd.Flags().DurationVar(&opts.Age, "age", 24*time.Hour,
		"Flag branches whose tip is older than this duration (Go syntax: 4h, 72h, 1h30m)")
	return cmd
}

func runStaleBranches(opts staleBranchesOptions) error {
	if opts.Age <= 0 {
		return fmt.Errorf("--age must be a positive duration, got %v", opts.Age)
	}
	scope, err := workspace.Resolve(workspaceFlagValue)
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-opts.Age)

	fmt.Println(workspaceSeparator)
	fmt.Printf("  %s\n", scopeBannerLine(scope))
	fmt.Printf("  Stale branches — tip older than %s per repo\n", opts.Age)
	fmt.Println(workspaceSeparator)

	cleanRepos, staleRepoCount, staleBranchTotal := 0, 0, 0
	for _, repo := range scope.Folders {
		fmt.Println()
		fmt.Printf("--- %s ---\n", relOrSelf(repo))
		stale, err := staleBranchesOneRepo(repo, cutoff)
		if err != nil {
			return fmt.Errorf("stale-branches in %s: %w", repo, err)
		}
		if len(stale) == 0 {
			fmt.Println("  ✓ no stale branches")
			cleanRepos++
			continue
		}
		staleRepoCount++
		for _, b := range stale {
			fmt.Printf("  ⚠ %s — last commit %s ago\n", b.Name, formatBranchAge(time.Since(b.Tip)))
			staleBranchTotal++
		}
	}

	fmt.Println()
	fmt.Println(workspaceSeparator)
	fmt.Printf("  Done. %d clean, %d with stale branches (%d total, threshold %s).\n",
		cleanRepos, staleRepoCount, staleBranchTotal, opts.Age)
	fmt.Println(workspaceSeparator)
	return nil
}

// staleBranchesOneRepo returns local branches (excluding main) whose tip
// committer-date is at or before cutoff. Sorted oldest-first so the worst
// offender appears first in the report.
func staleBranchesOneRepo(repo string, cutoff time.Time) ([]staleBranch, error) {
	out, err := captureGit(repo, "for-each-ref",
		"--format=%(refname:short)|%(committerdate:unix)",
		"refs/heads")
	if err != nil {
		return nil, fmt.Errorf("git for-each-ref: %w", err)
	}
	var stale []staleBranch
	for line := range strings.SplitSeq(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		name, tsStr, ok := strings.Cut(line, "|")
		if !ok {
			continue
		}
		if name == "main" {
			continue
		}
		ts, err := strconv.ParseInt(strings.TrimSpace(tsStr), 10, 64)
		if err != nil {
			continue
		}
		tip := time.Unix(ts, 0)
		if !tip.After(cutoff) {
			stale = append(stale, staleBranch{Name: name, Tip: tip})
		}
	}
	sort.Slice(stale, func(i, j int) bool { return stale[i].Tip.Before(stale[j].Tip) })
	return stale, nil
}

// formatBranchAge renders d as "<1h", "Nh", "Nd", or "Nd Mh" — coarse
// resolution suitable for a stale-branch report. Sub-hour ages are not useful
// here so they round down to "<1h".
func formatBranchAge(d time.Duration) string {
	if d < time.Hour {
		return "<1h"
	}
	hours := int(d.Hours())
	if hours < 24 {
		return fmt.Sprintf("%dh", hours)
	}
	days := hours / 24
	rem := hours % 24
	if rem == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd %dh", days, rem)
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

// runGitTeeStderr runs `git -C <repo> <args...>`, streams stderr to the
// operator AND returns it as a string so the caller can pattern-match on
// rejection messages. Used by pushWithRebaseRetry to detect non-fast-forward
// rejection without losing the operator's view of what git said.
func runGitTeeStderr(repo string, args ...string) (string, error) {
	full := append([]string{"-C", repo}, args...)
	cmd := exec.Command("git", full...)
	cmd.Stdout = os.Stdout
	var captured strings.Builder
	cmd.Stderr = io.MultiWriter(os.Stderr, &captured)
	err := cmd.Run()
	return captured.String(), err
}

// pullWithAutoStash runs `git pull --rebase` in repo, stashing any
// uncommitted tracked changes first and popping the stash afterwards. Mirrors
// `rebase.autoStash=true` regardless of the operator's git config, so the
// pre-commit pull inside `commit` works on a dirty working tree. Untracked
// files are left alone — `git pull --rebase` does not touch them unless an
// incoming change creates a file with the same name (rare, and the operator
// should resolve that explicitly).
func pullWithAutoStash(repo string) error {
	dirty := exec.Command("git", "-C", repo, "diff-index", "--quiet", "HEAD").Run() != nil
	if dirty {
		if err := runGit(repo, "stash", "push", "--quiet", "-m", "gh optivem pre-commit auto-stash"); err != nil {
			return fmt.Errorf("git stash in %s: %w", repo, err)
		}
	}
	pullErr := runGit(repo, "pull", "--rebase")
	if dirty {
		if popErr := runGit(repo, "stash", "pop", "--quiet"); popErr != nil {
			if pullErr != nil {
				return fmt.Errorf("git pull --rebase failed (%w) and stash pop failed (%v); inspect `git stash list` in %s", pullErr, popErr, repo)
			}
			return fmt.Errorf("git stash pop in %s: %w; inspect `git stash list`", repo, popErr)
		}
	}
	return pullErr
}

// pushWithRebaseRetry runs `git push` in repo, with up to maxPushAttempts
// attempts when the push is rejected for being non-fast-forward (e.g. the bot
// landed `Bump VERSION ...` on main between our pull and push — see
// docs/tbd.md:151-169). On rejection, runs `git pull --rebase` and retries.
// Non-rejection errors (auth, network, etc.) are not retried — another pull
// won't fix them.
func pushWithRebaseRetry(repo string) error {
	if err := mainForcePushGuard(repo); err != nil {
		return err
	}
	const maxPushAttempts = 3
	for attempt := 1; attempt <= maxPushAttempts; attempt++ {
		stderr, err := runGitTeeStderr(repo, "push")
		if err == nil {
			return nil
		}
		if !isNonFastForwardRejection(stderr) || attempt == maxPushAttempts {
			return fmt.Errorf("git push in %s: %w", repo, err)
		}
		fmt.Printf("  ↻ racing origin/main, retrying (%d/%d)…\n", attempt, maxPushAttempts-1)
		if err := pullWithAutoStash(repo); err != nil {
			return fmt.Errorf("git pull --rebase in %s during push retry: %w", repo, err)
		}
	}
	return nil
}

// currentBranch returns the short ref name of HEAD in repo (e.g. "main" or
// "feature/x"). Empty on detached HEAD or error.
func currentBranch(repo string) string {
	out, err := captureGit(repo, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// upstreamRef returns the short upstream tracking ref for the current branch
// (e.g. "origin/main"). Empty when the branch has no upstream — cross-repo
// loops already skip such repos via hasUpstream.
func upstreamRef(repo string) string {
	out, err := captureGit(repo, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// tbdModeBanner returns the one-line "plain TBD" / "Scaled TBD" banner
// printed at the top of each repo's commit/sync iteration. Names the
// docs/tbd.md framing where the operator is actually doing work so the
// model is visible, not just documented.
func tbdModeBanner(repo string) string {
	branch := currentBranch(repo)
	if branch == "" || branch == "HEAD" {
		return "TBD mode: unknown (detached HEAD)"
	}
	if branch == "main" {
		return "plain TBD (on `main`)"
	}
	up := upstreamRef(repo)
	if up == "" {
		return fmt.Sprintf("Scaled TBD (on `%s`)", branch)
	}
	return fmt.Sprintf("Scaled TBD (on `%s`, upstream `%s`)", branch, up)
}

// mainForcePushGuard refuses to push when the current branch is `main` and
// local has diverged from the upstream in a way that would require a
// force-push (commits on both sides of HEAD...@{u}). Defense-in-depth for
// docs/tbd.md's "never force-push main" rule — the retry loop in
// pushWithRebaseRetry brings origin into local on a normal non-fast-forward
// rejection, so this guard only fires for the actually-dangerous case
// (e.g. an accidental `git reset` of an already-pushed main).
func mainForcePushGuard(repo string) error {
	if currentBranch(repo) != "main" {
		return nil
	}
	out, err := captureGit(repo, "rev-list", "--left-right", "--count", "HEAD...@{u}")
	if err != nil {
		// No upstream / unknown error — cross-repo loops already filter
		// no-upstream repos; surface other errors verbatim from the push.
		return nil
	}
	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) != 2 {
		return nil
	}
	if fields[0] != "0" && fields[1] != "0" {
		return fmt.Errorf("refusing to push `%s`: local main has diverged from upstream (ahead %s, behind %s).\n       Pushing would require --force, which is forbidden on main (docs/tbd.md).\n       Inspect:  git -C %s log --oneline --left-right HEAD...@{u}",
			repo, fields[0], fields[1], repo)
	}
	return nil
}

// isNonFastForwardRejection reports whether stderr from `git push` shows the
// "someone else pushed first" pattern that a `pull --rebase + push` cycle
// resolves. Conservative — any other failure (auth, network, hook reject) is
// surfaced verbatim so the operator sees the real cause.
func isNonFastForwardRejection(stderr string) bool {
	return strings.Contains(stderr, "[rejected]") ||
		strings.Contains(stderr, "non-fast-forward") ||
		strings.Contains(stderr, "fetch first") ||
		strings.Contains(stderr, "Updates were rejected")
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
