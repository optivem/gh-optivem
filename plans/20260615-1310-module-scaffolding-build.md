# 2026-06-15 13:10:00 UTC — Child #5: Scaffolding module + shared `build` module

> **Child plan** of `20260615-0548-gh-optivem-modular-monolith-parent.md`. The package *moves* are already done (`internal/scaffolding/{steps,templates,files}`, `internal/build/{compiler,runner}`). This child **formalizes the boundary** and resolves the parent's two still-open seams — #3 `steps → compiler, runner` and #4 `preflight → runner` — by pinning where the shared `build` module formally sits so Scaffolding *and* Process both depend on it **downward, without a cycle**. Regression-safe: no behavior change, additive only (package docs + guard tests + parent bookkeeping).

## TL;DR

**Why:** The parent decomposition left `runner` + `compiler` physically moved into `internal/build/` but never *formalized* as a module: there is no documented public surface, no dependency rule, and no backstop preventing `build` from one day reaching up into Scaffolding or Process and creating a cycle. Seams #3 and #4 are still listed "open" in the parent even though the physical edges are already correct.

**End result:** `internal/build` is a documented shared module sitting **above the kernel, below both Scaffolding and Process**, with a `package build` doc marker stating its public surface (`compiler`, `runner`) and its one dependency rule (*may import only `internal/kernel/**`*), enforced by a build-level guard test that fails loudly if any file under `internal/build/**` imports anything outside the kernel. Scaffolding's module boundary is documented the same way. Parent seams #3 and #4 are marked **resolved** (legal downward edges), child #5 is marked done, and — as the last child — the parent's resume block flips to "all children complete." `go build ./...` and `go test ./...` stay green throughout.

## Outcomes

What we get out of this — the goals and deliverables:

- **`build` is a real module, not just a folder.** A `package build` doc marker (`internal/build/doc.go`) names its public surface (`compiler`, `runner`), states *who may depend on it* (Scaffolding, Process, CLI) and *what it may depend on* (kernel only), so the boundary is discoverable from the code, not just the parent plan.
- **A cycle backstop exists.** `internal/build/import_guard_test.go` walks the whole `internal/build/**` subtree and fails if any file imports a project package outside `internal/kernel/**`. This is what guarantees `build` can never grow a back-edge into Scaffolding/Process and turn the shared dependency into a cycle.
- **Scaffolding's boundary is documented** the same way (a `package scaffolding` doc marker naming `steps`/`templates`/`files` as the surface and `environment` as the command), so the two modules touched here are both self-describing.
- **Seam #3 (`steps → compiler, runner`) and seam #4 (`preflight → runner`) are formally resolved** in the parent's "Cross-module seams" list — recorded as legal downward edges onto the shared `build` module, with the guard test cited as the thing that keeps them legal.
- **The parent plan is reconciled:** child #5 marked ✅ done in the child list and confirmed-order line; since #5 is the last remaining child, the **▶ Next executable step** block flips to "decomposition complete — no children remain."
- **Zero behavior change.** Everything added is documentation + tests + plan bookkeeping. No production `.go` logic, no import-path edits, no moves. `go build ./...` and `go test ./...` green.

## ▶ Next executable step (resume here)

**Step 1 — add the `build` module doc marker + cycle-guard test.** Create `internal/build/doc.go` containing only a package comment and `package build` (a documentation-only marker package alongside the existing `compiler`/`runner` subpackages); the comment states: surface = `compiler` + `runner`; allowed dependents = Scaffolding, Process, CLI; allowed dependencies = `internal/kernel/**` only. Then create `internal/build/import_guard_test.go` (`package build`) modeled on `internal/config/import_guard_test.go` (AST `parser.ParseFile(..., parser.ImportsOnly)`), but **walking the subtree** (`filepath.WalkDir` from `.` over `compiler/` and `runner/`, skipping `_test.go` files) and asserting every `github.com/optivem/gh-optivem/internal/...` import is under `internal/kernel/`. Gate: `go test ./internal/build/...` green. This unblocks the seam-resolution bookkeeping (Steps 3–4), which only make sense once the guard that *makes* the seams safe actually exists.

> Verify the chosen marker-package shape compiles first: a `doc.go` with `package build` in a directory whose only other contents are subpackages creates a standalone empty `build` package. Confirm `go build ./internal/build/...` accepts it before writing the guard test; if not, fall back to per-subpackage `compiler/doc.go` + `runner/doc.go` and a guard test in each (see Open questions).

## Steps

- [ ] **Step 1: `build` doc marker + cycle-guard test.** **Decided: single `internal/build/doc.go` (`package build`, doc-only) + one walking guard.** Create the marker package, then `internal/build/import_guard_test.go` (`package build`) walking `internal/build/**`, asserting project imports stay within `internal/kernel/**`. Gate: `go build ./internal/build/...` then `go test ./internal/build/...` green. *(The build gate confirms the empty marker package compiles; only if Go rejects it, fall back to per-subpackage `compiler/doc.go` + `runner/doc.go` with a guard in each — see Open questions.)*
- [ ] **Step 2: Scaffolding boundary doc.** Add a `package scaffolding` doc marker (`internal/scaffolding/doc.go`) naming the public surface (`steps`, `templates`, `files`), the command it backs (`environment`), and its allowed downward deps (kernel, config, **build**). **Decided: no scaffolding-side guard** — the sibling rule is recorded in prose here; enforcement stays scoped to the `build` cycle-guard (Step 1), per the repo convention that guards track a *resolved* seam. Gate: `go build ./internal/scaffolding/...` green.
- [ ] **Step 3: Resolve seams #3 and #4 in the parent.** In `20260615-0548-...-parent.md` "Cross-module seams", rewrite items 3 and 4 from open descriptions to ✅ **resolved**: both are legal downward edges onto the shared `internal/build` module (Scaffolding via `steps`, Process via `preflight`); cite `internal/build/import_guard_test.go` as the backstop that keeps `build` from reaching back up. Targeted `Edit`s only.
- [ ] **Step 4: Mark child #5 done in the parent.** Update the child-list entry (#5) and the "Confirmed child order" line to ✅ done, and replace the parent's **▶ Next executable step** block with "modular-monolith decomposition complete — no children remain" (or the next real follow-up if one surfaced). Targeted `Edit`s only.
- [ ] **Step 5: Full green + commit.** `go build ./...` and `go test ./...` from the repo root. Then commit via the commit skill (`/commit`) — this child touches only `gh-optivem` (new docs/tests) and the parent plan file.

## Resolved decisions

- **Marker-package shape → single `internal/build/doc.go` (`package build`) + one walking guard.** An empty doc-only parent package coexisting with the `compiler`/`runner` subpackages is valid Go; it gives one home for the module doc and one guard covering the whole subtree (no duplicated guard files that drift). Fallback only if Step 1's `go build` rejects it: per-subpackage `compiler/doc.go` + `runner/doc.go` with the import guard in **both**.
- **Scaffolding-side guard → skip.** The `build` cycle-guard (Step 1) already forbids `build → scaffolding`, which is the cycle-critical edge for seams #3/#4. A speculative `atdd/** ✗→ scaffolding/**` guard corresponds to no seam resolved here and nothing violates it today; per repo convention guards track a *resolved* seam, so the sibling rule is recorded in prose (Step 2 doc) only. Full sibling-isolation enforcement, if ever wanted, is a separate cross-module pass.

## Open questions

- **Guard scope for `build`.** Plan asserts "kernel-only" for project imports. Confirmed clean today: `compiler → kernel/{projectconfig,shell}`, `runner → kernel/{pathx,spinner}`. If a future legitimate dep appears, the guard's allowlist is the single place to widen it.
