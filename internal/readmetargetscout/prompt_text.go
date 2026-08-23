package readmetargetscout

import (
	_ "embed"
	"strings"
)

//go:embed prompts/system.md
var promptSystemFile string

//go:embed prompts/user.md
var promptUserShapeFile string

var (
	promptSystem    = strings.TrimSuffix(promptSystemFile, "\n")
	promptUserShape = strings.TrimSuffix(promptUserShapeFile, "\n")
)
