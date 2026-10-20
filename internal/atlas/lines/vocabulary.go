package lines

import (
	_ "embed"
	"strings"
)

//go:embed prompts/evidence-vocabulary.md
var evidenceVocabulary string

func withVocabulary(prompt string) string {
	return strings.TrimRight(prompt, "\n") + "\n\n" + strings.TrimRight(evidenceVocabulary, "\n") + "\n"
}
