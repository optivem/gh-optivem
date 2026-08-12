// Package config — local-tool presence checks for `environment verify`.
//
// gh-optivem shells out to several binaries at scaffold time: `gh` (for repo
// creation, secret/variable setting, label management, workflow dispatch,
// run-watching — see internal/shell/github.go), `actionlint` (for static
// workflow validation before any push — see internal/steps/verify.go),
// `bash` (run-sonar.sh and the scaffolded shell scripts) and `docker` (the
// local-verify lifecycle's `docker compose`). All are local-environment
// preconditions; without them, scaffolding fails partway through with errors
// that don't obviously point back at the missing tool. `environment verify`
// calls these so the user learns about all missing pieces in one pass.
//
// Lives in config (not shell) because shell already imports config; the
// reverse direction would create a cycle. Uses os/exec directly.
package config

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/optivem/gh-optivem/internal/kernel/log"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// check is the unit the parallel runner in VerifyEnvironment fans out over:
// a label for the success / failure line and a no-arg function returning an
// error. Lifted to package level so the per-language dispatcher
// (compilerChecksFor) can return []check directly instead of an anonymous
// struct that would require a conversion loop at the call site.
type check struct {
	name string
	fn   func() error
}

// ghAuthRetrySleep is the backoff between the two `gh auth status` attempts.
// A package-level seam so tests can override it to a no-op instead of paying
// the real 2-5s jittered wait. See verifyGhAuth.
var ghAuthRetrySleep = time.Sleep

// requiredGhScopes are the OAuth scopes gh-optivem's own commands need on the
// `gh` token, asserted by verifyGhAuth before `init` creates anything:
//
//   - project — `gh project list/create/link` and updateProjectV2Field during
//     init's Ensure-project-board step, and `implement`'s GitHub board tracker
//     (internal/atdd/runtime/tracker/github). Not requested by a bare
//     `gh auth login`, which is the bug this set exists to catch.
//   - workflow — pushing .github/workflows/* during init's Push scaffold;
//     GitHub rejects a push touching workflow files without it.
//   - repo — repo creation, environments, secrets, variables, labels.
//
// read:org is deliberately absent: `gh auth login --help` documents repo,
// read:org and gist as the minimum a login token always carries, so asserting
// read:org could never fail and would only add noise. repo is in that minimum
// too but is kept — it is the one scope whose absence breaks nearly every
// step, and it costs nothing to assert against a hand-built or downgraded
// token.
var requiredGhScopes = []string{"repo", "workflow", "project"}

// ghTokenScopes extracts the scope names from `gh auth status` output, which
// reports them on a line shaped:
//
//	  - Token scopes: 'gist', 'project', 'read:org', 'repo', 'workflow'
//
// The bool reports whether that line was present at all — distinguishing "the
// token has no scopes" from "gh did not say", which verifyGhAuth treats
// differently. Only the first such line is read: with several accounts or
// hosts authenticated, gh prints one block each, and merging scopes across
// distinct tokens would assert a union no single token holds.
//
// A `Token scopes: none` line (gh's rendering for a token that carries no
// classic scopes, e.g. a fine-grained PAT) parses as present-but-empty.
func ghTokenScopes(output string) ([]string, bool) {
	const marker = "Token scopes:"
	idx := strings.Index(output, marker)
	if idx < 0 {
		return nil, false
	}
	rest := output[idx+len(marker):]
	if nl := strings.IndexAny(rest, "\r\n"); nl >= 0 {
		rest = rest[:nl]
	}
	if strings.TrimSpace(rest) == "none" {
		return nil, true
	}
	var scopes []string
	for _, raw := range strings.Split(rest, ",") {
		s := strings.Trim(strings.TrimSpace(raw), "'\"")
		if s != "" {
			scopes = append(scopes, s)
		}
	}
	return scopes, true
}

// missingGhScopes returns the members of requiredGhScopes absent from have,
// preserving requiredGhScopes' order so the message and the repair command
// read the same way every time. Comparison is by exact name: gh reports the
// granted names verbatim, and no required scope is implied by another, so
// modelling scope containment (repo ⊃ repo:status …) would add a guess
// without covering a real case.
func missingGhScopes(have []string) []string {
	granted := make(map[string]bool, len(have))
	for _, s := range have {
		granted[s] = true
	}
	var missing []string
	for _, want := range requiredGhScopes {
		if !granted[want] {
			missing = append(missing, want)
		}
	}
	return missing
}

// verifyGhAuth checks that the gh CLI is installed, authenticated, and that
// its token carries the scopes gh-optivem needs. Uses plain `gh auth status`
// (no -h flag) for symmetry with internal/shell/github.go, which never locks
// host either — both use whichever default host `gh` is configured for.
//
// The `gh auth status` call is retried once on failure: when concurrent
// acceptance-matrix jobs run against the same VERIFY_TOKEN, GitHub's
// per-token throttling can return a transient auth failure even though the
// token is valid. One jittered retry makes that vanishingly rare, mirroring
// the HTTP sibling githubUserAuthCheck (token_auth.go). A genuinely
// unauthenticated token still fails both attempts and surfaces the error.
//
// Exit code alone is not enough: it is 0 for any authenticated token whatever
// its scopes, which is how a token without `project` used to reach init and
// die at the Ensure-project-board step — one step after Create repositories,
// leaving a half-scaffolded repo on GitHub. The scope line is already in the
// output captured above, so asserting it here costs no extra call and moves
// the failure to before the first side effect.
//
// When gh reports no scope line, or reports none, the check warns and passes
// rather than blocking — same reasoning as verifyClaude above: a fine-grained
// PAT or a GH_TOKEN/GITHUB_TOKEN env token has no classic OAuth scopes to
// report, so a verdict here would be a guess, and rejecting one would be a
// false negative against a perfectly valid token. The check reports what it
// can prove and puts the requirement in the message; if the permission really
// is absent, the project step downstream still fails loudly.
func verifyGhAuth() error {
	if _, err := exec.LookPath("gh"); err != nil {
		return errors.New("gh CLI not found on PATH.\n    " +
			"Install: https://cli.github.com/")
	}
	out, err := exec.Command("gh", "auth", "status").CombinedOutput()
	if err != nil {
		// 2-5s jittered backoff so concurrent retriers don't re-collide.
		ghAuthRetrySleep(2*time.Second + time.Duration(rand.IntN(3001))*time.Millisecond)
		out, err = exec.Command("gh", "auth", "status").CombinedOutput()
	}
	if err != nil {
		return fmt.Errorf("gh CLI is not authenticated.\n    "+
			"Run: gh auth login\n    "+
			"Output:\n%s", string(out))
	}
	scopes, reported := ghTokenScopes(string(out))
	if !reported || len(scopes) == 0 {
		log.Warnf("gh did not report token scopes (fine-grained PAT or GH_TOKEN/GITHUB_TOKEN env token?) — "+
			"could not verify that it grants: %s. If a later step fails on permissions, that is why.",
			strings.Join(requiredGhScopes, ", "))
		return nil
	}
	if missing := missingGhScopes(scopes); len(missing) > 0 {
		return fmt.Errorf("gh token is missing required scope(s): %s.\n    "+
			"Grant them: gh auth refresh -s %s",
			strings.Join(missing, ", "), strings.Join(missing, ","))
	}
	return nil
}

// verifyActionlint checks that the actionlint binary is on PATH. gh-optivem
// invokes actionlint during scaffolding (internal/steps/verify.go) to catch
// broken workflow references and syntax errors before any push — issues that
// otherwise surface ~10 min into the gh-acceptance pipeline as opaque HTTP
// 422 errors.
func verifyActionlint() error {
	if _, err := exec.LookPath("actionlint"); err != nil {
		return errors.New("actionlint not found on PATH.\n    " +
			"Install: go install github.com/rhysd/actionlint/cmd/actionlint@v1")
	}
	return nil
}

// verifyBash checks that the bash binary is on PATH. Two direct, unconditional
// uses:
//
//   - sonarComponent (internal/scaffolding/steps/verify.go) runs the literal
//     command `bash ./run-sonar.sh` per component during init.
//   - realShell.Run (internal/atdd/process/actions/runners.go) routes every
//     shell command through $SHELL, defaulting to "bash" when unset — so
//     `system start`, `system-test run` and the whole `implement` pipeline
//     go through it, with no skip flag.
//
// verify.go does its own LookPath and Fatalfs, but only once init is already
// mid-flight. Checking here means a Windows user with no Git Bash learns
// before any repo is created rather than after.
func verifyBash() error {
	if _, err := exec.LookPath("bash"); err != nil {
		return errors.New("bash not found on PATH.\n    " +
			"Install Git Bash (Windows): https://git-scm.com/download/win\n    " +
			"macOS and Linux ship it already — check your PATH.")
	}
	return nil
}

// verifyClaude checks that the claude binary is on PATH. The ATDD pipeline
// dispatches every agent as a `claude` subprocess, so `implement` cannot run
// a single node without it — and the failure surfaces as an exec error naming
// the subprocess, not the missing install.
//
// Presence only, deliberately: `claude` exposes no non-interactive
// auth-status command, and `claude --version` exits 0 whether or not the user
// is signed in. Asserting sign-in would mean either launching interactively
// or leaning on an undocumented internal, so a signed-out machine would be
// indistinguishable from a working one. Rather than coerce that indeterminate
// answer into a verdict, the check reports what it can prove and puts the
// sign-in step in the failure message.
//
// Unconditional for local use, like bash and docker: `implement` is the
// reason the extension exists, and a setup-only user is the edge case. CI is
// the one real exception — see skipClaudeCheckEnv.
func verifyClaude() error {
	if _, err := exec.LookPath("claude"); err != nil {
		return errors.New("claude not found on PATH.\n    " +
			"Install: https://claude.com/claude-code\n    " +
			"Then sign in by running: claude")
	}
	return nil
}

// skipClaudeCheckEnv opts out of the claude presence check. Set it on
// GitHub-hosted runners: the scaffolding workflows run `environment verify`
// to gate the rest of the job, but runners have no claude installed and never
// reach `implement`, so the check would fail the job for a tool that job does
// not use.
//
// Scoped to claude alone rather than a general "skip tool checks" switch —
// every other tool in the list is genuinely needed in CI.
const skipClaudeCheckEnv = "GH_OPTIVEM_SKIP_CLAUDE_CHECK"

// claudeCheckSkipped reports whether skipClaudeCheckEnv opts out of the
// claude presence check.
//
// An unparseable value is an error, not a silent false: `SKIP=yes` reads as
// an opt-out to whoever wrote the workflow, and quietly running the check
// anyway would fail the job with a message about claude that says nothing
// about the typo that actually caused it.
func claudeCheckSkipped() (bool, error) {
	raw, ok := os.LookupEnv(skipClaudeCheckEnv)
	if !ok || raw == "" {
		return false, nil
	}
	skip, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s=%q is not a valid boolean.\n    "+
			"Use 1/true to skip the claude check, 0/false to run it.",
			skipClaudeCheckEnv, raw)
	}
	return skip, nil
}

// verifyNpm checks that the npm binary is on PATH. Required for the
// TypeScript compile sequence (internal/compiler/compiler.go), which runs
// `npm ci && npx tsc --noEmit` against the tier cwd.
func verifyNpm() error {
	if _, err := exec.LookPath("npm"); err != nil {
		return errors.New("npm not found on PATH.\n    " +
			"Install Node.js (bundles npm): https://nodejs.org/")
	}
	return nil
}

// verifyDotnet checks that the dotnet binary is on PATH. Required for the
// .NET compile sequence (`dotnet build`).
func verifyDotnet() error {
	if _, err := exec.LookPath("dotnet"); err != nil {
		return errors.New("dotnet not found on PATH.\n    " +
			"Install the .NET SDK: https://dotnet.microsoft.com/download")
	}
	return nil
}

// verifyJava checks that the java binary is on PATH. Required for the Java
// compile sequence — gradlew.bat (in-repo) shells out to whatever java is
// resolved from PATH / JAVA_HOME.
func verifyJava() error {
	if _, err := exec.LookPath("java"); err != nil {
		return errors.New("java not found on PATH.\n    " +
			"Install a JDK (Temurin recommended): https://adoptium.net/")
	}
	return nil
}

// compilerChecksFor returns the local-tool checks required for the given
// set of languages. Duplicates in langs are deduped — passing
// ["typescript", "typescript", "java"] returns one npm check and one java
// check. Unknown language strings are silently skipped (the language-flag
// validators in resolveLangs / IsValidLang run earlier and reject anything
// outside the known set, so reaching this dispatcher with an unknown value
// would be a programmer error, not user input).
func compilerChecksFor(langs []string) []check {
	seen := map[string]bool{}
	var out []check
	for _, l := range langs {
		if seen[l] {
			continue
		}
		seen[l] = true
		switch l {
		case projectconfig.LangTypescript:
			out = append(out, check{"npm", verifyNpm})
		case projectconfig.LangDotnet:
			out = append(out, check{"dotnet", verifyDotnet})
		case projectconfig.LangJava:
			out = append(out, check{"java", verifyJava})
		}
	}
	return out
}

// verifyDocker checks that the docker binary is on PATH. Required
// unconditionally, independent of the deploy target: the local-verify
// lifecycle (Build / Up / Run tests / Down / Clean in internal/runner)
// shells out to `docker compose` for every scaffold, so a cloud-run project
// still needs Docker to run its system locally. Compose v2 is a docker
// sub-command, so the `docker` binary alone is sufficient; legacy Compose v1
// (`docker-compose`) installs are not checked separately.
func verifyDocker() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return errors.New("docker not found on PATH.\n    " +
			"Install Docker Desktop (macOS/Windows): https://www.docker.com/products/docker-desktop\n    " +
			"Install Docker Engine (Linux): https://docs.docker.com/engine/install/")
	}
	return nil
}
