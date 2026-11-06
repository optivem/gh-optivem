package steps

import (
	"strings"
	"sync"

	"github.com/optivem/gh-optivem/internal/kernel/log"
	"github.com/optivem/gh-optivem/internal/kernel/shell"
)

// CloudVars are the repo-level Actions variables the scaffolded cloud stages
// (acceptance-stage.yml, qa-stage.yml, prod-stage.yml) need to deploy to
// Google Cloud Run. `init` does not set them; the user's setup-gcp.sh run does
// (see the scaffolded README's "Cloud setup" section).
var CloudVars = []string{
	"GCP_PROJECT_ID",
	"GCP_REGION",
	"GCP_WORKLOAD_IDENTITY_PROVIDER",
	"GCP_SERVICE_ACCOUNT",
	"SYSTEM_NAME",
}

// MissingCloudVars returns the CloudVars absent from present, in CloudVars
// order. An empty result means the repo is configured for the cloud stages.
func MissingCloudVars(present []string) []string {
	have := make(map[string]bool, len(present))
	for _, p := range present {
		have[p] = true
	}
	var missing []string
	for _, v := range CloudVars {
		if !have[v] {
			missing = append(missing, v)
		}
	}
	return missing
}

// CloudGate decides, once per init run, whether the cloud-deploying stages
// (QA, QA signoff, production) can be verified. A fresh scaffold has no GCP
// configuration, so verification stops after the Docker acceptance gates; a
// repo that already ran setup-gcp.sh keeps the full QA/prod verification.
type CloudGate struct {
	gh       *shell.GitHub
	once     sync.Once
	ready    bool
	listVars func() ([]string, error)
}

// NewCloudGate returns a gate that inspects gh's repo variables on first use.
func NewCloudGate(gh *shell.GitHub) *CloudGate {
	return &CloudGate{gh: gh, listVars: gh.VariableNames}
}

// Ready reports whether the cloud stages should be verified. The first call
// lists the repo variables and, when any CloudVar is missing, logs why the
// cloud stages are skipped. A failure to list the variables fails the step:
// guessing either way would hide a real problem. Later calls reuse the result.
func (c *CloudGate) Ready() bool {
	c.once.Do(func() {
		present, err := c.listVars()
		if err != nil {
			log.Fatalf("Cannot decide whether to verify the cloud stages: %v", err)
		}
		missing := MissingCloudVars(present)
		c.ready = len(missing) == 0
		if c.ready {
			log.Info("GCP configuration found: verifying the QA and production stages.")
			return
		}
		log.Warn("Skipping QA stage, QA signoff, production stage and cloud acceptance (acceptance-stage.yml): they deploy to Google Cloud Run and need cloud setup, which init does not do.")
		log.Warnf("  Missing repo variables on %s: %s", c.gh.Repo, strings.Join(missing, " "))
		log.Warnf("  See the README \"Cloud setup\" section: https://github.com/%s#cloud-setup", c.gh.Repo)
		log.Warnf("  After setup-gcp.sh has run, dispatch acceptance-stage.yml, qa-stage.yml and prod-stage.yml yourself.")
	})
	return c.ready
}
