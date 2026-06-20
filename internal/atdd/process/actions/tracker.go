package actions

import (
	"context"
	"fmt"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/intake"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/tracker"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// ---------------------------------------------------------------------------
// State-transition actions (MARK_* service tasks)
// ---------------------------------------------------------------------------

// moveToInRefinement flips the picked issue's status to "In refinement"
// via Tracker.SetStatus. Wired to the MARK_IN_REFINEMENT node at the
// start of refine-ticket.
func (a actions) moveToInRefinement(ctx *statemachine.Context) statemachine.Outcome {
	handle := ctx.GetString("issue-handle")
	if handle == "" {
		return statemachine.Outcome{Err: fmt.Errorf("move-to-in-refinement: issue-handle not in Context")}
	}
	if err := a.deps.Tracker.SetStatus(context.Background(), handle, "In refinement"); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("move-to-in-refinement: %w", err)}
	}
	fmt.Fprintln(a.deps.Out.Phase, "Moved card to In refinement.")
	return statemachine.Outcome{}
}

// moveToReady flips the picked issue's status to "Ready" via
// Tracker.SetStatus. Wired to the MARK_READY node at the end of
// refine-ticket.
func (a actions) moveToReady(ctx *statemachine.Context) statemachine.Outcome {
	handle := ctx.GetString("issue-handle")
	if handle == "" {
		return statemachine.Outcome{Err: fmt.Errorf("move-to-ready: issue-handle not in Context")}
	}
	if err := a.deps.Tracker.SetStatus(context.Background(), handle, "Ready"); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("move-to-ready: %w", err)}
	}
	fmt.Fprintln(a.deps.Out.Phase, "Moved card to Ready.")
	return statemachine.Outcome{}
}

// moveToInProgress sets the picked issue's status to "In progress" via
// Tracker.SetStatus. Reads issue-handle from Context — populated by the
// driver's issue-lookup path (preResolveIssue).
func (a actions) moveToInProgress(ctx *statemachine.Context) statemachine.Outcome {
	handle := ctx.GetString("issue-handle")
	if handle == "" {
		return statemachine.Outcome{Err: fmt.Errorf("move-to-in-progress: issue-handle not in Context (requires explicit pre-resolution)")}
	}
	if err := a.deps.Tracker.SetStatus(context.Background(), handle, "In progress"); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("move-to-in-progress: %w", err)}
	}
	fmt.Fprintln(a.deps.Out.Phase, "Moved card to In progress.")
	return statemachine.Outcome{}
}

// moveToInAcceptance sets the item status to "In acceptance" via
// Tracker.SetStatus. Errors out hard on failure — a missing Status
// option or a permission failure on edit is a misconfiguration the
// operator must fix before re-running.
func (a actions) moveToInAcceptance(ctx *statemachine.Context) statemachine.Outcome {
	handle := ctx.GetString("issue-handle")
	if handle == "" {
		return statemachine.Outcome{Err: fmt.Errorf("move-to-in-acceptance: issue-handle not in Context")}
	}
	if err := a.deps.Tracker.SetStatus(context.Background(), handle, "In acceptance"); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("move-to-in-acceptance: %w", err)}
	}
	fmt.Fprintln(a.deps.Out.Phase, "Moved card to In acceptance.")
	return statemachine.Outcome{}
}

// parseTicket runs the deterministic markdown parser against the picked
// issue's body and stashes the canonical sections into Context state
// under kebab-cased keys (description, acceptance-criteria,
// steps-to-reproduce, checklist, plus the External System Contract Criteria
// triple: ticket-has-escc, escc-systems, external-system-contract-criteria).
// YAML placeholders consume these
// directly via ExpandParams's state-fallback path; agent prompt bodies
// consume them via the renderer's struct→params translation in
// clauderun.go, which exposes them under the matching kebab-cased
// placeholders (${acceptance-criteria}, ${checklist}).
//
// Wired in implement-ticket between MARK_IN_PROGRESS and GATE_TICKET_KIND
// — runs once per ticket, before the gateway routes to the cycle. The
// parser is ticket-kind-agnostic (Decision 2 in plan
// 20260526-1300): it does shape-level validation only (AC XOR Checklist)
// and lets the load-bearing placeholder check in clauderun.go enforce
// per-kind required sections at dispatch time. That lets one PARSE_TICKET
// node serve all six branches off GATE_TICKET_KIND.
func (a actions) parseTicket(ctx *statemachine.Context) statemachine.Outcome {
	issue, err := issueFromContext(ctx)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("parse-ticket: %w", err)}
	}
	sections, err := a.deps.Tracker.ReadSections(context.Background(), issue, intake.CanonicalHeadings)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("parse-ticket: read sections: %w", err)}
	}
	r, err := intake.ParseSections(sections)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("parse-ticket: %w", err)}
	}
	ctx.Set("description", r.Description.Body)
	ctx.Set("acceptance-criteria", r.AcceptanceCriteria.Body)
	ctx.Set("steps-to-reproduce", r.StepsToReproduce.Body)
	ctx.Set("checklist", r.Checklist.Body)
	// External System Contract Criteria (optional). `ticket-has-escc` is the
	// routing signal the contract/stub room reads to open regardless of the
	// file-change-derived `at-external-driver-port-changed` cascade;
	// `escc-systems` feeds validate-external-systems-registered; the verbatim
	// body flows to the contract-test writers via ${external-system-contract-
	// criteria}. parse-ticket stays dumb — it reads only presence + names.
	ctx.Set("ticket-has-escc", r.ExternalSystemContractCriteria.Found)
	ctx.Set("escc-systems", r.ExternalSystemContractCriteria.Systems)
	ctx.Set("external-system-contract-criteria", r.ExternalSystemContractCriteria.Body)
	return statemachine.Outcome{}
}

// issueFromContext builds a tracker.Issue from the conventional Context
// keys preResolveIssue writes (issue-num, issue-url, issue-title,
// issue-handle). issue-url is the addressable form every Tracker call
// site needs; callers that don't seed it get a clear error rather than
// a downstream parse failure.
func issueFromContext(ctx *statemachine.Context) (tracker.Issue, error) {
	url := ctx.GetString("issue-url")
	if url == "" {
		return tracker.Issue{}, fmt.Errorf("issue-url not in Context")
	}
	return tracker.Issue{
		ID:     ctx.GetString("issue-num"),
		Title:  ctx.GetString("issue-title"),
		URL:    url,
		Handle: ctx.GetString("issue-handle"),
	}, nil
}
