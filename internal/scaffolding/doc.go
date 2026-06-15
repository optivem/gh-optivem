// Package scaffolding is the documentation marker for the scaffolding module:
// the machinery that materializes a new project from templates. It holds no
// code of its own — the module's behavior lives in three subpackages that form
// its public surface:
//
//	steps     — the ordered scaffolding operations (prepare, project, apply
//	            template, replacements, registration, finalize, verify, ...)
//	templates — template discovery and channel-type resolution
//	files     — file-level copy/replace primitives the steps build on
//
// It backs the `environment` command, which drives these steps to stand up a
// project's source tree and supporting files.
//
// # Dependency direction
//
// scaffolding depends downward only: on internal/kernel/**, internal/config/**,
// and the shared internal/build module (steps -> compiler, runner; see parent
// seam #3). It must not depend on Process or any sibling command module. There
// is no scaffolding-side guard test by repo convention — guards track a
// resolved cycle seam, and the cycle-critical edge (build -> scaffolding) is
// already forbidden by internal/build/import_guard_test.go. The sibling-
// isolation rule is recorded here in prose; broader enforcement, if ever
// wanted, is a separate cross-module pass.
package scaffolding
