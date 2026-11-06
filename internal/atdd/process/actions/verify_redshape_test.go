package actions

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/optivem/gh-optivem/internal/build/runner"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

func TestClassifyRedShape(t *testing.T) {
	for _, tc := range []struct {
		name      string
		ev        runner.ReportFailures
		output    string
		wantClass failureClass
	}{
		{
			name:      "assertion failure in report is a red",
			ev:        runner.ReportFailures{Complete: true, FreshReports: 1, Failures: []runner.FailedTestCase{{Name: "t", Detail: "org.opentest4j.AssertionFailedError: expected 1 but was 0"}}},
			wantClass: classRed,
		},
		{
			name:      "TODO stub exception (expected acceptance red) is a red",
			ev:        runner.ReportFailures{Complete: true, FreshReports: 1, Failures: []runner.FailedTestCase{{Name: "t", Detail: "java.lang.UnsupportedOperationException: TODO: DSL"}}},
			wantClass: classRed,
		},
		{
			name: "playwright TargetClosedError is infra (the implement-76 rehearsal)",
			ev: runner.ReportFailures{Complete: true, FreshReports: 1, Failures: []runner.FailedTestCase{
				{Name: "api", Detail: "AssertionError: expected 201"},
				{Name: "ui", Detail: "TargetClosedError: Target page, context or browser has been closed"},
			}},
			wantClass: classInfra,
		},
		{
			name:      "dotnet test host crash is infra",
			ev:        runner.ReportFailures{Complete: true, FreshReports: 1, Failures: []runner.FailedTestCase{{Name: "t", Detail: "Error\nThe active test run was aborted. Reason: Test host process crashed"}}},
			wantClass: classInfra,
		},
		{
			name:      "no fresh report written is infra (tests did not run)",
			ev:        runner.ReportFailures{Complete: true},
			wantClass: classInfra,
		},
		{
			name:      "fresh report with zero failures is infra",
			ev:        runner.ReportFailures{Complete: true, FreshReports: 2},
			wantClass: classInfra,
		},
		{
			name:      "global run error is infra",
			ev:        runner.ReportFailures{Complete: true, FreshReports: 1, GlobalErrors: []string{"globalSetup failed"}},
			wantClass: classInfra,
		},
		{
			name:      "fallback: no structured evidence, assertion output stays red",
			ev:        runner.ReportFailures{},
			output:    "Expected 'gift-wrapped: true' but got 'gift-wrapped: false'",
			wantClass: classRed,
		},
		{
			name:      "fallback: no structured evidence, crash output is infra",
			ev:        runner.ReportFailures{},
			output:    "Error: browserType.launch: Host system is missing dependencies to run browsers.",
			wantClass: classInfra,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, label := classifyRedShape(tc.ev, tc.output)
			if got != tc.wantClass {
				t.Fatalf("class: got %v (%q), want %v", got, label, tc.wantClass)
			}
			if (got == classInfra) != (label != "") {
				t.Fatalf("label %q inconsistent with class %v", label, got)
			}
		})
	}
}

// TestRunCommand_RedRequiresReportedTestFailure wires classifyRedShape through
// runCommand with a real tests.yaml-shaped config and on-disk reports.
func TestRunCommand_RedRequiresReportedTestFailure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		report string // "" = no report written
		stale  bool
		want   string
	}{
		{
			name:   "assertion failure accepted as red",
			report: `{"suites":[{"specs":[{"title":"shouldX","tests":[{"results":[{"status":"failed","error":{"message":"Error: expect(received).toBe(expected)"}}]}]}]}]}`,
			want:   "fail",
		},
		{
			name:   "playwright crash rejected",
			report: `{"suites":[{"specs":[{"title":"shouldX","tests":[{"results":[{"status":"failed","error":{"message":"TargetClosedError: Target page, context or browser has been closed"}}]}]}]}]}`,
			want:   "infra",
		},
		{name: "no report (tests never ran) rejected", want: "infra"},
		{
			name:   "stale report from an earlier run rejected",
			report: `{"suites":[{"specs":[{"title":"shouldX","tests":[{"results":[{"status":"failed","error":{"message":"Error: expect(received).toBe(expected)"}}]}]}]}]}`,
			stale:  true,
			want:   "infra",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.report != "" {
				p := filepath.Join(dir, "ts", "results.json")
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(tc.report), 0o644); err != nil {
					t.Fatal(err)
				}
				if tc.stale {
					old := time.Now().Add(-time.Hour)
					if err := os.Chtimes(p, old, old); err != nil {
						t.Fatal(err)
					}
				}
			}
			cfg := &runner.TestsConfig{Suites: []runner.Suite{{ID: "smoke", Name: "api", Path: "ts", TestCountPath: "results.json"}}}
			sh := &fakeShell{stderr: []byte("1 failed"), exitCode: 1, err: errors.New("exit 1")}
			a := newActions(Deps{Shell: sh, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, TestsConfig: cfg, TestsCwd: dir})
			ctx := statemachine.NewContext()
			ctx.Params["command"] = "gh optivem system-test run"
			ctx.Params["suite"] = "smoke"
			if out := a.runCommand(ctx); out.Err != nil {
				t.Fatalf("unexpected err: %v", out.Err)
			}
			if got := ctx.GetString("test-outcome"); got != tc.want {
				t.Fatalf("test-outcome: got %q, want %q (label %q)", got, tc.want, ctx.GetString("test-infra-label"))
			}
		})
	}
}
