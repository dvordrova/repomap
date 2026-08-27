package clientrecipe

import (
	"bytes"
	"path/filepath"
	"reflect"
	"testing"
)

func TestHypothesisEvaluation(t *testing.T) {
	repoRoot := filepath.Join(experimentRoot(t), "repo")
	authority := preparedFixtureAuthority(t)
	h0, err := BuildH0(authority)
	if err != nil {
		t.Fatal(err)
	}
	first, err := ExtractH1(repoRoot, authority)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ExtractH1(repoRoot, authority)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := DecodeOracle(readExperimentFile(t, filepath.Join(experimentRoot(t), "oracle.json")))
	if err != nil {
		t.Fatal(err)
	}
	result, err := EvaluateClientRecipe(h0, first, oracle, second)
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != EvaluationPass || result.H0.Verdict != EvaluationPartial || result.H1.Verdict != EvaluationPass {
		t.Fatalf("evaluation verdicts = overall %s, H0 %s, H1 %s", result.Verdict, result.H0.Verdict, result.H1.Verdict)
	}
	if result.H0.InstanceDiscovery != (EvaluationSetMetric{
		Truth: 4, Predicted: 6, Matched: 4, Precision: 0.666667, Recall: 1,
	}) || result.H0.CriticalFalsePositives != 2 || result.H0.TaskSemanticAuthority {
		t.Fatalf("H0 baseline = %#v", result.H0)
	}
	if result.H1.InstanceDiscovery != (EvaluationSetMetric{Truth: 4, Predicted: 4, Matched: 4, Precision: 1, Recall: 1}) ||
		result.H1.CriticalFalsePositives != 0 || !result.H1.TaskSemanticAuthority {
		t.Fatalf("H1 instance metrics = %#v", result.H1)
	}
	if result.H1.RoleCoverage != (EvaluationSetMetric{Truth: 32, Predicted: 32, Matched: 32, Precision: 1, Recall: 1}) ||
		result.H1.EvidenceGrounding != (EvaluationExactMetric{Correct: 43, Total: 43, Score: 1}) ||
		result.H1.ExclusionGrounding != (EvaluationExactMetric{Correct: 7, Total: 7, Score: 1}) {
		t.Fatalf("H1 role/grounding metrics = roles %#v, instances %#v, exclusions %#v",
			result.H1.RoleCoverage, result.H1.EvidenceGrounding, result.H1.ExclusionGrounding)
	}
	for label, metric := range map[string]EvaluationExactMetric{
		"completeness": result.H1.Completeness, "verification": result.H1.VerificationKind,
		"exclusion reasons": result.H1.ExclusionReason, "role reduction": result.H1.RoleReduction,
	} {
		if metric.Score != 1 {
			t.Errorf("%s score = %#v", label, metric)
		}
	}
	if result.H1.ExclusionDiscovery != (EvaluationSetMetric{Truth: 6, Predicted: 6, Matched: 6, Precision: 1, Recall: 1}) {
		t.Fatalf("H1 exclusion discovery = %#v", result.H1.ExclusionDiscovery)
	}
	if !reflect.DeepEqual(result.EligibleBest, []string{"kubernetes", "vault"}) ||
		result.H1.BestEligibility != (EvaluationSetMetric{Truth: 2, Predicted: 2, Matched: 2, Precision: 1, Recall: 1}) {
		t.Fatalf("best eligibility = %v / %#v", result.EligibleBest, result.H1.BestEligibility)
	}
	if result.Determinism != (EvaluationDeterminism{Runs: 2, UniqueOutputs: 1, Passed: true}) {
		t.Fatalf("determinism = %#v", result.Determinism)
	}
	assertEvaluatorRejectsInventedRole(t, h0, first, oracle)
	assertEvaluatorRejectsMissingCallbackClosure(t, h0, first, oracle)

	raw, err := EncodeEvaluation(result)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeEvaluation(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := decoded.ValidateAgainst(h0, first, oracle, second); err != nil {
		t.Fatal(err)
	}
	secondResult, err := EvaluateClientRecipe(h0, first, oracle, second)
	if err != nil {
		t.Fatal(err)
	}
	secondRaw, err := EncodeEvaluation(secondResult)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, secondRaw) {
		t.Fatal("evaluation bytes changed for identical sealed inputs")
	}
	assertStrictDecoder(t, raw, func(candidate []byte) error {
		_, err := DecodeEvaluation(candidate)
		return err
	})
	assertExperimentGolden(t, "04-evaluation.json", raw)
}

func assertEvaluatorRejectsMissingCallbackClosure(t *testing.T, h0 H0Result, h1 H1Result, oracle Oracle) {
	t.Helper()
	if len(h1.Callbacks.Closures) != 2 {
		t.Fatalf("callback closure fixture = %d, want 2", len(h1.Callbacks.Closures))
	}
	mutated := h1
	mutated.Callbacks.Closures = append([]H1CallbackClosure(nil), h1.Callbacks.Closures[:1]...)
	mutated.Callbacks.Closed = 1
	mutated.Callbacks.Frontier = mutated.Callbacks.Observed - mutated.Callbacks.Closed
	var err error
	mutated, err = sealH1(mutated)
	if err != nil {
		t.Fatal(err)
	}
	result, err := EvaluateClientRecipe(h0, mutated, oracle, mutated)
	if err != nil {
		t.Fatal(err)
	}
	if result.H1.Callbacks != (EvaluationCallbacks{Observed: 4, Closed: 1, Frontier: 3, Exact: false}) {
		t.Fatalf("tampered callback score = %#v", result.H1.Callbacks)
	}
	if result.H1.Verdict != EvaluationFail || result.Verdict != EvaluationFail {
		t.Fatalf("missing callback closure passed evaluator: overall %s / H1 %s", result.Verdict, result.H1.Verdict)
	}
}

func assertEvaluatorRejectsInventedRole(t *testing.T, h0 H0Result, h1 H1Result, oracle Oracle) {
	t.Helper()
	mutated := h1
	mutated.Instances = append([]H1Instance(nil), h1.Instances...)
	found := false
	for index := range mutated.Instances {
		if mutated.Instances[index].Complete {
			continue
		}
		mutated.Instances[index].Roles = append([]H1RoleEvidence(nil), mutated.Instances[index].Roles...)
		var evidence []H1Evidence
		for _, role := range mutated.Instances[index].Roles {
			if role.Role == H1RoleProductionOperation {
				evidence = append([]H1Evidence(nil), role.Evidence...)
				break
			}
		}
		if len(evidence) == 0 {
			t.Fatal("incomplete instance lacks evidence for evaluator mutation")
		}
		mutated.Instances[index].Roles = append(mutated.Instances[index].Roles,
			H1RoleEvidence{Role: H1RoleObservability, Evidence: evidence})
		mutated.Instances[index].Missing = []H1Role{H1RoleVerification, H1RoleFailurePolicy}
		found = true
		break
	}
	if !found {
		t.Fatal("evaluation fixture has no incomplete instance")
	}
	var err error
	mutated, err = sealH1(mutated)
	if err != nil {
		t.Fatal(err)
	}
	result, err := EvaluateClientRecipe(h0, mutated, oracle, mutated)
	if err != nil {
		t.Fatal(err)
	}
	if result.H1.RoleCoverage.Precision >= 1 || result.H1.RoleCoverage.Recall != 1 {
		t.Fatalf("invented-role score = %#v", result.H1.RoleCoverage)
	}
	if result.H1.Verdict != EvaluationFail || result.Verdict != EvaluationFail {
		t.Fatalf("invented role passed evaluator: overall %s / H1 %s", result.Verdict, result.H1.Verdict)
	}
}

func TestEvaluationDoesNotFallThroughToSecondBestExclusion(t *testing.T) {
	truth := []OracleExcluded{
		{ID: "a", Evidence: []SourceLocator{{Path: "a.go", Line: 1}, {Path: "shared.go", Line: 2}}},
		{ID: "b", Evidence: []SourceLocator{{Path: "shared.go", Line: 2}}},
	}
	predictions := []H1Excluded{
		{ID: "prediction-1", Evidence: []H1Evidence{{Path: "a.go", Line: 1}}},
		{ID: "prediction-2", Evidence: []H1Evidence{{Path: "a.go", Line: 1}, {Path: "shared.go", Line: 2}}},
	}
	if matches := matchH1Exclusions(predictions, truth); len(matches) != 0 {
		t.Fatalf("ambiguous best exclusion proposals fell through to a second-best truth: %#v", matches)
	}
}
