package report

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowproof"
	"github.com/dvordrova/repomap/internal/guidedtour"
)

func TestBuildGuidedTourBundleClassifiesNoCandidateAsExpectedSkip(t *testing.T) {
	t.Parallel()

	_, err := BuildGuidedTourBundle(&ReportData{
		RepoName:           "fixture",
		ArchitectureCanvas: &ArchitectureCanvas{Version: 1},
	})
	if !errors.Is(err, ErrNoGuidedTourCandidates) {
		t.Fatalf("BuildGuidedTourBundle() error = %v, want expected skip", err)
	}
}

func TestBuildGuidedTourBundleUsesExactTraceAndLegacyDirectionEvidence(t *testing.T) {
	t.Parallel()

	data := guidedTourReportFixture()
	bundle, err := BuildGuidedTourBundle(data)
	if err != nil {
		t.Fatalf("BuildGuidedTourBundle() error = %v", err)
	}
	if len(bundle.Candidates) != 2 {
		t.Fatalf("BuildGuidedTourBundle() candidates = %d, want 2", len(bundle.Candidates))
	}
	if err := bundle.Validate(); err != nil {
		t.Fatalf("BuildGuidedTourBundle() returned invalid bundle: %v", err)
	}

	trace := guidedTourCandidate(t, bundle, "saved-trace")
	if trace.Kind != guidedtour.CandidateSavedTrace || trace.OrderingBasis != guidedtour.OrderingTrace {
		t.Fatalf("saved trace candidate = %#v", trace)
	}
	if len(trace.Beats) != 3 {
		t.Fatalf("saved trace beats = %d, want 3", len(trace.Beats))
	}
	expectedStepIDs := []string{"trace-step-1", "trace-step-2", "trace-step-3"}
	for index, beat := range trace.Beats {
		if beat.Sequence != index || beat.FlowID != "saved-trace" ||
			len(beat.FlowStepIDs) != 1 || beat.FlowStepIDs[0] != expectedStepIDs[index] {
			t.Fatalf("saved trace beat %d = %#v", index, beat)
		}
		if len(beat.ComponentIDs) != 1 || beat.ComponentIDs[0] != "trace-component" ||
			len(beat.Evidence) == 0 || beat.Evidence[0].Location == nil {
			t.Fatalf("saved trace beat %d lost exact component/evidence: %#v", index, beat)
		}
	}

	direction := guidedTourCandidate(t, bundle, "legacy-direction")
	if direction.Kind != guidedtour.CandidateSuggestedDirection ||
		direction.OrderingBasis != guidedtour.OrderingEditorial {
		t.Fatalf("legacy direction candidate = %#v", direction)
	}
	if len(direction.Beats) != 3 {
		t.Fatalf("legacy direction beats = %d, want 3", len(direction.Beats))
	}
	wantPaths := []string{
		"cmd/orientation.go",
		"internal/report/parse.go",
		"internal/report/guided_tour.go",
	}
	for index, beat := range direction.Beats {
		if beat.Sequence != index || len(beat.ComponentIDs) != 1 ||
			beat.ComponentIDs[0] != "direction-component" || len(beat.Evidence) != 1 ||
			beat.Evidence[0].Location == nil || beat.Evidence[0].Location.Path != wantPaths[index] {
			t.Fatalf("direction beat %d = %#v, want exact path %q", index, beat, wantPaths[index])
		}
	}
	if len(direction.Gaps) != 1 || !strings.Contains(direction.Gaps[0].Detail, "runtime execution") ||
		!strings.Contains(direction.Gaps[0].Detail, "not proven") {
		t.Fatalf("direction gaps = %#v, want explicit unproven-runtime warning", direction.Gaps)
	}
}

func TestBuildGuidedTourBundleRejectsAmbiguousDirectionPathOwnership(t *testing.T) {
	t.Parallel()

	data := guidedTourReportFixture()
	data.ArchitectureCanvas.Components = append(data.ArchitectureCanvas.Components, ArchitectureComponent{
		ID: "ambiguous-component", Name: "Ambiguous", Description: "Second exact file owner",
		Members: []componentmap.Candidate{{
			ID: componentmap.MemberID{Kind: componentmap.MemberFile, Value: "ambiguous-file"},
			Facts: []componentmap.LocalFact{{
				Kind:      componentmap.FactRepositoryPath,
				Value:     "internal/report/guided_tour.go",
				Certainty: evidence.CertaintyStatic,
			}},
		}},
	})

	bundle, err := BuildGuidedTourBundle(data)
	if err != nil {
		t.Fatalf("BuildGuidedTourBundle() error = %v", err)
	}
	if len(bundle.Candidates) != 1 || bundle.Candidates[0].ID != "saved-trace" {
		t.Fatalf("ambiguous path produced an editorial candidate: %#v", bundle.Candidates)
	}
}

func TestBuildGuidedTourBundleMapsLegacyModulePathsWithoutSavedPackageFiles(t *testing.T) {
	t.Parallel()

	data := guidedTourReportFixture()
	data.RepositoryGraph.Packages = nil
	data.RepositoryGraph.Modules = []ModuleInfo{{Path: "example.com/fixture", Dir: ""}}
	data.CandidateDirections[0].LikelyFiles = []string{
		"internal/report/parse.go",
		"internal/report/guided_tour.go",
		"internal/report/render.go",
	}
	data.OpenablePaths = append(data.OpenablePaths,
		"internal/report/parse.go",
		"internal/report/guided_tour.go",
		"internal/report/render.go",
	)

	bundle, err := BuildGuidedTourBundle(data)
	if err != nil {
		t.Fatal(err)
	}
	direction := guidedTourCandidate(t, bundle, "legacy-direction")
	if len(direction.Beats) != 3 {
		t.Fatalf("legacy module-derived beats = %#v", direction.Beats)
	}
}

func TestBuildGuidedTourBundleAddsOnlyCandidateOwnedStaticPackageImports(t *testing.T) {
	t.Parallel()

	const (
		reportPackage    = "example.com/fixture/internal/report"
		workerPackage    = "example.com/fixture/internal/worker"
		unrelatedPackage = "example.com/fixture/internal/unrelated"
		ambiguousPackage = "example.com/fixture/internal/ambiguous"
		externalPackage  = "example.net/external"
	)
	packageComponent := func(id, packagePath string) ArchitectureComponent {
		return ArchitectureComponent{
			ID: componentmap.ComponentID(id), Name: id, Description: "Exact package owner",
			Members: []componentmap.Candidate{{
				ID: componentmap.MemberID{Kind: componentmap.MemberPackage, Value: packagePath},
				Facts: []componentmap.LocalFact{{
					Kind:      componentmap.FactDeclaration,
					Value:     packagePath,
					Certainty: evidence.CertaintyStatic,
				}},
			}},
		}
	}

	data := guidedTourReportFixture()
	data.OpenablePaths = append(data.OpenablePaths, "internal/worker/run.go")
	data.CandidateDirections[0].LikelyFiles = append(
		data.CandidateDirections[0].LikelyFiles,
		"internal/worker/run.go",
	)
	data.RepositoryGraph.Packages = append(data.RepositoryGraph.Packages,
		PackageInfo{CanonicalPath: workerPackage, Files: []string{"internal/worker/run.go"}},
	)
	data.RepositoryGraph.PackageEdges = []EdgeInfo{
		{From: reportPackage, To: externalPackage},
		{From: reportPackage, To: ambiguousPackage},
		{From: reportPackage, To: workerPackage},
		{From: reportPackage, To: unrelatedPackage},
		{From: reportPackage, To: workerPackage},
	}
	data.ArchitectureCanvas.Components = append(
		data.ArchitectureCanvas.Components,
		packageComponent("worker-component", workerPackage),
		packageComponent("unrelated-component", unrelatedPackage),
		packageComponent("ambiguous-component-a", ambiguousPackage),
		packageComponent("ambiguous-component-b", ambiguousPackage),
	)

	bundle, err := BuildGuidedTourBundle(data)
	if err != nil {
		t.Fatalf("BuildGuidedTourBundle() error = %v", err)
	}
	if err := bundle.Validate(); err != nil {
		t.Fatalf("BuildGuidedTourBundle() returned invalid bundle: %v", err)
	}
	direction := guidedTourCandidate(t, bundle, "legacy-direction")
	packageImportBeats := []guidedtour.Beat{}
	for _, beat := range direction.Beats {
		if beat.Kind == "package_import" {
			packageImportBeats = append(packageImportBeats, beat)
		}
	}
	if len(packageImportBeats) != 1 {
		t.Fatalf("package import beats = %#v, want one exact internal edge", packageImportBeats)
	}
	beat := packageImportBeats[0]
	if beat.Sequence != len(direction.Beats)-1 ||
		len(beat.ComponentIDs) != 2 ||
		beat.ComponentIDs[0] != "direction-component" ||
		beat.ComponentIDs[1] != "worker-component" {
		t.Fatalf("package import beat endpoints/order = %#v", beat)
	}
	if !strings.Contains(strings.ToLower(beat.Label), "static package import") ||
		!strings.Contains(strings.ToLower(beat.Label), "not a runtime call") ||
		!strings.Contains(beat.Detail, reportPackage) ||
		!strings.Contains(beat.Detail, workerPackage) ||
		!strings.Contains(strings.ToLower(beat.Detail), "not a runtime call") {
		t.Fatalf("package import beat does not state its static limitation: %#v", beat)
	}
	if len(beat.Evidence) != 1 || beat.Evidence[0].Location != nil ||
		beat.Evidence[0].ID != stableReportID(
			"guided-evidence",
			"package-import",
			"legacy-direction",
			reportPackage,
			workerPackage,
		) ||
		!strings.Contains(strings.ToLower(beat.Evidence[0].Label), "not a runtime call") {
		t.Fatalf("package import evidence = %#v", beat.Evidence)
	}
}

func TestReadRunDirDoesNotReplayGuidedStoryFromCandidateDirectionProof(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	writeArchitectureBuildFixture(t, runDir, "snapshot.json", []byte(`{"repo_name":"fixture"}`))
	writeArchitectureBuildFixture(t, runDir, "llm_bundle.json", []byte(`{
		"allowed_paths":["cmd/backup.go","internal/repo/repo.go"],
		"go": {
			"module_summaries": [{"module_path":"example.com/project","module_dir":"."}],
			"important_edges": [{"from":"example.com/project/cmd","to":"example.com/project/internal/repo"}]
		}
	}`))
	orientation, err := json.Marshal(map[string]any{
		"project_guess": "saved fixture",
		"candidate_flows": []map[string]any{{
			"name": "Backup", "trigger": "backup command",
			"local_proof": map[string]any{
				"version": flowproof.SessionVersion,
				"proof":   architectureBuildTestProof(),
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeArchitectureBuildFixture(t, runDir, "orientation_report.json", orientation)

	data, err := ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if containsWarning(data.Warnings, "guided tour") {
		t.Fatalf("absent %s produced a warning: %#v", GuidedStoryFile, data.Warnings)
	}
	if data.ArchitectureCanvas == nil || len(data.ArchitectureCanvas.Flows) != 0 {
		t.Fatalf("CandidateDirection proof became an architecture flow: %#v", data.ArchitectureCanvas)
	}
	if _, err := BuildGuidedTourBundle(data); !errors.Is(err, ErrNoGuidedTourCandidates) {
		t.Fatalf("BuildGuidedTourBundle() error = %v, want no exact candidate", err)
	}

	writeArchitectureBuildFixture(t, runDir, GuidedStoryFile, []byte(`{"broken"`))
	corrupt, err := ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if corrupt.GuidedTour != nil || !containsWarning(corrupt.Warnings, "cannot rebuild saved story bundle") {
		t.Fatalf("corrupt guided story result = %#v, warnings = %#v", corrupt.GuidedTour, corrupt.Warnings)
	}
}

func guidedTourReportFixture() *ReportData {
	traceComponent := ArchitectureComponent{
		ID: "trace-component", Name: "Trace", Description: "Saved trace component",
	}
	directionComponent := ArchitectureComponent{
		ID: "direction-component", Name: "Direction", Description: "Accepted direction component",
		Members: []componentmap.Candidate{{
			ID: componentmap.MemberID{Kind: componentmap.MemberPackage, Value: "direction-package"},
			Facts: []componentmap.LocalFact{{
				Kind:      componentmap.FactDeclaration,
				Value:     "example.com/fixture/internal/report",
				Certainty: evidence.CertaintyStatic,
			}},
		}},
	}
	return &ReportData{
		RepoName: "fixture",
		OpenablePaths: []string{
			"trace/one.go", "trace/two.go", "trace/three.go",
		},
		RepositoryGraph: &RepositoryGraph{Packages: []PackageInfo{{
			CanonicalPath: "example.com/fixture/internal/report",
			Files: []string{
				"cmd/orientation.go",
				"internal/report/parse.go",
				"internal/report/guided_tour.go",
			},
		}}},
		CandidateDirections: []CandidateDirection{{
			ID: "legacy-direction", Name: "CLI orientation", Trigger: "orientation command",
			LikelyFiles:    []string{"cmd/orientation.go", "internal/report/parse.go"},
			WhyInteresting: "Explains the local orientation path",
			// Empty disposition is intentional: older saved runs did not persist it.
		}},
		Flows: []FlowData{{
			ID: "legacy-direction",
			FilesToRead: []FileItem{{
				Path: "internal/report/guided_tour.go", Reason: "report integration",
			}},
		}},
		ArchitectureCanvas: &ArchitectureCanvas{
			Version: ArchitectureCanvasVersion,
			Components: []ArchitectureComponent{
				traceComponent,
				directionComponent,
			},
			Flows: []ArchitectureFlow{{
				ID: "saved-trace", Name: "Saved trace", Trigger: "saved trigger", Goal: "saved goal",
				Steps: []ArchitectureFlowStep{
					{ID: "trace-step-1", Kind: "function", Label: "first", Location: &evidence.Location{Path: "trace/one.go", Line: 10}, ComponentID: "trace-component"},
					{ID: "trace-step-2", Kind: "callsite", Label: "second", Location: &evidence.Location{Path: "trace/two.go", Line: 20}, ComponentID: "trace-component"},
					{ID: "trace-step-3", Kind: "operation", Label: "third", Location: &evidence.Location{Path: "trace/three.go", Line: 30}, ComponentID: "trace-component"},
				},
			}},
			Suggestions: []ArchitectureSuggestion{{
				ID: "legacy-direction", Title: "CLI orientation",
				TraceUnavailableReason: "No saved FlowProof connects these files.",
			}},
		},
	}
}

func guidedTourCandidate(t *testing.T, bundle guidedtour.Bundle, id string) guidedtour.Candidate {
	t.Helper()
	for _, candidate := range bundle.Candidates {
		if candidate.ID == id {
			return candidate
		}
	}
	t.Fatalf("guided tour candidate %q is missing", id)
	return guidedtour.Candidate{}
}
