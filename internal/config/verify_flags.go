// Package config — extracted flag-validation for `gh optivem environment verify`.
//
// The CLI command's `Run:` closure used to inline both validation and exit
// behaviour, which made the rejection paths untestable without subprocess
// hacks. ValidateVerifyFlags pulls the validation into a pure function that
// returns an aggregated error; the CLI surface stays responsible for
// printing + exit.
package config

import "fmt"

// ValidateVerifyFlags checks the --lang values passed to
// `gh optivem environment verify`. Returns nil when they are acceptable;
// otherwise an error describing every unsupported value so a typo in a
// long comma-separated list surfaces with all offenders rather than just
// the first.
//
// --lang: each value must satisfy IsValidLang (java / dotnet / typescript).
func ValidateVerifyFlags(langs []string) error {
	var bad []string
	for _, l := range langs {
		if !IsValidLang(l) {
			bad = append(bad, l)
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("--lang: unsupported value(s) %v; must be one of 'java', 'dotnet', 'typescript'", bad)
	}
	return nil
}
