package readmetargetscout

import (
	_ "embed"
	"strings"
)

//go:embed prompts/system.md
var promptSystemFile string

//go:embed prompts/user.md
var promptUserShapeFile string

//go:embed prompts/response-example.json
var responseExample string

var (
	promptSystem    = strings.TrimSuffix(promptSystemFile, "\n")
	promptUserShape = strings.TrimSuffix(promptUserShapeFile, "\n")
)
