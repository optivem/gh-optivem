package actions

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/build/runner"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// ---------------------------------------------------------------------------
// resolve-channel + validate-channels-registered — channel-aware system unroll
// (plan 20260619-1139). The in-cycle membership guard reads the RED acceptance
// run's on-disk acceptance-<ch> report (runner.NamesInReport) with no live run
// and no cache (decision #6): a failing test still appears in the report, so
// membership is independent of pass/fail.
// ---------------------------------------------------------------------------

// stageChannelReport writes a JUnit report file under dir and returns a Suite
// reading it via TestCountPath, matching what the RED acceptance run produced.
// An empty body still writes a valid (test-free) report; pass "" to model a
// partition whose filtered RED run matched nothing.
func stageChannelReport(t *testing.T, dir, id string, testNames ...string) runner.Suite {
	t.Helper()
	var cases strings.Builder
	for _, n := range testNames {
		cases.WriteString(`<testcase name="`)
		cases.WriteString(n)
		cases.WriteString(`"/>`)
	}
	body := `<testsuite tests="` + strconv.Itoa(len(testNames)) + `">` + cases.String() + `</testsuite>`
	file := id + ".xml"
	if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o644); err != nil {
		t.Fatalf("write report %s: %v", file, err)
	}
	return runner.Suite{ID: id, Name: id, Path: ".", TestCountPath: file}
}

// apiOnlyTestsConfig stages the #76 shape: an API-only acceptance test landed in
// the api partitions' reports, while the ui partitions exist but wrote no report
// (the RED filtered run matched nothing in UI). channels: api, ui is configured.
func apiOnlyTestsConfig(t *testing.T, dir, apiTest string) (*runner.TestsConfig, *projectconfig.Config) {
	t.Helper()
	tests := &runner.TestsConfig{Suites: []runner.Suite{
		stageChannelReport(t, dir, "acceptance-parallel-api", apiTest),
		stageChannelReport(t, dir, "acceptance-isolated-api"),
		// UI partitions declared, reports deliberately absent.
		{ID: "acceptance-parallel-ui", Name: "acceptance-parallel-ui", Path: ".", TestCountPath: "acceptance-parallel-ui.xml"},
		{ID: "acceptance-isolated-ui", Name: "acceptance-isolated-ui", Path: ".", TestCountPath: "acceptance-isolated-ui.xml"},
	}}
	cfg := &projectconfig.Config{Channels: []string{"api", "ui"}}
	return tests, cfg
}

func channelCtx(channel, testNames string) *statemachine.Context {
	ctx := statemachine.NewContext()
	ctx.Params = map[string]string{"channel": channel, "test-names": testNames}
	return ctx
}

// TestResolveChannel_APIOnlyTicket_UIUntouched is the #76 regression: an
// API-only acceptance test means the UI clone's gate reads an empty UI report →
// channel-touched=false → the UI implement/verify is skipped.
func TestResolveChannel_APIOnlyTicket_UIUntouched(t *testing.T) {
	dir := t.TempDir()
	tests, cfg := apiOnlyTestsConfig(t, dir, "shouldRejectQty100")
	a := newActions(Deps{Config: cfg, TestsConfig: tests, TestsCwd: dir})

	ctx := channelCtx("ui", "shouldRejectQty100")
	if out := a.resolveChannel(ctx); out.Err != nil {
		t.Fatalf("resolve-channel(ui): %v", out.Err)
	}
	if got, _ := ctx.State["channel-touched"].(bool); got {
		t.Errorf("channel-touched(ui): got true, want false (API-only ticket → UI clone must skip)")
	}
}

// TestResolveChannel_APIOnlyTicket_APITouched is the other half: the API clone
// reads its own non-empty report → channel-touched=true → it runs.
func TestResolveChannel_APIOnlyTicket_APITouched(t *testing.T) {
	dir := t.TempDir()
	tests, cfg := apiOnlyTestsConfig(t, dir, "shouldRejectQty100")
	a := newActions(Deps{Config: cfg, TestsConfig: tests, TestsCwd: dir})

	ctx := channelCtx("api", "shouldRejectQty100")
	if out := a.resolveChannel(ctx); out.Err != nil {
		t.Fatalf("resolve-channel(api): %v", out.Err)
	}
	if got, _ := ctx.State["channel-touched"].(bool); !got {
		t.Errorf("channel-touched(api): got false, want true (the API test is in the API report)")
	}
}

// TestResolveChannel_NoChannelParam_RunsNormally covers the structural callers
// (redesign/refactor reuse implement-and-verify-system with no channel unroll):
// an empty channel means "not a per-channel clone" → touched=true, inert guard.
func TestResolveChannel_NoChannelParam_RunsNormally(t *testing.T) {
	a := newActions(Deps{})
	ctx := statemachine.NewContext()
	ctx.Params = map[string]string{"test-names": ""}
	if out := a.resolveChannel(ctx); out.Err != nil {
		t.Fatalf("resolve-channel(no channel): %v", out.Err)
	}
	if got, _ := ctx.State["channel-touched"].(bool); !got {
		t.Errorf("channel-touched(no channel): got false, want true (structural caller must run)")
	}
}

// TestValidateChannelsRegistered_TestInAConfiguredChannel_OK: the API-only test
// ran in the api channel, which is configured, so the upfront guard passes.
func TestValidateChannelsRegistered_TestInAConfiguredChannel_OK(t *testing.T) {
	dir := t.TempDir()
	tests, cfg := apiOnlyTestsConfig(t, dir, "shouldRejectQty100")
	a := newActions(Deps{Config: cfg, TestsConfig: tests, TestsCwd: dir})

	ctx := statemachine.NewContext()
	ctx.Set("at-test-names", "shouldRejectQty100")
	if out := a.validateChannelsRegistered(ctx); out.Err != nil {
		t.Fatalf("validate-channels-registered: unexpected err %v", out.Err)
	}
}

// TestValidateChannelsRegistered_OrphanTestFailsLoud: a ticket test that ran in
// NO configured channel (registers for an unconfigured channel, or does not
// exist) must hard-error — never silently vanish through every per-channel gate.
func TestValidateChannelsRegistered_OrphanTestFailsLoud(t *testing.T) {
	dir := t.TempDir()
	tests, cfg := apiOnlyTestsConfig(t, dir, "shouldRejectQty100")
	a := newActions(Deps{Config: cfg, TestsConfig: tests, TestsCwd: dir})

	ctx := statemachine.NewContext()
	ctx.Set("at-test-names", "shouldDoMobileThing")
	out := a.validateChannelsRegistered(ctx)
	if out.Err == nil {
		t.Fatalf("validate-channels-registered: want hard error for a test in no configured channel, got nil")
	}
	if !strings.Contains(out.Err.Error(), "shouldDoMobileThing") {
		t.Errorf("error should name the orphan test, got: %v", out.Err)
	}
}
