// output_commands.go wires the `gh optivem output <verb>` subtree. The
// output noun is the structured-output channel between an ATDD agent
// (running under `claude` in a dispatched subprocess) and the gh-optivem
// dispatcher. The agent calls `gh optivem output write KEY=VALUE` from its
// `Bash` tool to emit a value the dispatcher then reads back from a
// per-invocation JSONL file. Replaces the older prose-YAML channel that
// could not cross the interactive-mode TTY boundary.
//
// Two env vars drive the channel; both are exported by the dispatcher
// before launching `claude`:
//
//   - GH_OPTIVEM_OUTPUT_FILE — absolute path to the JSONL file. Each
//     `write` invocation appends one JSON object as a single line.
//   - GH_OPTIVEM_OUTPUT_KEYS — comma-separated allow-list with types,
//     shape `key1:type1,key2:type2,...`. Derived from the call-activity's
//     BPMN `outputs:` list. Any unknown key or coercion failure exits
//     non-zero so the agent sees the error mid-run.
//
// Types: `string`, `bool`, `string-list` (comma-split). Coercion is
// self-contained in this file — there is no shared Go table; the BPMN
// declaration is the single source of truth, and the dispatcher hands the
// per-invocation slice in via the env var.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

const (
	envOutputFile = "GH_OPTIVEM_OUTPUT_FILE"
	envOutputKeys = "GH_OPTIVEM_OUTPUT_KEYS"
)

const (
	outputTypeString     = "string"
	outputTypeBool       = "bool"
	outputTypeStringList = "string-list"
)

// newOutputCmd builds the `gh optivem output` parent. The parent has no
// Run, so invoking it without a subcommand prints help. Siblings under
// this noun (e.g. `output read KEY`) are planned but not built yet.
func newOutputCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "output",
		Short: "ATDD agent output channel (called from a dispatched agent subprocess)",
	}
	cmd.AddCommand(newOutputWriteCmd())
	return cmd
}

// newOutputWriteCmd implements:
//
//	gh optivem output write KEY=VALUE [KEY=VALUE...]
//
// Appends one JSONL line per invocation to GH_OPTIVEM_OUTPUT_FILE. A
// single call with multiple KEY=VALUE arguments writes one combined
// JSON object, preserving the agent's emit-intent. Across calls the
// dispatcher's reader applies last-write-wins per key.
func newOutputWriteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "write KEY=VALUE [KEY=VALUE...]",
		Short: "Emit one or more structured outputs to the dispatcher",
		Example: `  gh optivem output write dsl-port-changed=true
  gh optivem output write test-names=shouldRegisterCustomer,shouldRejectDuplicate
  gh optivem output write test-names=foo,bar dsl-port-changed=false`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			filePath, hasFile := os.LookupEnv(envOutputFile)
			keysSpec, hasKeys := os.LookupEnv(envOutputKeys)
			exitOnError(runOutputWrite(args, filePath, hasFile, keysSpec, hasKeys))
		},
	}
}

// runOutputWrite is the testable core of `output write`. It is pure with
// respect to the OS environment: callers pass the resolved env-var values
// in directly. Returns nil on a successful append, or a descriptive error
// otherwise. Errors are surfaced to stderr via exitOnError at the Cobra
// boundary; the agent sees them in the `Bash` tool's stderr.
func runOutputWrite(args []string, filePath string, hasFile bool, keysSpec string, hasKeys bool) error {
	if !hasFile || filePath == "" {
		return fmt.Errorf("%q must run inside a gh-optivem agent dispatch (%s is not set)",
			"gh optivem output write", envOutputFile)
	}
	if !hasKeys {
		return fmt.Errorf("no outputs declared for this agent (%s is not set)", envOutputKeys)
	}

	declared, order, err := parseOutputKeysSpec(keysSpec)
	if err != nil {
		return err
	}

	line := make(map[string]any, len(args))
	seen := make(map[string]bool, len(args))
	for _, arg := range args {
		if err := applyOutputArg(arg, declared, order, seen, line); err != nil {
			return err
		}
	}

	encoded, err := json.Marshal(line)
	if err != nil {
		return fmt.Errorf("marshal output line: %w", err)
	}

	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", filePath, err)
	}
	defer f.Close()

	if _, err := f.Write(append(encoded, '\n')); err != nil {
		return fmt.Errorf("write to %s: %w", filePath, err)
	}
	return nil
}

func applyOutputArg(arg string, declared map[string]string, order []string, seen map[string]bool, line map[string]any) error {
	key, raw, err := splitOutputKeyValue(arg)
	if err != nil {
		return err
	}
	if seen[key] {
		return fmt.Errorf("duplicate key %q in a single `output write` call "+
			"(last-write-wins applies across calls, not within)", key)
	}
	seen[key] = true
	keyType, ok := declared[key]
	if !ok {
		return fmt.Errorf("unknown output key %q; declared keys: %s",
			key, strings.Join(order, ", "))
	}
	coerced, err := coerceOutputValue(raw, keyType, key)
	if err != nil {
		return err
	}
	line[key] = coerced
	return nil
}

// parseOutputKeysSpec parses the GH_OPTIVEM_OUTPUT_KEYS env-var format
// (`key1:type1,key2:type2,...`) into a key→type map plus the declared
// keys in their declaration order (used for the "declared keys: ..."
// hint when an unknown key is rejected). An empty spec yields an empty
// map; that distinguishes "empty allow-list" from "no allow-list
// configured" (the latter is rejected one level up by the !hasKeys
// guard in runOutputWrite).
func parseOutputKeysSpec(spec string) (map[string]string, []string, error) {
	declared := map[string]string{}
	var order []string
	if spec == "" {
		return declared, order, nil
	}
	for entry := range strings.SplitSeq(spec, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		idx := strings.Index(entry, ":")
		if idx <= 0 || idx == len(entry)-1 {
			return nil, nil, fmt.Errorf("malformed %s entry %q (want key:type)",
				envOutputKeys, entry)
		}
		key := entry[:idx]
		keyType := entry[idx+1:]
		switch keyType {
		case outputTypeString, outputTypeBool, outputTypeStringList:
		default:
			return nil, nil, fmt.Errorf("unknown type %q for key %q in %s "+
				"(want string, bool, or string-list)", keyType, key, envOutputKeys)
		}
		if _, dup := declared[key]; dup {
			return nil, nil, fmt.Errorf("duplicate key %q in %s", key, envOutputKeys)
		}
		declared[key] = keyType
		order = append(order, key)
	}
	return declared, order, nil
}

// splitOutputKeyValue splits a KEY=VALUE argument on the first '='. The
// key must be non-empty; the value may be empty (e.g. an empty
// string-list).
func splitOutputKeyValue(arg string) (string, string, error) {
	idx := strings.Index(arg, "=")
	if idx <= 0 {
		return "", "", fmt.Errorf("malformed KEY=VALUE argument %q (want key=value)", arg)
	}
	return arg[:idx], arg[idx+1:], nil
}

// coerceOutputValue applies the declared type to a raw string value.
// Coercion is strict: bool accepts only the literals `true` / `false`,
// not strconv.ParseBool's broader set, so a typoed value surfaces as a
// clear error rather than silently coercing (e.g. "1" → true).
func coerceOutputValue(raw, keyType, key string) (any, error) {
	switch keyType {
	case outputTypeString:
		return raw, nil
	case outputTypeBool:
		switch raw {
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			return nil, fmt.Errorf("output key %q expects bool, got %q "+
				"(accepted: true, false)", key, raw)
		}
	case outputTypeStringList:
		if raw == "" {
			return []string{}, nil
		}
		parts := strings.Split(raw, ",")
		for i, p := range parts {
			parts[i] = strings.TrimSpace(p)
		}
		return parts, nil
	default:
		// Defensive — parseOutputKeysSpec already rejects unknown types,
		// but keep this branch so a future type added there but missed
		// here surfaces a clear error.
		return nil, errors.New("unsupported type " + keyType + " for key " + key)
	}
}
