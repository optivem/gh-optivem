package steps

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/config"
)

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPipelineWorkflows_SinglePipelineCallers(t *testing.T) {
	t.Parallel()
	got := pipelineWorkflows(Names.MultitierPipelineStageWf, map[string]string{"testLang": "dotnet"})
	want := map[string]string{
		"multitier-dotnet-acceptance-stage.yml":                "acceptance-stage.yml",
		"multitier-dotnet-acceptance-stage-docker-compose.yml": "acceptance-stage-docker-compose.yml",
		"multitier-dotnet-qa-stage.yml":                        "qa-stage.yml",
		"multitier-dotnet-qa-signoff.yml":                      "qa-signoff.yml",
		"multitier-dotnet-prod-stage.yml":                      "prod-stage.yml",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pipelineWorkflows:\n got  %v\n want %v", got, want)
	}
}

// The shared workflows and local actions copied are exactly those the copied
// callers reference, transitively; shop's meta-release machinery never comes along.
func TestCopySharedPipeline_CopiesReferencedClosure(t *testing.T) {
	t.Parallel()
	shop, repo := t.TempDir(), t.TempDir()
	wf := func(root, name, body string) {
		writeTestFile(t, filepath.Join(root, ".github", "workflows", name), body)
	}
	act := func(name, body string) {
		writeTestFile(t, filepath.Join(shop, ".github", "actions", name, "action.yml"), body)
	}
	wf(shop, "_qa-stage.yml", "jobs:\n  deploy:\n    uses: ./.github/workflows/_deploy-stage.yml\n")
	wf(shop, "_deploy-stage.yml", "jobs:\n  k:\n    steps:\n      - uses: ./.github/actions/deploy-keycloak\n")
	wf(shop, "_acceptance-stage-docker-compose.yml", "jobs:\n  s:\n    steps:\n      - name: x\n        uses: ./.github/actions/sonar-scan\n")
	wf(shop, "_prerelease-pipeline.yml", "jobs:\n  l:\n    uses: ./.github/workflows/x.yml\n")
	act("deploy-keycloak", "runs:\n  steps:\n    - uses: ./.github/actions/helper\n")
	act("helper", "runs:\n  using: composite\n")
	act("sonar-scan", "runs:\n  using: composite\n")
	act("create-meta-release-tag", "runs:\n  using: composite\n")

	wf(repo, "qa-stage.yml", "jobs:\n  qa:\n    uses: ./.github/workflows/_qa-stage.yml\n")
	wf(repo, "acceptance-stage-docker-compose.yml", "jobs:\n  a:\n    uses: './.github/workflows/_acceptance-stage-docker-compose.yml'\n")
	wf(repo, "prod-stage.yml", "jobs:\n  b:\n    uses: ./.github/workflows/bump-patch-version.yml\n")

	if err := copySharedPipeline(shop, repo); err != nil {
		t.Fatal(err)
	}
	gotWf, _ := listFilesWithExt(filepath.Join(repo, ".github", "workflows"), ".yml")
	for i := range gotWf {
		gotWf[i] = filepath.Base(gotWf[i])
	}
	wantWf := []string{"_acceptance-stage-docker-compose.yml", "_deploy-stage.yml", "_qa-stage.yml",
		"acceptance-stage-docker-compose.yml", "prod-stage.yml", "qa-stage.yml"}
	sort.Strings(wantWf)
	if !reflect.DeepEqual(gotWf, wantWf) {
		t.Errorf("workflows: got %v, want %v", gotWf, wantWf)
	}
	entries, _ := os.ReadDir(filepath.Join(repo, ".github", "actions"))
	var gotAct []string
	for _, e := range entries {
		gotAct = append(gotAct, e.Name())
	}
	if want := []string{"deploy-keycloak", "helper", "sonar-scan"}; !reflect.DeepEqual(gotAct, want) {
		t.Errorf("actions: got %v, want %v", gotAct, want)
	}
}

func TestCopySharedPipeline_MissingSharedWorkflowFails(t *testing.T) {
	t.Parallel()
	shop, repo := t.TempDir(), t.TempDir()
	writeTestFile(t, filepath.Join(repo, ".github", "workflows", "qa-stage.yml"), "jobs:\n  qa:\n    uses: ./.github/workflows/_qa-stage.yml\n")
	err := copySharedPipeline(shop, repo)
	if err == nil || !strings.Contains(err.Error(), "_qa-stage.yml") {
		t.Fatalf("want error naming _qa-stage.yml, got %v", err)
	}
}

func applyRewritePairs(s string, pairs [][2]string) string {
	for _, p := range pairs {
		s = strings.ReplaceAll(s, p[0], p[1])
	}
	return s
}

// Shop's shared workflows derive paths, environments and tags from their
// inputs; the scaffold flattens each to the single-flavor literal.
func TestSharedWorkflowRewrites(t *testing.T) {
	t.Parallel()
	in := strings.Join([]string{
		"  VERSION_FILE: system/${{ inputs.architecture }}/${{ inputs.language }}/VERSION",
		"  COMPOSE_DIR: docker/${{ inputs.test-language || inputs.language }}/${{ inputs.architecture }}",
		"    environment: ${{ inputs.architecture }}-${{ inputs.language }}-acceptance",
		"    environment: ${{ inputs.architecture }}-${{ inputs.language }}-${{ inputs.stage }}",
		"          environment: ${{ env.PREFIX }}-qa",
		"      GH_OPTIVEM_CONFIG: gh-optivem-${{ inputs.architecture }}-${{ inputs.test-language || inputs.language }}${{ inputs.suite-set == 'legacy' && '-legacy' || '' }}.yaml",
		"      GH_OPTIVEM_CONFIG: gh-optivem-${{ inputs.architecture }}-${{ inputs.test-language || inputs.language }}.yaml",
		"          tag-patterns: ${{ env.PREFIX }}-v*-rc.*",
		"            system-test/${{ env.TEST_LANGUAGE }}",
		"            .github/workflows/${{ env.PREFIX }}-acceptance-stage.yml",
		"          prefix: ${{ env.PREFIX }}",
		"          command: cd system-test/java && ./gradlew --version",
		`          } > "docker/${TEST_LANGUAGE}/${ARCHITECTURE}/systems.yaml"`,
		"          tag: ${{ fromJSON(inputs.images).backend }}-v${{ steps.v.outputs.backend }}",
		`          p="${SYSTEM}-${PREFIX}-${STAGE}"`,
	}, "\n")
	want := strings.Join([]string{
		"  VERSION_FILE: VERSION",
		"  COMPOSE_DIR: docker",
		"    environment: acceptance",
		"    environment: ${{ inputs.stage }}",
		"          environment: qa",
		"      GH_OPTIVEM_CONFIG: gh-optivem${{ inputs.suite-set == 'legacy' && '.legacy' || '' }}.yaml",
		"      GH_OPTIVEM_CONFIG: gh-optivem.yaml",
		"          tag-patterns: v*-rc.*",
		"            system-test",
		"            .github/workflows/acceptance-stage.yml",
		"          prefix: ''",
		"          command: cd system-test && ./gradlew --version",
		`          } > "docker/systems.yaml"`,
		"          tag: v${{ steps.v.outputs.backend }}",
		// Cloud resource names keep the flavor prefix.
		`          p="${SYSTEM}-${PREFIX}-${STAGE}"`,
	}, "\n")
	got := applyRewritePairs(in, sharedWorkflowRewrites())
	if got != want {
		t.Errorf("rewrite mismatch:\n got:\n%s\n want:\n%s", got, want)
	}
	for _, ref := range leftoverSharedWorkflowRefs {
		if strings.Contains(got, ref) {
			t.Errorf("leftover %q survived", ref)
		}
	}
}

// Multirepo: images come from the component repos.
func TestMultirepoSharedWorkflowRewrites(t *testing.T) {
	t.Parallel()
	in := "          BASE: ghcr.io/${{ github.repository_owner }}/${{ github.event.repository.name }}\n" +
		"      images: '{\"frontend\":\"frontend\",\"backend\":\"backend\"}'\n" +
		"          entries: ${{ steps.component-entries.outputs.entries }}\n"
	got := applyRewritePairs(in, multirepoSharedWorkflowRewrites("acme", map[string]string{"backend": "shop-be", "frontend": "shop-fe"}))
	for _, want := range []string{
		"BASE: ghcr.io/${{ github.repository_owner }}\n",
		`images: '{"frontend":"shop-fe/frontend","backend":"shop-be/backend"}'`,
		"token: ${{ secrets.WORKFLOW_TOKEN }}",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestPruneForeignCaseArms(t *testing.T) {
	t.Parallel()
	in := `        case "$PROJECT" in
          system/monolith/java|system/multitier/backend-java)
            ./gradlew checkstyleAll ;;
          system/monolith/dotnet)
            dotnet format MyCompany.MyShop.Monolith.sln --verify-no-changes ;;
          system/monolith/typescript|system/multitier/backend-typescript|system/multitier/frontend-react)
            npm run lint ;;
          *)
            exit 1 ;;
        esac`
	got := pruneForeignCaseArms(applyRewritePairs(in, projectPathRewrites("monolith", "dotnet")))
	want := `        case "$PROJECT" in
          system)
            dotnet format MyCompany.MyShop.Monolith.sln --verify-no-changes ;;
          *)
            exit 1 ;;
        esac`
	if got != want {
		t.Errorf("monolith-dotnet:\n got:\n%s\n want:\n%s", got, want)
	}

	got = pruneForeignCaseArms(applyRewritePairs(in, projectPathRewrites("multitier", "java")))
	want = `        case "$PROJECT" in
          backend)
            ./gradlew checkstyleAll ;;
          frontend)
            npm run lint ;;
          *)
            exit 1 ;;
        esac`
	if got != want {
		t.Errorf("multitier-java:\n got:\n%s\n want:\n%s", got, want)
	}
}

func TestScheduleLegacyGate(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	path := filepath.Join(repo, ".github", "workflows", Names.DestLegacyAcceptWf)
	writeTestFile(t, path, "name: x\n\n# Legacy suites. Dispatched by the prerelease pipeline (no cron).\n\non:\n  workflow_dispatch:\n    inputs: {}\n")
	scheduleLegacyGate(repo)
	data, _ := os.ReadFile(path)
	s := string(data)
	if !strings.Contains(s, "on:\n  schedule:\n    - cron: '0 * * * *'") || !strings.Contains(s, "  workflow_dispatch:\n") {
		t.Errorf("no hourly schedule:\n%s", s)
	}
	if strings.Contains(s, "prerelease pipeline") {
		t.Errorf("shop-only comment kept:\n%s", s)
	}
	// --no-legacy: no file, no-op.
	scheduleLegacyGate(t.TempDir())
}

func TestPipelineSection(t *testing.T) {
	t.Parallel()
	with := pipelineSection(&config.Config{})
	without := pipelineSection(&config.Config{NoLegacy: true})
	for _, s := range []string{with, without} {
		for _, want := range []string{"Works out of the box", "acceptance-stage-docker-compose.yml", "Needs GCP set up first",
			"[Cloud setup](#cloud-setup)", "### Cloud setup", "_qa-stage.yml", "_prod-stage.yml",
			"(https://github.com/optivem/gh-optivem/blob/main/docs/cloud-setup.md)", "./setup-gcp.sh"} {
			if !strings.Contains(s, want) {
				t.Errorf("missing %q in:\n%s", want, s)
			}
		}
	}
	if !strings.Contains(with, "acceptance-stage-docker-compose-legacy.yml") {
		t.Error("legacy gate not mentioned")
	}
	if strings.Contains(with, "is coming") {
		t.Error("Cloud setup subsection is still a placeholder")
	}
	if strings.Contains(without, "legacy") {
		t.Error("--no-legacy: legacy gate mentioned")
	}
}
