# ACCEPTANCE TEST CYCLE

## Overview

RED - GREEN - REFACTOR

## PRE

See **[Acceptance Criteria Analysis](../../analysis/acceptance-criteria-analysis.md)**.

## RED

Between RED sub-phases, change-driven tests are disabled (and re-enabled at the start of the next phase) per [§Conventions → Disable-reason convention](../../shared/conventions.md#disable-reason-convention). This bookkeeping is handled outside the phase agent — phase agents must not annotate or strip `@Disabled` themselves.

The RED loop runs three sequential phases — see each per-phase doc for instructions:

1. **[AT - RED - TEST](at-red-test.md)**
2. **[AT - RED - DSL](at-red-dsl.md)**
3. **[AT - RED - SYSTEM DRIVER](at-red-system-driver.md)**

### RED: External System Driver

1. Go to the ATDD - CT Cycle ([`ct/ct-cycle.md`](ct/ct-cycle.md)).

## GREEN

See **[AT - GREEN - SYSTEM](at-green-system.md)**.

## REFACTOR

See **[AT - REFACTOR](at-refactor.md)**.

## Conventions

See **[shared/conventions.md](../../shared/conventions.md)**.
