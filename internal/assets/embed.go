// Package assets exposes the single embedded asset tree for gh-optivem.
//
// The tree is organized by delivery mechanism:
//
//   - runtime/agents/     — fed to `claude -p` via argv, never written to
//     disk in consumer repos. Per-phase agent definitions under runtime/agents/atdd/.
//   - runtime/shared/     — argv-injected preamble + scope rule, prepended
//     to every agent prompt.
//
// gh-optivem is the canonical owner of every file in this tree. Consumer
// repos hold zero ATDD assets on disk; updates propagate when the
// gh-optivem binary upgrades.
package assets

import "embed"

//go:embed runtime
var FS embed.FS
