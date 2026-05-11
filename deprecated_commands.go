// deprecated_commands.go wires the hidden verb-first aliases that ship for
// one release alongside the noun-first rename. Each alias prints a one-line
// stderr deprecation warning, then delegates to the same handler the new
// noun-first form uses (the constructors in system_commands.go /
// test_commands.go / token_commands.go return a fresh *cobra.Command per
// call, so the alias and the new-tree registration get independent
// instances — the deprecation PreRun only fires when the alias path is
// taken).
//
// Drop in v1.6 per plans/20260511-2010-drop-verb-first-aliases.md.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// newDeprecatedAlias wires `<oldParentUse> <oldChildUse>` as a hidden alias
// of `<newForm>`. The handler comes from `child` (built by the same
// constructor the new tree uses), with its Use overridden so it routes under
// the deprecated parent. PreRun prints a one-line warning to stderr before
// the new handler runs.
func newDeprecatedAlias(oldParentUse, oldChildUse, newForm string, child *cobra.Command) *cobra.Command {
	child.Use = oldChildUse
	child.Hidden = true
	child.PreRun = func(c *cobra.Command, args []string) {
		fmt.Fprintf(os.Stderr,
			"DEPRECATED: `gh optivem %s %s` will be removed in v1.6. "+
				"Use `gh optivem %s` instead.\n", oldParentUse, oldChildUse, newForm)
	}
	parent := &cobra.Command{
		Use:    oldParentUse,
		Hidden: true,
		Short:  "DEPRECATED: use `" + newForm + "`",
	}
	parent.AddCommand(child)
	return parent
}

// newDeprecatedLeaf wires a single hidden leaf (no parent) with a stderr
// deprecation warning. Used for hidden children of an existing
// (non-deprecated) parent — `test system` under the real `test` parent,
// and `compile system` / `compile system-tests` under the real bare
// `compile`. The caller is responsible for attaching the returned command
// to its parent.
func newDeprecatedLeaf(oldParentUse, oldChildUse, newForm string, child *cobra.Command) *cobra.Command {
	child.Use = oldChildUse
	child.Hidden = true
	child.Short = "DEPRECATED: use `gh optivem " + newForm + "`"
	child.PreRun = func(c *cobra.Command, args []string) {
		fmt.Fprintf(os.Stderr,
			"DEPRECATED: `gh optivem %s %s` will be removed in v1.6. "+
				"Use `gh optivem %s` instead.\n", oldParentUse, oldChildUse, newForm)
	}
	return child
}

func newDeprecatedBuildCmd() *cobra.Command {
	return newDeprecatedAlias("build", "system", "system build", newSystemBuildCmd())
}

func newDeprecatedRunCmd() *cobra.Command {
	return newDeprecatedAlias("run", "system", "system start", newSystemStartCmd())
}

func newDeprecatedStopCmd() *cobra.Command {
	return newDeprecatedAlias("stop", "system", "system stop", newSystemStopCmd())
}

func newDeprecatedCleanCmd() *cobra.Command {
	return newDeprecatedAlias("clean", "system", "system clean", newSystemCleanCmd())
}

func newDeprecatedVerifyCmd() *cobra.Command {
	return newDeprecatedAlias("verify", "tokens", "token verify", newTokenVerifyCmd())
}

// newDeprecatedTestSystemCmd returns a hidden `system` child for the real
// `test` parent — alias of `test run`.
func newDeprecatedTestSystemCmd() *cobra.Command {
	return newDeprecatedLeaf("test", "system", "test run", newTestRunCmd())
}

// newDeprecatedCompileSystemCmd returns a hidden `system` child for the
// real bare `compile` parent — alias of `system compile`.
func newDeprecatedCompileSystemCmd() *cobra.Command {
	return newDeprecatedLeaf("compile", "system", "system compile", newSystemCompileCmd())
}

// newDeprecatedCompileSystemTestsCmd returns a hidden `system-tests` child
// for the real bare `compile` parent — alias of `test compile`.
func newDeprecatedCompileSystemTestsCmd() *cobra.Command {
	return newDeprecatedLeaf("compile", "system-tests", "test compile", newTestCompileCmd())
}
