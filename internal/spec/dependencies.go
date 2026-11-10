package spec

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Dependencies is the parsed use case dependency diagram (DependencyDiagramFile,
// plan 20261110-1130): the declared use cases and the "B needs data A
// creates" edges between them.
type Dependencies struct {
	// UseCases are the declared node ids, sorted by UC number.
	UseCases []string
	// Titles maps a node id to its label.
	Titles map[string]string
	// Edges are the arrows in file order.
	Edges []DependencyEdge
}

// DependencyEdge is one `From -->|Reason| To` arrow: To needs data From creates.
type DependencyEdge struct {
	From, To, Reason string
}

// Dependency diagram format (the legacy-use-case-diagram-writer prompt documents
// the same): the file holds exactly one fenced ```mermaid block whose first
// line is `flowchart <dir>`; inside it every non-blank line is one of
//
//	UC-001["Register Product"]                 a use case (node); every use case is declared
//	UC-001 -->|"orders need a product"| UC-002 an edge with its one-line reason
//	%% comment
//
// Prose outside the block is ignored. Anything else inside it is an error, so
// a malformed diagram fails loud instead of silently losing an edge.
var (
	depFenceOpen = regexp.MustCompile("^\\s*```+\\s*mermaid\\s*$")
	depFlowchart = regexp.MustCompile(`^flowchart(\s+(TD|TB|BT|LR|RL))?\s*;?$`)
	depNode      = regexp.MustCompile(`^(UC-\d+)\["?(.*?)"?\]\s*;?$`)
	depEdge      = regexp.MustCompile(`^(UC-\d+)\s*-->\s*\|"?(.*?)"?\|\s*(UC-\d+)\s*;?$`)
)

// ParseDependencies parses the dependency diagram. It rejects a missing or
// repeated mermaid block, an unrecognised line, a node declared twice, an
// edge without a reason, and an edge naming an undeclared use case.
func ParseDependencies(content string) (Dependencies, error) {
	d := Dependencies{Titles: map[string]string{}}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	blocks := 0
	inBlock, sawHeader := false, false
	for i, raw := range lines {
		n := i + 1
		if !inBlock {
			if depFenceOpen.MatchString(raw) {
				blocks++
				if blocks > 1 {
					return Dependencies{}, fmt.Errorf("line %d: more than one mermaid block; keep the dependency diagram in one", n)
				}
				inBlock, sawHeader = true, false
			}
			continue
		}
		t := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(t, "```"):
			inBlock = false
			continue
		case t == "" || strings.HasPrefix(t, "%%"):
			continue
		case !sawHeader:
			if !depFlowchart.MatchString(t) {
				return Dependencies{}, fmt.Errorf("line %d: the mermaid block must start with `flowchart <direction>`, got %q", n, t)
			}
			sawHeader = true
			continue
		}
		if m := depEdge.FindStringSubmatch(t); m != nil {
			reason := strings.TrimSpace(m[2])
			if reason == "" {
				return Dependencies{}, fmt.Errorf("line %d: edge %s --> %s has no reason; write it as %s -->|\"<reason>\"| %s", n, m[1], m[3], m[1], m[3])
			}
			d.Edges = append(d.Edges, DependencyEdge{From: m[1], To: m[3], Reason: reason})
			continue
		}
		if strings.Contains(t, "-->") {
			return Dependencies{}, fmt.Errorf("line %d: write an edge as UC-A -->|\"<reason>\"| UC-B (plain ids, a one-line reason), got %q", n, t)
		}
		if m := depNode.FindStringSubmatch(t); m != nil {
			if _, dup := d.Titles[m[1]]; dup {
				return Dependencies{}, fmt.Errorf("line %d: use case %s is declared twice", n, m[1])
			}
			d.Titles[m[1]] = strings.TrimSpace(m[2])
			d.UseCases = append(d.UseCases, m[1])
			continue
		}
		return Dependencies{}, fmt.Errorf("line %d: not a use case node (UC-NNN[\"Title\"]) or a dependency edge (UC-A -->|\"reason\"| UC-B): %q", n, t)
	}
	if blocks == 0 {
		return Dependencies{}, fmt.Errorf("no mermaid block")
	}
	if inBlock {
		return Dependencies{}, fmt.Errorf("the mermaid block is not closed")
	}
	for _, e := range d.Edges {
		for _, id := range []string{e.From, e.To} {
			if _, ok := d.Titles[id]; !ok {
				return Dependencies{}, fmt.Errorf("edge %s --> %s names unknown use case %s; declare it as a node (%s[\"Title\"])", e.From, e.To, id, id)
			}
		}
	}
	slices.SortFunc(d.UseCases, CompareUseCaseIDs)
	return d, nil
}

// CompareUseCaseIDs orders UC-NNN ids by number (UC-9 before UC-10), then as
// strings.
func CompareUseCaseIDs(a, b string) int {
	na, ea := strconv.Atoi(strings.TrimPrefix(a, "UC-"))
	nb, eb := strconv.Atoi(strings.TrimPrefix(b, "UC-"))
	if ea == nil && eb == nil && na != nb {
		return na - nb
	}
	return strings.Compare(a, b)
}

// Prerequisites returns, per use case, the use cases it transitively depends
// on (its edges' From ids and theirs). A use case on a cycle includes itself.
func (d Dependencies) Prerequisites() map[string]map[string]bool {
	direct := map[string][]string{}
	for _, e := range d.Edges {
		direct[e.To] = append(direct[e.To], e.From)
	}
	out := map[string]map[string]bool{}
	for _, id := range d.UseCases {
		seen := map[string]bool{}
		stack := slices.Clone(direct[id])
		for len(stack) > 0 {
			x := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if seen[x] {
				continue
			}
			seen[x] = true
			stack = append(stack, direct[x]...)
		}
		out[id] = seen
	}
	return out
}

// Cycle returns one dependency cycle as ids with the first repeated at the
// end (UC-001, UC-002, UC-001), or nil when the diagram has none.
func (d Dependencies) Cycle() []string {
	next := map[string][]string{}
	for _, e := range d.Edges {
		next[e.From] = append(next[e.From], e.To)
	}
	for k := range next {
		slices.SortFunc(next[k], CompareUseCaseIDs)
	}
	const (
		unvisited = iota
		onStack
		finished
	)
	state := map[string]int{}
	var path []string
	var visit func(string) []string
	visit = func(id string) []string {
		state[id] = onStack
		path = append(path, id)
		for _, to := range next[id] {
			switch state[to] {
			case onStack:
				i := slices.Index(path, to)
				return append(slices.Clone(path[i:]), to)
			case unvisited:
				if c := visit(to); c != nil {
					return c
				}
			}
		}
		path = path[:len(path)-1]
		state[id] = finished
		return nil
	}
	for _, id := range d.UseCases {
		if state[id] == unvisited {
			if c := visit(id); c != nil {
				return c
			}
		}
	}
	return nil
}

// Order returns ids in dependency order: a use case after every use case of
// ids it transitively depends on (also through use cases outside ids), ties
// by the lower UC number. It returns an error naming the cycle when the
// diagram has one.
func (d Dependencies) Order(ids []string) ([]string, error) {
	if c := d.Cycle(); c != nil {
		return nil, fmt.Errorf("use case dependency cycle: %s", strings.Join(c, " --> "))
	}
	prereq := d.Prerequisites()
	left := slices.Clone(ids)
	slices.SortFunc(left, CompareUseCaseIDs)
	left = slices.Compact(left)
	var out []string
	for len(left) > 0 {
		// The lowest id none of whose prerequisites is still left. One
		// always exists: the graph is acyclic.
		pick := -1
		for i, id := range left {
			ready := true
			for _, other := range left {
				if other != id && prereq[id][other] {
					ready = false
					break
				}
			}
			if ready {
				pick = i
				break
			}
		}
		out = append(out, left[pick])
		left = slices.Delete(left, pick, pick+1)
	}
	return out, nil
}
