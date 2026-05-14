// doctor_commands.go wires the `gh optivem doctor` command. It verifies the
// three global git config keys docs/tbd.md requires for trunk-based
// development. With --fix, it sets any missing or wrong values at the global
// level. Replaces the "copy these three commands out of the doc" onboarding
// step with one command.
//
// Scope is intentionally narrow — config only. Broader repo-health checks
// belong in their own command, not here.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

type doctorOptions struct {
	Fix bool
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
		Short: "Verify the global git config docs/tbd.md requires; --fix to set missing keys",
		Long: `Verify the three global git config keys docs/tbd.md requires for
trunk-based development:

  pull.rebase = true
  rebase.autoStash = true
  rerere.enabled = true

With --fix, sets any missing or wrong values at the global level. Without
--fix, reports pass/fail per key and exits non-zero if any are wrong.`,
		Example: `  gh optivem doctor
  gh optivem doctor --fix`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			exitOnError(runDoctor(opts))
		},
	}
	cmd.Flags().BoolVar(&opts.Fix, "fix", false, "Set missing or wrong keys to the required values (global git config)")
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
