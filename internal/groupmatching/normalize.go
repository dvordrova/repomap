package groupmatching

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
)

type normalizedResponse struct {
	connections []connectionInput
	diagnostics []groupindex.Diagnostic
}

func normalizeResponse(raw []byte, compilation Compilation, request Request) (normalizedResponse, error) {
	normalized, err := llm.NormalizeJSON(raw)
	if err != nil {
		return normalizedResponse{}, err
	}
	if !hasExactObjectKeys(normalized, []string{"connections"}) {
		return normalizedResponse{}, fmt.Errorf("group matching: response envelope does not match the closed schema")
	}
	var envelope struct {
		Connections []json.RawMessage `json:"connections"`
	}
	decoder := json.NewDecoder(bytes.NewReader(normalized))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return normalizedResponse{}, fmt.Errorf("group matching: decode response: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return normalizedResponse{}, fmt.Errorf("group matching: response has trailing data")
	}
	if envelope.Connections == nil {
		return normalizedResponse{}, fmt.Errorf("group matching: response connections must be an array")
	}
	pair, pairKnown := compilation.pairByRef[request.Pair.Ref]
	if !pairKnown {
		return normalizedResponse{}, fmt.Errorf("group matching: request retained an unknown pair ref")
	}
	advertisedWitnesses := make(map[string]witnessCandidateAuthority, len(request.WitnessCandidates))
	for _, candidate := range request.WitnessCandidates {
		authority, known := pair.witnessByRef[candidate.Ref]
		if !known || compilation.witnessCandidateWire(authority) != candidate {
			continue
		}
		advertisedWitnesses[candidate.Ref] = authority
	}
	result := normalizedResponse{}
	for _, rawRow := range envelope.Connections {
		var row responseConnection
		if !decodeStrictRow(rawRow, &row) {
			result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
				Kind:   diagnosticMalformedConnection,
				Reason: "connection row does not match the closed response schema",
			})
			continue
		}
		proposalKey := row.PairRef + ":" + row.FromGroupRef + "->" + row.ToGroupRef + ":" + row.SemanticKind
		if !validText(row.PairRef) || !validText(row.FromGroupRef) || !validText(row.ToGroupRef) ||
			row.FromGroupRef == row.ToGroupRef || !validSnakeCase(row.SemanticKind) ||
			!validText(row.Label) || !validText(row.Summary) {
			result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
				Kind: diagnosticInvalidConnection, ProposalKey: proposalKey,
				Reason: "connection has an invalid pair, endpoint, semantic kind, label, or summary",
			})
			continue
		}
		if row.PairRef != request.Pair.Ref {
			result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
				Kind: diagnosticUnknownPairRef, ProposalKey: proposalKey,
				Reason: "connection pair ref was not owned by this request",
			})
			continue
		}
		forward := row.FromGroupRef == pair.leftGroupRef && row.ToGroupRef == pair.rightGroupRef
		reverse := row.FromGroupRef == pair.rightGroupRef && row.ToGroupRef == pair.leftGroupRef
		if !forward && !reverse {
			result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
				Kind: diagnosticInvalidPairEndpoint, ProposalKey: proposalKey,
				Reason: "connection endpoints are not the two groups advertised by its pair",
			})
			continue
		}
		witnessJoints := make([]witnessJoint, 0, len(row.WitnessJointRefs))
		directionConflict := false
		for _, ref := range canonicalStrings(row.WitnessJointRefs) {
			authority, known := advertisedWitnesses[ref]
			if !known {
				result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
					Kind: diagnosticInvalidWitnessJoint, ProposalKey: proposalKey,
					Reason: "witness ref is absent from this pair's closed candidate catalog",
				})
				continue
			}
			joint, reason, accepted := compilation.revalidateWitnessCandidate(authority, pair)
			if !accepted {
				result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
					Kind: diagnosticInvalidWitnessJoint, ProposalKey: proposalKey, Reason: reason,
				})
				continue
			}
			if authority.requiredFromGroupRef != "" &&
				(row.FromGroupRef != authority.requiredFromGroupRef || row.ToGroupRef != authority.requiredToGroupRef) {
				result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
					Kind: diagnosticInvalidDirection, ProposalKey: proposalKey,
					Reason: "connection direction contradicts its selected inbound-delivery and outbound-dependency witness",
				})
				directionConflict = true
				continue
			}
			witnessJoints = append(witnessJoints, joint)
		}
		if directionConflict {
			continue
		}
		witnessJoints = canonicalWitnessJoints(witnessJoints)
		if len(witnessJoints) == 0 {
			result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
				Kind: diagnosticMissingWitnessJoint, ProposalKey: proposalKey,
				Reason: "connection has no complete locally validated cross-target witness joint",
			})
			continue
		}
		evidenceRefs := make([]string, 0, len(witnessJoints)*2)
		for _, joint := range witnessJoints {
			evidenceRefs = append(evidenceRefs, joint.leftPatternRef, joint.rightPatternRef)
		}
		evidenceRefs = canonicalStrings(evidenceRefs)
		result.connections = append(result.connections, connectionInput{
			pairRef: row.PairRef, fromGroupRef: row.FromGroupRef, toGroupRef: row.ToGroupRef,
			semanticKind: row.SemanticKind, label: row.Label, summary: row.Summary,
			supportResolution: strongestWitnessResolution(witnessJoints),
			evidenceRefs:      evidenceRefs, witnessJoints: witnessJoints,
		})
	}
	result.connections = canonicalConnectionInputs(result.connections)
	if result.connections == nil {
		result.connections = []connectionInput{}
	}
	result.diagnostics = canonicalDiagnostics(result.diagnostics)
	return result, nil
}

func (compilation Compilation) revalidateWitnessCandidate(
	authority witnessCandidateAuthority,
	pair pairAuthority,
) (witnessJoint, string, bool) {
	stored := authority.joint
	revalidated, reason, accepted := compilation.normalizeWitnessJoint(responseWitnessJoint{
		Kind:                stored.kind,
		LeftBoundaryEdgeRef: stored.leftBoundaryEdgeRef, LeftArgumentRef: stored.leftArgumentRef,
		RightBoundaryEdgeRef: stored.rightBoundaryEdgeRef, RightArgumentRef: stored.rightArgumentRef,
	}, pair)
	if !accepted {
		return witnessJoint{}, reason, false
	}
	if revalidated != stored {
		return witnessJoint{}, "witness candidate no longer matches its compiled pair authority", false
	}
	requiredFromGroupRef, requiredToGroupRef := compilation.requiredWitnessDirection(revalidated, pair)
	if requiredFromGroupRef != authority.requiredFromGroupRef || requiredToGroupRef != authority.requiredToGroupRef {
		return witnessJoint{}, "witness candidate direction no longer matches its compiled pair authority", false
	}
	return revalidated, "", true
}

func (compilation Compilation) normalizeWitnessJoint(
	row responseWitnessJoint,
	pair pairAuthority,
) (witnessJoint, string, bool) {
	if row.Kind != witnessJointArgumentValue || !validText(row.LeftBoundaryEdgeRef) ||
		!validText(row.LeftArgumentRef) || !validText(row.RightBoundaryEdgeRef) ||
		!validText(row.RightArgumentRef) {
		return witnessJoint{}, "witness joint has an unsupported kind or invalid ref", false
	}
	leftBoundary, leftAllowed := pair.leftBoundaryEdges[row.LeftBoundaryEdgeRef]
	if !leftAllowed {
		return witnessJoint{}, "left boundary edge ref is absent from the left endpoint boundary-edge dossier", false
	}
	rightBoundary, rightAllowed := pair.rightBoundaryEdges[row.RightBoundaryEdgeRef]
	if !rightAllowed {
		return witnessJoint{}, "right boundary edge ref is absent from the right endpoint boundary-edge dossier", false
	}
	leftGroup, leftGroupKnown := compilation.groupByRef[pair.leftGroupRef]
	rightGroup, rightGroupKnown := compilation.groupByRef[pair.rightGroupRef]
	leftEdge, leftEdgeKnown := compilation.edgeByRef[row.LeftBoundaryEdgeRef]
	rightEdge, rightEdgeKnown := compilation.edgeByRef[row.RightBoundaryEdgeRef]
	leftRevalidated, leftValid := compilation.boundaryEdgeForGroup(leftGroup, leftEdge)
	rightRevalidated, rightValid := compilation.boundaryEdgeForGroup(rightGroup, rightEdge)
	if !leftGroupKnown || !rightGroupKnown || !leftEdgeKnown || !rightEdgeKnown || !leftValid || !rightValid ||
		leftRevalidated != leftBoundary || rightRevalidated != rightBoundary {
		return witnessJoint{}, "witness boundary edge no longer satisfies its endpoint authority", false
	}
	leftSubject, leftKnown := compilation.subjectByRef[leftBoundary.patternRef]
	rightSubject, rightKnown := compilation.subjectByRef[rightBoundary.patternRef]
	if !leftKnown || !rightKnown || leftSubject.subject.Pattern == nil || rightSubject.subject.Pattern == nil {
		return witnessJoint{}, "witness boundary edge source does not identify a pattern", false
	}
	leftArgument, leftArgumentKnown := compilation.argumentByRef[row.LeftArgumentRef]
	rightArgument, rightArgumentKnown := compilation.argumentByRef[row.RightArgumentRef]
	if !leftArgumentKnown || !rightArgumentKnown || leftArgument.ownerSubjectRef != leftBoundary.patternRef ||
		rightArgument.ownerSubjectRef != rightBoundary.patternRef {
		return witnessJoint{}, "witness argument ref is not owned by its cited boundary pattern", false
	}
	resolution, intersects := intersectArgumentValues(leftArgument.argument, rightArgument.argument)
	if !intersects {
		return witnessJoint{}, "witness arguments have no shared direct or reconstructed literal/template value", false
	}
	leftPossible := leftBoundary.resolution == programindex.PatternValuePossible
	rightPossible := rightBoundary.resolution == programindex.PatternValuePossible
	if leftPossible || rightPossible {
		if leftPossible && rightPossible {
			return witnessJoint{}, "two possible boundary edges cannot establish one direct cross-target joint", false
		}
		if resolution != programindex.PatternValueExact {
			return witnessJoint{}, "a possible boundary edge requires an exact shared argument value", false
		}
		resolution = programindex.PatternValuePossible
	}
	return witnessJoint{
		kind:                row.Kind,
		leftBoundaryEdgeRef: row.LeftBoundaryEdgeRef, leftPatternRef: leftBoundary.patternRef,
		leftArgumentRef:      row.LeftArgumentRef,
		rightBoundaryEdgeRef: row.RightBoundaryEdgeRef, rightPatternRef: rightBoundary.patternRef,
		rightArgumentRef: row.RightArgumentRef, resolution: resolution,
	}, "", true
}

func intersectArgumentValues(
	left groupindex.PatternArgument,
	right groupindex.PatternArgument,
) (programindex.PatternValueResolution, bool) {
	leftValues := argumentValues(left)
	rightValues := argumentValues(right)
	matched := false
	resolution := programindex.PatternValuePossible
	for key, leftResolution := range leftValues {
		rightResolution, exists := rightValues[key]
		if !exists {
			continue
		}
		matched = true
		if leftResolution == programindex.PatternValueExact && rightResolution == programindex.PatternValueExact {
			return programindex.PatternValueExact, true
		}
	}
	return resolution, matched
}

func argumentValues(value groupindex.PatternArgument) map[string]programindex.PatternValueResolution {
	result := make(map[string]programindex.PatternValueResolution, len(value.ValueCandidates)+1)
	if value.Kind == programindex.PatternLiteralString || value.Kind == programindex.PatternStringTemplate {
		result[patternValueKey(value.Kind, value.Value, value.Parts)] = programindex.PatternValueExact
	}
	for _, candidate := range value.ValueCandidates {
		key := patternValueKey(candidate.Kind, candidate.Value, candidate.Parts)
		previous, exists := result[key]
		if !exists || previous == programindex.PatternValuePossible && candidate.Resolution == programindex.PatternValueExact {
			result[key] = candidate.Resolution
		}
	}
	return result
}

func patternValueKey(
	kind programindex.PatternValueKind,
	value string,
	parts []programindex.PatternPart,
) string {
	wire, _ := json.Marshal(struct {
		Kind  programindex.PatternValueKind `json:"kind"`
		Value string                        `json:"value,omitempty"`
		Parts []programindex.PatternPart    `json:"parts"`
	}{Kind: kind, Value: value, Parts: parts})
	return string(wire)
}

func (compilation Compilation) restore(values []connectionInput) ([]groupindex.ConnectionInput, error) {
	result := make([]groupindex.ConnectionInput, len(values))
	for position, value := range values {
		from, fromOK := compilation.groupByRef[value.fromGroupRef]
		to, toOK := compilation.groupByRef[value.toGroupRef]
		if !fromOK || !toOK {
			return nil, fmt.Errorf("group matching: normalized connection retained an unknown group ref")
		}
		row := groupindex.ConnectionInput{
			From:         groupindex.Endpoint{TargetID: from.targetID, GroupID: from.group.ID},
			To:           groupindex.Endpoint{TargetID: to.targetID, GroupID: to.group.ID},
			SemanticKind: value.semanticKind, Label: value.label, Summary: value.summary,
			SupportResolution: value.supportResolution,
			Evidence:          make([]groupindex.SubjectEndpoint, 0, len(value.evidenceRefs)),
		}
		for _, ref := range value.evidenceRefs {
			subject, known := compilation.subjectByRef[ref]
			if !known {
				return nil, fmt.Errorf("group matching: normalized connection retained an unknown evidence ref")
			}
			row.Evidence = append(row.Evidence, groupindex.SubjectEndpoint{
				TargetID: subject.targetID, SubjectID: subject.subject.ID,
			})
		}
		result[position] = row
	}
	return result, nil
}

func decodeStrictRow(raw json.RawMessage, destination any) bool {
	if !hasExactObjectKeys(raw, []string{
		"pair_ref", "from_group_ref", "to_group_ref", "semantic_kind",
		"label", "summary", "witness_joint_refs",
	}) {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return false
	}
	var trailing any
	return errors.Is(decoder.Decode(&trailing), io.EOF)
}

// hasExactObjectKeys closes two gaps left by encoding/json's struct decoder:
// duplicate object keys and case-insensitive field matching. It validates only
// this object's key set; the normal decoder still owns typed value validation.
func hasExactObjectKeys(raw []byte, expected []string) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return false
	}
	wanted := make(map[string]struct{}, len(expected))
	for _, key := range expected {
		wanted[key] = struct{}{}
	}
	seen := make(map[string]struct{}, len(expected))
	for decoder.More() {
		token, tokenErr := decoder.Token()
		key, keyIsString := token.(string)
		if tokenErr != nil || !keyIsString {
			return false
		}
		if _, known := wanted[key]; !known {
			return false
		}
		if _, duplicate := seen[key]; duplicate {
			return false
		}
		seen[key] = struct{}{}
		var value json.RawMessage
		if decodeErr := decoder.Decode(&value); decodeErr != nil {
			return false
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') || len(seen) != len(wanted) {
		return false
	}
	var trailing any
	return errors.Is(decoder.Decode(&trailing), io.EOF)
}

func validText(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validSnakeCase(value string) bool {
	if value == "" || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	previousUnderscore := false
	for _, character := range value {
		switch {
		case character >= 'a' && character <= 'z', character >= '0' && character <= '9':
			previousUnderscore = false
		case character == '_' && !previousUnderscore:
			previousUnderscore = true
		default:
			return false
		}
	}
	return !previousUnderscore
}
