package projectconfig

// License enum values, surfaced as YAML strings.
const (
	LicenseMIT         = "mit"
	LicenseApache2     = "apache-2.0"
	LicenseGPL3        = "gpl-3.0"
	LicenseBSD2        = "bsd-2-clause"
	LicenseBSD3        = "bsd-3-clause"
	LicenseUnlicense   = "unlicense"
)

// licenseNames maps each accepted license key to its human-readable name.
// Single source of truth — internal/config.Config.LicenseName delegates
// to LicenseName below.
var licenseNames = map[string]string{
	LicenseMIT:       "MIT License",
	LicenseApache2:   "Apache License 2.0",
	LicenseGPL3:      "GNU General Public License v3.0",
	LicenseBSD2:      "BSD 2-Clause License",
	LicenseBSD3:      "BSD 3-Clause License",
	LicenseUnlicense: "The Unlicense",
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
