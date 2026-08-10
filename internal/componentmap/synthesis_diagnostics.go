package componentmap

import (
	"bytes"
	"encoding/json"
	"errors"
)

const (
	SynthesisFailureStageResponseValidation  = "response_validation"
	SynthesisFailureStageLandscapeValidation = "landscape_validation"

	SynthesisFailureCodeResponseEvaluationFailed = "componentmap.response_evaluation_failed"
	SynthesisFailureCodePartialModelInconsistent = "componentmap.partial_model_inconsistent"
)

var ErrPartialModelStateInconsistent = errors.New(
	"componentmap: partial model source has inconsistent state",
)

type SynthesisFailureDiagnostic struct {
	Stage  string
	Code   string
	Detail string
}

func DiagnoseSynthesisFailure(err error) SynthesisFailureDiagnostic {
	if errors.Is(err, ErrPartialModelStateInconsistent) {
		return SynthesisFailureDiagnostic{
			Stage:  SynthesisFailureStageLandscapeValidation,
			Code:   SynthesisFailureCodePartialModelInconsistent,
			Detail: "full or normalized coverage was classified as partial without a local remainder or item-local salvage",
		}
	}
	return SynthesisFailureDiagnostic{
		Stage: SynthesisFailureStageResponseValidation,
		Code:  SynthesisFailureCodeResponseEvaluationFailed,
	}
}

// SynthesisResponseShape is a diagnostic-only bounded cardinality summary.
// It deliberately retains no names, refs, prose, paths, or source bytes.
type SynthesisResponseShape struct {
	JSONValid              bool
	Grammar                string
	SubsystemCount         int
	ComponentCount         int
	MemberRefCount         int
	UnitRefCount           int
	AnchorRefCount         int
	MissingAnchorRefsCount int
	EmptyAnchorRefsCount   int
	NullAnchorRefsCount    int
}

func InspectSynthesisResponseShape(raw []byte) SynthesisResponseShape {
	shape := SynthesisResponseShape{JSONValid: json.Valid(raw)}
	if !shape.JSONValid || len(raw) == 0 || len(raw) > maxSynthesisResponseBytes {
		return shape
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return shape
	}
	if subsystems, exists := root["subsystems"]; exists {
		shape.Grammar = "nested"
		inspectNestedSynthesisShape(subsystems, &shape)
		return shape
	}
	if records, exists := root["records"]; exists {
		shape.Grammar = "flat"
		inspectFlatSynthesisShape(records, &shape)
	}
	return shape
}

// InspectEvaluatedSynthesisResponseShape reports the shape of the exact JSON
// proposal object used by synthesis evaluation. It shares the evaluator's
// bounded trailing-closing-delimiter normalization, but performs no broader
// response repair. The persisted provider response remains unchanged.
func InspectEvaluatedSynthesisResponseShape(raw []byte) SynthesisResponseShape {
	shape := InspectSynthesisResponseShape(raw)
	if shape.JSONValid || len(raw) == 0 || len(raw) > maxSynthesisResponseBytes {
		return shape
	}
	proposal, normalization, responseErr := extractProposalObject(raw)
	if responseErr != nil || normalization == nil {
		return shape
	}
	return InspectSynthesisResponseShape(proposal)
}

func SynthesisResponseStateForDiagnostics(raw []byte) ResponseState {
	if len(raw) == 0 {
		return ResponseEmpty
	}
	if len(raw) > maxSynthesisResponseBytes {
		return ResponseOversize
	}
	if synthesisResponseContainsCredential(raw) {
		return ResponseSensitiveOmitted
	}
	return ResponseCaptured
}

func inspectNestedSynthesisShape(raw json.RawMessage, shape *SynthesisResponseShape) {
	var subsystems []json.RawMessage
	if shape == nil || json.Unmarshal(raw, &subsystems) != nil {
		return
	}
	shape.SubsystemCount = len(subsystems)
	for _, rawSubsystem := range subsystems {
		var subsystem map[string]json.RawMessage
		if json.Unmarshal(rawSubsystem, &subsystem) != nil {
			continue
		}
		var components []json.RawMessage
		if json.Unmarshal(subsystem["components"], &components) != nil {
			continue
		}
		shape.ComponentCount += len(components)
		for _, rawComponent := range components {
			var component map[string]json.RawMessage
			if json.Unmarshal(rawComponent, &component) != nil {
				continue
			}
			shape.MemberRefCount += jsonArrayLength(component["member_refs"])
			shape.UnitRefCount += jsonArrayLength(component["unit_refs"])
			anchorRaw, exists := component["anchor_refs"]
			if !exists {
				shape.MissingAnchorRefsCount++
				continue
			}
			if bytes.Equal(bytes.TrimSpace(anchorRaw), []byte("null")) {
				shape.NullAnchorRefsCount++
				continue
			}
			count := jsonArrayLength(anchorRaw)
			shape.AnchorRefCount += count
			if count == 0 && jsonArray(anchorRaw) {
				shape.EmptyAnchorRefsCount++
			}
		}
	}
}

func inspectFlatSynthesisShape(raw json.RawMessage, shape *SynthesisResponseShape) {
	var records []json.RawMessage
	if shape == nil || json.Unmarshal(raw, &records) != nil {
		return
	}
	for _, rawRecord := range records {
		var record map[string]json.RawMessage
		if json.Unmarshal(rawRecord, &record) != nil {
			continue
		}
		var kind string
		if json.Unmarshal(record["kind"], &kind) != nil {
			continue
		}
		switch kind {
		case string(synthesisWireSubsystemRecord):
			shape.SubsystemCount++
		case string(synthesisWireComponentRecord):
			shape.ComponentCount++
			shape.MemberRefCount += jsonArrayLength(record["member_refs"])
			shape.UnitRefCount += jsonArrayLength(record["unit_refs"])
			anchorRaw, exists := record["anchor_refs"]
			if !exists {
				shape.MissingAnchorRefsCount++
				continue
			}
			if bytes.Equal(bytes.TrimSpace(anchorRaw), []byte("null")) {
				shape.NullAnchorRefsCount++
				continue
			}
			count := jsonArrayLength(anchorRaw)
			shape.AnchorRefCount += count
			if count == 0 && jsonArray(anchorRaw) {
				shape.EmptyAnchorRefsCount++
			}
		}
	}
}

func jsonArrayLength(raw json.RawMessage) int {
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) != nil {
		return 0
	}
	return len(values)
}

func jsonArray(raw json.RawMessage) bool {
	var values []json.RawMessage
	return json.Unmarshal(raw, &values) == nil && values != nil
}
