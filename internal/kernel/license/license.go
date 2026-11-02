// Package license owns the license texts the scaffolder can write into a
// generated repo, and everything derived from them: the offered key set, the
// human-readable names, the SPDX ids and the LICENSE rendering.
//
// texts/ holds one <key>.txt per offered license key. The directory IS the
// offered set: Keys() lists it, so adding a license means adding a .txt file
// (pulled with scripts/refresh-licenses.sh) plus its row in licenseMeta below.
// TestEveryKeyHasMetadata pins the two to each other.
//
// The package is a kernel leaf (no project imports) so that both
// internal/kernel/projectconfig (validation) and internal/scaffolding/steps
// (LICENSE writer) can depend on it without either reaching into the other.
//
// The texts are bundled rather than fetched from the GitHub licenses API at
// scaffold time for three reasons (plan 20260824-1119):
//
//   - No network dependency at scaffold time, so there is no transient
//     failure that could be downgraded into "repo shipped with no LICENSE".
//   - The API serves neither mit-0 nor 0bsd, so a fetch-based design
//     structurally cannot offer the license shop's own template uses.
//   - The placeholder-filling rule below is a property of text we control,
//     not of upstream text that can change under us silently.
package license

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed texts/*.txt
var textFS embed.FS

const textDir = "texts"

// Placeholders for the operative copyright notice. They are GitHub's own
// convention (the /licenses/{key} API bodies for mit, bsd-2-clause and
// bsd-3-clause carry them verbatim), so refreshed texts need no conversion.
const (
	yearPlaceholder   = "[year]"
	holderPlaceholder = "[fullname]"
)

// meta is the per-key data that cannot be read off the text itself.
type meta struct {
	// name is the human-readable license name (README badge, scaffold banner).
	name string
	// spdx is the SPDX identifier for machine-readable package manifests
	// (package.json "license", .csproj PackageLicenseExpression). Not
	// derivable from the key by rule (0bsd -> 0BSD, gpl-3.0 -> GPL-3.0-only).
	spdx string
}

// licenseMeta carries name and SPDX id per bundled key.
//
// The GNU licenses use the "-only" SPDX form: the bare ids ("GPL-3.0",
// which the GitHub API still reports) are deprecated in the SPDX license
// list, and the bundled documents grant their version alone (the "or later"
// election is a per-file header choice, not a property of the document).
var licenseMeta = map[string]meta{
	"0bsd":         {"BSD Zero Clause License", "0BSD"},
	"agpl-3.0":     {"GNU Affero General Public License v3.0", "AGPL-3.0-only"},
	"apache-2.0":   {"Apache License 2.0", "Apache-2.0"},
	"bsd-2-clause": {"BSD 2-Clause License", "BSD-2-Clause"},
	"bsd-3-clause": {"BSD 3-Clause License", "BSD-3-Clause"},
	"bsl-1.0":      {"Boost Software License 1.0", "BSL-1.0"},
	"cc0-1.0":      {"Creative Commons Zero v1.0 Universal", "CC0-1.0"},
	"epl-2.0":      {"Eclipse Public License 2.0", "EPL-2.0"},
	"gpl-2.0":      {"GNU General Public License v2.0", "GPL-2.0-only"},
	"gpl-3.0":      {"GNU General Public License v3.0", "GPL-3.0-only"},
	"lgpl-2.1":     {"GNU Lesser General Public License v2.1", "LGPL-2.1-only"},
	"lgpl-3.0":     {"GNU Lesser General Public License v3.0", "LGPL-3.0-only"},
	"mit":          {"MIT License", "MIT"},
	"mit-0":        {"MIT No Attribution", "MIT-0"},
	"mpl-2.0":      {"Mozilla Public License 2.0", "MPL-2.0"},
	"unlicense":    {"The Unlicense", "Unlicense"},
}

// keys is the sorted offered set, computed once from the embedded directory.
var keys = func() []string {
	entries, err := fs.ReadDir(textFS, textDir)
	if err != nil {
		// The directory is compiled in; failing to read it is a broken build.
		panic(fmt.Sprintf("license: read embedded %s: %v", textDir, err))
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}
		out = append(out, strings.TrimSuffix(e.Name(), ".txt"))
	}
	sort.Strings(out)
	return out
}()

// Keys returns the offered license keys, sorted. Callers that render a choice
// list or an error message use this rather than hardcoding the set.
func Keys() []string {
	out := make([]string, len(keys))
	copy(out, keys)
	return out
}

// KeyList returns the offered keys as a comma-separated string, for --license
// usage text and validation errors.
func KeyList() string {
	return strings.Join(keys, ", ")
}

// IsValid reports whether key has a bundled text.
func IsValid(key string) bool {
	i := sort.SearchStrings(keys, key)
	return i < len(keys) && keys[i] == key
}

// Name returns the human-readable license name for a key, or the key itself
// when it has no metadata.
func Name(key string) string {
	if m, ok := licenseMeta[key]; ok {
		return m.name
	}
	return key
}

// SPDXID returns the SPDX identifier for a key, or "" when the key is not
// known. Callers templating a package manifest must treat "" as a hard
// failure rather than writing an empty license field: an empty field is
// indistinguishable from "unlicensed" to npm and SBOM tooling.
func SPDXID(key string) string {
	return licenseMeta[key].spdx
}

// Text returns the bundled text for key exactly as shipped, with no
// substitution applied. A key with no bundled asset is an error, never an
// empty string: callers must fail rather than write a zero-byte LICENSE.
func Text(key string) (string, error) {
	if !IsValid(key) {
		return "", fmt.Errorf("no bundled license text for key %q (expected embedded asset internal/kernel/license/%s/%s.txt); must be one of %s",
			key, textDir, key, KeyList())
	}
	b, err := textFS.ReadFile(textDir + "/" + key + ".txt")
	if err != nil {
		return "", fmt.Errorf("read bundled license text for key %q: %w", key, err)
	}
	return string(b), nil
}

// HasFillableNotice reports whether key's bundled text carries the operative
// copyright notice inline, marked by the [fullname] placeholder, which Render
// fills. Derived from the text, so there is no second list to keep in sync.
//
// Only the MIT/BSD-style texts carry it. Apache-2.0 and the GNU licenses have
// placeholders too, but in a different spelling ("[yyyy] [name of copyright
// owner]", "<year>  <name of author>") and only inside an instructional
// appendix telling the reader how to head their own source files. Those
// documents ship verbatim; GPL even says "changing it is not allowed".
func HasFillableNotice(key string) bool {
	text, err := Text(key)
	return err == nil && strings.Contains(text, holderPlaceholder)
}

// Render returns the license text for key with the copyright notice filled
// in, when the key's notice is fillable (see HasFillableNotice).
//
// year and holder are only consulted for the fillable keys; for the others
// the returned text is byte-identical to the bundled asset. An unknown key is
// an error (see Text).
func Render(key, year, holder string) (string, error) {
	text, err := Text(key)
	if err != nil {
		return "", err
	}
	if !strings.Contains(text, holderPlaceholder) {
		return text, nil
	}
	// Substituted token-by-token rather than as a whole line: the bundled
	// texts differ in punctuation between the two ("[year] [fullname]" for
	// mit/mit-0/0bsd, "[year], [fullname]" for the BSD clauses), and the
	// per-token form is agnostic to that.
	r := strings.NewReplacer(yearPlaceholder, year, holderPlaceholder, holder)
	return r.Replace(text), nil
}
