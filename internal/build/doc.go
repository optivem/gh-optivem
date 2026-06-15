// Package build is the documentation marker for the shared build module: the
// home of the project's compile and system-test execution machinery. It holds
// no code of its own — the module's behavior lives in two subpackages that form
// its public surface:
//
//	compiler — runs source-level compile sequences for a tier
//	runner   — orchestrates docker-compose-backed system tests
//
// # Position in the dependency graph
//
// build sits above the kernel and below every caller that drives a build:
// Scaffolding (via steps), Process (via preflight), and the CLI. Those callers
// may depend on build; build depends downward only.
//
// # The one dependency rule
//
// Files under internal/build/** may import project packages from
// internal/kernel/** and nothing else. This keeps build a leaf shared module:
// it can never reach back up into Scaffolding or Process and turn the shared
// dependency into an import cycle. import_guard_test.go is the backstop that
// enforces this — it walks the whole subtree and fails loudly if any file
// imports a project package outside internal/kernel/.
package build
