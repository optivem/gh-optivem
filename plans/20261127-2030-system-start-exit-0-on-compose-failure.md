# 2026-11-27 20:30 UTC — `gh optivem system start` exits 0 when compose fails

## TL;DR

**Why:** `gh optivem system start` exited 0 even though `docker compose up` had failed (a port conflict). A caller such as a script, an agent or CI therefore reads a stack that never came up as "started". The real cause stays hidden until a later step, like the system tests, fails with a confusing error.
**End result:** Any `docker compose` failure during `system start` (build, up, or health wait) makes the command exit non-zero with a clear `::error::`-style message that names the compose file, the service and the cause.

## Observed (2026-11-27, shop repo)

- Context: shop plan `plans/20261127-1904-frontend-ui-ux-refresh.md`, Step 3. Two agents ran stacks at the same time on one machine.
- Agent A had `step2-java-*` stacks running, which bound host port 8192 (the Keycloak stub).
- Agent B ran `GH_OPTIVEM_CONFIG=gh-optivem-multitier-java.yaml gh optivem system start` for the multitier Java **stub** stack.
- The compose up failed on port 8192 (already allocated), but `gh optivem system start` **exited 0**.
- Not yet root-caused. Code entry point to inspect: `internal/build/runner/system.go`, specifically the `up` and retry path, the `upComposeArgs` / `buildUpArgs` handling, and the log dump when a container fails, around lines 200–240.

## ▶ Next executable step (resume here)

Reproduce:
1. Bind a port the stub stack uses, e.g. `docker run -d -p 8192:80 nginx`.
2. Run `gh optivem system start` with a shop multitier stub config.
3. Check `echo $?`.
4. Trace where the compose error is caught in `internal/build/runner/system.go` and why it isn't returned as an error.

## Steps

- [ ] Step 1: Reproduce as above, and record the exact output and exit code.
- [ ] Step 2: Fix the error propagation so every compose failure (build, up, timeout, health) returns non-zero. Never swallow stderr; surface compose's own error text.
- [ ] Step 3: Add a unit test for the failure path, using the runner's existing fakes in `internal/build/runner/system_test.go`.
- [ ] Step 4 (optional): Check whether a port conflict with another running project could be detected up front and reported clearly, e.g. "port 8192 is held by container X (project Y)".
