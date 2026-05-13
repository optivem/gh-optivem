// Package promptio centralises every human y/n decision the CLI surfaces.
// One helper, one convention: prompts are appended with " [y/n]: ", input is
// case-insensitive, unrecognised answers (including bare Enter) re-prompt
// until y/yes or n/no arrives. There is no Enter-default — operators must
// type an explicit letter, eliminating the "is the default yes or no here?"
// lookup that the legacy per-site [y/N]/[Y/n] hints required.
//
// Design notes:
//   - Two entry points exist because the codebase has two prompt shapes:
//     direct io.Reader/io.Writer (release.InteractiveConfirmer, main.go,
//     config.go, …) and the line-by-line Prompter abstraction used by the
//     BPMN bindings (gates, actions). Both share the same loop semantics.
//   - EOF on stdin terminates the loop and returns false. The loop would
//     otherwise spin forever on a closed stream; declining on EOF preserves
//     the "silence = no" guarantee non-interactive callers rely on.
//   - The helper writes the "Please answer y or n." reminder to the same
//     writer the prompt itself goes to. Callers that want stderr-only
//     prompts pass stderr; callers that want stdout pass stdout.
package promptio

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Asker is the line-by-line prompt abstraction used by the BPMN bindings.
// It is defined structurally so the existing Prompter interfaces in gates,
// actions, and elsewhere satisfy it without an explicit dependency.
type Asker interface {
	Ask(prompt string) (string, error)
}

// ConfirmYN appends " [y/n]: " to prompt, writes it to out, and reads from
// in until it gets y/yes or n/no (case-insensitive). Unrecognised input —
// including bare Enter — re-prompts. EOF returns (false, nil).
func ConfirmYN(in io.Reader, out io.Writer, prompt string) (bool, error) {
	reader := bufio.NewReader(in)
	for {
		if _, err := fmt.Fprint(out, prompt+" [y/n]: "); err != nil {
			return false, err
		}
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		if err == io.EOF {
			return false, nil
		}
		fmt.Fprintln(out, "Please answer y or n.")
	}
}

// ConfirmYNVia is the Asker-routed variant. Each iteration calls asker.Ask
// with the prompt + " [y/n]: " suffix; unrecognised replies trigger another
// Ask. An asker error terminates the loop and is returned to the caller.
// An empty reply with no error (the Prompter contract for EOF, see the
// stdinPrompter implementations in gates/actions) returns (false, nil) so
// non-interactive callers do not spin forever.
func ConfirmYNVia(asker Asker, out io.Writer, prompt string) (bool, error) {
	for {
		answer, err := asker.Ask(prompt + " [y/n]: ")
		if err != nil {
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		if answer == "" {
			return false, nil
		}
		fmt.Fprintln(out, "Please answer y or n.")
	}
}

// ParseYN is the stateless coercion used outside the interactive prompt
// loops — e.g. by BPMN gates reading a pre-set Context value that was
// populated by an upstream service task or an environment-style override.
// It accepts the broader vocabulary the engine writes: "y"/"yes"/"true"/
// "1" → (true, true); "n"/"no"/"false"/"0"/"" → (false, true); anything
// else → (_, false) so callers can surface "unrecognised value" instead
// of silently coercing to false.
func ParseYN(s string) (value, ok bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "y", "yes", "true", "1":
		return true, true
	case "n", "no", "false", "0", "":
		return false, true
	default:
		return false, false
	}
}
