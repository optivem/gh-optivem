package driver

import (
	"errors"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/process"
)

func logRecord(sha, subject string, trailers ...string) string {
	return sha + logFieldSep + subject + "\n\n" + strings.Join(trailers, "\n") + "\n" + logRecordSep + "\n"
}

func TestMatchUnitCommit_PicksNewestExactTicketAndUnit(t *testing.T) {
	out := logRecord("new", "s", "Atdd-Ticket: 65", "Atdd-Unit: p@DSL", `Atdd-State: {"n":"new"}`) +
		logRecord("wrong-unit", "s", "Atdd-Ticket: 65", "Atdd-Unit: p@OTHER", `Atdd-State: {}`) +
		logRecord("wrong-ticket", "s", "Atdd-Ticket: 650", "Atdd-Unit: p@DSL", `Atdd-State: {}`) +
		logRecord("old", "s", "Atdd-Ticket: 65", "Atdd-Unit: p@DSL", `Atdd-State: {"n":"old"}`)

	got, found, err := matchUnitCommit(out, "65", "p@DSL")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if got.SHA != "new" || got.State != `{"n":"new"}` {
		t.Errorf("got %+v, want the newest matching commit", got)
	}
}

func TestMatchUnitCommit_NoMarkerOrOldCommitsWithoutTrailers(t *testing.T) {
	out := logRecord("plain", "#65 Add thing - DSL")
	if _, found, err := matchUnitCommit(out, "65", "p@DSL"); found || err != nil {
		t.Fatalf("found=%v err=%v, want not found", found, err)
	}
}

func TestMatchUnitCommit_MarkerWithoutStateFailsLoud(t *testing.T) {
	out := logRecord("bad", "s", "Atdd-Ticket: 65", "Atdd-Unit: p@DSL")
	if _, _, err := matchUnitCommit(out, "65", "p@DSL"); err == nil {
		t.Fatal("a marker commit lacking Atdd-State must fail loud")
	}
}

func TestGitResumeProbe_FindUnitCommitAndTreeState(t *testing.T) {
	prev := gitRunFn
	defer func() { gitRunFn = prev }()
	var logArgs []string
	status := ""
	gitRunFn = func(_ string, args ...string) ([]byte, error) {
		switch args[0] {
		case "rev-parse":
			return []byte("deadbeef\n"), nil
		case "log":
			logArgs = args
			return []byte(logRecord("sha1", "s", "Atdd-Ticket: 65", "Atdd-Unit: p@DSL", `Atdd-State: {}`)), nil
		case "status":
			return []byte(status), nil
		}
		return nil, errors.New("unexpected git call")
	}
	p := gitResumeProbe{repoPath: "r"}

	c, found, err := p.FindUnitCommit("65", "p@DSL")
	if err != nil || !found || c.SHA != "sha1" {
		t.Fatalf("c=%+v found=%v err=%v", c, found, err)
	}
	if logArgs[1] != "HEAD" {
		t.Errorf("history must be the current branch (HEAD), args = %v", logArgs)
	}
	if clean, err := p.WorkingTreeClean(); err != nil || !clean {
		t.Errorf("clean = %v err = %v, want clean", clean, err)
	}
	status = "?? new.txt\n"
	if clean, _ := p.WorkingTreeClean(); clean {
		t.Error("untracked file must make the tree dirty")
	}
	status = "?? .atdd-prompt-123.tmp.md\n"
	if clean, _ := p.WorkingTreeClean(); !clean {
		t.Error("prompt temp file alone must not make the tree dirty")
	}
	status = "?? .atdd-prompt-123.tmp.md\n M a.go\n"
	if clean, _ := p.WorkingTreeClean(); clean {
		t.Error("tracked edit must make the tree dirty even with a prompt temp file")
	}
}

func TestGitResumeProbe_NoHeadMeansNoMarker(t *testing.T) {
	prev := gitRunFn
	defer func() { gitRunFn = prev }()
	gitRunFn = func(_ string, args ...string) ([]byte, error) {
		if args[0] == "rev-parse" {
			return nil, errors.New("no HEAD")
		}
		return nil, errors.New("log must not run without HEAD")
	}
	if _, found, err := (gitResumeProbe{repoPath: "r"}).FindUnitCommit("65", "p@DSL"); found || err != nil {
		t.Fatalf("found=%v err=%v, want not found without error", found, err)
	}
}

func TestGitResumeProbe_HistoryReadErrorPropagates(t *testing.T) {
	prev := gitRunFn
	defer func() { gitRunFn = prev }()
	gitRunFn = func(_ string, args ...string) ([]byte, error) {
		if args[0] == "log" {
			return nil, errors.New("boom")
		}
		return []byte("x"), nil
	}
	if _, _, err := (gitResumeProbe{repoPath: "r"}).FindUnitCommit("65", "p@DSL"); err == nil {
		t.Fatal("git log failure must be surfaced")
	}
}

// TestNewResumeConfig_ScopesUnitsToUseCase: characterize runs its layer units
// once per use case, so the unit id carries use-case-id.
func TestNewResumeConfig_ScopesUnitsToUseCase(t *testing.T) {
	cfg := newResumeConfig(t.TempDir(), nil)
	if len(cfg.UnitKeyState) != 1 || cfg.UnitKeyState[0] != "use-case-id" {
		t.Errorf("UnitKeyState = %q, want [use-case-id]", cfg.UnitKeyState)
	}
}

// TestTopProcesses_CoverCLIEntryProcesses: every process a CLI command enters
// gets phase banners and exists in the embedded flow.
func TestTopProcesses_CoverCLIEntryProcesses(t *testing.T) {
	eng, err := process.Load()
	if err != nil {
		t.Fatalf("load embedded flow: %v", err)
	}
	for _, name := range []string{DefaultProcessName, CharacterizeProcessName} {
		if !topProcesses[name] {
			t.Errorf("topProcesses is missing CLI entry process %q", name)
		}
		if _, ok := eng.Processes[name]; !ok {
			t.Errorf("CLI entry process %q is not in the embedded flow", name)
		}
	}
}
