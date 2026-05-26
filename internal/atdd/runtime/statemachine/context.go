package statemachine

import "fmt"

// Context is the live state of one pipeline run. It carries:
//
//   - State: a key/value map read by predicate evaluation. Initial values
//     (e.g. mode=board, ticket_type=story) are seeded by the caller before
//     Run; gateway outcomes are recorded under the gateway's binding name as
//     the run progresses, so later predicates can read upstream results.
//
//   - Params: parameter substitutions for the current call-activity scope.
//     The structural_cycle flow uses ${change_type}, ${agent} which are
//     resolved against this map at dispatch time (see ExpandParams).
//
// Context is mutable — service tasks and gateways can write to State, and
// call-activity dispatch swaps Params on entry and restores it on return.
type Context struct {
	State  map[string]any
	Params map[string]string
}

// NewContext constructs an empty Context with both maps initialised.
func NewContext() *Context {
	return &Context{
		State:  map[string]any{},
		Params: map[string]string{},
	}
}

// Set records a value in the state map under the given key. Used by gateway
// node bodies to record their decision so downstream `when:` clauses can read
// it back.
func (c *Context) Set(key string, value any) {
	c.State[key] = value
}

// Get returns the state value for key (any type). Returns nil if unset.
func (c *Context) Get(key string) any {
	return c.State[key]
}

// GetString returns the state value for key coerced to string (best-effort:
// strings pass through; bools become "true"/"false"; everything else is
// rendered via fmt). Predicate evaluation uses this for `==` and `in`
// comparisons.
func (c *Context) GetString(key string) string {
	v, ok := c.State[key]
	if !ok {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprint(v)
	}
}
