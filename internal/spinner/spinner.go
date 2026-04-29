// Package spinner shows an animated liveness indicator for long-running
// waits where the duration is unknown. The goal is purely UX: keep the user
// from killing a process that's actually making progress.
//
// Behavior:
//
//	TTY     — animated braille glyph + status text on a single line, redrawn
//	          every ~100ms via \r. Status text is updateable via Update().
//	non-TTY — falls back to a single log line on Start and periodic log lines
//	          (every 30s by default) so piped/redirected output stays clean.
//	--quiet — same as non-TTY: no animation, periodic log lines suppressed.
//
// Always safe to Stop() — idempotent. If Start was a no-op (non-TTY), Stop
// just emits a final log line so the user sees the wait ended.
package spinner

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/optivem/gh-optivem/internal/log"
)

const (
	frameInterval = 100 * time.Millisecond
	plainTickGap  = 30 * time.Second
)

var frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Active-spinner gate: only the outermost Spinner animates / ticks. Nested
// Start calls (e.g. CheckRateLimit fired from inside pollRunUntilComplete)
// return a suppressed Spinner that just logs the upfront message and is a
// no-op for Update/Stop. Avoids two animations competing for the same line.
var (
	activeMu    sync.Mutex
	activeCount int
)

// Spinner is a live status indicator. Returned by Start; finalized by Stop.
type Spinner struct {
	mu         sync.Mutex
	status     string
	started    time.Time
	stopCh     chan struct{}
	doneCh     chan struct{}
	tty        bool
	stopped    bool
	prefix     string
	suppressed bool
}

// Start prints an upfront "this may take several minutes — please don't stop
// the process" line, then begins animating. message is shown as the spinner
// prefix; pass something like "Waiting for workflow run #4821".
//
// Nested Starts (a Start while another Spinner is still active) return a
// suppressed Spinner so two animations don't compete for the same line.
func Start(message string) *Spinner {
	activeMu.Lock()
	suppressed := activeCount > 0
	activeCount++
	activeMu.Unlock()

	tty := isTerminal(os.Stdout)
	s := &Spinner{
		status:     "",
		started:    time.Now(),
		stopCh:     make(chan struct{}),
		doneCh:     make(chan struct{}),
		tty:        tty,
		prefix:     message,
		suppressed: suppressed,
	}

	log.Infof("%s — this may take several minutes, please don't stop the process.", message)

	if suppressed {
		close(s.doneCh) // no goroutine started; Stop() must not block.
		return s
	}

	if tty {
		go s.animate()
	} else {
		go s.tick() // periodic log lines for non-TTY
	}
	return s
}

// Update changes the status suffix shown after the prefix and elapsed time.
// Safe to call from any goroutine. No-op after Stop.
func (s *Spinner) Update(status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	s.status = status
}

// Stop halts animation and clears the spinner line (TTY) or prints a final
// completion log line (non-TTY). Idempotent.
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
	<-s.doneCh

	if s.tty {
		// Clear the spinner line so the next log line starts clean.
		fmt.Fprint(os.Stdout, "\r\033[K")
	}
}

func (s *Spinner) animate() {
	defer close(s.doneCh)
	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()
	i := 0
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.mu.Lock()
			line := fmt.Sprintf("\r%s %s · %s", frames[i%len(frames)], s.line(), elapsed(s.started))
			s.mu.Unlock()
			fmt.Fprint(os.Stdout, line, "\033[K") // \033[K clears to end of line
			i++
		}
	}
}

func (s *Spinner) tick() {
	defer close(s.doneCh)
	ticker := time.NewTicker(plainTickGap)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.mu.Lock()
			msg := fmt.Sprintf("%s · %s · %s", s.prefix, s.status, elapsed(s.started))
			s.mu.Unlock()
			log.Infof("%s", msg)
		}
	}
}

// line returns the current display text (prefix + status). Caller holds s.mu.
func (s *Spinner) line() string {
	if s.status == "" {
		return s.prefix
	}
	return s.prefix + " — " + s.status
}

func elapsed(start time.Time) string {
	d := time.Since(start).Round(time.Second)
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	return fmt.Sprintf("%d:%02d elapsed", mins, secs)
}

// isTerminal reports whether f is connected to a terminal. Works on both
// Unix and Windows without an external dependency.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
