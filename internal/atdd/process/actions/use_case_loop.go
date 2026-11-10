package actions

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// Use case loop of the characterize (legacy baseline) flow (plan
// 20261110-1130, section 2b). characterize-layer runs one layer over a list of
// use cases: select-next-use-case pops the next one, GW_MORE routes on
// use-case-left, and the layer runs for the popped use case. Per-use-case
// State is re-derived from the files and the branch history each time
// (stampUseCase, use_case_state.go), so a bulk layer never sees the previous
// use case's values and a resumed run picks up the same ones.

const (
	// CtxKeyUseCaseID is the use case the current layer pass works on.
	CtxKeyUseCaseID = "use-case-id"
	// CtxKeyUseCaseLeft is the GW_MORE gateway signal.
	CtxKeyUseCaseLeft = "use-case-left"
	// CtxKeyUseCaseCursors is the JSON object {"<layer-process>|<use-cases>":
	// <index of the next use case>}: one cursor per characterize-layer call
	// site, so the core and bulk passes of each layer advance independently.
	CtxKeyUseCaseCursors = "use-case-cursors"
)

// selectNextUseCase is the NEXT_UC service task. It reads the call's
// `use-cases` (comma- or space-separated UC ids) and `layer-process` params,
// pops the call site's next use case and stamps its State; with none left (or
// an empty list: a system with only the core use case) it stamps
// use-case-left=false and keeps the last use case's State (a declined core
// layer re-runs it). With the `same-use-case: "true"` param it re-stamps
// the current use case without popping (late external discovery in the DSL
// layer, after the criteria gained an ESCC section).
func (a actions) selectNextUseCase(ctx *statemachine.Context) statemachine.Outcome {
	if ctx.Params["same-use-case"] == "true" {
		return a.restampUseCase(ctx)
	}
	ids := splitUseCaseIDs(ctx.Params["use-cases"])
	if len(ids) == 0 {
		// A system with only the core use case has no other use cases: the
		// bulk passes have nothing to do. The State is left as it is.
		ctx.Set(CtxKeyUseCaseLeft, false)
		fmt.Fprintf(a.deps.Out.Phase, "No use cases for %s; skipping.\n", ctx.Params["layer-process"])
		return statemachine.Outcome{}
	}
	cursors := map[string]int{}
	if raw := ctx.GetString(CtxKeyUseCaseCursors); raw != "" {
		if err := json.Unmarshal([]byte(raw), &cursors); err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("select-next-use-case: decode %s: %w", CtxKeyUseCaseCursors, err)}
		}
	}
	site := ctx.Params["layer-process"] + "|" + strings.Join(ids, ",")
	next := cursors[site]
	if next >= len(ids) {
		ctx.Set(CtxKeyUseCaseLeft, false)
		return statemachine.Outcome{}
	}
	if err := a.stampUseCase(ctx, ids[next]); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("select-next-use-case: %w", err)}
	}
	cursors[site] = next + 1
	raw, err := json.Marshal(cursors)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("select-next-use-case: encode %s: %w", CtxKeyUseCaseCursors, err)}
	}
	ctx.Set(CtxKeyUseCaseCursors, string(raw))
	ctx.Set(CtxKeyUseCaseLeft, true)
	fmt.Fprintf(a.deps.Out.Phase, "Use case %s (%d of %d).\n", ids[next], next+1, len(ids))
	return statemachine.Outcome{}
}

// restampUseCase is selectNextUseCase's same-use-case mode, also registered as
// restamp-current-use-case: service tasks carry no params of their own, so the
// DSL layer's RESTAMP node can use the alias instead of a wrapper process.
func (a actions) restampUseCase(ctx *statemachine.Context) statemachine.Outcome {
	id := ctx.GetString(CtxKeyUseCaseID)
	if id == "" {
		return statemachine.Outcome{Err: fmt.Errorf("select-next-use-case: same-use-case requested but %s is not in State; a use case must have been selected first", CtxKeyUseCaseID)}
	}
	if err := a.stampUseCase(ctx, id); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("select-next-use-case: %w", err)}
	}
	ctx.Set(CtxKeyUseCaseLeft, true)
	fmt.Fprintf(a.deps.Out.Phase, "Re-stamped use case %s.\n", id)
	return statemachine.Outcome{}
}

// splitUseCaseIDs splits a comma- or space-separated list (optionally in
// brackets), dropping empties and duplicates.
func splitUseCaseIDs(raw string) []string {
	raw = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(raw), "["), "]")
	var out []string
	seen := map[string]bool{}
	for _, id := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || unicode.IsSpace(r) }) {
		id = strings.Trim(id, `"'`)
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// CtxKeyCriterionIDs is the legacy-use-case-writer's `criterion-ids` output:
// the UC-NNN.M ids of the criteria it wrote this dispatch.
const CtxKeyCriterionIDs = "criterion-ids"

// recordUseCaseCriteria runs after the legacy-use-case-writer in
// write-use-case-criteria (plan 20261110-1130). It appends the criterion ids
// the writer reported to merged-criteria, the list apply-ac-policy deletes at
// the end of characterize, keeping the order and dropping duplicates (the
// late-discovery route re-runs the writer on the same use case). It then
// clears criterion-ids: the output is per dispatch and optional, so a stale
// list must not be appended again by a later dispatch that reported none.
func (a actions) recordUseCaseCriteria(ctx *statemachine.Context) statemachine.Outcome {
	var ids []string
	seen := map[string]bool{}
	add := func(raw string) {
		for _, id := range splitUseCaseIDs(raw) {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	add(ctx.GetString(CtxKeyMergedCriteria))
	before := len(ids)
	add(ctx.GetString(CtxKeyCriterionIDs))
	ctx.Set(CtxKeyMergedCriteria, strings.Join(ids, ", "))
	ctx.Unset(CtxKeyCriterionIDs)
	fmt.Fprintf(a.deps.Out.Phase, "Recorded %d new criterion id(s) for deletion at the end (%d in total).\n", len(ids)-before, len(ids))
	return statemachine.Outcome{}
}
