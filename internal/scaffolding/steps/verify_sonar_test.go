package steps

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/optivem/gh-optivem/internal/kernel/shell"
)

// stubSonarRun replaces the scanner subprocess seam with a script of outputs
// (the last one repeats) and records the number of attempts. Backoff sleeps
// are recorded, not slept.
func stubSonarRun(t *testing.T, outs []string, failFrom int) (calls *int, sleeps *[]time.Duration) {
	t.Helper()
	n := 0
	var slept []time.Duration
	prevRun := sonarRunFn
	sonarRunFn = func(string, bool, string) (string, error) {
		i := n
		n++
		if i >= len(outs) {
			i = len(outs) - 1
		}
		if n < failFrom {
			return outs[i], errors.New("exit status 1")
		}
		return outs[i], nil
	}
	restoreSleep := shell.SetSleepForTest(func(d time.Duration) { slept = append(slept, d) })
	t.Cleanup(func() { sonarRunFn = prevRun; restoreSleep() })
	return &n, &slept
}

func readSonarFixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/sonar_scan_504.txt")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(b)
}

// The fixture contains the literal frame failIfUnauthorized next to a 504;
// classifying the whole log would hard-fail it as an auth error (Q1 trap).
func TestRunSonarScan_504RetriesThenSucceeds(t *testing.T) {
	fixture := readSonarFixture(t)
	calls, sleeps := stubSonarRun(t, []string{fixture, fixture, "BUILD SUCCESSFUL"}, 3)

	out, err := runSonarScan("dir")
	if err != nil || out != "BUILD SUCCESSFUL" || *calls != 3 {
		t.Fatalf("calls=%d out=%q err=%v, want 3 calls, success", *calls, out, err)
	}
	if len(*sleeps) != 2 || (*sleeps)[0] != 15*time.Second || (*sleeps)[1] != 45*time.Second {
		t.Fatalf("sleeps = %v, want [15s 45s]", *sleeps)
	}
}

func TestRunSonarScan_AuthFailureFailsFirstAttempt(t *testing.T) {
	out401 := "HttpException: Error 401 on https://sonarcloud.io/batch/project : Not authorized\n" +
		"\tat org.sonar.scanner.http.DefaultScannerWsClient.failIfUnauthorized(DefaultScannerWsClient.java:87)\n"
	calls, sleeps := stubSonarRun(t, []string{out401}, 1<<30)

	out, err := runSonarScan("dir")
	if err == nil || *calls != 1 || len(*sleeps) != 0 {
		t.Fatalf("calls=%d sleeps=%v err=%v, want 1 call, no sleep, error", *calls, *sleeps, err)
	}
	if out != out401 {
		t.Fatalf("output must be returned unfiltered, got %q", out)
	}
}

func TestRunSonarScan_ExhaustionReturnsLastErrorAndFullOutput(t *testing.T) {
	fixture := readSonarFixture(t)
	calls, _ := stubSonarRun(t, []string{fixture}, 1<<30)

	out, err := runSonarScan("dir")
	if err == nil || *calls != sonarScanAttempts {
		t.Fatalf("calls=%d err=%v, want %d calls and an error", *calls, err, sonarScanAttempts)
	}
	if !strings.Contains(out, "failIfUnauthorized") {
		t.Fatalf("exhaustion output must keep the full unfiltered scanner log")
	}
}

func TestRunSonarScan_ForceRetryReclaimsJreProvisioning403(t *testing.T) {
	jre403 := "Failed to query JRE metadata: GET https://api.sonarcloud.io/analysis/jres failed with HTTP 403 Forbidden\n"
	calls, _ := stubSonarRun(t, []string{jre403, "BUILD SUCCESSFUL"}, 2)

	out, err := runSonarScan("dir")
	if err != nil || out != "BUILD SUCCESSFUL" || *calls != 2 {
		t.Fatalf("calls=%d out=%q err=%v, want retry then success", *calls, out, err)
	}
}
