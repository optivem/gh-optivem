// Package assets exposes the embedded asset tree for Claude Code setup.
//
// It lives at internal/claude/assets, alongside the claude subcommands
// (claude_commands.go at the repo root). Every file here is a Claude asset.
//
// The tree is organized by type:
//
//   - commands/ — Claude slash command definitions (.md), written to
//     ~/.claude/commands/ by `gh optivem claude install`.
//   - config/   — settings.json and CLAUDE.md, merged into ~/.claude/
//     non-destructively by `gh optivem claude configure`.
//
// gh-optivem is the canonical owner of every file in this tree. Users
// install or update by running `gh optivem claude setup` after upgrading
// the extension.
package assets

import "embed"

//go:embed commands config
var FS embed.FS
