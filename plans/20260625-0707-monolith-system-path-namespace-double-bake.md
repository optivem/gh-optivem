# 2026-06-25 07:07:00 UTC — Fix monolith system.path namespace double-bake in scaffolded gh-optivem.yaml

## TL;DR

**Why:** The scaffolded monolith `gh-optivem.yaml` writes `system.path` with the repo name baked in — and doubled (`system/<repo>/<repo>`) — because `buildSystem` joins a `sutNamespace` segment that (a) is applied twice via a `runInit` round-trip and (b) is wrong even once (shop's real `system.path` is the plain code root, no namespace). The new component-test runner is the first consumer to resolve `<system.path>/component-tests.yaml`, so monolith commit-stages now fail with `no such file or directory`.
**End result:** `buildSystem` emits `system.path = cfg.SystemPath` verbatim (matching the flat scaffold layout `system/`, the shop reference `system/monolith/java`, and multitier's un-baked `backend`/`frontend`). The double-bake is gone by construction; monolith java/dotnet/typescript commit-stages find `system/component-tests.yaml`; tests and the path-key doctrine reflect the no-namespace rule.

## Outcomes

What we get out of this — the goals and deliverables:

- A scaffolded monolith repo's `gh-optivem.yaml` has `system.path: system` (no repo-name segment, not doubled), so `<system.path>/component-tests.yaml` resolves to the real `system/component-tests.yaml`.
- The monolith Smoke jobs (`TestValidMonolithConfigurations`, all of java/dotnet/typescript) pass the commit-stage "Set Up / Compile component-tests" step.
- `BuildOptivemYAML` is idempotent for `system.path` — round-tripping a built config back through the flags and re-building yields the same value (no second bake).
- Monolith and multitier are consistent: both write tier paths verbatim from `cfg`, no namespace join.
- Tests and `path-keys.md` doctrine state the correct rule (monolith `system.path` is the verbatim system code root); no stale "sut-namespace baked in" claims remain.
- The dead `sutNamespace` derivation + `lastSlashSegment` helper are removed (or proven still-needed and kept) — no orphaned code.

## ▶ Next executable step (resume here)

Edit `internal/config/optivemyaml/optivemyaml.go`, `buildSystem()` monolith case (lines ~173-177): replace the `sutNamespace` join with `s.Path = cfg.SystemPath`. Then remove the now-unused `sutNamespace` derivation (line ~77), its param on `buildSystem` (line ~89 call + signature ~170), and the `lastSlashSegment` helper (~123-132) — confirm via grep they have no other consumers; keep the `path` import (still used by `javaPackage` at ~86). Update the SSoT comments in `buildSystem`/`BuildOptivemYAML` to say monolith `system.path` is the verbatim code root (no namespace), matching shop and multitier. This unblocks the test + doc updates in Steps 2-4.

## Steps

- [ ] **Step 1 — Fix the bake (`internal/config/optivemyaml/optivemyaml.go`).** In `buildSystem()` monolith case, set `s.Path = cfg.SystemPath` verbatim (drop `path.Join(cfg.SystemPath, sutNamespace)`). Remove the dead `sutNamespace` local (line ~77), the `buildSystem` param threading (call ~89 + signature ~170), and the `lastSlashSegment` helper (~123-132) once grep confirms no other consumers. Keep the `path` import (used by `javaPackage`). Rewrite the SSoT comments to describe the verbatim-code-root rule.
- [ ] **Step 2 — Update `internal/config/optivemyaml/optivemyaml_test.go`.** Fix the assertions that pinned the baked value: line ~52 (`system/shop` → `system`), line ~184 (`system/shop-system` → `system`), line ~214 (`system/monolith/java/shop` → `system/monolith/java`). Update each accompanying SSoT comment. (The `NonScaffoldPaths` case at ~214 keeps its explicit input `system/monolith/java` and now expects it back unchanged.)
- [ ] **Step 3 — Update `config_commands_test.go`.** Line ~99 (`system/monolith/java/page-turner` → `system/monolith/java`); lines ~130-132 (`wantSystemPath: config.DefaultSystemPath + "/page-turner"` → `config.DefaultSystemPath`); refresh the explanatory comments at ~95-98 and ~457 to drop the "sut-namespace baked in" framing.
- [ ] **Step 4 — Correct doctrine in `internal/kernel/projectconfig/path-keys.md`.** Line ~34: change `system-path | system.path (fully resolved, sut-namespace baked in per SSoT)` to describe it as the verbatim system code root (no namespace bake). Fix the stale historical note (~291-302) that claims `config migrate` joins sut-namespace into `system.path` — migrate no longer does this (verified `config_commands.go` only back-fills `db-migration-path`), so note there's no reintroduction path.
- [ ] **Step 5 — Verify.** Run `go test -p 2 ./internal/config/...` (optivemyaml + config_commands round-trip), then a broader `go test -p 2 ./...` (or `scripts/test.sh`). Never run unbounded `go test ./...` on Windows.

## Verification

- `go test -p 2 ./internal/config/...` green (build + the updated round-trip/assertion tests).
- Broader `go test -p 2 ./...` (or `scripts/test.sh`) green — no other consumer regressed by the no-namespace change.
- Operator re-runs the Smoke workflow / `TestValidMonolithConfigurations` and confirms the monolith commit-stage resolves `system/component-tests.yaml` end-to-end (the original failure in run 28127099014 is gone).

## Out of scope (track separately)

- The multitier/typescript Smoke failure in the same run (`ENOENT … db/migrations` during `npm run test:integration`) is an unrelated bug — handle via its own `/fix-bug`.

## Open questions

- **Remove vs. keep `lastSlashSegment`/`sutNamespace`?** Plan assumes full removal once grep confirms `buildSystem` is the only consumer. If a consumer outside this file turns up during execution, keep the derivation and only change the monolith `s.Path` line. (Inferred — confirm during Step 1.)
