package main

import (
	"fmt"
	"regexp"

	"github.com/dvordrova/repomap/internal/debugdump"
)

const maxRejectedModelResponseBytes = 2 << 20

var modelResponseStagePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// writeRejectedModelResponse is the single bounded persistence boundary for
// rejected optional-stage provider text. It is called only for explicit
// --dump-llm diagnostics and always persists the mandatory-redacted bytes.
func writeRejectedModelResponse(runDir, stage string, attempt int, raw []byte) error {
	if runDir == "" || !modelResponseStagePattern.MatchString(stage) ||
		attempt < 1 || attempt > 4096 || len(raw) == 0 ||
		len(raw) > maxRejectedModelResponseBytes {
		return fmt.Errorf("model response debug: invalid bounded artifact")
	}
	name := fmt.Sprintf("%s-rejected-%03d.redacted.json", stage, attempt)
	return debugdump.WriteRedactedRootArtifact(runDir, "model_responses", name, raw)
}
