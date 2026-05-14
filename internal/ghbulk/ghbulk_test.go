package ghbulk

import (
	"testing"
	"time"
)

func TestParseSlug(t *testing.T) {
	tests := []struct {
		in      string
		wantOwn string
		wantRep string
		wantErr bool
	}{
		{"optivem/shop", "optivem", "shop", false},
		{"valentinajemuovic/greeter-java", "valentinajemuovic", "greeter-java", false},
		{"foo", "", "", true},
		{"foo/bar/baz", "", "", true},
		{"/bar", "", "", true},
		{"foo/", "", "", true},
		{"", "", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			owner, repo, err := ParseSlug(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParseSlug(%q) err=%v wantErr=%v", tc.in, err, tc.wantErr)
			}
			if owner != tc.wantOwn || repo != tc.wantRep {
				t.Fatalf("ParseSlug(%q) = (%q,%q), want (%q,%q)",
					tc.in, owner, repo, tc.wantOwn, tc.wantRep)
			}
		})
	}
}

func TestOptionsDefaults(t *testing.T) {
	var zero Options
	if got := zero.pageSize(); got != defaultPageSize {
		t.Errorf("zero pageSize() = %d, want %d", got, defaultPageSize)
	}
	if got := zero.delay(); got != defaultDelayBetweenDeletes {
		t.Errorf("zero delay() = %v, want %v", got, defaultDelayBetweenDeletes)
	}

	custom := Options{PageSize: 25, DelayBetweenDeletes: 2 * time.Second}
	if got := custom.pageSize(); got != 25 {
		t.Errorf("custom pageSize() = %d, want 25", got)
	}
	if got := custom.delay(); got != 2*time.Second {
		t.Errorf("custom delay() = %v, want 2s", got)
	}
}

func TestPassesBeforeDate(t *testing.T) {
	cutoff := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	o := Options{BeforeDate: cutoff}

	older := time.Date(2025, 12, 31, 23, 59, 0, 0, time.UTC)
	if !o.passesBeforeDate(older) {
		t.Error("expected older time to pass before-date filter")
	}
	if o.passesBeforeDate(cutoff) {
		t.Error("expected cutoff itself to fail (filter is exclusive)")
	}
	newer := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	if o.passesBeforeDate(newer) {
		t.Error("expected newer time to fail before-date filter")
	}

	// Zero BeforeDate ⇒ no filter, everything passes.
	none := Options{}
	if !none.passesBeforeDate(older) || !none.passesBeforeDate(newer) {
		t.Error("zero BeforeDate must accept all times")
	}
}
