package actions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/intake"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// Use case loop of the characterize (legacy baseline) flow (plan
// 20261110-1130, section 2b). characterize-layer runs one layer over a list of
// use cases: select-next-use-case pops the next one, GW_MORE routes on
// use-case-left, and the layer runs for the popped use case. Per-use-case
// State is re-derived from the files each time, so a resumed run picks up the
// same values: the use case id, and the External System Contract Criteria
// keys parse-ticket would stamp, read from the use case's criteria file.

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
// pops the call site's next use case and stamps its State; with none left it
// stamps use-case-left=false and keeps the last use case's State (a declined
// core layer re-runs it). With the `same-use-case: "true"` param it re-stamps
// the current use case without popping (late external discovery in the DSL
// layer, after the criteria gained an ESCC section).
func (a actions) selectNextUseCase(ctx *statemachine.Context) statemachine.Outcome {
	if ctx.Params["same-use-case"] == "true" {
		return a.restampUseCase(ctx)
	}
	ids := splitUseCaseIDs(ctx.Params["use-cases"])
	if len(ids) == 0 {
		return statemachine.Outcome{Err: fmt.Errorf("select-next-use-case: the use-cases param is empty; characterize-layer must be called with at least one use case")}
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

// stampUseCase sets use-case-id and the ESCC keys from the use case's
// criteria file. A use case with no criteria file yet (the criteria layer
// writes it after this runs) has no ESCC. The file is not a ticket body
// (front matter, narrative and business-rule headings), so only its External
// System Contract Criteria section goes through intake's parser, which
// validates it and extracts the system names exactly as for a ticket.
func (a actions) stampUseCase(ctx *statemachine.Context, id string) error {
	ucs, dir, err := a.loadUseCases()
	if err != nil {
		return err
	}
	var body string
	for _, uc := range ucs {
		if uc.ID != id {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(uc.File)))
		if err != nil {
			return err
		}
		body = strings.ReplaceAll(string(data), "\r\n", "\n")
		break
	}
	escc := intake.ExtractSection(body, intake.SectionExternalSystemContractCriteria).Body
	r, err := intake.ParseSections(map[string]string{intake.SectionExternalSystemContractCriteria: escc})
	if err != nil {
		return fmt.Errorf("use case %s: %w", id, err)
	}
	ctx.Set(CtxKeyUseCaseID, id)
	stampESCC(ctx, r.ExternalSystemContractCriteria)
	return nil
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
