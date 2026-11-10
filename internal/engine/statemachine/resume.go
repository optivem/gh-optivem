package statemachine

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Commit-trailer keys that mark a resumable unit on the branch history.
const (
	// TrailerTicket scopes a unit marker to one ticket so commits left on a
	// shared base branch by an earlier ticket never count as markers.
	TrailerTicket = "Atdd-Ticket"
	// TrailerUnit identifies the unit: "<calling process>@<call-activity node>".
	TrailerUnit = "Atdd-Unit"
	// TrailerState carries the unit's output state as one line of JSON.
	TrailerState = "Atdd-State"

	ticketStateKey = "ticket-id"
)

// UnitCommit is the newest branch commit carrying a unit's trailers.
type UnitCommit struct {
	SHA   string
	State string // raw Atdd-State value
}

// ResumeProbe is the engine's read-only view of the repository, used to prove
// a unit is already complete.
type ResumeProbe interface {
	// FindUnitCommit returns the newest commit on the current branch's history
	// carrying both the ticket and unit trailers; found is false when none does.
	FindUnitCommit(ticket, unit string) (commit UnitCommit, found bool, err error)
	// WorkingTreeClean reports whether the working tree has no uncommitted
	// changes (tracked or untracked).
	WorkingTreeClean() (bool, error)
}

// ResumeConfig enables skipping call-activities that a previous run already
// completed and committed on the current branch. A unit is skipped only when
// its trailers are found on the branch history AND the working tree is clean;
// anything else runs normally.
type ResumeConfig struct {
	// Units is the set of callee process names that end in a commit of their
	// own and may therefore be skipped.
	Units map[string]bool
	// ExcludeWhenParams lists params whose presence (non-empty) in the call
	// scope keeps a call-activity out of the skippable set — used for the
	// unrolled external-system units, which always re-run whole.
	ExcludeWhenParams []string
	// UnitKeyState lists state keys whose non-empty value is appended to the
	// unit id ("<parent>@<node>@<value>"), so a unit entered once per item in
	// one walk (characterize's layers, once per use case under
	// `use-case-id`) is proven done per item: without it the first item's
	// marker would skip the same unit for every later item.
	UnitKeyState []string
	// ExtraStateKeys are state keys persisted besides the declared agent
	// outputs.
	ExtraStateKeys []string
	Probe          ResumeProbe
	// Log receives one line per skip decision that is not a plain "run".
	Log io.Writer
}

// resumableUnit returns the unit id for a call-activity when it is eligible
// for resume, or "" when it is not.
func (e *Engine) resumableUnit(ctx *Context, parentProcess string, raw RawNode, calleeProcess string, params map[string]string) string {
	cfg := e.Resume
	if cfg == nil || !cfg.Units[calleeProcess] {
		return ""
	}
	for _, p := range cfg.ExcludeWhenParams {
		if params[p] != "" {
			return ""
		}
	}
	unit := parentProcess + "@" + raw.ID
	for _, k := range cfg.UnitKeyState {
		if v := ctx.GetString(k); v != "" {
			unit += "@" + v
		}
	}
	return unit
}

// persistedKeys returns the state keys saved in a unit's trailer, mapped to
// their coercion type: every declared agent output (minus the scope-exception
// envelope) plus the configured extras.
func (e *Engine) persistedKeys() map[string]string {
	keys := map[string]string{}
	for _, p := range e.Processes {
		for _, o := range p.Outputs {
			keys[o.Key] = o.Type
		}
	}
	for _, o := range EnvelopeOutputSpecs() {
		delete(keys, o.Key)
	}
	if e.Resume != nil {
		for _, k := range e.Resume.ExtraStateKeys {
			if _, ok := keys[k]; !ok {
				keys[k] = OutputTypeString
			}
		}
	}
	return keys
}

// CommitTrailers returns the trailer lines ("Key: value") the commit made
// inside the current resumable unit must carry. It returns nil when the
// commit is not inside a resumable unit.
func (e *Engine) CommitTrailers(ctx *Context) ([]string, error) {
	if ctx.Unit == "" {
		return nil, nil
	}
	ticket := ctx.GetString(ticketStateKey)
	if ticket == "" {
		return nil, fmt.Errorf("resume: cannot mark unit %q: state key %q is not set", ctx.Unit, ticketStateKey)
	}
	state := map[string]any{}
	for k := range e.persistedKeys() {
		if v, ok := ctx.State[k]; ok {
			state[k] = v
		}
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("resume: encode state for unit %q: %w", ctx.Unit, err)
	}
	return []string{
		TrailerTicket + ": " + ticket,
		TrailerUnit + ": " + ctx.Unit,
		TrailerState + ": " + string(raw),
	}, nil
}

// restoreState writes a unit's saved state back into ctx.State, coercing each
// value to its declared type. An unknown key or a mistyped value is an error:
// a trailer that cannot be restored exactly is never trusted.
func (e *Engine) restoreState(ctx *Context, raw string) error {
	var saved map[string]any
	if err := json.Unmarshal([]byte(raw), &saved); err != nil {
		return fmt.Errorf("decode %s: %w", TrailerState, err)
	}
	types := e.persistedKeys()
	restored := make(map[string]any, len(saved))
	for k, v := range saved {
		typ, known := types[k]
		if !known {
			return fmt.Errorf("%s carries unknown state key %q", TrailerState, k)
		}
		val, err := coerceSaved(k, typ, v)
		if err != nil {
			return err
		}
		restored[k] = val
	}
	for k, v := range restored {
		ctx.Set(k, v)
	}
	return nil
}

func coerceSaved(key, typ string, v any) (any, error) {
	switch typ {
	case OutputTypeBool:
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("%s key %q: expected bool, got %T", TrailerState, key, v)
		}
		return b, nil
	case OutputTypeStringList:
		items, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("%s key %q: expected list, got %T", TrailerState, key, v)
		}
		out := make([]string, len(items))
		for i, item := range items {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%s key %q: list item %d is %T, expected string", TrailerState, key, i, item)
			}
			out[i] = s
		}
		return out, nil
	default:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%s key %q: expected string, got %T", TrailerState, key, v)
		}
		return s, nil
	}
}

// trySkipUnit decides whether the unit is provably complete. It returns true
// (after restoring the unit's state and logging the proof) only when the
// unit's trailers are on the branch history and the working tree is clean.
// A unit already entered earlier in this run is a deliberate re-execution and
// is never skipped.
func (e *Engine) trySkipUnit(ctx *Context, unit string) (bool, error) {
	cfg := e.Resume
	if ctx.unitsSeen == nil {
		ctx.unitsSeen = map[string]bool{}
	}
	seen := ctx.unitsSeen[unit]
	ctx.unitsSeen[unit] = true
	if seen {
		return false, nil
	}
	if cfg.Probe == nil {
		return false, fmt.Errorf("resume: unit %q: no repository probe configured", unit)
	}
	ticket := ctx.GetString(ticketStateKey)
	if ticket == "" {
		return false, fmt.Errorf("resume: unit %q: state key %q is not set", unit, ticketStateKey)
	}
	commit, found, err := cfg.Probe.FindUnitCommit(ticket, unit)
	if err != nil {
		return false, fmt.Errorf("resume: unit %q: %w", unit, err)
	}
	if !found {
		return false, nil
	}
	clean, err := cfg.Probe.WorkingTreeClean()
	if err != nil {
		return false, fmt.Errorf("resume: unit %q: %w", unit, err)
	}
	if !clean {
		e.logResume("resume: re-running %s: commit %s marks it done but the working tree has uncommitted changes (restore or commit them, e.g. git checkout HEAD -- <file>, to skip this unit instead of re-running it)", unit, shortSHA(commit.SHA))
		return false, nil
	}
	if err := e.restoreState(ctx, commit.State); err != nil {
		return false, fmt.Errorf("resume: unit %q commit %s: %w", unit, shortSHA(commit.SHA), err)
	}
	e.logResume("resume: skipping %s: already done in commit %s", unit, commit.SHA)
	return true, nil
}

func (e *Engine) logResume(format string, args ...any) {
	if e.Resume != nil && e.Resume.Log != nil {
		fmt.Fprintf(e.Resume.Log, format+"\n", args...)
	}
}

func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// ParseTrailerLines extracts the trailer values named key from a commit
// message body, in order of appearance.
func ParseTrailerLines(message, key string) []string {
	var out []string
	prefix := key + ": "
	for _, line := range strings.Split(message, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, prefix) {
			out = append(out, strings.TrimPrefix(line, prefix))
		}
	}
	return out
}
