// Package spinner shows an animated liveness indicator for long-running
// waits where the duration is unknown. The goal is purely UX: keep the user
// from killing a process that's actually making progress.
//
// Behavior:
//
//	TTY     — animated braille glyph + status text on a single line, redrawn
//	          every ~100ms. Status text is updateable via Update(). Backed by
//	          github.com/briandowns/spinner (handles cursor hide, terminal
//	          quirks, NO_COLOR, Windows fallback).
//	non-TTY — the underlying library auto-disables animation; we still print
//	          the upfront log line so piped/redirected output records the wait.
//
// Nested Starts (a Start while another Spinner is still active) return a
// suppressed Spinner so two animations don't compete for the same line.
package spinner

import (
	"fmt"
	"sync"
	"time"

	libspinner "github.com/briandowns/spinner"

	"github.com/optivem/gh-optivem/internal/kernel/log"
)

const frameInterval = 100 * time.Millisecond

// CharSets[14] is the braille-dot animation: ⠋ ⠙ ⠹ ⠸ ⠼ ⠴ ⠦ ⠧ ⠇ ⠏
const charsetIdx = 14

// Active-spinner gate: only the outermost Spinner animates. Nested Start
// calls (e.g. CheckRateLimit fired from inside pollRunUntilComplete) return
// a suppressed Spinner that just logs the upfront message and is a no-op for
// Update/Stop.
var (
	activeMu    sync.Mutex
	activeCount int
)

// Spinner is a live status indicator. Returned by Start; finalized by Stop.
type Spinner struct {
	inner      *libspinner.Spinner
	started    time.Time
	prefix     string
	status     string
	suppressed bool
	stopped    bool
	stopCh     chan struct{}
	mu         sync.Mutex
}

// Start prints an upfront "this may take several minutes — please don't stop
// the process" line, then begins animating. message is shown as the spinner
// prefix; pass something like "Waiting for workflow run #4821".
func Start(message string) *Spinner {
	activeMu.Lock()
	suppressed := activeCount > 0
	activeCount++
	activeMu.Unlock()

	log.Infof("%s — this may take several minutes, please don't stop the process.", message)

	s := &Spinner{
		started:    time.Now(),
		prefix:     message,
		suppressed: suppressed,
		stopCh:     make(chan struct{}),
	}
	if suppressed {
		return s
	}

	s.inner = libspinner.New(libspinner.CharSets[charsetIdx], frameInterval)
	s.inner.FinalMSG = "" // clear line on Stop
	s.refreshSuffix()
	s.inner.Start()
	go s.tickElapsed()
	return s
}

// tickElapsed refreshes the suffix every second so the elapsed counter
// advances even when no one calls Update.
func (s *Spinner) tickElapsed() {
	t := time.NewTicker(1 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-t.C:
			s.mu.Lock()
			if !s.stopped {
				s.refreshSuffix()
			}
			s.mu.Unlock()
		}
	}
}

// refreshSuffix rebuilds the suffix from current state. Caller holds s.mu
// (or is initializing before any goroutine sees s).
func (s *Spinner) refreshSuffix() {
	if s.status == "" {
		s.inner.Suffix = fmt.Sprintf(" %s · %s", s.prefix, elapsed(s.started))
	} else {
		s.inner.Suffix = fmt.Sprintf(" %s — %s · %s", s.prefix, s.status, elapsed(s.started))
	}
}

// Update changes the status suffix shown after the prefix and elapsed time.
// Safe to call from any goroutine. No-op after Stop or for suppressed spinners.
func (s *Spinner) Update(status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped || s.suppressed {
		return
	}
	s.status = status
	s.refreshSuffix()
}

// Stop halts animation and clears the spinner line. Idempotent.
func (s *Spinner) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	suppressed := s.suppressed
	s.mu.Unlock()

	activeMu.Lock()
	activeCount--
	activeMu.Unlock()

	if suppressed {
		return
	}
	close(s.stopCh)
	s.inner.Stop()
}

func elapsed(start time.Time) string {
	d := time.Since(start).Round(time.Second)
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	return fmt.Sprintf("%d:%02d elapsed", mins, secs)
}
