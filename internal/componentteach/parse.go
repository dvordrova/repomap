package componentteach

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

type sectionSpec struct {
	field string
	slug  string
	set   func(*Report, []Item)
}

var reportSections = []sectionSpec{
	{"mental_model", "mental-model", func(report *Report, items []Item) { report.MentalModel = items }},
	{"lifecycle_steps", "lifecycle-step", func(report *Report, items []Item) { report.LifecycleSteps = items }},
	{"boundaries", "boundary", func(report *Report, items []Item) { report.Boundaries = items }},
	{"design_notes", "design-note", func(report *Report, items []Item) { report.DesignNotes = items }},
	{"failures_and_observability", "failure-observability", func(report *Report, items []Item) { report.FailuresAndObservability = items }},
	{"tests_and_checks", "test-check", func(report *Report, items []Item) { report.TestsAndChecks = items }},
	{"unknowns", "unknown", func(report *Report, items []Item) { report.Unknowns = items }},
	{"next_dive", "next-dive", func(report *Report, items []Item) { report.NextDive = items }},
}

// ParseReport tolerates common weak-model envelope and singleton drift while
// keeping every surviving statement anchored to locally known evidence.
func ParseReport(bundle Bundle, raw []byte) (ParseResult, error) {
	if err := bundle.Validate(); err != nil {
		return ParseResult{}, err
	}
	object, diagnostics, err := decodeObject(raw)
	if err != nil {
		return ParseResult{}, err
	}
	knownFields := map[string]struct{}{"version": {}, "primary_question_id": {}}
	for _, section := range reportSections {
		knownFields[section.field] = struct{}{}
	}
	unknownFields := make([]string, 0)
	for field := range object {
		if _, exists := knownFields[field]; !exists {
			unknownFields = append(unknownFields, field)
		}
	}
	sort.Strings(unknownFields)
	for _, field := range unknownFields {
		diagnostics = append(diagnostics, diag("field.unknown_ignored", field, "ignored unknown response field"))
	}

	report := Report{
		Version:           ReportVersion,
		PrimaryQuestionID: bundle.PrimaryQuestion.ID,
		MentalModel:       []Item{}, LifecycleSteps: []Item{}, Boundaries: []Item{}, DesignNotes: []Item{},
		FailuresAndObservability: []Item{}, TestsAndChecks: []Item{}, Unknowns: []Item{}, NextDive: []Item{},
	}
	if value, exists := object["version"]; exists {
		var version int
		if err := json.Unmarshal(value, &version); err != nil || version != ReportVersion {
			diagnostics = append(diagnostics, diag("version.repaired", "version", "replaced unsupported response version with the local report version"))
		}
	}
	if id, ok := decodeString(object["primary_question_id"]); !ok || strings.TrimSpace(id) != bundle.PrimaryQuestion.ID {
		diagnostics = append(diagnostics, diag("primary_question.repaired", "primary_question_id", "bound the report to the original primary question"))
	}
	knownEvidence := citableEvidenceIDs(bundle)
	evidenceByID := make(map[string]EvidenceItem, len(bundle.Evidence))
	for _, item := range bundle.Evidence {
		if item.Kind != EvidenceOrientationNote {
			evidenceByID[item.ID] = item
		}
	}
	knownFrontier := make(map[string]struct{}, len(bundle.UnresolvedFrontierIDs))
	for _, id := range bundle.UnresolvedFrontierIDs {
		knownFrontier[id] = struct{}{}
	}
	for _, section := range reportSections {
		var items []Item
		items, diagnostics = parseSection(section, object[section.field], knownEvidence, evidenceByID, knownFrontier, diagnostics)
		section.set(&report, items)
	}
	if !report.usable() {
		return ParseResult{}, fmt.Errorf("component teach: response contains no grounded explanation or grounded unknown")
	}
	if err := report.Validate(bundle); err != nil {
		return ParseResult{}, err
	}
	return ParseResult{Report: report, Diagnostics: diagnostics}, nil
}

func parseSection(
	section sectionSpec,
	value json.RawMessage,
	knownEvidence map[string]struct{},
	evidenceByID map[string]EvidenceItem,
	knownFrontier map[string]struct{},
	diagnostics []Diagnostic,
) ([]Item, []Diagnostic) {
	rawItems, diagnostics := responseObjects(value, section.field, diagnostics)
	items := make([]Item, 0, min(len(rawItems), maxItems))
	for index, raw := range rawItems {
		field := fmt.Sprintf("%s[%d]", section.field, index)
		var object map[string]json.RawMessage
		if err := json.Unmarshal(raw, &object); err != nil || object == nil {
			diagnostics = append(diagnostics, diag("item.invalid_dropped", field, "dropped malformed section item"))
			continue
		}
		if _, supplied := object["id"]; supplied {
			diagnostics = append(diagnostics, diag("item.model_id_ignored", field+".id", "ignored model-provided item id; ids are assigned locally"))
		}
		text, textDiagnostics := parseText(object["text"], field+".text")
		diagnostics = append(diagnostics, textDiagnostics...)
		if text == "" {
			diagnostics = append(diagnostics, diag("item.text_missing_dropped", field, "dropped item without usable text"))
			continue
		}
		evidenceIDs, idDiagnostics := parseKnownIDs(object["evidence_ids"], field+".evidence_ids", knownEvidence, 12)
		diagnostics = append(diagnostics, idDiagnostics...)
		frontierIDs, frontierDiagnostics := parseKnownIDs(object["frontier_ids"], field+".frontier_ids", knownFrontier, 6)
		diagnostics = append(diagnostics, frontierDiagnostics...)
		if len(evidenceIDs) == 0 {
			diagnostics = append(diagnostics, diag("item.ungrounded_dropped", field+".evidence_ids", "dropped item without a known anchor evidence id"))
			continue
		}
		if section.field != "unknowns" && section.field != "next_dive" &&
			unsupportedClosedWorldClaim(text, evidenceIDs, evidenceByID) {
			diagnostics = append(diagnostics, diag(
				"claim.closed_world_dropped",
				field+".text",
				"dropped an unqualified absence or exclusivity claim grounded only in bounded navigation evidence",
			))
			continue
		}
		if len(items) >= maxItems {
			diagnostics = append(diagnostics, diag("section.limit_dropped", field, "dropped item beyond the section limit"))
			continue
		}
		items = append(items, Item{
			ID:   fmt.Sprintf("item-%s-%03d", section.slug, len(items)+1),
			Text: text, EvidenceIDs: evidenceIDs, FrontierIDs: frontierIDs,
		})
	}
	return items, diagnostics
}

func unsupportedClosedWorldClaim(text string, evidenceIDs []string, evidenceByID map[string]EvidenceItem) bool {
	lower := " " + strings.ToLower(strings.TrimSpace(text)) + " "
	for _, qualifier := range []string{" supplied ", " bounded ", " reported ", " shown ", " provided "} {
		if strings.Contains(lower, qualifier) {
			return false
		}
	}
	absence := false
	for _, phrase := range []string{
		" does not ", " doesn't ", " never ", " not in the ",
		" only used ", " used only ", " sole caller ", " only caller ",
	} {
		if strings.Contains(lower, phrase) {
			absence = true
			break
		}
	}
	if !absence {
		return false
	}
	for _, id := range evidenceIDs {
		if evidenceByID[id].SupportBasis == SupportSource {
			return false
		}
	}
	return true
}

func responseObjects(value json.RawMessage, field string, diagnostics []Diagnostic) ([]json.RawMessage, []Diagnostic) {
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return []json.RawMessage{}, diagnostics
	}
	var items []json.RawMessage
	if err := json.Unmarshal(trimmed, &items); err == nil {
		return items, diagnostics
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &object); err == nil && object != nil {
		diagnostics = append(diagnostics, diag("array.singleton_object_accepted", field, "accepted one object as a singleton section array"))
		return []json.RawMessage{append(json.RawMessage(nil), trimmed...)}, diagnostics
	}
	diagnostics = append(diagnostics, diag("array.invalid_ignored", field, "ignored malformed section array"))
	return []json.RawMessage{}, diagnostics
}

func parseKnownIDs(value json.RawMessage, field string, known map[string]struct{}, limit int) ([]string, []Diagnostic) {
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return []string{}, nil
	}
	var rawIDs []json.RawMessage
	if err := json.Unmarshal(trimmed, &rawIDs); err != nil {
		if _, ok := decodeString(trimmed); ok {
			rawIDs = []json.RawMessage{append(json.RawMessage(nil), trimmed...)}
		} else {
			return []string{}, []Diagnostic{diag("ids.invalid_ignored", field, "ignored malformed id list")}
		}
		return parseKnownIDItems(rawIDs, field, known, limit, []Diagnostic{
			diag("array.scalar_accepted", field, "accepted scalar id as a singleton list"),
		})
	}
	return parseKnownIDItems(rawIDs, field, known, limit, nil)
}

func parseKnownIDItems(rawIDs []json.RawMessage, field string, known map[string]struct{}, limit int, diagnostics []Diagnostic) ([]string, []Diagnostic) {
	ids := make([]string, 0, min(len(rawIDs), limit))
	seen := make(map[string]struct{}, len(rawIDs))
	for index, raw := range rawIDs {
		id, ok := decodeString(raw)
		id = strings.TrimSpace(id)
		itemField := fmt.Sprintf("%s[%d]", field, index)
		if !ok || id == "" {
			diagnostics = append(diagnostics, diag("id.non_string_dropped", itemField, "dropped non-string id"))
			continue
		}
		if _, exists := known[id]; !exists {
			diagnostics = append(diagnostics, diag("id.unknown_dropped", itemField, "dropped id absent from the bounded teacher bundle"))
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		if len(ids) >= limit {
			diagnostics = append(diagnostics, diag("ids.limit_dropped", itemField, "dropped id beyond the field limit"))
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, diagnostics
}

func parseText(value json.RawMessage, field string) (string, []Diagnostic) {
	text, ok := decodeString(value)
	if !ok {
		return "", nil
	}
	original := text
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)
	text = strings.Join(strings.Fields(text), " ")
	diagnostics := []Diagnostic{}
	if text != original {
		diagnostics = append(diagnostics, diag("text.normalized", field, "normalized whitespace or control characters"))
	}
	if len(text) > maxItemText {
		text = truncateUTF8(text, maxItemText)
		text = strings.TrimSpace(text)
		diagnostics = append(diagnostics, diag("text.truncated", field, "truncated item text to the local bound"))
	}
	return text, diagnostics
}

func decodeObject(raw []byte) (map[string]json.RawMessage, []Diagnostic, error) {
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
		open := searchFrom + openOffset + 3
		closeOffset := strings.Index(text[open:], "```")
		if closeOffset < 0 {
			break
		}
		close := open + closeOffset
		candidate := strings.TrimSpace(text[open:close])
		if newline := strings.IndexByte(candidate, '\n'); newline >= 0 && strings.EqualFold(strings.TrimSpace(candidate[:newline]), "json") {
			candidate = strings.TrimSpace(candidate[newline+1:])
		}
		object = nil
		if err := json.Unmarshal([]byte(candidate), &object); err == nil && object != nil {
			return object, []Diagnostic{diag("response.fenced_json_accepted", "", "accepted a JSON object from a markdown fence")}, nil
		}
		searchFrom = close + 3
	}
	for index := 0; index < len(text); index++ {
		if text[index] != '{' {
			continue
		}
		decoder := json.NewDecoder(strings.NewReader(text[index:]))
		object = nil
		if err := decoder.Decode(&object); err == nil && object != nil {
			return object, []Diagnostic{diag("response.embedded_json_accepted", "", "accepted a JSON object embedded in surrounding text")}, nil
		}
	}
	return nil, nil, fmt.Errorf("component teach: response contains no recoverable json object")
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

func (r Report) Validate(bundle Bundle) error {
	if err := bundle.Validate(); err != nil {
		return err
	}
	if r.Version != ReportVersion || r.PrimaryQuestionID != bundle.PrimaryQuestion.ID {
		return fmt.Errorf("component teach: report is not bound to the primary question")
	}
	knownEvidence := citableEvidenceIDs(bundle)
	knownFrontier := make(map[string]struct{}, len(bundle.UnresolvedFrontierIDs))
	for _, id := range bundle.UnresolvedFrontierIDs {
		knownFrontier[id] = struct{}{}
	}
	seen := make(map[string]struct{})
	sections := [][]Item{r.MentalModel, r.LifecycleSteps, r.Boundaries, r.DesignNotes, r.FailuresAndObservability, r.TestsAndChecks, r.Unknowns, r.NextDive}
	for _, items := range sections {
		if len(items) > maxItems {
			return fmt.Errorf("component teach: report section exceeds item bound")
		}
		for _, item := range items {
			if item.ID == "" || strings.TrimSpace(item.Text) == "" || len(item.Text) > maxItemText || len(item.EvidenceIDs) == 0 {
				return fmt.Errorf("component teach: report contains an ungrounded item")
			}
			if _, exists := seen[item.ID]; exists {
				return fmt.Errorf("component teach: report repeats a local item id")
			}
			seen[item.ID] = struct{}{}
			for _, id := range item.EvidenceIDs {
				if _, exists := knownEvidence[id]; !exists {
					return fmt.Errorf("component teach: report references unknown evidence")
				}
			}
			for _, id := range item.FrontierIDs {
				if _, exists := knownFrontier[id]; !exists {
					return fmt.Errorf("component teach: report references unknown frontier")
				}
			}
		}
	}
	if !r.usable() {
		return fmt.Errorf("component teach: report has no grounded explanation or unknown")
	}
	return nil
}

func (r Report) usable() bool {
	explanations := len(r.MentalModel) + len(r.LifecycleSteps) + len(r.Boundaries) + len(r.DesignNotes) + len(r.FailuresAndObservability) + len(r.TestsAndChecks)
	return explanations > 0 || len(r.Unknowns) > 0
}

func citableEvidenceIDs(bundle Bundle) map[string]struct{} {
	known := make(map[string]struct{}, len(bundle.Evidence))
	for _, item := range bundle.Evidence {
		// Orientation is model context, not repository support. Build currently
		// emits none; filtering here keeps manually assembled bundles honest too.
		if item.Kind == EvidenceOrientationNote {
			continue
		}
		known[item.ID] = struct{}{}
	}
	return known
}

func diag(code, field, message string) Diagnostic {
	return Diagnostic{Code: code, Field: field, Message: message}
}
