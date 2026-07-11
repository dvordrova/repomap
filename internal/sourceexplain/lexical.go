package sourceexplain

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/dvordrova/repomap/internal/sourcecard"
)

var (
	identifierPattern    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	nilComparisonPattern = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\s*(?:!=|==)\s*nil\b`)
)

// supportedClaimStatement conservatively recognizes a few common Go source
// shapes. It deliberately does not infer callee behavior from a callee name.
func supportedClaimStatement(bundle Bundle, question Question, evidenceIDs []string) (string, bool) {
	lines := selectedSourceLines(bundle, evidenceIDs)
	anchor, ok := sourceLineByID(bundle.Source.Lines, question.AnchorSourceEvidenceID)
	if !ok || !mentionsCall(anchor.Text, question.CalleeName) {
		return "", false
	}

	switch question.Predicate {
	case PredicateValidatesInput:
		if !callResultIsGuarded(anchor, lines, question.CalleeName) {
			return "", false
		}
		return fmt.Sprintf(
			"The source uses the result of calling %s in a conditional guard inside %s; the callee's behavior remains unverified.",
			question.CalleeName,
			bundle.Target.Name,
		), true
	case PredicateDelegatesOperation:
		switch {
		case hasAssignmentBeforeCall(anchor.Text, question.CalleeName):
			return fmt.Sprintf(
				"The source assigns value(s) returned by %s while executing %s; the callee's behavior remains unverified.",
				question.CalleeName,
				bundle.Target.Name,
			), true
		case hasKeywordBeforeCall(anchor.Text, "return", question.CalleeName):
			return fmt.Sprintf(
				"The source returns value(s) from %s while executing %s; the callee's behavior remains unverified.",
				question.CalleeName,
				bundle.Target.Name,
			), true
		default:
			return "", false
		}
	case PredicateMapsError:
		if !hasKeywordBeforeCall(anchor.Text, "return", question.CalleeName) || !hasVisibleErrorGuard(anchor, lines) {
			return "", false
		}
		return fmt.Sprintf(
			"On a locally visible nil comparison, the source returns value(s) from %s inside %s.",
			question.CalleeName,
			bundle.Target.Name,
		), true
	case PredicateFillsResponse:
		return fmt.Sprintf(
			"The source calls %s from %s; what the callee changes remains unverified.",
			question.CalleeName,
			bundle.Target.Name,
		), true
	case PredicatePersistsState:
		return fmt.Sprintf(
			"The source calls %s from %s; any persistence side effect remains unverified.",
			question.CalleeName,
			bundle.Target.Name,
		), true
	case PredicatePerformsIO:
		return fmt.Sprintf(
			"The source calls %s from %s; any runtime I/O effect remains unverified.",
			question.CalleeName,
			bundle.Target.Name,
		), true
	default:
		return "", false
	}
}

func selectedSourceLines(bundle Bundle, evidenceIDs []string) []sourcecard.Line {
	selected := makeStringSet(evidenceIDs)
	lines := make([]sourcecard.Line, 0, len(evidenceIDs))
	for _, line := range bundle.Source.Lines {
		if _, ok := selected[line.EvidenceID]; ok {
			lines = append(lines, line)
		}
	}
	return lines
}

func sourceLineByID(lines []sourcecard.Line, evidenceID string) (sourcecard.Line, bool) {
	for _, line := range lines {
		if line.EvidenceID == evidenceID {
			return line, true
		}
	}
	return sourcecard.Line{}, false
}

func callResultIsGuarded(anchor sourcecard.Line, selected []sourcecard.Line, calleeName string) bool {
	if condition, ok := conditionText(anchor.Text); ok && mentionsCall(condition, calleeName) {
		return true
	}

	assigned := assignedIdentifiers(anchor.Text, calleeName)
	if len(assigned) == 0 {
		return false
	}
	for _, line := range selected {
		if line.Line <= anchor.Line {
			continue
		}
		condition, ok := conditionText(line.Text)
		if !ok {
			continue
		}
		for _, identifier := range assigned {
			if mentionsIdentifier(condition, identifier) {
				return true
			}
		}
	}
	return false
}

func hasVisibleErrorGuard(anchor sourcecard.Line, selected []sourcecard.Line) bool {
	for _, line := range selected {
		if line.Line > anchor.Line {
			continue
		}
		condition, ok := conditionText(line.Text)
		if !ok {
			continue
		}
		matches := nilComparisonPattern.FindAllStringSubmatch(condition, -1)
		for _, match := range matches {
			if len(match) == 2 && mentionsIdentifier(anchor.Text, match[1]) {
				return true
			}
		}
	}
	return false
}

func conditionText(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "if ") && !strings.HasPrefix(trimmed, "if\t") {
		return "", false
	}
	brace := strings.IndexByte(trimmed, '{')
	if brace < 0 {
		return "", false
	}
	return trimmed[:brace], true
}

func assignedIdentifiers(line, calleeName string) []string {
	callIndex := callStart(line, calleeName)
	if callIndex < 0 {
		return nil
	}
	prefix := line[:callIndex]
	operatorIndex := assignmentOperatorIndex(prefix)
	if operatorIndex < 0 {
		return nil
	}
	lhs := strings.TrimSpace(prefix[:operatorIndex])
	if separator := strings.LastIndexAny(lhs, ";{}"); separator >= 0 {
		lhs = lhs[separator+1:]
	}
	parts := strings.Split(lhs, ",")
	identifiers := make([]string, 0, len(parts))
	for _, part := range parts {
		identifier := strings.TrimSpace(part)
		if identifier != "_" && identifierPattern.MatchString(identifier) {
			identifiers = append(identifiers, identifier)
		}
	}
	return identifiers
}

func hasAssignmentBeforeCall(line, calleeName string) bool {
	callIndex := callStart(line, calleeName)
	return callIndex >= 0 && assignmentOperatorIndex(line[:callIndex]) >= 0
}

func assignmentOperatorIndex(value string) int {
	if index := strings.LastIndex(value, ":="); index >= 0 {
		return index
	}
	for index := len(value) - 1; index >= 0; index-- {
		if value[index] != '=' {
			continue
		}
		var previous, next byte
		if index > 0 {
			previous = value[index-1]
		}
		if index+1 < len(value) {
			next = value[index+1]
		}
		if previous != '=' && previous != '!' && previous != '<' && previous != '>' && next != '=' {
			return index
		}
	}
	return -1
}

func hasKeywordBeforeCall(line, keyword, calleeName string) bool {
	callIndex := callStart(line, calleeName)
	if callIndex < 0 {
		return false
	}
	pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(keyword) + `\b`)
	return pattern.MatchString(line[:callIndex])
}

func mentionsCall(line, calleeName string) bool {
	return callStart(line, calleeName) >= 0
}

func callStart(line, calleeName string) int {
	pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(calleeName) + `\s*\(`)
	location := pattern.FindStringIndex(line)
	if location == nil {
		return -1
	}
	return location[0]
}
