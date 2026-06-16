// Tests for fix_progress.go — the verify-tests-pass no-progress guard.
package actions

import (
	"testing"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// fixFailureSignature must collapse the volatile tokens that differ
// between two runs of the SAME failing tests (durations, clock times,
// ANSI colour, cosmetic whitespace) to equal keys, while keeping
// genuinely different failures distinct.
func TestFixFailureSignature(t *testing.T) {
	t.Run("empty / blank → empty", func(t *testing.T) {
		for _, in := range []string{"", "   ", "\n\t\n"} {
			if got := fixFailureSignature(in); got != "" {
				t.Errorf("fixFailureSignature(%q) = %q, want empty", in, got)
			}
		}
	})

	t.Run("same failure, volatile tokens differ → equal", func(t *testing.T) {
		pairs := []struct{ a, b string }{
			// Gradle build-time tail differs only in the duration.
			{"PlaceOrderNegativeTest > shouldReject() FAILED\nBUILD FAILED in 12s",
				"PlaceOrderNegativeTest > shouldReject() FAILED\nBUILD FAILED in 1m 4s"},
			// Maven "Total time" + a leading wall-clock timestamp.
			{"07:10:29 ERROR Total time: 12.345 s\nTests run: 5, Failures: 1",
				"08:02:11 ERROR Total time: 9.001 s\nTests run: 5, Failures: 1"},
			// ANSI colour + reflowed whitespace.
			{"\x1b[31mFAILED\x1b[0m   shouldReject",
				"FAILED shouldReject"},
		}
		for i, p := range pairs {
			if fixFailureSignature(p.a) != fixFailureSignature(p.b) {
				t.Errorf("pair %d: signatures differ but should match\n a=%q\n b=%q", i,
					fixFailureSignature(p.a), fixFailureSignature(p.b))
			}
		}
	})

	t.Run("different failures → distinct", func(t *testing.T) {
		a := "PlaceOrderNegativeTest > shouldRejectQuantity100() FAILED\nBUILD FAILED in 12s"
		b := "PlaceOrderNegativeTest > shouldRejectQuantity200() FAILED\nBUILD FAILED in 12s"
		if fixFailureSignature(a) == fixFailureSignature(b) {
			t.Errorf("distinct failing tests collapsed to one signature: %q", fixFailureSignature(a))
		}
	})

	t.Run("unit-less numbers in test names survive", func(t *testing.T) {
		// The duration rule must not eat "100" in a test name (it has no
		// time-unit suffix), so two different ...Of100 / ...Of200 failures
		// stay distinct — already covered above, but pin the token directly.
		if got := fixFailureSignature("shouldRejectOrderWithLineQuantityOf100 FAILED"); got != "shouldRejectOrderWithLineQuantityOf100 FAILED" {
			t.Errorf("unit-less number was altered: %q", got)
		}
	})
}

// checkFixProgress drives the no-progress gateway. The first fail records
// a baseline and lets the fixer run; a later fail with an identical
// signature halts; a fail whose signature changed advances the baseline
// and continues; an empty payload fails safe (never halts).
func TestCheckFixProgress(t *testing.T) {
	a := actions{}

	progressing := func(ctx *statemachine.Context) (bool, bool) {
		v, ok := ctx.State["fix-loop-progressing"]
		b, _ := v.(bool)
		return b, ok
	}

	t.Run("first fail records baseline and continues", func(t *testing.T) {
		ctx := statemachine.NewContext()
		ctx.Set("verify_failure_output", "shouldReject FAILED\nBUILD FAILED in 3s")
		a.checkFixProgress(ctx)
		if p, ok := progressing(ctx); !ok || !p {
			t.Fatalf("first fail: fix-loop-progressing = (%v, set=%v), want (true, true)", p, ok)
		}
		if ctx.GetString("fix-prev-failure-signature") == "" {
			t.Error("first fail: baseline signature not recorded")
		}
	})

	t.Run("identical failure across passes halts", func(t *testing.T) {
		ctx := statemachine.NewContext()
		// Pass 1.
		ctx.Set("verify_failure_output", "shouldReject FAILED\nBUILD FAILED in 3s")
		a.checkFixProgress(ctx)
		// Pass 2: same failing test, different build duration only.
		ctx.Set("verify_failure_output", "shouldReject FAILED\nBUILD FAILED in 9s")
		a.checkFixProgress(ctx)
		if p, _ := progressing(ctx); p {
			t.Error("identical failure: want fix-loop-progressing == false (halt)")
		}
	})

	t.Run("changed failure advances baseline and continues", func(t *testing.T) {
		ctx := statemachine.NewContext()
		ctx.Set("verify_failure_output", "shouldRejectQuantity100 FAILED")
		a.checkFixProgress(ctx)
		first := ctx.GetString("fix-prev-failure-signature")
		ctx.Set("verify_failure_output", "shouldRejectQuantity200 FAILED")
		a.checkFixProgress(ctx)
		if p, _ := progressing(ctx); !p {
			t.Error("changed failure: want fix-loop-progressing == true (continue)")
		}
		if ctx.GetString("fix-prev-failure-signature") == first {
			t.Error("changed failure: baseline not advanced to the new signature")
		}
	})

	t.Run("empty payload fails safe (never halts)", func(t *testing.T) {
		ctx := statemachine.NewContext()
		// A prior baseline exists, but the current run produced no output.
		ctx.Set("fix-prev-failure-signature", "shouldReject FAILED")
		ctx.Set("verify_failure_output", "")
		a.checkFixProgress(ctx)
		if p, ok := progressing(ctx); !ok || !p {
			t.Errorf("empty payload: want progressing == true (defer to count cap), got (%v, set=%v)", p, ok)
		}
	})
}
