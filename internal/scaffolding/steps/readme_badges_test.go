package steps

import (
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/config"
)

func TestWriteBadges_EventFilter(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	writeBadges(&b, [][3]string{
		{"u/a.yml", "a", eventPush},
		{"u/b.yml", "b", ""},
	})
	want := "[![a](u/a.yml/badge.svg?event=push)](u/a.yml)\n[![b](u/b.yml/badge.svg)](u/b.yml)\n"
	if b.String() != want {
		t.Errorf("got %q, want %q", b.String(), want)
	}
}

func TestPipelineBadges_ScheduleOnly(t *testing.T) {
	t.Parallel()
	got := pipelineBadges("base", &config.Config{})
	if len(got) != 2 {
		t.Fatalf("got %d badges, want 2: %v", len(got), got)
	}
	for _, bd := range got {
		if bd[2] != eventSchedule {
			t.Errorf("badge %s event = %q, want %q", bd[1], bd[2], eventSchedule)
		}
	}
}
