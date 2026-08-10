package symbol

import (
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/evidence"
)

type Basis string

const (
	BasisStaticFact Basis = "static_fact"
	BasisInference  Basis = "inference"
)

type Report struct {
	Target             EntityRef            `json:"target"`
	Summary            Claim                `json:"summary"`
	Responsibility     Claim                `json:"responsibility"`
	Callers            []Relationship       `json:"callers"`
	Callees            []Relationship       `json:"callees"`
	FilesToReadInOrder []FileRecommendation `json:"files_to_read_in_order"`
	TestsToRead        []FileRecommendation `json:"tests_to_read"`
	Unknowns           []string             `json:"unknowns"`
	NextQueries        []NextQuery          `json:"next_queries"`
	Warnings           []string             `json:"warnings"`
}

type EntityRef struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column,omitempty"`
}

type Claim struct {
	Statement   string   `json:"statement"`
	Basis       Basis    `json:"basis"`
	EvidenceIDs []string `json:"evidence_ids"`
	Confidence  float64  `json:"confidence"`
}

type Relationship struct {
	Symbol       EntityRef `json:"symbol"`
	Relationship string    `json:"relationship"`
	Basis        Basis     `json:"basis"`
	EvidenceIDs  []string  `json:"evidence_ids"`
	Confidence   float64   `json:"confidence"`
}

type FileRecommendation struct {
	Path           string   `json:"path"`
	Line           int      `json:"line,omitempty"`
	StructuralRole string   `json:"structural_role"`
	EvidenceIDs    []string `json:"evidence_ids"`
}

type NextQuery struct {
	Query  string `json:"query"`
	Reason string `json:"reason"`
}

func ValidateReport(bundle Bundle, report Report) error {
	if err := validateTarget(bundle, report.Target); err != nil {
		return err
	}
	evidenceCertainty := evidenceCertainties(bundle)
	evidencePaths := evidencePathSets(bundle)
	evidenceRoles := evidenceRoleSets(bundle)
	incomingEntities := directionalEntityRefs(bundle.IncomingCalls, true)
	outgoingEntities := directionalEntityRefs(bundle.OutgoingCalls, false)
	allowedPaths := makeStringSet(bundle.AllowedPaths)

	if err := validateClaim("summary", report.Summary, evidenceCertainty); err != nil {
		return err
	}
	if report.Summary.Basis != BasisInference {
		return fmt.Errorf("symbol report: summary basis must be inference")
	}
	if report.Summary.Confidence > 0.75 {
		return fmt.Errorf("symbol report: summary confidence exceeds static-only inference cap 0.75")
	}
	if err := validateClaim("responsibility", report.Responsibility, evidenceCertainty); err != nil {
		return err
	}
	if report.Responsibility.Basis != BasisInference {
		return fmt.Errorf("symbol report: responsibility basis must be inference")
	}
	if report.Responsibility.Confidence > 0.75 {
		return fmt.Errorf("symbol report: responsibility confidence exceeds static-only inference cap 0.75")
	}
	for i, relationship := range report.Callers {
		if err := validateRelationship(fmt.Sprintf("callers[%d]", i), relationship, "call-in-", "statically calls the target under the stated build scenario", incomingEntities, allowedPaths, evidenceCertainty); err != nil {
			return err
		}
	}
	for i, relationship := range report.Callees {
		if err := validateRelationship(fmt.Sprintf("callees[%d]", i), relationship, "call-out-", "is statically called by the target under the stated build scenario", outgoingEntities, allowedPaths, evidenceCertainty); err != nil {
			return err
		}
	}
	for i, file := range report.FilesToReadInOrder {
		if err := validateFileRecommendation(fmt.Sprintf("files_to_read_in_order[%d]", i), file, false, allowedPaths, evidenceCertainty, evidencePaths, evidenceRoles); err != nil {
			return err
		}
	}
	for i, file := range report.TestsToRead {
		if err := validateFileRecommendation(fmt.Sprintf("tests_to_read[%d]", i), file, true, allowedPaths, evidenceCertainty, evidencePaths, evidenceRoles); err != nil {
			return err
		}
	}
	for i, query := range report.NextQueries {
		if query.Query == "" || query.Reason == "" {
			return fmt.Errorf("symbol report: next_queries[%d] requires query and reason", i)
		}
	}
	return nil
}

func validateTarget(bundle Bundle, target EntityRef) error {
	expected := bundle.Target.Entity
	if expected.Location == nil {
		return fmt.Errorf("symbol report: bundle target has no location")
	}
	if target.Name != expected.Name ||
		target.Kind != string(expected.Kind) ||
		target.Path != expected.Location.Path ||
		target.Line != expected.Location.Line ||
		target.Column != expected.Location.Column {
		return fmt.Errorf("symbol report: target does not match resolved bundle target")
	}
	return nil
}

func validateClaim(field string, claim Claim, certainties map[string]evidenceCertainty) error {
	if claim.Statement == "" {
		return fmt.Errorf("symbol report: %s statement is required", field)
	}
	if !claim.Basis.valid() {
		return fmt.Errorf("symbol report: %s has invalid basis %q", field, claim.Basis)
	}
	if err := validateConfidence(field, claim.Confidence); err != nil {
		return err
	}
	if err := validateEvidenceIDs(field, claim.EvidenceIDs, certainties); err != nil {
		return err
	}
	if claim.Basis == BasisStaticFact {
		for _, id := range claim.EvidenceIDs {
			if !certainties[id].supportsStaticFact {
				return fmt.Errorf("symbol report: %s cites non-static evidence %q as static_fact", field, id)
			}
		}
	}
	return nil
}

func validateRelationship(field string, relationship Relationship, evidencePrefix, expectedStatement string, expectedEntities map[string]EntityRef, allowedPaths map[string]struct{}, certainties map[string]evidenceCertainty) error {
	if relationship.Relationship != expectedStatement {
		return fmt.Errorf("symbol report: %s relationship must be %q", field, expectedStatement)
	}
	if relationship.Basis != BasisStaticFact {
		return fmt.Errorf("symbol report: %s basis must be static_fact", field)
	}
	if err := validateConfidence(field, relationship.Confidence); err != nil {
		return err
	}
	if err := validateEntityRef(field+".symbol", relationship.Symbol, allowedPaths); err != nil {
		return err
	}
	if err := validateEvidenceIDs(field, relationship.EvidenceIDs, certainties); err != nil {
		return err
	}
	if relationship.Basis == BasisStaticFact {
		for _, id := range relationship.EvidenceIDs {
			if !certainties[id].supportsStaticFact {
				return fmt.Errorf("symbol report: %s cites non-static evidence %q as static_fact", field, id)
			}
		}
	}
	hasDirectionalEvidence := false
	hasMatchingEntity := false
	for _, id := range relationship.EvidenceIDs {
		if !strings.HasPrefix(id, evidencePrefix) {
			continue
		}
		hasDirectionalEvidence = true
		if expected, ok := expectedEntities[id]; ok && sameEntityRef(relationship.Symbol, expected) {
			hasMatchingEntity = true
		}
	}
	if !hasDirectionalEvidence {
		return fmt.Errorf("symbol report: %s must cite at least one %s evidence id", field, evidencePrefix)
	}
	if !hasMatchingEntity {
		return fmt.Errorf("symbol report: %s symbol does not match its %s evidence", field, evidencePrefix)
	}
	return nil
}

func validateEntityRef(field string, entity EntityRef, allowedPaths map[string]struct{}) error {
	if entity.Name == "" || entity.Path == "" || entity.Line <= 0 {
		return fmt.Errorf("symbol report: %s requires name, path, and positive line", field)
	}
	if _, ok := allowedPaths[entity.Path]; !ok {
		return fmt.Errorf("symbol report: %s references path outside allowed_paths: %q", field, entity.Path)
	}
	return nil
}

func validateFileRecommendation(field string, file FileRecommendation, requireTest bool, allowedPaths map[string]struct{}, certainties map[string]evidenceCertainty, paths map[string]map[string]struct{}, roles map[string]map[string]map[string]struct{}) error {
	if file.Path == "" || file.StructuralRole == "" {
		return fmt.Errorf("symbol report: %s requires path and structural_role", field)
	}
	if _, ok := allowedPaths[file.Path]; !ok {
		return fmt.Errorf("symbol report: %s references path outside allowed_paths: %q", field, file.Path)
	}
	if requireTest && !strings.HasSuffix(file.Path, "_test.go") {
		return fmt.Errorf("symbol report: %s path is not an explicit go test file: %q", field, file.Path)
	}
	if requireTest && file.StructuralRole != "test_reference" {
		return fmt.Errorf("symbol report: %s structural_role must be test_reference", field)
	}
	if err := validateEvidenceIDs(field, file.EvidenceIDs, certainties); err != nil {
		return err
	}
	for _, id := range file.EvidenceIDs {
		if _, ok := paths[id][file.Path]; !ok {
			continue
		}
		if _, ok := roles[id][file.Path][file.StructuralRole]; ok {
			return nil
		}
	}
	return fmt.Errorf("symbol report: %s path and structural_role are not supported by its evidence ids", field)
}

func validateEvidenceIDs(field string, ids []string, certainties map[string]evidenceCertainty) error {
	if len(ids) == 0 {
		return fmt.Errorf("symbol report: %s requires at least one evidence id", field)
	}
	for _, id := range ids {
		if _, ok := certainties[id]; !ok {
			return fmt.Errorf("symbol report: %s references unknown evidence id %q", field, id)
		}
	}
	return nil
}

func validateConfidence(field string, confidence float64) error {
	if confidence < 0 || confidence > 1 {
		return fmt.Errorf("symbol report: %s confidence must be between 0 and 1", field)
	}
	return nil
}

func (b Basis) valid() bool {
	switch b {
	case BasisStaticFact, BasisInference:
		return true
	default:
		return false
	}
}

type evidenceCertainty struct {
	supportsStaticFact bool
}

func evidenceCertainties(bundle Bundle) map[string]evidenceCertainty {
	result := make(map[string]evidenceCertainty)
	add := func(id string, certainty string) {
		result[id] = evidenceCertainty{supportsStaticFact: certainty == "static" || certainty == "observed" || certainty == "verified"}
	}
	add(bundle.Target.EvidenceID, string(bundle.Target.Certainty))
	for _, candidate := range bundle.Candidates {
		add(candidate.EvidenceID, string(candidate.Certainty))
	}
	for _, call := range bundle.IncomingCalls {
		add(call.EvidenceID, string(call.Certainty))
	}
	for _, call := range bundle.OutgoingCalls {
		add(call.EvidenceID, string(call.Certainty))
	}
	return result
}

func evidencePathSets(bundle Bundle) map[string]map[string]struct{} {
	result := make(map[string]map[string]struct{})
	addEntity := func(id string, entity EntityRef) {
		if result[id] == nil {
			result[id] = make(map[string]struct{})
		}
		if entity.Path != "" {
			result[id][entity.Path] = struct{}{}
		}
	}
	addEntity(bundle.Target.EvidenceID, entityRef(bundle.Target.Entity))
	for _, candidate := range bundle.Candidates {
		addEntity(candidate.EvidenceID, entityRef(candidate.Entity))
	}
	for _, call := range append(append([]CallFact{}, bundle.IncomingCalls...), bundle.OutgoingCalls...) {
		addEntity(call.EvidenceID, entityRef(call.Caller))
		addEntity(call.EvidenceID, entityRef(call.Callee))
		if call.Callsite != nil && call.Callsite.Path != "" {
			// addEntity above initializes this evidence bucket for every call.
			//nolint:nilaway
			result[call.EvidenceID][call.Callsite.Path] = struct{}{}
		}
	}
	return result
}

func evidenceRoleSets(bundle Bundle) map[string]map[string]map[string]struct{} {
	result := make(map[string]map[string]map[string]struct{})
	add := func(id, path, role string) {
		if path == "" {
			return
		}
		if result[id] == nil {
			result[id] = make(map[string]map[string]struct{})
		}
		if result[id][path] == nil {
			result[id][path] = make(map[string]struct{})
		}
		result[id][path][role] = struct{}{}
		if strings.HasSuffix(path, "_test.go") {
			result[id][path]["test_reference"] = struct{}{}
		}
	}
	add(bundle.Target.EvidenceID, bundle.Target.Entity.Location.Path, "target")
	for _, candidate := range bundle.Candidates {
		if candidate.Entity.Location != nil {
			add(candidate.EvidenceID, candidate.Entity.Location.Path, "candidate")
		}
	}
	for _, call := range bundle.IncomingCalls {
		if call.Caller.Location != nil {
			add(call.EvidenceID, call.Caller.Location.Path, "static_caller")
		}
		if call.Callee.Location != nil {
			add(call.EvidenceID, call.Callee.Location.Path, "target")
		}
		if call.Callsite != nil {
			add(call.EvidenceID, call.Callsite.Path, "callsite")
		}
	}
	for _, call := range bundle.OutgoingCalls {
		if call.Caller.Location != nil {
			add(call.EvidenceID, call.Caller.Location.Path, "target")
		}
		if call.Callee.Location != nil {
			add(call.EvidenceID, call.Callee.Location.Path, "static_callee")
		}
		if call.Callsite != nil {
			add(call.EvidenceID, call.Callsite.Path, "callsite")
		}
	}
	return result
}

func directionalEntityRefs(calls []CallFact, useCaller bool) map[string]EntityRef {
	result := make(map[string]EntityRef, len(calls))
	for _, call := range calls {
		entity := call.Callee
		if useCaller {
			entity = call.Caller
		}
		result[call.EvidenceID] = entityRef(entity)
	}
	return result
}

func entityRef(entity evidence.Entity) EntityRef {
	result := EntityRef{Name: entity.Name, Kind: string(entity.Kind)}
	if entity.Location != nil {
		result.Path = entity.Location.Path
		result.Line = entity.Location.Line
		result.Column = entity.Location.Column
	}
	return result
}

func sameEntityRef(left, right EntityRef) bool {
	return left.Name == right.Name && left.Kind == right.Kind && left.Path == right.Path && left.Line == right.Line
}

func makeStringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}
