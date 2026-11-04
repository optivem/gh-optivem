package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
)

// fingerprintHexLen is the number of hex characters kept from the SHA-256
// digest. 16 hex characters (64 bits) is ample to detect an edited criterion
// and keeps the state file readable.
const fingerprintHexLen = 16

// Fingerprint returns the criterion fingerprint of a criterion's text: a
// truncated SHA-256 over the text with surrounding whitespace trimmed and
// inner whitespace runs collapsed, so reflowing a line does not count as a
// change but any wording change does.
func Fingerprint(text string) string {
	norm := strings.Join(strings.Fields(text), " ")
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:])[:fingerprintHexLen]
}

// FingerprintRecording is the outcome of RecordFingerprints.
type FingerprintRecording struct {
	// Fingerprints is the new fingerprint map to persist.
	Fingerprints map[string]string
	// Added are Verified criteria that had no fingerprint and now have one.
	Added []string
	// Refreshed are changed criteria whose fingerprint was replaced because
	// they were named in refresh.
	Refreshed []string
	// StillChanged are Verified criteria whose text differs from the stored
	// fingerprint and that were not named in refresh; their fingerprint is
	// left untouched so the change stays visible.
	StillChanged []string
	// Pruned are stored fingerprints whose criterion no longer exists.
	Pruned []string
}

// RecordFingerprints derives the fingerprint map to persist from a report that
// was built WITHOUT fingerprints (so a criterion's Verified state is purely
// test-derived). A Verified criterion with no stored fingerprint gets one (the
// moment it first becomes verified). A Verified criterion whose stored
// fingerprint differs from its current text keeps the old value, unless its id
// is in refresh, which re-records it deliberately. Fingerprints of criteria
// that no longer exist in the spec are dropped.
func RecordFingerprints(rep Report, existing map[string]string, refresh []string) FingerprintRecording {
	out := FingerprintRecording{Fingerprints: map[string]string{}}
	refreshSet := map[string]bool{}
	for _, id := range refresh {
		refreshSet[id] = true
	}
	exists := map[string]bool{}
	for _, uc := range rep.UseCases {
		for _, c := range uc.Criteria {
			exists[c.ID] = true
			stored, has := existing[c.ID]
			if has {
				out.Fingerprints[c.ID] = stored
			}
			if c.State != StateVerified {
				continue
			}
			cur := Fingerprint(c.Text)
			switch {
			case !has:
				out.Fingerprints[c.ID] = cur
				out.Added = append(out.Added, c.ID)
			case stored != cur && refreshSet[c.ID]:
				out.Fingerprints[c.ID] = cur
				out.Refreshed = append(out.Refreshed, c.ID)
			case stored != cur:
				out.StillChanged = append(out.StillChanged, c.ID)
			}
		}
	}
	for id := range existing {
		if !exists[id] {
			out.Pruned = append(out.Pruned, id)
		}
	}
	slices.Sort(out.Added)
	slices.Sort(out.Refreshed)
	slices.Sort(out.StillChanged)
	slices.Sort(out.Pruned)
	return out
}
