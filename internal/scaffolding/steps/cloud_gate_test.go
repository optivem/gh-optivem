package steps

import (
	"errors"
	"reflect"
	"testing"

	"github.com/optivem/gh-optivem/internal/kernel/log"
	"github.com/optivem/gh-optivem/internal/kernel/shell"
)

func TestMissingCloudVars(t *testing.T) {
	tests := []struct {
		name    string
		present []string
		want    []string
	}{
		{"fresh scaffold (init sets only DOCKERHUB_USERNAME)", []string{"DOCKERHUB_USERNAME"}, CloudVars},
		{"no variables", nil, CloudVars},
		{"all set", append([]string{"DOCKERHUB_USERNAME"}, CloudVars...), nil},
		{"partially set", []string{"GCP_PROJECT_ID", "GCP_REGION", "SYSTEM_NAME"},
			[]string{"GCP_WORKLOAD_IDENTITY_PROVIDER", "GCP_SERVICE_ACCOUNT"}},
		{"names are case-sensitive", []string{"gcp_project_id", "GCP_REGION", "GCP_WORKLOAD_IDENTITY_PROVIDER", "GCP_SERVICE_ACCOUNT", "SYSTEM_NAME"},
			[]string{"GCP_PROJECT_ID"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MissingCloudVars(tt.present); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MissingCloudVars(%v) = %v, want %v", tt.present, got, tt.want)
			}
		})
	}
}

func newTestGate(list func() ([]string, error)) *CloudGate {
	g := NewCloudGate(shell.NewGitHub("owner/repo"))
	g.listVars = list
	return g
}

func TestCloudGate_NotReadyWithoutGCPVars(t *testing.T) {
	g := newTestGate(func() ([]string, error) { return []string{"DOCKERHUB_USERNAME"}, nil })
	if g.Ready() {
		t.Fatal("Ready() = true for a repo without GCP variables, want false (stop after Docker gates)")
	}
}

func TestCloudGate_ReadyWithAllGCPVars(t *testing.T) {
	g := newTestGate(func() ([]string, error) { return append([]string{"DOCKERHUB_USERNAME"}, CloudVars...), nil })
	if !g.Ready() {
		t.Fatal("Ready() = false for a repo with every GCP variable, want true (verify QA/prod)")
	}
}

func TestCloudGate_ListsVariablesOnce(t *testing.T) {
	calls := 0
	g := newTestGate(func() ([]string, error) { calls++; return nil, nil })
	g.Ready()
	g.Ready()
	if calls != 1 {
		t.Errorf("variables listed %d times, want 1 (QA and prod share one decision)", calls)
	}
}

func TestCloudGate_ListFailureFailsTheStep(t *testing.T) {
	g := newTestGate(func() ([]string, error) { return nil, errors.New("HTTP 403") })
	defer func() {
		r := recover()
		if _, ok := r.(*log.StepError); !ok {
			t.Fatalf("recover() = %v, want *log.StepError (a list failure must not silently skip or run the cloud stages)", r)
		}
	}()
	g.Ready()
}
