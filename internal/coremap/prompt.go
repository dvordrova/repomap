package coremap

import (
	_ "embed"
	"strings"
)

//go:embed prompts/baseline.md
var baselinePromptFile string

//go:embed prompts/refined.md
var refinedPromptFile string

//go:embed prompts/reduce.md
var reducePromptFile string

//go:embed prompts/group.md
var groupPromptFile string

var (
	baselinePrompt = strings.TrimSpace(baselinePromptFile)
	refinedPrompt  = strings.TrimSpace(refinedPromptFile)
	reducePrompt   = strings.TrimSpace(reducePromptFile)
	groupPrompt    = strings.TrimSpace(groupPromptFile)
)
