package sourceexplain

import (
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/sourcecard"
)

const ReportVersion = 1

type Verdict string

const (
	VerdictShown     Verdict = "shown"
	VerdictNotShown  Verdict = "not_shown"
	VerdictAmbiguous Verdict = "ambiguous"
)

type EvidenceLevel string

const EvidenceLevelSourceSupported EvidenceLevel = "source_supported"

type UnknownKind string

const (
	UnknownCalleeBehavior      UnknownKind = "callee_behavior"
	UnknownTestCoverage        UnknownKind = "test_coverage"
	UnknownRuntimeReachability UnknownKind = "runtime_reachability"
	UnknownDynamicCalls        UnknownKind = "dynamic_calls"
	UnknownBuildVariants       UnknownKind = "build_variants"
)

type ActionOrigin string

const (
	ActionOriginModel       ActionOrigin = "model"
	ActionOriginLocalPolicy ActionOrigin = "local_policy"
)

type Report struct {
	Version     int               `json:"version"`
	Target      sourcecard.Target `json:"target"`
	Assessments []Assessment      `json:"assessments"`
	Claims      []Claim           `json:"claims"`
	Unknowns    []Unknown         `json:"unknowns"`
	NextAction  Action            `json:"next_action"`
}

type Assessment struct {
	QuestionID        string   `json:"question_id"`
	Verdict           Verdict  `json:"verdict"`
	SourceEvidenceIDs []string `json:"source_evidence_ids"`
}

type Claim struct {
	Predicate             Predicate     `json:"predicate"`
	Statement             string        `json:"statement"`
	EvidenceLevel         EvidenceLevel `json:"evidence_level"`
	SourceEvidenceIDs     []string      `json:"source_evidence_ids"`
	StructuralEvidenceIDs []string      `json:"structural_evidence_ids"`
}

type Unknown struct {
	Kind             UnknownKind `json:"kind"`
	AnchorEvidenceID string      `json:"anchor_evidence_id"`
}

type Action struct {
	ID               string       `json:"id"`
	Operation        Operation    `json:"operation"`
	AnchorEvidenceID string       `json:"anchor_evidence_id"`
	Origin           ActionOrigin `json:"origin"`
}

func ValidateReport(bundle Bundle, report Report) error {
	if err := bundle.Validate(); err != nil {
		return err
	}
	if report.Version != ReportVersion {
		return fmt.Errorf("source explain report: unsupported version %d", report.Version)
	}
	if report.Target != bundle.Target {
		return fmt.Errorf("source explain report: target does not match bundle")
	}
	if len(report.Assessments) != len(bundle.Questions) {
		return fmt.Errorf("source explain report: assessments do not cover every question")
	}

	questions := questionMap(bundle)
	assessmentByQuestion := make(map[string]Assessment, len(report.Assessments))
	for index, assessment := range report.Assessments {
		question, ok := questions[assessment.QuestionID]
		if !ok {
			return fmt.Errorf("source explain report: assessments[%d] has unknown question %q", index, assessment.QuestionID)
		}
		if _, exists := assessmentByQuestion[assessment.QuestionID]; exists {
			return fmt.Errorf("source explain report: duplicate assessment for %q", assessment.QuestionID)
		}
		if !assessment.Verdict.valid() {
			return fmt.Errorf("source explain report: assessment %q has invalid verdict %q", assessment.QuestionID, assessment.Verdict)
		}
		if err := validateAssessmentEvidence(bundle, question, assessment); err != nil {
			return err
		}
		assessmentByQuestion[assessment.QuestionID] = assessment
	}

	claimByPredicateAnchor := make(map[string]Claim, len(report.Claims))
	for index, claim := range report.Claims {
		if claim.EvidenceLevel != EvidenceLevelSourceSupported {
			return fmt.Errorf("source explain report: claims[%d] has invalid evidence level %q", index, claim.EvidenceLevel)
		}
		if len(claim.StructuralEvidenceIDs) != 1 {
			return fmt.Errorf("source explain report: claims[%d] requires one structural anchor", index)
		}
		key := claimKey(claim.Predicate, claim.StructuralEvidenceIDs[0])
		if _, exists := claimByPredicateAnchor[key]; exists {
			return fmt.Errorf("source explain report: duplicate claim %q", key)
		}
		claimByPredicateAnchor[key] = claim
	}
	matchedClaims := 0
	for _, question := range bundle.Questions {
		assessment := assessmentByQuestion[question.ID]
		key := claimKey(question.Predicate, question.AnchorEvidenceID)
		claim, hasClaim := claimByPredicateAnchor[key]
		if assessment.Verdict != VerdictShown {
			if hasClaim {
				return fmt.Errorf("source explain report: unresolved question %q produced a claim", question.ID)
			}
			continue
		}
		if !hasClaim {
			return fmt.Errorf("source explain report: shown question %q has no claim", question.ID)
		}
		matchedClaims++
		statement, supported := supportedClaimStatement(bundle, question, assessment.SourceEvidenceIDs)
		if !supported {
			return fmt.Errorf("source explain report: shown question %q lacks predicate-specific lexical support", question.ID)
		}
		if claim.Statement != statement ||
			!equalStringSlices(claim.SourceEvidenceIDs, assessment.SourceEvidenceIDs) {
			return fmt.Errorf("source explain report: claim for %q was not reconstructed locally", question.ID)
		}
	}
	if matchedClaims != len(report.Claims) {
		return fmt.Errorf("source explain report: claim accounting mismatch")
	}

	knownAnchors := knownAnchorSet(bundle)
	seenUnknowns := make(map[string]struct{}, len(report.Unknowns))
	for index, unknown := range report.Unknowns {
		if !unknown.Kind.valid() {
			return fmt.Errorf("source explain report: unknowns[%d] has invalid kind %q", index, unknown.Kind)
		}
		if _, ok := knownAnchors[unknown.AnchorEvidenceID]; !ok {
			return fmt.Errorf("source explain report: unknowns[%d] has invalid anchor %q", index, unknown.AnchorEvidenceID)
		}
		key := string(unknown.Kind) + "\x00" + unknown.AnchorEvidenceID
		if _, exists := seenUnknowns[key]; exists {
			return fmt.Errorf("source explain report: duplicate unknown %q", key)
		}
		seenUnknowns[key] = struct{}{}
	}
	if _, ok := seenUnknowns[string(UnknownTestCoverage)+"\x00"+bundle.Target.EvidenceID]; !ok {
		return fmt.Errorf("source explain report: test coverage must remain unknown")
	}
	if _, ok := seenUnknowns[string(UnknownRuntimeReachability)+"\x00"+bundle.Target.EvidenceID]; !ok {
		return fmt.Errorf("source explain report: runtime reachability must remain unknown")
	}

	allowedActions := actionMap(bundle)
	allowed, ok := allowedActions[report.NextAction.ID]
	if !ok || allowed.Operation != report.NextAction.Operation || allowed.AnchorEvidenceID != report.NextAction.AnchorEvidenceID {
		return fmt.Errorf("source explain report: next action is not allowed")
	}
	if report.NextAction.Origin != ActionOriginModel && report.NextAction.Origin != ActionOriginLocalPolicy {
		return fmt.Errorf("source explain report: invalid action origin %q", report.NextAction.Origin)
	}
	return nil
}

func validateAssessmentEvidence(bundle Bundle, question Question, assessment Assessment) error {
	candidates := makeStringSet(question.CandidateSourceEvidenceIDs)
	seen := make(map[string]struct{}, len(assessment.SourceEvidenceIDs))
	hasAnchor := false
	for _, id := range assessment.SourceEvidenceIDs {
		if _, ok := candidates[id]; !ok {
			return fmt.Errorf("source explain report: assessment %q cites irrelevant source evidence %q", question.ID, id)
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("source explain report: assessment %q repeats source evidence %q", question.ID, id)
		}
		seen[id] = struct{}{}
		if id == question.AnchorSourceEvidenceID {
			hasAnchor = true
		}
	}
	if assessment.Verdict == VerdictShown && !hasAnchor {
		return fmt.Errorf("source explain report: shown assessment %q omits its anchor source evidence", question.ID)
	}
	if assessment.Verdict == VerdictShown {
		if _, supported := supportedClaimStatement(bundle, question, assessment.SourceEvidenceIDs); !supported {
			return fmt.Errorf("source explain report: shown assessment %q lacks predicate-specific lexical support", question.ID)
		}
	}
	if assessment.Verdict == VerdictNotShown {
		return fmt.Errorf("source explain report: lexical source cannot validate not_shown assessment %q", question.ID)
	}
	return nil
}

func questionMap(bundle Bundle) map[string]Question {
	result := make(map[string]Question, len(bundle.Questions))
	for _, question := range bundle.Questions {
		result[question.ID] = question
	}
	return result
}

func actionMap(bundle Bundle) map[string]AllowedAction {
	result := make(map[string]AllowedAction, len(bundle.AllowedActions))
	for _, action := range bundle.AllowedActions {
		result[action.ID] = action
	}
	return result
}

func knownAnchorSet(bundle Bundle) map[string]struct{} {
	result := map[string]struct{}{bundle.Target.EvidenceID: {}}
	for _, question := range bundle.Questions {
		result[question.AnchorEvidenceID] = struct{}{}
	}
	return result
}

func makeStringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func claimKey(predicate Predicate, anchor string) string {
	return string(predicate) + "\x00" + anchor
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sortEvidenceIDs(ids []string, lines []sourcecard.Line) {
	lineNumbers := make(map[string]int, len(lines))
	for _, line := range lines {
		lineNumbers[line.EvidenceID] = line.Line
	}
	sort.Slice(ids, func(i, j int) bool {
		return lineNumbers[ids[i]] < lineNumbers[ids[j]]
	})
}

func (v Verdict) valid() bool {
	return v == VerdictShown || v == VerdictNotShown || v == VerdictAmbiguous
}

func (k UnknownKind) valid() bool {
	switch k {
	case UnknownCalleeBehavior,
		UnknownTestCoverage,
		UnknownRuntimeReachability,
		UnknownDynamicCalls,
		UnknownBuildVariants:
		return true
	default:
		return false
	}
}
