package steps

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/kernel/approval"
	"github.com/optivem/gh-optivem/internal/kernel/log"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
	"github.com/optivem/gh-optivem/internal/kernel/shell"
)

// PreviewOutcome is what the Preview phase decided.
type PreviewOutcome int

const (
	// PreviewProceed: the operator confirmed (or passed --yes); continue to push.
	PreviewProceed PreviewOutcome = iota
	// PreviewDryRun: --dry-run; the caller must exit 0 before any write.
	PreviewDryRun
	// PreviewDeclined: the operator answered no; the caller must abort.
	PreviewDeclined
	// PreviewInactive: no pre-existing repository, so there is nothing to preview.
	PreviewInactive
)

// repoTarget pairs a repository with its local clone dir.
type repoTarget struct {
	FullRepo string
	Dir      string
}

// repoTargets lists every repository init clones, matching CloneRepos.
func repoTargets(cfg *config.Config) []repoTarget {
	out := []repoTarget{{cfg.FullRepo, cfg.RepoDir}}
	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "multitier" {
			out = append(out,
				repoTarget{cfg.FrontendFullRepo, cfg.FrontendRepoDir},
				repoTarget{cfg.BackendFullRepo, cfg.BackendRepoDir})
		} else {
			out = append(out, repoTarget{cfg.SystemFullRepo, cfg.SystemRepoDir})
		}
	}
	return out
}

// scaffoldBranch is the branch init pushes to and whose workflows the verify
// stages watch.
const scaffoldBranch = "main"

// baselineDir is where the pre-scaffold content of dir is parked.
func baselineDir(dir string) string { return dir + ".baseline" }

// repoAccessProbe reads the push permission and default branch of a
// repository. Tests replace it.
var repoAccessProbe = func(fullRepo string) (string, error) {
	return shell.RunCaptureWithRetry(fmt.Sprintf("gh api repos/%s --jq \"[.permissions.push,.default_branch]|@tsv\"", fullRepo), "")
}

// checkRepoAccess fails with a specific cause: 404 (not found / not visible),
// 403 (forbidden), 401 (bad token), no push permission, or any other error.
// These are never folded into one message.
func checkRepoAccess(fullRepo string) error {
	raw, err := repoAccessProbe(fullRepo)
	if err != nil {
		lower := strings.ToLower(err.Error())
		switch {
		case strings.Contains(lower, "http 404"):
			return fmt.Errorf("repository %s was not found (HTTP 404): it existed when init started, so it was deleted meanwhile or the token cannot see it; check the name and the token's repo scope", fullRepo)
		case strings.Contains(lower, "http 403"):
			return fmt.Errorf("access to repository %s is forbidden (HTTP 403): the token lacks permission or the organization restricts it (SSO authorization, IP allow list); fix the token and retry", fullRepo)
		case strings.Contains(lower, "http 401"):
			return fmt.Errorf("repository %s could not be read (HTTP 401): the token is invalid or expired; re-authenticate (gh auth login) and retry", fullRepo)
		}
		return fmt.Errorf("cannot determine access to repository %s: %v; check network connectivity and retry", fullRepo, err)
	}
	fields := strings.Split(strings.TrimSpace(raw), "\t")
	if len(fields) < 2 || fields[1] == "" {
		return fmt.Errorf("unexpected response reading repository %s (no default branch): %q", fullRepo, strings.TrimSpace(raw))
	}
	if fields[1] != scaffoldBranch {
		return fmt.Errorf("repository %s has default branch %q, but init pushes the scaffold to %q and its workflows run there; rename the default branch to %q (or use a repository whose default is %q) and retry", fullRepo, fields[1], scaffoldBranch, scaffoldBranch, scaffoldBranch)
	}
	if fields[0] != "true" {
		return fmt.Errorf("the token has no push access to repository %s; init pushes the scaffold to its default branch %q, so grant write access and retry", fullRepo, fields[1])
	}
	return nil
}

// CaptureExistingBaselines runs right after CloneRepos for every pre-existing
// repository. It checks reachability and push access, requires the fresh clone
// to be clean, then parks the clone's current files in a sibling baseline dir
// and empties the working tree (keeping .git). The template then renders as a
// pure scaffold, which makes the preview exact; RunPreview restores the files
// the scaffold does not touch. A repository with no commits yet gets an empty
// baseline, so it is previewed as a plain fresh scaffold.
func CaptureExistingBaselines(cfg *config.Config) {
	for _, t := range repoTargets(cfg) {
		if !cfg.PreExistingRepos[t.FullRepo] {
			continue
		}
		if err := checkRepoAccess(t.FullRepo); err != nil {
			log.Fatalf("%v", err)
		}
		if err := snapshotClone(t.Dir); err != nil {
			log.Fatalf("%s: %v", t.FullRepo, err)
		}
	}
}

func snapshotClone(dir string) error {
	status, err := shell.RunCapture("git status --porcelain", dir)
	if err != nil {
		return fmt.Errorf("cannot read git status of local clone %s: %v", dir, err)
	}
	if strings.TrimSpace(status) != "" {
		return fmt.Errorf("local clone %s is not clean right after cloning; remove it and re-run so the preview reflects the remote default branch", dir)
	}
	base := baselineDir(dir)
	if err := os.RemoveAll(base); err != nil {
		return fmt.Errorf("clear stale baseline %s: %v", base, err)
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return fmt.Errorf("create baseline %s: %v", base, err)
	}
	if _, err := shell.RunCapture("git rev-parse --verify HEAD", dir); err != nil {
		log.Infof("%s has no commits on its default branch; treating it as a fresh scaffold", dir)
		return nil
	}
	if err := copyTree(dir, base); err != nil {
		return fmt.Errorf("snapshot %s: %v", dir, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name() == ".git" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return fmt.Errorf("empty working tree %s: %v", dir, err)
		}
	}
	return nil
}

// copyTree copies regular files under src to dst, skipping .git.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(src, path)
		if err != nil || rel == "." {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return copyFileWithMode(path, target)
	})
}

func copyFileWithMode(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, info.Mode().Perm())
}

// RepoPreview is the file-level preview of one pre-existing repository.
type RepoPreview struct {
	FullRepo string
	// Dir holds the scaffold; Baseline holds the repository's existing files.
	Dir      string
	Baseline string
	Entries  []PreviewEntry
}

func (p RepoPreview) count(c PathClass) int {
	n := 0
	for _, e := range p.Entries {
		if e.Class == c {
			n++
		}
	}
	return n
}

// PreviewIO carries what the confirmation needs from the environment.
type PreviewIO struct {
	In  io.Reader
	Out io.Writer
	// Interactive reports whether stdin is a terminal.
	Interactive bool
}

// DefaultPreviewIO is the real terminal.
func DefaultPreviewIO() PreviewIO {
	interactive := false
	if fi, err := os.Stdin.Stat(); err == nil {
		interactive = fi.Mode()&os.ModeCharDevice != 0
	}
	return PreviewIO{In: os.Stdin, Out: os.Stdout, Interactive: interactive}
}

// RunPreview builds the file and remote previews, prints them, and decides
// whether to continue. Nothing remote is written before it returns. On
// PreviewProceed the files the scaffold does not touch are restored into the
// working trees so the commit contains scaffold plus untouched files.
func RunPreview(cfg *config.Config, pc *projectconfig.Config, sc *shell.SonarCloud, pio PreviewIO) (PreviewOutcome, error) {
	if len(cfg.PreExistingRepos) == 0 {
		return PreviewInactive, nil
	}

	var repos []RepoPreview
	for _, t := range repoTargets(cfg) {
		if !cfg.PreExistingRepos[t.FullRepo] {
			continue
		}
		base := baselineDir(t.Dir)
		if _, err := os.Stat(base); err != nil {
			return PreviewDeclined, fmt.Errorf("no baseline for %s at %s (capture step did not run): %v", t.FullRepo, base, err)
		}
		entries, err := BuildPreview(t.Dir, base)
		if err != nil {
			return PreviewDeclined, fmt.Errorf("preview %s: %w", t.FullRepo, err)
		}
		repos = append(repos, RepoPreview{FullRepo: t.FullRepo, Dir: t.Dir, Baseline: base, Entries: entries})
	}

	targets := RemoteTargets{SonarProjectKeys: sonarProjectKeysFromConfig(pc)}
	if pc != nil {
		targets.SonarOrg = pc.Sonar.Organization
	}
	if sc != nil {
		targets.Sonar = sc
	}
	remote, err := ProbeRemote(cfg, targets)
	if err != nil {
		return PreviewDeclined, err
	}

	RenderPreview(pio.Out, repos, remote, cfg.Verbose)

	if cfg.DryRun {
		fmt.Fprintln(pio.Out, "\n--dry-run: nothing was written. Exiting.")
		return PreviewDryRun, nil
	}

	switch {
	case cfg.AssumeYes:
		fmt.Fprintln(pio.Out, "\n--yes: proceeding without confirmation.")
	case !pio.Interactive:
		return PreviewDeclined, errors.New("init into existing repositories needs confirmation but stdin is not a terminal: review the preview above (or run with --dry-run), then re-run with --yes to apply it unattended")
	default:
		ok, err := approval.Confirm(cfg.Approval, approval.CategoryHuman, pio.In, pio.Out, "\nApply these changes to the existing repositories?")
		if err != nil {
			return PreviewDeclined, fmt.Errorf("read confirmation: %w", err)
		}
		if !ok {
			return PreviewDeclined, nil
		}
	}

	for _, r := range repos {
		if err := restoreKept(r); err != nil {
			return PreviewDeclined, err
		}
	}
	return PreviewProceed, nil
}

// restoreKept copies the repository's KEPT files from the baseline back into
// the working tree.
func restoreKept(r RepoPreview) error {
	for _, e := range r.Entries {
		if e.Class != ClassKept {
			continue
		}
		rel := filepath.FromSlash(e.Path)
		dst := filepath.Join(r.Dir, rel)
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if err := copyFileWithMode(filepath.Join(r.Baseline, rel), dst); err != nil {
			return fmt.Errorf("restore kept file %s of %s: %w", e.Path, r.FullRepo, err)
		}
	}
	return nil
}

// RenderPreview prints the summary counts, the OVERWRITE list prominently, and
// the remote resource classes. With verbose it also lists NEW paths and prints
// the full diff of every OVERWRITE text file. Secret values are never printed:
// RemoteEntry.Detail carries none.
func RenderPreview(w io.Writer, repos []RepoPreview, remote []RemoteEntry, verbose bool) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, strings.Repeat("=", 70))
	fmt.Fprintln(w, "  Preview: changes to existing repositories")
	fmt.Fprintln(w, strings.Repeat("=", 70))

	for _, r := range repos {
		renderRepoPreview(w, r, verbose)
	}

	renderRemote(w, remote)
}

// renderRepoPreview prints one repository's section of the preview.
func renderRepoPreview(w io.Writer, r RepoPreview, verbose bool) {
	fmt.Fprintf(w, "\n%s\n", r.FullRepo)
	fmt.Fprintf(w, "  %d new, %d identical, %d OVERWRITE, %d kept (untouched)\n",
		r.count(ClassNew), r.count(ClassIdentical), r.count(ClassOverwrite), r.count(ClassKept))

	renderOverwrites(w, r)
	renderCaseNotes(w, r)
	if verbose {
		renderVerboseDetail(w, r)
	}
}

// renderOverwrites prints the OVERWRITE list, or a reassurance line when
// nothing is overwritten.
func renderOverwrites(w io.Writer, r RepoPreview) {
	n := r.count(ClassOverwrite)
	if n == 0 {
		fmt.Fprintln(w, "  No existing file will be overwritten.")
		return
	}
	fmt.Fprintf(w, "\n  !! %d file(s) will be OVERWRITTEN; git history is the only undo:\n", n)
	for _, e := range r.Entries {
		if e.Class == ClassOverwrite {
			fmt.Fprintf(w, "    OVERWRITE  %s  %s\n", e.Path, overwriteSize(e))
		}
	}
}

// renderCaseNotes prints a note for every entry whose repo spelling differs
// only in case.
func renderCaseNotes(w io.Writer, r RepoPreview) {
	for _, e := range r.Entries {
		if e.ExistingPath != "" {
			fmt.Fprintf(w, "  note: %q is spelled %q in the repo (case-only difference)\n", e.Path, e.ExistingPath)
		}
	}
}

// renderVerboseDetail lists NEW paths and prints the diff of every OVERWRITE
// text file.
func renderVerboseDetail(w io.Writer, r RepoPreview) {
	for _, e := range r.Entries {
		if e.Class == ClassNew {
			fmt.Fprintf(w, "    NEW        %s\n", e.Path)
		}
	}
	for _, e := range r.Entries {
		if e.Class == ClassOverwrite && !e.Binary {
			fmt.Fprintf(w, "\n  --- diff: %s ---\n%s", e.Path, fileDiff(r, e))
		}
	}
}

func overwriteSize(e PreviewEntry) string {
	if e.Binary {
		return "(binary)"
	}
	return fmt.Sprintf("(+%d -%d)", e.Added, e.Removed)
}

func renderRemote(w io.Writer, remote []RemoteEntry) {
	fmt.Fprintln(w, "\nRemote resources")
	counts := map[RemoteClass]int{}
	for _, e := range remote {
		counts[e.Class]++
	}
	fmt.Fprintf(w, "  %d create, %d reuse, %d differs\n", counts[RemoteCreate], counts[RemoteReuse], counts[RemoteDiffers])

	sorted := append([]RemoteEntry(nil), remote...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Class < sorted[j].Class })
	for _, e := range sorted {
		if e.Class == RemoteReuse {
			continue
		}
		name := e.Name
		if e.Repo != "" {
			name = e.Repo + " " + e.Name
		}
		line := fmt.Sprintf("    %-8s %s %s", e.Class, e.Kind, name)
		if e.Detail != "" {
			line += "  -- " + e.Detail
		}
		fmt.Fprintln(w, line)
	}
}

// diffRunner returns the unified diff of two files. Tests replace it.
var diffRunner = func(oldPath, newPath string) string {
	out, _ := exec.Command("git", "diff", "--no-index", "--no-color", "--ignore-cr-at-eol", "--", oldPath, newPath).CombinedOutput()
	return string(out)
}

func fileDiff(r RepoPreview, e PreviewEntry) string {
	existing := e.Path
	if e.ExistingPath != "" {
		existing = e.ExistingPath
	}
	d := diffRunner(filepath.Join(r.Baseline, filepath.FromSlash(existing)), filepath.Join(r.Dir, filepath.FromSlash(e.Path)))
	if !strings.HasSuffix(d, "\n") {
		d += "\n"
	}
	return d
}
