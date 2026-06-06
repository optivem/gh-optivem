# Plan: retire the inert `--stubs-path` / `--simulators-path` scaffold flags

## TL;DR

**Why:** Plan `20260606-1356` (fork #1, session 1) made `gh optivem init` stop scaffolding
an `external-systems:` block — operators hand-add per-system entries. That left the
`--stubs-path` / `--simulators-path` flags, their `Default*` consts, the `StubsPath` /
`SimulatorsPath` struct fields, and the matching configinit prompt + test assertions feeding
**no output**. Dead CLI surface students can still see and set.
**End result:** the inert flags, defaults, struct fields, interactive prompt, and their test
assertions are removed; nothing references the symbols; `gh optivem init` behaves identically.

## Why

`buildExternals` / `externalsRepoSlug` were deleted in 1356 session 1, and
`FillRawFlagsFromYAML` no longer reads external-systems — so the values these flags carry now
land nowhere. Leaving them is a teaching-repo smell: a student running `gh optivem init
--help` sees `--stubs-path` / `--simulators-path` flags that silently do nothing, and the
scaffolded config no longer has the block they'd populate.

Removal (not deprecation) is the right call for this repo: teaching repos regenerate configs
and carry **no** legacy-alias / escape-hatch machinery (see the parent plan's "operator adds
the lines" posture and the repo-wide no-legacy doctrine).

## Scope — in-repo

All symbols live in `internal/config/config.go` (+ its tests). Concrete targets (line numbers
are 2026-06-06 starting points — trace each consumer, don't trust the offsets blindly):

- **Flags:** `--stubs-path` (`config.go:540`), `--simulators-path` (`:541`).
- **Default consts:** `DefaultStubsPath` (`:701`), `DefaultSimulatorsPath` (`:702`).
- **Struct fields:** `StubsPath` / `SimulatorsPath` on the flags struct (`:497–498`) and the
  sibling config struct (`:67–68`, comment "apply to both" monolith/multitier).
- **Threading/copies (now dead targets):** `:1124–1125`, `:1353–1354`, and the default-fill
  at `:1383–1387`.
- **Interactive prompt:** the configinit prompt that asks for stubs/simulators paths, if
  present (parity rule — a removed flag must not leave a live prompt).
- **Tests:** the configinit prompt assertions + any flag-parsing test touching these.

## Steps

1. **Precondition — confirm truly inert.** Grep every consumer of `StubsPath` /
   `SimulatorsPath` / `DefaultStubsPath` / `DefaultSimulatorsPath` and confirm each
   terminates in dead code (no output, no file written, no validation). If any live consumer
   remains, STOP — the field is not inert and removal scope is wrong; reassess.
2. Remove the flag registrations, the `Default*` consts, the struct fields, and the
   threading/copy/default-fill lines end-to-end (delete the whole thread, don't relocate).
3. Remove the configinit prompt for these paths and its test assertions.
4. Confirm `gh optivem init --help` no longer lists the flags and `gh optivem init` produces
   the same config it does today (no `external-systems:` block).

## Verification

- `go build ./...`
- `go test ./internal/config/...`
- `gh optivem init --help` — no `--stubs-path` / `--simulators-path`.
- `gh optivem init` round-trip unchanged (no external-systems block scaffolded).

## Cross-references

- Parent plan (made the flags inert): `plans/20260606-1356-external-system-real-kind-simulator-ct.md`
  (residual "Retire the inert scaffold flags" item this plan spins out; fork #1, session 1).
- Symbols: `internal/config/config.go`.
