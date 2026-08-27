package clientrecipe

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"path"
	"sort"
)

const EvaluationVersion = 1

type EvaluationVerdict string

const (
	EvaluationPass    EvaluationVerdict = "PASS"
	EvaluationPartial EvaluationVerdict = "PARTIAL"
	EvaluationFail    EvaluationVerdict = "FAIL"
)

type EvaluationSetMetric struct {
	Truth     int     `json:"truth"`
	Predicted int     `json:"predicted"`
	Matched   int     `json:"matched"`
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
}

type EvaluationExactMetric struct {
	Correct int     `json:"correct"`
	Total   int     `json:"total"`
	Score   float64 `json:"score"`
}

type EvaluationStage struct {
	TaskSemanticAuthority  bool                  `json:"task_semantic_authority"`
	InstanceDiscovery      EvaluationSetMetric   `json:"instance_discovery"`
	CriticalFalsePositives int                   `json:"critical_false_positives"`
	RoleCoverage           EvaluationSetMetric   `json:"role_coverage"`
	EvidenceGrounding      EvaluationExactMetric `json:"evidence_grounding"`
	ExclusionGrounding     EvaluationExactMetric `json:"exclusion_grounding"`
	Completeness           EvaluationExactMetric `json:"completeness"`
	VerificationKind       EvaluationExactMetric `json:"verification_kind"`
	ExclusionDiscovery     EvaluationSetMetric   `json:"exclusion_discovery"`
	ExclusionReason        EvaluationExactMetric `json:"exclusion_reason"`
	RoleReduction          EvaluationExactMetric `json:"role_reduction"`
	BestEligibility        EvaluationSetMetric   `json:"best_eligibility"`
	Accounting             EvaluationExactMetric `json:"accounting"`
	Callbacks              EvaluationCallbacks   `json:"callbacks"`
	ExactAccounting        bool                  `json:"exact_accounting"`
	Verdict                EvaluationVerdict     `json:"verdict"`
	Reasons                []string              `json:"reasons"`
}

type EvaluationCallbacks struct {
	Observed int  `json:"observed"`
	Closed   int  `json:"closed"`
	Frontier int  `json:"frontier"`
	Exact    bool `json:"exact"`
}

type EvaluationThresholds struct {
	InstancePrecisionMin float64 `json:"instance_precision_min"`
	InstanceRecallMin    float64 `json:"instance_recall_min"`
	CriticalFPMax        int     `json:"critical_fp_max"`
	RolePrecisionMin     float64 `json:"role_precision_min"`
	RoleCoverageMin      float64 `json:"role_coverage_min"`
	GroundingMin         float64 `json:"grounding_min"`
	BestPrecisionMin     float64 `json:"best_precision_min"`
	BestRecallMin        float64 `json:"best_recall_min"`
}

type EvaluationMatch struct {
	OracleID    string `json:"oracle_id"`
	Prediction  string `json:"prediction"`
	SharedFacts int    `json:"shared_facts"`
}

type EvaluationDeterminism struct {
	Runs          int  `json:"runs"`
	UniqueOutputs int  `json:"unique_outputs"`
	Passed        bool `json:"passed"`
}

type EvaluationResult struct {
	Version          int                   `json:"version"`
	H0SHA256         string                `json:"h0_sha256"`
	H1SHA256         string                `json:"h1_sha256"`
	OracleSHA256     string                `json:"oracle_sha256"`
	Rules            []string              `json:"rules"`
	Thresholds       EvaluationThresholds  `json:"thresholds"`
	H0               EvaluationStage       `json:"h0"`
	H1               EvaluationStage       `json:"h1"`
	InstanceMatches  []EvaluationMatch     `json:"instance_matches"`
	ExclusionMatches []EvaluationMatch     `json:"exclusion_matches"`
	EligibleBest     []string              `json:"eligible_best_instance_ids"`
	Determinism      EvaluationDeterminism `json:"determinism"`
	Verdict          EvaluationVerdict     `json:"verdict"`
	SHA256           string                `json:"sha256"`
}

// EvaluateClientRecipe is an evaluator-only boundary. It sees sealed H0/H1
// results and the hidden oracle, never repository source or extraction state.
func EvaluateClientRecipe(h0 H0Result, h1 H1Result, oracle Oracle, repeatedH1 ...H1Result) (EvaluationResult, error) {
	if err := h0.Validate(); err != nil {
		return EvaluationResult{}, err
	}
	if err := h1.Validate(); err != nil {
		return EvaluationResult{}, err
	}
	if err := oracle.Validate(); err != nil {
		return EvaluationResult{}, err
	}
	if h1.H0SHA256 != h0.SHA256 {
		return EvaluationResult{}, fmt.Errorf("client recipe evaluation: H0/H1 binding mismatch")
	}
	for _, repeated := range repeatedH1 {
		if err := repeated.Validate(); err != nil {
			return EvaluationResult{}, err
		}
	}

	h0Matched, h0Critical := matchH0Instances(h0.Candidates, oracle)
	h1Matches := matchH1Instances(h1.Instances, oracle.Instances)
	exclusionMatches := matchH1Exclusions(h1.Excluded, oracle.Excluded)

	h0Stage := EvaluationStage{
		TaskSemanticAuthority:  false,
		InstanceDiscovery:      newSetMetric(len(oracle.Instances), len(h0.Candidates), len(h0Matched)),
		CriticalFalsePositives: h0Critical,
		RoleCoverage:           newSetMetric(oracleRoleCount(oracle), 0, 0),
		EvidenceGrounding:      newExactMetric(0, oracleEvidenceCount(oracle)),
		ExclusionGrounding:     newExactMetric(0, oracleExclusionEvidenceCount(oracle)),
		Completeness:           newExactMetric(0, len(oracle.Instances)),
		VerificationKind:       newExactMetric(0, len(oracle.Instances)),
		ExclusionDiscovery:     newSetMetric(len(oracle.Excluded), 0, 0),
		ExclusionReason:        newExactMetric(0, len(oracle.Excluded)),
		RoleReduction:          newExactMetric(0, len(oracle.ExpectedRoles)),
		BestEligibility:        newSetMetric(len(oracle.AllowedBest), 0, 0),
		Accounting:             newExactMetric(h0.Ledger.Admitted+h0.Ledger.Excluded, h0.Ledger.Observed),
		Callbacks:              EvaluationCallbacks{},
		ExactAccounting:        h0.Ledger.Observed == len(h0.Candidates)+len(h0.Excluded),
		Verdict:                EvaluationPartial,
		Reasons: []string{
			"dependency facts do not establish task roles, completeness, grounding, or a best example",
			"dependency/importer candidates retain critical generated and unreachable false positives",
		},
	}

	roleMatched, rolePredicted := matchedRoles(h1, oracle, h1Matches)
	grounded, groundingTotal := matchedEvidence(h1, oracle, h1Matches)
	exclusionGrounded, exclusionGroundingTotal := matchedExclusionEvidence(h1, oracle, exclusionMatches)
	completeCorrect, verificationCorrect := matchedInstanceClassifications(h1, oracle, h1Matches)
	exclusionReasonCorrect := matchedExclusionReasons(h1, oracle, exclusionMatches)
	roleReductionCorrect := matchedRoleReduction(h1, oracle)
	eligibleBest := eligibleBestInstances(h1, h1Matches)
	bestMatched := stringIntersectionCount(eligibleBest, oracle.AllowedBest)
	h1Stage := EvaluationStage{
		TaskSemanticAuthority:  true,
		InstanceDiscovery:      newSetMetric(len(oracle.Instances), len(h1.Instances), len(h1Matches)),
		CriticalFalsePositives: len(h1.Instances) - len(h1Matches),
		RoleCoverage:           newSetMetric(oracleRoleCount(oracle), rolePredicted, roleMatched),
		EvidenceGrounding:      newExactMetric(grounded, groundingTotal),
		ExclusionGrounding:     newExactMetric(exclusionGrounded, exclusionGroundingTotal),
		Completeness:           newExactMetric(completeCorrect, len(oracle.Instances)),
		VerificationKind:       newExactMetric(verificationCorrect, len(oracle.Instances)),
		ExclusionDiscovery:     newSetMetric(len(oracle.Excluded), len(h1.Excluded), len(exclusionMatches)),
		ExclusionReason:        newExactMetric(exclusionReasonCorrect, len(oracle.Excluded)),
		RoleReduction:          newExactMetric(roleReductionCorrect, len(oracle.ExpectedRoles)),
		BestEligibility:        newSetMetric(len(oracle.AllowedBest), len(eligibleBest), bestMatched),
		Accounting:             newExactMetric(h1.Ledger.Admitted+h1.Ledger.Excluded, h1.Ledger.Observed),
		Callbacks: EvaluationCallbacks{
			Observed: h1.Callbacks.Observed, Closed: h1.Callbacks.Closed, Frontier: h1.Callbacks.Frontier,
			Exact: h1.Callbacks.Observed == oracle.Callbacks.Observed && h1.Callbacks.Closed == oracle.Callbacks.Closed &&
				h1.Callbacks.Frontier == oracle.Callbacks.Frontier,
		},
		ExactAccounting: h1.Ledger.Observed == len(h1.Instances)+len(h1.Excluded),
		Reasons:         []string{},
	}
	thresholds := EvaluationThresholds{
		InstancePrecisionMin: 1, InstanceRecallMin: 1, CriticalFPMax: 0,
		RolePrecisionMin: 1, RoleCoverageMin: 0.85, GroundingMin: 1,
		BestPrecisionMin: 1, BestRecallMin: 1,
	}
	h1Stage.Verdict = evaluateH1Verdict(h1Stage, thresholds)
	if h1Stage.Verdict != EvaluationPass {
		h1Stage.Reasons = append(h1Stage.Reasons, "one or more task-readiness thresholds failed")
	}

	determinism := evaluateDeterminism(h1, repeatedH1)
	result := EvaluationResult{
		Version: EvaluationVersion, H0SHA256: h0.SHA256, H1SHA256: h1.SHA256,
		OracleSHA256: oracleEvaluationDigest(oracle),
		Rules: []string{
			"best=maximize_learned_required_and_common_role_coverage_among_complete_instances",
			"callbacks=exact_hidden_oracle_observed_closed_frontier",
			"exclusion=unique_maximum_exact_path_line_overlap",
			"grounding=oracle_path_line_present_in_same_matched_role_or_exclusion",
			"h0_instance=importer_directory_equals_oracle_local_wrapper_directory",
			"h1_instance=exact_oracle_local_wrapper_path_line_anchor",
			"role=closed_role_label_after_one_to_one_instance_match",
		},
		Thresholds: thresholds, H0: h0Stage, H1: h1Stage,
		InstanceMatches: h1Matches, ExclusionMatches: exclusionMatches,
		EligibleBest: eligibleBest, Determinism: determinism,
		Verdict: EvaluationPass,
	}
	if h1Stage.Verdict != EvaluationPass || !determinism.Passed {
		result.Verdict = EvaluationFail
	}
	result.SHA256 = evaluationDigest(result)
	if err := result.Validate(); err != nil {
		return EvaluationResult{}, err
	}
	return result, nil
}

func EncodeEvaluation(value EvaluationResult) ([]byte, error) {
	if err := value.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("client recipe evaluation: encode: %w", err)
	}
	return append(raw, '\n'), nil
}

func DecodeEvaluation(raw []byte) (EvaluationResult, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var value EvaluationResult
	if err := decoder.Decode(&value); err != nil {
		return EvaluationResult{}, fmt.Errorf("client recipe evaluation: decode: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return EvaluationResult{}, fmt.Errorf("client recipe evaluation: trailing data")
	}
	if err := value.Validate(); err != nil {
		return EvaluationResult{}, err
	}
	canonical, err := EncodeEvaluation(value)
	if err != nil {
		return EvaluationResult{}, err
	}
	if !bytes.Equal(raw, canonical) {
		return EvaluationResult{}, fmt.Errorf("client recipe evaluation: non-canonical bytes")
	}
	return value, nil
}

func (value EvaluationResult) Validate() error {
	if value.Version != EvaluationVersion || !validSHA256(value.H0SHA256) || !validSHA256(value.H1SHA256) ||
		!validSHA256(value.OracleSHA256) || !validSHA256(value.SHA256) || len(value.Rules) == 0 ||
		!sortedUnique(value.Rules) || value.InstanceMatches == nil || value.ExclusionMatches == nil ||
		value.EligibleBest == nil || !sortedUnique(value.EligibleBest) {
		return fmt.Errorf("client recipe evaluation: invalid identity")
	}
	if err := validateEvaluationStage(value.H0); err != nil {
		return err
	}
	if err := validateEvaluationStage(value.H1); err != nil {
		return err
	}
	if value.Thresholds != (EvaluationThresholds{
		InstancePrecisionMin: 1, InstanceRecallMin: 1, CriticalFPMax: 0,
		RolePrecisionMin: 1, RoleCoverageMin: 0.85, GroundingMin: 1,
		BestPrecisionMin: 1, BestRecallMin: 1,
	}) || value.H0.Verdict != EvaluationPartial || value.H1.Verdict != evaluateH1Verdict(value.H1, value.Thresholds) {
		return fmt.Errorf("client recipe evaluation: invalid thresholds or stage verdict")
	}
	if !canonicalEvaluationMatches(value.InstanceMatches) || !canonicalEvaluationMatches(value.ExclusionMatches) {
		return fmt.Errorf("client recipe evaluation: non-canonical matches")
	}
	if value.Determinism.Runs < 1 || value.Determinism.UniqueOutputs < 1 ||
		value.Determinism.UniqueOutputs > value.Determinism.Runs ||
		value.Determinism.Passed != (value.Determinism.Runs >= 2 && value.Determinism.UniqueOutputs == 1) {
		return fmt.Errorf("client recipe evaluation: invalid determinism accounting")
	}
	wantVerdict := EvaluationFail
	if value.H1.Verdict == EvaluationPass && value.Determinism.Passed {
		wantVerdict = EvaluationPass
	}
	if value.Verdict != wantVerdict || value.SHA256 != evaluationDigest(value) {
		return fmt.Errorf("client recipe evaluation: verdict or digest mismatch")
	}
	return nil
}

func (value EvaluationResult) ValidateAgainst(h0 H0Result, h1 H1Result, oracle Oracle, repeatedH1 ...H1Result) error {
	if err := value.Validate(); err != nil {
		return err
	}
	want, err := EvaluateClientRecipe(h0, h1, oracle, repeatedH1...)
	if err != nil {
		return err
	}
	actualRaw, err := EncodeEvaluation(value)
	if err != nil {
		return err
	}
	wantRaw, err := EncodeEvaluation(want)
	if err != nil {
		return err
	}
	if !bytes.Equal(actualRaw, wantRaw) {
		return fmt.Errorf("client recipe evaluation: input-bound projection mismatch")
	}
	return nil
}

func validateEvaluationStage(value EvaluationStage) error {
	for _, metric := range []EvaluationSetMetric{value.InstanceDiscovery, value.RoleCoverage, value.ExclusionDiscovery, value.BestEligibility} {
		if metric.Truth < 0 || metric.Predicted < 0 || metric.Matched < 0 || metric.Matched > metric.Truth ||
			metric.Matched > metric.Predicted || metric.Precision != evaluationRatio(metric.Matched, metric.Predicted) ||
			metric.Recall != evaluationRatio(metric.Matched, metric.Truth) {
			return fmt.Errorf("client recipe evaluation: invalid set metric")
		}
	}
	for _, metric := range []EvaluationExactMetric{value.EvidenceGrounding, value.ExclusionGrounding, value.Completeness, value.VerificationKind, value.ExclusionReason, value.RoleReduction, value.Accounting} {
		if metric.Correct < 0 || metric.Total < 0 || metric.Correct > metric.Total || metric.Score != evaluationRatio(metric.Correct, metric.Total) {
			return fmt.Errorf("client recipe evaluation: invalid exact metric")
		}
	}
	if value.CriticalFalsePositives < 0 || !value.Verdict.Valid() || value.Reasons == nil || !sortedUnique(value.Reasons) {
		return fmt.Errorf("client recipe evaluation: invalid stage")
	}
	if value.Callbacks.Observed < 0 || value.Callbacks.Closed < 0 || value.Callbacks.Frontier < 0 {
		return fmt.Errorf("client recipe evaluation: invalid callback metric")
	}
	if !value.TaskSemanticAuthority && value.Callbacks != (EvaluationCallbacks{}) {
		return fmt.Errorf("client recipe evaluation: unavailable callbacks carry authority")
	}
	return nil
}

func (value EvaluationVerdict) Valid() bool {
	return value == EvaluationPass || value == EvaluationPartial || value == EvaluationFail
}

func newSetMetric(truth, predicted, matched int) EvaluationSetMetric {
	return EvaluationSetMetric{Truth: truth, Predicted: predicted, Matched: matched,
		Precision: evaluationRatio(matched, predicted), Recall: evaluationRatio(matched, truth)}
}

func newExactMetric(correct, total int) EvaluationExactMetric {
	return EvaluationExactMetric{Correct: correct, Total: total, Score: evaluationRatio(correct, total)}
}

func evaluationRatio(numerator, denominator int) float64 {
	if denominator == 0 {
		if numerator == 0 {
			return 1
		}
		return 0
	}
	return math.Round(float64(numerator)/float64(denominator)*1_000_000) / 1_000_000
}

func evaluateH1Verdict(stage EvaluationStage, thresholds EvaluationThresholds) EvaluationVerdict {
	if stage.InstanceDiscovery.Precision >= thresholds.InstancePrecisionMin &&
		stage.InstanceDiscovery.Recall >= thresholds.InstanceRecallMin &&
		stage.CriticalFalsePositives <= thresholds.CriticalFPMax &&
		stage.RoleCoverage.Precision >= thresholds.RolePrecisionMin &&
		stage.RoleCoverage.Recall >= thresholds.RoleCoverageMin &&
		stage.EvidenceGrounding.Score >= thresholds.GroundingMin &&
		stage.ExclusionGrounding.Score >= thresholds.GroundingMin &&
		stage.Completeness.Score == 1 && stage.VerificationKind.Score == 1 &&
		stage.ExclusionDiscovery.Precision == 1 && stage.ExclusionDiscovery.Recall == 1 &&
		stage.ExclusionReason.Score == 1 && stage.RoleReduction.Score == 1 &&
		stage.BestEligibility.Precision >= thresholds.BestPrecisionMin &&
		stage.BestEligibility.Recall >= thresholds.BestRecallMin && stage.Accounting.Score == 1 &&
		stage.ExactAccounting && stage.Callbacks.Exact && stage.TaskSemanticAuthority {
		return EvaluationPass
	}
	return EvaluationFail
}

func evaluateDeterminism(primary H1Result, repeats []H1Result) EvaluationDeterminism {
	unique := map[string]struct{}{primary.SHA256: {}}
	for _, repeated := range repeats {
		unique[repeated.SHA256] = struct{}{}
	}
	runs := 1 + len(repeats)
	return EvaluationDeterminism{Runs: runs, UniqueOutputs: len(unique), Passed: runs >= 2 && len(unique) == 1}
}

func matchH0Instances(candidates []H0Candidate, oracle Oracle) ([]EvaluationMatch, int) {
	result := make([]EvaluationMatch, 0)
	critical := 0
	used := make(map[string]struct{})
	for _, candidate := range candidates {
		matches := make([]string, 0, 1)
		for _, instance := range oracle.Instances {
			locator, found := oracleRoleAnchor(instance, string(H1RoleLocalWrapper))
			if found && path.Dir(locator.Path) == candidate.ImporterRepositoryPath {
				matches = append(matches, instance.ID)
			}
		}
		if len(matches) == 1 {
			if _, duplicate := used[matches[0]]; duplicate {
				critical++
				continue
			}
			used[matches[0]] = struct{}{}
			result = append(result, EvaluationMatch{OracleID: matches[0], Prediction: candidate.ID, SharedFacts: 1})
		} else {
			critical++
		}
	}
	sortEvaluationMatches(result)
	return result, critical
}

func matchH1Instances(predictions []H1Instance, truth []OracleInstance) []EvaluationMatch {
	proposals := make([]EvaluationMatch, 0)
	for _, prediction := range predictions {
		predictionAnchor, found := h1RoleAnchor(prediction, H1RoleLocalWrapper)
		if !found {
			continue
		}
		matchingTruth := make([]string, 0, 1)
		for _, candidate := range truth {
			truthAnchor, found := oracleRoleAnchor(candidate, string(H1RoleLocalWrapper))
			if found && evaluationLocatorKey(predictionAnchor.Path, predictionAnchor.Line) == evaluationLocatorKey(truthAnchor.Path, truthAnchor.Line) {
				matchingTruth = append(matchingTruth, candidate.ID)
			}
		}
		if len(matchingTruth) == 1 {
			proposals = append(proposals, EvaluationMatch{OracleID: matchingTruth[0], Prediction: prediction.ID, SharedFacts: 1})
		}
	}
	return uniqueEvaluationProposals(proposals)
}

func matchH1Exclusions(predictions []H1Excluded, truth []OracleExcluded) []EvaluationMatch {
	proposals := make([]EvaluationMatch, 0)
	for _, prediction := range predictions {
		bestID, bestScore, tied := "", 0, false
		for _, candidate := range truth {
			score := evidenceOverlap(prediction.Evidence, candidate.Evidence)
			switch {
			case score > bestScore:
				bestID, bestScore, tied = candidate.ID, score, false
			case score > 0 && score == bestScore:
				tied = true
			}
		}
		if bestScore > 0 && !tied {
			proposals = append(proposals, EvaluationMatch{OracleID: bestID, Prediction: prediction.ID, SharedFacts: bestScore})
		}
	}
	return uniqueEvaluationProposals(proposals)
}

func uniqueEvaluationProposals(proposals []EvaluationMatch) []EvaluationMatch {
	counts := make(map[string]int, len(proposals))
	for _, proposal := range proposals {
		counts[proposal.OracleID]++
	}
	result := make([]EvaluationMatch, 0, len(proposals))
	for _, proposal := range proposals {
		if counts[proposal.OracleID] == 1 {
			result = append(result, proposal)
		}
	}
	sortEvaluationMatches(result)
	return result
}

func evidenceOverlap(left []H1Evidence, right []SourceLocator) int {
	leftSet := make(map[string]struct{}, len(left))
	for _, evidence := range left {
		leftSet[evaluationLocatorKey(evidence.Path, evidence.Line)] = struct{}{}
	}
	result := 0
	for _, evidence := range right {
		if _, found := leftSet[evaluationLocatorKey(evidence.Path, evidence.Line)]; found {
			result++
		}
	}
	return result
}

func h1RoleAnchor(instance H1Instance, role H1Role) (H1Evidence, bool) {
	for _, candidate := range instance.Roles {
		if candidate.Role == role && len(candidate.Evidence) == 1 {
			return candidate.Evidence[0], true
		}
	}
	return H1Evidence{}, false
}

func oracleRoleAnchor(instance OracleInstance, role string) (SourceLocator, bool) {
	for _, candidate := range instance.Slots {
		if candidate.Role == role && len(candidate.Evidence) == 1 {
			return candidate.Evidence[0], true
		}
	}
	return SourceLocator{}, false
}

func matchedRoles(h1 H1Result, oracle Oracle, matches []EvaluationMatch) (int, int) {
	predictedByID := make(map[string]H1Instance, len(h1.Instances))
	for _, instance := range h1.Instances {
		predictedByID[instance.ID] = instance
	}
	truthByID := make(map[string]OracleInstance, len(oracle.Instances))
	for _, instance := range oracle.Instances {
		truthByID[instance.ID] = instance
	}
	matched := 0
	predicted := 0
	for _, instance := range h1.Instances {
		predicted += len(instance.Roles)
	}
	for _, match := range matches {
		truthRoles := make(map[string]struct{})
		for _, slot := range truthByID[match.OracleID].Slots {
			truthRoles[slot.Role] = struct{}{}
		}
		for _, role := range predictedByID[match.Prediction].Roles {
			if _, found := truthRoles[string(role.Role)]; found {
				matched++
			}
		}
	}
	return matched, predicted
}

func matchedEvidence(h1 H1Result, oracle Oracle, matches []EvaluationMatch) (int, int) {
	predictedByID := make(map[string]H1Instance, len(h1.Instances))
	for _, instance := range h1.Instances {
		predictedByID[instance.ID] = instance
	}
	truthByID := make(map[string]OracleInstance, len(oracle.Instances))
	for _, instance := range oracle.Instances {
		truthByID[instance.ID] = instance
	}
	correct, total := 0, 0
	for _, instance := range oracle.Instances {
		for _, slot := range instance.Slots {
			total += len(slot.Evidence)
		}
	}
	for _, match := range matches {
		predictedRoles := make(map[H1Role][]H1Evidence)
		for _, role := range predictedByID[match.Prediction].Roles {
			predictedRoles[role.Role] = role.Evidence
		}
		for _, slot := range truthByID[match.OracleID].Slots {
			correct += evidenceOverlap(predictedRoles[H1Role(slot.Role)], slot.Evidence)
		}
	}
	return correct, total
}

func matchedExclusionEvidence(h1 H1Result, oracle Oracle, matches []EvaluationMatch) (int, int) {
	predictedByID := make(map[string]H1Excluded, len(h1.Excluded))
	for _, row := range h1.Excluded {
		predictedByID[row.ID] = row
	}
	truthByID := make(map[string]OracleExcluded, len(oracle.Excluded))
	total := 0
	for _, row := range oracle.Excluded {
		truthByID[row.ID] = row
		total += len(row.Evidence)
	}
	correct := 0
	for _, match := range matches {
		correct += evidenceOverlap(predictedByID[match.Prediction].Evidence, truthByID[match.OracleID].Evidence)
	}
	return correct, total
}

func matchedInstanceClassifications(h1 H1Result, oracle Oracle, matches []EvaluationMatch) (int, int) {
	predictedByID := make(map[string]H1Instance, len(h1.Instances))
	for _, instance := range h1.Instances {
		predictedByID[instance.ID] = instance
	}
	truthByID := make(map[string]OracleInstance, len(oracle.Instances))
	for _, instance := range oracle.Instances {
		truthByID[instance.ID] = instance
	}
	completeCorrect, verificationCorrect := 0, 0
	for _, match := range matches {
		prediction, truth := predictedByID[match.Prediction], truthByID[match.OracleID]
		if prediction.Complete == truth.Complete {
			completeCorrect++
		}
		if prediction.VerificationKind == truth.VerificationKind {
			verificationCorrect++
		}
	}
	return completeCorrect, verificationCorrect
}

func matchedExclusionReasons(h1 H1Result, oracle Oracle, matches []EvaluationMatch) int {
	predictedByID := make(map[string]H1Excluded, len(h1.Excluded))
	for _, row := range h1.Excluded {
		predictedByID[row.ID] = row
	}
	truthByID := make(map[string]OracleExcluded, len(oracle.Excluded))
	for _, row := range oracle.Excluded {
		truthByID[row.ID] = row
	}
	correct := 0
	for _, match := range matches {
		if string(predictedByID[match.Prediction].Reason) == truthByID[match.OracleID].Reason {
			correct++
		}
	}
	return correct
}

func matchedRoleReduction(h1 H1Result, oracle Oracle) int {
	predicted := make(map[string]H1RoleFrequency, len(h1.Roles))
	for _, role := range h1.Roles {
		predicted[string(role.Role)] = role
	}
	correct := 0
	for _, truth := range oracle.ExpectedRoles {
		role, found := predicted[truth.Role]
		if found && role.CompleteInstances == truth.ObservedCompleteInstances && string(role.Necessity) == truth.Necessity {
			correct++
		}
	}
	return correct
}

func eligibleBestInstances(h1 H1Result, matches []EvaluationMatch) []string {
	predictedByID := make(map[string]H1Instance, len(h1.Instances))
	for _, instance := range h1.Instances {
		predictedByID[instance.ID] = instance
	}
	learned := make(map[H1Role]H1Necessity, len(h1.Roles))
	for _, role := range h1.Roles {
		learned[role.Role] = role.Necessity
	}
	type scored struct {
		oracleID string
		score    int
	}
	values := make([]scored, 0)
	bestScore := -1
	for _, match := range matches {
		instance := predictedByID[match.Prediction]
		if !instance.Complete {
			continue
		}
		score := 0
		for _, role := range instance.Roles {
			if learned[role.Role] == H1Required || learned[role.Role] == H1Common {
				score++
			}
		}
		values = append(values, scored{oracleID: match.OracleID, score: score})
		if score > bestScore {
			bestScore = score
		}
	}
	result := make([]string, 0)
	for _, value := range values {
		if value.score == bestScore {
			result = append(result, value.oracleID)
		}
	}
	sort.Strings(result)
	return result
}

func oracleRoleCount(oracle Oracle) int {
	result := 0
	for _, instance := range oracle.Instances {
		result += len(instance.Slots)
	}
	return result
}

func oracleEvidenceCount(oracle Oracle) int {
	result := 0
	for _, instance := range oracle.Instances {
		for _, slot := range instance.Slots {
			result += len(slot.Evidence)
		}
	}
	return result
}

func oracleExclusionEvidenceCount(oracle Oracle) int {
	result := 0
	for _, row := range oracle.Excluded {
		result += len(row.Evidence)
	}
	return result
}

func stringIntersectionCount(left, right []string) int {
	set := make(map[string]struct{}, len(left))
	for _, value := range left {
		set[value] = struct{}{}
	}
	result := 0
	for _, value := range right {
		if _, found := set[value]; found {
			result++
		}
	}
	return result
}

func evaluationLocatorKey(path string, line int) string {
	return fmt.Sprintf("%s:%09d", path, line)
}

func sortEvaluationMatches(values []EvaluationMatch) {
	sort.Slice(values, func(i, j int) bool {
		if values[i].OracleID != values[j].OracleID {
			return values[i].OracleID < values[j].OracleID
		}
		return values[i].Prediction < values[j].Prediction
	})
}

func canonicalEvaluationMatches(values []EvaluationMatch) bool {
	previous := ""
	seenPredictions := make(map[string]struct{}, len(values))
	seenOracle := make(map[string]struct{}, len(values))
	for _, value := range values {
		key := value.OracleID + "\x00" + value.Prediction
		if !validID(value.OracleID) || !validText(value.Prediction) || value.SharedFacts <= 0 ||
			(previous != "" && previous >= key) {
			return false
		}
		if _, duplicate := seenPredictions[value.Prediction]; duplicate {
			return false
		}
		if _, duplicate := seenOracle[value.OracleID]; duplicate {
			return false
		}
		seenPredictions[value.Prediction] = struct{}{}
		seenOracle[value.OracleID] = struct{}{}
		previous = key
	}
	return true
}

func oracleEvaluationDigest(value Oracle) string {
	raw, _ := json.Marshal(value)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func evaluationDigest(value EvaluationResult) string {
	value.SHA256 = ""
	raw, _ := json.Marshal(value)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
