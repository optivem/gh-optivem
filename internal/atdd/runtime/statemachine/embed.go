// Embed binds the canonical process-flow document into the statemachine
// package binary. The YAML stops being a consumer-repo file or a test
// fixture; gh-optivem owns it end-to-end and both production callers and
// tests load via LoadDefault.
package statemachine

import _ "embed"

//go:embed process-flow.yaml
var DefaultYAML []byte

// LoadDefault loads the canonical embedded process-flow document.
// Equivalent to LoadBytes(DefaultYAML).
func LoadDefault() (*Engine, error) {
	return LoadBytes(DefaultYAML)
}
