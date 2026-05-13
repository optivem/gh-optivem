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
	"os/exec"
)

// verifyGhAuth checks that the gh CLI is installed and authenticated. Uses
// plain `gh auth status` (no -h flag) for symmetry with internal/shell/github.go,
// which never locks host either — both use whichever default host `gh` is
// configured for.
func verifyGhAuth() error {
	if _, err := exec.LookPath("gh"); err != nil {
		return errors.New("gh CLI not found on PATH.\n    " +
			"Install: https://cli.github.com/")
	}
	cmd := exec.Command("gh", "auth", "status")
	out, err := cmd.CombinedOutput()
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
