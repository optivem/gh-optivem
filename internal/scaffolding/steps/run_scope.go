package steps

import (
	"sync"

	"github.com/optivem/gh-optivem/internal/kernel/log"
	"github.com/optivem/gh-optivem/internal/kernel/shell"
)

// Repositories that existed before init already have workflow runs, so the
// verify stages cannot treat "the newest run of workflow X" as the run this
// scaffold produced. For each pre-existing repository, the push step records
// the newest run ID before pushing (everything at or below it is stale) and
// whether the push was a no-op (then no push-triggered run exists at all).
var (
	runScopeMu     sync.Mutex
	runBaselines   = map[string]int64{}
	unchangedRepos = map[string]bool{}
)

// latestRunIDFn lists the newest run ID of a repository. Tests replace it.
var latestRunIDFn = func(fullRepo string) (int64, error) {
	return shell.NewGitHub(fullRepo).LatestRunID()
}

// recordRunBaseline captures the run history watermark of a pre-existing
// repository. It must run before the push. An unreadable history aborts: a
// guess here would let a stale run be reported as this scaffold's verdict.
func recordRunBaseline(fullRepo string) error {
	id, err := latestRunIDFn(fullRepo)
	if err != nil {
		return err
	}
	runScopeMu.Lock()
	defer runScopeMu.Unlock()
	runBaselines[fullRepo] = id
	return nil
}

func recordUnchanged(fullRepo string) {
	runScopeMu.Lock()
	defer runScopeMu.Unlock()
	unchangedRepos[fullRepo] = true
}

func isUnchanged(fullRepo string) bool {
	runScopeMu.Lock()
	defer runScopeMu.Unlock()
	return unchangedRepos[fullRepo]
}

// scopeToThisRun returns gh restricted to runs newer than the baseline recorded
// for its repository. Repositories created by this run have no baseline and are
// returned unchanged.
func scopeToThisRun(gh *shell.GitHub) *shell.GitHub {
	runScopeMu.Lock()
	id, ok := runBaselines[gh.Repo]
	runScopeMu.Unlock()
	if !ok || id <= 0 {
		return gh
	}
	return gh.WithMinRunID(id)
}

// skipUnchangedPushStage reports whether a push-triggered stage must be skipped
// because the push to fullRepo was a no-op: nothing was pushed, so no run was
// triggered, and the newest run on the branch is not evidence about this
// scaffold. The skip is announced, never silent. Stages started by an explicit
// dispatch are unaffected: they run against the default branch as usual.
func skipUnchangedPushStage(fullRepo, label string) bool {
	if !isUnchanged(fullRepo) {
		return false
	}
	log.Warnf("Skipping %s verification for %s: the scaffold matched what was already there, so nothing was pushed and no run was triggered. Dispatched stages still run against the default branch.", label, fullRepo)
	return true
}
