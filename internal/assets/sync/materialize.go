// File materialize.go implements per-project reference-doc substitution.
//
// EnsureSynced writes the embedded runtime/references/ tree to
// ~/.gh-optivem/references/ verbatim — that location is shared across
// every project the user touches and MUST NOT carry per-project
// substitutions (different projects' ${driver-port} values would clobber
// each other).
//
// MaterializeProject is the project-aware counterpart. It reads the same
// embedded sources, substitutes ${name} placeholders against the
// project's projectconfig.Config-derived placeholder map, and writes
// the substituted output under ./.gh-optivem/references/ inside the
// project tree. A sidecar at ./.gh-optivem/.materialized.yaml records the
// placeholder values used so the next invocation skips re-materializing
// when nothing has changed.
//
// Stale-detection uses VALUE comparison, not mtime. A `touch
// gh-optivem.yaml` with no semantic change does not trigger
// re-materialization; a binary upgrade does (binary_version is part of
// the sidecar).

package sync

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/optivem/gh-optivem/internal/assets"
	"github.com/optivem/gh-optivem/internal/expand"
)

const (
	// projectReferencesSubdir is the path (relative to the project repo
	// root) where MaterializeProject writes substituted reference docs.
	// Mirrors dirGhOptivem at the project level — `.gh-optivem/references/`
	// sits in the project tree the same way `~/.gh-optivem/references/`
	// sits in the user home.
	projectReferencesSubdir = ".gh-optivem/references"

	// projectSidecarPath is the project-relative path of the staleness
	// sidecar. Sits alongside references/ (not inside) so a wipe-then-write
	// of the references subtree doesn't disturb the sidecar accidentally.
	projectSidecarPath = ".gh-optivem/.materialized.yaml"
)

// placeholderRE matches a single ${name} occurrence so MaterializeProject
// can build the per-file frontmatter audit block (which keys actually
// appeared in this body). Same shape as expand.FindUnfilled's regex,
// but unanchored — we want every match, not just deduplicated names.
var placeholderRE = regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_-]*)\}`)

// sidecar is the on-disk shape of .gh-optivem/.materialized.yaml.
// binary_version forces a re-materialize after a gh-optivem upgrade
// even when the project config hasn't changed (the embedded template
// itself may have).
type sidecar struct {
	BinaryVersion string            `yaml:"binary_version"`
	Placeholders  map[string]string `yaml:"placeholders"`
}

// MaterializeProject substitutes the embedded reference docs against the
// project's placeholder map and writes them to ./.gh-optivem/references/
// under repoPath. Idempotent — when the on-disk sidecar matches the
// inputs, returns the project-references root path without re-materializing.
//
// When binaryVersion is empty, treats the binary version as a wildcard
// (matches any sidecar value); used by tests that don't pin a version.
//
// Returns the absolute path of the materialized project-references root so
// callers can substitute it for ${references-root} in agent prompts.
func MaterializeProject(repoPath, binaryVersion string, placeholders map[string]string) (string, error) {
	if repoPath == "" {
		return "", fmt.Errorf("materialize: repoPath is required")
	}
	if placeholders == nil {
		placeholders = map[string]string{}
	}

	projectRoot := filepath.Join(repoPath, projectReferencesSubdir)
	sidecarPath := filepath.Join(repoPath, projectSidecarPath)

	stale, err := projectStale(sidecarPath, binaryVersion, placeholders)
	if err != nil {
		return "", err
	}
	if !stale {
		return projectRoot, nil
	}

	syncMu.Lock()
	defer syncMu.Unlock()

	// Re-check under the mutex so a concurrent caller that already
	// materialized lets us short-circuit without re-doing the walk.
	stale, err = projectStale(sidecarPath, binaryVersion, placeholders)
	if err != nil {
		return "", err
	}
	if !stale {
		return projectRoot, nil
	}

	if err := os.RemoveAll(projectRoot); err != nil {
		return "", fmt.Errorf("materialize: wipe %s: %w", projectRoot, err)
	}

	walkErr := fs.WalkDir(assets.FS, embeddedReferencesRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(path, embeddedReferencesRoot+"/")
		dest := filepath.Join(repoPath, projectReferencesSubdir, rel)
		data, err := assets.FS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("materialize: read embedded %s: %w", path, err)
		}

		// Substitute only the body of files that look like markdown.
		// Other extensions (e.g. embedded YAML or images) round-trip
		// verbatim — they don't carry the ${name} convention.
		out := data
		if strings.HasSuffix(path, ".md") {
			substituted, err := substituteDoc(string(data), placeholders, rel)
			if err != nil {
				return err
			}
			out = []byte(substituted)
		}
		return atomicWriteFile(dest, out)
	})
	if walkErr != nil {
		return "", walkErr
	}

	if err := writeSidecar(sidecarPath, binaryVersion, placeholders); err != nil {
		return "", err
	}
	return projectRoot, nil
}

// ProjectReferencesRoot returns the absolute path of the materialized
// project-references root for repoPath — i.e. <repoPath>/.gh-optivem/references.
// Does NOT verify that the path exists or is current; callers that
// need that should call MaterializeProject (which is idempotent).
func ProjectReferencesRoot(repoPath string) string {
	return filepath.Join(repoPath, projectReferencesSubdir)
}

// substituteDoc applies the placeholder map to body and prepends a YAML
// frontmatter audit block listing only the keys that appeared in body.
// Returns an error if any ${name} survives substitution — that signals
// a doc references a placeholder the project's config doesn't define,
// which would silently render a broken doc to the agent.
func substituteDoc(body string, placeholders map[string]string, sourcePath string) (string, error) {
	usedKeys := collectPlaceholderNames(body)
	substituted := expand.Apply(body, placeholders)
	if leftovers := expand.FindUnfilled(substituted); len(leftovers) > 0 {
		declared := sortedKeys(placeholders)
		return "", fmt.Errorf(
			"materialize: %s references unfilled placeholders %s; declared: %s",
			sourcePath, strings.Join(leftovers, ", "), strings.Join(declared, ", "))
	}
	if len(usedKeys) == 0 {
		// No placeholders in this file — skip the frontmatter block
		// entirely so the on-disk content is byte-identical to the
		// embedded source.
		return substituted, nil
	}
	frontmatter := buildFrontmatter(usedKeys, placeholders)
	if strings.HasPrefix(substituted, "---\n") {
		// File already starts with a frontmatter block — merge our
		// substituted: key in by inserting it after the opening delimiter.
		return strings.Replace(substituted, "---\n", "---\nsubstituted:\n"+frontmatter, 1), nil
	}
	return "---\nsubstituted:\n" + frontmatter + "---\n\n" + substituted, nil
}

// collectPlaceholderNames returns the set of distinct ${name} identifiers
// referenced in s, sorted lexicographically for stable frontmatter
// output.
func collectPlaceholderNames(s string) []string {
	matches := placeholderRE.FindAllStringSubmatch(s, -1)
	seen := map[string]struct{}{}
	for _, m := range matches {
		seen[m[1]] = struct{}{}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// buildFrontmatter renders the substituted: block body — one indented
// `  key: value` line per key, in the order keys is given (already
// sorted by collectPlaceholderNames). The closing `---\n` delimiter is
// added by the caller.
func buildFrontmatter(keys []string, placeholders map[string]string) string {
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "  %s: %s\n", k, placeholders[k])
	}
	return b.String()
}

// projectStale reports whether the on-disk sidecar at sidecarPath
// matches the given inputs. Returns true (stale) when the sidecar is
// absent, malformed, or its content differs from the inputs.
func projectStale(sidecarPath, binaryVersion string, placeholders map[string]string) (bool, error) {
	data, err := os.ReadFile(sidecarPath)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return true, fmt.Errorf("materialize: read sidecar %s: %w", sidecarPath, err)
	}
	var on sidecar
	if err := yaml.Unmarshal(data, &on); err != nil {
		// Corrupt sidecar — re-materialize and overwrite.
		return true, nil
	}
	if binaryVersion != "" && on.BinaryVersion != binaryVersion {
		return true, nil
	}
	if !mapsEqual(on.Placeholders, placeholders) {
		return true, nil
	}
	return false, nil
}

// writeSidecar marshals the current inputs to sidecarPath atomically.
func writeSidecar(sidecarPath, binaryVersion string, placeholders map[string]string) error {
	on := sidecar{
		BinaryVersion: binaryVersion,
		Placeholders:  placeholders,
	}
	data, err := yaml.Marshal(&on)
	if err != nil {
		return fmt.Errorf("materialize: marshal sidecar: %w", err)
	}
	return atomicWriteFile(sidecarPath, data)
}

// mapsEqual reports whether two string maps have identical key sets and
// per-key values. nil and empty are treated as equal.
func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		bv, ok := b[k]
		if !ok || bv != v {
			return false
		}
	}
	return true
}

// sortedKeys returns the keys of m sorted lexicographically. Used by
// substituteDoc to render a deterministic "declared: ..." list in error
// messages.
func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
