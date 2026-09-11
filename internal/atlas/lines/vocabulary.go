package lines

import (
	_ "embed"
	"strings"
)

//go:embed prompts/evidence-vocabulary.md
var evidenceVocabulary string

// EvidenceVocabulary defines every enumerated value a symbol or operation
// row renders: invocation and resolution forms and their defaults, origin
// node kinds, dispatch witness kinds, extractors and their labels, the
// binding table shape. It is appended to those tables' prompts so that no
// internal enumeration reaches the model undefined; the contract tests
// render the Go fixtures' rows and check each value against it.
func EvidenceVocabulary() string { return evidenceVocabulary }

func withVocabulary(prompt string) string {
	return strings.TrimRight(prompt, "\n") + "\n\n" + strings.TrimRight(evidenceVocabulary, "\n") + "\n"
}
