package symbol

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/evidence"
)

type ParseWarning struct {
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

type ParseResult struct {
	Report   Report         `json:"report"`
	Warnings []ParseWarning `json:"warnings"`
	Format   string         `json:"format"`
}

var trailingCommaPattern = regexp.MustCompile(`,\s*([}\]])`)

var reportFields = map[string]struct{}{
	"target": {}, "summary": {}, "responsibility": {}, "callers": {}, "callees": {},
	"files_to_read_in_order": {}, "files_to_read": {}, "tests_to_read": {}, "tests": {},
	"read_evidence_ids": {}, "test_evidence_ids": {},
	"unknowns": {}, "next_queries": {}, "warnings": {},
}

func ParseReport(bundle Bundle, data []byte) (ParseResult, error) {
	object, format, warnings, err := decodeReportObject(data)
	if err != nil {
		return ParseResult{}, err
	}
	for field := range object {
		if _, ok := reportFields[field]; !ok {
			warnings = append(warnings, warning("schema.unknown_field", field, "ignored unknown top-level field"))
		}
	}

	_, compactJSON := object["read_evidence_ids"]
	if _, ok := object["test_evidence_ids"]; ok {
		compactJSON = true
	}

	report := Report{Target: entityRef(bundle.Target.Entity)}
	if rawTarget, ok := object["target"]; ok {
		parsedTarget, targetWarnings := parseEntityRef(rawTarget, "target")
		warnings = append(warnings, targetWarnings...)
		if !sameEntityRef(parsedTarget, report.Target) || parsedTarget.Column != report.Target.Column {
			warnings = append(warnings, warning("target.replaced", "target", "replaced model target with the uniquely resolved bundle target"))
		}
	} else if format == "json" && !compactJSON {
		warnings = append(warnings, warning("target.missing", "target", "filled target from the uniquely resolved bundle target"))
	}

	report.Summary, warnings = parseInterpretationClaim(object["summary"], "summary", bundle, warnings)
	report.Responsibility, warnings = parseInterpretationClaim(object["responsibility"], "responsibility", bundle, warnings)

	if format == "json" && !compactJSON {
		warnings = inspectRelationshipContract(object["callers"], "callers", bundle.IncomingCalls, true, warnings)
		warnings = inspectRelationshipContract(object["callees"], "callees", bundle.OutgoingCalls, false, warnings)
	}
	report.Callers = buildRelationships(bundle.IncomingCalls, true)
	report.Callees = buildRelationships(bundle.OutgoingCalls, false)

	filesValue, hasFiles := object["files_to_read_in_order"]
	if !hasFiles {
		filesValue, hasFiles = object["files_to_read"]
		if hasFiles {
			warnings = append(warnings, warning("files.alias_used", "files_to_read", "accepted files_to_read as files_to_read_in_order"))
		}
	}
	if !hasFiles {
		if evidenceIDs, ok := object["read_evidence_ids"]; ok {
			filesValue = evidenceRecommendationItems(evidenceIDs)
		}
	}
	expectedEvidenceOnly := compactJSON || format == "tagged"
	report.FilesToReadInOrder, warnings = parseFileRecommendations(filesValue, "files_to_read_in_order", false, expectedEvidenceOnly, bundle, warnings)
	report.FilesToReadInOrder = ensureTargetRecommendation(report.FilesToReadInOrder, bundle)

	testsValue, hasTests := object["tests_to_read"]
	if !hasTests {
		testsValue, hasTests = object["tests"]
		if hasTests {
			warnings = append(warnings, warning("tests.alias_used", "tests", "accepted tests as tests_to_read"))
		}
	}
	if !hasTests {
		if evidenceIDs, ok := object["test_evidence_ids"]; ok {
			testsValue = evidenceRecommendationItems(evidenceIDs)
		}
	}
	report.TestsToRead, warnings = parseFileRecommendations(testsValue, "tests_to_read", true, expectedEvidenceOnly, bundle, warnings)
	report.Unknowns, warnings = parseStringList(object["unknowns"], "unknowns", warnings)
	report.Warnings, warnings = parseStringList(object["warnings"], "warnings", warnings)
	report.NextQueries, warnings = parseNextQueries(object["next_queries"], warnings)

	for _, item := range warnings {
		report.Warnings = append(report.Warnings, "parser: "+item.Message)
	}
	report.Warnings = uniqueStrings(report.Warnings)

	if err := ValidateReport(bundle, report); err != nil {
		return ParseResult{}, fmt.Errorf("symbol report: normalized report is invalid: %w", err)
	}
	return ParseResult{Report: report, Warnings: warnings, Format: format}, nil
}

func decodeReportObject(data []byte) (map[string]any, string, []ParseWarning, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, "", nil, fmt.Errorf("symbol report: response is empty")
	}
	if object, err := decodeObject(trimmed); err == nil {
		return object, "json", nil, nil
	}

	recovered := recoverJSONObject(trimmed)
	if len(recovered) == 0 {
		if object, taggedWarnings, ok := decodeTaggedReport(string(trimmed)); ok {
			return object, "tagged", taggedWarnings, nil
		}
		return nil, "", nil, fmt.Errorf("symbol report: no json object or tagged report found in response")
	}
	warnings := []ParseWarning{warning("json.recovered", "", "recovered json object from surrounding text or markdown")}
	if object, err := decodeObject(recovered); err == nil {
		return object, "json", warnings, nil
	}

	withoutTrailingCommas := trailingCommaPattern.ReplaceAll(recovered, []byte("$1"))
	object, err := decodeObject(withoutTrailingCommas)
	if err != nil {
		if object, taggedWarnings, ok := decodeTaggedReport(string(trimmed)); ok {
			warnings = append(warnings, warning("json.recovery_failed", "", "ignored an invalid json-looking fragment and parsed tagged lines"))
			warnings = append(warnings, taggedWarnings...)
			return object, "tagged", warnings, nil
		}
		return nil, "", warnings, fmt.Errorf("symbol report: decode recovered json: %w", err)
	}
	warnings = append(warnings, warning("json.trailing_comma_removed", "", "removed trailing comma from model json"))
	return object, "json", warnings, nil
}

func decodeTaggedReport(text string) (map[string]any, []ParseWarning, bool) {
	values := make(map[string][]string)
	var warnings []ParseWarning
	recognized := 0
	for lineIndex, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || line == "```" || strings.EqualFold(line, "```text") {
			continue
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			warnings = append(warnings, warning("tagged.line_ignored", fmt.Sprintf("line[%d]", lineIndex+1), "ignored line without KEY: VALUE separator"))
			continue
		}
		key = strings.ToUpper(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "SUMMARY", "SUMMARY_EVIDENCE", "SUMMARY_CONFIDENCE",
			"RESPONSIBILITY", "RESPONSIBILITY_EVIDENCE", "RESPONSIBILITY_CONFIDENCE",
			"READ", "TEST", "UNKNOWN", "NEXT_QUERY", "WARNING":
			values[key] = append(values[key], value)
			recognized++
		default:
			warnings = append(warnings, warning("tagged.key_ignored", key, "ignored unknown tagged key"))
		}
	}
	if recognized == 0 {
		return nil, nil, false
	}
	object := map[string]any{
		"summary": map[string]any{
			"statement":    firstTagged(values, "SUMMARY"),
			"basis":        "inference",
			"evidence_ids": splitTaggedList(firstTagged(values, "SUMMARY_EVIDENCE")),
			"confidence":   firstTagged(values, "SUMMARY_CONFIDENCE"),
		},
		"responsibility": map[string]any{
			"statement":    firstTagged(values, "RESPONSIBILITY"),
			"basis":        "inference",
			"evidence_ids": splitTaggedList(firstTagged(values, "RESPONSIBILITY_EVIDENCE")),
			"confidence":   firstTagged(values, "RESPONSIBILITY_CONFIDENCE"),
		},
		"files_to_read_in_order": taggedEvidenceItems(values["READ"]),
		"tests_to_read":          taggedEvidenceItems(values["TEST"]),
		"unknowns":               stringItems(values["UNKNOWN"]),
		"warnings":               stringItems(values["WARNING"]),
	}
	var nextQueries []any
	for _, value := range values["NEXT_QUERY"] {
		query, reason, found := strings.Cut(value, "||")
		if !found {
			nextQueries = append(nextQueries, strings.TrimSpace(value))
			continue
		}
		nextQueries = append(nextQueries, map[string]any{"query": strings.TrimSpace(query), "reason": strings.TrimSpace(reason)})
	}
	object["next_queries"] = nextQueries
	return object, warnings, true
}

func firstTagged(values map[string][]string, key string) string {
	if len(values[key]) == 0 {
		return ""
	}
	return values[key][0]
}

func splitTaggedList(value string) []any {
	var items []any
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}
	return items
}

func taggedEvidenceItems(values []string) []any {
	items := make([]any, 0, len(values))
	for _, value := range values {
		items = append(items, map[string]any{"evidence_ids": splitTaggedList(value)})
	}
	return items
}

func evidenceRecommendationItems(value any) []any {
	if items, ok := arrayValue(value); ok {
		result := make([]any, 0, len(items))
		for _, item := range items {
			result = append(result, map[string]any{"evidence_ids": []any{item}})
		}
		return result
	}
	return []any{map[string]any{"evidence_ids": []any{value}}}
}

func stringItems(values []string) []any {
	items := make([]any, 0, len(values))
	for _, value := range values {
		items = append(items, value)
	}
	return items
}

func decodeObject(data []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("top-level json value is not an object")
	}
	return object, nil
}

func recoverJSONObject(data []byte) []byte {
	text := strings.TrimSpace(string(data))
	if strings.HasPrefix(text, "```") {
		if newline := strings.IndexByte(text, '\n'); newline >= 0 {
			text = text[newline+1:]
		}
		if fence := strings.LastIndex(text, "```"); fence >= 0 {
			text = text[:fence]
		}
	}
	start := strings.IndexByte(text, '{')
	end := strings.LastIndexByte(text, '}')
	if start < 0 || end <= start {
		return nil
	}
	return []byte(text[start : end+1])
}

func parseInterpretationClaim(value any, field string, bundle Bundle, warnings []ParseWarning) (Claim, []ParseWarning) {
	claim := Claim{Basis: BasisInference, Confidence: 0.5}
	if text, ok := stringValue(value); ok {
		claim.Statement = text
		warnings = append(warnings, warning(field+".string_accepted", field, "accepted string as claim statement"))
	} else if object, ok := objectValue(value); ok {
		claim.Statement, _ = firstString(object, "statement", "summary", "description", "text")
		basis, _ := firstString(object, "basis")
		if basis != "" && basis != string(BasisInference) {
			warnings = append(warnings, warning(field+".basis_normalized", field+".basis", "normalized interpretation basis to inference"))
		}
		if confidence, ok := numberValue(object["confidence"]); ok {
			claim.Confidence = confidence
		} else if _, exists := object["confidence"]; exists {
			warnings = append(warnings, warning(field+".confidence_defaulted", field+".confidence", "replaced invalid confidence with 0.5"))
		}
		claim.EvidenceIDs, warnings = parseEvidenceIDs(object["evidence_ids"], field+".evidence_ids", bundle, warnings)
	} else {
		warnings = append(warnings, warning(field+".missing", field, "filled missing interpretation with a low-confidence fallback"))
	}

	if strings.TrimSpace(claim.Statement) == "" {
		claim.Statement = "The model did not provide a usable " + field + "."
		claim.Confidence = 0
		warnings = append(warnings, warning(field+".statement_defaulted", field+".statement", "filled missing claim statement"))
	}
	if claim.Confidence < 0 {
		claim.Confidence = 0
		warnings = append(warnings, warning(field+".confidence_clamped", field+".confidence", "clamped confidence to 0"))
	}
	if claim.Confidence > 0.75 {
		claim.Confidence = 0.75
		warnings = append(warnings, warning(field+".confidence_capped", field+".confidence", "capped static-only inference confidence at 0.75"))
	}
	if len(claim.EvidenceIDs) == 0 {
		claim.EvidenceIDs = []string{bundle.Target.EvidenceID}
		warnings = append(warnings, warning(field+".evidence_defaulted", field+".evidence_ids", "grounded claim with target resolution evidence"))
	}
	return claim, warnings
}

func inspectRelationshipContract(value any, field string, calls []CallFact, incoming bool, warnings []ParseWarning) []ParseWarning {
	items, ok := arrayValue(value)
	if !ok {
		return append(warnings, warning(field+".rebuilt", field, "rebuilt structural relationships from the local bundle"))
	}
	if len(items) != len(calls) {
		warnings = append(warnings, warning(field+".count_repaired", field, fmt.Sprintf("rebuilt %s to match %d local call facts", field, len(calls))))
	}
	expected := make(map[string]CallFact, len(calls))
	for _, call := range calls {
		expected[call.EvidenceID] = call
	}
	for i, item := range items {
		object, ok := objectValue(item)
		if !ok {
			warnings = append(warnings, warning(field+".item_ignored", fmt.Sprintf("%s[%d]", field, i), "ignored non-object relationship"))
			continue
		}
		ids, nextWarnings := parseEvidenceIDs(object["evidence_ids"], fmt.Sprintf("%s[%d].evidence_ids", field, i), bundleFromCalls(calls), warnings)
		warnings = nextWarnings
		var matched *CallFact
		for _, id := range ids {
			if call, exists := expected[id]; exists {
				copy := call
				matched = &copy
				break
			}
		}
		if matched == nil {
			warnings = append(warnings, warning(field+".item_rebuilt", fmt.Sprintf("%s[%d]", field, i), "relationship did not cite a matching local call fact"))
			continue
		}
		parsed, entityWarnings := parseEntityRef(object["symbol"], fmt.Sprintf("%s[%d].symbol", field, i))
		warnings = append(warnings, entityWarnings...)
		expectedEntity := matched.Callee
		if incoming {
			expectedEntity = matched.Caller
		}
		if !sameEntityRef(parsed, entityRef(expectedEntity)) {
			warnings = append(warnings, warning(field+".identity_repaired", fmt.Sprintf("%s[%d].symbol", field, i), "replaced relationship symbol with local call fact identity"))
		}
	}
	return warnings
}

// bundleFromCalls is only used to filter evidence IDs while inspecting a
// relationship array. Structural output is rebuilt from the original bundle.
func bundleFromCalls(calls []CallFact) Bundle {
	bundle := Bundle{}
	for _, call := range calls {
		if strings.HasPrefix(call.EvidenceID, "call-in-") {
			bundle.IncomingCalls = append(bundle.IncomingCalls, call)
		} else {
			bundle.OutgoingCalls = append(bundle.OutgoingCalls, call)
		}
	}
	return bundle
}

func buildRelationships(calls []CallFact, incoming bool) []Relationship {
	relationships := make([]Relationship, 0, len(calls))
	for _, call := range calls {
		entity := call.Callee
		statement := "is statically called by the target under the stated build scenario"
		if incoming {
			entity = call.Caller
			statement = "statically calls the target under the stated build scenario"
		}
		relationships = append(relationships, Relationship{
			Symbol:       entityRef(entity),
			Relationship: statement,
			Basis:        BasisStaticFact,
			EvidenceIDs:  []string{call.EvidenceID},
			Confidence:   1,
		})
	}
	return relationships
}

func parseFileRecommendations(value any, field string, requireTest, evidenceOnlyContract bool, bundle Bundle, warnings []ParseWarning) ([]FileRecommendation, []ParseWarning) {
	items, ok := arrayValue(value)
	if !ok {
		if value != nil {
			items = []any{value}
			warnings = append(warnings, warning(field+".scalar_accepted", field, "accepted scalar as one file recommendation"))
		} else {
			return nil, warnings
		}
	}
	var recommendations []FileRecommendation
	seen := make(map[string]struct{})
	for i, item := range items {
		path := ""
		role := ""
		line := 0
		var ids []string
		if text, ok := stringValue(item); ok {
			path = text
			warnings = append(warnings, warning(field+".string_accepted", fmt.Sprintf("%s[%d]", field, i), "accepted string as file path"))
		} else if object, ok := objectValue(item); ok {
			path, _ = firstString(object, "path", "file")
			role, _ = firstString(object, "structural_role", "role", "kind")
			line, _ = intValue(object["line"])
			ids, warnings = parseEvidenceIDs(object["evidence_ids"], fmt.Sprintf("%s[%d].evidence_ids", field, i), bundle, warnings)
		} else {
			warnings = append(warnings, warning(field+".item_dropped", fmt.Sprintf("%s[%d]", field, i), "dropped non-string and non-object file recommendation"))
			continue
		}
		path = strings.TrimSpace(path)
		if path == "" && len(ids) > 0 {
			if inferredPath, inferredRole, inferredLine, ok := inferPathFromEvidence(bundle, ids, requireTest); ok {
				path, role, line = inferredPath, inferredRole, inferredLine
				if !evidenceOnlyContract {
					warnings = append(warnings, warning(field+".path_inferred", fmt.Sprintf("%s[%d].path", field, i), "inferred file path from evidence id"))
				}
			}
		}
		if !containsString(bundle.AllowedPaths, path) {
			warnings = append(warnings, warning(field+".path_dropped", fmt.Sprintf("%s[%d].path", field, i), "dropped path outside allowed_paths"))
			continue
		}
		if requireTest && !strings.HasSuffix(path, "_test.go") {
			warnings = append(warnings, warning(field+".non_test_dropped", fmt.Sprintf("%s[%d].path", field, i), "dropped non-test path from tests_to_read"))
			continue
		}
		inferredRole, inferredID, inferredLine, ok := inferFileRole(bundle, path, role, ids, requireTest)
		if !ok {
			warnings = append(warnings, warning(field+".unsupported_dropped", fmt.Sprintf("%s[%d]", field, i), "dropped recommendation not supported by local evidence"))
			continue
		}
		if role != inferredRole && !evidenceOnlyContract {
			warnings = append(warnings, warning(field+".role_inferred", fmt.Sprintf("%s[%d].structural_role", field, i), "inferred structural role from local evidence"))
		}
		if !containsString(ids, inferredID) && !evidenceOnlyContract {
			warnings = append(warnings, warning(field+".evidence_inferred", fmt.Sprintf("%s[%d].evidence_ids", field, i), "inferred evidence id from local evidence"))
		}
		if line <= 0 {
			line = inferredLine
		}
		key := path + "\x00" + inferredRole
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		recommendations = append(recommendations, FileRecommendation{
			Path:           path,
			Line:           line,
			StructuralRole: inferredRole,
			EvidenceIDs:    []string{inferredID},
		})
	}
	return recommendations, warnings
}

func inferPathFromEvidence(bundle Bundle, ids []string, requireTest bool) (string, string, int, bool) {
	for _, id := range ids {
		if id == bundle.Target.EvidenceID && bundle.Target.Entity.Location != nil {
			location := bundle.Target.Entity.Location
			if !requireTest || strings.HasSuffix(location.Path, "_test.go") {
				role := "target"
				if requireTest {
					role = "test_reference"
				}
				return location.Path, role, location.Line, true
			}
		}
		for _, call := range bundle.IncomingCalls {
			if call.EvidenceID != id || call.Caller.Location == nil {
				continue
			}
			location := call.Caller.Location
			if requireTest && !strings.HasSuffix(location.Path, "_test.go") {
				continue
			}
			role := "static_caller"
			if requireTest {
				role = "test_reference"
			}
			return location.Path, role, location.Line, true
		}
		for _, call := range bundle.OutgoingCalls {
			if call.EvidenceID != id || call.Callee.Location == nil {
				continue
			}
			location := call.Callee.Location
			if requireTest && !strings.HasSuffix(location.Path, "_test.go") {
				continue
			}
			role := "static_callee"
			if requireTest {
				role = "test_reference"
			}
			return location.Path, role, location.Line, true
		}
		for _, candidate := range bundle.Candidates {
			if candidate.EvidenceID != id || candidate.Entity.Location == nil {
				continue
			}
			location := candidate.Entity.Location
			if requireTest && !strings.HasSuffix(location.Path, "_test.go") {
				continue
			}
			role := "candidate"
			if requireTest {
				role = "test_reference"
			}
			return location.Path, role, location.Line, true
		}
	}
	return "", "", 0, false
}

func inferFileRole(bundle Bundle, path, requestedRole string, ids []string, requireTest bool) (string, string, int, bool) {
	type choice struct {
		role string
		id   string
		line int
	}
	var choices []choice
	add := func(role, id string, location *evidence.Location) {
		if location == nil || location.Path != path {
			return
		}
		if requireTest {
			role = "test_reference"
		}
		choices = append(choices, choice{role: role, id: id, line: location.Line})
	}
	add("target", bundle.Target.EvidenceID, bundle.Target.Entity.Location)
	for _, call := range bundle.IncomingCalls {
		add("static_caller", call.EvidenceID, call.Caller.Location)
		add("target", call.EvidenceID, call.Callee.Location)
		add("callsite", call.EvidenceID, call.Callsite)
	}
	for _, call := range bundle.OutgoingCalls {
		add("target", call.EvidenceID, call.Caller.Location)
		add("static_callee", call.EvidenceID, call.Callee.Location)
		add("callsite", call.EvidenceID, call.Callsite)
	}
	for _, candidate := range bundle.Candidates {
		add("candidate", candidate.EvidenceID, candidate.Entity.Location)
	}
	for _, candidate := range choices {
		if requestedRole == candidate.role && containsString(ids, candidate.id) {
			return candidate.role, candidate.id, candidate.line, true
		}
	}
	for _, candidate := range choices {
		if containsString(ids, candidate.id) {
			return candidate.role, candidate.id, candidate.line, true
		}
	}
	if len(choices) > 0 {
		return choices[0].role, choices[0].id, choices[0].line, true
	}
	return "", "", 0, false
}

func ensureTargetRecommendation(files []FileRecommendation, bundle Bundle) []FileRecommendation {
	path := bundle.Target.Entity.Location.Path
	for _, file := range files {
		if file.Path == path && file.StructuralRole == "target" {
			return files
		}
	}
	target := FileRecommendation{
		Path:           path,
		Line:           bundle.Target.Entity.Location.Line,
		StructuralRole: "target",
		EvidenceIDs:    []string{bundle.Target.EvidenceID},
	}
	return append([]FileRecommendation{target}, files...)
}

func parseNextQueries(value any, warnings []ParseWarning) ([]NextQuery, []ParseWarning) {
	items, ok := arrayValue(value)
	if !ok {
		if value == nil {
			return nil, warnings
		}
		items = []any{value}
		warnings = append(warnings, warning("next_queries.scalar_accepted", "next_queries", "accepted scalar as one next query"))
	}
	var queries []NextQuery
	for i, item := range items {
		if text, ok := stringValue(item); ok {
			queries = append(queries, NextQuery{Query: text, Reason: "suggested by the model"})
			warnings = append(warnings, warning("next_queries.string_accepted", fmt.Sprintf("next_queries[%d]", i), "accepted string as next query"))
			continue
		}
		object, ok := objectValue(item)
		if !ok {
			warnings = append(warnings, warning("next_queries.item_dropped", fmt.Sprintf("next_queries[%d]", i), "dropped invalid next query"))
			continue
		}
		query, _ := firstString(object, "query", "search", "symbol")
		reason, _ := firstString(object, "reason", "why", "description")
		if query == "" {
			warnings = append(warnings, warning("next_queries.item_dropped", fmt.Sprintf("next_queries[%d]", i), "dropped next query without query text"))
			continue
		}
		if reason == "" {
			reason = "suggested by the model"
			warnings = append(warnings, warning("next_queries.reason_defaulted", fmt.Sprintf("next_queries[%d].reason", i), "filled missing next query reason"))
		}
		queries = append(queries, NextQuery{Query: query, Reason: reason})
	}
	return queries, warnings
}

func parseEntityRef(value any, field string) (EntityRef, []ParseWarning) {
	object, ok := objectValue(value)
	if !ok {
		return EntityRef{}, []ParseWarning{warning("entity.invalid", field, "could not parse entity object")}
	}
	name, _ := firstString(object, "name", "symbol")
	kind, _ := firstString(object, "kind", "type")
	path, _ := firstString(object, "path", "file")
	line, _ := intValue(object["line"])
	column, _ := intValue(object["column"])
	return EntityRef{Name: name, Kind: kind, Path: path, Line: line, Column: column}, nil
}

func parseEvidenceIDs(value any, field string, bundle Bundle, warnings []ParseWarning) ([]string, []ParseWarning) {
	values, nextWarnings := parseStringList(value, field, warnings)
	known := evidenceCertainties(bundle)
	var result []string
	for _, id := range values {
		if _, ok := known[id]; !ok {
			nextWarnings = append(nextWarnings, warning("evidence.unknown_dropped", field, "dropped unknown evidence id "+strconv.Quote(id)))
			continue
		}
		result = append(result, id)
	}
	return uniqueStrings(result), nextWarnings
}

func parseStringList(value any, field string, warnings []ParseWarning) ([]string, []ParseWarning) {
	if value == nil {
		return nil, warnings
	}
	if text, ok := stringValue(value); ok {
		return []string{text}, append(warnings, warning(field+".string_accepted", field, "accepted string as one list item"))
	}
	items, ok := arrayValue(value)
	if !ok {
		return nil, append(warnings, warning(field+".invalid_ignored", field, "ignored non-string and non-array value"))
	}
	var result []string
	for i, item := range items {
		if text, ok := stringValue(item); ok {
			result = append(result, text)
			continue
		}
		if object, ok := objectValue(item); ok {
			if text, ok := firstString(object, "id", "evidence_id", "value", "text", "message"); ok {
				result = append(result, text)
				warnings = append(warnings, warning(field+".object_accepted", fmt.Sprintf("%s[%d]", field, i), "accepted object list item as string"))
				continue
			}
		}
		warnings = append(warnings, warning(field+".item_ignored", fmt.Sprintf("%s[%d]", field, i), "ignored non-string list item"))
	}
	return uniqueStrings(result), warnings
}

func stringValue(value any) (string, bool) {
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	text = strings.TrimSpace(text)
	return text, text != ""
}

func objectValue(value any) (map[string]any, bool) {
	object, ok := value.(map[string]any)
	return object, ok
}

func arrayValue(value any) ([]any, bool) {
	items, ok := value.([]any)
	return items, ok
}

func firstString(object map[string]any, keys ...string) (string, bool) {
	for _, key := range keys {
		if text, ok := stringValue(object[key]); ok {
			return text, true
		}
	}
	return "", false
}

func numberValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case json.Number:
		number, err := typed.Float64()
		return number, err == nil
	case float64:
		return typed, true
	case string:
		text := strings.TrimSpace(typed)
		percentage := strings.HasSuffix(text, "%")
		text = strings.TrimSuffix(text, "%")
		number, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return 0, false
		}
		if percentage {
			number /= 100
		}
		return number, true
	default:
		return 0, false
	}
}

func intValue(value any) (int, bool) {
	number, ok := numberValue(value)
	if !ok {
		return 0, false
	}
	return int(number), true
}

func warning(code, field, message string) ParseWarning {
	return ParseWarning{Code: code, Field: field, Message: message}
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sortedWarningCodes(warnings []ParseWarning) []string {
	codes := make([]string, 0, len(warnings))
	for _, item := range warnings {
		codes = append(codes, item.Code)
	}
	sort.Strings(codes)
	return codes
}
