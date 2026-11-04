package clauderun

// NewClaudeRunner returns the production ClaudeRunner that shells out to the
// `claude` CLI. Callers outside the ATDD dispatcher (for example the kickoff
// command) use it to run a single prompt without the ticket-shaped Options
// that Dispatch requires.
func NewClaudeRunner() ClaudeRunner { return execClaude{} }
