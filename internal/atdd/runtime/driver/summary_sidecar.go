// summary_sidecar.go owns the on-disk shape and read/write of
// `.gh-optivem/runs/<run-ts>/summary.jsonl`. The sidecar is the durable
// twin of the in-memory agent-summary table: appendSummaryLine writes
// one JSON object per dispatch as it completes, so a binary crash
// mid-run still leaves on disk every row that completed before the
// bust. PrintSummaryFile is the entry point the cobra layer's
// `gh optivem run summary [ts]` calls — it loads the file and routes
// through renderAgentSummary so the historical view stays byte-identical
// with the live banner.
//
// The driver package owns the on-disk shape (rather than reusing
// clauderun.TokenUsage's struct tags directly) so a future change to
// clauderun's parse-time decode shape can't silently break sidecar
// readers. dispatchRecordJSON is the single conversion point.
package driver

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/clauderun"
)

// dispatchRecordJSON is the on-disk shape of one dispatch row in
// summary.jsonl. Exported field names so the encoder/decoder can see
// them; the type itself stays unexported because callers outside this
// package read summary files through PrintSummaryFile, not by holding
// the record value.
//
// ElapsedNS is the raw time.Duration int64 nanoseconds — encoding the
// duration as ns rather than a human-readable string keeps the file
// machine-readable while still letting the renderer round to seconds at
// print time. Usage is omitted entirely when the dispatch ran without
// a parsed envelope (interactive mode, or a headless run that crashed
// before the terminal `type:"result"` event) — `omitempty` keeps the
// JSON tight and makes the absence semantically meaningful at read time.
// Error is the dispatch's error string when non-empty; absence means
// success.
type dispatchRecordJSON struct {
	Agent     string     `json:"agent"`
	Model     string     `json:"model"`
	Effort    string     `json:"effort"`
	ElapsedNS int64      `json:"elapsed_ns"`
	Usage     *usageJSON `json:"usage,omitempty"`
	Error     string     `json:"error,omitempty"`
}

// usageJSON mirrors clauderun.TokenUsage's field shape but with stable
// JSON tags the driver package owns. Decoupled from clauderun's struct
// tags so a future change to clauderun's parse-time decode shape
// (currently TotalCostUSD is `json:"-"` because it's sourced from a
// sibling envelope field at parse time) can't silently break sidecar
// readers.
type usageJSON struct {
	InputTokens              int     `json:"input_tokens"`
	OutputTokens             int     `json:"output_tokens"`
	CacheCreationInputTokens int     `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int     `json:"cache_read_input_tokens,omitempty"`
	TotalCostUSD             float64 `json:"total_cost_usd,omitempty"`
}

func recordToJSON(r dispatchRecord) dispatchRecordJSON {
	out := dispatchRecordJSON{
		Agent:     r.agent,
		Model:     r.model,
		Effort:    r.effort,
		ElapsedNS: r.elapsed.Nanoseconds(),
	}
	if r.usage != nil {
		out.Usage = &usageJSON{
			InputTokens:              r.usage.InputTokens,
			OutputTokens:             r.usage.OutputTokens,
			CacheCreationInputTokens: r.usage.CacheCreationInputTokens,
			CacheReadInputTokens:     r.usage.CacheReadInputTokens,
			TotalCostUSD:             r.usage.TotalCostUSD,
		}
	}
	if r.err != nil {
		out.Error = r.err.Error()
	}
	return out
}

func recordFromJSON(j dispatchRecordJSON) dispatchRecord {
	out := dispatchRecord{
		agent:   j.Agent,
		model:   j.Model,
		effort:  j.Effort,
		elapsed: time.Duration(j.ElapsedNS),
	}
	if j.Usage != nil {
		out.usage = &clauderun.TokenUsage{
			InputTokens:              j.Usage.InputTokens,
			OutputTokens:             j.Usage.OutputTokens,
			CacheCreationInputTokens: j.Usage.CacheCreationInputTokens,
			CacheReadInputTokens:     j.Usage.CacheReadInputTokens,
			TotalCostUSD:             j.Usage.TotalCostUSD,
		}
	}
	if j.Error != "" {
		out.err = errors.New(j.Error)
	}
	return out
}

// summaryPath returns the absolute path to this run's summary sidecar.
// Empty string when rs is nil (test fixtures that bypass the driver-
// managed runState); appendSummaryLine treats an empty path as
// "skip the sidecar", same contract as the prompt/events log paths.
func (rs *runState) summaryPath() string {
	if rs == nil {
		return ""
	}
	return filepath.Join(rs.repoPath, ".gh-optivem", "runs", rs.runTimestamp, "summary.jsonl")
}

// appendSummaryLine writes one JSON record to the run's summary sidecar.
// Best-effort: MkdirAll the parent on first write, append-open the file,
// write a single line, close. Empty path is a no-op (rs was nil at
// dispatch time). All errors are returned to the caller, which logs them
// as a warning — the sidecar is diagnostics, not load-bearing.
func appendSummaryLine(path string, r dispatchRecord) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create summary sidecar dir: %w", err)
	}
	data, err := json.Marshal(recordToJSON(r))
	if err != nil {
		return fmt.Errorf("marshal summary record: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open summary sidecar: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("append summary line: %w", err)
	}
	return nil
}

// loadSummary reads one record per line from a sidecar file and returns
// the records in file order. Empty/missing file → (nil, error). Malformed
// lines are skipped silently with no error — same forgiving stance as
// clauderun's parseClaudeStreamJSON, so a single bad line doesn't
// invalidate the whole replay.
func loadSummary(path string) ([]dispatchRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []dispatchRecord
	scanner := bufio.NewScanner(f)
	// Sidecar lines are small (one dispatch each, no embedded prompt
	// bodies) but bump the buffer cap to 1 MiB so a future field
	// addition can't truncate the line silently.
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var j dispatchRecordJSON
		if err := json.Unmarshal(line, &j); err != nil {
			continue
		}
		out = append(out, recordFromJSON(j))
	}
	if err := scanner.Err(); err != nil {
		return out, fmt.Errorf("read summary sidecar: %w", err)
	}
	return out, nil
}

// LatestRunDir returns the path of the most-recently-modified
// <repoPath>/.gh-optivem/runs/<ts>/ directory. Returns an error when the
// runs/ root is missing or contains no subdirectories. Lower-level
// primitive — for the `gh optivem run summary` "no arg" lookup use
// LatestRunWithSummary instead, which skips runs without a sidecar.
func LatestRunDir(repoPath string) (string, error) {
	dirs, err := runDirsByMtime(repoPath)
	if err != nil {
		return "", err
	}
	return dirs[0], nil
}

// LatestRunWithSummary returns the newest <runs>/<ts>/ directory that
// has a non-empty summary.jsonl sidecar. Pre-feature runs and runs that
// bombed before any dispatch fired have no sidecar — `gh optivem run
// summary` (no arg) skips them so the operator lands on actionable
// content rather than an empty-file error pointing at the wrong run.
// Returns an error listing the checked timestamps when no run has a
// sidecar.
func LatestRunWithSummary(repoPath string) (string, error) {
	dirs, err := runDirsByMtime(repoPath)
	if err != nil {
		return "", err
	}
	var skipped []string
	for _, d := range dirs {
		path := filepath.Join(d, "summary.jsonl")
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			return d, nil
		}
		skipped = append(skipped, filepath.Base(d))
	}
	return "", fmt.Errorf("no runs under %s/.gh-optivem/runs have a summary.jsonl sidecar (checked %d run(s): %s)",
		repoPath, len(skipped), strings.Join(skipped, ", "))
}

// runDirsByMtime returns every <runs>/<ts>/ subdirectory sorted
// newest-first by mtime. Errors when the runs/ root is missing or
// contains no subdirectories.
func runDirsByMtime(repoPath string) ([]string, error) {
	root := filepath.Join(repoPath, ".gh-optivem", "runs")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read runs dir %s: %w", root, err)
	}
	type runEntry struct {
		path  string
		mtime time.Time
	}
	var runs []runEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		runs = append(runs, runEntry{path: filepath.Join(root, e.Name()), mtime: info.ModTime()})
	}
	if len(runs) == 0 {
		return nil, fmt.Errorf("no runs found under %s", root)
	}
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].mtime.After(runs[j].mtime)
	})
	out := make([]string, len(runs))
	for i, r := range runs {
		out[i] = r.path
	}
	return out, nil
}

// PrintSummaryFile loads the records from the summary sidecar at path
// and renders the agent-summary table to w. Exported so the cobra layer
// can wire `gh optivem run summary [ts]` without exposing the
// dispatchRecord type.
//
// Returns an error when the file is missing or unreadable; an empty file
// (zero successfully decoded records) is NOT an error — the renderer
// no-ops on empty input and the caller sees a clean exit. That matches
// the "no work produced no summary" semantics of the live banner.
func PrintSummaryFile(w io.Writer, path string) error {
	records, err := loadSummary(path)
	if err != nil {
		return err
	}
	renderAgentSummary(w, records)
	return nil
}

// runDigest carries the ticket context the Markdown run digest fronts the
// agent-summary table with: the issue number / title / url, the parsed
// description + acceptance-criteria excerpt (D2 — checklist and raw body
// are deliberately omitted to keep the digest short), the dispatch
// records (the same []dispatchRecord renderAgentSummary consumes), and
// the overall verdict (nil → succeeded, non-nil → failed).
type runDigest struct {
	issueNum           string
	title              string
	url                string
	description        string
	acceptanceCriteria string
	records            []dispatchRecord
	result             error
}

// summaryMarkdownPath returns the absolute path to this run's human
// digest, beside the machine sidecar summaryPath writes. Empty string
// when rs is nil (test fixtures that bypass the driver-managed runState);
// writeRunDigest treats an empty path as "skip the digest", same contract
// as summaryPath/appendSummaryLine.
func (rs *runState) summaryMarkdownPath() string {
	if rs == nil {
		return ""
	}
	return filepath.Join(rs.repoPath, ".gh-optivem", "runs", rs.runTimestamp, "summary.md")
}

// renderRunDigest writes a short, GitHub-renderable run digest to w:
// ticket header + overall verdict + ticket link, the description and
// acceptance-criteria excerpt as blockquotes (each omitted when empty),
// and the agent-summary table reused verbatim from renderAgentSummary
// inside a fenced block so its column alignment survives Markdown
// rendering. No-op when w is nil.
//
// Single renderer, two callers: writeRunDigest (live emission at run end)
// and PrintRunDigestFile (replay via `gh optivem run summary --markdown`)
// both route through here, mirroring how renderAgentSummary backs both
// the live banner and PrintSummaryFile so the two views never drift.
func renderRunDigest(w io.Writer, d runDigest) {
	if w == nil {
		return
	}

	num := strings.TrimSpace(d.issueNum)
	title := strings.TrimSpace(d.title)
	switch {
	case num != "" && title != "":
		fmt.Fprintf(w, "# Run digest — #%s %s\n", num, title)
	case num != "":
		fmt.Fprintf(w, "# Run digest — #%s\n", num)
	case title != "":
		fmt.Fprintf(w, "# Run digest — %s\n", title)
	default:
		fmt.Fprintln(w, "# Run digest")
	}
	fmt.Fprintln(w)

	if d.result == nil {
		fmt.Fprintln(w, "**Result:** ✅ succeeded")
	} else {
		fmt.Fprintf(w, "**Result:** ❌ failed: %s\n", d.result.Error())
	}
	fmt.Fprintln(w)

	if url := strings.TrimSpace(d.url); url != "" {
		fmt.Fprintf(w, "**Ticket:** %s\n", url)
		fmt.Fprintln(w)
	}

	if desc := strings.TrimSpace(d.description); desc != "" {
		fmt.Fprintln(w, "## Description")
		fmt.Fprintln(w)
		writeBlockquote(w, desc)
		fmt.Fprintln(w)
	}

	if ac := strings.TrimSpace(d.acceptanceCriteria); ac != "" {
		fmt.Fprintln(w, "## Acceptance criteria")
		fmt.Fprintln(w)
		writeBlockquote(w, ac)
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, "## Agents dispatched")
	if len(d.records) == 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "_No agents were dispatched._")
		return
	}
	// renderAgentSummary leads with a blank line then the `=== Agent
	// summary ===` header; fencing it keeps the monospace columns aligned
	// when GitHub renders the Markdown.
	fmt.Fprintln(w, "```")
	renderAgentSummary(w, d.records)
	fmt.Fprintln(w, "```")
}

// writeBlockquote emits text as a Markdown blockquote, prefixing each
// line with "> " (bare ">" for blank lines so the quote block stays
// contiguous in the rendered output).
func writeBlockquote(w io.Writer, text string) {
	for line := range strings.SplitSeq(text, "\n") {
		if line == "" {
			fmt.Fprintln(w, ">")
			continue
		}
		fmt.Fprintf(w, "> %s\n", line)
	}
}

// writeRunDigest renders d to <path> with a truncating create (each run
// owns its own dir, so the file is fresh per run — same shape as the
// --log-file mirror). Best-effort: an empty path is a no-op (rs was nil),
// and all errors are returned to the caller, which logs them as a warning
// — the digest is a convenience artefact, never load-bearing, mirroring
// appendSummaryLine's stance.
func writeRunDigest(path string, d runDigest) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create run digest dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open run digest: %w", err)
	}
	defer f.Close()
	renderRunDigest(f, d)
	return nil
}

// PrintRunDigestFile copies the rendered run digest at path to w. Exported
// so the cobra layer can wire `gh optivem run summary --markdown` by
// reading the emitted file (so the replay is byte-identical with what the
// run wrote), parallel to PrintSummaryFile for the table view. Returns an
// error when the file is missing or unreadable.
func PrintRunDigestFile(w io.Writer, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
