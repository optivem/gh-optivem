package tracker_test

import (
	"context"
	"errors"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/tracker"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// fakeTracker is a no-op implementation that the compiler validates
// against the Tracker interface. Updates to the interface that break
// implementers will fail this file's build.
type fakeTracker struct{}

func (fakeTracker) PickReady(context.Context) (tracker.Issue, error) {
	return tracker.Issue{}, nil
}
func (fakeTracker) FindIssue(context.Context, string) (tracker.Issue, error) {
	return tracker.Issue{}, nil
}
func (fakeTracker) SetStatus(context.Context, string, string) error { return nil }
func (fakeTracker) Verify(context.Context) error                    { return nil }
func (fakeTracker) Classify(context.Context, tracker.Issue) (string, bool, error) {
	return "", false, nil
}
func (fakeTracker) ReadSections(context.Context, tracker.Issue, []string) (map[string]string, error) {
	return nil, nil
}
func (fakeTracker) MarkChecklistComplete(context.Context, tracker.Issue) error { return nil }

// TestTrackerInterfaceContract is a compile-time assertion that
// fakeTracker satisfies tracker.Tracker. The runtime body never
// exercises the value; the assignment alone does the check.
func TestTrackerInterfaceContract(t *testing.T) {
	var _ tracker.Tracker = fakeTracker{}
}

func TestOpen_StubReturnsErrNotImplemented(t *testing.T) {
	_, err := tracker.Open(context.Background(), projectconfig.Project{
		URL: "https://github.com/orgs/optivem/projects/20",
	})
	if !errors.Is(err, tracker.ErrNotImplemented) {
		t.Errorf("expected tracker.ErrNotImplemented, got %v", err)
	}
}
