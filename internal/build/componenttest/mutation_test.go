package componenttest

import (
	"strings"
	"testing"
)

const pitSample = `<?xml version="1.0" encoding="UTF-8"?>
<mutations>
  <mutation detected='true' status='KILLED'><mutatedClass>com.x.Order</mutatedClass><lineNumber>10</lineNumber><mutator>org.pitest.mutationtest.engine.gregor.mutators.MathMutator</mutator><description>Replaced + with -</description></mutation>
  <mutation detected='true' status='TIMED_OUT'><mutatedClass>com.x.Order</mutatedClass><lineNumber>11</lineNumber><mutator>org.pitest.mutationtest.engine.gregor.mutators.NegateConditionalsMutator</mutator><description>negated conditional</description></mutation>
  <mutation detected='false' status='SURVIVED'><mutatedClass>com.x.Order</mutatedClass><lineNumber>20</lineNumber><mutator>org.pitest.mutationtest.engine.gregor.mutators.returns.BooleanTrueReturnValsMutator</mutator><description>replaced return with true</description></mutation>
  <mutation detected='false' status='NO_COVERAGE'><mutatedClass>com.x.Cart</mutatedClass><lineNumber>5</lineNumber><mutator>org.pitest.mutationtest.engine.gregor.mutators.VoidMethodCallMutator</mutator><description>removed call</description></mutation>
  <mutation detected='false' status='NON_VIABLE'><mutatedClass>com.x.Cart</mutatedClass><lineNumber>6</lineNumber><mutator>m.Other</mutator><description>did not compile</description></mutation>
</mutations>`

func TestScorePitReport(t *testing.T) {
	s, err := ScorePitReport([]byte(pitSample))
	if err != nil {
		t.Fatal(err)
	}
	if s.Total != 4 || s.Detected != 2 {
		t.Fatalf("total/detected = %d/%d, want 4/2 (non-viable excluded)", s.Total, s.Detected)
	}
	if s.Percent() != 50 {
		t.Errorf("percent = %d, want 50", s.Percent())
	}
	if len(s.Survivors) != 2 || s.Survivors[0].Class != "com.x.Cart" || s.Survivors[1].Mutator != "BooleanTrueReturnValsMutator" {
		t.Errorf("survivors not sorted/shortened as expected: %+v", s.Survivors)
	}
}

func TestScorePitReport_EmptyIs100(t *testing.T) {
	s, err := ScorePitReport([]byte(`<mutations></mutations>`))
	if err != nil || s.Percent() != 100 {
		t.Fatalf("got %d, %v; want 100, nil", s.Percent(), err)
	}
}

func TestScorePitReport_Malformed(t *testing.T) {
	if _, err := ScorePitReport([]byte(`<mutations><mutation`)); err == nil {
		t.Fatal("malformed report must error")
	}
}

func TestJudge(t *testing.T) {
	s, _ := ScorePitReport([]byte(pitSample))
	var out strings.Builder
	if err := s.Judge(80, &out); err == nil || !strings.Contains(err.Error(), "below") {
		t.Errorf("score 50 vs threshold 80: want below-threshold error, got %v", err)
	}
	if !strings.Contains(out.String(), "com.x.Order:20") || !strings.Contains(out.String(), "NO_COVERAGE") {
		t.Errorf("survivor list missing from output:\n%s", out.String())
	}
	out.Reset()
	if err := s.Judge(50, &out); err != nil {
		t.Errorf("score 50 vs threshold 50: want pass, got %v", err)
	}
	out.Reset()
	if err := s.Judge(0, &out); err != nil || !strings.Contains(out.String(), "report only") {
		t.Errorf("unset threshold: want report-only pass, got %v / %s", err, out.String())
	}
}

func TestJudge_CapsSurvivorList(t *testing.T) {
	s := MutationScore{Total: 100, Detected: 0}
	for i := 0; i < maxSurvivorsShown+5; i++ {
		s.Survivors = append(s.Survivors, Survivor{Class: "c.C", Line: i, Mutator: "M", Status: "SURVIVED"})
	}
	var out strings.Builder
	_ = s.Judge(10, &out)
	if !strings.Contains(out.String(), "and 5 more") {
		t.Errorf("expected capped list with remainder note:\n%s", out.String())
	}
}

func TestJavaClassFromPath(t *testing.T) {
	prefix := "system/multitier/backend-java/src/main/java/"
	cases := []struct {
		path string
		want string
		ok   bool
	}{
		{prefix + "com/x/Order.java", "com.x.Order", true},
		{prefix + "com/x/package-info.java", "", false},
		{"system/multitier/backend-java/src/test/java/com/x/OrderTest.java", "", false},
		{prefix + "com/x/application.yml", "", false},
	}
	for _, tc := range cases {
		got, ok := javaClassFromPath(tc.path, prefix)
		if got != tc.want || ok != tc.ok {
			t.Errorf("%s: got (%q,%v), want (%q,%v)", tc.path, got, ok, tc.want, tc.ok)
		}
	}
}
