package sourceexplain

import (
	"fmt"
	"go/scanner"
	"go/token"
	"regexp"
	"strings"

	"github.com/dvordrova/repomap/internal/sourcecard"
)

var (
	nilComparisonPattern = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\s*(?:!=|==)\s*nil\b`)
)

const maxValidationProofLines = 24

type validationUse string

const (
	validationConditionalGuard      validationUse = "conditional_guard"
	validationReturnedNilComparison validationUse = "returned_nil_comparison"
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
		proofIDs, use, proven := validationProofSourceIDs(question, bundle.Source.Lines)
		if !proven || !containsAllStrings(evidenceIDs, proofIDs) {
			return "", false
		}
		if use == validationReturnedNilComparison {
			return fmt.Sprintf(
				"The source uses the value assigned from calling %s in a returned nil comparison inside %s; the callee's behavior remains unverified.",
				question.CalleeName,
				bundle.Target.Name,
			), true
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

func validationProofSourceIDs(
	question Question,
	lines []sourcecard.Line,
) ([]string, validationUse, bool) {
	anchor, ok := sourceLineByID(lines, question.AnchorSourceEvidenceID)
	if !ok || anchor.Truncated || !mentionsCall(anchor.Text, question.CalleeName) {
		return nil, "", false
	}
	segment, lineByNumber, ok := validationSourceSegment(anchor, lines)
	if !ok {
		return nil, "", false
	}
	tokens, ok := scanValidationTokens(segment, anchor.Line)
	if !ok {
		return nil, "", false
	}
	callIndex, closingIndex, ok := uniqueCallBounds(tokens, anchor.Line, question.CalleeName)
	if !ok {
		return nil, "", false
	}
	if directCallCondition(tokens, anchor.Line, callIndex, closingIndex) {
		return []string{anchor.EvidenceID}, validationConditionalGuard, true
	}
	assigned, ok := assignedCallResultIdentifiers(tokens, callIndex)
	if !ok {
		return nil, "", false
	}
	proofLine, use, ok := immediateValidationResultUse(
		anchor,
		tokens,
		lineByNumber,
		closingIndex,
		assigned,
	)
	if !ok || proofLine.Truncated {
		return nil, "", false
	}
	if proofLine.EvidenceID == anchor.EvidenceID {
		return []string{anchor.EvidenceID}, use, true
	}
	return []string{anchor.EvidenceID, proofLine.EvidenceID}, use, true
}

type scannedToken struct {
	token   token.Token
	literal string
	line    int
}

func immediateValidationResultUse(
	anchor sourcecard.Line,
	tokens []scannedToken,
	lineByNumber map[int]sourcecard.Line,
	closingIndex int,
	assigned []string,
) (sourcecard.Line, validationUse, bool) {
	index := skipSemicolons(tokens, closingIndex+1)
	if index >= len(tokens) {
		return sourcecard.Line{}, "", false
	}
	use := validationUse("")
	switch {
	case tokens[index].token == token.RETURN:
		index++
		use = validationReturnedNilComparison
	case tokens[index].token == token.IF:
		index++
		use = validationConditionalGuard
	case startsIfStatement(anchor.Text):
		use = validationConditionalGuard
	default:
		return sourcecard.Line{}, "", false
	}

	line, next, ok := exactAssignedNilComparison(tokens, index, assigned)
	if !ok || next >= len(tokens) {
		return sourcecard.Line{}, "", false
	}
	switch use {
	case validationReturnedNilComparison:
		if tokens[next].token != token.SEMICOLON {
			return sourcecard.Line{}, "", false
		}
	case validationConditionalGuard:
		if tokens[next].token != token.LBRACE {
			return sourcecard.Line{}, "", false
		}
	}
	proofLine, ok := lineByNumber[line]
	if !ok {
		return sourcecard.Line{}, "", false
	}
	return proofLine, use, true
}

func validationSourceSegment(
	anchor sourcecard.Line,
	lines []sourcecard.Line,
) (string, map[int]sourcecard.Line, bool) {
	var builder strings.Builder
	lineByNumber := make(map[int]sourcecard.Line)
	foundAnchor := false
	lastLine := anchor.Line - 1
	for _, line := range lines {
		if line.Line < anchor.Line {
			continue
		}
		if line.Line > anchor.Line+maxValidationProofLines {
			break
		}
		if line.Line != lastLine+1 {
			return "", nil, false
		}
		if line.Truncated {
			break
		}
		foundAnchor = true
		lastLine = line.Line
		lineByNumber[line.Line] = line
		builder.WriteString(line.Text)
		builder.WriteByte('\n')
	}
	return builder.String(), lineByNumber, foundAnchor
}

func scanValidationTokens(source string, firstLine int) ([]scannedToken, bool) {
	fileSet := token.NewFileSet()
	file := fileSet.AddFile("source.go", -1, len(source))
	var lexer scanner.Scanner
	hadError := false
	lexer.Init(file, []byte(source), func(_ token.Position, _ string) {
		hadError = true
	}, scanner.ScanComments)

	result := make([]scannedToken, 0, 64)
	for {
		position, item, literal := lexer.Scan()
		if item == token.EOF {
			break
		}
		if item == token.COMMENT {
			continue
		}
		result = append(result, scannedToken{
			token:   item,
			literal: literal,
			line:    firstLine + fileSet.Position(position).Line - 1,
		})
	}
	return result, !hadError
}

func uniqueCallBounds(
	tokens []scannedToken,
	anchorLine int,
	calleeName string,
) (int, int, bool) {
	callee := calleeName
	if separator := strings.LastIndex(callee, "."); separator >= 0 {
		callee = callee[separator+1:]
	}
	callIndexes := make([]int, 0, 1)
	for index := 0; index+1 < len(tokens); index++ {
		if tokens[index].line == anchorLine && tokens[index].token == token.IDENT &&
			tokens[index].literal == callee &&
			tokens[index+1].token == token.LPAREN {
			callIndexes = append(callIndexes, index)
		}
	}
	if len(callIndexes) != 1 {
		return 0, 0, false
	}

	depth := 0
	for index := callIndexes[0] + 1; index < len(tokens); index++ {
		switch tokens[index].token {
		case token.LPAREN:
			depth++
		case token.RPAREN:
			depth--
			if depth == 0 {
				return callIndexes[0], index, true
			}
		}
	}
	return 0, 0, false
}

func directCallCondition(
	tokens []scannedToken,
	anchorLine int,
	callIndex int,
	closingIndex int,
) bool {
	start, ok := callExpressionStart(tokens, callIndex)
	if !ok {
		return false
	}
	prefix := tokens[:start]
	if len(prefix) != 1 || prefix[0].token != token.IF {
		if len(prefix) != 2 || prefix[0].token != token.IF || prefix[1].token != token.NOT {
			return false
		}
	}
	if tokens[closingIndex].line != anchorLine {
		return false
	}
	next := closingIndex + 1
	if next < len(tokens) && tokens[next].token == token.LBRACE && tokens[next].line == anchorLine {
		return true
	}
	if next+2 >= len(tokens) ||
		(tokens[next].token != token.EQL && tokens[next].token != token.NEQ) ||
		tokens[next+1].token != token.IDENT || tokens[next+1].literal != "nil" ||
		tokens[next+2].token != token.LBRACE {
		return false
	}
	return tokens[next].line == anchorLine &&
		tokens[next+1].line == anchorLine &&
		tokens[next+2].line == anchorLine
}

func assignedCallResultIdentifiers(tokens []scannedToken, callIndex int) ([]string, bool) {
	start, ok := callExpressionStart(tokens, callIndex)
	if !ok || start == 0 {
		return nil, false
	}
	assignmentIndex := start - 1
	if tokens[assignmentIndex].token != token.DEFINE && tokens[assignmentIndex].token != token.ASSIGN {
		return nil, false
	}
	lhs := tokens[:assignmentIndex]
	if len(lhs) > 0 && lhs[0].token == token.IF {
		lhs = lhs[1:]
	}
	if len(lhs) == 0 || len(lhs)%2 == 0 {
		return nil, false
	}
	identifiers := make([]string, 0, (len(lhs)+1)/2)
	for index, item := range lhs {
		if index%2 == 0 {
			if item.token != token.IDENT {
				return nil, false
			}
			if item.literal != "_" {
				identifiers = append(identifiers, item.literal)
			}
			continue
		}
		if item.token != token.COMMA {
			return nil, false
		}
	}
	return identifiers, len(identifiers) > 0
}

func callExpressionStart(tokens []scannedToken, callIndex int) (int, bool) {
	if callIndex < 0 || callIndex >= len(tokens) || tokens[callIndex].token != token.IDENT {
		return 0, false
	}
	start := callIndex
	for start >= 2 && tokens[start-1].token == token.PERIOD && tokens[start-2].token == token.IDENT {
		start -= 2
	}
	return start, true
}

func skipSemicolons(tokens []scannedToken, index int) int {
	for index < len(tokens) && tokens[index].token == token.SEMICOLON {
		index++
	}
	return index
}

func exactAssignedNilComparison(
	tokens []scannedToken,
	index int,
	assigned []string,
) (int, int, bool) {
	if index+2 >= len(tokens) || tokens[index].token != token.IDENT ||
		(tokens[index+1].token != token.EQL && tokens[index+1].token != token.NEQ) ||
		tokens[index+2].token != token.IDENT || tokens[index+2].literal != "nil" ||
		!containsString(assigned, tokens[index].literal) {
		return 0, 0, false
	}
	if tokens[index].line != tokens[index+1].line || tokens[index].line != tokens[index+2].line {
		return 0, 0, false
	}
	return tokens[index].line, index + 3, true
}

func startsIfStatement(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "if ") || strings.HasPrefix(trimmed, "if\t")
}

func containsAllStrings(values, required []string) bool {
	set := makeStringSet(values)
	for _, value := range required {
		if _, ok := set[value]; !ok {
			return false
		}
	}
	return true
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
