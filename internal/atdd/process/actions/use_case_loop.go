package actions

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// Use case loop of the characterize (legacy baseline) flow (plan
// 20261110-1130). characterize-use-cases runs characterize-use-case over the
// use cases in use-case-order: select-next-use-case pops the next one,
// GATE_USE_CASE_LEFT routes on use-case-left, and the use case runs through
// every step. Per-use-case State is re-derived from the files and the branch
// history each time (stampUseCase, use_case_state.go), so a use case never
// sees the previous one's values and a resumed run picks up the same ones.

const (
	// CtxKeyUseCaseID is the use case the current layer pass works on.
	CtxKeyUseCaseID = "use-case-id"
	// CtxKeyUseCaseLeft is the GATE_USE_CASE_LEFT gateway signal.
	CtxKeyUseCaseLeft = "use-case-left"
	// CtxKeyUseCaseCursors is the JSON object {"<layer-process>|<use-cases>":
	// <index of the next use case>}: one cursor per list, so the pilot and the
	// scaling orders (and a re-order after a deferral) advance independently.
	CtxKeyUseCaseCursors = "use-case-cursors"
)

// selectNextUseCase is the SELECT_NEXT_USE_CASE service task. It reads the
// call's `use-cases` (comma- or space-separated UC ids) and `layer-process`
// params, pops the list's next use case and stamps its State; with none left
// (or an empty list: no use case left to do) it stamps use-case-left=false
// and keeps the last use case's State. With the `same-use-case: "true"` param
// it re-stamps the current use case without popping.
func (a actions) selectNextUseCase(ctx *statemachine.Context) statemachine.Outcome {
	if ctx.Params["same-use-case"] == "true" {
		return a.restampUseCase(ctx)
	}
	ids := splitUseCaseIDs(ctx.Params["use-cases"])
	if len(ids) == 0 {
		// Nothing to do (e.g. the pilot covered every use case, so scaling
		// has none left). The State is left as it is.
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
// RESTAMP nodes (characterize-test-layers, the DSL layer's late discovery) use
// the alias instead of a wrapper process.
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
