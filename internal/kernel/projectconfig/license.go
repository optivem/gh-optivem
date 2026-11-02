package projectconfig

import "github.com/optivem/gh-optivem/internal/kernel/license"

// License enum values, surfaced as YAML strings. Named constants exist only
// for the keys code refers to by name (defaults, tests); the offered set is
// license.Keys(), which is derived from the bundled texts under
// internal/kernel/license/texts, so adding a license needs no edit here.
const (
	LicenseMIT       = "mit"
	LicenseMIT0      = "mit-0"
	LicenseApache2   = "apache-2.0"
	LicenseGPL3      = "gpl-3.0"
	LicenseAGPL3     = "agpl-3.0"
	LicenseBSD2      = "bsd-2-clause"
	LicenseBSD3      = "bsd-3-clause"
	License0BSD      = "0bsd"
	LicenseUnlicense = "unlicense"
)

// LicenseKeys returns the offered license keys, sorted. It is the set of
// bundled texts (license.Keys). Callers that render a choice list or an
// error message use this rather than hardcoding the set.
func LicenseKeys() []string {
	return license.Keys()
}

// LicenseKeyList returns the offered keys as a comma-separated string, for
// --license usage text and validation errors.
func LicenseKeyList() string {
	return license.KeyList()
}

// LicenseName returns the human-readable license name for a key, or the
// key itself if the key is not in the known set. Used in scaffold banners
// and README generation.
func LicenseName(key string) string {
	return license.Name(key)
}

// LicenseSPDXID returns the SPDX identifier for a key, or "" when the key is
// not in the known set. Callers templating a package manifest must treat ""
// as a hard failure rather than writing an empty license field.
func LicenseSPDXID(key string) string {
	return license.SPDXID(key)
}

// IsValidLicense reports whether key is a known license (has a bundled
// text). Used by Validate and by internal/config to reject bad --license
// input.
func IsValidLicense(key string) bool {
	return license.IsValid(key)
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
