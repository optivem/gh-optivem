package kickoff

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"
)

// removeSampleUseCases deletes the shop sample's use case narrative. Its id
// (UC-001) would collide with the user's first use case, so this runs before
// the narratives are written, not with the rest of the shop removal.
func removeSampleUseCases(l Layout) error {
	err := os.Remove(l.Abs(sampleUseCaseFile))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove sample use case %s: %w", sampleUseCaseFile, err)
	}
	return nil
}

// ErrRemovalUndecided is returned by DefaultRemove until the shop removal
// inventory is settled (see the kickoff plan, Step 5).
var ErrRemovalUndecided = errors.New("shop removal is not implemented yet: what the repo keeps after removal (an empty test architecture and a buildable system skeleton) is still an open design decision")

// DefaultRemove is the production RemoveFunc.
func DefaultRemove(Layout) error { return ErrRemovalUndecided }

// sampleRemoved reports whether the shop sample is gone. Until shop removal
// exists nothing can have been removed by kickoff, so the answer is false.
func sampleRemoved(Layout) (bool, error) { return false, nil }

// validateNarratives is replaced in tests.
var validateNarratives = func(l Layout) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate gh-optivem binary: %w", err)
	}
	cmd := exec.Command(exe, "spec", "validate")
	cmd.Dir = l.Root
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("`gh optivem spec validate` failed on the generated use cases; re-run `gh optivem kickoff --feedback \"...\"` or fix them by hand:\n%s", strings.TrimSpace(out.String()))
	}
	return nil
}

// ValidateNarratives runs `gh optivem spec validate` over the use cases.
func ValidateNarratives(l Layout) error { return validateNarratives(l) }
