package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowproof"
)

func TestReplayArchitectureSynthesisChangesOnlyValidatedConceptualMembership(t *testing.T) {
	t.Parallel()

	input, err := BuildArchitectureCanvasInput(&ReportData{CandidateDirections: []CandidateDirection{{
		ID: "backup", Name: "Backup",
		LocalProof: &flowproof.Session{Version: flowproof.SessionVersion, Proof: flowproof.Proof{
			Version: flowproof.Version, ID: "backup", Archetype: flowproof.ArchetypeCLI,
			Anchors: []flowproof.Anchor{{ID: "unknown", Kind: flowproof.AnchorOperation, Label: "unknown"}},
		}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	memberID := input.CandidateBundle.Candidates[0].ID
	response, err := json.Marshal(componentmap.Proposal{
		Version: componentmap.ContractVersion,
		Subsystems: []componentmap.ProposedSubsystem{{
			Name: "Data protection",
			Components: []componentmap.ProposedComponent{{
				Name: "Backup execution", MemberIDs: []componentmap.MemberID{memberID},
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := componentmap.RecordSynthesisResponse(
		input.CandidateBundle,
		"revision-test",
		"openai-compatible/bearer",
		"test-model",
		12*time.Millisecond,
		response,
	)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := json.Marshal(result.Record)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := ReplayArchitectureSynthesis(input, saved)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Landscape.Fallback || len(replayed.Landscape.Subsystems) != 1 ||
		replayed.Landscape.Subsystems[0].Components[0].Name != "Backup execution" {
		t.Fatalf("replayed landscape = %#v", replayed.Landscape)
	}
	if len(replayed.CandidateBundle.AnchorBindings) != len(input.CandidateBundle.AnchorBindings) ||
		len(replayed.Flows) != len(input.Flows) {
		t.Fatal("conceptual replay changed exact proof inputs")
	}
}

func TestReadRunDirRequiresSavedSynthesisBeforeProjectingArchitectureCanvas(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	writeArchitectureBuildFixture(t, runDir, "snapshot.json", []byte(`{"repo_name":"fixture"}`))
	writeArchitectureBuildFixture(t, runDir, "llm_bundle.json", []byte(`{
		"go": {
			"module_summaries": [{"module_path":"example.com/project","module_dir":"."}],
			"important_edges": [{"from":"example.com/project/cmd","to":"example.com/project/internal/repo"}]
		}
	}`))
	orientation, err := json.Marshal(map[string]any{
		"project_guess": "saved fixture",
		"candidate_flows": []map[string]any{{
			"name": "Backup", "trigger": "backup command",
			"local_proof": flowproof.Session{
				Version: flowproof.SessionVersion,
				Proof:   architectureBuildTestProof(),
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
	if data.RepositoryGraph == nil || len(data.RepositoryGraph.PackageEdges) != 1 {
		t.Fatalf("repository graph = %#v, want the saved package witness", data.RepositoryGraph)
	}
	if data.ArchitectureCanvas != nil {
		t.Fatalf("architecture canvas = %#v, want no package fallback without synthesis", data.ArchitectureCanvas)
	}
	writeArchitectureBuildSynthesis(t, runDir, data, "revision-proof")
	data, err = ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if data.ArchitectureCanvas == nil || data.ArchitectureCanvas.Fallback ||
		len(data.ArchitectureCanvas.Flows) != 1 || data.ArchitectureCanvas.Flows[0].ID != "backup" {
		t.Fatalf("architecture canvas = %#v", data.ArchitectureCanvas)
	}

	writeArchitectureBuildFixture(t, runDir, ArchitectureSynthesisFile, []byte(`{"broken"`))
	data, err = ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if data.ArchitectureCanvas != nil {
		t.Fatalf("invalid saved synthesis produced a fallback canvas: %#v", data.ArchitectureCanvas)
	}
	foundUnavailableWarning := false
	for _, warning := range data.Warnings {
		if strings.Contains(warning, "architecture map unavailable") {
			foundUnavailableWarning = true
		}
	}
	if !foundUnavailableWarning {
		t.Fatalf("warnings = %#v, want invalid synthesis warning", data.Warnings)
	}
}

func TestReadRunDirReportsFailedArchitectureSynthesisWithoutProductFallback(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	writeArchitectureBuildFixture(t, runDir, "snapshot.json", []byte(`{"repo_name":"fixture"}`))
	writeArchitectureBuildFixture(t, runDir, "metadata.json", []byte(`{
		"model":"test-model",
		"provider_request_count":1
	}`))
	writeArchitectureBuildFixture(t, runDir, "orientation_report.json", []byte(`{
		"project_guess":"fixture",
		"high_level_map":[],
		"candidate_flows":[]
	}`))
	writeArchitectureBuildFixture(t, runDir, ArchitectureSynthesisStatusFile, []byte(`{
		"version":1,
		"state":"failed",
		"prompt_bytes":1200,
		"latency_ms":340,
		"provider_request_count":1,
		"error_code":"empty_response"
	}`))

	data, err := ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if data.ArchitectureCanvas != nil {
		t.Fatalf("architecture canvas = %#v, want no substitute graph", data.ArchitectureCanvas)
	}
	if data.ArchitectureSynthesis == nil || data.ArchitectureSynthesis.State != ArchitectureSynthesisFailed {
		t.Fatalf("architecture status = %#v, want failed", data.ArchitectureSynthesis)
	}
	if data.Run == nil || data.Run.ProviderRequestCount != 2 {
		t.Fatalf("run = %#v, want both orientation and architecture provider attempts", data.Run)
	}
	if !containsWarning(data.Warnings, "grouping request returned no content") {
		t.Fatalf("warnings = %#v, want concise architecture failure", data.Warnings)
	}
}

func containsWarning(warnings []string, substring string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, substring) {
			return true
		}
	}
	return false
}

func TestReadRunDirProjectsArchitectureLandscapeWithoutFlowProof(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	writeArchitectureBuildFixture(t, runDir, "snapshot.json", []byte(`{"repo_name":"fixture"}`))
	writeArchitectureBuildFixture(t, runDir, "llm_bundle.json", []byte(`{
		"go": {
			"module_summaries": [{"module_path":"example.com/project","module_dir":"."}],
			"important_edges": [{"from":"example.com/project/cmd","to":"example.com/project/internal/repo"}]
		}
	}`))
	writeArchitectureBuildFixture(t, runDir, "orientation_report.json", []byte(`{
		"project_guess":"saved fixture",
		"candidate_flows":[{
			"name":"Server startup",
			"trigger":"process starts",
			"likely_entrypoint":"cmd/main.go",
			"likely_files":["cmd/main.go"]
		}]
	}`))

	data, err := ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if data.ArchitectureCanvas != nil {
		t.Fatalf("architecture canvas = %#v, want no fallback without synthesis", data.ArchitectureCanvas)
	}
	writeArchitectureBuildSynthesis(t, runDir, data, "revision-landscape")
	data, err = ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if data.ArchitectureCanvas == nil || data.ArchitectureCanvas.Fallback ||
		len(data.ArchitectureCanvas.Components) == 0 || len(data.ArchitectureCanvas.Flows) != 0 {
		t.Fatalf("architecture canvas = %#v, want landscape with no flow overlay", data.ArchitectureCanvas)
	}
}

func writeArchitectureBuildSynthesis(t *testing.T, runDir string, data *ReportData, revision string) {
	t.Helper()
	input, err := BuildArchitectureCanvasInput(data)
	if err != nil {
		t.Fatal(err)
	}
	components := make([]componentmap.ProposedComponent, 0, len(input.CandidateBundle.Candidates))
	for index, candidate := range input.CandidateBundle.Candidates {
		components = append(components, componentmap.ProposedComponent{
			Name:      fmt.Sprintf("Component %d", index+1),
			MemberIDs: []componentmap.MemberID{candidate.ID},
		})
	}
	response, err := json.Marshal(componentmap.Proposal{
		Version: componentmap.ContractVersion,
		Subsystems: []componentmap.ProposedSubsystem{{
			Name: "Runtime", Components: components,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := componentmap.RecordSynthesisResponse(
		input.CandidateBundle,
		revision,
		"test",
		"test-model",
		time.Millisecond,
		response,
	)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := json.Marshal(result.Record)
	if err != nil {
		t.Fatal(err)
	}
	writeArchitectureBuildFixture(t, runDir, ArchitectureSynthesisFile, saved)
}

func writeArchitectureBuildFixture(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestBuildArchitectureCanvasInputUsesExactFlowBindingsAndPackageWitnesses(t *testing.T) {
	t.Parallel()

	proof := architectureBuildTestProof()
	data := &ReportData{
		CandidateDirections: []CandidateDirection{{
			ID: "backup", Name: "Backup", Trigger: "backup command",
			LocalProof: &flowproof.Session{Version: flowproof.SessionVersion, Proof: proof},
		}},
		RepositoryGraph: &RepositoryGraph{
			Modules: []ModuleInfo{{Path: "example.com/project"}},
			PackageEdges: []EdgeInfo{
				{From: "example.com/project/cmd", To: "example.com/project/internal/repo"},
				{From: "example.com/project/cmd", To: "example.com/project/internal/repo"},
			},
		},
	}

	input, err := BuildArchitectureCanvasInput(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := input.CandidateBundle.Validate(); err != nil {
		t.Fatalf("candidate bundle is invalid: %v", err)
	}
	if !input.Landscape.Fallback || input.Landscape.FallbackReason != componentmap.FallbackModelDisabled {
		t.Fatalf(
			"fallback = %v (%q), want explicit model-disabled deterministic landscape",
			input.Landscape.Fallback,
			input.Landscape.FallbackReason,
		)
	}
	if len(input.CandidateBundle.Relations) != 1 {
		t.Fatalf("relations = %#v, want one deduplicated package witness", input.CandidateBundle.Relations)
	}
	relation := input.CandidateBundle.Relations[0]
	if relation.From.Kind != componentmap.MemberPackage || relation.To.Kind != componentmap.MemberPackage {
		t.Fatalf("relation endpoints = %#v -> %#v, want typed package members", relation.From, relation.To)
	}
	if relation.Location != nil || relation.Provenance[0].Location != nil {
		t.Fatalf("relation manufactured an unavailable import callsite: %#v", relation)
	}
	if len(relation.Scenarios) != 1 ||
		relation.Scenarios[0].Build.GOOS != "" ||
		relation.Scenarios[0].Build.GOARCH != "" ||
		len(relation.Scenarios[0].Build.BuildTags) != 0 {
		t.Fatalf("relation scenario = %#v, want explicit unknown build values", relation.Scenarios)
	}

	bindings := architectureBuildTestBindingIndex(input.CandidateBundle.AnchorBindings)
	if got := bindings["handler"].MemberID.Kind; got != componentmap.MemberEntrypoint {
		t.Fatalf("handler member kind = %q, want exact verified entrypoint", got)
	}
	if got := bindings["save"].MemberID.Kind; got != componentmap.MemberSymbol {
		t.Fatalf("save member kind = %q, want exact declaration symbol", got)
	}
	if got := bindings["call-save"].MemberID.Kind; got != componentmap.MemberFile {
		t.Fatalf("callsite member kind = %q, want containing file without declaration inference", got)
	}
}

func TestBuildArchitectureCanvasInputAllowsRepositoryLandscapeWithoutFlowProof(t *testing.T) {
	t.Parallel()

	input, err := BuildArchitectureCanvasInput(&ReportData{RepositoryGraph: &RepositoryGraph{
		Modules: []ModuleInfo{{Path: "example.com/project"}},
		PackageEdges: []EdgeInfo{{
			From: "example.com/project/cmd", To: "example.com/project/internal/repo",
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Flows) != 0 || len(input.CandidateBundle.Flows) != 0 {
		t.Fatalf("flows = %#v / %#v, want no invented flow", input.Flows, input.CandidateBundle.Flows)
	}
	canvas, err := ProjectArchitectureCanvas(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(canvas.Components) == 0 || len(canvas.StructuralFacts) != 1 || len(canvas.Flows) != 0 {
		t.Fatalf("canvas = %#v, want structural landscape without flow overlay", canvas)
	}
}

func TestBuildArchitectureCanvasInputPrioritizesGroundedBehavior(t *testing.T) {
	t.Parallel()

	scenario := architectureGroundingScenario{ID: "go:test", GOOS: "darwin", GOARCH: "arm64"}
	producer := evidence.Provenance{Provider: "go_ssa", Version: "test", Operation: "fixture"}
	data := &ReportData{
		RepositoryGraph: &RepositoryGraph{
			Modules:      []ModuleInfo{{Path: "example.com/project"}},
			PackageEdges: []EdgeInfo{{From: "example.com/project/cmd", To: "example.com/project/internal/config"}},
		},
		ArchitectureGrounding: &ArchitectureGrounding{
			Version:             ArchitectureGroundingVersion,
			RepositoryArchetype: ArchitectureArchetype{Selected: componentmap.ArchetypeApplication},
			GroundingMode:       componentmap.GroundingMixed,
			BehaviorAnchors: []ArchitectureBehaviorAnchor{
				architectureGroundingTestAnchor("process", componentmap.AnchorProcessEntry, "cmd/main.go", 10, "example.com/project/cmd.main", scenario, producer),
				architectureGroundingTestAnchor("config", componentmap.AnchorConfigApply, "internal/config/config.go", 20, "example.com/project/internal/config.Apply", scenario, producer),
			},
			Relationships: []ArchitectureBehaviorHandoff{{
				ID: "process-config", From: "process", To: "config", Kind: "bounded_direct_call",
				Location:  evidence.Location{Path: "cmd/main.go", Line: 12, Column: 2},
				Certainty: evidence.CertaintyStatic, Producer: producer,
			}},
		},
	}

	input, err := BuildArchitectureCanvasInput(data)
	if err != nil {
		t.Fatal(err)
	}
	if input.CandidateBundle.RepositoryArchetype != componentmap.ArchetypeApplication ||
		input.CandidateBundle.GroundingMode != componentmap.GroundingMixed ||
		len(input.CandidateBundle.BehaviorAnchors) != 2 {
		t.Fatalf("grounded bundle = %#v", input.CandidateBundle)
	}
	canvas, err := ProjectArchitectureCanvas(input)
	if err != nil {
		t.Fatal(err)
	}
	if canvas.Title != "Evidence-backed architecture skeleton" || len(canvas.StructuralFacts) != 2 || len(canvas.StructuralEdges) != 1 {
		t.Fatalf("canvas grounding/edges = %q / %#v / %#v", canvas.Title, canvas.StructuralFacts, canvas.StructuralEdges)
	}
	if canvas.StructuralEdges[0].Witness.Kind != componentmap.StructuralRelationBehaviorHandoff {
		t.Fatalf("primary edge = %#v", canvas.StructuralEdges[0])
	}
}

func TestCompactArchitectureGroundingRelationshipsAggregatesRegistrationWitnesses(t *testing.T) {
	t.Parallel()

	grounding := &ArchitectureGrounding{
		BehaviorAnchors: []ArchitectureBehaviorAnchor{
			{ID: "registry", Kind: componentmap.AnchorRegistryWrite},
			{ID: "extensions", Kind: componentmap.AnchorExtensionFamily},
		},
	}
	for index := range 134 {
		grounding.Relationships = append(grounding.Relationships, ArchitectureBehaviorHandoff{
			ID: fmt.Sprintf("witness-%03d", index), From: "registry", To: "extensions",
			Kind: "bounded_direct_call", Location: evidence.Location{
				Path: fmt.Sprintf("modules/family-%02d/register.go", index%17), Line: index + 1, Column: 1,
			},
			Certainty: evidence.CertaintyStatic,
		})
	}
	compacted := compactArchitectureGroundingRelationships(grounding)
	if len(compacted) != 1 {
		t.Fatalf("compacted relationships = %d, want 1", len(compacted))
	}
	relation := compacted[0]
	if relation.Kind != "registers_extension_family" || relation.EvidenceKind != "bounded_direct_call" ||
		relation.WitnessCount != 134 || len(relation.WitnessIDs) != 134 || relation.PackageCount != 17 ||
		len(relation.RepresentativeLocations) != 8 {
		t.Fatalf("aggregated relationship = %#v", relation)
	}
}

func architectureGroundingTestAnchor(
	id string,
	kind componentmap.BehaviorAnchorKind,
	path string,
	line int,
	symbol string,
	scenario architectureGroundingScenario,
	producer evidence.Provenance,
) ArchitectureBehaviorAnchor {
	location := evidence.Location{Path: path, Line: line, Column: 1}
	return ArchitectureBehaviorAnchor{
		ID: id, Kind: kind, Label: symbol, Location: location, Scenario: scenario,
		Producer: producer, Certainty: evidence.CertaintyStatic,
		AssociatedMembers: []ArchitectureAnchorMember{{ID: symbol, Package: symbol, Name: symbol, Location: location}},
		Limitations:       []string{"Static fixture evidence; runtime execution is not observed."},
	}
}

func TestBuildArchitectureCanvasInputLeavesUnsupportedOwnershipAsFrontier(t *testing.T) {
	t.Parallel()

	proof := architectureBuildTestProof()
	proof.Anchors = append(proof.Anchors, flowproof.Anchor{
		ID: "unlisted-file", Kind: flowproof.AnchorOperation, Label: "plugin dispatch",
		Location: &evidence.Location{Path: "plugins/runtime.go", Line: 12},
	})
	proof.Transitions = append(proof.Transitions, flowproof.Transition{
		ID: "dynamic-plugin", From: "handler", To: "unlisted-file",
		Relation: evidence.RelationDispatches, Resolution: evidence.ResolutionUnresolved,
		Invocation: evidence.InvocationSynchronous, Certainty: evidence.CertaintyStatic,
		Evidence: evidence.Location{Path: "cmd/backup.go", Line: 24}, Provider: "go_types",
	})
	data := &ReportData{
		CandidateDirections: []CandidateDirection{{
			ID: "backup", Name: "Backup",
			LocalProof: &flowproof.Session{Version: flowproof.SessionVersion, Proof: proof},
		}},
		RepositoryGraph: &RepositoryGraph{
			Modules: []ModuleInfo{{Path: "example.com/project"}},
			PackageEdges: []EdgeInfo{{
				From: "example.com/project/cmd", To: "example.com/project/internal/repo",
			}},
		},
	}

	input, err := BuildArchitectureCanvasInput(data)
	if err != nil {
		t.Fatal(err)
	}
	bindings := architectureBuildTestBindingIndex(input.CandidateBundle.AnchorBindings)
	if _, exists := bindings["unknown-backend"]; exists {
		t.Fatal("locationless backend anchor received an invented member binding")
	}
	unlisted := architectureBuildTestCandidate(input.CandidateBundle, bindings["unlisted-file"].MemberID)
	if unlisted.ParentID != nil {
		t.Fatalf("unlisted file parent = %#v, want no package inferred outside exact saved endpoints", unlisted.ParentID)
	}

	canvas, err := ProjectArchitectureCanvas(input)
	if err != nil {
		t.Fatal(err)
	}
	if !architectureHasAnchorFrontier(canvas, "unassigned_component", "unknown-backend") {
		t.Fatalf("frontiers = %#v, want locationless anchor to remain explicit", canvas.Frontiers)
	}
	if !architectureHasFrontier(canvas, "unresolved_transition", "dynamic-plugin") {
		t.Fatalf("frontiers = %#v, want unresolved dynamic dispatch frontier", canvas.Frontiers)
	}
}

func architectureBuildTestProof() flowproof.Proof {
	return flowproof.Proof{
		Version: flowproof.Version, ID: "backup", Archetype: flowproof.ArchetypeCLI,
		Goal: "save repository data", Command: "backup",
		Slots: []flowproof.Slot{{
			Kind: flowproof.SlotEntrypoint, Status: flowproof.SlotVerified,
			EvidenceIDs: []string{"handler"}, Summary: "exact process entrypoint",
		}},
		Anchors: []flowproof.Anchor{
			{
				ID: "handler", Kind: flowproof.AnchorFunction, Label: "runBackup",
				QualifiedName: "cmd.runBackup", Location: &evidence.Location{Path: "cmd/backup.go", Line: 10},
			},
			{
				ID: "call-save", Kind: flowproof.AnchorCallsite, Label: "repo.Save",
				Location: &evidence.Location{Path: "cmd/backup.go", Line: 20},
			},
			{
				ID: "save", Kind: flowproof.AnchorMethod, Label: "Repository.Save",
				QualifiedName: "internal/repo.Repository.Save",
				Location:      &evidence.Location{Path: "internal/repo/repo.go", Line: 30},
			},
			{ID: "unknown-backend", Kind: flowproof.AnchorOperation, Label: "selected backend"},
		},
		Transitions: []flowproof.Transition{
			architectureBuildTestTransition("call", "handler", "call-save", evidence.RelationCalls, evidence.ResolutionStatic, 20),
			architectureBuildTestTransition("resolve", "call-save", "save", evidence.RelationResolvesTo, evidence.ResolutionStatic, 20),
			architectureBuildTestTransition("backend", "handler", "unknown-backend", evidence.RelationDispatches, evidence.ResolutionUnresolved, 22),
		},
	}
}

func architectureBuildTestTransition(
	id, from, to string,
	relation evidence.RelationKind,
	resolution evidence.ResolutionKind,
	line int,
) flowproof.Transition {
	return flowproof.Transition{
		ID: id, From: from, To: to, Relation: relation, Resolution: resolution,
		Invocation: evidence.InvocationSynchronous, Certainty: evidence.CertaintyStatic,
		Evidence: evidence.Location{Path: "cmd/backup.go", Line: line}, Provider: "go_types",
	}
}

func architectureBuildTestBindingIndex(
	bindings []componentmap.FlowAnchorBinding,
) map[string]componentmap.FlowAnchorBinding {
	result := make(map[string]componentmap.FlowAnchorBinding, len(bindings))
	for _, binding := range bindings {
		result[binding.AnchorID] = binding
	}
	return result
}

func architectureBuildTestCandidate(
	bundle componentmap.CandidateBundle,
	id componentmap.MemberID,
) componentmap.Candidate {
	for _, candidate := range bundle.Candidates {
		if candidate.ID == id {
			return candidate
		}
	}
	panic("missing candidate " + string(id.Kind) + ":" + id.Value)
}

func TestBuildArchitectureCanvasInputPreservesEntirelyUnboundFlow(t *testing.T) {
	t.Parallel()

	input, err := BuildArchitectureCanvasInput(&ReportData{CandidateDirections: []CandidateDirection{{
		ID: "unknown", Name: "Unknown",
		LocalProof: &flowproof.Session{Version: flowproof.SessionVersion, Proof: flowproof.Proof{
			Version: flowproof.Version, ID: "unknown", Archetype: flowproof.ArchetypeCLI,
			Anchors: []flowproof.Anchor{{ID: "opaque", Kind: flowproof.AnchorOperation, Label: "opaque"}},
		}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(input.CandidateBundle.Candidates) != 1 ||
		input.CandidateBundle.Candidates[0].ID.Kind != componentmap.MemberFlow {
		t.Fatalf("candidates = %#v, want only the exact saved flow member", input.CandidateBundle.Candidates)
	}
	if len(input.CandidateBundle.AnchorBindings) != 0 {
		t.Fatalf("bindings = %#v, want no invented member for locationless anchor", input.CandidateBundle.AnchorBindings)
	}
	canvas, err := ProjectArchitectureCanvas(input)
	if err != nil {
		t.Fatal(err)
	}
	if !architectureHasAnchorFrontier(canvas, "unassigned_component", "opaque") {
		t.Fatalf("frontiers = %#v, want explicit unbound anchor", canvas.Frontiers)
	}
}
