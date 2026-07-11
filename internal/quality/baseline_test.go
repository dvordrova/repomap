package quality

import "testing"

func TestEtcdPutBaselineReplay(t *testing.T) {
	t.Parallel()

	loaded, err := Load("testdata/etcd-put-v1/task.json")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	result, err := Evaluate(loaded)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !result.Passed {
		t.Fatalf("baseline did not pass: %#v", result)
	}
	orientationContract := result.ContractAdherence.OrientationResponse
	if !orientationContract.Valid || orientationContract.Measured || orientationContract.Clean ||
		len(orientationContract.WarningCodes) != 1 ||
		orientationContract.WarningCodes[0] != orientationContractUnmeasuredWarning {
		t.Fatalf("orientation contract = %#v", orientationContract)
	}

	const expectedDirections = 5
	if len(result.DirectionCoverage.Checks) != expectedDirections {
		t.Fatalf("direction checks = %d, want %d", len(result.DirectionCoverage.Checks), expectedDirections)
	}
	for _, check := range result.DirectionCoverage.Checks {
		if !check.Covered || len(check.CandidateNames) != 1 || check.SelectedCandidate == "" {
			t.Errorf("direction %q association = %#v", check.DirectionID, check)
		}
	}

	const expectedStructuredPaths = 21
	if !result.Grounding.Valid || len(result.Grounding.InvalidReferences) != 0 {
		t.Fatalf("grounding = %#v", result.Grounding)
	}
	if result.Grounding.ReferencedPathCount != expectedStructuredPaths {
		t.Fatalf(
			"unique structured paths = %d, want %d",
			result.Grounding.ReferencedPathCount,
			expectedStructuredPaths,
		)
	}

	drilldown := result.SemanticDrilldown
	if len(drilldown.Predicates) != len(loaded.Task.Expected.Drilldown.SourcePredicates) {
		t.Fatalf(
			"predicate checks = %d, want %d",
			len(drilldown.Predicates),
			len(loaded.Task.Expected.Drilldown.SourcePredicates),
		)
	}
	for _, check := range drilldown.Predicates {
		if !check.Found {
			t.Errorf("expected source predicate %q was not found", check.Predicate)
		}
	}
	if len(drilldown.Tests) != len(loaded.Task.Expected.Drilldown.TestReferencePaths) {
		t.Fatalf(
			"test checks = %d, want %d",
			len(drilldown.Tests),
			len(loaded.Task.Expected.Drilldown.TestReferencePaths),
		)
	}
	for _, check := range drilldown.Tests {
		if !check.Found || !check.ContextCompatible {
			t.Errorf("expected test reference %q was not found", check.Path)
		}
	}
	if len(drilldown.MissingPredicates) != 0 || len(drilldown.MissingTests) != 0 ||
		len(drilldown.IncompatibleTests) != 0 {
		t.Fatalf(
			"drilldown omissions: predicates=%v tests=%v incompatible=%v",
			drilldown.MissingPredicates,
			drilldown.MissingTests,
			drilldown.IncompatibleTests,
		)
	}

	sourceContract := result.ContractAdherence.SourceResponse
	if !sourceContract.Valid || !sourceContract.Clean || sourceContract.Evaluation == nil {
		t.Fatalf("source contract = %#v", sourceContract)
	}
	if sourceContract.Evaluation.Score != 100 || sourceContract.Evaluation.MaxScore != 100 {
		t.Fatalf(
			"source evaluation = %d/%d, want 100/100",
			sourceContract.Evaluation.Score,
			sourceContract.Evaluation.MaxScore,
		)
	}
	if len(sourceContract.WarningCodes) != 0 {
		t.Fatalf("source warning codes = %v, want none", sourceContract.WarningCodes)
	}

	if result.BytesAndLatency.Orientation.LatencyMillis != nil || result.BytesAndLatency.Source.LatencyMillis != nil {
		t.Fatalf(
			"captured latencies = orientation:%v source:%v, want nil",
			result.BytesAndLatency.Orientation.LatencyMillis,
			result.BytesAndLatency.Source.LatencyMillis,
		)
	}
}
