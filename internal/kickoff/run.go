package kickoff

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Agents runs a kickoff agent by name.
type Agents interface {
	Run(ctx context.Context, name string, vars map[string]string) error
}

// RemoveFunc deletes the shop sample so the repo keeps the empty test
// architecture. It must be deterministic code, never an agent.
type RemoveFunc func(l Layout) error

// ImplementFunc runs the ATDD flow over one created story.
type ImplementFunc func(ctx context.Context, story Created) error

// Options configures one `kickoff` invocation. Every collaborator is a field
// so tests can drive the flow without claude, gh or a terminal.
type Options struct {
	Layout Layout
	Stdin  io.Reader
	Stdout io.Writer
	// Interactive is true when Stdin is a terminal: checkpoints then ask the
	// user to approve, give feedback, or stop.
	Interactive bool

	Redo      Phase  // reopen this approval phase (and invalidate every later one)
	Approve   bool   // approve the pending phase without prompting
	Feedback  string // feedback for the pending phase's agent
	Autopilot bool   // after use case approval, run every story unattended

	Description DescriptionInput

	Agents    Agents
	Remove    RemoveFunc
	Publisher Publisher
	Implement ImplementFunc
	// Confirm gates irreversible actions (creating the stories). It must not be
	// auto-approvable; nil means always proceed (tests).
	Confirm func(prompt string) (bool, error)

	User string // recorded as the approver
	Now  func() time.Time

	// SampleRemoved overrides how "sample removed" is derived (tests).
	SampleRemoved func(Layout) (bool, error)

	in *bufio.Reader
}

// Run resumes the flow at the next incomplete phase. Each approval phase ends
// at a checkpoint: with a terminal the user answers there, otherwise the
// command stops and says what to review; either way re-running continues.
func Run(ctx context.Context, o Options) error {
	o = withRunDefaults(o)
	prog, err := LoadProgress(o.Layout)
	if err != nil {
		return err
	}
	pendingFeedback := o.Feedback
	approveBudget := o.Approve
	forceRegenerate, err := reopenPhase(o, prog)
	if err != nil {
		return err
	}

	probes := Probes{
		SampleRemoved:  sampleProbe(o),
		StoriesCreated: storiesCreatedProbe(ctx, o),
	}

	for step := 0; step < 64; step++ {
		st, err := Detect(o.Layout, prog, Probes{
			SampleRemoved: probes.SampleRemoved,
			// Stories are only probed once the earlier phases are done, so an
			// unconfigured tracker does not block the early phases.
			StoriesCreated: func() (bool, error) { return false, nil },
		})
		if err != nil {
			return err
		}
		done, err := resolveStoriesPhase(probes, &st)
		if err != nil {
			return err
		}
		if done {
			return finish(ctx, o, st)
		}
		stop, err := runNextPhase(ctx, o, prog, st, &pendingFeedback, &approveBudget, &forceRegenerate)
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
	}
	return errors.New("kickoff did not converge after 64 steps: this is a bug")
}

// withRunDefaults fills in the clock, stdin and the buffered reader Run uses.
func withRunDefaults(o Options) Options {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Stdin == nil {
		o.Stdin = strings.NewReader("")
	}
	o.in = bufio.NewReader(o.Stdin)
	return o
}

// reopenPhase applies --redo: it invalidates the phase (and every later one)
// and returns the phase whose output must be regenerated.
func reopenPhase(o Options, prog *Progress) (Phase, error) {
	if o.Redo == "" {
		return "", nil
	}
	prog.Invalidate(o.Redo)
	if err := prog.Save(o.Layout); err != nil {
		return "", err
	}
	fmt.Fprintf(o.Stdout, "Reopened %s; later approvals need to be given again.\n", o.Redo)
	return o.Redo, nil
}

func storiesCreatedProbe(ctx context.Context, o Options) func() (bool, error) {
	return func() (bool, error) {
		if o.Publisher == nil {
			return false, errors.New("no tracker configured to check created stories")
		}
		existing, err := o.Publisher.Existing(ctx)
		return len(existing) > 0, err
	}
}

// resolveStoriesPhase probes the tracker once every earlier phase is done. It
// returns done=true (and marks the stories phase done) when the stories exist;
// otherwise it points st at the stories phase.
func resolveStoriesPhase(probes Probes, st *State) (bool, error) {
	if st.Next != "" && st.Next != PhaseStories {
		return false, nil
	}
	created, err := probes.StoriesCreated()
	if err != nil {
		return false, err
	}
	if created {
		st.Statuses[len(st.Statuses)-1].Done = true
		return true, nil
	}
	st.Next = PhaseStories
	return false, nil
}

// runNextPhase runs st.Next. It returns stop=true when the command should end
// for the user to review.
func runNextPhase(ctx context.Context, o Options, prog *Progress, st State, feedback *string, approveBudget *bool, force *Phase) (bool, error) {
	switch st.Next {
	case PhaseDescription:
		return false, runDescription(o)
	case PhasePrototype, PhaseDiagram, PhaseUseCases:
		return runApprovalPhase(ctx, o, prog, st, feedback, approveBudget, force)
	case PhaseRemove:
		return false, runRemove(o)
	case PhaseStories:
		return false, runStories(ctx, o)
	}
	return false, nil
}

func runRemove(o Options) error {
	if o.Remove == nil {
		return errors.New("shop removal is not available")
	}
	fmt.Fprintln(o.Stdout, "Removing the shop sample ...")
	if err := o.Remove(o.Layout); err != nil {
		return fmt.Errorf("remove shop sample: %w", err)
	}
	if gone, err := sampleProbe(o)(o.Layout); err != nil || !gone {
		return fmt.Errorf("shop removal finished but the sample is still detected (check: %v): this is a bug in the removal step", err)
	}
	return nil
}

func finish(ctx context.Context, o Options, st State) error {
	fmt.Fprint(o.Stdout, st.Describe())
	if !o.Autopilot {
		fmt.Fprintln(o.Stdout, "All kickoff phases are done. Build the stories one at a time with `gh optivem implement <issue>` (attended), or re-run with --autopilot.")
		return nil
	}
	return RunAutopilot(ctx, o)
}

func runDescription(o Options) error {
	in := o.Description
	if in.Text == "" && in.File == "" && in.Prompt == nil && o.Interactive {
		in.Prompt = func() (string, error) { return PromptDescription(o.Stdin, o.Stdout) }
	}
	if err := CaptureDescription(o.Layout, in); err != nil {
		return err
	}
	fmt.Fprintf(o.Stdout, "Wrote %s.\n", ProductFile)
	return nil
}

func agentFor(ph Phase) string {
	switch ph {
	case PhasePrototype:
		return AgentPrototyper
	case PhaseDiagram:
		return AgentDiagramWriter
	}
	return AgentUseCaseWriter
}

func agentVars(feedback string) map[string]string {
	if strings.TrimSpace(feedback) == "" {
		feedback = "(none)"
	}
	return map[string]string{
		"description-path": ProductFile,
		"prototype-dir":    PrototypeDir,
		"diagram-path":     DiagramFile,
		"use-cases-dir":    UseCasesDir,
		"stories-dir":      StoriesDraft,
		"feedback":         feedback,
	}
}

func reviewHint(ph Phase) string {
	switch ph {
	case PhasePrototype:
		return "Open " + PrototypeDir + "/index.html in a browser and click through it."
	case PhaseDiagram:
		return "Review " + DiagramFile + ": the screen-to-use-case table, the use case diagram and the external systems."
	}
	return "Review the narratives in " + UseCasesDir + "/ and their UC-NNN.M criteria."
}

// runApprovalPhase generates the phase output when needed, then reaches the
// checkpoint. It returns stop=true when the command should end for the user
// to review.
func runApprovalPhase(ctx context.Context, o Options, prog *Progress, st State, feedback *string, approveBudget *bool, force *Phase) (bool, error) {
	ph := st.Next
	if err := generatePhase(ctx, o, ph, st.Status(ph), feedback, force); err != nil {
		return false, err
	}

	for {
		fmt.Fprintf(o.Stdout, "\n%s\n", reviewHint(ph))
		switch {
		case *approveBudget:
			*approveBudget = false
			return false, approve(o, prog, ph)
		case !o.Interactive:
			fmt.Fprintf(o.Stdout, "Review it, then run `gh optivem kickoff --approve` to approve %s, or `gh optivem kickoff --feedback \"...\"` to change it.\n", ph)
			return true, nil
		}
		answer, err := askApproval(o, ph)
		if err != nil {
			return false, err
		}
		switch strings.ToLower(answer) {
		case "":
			fmt.Fprintln(o.Stdout, "Stopped. Re-run `gh optivem kickoff` to continue.")
			return true, nil
		case "y", "yes":
			return false, approve(o, prog, ph)
		}
		fmt.Fprintf(o.Stdout, "Running the %s agent with your feedback ...\n", agentFor(ph))
		if err := runPhaseAgent(ctx, o, ph, answer); err != nil {
			return false, err
		}
	}
}

// generatePhase runs the phase agent when the output is missing, was reopened,
// or has pending feedback; otherwise it reports the phase's status detail.
func generatePhase(ctx context.Context, o Options, ph Phase, status PhaseStatus, feedback *string, force *Phase) error {
	regenerate := !status.Generated || *force == ph || *feedback != ""
	if !regenerate {
		if status.Detail != "" {
			fmt.Fprintf(o.Stdout, "%s: %s\n", ph, status.Detail)
		}
		return nil
	}
	if ph == PhaseUseCases {
		if err := removeSampleUseCases(o.Layout); err != nil {
			return err
		}
	}
	fmt.Fprintf(o.Stdout, "Running the %s agent ...\n", agentFor(ph))
	if err := runPhaseAgent(ctx, o, ph, *feedback); err != nil {
		return err
	}
	*feedback = ""
	*force = ""
	return nil
}

// runPhaseAgent runs the phase's agent with feedback and, for use cases,
// validates the narratives it wrote.
func runPhaseAgent(ctx context.Context, o Options, ph Phase, feedback string) error {
	if err := o.Agents.Run(ctx, agentFor(ph), agentVars(feedback)); err != nil {
		return err
	}
	if ph == PhaseUseCases {
		return ValidateNarratives(o.Layout)
	}
	return nil
}

// askApproval prompts at the checkpoint and returns the trimmed answer.
func askApproval(o Options, ph Phase) (string, error) {
	fmt.Fprintf(o.Stdout, "Approve %s? [y] approve, type feedback for the agent, or press Enter to stop and review later: ", ph)
	line, err := o.in.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read answer: %w", err)
	}
	return strings.TrimSpace(line), nil
}

func approve(o Options, prog *Progress, ph Phase) error {
	if o.User == "" {
		return errors.New("cannot record an approval: no approver identity (set git config user.name)")
	}
	if err := prog.Approve(o.Layout, ph, o.User, o.Now()); err != nil {
		return err
	}
	if err := prog.Save(o.Layout); err != nil {
		return err
	}
	fmt.Fprintf(o.Stdout, "Approved %s (recorded in %s).\n", ph, ProgressFile)
	return nil
}

func runStories(ctx context.Context, o Options) error {
	if o.Publisher == nil {
		return errors.New("no tracker configured: cannot create stories")
	}
	drafts, err := LoadStoryDrafts(o.Layout)
	if err != nil {
		var noDrafts = strings.HasPrefix(err.Error(), "no story drafts")
		if !noDrafts {
			return err
		}
		fmt.Fprintf(o.Stdout, "Running the %s agent ...\n", AgentStoryWriter)
		if err := o.Agents.Run(ctx, AgentStoryWriter, agentVars("")); err != nil {
			return err
		}
		drafts, err = LoadStoryDrafts(o.Layout)
		if err != nil {
			return err
		}
	}
	fmt.Fprintf(o.Stdout, "\n%d stories ready to create, in build order:\n", len(drafts))
	for i, d := range drafts {
		fmt.Fprintf(o.Stdout, "  %d. %s (covers %s)\n", i+1, d.Title, strings.Join(d.Covers, ", "))
	}
	if o.Confirm != nil {
		ok, err := o.Confirm("Create these stories in the tracker?")
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("story creation declined; the drafts stay in " + StoriesDraft + " and `gh optivem kickoff` resumes here")
		}
	}
	created, err := o.Publisher.Publish(ctx, drafts)
	for _, c := range created {
		fmt.Fprintf(o.Stdout, "Created %s %s\n", c.ID, c.Title)
	}
	if err != nil {
		return err
	}
	return os.RemoveAll(o.Layout.Abs(StoriesDraft))
}

// RunAutopilot runs the ATDD flow over every open kickoff story in build
// order, stopping at the first failure.
func RunAutopilot(ctx context.Context, o Options) error {
	if o.Implement == nil || o.Publisher == nil {
		return errors.New("autopilot needs a tracker and the implement runner")
	}
	open, err := o.Publisher.Open(ctx)
	if err != nil {
		return err
	}
	if len(open) == 0 {
		fmt.Fprintln(o.Stdout, "Autopilot: every kickoff story is already done.")
		return nil
	}
	for i, s := range open {
		fmt.Fprintf(o.Stdout, "\nAutopilot %d/%d: %s %s\n", i+1, len(open), s.ID, s.Title)
		if err := o.Implement(ctx, s); err != nil {
			return fmt.Errorf("autopilot stopped at story %s (%s): %w; fix it, then re-run `gh optivem kickoff --autopilot` to continue with the remaining stories", s.ID, s.Title, err)
		}
	}
	fmt.Fprintln(o.Stdout, "\nAutopilot finished: every kickoff story ran.")
	return nil
}

// RunPhase runs exactly one phase, with no checkpoint loop and no approval. It
// backs the hidden `kickoff phase <name>` subcommands used by tests and for
// debugging; users go through Run.
func RunPhase(ctx context.Context, o Options, ph Phase) error {
	if o.Stdin == nil {
		o.Stdin = strings.NewReader("")
	}
	switch ph {
	case PhaseDescription:
		return runDescription(o)
	case PhasePrototype, PhaseDiagram, PhaseUseCases:
		if ph == PhaseUseCases {
			if err := removeSampleUseCases(o.Layout); err != nil {
				return err
			}
		}
		if err := o.Agents.Run(ctx, agentFor(ph), agentVars(o.Feedback)); err != nil {
			return err
		}
		if ph == PhaseUseCases {
			return ValidateNarratives(o.Layout)
		}
		return nil
	case PhaseRemove:
		if o.Remove == nil {
			return errors.New("shop removal is not available")
		}
		return o.Remove(o.Layout)
	case PhaseStories:
		return runStories(ctx, o)
	}
	return fmt.Errorf("unknown phase %q", ph)
}

func sampleProbe(o Options) func(Layout) (bool, error) {
	if o.SampleRemoved != nil {
		return o.SampleRemoved
	}
	return sampleRemoved
}
