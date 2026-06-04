package driver

import (
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// fakeScopedGit returns a gitRunFn stand-in that simulates a committed tree: a
// pathspec in `done` has tracked files (`git ls-files` non-empty), one in
// `dirty` has uncommitted changes (`git status --porcelain` non-empty). The
// real gitRunFn shells out; tests swap this in to assert classification without
// a scratch repo. DIRTY is checked before DONE in classifyFootprint, so a
// pathspec present in both reads as DIRTY.
func fakeScopedGit(done, dirty map[string]bool) func(string, ...string) ([]byte, error) {
	return func(_ string, args ...string) ([]byte, error) {
		var specs []string
		afterSep := false
		for _, a := range args {
			if afterSep {
				specs = append(specs, a)
				continue
			}
			if a == "--" {
				afterSep = true
			}
		}
		cmd := args[0]
		for _, s := range specs {
			switch cmd {
			case "status":
				if dirty[s] {
					return []byte(" M " + s + "/x\n"), nil
				}
			case "ls-files":
				if done[s] {
					return []byte(s + "/x\n"), nil
				}
			}
		}
		return nil, nil
	}
}

// withFakeGit swaps gitRunFn for the duration of fn and restores it after.
func withFakeGit(t *testing.T, done, dirty map[string]bool, fn func()) {
	t.Helper()
	prev := gitRunFn
	gitRunFn = fakeScopedGit(done, dirty)
	defer func() { gitRunFn = prev }()
	fn()
}

// testCfg builds a minimal project config whose PlaceholderMap resolves the
// three footprint keys the resume guard reads (at-test, driver-adapter,
// system-db-migration-path) and declares two channels in api,ui order.
func testCfg() *projectconfig.Config {
	cfg := &projectconfig.Config{Channels: []string{"api", "ui"}}
	cfg.SystemTest.Paths = map[string]string{
		"at-test":        "system-test/at",
		"driver-adapter": "system-test/driver/adapter",
	}
	cfg.System.DbMigrationPath = "system/db/migrations"
	return cfg
}

func TestClassifyFootprint(t *testing.T) {
	paths := []string{"a/b"}
	cases := []struct {
		name  string
		done  map[string]bool
		dirty map[string]bool
		want  artifactState
	}{
		{"absent", nil, nil, stateAbsent},
		{"done", map[string]bool{"a/b": true}, nil, stateDone},
		{"dirty", nil, map[string]bool{"a/b": true}, stateDirty},
		{"dirty-wins-over-committed", map[string]bool{"a/b": true}, map[string]bool{"a/b": true}, stateDirty},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withFakeGit(t, tc.done, tc.dirty, func() {
				got, err := classifyFootprint("/repo", paths)
				if err != nil {
					t.Fatalf("classifyFootprint: %v", err)
				}
				if got != tc.want {
					t.Errorf("classifyFootprint = %v, want %v", got, tc.want)
				}
			})
		})
	}
}

// TestClassifyFootprint_EmptyIsError guards the safety rail: a nil/empty
// footprint must not degrade into a bare `git ls-files` over the whole tree.
func TestClassifyFootprint_EmptyIsError(t *testing.T) {
	if _, err := classifyFootprint("/repo", nil); err == nil {
		t.Fatal("classifyFootprint(nil): want error, got nil")
	}
}

func TestResolveScopedEntry_ChannelRule(t *testing.T) {
	cfg := testCfg()
	cases := []struct {
		name    string
		target  Target
		channel string
		wantErr string // substring; "" → expect success
	}{
		{"test-rejects-channel", TargetTest, "api", "channel-agnostic"},
		{"driver-adapter-requires-channel", TargetDriverAdapter, "", "requires --channel"},
		{"system-requires-channel", TargetSystem, "", "requires --channel"},
		{"unknown-channel", TargetDriverAdapter, "grpc", "not declared"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withFakeGit(t, nil, nil, func() {
				sCtx := statemachine.NewContext()
				_, err := resolveScopedEntry(tc.target, tc.channel, cfg, "/repo", sCtx)
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("resolveScopedEntry err = %v, want substring %q", err, tc.wantErr)
				}
			})
		})
	}
}

func TestResolveScopedEntry_Test_NoUpstream_SeedsFailure(t *testing.T) {
	cfg := testCfg()
	withFakeGit(t, nil, nil, func() { // empty tree: test slice has no upstream so it still resolves
		sCtx := statemachine.NewContext()
		proc, err := resolveScopedEntry(TargetTest, "", cfg, "/repo", sCtx)
		if err != nil {
			t.Fatalf("resolveScopedEntry(test): %v", err)
		}
		if proc != "shared-contract" {
			t.Errorf("process = %q, want shared-contract", proc)
		}
		if got := sCtx.Params["expected-test-result"]; got != "failure" {
			t.Errorf("expected-test-result = %q, want failure", got)
		}
		if _, ok := sCtx.Params["channel"]; ok {
			t.Errorf("test slice must not seed a channel param, got %q", sCtx.Params["channel"])
		}
	})
}

func TestResolveScopedEntry_DriverAdapter_BlockedUntilSharedContract(t *testing.T) {
	cfg := testCfg()
	// Shared contract ABSENT → refuse.
	withFakeGit(t, nil, nil, func() {
		sCtx := statemachine.NewContext()
		_, err := resolveScopedEntry(TargetDriverAdapter, "api", cfg, "/repo", sCtx)
		if err == nil || !strings.Contains(err.Error(), "shared contract") {
			t.Fatalf("err = %v, want shared-contract block", err)
		}
	})
	// Shared contract DONE → allow, seed channel params.
	withFakeGit(t, map[string]bool{"system-test/at": true}, nil, func() {
		sCtx := statemachine.NewContext()
		proc, err := resolveScopedEntry(TargetDriverAdapter, "api", cfg, "/repo", sCtx)
		if err != nil {
			t.Fatalf("resolveScopedEntry(driver-adapter,api): %v", err)
		}
		if proc != "implement-and-verify-system-driver-adapters" {
			t.Errorf("process = %q", proc)
		}
		if sCtx.Params["channel"] != "api" || sCtx.Params["tests"] != "acceptance" || sCtx.Params["expected-test-result"] != "failure" {
			t.Errorf("params = %#v", sCtx.Params)
		}
	})
}

func TestResolveScopedEntry_DriverAdapter_DirtyIsNotDone(t *testing.T) {
	cfg := testCfg()
	// Shared contract present but uncommitted → DIRTY → still refused (the
	// cross-clone handoff is the commit).
	withFakeGit(t, nil, map[string]bool{"system-test/at": true}, func() {
		sCtx := statemachine.NewContext()
		_, err := resolveScopedEntry(TargetDriverAdapter, "api", cfg, "/repo", sCtx)
		if err == nil || !strings.Contains(err.Error(), "uncommitted") {
			t.Fatalf("err = %v, want uncommitted (dirty) block", err)
		}
	})
}

func TestResolveScopedEntry_System_FirstChannel_CommonTrue(t *testing.T) {
	cfg := testCfg() // channels: [api, ui]; api is first
	done := map[string]bool{
		"system-test/at":                 true, // shared contract
		"system-test/driver/adapter/api": true, // api adapter
	}
	withFakeGit(t, done, nil, func() {
		sCtx := statemachine.NewContext()
		proc, err := resolveScopedEntry(TargetSystem, "api", cfg, "/repo", sCtx)
		if err != nil {
			t.Fatalf("resolveScopedEntry(system,api): %v", err)
		}
		if proc != "implement-and-verify-system" {
			t.Errorf("process = %q", proc)
		}
		if sCtx.Params["common"] != "true" {
			t.Errorf("common = %q, want true (first channel)", sCtx.Params["common"])
		}
		if sCtx.Params["suite"] != "acceptance-api" {
			t.Errorf("suite = %q, want acceptance-api", sCtx.Params["suite"])
		}
		if sCtx.Params["expected-test-result"] != "success" {
			t.Errorf("expected-test-result = %q, want success", sCtx.Params["expected-test-result"])
		}
	})
}

func TestResolveScopedEntry_System_LaterChannel_RequiresCommon(t *testing.T) {
	cfg := testCfg() // ui is NOT first → needs common (migration) DONE
	base := map[string]bool{
		"system-test/at":                true, // shared contract
		"system-test/driver/adapter/ui": true, // ui adapter
	}
	// Common (migration) ABSENT → refuse.
	withFakeGit(t, base, nil, func() {
		sCtx := statemachine.NewContext()
		_, err := resolveScopedEntry(TargetSystem, "ui", cfg, "/repo", sCtx)
		if err == nil || !strings.Contains(err.Error(), "common layer") {
			t.Fatalf("err = %v, want common-layer block", err)
		}
	})
	// Common present → allow, common:false for the later channel.
	withCommon := map[string]bool{
		"system-test/at":                true,
		"system-test/driver/adapter/ui": true,
		"system/db/migrations":          true,
	}
	withFakeGit(t, withCommon, nil, func() {
		sCtx := statemachine.NewContext()
		_, err := resolveScopedEntry(TargetSystem, "ui", cfg, "/repo", sCtx)
		if err != nil {
			t.Fatalf("resolveScopedEntry(system,ui): %v", err)
		}
		if sCtx.Params["common"] != "false" {
			t.Errorf("common = %q, want false (later channel)", sCtx.Params["common"])
		}
	})
}

// TestResolveScopedEntry_NilConfig guards that a scoped run without a
// gh-optivem.yaml fails with a clear message rather than a nil-map panic.
func TestResolveScopedEntry_NilConfig(t *testing.T) {
	withFakeGit(t, nil, nil, func() {
		sCtx := statemachine.NewContext()
		_, err := resolveScopedEntry(TargetTest, "", nil, "/repo", sCtx)
		if err == nil || !strings.Contains(err.Error(), "gh-optivem.yaml") {
			t.Fatalf("err = %v, want gh-optivem.yaml requirement", err)
		}
	})
}

// TestTarget_ExpectedTestResult pins the per-slice gate values (D-red-gate).
func TestTarget_ExpectedTestResult(t *testing.T) {
	cases := []struct {
		target Target
		want   string
		wantOK bool
	}{
		{TargetTest, "failure", true},
		{TargetDriverAdapter, "failure", true},
		{TargetSystem, "success", true},
		{TargetUnset, "", false},
	}
	for _, tc := range cases {
		got, ok := tc.target.ExpectedTestResult()
		if ok != tc.wantOK || got != tc.want {
			t.Errorf("%q.ExpectedTestResult() = (%q,%v), want (%q,%v)", tc.target, got, ok, tc.want, tc.wantOK)
		}
	}
}
