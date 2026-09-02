// Package groupmatching owns model-assisted semantic matching between complete
// GroupsIndexes. It consumes no ProgramIndex or documentation authority and
// restores only request-local refs before delegating graph mutation and sealing
// to groupindex.WithConnections.
package groupmatching

import (
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

const (
	requestVersion        = 8
	executionContract     = "repomap.group-matching.v10"
	preparationVersion    = 10
	responseSchemaVersion = 5
	outputTokenCount      = 128_000
)

const (
	diagnosticMalformedConnection = "malformed_connection"
	diagnosticInvalidConnection   = "invalid_connection"
	diagnosticUnknownPairRef      = "unknown_pair_ref"
	diagnosticInvalidPairEndpoint = "invalid_pair_endpoint"
	diagnosticInvalidDirection    = "invalid_connection_direction"
	diagnosticInvalidWitnessJoint = "invalid_witness_joint"
	diagnosticMissingWitnessJoint = "missing_witness_joint"
)

const witnessJointArgumentValue = "argument_value"

type witnessJoint struct {
	kind                 string
	leftBoundaryEdgeRef  string
	leftPatternRef       string
	leftArgumentRef      string
	rightBoundaryEdgeRef string
	rightPatternRef      string
	rightArgumentRef     string
	resolution           programindex.PatternValueResolution
}

type witnessCandidateAuthority struct {
	ref                  string
	joint                witnessJoint
	requiredFromGroupRef string
	requiredToGroupRef   string
}

type connectionInput struct {
	pairRef           string
	fromGroupRef      string
	toGroupRef        string
	semanticKind      string
	label             string
	summary           string
	supportResolution programindex.PatternValueResolution
	evidenceRefs      []string
	witnessJoints     []witnessJoint
}

func canonicalConnectionInputs(values []connectionInput) []connectionInput {
	type slot struct {
		pairRef      string
		fromGroupRef string
		toGroupRef   string
		semanticKind string
		label        string
		summary      string
	}
	bySlot := make(map[slot]connectionInput, len(values))
	for _, value := range values {
		value.evidenceRefs = canonicalStrings(value.evidenceRefs)
		key := slot{
			pairRef: value.pairRef, fromGroupRef: value.fromGroupRef, toGroupRef: value.toGroupRef,
			semanticKind: value.semanticKind, label: value.label, summary: value.summary,
		}
		previous, exists := bySlot[key]
		if exists {
			previous.evidenceRefs = unionStrings(previous.evidenceRefs, value.evidenceRefs)
			previous.witnessJoints = unionWitnessJoints(previous.witnessJoints, value.witnessJoints)
			previous.supportResolution = strongestWitnessResolution(previous.witnessJoints)
			bySlot[key] = previous
			continue
		}
		value.witnessJoints = canonicalWitnessJoints(value.witnessJoints)
		value.supportResolution = strongestWitnessResolution(value.witnessJoints)
		bySlot[key] = value
	}
	result := make([]connectionInput, 0, len(bySlot))
	for _, value := range bySlot {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		return connectionInputKey(result[i]) < connectionInputKey(result[j])
	})
	if result == nil {
		result = []connectionInput{}
	}
	return result
}

func strongestWitnessResolution(values []witnessJoint) programindex.PatternValueResolution {
	result := programindex.PatternValuePossible
	for _, value := range values {
		if value.resolution == programindex.PatternValueExact {
			return programindex.PatternValueExact
		}
	}
	return result
}

func canonicalWitnessJoints(values []witnessJoint) []witnessJoint {
	result := append([]witnessJoint(nil), values...)
	sort.Slice(result, func(i, j int) bool { return witnessJointKey(result[i]) < witnessJointKey(result[j]) })
	write := 0
	for _, value := range result {
		if write > 0 && witnessJointKey(result[write-1]) == witnessJointKey(value) {
			continue
		}
		result[write] = value
		write++
	}
	result = result[:write]
	if result == nil {
		result = []witnessJoint{}
	}
	return result
}

func unionWitnessJoints(left, right []witnessJoint) []witnessJoint {
	return canonicalWitnessJoints(append(append([]witnessJoint(nil), left...), right...))
}

func witnessJointKey(value witnessJoint) string {
	return strings.Join([]string{
		value.kind, value.leftBoundaryEdgeRef, value.leftPatternRef, value.leftArgumentRef,
		value.rightBoundaryEdgeRef, value.rightPatternRef, value.rightArgumentRef, string(value.resolution),
	}, "\x00")
}

func connectionInputKey(value connectionInput) string {
	return strings.Join([]string{
		value.pairRef, value.fromGroupRef, value.toGroupRef, value.semanticKind,
		value.label, value.summary, string(value.supportResolution), strings.Join(value.evidenceRefs, "\x01"),
	}, "\x00")
}

func canonicalDiagnostics(values []groupindex.Diagnostic) []groupindex.Diagnostic {
	sort.Slice(values, func(i, j int) bool {
		return diagnosticKey(values[i]) < diagnosticKey(values[j])
	})
	result := values[:0]
	for _, value := range values {
		if len(result) > 0 && diagnosticKey(result[len(result)-1]) == diagnosticKey(value) {
			continue
		}
		result = append(result, value)
	}
	if result == nil {
		result = []groupindex.Diagnostic{}
	}
	return result
}

func diagnosticKey(value groupindex.Diagnostic) string {
	return value.Kind + "\x00" + value.ProposalKey + "\x00" + value.Reason
}

func canonicalStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	write := 0
	for _, value := range result {
		if write > 0 && result[write-1] == value {
			continue
		}
		result[write] = value
		write++
	}
	result = result[:write]
	if result == nil {
		result = []string{}
	}
	return result
}

func unionStrings(left, right []string) []string {
	return canonicalStrings(append(append([]string(nil), left...), right...))
}
