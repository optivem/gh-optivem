package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/repolocator"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// System-code safety gate of the characterize (legacy baseline) flow (plan
// 20261110-1130, section 2c). The baseline never changes the system under
// test, so record-start-commits pins a start commit per system repo once, and
// check-system-unchanged diffs the system paths against it after every layer
// commit; check-no-assertion-removed guards the "never weaken an assertion"
// rule over the test paths. All deterministic — no agent. A failed check is a
// gateway outcome routed to an error end (no fix agent); a git failure is a
// hard error, never "unchanged".

// State keys written by the safety-gate actions. Values are strings so they
// survive the resume trailer round-trip (persisted state is coerced to
// strings).
const (
	// CtxKeySystemStartCommits is the JSON list of systemRepoBaseline: one
	// entry per repo holding system paths, with its start commit and paths.
	CtxKeySystemStartCommits = "system-start-commits"
	// CtxKeyStandInPaths is the newline-joined stub/simulator paths from the
	// external-systems: registry: the stand-ins the baseline may write.
	CtxKeyStandInPaths = "stand-in-paths"
	// CtxKeyAssertionCheckedCommit is the system-test repo commit the
	// assertion-removal check diffs from; advanced on every pass.
	CtxKeyAssertionCheckedCommit = "assertion-checked-commit"
	// CtxKeySystemUnchanged is the GW_SYS gateway signal.
	CtxKeySystemUnchanged = "system-unchanged"
	// CtxKeySystemChangedFiles lists the offending files when the system changed.
	CtxKeySystemChangedFiles = "system-changed-files"
	// CtxKeyNoAssertionRemoved is the GW_ASSERT gateway signal.
	CtxKeyNoAssertionRemoved = "no-assertion-removed"
	// CtxKeyRemovedAssertions lists the removed assertion lines (file:line: text).
	CtxKeyRemovedAssertions = "removed-assertions"
)

// systemRepoBaseline is one repo's entry in CtxKeySystemStartCommits.
type systemRepoBaseline struct {
	Repo   string   `json:"repo"`
	Commit string   `json:"commit"`
	Paths  []string `json:"paths"`
}

// assertionTestLayers are the test-side layers whose removed assertion lines
// halt the flow: acceptance and contract tests, the DSL, and the driver
// adapters.
var assertionTestLayers = []string{"at-test", "ct-test", "dsl-port", "dsl-core", "system-driver-adapter", "external-system-driver-adapter"}

// assertionPatterns match an assertion on one source line, per system-test
// language. The DSL's `.shouldSucceed()` / `.shouldFail()` style is covered
// by the `.should…(` arm in every language.
var assertionPatterns = map[string][]*regexp.Regexp{
	projectconfig.LangJava: {
		regexp.MustCompile(`\bassert\w*\s*\(`), // assertThat(, assertEquals(, assertTrue(, ...
		regexp.MustCompile(`^\s*assert\s`),     // the assert keyword
		regexp.MustCompile(`\.should\w*\s*\(`),
		regexp.MustCompile(`\bverify\s*\(`),
	},
	projectconfig.LangDotnet: {
		regexp.MustCompile(`\bAssert\.\w+`),
		regexp.MustCompile(`\.Should\w*\s*\(`), // .Should(), .ShouldSucceed()
		regexp.MustCompile(`\.should\w*\s*\(`),
	},
	projectconfig.LangTypescript: {
		regexp.MustCompile(`\bexpect\s*\(`),
		regexp.MustCompile(`\bassert\w*\s*[.(]`), // assert(, assert.equal(, assertThat(
		regexp.MustCompile(`\.should\w*\s*\(`),
	},
}

// recordStartCommits is the RECORD service task at the start of characterize.
// It resolves the system paths (the surface plus the db migrations) and the
// stand-in paths, fails hard when they cannot be resolved or overlap, and
// records the start commit of every repo holding system paths plus the
// system-test commit the assertion check starts from. Resume-safe: start
// commits already in State are kept.
func (a actions) recordStartCommits(ctx *statemachine.Context) statemachine.Outcome {
	cfg := a.deps.Config
	if cfg == nil {
		return statemachine.Outcome{Err: fmt.Errorf("record-start-commits: no gh-optivem config wired (driver bug)")}
	}
	repos, err := systemRepoPaths(cfg)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("record-start-commits: %w", err)}
	}
	standIns := standInPaths(cfg)
	if overlaps := overlappingPaths(repos, standIns); len(overlaps) > 0 {
		return statemachine.Outcome{Err: fmt.Errorf("record-start-commits: stand-in paths overlap system paths (%s); a stub/simulator dir must be disjoint from the system code so the safety gate can tell them apart — fix the paths in gh-optivem.yaml", strings.Join(overlaps, "; "))}
	}
	ctx.Set(CtxKeyStandInPaths, strings.Join(standIns, "\n"))

	if ctx.GetString(CtxKeySystemStartCommits) != "" {
		fmt.Fprintln(a.deps.Out.Phase, "Keeping the recorded system start commits (resumed run).")
	} else {
		for i := range repos {
			sha, err := a.headCommit(cfg, repos[i].Repo)
			if err != nil {
				return statemachine.Outcome{Err: fmt.Errorf("record-start-commits: %w", err)}
			}
			repos[i].Commit = sha
		}
		raw, err := json.Marshal(repos)
		if err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("record-start-commits: encode: %w", err)}
		}
		ctx.Set(CtxKeySystemStartCommits, string(raw))
		for _, r := range repos {
			fmt.Fprintf(a.deps.Out.Phase, "System start commit %s%s for %s.\n", repoLabel(cfg, r.Repo), shortSHA(r.Commit), strings.Join(r.Paths, ", "))
		}
	}
	if ctx.GetString(CtxKeyAssertionCheckedCommit) == "" {
		sha, err := a.headCommit(cfg, cfg.SystemTest.Repo)
		if err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("record-start-commits: %w", err)}
		}
		ctx.Set(CtxKeyAssertionCheckedCommit, sha)
	}
	return statemachine.Outcome{}
}

// checkSystemUnchanged is the DIFF_SYSTEM service task. Per system repo it
// lists every file under the system paths that differs from the start commit
// in the working tree (committed or not) or is untracked, and stamps
// system-unchanged for GW_SYS.
func (a actions) checkSystemUnchanged(ctx *statemachine.Context) statemachine.Outcome {
	cfg := a.deps.Config
	if cfg == nil {
		return statemachine.Outcome{Err: fmt.Errorf("check-system-unchanged: no gh-optivem config wired (driver bug)")}
	}
	repos, err := recordedSystemRepos(ctx)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("check-system-unchanged: %w", err)}
	}
	var changed []string
	for _, r := range repos {
		files, err := a.systemChangedFiles(cfg, r)
		if err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("check-system-unchanged: %w", err)}
		}
		for _, f := range files {
			changed = append(changed, repoLabel(cfg, r.Repo)+f)
		}
	}
	if len(changed) == 0 {
		ctx.Set(CtxKeySystemUnchanged, true)
		ctx.Unset(CtxKeySystemChangedFiles)
		fmt.Fprintln(a.deps.Out.Phase, "System code unchanged since the start commit.")
		return statemachine.Outcome{}
	}
	ctx.Set(CtxKeySystemUnchanged, false)
	ctx.Set(CtxKeySystemChangedFiles, strings.Join(changed, "\n"))
	fmt.Fprintf(a.deps.Out.Phase, "HALT: system code changed since the start commit; the baseline must never change the system under test. Changed files:\n  %s\n", strings.Join(changed, "\n  "))
	return statemachine.Outcome{}
}

// systemChangedFiles returns the sorted files under r.Paths that differ from
// r.Commit in the working tree, plus the untracked ones.
func (a actions) systemChangedFiles(cfg *projectconfig.Config, r systemRepoBaseline) ([]string, error) {
	dir, err := a.repoDir(cfg, r.Repo)
	if err != nil {
		return nil, err
	}
	diff, err := a.gitIn(dir, append([]string{"diff", "--name-only", "--no-renames", r.Commit, "--"}, r.Paths...)...)
	if err != nil {
		return nil, err
	}
	status, err := a.gitIn(dir, append([]string{"status", "--porcelain", "--untracked-files=all", "--"}, r.Paths...)...)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for line := range strings.SplitSeq(string(diff), "\n") {
		if f := strings.TrimSpace(line); f != "" {
			seen[f] = true
		}
	}
	for _, f := range porcelainPaths(string(status)) {
		seen[f] = true
	}
	out := make([]string, 0, len(seen))
	for f := range seen {
		out = append(out, f)
	}
	slices.Sort(out)
	return out, nil
}

// checkNoAssertionRemoved is the CHECK_ASSERT service task. It scans the
// lines removed from the test paths since the last checked commit for
// assertions and stamps no-assertion-removed for GW_ASSERT; on a pass it
// advances the last checked commit to HEAD.
func (a actions) checkNoAssertionRemoved(ctx *statemachine.Context) statemachine.Outcome {
	cfg := a.deps.Config
	if cfg == nil {
		return statemachine.Outcome{Err: fmt.Errorf("check-no-assertion-removed: no gh-optivem config wired (driver bug)")}
	}
	from := ctx.GetString(CtxKeyAssertionCheckedCommit)
	if from == "" {
		return statemachine.Outcome{Err: fmt.Errorf("check-no-assertion-removed: %s not in State; record-start-commits must run first", CtxKeyAssertionCheckedCommit)}
	}
	paths, err := ResolveLayerPaths(assertionTestLayers, cfg)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("check-no-assertion-removed: %w", err)}
	}
	dir, err := a.repoDir(cfg, cfg.SystemTest.Repo)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("check-no-assertion-removed: %w", err)}
	}
	diff, err := a.gitIn(dir, append([]string{"diff", "-U0", "--no-color", "--no-ext-diff", "--no-renames", from, "--"}, paths...)...)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("check-no-assertion-removed: %w", err)}
	}
	hits := RemovedAssertions(cfg.SystemTest.Lang, string(diff))
	if len(hits) > 0 {
		ctx.Set(CtxKeyNoAssertionRemoved, false)
		ctx.Set(CtxKeyRemovedAssertions, strings.Join(hits, "\n"))
		fmt.Fprintf(a.deps.Out.Phase, "HALT: assertion lines were removed from the tests since %s; a characterization fix must never weaken an assertion:\n  %s\n", shortSHA(from), strings.Join(hits, "\n  "))
		return statemachine.Outcome{}
	}
	head, err := a.headCommit(cfg, cfg.SystemTest.Repo)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("check-no-assertion-removed: %w", err)}
	}
	ctx.Set(CtxKeyNoAssertionRemoved, true)
	ctx.Unset(CtxKeyRemovedAssertions)
	ctx.Set(CtxKeyAssertionCheckedCommit, head)
	fmt.Fprintln(a.deps.Out.Phase, "No assertion removed from the tests.")
	return statemachine.Outcome{}
}

// RemovedAssertions returns "file:line: text" for every line a unified diff
// (-U0) removes that is an assertion in lang (the union of every language's
// patterns when lang is unknown). A removed line whose exact text is added
// anywhere in the same diff is a move, not a removal, and is not reported;
// comment lines are ignored.
func RemovedAssertions(lang, diff string) []string {
	patterns, ok := assertionPatterns[lang]
	if !ok {
		for _, l := range []string{projectconfig.LangJava, projectconfig.LangDotnet, projectconfig.LangTypescript} {
			patterns = append(patterns, assertionPatterns[l]...)
		}
	}
	type removed struct {
		file, text string
		line       int
	}
	var dels []removed
	added := map[string]int{}
	file, oldLine, inHeader := "", 0, false
	for l := range strings.SplitSeq(diff, "\n") {
		switch {
		case strings.HasPrefix(l, "diff --git "):
			inHeader = true
		case inHeader && strings.HasPrefix(l, "--- "):
			file = strings.TrimPrefix(strings.TrimPrefix(l, "--- "), "a/")
		case inHeader && strings.HasPrefix(l, "+++ "):
			if f := strings.TrimPrefix(l, "+++ "); f != "/dev/null" && file == "/dev/null" {
				file = strings.TrimPrefix(f, "b/")
			}
		case inHeader && !strings.HasPrefix(l, "@@"):
			// index / mode / new-file lines of the file header.
		case strings.HasPrefix(l, "@@"):
			inHeader = false
			oldLine = hunkOldStart(l)
		case strings.HasPrefix(l, "-"):
			dels = append(dels, removed{file: file, text: l[1:], line: oldLine})
			oldLine++
		case strings.HasPrefix(l, "+"):
			added[strings.TrimSpace(l[1:])]++
		}
	}
	var hits []string
	for _, d := range dels {
		t := strings.TrimSpace(d.text)
		if t == "" || isCommentLine(t) || !matchesAny(patterns, d.text) {
			continue
		}
		if added[t] > 0 {
			added[t]--
			continue
		}
		hits = append(hits, fmt.Sprintf("%s:%d: %s", d.file, d.line, t))
	}
	return hits
}

// hunkOldStart parses the old-side start line of a "@@ -a,b +c,d @@" header.
func hunkOldStart(header string) int {
	f := strings.Fields(header)
	if len(f) < 2 || !strings.HasPrefix(f[1], "-") {
		return 0
	}
	n, _, _ := strings.Cut(f[1][1:], ",")
	v, err := strconv.Atoi(n)
	if err != nil {
		return 0
	}
	return v
}

func isCommentLine(t string) bool {
	return strings.HasPrefix(t, "//") || strings.HasPrefix(t, "/*") || strings.HasPrefix(t, "*")
}

func matchesAny(patterns []*regexp.Regexp, s string) bool {
	for _, p := range patterns {
		if p.MatchString(s) {
			return true
		}
	}
	return false
}

// systemRepoPaths resolves the system paths — cfg.SystemSurfacePaths("") plus
// system.db-migration-path — grouped by the repo that holds them, in config
// order. The db migrations are attributed to the first surface tier's repo
// (the monolith repo, or the backend repo on multitier). Microservices and an
// unset architecture are not supported: an error, never an empty list.
func systemRepoPaths(cfg *projectconfig.Config) ([]systemRepoBaseline, error) {
	surface, ok := cfg.SystemSurfacePaths("")
	if !ok {
		return nil, fmt.Errorf("system.architecture %q has no resolvable system paths; characterize supports monolith and multitier only", cfg.System.Architecture)
	}
	var tierRepos []string
	switch cfg.System.Architecture {
	case projectconfig.ArchMonolith:
		tierRepos = []string{cfg.System.Repo}
	default: // multitier: SystemSurfacePaths("") is [backend, frontend]
		tierRepos = []string{cfg.System.Backend.Repo, cfg.System.Frontend.Repo}
	}
	if len(tierRepos) != len(surface) {
		return nil, fmt.Errorf("system surface has %d paths but %d tier repos (resolver drift)", len(surface), len(tierRepos))
	}
	if cfg.System.DbMigrationPath == "" {
		return nil, fmt.Errorf("system.db-migration-path is empty in gh-optivem.yaml")
	}
	paths := append(slices.Clone(surface), cfg.System.DbMigrationPath)
	repos := append(tierRepos, tierRepos[0])

	var out []systemRepoBaseline
	for i, p := range paths {
		if p == "" {
			return nil, fmt.Errorf("a system tier path is empty in gh-optivem.yaml (architecture %s)", cfg.System.Architecture)
		}
		p = cleanRelPath(p)
		j := slices.IndexFunc(out, func(r systemRepoBaseline) bool { return r.Repo == repos[i] })
		if j < 0 {
			out = append(out, systemRepoBaseline{Repo: repos[i]})
			j = len(out) - 1
		}
		if !slices.Contains(out[j].Paths, p) {
			out[j].Paths = append(out[j].Paths, p)
		}
	}
	return out, nil
}

// standInPaths returns the sorted, distinct stub and simulator paths the
// external-systems: registry declares (none without a registry).
func standInPaths(cfg *projectconfig.Config) []string {
	var out []string
	for _, key := range []string{projectconfig.ExternalStubPathKey, projectconfig.ExternalSimulatorPathKey} {
		paths, _ := cfg.ExternalRegistryPaths(key)
		for _, p := range paths {
			if p = cleanRelPath(p); !slices.Contains(out, p) {
				out = append(out, p)
			}
		}
	}
	slices.Sort(out)
	return out
}

// overlappingPaths returns "stand-in X overlaps system Y" for every stand-in
// path equal to, inside, or containing a system path. Repos are not compared:
// a same-named path in another repo is reported too (conservative).
func overlappingPaths(repos []systemRepoBaseline, standIns []string) []string {
	var out []string
	for _, s := range standIns {
		for _, r := range repos {
			for _, p := range r.Paths {
				if pathsOverlap(s, p) {
					out = append(out, fmt.Sprintf("stand-in %s overlaps system %s", s, p))
				}
			}
		}
	}
	return out
}

func pathsOverlap(a, b string) bool {
	return a == b || a == "." || b == "." || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

// cleanRelPath normalises a config path to slash form without "./" or a
// trailing slash.
func cleanRelPath(p string) string {
	return path.Clean(filepath.ToSlash(p))
}

// recordedSystemRepos decodes CtxKeySystemStartCommits.
func recordedSystemRepos(ctx *statemachine.Context) ([]systemRepoBaseline, error) {
	raw := ctx.GetString(CtxKeySystemStartCommits)
	if raw == "" {
		return nil, fmt.Errorf("%s not in State; record-start-commits must run first", CtxKeySystemStartCommits)
	}
	var repos []systemRepoBaseline
	if err := json.Unmarshal([]byte(raw), &repos); err != nil {
		return nil, fmt.Errorf("decode %s: %w", CtxKeySystemStartCommits, err)
	}
	if len(repos) == 0 {
		return nil, fmt.Errorf("%s is empty", CtxKeySystemStartCommits)
	}
	for _, r := range repos {
		if r.Commit == "" || len(r.Paths) == 0 {
			return nil, fmt.Errorf("%s entry for repo %q has no commit or paths", CtxKeySystemStartCommits, r.Repo)
		}
	}
	return repos, nil
}

// repoDir returns the local checkout of the repo named by slug: the working
// repo on a mono-repo project, the sibling clone repolocator resolves on a
// multi-repo one.
func (a actions) repoDir(cfg *projectconfig.Config, slug string) (string, error) {
	if cfg.RepoStrategy != projectconfig.RepoStrategyMultiRepo {
		return a.deps.RepoPath, nil
	}
	res, err := repolocator.Resolve(cfg, "", a.deps.RepoPath)
	if err != nil {
		return "", err
	}
	dir := res.Local[slug]
	if dir == "" {
		return "", fmt.Errorf("no local clone resolved for repo %q", slug)
	}
	return dir, nil
}

// headCommit returns HEAD of the repo named by slug.
func (a actions) headCommit(cfg *projectconfig.Config, slug string) (string, error) {
	dir, err := a.repoDir(cfg, slug)
	if err != nil {
		return "", err
	}
	out, err := a.gitIn(dir, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(string(out))
	if sha == "" {
		return "", fmt.Errorf("git rev-parse HEAD in %s returned no commit", dir)
	}
	return sha, nil
}

// gitIn runs git in dir (the current directory when dir is empty).
func (a actions) gitIn(dir string, args ...string) ([]byte, error) {
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	return a.deps.Git.Run(context.Background(), args...)
}

// porcelainPaths returns the paths of `git status --porcelain` lines (both
// endpoints of a rename).
func porcelainPaths(status string) []string {
	var out []string
	for line := range strings.SplitSeq(status, "\n") {
		if len(line) < 4 {
			continue
		}
		rest := line[3:]
		if from, to, ok := strings.Cut(rest, " -> "); ok {
			out = append(out, strings.TrimSpace(from), strings.TrimSpace(to))
			continue
		}
		if p := strings.TrimSpace(rest); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// repoLabel prefixes a file with its repo slug ("owner/repo:") on a
// multi-repo project, where files of several repos are listed together.
func repoLabel(cfg *projectconfig.Config, slug string) string {
	if slug == "" || cfg.RepoStrategy != projectconfig.RepoStrategyMultiRepo {
		return ""
	}
	return slug + ":"
}

func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
