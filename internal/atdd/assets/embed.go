// Package assets exposes the embedded asset tree for the ATDD process.
//
// It lives at internal/atdd/assets, alongside the ATDD process definition
// (internal/atdd/process) and the Go engine that runs it
// (internal/atdd/runtime). Every file here is an ATDD prompt asset.
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
