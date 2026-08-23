package activityentrypoint

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"strings"
)

//go:embed prompts/classifier.md
var classifierPromptFile string

var (
	classifierPrompt = strings.TrimSpace(classifierPromptFile)
	promptVersion    = "activity-entrypoint-prompt-" + shortPromptSHA(classifierPrompt)
)

func shortPromptSHA(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])[:12]
}
