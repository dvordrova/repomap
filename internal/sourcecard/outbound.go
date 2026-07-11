package sourcecard

import (
	"fmt"

	"github.com/dvordrova/repomap/internal/secretscan"
)

// ValidateForRemote fails closed when bounded source evidence contains an
// obvious credential marker. It deliberately does not redact exact evidence.
func ValidateForRemote(card Card) error {
	if err := card.Validate(); err != nil {
		return err
	}
	return ValidateLinesForRemote(card.Lines)
}

// ValidateLinesForRemote applies the same fail-closed credential check at the
// final model-provider boundary, where only source lines may be available.
func ValidateLinesForRemote(lines []Line) error {
	for _, line := range lines {
		if kind, found := secretscan.Detect(line.Text); found {
			return fmt.Errorf("sourcecard: %s detected at %s; refusing remote use", kind, line.EvidenceID)
		}
	}
	return nil
}
