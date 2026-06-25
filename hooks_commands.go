// hooks_commands.go wires the `gh optivem hooks` subtree. Today the only
// child is `install`, which drops a pre-push hook into the current repo that
// refuses non-fast-forward pushes to `main`. Belt-and-suspenders for the
// "never force-push main" rule (docs/tbd.md) — the in-tool guard in
// workspace_commands.go covers tool-mediated pushes; the hook covers raw
// `git push --force` invocations the tool never sees.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

// prePushMarker identifies a pre-push hook installed by this command, so a
// re-run can safely overwrite the file in place. Any pre-existing hook
// without this marker is left alone — the operator may be hand-managing it.
const prePushMarker = "# gh-optivem-pre-push v1"

// prePushHook is the script written to .git/hooks/pre-push. It reads each
// `<local_ref> <local_sha> <remote_ref> <remote_sha>` line from stdin (git's
// pre-push protocol) and refuses any push to refs/heads/main where the local
// sha does not have the remote sha as an ancestor — i.e. a force-push.
//
// The shebang is /bin/sh for portability (git-for-windows ships bash).
const prePushHook = `#!/bin/sh
` + prePushMarker + `
# Refuses force-push to refs/heads/main. See docs/tbd.md.
# Installed by ` + "`gh optivem hooks install`" + `; re-run that command to update.

zero=$(git hash-object --stdin </dev/null | tr '0-9a-f' '0')

while read local_ref local_sha remote_ref remote_sha; do
    case "$remote_ref" in
        refs/heads/main) ;;
        *) continue ;;
    esac

    if [ "$local_sha" = "$zero" ]; then
        echo "gh-optivem pre-push: refusing to delete refs/heads/main." >&2
        exit 1
    fi

    if [ "$remote_sha" = "$zero" ]; then
        # First push of main — no prior history to rewrite.
        continue
    fi

    if ! git merge-base --is-ancestor "$remote_sha" "$local_sha"; then
        echo "gh-optivem pre-push: refusing non-fast-forward push to refs/heads/main." >&2
        echo "Force-pushing main is forbidden. See docs/tbd.md." >&2
        exit 1
    fi
done

exit 0
`

func newHooksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hooks",
		Short: "Install git hooks that enforce docs/tbd.md invariants locally",
	}
	cmd.AddCommand(newHooksInstallCmd())
	return cmd
}

func newHooksInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install a pre-push hook that refuses non-fast-forward pushes to main",
		Long: `Installs ` + "`.git/hooks/pre-push`" + ` in the current repo. The hook refuses any
push to refs/heads/main that would rewrite history (force-push or
force-with-lease). Defense-in-depth for the "never force-push main" rule in
docs/tbd.md.

Idempotent: re-runs replace a previously installed hook. A pre-existing
hook without this command's marker is left in place and the install fails.`,
		Example: `  gh optivem hooks install`,
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			exitOnError(runHooksInstall())
		},
	}
}

func runHooksInstall() error {
	hooksDir, err := gitHooksDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return fmt.Errorf("create hooks dir %s: %w", hooksDir, err)
	}

	target := filepath.Join(hooksDir, "pre-push")
	if existing, err := os.ReadFile(target); err == nil {
		if !strings.Contains(string(existing), prePushMarker) {
			return fmt.Errorf("refusing to overwrite existing pre-push hook at %s.\n       Inspect it and remove it manually if you want gh-optivem to manage this hook.", target)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read %s: %w", target, err)
	}

	if err := os.WriteFile(target, []byte(prePushHook), 0o700); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	// Belt for non-Windows platforms where WriteFile's perm bits may be masked.
	if runtime.GOOS != "windows" {
		if err := os.Chmod(target, 0o700); err != nil {
			return fmt.Errorf("chmod %s: %w", target, err)
		}
	}

	fmt.Printf("Installed pre-push hook at %s\n", target)
	fmt.Println("Refuses non-fast-forward pushes to refs/heads/main.")
	return nil
}

// gitHooksDir resolves the absolute path to the current repo's hooks dir via
// `git rev-parse --git-path hooks`. Honours worktrees and the GIT_DIR env
// variable so it works in non-standard repo layouts.
func gitHooksDir() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--git-path", "hooks").Output()
	if err != nil {
		return "", fmt.Errorf("not inside a git repository (git rev-parse failed): %w", err)
	}
	rel := strings.TrimSpace(string(out))
	if rel == "" {
		return "", fmt.Errorf("git rev-parse --git-path hooks returned empty path")
	}
	if filepath.IsAbs(rel) {
		return rel, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(cwd, rel), nil
}
