// Package secretscan detects obvious credentials before repository content is
// sent to a model provider. It fails closed; callers decide how to identify the
// affected artifact without copying the secret into an error.
package secretscan

import (
	"regexp"
	"strings"
	"sync/atomic"
)

var patterns = []struct {
	kind    string
	pattern *regexp.Regexp
}{
	{kind: "private key", pattern: regexp.MustCompile(`(?i)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`)},
	{kind: "bearer credential", pattern: regexp.MustCompile(`(?i)\bBearer[ \t]+[A-Za-z0-9._~+/=-]{12,}`)},
	{kind: "secret key", pattern: regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}\b`)},
	{kind: "github token", pattern: regexp.MustCompile(`\b(?:ghp_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})\b`)},
	{kind: "aws access key", pattern: regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
	{kind: "credential assignment", pattern: regexp.MustCompile(`(?i)\b(?:api[_-]?key|client[_-]?secret|token|password|passwd|secret|private[_-]?key|access[_-]?key|refresh[_-]?token)\b\s*(?::=|=)\s*["` + "`" + `][^"` + "`" + `]{8,}`)},
	{kind: "credential assignment", pattern: regexp.MustCompile(`(?i)\b(?:api[_-]?key|client[_-]?secret|token|password|passwd|secret|private[_-]?key|access[_-]?key|refresh[_-]?token)\b\s*(?::=|=)\s*[A-Za-z0-9][A-Za-z0-9._~+/=-]{7,}`)},
	{kind: "credential assignment", pattern: regexp.MustCompile(`(?i)["']?(?:api[_-]?key|client[_-]?secret|token|password|passwd|secret|private[_-]?key|access[_-]?key|refresh[_-]?token)["']?\s*:\s*["']?[A-Za-z0-9][A-Za-z0-9._~+/=-]{7,}`)},
}

var dynamicCredentialReference = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)+$`)
var disabled atomic.Bool

// SetDisabled changes credential detection for the current process and returns
// a restore function. It exists for the explicitly unsafe CLI override; normal
// callers must leave detection enabled.
func SetDisabled(value bool) func() {
	previous := disabled.Swap(value)
	return func() {
		disabled.Store(previous)
	}
}

// Detect returns the credential kind without returning the matched value.
func Detect(text string) (string, bool) {
	if disabled.Load() {
		return "", false
	}
	for _, candidate := range patterns {
		for _, match := range candidate.pattern.FindAllString(text, -1) {
			if candidate.kind == "credential assignment" {
				if looksLikePlaceholder(match) || !looksLikeCredentialAssignment(match) {
					continue
				}
			}
			return candidate.kind, true
		}
	}
	return "", false
}

func looksLikeCredentialAssignment(match string) bool {
	value := credentialAssignmentValue(match)
	if value == "" {
		return false
	}
	if !credentialAssignmentValueIsQuoted(match) && dynamicCredentialReference.MatchString(value) {
		return false
	}
	valueStart := strings.Index(match, value)
	if valueStart < 0 {
		return false
	}
	if strings.ContainsAny(match[valueStart:], "\"'`") || len(value) >= 16 {
		return true
	}
	if len(value) < 8 {
		return false
	}
	for _, character := range value {
		if character >= '0' && character <= '9' || strings.ContainsRune("-_./+=~", character) {
			return true
		}
	}
	return false
}

func credentialAssignmentValueIsQuoted(match string) bool {
	separator := strings.IndexAny(match, "=:")
	if separator < 0 || separator+1 >= len(match) {
		return false
	}
	valueStart := separator + 1
	if match[separator] == ':' && valueStart < len(match) && match[valueStart] == '=' {
		valueStart++
	}
	tail := strings.TrimSpace(match[valueStart:])
	return tail != "" && strings.ContainsRune("\"'`", rune(tail[0]))
}

func credentialAssignmentValue(match string) string {
	separator := strings.IndexAny(match, "=:")
	if separator < 0 || separator+1 >= len(match) {
		return ""
	}
	valueStart := separator + 1
	if match[separator] == ':' && valueStart < len(match) && match[valueStart] == '=' {
		valueStart++
	}
	return strings.Trim(strings.TrimSpace(match[valueStart:]), "\"'`")
}

func looksLikePlaceholder(match string) bool {
	lower := strings.ToLower(match)
	for _, marker := range []string{
		"<your", "${", "placeholder", "redacted", "replace-me", "replace_me",
		"your-api", "your_api", "example", "dummy", "changeme", "change-me",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	value := credentialAssignmentValue(match)
	return len(value) >= 8 && strings.Trim(value, "0") == ""
}
