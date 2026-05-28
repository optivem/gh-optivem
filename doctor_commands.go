// doctor_commands.go wires the `gh optivem doctor` command. It verifies
// local state invariants the rest of the binary depends on:
//
//   - global git config keys docs/tbd.md requires for trunk-based development
//     (the original scope; --fix sets them).
//   - orphan claude.exe subprocesses left behind by crashed `gh optivem
//     implement` runs, surfaced via --orphans and recovered from the
//     per-dispatch PID markers internal/userstate owns. See
//     doctor_orphans.go for the recovery logic.
//
// Broader repo-health checks belong in their own command; this one stays
// focused on "the things gh-optivem itself needs in order to work."
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

type doctorOptions struct {
	Fix     bool
	Orphans bool
}

// requiredGitConfig is the canonical list of global git config keys
// docs/tbd.md mandates. Order is the order they're reported in.
var requiredGitConfig = []struct {
	Key  string
	Want string
}{
	{"pull.rebase", "true"},
	{"rebase.autoStash", "true"},
	{"rerere.enabled", "true"},
}

func newDoctorCmd() *cobra.Command {
	opts := doctorOptions{}
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Verify local state invariants (git config; --orphans for crashed-run cleanup)",
		Long: `Verify local state invariants gh-optivem itself depends on:

  - the three global git config keys docs/tbd.md requires for trunk-
    based development (pull.rebase, rebase.autoStash, rerere.enabled).
  - orphan claude subprocesses left behind by crashed 'implement' runs
    (Ctrl+C in the parent terminal, terminal closed, kernel kill, panic).
    Run with --orphans to scan, classify, and interactively kill them.

Without flags, runs the read-only git-config check. With --fix, sets any
missing or wrong git config keys at the global level. With --orphans,
runs the orphan-recovery sweep INSTEAD of the git-config check: lists
orphan claude subprocesses tracked by per-dispatch PID markers under the
user-level state dir, classifies each as stale (child already dead —
silently cleaned), live (parent still running — left alone), or orphan
(child alive, parent dead — prompts y/n to kill). To run both sweeps,
invoke the command twice.`,
		Example: `  gh optivem doctor
  gh optivem doctor --fix
  gh optivem doctor --orphans`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if opts.Orphans {
				exitOnError(runDoctorOrphans(cmd.InOrStdin(), cmd.OutOrStdout()))
				return
			}
			exitOnError(runDoctor(opts))
		},
	}
	cmd.Flags().BoolVar(&opts.Fix, "fix", false, "Set missing or wrong keys to the required values (global git config)")
	cmd.Flags().BoolVar(&opts.Orphans, "orphans", false, "Scan the user-level state dir for orphan claude subprocesses from crashed 'implement' runs and prompt to kill them")
	return cmd
}

func runDoctor(opts doctorOptions) error {
	fmt.Println(separator)
	fmt.Println("  Doctor — git config for trunk-based development")
	fmt.Println(separator)
	fmt.Println()

	failed := 0
	for _, kv := range requiredGitConfig {
		actual := readGlobalGitConfig(kv.Key)
		if actual == kv.Want {
			fmt.Printf("  ✓ %s = %s\n", kv.Key, kv.Want)
			continue
		}
		if opts.Fix {
			if err := setGlobalGitConfig(kv.Key, kv.Want); err != nil {
				fmt.Printf("  ✗ %s: failed to set: %v\n", kv.Key, err)
				failed++
				continue
			}
			fmt.Printf("  ✓ %s = %s (fixed)\n", kv.Key, kv.Want)
			continue
		}
		if actual == "" {
			fmt.Printf("  ✗ %s is unset (want %s)\n", kv.Key, kv.Want)
		} else {
			fmt.Printf("  ✗ %s = %s (want %s)\n", kv.Key, actual, kv.Want)
		}
		failed++
	}

	fmt.Println()
	fmt.Println(separator)
	if failed > 0 {
		if !opts.Fix {
			fmt.Printf("  %d setting(s) wrong. Re-run with --fix to set them.\n", failed)
		} else {
			fmt.Printf("  %d setting(s) could not be fixed.\n", failed)
		}
		fmt.Println(separator)
		return fmt.Errorf("doctor: %d required git config setting(s) wrong", failed)
	}
	fmt.Println("  All required git config keys are set.")
	fmt.Println(separator)
	return nil
}

// readGlobalGitConfig returns the trimmed value of a global git config key,
// or "" when unset. An error from `git config --get` (typically exit 1 for
// "key not present") is treated as unset — the doctor either reports "unset"
// or, with --fix, sets the canonical value.
func readGlobalGitConfig(key string) string {
	out, err := exec.Command("git", "config", "--global", "--get", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func setGlobalGitConfig(key, val string) error {
	cmd := exec.Command("git", "config", "--global", key, val)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
