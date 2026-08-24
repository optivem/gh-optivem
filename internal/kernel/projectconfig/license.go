package projectconfig

import "strings"

// License enum values, surfaced as YAML strings.
const (
	LicenseMIT       = "mit"
	LicenseMIT0      = "mit-0"
	LicenseApache2   = "apache-2.0"
	LicenseGPL3      = "gpl-3.0"
	LicenseBSD2      = "bsd-2-clause"
	LicenseBSD3      = "bsd-3-clause"
	License0BSD      = "0bsd"
	LicenseUnlicense = "unlicense"
)

// licenseKeys is the offered set in presentation order — the order the
// `config init` prompt numbers them and the order --license's usage string
// and validation errors list them. Every key here MUST have a bundled text
// at internal/scaffolding/assets/licenses/<key>.txt; TestEveryOfferedKeyHasAsset
// pins that both ways.
//
// mit-0 and 0bsd are offered even though the GitHub licenses API does not
// serve them: since plan 20260824-1119 the texts are bundled assets, not
// fetched, so the offered set is no longer capped by what that API carries.
// shop's own template is MIT-0.
var licenseKeys = []string{
	LicenseMIT,
	LicenseMIT0,
	LicenseApache2,
	LicenseGPL3,
	LicenseBSD2,
	LicenseBSD3,
	License0BSD,
	LicenseUnlicense,
}

// licenseNames maps each accepted license key to its human-readable name.
// Single source of truth — internal/config.Config.LicenseName delegates
// to LicenseName below.
var licenseNames = map[string]string{
	LicenseMIT:       "MIT License",
	LicenseMIT0:      "MIT No Attribution",
	LicenseApache2:   "Apache License 2.0",
	LicenseGPL3:      "GNU General Public License v3.0",
	LicenseBSD2:      "BSD 2-Clause License",
	LicenseBSD3:      "BSD 3-Clause License",
	License0BSD:      "BSD Zero Clause License",
	LicenseUnlicense: "The Unlicense",
}

// licenseSPDXIDs maps each accepted license key to the SPDX identifier that
// belongs in a machine-readable package manifest (package.json "license",
// .csproj PackageLicenseExpression). The YAML key is a lowercase slug for
// operator ergonomics; SPDX ids are case-sensitive and not derivable from it
// by rule (mit -> MIT, but 0bsd -> 0BSD and gpl-3.0 -> GPL-3.0-only), so the
// mapping is explicit.
//
// GPL-3.0 uses the "-only" form: the bare "GPL-3.0" id is deprecated in the
// SPDX license list, and the bundled gpl-3.0 text grants version 3 alone
// (the "or later" election is a per-file header choice, not a property of
// the document we ship).
var licenseSPDXIDs = map[string]string{
	LicenseMIT:       "MIT",
	LicenseMIT0:      "MIT-0",
	LicenseApache2:   "Apache-2.0",
	LicenseGPL3:      "GPL-3.0-only",
	LicenseBSD2:      "BSD-2-Clause",
	LicenseBSD3:      "BSD-3-Clause",
	License0BSD:      "0BSD",
	LicenseUnlicense: "Unlicense",
}

// LicenseKeys returns the offered license keys in presentation order.
// Callers that render a choice list or an error message use this rather
// than hardcoding the set, so adding a license is a one-line change here.
func LicenseKeys() []string {
	out := make([]string, len(licenseKeys))
	copy(out, licenseKeys)
	return out
}

// LicenseKeyList returns the offered keys as a comma-separated string, for
// --license usage text and validation errors.
func LicenseKeyList() string {
	return strings.Join(licenseKeys, ", ")
}

// LicenseName returns the human-readable license name for a key, or the
// key itself if the key is not in the known set. Used in scaffold banners
// and README generation.
func LicenseName(key string) string {
	if name, ok := licenseNames[key]; ok {
		return name
	}
	return key
}

// LicenseSPDXID returns the SPDX identifier for a key, or "" when the key is
// not in the known set. Callers templating a package manifest must treat ""
// as a hard failure rather than writing an empty license field — an empty
// field is indistinguishable from "unlicensed" to npm and SBOM tooling.
func LicenseSPDXID(key string) string {
	return licenseSPDXIDs[key]
}

// IsValidLicense reports whether key is a known license. Used by Validate
// and by internal/config to reject bad --license input.
func IsValidLicense(key string) bool {
	_, ok := licenseNames[key]
	return ok
}

// Deploy enum values.
const (
	DeployDocker   = "docker"
	DeployCloudRun = "cloud-run"
)

// IsValidDeploy reports whether v is a known deploy target.
func IsValidDeploy(v string) bool {
	return v == DeployDocker || v == DeployCloudRun
}
