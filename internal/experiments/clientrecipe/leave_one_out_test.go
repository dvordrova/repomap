package clientrecipe

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const (
	reducerLeaveOneOutVersion = 1
	reducerCandidateVersion   = 1
	frozenH1SHA256            = "ec8f7411ad09e3c68f5b8deabd30ea71753fb7774b2f6806c5b482c5a5c4cb01"
	frozenReceiptSHA256       = "014affc25712645629fc2e65aad1d98dc997ab9a12ae52a1c8c503b974395c30"
)

type reducerCandidate struct {
	Version             int      `json:"version"`
	H1SHA256            string   `json:"h1_sha256"`
	HeldOutInstanceID   string   `json:"held_out_instance_id"`
	TrainingInstanceIDs []string `json:"training_instance_ids"`
	RoleUniverse        []H1Role `json:"role_universe"`
	TaskRequiredRoles   []H1Role `json:"task_required_roles"`
	PredictedPatterns   []H1Role `json:"predicted_patterns"`
	SHA256              string   `json:"sha256"`
}

type reducerLeaveOneOutGolden struct {
	Version                  int                      `json:"version"`
	FreezeReceiptSHA256      string                   `json:"freeze_receipt_sha256"`
	H1SHA256                 string                   `json:"h1_sha256"`
	Hypothesis               string                   `json:"hypothesis"`
	Scope                    string                   `json:"scope"`
	ExtractionIsolation      bool                     `json:"extraction_isolation"`
	CandidateOracleIsolation bool                     `json:"candidate_oracle_isolation"`
	TaskContractStatus       string                   `json:"task_contract_status"`
	TaskRequiredRoles        []H1Role                 `json:"task_required_roles"`
	PatternRoleUniverse      []H1Role                 `json:"pattern_role_universe"`
	Folds                    []reducerLeaveOneOutFold `json:"folds"`
	Aggregate                reducerLeaveOneOutTotals `json:"aggregate"`
	GeneralizationStatus     string                   `json:"generalization_status"`
	UserUtilityStatus        string                   `json:"user_utility_status"`
	ProductionReadiness      string                   `json:"production_readiness"`
	Verdict                  EvaluationVerdict        `json:"verdict"`
	SHA256                   string                   `json:"sha256"`
}

type reducerLeaveOneOutFold struct {
	HeldOutInstanceID     string            `json:"held_out_instance_id"`
	HeldOutOracleID       string            `json:"held_out_oracle_id"`
	HeldOutName           string            `json:"held_out_name"`
	TrainingInstanceIDs   []string          `json:"training_instance_ids"`
	CandidateSHA256       string            `json:"candidate_sha256"`
	TaskContractStatus    string            `json:"task_contract_status"`
	TaskContractMatched   int               `json:"task_contract_matched"`
	TaskContractTotal     int               `json:"task_contract_total"`
	PredictedPatternRoles []H1Role          `json:"predicted_pattern_roles"`
	HeldOutPatternRoles   []H1Role          `json:"held_out_pattern_roles"`
	TruePositiveRoles     []H1Role          `json:"true_positive_roles"`
	FalsePositiveRoles    []H1Role          `json:"false_positive_roles"`
	FalseNegativeRoles    []H1Role          `json:"false_negative_roles"`
	TruePositives         int               `json:"true_positives"`
	FalsePositives        int               `json:"false_positives"`
	FalseNegatives        int               `json:"false_negatives"`
	ExactPatternMatch     bool              `json:"exact_pattern_match"`
	Verdict               EvaluationVerdict `json:"verdict"`
}

type reducerLeaveOneOutTotals struct {
	Folds                 int `json:"folds"`
	ExactPatternMatches   int `json:"exact_pattern_matches"`
	TaskContractMatched   int `json:"task_contract_matched"`
	TaskContractTotal     int `json:"task_contract_total"`
	PatternTruePositives  int `json:"pattern_true_positives"`
	PatternFalsePositives int `json:"pattern_false_positives"`
	PatternFalseNegatives int `json:"pattern_false_negatives"`
}

func TestReducerLeaveOneCompleteBoundaryOut(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join(experimentRoot(t), "..", "..", ".."))
	freeze := loadReducerFreeze(t, repositoryRoot)
	h1, err := DecodeH1(readExperimentFile(t, filepath.Join(experimentRoot(t), "golden", "03-h1-structural.json")))
	if err != nil {
		t.Fatal(err)
	}
	if h1.SHA256 != frozenH1SHA256 || h1.SHA256 != freeze.BaselineH1SHA256 {
		t.Fatalf("reducer leave-one-out: H1 identity = %q", h1.SHA256)
	}
	oracle, err := DecodeOracle(readExperimentFile(t, filepath.Join(experimentRoot(t), "oracle.json")))
	if err != nil {
		t.Fatal(err)
	}

	first, err := buildReducerLeaveOneOut(h1, oracle, freeze.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	second, err := buildReducerLeaveOneOut(h1, oracle, freeze.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("reducer leave-one-out: two executions differ")
	}
	assertReducerExpectedFailure(t, first)
	assertReducerPassingScorecardRepresentable(t, first)
	assertReducerCandidateOracleIsolation(t, h1, oracle)

	raw, err := encodeReducerLeaveOneOut(first)
	if err != nil {
		t.Fatal(err)
	}
	var decoded reducerLeaveOneOutGolden
	if err := decodeStrict(raw, &decoded, "reducer leave-one-out golden"); err != nil {
		t.Fatal(err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, decoded) {
		t.Fatal("reducer leave-one-out: strict round trip changed scorecard")
	}
	assertStrictDecoder(t, raw, func(candidate []byte) error {
		var value reducerLeaveOneOutGolden
		return decodeStrict(candidate, &value, "reducer leave-one-out golden")
	})
	assertExperimentGolden(t, "07-reducer-leave-one-out.json", raw)
}

func loadReducerFreeze(t *testing.T, repositoryRoot string) h1FreezeReceipt {
	t.Helper()
	raw := readExperimentFile(t, filepath.Join(experimentRoot(t), "golden", "05-robustness.json"))
	if blindBytesSHA256(raw) != historicalRobustnessRawSHA256 {
		t.Fatal("reducer leave-one-out: historical robustness bytes changed")
	}
	var golden robustnessGolden
	if err := decodeStrict(raw, &golden, "robustness golden"); err != nil {
		t.Fatal(err)
	}
	if err := golden.Validate(repositoryRoot); err != nil {
		t.Fatal(err)
	}
	if golden.ExtractorFreeze.SHA256 != frozenReceiptSHA256 {
		t.Fatalf("reducer leave-one-out: freeze receipt = %q", golden.ExtractorFreeze.SHA256)
	}
	return golden.ExtractorFreeze
}

func buildReducerLeaveOneOut(h1 H1Result, oracle Oracle, receiptSHA string) (reducerLeaveOneOutGolden, error) {
	if err := h1.Validate(); err != nil {
		return reducerLeaveOneOutGolden{}, err
	}
	if err := oracle.Validate(); err != nil {
		return reducerLeaveOneOutGolden{}, err
	}
	complete := make([]H1Instance, 0, len(h1.Instances))
	for _, instance := range h1.Instances {
		if instance.Complete {
			complete = append(complete, instance)
		}
	}
	if len(complete) != 3 {
		return reducerLeaveOneOutGolden{}, fmt.Errorf("reducer leave-one-out: complete instances = %d, want 3", len(complete))
	}
	sort.Slice(complete, func(i, j int) bool { return complete[i].ID < complete[j].ID })

	result := reducerLeaveOneOutGolden{
		Version: reducerLeaveOneOutVersion, FreezeReceiptSHA256: receiptSHA, H1SHA256: h1.SHA256,
		Hypothesis: "required/common roles learned from two complete boundary instances predict the held-out instance's non-contract repository patterns",
		Scope:      "controlled_fixture_reducer_only", ExtractionIsolation: false,
		TaskContractStatus: "TASK_SUPPLIED_NOT_LEARNED", TaskRequiredRoles: reducerTaskRoles(),
		PatternRoleUniverse: reducerPatternRoles(), Folds: []reducerLeaveOneOutFold{},
		GeneralizationStatus: "NOT_ESTABLISHED", UserUtilityStatus: "NOT_TESTED",
		ProductionReadiness: "NOT_READY",
	}
	for index, heldOut := range complete {
		training := make([]H1Instance, 0, len(complete)-1)
		for candidateIndex, candidate := range complete {
			if candidateIndex != index {
				training = append(training, candidate)
			}
		}
		candidate, candidateRaw, err := buildReducerCandidate(h1.SHA256, heldOut.ID, training)
		if err != nil {
			return reducerLeaveOneOutGolden{}, err
		}
		repeated, repeatedRaw, err := buildReducerCandidate(h1.SHA256, heldOut.ID, training)
		if err != nil {
			return reducerLeaveOneOutGolden{}, err
		}
		if !reflect.DeepEqual(candidate, repeated) || !bytes.Equal(candidateRaw, repeatedRaw) {
			return reducerLeaveOneOutGolden{}, fmt.Errorf("reducer leave-one-out: candidate for %s is not deterministic", heldOut.ID)
		}
		fold, err := evaluateReducerCandidate(candidate, h1, oracle)
		if err != nil {
			return reducerLeaveOneOutGolden{}, err
		}
		result.Folds = append(result.Folds, fold)
	}
	result.Aggregate = reducerTotalsForFolds(result.Folds)
	result.Verdict = reducerAggregateVerdict(result.Aggregate)
	result.CandidateOracleIsolation = true
	result.SHA256 = reducerLeaveOneOutDigest(result)
	if err := result.Validate(); err != nil {
		return reducerLeaveOneOutGolden{}, err
	}
	return result, nil
}

func buildReducerCandidate(h1SHA, heldOutID string, training []H1Instance) (reducerCandidate, []byte, error) {
	if !validSHA256(h1SHA) || heldOutID == "" || len(training) != 2 {
		return reducerCandidate{}, nil, fmt.Errorf("reducer candidate: invalid input")
	}
	trainingCopy := append([]H1Instance(nil), training...)
	sort.Slice(trainingCopy, func(i, j int) bool { return trainingCopy[i].ID < trainingCopy[j].ID })
	ids := make([]string, 0, len(trainingCopy))
	for _, instance := range trainingCopy {
		if !instance.Complete || instance.ID == heldOutID {
			return reducerCandidate{}, nil, fmt.Errorf("reducer candidate: invalid training instance")
		}
		ids = append(ids, instance.ID)
	}
	if ids[0] == ids[1] {
		return reducerCandidate{}, nil, fmt.Errorf("reducer candidate: duplicate training instance")
	}
	frequencies, err := reduceH1Roles(trainingCopy)
	if err != nil {
		return reducerCandidate{}, nil, err
	}
	taskSet := reducerRoleSet(reducerTaskRoles())
	predicted := make([]H1Role, 0, 1)
	for _, frequency := range frequencies {
		if _, supplied := taskSet[frequency.Role]; supplied {
			continue
		}
		if frequency.Necessity == H1Required || frequency.Necessity == H1Common {
			predicted = append(predicted, frequency.Role)
		}
	}
	reducerSortRoles(predicted)
	result := reducerCandidate{
		Version: reducerCandidateVersion, H1SHA256: h1SHA, HeldOutInstanceID: heldOutID,
		TrainingInstanceIDs: ids, RoleUniverse: reducerAllRoles(), TaskRequiredRoles: reducerTaskRoles(),
		PredictedPatterns: predicted,
	}
	result.SHA256 = reducerCandidateDigest(result)
	if err := result.Validate(); err != nil {
		return reducerCandidate{}, nil, err
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return reducerCandidate{}, nil, fmt.Errorf("reducer candidate: encode: %w", err)
	}
	return result, raw, nil
}

func evaluateReducerCandidate(candidate reducerCandidate, h1 H1Result, oracle Oracle) (reducerLeaveOneOutFold, error) {
	if err := candidate.Validate(); err != nil {
		return reducerLeaveOneOutFold{}, err
	}
	if err := oracle.Validate(); err != nil {
		return reducerLeaveOneOutFold{}, err
	}
	var heldOut H1Instance
	found := false
	for _, instance := range h1.Instances {
		if instance.ID == candidate.HeldOutInstanceID {
			heldOut, found = instance, true
			break
		}
	}
	if !found || !heldOut.Complete {
		return reducerLeaveOneOutFold{}, fmt.Errorf("reducer leave-one-out: held-out instance is unavailable")
	}
	matches := matchH1Instances([]H1Instance{heldOut}, oracle.Instances)
	if len(matches) != 1 || matches[0].Prediction != heldOut.ID {
		return reducerLeaveOneOutFold{}, fmt.Errorf("reducer leave-one-out: held-out oracle join is not exact")
	}
	var truth OracleInstance
	for _, instance := range oracle.Instances {
		if instance.ID == matches[0].OracleID {
			truth = instance
			break
		}
	}
	if truth.ID == "" || !truth.Complete {
		return reducerLeaveOneOutFold{}, fmt.Errorf("reducer leave-one-out: held-out oracle truth is invalid")
	}
	taskSet := reducerRoleSet(candidate.TaskRequiredRoles)
	truthRoles := make([]H1Role, 0, 1)
	taskMatched := 0
	for _, slot := range truth.Slots {
		role := H1Role(slot.Role)
		if _, supplied := taskSet[role]; supplied {
			taskMatched++
			continue
		}
		truthRoles = append(truthRoles, role)
	}
	reducerSortRoles(truthRoles)
	tp, fp, fn := reducerRoleComparison(candidate.PredictedPatterns, truthRoles)
	fold := reducerLeaveOneOutFold{
		HeldOutInstanceID: heldOut.ID, HeldOutOracleID: truth.ID, HeldOutName: truth.Name,
		TrainingInstanceIDs: append([]string(nil), candidate.TrainingInstanceIDs...), CandidateSHA256: candidate.SHA256,
		TaskContractStatus: "TASK_SUPPLIED_NOT_LEARNED", TaskContractMatched: taskMatched,
		TaskContractTotal: len(candidate.TaskRequiredRoles), PredictedPatternRoles: append([]H1Role{}, candidate.PredictedPatterns...),
		HeldOutPatternRoles: truthRoles, TruePositiveRoles: tp, FalsePositiveRoles: fp, FalseNegativeRoles: fn,
		TruePositives: len(tp), FalsePositives: len(fp), FalseNegatives: len(fn),
	}
	fold.ExactPatternMatch = fold.FalsePositives == 0 && fold.FalseNegatives == 0
	fold.Verdict = reducerFoldVerdict(fold)
	return fold, nil
}

func assertReducerCandidateOracleIsolation(t *testing.T, h1 H1Result, oracle Oracle) {
	t.Helper()
	complete := make([]H1Instance, 0, 3)
	for _, instance := range h1.Instances {
		if instance.Complete {
			complete = append(complete, instance)
		}
	}
	sort.Slice(complete, func(i, j int) bool { return complete[i].ID < complete[j].ID })
	heldOut := complete[0]
	training := append([]H1Instance(nil), complete[1:]...)
	candidate, before, err := buildReducerCandidate(h1.SHA256, heldOut.ID, training)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := evaluateReducerCandidate(candidate, h1, oracle)
	if err != nil {
		t.Fatal(err)
	}
	mutated := reducerOracleWithoutFailurePolicy(t, oracle, baseline.HeldOutOracleID)
	rebuilt, after, err := buildReducerCandidate(h1.SHA256, heldOut.ID, training)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(candidate, rebuilt) || !bytes.Equal(before, after) {
		t.Fatal("reducer candidate changed after evaluator-only oracle mutation")
	}
	changed, err := evaluateReducerCandidate(rebuilt, h1, mutated)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(baseline, changed) || baseline.FalseNegatives != 1 || changed.FalseNegatives != 0 {
		t.Fatalf("reducer evaluator did not react only to oracle truth: before=%#v after=%#v", baseline, changed)
	}
}

func reducerOracleWithoutFailurePolicy(t *testing.T, value Oracle, instanceID string) Oracle {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var clone Oracle
	if err := json.Unmarshal(raw, &clone); err != nil {
		t.Fatal(err)
	}
	removed := false
	for index := range clone.Instances {
		if clone.Instances[index].ID != instanceID {
			continue
		}
		slots := clone.Instances[index].Slots[:0]
		for _, slot := range clone.Instances[index].Slots {
			if slot.Role == string(H1RoleFailurePolicy) {
				removed = true
				continue
			}
			slots = append(slots, slot)
		}
		clone.Instances[index].Slots = slots
	}
	if !removed {
		t.Fatalf("reducer isolation: %s has no failure policy to remove", instanceID)
	}
	for index := range clone.ExpectedRoles {
		if clone.ExpectedRoles[index].Role == string(H1RoleFailurePolicy) {
			clone.ExpectedRoles[index].ObservedCompleteInstances--
			clone.ExpectedRoles[index].Necessity = roleNecessity(clone.ExpectedRoles[index].ObservedCompleteInstances, 3)
		}
	}
	if err := clone.Validate(); err != nil {
		t.Fatal(err)
	}
	return clone
}

func assertReducerExpectedFailure(t *testing.T, value reducerLeaveOneOutGolden) {
	t.Helper()
	if value.Verdict != EvaluationFail || value.ExtractionIsolation || !value.CandidateOracleIsolation ||
		value.Aggregate != (reducerLeaveOneOutTotals{
			Folds: 3, ExactPatternMatches: 0, TaskContractMatched: 24, TaskContractTotal: 24,
			PatternTruePositives: 0, PatternFalsePositives: 1, PatternFalseNegatives: 2,
		}) {
		t.Fatalf("reducer leave-one-out result = %#v", value)
	}
	want := []struct {
		oracleID string
		fp       int
		fn       int
	}{
		{oracleID: "kubernetes", fp: 0, fn: 1},
		{oracleID: "clickhouse", fp: 1, fn: 0},
		{oracleID: "vault", fp: 0, fn: 1},
	}
	if len(value.Folds) != len(want) {
		t.Fatalf("reducer leave-one-out folds = %d", len(value.Folds))
	}
	for index, expected := range want {
		fold := value.Folds[index]
		if fold.HeldOutOracleID != expected.oracleID || fold.TruePositives != 0 ||
			fold.FalsePositives != expected.fp || fold.FalseNegatives != expected.fn ||
			fold.ExactPatternMatch || fold.Verdict != EvaluationFail || fold.TaskContractMatched != 8 ||
			fold.TaskContractTotal != 8 || fold.TaskContractStatus != "TASK_SUPPLIED_NOT_LEARNED" {
			t.Fatalf("reducer leave-one-out fold %d = %#v", index, fold)
		}
	}
}

func assertReducerPassingScorecardRepresentable(t *testing.T, value reducerLeaveOneOutGolden) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var passing reducerLeaveOneOutGolden
	if err := json.Unmarshal(raw, &passing); err != nil {
		t.Fatal(err)
	}
	for index := range passing.Folds {
		fold := &passing.Folds[index]
		fold.HeldOutPatternRoles = append([]H1Role{}, fold.PredictedPatternRoles...)
		fold.TruePositiveRoles = append([]H1Role{}, fold.PredictedPatternRoles...)
		fold.FalsePositiveRoles = []H1Role{}
		fold.FalseNegativeRoles = []H1Role{}
		fold.TruePositives = len(fold.TruePositiveRoles)
		fold.FalsePositives = 0
		fold.FalseNegatives = 0
		fold.ExactPatternMatch = true
		fold.Verdict = reducerFoldVerdict(*fold)
	}
	passing.Aggregate = reducerTotalsForFolds(passing.Folds)
	passing.Verdict = reducerAggregateVerdict(passing.Aggregate)
	passing.SHA256 = reducerLeaveOneOutDigest(passing)
	if passing.Verdict != EvaluationPass {
		t.Fatalf("reducer leave-one-out: exact synthetic scorecard verdict = %s", passing.Verdict)
	}
	if err := passing.Validate(); err != nil {
		t.Fatalf("reducer leave-one-out: PASS is not representable: %v", err)
	}
}

func (value reducerCandidate) Validate() error {
	if value.Version != reducerCandidateVersion || !validSHA256(value.H1SHA256) || value.HeldOutInstanceID == "" ||
		len(value.TrainingInstanceIDs) != 2 || len(value.RoleUniverse) != 9 || len(value.TaskRequiredRoles) != 8 ||
		value.PredictedPatterns == nil || !validSHA256(value.SHA256) || !reducerSortedStrings(value.TrainingInstanceIDs) ||
		!reducerSortedRoles(value.RoleUniverse) || !reducerSortedRoles(value.TaskRequiredRoles) ||
		!reducerSortedRoles(value.PredictedPatterns) {
		return fmt.Errorf("reducer candidate: invalid identity or canonical form")
	}
	if !reflect.DeepEqual(value.RoleUniverse, reducerAllRoles()) || !reflect.DeepEqual(value.TaskRequiredRoles, reducerTaskRoles()) {
		return fmt.Errorf("reducer candidate: role contract drift")
	}
	taskSet := reducerRoleSet(value.TaskRequiredRoles)
	universe := reducerRoleSet(value.RoleUniverse)
	for _, role := range value.PredictedPatterns {
		if _, found := universe[role]; !found {
			return fmt.Errorf("reducer candidate: predicted role outside universe")
		}
		if _, supplied := taskSet[role]; supplied {
			return fmt.Errorf("reducer candidate: task-supplied role entered learned prediction")
		}
	}
	if value.SHA256 != reducerCandidateDigest(value) {
		return fmt.Errorf("reducer candidate: digest mismatch")
	}
	return nil
}

func (value reducerLeaveOneOutGolden) Validate() error {
	if value.Version != reducerLeaveOneOutVersion || value.FreezeReceiptSHA256 != frozenReceiptSHA256 ||
		value.H1SHA256 != frozenH1SHA256 || value.Hypothesis == "" || value.Scope != "controlled_fixture_reducer_only" ||
		value.ExtractionIsolation || !value.CandidateOracleIsolation || value.TaskContractStatus != "TASK_SUPPLIED_NOT_LEARNED" ||
		!reflect.DeepEqual(value.TaskRequiredRoles, reducerTaskRoles()) ||
		!reflect.DeepEqual(value.PatternRoleUniverse, reducerPatternRoles()) || len(value.Folds) != 3 ||
		value.GeneralizationStatus != "NOT_ESTABLISHED" || value.UserUtilityStatus != "NOT_TESTED" ||
		value.ProductionReadiness != "NOT_READY" || !value.Verdict.Valid() || !validSHA256(value.SHA256) {
		return fmt.Errorf("reducer leave-one-out: invalid identity or status")
	}
	previous := ""
	for _, fold := range value.Folds {
		truePositives, falsePositives, falseNegatives := reducerRoleComparison(
			fold.PredictedPatternRoles,
			fold.HeldOutPatternRoles,
		)
		exact := len(falsePositives) == 0 && len(falseNegatives) == 0
		if fold.HeldOutInstanceID == "" || fold.HeldOutOracleID == "" || fold.HeldOutName == "" ||
			!validSHA256(fold.CandidateSHA256) || len(fold.TrainingInstanceIDs) != 2 ||
			!reducerSortedStrings(fold.TrainingInstanceIDs) || fold.TaskContractStatus != value.TaskContractStatus ||
			fold.TaskContractMatched != 8 || fold.TaskContractTotal != 8 ||
			fold.PredictedPatternRoles == nil || fold.HeldOutPatternRoles == nil || fold.TruePositiveRoles == nil ||
			fold.FalsePositiveRoles == nil || fold.FalseNegativeRoles == nil ||
			!reducerSortedRoles(fold.PredictedPatternRoles) || !reducerSortedRoles(fold.HeldOutPatternRoles) ||
			!reducerSortedRoles(fold.TruePositiveRoles) || !reducerSortedRoles(fold.FalsePositiveRoles) ||
			!reducerSortedRoles(fold.FalseNegativeRoles) || fold.TruePositives != len(fold.TruePositiveRoles) ||
			fold.FalsePositives != len(fold.FalsePositiveRoles) || fold.FalseNegatives != len(fold.FalseNegativeRoles) ||
			!reflect.DeepEqual(fold.TruePositiveRoles, truePositives) ||
			!reflect.DeepEqual(fold.FalsePositiveRoles, falsePositives) ||
			!reflect.DeepEqual(fold.FalseNegativeRoles, falseNegatives) ||
			fold.ExactPatternMatch != exact || fold.Verdict != reducerFoldVerdict(fold) ||
			(previous != "" && previous >= fold.HeldOutInstanceID) {
			return fmt.Errorf("reducer leave-one-out: invalid fold %q", fold.HeldOutInstanceID)
		}
		previous = fold.HeldOutInstanceID
	}
	totals := reducerTotalsForFolds(value.Folds)
	if !reflect.DeepEqual(value.Aggregate, totals) || value.Verdict != reducerAggregateVerdict(totals) ||
		value.SHA256 != reducerLeaveOneOutDigest(value) {
		return fmt.Errorf("reducer leave-one-out: aggregate or digest mismatch")
	}
	return nil
}

func reducerFoldVerdict(fold reducerLeaveOneOutFold) EvaluationVerdict {
	if fold.FalsePositives == 0 && fold.FalseNegatives == 0 {
		return EvaluationPass
	}
	return EvaluationFail
}

func reducerAggregateVerdict(totals reducerLeaveOneOutTotals) EvaluationVerdict {
	if totals.ExactPatternMatches == totals.Folds && totals.PatternFalsePositives == 0 &&
		totals.PatternFalseNegatives == 0 {
		return EvaluationPass
	}
	return EvaluationFail
}

func reducerTotalsForFolds(folds []reducerLeaveOneOutFold) reducerLeaveOneOutTotals {
	totals := reducerLeaveOneOutTotals{}
	for _, fold := range folds {
		totals.Folds++
		totals.TaskContractMatched += fold.TaskContractMatched
		totals.TaskContractTotal += fold.TaskContractTotal
		totals.PatternTruePositives += fold.TruePositives
		totals.PatternFalsePositives += fold.FalsePositives
		totals.PatternFalseNegatives += fold.FalseNegatives
		if fold.ExactPatternMatch {
			totals.ExactPatternMatches++
		}
	}
	return totals
}

func encodeReducerLeaveOneOut(value reducerLeaveOneOutGolden) ([]byte, error) {
	if err := value.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("reducer leave-one-out: encode: %w", err)
	}
	return append(raw, '\n'), nil
}

func reducerRoleComparison(predicted, truth []H1Role) (tp, fp, fn []H1Role) {
	tp = []H1Role{}
	fp = []H1Role{}
	fn = []H1Role{}
	predictedSet := reducerRoleSet(predicted)
	truthSet := reducerRoleSet(truth)
	for _, role := range predicted {
		if _, found := truthSet[role]; found {
			tp = append(tp, role)
		} else {
			fp = append(fp, role)
		}
	}
	for _, role := range truth {
		if _, found := predictedSet[role]; !found {
			fn = append(fn, role)
		}
	}
	return tp, fp, fn
}

func reducerAllRoles() []H1Role {
	roles := append([]H1Role(nil), h1Roles...)
	reducerSortRoles(roles)
	return roles
}

func reducerTaskRoles() []H1Role {
	roles := make([]H1Role, 0, len(h1MandatoryRoles))
	for role := range h1MandatoryRoles {
		roles = append(roles, role)
	}
	reducerSortRoles(roles)
	return roles
}

func reducerPatternRoles() []H1Role {
	task := reducerRoleSet(reducerTaskRoles())
	roles := make([]H1Role, 0, 1)
	for _, role := range reducerAllRoles() {
		if _, supplied := task[role]; !supplied {
			roles = append(roles, role)
		}
	}
	return roles
}

func reducerRoleSet(roles []H1Role) map[H1Role]struct{} {
	result := make(map[H1Role]struct{}, len(roles))
	for _, role := range roles {
		result[role] = struct{}{}
	}
	return result
}

func reducerSortRoles(roles []H1Role) {
	sort.Slice(roles, func(i, j int) bool { return roles[i] < roles[j] })
}

func reducerSortedRoles(roles []H1Role) bool {
	for index, role := range roles {
		if role == "" || (index > 0 && roles[index-1] >= role) {
			return false
		}
	}
	return true
}

func reducerSortedStrings(values []string) bool {
	for index, value := range values {
		if strings.TrimSpace(value) == "" || (index > 0 && values[index-1] >= value) {
			return false
		}
	}
	return true
}

func reducerCandidateDigest(value reducerCandidate) string {
	value.SHA256 = ""
	raw, _ := json.Marshal(value)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func reducerLeaveOneOutDigest(value reducerLeaveOneOutGolden) string {
	value.SHA256 = ""
	raw, _ := json.Marshal(value)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
