// Package assets exposes the single embedded asset tree for gh-optivem.
//
// The tree is organized by consumer (how the asset is delivered):
//
//   - runtime/  — fed to `claude -p` via argv, never written to disk in
//     consumer repos. Holds per-phase prompts under runtime/prompts/atdd/
//     and the shared preamble + session-end bookends under runtime/shared/.
//   - global/   — synced to per-user global paths (~/.gh-optivem/docs/,
//     ~/.claude/agents/, ~/.claude/commands/) by internal/assets/sync.
//     Holds methodology docs and Claude Code interactive subagents.
//
// gh-optivem is the canonical owner of every file in this tree. Consumer
// repos hold zero ATDD assets on disk; updates propagate when the
// gh-optivem binary upgrades.
package assets

import "embed"

//go:embed runtime global
var FS embed.FS
