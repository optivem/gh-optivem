package runner

import (
	"fmt"
	"net/http"
	"time"
)

// Default polling parameters for health checks. Mirrors the PS1 runner's
// behavior (30 attempts × 1s with a 2s per-request timeout).
const (
	defaultHealthAttempts = 30
	defaultHealthInterval = 1 * time.Second
	defaultHealthTimeout  = 2 * time.Second
)

// HealthOptions tunes the polling loop. Zero-values fall back to defaults.
type HealthOptions struct {
	Attempts int           // max attempts per URL
	Interval time.Duration // sleep between attempts
	Timeout  time.Duration // per-request timeout
}

func (o HealthOptions) attempts() int {
	if o.Attempts <= 0 {
		return defaultHealthAttempts
	}
	return o.Attempts
}

func (o HealthOptions) interval() time.Duration {
	if o.Interval <= 0 {
		return defaultHealthInterval
	}
	return o.Interval
}

func (o HealthOptions) timeout() time.Duration {
	if o.Timeout <= 0 {
		return defaultHealthTimeout
	}
	return o.Timeout
}

// WaitForURL polls url until it returns 200 OK or attempts is exhausted.
// Returns nil on success; an error including the URL on timeout.
func WaitForURL(url string, opts HealthOptions) error {
	if url == "" {
		return nil
	}
	client := &http.Client{Timeout: opts.timeout()}
	for i := 0; i < opts.attempts(); i++ {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(opts.interval())
	}
	return fmt.Errorf("service at %s not ready after %d attempts", url, opts.attempts())
}

// WaitForSystem polls every component+externalSystem URL in sys until each
// returns 200 OK. Components without a URL are skipped. Returns the first
// failure (with the failed URL) or nil on success.
func WaitForSystem(sys SystemEntry, opts HealthOptions) error {
	for _, ext := range sys.ExternalSystems {
		if err := WaitForURL(ext.URL, opts); err != nil {
			return fmt.Errorf("%s: %w", ext.Name, err)
		}
	}
	for _, comp := range sys.Components {
		if comp.URL == "" {
			continue
		}
		if err := WaitForURL(comp.URL, opts); err != nil {
			return fmt.Errorf("%s: %w", comp.Name, err)
		}
	}
	return nil
}

// IsAnyURLUp returns true if any of the URLs in sys responds with 200 OK
// inside a single short attempt. Used as a fast "is the system already
// running?" probe so we can skip a docker-compose restart when not needed.
func IsAnyURLUp(sys SystemEntry, opts HealthOptions) bool {
	client := &http.Client{Timeout: opts.timeout()}
	check := func(url string) bool {
		if url == "" {
			return false
		}
		resp, err := client.Get(url)
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}
	for _, ext := range sys.ExternalSystems {
		if check(ext.URL) {
			return true
		}
	}
	for _, comp := range sys.Components {
		if check(comp.URL) {
			return true
		}
	}
	return false
}
