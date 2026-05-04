// Package version provides build-time version information.
package version

import "fmt"

// Version is set at build time via -ldflags "-X github.com/optivem/gh-optivem/internal/version.Version=v1.0.0"
var Version = "dev"

// MinGhCLIVersion is the gh CLI floor we test against. Older releases may
// still work but are unsupported — bump this when local/CI gh is upgraded
// and the test suite passes against the new version. Frame to users as the
// "tested-against" floor (npm engines / go.mod-style), not a hard minimum.
const MinGhCLIVersion = "2.92.0"

// ShopRef is the optivem/shop commit SHA baked into this build.
// Set at release time via -ldflags "-X github.com/optivem/gh-optivem/internal/version.ShopRef=<40-char SHA>"
// Empty in dev builds — resolveShopRef() falls back to the latest meta-v* release.
var ShopRef = ""

// ShopTag is an optional human-readable tag (e.g. "v1.0.7-rc.26") pointing at ShopRef.
// Set at release time via -ldflags "-X github.com/optivem/gh-optivem/internal/version.ShopTag=<tag>"
// May be empty if no tag points at the verified SHA.
var ShopTag = ""

// Full returns a multi-line human-readable version string:
//
//	gh-optivem v1.4.2
//	shop:      v1.2.3 (abc1234...)     ← when ShopTag is set
//	shop:      abc1234...              ← when ShopTag is empty but ShopRef is set
//	shop:      HEAD (dev build)        ← when both are empty
func Full() string {
	var shop string
	switch {
	case ShopTag != "" && ShopRef != "":
		shop = fmt.Sprintf("%s (%s)", ShopTag, ShopRef)
	case ShopRef != "":
		shop = ShopRef
	default:
		shop = "HEAD (dev build)"
	}
	return fmt.Sprintf("gh-optivem %s\nshop:      %s", Version, shop)
}
