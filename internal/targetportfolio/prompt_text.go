package targetportfolio

import (
	_ "embed"
	"strings"
)

//go:embed prompts/system.md
var promptSystemFile string

//go:embed prompts/user.md
var promptUserShapeFile string

//go:embed prompts/default_system.md
var defaultPromptSystemFile string

//go:embed prompts/default_user.md
var defaultPromptUserShapeFile string

var (
	promptSystem           = strings.TrimSuffix(promptSystemFile, "\n")
	promptUserShape        = strings.TrimSuffix(promptUserShapeFile, "\n")
	defaultPromptSystem    = strings.TrimSuffix(defaultPromptSystemFile, "\n")
	defaultPromptUserShape = strings.TrimSuffix(defaultPromptUserShapeFile, "\n")
)
