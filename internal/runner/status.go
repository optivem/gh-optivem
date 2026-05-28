package runner

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// defaultStatusTimeout is the per-URL probe ceiling when StatusOptions.Timeout
// is zero. Picked low (2s) because `status` is a snapshot — operators expect
// it to return quickly even when several components are down.
const defaultStatusTimeout = 2 * time.Second

// StatusOptions tunes Status. Zero-values use a 2s per-URL timeout.
type StatusOptions struct {
	Timeout time.Duration
}

func (o StatusOptions) timeout() time.Duration {
	if o.Timeout <= 0 {
		return defaultStatusTimeout
	}
	return o.Timeout
}

// Status writes a per-component verdict line for every system in sys to w.
// Each line is "  <symbol> <name>: <url>" where symbol is "OK" (200 OK within
// Timeout) or "DOWN" (anything else, including connection refused, non-200,
// or timeout). Empty URLs are skipped. Returns the number of DOWN components
// so the caller can choose an exit code. No retries — status is a snapshot,
// not a wait. ASCII symbols (not ✓/✗) so Windows consoles don't mangle output.
func Status(w io.Writer, sys *SystemConfig, opts StatusOptions) int {
	client := &http.Client{Timeout: opts.timeout()}
	down := 0
	probe := func(name, url string) {
		ok := probeOK(client, url)
		symbol := "OK"
		if !ok {
			symbol = "DOWN"
			down++
		}
		fmt.Fprintf(w, "  %s %s: %s\n", symbol, name, url)
	}
	for _, s := range sys.Systems {
		for _, c := range s.Components {
			if c.URL == "" {
				continue
			}
			probe(c.Name, c.URL)
		}
		for _, e := range s.ExternalSystems {
			if e.URL == "" {
				continue
			}
			probe(e.Name, e.URL)
		}
	}
	return down
}

// probeOK returns true iff a single GET against url returns 200 OK within the
// client's configured timeout.
func probeOK(client *http.Client, url string) bool {
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
