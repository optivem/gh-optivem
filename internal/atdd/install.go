// Package atdd installs ATDD (Acceptance-Test-Driven Development) Claude
// assets — agents, commands, and prompt docs — from a shop checkout into a
// scaffolded project.
//
// Source-of-truth: the shop checkout at Options.ShopPath (the same checkout
// `gh optivem init` copies system code, workflows, and externals from). This
// package never embeds templates — it copies straight from disk and applies
// install-time substitutions so the consumer's scaffold layout (monorepo,
// multirepo monolith, multirepo multitier) gets correct repo references.
//
// Used by `gh optivem atdd install` (standalone) and by `gh optivem init`
// at the end of the apply-template phase.
package atdd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Options describes a single install run.
type Options struct {
	// ShopPath is the local checkout of optivem/shop pinned at the desired ref.
	ShopPath string
	// DestDir is the root of the consumer repo (where .claude/ and docs/prompts/ go).
	DestDir string
	// Repo is the root repo name (e.g. "page-turner"). Substituted everywhere
	// `shop` appears in the source content (other than `shop/`, the doctrine
	// package convention).
	Repo string
	// Arch is "monolith" or "multitier".
	Arch string
	// RepoStrategy is "monorepo" or "multirepo".
	RepoStrategy string
	// Force overwrites existing files even when their content diverges from
	// the expected install (i.e. the student edited them in place).
	Force bool
	// DryRun logs what would be written without touching disk.
	DryRun bool
}

// Validate checks Options for correctness.
func (o Options) Validate() error {
	if o.ShopPath == "" {
		return fmt.Errorf("ShopPath is required")
	}
	if o.DestDir == "" {
		return fmt.Errorf("DestDir is required")
	}
	if o.Repo == "" {
		return fmt.Errorf("Repo is required")
	}
	switch o.Arch {
	case "monolith", "multitier":
	default:
		return fmt.Errorf("Arch must be 'monolith' or 'multitier' (got %q)", o.Arch)
	}
	switch o.RepoStrategy {
	case "monorepo", "multirepo":
	default:
		return fmt.Errorf("RepoStrategy must be 'monorepo' or 'multirepo' (got %q)", o.RepoStrategy)
	}
	return nil
}

// FileSpec is one managed file: source path within shop, dest path within the
// consumer repo, and the transformed content to write.
type FileSpec struct {
	Src     string
	Dest    string
	Content string
}

// managedAgentDir / managedCommandDir / managedPromptSubdirs declare the
// install's ownership scope. Files matching these patterns are wiped and
// re-written on every install. Anything else in the consumer repo is left
// alone — that includes student-authored agents/commands at the root of
// .claude/ that don't start with the `atdd-` prefix.
const (
	managedAgentDir   = ".claude/agents"
	managedCommandDir = ".claude/commands"
	managedAgentGlob  = "atdd-*.md"
)

var managedPromptSubdirs = []string{"atdd", "architecture", "code"}

// Plan walks the shop checkout, builds the list of managed files, and applies
// install-time transforms (block rewrites, TODO stripping, bulk substitution).
// The returned specs are ready to write to disk.
func Plan(opts Options) ([]FileSpec, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	var specs []FileSpec

	for _, dir := range []string{managedAgentDir, managedCommandDir} {
		s, err := planGlob(opts, dir, managedAgentGlob)
		if err != nil {
			return nil, err
		}
		specs = append(specs, s...)
	}

	for _, sub := range managedPromptSubdirs {
		s, err := planTree(opts, filepath.Join("docs", "prompts", sub))
		if err != nil {
			return nil, err
		}
		specs = append(specs, s...)
	}

	return specs, nil
}

func planGlob(opts Options, rel, pattern string) ([]FileSpec, error) {
	srcDir := filepath.Join(opts.ShopPath, rel)
	matches, err := filepath.Glob(filepath.Join(srcDir, pattern))
	if err != nil {
		return nil, err
	}
	var specs []FileSpec
	for _, src := range matches {
		spec, err := buildSpec(opts, src, filepath.Join(opts.DestDir, rel, filepath.Base(src)))
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

func planTree(opts Options, rel string) ([]FileSpec, error) {
	srcRoot := filepath.Join(opts.ShopPath, rel)
	var specs []FileSpec
	err := filepath.Walk(srcRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		relPath, err := filepath.Rel(opts.ShopPath, path)
		if err != nil {
			return err
		}
		spec, err := buildSpec(opts, path, filepath.Join(opts.DestDir, relPath))
		if err != nil {
			return err
		}
		specs = append(specs, spec)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return specs, nil
}

func buildSpec(opts Options, src, dest string) (FileSpec, error) {
	raw, err := os.ReadFile(src)
	if err != nil {
		return FileSpec{}, fmt.Errorf("read %s: %w", src, err)
	}
	return FileSpec{Src: src, Dest: dest, Content: Transform(string(raw), opts)}, nil
}

// Transform applies the install-time transforms in pipeline order:
//
//  1. Block rewrites (multirepo only) — replace 4 source blocks with
//     strategy-specific variants. These target the literal source text
//     containing `shop`, so they run before bulk substitution.
//  2. Strip multirepo TODO comments — done in every strategy; for monorepo
//     they're noise, and for multirepo we've handled the substitution inline.
//  3. Bulk substitution — `shop` → opts.Repo, preserving `shop/` (the ATDD
//     package convention is doctrine-fixed and must not be substituted).
func Transform(content string, opts Options) string {
	content = applyBlockRewrites(content, opts)
	content = stripMultirepoTODOs(content)
	content = substituteRepoName(content, opts.Repo)
	return content
}

// substituteRepoName replaces bare `shop` with repo, preserving `shop/`. Uses
// a placeholder pass because Go's RE2-based regexp doesn't support negative
// lookahead.
func substituteRepoName(content, repo string) string {
	const placeholder = "\x00ATDD_SHOP_PKG\x00"
	content = strings.ReplaceAll(content, "shop/", placeholder+"/")
	content = strings.ReplaceAll(content, "shop", repo)
	content = strings.ReplaceAll(content, placeholder, "shop")
	return content
}

// todoMultirepoRE matches a single-line HTML comment starting with
// `<!-- TODO(gh-optivem): multirepo support`, including any leading
// whitespace and the trailing newline. The four such comments in shop's
// ATDD content are removed at install time — for multirepo they've been
// handled by block rewrites; for monorepo they're inapplicable.
var todoMultirepoRE = regexp.MustCompile(`(?m)^[ \t]*<!--\s*TODO\(gh-optivem\): multirepo support.*?-->[ \t]*\r?\n?`)

func stripMultirepoTODOs(content string) string {
	return todoMultirepoRE.ReplaceAllString(content, "")
}

// applyBlockRewrites does multirepo-strategy-specific block rewrites for the
// 4 flagged sites. Monorepo strategy is a no-op — the bulk substitution
// already produces correct monorepo wording.
func applyBlockRewrites(content string, opts Options) string {
	if opts.RepoStrategy == "monorepo" {
		return content
	}
	for _, b := range blocksForStrategy(opts) {
		content = strings.Replace(content, b.from, b.to, 1)
	}
	return content
}

type rewrite struct {
	from string
	to   string
}

func blocksForStrategy(opts Options) []rewrite {
	if opts.Arch == "multitier" {
		return []rewrite{
			{managerSourceBlock, managerMultirepoMultitierBlock},
			{manageProjectSourceBlock, manageProjectMultirepoMultitierBlock},
			{acceptanceCommitSourceBlock, acceptanceCommitMultirepoMultitierBlock},
		}
	}
	return []rewrite{
		{managerSourceBlock, managerMultirepoMonolithBlock},
		{manageProjectSourceBlock, manageProjectMultirepoMonolithBlock},
		{acceptanceCommitSourceBlock, acceptanceCommitMultirepoMonolithBlock},
	}
}

// =====================================================================
// Source/replacement blocks
//
// All blocks use literal `shop` / `shop-system` / `shop-backend` / `shop-frontend`.
// Bulk substitution rewrites `shop` → opts.Repo afterwards, so a multirepo
// monolith install with --repo page-turner ends up with `page-turner-system`,
// and a multirepo multitier install ends up with `page-turner-backend` /
// `page-turner-frontend`.
//
// Each `from` block is the EXACT source text from shop; whitespace and
// punctuation must match byte-for-byte. The atdd-implement-ticket.md and
// atdd-manage-project.md blocks are textually identical, so we reuse the
// same constant for both.
// =====================================================================

// atdd-manager.md, lines 25–27 — "Known repositories" sub-list inside the
// manager-agent's repo-resolution step.
const managerSourceBlock = "     - Known test repositories: `shop` (system tests live in the same monorepo as the system).\n" +
	"     - Known system repositories: `shop`.\n" +
	"     - If the issue gives no clear signal, default to the `shop` repository."

const managerMultirepoMonolithBlock = "     - Known test repositories: `shop` (the orchestration root repo, which hosts the system tests).\n" +
	"     - Known system repositories: `shop-system`.\n" +
	"     - If the issue gives no clear signal, default to `shop` for tests and `shop-system` for system code."

const managerMultirepoMultitierBlock = "     - Known test repositories: `shop` (the orchestration root repo, which hosts the system tests).\n" +
	"     - Known system repositories: `shop-backend`, `shop-frontend`.\n" +
	"     - If the issue gives no clear signal, default to `shop` for tests and pick `shop-backend` or `shop-frontend` for system code based on the issue context (backend logic vs UI)."

// atdd-manage-project.md / atdd-implement-ticket.md, lines 10–11 —
// --test-repos / --system-repos example lines.
const manageProjectSourceBlock = "- `--test-repos <repo1>,<repo2>,...` — the test repositories to implement in (e.g. `shop`)\n" +
	"- `--system-repos <repo1>,<repo2>,...` — the system (backend/frontend) repositories (e.g. `shop`)"

const manageProjectMultirepoMonolithBlock = "- `--test-repos <repo1>,<repo2>,...` — the test repositories to implement in (e.g. `shop`)\n" +
	"- `--system-repos <repo1>,<repo2>,...` — the system (backend/frontend) repositories (e.g. `shop-system`)"

const manageProjectMultirepoMultitierBlock = "- `--test-repos <repo1>,<repo2>,...` — the test repositories to implement in (e.g. `shop`)\n" +
	"- `--system-repos <repo1>,<repo2>,...` — the system (backend/frontend) repositories (e.g. `shop-backend`,`shop-frontend`)"

// acceptance-tests.md, lines 129–137 — the AT - GREEN - SYSTEM - COMMIT
// numbered list. Steps 1, 4, 5 reference `shop`; in multirepo scaffolds,
// step 1 (the system commit) lives in the system/component repos and steps
// 4–5 (the test commit) live in the test/orchestration repo.
const acceptanceCommitSourceBlock = "1. In the `shop` repository: COMMIT all backend and frontend changes with message `<Scenario> | AT - GREEN - SYSTEM`.\n" +
	"2. Remove the disabled annotation (reason `\"AT - RED - SYSTEM DRIVER\"`) from the tests.\n" +
	"3. Run the tests and verify they all pass:\n" +
	"   ```\n" +
	"   gh optivem test system --suite <acceptance-api> --test <TestMethodName>\n" +
	"   gh optivem test system --suite <acceptance-ui> --test <TestMethodName>\n" +
	"   ```\n" +
	"4. Ensure that there are no non-test files in the list of changed files in the `shop` repository.\n" +
	"5. COMMIT in the `shop` repository with message `<Scenario> | AT - GREEN - SYSTEM`."

const acceptanceCommitMultirepoMonolithBlock = "1. In the `shop-system` repository: COMMIT all backend and frontend changes with message `<Scenario> | AT - GREEN - SYSTEM`.\n" +
	"2. Remove the disabled annotation (reason `\"AT - RED - SYSTEM DRIVER\"`) from the tests.\n" +
	"3. Run the tests and verify they all pass:\n" +
	"   ```\n" +
	"   gh optivem test system --suite <acceptance-api> --test <TestMethodName>\n" +
	"   gh optivem test system --suite <acceptance-ui> --test <TestMethodName>\n" +
	"   ```\n" +
	"4. Ensure that there are no non-test files in the list of changed files in the `shop` repository.\n" +
	"5. COMMIT in the `shop` repository with message `<Scenario> | AT - GREEN - SYSTEM`."

const acceptanceCommitMultirepoMultitierBlock = "1. Commit system changes:\n" +
	"   - In the `shop-backend` repository: COMMIT backend changes with message `<Scenario> | AT - GREEN - SYSTEM`.\n" +
	"   - In the `shop-frontend` repository: COMMIT frontend changes with message `<Scenario> | AT - GREEN - SYSTEM`.\n" +
	"2. Remove the disabled annotation (reason `\"AT - RED - SYSTEM DRIVER\"`) from the tests.\n" +
	"3. Run the tests and verify they all pass:\n" +
	"   ```\n" +
	"   gh optivem test system --suite <acceptance-api> --test <TestMethodName>\n" +
	"   gh optivem test system --suite <acceptance-ui> --test <TestMethodName>\n" +
	"   ```\n" +
	"4. Ensure that there are no non-test files in the list of changed files in the `shop` repository.\n" +
	"5. COMMIT in the `shop` repository with message `<Scenario> | AT - GREEN - SYSTEM`."

// =====================================================================
// Install (runtime — disk operations)
// =====================================================================

// Install runs the full install flow: pre-flight check, wipe managed sets,
// write fresh content, post-condition validation. Idempotent.
func Install(opts Options) error {
	specs, err := Plan(opts)
	if err != nil {
		return err
	}
	if !opts.Force {
		diverged, err := preflightDivergence(specs, opts.DestDir)
		if err != nil {
			return err
		}
		if len(diverged) > 0 {
			return fmt.Errorf("%d managed file(s) have local edits — pass --force to overwrite:\n  %s",
				len(diverged), strings.Join(diverged, "\n  "))
		}
	}
	if opts.DryRun {
		for _, s := range specs {
			rel, _ := filepath.Rel(opts.DestDir, s.Dest)
			fmt.Printf("[DRY RUN] write %s (%d bytes)\n", rel, len(s.Content))
		}
		return nil
	}
	if err := wipeManagedSets(opts.DestDir); err != nil {
		return err
	}
	for _, s := range specs {
		if err := writeFile(s.Dest, []byte(s.Content)); err != nil {
			return err
		}
	}
	return Validate(opts.DestDir)
}

// preflightDivergence returns repo-relative paths whose existing content
// differs from what Install would write. Files that don't exist on disk are
// not divergent — they'll just be written fresh.
func preflightDivergence(specs []FileSpec, destDir string) ([]string, error) {
	var diverged []string
	for _, s := range specs {
		existing, err := os.ReadFile(s.Dest)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("preflight read %s: %w", s.Dest, err)
		}
		if !bytes.Equal(existing, []byte(s.Content)) {
			rel, _ := filepath.Rel(destDir, s.Dest)
			diverged = append(diverged, rel)
		}
	}
	return diverged, nil
}

// wipeManagedSets removes every managed file under destDir. After this:
//   - .claude/agents/atdd-*.md and .claude/commands/atdd-*.md are gone
//     (other files in those dirs are preserved).
//   - docs/prompts/{atdd,architecture,code}/ are entirely gone.
func wipeManagedSets(destDir string) error {
	for _, dir := range []string{managedAgentDir, managedCommandDir} {
		matches, err := filepath.Glob(filepath.Join(destDir, dir, managedAgentGlob))
		if err != nil {
			return err
		}
		for _, m := range matches {
			if err := os.Remove(m); err != nil {
				return fmt.Errorf("remove %s: %w", m, err)
			}
		}
	}
	for _, sub := range managedPromptSubdirs {
		target := filepath.Join(destDir, "docs", "prompts", sub)
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("remove %s: %w", target, err)
		}
	}
	return nil
}

func writeFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

// Validate scans every managed file under destDir for forbidden literals.
// A surviving lowercase `shop` (other than `shop/`, the doctrine package
// convention) means a substitution rule is missing. Mirrors apply_template's
// ValidateNoLeftoverTemplateRefs philosophy.
func Validate(destDir string) error {
	paths, err := managedDestPaths(destDir)
	if err != nil {
		return err
	}
	var leftovers []string
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		hits := findBareShop(string(data))
		if len(hits) == 0 {
			continue
		}
		rel, _ := filepath.Rel(destDir, p)
		for _, h := range hits {
			leftovers = append(leftovers, fmt.Sprintf("%s:%d: %s", rel, h.lineNo, h.text))
		}
	}
	if len(leftovers) > 0 {
		return fmt.Errorf("post-condition: %d leftover `shop` reference(s) found — substitution incomplete:\n  %s",
			len(leftovers), strings.Join(leftovers, "\n  "))
	}
	return nil
}

type leftover struct {
	lineNo int
	text   string
}

// findBareShop returns positions of `shop` (lowercase, not followed by `/`)
// in content. The post-condition validator flags these as missed substitutions.
func findBareShop(content string) []leftover {
	var hits []leftover
	for i, line := range strings.Split(content, "\n") {
		idx := 0
		for idx <= len(line)-len("shop") {
			j := strings.Index(line[idx:], "shop")
			if j < 0 {
				break
			}
			j += idx
			after := j + len("shop")
			if after < len(line) && line[after] == '/' {
				idx = after + 1
				continue
			}
			hits = append(hits, leftover{lineNo: i + 1, text: strings.TrimSpace(line)})
			break
		}
	}
	return hits
}

// managedDestPaths returns dest paths of every managed file currently under
// destDir.
func managedDestPaths(destDir string) ([]string, error) {
	var paths []string
	for _, dir := range []string{managedAgentDir, managedCommandDir} {
		matches, err := filepath.Glob(filepath.Join(destDir, dir, managedAgentGlob))
		if err != nil {
			return nil, err
		}
		paths = append(paths, matches...)
	}
	for _, sub := range managedPromptSubdirs {
		root := filepath.Join(destDir, "docs", "prompts", sub)
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if !info.IsDir() {
				paths = append(paths, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return paths, nil
}
