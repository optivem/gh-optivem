package driver

import (
	"fmt"
	"io"
	"strings"

	"github.com/optivem/gh-optivem/internal/atdd/process/actions"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// resumableUnits are the callee processes that end in a commit of their own,
// so a later run can prove them done from the branch history. Contract-test and
// stub-fidelity writers and the external-system adapters have no commit of
// their own and always re-run as part of their external-system unit.
var resumableUnits = map[string]bool{
	"write-and-verify-acceptance-test-code":       true,
	"implement-and-verify-dsl":                    true,
	"implement-and-verify-system":                 true,
	"implement-and-verify-system-driver-adapters": true,
	"refactor-and-verify-tests":                   true,
}

// resumeExcludeParams keeps the unrolled external-system calls out of the
// skippable set: the external-system unit always re-runs whole.
var resumeExcludeParams = []string{"external-system-name"}

// resumeUnitKeyState scopes characterize's layer units to the current use
// case: the same call-activity runs once per use case, so its marker must
// name the use case (ticket flows never set use-case-id; their unit ids are
// unchanged).
var resumeUnitKeyState = []string{actions.CtxKeyUseCaseID}

// resumeExtraStateKeys are derived state keys (not declared agent outputs)
// that later steps read and a skipped unit must therefore restore.
//
// The characterize keys keep the start commits the safety gate diffs against,
// the stand-in paths, the assertion-check cursor and the use case loop
// cursors and the criteria to delete at the end across a resume (plan
// 20261110-1130).
var resumeExtraStateKeys = []string{
	"external-driver-port-changed-paths",
	actions.CtxKeyAcceptanceReports,
	actions.CtxKeySystemStartCommits,
	actions.CtxKeyStandInPaths,
	actions.CtxKeyAssertionCheckedCommit,
	actions.CtxKeySystemTestStartCommit,
	actions.CtxKeyUseCaseCursors,
	actions.CtxKeyUseCaseID,
	actions.CtxKeyMergedCriteria,
}

// newResumeConfig builds the engine's resume configuration for a full-pipeline
// walk over the repository at repoPath.
func newResumeConfig(repoPath string, log io.Writer) *statemachine.ResumeConfig {
	return &statemachine.ResumeConfig{
		Units:             resumableUnits,
		ExcludeWhenParams: resumeExcludeParams,
		UnitKeyState:      resumeUnitKeyState,
		ExtraStateKeys:    resumeExtraStateKeys,
		Probe:             gitResumeProbe{repoPath: repoPath},
		Log:               log,
	}
}

// gitResumeProbe proves unit completion from the current branch's history.
type gitResumeProbe struct {
	repoPath string
}

const (
	logFieldSep  = "\x1f"
	logRecordSep = "\x1e"
)

func (p gitResumeProbe) FindUnitCommit(ticket, unit string) (statemachine.UnitCommit, bool, error) {
	if _, err := gitRunFn(p.repoPath, "rev-parse", "--verify", "--quiet", "HEAD"); err != nil {
		// No HEAD commit yet: nothing can be marked done.
		return statemachine.UnitCommit{}, false, nil
	}
	out, err := gitRunFn(p.repoPath, "log", "HEAD", "--fixed-strings",
		"--grep="+statemachine.TrailerTicket+": "+ticket,
		"--format=%H"+logFieldSep+"%B"+logRecordSep)
	if err != nil {
		return statemachine.UnitCommit{}, false, fmt.Errorf("read branch history: %w", err)
	}
	return matchUnitCommit(string(out), ticket, unit)
}

// matchUnitCommit picks the newest record (git log order) whose trailers name
// exactly this ticket and unit.
func matchUnitCommit(logOut, ticket, unit string) (statemachine.UnitCommit, bool, error) {
	for _, rec := range strings.Split(logOut, logRecordSep) {
		sha, body, ok := strings.Cut(strings.TrimLeft(rec, "\r\n"), logFieldSep)
		if !ok {
			continue
		}
		if !hasTrailer(body, statemachine.TrailerTicket, ticket) || !hasTrailer(body, statemachine.TrailerUnit, unit) {
			continue
		}
		states := statemachine.ParseTrailerLines(body, statemachine.TrailerState)
		if len(states) != 1 {
			return statemachine.UnitCommit{}, false, fmt.Errorf("commit %s marks unit %q but carries %d %s trailers (want 1)", sha, unit, len(states), statemachine.TrailerState)
		}
		return statemachine.UnitCommit{SHA: sha, State: states[0]}, true, nil
	}
	return statemachine.UnitCommit{}, false, nil
}

func hasTrailer(body, key, want string) bool {
	for _, v := range statemachine.ParseTrailerLines(body, key) {
		if strings.TrimSpace(v) == want {
			return true
		}
	}
	return false
}

func (p gitResumeProbe) WorkingTreeClean() (bool, error) {
	out, err := gitRunFn(p.repoPath, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("read working tree status: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" || isPromptTempFile(line) {
			continue
		}
		return false, nil
	}
	return true, nil
}

// isPromptTempFile reports whether a `git status --porcelain` line is an
// untracked prompt temp file (.atdd-prompt-*.tmp.md) left behind by an agent
// dispatch that was killed mid-run. It is harness scratch, not unfinished work.
func isPromptTempFile(line string) bool {
	path, ok := strings.CutPrefix(line, "?? ")
	if !ok {
		return false
	}
	return strings.HasPrefix(path, ".atdd-prompt-") && strings.HasSuffix(path, ".tmp.md")
}
