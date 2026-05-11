package projectconfig

import "testing"

// TestDeriveSonarProjects pins the canonical SonarCloud project keys for
// every architecture × repo_strategy quadrant. Mirrors the legacy
// TestSonarProjectKeys in internal/config/config_test.go (which exercised
// steps.GetSonarProjectKeys) and extends it to assert the always-present
// SystemTest key the scaffold-time replacement now creates.
func TestDeriveSonarProjects(t *testing.T) {
	tests := []struct {
		name         string
		owner        string
		repo         string
		arch         string
		repoStrategy string
		want         DerivedSonar
	}{
		{
			name: "monolith monorepo",
			owner: "acme", repo: "page-turner",
			arch: ArchMonolith, repoStrategy: RepoStrategyMonoRepo,
			want: DerivedSonar{
				System:     "acme_page-turner-system",
				SystemTest: "acme_page-turner-system-test",
			},
		},
		{
			name: "monolith multirepo",
			owner: "acme", repo: "page-turner",
			arch: ArchMonolith, repoStrategy: RepoStrategyMultiRepo,
			want: DerivedSonar{
				System:     "acme_page-turner-system",
				SystemTest: "acme_page-turner-system-test",
			},
		},
		{
			name: "multitier monorepo",
			owner: "acme", repo: "page-turner",
			arch: ArchMultitier, repoStrategy: RepoStrategyMonoRepo,
			want: DerivedSonar{
				Backend:    "acme_page-turner-backend",
				Frontend:   "acme_page-turner-frontend",
				SystemTest: "acme_page-turner-system-test",
			},
		},
		{
			name: "multitier multirepo",
			owner: "acme", repo: "page-turner",
			arch: ArchMultitier, repoStrategy: RepoStrategyMultiRepo,
			want: DerivedSonar{
				Backend:    "acme_page-turner-backend",
				Frontend:   "acme_page-turner-frontend",
				SystemTest: "acme_page-turner-system-test",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveSonarProjects(tt.owner, tt.repo, tt.arch, tt.repoStrategy)
			if got != tt.want {
				t.Errorf("DeriveSonarProjects = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestDeriveSonarProjectsSystemTestUniformAcrossQuadrants pins the
// language-agnostic, strategy-agnostic SystemTest key — the property the
// scaffold-time -tests-<lang> → -system-test replacement makes safe to
// rely on.
func TestDeriveSonarProjectsSystemTestUniformAcrossQuadrants(t *testing.T) {
	const owner = "acme"
	const repo = "page-turner"
	const want = "acme_page-turner-system-test"
	for _, arch := range []string{ArchMonolith, ArchMultitier} {
		for _, strategy := range []string{RepoStrategyMonoRepo, RepoStrategyMultiRepo} {
			got := DeriveSonarProjects(owner, repo, arch, strategy).SystemTest
			if got != want {
				t.Errorf("arch=%s strategy=%s: SystemTest = %q, want %q",
					arch, strategy, got, want)
			}
		}
	}
}
