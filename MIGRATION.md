# Plan: Configurable Verify Level for Pipeline Stages

## Context

Currently the CLI has a binary `--skip-verify` flag — it either verifies all pipeline stages (commit, acceptance, acceptance-legacy, QA, QA signoff, production, local tests) or none. The user wants granular control over how deep verification goes, organized into tiers:

- **commit** — only verify commit stage
- **acceptance** — commit + acceptance stage + local system tests
- **release** — commit + acceptance + QA + QA signoff + production + local system tests

Plus a separate opt-out `--exclude-legacy` flag for acceptance-stage-legacy (runs by default).

## Changes

### 1. Config struct — `internal/config/config.go`

Add two fields to `Config` (after `SkipVerify` on line 33):

```go
VerifyLevel   string // "none", "commit", "acceptance", "release"
ExcludeLegacy bool   // exclude acceptance-stage-legacy verification
```

### 2. Flag parsing — `internal/config/config.go`

Add two new flags (around line 319, near `skipVerify`):

```go
verifyLevel := flag.String("verify-level", "", "Verification level: none, commit, acceptance, release (default: release)")
excludeLegacy := flag.Bool("include-legacy", false, "Include acceptance-stage-legacy verification")
```

After `flag.Parse()`, resolve the verify level:

- If both `--skip-verify` and `--verify-level` are set → error
- If `--skip-verify` → level = `"none"`
- If `--verify-level` set → validate it's one of none/commit/acceptance/release
- Otherwise → default `"release"`

Set `SkipVerify = (level == "none")` so existing code that reads `cfg.SkipVerify` still works.

Wire into the returned Config struct (around line 487):
```go
SkipVerify:    resolvedLevel == "none",
VerifyLevel:   resolvedLevel,
ExcludeLegacy: *excludeLegacy,
```

### 3. Step sequence — `main.go` (lines 96-108)

Replace the `if !cfg.SkipVerify` block with tier-based logic:

```go
if cfg.VerifyLevel != "none" {
    // commit tier
    allSteps = append(allSteps,
        stepDef{"Verify commit stage", func() { steps.VerifyCommitStage(cfg, gh) }},
    )

    if cfg.VerifyLevel == "acceptance" || cfg.VerifyLevel == "release" {
        // acceptance tier
        allSteps = append(allSteps,
            stepDef{"Verify acceptance stage", func() { steps.VerifyAcceptanceStage(cfg, gh) }},
        )
        if !cfg.ExcludeLegacy {
            allSteps = append(allSteps,
                stepDef{"Verify acceptance stage legacy", func() { steps.VerifyAcceptanceStageLegacy(cfg, gh) }},
            )
        }
        allSteps = append(allSteps,
            stepDef{"Run local system tests", func() { steps.RunLocalSystemTests(cfg) }},
        )
    }

    if cfg.VerifyLevel == "release" {
        // release tier
        allSteps = append(allSteps,
            stepDef{"Verify QA stage", func() { steps.VerifyQAStage(cfg, gh) }},
            stepDef{"Verify QA signoff", func() { steps.VerifyQASignoff(cfg, gh) }},
            stepDef{"Verify production stage", func() { steps.VerifyProdStage(cfg, gh) }},
        )
    }
} else {
    log.Logf("Skipping workflow verification (--skip-verify / --verify-level none)")
}
```

### 4. Test helper — `internal/config/config_system_test.go`

Add a `verifyFlags()` helper (mirrors `cleanupFlags()` pattern) that reads `TEST_VERIFY_LEVEL` env var:

```go
func verifyFlags() []string {
    level := os.Getenv("TEST_VERIFY_LEVEL")
    if level != "" {
        return []string{"--verify-level", level}
    }
    return nil
}
```

Wire into `withBase()`:
```go
func withBase(extra ...string) []string {
    args := []string{"--owner", testOwner()}
    args = append(args, baseArgs...)
    args = append(args, extra...)
    args = append(args, cleanupFlags()...)
    args = append(args, verifyFlags()...)
    return args
}
```

### 5. GitHub Actions workflow — `.github/workflows/acceptance-stage.yml`

Add `TEST_VERIFY_LEVEL` to the env block of the "System test" step (line 88+):

```yaml
env:
  TEST_VERIFY_LEVEL: acceptance
  GH_TOKEN: ${{ secrets.VERIFY_TOKEN }}
  # ... rest unchanged
```

This makes the acceptance-stage workflow verify up to the acceptance tier (not full release).

## Files to modify

| File | What changes |
|---|---|
| `internal/config/config.go` | Add `VerifyLevel`, `ExcludeLegacy` fields + flag parsing + validation. Legacy runs by default. |
| `main.go` | Replace lines 96-108 with tier-based step assembly |
| `internal/config/config_system_test.go` | Add `verifyFlags()` helper, wire into `withBase()` |
| `.github/workflows/acceptance-stage.yml` | Add `TEST_VERIFY_LEVEL: acceptance` env var |

## Backward compatibility

- `--skip-verify` continues to work (maps to `--verify-level none`)
- No flags = `--verify-level release` + legacy = current full behavior exactly
- Using both `--skip-verify` and `--verify-level` → clear error message

## Verification

1. `go build .` — compiles
2. `go test ./internal/config/` — unit tests pass (TestInvalidConfigurations)
3. Manual dry-run checks:
   - `gh-optivem init --dry-run --verify-level commit ...` → shows only commit stage step
   - `gh-optivem init --dry-run --verify-level acceptance ...` → shows commit + acceptance + local tests
   - `gh-optivem init --dry-run --verify-level release ...` → shows all stages
   - `gh-optivem init --dry-run --verify-level acceptance --exclude-legacy ...` → shows acceptance without legacy
   - `gh-optivem init --dry-run --skip-verify --verify-level commit ...` → error
