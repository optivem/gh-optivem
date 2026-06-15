// Package process holds the concrete ATDD/BPMN process definition that the
// generic engine loads via its LoadBytes contract. The engine itself embeds
// no process; this package binds process-flow.yaml to it.
package process

import (
	_ "embed"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

//go:embed process-flow.yaml
var DefaultYAML []byte

// Load parses the canonical embedded ATDD process-flow document.
func Load() (*statemachine.Engine, error) { return statemachine.LoadBytes(DefaultYAML) }
