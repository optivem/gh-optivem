// Package assets exposes the embedded asset tree the scaffolder writes into
// generated repos.
//
// licenses/ holds the full text of every license key the scaffolder offers,
// one <key>.txt per key in internal/kernel/projectconfig.LicenseKeys(). The
// texts are bundled rather than fetched from the GitHub licenses API for
// three reasons (plan 20260824-1119):
//
//   - No network dependency at scaffold time, so there is no transient
//     failure that could be downgraded into "repo shipped with no LICENSE".
//   - The API serves neither mit-0 nor 0bsd, so a fetch-based design
//     structurally cannot offer the license shop's own template uses.
//   - The placeholder-filling rule below is a property of text we control,
//     not of upstream text that can change under us silently.
package assets

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed licenses
var licenseFS embed.FS

// licensesWithFillableNotice are the keys whose text carries the operative
// copyright notice inline, where the [year]/[fullname] placeholders name the
// actual copyright holder and MUST be filled.
//
// Apache-2.0 and GPL-3.0 are deliberately absent. Their placeholders appear
// only inside an instructional appendix ("APPENDIX: How to apply the Apache
// License to your work", "How to Apply These Terms to Your New Programs")
// telling the reader how to head their own source files — the license
// document itself ships verbatim, and GPL-3.0 says so outright on line 6:
// "of this license document, but changing it is not allowed." The Unlicense
// carries no placeholder at all.
var licensesWithFillableNotice = map[string]bool{
	"mit":          true,
	"mit-0":        true,
	"bsd-2-clause": true,
	"bsd-3-clause": true,
	"0bsd":         true,
}

// LicenseText returns the bundled text for key exactly as shipped, with no
// substitution applied. A key with no bundled asset is an error, never an
// empty string: callers must fail rather than write a zero-byte LICENSE.
func LicenseText(key string) (string, error) {
	path := "licenses/" + key + ".txt"
	b, err := licenseFS.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("no bundled license text for key %q (expected embedded asset internal/scaffolding/assets/%s)", key, path)
	}
	return string(b), nil
}

// RenderLicense returns the license text for key with the copyright notice
// filled in, when the key's notice is fillable.
//
// year and holder are only consulted for the fillable keys; for the others
// the returned text is byte-identical to the bundled asset. An unknown key
// is an error — see LicenseText.
func RenderLicense(key, year, holder string) (string, error) {
	text, err := LicenseText(key)
	if err != nil {
		return "", err
	}
	if !licensesWithFillableNotice[key] {
		return text, nil
	}
	// Substituted token-by-token rather than as a whole line: the bundled
	// texts differ in punctuation between the two ("[year] [fullname]" for
	// mit/mit-0/0bsd, "[year], [fullname]" for the BSD clauses), and the
	// per-token form is agnostic to that.
	r := strings.NewReplacer("[year]", year, "[fullname]", holder)
	return r.Replace(text), nil
}

// HasFillableNotice reports whether key's bundled text carries an inline
// copyright notice that RenderLicense fills. Exported for tests and for
// callers that need to explain the behaviour.
func HasFillableNotice(key string) bool {
	return licensesWithFillableNotice[key]
}
