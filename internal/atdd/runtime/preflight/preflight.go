// Package preflight validates that the consumer's gh-optivem.yaml maps
// onto a real on-disk layout and a real remote setup before any ATDD
// agent or board work runs. It is the runtime backstop for "did the
// operator's directories actually match the schema, and do the services
// the schema names actually exist?" — the answer must be yes before
// implement-ticket dispatches anything.
//
// Three classes of check:
//
//  1. Repo-level local. Every slug in cfg.Repos() must resolve (via
//     repolocator) to a path that exists, is a directory, and contains
//     a `.git` entry.
//  2. Tier-level local. Every populated tier (system or backend+frontend,
//     plus system-test, plus declared external-systems) must have its
//     `path` join cleanly with its host repo's local clone.
//  3. Remote (optional). When the corresponding Options field is non-nil,
//     also verify that every repo slug exists on GitHub, every declared
//     sonar org + project key exists on SonarCloud, and the project board
//     URL resolves. nil fields mean "skip that class" — tests inject
//     fakes; production wires real clients in the cobra layer.
//
// Failures are aggregated — preflight does not return on the first error.
// The operator gets one multi-line error block listing every missing item.
package preflight

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/optivem/gh-optivem/internal/atdd/process/actions"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/repolocator"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/testselect"
	"github.com/optivem/gh-optivem/internal/build/runner"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// Options bundles the optional inputs to Run. All four remote-check
// fields default to nil = skip; production wires them via the cobra
// layer (see runImplementTicketPreflight, runConfigPreflight). Tests
// inject fakes per scenario without dragging in real network clients.
//
// Function-typed fields beat single-method interfaces here because each
// remote check is exactly one call — wrapping it in an interface adds
// boilerplate without buying polymorphism.
type Options struct {
	// Workspace is the operator-supplied workspace root (from the
	// --workspace flag). When empty, repolocator defaults to
	// filepath.Dir(cwd).
	Workspace string

	// Cwd is the working directory used by the default branch of the
	// resolver. When empty, the process CWD is used.
	Cwd string

	// RepoExists reports whether a GitHub repo slug (owner/name) is
	// visible to the authenticated gh CLI. nil = skip the repo-existence
	// remote check.
	RepoExists func(ctx context.Context, slug string) (bool, error)

	// SonarOrgExists reports whether a SonarCloud organization with the
	// given key exists. nil = skip the org check (and per-tier project
	// checks; without an org to anchor them they have no remote contract
	// to verify).
	SonarOrgExists func(ctx context.Context, key string) (bool, error)

	// SonarProjectExists reports whether a SonarCloud project with the
	// given key exists. nil = skip the project-existence remote check.
	SonarProjectExists func(ctx context.Context, key string) (bool, error)

	// BoardURLOK verifies that cfg.Project.URL resolves and is visible
	// to the authenticated gh CLI. nil = skip the board-URL check.
	BoardURLOK func(ctx context.Context, projectURL string) error

	// Engine is the loaded ATDD state machine (process-flow.yaml). When
	// non-nil, preflight runs two engine-derived sweeps against cfg: the
	// scope-resolution sweep (every writing-agent MID's inline `read:` /
	// `write:` scope lists through actions.ResolveLayerPaths, surfacing any
	// unresolvable layer — missing Family A switch case, blank cfg path,
	// unknown Family B key) and the suite-existence sweep (every `suite:`
	// literal the flow requests, expanded to concrete per-channel ids and
	// checked against tests.yaml). Both surface at startup instead of mid-run
	// — the scope sweep inside validate-outputs-and-scopes, the suite check
	// inside runner.selectSuites after agents have already committed.
	//
	// Both `gh optivem implement` and `gh optivem config preflight` wire the
	// embedded default engine here (via defaultPreflightOptions), so the two
	// surfaces validate against one definition of "ready to implement". nil =
	// skip both sweeps — the function-level contract tests rely on this, and
	// any future caller that wants the structural-only checks can pass nil.
	Engine *statemachine.Engine

	// MissingEnvVars, when non-nil, returns the names of every required
	// credential environment variable that is currently unset. Each name
	// becomes one aggregated failure line, so a missing token folds into the
	// same error block as missing repos/tiers/suites — the operator sees
	// every gap in one pass and fixes them with a single shell restart
	// instead of fix-one-restart-discover-next. nil = skip (tests and any
	// caller with no env-var contract). Runs even on a nil cfg — credential
	// presence is independent of project layout. Production wires
	// config.MissingRequiredEnvVars via defaultPreflightOptions.
	MissingEnvVars func() []string

	// ClaudeCheck verifies the `claude` CLI is on PATH and runnable.
	// nil = skip (used by the v1 --manual-agents fallback that doesn't
	// need the CLI, and by config-flow callers that don't dispatch
	// agents). Production callers point at preflight.VerifyClaude;
	// tests inject canned-result stubs. Same nil=skip convention as
	// the remote-check fields above so a failed claude check folds
	// into the aggregated error block alongside any missing repos or
	// tier directories. Runs even on a nil cfg — claude readiness is
	// independent of project layout.
	ClaudeCheck func(ctx context.Context) error
}

// Run validates cfg's declared layout (local FS) and optionally its
// remote setup (GitHub repos, SonarCloud org + projects, project board
// URL). Returns nil when everything checks out. Otherwise returns a
// single error whose Error() lists every failure on its own line,
// prefixed with "  - ". Callers print it directly to stderr and exit
// non-zero.
//
// A nil cfg skips the structural checks (nothing to compare against)
// but still runs Options.ClaudeCheck when set, since claude readiness is
// independent of project layout.
func Run(ctx context.Context, cfg *projectconfig.Config, opts Options) error {
	var failures []string

	// Required env-var presence. Runs before any cfg-dependent work so it
	// surfaces even on a nil cfg, and folds into the same aggregated error
	// block as the structural failures below — one restart fixes every
	// missing credential at once.
	if opts.MissingEnvVars != nil {
		for _, name := range opts.MissingEnvVars() {
			failures = append(failures, fmt.Sprintf("environment variable %s is not set (required)", name))
		}
	}

	// Local-tool check: claude CLI presence. Runs before any cfg-dependent
	// work so its failure surfaces even on a nil cfg, and folds into the
	// same aggregated error block as the structural failures below.
	if opts.ClaudeCheck != nil {
		if err := opts.ClaudeCheck(ctx); err != nil {
			failures = append(failures, err.Error())
		}
	}

	if cfg == nil {
		return aggregateFailures(failures)
	}
	res, err := repolocator.Resolve(cfg, opts.Workspace, opts.Cwd)
	if err != nil {
		return fmt.Errorf("preflight: resolve repos: %w", err)
	}

	// Repo-level local checks.
	slugs := cfg.Repos()
	for _, slug := range slugs {
		path, ok := res.Local[slug]
		if !ok || path == "" {
			failures = append(failures, fmt.Sprintf("repo %s: no local path resolved", slug))
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				failures = append(failures, fmt.Sprintf("repo %s: local clone %s does not exist", slug, path))
			} else {
				failures = append(failures, fmt.Sprintf("repo %s: stat %s: %v", slug, path, err))
			}
			continue
		}
		if !info.IsDir() {
			failures = append(failures, fmt.Sprintf("repo %s: %s is not a directory", slug, path))
			continue
		}
		if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
			failures = append(failures, fmt.Sprintf("repo %s: %s is not a git repository (no .git entry)", slug, path))
		}
	}

	// Tier-level local checks. Each tier has a (field-name, repo-slug,
	// path) triple; we look up the host repo's local clone and join the
	// tier's path onto it.
	tiers := collectTiers(cfg)
	for _, tier := range tiers {
		hostPath, ok := res.Local[tier.repo]
		if !ok || hostPath == "" {
			// Already reported above as a repo-level failure; skip
			// the tier-level check to avoid double-counting.
			continue
		}
		full := filepath.Join(hostPath, tier.path)
		info, err := os.Stat(full)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				failures = append(failures, fmt.Sprintf("%s: %s does not exist", tier.field, full))
			} else {
				failures = append(failures, fmt.Sprintf("%s: stat %s: %v", tier.field, full, err))
			}
			continue
		}
		if !info.IsDir() {
			failures = append(failures, fmt.Sprintf("%s: %s is not a directory", tier.field, full))
		}
	}

	// Remote checks (optional; only run when the corresponding Options
	// field is non-nil).
	failures = append(failures, runRepoExistsChecks(ctx, slugs, opts.RepoExists)...)
	failures = append(failures, runSonarChecks(ctx, cfg, opts)...)
	failures = append(failures, runBoardURLCheck(ctx, cfg, opts.BoardURLOK)...)

	// Scope-resolution sweep (optional; only runs when an engine is wired).
	failures = append(failures, runScopeResolutionChecks(cfg, opts.Engine)...)

	// Suite-existence sweep (optional; only runs when an engine is wired).
	// Resolve the project's tests.yaml against the system-test repo's local
	// clone. An empty path — no system-test.config declared, or its repo
	// didn't resolve above (already reported as a repo-level failure) — skips
	// the check.
	testsYAML := ""
	if cfg.SystemTest.Config != "" {
		if hostPath := res.Local[cfg.SystemTest.Repo]; hostPath != "" {
			testsYAML = filepath.Join(hostPath, cfg.SystemTest.Config)
		}
	}
	failures = append(failures, runSuiteExistenceChecks(cfg, opts.Engine, testsYAML)...)

	// Effective-suite resolution sweep (optional; only runs when an engine is
	// wired). Independent of tests.yaml: it validates that every `${suite}`
	// placeholder is statically resolvable through the call graph, not that the
	// resolved ids are declared (runSuiteExistenceChecks owns that).
	failures = append(failures, runEffectiveSuiteResolutionChecks(opts.Engine)...)

	return aggregateFailures(failures)
}

// runScopeResolutionChecks iterates every writing-agent MID in the loaded
// state machine and resolves each MID's `read:` / `write:` layer lists
// against cfg via actions.ResolveLayerPaths. Any layer that fails to
// resolve — Family A key missing a switch case, blank cfg path,
// unknown Family B key — becomes one failure line, prefixed with the MID
// name and list (`read`/`write`) so the operator can locate the drift.
//
// nil engine = skip (matches the existing nil-check convention for
// optional check classes). Templated task-names on the `fix` LOW
// (`task-name: "fix-${failure-kind}"`) are skipped because their scope is
// inherited from `originating-task-name` at runtime and they have no
// concrete MID identity of their own.
func runScopeResolutionChecks(cfg *projectconfig.Config, eng *statemachine.Engine) []string {
	if eng == nil || cfg == nil {
		return nil
	}
	var failures []string
	for processName, proc := range eng.Processes {
		for _, node := range proc.Nodes {
			if node.Kind != statemachine.CallActivity || node.Raw.Process != "execute-agent" {
				continue
			}
			task := node.Raw.Params["task-name"]
			if task == "" || strings.Contains(task, "${") {
				continue
			}
			for _, list := range []struct {
				name   string
				layers []string
			}{
				{"read", node.Raw.Read},
				{"write", node.Raw.Write},
			} {
				if len(list.layers) == 0 {
					continue
				}
				if _, err := actions.ResolveLayerPaths(list.layers, cfg); err != nil {
					failures = append(failures, fmt.Sprintf("MID %s %s scope: %v", processName, list.name, err))
				}
			}
			break
		}
	}
	return failures
}

// runSuiteExistenceChecks validates that every test-suite id the ATDD
// process flow will request via `gh optivem test run --suite=…` actually
// exists in the project's tests.yaml — surfacing a renamed or missing suite
// at preflight time instead of deep inside the pipeline (runner.selectSuites)
// after agents have already done work and committed.
//
// The expected set is derived deterministically, never invented: the distinct
// `suite:` literals declared on the loaded engine's nodes, expanded to
// concrete per-channel ids against cfg, then filtered to the suites the
// project can actually reach (contract suites only when external systems are
// configured — they live solely on the external-driver-port-changed branch a
// no-external project never takes). nil engine / cfg, or an empty
// testsYAMLPath (no system-test.config declared, or its repo didn't resolve),
// skips the check — matching the nil-skip convention of the other optional
// check classes. A tests.yaml that fails to load is itself surfaced as one
// failure line naming the path.
func runSuiteExistenceChecks(cfg *projectconfig.Config, eng *statemachine.Engine, testsYAMLPath string) []string {
	if eng == nil || cfg == nil || testsYAMLPath == "" {
		return nil
	}
	literals := collectSuiteLiterals(eng)
	if len(literals) == 0 {
		return nil
	}
	tests, err := runner.LoadTests(testsYAMLPath)
	if err != nil {
		// LoadTests already names the path in its wrapped error, so the
		// field-name prefix alone keeps the line from repeating the path twice.
		return []string{fmt.Sprintf("system-test.config: %v", err)}
	}
	declared := make(map[string]bool, len(tests.Suites))
	for _, id := range tests.SuiteIDs() {
		declared[id] = true
	}

	hasExternal := len(cfg.ExternalSystems) > 0

	expected := make(map[string]bool)
	for _, lit := range literals {
		if strings.HasPrefix(lit, "contract") && !hasExternal {
			continue
		}
		// Channel-aware expansion through the SAME source the runtime/CLI use
		// (testselect.ExpandSuiteGroups): the `acceptance` alias unrolls to
		// acceptance-parallel-<ch> + acceptance-isolated-<ch> per cfg.Channels, every
		// other literal passes through (project-declared group aliases resolve
		// via tests.SuiteGroups). One function = preflight validates exactly
		// the ids the runtime's `--suite=` emission requests.
		for _, id := range testselect.ExpandSuiteGroups([]string{lit}, tests.SuiteGroups, cfg.Channels) {
			expected[id] = true
		}
	}

	var failures []string
	for id := range expected {
		if !declared[id] {
			failures = append(failures, fmt.Sprintf(
				"tests suite %q is required by the ATDD process flow but not declared in %s; available: %s",
				id, testsYAMLPath, strings.Join(tests.SuiteIDs(), ", ")))
		}
	}
	sort.Strings(failures)
	return failures
}

// collectSuiteLiterals returns the distinct, concrete `suite:` param values
// declared across every node in the engine, in sorted order. Mirrors the
// node-walk shape of runScopeResolutionChecks. Skips the explicit-empty
// verify-noop sentinel ("") and any value carrying a ${…} placeholder
// (runtime-resolved; its concrete value originates at the literal call site
// the sweep already sees).
func collectSuiteLiterals(eng *statemachine.Engine) []string {
	seen := map[string]bool{}
	var out []string
	for _, proc := range eng.Processes {
		for _, node := range proc.Nodes {
			suite := node.Raw.Params["suite"]
			if suite == "" || strings.Contains(suite, "${") {
				continue
			}
			if seen[suite] {
				continue
			}
			seen[suite] = true
			out = append(out, suite)
		}
	}
	sort.Strings(out)
	return out
}

// runEffectiveSuiteResolutionChecks is the placeholder-coverage half of the
// suite-existence sweep. collectSuiteLiterals validates the concrete `suite:`
// literals but deliberately skips `${…}` placeholders; this pass closes that
// gap by walking the engine's call graph from its root processes, threading
// each call-activity's resolved param environment down into the callee exactly
// as the runtime's ExpandParams params-chain does, and flagging any node whose
// `suite` param cannot be resolved to a concrete value from static call-site
// params — i.e. one that bottoms out in runtime state.
//
// That is the precise failure that let `--suite=contract` reach the runner
// (plan 20260606-1458): a `${suite}` placeholder resolved at run time from a
// value no static call site bound, the runner rejected it ("suite(s) not
// found"), and the never-ran command was mis-read as a red test that spun the
// fixer for hours. Surfacing it here turns that into a sub-second startup error.
//
// Resolvable placeholders are NOT re-validated against tests.yaml here — every
// value they can resolve to is itself a `suite:` literal bound at some call
// site, which collectSuiteLiterals already checks. nil engine = skip, matching
// the nil-skip convention of the other optional check classes.
func runEffectiveSuiteResolutionChecks(eng *statemachine.Engine) []string {
	if eng == nil {
		return nil
	}
	// Root processes = those never targeted by a call-activity. The walk
	// starts there with an empty env; concrete `suite:` values are bound at
	// intermediate call sites and propagate down through `${suite}` forwarders.
	called := map[string]bool{}
	for _, proc := range eng.Processes {
		for _, node := range proc.Nodes {
			if node.Raw.Process != "" {
				called[node.Raw.Process] = true
			}
		}
	}

	failures := map[string]bool{}
	visited := map[string]bool{}
	var walk func(procName string, env map[string]string)
	walk = func(procName string, env map[string]string) {
		proc, ok := eng.Processes[procName]
		if !ok {
			return
		}
		// Memoise on (process, effective suite): the suite param is the only
		// value this sweep cares about, so two invocations that agree on it
		// explore identically. Bounds the fix-loop / shared-subprocess fan-in.
		sig := procName + "\x00" + env["suite"]
		if visited[sig] {
			return
		}
		visited[sig] = true

		for _, node := range proc.Nodes {
			if raw, ok := node.Raw.Params["suite"]; ok {
				if _, resolved := resolveParam(raw, env); !resolved {
					failures[fmt.Sprintf(
						"node %s in process %q requests a test suite that is not statically resolvable (%q resolves only from runtime state); every verify call site must bind a concrete suite so a bad suite cannot reach the runner",
						node.ID, procName, raw)] = true
				}
			}
			if node.Raw.Process != "" {
				childEnv := map[string]string{}
				for k, v := range node.Raw.Params {
					if rv, ok := resolveParam(v, env); ok {
						childEnv[k] = rv
					}
					// Unresolved params are omitted so the callee sees the key
					// as absent (still unresolved) rather than as empty-bound.
				}
				walk(node.Raw.Process, childEnv)
			}
		}
	}

	roots := make([]string, 0, len(eng.Processes))
	for name := range eng.Processes {
		if !called[name] {
			roots = append(roots, name)
		}
	}
	sort.Strings(roots) // deterministic walk order
	for _, name := range roots {
		walk(name, map[string]string{})
	}

	out := make([]string, 0, len(failures))
	for f := range failures {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// resolveParam substitutes ${k} occurrences in a YAML param value against env,
// the same params-chain ExpandParams uses (state fallback excluded — that is
// exactly the dynamic resolution this sweep refuses to simulate). A literal
// (including the empty-string verify-noop sentinel) resolves to itself. A value
// with a ${…} whose key is absent from env is reported unresolved.
func resolveParam(raw string, env map[string]string) (string, bool) {
	if !strings.Contains(raw, "${") {
		return raw, true
	}
	out := raw
	for k, v := range env {
		out = strings.ReplaceAll(out, "${"+k+"}", v)
	}
	if strings.Contains(out, "${") {
		return "", false
	}
	return out, true
}

// aggregateFailures returns nil when failures is empty; otherwise a
// single error whose Error() lists every entry on its own bulleted line
// in sorted order. Centralised so the nil-cfg short-circuit path and
// the full structural path use one format.
func aggregateFailures(failures []string) error {
	if len(failures) == 0 {
		return nil
	}
	sort.Strings(failures)
	return fmt.Errorf("preflight failed:\n  - %s", strings.Join(failures, "\n  - "))
}

// runRepoExistsChecks calls RepoExists once per slug. A nil checker
// means the operator opted out; the call is a no-op.
func runRepoExistsChecks(ctx context.Context, slugs []string, check func(context.Context, string) (bool, error)) []string {
	if check == nil {
		return nil
	}
	var failures []string
	for _, slug := range slugs {
		ok, err := check(ctx, slug)
		if err != nil {
			failures = append(failures, fmt.Sprintf("repo %s: remote check failed: %v", slug, err))
			continue
		}
		if !ok {
			failures = append(failures, fmt.Sprintf("repo %s: does not exist on GitHub (or not visible to your gh auth)", slug))
		}
	}
	return failures
}

// runSonarChecks verifies the SonarCloud org + every declared per-tier
// project key. Org check gates the per-tier checks: if the org isn't
// there, the project keys can't exist either and the per-tier failures
// would just be noise.
func runSonarChecks(ctx context.Context, cfg *projectconfig.Config, opts Options) []string {
	if opts.SonarOrgExists == nil && opts.SonarProjectExists == nil {
		return nil
	}
	if cfg.Sonar.Organization == "" {
		// Validate already rejects empty org when arch is set, so the
		// only way to land here is a partial config where there's
		// nothing meaningful to check remotely.
		return nil
	}
	var failures []string
	orgOK := true
	if opts.SonarOrgExists != nil {
		ok, err := opts.SonarOrgExists(ctx, cfg.Sonar.Organization)
		if err != nil {
			failures = append(failures, fmt.Sprintf("sonar.organization %s: remote check failed: %v", cfg.Sonar.Organization, err))
			orgOK = false
		} else if !ok {
			failures = append(failures, fmt.Sprintf("sonar.organization %s: does not exist on SonarCloud", cfg.Sonar.Organization))
			orgOK = false
		}
	}
	if !orgOK || opts.SonarProjectExists == nil {
		return failures
	}
	for _, sp := range collectSonarProjects(cfg) {
		ok, err := opts.SonarProjectExists(ctx, sp.key)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s %s: remote check failed: %v", sp.field, sp.key, err))
			continue
		}
		if !ok {
			failures = append(failures, fmt.Sprintf("%s %s: does not exist on SonarCloud", sp.field, sp.key))
		}
	}
	return failures
}

// runBoardURLCheck verifies cfg.Project.URL resolves via the gh CLI.
// Empty project.url is accepted at validate-time (Rule 9 in
// projectconfig.Validate) — preflight respects that: with no URL set,
// there's nothing to verify here. The ATDD runtime still re-checks
// presence at board-resolution time.
func runBoardURLCheck(ctx context.Context, cfg *projectconfig.Config, check func(context.Context, string) error) []string {
	if check == nil || cfg.Project.URL == "" {
		return nil
	}
	if err := check(ctx, cfg.Project.URL); err != nil {
		return []string{fmt.Sprintf("project.url %s: %v", cfg.Project.URL, err)}
	}
	return nil
}

// tierCheck packages one populated tier for the per-tier loop. Field is
// the YAML path used in error messages (e.g. "system.backend.path");
// repo is the slug whose local clone hosts the tier; path is the
// repo-relative directory.
type tierCheck struct {
	field string
	repo  string
	path  string
}

// collectTiers returns every populated tier in cfg, in a deterministic
// order suitable for stable error output. The trailing `system-test.paths.*`
// entries are the eight canonical Family B testkit locations (system-driver-port,
// system-driver-adapter, …) consumed by the runtime scope checker
// (validate-outputs-and-scopes → resolveLayerPaths → pathInScope); checking
// them here is what surfaces a stale or wrong-leaf paths: block at
// `gh optivem config preflight` time instead of mid-`implement` agent run.
func collectTiers(cfg *projectconfig.Config) []tierCheck {
	var out []tierCheck
	switch cfg.System.Architecture {
	case projectconfig.ArchMonolith:
		if cfg.System.Path != "" || cfg.System.Repo != "" {
			out = append(out, tierCheck{
				field: "system.path",
				repo:  cfg.System.Repo,
				path:  cfg.System.Path,
			})
		}
	case projectconfig.ArchMultitier:
		if !cfg.System.Backend.IsEmpty() {
			out = append(out, tierCheck{
				field: "system.backend.path",
				repo:  cfg.System.Backend.Repo,
				path:  cfg.System.Backend.Path,
			})
		}
		if !cfg.System.Frontend.IsEmpty() {
			out = append(out, tierCheck{
				field: "system.frontend.path",
				repo:  cfg.System.Frontend.Repo,
				path:  cfg.System.Frontend.Path,
			})
		}
	}
	if !cfg.SystemTest.IsEmpty() {
		out = append(out, tierCheck{
			field: "system-test.path",
			repo:  cfg.SystemTest.Repo,
			path:  cfg.SystemTest.Path,
		})
		// system-test.paths.* — eight repo-relative testkit locations. Iterate
		// in CanonicalPathKeys order so error output is stable and matches the
		// vocabulary doc (internal/projectconfig/path-keys.md). Skip absent
		// keys: Validate's Rule 22a already requires the full set when
		// system.architecture is set, so a missing entry here would already
		// have failed schema validation upstream of preflight.
		for _, key := range projectconfig.CanonicalPathKeys() {
			val, ok := cfg.SystemTest.Paths[key]
			if !ok || val == "" {
				continue
			}
			out = append(out, tierCheck{
				field: "system-test.paths." + key,
				repo:  cfg.SystemTest.Repo,
				path:  val,
			})
		}
		// system-test.system-driver-adapter-channels.* — the per-channel adapter
		// members live OUTSIDE the flat Paths map / CanonicalPathKeys, so the
		// loop above never reaches them; stat each one explicitly. Sorted by
		// channel for stable error output. Validate's Rule 24 already requires a
		// member per declared channel when architecture is set, so a present
		// member is one the operator is committed to having on disk.
		channels := make([]string, 0, len(cfg.SystemTest.SystemDriverAdapterChannels))
		for ch := range cfg.SystemTest.SystemDriverAdapterChannels {
			channels = append(channels, ch)
		}
		sort.Strings(channels)
		for _, ch := range channels {
			val := cfg.SystemTest.SystemDriverAdapterChannels[ch]
			if val == "" {
				continue
			}
			out = append(out, tierCheck{
				field: "system-test.system-driver-adapter-channels." + ch,
				repo:  cfg.SystemTest.Repo,
				path:  val,
			})
		}
	}
	// External systems — one entry per named system. The stub is always
	// present; the simulator only when real-kind: simulator (its absence is
	// the test-instance marker, so there's nothing to stat).
	for _, name := range cfg.ExternalSystemNames() {
		es := cfg.ExternalSystems[name]
		out = append(out, tierCheck{
			field: "external-systems." + name + ".stub.path",
			repo:  es.Stub.Repo,
			path:  es.Stub.Path,
		})
		if !es.Simulator.IsEmpty() {
			out = append(out, tierCheck{
				field: "external-systems." + name + ".simulator.path",
				repo:  es.Simulator.Repo,
				path:  es.Simulator.Path,
			})
		}
	}
	return out
}

// sonarProjectCheck pairs a YAML field-name with the sonar-project key
// declared at that field. Field is used purely for error messages.
type sonarProjectCheck struct {
	field string
	key   string
}

// collectSonarProjects returns every populated sonar-project entry in
// cfg, in the same deterministic order collectTiers uses for the local
// pass.
func collectSonarProjects(cfg *projectconfig.Config) []sonarProjectCheck {
	var out []sonarProjectCheck
	switch cfg.System.Architecture {
	case projectconfig.ArchMonolith:
		if cfg.System.SonarProject != "" {
			out = append(out, sonarProjectCheck{
				field: "system.sonar-project",
				key:   cfg.System.SonarProject,
			})
		}
	case projectconfig.ArchMultitier:
		if cfg.System.Backend.SonarProject != "" {
			out = append(out, sonarProjectCheck{
				field: "system.backend.sonar-project",
				key:   cfg.System.Backend.SonarProject,
			})
		}
		if cfg.System.Frontend.SonarProject != "" {
			out = append(out, sonarProjectCheck{
				field: "system.frontend.sonar-project",
				key:   cfg.System.Frontend.SonarProject,
			})
		}
	}
	if cfg.SystemTest.SonarProject != "" {
		out = append(out, sonarProjectCheck{
			field: "system-test.sonar-project",
			key:   cfg.SystemTest.SonarProject,
		})
	}
	return out
}
