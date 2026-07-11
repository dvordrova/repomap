package sourcecard

import (
	"fmt"
	"regexp"
)

var credentialPatterns = []struct {
	name    string
	pattern *regexp.Regexp
}{
	{name: "private key", pattern: regexp.MustCompile(`(?i)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`)},
	{name: "bearer credential", pattern: regexp.MustCompile(`(?i)\bBearer[ \t]+[A-Za-z0-9._~+/=-]{12,}`)},
	{name: "secret key", pattern: regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}\b`)},
	{name: "github token", pattern: regexp.MustCompile(`\b(?:ghp_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})\b`)},
	{name: "aws access key", pattern: regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
	{name: "credential assignment", pattern: regexp.MustCompile(`(?i)\b(?:api[_-]?key|client[_-]?secret|token|password|passwd|secret|private[_-]?key|access[_-]?key|refresh[_-]?token)\b\s*(?::=|=)\s*["` + "`" + `][^"` + "`" + `]{8,}`)},
}

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
		for _, candidate := range credentialPatterns {
			if candidate.pattern.MatchString(line.Text) {
				return fmt.Errorf("sourcecard: %s detected at %s; refusing remote use", candidate.name, line.EvidenceID)
			}
		}
	}
	return nil
}
