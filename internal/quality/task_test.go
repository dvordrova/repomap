package quality

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/sourceexplain"
)

func TestTaskValidate(t *testing.T) {
	t.Parallel()

	latency := int64(250)
	tests := []struct {
		name      string
		mutate    func(*Task)
		wantError string
	}{
		{
			name: "unsupported version",
			mutate: func(task *Task) {
				task.Version++
			},
			wantError: "unsupported task version",
		},
		{
			name: "abbreviated revision",
			mutate: func(task *Task) {
				task.Repository.Revision = "abc123"
			},
			wantError: "full lowercase git hash",
		},
		{
			name: "implicit build tags",
			mutate: func(task *Task) {
				task.Repository.Scenario.BuildTags = nil
			},
			wantError: "build_tags must be explicit",
		},
		{
			name: "invalid capture date",
			mutate: func(task *Task) {
				task.Captures.Source.CapturedAt = "yesterday"
			},
			wantError: "yyyy-mm-dd",
		},
		{
			name: "negative latency",
			mutate: func(task *Task) {
				negative := -1 * latency
				task.Captures.Source.LatencyMillis = &negative
			},
			wantError: "latency cannot be negative",
		},
		{
			name: "provider request sha without bytes",
			mutate: func(task *Task) {
				requestSHA := strings.Repeat("8", 64)
				task.Captures.Orientation.ProviderRequestSHA256 = &requestSHA
			},
			wantError: "must both be known or both be null",
		},
		{
			name: "provider request bytes without sha",
			mutate: func(task *Task) {
				task.Captures.Source.ProviderRequestSHA256 = nil
			},
			wantError: "must both be known or both be null",
		},
		{
			name: "invalid model context bytes",
			mutate: func(task *Task) {
				task.Captures.Source.ModelContextBytes = 0
			},
			wantError: "model context byte count",
		},
		{
			name: "invalid provider request bytes",
			mutate: func(task *Task) {
				zero := 0
				task.Captures.Source.ProviderRequestBytes = &zero
			},
			wantError: "provider request byte count",
		},
		{
			name: "artifact traversal",
			mutate: func(task *Task) {
				task.Artifacts.SourceBundle.Path = "../source.json"
			},
			wantError: "invalid manifest-relative path",
		},
		{
			name: "duplicate artifact path",
			mutate: func(task *Task) {
				task.Artifacts.SourceResponse.Path = task.Artifacts.SourceBundle.Path
			},
			wantError: "use the same path",
		},
		{
			name: "directions required",
			mutate: func(task *Task) {
				task.Expected.Directions = nil
			},
			wantError: "directions must contain",
		},
		{
			name: "unknown predicate",
			mutate: func(task *Task) {
				task.Expected.Drilldown.SourcePredicates[0] = "model_guess"
			},
			wantError: "invalid source predicate",
		},
		{
			name: "test reference must be test file",
			mutate: func(task *Task) {
				task.Expected.Drilldown.TestReferencePaths[0] = "server/key.go"
			},
			wantError: "non-test path",
		},
		{
			name: "overclaim scope is closed",
			mutate: func(task *Task) {
				task.Expected.ForbiddenOverclaims[0].Scope = "everything"
			},
			wantError: "invalid scope",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			task := validTask()
			test.mutate(&task)
			if err := task.Validate(); err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.wantError)
			}
		})
	}
}

func TestTaskValidateAcceptsCaptureDatePrecisions(t *testing.T) {
	t.Parallel()

	task := validTask()
	task.Captures.Orientation.CapturedAt = "2026-05-24T15:11:40Z"
	task.Captures.Source.CapturedAt = "2026-07-10"
	if err := task.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestOrientationGroundingContextValidateAllowsLargeSortedPathSet(t *testing.T) {
	t.Parallel()

	paths := make([]string, 150)
	for index := range paths {
		paths[index] = fmt.Sprintf("server/package-%03d/file.go", index)
	}
	context := OrientationGroundingContext{
		Version:      OrientationGroundingContextVersion,
		RepoName:     "etcd",
		AllowedPaths: paths,
	}
	if err := context.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	context.AllowedPaths[0], context.AllowedPaths[1] = context.AllowedPaths[1], context.AllowedPaths[0]
	if err := context.Validate(); err == nil || !strings.Contains(err.Error(), "must be sorted") {
		t.Fatalf("Validate() error = %v, want sorted-path error", err)
	}
}

func validTask() Task {
	sourceRequestSHA256 := strings.Repeat("8", 64)
	sourceRequestBytes := 6601
	return Task{
		Version: TaskVersion,
		ID:      "etcd-put-v1",
		Repository: Repository{
			Name:     "etcd",
			Revision: strings.Repeat("a", 40),
			Scenario: BuildScenario{
				Orientation:  "default local go list",
				GOOS:         "darwin",
				GOARCH:       "amd64",
				GoVersion:    "go1.26.4",
				GoplsVersion: "v0.23.0",
				BuildTags:    []string{},
			},
		},
		Goal: "find the client Put entry and inspect kvServer.Put",
		Captures: Captures{
			Orientation: StageCapture{
				Provider:           "deepseek",
				Model:              "unknown",
				PromptVersion:      "legacy-orientation-unversioned",
				CapturedAt:         "2026-05-24T15:11:40Z",
				ModelContextSHA256: strings.Repeat("1", 64),
				ModelContextBytes:  167957,
			},
			Source: StageCapture{
				Provider:              "deepseek",
				Model:                 "deepseek-v4-flash",
				PromptVersion:         "source-assessment-json-v2",
				CapturedAt:            "2026-07-10",
				ModelContextSHA256:    strings.Repeat("2", 64),
				ModelContextBytes:     3001,
				ProviderRequestSHA256: &sourceRequestSHA256,
				ProviderRequestBytes:  &sourceRequestBytes,
			},
		},
		Artifacts: Artifacts{
			OrientationContext:  ArtifactRef{Path: "artifacts/orientation-context.json", SHA256: strings.Repeat("3", 64)},
			OrientationResponse: ArtifactRef{Path: "artifacts/orientation-response.json", SHA256: strings.Repeat("4", 64)},
			SourceBundle:        ArtifactRef{Path: "artifacts/source-bundle.json", SHA256: strings.Repeat("5", 64)},
			SourceResponse:      ArtifactRef{Path: "artifacts/source-response.json", SHA256: strings.Repeat("6", 64)},
			TestEvidence:        ArtifactRef{Path: "artifacts/test-evidence.json", SHA256: strings.Repeat("7", 64)},
		},
		Expected: Expectations{
			Directions: []DirectionExpectation{{
				ID:             "put-write-path",
				Aliases:        []string{"grpc put request", "write path"},
				ImportantPaths: []string{"server/etcdserver/api/v3rpc/key.go"},
			}},
			Drilldown: DrilldownExpectation{
				Symbol:             "kvServer.Put",
				Path:               "server/etcdserver/api/v3rpc/key.go",
				SourcePredicates:   []sourceexplain.Predicate{sourceexplain.PredicateValidatesInput},
				TestReferencePaths: []string{"server/etcdserver/api/v3rpc/util_test.go"},
			},
			ForbiddenOverclaims: []ForbiddenOverclaim{{
				ID:          "runtime-observed",
				Scope:       OverclaimScopeDrilldown,
				ContainsAll: []string{"observed", "runtime"},
			}},
		},
	}
}
