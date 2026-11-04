package kickoff

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/optivem/gh-optivem/internal/atdd/process/clauderun"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/intake"
	"github.com/optivem/gh-optivem/internal/spec"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const goodDescription = "A booking site for small yoga studios where members book and cancel classes."

const narrative = "---\nid: UC-001\ntitle: Book Class\n---\n\n# UC-001 Book Class\n\n## Main scenario\n1. Member books.\n\n- UC-001.1: A class is booked.\n\n## Extensions\n### 1a. Full\n- UC-001.2: A full class is refused.\n"

const goodStory = "# Book a class\n\n## Description\nA member books a class. Covers: UC-001.1\n\n## Acceptance Criteria\n@UC-001.1\nScenario: member books a class\n  Given a class with free seats\n  When a member books it\n  Then the booking is confirmed\n"

const goodStory2 = "# Refuse a full class\n\n## Description\nA full class is refused. Covers: UC-001.2\n\n## Acceptance Criteria\n@UC-001.2\nScenario: full class\n  Given a full class\n  When a member books it\n  Then the booking is refused\n"

func TestValidateDescription(t *testing.T) {
	for name, tc := range map[string]struct {
		in      string
		wantErr string
	}{
		"empty":     {"  \n", "empty"},
		"too short": {"a site", "too short"},
		"ok":        {goodDescription, ""},
		"too long":  {strings.Repeat("word ", 5000), "too long"},
	} {
		t.Run(name, func(t *testing.T) {
			err := ValidateDescription(tc.in)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestCaptureDescriptionSharesValidationAcrossSources(t *testing.T) {
	root := t.TempDir()
	l := Layout{Root: root}
	file := filepath.Join(root, "in.txt")
	os.WriteFile(file, []byte("tiny"), 0o644)

	if err := CaptureDescription(l, DescriptionInput{Text: "tiny"}); err == nil {
		t.Fatal("flag text: want validation error")
	}
	if err := CaptureDescription(l, DescriptionInput{File: file}); err == nil {
		t.Fatal("file: want validation error")
	}
	if err := CaptureDescription(l, DescriptionInput{Prompt: func() (string, error) { return "tiny", nil }}); err == nil {
		t.Fatal("prompt: want validation error")
	}
	if err := CaptureDescription(l, DescriptionInput{}); err == nil {
		t.Fatal("no source: want error")
	}
	if err := CaptureDescription(l, DescriptionInput{Text: goodDescription, File: file}); err == nil {
		t.Fatal("both sources: want error")
	}
	if err := CaptureDescription(l, DescriptionInput{Text: goodDescription}); err != nil {
		t.Fatal(err)
	}
	if err := CaptureDescription(l, DescriptionInput{Text: goodDescription}); err == nil {
		t.Fatal("existing file must not be overwritten")
	}
}

func TestFingerprintStableAcrossLineEndingsAndChangesOnEdit(t *testing.T) {
	root := t.TempDir()
	l := Layout{Root: root}
	write(t, root, "docs/prototype/index.html", "<a>\n</a>\n")
	a, err := Fingerprint(l, PhasePrototype)
	if err != nil {
		t.Fatal(err)
	}
	write(t, root, "docs/prototype/index.html", "<a>\r\n</a>\r\n")
	b, _ := Fingerprint(l, PhasePrototype)
	if a != b {
		t.Fatal("CRLF must not change the fingerprint")
	}
	write(t, root, "docs/prototype/two.html", "x")
	c, _ := Fingerprint(l, PhasePrototype)
	if a == c {
		t.Fatal("adding a file must change the fingerprint")
	}
	if _, err := Fingerprint(l, PhaseDiagram); err == nil {
		t.Fatal("missing diagram must be an error")
	}
}

func TestParseRedoPhase(t *testing.T) {
	for _, ok := range []string{"prototype", "use-case-diagram", "use-cases"} {
		if _, err := ParseRedoPhase(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"description", "remove-sample", "stories", "nonsense"} {
		if _, err := ParseRedoPhase(bad); err == nil {
			t.Errorf("%s: want error", bad)
		}
	}
}

func approved(t *testing.T, l Layout, phases ...Phase) *Progress {
	t.Helper()
	p := &Progress{Approvals: map[string]Approval{}}
	for _, ph := range phases {
		if err := p.Approve(l, ph, "tester", time.Unix(0, 0)); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func TestDetectDerivesFromFilesAndInvalidatesLaterApprovalsOnEdit(t *testing.T) {
	root := t.TempDir()
	l := Layout{Root: root}
	st, err := Detect(l, &Progress{Approvals: map[string]Approval{}}, Probes{})
	if err != nil || st.Next != PhaseDescription {
		t.Fatalf("empty repo: next=%q err=%v", st.Next, err)
	}

	write(t, root, ProductFile, goodDescription)
	write(t, root, "docs/prototype/index.html", "v1")
	write(t, root, DiagramFile, "diagram v1")
	write(t, root, "docs/use-cases/UC-001-book.md", narrative)
	write(t, root, sampleUseCaseFile, "sample")

	prog := approved(t, l, PhasePrototype, PhaseDiagram, PhaseUseCases)
	st, _ = Detect(l, prog, Probes{})
	if st.Next != PhaseRemove {
		t.Fatalf("all approved: next=%q, want %q\n%s", st.Next, PhaseRemove, st.Describe())
	}

	write(t, root, "docs/prototype/index.html", "v2")
	st, _ = Detect(l, prog, Probes{})
	if st.Next != PhasePrototype {
		t.Fatalf("edited prototype: next=%q", st.Next)
	}
	if !strings.Contains(st.Status(PhasePrototype).Detail, "re-approval") {
		t.Fatalf("detail = %q", st.Status(PhasePrototype).Detail)
	}
	if st.Status(PhaseDiagram).Done || st.Status(PhaseUseCases).Done {
		t.Fatal("later approvals must be invalid once an earlier one is")
	}
}

func TestSampleUseCaseIsNotACountedNarrative(t *testing.T) {
	root := t.TempDir()
	l := Layout{Root: root}
	write(t, root, sampleUseCaseFile, "sample")
	files, _ := narrativeFiles(l)
	if len(files) != 0 {
		t.Fatalf("sample counted as narrative: %v", files)
	}
}

func TestProgressSaveLoadAndInvalidate(t *testing.T) {
	root := t.TempDir()
	l := Layout{Root: root}
	write(t, root, "docs/prototype/index.html", "v1")
	write(t, root, DiagramFile, "d")
	prog := approved(t, l, PhasePrototype, PhaseDiagram)
	if err := prog.Save(l); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadProgress(l)
	if err != nil || len(loaded.Approvals) != 2 {
		t.Fatalf("loaded=%v err=%v", loaded, err)
	}
	loaded.Invalidate(PhasePrototype)
	if len(loaded.Approvals) != 0 {
		t.Fatalf("invalidate must drop later approvals too: %v", loaded.Approvals)
	}
	loaded.Save(l)
	if _, err := os.Stat(l.Abs(ProgressFile)); !os.IsNotExist(err) {
		t.Fatal("empty progress must remove the file")
	}
	write(t, root, ProgressFile, "approvals:\n  stories:\n    by: x\n")
	if _, err := LoadProgress(l); err == nil {
		t.Fatal("non-approval phase in progress file must be rejected")
	}
}

func TestRenderAgentFillsEveryPlaceholder(t *testing.T) {
	for _, name := range AgentNames {
		prompt, tuning, err := RenderAgent(name, agentVars("make it blue"))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if tuning.Model == "" || tuning.Effort == "" {
			t.Errorf("%s: tuning %+v", name, tuning)
		}
		if strings.Contains(prompt, "${") {
			t.Errorf("%s: unfilled placeholder", name)
		}
		if strings.HasPrefix(prompt, "---") {
			t.Errorf("%s: frontmatter leaked into the prompt", name)
		}
		if name != AgentStoryWriter && !strings.Contains(prompt, "make it blue") {
			t.Errorf("%s: feedback missing", name)
		}
	}
	prompt, _, _ := RenderAgent(AgentStoryWriter, agentVars(""))
	if !strings.Contains(prompt, "Scenario") {
		t.Error("story-writer must embed the acceptance criteria format")
	}
}

func TestRenderAgentRejectsUnknownAndUnfilled(t *testing.T) {
	if _, _, err := RenderAgent("nope", nil); err == nil {
		t.Fatal("unknown agent must fail")
	}
	if _, _, err := RenderAgent(AgentPrototyper, map[string]string{}); err == nil {
		t.Fatal("unfilled placeholders must fail")
	}
}

func TestOutOfScope(t *testing.T) {
	before := map[string]string{"a.txt": "1"}
	after := map[string]string{"a.txt": "2", "docs/prototype/x.html": "h", "docs/other.md": "h"}
	got := outOfScope(before, after, []string{PrototypeDir + "/"})
	if strings.Join(got, ",") != "a.txt,docs/other.md" {
		t.Fatalf("got %v", got)
	}
	if len(outOfScope(before, after, []string{"a.txt", "docs/"})) != 0 {
		t.Fatal("exact and prefix scopes must both allow")
	}
}

type fakeClaude struct {
	prompts []string
	edit    func()
}

func (f *fakeClaude) Run(_ context.Context, o clauderun.RunOpts) (clauderun.RunResult, error) {
	f.prompts = append(f.prompts, o.Prompt)
	if f.edit != nil {
		f.edit()
	}
	return clauderun.RunResult{}, nil
}

func TestAgentRunnerEnforcesScope(t *testing.T) {
	root := t.TempDir()
	snap := map[string]string{}
	fc := &fakeClaude{edit: func() { snap["README.md"] = "changed" }}
	r := AgentRunner{Layout: Layout{Root: root}, Claude: fc, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		Snapshot: func(string) (map[string]string, error) {
			c := map[string]string{}
			for k, v := range snap {
				c[k] = v
			}
			return c, nil
		}}
	err := r.Run(context.Background(), AgentPrototyper, agentVars(""))
	if err == nil || !strings.Contains(err.Error(), "README.md") {
		t.Fatalf("want out-of-scope error naming README.md, got %v", err)
	}
	if len(fc.prompts) != 1 {
		t.Fatal("agent must have run once")
	}
}

type fakeAgents struct {
	root string
	ran  []string
	// onRun writes the agent's output.
	onRun func(name string)
}

func (f *fakeAgents) Run(_ context.Context, name string, _ map[string]string) error {
	f.ran = append(f.ran, name)
	f.onRun(name)
	return nil
}

func newFlowOptions(t *testing.T, root string, out *bytes.Buffer) (Options, *fakeAgents, *fakePublisher) {
	t.Helper()
	validateNarratives = func(Layout) error { return nil }
	t.Cleanup(func() { validateNarratives = nil })
	fa := &fakeAgents{root: root}
	fa.onRun = func(name string) {
		switch name {
		case AgentPrototyper:
			write(t, root, "docs/prototype/index.html", "proto")
		case AgentDiagramWriter:
			write(t, root, DiagramFile, "diagram")
		case AgentUseCaseWriter:
			write(t, root, "docs/use-cases/UC-001-book.md", narrative)
		case AgentStoryWriter:
			write(t, root, StoriesDraft+"/001-book.md", goodStory)
			write(t, root, StoriesDraft+"/002-full.md", goodStory2)
		}
	}
	removed := false
	fp := &fakePublisher{}
	return Options{
		Layout: Layout{Root: root}, Stdout: out, Agents: fa, Publisher: fp,
		Remove: func(l Layout) error { removed = true; return nil }, SampleRemoved: func(Layout) (bool, error) { return removed, nil }, User: "tester", Now: func() time.Time { return time.Unix(0, 0) },
	}, fa, fp
}

type fakePublisher struct {
	created []Created
	closed  map[string]bool
}

func (p *fakePublisher) Publish(_ context.Context, d []StoryDraft) ([]Created, error) {
	for i, s := range d {
		p.created = append(p.created, Created{ID: string(rune('1' + i)), Title: s.Title})
	}
	return p.created, nil
}
func (p *fakePublisher) Existing(context.Context) ([]Created, error) { return p.created, nil }
func (p *fakePublisher) Open(context.Context) ([]Created, error) {
	var o []Created
	for _, c := range p.created {
		if !p.closed[c.ID] {
			o = append(o, c)
		}
	}
	return o, nil
}

func TestRunNonInteractiveStopsAtEachCheckpointAndResumes(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	o, fa, _ := newFlowOptions(t, root, &out)
	o.Description = DescriptionInput{Text: goodDescription}
	ctx := context.Background()

	if err := Run(ctx, o); err != nil {
		t.Fatal(err)
	}
	if strings.Join(fa.ran, ",") != AgentPrototyper {
		t.Fatalf("first run ran %v", fa.ran)
	}
	o.Description = DescriptionInput{}

	// re-run without approval: no new agent run
	if err := Run(ctx, o); err != nil || len(fa.ran) != 1 {
		t.Fatalf("re-run must not regenerate: ran=%v err=%v", fa.ran, err)
	}
	o.Approve = true
	if err := Run(ctx, o); err != nil {
		t.Fatal(err)
	}
	if strings.Join(fa.ran, ",") != AgentPrototyper+","+AgentDiagramWriter {
		t.Fatalf("after approve ran %v", fa.ran)
	}
	if _, err := os.Stat(filepath.Join(root, ProgressFile)); err != nil {
		t.Fatal("approval must be recorded")
	}
}

func TestRunFullFlowThroughStoriesAndAutopilot(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	o, fa, fp := newFlowOptions(t, root, &out)
	write(t, root, sampleUseCaseFile, "sample")
	o.Description = DescriptionInput{Text: goodDescription}
	var implemented []string
	o.Implement = func(_ context.Context, s Created) error {
		implemented = append(implemented, s.ID)
		return nil
	}
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		o.Approve = true
		if err := Run(ctx, o); err != nil {
			t.Fatalf("run %d: %v\n%s", i, err, out.String())
		}
	}
	if len(fp.created) != 2 {
		t.Fatalf("stories created = %v\nran %v\n%s", fp.created, fa.ran, out.String())
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(sampleUseCaseFile))); !os.IsNotExist(err) {
		t.Fatal("sample use case must be deleted before the narratives are written")
	}
	o.Approve = false
	o.Autopilot = true
	if err := Run(ctx, o); err != nil {
		t.Fatal(err)
	}
	if strings.Join(implemented, ",") != "1,2" {
		t.Fatalf("autopilot implemented %v", implemented)
	}
}

func TestAutopilotStopsOnFailureAndResumesWithRemaining(t *testing.T) {
	fp := &fakePublisher{created: []Created{{ID: "1", Title: "a"}, {ID: "2", Title: "b"}, {ID: "3", Title: "c"}}, closed: map[string]bool{}}
	var out bytes.Buffer
	var ran []string
	o := Options{Stdout: &out, Publisher: fp}
	o.Implement = func(_ context.Context, s Created) error {
		ran = append(ran, s.ID)
		if s.ID == "2" {
			return errors.New("red")
		}
		fp.closed[s.ID] = true
		return nil
	}
	err := RunAutopilot(context.Background(), o)
	if err == nil || !strings.Contains(err.Error(), "story 2") {
		t.Fatalf("err = %v", err)
	}
	fp.closed["2"] = true
	ran = nil
	o.Implement = func(_ context.Context, s Created) error { ran = append(ran, s.ID); return nil }
	if err := RunAutopilot(context.Background(), o); err != nil || strings.Join(ran, ",") != "3" {
		t.Fatalf("resume ran %v err=%v", ran, err)
	}
}

func TestRunRedoDropsLaterApprovalsAndRegenerates(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	o, fa, _ := newFlowOptions(t, root, &out)
	l := o.Layout
	write(t, root, ProductFile, goodDescription)
	write(t, root, "docs/prototype/index.html", "proto")
	write(t, root, DiagramFile, "diagram")
	prog := approved(t, l, PhasePrototype, PhaseDiagram)
	prog.Save(l)

	o.Redo = PhasePrototype
	if err := Run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if len(fa.ran) != 1 || fa.ran[0] != AgentPrototyper {
		t.Fatalf("redo must regenerate the prototype only: %v", fa.ran)
	}
	loaded, _ := LoadProgress(l)
	if len(loaded.Approvals) != 0 {
		t.Fatalf("approvals after redo: %v", loaded.Approvals)
	}
}

func TestRunRemovalUndecidedFailsLoud(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	o, _, _ := newFlowOptions(t, root, &out)
	o.Remove = DefaultRemove
	write(t, root, ProductFile, goodDescription)
	for _, a := range []struct {
		rel, c string
	}{{"docs/prototype/index.html", "p"}, {DiagramFile, "d"}, {"docs/use-cases/UC-001-book.md", narrative}} {
		write(t, root, a.rel, a.c)
	}
	approved(t, o.Layout, PhasePrototype, PhaseDiagram, PhaseUseCases).Save(o.Layout)
	err := Run(context.Background(), o)
	if !errors.Is(err, ErrRemovalUndecided) {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadStoryDraftsValidation(t *testing.T) {
	root := t.TempDir()
	l := Layout{Root: root}
	write(t, root, "docs/use-cases/UC-001-book.md", narrative)

	if _, err := LoadStoryDrafts(l); err == nil || !strings.Contains(err.Error(), "no story drafts") {
		t.Fatalf("err = %v", err)
	}
	write(t, root, StoriesDraft+"/001-book.md", goodStory)
	if _, err := LoadStoryDrafts(l); err == nil || !strings.Contains(err.Error(), "UC-001.2") {
		t.Fatalf("uncovered criterion must be reported: %v", err)
	}
	write(t, root, StoriesDraft+"/002-full.md", goodStory2)
	d, err := LoadStoryDrafts(l)
	if err != nil || len(d) != 2 || d[0].Title != "Book a class" {
		t.Fatalf("drafts=%v err=%v", d, err)
	}
	write(t, root, StoriesDraft+"/003-bad.md", "# Bad\n\n## Description\nCovers UC-009.1\n\n## Acceptance Criteria\n@UC-009.1\nScenario: x\n  Given a\n  When b\n  Then c\n")
	if _, err := LoadStoryDrafts(l); err == nil || !strings.Contains(err.Error(), "UC-009.1") {
		t.Fatalf("unknown criterion must be reported: %v", err)
	}
	write(t, root, StoriesDraft+"/003-bad.md", "# Bad\n\n## Notes\nstuff\n")
	if _, err := LoadStoryDrafts(l); err == nil {
		t.Fatal("non-canonical section must be rejected by intake")
	}
}

type fakeGH struct {
	calls [][]string
	out   map[string]string
}

func (f *fakeGH) Run(_ context.Context, stdin string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	key := strings.Join(args[:2], " ")
	return []byte(f.out[key]), nil
}

func TestGitHubPublisher(t *testing.T) {
	gh := &fakeGH{out: map[string]string{
		"api --method":     `{"number": 7, "html_url": "https://github.com/o/r/issues/7"}`,
		"issue list":       `[{"number":9,"title":"b","url":"u9"},{"number":7,"title":"a","url":"u7"}]`,
		"label create":     "",
		"project item-add": "",
	}}
	p := GitHubPublisher{GH: gh, Repo: "o/r", ProjectURL: "https://github.com/orgs/o/projects/3"}
	created, err := p.Publish(context.Background(), []StoryDraft{{Title: "a", Body: "b"}})
	if err != nil || len(created) != 1 || created[0].ID != "7" {
		t.Fatalf("created=%v err=%v", created, err)
	}
	last := strings.Join(gh.calls[len(gh.calls)-1], " ")
	if !strings.Contains(last, "project item-add 3 --owner o --url https://github.com/o/r/issues/7") {
		t.Fatalf("board add call = %q", last)
	}
	ex, _ := p.Existing(context.Background())
	if len(ex) != 2 || ex[0].ID != "7" {
		t.Fatalf("existing must be ordered by number: %v", ex)
	}
	bad := GitHubPublisher{GH: gh, Repo: "o/r", ProjectURL: "nope"}
	if _, err := bad.Publish(context.Background(), nil); err == nil {
		t.Fatal("bad project url must fail")
	}
}

func TestMarkdownPublisher(t *testing.T) {
	board := t.TempDir()
	p := MarkdownPublisher{BoardDir: board}
	created, err := p.Publish(context.Background(), []StoryDraft{{File: "001-book.md", Title: "Book", Body: "## Description\nx\n"}})
	if err != nil || created[0].ID != "story-kickoff-001-book" {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if _, err := p.Publish(context.Background(), []StoryDraft{{File: "001-book.md", Title: "Book", Body: "x"}}); err == nil {
		t.Fatal("must not overwrite")
	}
	open, _ := p.Open(context.Background())
	if len(open) != 1 {
		t.Fatalf("open = %v", open)
	}
	os.Rename(filepath.Join(board, "ready", "story-kickoff-001-book.md"), filepath.Join(board, "ready", "x.md"))
	os.MkdirAll(filepath.Join(board, "done"), 0o755)
	os.Rename(filepath.Join(board, "ready", "x.md"), filepath.Join(board, "done", "story-kickoff-001-book.md"))
	open, _ = p.Open(context.Background())
	ex, _ := p.Existing(context.Background())
	if len(open) != 0 || len(ex) != 1 {
		t.Fatalf("open=%v existing=%v", open, ex)
	}
}

func TestUntaggedScenarioIsRejected(t *testing.T) {
	_, err := parseDraft("001-x.md", strings.Replace(goodStory, "@UC-001.1\n", "", 1))
	if err == nil || !strings.Contains(err.Error(), "no @UC-NNN.M tag") {
		t.Fatalf("err = %v", err)
	}
}

// Generated stories must pass the story-merge no-op check, otherwise unattended
// runs stall on a merge approval.
func TestGeneratedStoriesPassStoryMergeNoOpCheck(t *testing.T) {
	root := t.TempDir()
	write(t, root, "docs/use-cases/UC-001-book.md", narrative)
	ucs, err := spec.LoadDir(filepath.Join(root, "docs", "use-cases"))
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{goodStory, goodStory2} {
		d, err := parseDraft("001-x.md", body)
		if err != nil {
			t.Fatal(err)
		}
		res, err := intake.Parse(d.Body)
		if err != nil {
			t.Fatal(err)
		}
		merged, ids := spec.StoryAlreadyMerged(spec.StoryScenarios(res.AcceptanceCriteria.Body), ucs)
		if !merged || len(ids) == 0 {
			t.Fatalf("story would need a merge approval: merged=%v ids=%v", merged, ids)
		}
	}
}
