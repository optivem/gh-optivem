package tracker_test

import (
	"context"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/tracker"
)

// fakeTracker is a no-op implementation that the compiler validates
// against the Tracker interface. Updates to the interface that break
// implementers will fail this file's build.
type fakeTracker struct{}

func (fakeTracker) FindIssue(context.Context, string) (tracker.Issue, error) {
	return tracker.Issue{}, nil
}
func (fakeTracker) SetStatus(context.Context, string, string) error { return nil }
func (fakeTracker) Verify(context.Context) error                    { return nil }
func (fakeTracker) Classify(context.Context, tracker.Issue) (string, bool, error) {
	return "", false, nil
}
func (fakeTracker) Subtypes(context.Context, tracker.Issue) ([]string, error) {
	return nil, nil
}
func (fakeTracker) ReadBody(context.Context, tracker.Issue) (string, error) {
	return "", nil
}

// TestTrackerInterfaceContract is a compile-time assertion that
// fakeTracker satisfies tracker.Tracker. The runtime body never
// exercises the value; the assignment alone does the check.
func TestTrackerInterfaceContract(t *testing.T) {
	var _ tracker.Tracker = fakeTracker{}
}
