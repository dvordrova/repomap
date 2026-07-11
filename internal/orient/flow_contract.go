package orient

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/flowexplain"
)

type flowReportFields struct {
	Summary            string             `json:"summary"`
	Confidence         float64            `json:"confidence"`
	FilesToReadInOrder fileToOpenList     `json:"files_to_read_in_order"`
	TestsToRead        fileToOpenList     `json:"tests_to_read"`
	LikelyChain        []flowChainStep    `json:"likely_chain"`
	UnverifiedPaths    unverifiedPathList `json:"unverified_paths"`
	Unknowns           []string           `json:"unknowns"`
	NextQueries        []string           `json:"next_queries,omitempty"`
	Warnings           []string           `json:"warnings"`
}

type flowReportWire struct {
	Summary            string              `json:"summary"`
	Confidence         *float64            `json:"confidence"`
	FilesToReadInOrder fileToOpenList      `json:"files_to_read_in_order"`
	TestsToRead        fileToOpenList      `json:"tests_to_read"`
	LikelyChain        []flowChainStepWire `json:"likely_chain"`
	UnverifiedPaths    unverifiedPathList  `json:"unverified_paths"`
	Unknowns           flexibleStrings     `json:"unknowns"`
	NextQueries        flexibleStrings     `json:"next_queries"`
	Warnings           flexibleStrings     `json:"warnings"`
}

type flowChainStep struct {
	Step          int      `json:"step"`
	Name          string   `json:"name"`
	WhatHappens   string   `json:"what_happens"`
	EvidenceFiles []string `json:"evidence_files"`
	Confidence    float64  `json:"confidence"`
}

type flowChainStepWire struct {
	Step          flexibleInt `json:"step"`
	Name          string      `json:"name"`
	WhatHappens   string      `json:"what_happens"`
	Description   string      `json:"description"`
	Reason        string      `json:"reason"`
	Role          string      `json:"role"`
	Function      string      `json:"function"`
	File          string      `json:"file"`
	EvidenceFiles pathList    `json:"evidence_files"`
	Confidence    float64     `json:"confidence"`
}

type pathList []string

func (paths *pathList) UnmarshalJSON(data []byte) error {
	decoded, err := decodePathItems(data)
	if err != nil {
		return err
	}
	*paths = decoded
	return nil
}

type flexibleInt int

func (value *flexibleInt) UnmarshalJSON(data []byte) error {
	var number int
	if err := json.Unmarshal(data, &number); err == nil {
		*value = flexibleInt(number)
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("step must be an integer or string")
	}
	text = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(text, "Step "), "step "))
	number, err := strconv.Atoi(text)
	if err != nil {
		*value = 0
		return nil
	}
	*value = flexibleInt(number)
	return nil
}

type flexibleStrings struct {
	Values   []string
	Repaired bool
}

func (values *flexibleStrings) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	var direct []string
	if err := json.Unmarshal(data, &direct); err == nil {
		values.Values = compactStrings(direct)
		return nil
	}
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		values.Values = compactStrings([]string{single})
		values.Repaired = true
		return nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err == nil {
		for _, item := range items {
			text, ok := flexibleObjectText(item)
			if !ok {
				return fmt.Errorf("expected strings or text-like objects")
			}
			values.Values = append(values.Values, text)
		}
		values.Values = compactStrings(values.Values)
		values.Repaired = true
		return nil
	}
	return fmt.Errorf("expected a string, string array, or text-like object array")
}

func flexibleObjectText(data json.RawMessage) (string, bool) {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		return strings.TrimSpace(text), strings.TrimSpace(text) != ""
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return "", false
	}
	for _, key := range []string{"text", "description", "reason", "question", "uncertainty", "warning", "message"} {
		if raw, exists := object[key]; exists && json.Unmarshal(raw, &text) == nil && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text), true
		}
	}
	return "", false
}

func compactStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func validateFlowReport(data []byte, bundle flowexplain.FlowBundle) error {
	_, err := normalizeFlowReport(data, bundle)
	return err
}

func normalizeFlowReport(data []byte, bundle flowexplain.FlowBundle) ([]byte, error) {
	var rawFields map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawFields); err != nil {
		return nil, fmt.Errorf("flow %q: invalid json: %w", bundle.FlowSeed.Name, err)
	}
	for _, field := range []string{
		"summary",
		"confidence",
		"files_to_read_in_order",
		"tests_to_read",
		"likely_chain",
		"unknowns",
		"warnings",
	} {
		if _, exists := rawFields[field]; !exists {
			return nil, fmt.Errorf("flow %q: required field %s is missing", bundle.FlowSeed.Name, field)
		}
	}

	var wire flowReportWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, fmt.Errorf("flow %q: decode report: %w", bundle.FlowSeed.Name, err)
	}
	if strings.TrimSpace(wire.Summary) == "" {
		return nil, fmt.Errorf("flow %q: summary is required", bundle.FlowSeed.Name)
	}
	if wire.Confidence == nil || *wire.Confidence < 0 || *wire.Confidence > 1 {
		return nil, fmt.Errorf("flow %q: confidence must be within [0,1]", bundle.FlowSeed.Name)
	}

	allowed := flowAllowedPaths(bundle)
	validatePaths := func(field string, paths []string, requireAllowed bool) error {
		for index, path := range paths {
			if !validRepoRelativePath(path) {
				return fmt.Errorf("flow %q: %s[%d] has invalid path %q", bundle.FlowSeed.Name, field, index, path)
			}
			if requireAllowed {
				if _, ok := allowed[path]; !ok {
					return fmt.Errorf("flow %q: %s[%d] references unprovided path %q", bundle.FlowSeed.Name, field, index, path)
				}
			}
		}
		return nil
	}

	files := make([]string, 0, len(wire.FilesToReadInOrder))
	for _, file := range wire.FilesToReadInOrder {
		files = append(files, file.Path)
	}
	if err := validatePaths("files_to_read_in_order", files, true); err != nil {
		return nil, err
	}
	tests := make([]string, 0, len(wire.TestsToRead))
	for _, test := range wire.TestsToRead {
		tests = append(tests, test.Path)
		if !strings.HasSuffix(strings.ToLower(test.Path), "_test.go") {
			return nil, fmt.Errorf("flow %q: tests_to_read path is not a Go test file: %q", bundle.FlowSeed.Name, test.Path)
		}
	}
	if err := validatePaths("tests_to_read", tests, true); err != nil {
		return nil, err
	}

	normalizedChain := make([]flowChainStep, 0, len(wire.LikelyChain))
	for index, step := range wire.LikelyChain {
		evidenceFiles := append([]string{}, step.EvidenceFiles...)
		if len(evidenceFiles) == 0 && step.File != "" {
			evidenceFiles = append(evidenceFiles, step.File)
		}
		if len(evidenceFiles) == 0 {
			return nil, fmt.Errorf("flow %q: likely_chain[%d] has no evidence_files", bundle.FlowSeed.Name, index)
		}
		if err := validatePaths(fmt.Sprintf("likely_chain[%d].evidence_files", index), evidenceFiles, true); err != nil {
			return nil, err
		}
		if step.Confidence < 0 || step.Confidence > 1 {
			return nil, fmt.Errorf("flow %q: likely_chain[%d] confidence is outside [0,1]", bundle.FlowSeed.Name, index)
		}
		name := strings.TrimSpace(step.Name)
		if name == "" {
			name = strings.TrimSpace(step.Role)
		}
		if name == "" {
			name = strings.TrimSpace(step.Function)
		}
		whatHappens := strings.TrimSpace(step.WhatHappens)
		if whatHappens == "" {
			whatHappens = strings.TrimSpace(step.Description)
		}
		if whatHappens == "" {
			whatHappens = strings.TrimSpace(step.Reason)
		}
		if whatHappens == "" {
			return nil, fmt.Errorf("flow %q: likely_chain[%d] has no supported description", bundle.FlowSeed.Name, index)
		}
		stepNumber := int(step.Step)
		if stepNumber <= 0 {
			stepNumber = index + 1
		}
		normalizedChain = append(normalizedChain, flowChainStep{
			Step:          stepNumber,
			Name:          name,
			WhatHappens:   whatHappens,
			EvidenceFiles: evidenceFiles,
			Confidence:    step.Confidence,
		})
	}

	unverified := make([]string, 0, len(wire.UnverifiedPaths))
	for _, path := range wire.UnverifiedPaths {
		unverified = append(unverified, path.Path)
	}
	if err := validatePaths("unverified_paths", unverified, false); err != nil {
		return nil, err
	}

	warnings := append([]string{}, wire.Warnings.Values...)
	for _, repair := range []struct {
		field    string
		repaired bool
	}{
		{field: "unknowns", repaired: wire.Unknowns.Repaired},
		{field: "next_queries", repaired: wire.NextQueries.Repaired},
		{field: "warnings", repaired: wire.Warnings.Repaired},
	} {
		if repair.repaired {
			warnings = append(warnings, fmt.Sprintf("parser normalized flexible %s items", repair.field))
		}
	}
	knownFields := map[string]struct{}{
		"summary": {}, "confidence": {}, "files_to_read_in_order": {}, "tests_to_read": {},
		"likely_chain": {}, "unverified_paths": {}, "unknowns": {}, "next_queries": {}, "warnings": {},
	}
	var unknownFields []string
	for field := range rawFields {
		if _, known := knownFields[field]; !known {
			unknownFields = append(unknownFields, field)
		}
	}
	sort.Strings(unknownFields)
	for _, field := range unknownFields {
		warnings = append(warnings, fmt.Sprintf("parser ignored unknown field %q", field))
	}

	if len(wire.FilesToReadInOrder) == 0 && len(normalizedChain) == 0 {
		return nil, fmt.Errorf("flow %q: report has neither a reading order nor an evidence-backed chain", bundle.FlowSeed.Name)
	}
	normalized := flowReportFields{
		Summary:            strings.TrimSpace(wire.Summary),
		Confidence:         *wire.Confidence,
		FilesToReadInOrder: wire.FilesToReadInOrder,
		TestsToRead:        wire.TestsToRead,
		LikelyChain:        normalizedChain,
		UnverifiedPaths:    wire.UnverifiedPaths,
		Unknowns:           wire.Unknowns.Values,
		NextQueries:        wire.NextQueries.Values,
		Warnings:           warnings,
	}
	normalizedJSON, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("flow %q: marshal normalized report: %w", bundle.FlowSeed.Name, err)
	}
	return normalizedJSON, nil
}

func flowAllowedPaths(bundle flowexplain.FlowBundle) map[string]struct{} {
	allowed := make(map[string]struct{})
	for _, path := range bundle.FlowSeed.ValidSeedFiles {
		allowed[path] = struct{}{}
	}
	for _, file := range bundle.SelectedFiles {
		allowed[file.Path] = struct{}{}
	}
	for _, file := range bundle.SelectedTests {
		allowed[file.Path] = struct{}{}
	}
	for _, file := range bundle.SelectedDocs {
		allowed[file.Path] = struct{}{}
	}
	return allowed
}

func decodePathItems(data json.RawMessage) ([]string, error) {
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	var objects []struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(data, &objects); err == nil {
		paths := make([]string, 0, len(objects))
		for _, object := range objects {
			paths = append(paths, object.Path)
		}
		return paths, nil
	}
	var paths []string
	if err := json.Unmarshal(data, &paths); err != nil {
		return nil, err
	}
	return paths, nil
}
