package runner

// RunShell executes a command string in cwd with an optional env overlay,
// streaming output to the terminal. It is a thin exported wrapper over the
// package-internal runShell so sibling runners (e.g. the component-test runner
// in internal/build/componenttest) reuse the same quote-aware, cmd.exe-safe
// shell execution without duplicating the platform-specific batch-file
// metacharacter handling.
func RunShell(command, cwd string, env map[string]string) error {
	return runShell(command, cwd, env)
}

// ApplyTestFilter substitutes the supplied test names into testFilter and
// merges the result into command, per the join semantics documented on the
// internal applyTestFilter. Exported so sibling runners share one
// implementation of the <test> placeholder substitution.
func ApplyTestFilter(command, testFilter, join string, names []string) string {
	return applyTestFilter(command, testFilter, join, names)
}
