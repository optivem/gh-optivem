package spec

import (
	"os"
	"strings"
	"testing"
)

func TestParseFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/UC-001-place-order.md")
	if err != nil {
		t.Fatal(err)
	}
	uc := Parse("UC-001-place-order.md", string(data))
	if uc.FrontErr != "" {
		t.Fatalf("unexpected front matter error: %s", uc.FrontErr)
	}
	if uc.ID != "UC-001" || uc.Title != "Place Order" {
		t.Errorf("got id=%q title=%q", uc.ID, uc.Title)
	}
	if len(uc.Criteria) != 14 {
		t.Fatalf("got %d criteria, want 14", len(uc.Criteria))
	}
	first, last := uc.Criteria[0], uc.Criteria[13]
	if first.ID != "UC-001.1" || first.Group != "Acceptance criteria" || !strings.HasPrefix(first.Text, "A valid order") {
		t.Errorf("unexpected first criterion: %+v", first)
	}
	if last.ID != "UC-001.14" || last.Text != "ERP unavailable message (narrative 5a)." {
		t.Errorf("unexpected last criterion: %+v", last)
	}
	if errs := Validate([]UseCase{uc}); len(errs) != 0 {
		t.Errorf("fixture should validate, got %v", errs)
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantID    string
		wantFront string
		wantCrit  []Criterion
	}{
		{
			name:    "flag and group",
			content: "---\nid: UC-002\ntitle: Cancel\n---\n## Extensions\n### 5a. ERP down\n- UC-002.1 [flag: erp-fallback]: Uses cached prices.\n- UC-002.2: Plain.\n",
			wantID:  "UC-002",
			wantCrit: []Criterion{
				{ID: "UC-002.1", Text: "Uses cached prices.", Group: "5a. ERP down", Flag: "erp-fallback"},
				{ID: "UC-002.2", Text: "Plain.", Group: "5a. ERP down"},
			},
		},
		{
			name:    "prose and missing colon ignored",
			content: "---\nid: UC-003\ntitle: T\n---\nsome prose UC-003.1: not a list\n- UC-003.2 no colon\n- UC-003.3: ok\n",
			wantID:  "UC-003",
			wantCrit: []Criterion{
				{ID: "UC-003.3", Text: "ok"},
			},
		},
		{
			name:    "crlf",
			content: "---\r\nid: UC-004\r\ntitle: T\r\n---\r\n- UC-004.1: ok\r\n",
			wantID:  "UC-004",
			wantCrit: []Criterion{
				{ID: "UC-004.1", Text: "ok"},
			},
		},
		{
			name: "no front matter", content: "# UC-001\n- UC-001.1: x\n", wantFront: "missing front matter",
			wantCrit: []Criterion{{ID: "UC-001.1", Text: "x", Group: "UC-001"}},
		},
		{name: "unclosed", content: "---\nid: UC-001\ntitle: T\n- UC-001.1: x\n", wantFront: "not closed",
			wantCrit: []Criterion{{ID: "UC-001.1", Text: "x"}}},
		{name: "missing id", content: "---\ntitle: T\n---\n", wantFront: "missing id"},
		{name: "missing title", content: "---\nid: UC-001\n---\n", wantFront: "missing title"},
		{name: "bad id", content: "---\nid: X-1\ntitle: T\n---\n", wantFront: "does not match"},
		{name: "bad line", content: "---\nid UC-001\n---\n", wantFront: "malformed front matter line"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := Parse("f.md", tt.content)
			if tt.wantFront == "" && uc.FrontErr != "" {
				t.Fatalf("unexpected front error %q", uc.FrontErr)
			}
			if tt.wantFront != "" && !strings.Contains(uc.FrontErr, tt.wantFront) {
				t.Fatalf("front error %q, want containing %q", uc.FrontErr, tt.wantFront)
			}
			if tt.wantID != "" && uc.ID != tt.wantID {
				t.Errorf("id %q, want %q", uc.ID, tt.wantID)
			}
			if len(uc.Criteria) != len(tt.wantCrit) {
				t.Fatalf("criteria %+v, want %+v", uc.Criteria, tt.wantCrit)
			}
			for i, c := range tt.wantCrit {
				if uc.Criteria[i] != c {
					t.Errorf("criterion %d = %+v, want %+v", i, uc.Criteria[i], c)
				}
			}
		})
	}
}

func TestValidate(t *testing.T) {
	mk := func(id string, crit ...string) UseCase {
		uc := UseCase{ID: id, Title: "T", File: id + ".md"}
		for _, c := range crit {
			uc.Criteria = append(uc.Criteria, Criterion{ID: c})
		}
		return uc
	}
	tests := []struct {
		name  string
		input []UseCase
		codes []string
	}{
		{"valid", []UseCase{mk("UC-001", "UC-001.1", "UC-001.2"), mk("UC-002", "UC-002.1")}, nil},
		{"duplicate within file", []UseCase{mk("UC-001", "UC-001.1", "UC-001.1")}, []string{"duplicate-id"}},
		{"duplicate across files", []UseCase{mk("UC-001", "UC-001.1"), mk("UC-001", "UC-001.1")}, []string{"duplicate-id"}},
		{"prefix mismatch", []UseCase{mk("UC-001", "UC-002.1")}, []string{"prefix-mismatch"}},
		{"front matter", []UseCase{{File: "x.md", FrontErr: "missing front matter"}}, []string{"front-matter"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.input)
			if len(errs) != len(tt.codes) {
				t.Fatalf("got %v, want codes %v", errs, tt.codes)
			}
			for i, c := range tt.codes {
				if errs[i].Code != c {
					t.Errorf("error %d code %q, want %q", i, errs[i].Code, c)
				}
			}
		})
	}
}

func TestLoadDir(t *testing.T) {
	ucs, err := LoadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}
	if len(ucs) != 1 || ucs[0].ID != "UC-001" {
		t.Fatalf("unexpected use cases: %+v", ucs)
	}
	if _, err := LoadDir("testdata/missing"); err == nil {
		t.Error("expected error for missing dir")
	}
}
