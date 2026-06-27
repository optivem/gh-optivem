// Package ghbulk ports the paginated bulk-delete helpers from
// github-utils/scripts/{delete-releases,delete-packages,delete-repos}.sh.
// It exposes thin iterators over the GitHub REST API plus matching delete
// helpers; rate-limit guarding and gh retry happen via internal/shell.
//
// Each delete helper honours Options.DryRun internally — callers do not
// branch on it, they just hand the helper a fully-resolved target.
package ghbulk

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/optivem/gh-optivem/internal/kernel/log"
	"github.com/optivem/gh-optivem/internal/kernel/shell"
)

const (
	defaultPageSize            = 100
	defaultDelayBetweenDeletes = 10 * time.Second
)

// Options configures a bulk run. The zero value is valid and uses defaults
// matched to the bash scripts: 100-per-page listing, 10s sleep between
// destructive calls.
type Options struct {
	// DryRun prints what would be deleted but performs no destructive call.
	DryRun bool
	// PageSize is the per_page parameter for paginated listings. Zero ⇒ 100.
	PageSize int
	// DelayBetweenDeletes is the sleep after each destructive call to stay
	// under GitHub's 80-mutating-calls/min secondary limit. Zero ⇒ 10s.
	DelayBetweenDeletes time.Duration
	// BeforeDate, if non-zero, filters listings to items strictly older than
	// the given time (mirrors BEFORE_DATE in the bash scripts).
	BeforeDate time.Time
}

func (o Options) pageSize() int {
	if o.PageSize <= 0 {
		return defaultPageSize
	}
	return o.PageSize
}

func (o Options) delay() time.Duration {
	if o.DelayBetweenDeletes <= 0 {
		return defaultDelayBetweenDeletes
	}
	return o.DelayBetweenDeletes
}

// passesBeforeDate reports whether t is strictly before opt.BeforeDate, or
// true when BeforeDate is unset (no filter).
func (o Options) passesBeforeDate(t time.Time) bool {
	if o.BeforeDate.IsZero() {
		return true
	}
	return t.Before(o.BeforeDate)
}

// ── Releases ────────────────────────────────────────────────────────────

// Release is the subset of fields the cleanup commands consume.
type Release struct {
	ID        int64     `json:"id"`
	TagName   string    `json:"tag_name"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// ForEachRelease iterates every release of owner/repo (paginated) and calls
// fn for each release that passes the BeforeDate filter. fn is invoked even
// when opt.DryRun is true; callers typically pass DeleteRelease which itself
// short-circuits in dry-run mode. Iteration stops on the first error.
func ForEachRelease(owner, repo string, opt Options, fn func(Release) error) error {
	for page := 1; ; page++ {
		shell.CheckRateLimit()
		path := fmt.Sprintf("repos/%s/%s/releases?per_page=%d&page=%d",
			owner, repo, opt.pageSize(), page)
		var releases []Release
		if err := apiGet(path, &releases); err != nil {
			return fmt.Errorf("list releases for %s/%s page %d: %w", owner, repo, page, err)
		}
		if len(releases) == 0 {
			return nil
		}
		for _, r := range releases {
			if !opt.passesBeforeDate(r.CreatedAt) {
				log.Infof("  Skipping: %s (created: %s, on or after %s)",
					r.Name, r.CreatedAt.Format(time.RFC3339), opt.BeforeDate.Format("2006-01-02"))
				continue
			}
			if err := fn(r); err != nil {
				return err
			}
		}
		if len(releases) < opt.pageSize() {
			return nil
		}
	}
}

// DeleteRelease removes the release (DELETE repos/owner/repo/releases/{id})
// and its associated git tag (DELETE repos/owner/repo/git/refs/tags/{tag}).
// Honours opt.DryRun. Sleeps opt.DelayBetweenDeletes after the destructive
// pair lands.
func DeleteRelease(owner, repo string, rel Release, opt Options) error {
	if opt.DryRun {
		fmt.Printf("  [DRY RUN] Would delete: %s (tag: %s, created: %s)\n",
			rel.Name, rel.TagName, rel.CreatedAt.Format(time.RFC3339))
		return nil
	}
	fmt.Printf("  Deleting release: %s (tag: %s)...\n", rel.Name, rel.TagName)

	shell.CheckRateLimit()
	if err := apiDelete(fmt.Sprintf("repos/%s/%s/releases/%d", owner, repo, rel.ID)); err != nil {
		return fmt.Errorf("delete release %d: %w", rel.ID, err)
	}
	fmt.Println("    ✓ Release deleted")

	shell.CheckRateLimit()
	if err := apiDelete(fmt.Sprintf("repos/%s/%s/git/refs/tags/%s", owner, repo, rel.TagName)); err != nil {
		return fmt.Errorf("delete tag %s: %w", rel.TagName, err)
	}
	fmt.Println("    ✓ Tag deleted")

	sleepFn(opt.delay())
	return nil
}

// ── Packages ────────────────────────────────────────────────────────────

// Package is the subset of fields the cleanup commands consume.
type Package struct {
	Name        string    `json:"name"`
	PackageType string    `json:"package_type"`
	Visibility  string    `json:"visibility"`
	CreatedAt   time.Time `json:"created_at"`
	Repository  struct {
		Name string `json:"name"`
	} `json:"repository"`
}

// PackageTypes are the GitHub package_type values delete-packages.sh
// enumerates. GitHub's package listing endpoint requires a package_type
// parameter, so the loop fans out across the supported types.
var PackageTypes = []string{"npm", "maven", "docker", "nuget", "rubygems", "container"}

// ForEachPackage iterates every package owned by `owner` whose
// repository.name == repoName, across all PackageTypes. fn is invoked for
// each package; iteration stops on the first error.
func ForEachPackage(owner, repoName string, opt Options, fn func(Package) error) error {
	ownerType, err := OwnerType(owner)
	if err != nil {
		return fmt.Errorf("detect owner type for %s: %w", owner, err)
	}
	for _, pkgType := range PackageTypes {
		for page := 1; ; page++ {
			shell.CheckRateLimit()
			path := fmt.Sprintf("%s/%s/packages?package_type=%s&per_page=%d&page=%d",
				ownerType, owner, pkgType, opt.pageSize(), page)
			var packages []Package
			// A 404 here means "no packages of this type" — common, not an error.
			err := apiGet(path, &packages)
			if err != nil {
				if isNotFound(err) {
					break
				}
				return fmt.Errorf("list packages %s page %d: %w", pkgType, page, err)
			}
			if len(packages) == 0 {
				break
			}
			matched := 0
			for _, p := range packages {
				if p.Repository.Name != repoName {
					continue
				}
				if !opt.passesBeforeDate(p.CreatedAt) {
					log.Infof("    Skipping: %s (on or after %s)",
						p.Name, opt.BeforeDate.Format("2006-01-02"))
					continue
				}
				matched++
				if err := fn(p); err != nil {
					return err
				}
			}
			if len(packages) < opt.pageSize() {
				break
			}
			_ = matched
		}
	}
	return nil
}

// DeletePackage removes the package in one call (DELETE
// /{owner_type}/{owner}/packages/{type}/{name}). Public packages cannot be
// deleted via the API — they must be made private via the GitHub UI first,
// so DeletePackage refuses to act on them and prints the settings URL the
// operator needs.
//
// Returns the count contribution for the caller's summary: (1,0) when a
// delete landed, (0,1) when skipped public, (0,0) on dry-run.
func DeletePackage(owner, ownerType string, p Package, opt Options) (deleted, skipped int, err error) {
	encoded := url.PathEscape(p.Name)
	if p.Visibility == "public" {
		fmt.Println("    ⚠️  SKIPPED: Package is public. Make it private first via GitHub UI:")
		fmt.Printf("       https://github.com/orgs/%s/packages/%s/%s/settings\n",
			owner, p.PackageType, encoded)
		return 0, 1, nil
	}
	if opt.DryRun {
		fmt.Printf("    [DRY RUN] Would delete package: %s\n", p.Name)
		return 0, 0, nil
	}
	fmt.Println("    Deleting package...")
	shell.CheckRateLimit()
	if err := apiDelete(fmt.Sprintf("%s/%s/packages/%s/%s",
		ownerType, owner, p.PackageType, encoded)); err != nil {
		return 0, 0, fmt.Errorf("delete package %s: %w", p.Name, err)
	}
	fmt.Println("      ✓ Package deleted")
	sleepFn(opt.delay())
	return 1, 0, nil
}

// ── Repos ───────────────────────────────────────────────────────────────

// Repo is the subset of fields the repo cleanup consumes.
type Repo struct {
	Name string `json:"name"`
}

// ForEachRepoByPrefix iterates every repo owned by `owner` (paginated)
// whose name starts with `prefix`. Used by `cleanup repos --prefix`.
func ForEachRepoByPrefix(owner, prefix string, opt Options, fn func(Repo) error) error {
	ownerType, err := OwnerType(owner)
	if err != nil {
		return fmt.Errorf("detect owner type for %s: %w", owner, err)
	}
	for page := 1; ; page++ {
		shell.CheckRateLimit()
		path := fmt.Sprintf("%s/%s/repos?per_page=%d&page=%d&type=owner",
			ownerType, owner, opt.pageSize(), page)
		var repos []Repo
		if err := apiGet(path, &repos); err != nil {
			return fmt.Errorf("list repos page %d: %w", page, err)
		}
		if len(repos) == 0 {
			return nil
		}
		if err := processRepoPage(repos, prefix, fn); err != nil {
			return err
		}
		if len(repos) < opt.pageSize() {
			return nil
		}
	}
}

func processRepoPage(repos []Repo, prefix string, fn func(Repo) error) error {
	for _, r := range repos {
		if !strings.HasPrefix(r.Name, prefix) {
			continue
		}
		if err := fn(r); err != nil {
			return err
		}
	}
	return nil
}

// DeleteRepo removes a repository in one call (DELETE /repos/owner/name).
// Honours opt.DryRun.
func DeleteRepo(owner, name string, opt Options) error {
	full := owner + "/" + name
	if opt.DryRun {
		fmt.Printf("  [DRY RUN] Would delete: %s\n", full)
		return nil
	}
	fmt.Printf("  Deleting: %s ...\n", full)
	shell.CheckRateLimit()
	if err := apiDelete("repos/" + full); err != nil {
		return fmt.Errorf("delete repo %s: %w", full, err)
	}
	fmt.Println("    ✓ Deleted")
	sleepFn(opt.delay())
	return nil
}

// ── Owner type detection ────────────────────────────────────────────────

// OwnerType reports whether `owner` is a user or an organization, returning
// the segment ("users" or "orgs") that the packages and repos REST endpoints
// expect in their path. Mirrors get_owner_type in delete-packages.sh.
func OwnerType(owner string) (string, error) {
	var data struct {
		Type string `json:"type"`
	}
	if err := apiGet("users/"+owner, &data); err != nil {
		// Unknown owner; the caller will hit a 404 on the listing call anyway.
		// Default to "users" — same fallback as the bash script.
		return "users", nil
	}
	if data.Type == "Organization" {
		return "orgs", nil
	}
	return "users", nil
}

// ── Slug parsing ────────────────────────────────────────────────────────

// ParseSlug splits "owner/repo" and rejects bare names or extra segments.
// The bash scripts accepted whatever the caller passed and let the API
// produce a 404; the Go port rejects malformed slugs at the argument-parser
// layer so a typo surfaces immediately.
func ParseSlug(s string) (owner, repo string, err error) {
	parts := strings.Split(s, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("expected owner/repo, got %q", s)
	}
	return parts[0], parts[1], nil
}

// ── Internal helpers ────────────────────────────────────────────────────

// sleepFn is package-level so tests can replace it with a no-op.
var sleepFn = time.Sleep

// notFoundError carries a 404-classified error from apiGet so callers can
// distinguish "endpoint doesn't exist for this owner" from real failures.
type notFoundError struct{ msg string }

func (e *notFoundError) Error() string { return e.msg }

func isNotFound(err error) bool {
	var nfe *notFoundError
	return errors.As(err, &nfe)
}

// apiGet runs `gh api <path>` (with retry) and unmarshals the result into
// dst. A response containing "HTTP 404" is wrapped as notFoundError so the
// packages iterator can swallow it.
func apiGet(path string, dst any) error {
	cmd := "gh api " + path
	out, err := shell.RunWithRetry(cmd, true, "")
	if err != nil {
		if strings.Contains(out, "HTTP 404") || strings.Contains(out, "Not Found") {
			return &notFoundError{msg: err.Error()}
		}
		return err
	}
	if strings.TrimSpace(out) == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(out), dst); err != nil {
		return fmt.Errorf("parse response from %s: %w", path, err)
	}
	return nil
}

// apiDelete runs `gh api -X DELETE <path>` (with retry) and returns any
// error verbatim. Output is discarded because DELETE responses are empty.
func apiDelete(path string) error {
	_, err := shell.RunWithRetry("gh api -X DELETE "+path, true, "")
	return err
}
