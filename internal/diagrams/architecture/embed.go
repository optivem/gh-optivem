// Embed binds the canonical architecture document into the architecture
// package binary. The YAML stops being a consumer-repo file or a test
// fixture; gh-optivem owns it end-to-end and both production callers
// and tests load via LoadDefault.
package architecture

// Blank import: registers the //go:embed directive support so the
// directive below can bind architecture.yaml into DefaultYAML at compile
// time. The package exposes no identifiers we call directly.
import _ "embed"

//go:embed architecture.yaml
var DefaultYAML []byte

// LoadDefault loads the canonical embedded architecture document.
// Equivalent to Parse(DefaultYAML).
func LoadDefault() (*Document, error) {
	return Parse(DefaultYAML)
}
