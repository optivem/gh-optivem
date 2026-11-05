package spec

import (
	"slices"
	"testing"
)

func TestFingerprintIgnoresWhitespaceOnly(t *testing.T) {
	a := Fingerprint("The customer places an order")
	if got := Fingerprint("  The  customer\tplaces an\norder "); got != a {
		t.Fatalf("whitespace reflow changed the fingerprint: %s vs %s", got, a)
	}
	if Fingerprint("The customer places two orders") == a {
		t.Fatal("wording change must change the fingerprint")
	}
	if len(a) != fingerprintHexLen {
		t.Fatalf("fingerprint length = %d", len(a))
	}
}

func fingerprintFixture(t *testing.T) (uc []UseCase, passing []Test) {
	t.Helper()
	uc = loadSpec(t)
	return uc, readFixture(t, "junit-all-pass.xml")
}

func TestReportMarksChangedCriterion(t *testing.T) {
	ucs, tests := fingerprintFixture(t)
	pure := BuildReport(ucs, tests, Config{}, Options{Now: testNow})
	if states(pure)["UC-001.1"] != StateVerified {
		t.Fatalf("precondition: %v", states(pure))
	}
	b := NewBaseline(pure)
	if _, ok := b.Fingerprints["UC-001.1"]; !ok || len(b.Fingerprints) != pure.Summary.Verified {
		t.Fatalf("adopt must fingerprint exactly the verified criteria: %v", b.Fingerprints)
	}

	// Same text: still Verified.
	rep := BuildReport(ucs, tests, Config{}, Options{Now: testNow, Fingerprints: b.Fingerprints})
	if states(rep)["UC-001.1"] != StateVerified {
		t.Fatalf("unchanged criterion must stay Verified: %v", states(rep))
	}

	// Edited text: a passing old test no longer verifies it.
	edited := slices.Clone(ucs)
	edited[0].Criteria = slices.Clone(ucs[0].Criteria)
	edited[0].Criteria[0].Text += " within five minutes"
	rep = BuildReport(edited, tests, Config{}, Options{Now: testNow, Fingerprints: b.Fingerprints})
	if got := states(rep)["UC-001.1"]; got != StateChanged {
		t.Fatalf("edited criterion = %s, want Changed", got)
	}
	if rep.Summary.Changed != 1 || rep.Summary.Verified != pure.Summary.Verified-1 {
		t.Fatalf("summary: %+v", rep.Summary)
	}
	// Strict blocks a Changed criterion; lenient does not.
	rep.Mode = ModeStrict
	rep.Threshold = 0
	_ = ApplyGate(&rep, nil)
	if rep.Gate.Passed {
		t.Fatal("strict must block a Changed criterion")
	}
	rep.Mode = ModeLenient
	_ = ApplyGate(&rep, nil)
	if !rep.Gate.Passed {
		t.Fatalf("lenient must not block Changed: %v", rep.Gate.Reasons)
	}
}

func TestChangedDoesNotMaskFailingOrNewCriteria(t *testing.T) {
	ucs, _ := fingerprintFixture(t)
	red := readFixture(t, "junit-one-red.xml")
	fps := map[string]string{}
	for _, c := range ucs[0].Criteria {
		fps[c.ID] = "stale"
	}
	rep := BuildReport(ucs, red, Config{}, Options{Now: testNow, Fingerprints: fps})
	for id, st := range states(rep) {
		if st == StateVerified {
			t.Fatalf("%s Verified despite a stale fingerprint", id)
		}
	}
	if rep.Summary.Failing == 0 {
		t.Fatal("a failing criterion must stay Failing, not Changed")
	}
	// A criterion with no recorded fingerprint is never Changed.
	rep = BuildReport(ucs, readFixture(t, "junit-all-pass.xml"), Config{}, Options{Now: testNow, Fingerprints: map[string]string{}})
	if rep.Summary.Changed != 0 {
		t.Fatalf("no recorded fingerprint must not be Changed: %+v", rep.Summary)
	}
}

func TestRecordFingerprints(t *testing.T) {
	ucs, tests := fingerprintFixture(t)
	pure := BuildReport(ucs, tests, Config{}, Options{Now: testNow})

	rec := RecordFingerprints(pure, nil, nil, nil)
	if len(rec.Added) != pure.Summary.Verified || len(rec.Fingerprints) != pure.Summary.Verified {
		t.Fatalf("first record: %+v", rec)
	}

	// A changed criterion keeps its old fingerprint unless refreshed.
	edited := slices.Clone(ucs)
	edited[0].Criteria = slices.Clone(ucs[0].Criteria)
	edited[0].Criteria[0].Text += " within five minutes"
	pureEdited := BuildReport(edited, tests, Config{}, Options{Now: testNow})
	old := rec.Fingerprints["UC-001.1"]
	kept := RecordFingerprints(pureEdited, rec.Fingerprints, nil, nil)
	if kept.Fingerprints["UC-001.1"] != old || !slices.Equal(kept.StillChanged, []string{"UC-001.1"}) {
		t.Fatalf("changed criterion must keep its fingerprint: %+v", kept)
	}
	refreshed := RecordFingerprints(pureEdited, rec.Fingerprints, []string{"UC-001.1"}, nil)
	if refreshed.Fingerprints["UC-001.1"] == old || !slices.Equal(refreshed.Refreshed, []string{"UC-001.1"}) {
		t.Fatalf("--refresh must re-record: %+v", refreshed)
	}

	// Fingerprints of retired criteria are pruned.
	withStale := map[string]string{"UC-099.1": "x"}
	for k, v := range rec.Fingerprints {
		withStale[k] = v
	}
	if got := RecordFingerprints(pure, withStale, nil, nil); !slices.Equal(got.Pruned, []string{"UC-099.1"}) {
		t.Fatalf("pruned = %v", got.Pruned)
	}

	// A scope limits adding to the named criteria and reports named ones that
	// are not Verified.
	var verifiedID, unverifiedID string
	for _, uc := range pure.UseCases {
		for _, c := range uc.Criteria {
			if c.State == StateVerified && verifiedID == "" {
				verifiedID = c.ID
			}
			if c.State != StateVerified && unverifiedID == "" {
				unverifiedID = c.ID
			}
		}
	}
	if verifiedID == "" || unverifiedID == "" {
		t.Fatalf("fixture needs a verified and an unverified criterion")
	}
	scoped := RecordFingerprints(pure, nil, nil, []string{verifiedID, unverifiedID})
	if !slices.Equal(scoped.Added, []string{verifiedID}) || len(scoped.Fingerprints) != 1 || !slices.Equal(scoped.NotVerified, []string{unverifiedID}) {
		t.Fatalf("scoped record: %+v", scoped)
	}
}
