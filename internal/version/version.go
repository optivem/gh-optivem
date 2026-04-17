// Package version provides build-time version information.
package version

// Version is set at build time via -ldflags "-X github.com/optivem/gh-optivem/internal/version.Version=v1.0.0"
var Version = "dev"

// StarterRef is the optivem/starter commit SHA baked into this build.
// Set at release time via -ldflags "-X github.com/optivem/gh-optivem/internal/version.StarterRef=<40-char SHA>"
// Empty in dev builds — cloneStarter() falls back to HEAD of default branch.
var StarterRef = ""

// StarterTag is an optional human-readable tag (e.g. "v1.0.7-rc.26") pointing at StarterRef.
// Set at release time via -ldflags "-X github.com/optivem/gh-optivem/internal/version.StarterTag=<tag>"
// May be empty if no tag points at the verified SHA.
var StarterTag = ""
