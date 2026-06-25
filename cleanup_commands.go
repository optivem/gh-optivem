// cleanup_commands.go wires the `gh optivem cleanup <verb>` subtree.
// The cleanup noun spans bulk-destructive operations against remote
// systems (GitHub releases/packages/repos, SonarCloud projects). The
// Go ports replace the bash scripts in academy/github-utils/scripts/ —
// see plans/20260514-0914-migrate-workspace-scripts-to-gh-optivem.md.
//
// The DRY_RUN env var idiom from bash becomes a `--dry-run` flag here
// (consistent with the rest of gh-optivem). Each subcommand pre-flights
// `gh api rate_limit` before each destructive call via internal/shell.
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/devworkflow/ghbulk"
	"github.com/optivem/gh-optivem/internal/devworkflow/sonar"
)

const cleanupSeparator = "============================================"

// cleanupRule is the per-repo banner rule (41 chars), distinct from the wider
// cleanupSeparator. Kept verbatim to preserve existing output width.
const cleanupRule = "========================================="

// dryRunModeLine is the banner line shown when a cleanup runs in dry-run mode.
const dryRunModeLine = "  Mode: DRY RUN (no changes will be made)"

// newCleanupCmd builds the `gh optivem cleanup` parent. The parent has no
// Run, so invoking it without a subcommand prints help.
func newCleanupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Bulk-delete remote artifacts (releases, packages, repos, SonarCloud projects)",
		Long: `Destructive operations against remote systems. Pass --dry-run first to
preview what would be deleted. GitHub-side subcommands pre-flight the
GitHub rate limit before each destructive call; the SonarCloud subcommand
applies the --delay-seconds throttle between deletes.`,
	}
	cmd.AddCommand(
		newCleanupReleasesCmd(),
		newCleanupPackagesCmd(),
		newCleanupReposCmd(),
		newCleanupSonarProjectsCmd(),
	)
	return cmd
}

// ── shared flag bindings ────────────────────────────────────────────────

// commonFlags holds the flags every cleanup subcommand exposes. Bound by
// bindCommonFlags and decoded into ghbulk.Options via toOptions.
type commonFlags struct {
	DryRun     bool
	BeforeDate string
	DelaySecs  int
}

func bindCommonFlags(cmd *cobra.Command, f *commonFlags) {
	cmd.Flags().BoolVar(&f.DryRun, "dry-run", false, "Print what would be deleted; do not delete")
	cmd.Flags().StringVar(&f.BeforeDate, "before-date", "",
		"Only delete items created before this date (YYYY-MM-DD, exclusive)")
	cmd.Flags().IntVar(&f.DelaySecs, "delay-seconds", 10,
		"Sleep this many seconds after each destructive call (rate-limit guard)")
}

// toOptions decodes commonFlags into ghbulk.Options or surfaces a validation
// error matching the bash scripts' wording.
func (f commonFlags) toOptions() (ghbulk.Options, error) {
	opt := ghbulk.Options{
		DryRun:              f.DryRun,
		DelayBetweenDeletes: time.Duration(f.DelaySecs) * time.Second,
	}
	if f.BeforeDate != "" {
		t, err := time.Parse("2006-01-02", f.BeforeDate)
		if err != nil {
			return ghbulk.Options{}, fmt.Errorf("invalid --before-date %q: expected YYYY-MM-DD", f.BeforeDate)
		}
		opt.BeforeDate = t
	}
	return opt, nil
}

// ── releases ────────────────────────────────────────────────────────────

func newCleanupReleasesCmd() *cobra.Command {
	var flags commonFlags
	cmd := &cobra.Command{
		Use:   "releases <owner/repo> [<owner/repo>...]",
		Short: "Bulk-delete every GitHub release (and its git tag) for the given repos",
		Long: `Iterate every release of each owner/repo and delete it, along with the
associated git tag. Each deletion = 2 mutating calls (release + tag), so the
default 10s delay keeps you under GitHub's 80-mutating-calls/min secondary
limit.`,
		Example: `  gh optivem cleanup releases myorg/myrepo --dry-run
  gh optivem cleanup releases myorg/repo-a myorg/repo-b
  gh optivem cleanup releases myorg/myrepo --before-date 2026-01-01`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opt, err := flags.toOptions()
			if err != nil {
				return err
			}
			slugs, err := parseSlugs(args)
			if err != nil {
				return err
			}
			return runCleanupReleases(slugs, opt)
		},
	}
	bindCommonFlags(cmd, &flags)
	return cmd
}

func runCleanupReleases(slugs []slug, opt ghbulk.Options) error {
	printHeader("GitHub Release Cleanup", slugs, opt)
	for i, s := range slugs {
		fmt.Println()
		fmt.Println(cleanupRule)
		fmt.Printf("  Processing: %s/%s\n", s.Owner, s.Repo)
		fmt.Println(cleanupRule)

		deleted := 0
		err := ghbulk.ForEachRelease(s.Owner, s.Repo, opt, func(rel ghbulk.Release) error {
			if err := ghbulk.DeleteRelease(s.Owner, s.Repo, rel, opt); err != nil {
				return err
			}
			if !opt.DryRun {
				deleted++
			}
			return nil
		})
		if err != nil {
			return err
		}
		if !opt.DryRun {
			fmt.Printf("  Done. Deleted %d releases from %s/%s.\n", deleted, s.Owner, s.Repo)
		}
		if i < len(slugs)-1 {
			time.Sleep(opt.DelayBetweenDeletes)
		}
	}
	fmt.Println()
	fmt.Println("✅ All done!")
	return nil
}

// ── packages ────────────────────────────────────────────────────────────

func newCleanupPackagesCmd() *cobra.Command {
	var flags commonFlags
	cmd := &cobra.Command{
		Use:   "packages <owner/repo> [<owner/repo>...]",
		Short: "Bulk-delete private GitHub packages linked to the given repos",
		Long: `Iterate every package owned by each repo's owner (across npm, maven,
docker, nuget, rubygems, container) and delete those whose repository
matches. Public packages are skipped — they must be made private via the
GitHub UI first, and the URL to do so is printed.`,
		Example: `  gh optivem cleanup packages myorg/myrepo --dry-run
  gh optivem cleanup packages myorg/repo-a myorg/repo-b
  gh optivem cleanup packages myorg/myrepo --before-date 2026-01-01`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opt, err := flags.toOptions()
			if err != nil {
				return err
			}
			slugs, err := parseSlugs(args)
			if err != nil {
				return err
			}
			return runCleanupPackages(slugs, opt)
		},
	}
	bindCommonFlags(cmd, &flags)
	return cmd
}

func runCleanupPackages(slugs []slug, opt ghbulk.Options) error {
	printHeader("GitHub Package Cleanup", slugs, opt)
	for i, s := range slugs {
		fmt.Println()
		fmt.Println(cleanupRule)
		fmt.Printf("  Processing: %s/%s\n", s.Owner, s.Repo)
		fmt.Println(cleanupRule)

		ownerType, err := ghbulk.OwnerType(s.Owner)
		if err != nil {
			return err
		}

		deleted, skipped := 0, 0
		err = ghbulk.ForEachPackage(s.Owner, s.Repo, opt, func(p ghbulk.Package) error {
			fmt.Println()
			fmt.Printf("    Package: %s (type: %s, visibility: %s, created: %s)\n",
				p.Name, p.PackageType, p.Visibility, p.CreatedAt.Format(time.RFC3339))
			d, sk, err := ghbulk.DeletePackage(s.Owner, ownerType, p, opt)
			deleted += d
			skipped += sk
			return err
		})
		if err != nil {
			return err
		}
		if !opt.DryRun {
			if deleted == 0 && skipped == 0 {
				fmt.Println("  No packages found.")
			} else {
				fmt.Printf("  Done. Deleted %d, skipped %d (public) packages from %s/%s.\n",
					deleted, skipped, s.Owner, s.Repo)
			}
		}
		if i < len(slugs)-1 {
			time.Sleep(opt.DelayBetweenDeletes)
		}
	}
	fmt.Println()
	fmt.Println("✅ All done!")
	return nil
}

// ── repos ───────────────────────────────────────────────────────────────

// repoFlags adds --prefix on top of commonFlags. The repos cleanup takes a
// single positional <owner> plus either --prefix, explicit names, or both
// (matching delete-repos.sh).
type repoFlags struct {
	commonFlags
	Prefix string
}

func newCleanupReposCmd() *cobra.Command {
	var flags repoFlags
	cmd := &cobra.Command{
		Use:   "repos <owner> [--prefix <prefix>] [<repo-name>...]",
		Short: "Bulk-delete GitHub repos under <owner> by prefix and/or explicit names",
		Long: `Delete repos owned by <owner>. At least one of --prefix or explicit names
must be provided. --prefix matches the start of each repo name; explicit
names are deleted unconditionally (no prefix check). Both can be combined.`,
		Example: `  gh optivem cleanup repos myorg --prefix course-tester- --dry-run
  gh optivem cleanup repos myorg repo-a repo-b
  gh optivem cleanup repos myorg --prefix temp- specific-repo`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			owner := args[0]
			names := args[1:]
			if flags.Prefix == "" && len(names) == 0 {
				return errors.New("provide --prefix, explicit repo names, or both")
			}
			opt, err := flags.commonFlags.toOptions()
			if err != nil {
				return err
			}
			return runCleanupRepos(owner, flags.Prefix, names, opt)
		},
	}
	bindCommonFlags(cmd, &flags.commonFlags)
	cmd.Flags().StringVar(&flags.Prefix, "prefix", "", "Delete all repos whose names start with prefix")
	return cmd
}

func runCleanupRepos(owner, prefix string, explicit []string, opt ghbulk.Options) error {
	fmt.Println(cleanupSeparator)
	fmt.Println("  GitHub Repo Cleanup")
	fmt.Printf("  Owner:  %s\n", owner)
	if prefix != "" {
		fmt.Printf("  Prefix: %s\n", prefix)
	}
	if len(explicit) > 0 {
		fmt.Printf("  Repos:  %s\n", strings.Join(explicit, " "))
	}
	if opt.DryRun {
		fmt.Println(dryRunModeLine)
	}
	fmt.Println(cleanupSeparator)

	deleted := 0
	if prefix != "" {
		foundAny := false
		err := ghbulk.ForEachRepoByPrefix(owner, prefix, opt, func(r ghbulk.Repo) error {
			foundAny = true
			if err := ghbulk.DeleteRepo(owner, r.Name, opt); err != nil {
				return err
			}
			if !opt.DryRun {
				deleted++
			}
			return nil
		})
		if err != nil {
			return err
		}
		if !foundAny {
			fmt.Printf("  No repos found matching prefix '%s' under %s.\n", prefix, owner)
		}
	}
	for _, name := range explicit {
		if err := ghbulk.DeleteRepo(owner, name, opt); err != nil {
			return err
		}
		if !opt.DryRun {
			deleted++
		}
	}

	fmt.Println()
	if opt.DryRun {
		fmt.Println("✅ Dry run complete. Re-run without --dry-run to delete.")
	} else {
		fmt.Printf("✅ Done. Deleted %d repo(s).\n", deleted)
	}
	return nil
}

// ── sonar-projects ──────────────────────────────────────────────────────

type sonarFlags struct {
	commonFlags
	Prefix string
}

func newCleanupSonarProjectsCmd() *cobra.Command {
	var flags sonarFlags
	cmd := &cobra.Command{
		Use:   "sonar-projects <organization> [--prefix <prefix>] [<project-key>...]",
		Short: "Bulk-delete SonarCloud projects under <organization> by prefix and/or explicit keys",
		Long: `Delete SonarCloud projects in <organization>. At least one of --prefix
or explicit project keys must be provided. Requires SONAR_TOKEN in the
environment (the same env var the scaffolder uses). The API base URL can
be overridden with SONAR_API_URL.`,
		Example: `  gh optivem cleanup sonar-projects myorg --prefix myorg_course-tester- --dry-run
  gh optivem cleanup sonar-projects myorg myorg_repo-a myorg_repo-b
  gh optivem cleanup sonar-projects myorg --prefix myorg_temp- myorg_specific-project`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org := args[0]
			keys := args[1:]
			if flags.Prefix == "" && len(keys) == 0 {
				return errors.New("provide --prefix, explicit project keys, or both")
			}
			token := os.Getenv("SONAR_TOKEN")
			if token == "" {
				return errors.New("SONAR_TOKEN is not set\n       Generate a token at https://sonarcloud.io/account/security")
			}
			opt, err := flags.commonFlags.toOptions()
			if err != nil {
				return err
			}
			client := sonar.NewClient(os.Getenv("SONAR_API_URL"), token)
			return runCleanupSonarProjects(client, org, flags.Prefix, keys, opt)
		},
	}
	bindCommonFlags(cmd, &flags.commonFlags)
	cmd.Flags().StringVar(&flags.Prefix, "prefix", "", "Delete all projects whose keys start with prefix")
	return cmd
}

func runCleanupSonarProjects(client *sonar.Client, organization, prefix string, explicit []string, opt ghbulk.Options) error {
	fmt.Println(cleanupSeparator)
	fmt.Println("  SonarCloud Project Cleanup")
	fmt.Printf("  Organization: %s\n", organization)
	if prefix != "" {
		fmt.Printf("  Prefix:       %s\n", prefix)
	}
	if len(explicit) > 0 {
		fmt.Printf("  Projects:     %s\n", strings.Join(explicit, " "))
	}
	if opt.DryRun {
		fmt.Println(dryRunModeLine)
	}
	fmt.Println(cleanupSeparator)

	pageSize := 100
	deleted := 0

	deleteOne := func(key string) error {
		if opt.DryRun {
			fmt.Printf("  [DRY RUN] Would delete: %s\n", key)
			return nil
		}
		fmt.Printf("  Deleting: %s ...\n", key)
		if err := client.DeleteProject(key); err != nil {
			return err
		}
		fmt.Println("    ✓ Deleted")
		deleted++
		time.Sleep(opt.DelayBetweenDeletes)
		return nil
	}

	if prefix != "" {
		foundAny := false
		for page := 1; ; page++ {
			resp, err := client.SearchProjects(organization, page, pageSize)
			if err != nil {
				return err
			}
			matched := 0
			for _, p := range resp.Components {
				if !strings.HasPrefix(p.Key, prefix) {
					continue
				}
				matched++
				foundAny = true
				if err := deleteOne(p.Key); err != nil {
					return err
				}
			}
			maxPage := sonar.MaxPage(resp.Paging.Total, pageSize)
			if page >= maxPage {
				break
			}
			_ = matched
		}
		if !foundAny {
			fmt.Printf("  No projects found matching prefix '%s' under %s.\n", prefix, organization)
		}
	}
	for _, key := range explicit {
		if err := deleteOne(key); err != nil {
			return err
		}
	}

	fmt.Println()
	if opt.DryRun {
		fmt.Println("✅ Dry run complete. Re-run without --dry-run to delete.")
	} else {
		fmt.Printf("✅ Done. Deleted %d project(s).\n", deleted)
	}
	return nil
}

// ── helpers ─────────────────────────────────────────────────────────────

type slug struct{ Owner, Repo string }

// parseSlugs validates every positional arg as owner/repo. Returns the full
// list so a single bad slug rejects the whole batch — matching the bash
// scripts' fail-fast set -e behaviour.
func parseSlugs(args []string) ([]slug, error) {
	slugs := make([]slug, 0, len(args))
	for _, a := range args {
		owner, repo, err := ghbulk.ParseSlug(a)
		if err != nil {
			return nil, err
		}
		slugs = append(slugs, slug{Owner: owner, Repo: repo})
	}
	return slugs, nil
}

// printHeader renders the cleanup-script banner. Shared between releases
// and packages because their headers are identical apart from the title.
func printHeader(title string, slugs []slug, opt ghbulk.Options) {
	fmt.Println(cleanupSeparator)
	fmt.Printf("  %s\n", title)
	names := make([]string, len(slugs))
	for i, s := range slugs {
		names[i] = s.Owner + "/" + s.Repo
	}
	fmt.Printf("  Repos: %s\n", strings.Join(names, " "))
	if !opt.BeforeDate.IsZero() {
		fmt.Printf("  Filter: items created before %s (exclusive)\n", opt.BeforeDate.Format("2006-01-02"))
	}
	if opt.DryRun {
		fmt.Println(dryRunModeLine)
	}
	fmt.Println(cleanupSeparator)
}
