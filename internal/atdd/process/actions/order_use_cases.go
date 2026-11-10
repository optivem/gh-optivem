package actions

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
	"github.com/optivem/gh-optivem/internal/spec"
)

// Use case ordering of the characterize flow (plan 20261110-1130, "Redesign:
// use case by use case"). order-use-cases reads the dependency diagram
// (spec.DependencyDiagramFile in the use cases dir, parsed by
// spec.ParseDependencies) and writes the list the use case loop works
// through. The order is deterministic: a use case after every use case it
// transitively depends on, ties by the lower UC number; a dependency cycle or
// an edge naming an undeclared use case is a hard error, as is a use case
// file whose id the diagram does not declare.
//
// State keys:
//
//   - reads proposed-use-case (pilot: the use case the human approved at the
//     diagram gate, the diagram writer's output), use-case-id and
//     use-case-order (reorder), pilot-use-cases (scaling);
//   - writes use-case-order: the comma-separated UC ids still to do, in
//     order, the form select-next-use-case's `use-cases` param takes
//     (`use-cases: ${use-case-order}`);
//   - writes pilot-use-cases (pilot): the whole pilot set in order, done ones
//     included, which scaling checks are all done;
//   - writes use-case-deferred (reorder, bool): true when the current use
//     case's narrative added a dependency on a use case not done yet. The new
//     use-case-order then starts with that dependency and has the current use
//     case behind it, and the loop must leave the current use case and select
//     the next one (its narrative commit stays). The cursor of the new list is
//     reset, so select-next-use-case pops from its start.
//
// A use case is done when at least one acceptance test is tagged with one of
// its UC-NNN.M criteria and its use case file holds no criteria any more (the
// last per-use-case step deletes them): re-derived from the files like
// select-next-use-case's per-use-case State, so a resumed run orders the same.
// Done use cases are left out of use-case-order in every mode.

const (
	// CtxKeyProposedUseCase is the use case the pilot is built around.
	CtxKeyProposedUseCase = "proposed-use-case"
	// CtxKeyUseCaseOrder is the list the use case loop works through.
	CtxKeyUseCaseOrder = "use-case-order"
	// CtxKeyPilotUseCases is the pilot set (pilot mode).
	CtxKeyPilotUseCases = "pilot-use-cases"
	// CtxKeyUseCaseDeferred is the reorder verdict.
	CtxKeyUseCaseDeferred = "use-case-deferred"
)

// Order modes: the `order-mode` param of order-use-cases, or the alias.
const (
	OrderModePilot   = "pilot"
	OrderModeScaling = "scaling"
	OrderModeReorder = "reorder"
)

// orderUseCases is order-use-cases: the mode comes from the `order-mode` param
// (a call-activity wrapper's params; service tasks carry none of their own).
// The aliases order-pilot-use-cases, order-scaling-use-cases and
// reorder-use-cases fix the mode for a bare service task.
func (a actions) orderUseCases(ctx *statemachine.Context) statemachine.Outcome {
	return a.orderUseCasesIn(ctx, ctx.Params["order-mode"])
}

func (a actions) orderPilotUseCases(ctx *statemachine.Context) statemachine.Outcome {
	return a.orderUseCasesIn(ctx, OrderModePilot)
}

func (a actions) orderScalingUseCases(ctx *statemachine.Context) statemachine.Outcome {
	return a.orderUseCasesIn(ctx, OrderModeScaling)
}

func (a actions) reorderUseCases(ctx *statemachine.Context) statemachine.Outcome {
	return a.orderUseCasesIn(ctx, OrderModeReorder)
}

func (a actions) orderUseCasesIn(ctx *statemachine.Context, mode string) statemachine.Outcome {
	fail := func(err error) statemachine.Outcome {
		return statemachine.Outcome{Err: fmt.Errorf("order-use-cases (%s): %w", mode, err)}
	}
	if mode != OrderModePilot && mode != OrderModeScaling && mode != OrderModeReorder {
		return fail(fmt.Errorf("unknown mode %q; want %s, %s or %s", mode, OrderModePilot, OrderModeScaling, OrderModeReorder))
	}
	deps, done, err := a.useCaseDependencies()
	if err != nil {
		return fail(err)
	}
	// A cycle is an error in every mode, also when it does not touch the
	// use cases this mode orders.
	if _, err := deps.Order(nil); err != nil {
		return fail(err)
	}
	declared := func(id string) error {
		if _, ok := deps.Titles[id]; !ok {
			return fmt.Errorf("use case %s is not in the dependency diagram %s", id, spec.DependencyDiagramFile)
		}
		return nil
	}
	notDone := func(ids []string) []string {
		var out []string
		for _, id := range ids {
			if !done[id] {
				out = append(out, id)
			}
		}
		return out
	}
	prereq := deps.Prerequisites()

	switch mode {
	case OrderModePilot:
		chosen := strings.TrimSpace(ctx.GetString(CtxKeyProposedUseCase))
		if chosen == "" {
			return fail(fmt.Errorf("%s is not in State; the use case diagram writer must have run", CtxKeyProposedUseCase))
		}
		if err := declared(chosen); err != nil {
			return fail(err)
		}
		set := []string{chosen}
		for id := range prereq[chosen] {
			set = append(set, id)
		}
		pilot, err := deps.Order(set)
		if err != nil {
			return fail(err)
		}
		order := notDone(pilot)
		ctx.Set(CtxKeyPilotUseCases, strings.Join(pilot, ", "))
		ctx.Set(CtxKeyUseCaseOrder, strings.Join(order, ", "))
		fmt.Fprintf(a.deps.Out.Phase, "Pilot use cases (%s and its dependencies): %s; %d to do.\n", chosen, strings.Join(pilot, ", "), len(order))

	case OrderModeScaling:
		raw, ok := ctx.Get(CtxKeyPilotUseCases).(string)
		if !ok {
			return fail(fmt.Errorf("%s is not in State; the pilot order must run first", CtxKeyPilotUseCases))
		}
		var open []string
		for _, id := range splitUseCaseIDs(raw) {
			if !done[id] {
				open = append(open, id)
			}
		}
		if len(open) > 0 {
			return fail(fmt.Errorf("pilot use case(s) %s are not done (no tagged acceptance test, or criteria still in the use case file); the pilot must finish first", strings.Join(open, ", ")))
		}
		order, err := deps.Order(notDone(deps.UseCases))
		if err != nil {
			return fail(err)
		}
		ctx.Set(CtxKeyUseCaseOrder, strings.Join(order, ", "))
		fmt.Fprintf(a.deps.Out.Phase, "Scaling use cases: %d to do (%s).\n", len(order), strings.Join(order, ", "))

	case OrderModeReorder:
		cur := strings.TrimSpace(ctx.GetString(CtxKeyUseCaseID))
		if cur == "" {
			return fail(fmt.Errorf("%s is not in State; a use case must have been selected first", CtxKeyUseCaseID))
		}
		if err := declared(cur); err != nil {
			return fail(err)
		}
		raw, ok := ctx.Get(CtxKeyUseCaseOrder).(string)
		if !ok {
			return fail(fmt.Errorf("%s is not in State; the pilot or scaling order must run first", CtxKeyUseCaseOrder))
		}
		var missing []string
		for id := range prereq[cur] {
			if !done[id] {
				missing = append(missing, id)
			}
		}
		if len(missing) == 0 {
			ctx.Set(CtxKeyUseCaseDeferred, false)
			fmt.Fprintf(a.deps.Out.Phase, "Use case %s: every dependency is done; order unchanged.\n", cur)
			return statemachine.Outcome{}
		}
		slices.SortFunc(missing, spec.CompareUseCaseIDs)
		for _, id := range splitUseCaseIDs(raw) {
			if err := declared(id); err != nil {
				return fail(err)
			}
		}
		order, err := deps.Order(append(notDone(append(splitUseCaseIDs(raw), cur)), missing...))
		if err != nil {
			return fail(err)
		}
		list := strings.Join(order, ", ")
		if err := resetUseCaseCursor(ctx, list); err != nil {
			return fail(err)
		}
		ctx.Set(CtxKeyUseCaseOrder, list)
		ctx.Set(CtxKeyUseCaseDeferred, true)
		fmt.Fprintf(a.deps.Out.Phase, "Use case %s depends on %s, not done yet; it goes back behind them: %s.\n", cur, strings.Join(missing, ", "), list)
	}
	return statemachine.Outcome{}
}

// resetUseCaseCursor drops select-next-use-case's cursors for the list (the
// key is "<layer-process>|<ids joined by commas>"), so a deferred order is
// worked through from its start even if the same list was used before.
func resetUseCaseCursor(ctx *statemachine.Context, list string) error {
	raw := ctx.GetString(CtxKeyUseCaseCursors)
	if raw == "" {
		return nil
	}
	cursors := map[string]int{}
	if err := json.Unmarshal([]byte(raw), &cursors); err != nil {
		return fmt.Errorf("decode %s: %w", CtxKeyUseCaseCursors, err)
	}
	suffix := "|" + strings.Join(splitUseCaseIDs(list), ",")
	for k := range cursors {
		if strings.HasSuffix(k, suffix) {
			delete(cursors, k)
		}
	}
	b, err := json.Marshal(cursors)
	if err != nil {
		return fmt.Errorf("encode %s: %w", CtxKeyUseCaseCursors, err)
	}
	ctx.Set(CtxKeyUseCaseCursors, string(b))
	return nil
}

// useCaseDependencies parses the dependency diagram in the use cases dir and
// returns it with the done use cases. A use case file whose id the diagram
// does not declare is an error: the diagram must list every use case.
func (a actions) useCaseDependencies() (spec.Dependencies, map[string]bool, error) {
	cfg := a.deps.Config
	if cfg == nil {
		return spec.Dependencies{}, nil, fmt.Errorf("no gh-optivem config wired (driver bug)")
	}
	ucs, dir, err := a.loadUseCases()
	if err != nil {
		return spec.Dependencies{}, nil, err
	}
	file := filepath.Join(dir, spec.DependencyDiagramFile)
	data, err := os.ReadFile(file)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return spec.Dependencies{}, nil, fmt.Errorf("dependency diagram %s not found; the use case diagram writer writes it", file)
		}
		return spec.Dependencies{}, nil, err
	}
	deps, err := spec.ParseDependencies(string(data))
	if err != nil {
		return spec.Dependencies{}, nil, fmt.Errorf("%s: %w", file, err)
	}
	criteria := map[string]int{}
	for _, uc := range ucs {
		if uc.ID == "" {
			continue
		}
		if _, ok := deps.Titles[uc.ID]; !ok {
			return spec.Dependencies{}, nil, fmt.Errorf("use case %s (%s) is not in the dependency diagram %s; declare it as a node", uc.ID, uc.File, file)
		}
		criteria[uc.ID] += len(uc.Criteria)
	}
	done := map[string]bool{}
	for _, id := range deps.UseCases {
		if criteria[id] > 0 {
			continue
		}
		names, err := a.useCaseTestNames(cfg, []string{id})
		if err != nil {
			return spec.Dependencies{}, nil, fmt.Errorf("use case %s: %w", id, err)
		}
		done[id] = len(names) > 0
	}
	return deps, done, nil
}
