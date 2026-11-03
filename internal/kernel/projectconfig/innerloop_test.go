package projectconfig

import (
	"strings"
	"testing"
)

func TestInnerLoop_AbsentDefaultsToNone(t *testing.T) {
	t.Parallel()
	cfg := validMultitierBase()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("baseline should validate: %v", err)
	}
	if got := cfg.System.Backend.InnerLoop.EffectiveMode(); got != InnerLoopNone {
		t.Fatalf("EffectiveMode = %q, want none", got)
	}
	if cfg.System.Backend.InnerLoop.Enabled() {
		t.Fatal("absent inner-loop must not be enabled")
	}
}

func TestInnerLoop_ValidOnTierMonolithServiceComponent(t *testing.T) {
	t.Parallel()
	loop := InnerLoop{Mode: InnerLoopTestAfter, MutationThreshold: 70}

	mt := validMultitierBase()
	mt.System.Backend.InnerLoop = loop
	if err := mt.Validate(); err != nil {
		t.Fatalf("multitier backend: %v", err)
	}

	mono := validMonolithBase()
	mono.System.InnerLoop = InnerLoop{Mode: InnerLoopTDD}
	if err := mono.Validate(); err != nil {
		t.Fatalf("monolith: %v", err)
	}

	ms := validMicroservicesConfig()
	svc := ms.System.BackendServices["orders"]
	svc.InnerLoop = loop
	ms.System.BackendServices["orders"] = svc
	if err := ms.Validate(); err != nil {
		t.Fatalf("microservices service: %v", err)
	}
}

func TestInnerLoop_Rejections(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		loop InnerLoop
		want string
	}{
		{"bad mode", InnerLoop{Mode: "unit-tests"}, "inner-loop mode"},
		{"threshold without mode", InnerLoop{MutationThreshold: 50}, "requires"},
		{"threshold with none", InnerLoop{Mode: InnerLoopNone, MutationThreshold: 50}, "requires"},
		{"threshold too high", InnerLoop{Mode: InnerLoopTDD, MutationThreshold: 101}, "out of range"},
		{"threshold negative", InnerLoop{Mode: InnerLoopTDD, MutationThreshold: -1}, "out of range"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := validMultitierBase()
			cfg.System.Backend.InnerLoop = tc.loop
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestInnerLoop_RejectedOnSystemTest(t *testing.T) {
	t.Parallel()
	cfg := validMultitierBase()
	cfg.SystemTest.InnerLoop = InnerLoop{Mode: InnerLoopTestAfter}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "system-test.inner-loop") {
		t.Fatalf("expected system-test.inner-loop rejection, got %v", err)
	}
}

func TestInnerLoop_YAMLRoundTrip(t *testing.T) {
	t.Parallel()
	cfg := validMultitierBase()
	cfg.System.Backend.InnerLoop = InnerLoop{Mode: InnerLoopTestAfter, MutationThreshold: 70}
	dir := t.TempDir()
	if err := Write(dir, cfg); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.System.Backend.InnerLoop != cfg.System.Backend.InnerLoop {
		t.Fatalf("round-trip lost inner-loop: got %+v", got.System.Backend.InnerLoop)
	}
}
