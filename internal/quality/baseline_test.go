package quality

import (
	"path/filepath"
	"slices"
	"testing"
)

type committedBaseline struct {
	name     string
	taskPath string
	taskID   string
}

var committedBaselines = []committedBaseline{
	{
		name:     "etcd put orientation and drilldown",
		taskPath: "testdata/etcd-put-v1/task.json",
		taskID:   "etcd-put-orientation-drilldown-v1",
	},
	{
		name:     "k6 metrics orientation and drilldown",
		taskPath: "testdata/k6-metrics-v1/task.json",
		taskID:   "k6-metrics-orientation-drilldown-v1",
	},
}

func TestCommittedBaselineSuiteMembership(t *testing.T) {
	t.Parallel()

	actual, err := filepath.Glob("testdata/*/task.json")
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	expected := make([]string, 0, len(committedBaselines))
	for _, baseline := range committedBaselines {
		expected = append(expected, baseline.taskPath)
	}
	slices.Sort(actual)
	slices.Sort(expected)
	if !slices.Equal(actual, expected) {
		t.Fatalf("committed task manifests = %v, want %v", actual, expected)
	}
}

func TestCommittedBaselinesReplay(t *testing.T) {
	t.Parallel()

	for _, baseline := range committedBaselines {
		t.Run(baseline.name, func(t *testing.T) {
			t.Parallel()

			loaded, err := Load(baseline.taskPath)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			result, err := Evaluate(loaded)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}
			if !result.Passed || result.TaskID != baseline.taskID {
				t.Fatalf("baseline result = %#v", result)
			}
		})
	}
}

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
	if !drilldown.OrientationLink.Linked {
		t.Fatalf("orientation to drilldown link = %#v", drilldown.OrientationLink)
	}
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

func TestK6MetricsBaselineReplay(t *testing.T) {
	t.Parallel()

	loaded, err := Load("testdata/k6-metrics-v1/task.json")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	result, err := Evaluate(loaded)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !result.Passed || result.Version != EvaluationVersion {
		t.Fatalf("baseline result = %#v", result)
	}
	if len(result.DirectionCoverage.Checks) != 3 ||
		len(result.DirectionCoverage.Missing) != 0 ||
		len(result.DirectionCoverage.Ambiguous) != 0 {
		t.Fatalf("direction coverage = %#v", result.DirectionCoverage)
	}
	for _, check := range result.DirectionCoverage.Checks {
		if !check.Covered || len(check.CandidateNames) != 1 || check.SelectedCandidate == "" {
			t.Errorf("direction %q association = %#v", check.DirectionID, check)
		}
	}
	grounding := result.Grounding
	if !grounding.Valid || grounding.AllowedPathCount != 60 ||
		grounding.ReferencedPathCount != 11 || grounding.UnscoredProseEvidenceCount != 17 ||
		len(grounding.InvalidReferences) != 0 {
		t.Fatalf("grounding = %#v", grounding)
	}
	if !result.ImportantEvidence.Complete || len(result.ImportantEvidence.Checks) != 7 {
		t.Fatalf("important evidence = %#v", result.ImportantEvidence)
	}
	drilldown := result.SemanticDrilldown
	if !drilldown.Complete || !drilldown.OrientationLink.Linked || len(drilldown.Predicates) != 1 ||
		!drilldown.Predicates[0].Found || len(drilldown.Tests) != 1 ||
		!drilldown.Tests[0].ContextCompatible {
		t.Fatalf("drilldown = %#v", drilldown)
	}
	orientationContract := result.ContractAdherence.OrientationResponse
	if !orientationContract.Valid || !orientationContract.Measured || !orientationContract.Clean {
		t.Fatalf("orientation contract = %#v", orientationContract)
	}
	sourceContract := result.ContractAdherence.SourceResponse
	if !sourceContract.Valid || !sourceContract.Clean || sourceContract.Evaluation == nil ||
		sourceContract.Evaluation.Score != 100 || sourceContract.Evaluation.MaxScore != 100 {
		t.Fatalf("source contract = %#v", sourceContract)
	}
	observations := result.BytesAndLatency
	if observations.Orientation.ReplayInputBytes != 2435 ||
		observations.Orientation.ResponseBytes != 6311 ||
		observations.Orientation.ModelContextBytes != 32659 ||
		observations.Orientation.ProviderRequestBytes == nil ||
		*observations.Orientation.ProviderRequestBytes != 38838 ||
		observations.Source.ReplayInputBytes != 2553 ||
		observations.Source.ResponseBytes != 416 ||
		observations.Source.ModelContextBytes != 1804 ||
		observations.Source.ProviderRequestBytes == nil ||
		*observations.Source.ProviderRequestBytes != 5167 ||
		observations.TestEvidenceBytes != 1880 ||
		observations.Orientation.LatencyMillis != nil || observations.Source.LatencyMillis != nil {
		t.Fatalf("bytes and latency = %#v", observations)
	}
	const (
		orientationRequestSHA = "9b24367e89e20e644d15e2c6e559bb450459590f3ddf8c17d838e86a5313c4e2"
		sourceRequestSHA      = "e07df08d66f82c02f41b53bce6ddb83e9af21b573c2417cc40206b41fe59b122"
	)
	if loaded.Task.Captures.Orientation.ProviderRequestSHA256 == nil ||
		*loaded.Task.Captures.Orientation.ProviderRequestSHA256 != orientationRequestSHA ||
		loaded.Task.Captures.Source.ProviderRequestSHA256 == nil ||
		*loaded.Task.Captures.Source.ProviderRequestSHA256 != sourceRequestSHA {
		t.Fatalf("request capture metadata = %#v", loaded.Task.Captures)
	}
}
