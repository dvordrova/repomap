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

// persistencePatterns are an always-on, deliberately narrow guard for bytes
// that may be written to a cache, journal, or diagnostic. Unlike patterns,
// these do not try to classify repository source: they cover only concrete
// provider-token shapes, private-key headers, explicit Authorization headers,
// and literal api_key assignments.
var persistencePatterns = []struct {
	kind       string
	pattern    *regexp.Regexp
	bearer     bool
	assignment bool
}{
	{kind: ClosedKindPrivateKey, pattern: regexp.MustCompile(`(?i)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`)},
	{kind: ClosedKindBearerCredential, pattern: regexp.MustCompile(`(?i)\bBearer[ \t]+[A-Za-z0-9._~+/=-]{8,}`), bearer: true},
	// Require a header/object-key boundary. Program identities legitimately
	// contain path/ref fragments such as `routes/authorization:authorize`;
	// treating the colon in that identity as an HTTP header makes compact JSON
	// scan across unrelated fields on the same line.
	{kind: ClosedKindBearerCredential, pattern: regexp.MustCompile("(?im)(?:^[ \\t]*|[{\"'`(,;][ \\t]*)Authorization[ \\t]*:[ \\t]*[^\\r\\n]{4,}")},
	{kind: ClosedKindSecretKey, pattern: regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{8,}\b`)},
	{kind: ClosedKindGitHubToken, pattern: regexp.MustCompile(`\b(?:ghp_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})\b`)},
	{kind: ClosedKindAWSAccessKey, pattern: regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
	{kind: ClosedKindCredentialAssignment, pattern: regexp.MustCompile(`(?i)["']?api[_-]?key["']?[ \t]*(?::=|=|:)[ \t]*(?:\\?["'])[^"'\r\n]{8,}(?:\\?["'])`), assignment: true},
	{kind: ClosedKindCredentialAssignment, pattern: regexp.MustCompile(`(?i)["']?api[_-]?key["']?[ \t]*(?::=|=|:)[ \t]*[A-Za-z0-9][A-Za-z0-9._~+/=-]{15,}`), assignment: true},
}

var dynamicCredentialReference = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)+$`)
var formatSpecifier = regexp.MustCompile(`%[sdvqxTtbf]`)
var bareIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var bearerCredentialContext = regexp.MustCompile(`(?i)(?:authorization|auth|access[_-]?token|token|header)\b["']?\s*(?::=|=|:)\s*["']?$`)
var enabled atomic.Bool

const (
	ClosedKindPrivateKey           = "private_key"
	ClosedKindBearerCredential     = "bearer_credential"
	ClosedKindSecretKey            = "secret_key"
	ClosedKindGitHubToken          = "github_token"
	ClosedKindAWSAccessKey         = "aws_access_key"
	ClosedKindCredentialAssignment = "credential_assignment"
)

// SetEnabled changes heuristic credential detection for the current process
// and returns a restore function. The ordinary CLI enables it only when the
// caller passes --scan-secrets.
func SetEnabled(value bool) func() {
	previous := enabled.Swap(value)
	return func() {
		enabled.Store(previous)
	}
}

// Detect returns the credential kind without returning the matched value.
func Detect(text string) (string, bool) {
	if !enabled.Load() {
		return "", false
	}
	return detect(text)
}

// DetectPersistenceSensitive reports a closed credential kind independently
// of --scan-secrets. It is only for persistence/observation boundaries and
// must not be used as a broad gate over trusted repository source.
func DetectPersistenceSensitive(text string) (string, bool) {
	for _, candidate := range persistencePatterns {
		for _, location := range candidate.pattern.FindAllStringIndex(text, -1) {
			match := text[location[0]:location[1]]
			if candidate.bearer && !looksLikeBearerCredential(text, location, match) {
				continue
			}
			if candidate.assignment && (looksLikePlaceholder(match) || !looksLikeCredentialAssignment(match)) {
				continue
			}
			return candidate.kind, true
		}
	}
	return "", false
}

func detect(text string) (string, bool) {
	for _, candidate := range patterns {
		for _, location := range candidate.pattern.FindAllStringIndex(text, -1) {
			match := text[location[0]:location[1]]
			if candidate.kind == "bearer credential" &&
				!looksLikeBearerCredential(text, location, match) {
				continue
			}
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

// looksLikeBearerCredential distinguishes an obvious token from ordinary use
// of the authentication scheme name. A long opaque-looking value always
// closes; a shorter all-lowercase word closes only in an exact credential
// assignment/header context. This avoids an unbounded prose dictionary while
// retaining `Authorization: Bearer ...` and `auth = "Bearer ..."` detection.
func looksLikeBearerCredential(text string, location []int, match string) bool {
	fields := strings.Fields(match)
	if len(fields) != 2 || !strings.EqualFold(fields[0], "bearer") {
		return true
	}
	token := fields[1]
	if len(token) >= 24 {
		return true
	}
	hyphens := 0
	uppercase := 0
	for position, character := range token {
		switch {
		case character >= '0' && character <= '9':
			return true
		case strings.ContainsRune("._~+/=", character):
			return true
		case character == '-':
			hyphens++
		case character >= 'A' && character <= 'Z':
			uppercase++
			if position != 0 {
				return true
			}
		case character < 'a' || character > 'z':
			return true
		}
	}
	if hyphens >= 2 || uppercase > 1 {
		return true
	}
	prefixStart := max(0, location[0]-96)
	return bearerCredentialContext.MatchString(text[prefixStart:location[0]])
}

func isBareIdentifier(value string) bool {
	return bareIdentifier.MatchString(value)
}

func looksLikeCredentialAssignment(match string) bool {
	value := credentialAssignmentValue(match)
	if value == "" {
		return false
	}
	// A message-template or format string ("token: %s is not supported") is
	// not a credential assignment: the quoted tail is prose, not a secret.
	// Real source legitimately mentions credential-shaped words in log and
	// error messages; those must never fail a provider request.
	if strings.ContainsAny(value, "%") && formatSpecifier.MatchString(value) {
		return false
	}
	if !credentialAssignmentValueIsQuoted(match) && dynamicCredentialReference.MatchString(value) {
		return false
	}
	// A bare runtime identifier ("PrivateKey: tokenJwtPrivateKey",
	// "password: myVar") is a variable reference, not a credential literal.
	// Only a literal — quoted, long, or digit/symbol-bearing — fails closed.
	if !credentialAssignmentValueIsQuoted(match) && isBareIdentifier(value) {
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
		if character >= '0' && character <= '9' || strings.ContainsRune(".-/+=~", character) {
			return true
		}
	}
	// A short, unquoted, pure-identifier value is a variable or enum
	// reference, not a credential literal ("token:Grant_type" in a message,
	// "password: my_secret" in a runtime selector). Underscores alone are not
	// credential evidence.
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
