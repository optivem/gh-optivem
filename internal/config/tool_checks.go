// Package config — local-tool presence checks for `environment verify`.
//
// gh-optivem shells out to two binaries at scaffold time: `gh` (for repo
// creation, secret/variable setting, label management, workflow dispatch,
// run-watching — see internal/shell/github.go) and `actionlint` (for static
// workflow validation before any push — see internal/steps/verify.go). Both
// are local-environment preconditions; without them, scaffolding fails
// partway through with errors that don't obviously point back at the missing
// tool. `environment verify` calls these so the user learns about all
// missing pieces in one pass.
//
// Lives in config (not shell) because shell already imports config; the
// reverse direction would create a cycle. Uses os/exec directly.
package config

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"os/exec"
	"time"

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

// verifyGhAuth checks that the gh CLI is installed and authenticated. Uses
// plain `gh auth status` (no -h flag) for symmetry with internal/shell/github.go,
// which never locks host either — both use whichever default host `gh` is
// configured for.
//
// The `gh auth status` call is retried once on failure: when concurrent
// acceptance-matrix jobs run against the same VERIFY_TOKEN, GitHub's
// per-token throttling can return a transient auth failure even though the
// token is valid. One jittered retry makes that vanishingly rare, mirroring
// the HTTP sibling githubUserAuthCheck (token_auth.go). A genuinely
// unauthenticated token still fails both attempts and surfaces the error.
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

// verifyDocker checks that the docker binary is on PATH. Required for every
// scaffold using --deploy docker (the default) — the local-verify lifecycle
// (Build / Up / Run tests / Down / Clean in internal/runner) shells out to
// `docker compose`. Compose v2 is a docker sub-command, so the `docker`
// binary alone is sufficient; legacy Compose v1 (`docker-compose`) installs
// are not checked separately.
func verifyDocker() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return errors.New("docker not found on PATH.\n    " +
			"Install Docker Desktop (macOS/Windows): https://www.docker.com/products/docker-desktop\n    " +
			"Install Docker Engine (Linux): https://docs.docker.com/engine/install/")
	}
	return nil
}

// deployChecksFor returns the local-tool checks required for the given
// deploy target. Empty deploy string returns nil — callers that don't
// know the deploy target skip the deploy-conditional check (same idiom
// as compilerChecksFor with an empty langs slice). Unknown deploy values
// also return nil for the same programmer-error reason given in
// compilerChecksFor — the deploy-flag validator (projectconfig.IsValidDeploy)
// runs upstream.
func deployChecksFor(deploy string) []check {
	switch deploy {
	case projectconfig.DeployDocker:
		return []check{{"docker", verifyDocker}}
	}
	return nil
}
