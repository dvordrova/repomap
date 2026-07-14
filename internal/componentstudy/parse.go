package componentstudy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ParsePlan recovers a useful partial plan from a JSON object or fenced JSON.
// Unknown IDs and malformed entries are diagnostics, not whole-response
// failures. It fails only when no framing, question, or structural selection
// survives normalization.
func ParsePlan(bundle Bundle, raw []byte) (Result, error) {
	if err := bundle.Validate(); err != nil {
		return Result{}, err
	}
	object, diagnostics, err := decodePlanObject(raw)
	if err != nil {
		return Result{}, err
	}

	knownFields := map[string]struct{}{
		"version":             {},
		"framing":             {},
		"questions":           {},
		"primary_question_id": {},
		"selected_files":      {},
		"selected_symbols":    {},
		"unknowns":            {},
		"warnings":            {},
	}
	unknownFields := make([]string, 0)
	for field := range object {
		if _, known := knownFields[field]; !known {
			unknownFields = append(unknownFields, field)
		}
	}
	sort.Strings(unknownFields)
	for _, field := range unknownFields {
		diagnostics = append(diagnostics, diagnostic(
			"field.unknown_ignored",
			field,
			"ignored unknown response field",
		))
	}

	plan := Plan{
		Version:         PlanVersion,
		Questions:       []Question{},
		SelectedFiles:   []FileCandidate{},
		SelectedSymbols: []SymbolCandidate{},
		Unknowns:        []string{},
		Warnings:        []string{},
	}
	if value, ok := object["version"]; ok {
		var version int
		if err := json.Unmarshal(value, &version); err != nil || version != PlanVersion {
			diagnostics = append(diagnostics, diagnostic(
				"version.repaired",
				"version",
				"replaced missing or unsupported response version with the local plan version",
			))
		}
	}

	plan.Framing, diagnostics = parseResponseText(
		object["framing"],
		"framing",
		maxResponseTextBytes,
		diagnostics,
	)
	plan.Questions, diagnostics = parseQuestions(bundle, object["questions"], diagnostics)
	plan.SelectedFiles, diagnostics = parseFileSelections(
		bundle,
		object["selected_files"],
		diagnostics,
	)
	plan.SelectedSymbols, diagnostics = parseSymbolSelections(
		bundle,
		object["selected_symbols"],
		diagnostics,
	)
	plan.PrimaryQuestionID, diagnostics = normalizePrimaryQuestion(
		object["primary_question_id"],
		plan.Questions,
		diagnostics,
	)
	if primaryQuestionSelectionGap(plan) {
		diagnostics = append(diagnostics, diagnostic(
			"primary_question.selection_gap",
			"primary_question_id",
			"kept the plan, but the primary question cites no selected file or symbol",
		))
	}
	plan.Unknowns, diagnostics = parseTextList(
		object["unknowns"],
		"unknowns",
		diagnostics,
	)
	plan.Warnings, diagnostics = parseTextList(
		object["warnings"],
		"warnings",
		diagnostics,
	)

	if len(plan.Questions) > 0 && len(plan.Questions) < 2 {
		diagnostics = append(diagnostics, diagnostic(
			"questions.below_schema_minimum",
			"questions",
			"kept the usable question even though the requested plan schema asks for at least two",
		))
	}
	if !plan.usable() {
		return Result{}, fmt.Errorf("component study: response contains no usable framing, questions, or selections")
	}
	if err := plan.Validate(bundle); err != nil {
		return Result{}, err
	}
	return Result{Plan: plan, Diagnostics: diagnostics}, nil
}

func (p Plan) Validate(bundle Bundle) error {
	if err := bundle.Validate(); err != nil {
		return err
	}
	if p.Version != PlanVersion {
		return fmt.Errorf("component study: unsupported plan version %d", p.Version)
	}
	if p.Framing != "" {
		if err := validateText("plan.framing", p.Framing, maxResponseTextBytes); err != nil {
			return err
		}
	}
	if len(p.Questions) > maxQuestionCount {
		return fmt.Errorf("component study: plan has too many questions")
	}
	knownEvidence := bundleEvidenceIDs(bundle)
	seenQuestions := make(map[string]struct{}, len(p.Questions))
	for index, question := range p.Questions {
		if err := validateID("plan.question.id", question.ID); err != nil {
			return err
		}
		if _, exists := seenQuestions[question.ID]; exists {
			return fmt.Errorf("component study: duplicate plan question id %q", question.ID)
		}
		seenQuestions[question.ID] = struct{}{}
		if err := validateText("plan.question.question", question.Question, maxResponseTextBytes); err != nil {
			return err
		}
		if err := validateText("plan.question.why", question.Why, maxResponseTextBytes); err != nil {
			return err
		}
		if len(question.EvidenceIDs) == 0 {
			return fmt.Errorf("component study: questions[%d] has no evidence ids", index)
		}
		seenEvidence := make(map[string]struct{}, len(question.EvidenceIDs))
		for _, id := range question.EvidenceIDs {
			if _, exists := knownEvidence[id]; !exists {
				return fmt.Errorf("component study: question references unknown evidence id")
			}
			if _, exists := seenEvidence[id]; exists {
				return fmt.Errorf("component study: question repeats an evidence id")
			}
			seenEvidence[id] = struct{}{}
		}
	}
	if len(p.Questions) == 0 {
		if p.PrimaryQuestionID != "" {
			return fmt.Errorf("component study: plan without questions has a primary question id")
		}
	} else if _, exists := seenQuestions[p.PrimaryQuestionID]; !exists {
		return fmt.Errorf("component study: primary question id is absent from questions")
	}
	if len(p.SelectedFiles) > maxSelectedFileCount {
		return fmt.Errorf("component study: plan has too many selected files")
	}
	fileByID := make(map[string]FileCandidate, len(bundle.Files))
	for _, candidate := range bundle.Files {
		fileByID[candidate.ID] = candidate
	}
	seenFiles := make(map[string]struct{}, len(p.SelectedFiles))
	for _, selected := range p.SelectedFiles {
		candidate, exists := fileByID[selected.ID]
		if !exists || candidate != selected {
			return fmt.Errorf("component study: selected file was not reconstructed from bundle")
		}
		if _, exists := seenFiles[selected.ID]; exists {
			return fmt.Errorf("component study: selected file is repeated")
		}
		seenFiles[selected.ID] = struct{}{}
	}
	if len(p.SelectedSymbols) > maxSelectedSymbolCount {
		return fmt.Errorf("component study: plan has too many selected symbols")
	}
	symbolByID := make(map[string]SymbolCandidate, len(bundle.Symbols))
	for _, candidate := range bundle.Symbols {
		symbolByID[candidate.ID] = candidate
	}
	seenSymbols := make(map[string]struct{}, len(p.SelectedSymbols))
	for _, selected := range p.SelectedSymbols {
		candidate, exists := symbolByID[selected.ID]
		if !exists || candidate != selected {
			return fmt.Errorf("component study: selected symbol was not reconstructed from bundle")
		}
		if _, exists := seenSymbols[selected.ID]; exists {
			return fmt.Errorf("component study: selected symbol is repeated")
		}
		seenSymbols[selected.ID] = struct{}{}
	}
	if err := validateTextValues("plan.unknowns", p.Unknowns); err != nil {
		return err
	}
	return validateTextValues("plan.warnings", p.Warnings)
}

func normalizePrimaryQuestion(
	value json.RawMessage,
	questions []Question,
	diagnostics []Diagnostic,
) (string, []Diagnostic) {
	id, ok := decodeString(value)
	id = strings.TrimSpace(id)
	if ok && validateID("primary_question_id", id) == nil {
		for _, question := range questions {
			if question.ID == id {
				return id, diagnostics
			}
		}
	}
	if len(questions) == 0 {
		if len(value) > 0 {
			diagnostics = append(diagnostics, diagnostic(
				"primary_question.invalid_ignored",
				"primary_question_id",
				"ignored a primary question id because no matching question survived",
			))
		}
		return "", diagnostics
	}
	diagnostics = append(diagnostics, diagnostic(
		"primary_question.repaired",
		"primary_question_id",
		"selected the first surviving question as the primary research step",
	))
	return questions[0].ID, diagnostics
}

func primaryQuestionSelectionGap(plan Plan) bool {
	if plan.PrimaryQuestionID == "" {
		return false
	}
	selected := make(map[string]struct{}, len(plan.SelectedFiles)+len(plan.SelectedSymbols))
	for _, candidate := range plan.SelectedFiles {
		selected[candidate.ID] = struct{}{}
	}
	for _, candidate := range plan.SelectedSymbols {
		selected[candidate.ID] = struct{}{}
	}
	for _, question := range plan.Questions {
		if question.ID != plan.PrimaryQuestionID {
			continue
		}
		for _, id := range question.EvidenceIDs {
			if _, ok := selected[id]; ok {
				return false
			}
		}
		return true
	}
	return false
}

func parseQuestions(
	bundle Bundle,
	value json.RawMessage,
	diagnostics []Diagnostic,
) ([]Question, []Diagnostic) {
	items, diagnostics := responseItems(value, "questions", diagnostics)
	knownEvidence := bundleEvidenceIDs(bundle)
	questions := make([]Question, 0, min(len(items), maxQuestionCount))
	seenIDs := make(map[string]struct{}, len(items))
	for index, item := range items {
		field := fmt.Sprintf("questions[%d]", index)
		var object map[string]json.RawMessage
		if err := json.Unmarshal(item, &object); err != nil || object == nil {
			diagnostics = append(diagnostics, diagnostic(
				"question.invalid_dropped",
				field,
				"dropped malformed question",
			))
			continue
		}
		id, idOK := decodeString(object["id"])
		id = strings.TrimSpace(id)
		if !idOK || validateID("question.id", id) != nil {
			diagnostics = append(diagnostics, diagnostic(
				"question.invalid_id_dropped",
				field+".id",
				"dropped question with invalid id",
			))
			continue
		}
		if _, exists := seenIDs[id]; exists {
			diagnostics = append(diagnostics, diagnostic(
				"question.duplicate_dropped",
				field+".id",
				"dropped duplicate question id",
			))
			continue
		}
		var questionText string
		questionText, diagnostics = parseResponseText(
			object["question"],
			field+".question",
			maxResponseTextBytes,
			diagnostics,
		)
		var why string
		why, diagnostics = parseResponseText(
			object["why"],
			field+".why",
			maxResponseTextBytes,
			diagnostics,
		)
		if questionText == "" || why == "" {
			diagnostics = append(diagnostics, diagnostic(
				"question.incomplete_dropped",
				field,
				"dropped question without bounded question and why text",
			))
			continue
		}
		var evidenceIDs []string
		evidenceIDs, diagnostics = parseKnownIDs(
			object["evidence_ids"],
			field+".evidence_ids",
			knownEvidence,
			8,
			diagnostics,
		)
		if len(evidenceIDs) == 0 {
			diagnostics = append(diagnostics, diagnostic(
				"question.ungrounded_dropped",
				field+".evidence_ids",
				"dropped question without known evidence ids",
			))
			continue
		}
		if len(questions) >= maxQuestionCount {
			diagnostics = append(diagnostics, diagnostic(
				"questions.limit_dropped",
				field,
				"dropped question beyond the four-question plan limit",
			))
			continue
		}
		seenIDs[id] = struct{}{}
		questions = append(questions, Question{
			ID:          id,
			Question:    questionText,
			Why:         why,
			EvidenceIDs: evidenceIDs,
		})
	}
	return questions, diagnostics
}

func parseFileSelections(
	bundle Bundle,
	value json.RawMessage,
	diagnostics []Diagnostic,
) ([]FileCandidate, []Diagnostic) {
	byID := make(map[string]FileCandidate, len(bundle.Files))
	for _, candidate := range bundle.Files {
		byID[candidate.ID] = candidate
	}
	ids, diagnostics := parseKnownIDs(
		value,
		"selected_files",
		candidateIDSet(bundle.Files, func(candidate FileCandidate) string { return candidate.ID }),
		maxSelectedFileCount,
		diagnostics,
	)
	selected := make([]FileCandidate, 0, len(ids))
	for _, id := range ids {
		selected = append(selected, byID[id])
	}
	return selected, diagnostics
}

func parseSymbolSelections(
	bundle Bundle,
	value json.RawMessage,
	diagnostics []Diagnostic,
) ([]SymbolCandidate, []Diagnostic) {
	byID := make(map[string]SymbolCandidate, len(bundle.Symbols))
	for _, candidate := range bundle.Symbols {
		byID[candidate.ID] = candidate
	}
	ids, diagnostics := parseKnownIDs(
		value,
		"selected_symbols",
		candidateIDSet(bundle.Symbols, func(candidate SymbolCandidate) string { return candidate.ID }),
		maxSelectedSymbolCount,
		diagnostics,
	)
	selected := make([]SymbolCandidate, 0, len(ids))
	for _, id := range ids {
		selected = append(selected, byID[id])
	}
	return selected, diagnostics
}

func parseKnownIDs(
	value json.RawMessage,
	field string,
	known map[string]struct{},
	limit int,
	diagnostics []Diagnostic,
) ([]string, []Diagnostic) {
	items, diagnostics := responseItems(value, field, diagnostics)
	ids := make([]string, 0, min(len(items), limit))
	seen := make(map[string]struct{}, len(items))
	for index, item := range items {
		itemField := fmt.Sprintf("%s[%d]", field, index)
		id, objectID, ok := decodeSelectionID(item)
		id = strings.TrimSpace(id)
		if !ok || validateID("selection.id", id) != nil {
			diagnostics = append(diagnostics, diagnostic(
				"selection.non_id_dropped",
				itemField,
				"dropped selection that was not an opaque id",
			))
			continue
		}
		if objectID {
			diagnostics = append(diagnostics, diagnostic(
				"selection.object_id_accepted",
				itemField,
				"accepted an id-only object as an opaque selection",
			))
		}
		if _, exists := known[id]; !exists {
			diagnostics = append(diagnostics, diagnostic(
				"selection.unknown_id_dropped",
				itemField,
				"dropped id absent from the bounded bundle",
			))
			continue
		}
		if _, exists := seen[id]; exists {
			diagnostics = append(diagnostics, diagnostic(
				"selection.duplicate_dropped",
				itemField,
				"dropped duplicate id",
			))
			continue
		}
		if len(ids) >= limit {
			diagnostics = append(diagnostics, diagnostic(
				"selection.limit_dropped",
				itemField,
				"dropped id beyond the field selection limit",
			))
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, diagnostics
}

func parseTextList(
	value json.RawMessage,
	field string,
	diagnostics []Diagnostic,
) ([]string, []Diagnostic) {
	items, diagnostics := responseItems(value, field, diagnostics)
	values := make([]string, 0, min(len(items), maxResponseListItems))
	seen := make(map[string]struct{}, len(items))
	for index, item := range items {
		itemField := fmt.Sprintf("%s[%d]", field, index)
		var text string
		text, diagnostics = parseResponseText(
			item,
			itemField,
			maxResponseTextBytes,
			diagnostics,
		)
		if text == "" {
			continue
		}
		if _, exists := seen[text]; exists {
			continue
		}
		if len(values) >= maxResponseListItems {
			diagnostics = append(diagnostics, diagnostic(
				"text_list.limit_dropped",
				itemField,
				"dropped text beyond the list limit",
			))
			continue
		}
		seen[text] = struct{}{}
		values = append(values, text)
	}
	return values, diagnostics
}

func responseItems(
	value json.RawMessage,
	field string,
	diagnostics []Diagnostic,
) ([]json.RawMessage, []Diagnostic) {
	if len(value) == 0 || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return []json.RawMessage{}, diagnostics
	}
	var items []json.RawMessage
	if err := json.Unmarshal(value, &items); err == nil {
		return items, diagnostics
	}
	var scalar json.RawMessage
	if err := json.Unmarshal(value, &scalar); err == nil {
		diagnostics = append(diagnostics, diagnostic(
			"array.scalar_accepted",
			field,
			"accepted scalar value as one array item",
		))
		return []json.RawMessage{append(json.RawMessage{}, value...)}, diagnostics
	}
	diagnostics = append(diagnostics, diagnostic(
		"array.invalid_ignored",
		field,
		"ignored malformed array field",
	))
	return []json.RawMessage{}, diagnostics
}

func parseResponseText(
	value json.RawMessage,
	field string,
	maxBytes int,
	diagnostics []Diagnostic,
) (string, []Diagnostic) {
	text, ok := decodeString(value)
	if !ok {
		if len(value) > 0 {
			diagnostics = append(diagnostics, diagnostic(
				"text.invalid_ignored",
				field,
				"ignored non-string text field",
			))
		}
		return "", diagnostics
	}
	normalized, changed, truncated := normalizeResponseText(text, maxBytes)
	if changed {
		diagnostics = append(diagnostics, diagnostic(
			"text.normalized",
			field,
			"removed control characters or surrounding whitespace",
		))
	}
	if truncated {
		diagnostics = append(diagnostics, diagnostic(
			"text.truncated",
			field,
			"truncated text to the local response bound",
		))
	}
	return normalized, diagnostics
}

func normalizeResponseText(value string, maxBytes int) (string, bool, bool) {
	original := value
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	changed := value != original
	if len(value) <= maxBytes {
		return value, changed, false
	}

	var result strings.Builder
	result.Grow(maxBytes)
	for _, r := range value {
		if result.Len()+utf8.RuneLen(r) > maxBytes {
			break
		}
		result.WriteRune(r)
	}
	return strings.TrimSpace(result.String()), true, true
}

func decodePlanObject(raw []byte) (map[string]json.RawMessage, []Diagnostic, error) {
	trimmed := bytes.TrimSpace(raw)
	var object map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &object); err == nil && object != nil {
		return object, []Diagnostic{}, nil
	}

	text := string(trimmed)
	searchFrom := 0
	for {
		openOffset := strings.Index(text[searchFrom:], "```")
		if openOffset < 0 {
			break
		}
		open := searchFrom + openOffset + len("```")
		closeOffset := strings.Index(text[open:], "```")
		if closeOffset < 0 {
			break
		}
		close := open + closeOffset
		candidate := strings.TrimSpace(text[open:close])
		if newline := strings.IndexByte(candidate, '\n'); newline >= 0 {
			language := strings.TrimSpace(candidate[:newline])
			if strings.EqualFold(language, "json") {
				candidate = strings.TrimSpace(candidate[newline+1:])
			}
		}
		object = nil
		if err := json.Unmarshal([]byte(candidate), &object); err == nil && object != nil {
			return object, []Diagnostic{diagnostic(
				"response.fenced_json_accepted",
				"",
				"accepted a JSON object from a markdown fence",
			)}, nil
		}
		searchFrom = close + len("```")
	}
	for index := 0; index < len(text); index++ {
		if text[index] != '{' {
			continue
		}
		decoder := json.NewDecoder(strings.NewReader(text[index:]))
		object = nil
		if err := decoder.Decode(&object); err == nil && object != nil {
			return object, []Diagnostic{diagnostic(
				"response.embedded_json_accepted",
				"",
				"accepted a JSON object embedded in surrounding text",
			)}, nil
		}
	}
	return nil, nil, fmt.Errorf("component study: response contains no recoverable json object")
}

func decodeString(value json.RawMessage) (string, bool) {
	if len(value) == 0 {
		return "", false
	}
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		return "", false
	}
	return text, true
}

func decodeSelectionID(value json.RawMessage) (id string, object bool, ok bool) {
	if id, ok := decodeString(value); ok {
		return id, false, true
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(value, &fields); err != nil || len(fields) != 1 {
		return "", false, false
	}
	id, ok = decodeString(fields["id"])
	return id, true, ok
}

func bundleEvidenceIDs(bundle Bundle) map[string]struct{} {
	ids := make(map[string]struct{}, len(bundle.Anchors)+len(bundle.Files)+len(bundle.Symbols)+len(bundle.Evidence))
	for _, candidate := range bundle.Anchors {
		ids[candidate.ID] = struct{}{}
	}
	for _, candidate := range bundle.Files {
		ids[candidate.ID] = struct{}{}
	}
	for _, candidate := range bundle.Symbols {
		ids[candidate.ID] = struct{}{}
	}
	for _, candidate := range bundle.Evidence {
		ids[candidate.ID] = struct{}{}
	}
	return ids
}

func candidateIDSet[T any](items []T, id func(T) string) map[string]struct{} {
	result := make(map[string]struct{}, len(items))
	for _, item := range items {
		result[id(item)] = struct{}{}
	}
	return result
}

func validateTextValues(field string, values []string) error {
	if len(values) > maxResponseListItems {
		return fmt.Errorf("component study: %s exceeds its item bound", field)
	}
	for _, value := range values {
		if err := validateText(field, value, maxResponseTextBytes); err != nil {
			return err
		}
	}
	return nil
}

func (p Plan) usable() bool {
	return p.Framing != "" || len(p.Questions) > 0 ||
		len(p.SelectedFiles) > 0 || len(p.SelectedSymbols) > 0
}

func diagnostic(code, field, message string) Diagnostic {
	return Diagnostic{Code: code, Field: field, Message: message}
}
