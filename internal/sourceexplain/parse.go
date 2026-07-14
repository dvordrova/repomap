package sourceexplain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/sourcecard"
)

const ParserVersion = 1

type ParseWarning struct {
	Code    string `json:"code"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ParseResult struct {
	Report   Report         `json:"report"`
	Warnings []ParseWarning `json:"warnings"`
}

var trailingCommaPattern = regexp.MustCompile(`,\s*([}\]])`)

var responseFields = map[string]struct{}{
	"assessments":    {},
	"unknowns":       {},
	"next_action_id": {},
}

func ParseReport(bundle Bundle, data []byte) (ParseResult, error) {
	if err := bundle.Validate(); err != nil {
		return ParseResult{}, err
	}
	object, formatWarnings, err := decodeResponseObject(data)
	if err != nil {
		return ParseResult{}, err
	}
	warnings := append([]ParseWarning{}, formatWarnings...)
	for field := range object {
		if _, ok := responseFields[field]; !ok {
			warnings = append(warnings, parseWarning("response.unknown_field", field, "ignored unknown top-level field"))
		}
	}

	assessments, nextWarnings := parseAssessments(bundle, object["assessments"], warnings)
	warnings = nextWarnings
	unknownsValue, hasUnknowns := object["unknowns"]
	if !hasUnknowns {
		warnings = append(warnings, parseWarning("unknowns.missing_defaulted", "unknowns", "missing unknowns field was defaulted locally"))
	}
	unknowns, nextWarnings := parseUnknowns(bundle, unknownsValue, warnings)
	warnings = nextWarnings
	if !hasModelMandatoryUnknowns(bundle, unknowns) {
		warnings = append(warnings, parseWarning("unknowns.mandatory_defaulted", "unknowns", "missing mandatory unknowns were defaulted locally"))
	}
	action, nextWarnings := parseAction(bundle, object["next_action_id"], warnings)
	warnings = nextWarnings

	report := Report{
		Version:     ReportVersion,
		Target:      bundle.Target,
		Assessments: assessments,
		Claims:      buildClaims(bundle, assessments),
		Unknowns:    ensureMandatoryUnknowns(bundle, unknowns),
		NextAction:  action,
	}
	if err := ValidateReport(bundle, report); err != nil {
		return ParseResult{}, err
	}
	sort.Slice(warnings, func(i, j int) bool {
		if warnings[i].Code != warnings[j].Code {
			return warnings[i].Code < warnings[j].Code
		}
		return warnings[i].Field < warnings[j].Field
	})
	return ParseResult{Report: report, Warnings: warnings}, nil
}

func parseAssessments(bundle Bundle, value any, warnings []ParseWarning) ([]Assessment, []ParseWarning) {
	items, ok := value.([]any)
	if !ok {
		if object, isObject := value.(map[string]any); isObject {
			items = []any{object}
			warnings = append(warnings, parseWarning("assessments.object_accepted", "assessments", "accepted one assessment object as an array"))
		} else if value != nil {
			warnings = append(warnings, parseWarning("assessments.invalid_ignored", "assessments", "ignored invalid assessments value"))
		}
	}

	questions := questionMap(bundle)
	parsed := make(map[string]Assessment, len(items))
	conflicted := make(map[string]struct{})
	for index, item := range items {
		field := fmt.Sprintf("assessments[%d]", index)
		object, ok := item.(map[string]any)
		if !ok {
			warnings = append(warnings, parseWarning("assessment.invalid_ignored", field, "ignored non-object assessment"))
			continue
		}
		questionID, _ := stringValue(object["question_id"])
		question, known := questions[questionID]
		if !known {
			warnings = append(warnings, parseWarning("assessment.unknown_question_ignored", field+".question_id", "ignored assessment with unknown question id"))
			continue
		}
		if _, duplicate := parsed[questionID]; duplicate {
			conflicted[questionID] = struct{}{}
			warnings = append(warnings, parseWarning("assessment.duplicate_ambiguous", field, "duplicate assessment was reduced to ambiguous"))
			continue
		}
		verdictText, _ := stringValue(object["verdict"])
		normalizedVerdict := strings.ToLower(verdictText)
		if normalizedVerdict != verdictText && verdictText != "" {
			warnings = append(warnings, parseWarning("assessment.verdict_case_normalized", field+".verdict", "normalized verdict case"))
		}
		verdict := Verdict(normalizedVerdict)
		if !verdict.valid() {
			verdict = VerdictAmbiguous
			warnings = append(warnings, parseWarning("assessment.verdict_ambiguous", field+".verdict", "invalid or missing verdict became ambiguous"))
		}
		evidenceValue := object["source_evidence_ids"]
		if evidenceValue == nil {
			if alias := object["evidence_ids"]; alias != nil {
				evidenceValue = alias
				warnings = append(warnings, parseWarning("assessment.evidence_alias_accepted", field+".evidence_ids", "accepted evidence_ids alias"))
			}
		}
		evidenceIDs, nextWarnings := parseAssessmentEvidence(question, evidenceValue, field, warnings)
		warnings = nextWarnings
		if verdict == VerdictShown && !containsString(evidenceIDs, question.AnchorSourceEvidenceID) {
			verdict = VerdictAmbiguous
			warnings = append(warnings, parseWarning("assessment.shown_without_anchor", field, "shown assessment without its anchor source evidence became ambiguous"))
		}
		if verdict == VerdictShown {
			if _, supported := supportedClaimStatement(bundle, question, evidenceIDs); !supported {
				verdict = VerdictAmbiguous
				warnings = append(warnings, parseWarning("assessment.shown_without_predicate_support", field, "shown assessment without predicate-specific lexical support became ambiguous"))
			}
		}
		if verdict == VerdictNotShown {
			evidenceIDs = []string{}
			verdict = VerdictAmbiguous
			warnings = append(warnings, parseWarning("assessment.not_shown_lexical_window", field, "not_shown became ambiguous because a lexical window cannot prove absence"))
		}
		parsed[questionID] = Assessment{
			QuestionID:        questionID,
			Verdict:           verdict,
			SourceEvidenceIDs: evidenceIDs,
		}
	}

	result := make([]Assessment, 0, len(bundle.Questions))
	for _, question := range bundle.Questions {
		assessment, ok := parsed[question.ID]
		if _, conflict := conflicted[question.ID]; conflict {
			assessment = Assessment{QuestionID: question.ID, Verdict: VerdictAmbiguous, SourceEvidenceIDs: []string{}}
			ok = true
		}
		if !ok {
			assessment = Assessment{QuestionID: question.ID, Verdict: VerdictAmbiguous, SourceEvidenceIDs: []string{}}
			warnings = append(warnings, parseWarning("assessment.missing_ambiguous", "assessments", "missing question assessment became ambiguous: "+question.ID))
		}
		result = append(result, assessment)
	}
	return result, warnings
}

func parseAssessmentEvidence(question Question, value any, field string, warnings []ParseWarning) ([]string, []ParseWarning) {
	values, scalar := stringList(value)
	if scalar {
		warnings = append(warnings, parseWarning("assessment.evidence_scalar_accepted", field+".source_evidence_ids", "accepted one source evidence id as an array"))
	}
	candidates := makeStringSet(question.CandidateSourceEvidenceIDs)
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, id := range values {
		if _, ok := candidates[id]; !ok {
			warnings = append(warnings, parseWarning("assessment.evidence_irrelevant_dropped", field+".source_evidence_ids", "dropped unknown or irrelevant source evidence id "+fmt.Sprintf("%q", id)))
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	sortEvidenceIDs(result, sourceLinesForQuestion(question))
	return result, warnings
}

func parseUnknowns(bundle Bundle, value any, warnings []ParseWarning) ([]Unknown, []ParseWarning) {
	items, ok := value.([]any)
	if !ok {
		if value == nil {
			return []Unknown{}, warnings
		}
		items = []any{value}
		warnings = append(warnings, parseWarning("unknowns.scalar_accepted", "unknowns", "accepted one unknown as an array"))
	}
	knownAnchors := knownAnchorSet(bundle)
	seen := make(map[string]struct{})
	result := make([]Unknown, 0, len(items))
	for index, item := range items {
		field := fmt.Sprintf("unknowns[%d]", index)
		var kindText, anchor string
		switch typed := item.(type) {
		case string:
			kindText = typed
			anchor = bundle.Target.EvidenceID
		case map[string]any:
			kindText, _ = stringValue(typed["kind"])
			anchor, _ = stringValue(typed["anchor_evidence_id"])
		default:
			warnings = append(warnings, parseWarning("unknown.invalid_ignored", field, "ignored invalid unknown"))
			continue
		}
		kind := UnknownKind(strings.ToLower(kindText))
		if !kind.valid() {
			warnings = append(warnings, parseWarning("unknown.kind_ignored", field+".kind", "ignored unknown kind"))
			continue
		}
		if _, ok := knownAnchors[anchor]; !ok {
			warnings = append(warnings, parseWarning("unknown.anchor_ignored", field+".anchor_evidence_id", "ignored unknown with invalid anchor"))
			continue
		}
		key := string(kind) + "\x00" + anchor
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, Unknown{Kind: kind, AnchorEvidenceID: anchor})
	}
	return result, warnings
}

func parseAction(bundle Bundle, value any, warnings []ParseWarning) (Action, []ParseWarning) {
	actionID, _ := stringValue(value)
	allowed, ok := actionMap(bundle)[actionID]
	origin := ActionOriginModel
	if !ok {
		allowed = bundle.AllowedActions[0]
		origin = ActionOriginLocalPolicy
		warnings = append(warnings, parseWarning("action.local_fallback", "next_action_id", "invalid or missing action id used the local default"))
	}
	return Action{
		ID:               allowed.ID,
		Operation:        allowed.Operation,
		AnchorEvidenceID: allowed.AnchorEvidenceID,
		Origin:           origin,
	}, warnings
}

func buildClaims(bundle Bundle, assessments []Assessment) []Claim {
	questions := questionMap(bundle)
	claims := make([]Claim, 0, len(assessments))
	for _, assessment := range assessments {
		if assessment.Verdict != VerdictShown {
			continue
		}
		question := questions[assessment.QuestionID]
		statement, supported := supportedClaimStatement(bundle, question, assessment.SourceEvidenceIDs)
		if !supported {
			continue
		}
		claims = append(claims, Claim{
			Predicate:             question.Predicate,
			Statement:             statement,
			EvidenceLevel:         EvidenceLevelSourceSupported,
			SourceEvidenceIDs:     append([]string{}, assessment.SourceEvidenceIDs...),
			StructuralEvidenceIDs: []string{question.AnchorEvidenceID},
		})
	}
	return claims
}

func ensureMandatoryUnknowns(bundle Bundle, unknowns []Unknown) []Unknown {
	result := append([]Unknown{}, unknowns...)
	result = appendUnknown(result, Unknown{Kind: UnknownTestCoverage, AnchorEvidenceID: bundle.Target.EvidenceID})
	result = appendUnknown(result, Unknown{Kind: UnknownRuntimeReachability, AnchorEvidenceID: bundle.Target.EvidenceID})
	return result
}

func appendUnknown(values []Unknown, candidate Unknown) []Unknown {
	for _, value := range values {
		if value == candidate {
			return values
		}
	}
	return append(values, candidate)
}

func hasModelMandatoryUnknowns(bundle Bundle, unknowns []Unknown) bool {
	hasTestCoverage := false
	hasRuntimeReachability := false
	for _, unknown := range unknowns {
		if unknown.AnchorEvidenceID != bundle.Target.EvidenceID {
			continue
		}
		switch unknown.Kind {
		case UnknownTestCoverage:
			hasTestCoverage = true
		case UnknownRuntimeReachability:
			hasRuntimeReachability = true
		}
	}
	return hasTestCoverage && hasRuntimeReachability
}

func decodeResponseObject(data []byte) (map[string]any, []ParseWarning, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, nil, fmt.Errorf("source explain: model response is empty")
	}
	if object, err := decodeObject(data); err == nil {
		return object, nil, nil
	}
	recovered := recoverJSONObject(data)
	if len(recovered) == 0 {
		return nil, nil, fmt.Errorf("source explain: model response does not contain a json object")
	}
	warnings := []ParseWarning{parseWarning("response.object_recovered", "", "recovered json object from surrounding text")}
	if object, err := decodeObject(recovered); err == nil {
		return object, warnings, nil
	}
	repaired := trailingCommaPattern.ReplaceAll(recovered, []byte("$1"))
	object, err := decodeObject(repaired)
	if err != nil {
		return nil, nil, fmt.Errorf("source explain: parse model json object: %w", err)
	}
	warnings = append(warnings, parseWarning("response.trailing_comma_repaired", "", "removed trailing commas from json object"))
	return object, warnings, nil
}

func decodeObject(data []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		return nil, err
	}
	if object == nil {
		return nil, fmt.Errorf("expected json object")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("unexpected trailing json value")
		}
		return nil, err
	}
	return object, nil
}

func recoverJSONObject(data []byte) []byte {
	start := bytes.IndexByte(data, '{')
	if start < 0 {
		return nil
	}
	depth := 0
	inString := false
	escaped := false
	for index := start; index < len(data); index++ {
		char := data[index]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if char == '\\' {
				escaped = true
				continue
			}
			if char == '"' {
				inString = false
			}
			continue
		}
		switch char {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return data[start : index+1]
			}
		}
	}
	return nil
}

func stringValue(value any) (string, bool) {
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	text = strings.TrimSpace(text)
	return text, text != ""
}

func stringList(value any) ([]string, bool) {
	if text, ok := stringValue(value); ok {
		return []string{text}, true
	}
	items, ok := value.([]any)
	if !ok {
		return []string{}, false
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := stringValue(item); ok {
			result = append(result, text)
		}
	}
	return result, false
}

func sourceLinesForQuestion(question Question) []sourcecard.Line {
	lines := make([]sourcecard.Line, 0, len(question.CandidateSourceEvidenceIDs))
	for index, id := range question.CandidateSourceEvidenceIDs {
		lines = append(lines, sourcecard.Line{EvidenceID: id, Line: index + 1})
	}
	return lines
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func parseWarning(code, field, message string) ParseWarning {
	return ParseWarning{Code: code, Field: field, Message: message}
}
