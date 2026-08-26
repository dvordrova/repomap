package targetportfolio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/secretscan"
)

// ResolveResponse validates the exact positive target selection and restores
// every omitted input as unclassified. There is no complement acceptance,
// fuzzy matching, or repair.
func ResolveResponse(compilation Compilation, raw []byte) (Selection, error) {
	if err := validateCompilation(compilation); err != nil {
		return Selection{}, err
	}
	if len(raw) == 0 || len(raw) > MaxResponseBytes {
		return Selection{}, fmt.Errorf("target portfolio: response exceeds bounded envelope")
	}
	if _, found := secretscan.Detect(string(raw)); found {
		return Selection{}, fmt.Errorf("target portfolio: response contains credential-shaped content")
	}

	var response Response
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return Selection{}, fmt.Errorf("target portfolio: invalid JSON response")
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Selection{}, err
	}
	if response.TargetFileRefs == nil {
		return Selection{}, fmt.Errorf("target portfolio: response must contain target_file_refs as an array")
	}
	var exactFields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &exactFields); err != nil || len(exactFields) != 2 ||
		exactFields["default_file_ref"] == nil || exactFields["target_file_refs"] == nil {
		return Selection{}, fmt.Errorf("target portfolio: response must contain exactly default_file_ref and target_file_refs")
	}

	authority := make(map[corpus.FileID]VisibleCandidate, len(compilation.Request.Candidates))
	for _, candidate := range compilation.Request.Candidates {
		authority[candidate.FileRef] = candidate
	}
	targetSet := make(map[corpus.FileID]struct{}, len(response.TargetFileRefs))
	for _, fileRef := range response.TargetFileRefs {
		if _, known := authority[fileRef]; !known {
			continue
		}
		targetSet[fileRef] = struct{}{}
	}
	if compilation.requiredAuthorityBound {
		for _, fileRef := range compilation.requiredTargetFileRefs {
			if _, selected := targetSet[fileRef]; !selected {
				return Selection{}, fmt.Errorf("target portfolio: selection omits exact required target authority")
			}
		}
	}
	if len(targetSet) == 0 {
		if response.DefaultFileRef != nil {
			return Selection{}, fmt.Errorf("target portfolio: empty known target_file_refs requires null default_file_ref")
		}
		unclassified := make([]VisibleCandidate, 0, len(compilation.Request.Candidates))
		for _, candidate := range compilation.Request.Candidates {
			unclassified = append(unclassified, cloneVisibleCandidate(candidate))
		}
		return Selection{
			Default: nil, Targets: []VisibleCandidate{}, Unclassified: unclassified,
		}, nil
	}
	if response.DefaultFileRef == nil {
		return Selection{}, fmt.Errorf("target portfolio: non-empty target_file_refs requires default_file_ref")
	}
	if compilation.executableAuthorityBound && len(compilation.executableFileRefs) != 0 {
		executableSet := make(map[corpus.FileID]struct{}, len(compilation.executableFileRefs))
		for _, fileRef := range compilation.executableFileRefs {
			executableSet[fileRef] = struct{}{}
		}
		selectedExecutable := false
		for fileRef := range targetSet {
			if _, executable := executableSet[fileRef]; executable {
				selectedExecutable = true
				break
			}
		}
		if !selectedExecutable {
			return Selection{}, fmt.Errorf("target portfolio: positive selection omits exact executable authority")
		}
		if _, executable := executableSet[*response.DefaultFileRef]; !executable {
			return Selection{}, fmt.Errorf("target portfolio: positive selection requires an exact executable default")
		}
	}
	defaultCandidate, known := authority[*response.DefaultFileRef]
	if !known {
		return Selection{}, fmt.Errorf("target portfolio: response cites unknown default_file_ref")
	}

	if _, selected := targetSet[*response.DefaultFileRef]; !selected {
		return Selection{}, fmt.Errorf("target portfolio: default_file_ref is absent from target_file_refs")
	}

	defaultCopy := cloneVisibleCandidate(defaultCandidate)
	result := Selection{
		Default:      &defaultCopy,
		Targets:      make([]VisibleCandidate, 0, len(targetSet)),
		Unclassified: make([]VisibleCandidate, 0, len(compilation.Request.Candidates)-len(targetSet)),
	}
	for _, candidate := range compilation.Request.Candidates {
		if _, selected := targetSet[candidate.FileRef]; selected {
			result.Targets = append(result.Targets, cloneVisibleCandidate(candidate))
			continue
		}
		result.Unclassified = append(result.Unclassified, cloneVisibleCandidate(candidate))
	}
	if result.Default == nil || len(result.Targets) == 0 ||
		len(result.Targets)+len(result.Unclassified) != len(compilation.Request.Candidates) {
		return Selection{}, fmt.Errorf("target portfolio: response does not restore a complete candidate partition")
	}
	return result, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("target portfolio: trailing JSON value")
		}
		return fmt.Errorf("target portfolio: invalid trailing response data")
	}
	return nil
}
