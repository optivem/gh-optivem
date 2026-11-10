package actions

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

const useCaseWithESCC = `---
id: UC-002
title: Place Order
---

# Place Order

## Narrative

A customer places an order.

## Business Rules

- The total includes tax.

## Acceptance Criteria

- UC-002.1: An order is placed.

## External System Contract Criteria

External System: ERP
  Shared (stub + real):
    Given products Apple (1.00)
    Then ERP has products Apple (1.00)
`

const useCaseWithoutESCC = `---
id: UC-001
title: Browse Products
---

## Narrative

A customer browses products.
`

func useCaseRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range map[string]string{
		"UC-001-browse-products/acceptance-criteria.md": useCaseWithoutESCC,
		"UC-002-place-order/acceptance-criteria.md":     useCaseWithESCC,
	} {
		p := filepath.Join(root, "docs", "use-cases", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func layerCtx(useCases, layer string) *statemachine.Context {
	ctx := statemachine.NewContext()
	ctx.Params["use-cases"] = useCases
	ctx.Params["layer-process"] = layer
	return ctx
}

func TestSelectNextUseCase_PopsInOrderThenNoneLeft(t *testing.T) {
	a := newActions(Deps{RepoPath: useCaseRepo(t), Stdout: &bytes.Buffer{}})
	ctx := layerCtx("UC-002, UC-001, UC-003", "characterize-at-layer")

	for _, want := range []string{"UC-002", "UC-001", "UC-003"} {
		if out := a.selectNextUseCase(ctx); out.Err != nil {
			t.Fatal(out.Err)
		}
		if ctx.Get(CtxKeyUseCaseLeft) != true || ctx.GetString(CtxKeyUseCaseID) != want {
			t.Fatalf("got left=%v id=%q, want %s", ctx.Get(CtxKeyUseCaseLeft), ctx.GetString(CtxKeyUseCaseID), want)
		}
	}
	if out := a.selectNextUseCase(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	if ctx.Get(CtxKeyUseCaseLeft) != false {
		t.Fatal("want use-case-left=false after the last use case")
	}
	if ctx.GetString(CtxKeyUseCaseID) != "UC-003" {
		t.Errorf("the last use case must stay current for a re-run, got %q", ctx.GetString(CtxKeyUseCaseID))
	}
}

func TestSelectNextUseCase_CursorPerCallSite(t *testing.T) {
	a := newActions(Deps{RepoPath: useCaseRepo(t), Stdout: &bytes.Buffer{}})
	ctx := layerCtx("UC-001", "characterize-at-layer")
	a.selectNextUseCase(ctx)
	a.selectNextUseCase(ctx)
	if ctx.Get(CtxKeyUseCaseLeft) != false {
		t.Fatal("AT layer core pass should be exhausted")
	}
	// The next layer (same use case list) starts from the beginning.
	ctx.Params["layer-process"] = "characterize-dsl-layer"
	if out := a.selectNextUseCase(ctx); out.Err != nil || ctx.Get(CtxKeyUseCaseLeft) != true || ctx.GetString(CtxKeyUseCaseID) != "UC-001" {
		t.Fatalf("DSL layer: err=%v left=%v id=%q", out.Err, ctx.Get(CtxKeyUseCaseLeft), ctx.GetString(CtxKeyUseCaseID))
	}
}

func TestSelectNextUseCase_StampsESCC(t *testing.T) {
	a := newActions(Deps{RepoPath: useCaseRepo(t), Stdout: &bytes.Buffer{}})
	ctx := layerCtx("UC-002,UC-001", "characterize-adapter-layer")

	if out := a.selectNextUseCase(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	if ctx.Get("ticket-has-escc") != true {
		t.Fatalf("ticket-has-escc = %v, want true", ctx.Get("ticket-has-escc"))
	}
	if systems, _ := ctx.Get("escc-systems").([]string); len(systems) != 1 || systems[0] != "ERP" {
		t.Errorf("escc-systems = %v, want [ERP]", ctx.Get("escc-systems"))
	}
	if body := ctx.GetString("external-system-contract-criteria"); body == "" || body[:len("External System: ERP")] != "External System: ERP" {
		t.Errorf("external-system-contract-criteria = %q", body)
	}

	if out := a.selectNextUseCase(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	if ctx.Get("ticket-has-escc") != false || ctx.GetString("external-system-contract-criteria") != "" || ctx.Get("escc-systems") != nil && len(ctx.Get("escc-systems").([]string)) != 0 {
		t.Fatalf("UC-001 has no ESCC; stale keys: has=%v systems=%v", ctx.Get("ticket-has-escc"), ctx.Get("escc-systems"))
	}
}

func TestSelectNextUseCase_NoCriteriaFileYetHasNoESCC(t *testing.T) {
	a := newActions(Deps{RepoPath: useCaseRepo(t), Stdout: &bytes.Buffer{}})
	ctx := layerCtx("UC-009", "write-use-case-criteria")
	if out := a.selectNextUseCase(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	if ctx.GetString(CtxKeyUseCaseID) != "UC-009" || ctx.Get("ticket-has-escc") != false {
		t.Fatalf("id=%q has-escc=%v", ctx.GetString(CtxKeyUseCaseID), ctx.Get("ticket-has-escc"))
	}
}

func TestSelectNextUseCase_SameUseCaseRestamps(t *testing.T) {
	root := useCaseRepo(t)
	a := newActions(Deps{RepoPath: root, Stdout: &bytes.Buffer{}})
	ctx := layerCtx("UC-001,UC-002", "characterize-dsl-layer")
	if out := a.selectNextUseCase(ctx); out.Err != nil || ctx.Get("ticket-has-escc") != false {
		t.Fatalf("err=%v has-escc=%v", out.Err, ctx.Get("ticket-has-escc"))
	}
	// Late discovery: the criteria writer adds an ESCC section to UC-001.
	p := filepath.Join(root, "docs", "use-cases", "UC-001-browse-products", "acceptance-criteria.md")
	escc := "\n## External System Contract Criteria\n\nExternal System: Clock\n  Shared (stub + real):\n    Given the time is 10:00\n    Then Clock returns 10:00\n"
	if err := os.WriteFile(p, []byte(useCaseWithoutESCC+escc), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, restamp := range []func(*statemachine.Context) statemachine.Outcome{
		func(c *statemachine.Context) statemachine.Outcome {
			c.Params["same-use-case"] = "true"
			defer delete(c.Params, "same-use-case")
			return a.selectNextUseCase(c)
		},
		a.restampUseCase,
	} {
		if out := restamp(ctx); out.Err != nil {
			t.Fatal(out.Err)
		}
		if ctx.GetString(CtxKeyUseCaseID) != "UC-001" || ctx.Get("ticket-has-escc") != true || ctx.Get(CtxKeyUseCaseLeft) != true {
			t.Fatalf("restamp: id=%q has-escc=%v left=%v", ctx.GetString(CtxKeyUseCaseID), ctx.Get("ticket-has-escc"), ctx.Get(CtxKeyUseCaseLeft))
		}
	}
	// The cursor did not move: the next pop is UC-002.
	if out := a.selectNextUseCase(ctx); out.Err != nil || ctx.GetString(CtxKeyUseCaseID) != "UC-002" {
		t.Fatalf("err=%v id=%q, want UC-002", out.Err, ctx.GetString(CtxKeyUseCaseID))
	}
}

func TestSelectNextUseCase_Errors(t *testing.T) {
	a := newActions(Deps{RepoPath: useCaseRepo(t), Stdout: &bytes.Buffer{}})
	if out := a.selectNextUseCase(layerCtx("", "x")); out.Err == nil {
		t.Error("empty use-cases param must error")
	}
	if out := a.restampUseCase(statemachine.NewContext()); out.Err == nil {
		t.Error("restamp without a current use case must error")
	}
}
