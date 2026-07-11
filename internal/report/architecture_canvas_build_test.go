package report

import (
	"encoding/json"
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

func TestReadRunDirProjectsSavedFlowProofIntoArchitectureCanvas(t *testing.T) {
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
	if data.ArchitectureCanvas == nil {
		t.Fatalf("architecture canvas missing; warnings = %#v", data.Warnings)
	}
	if !data.ArchitectureCanvas.Fallback || len(data.ArchitectureCanvas.Flows) != 1 ||
		data.ArchitectureCanvas.Flows[0].ID != "backup" {
		t.Fatalf("architecture canvas = %#v", data.ArchitectureCanvas)
	}

	writeArchitectureBuildFixture(t, runDir, ArchitectureSynthesisFile, []byte(`{"broken"`))
	data, err = ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if data.ArchitectureCanvas == nil || !data.ArchitectureCanvas.Fallback {
		t.Fatalf("invalid saved synthesis removed deterministic canvas: %#v", data.ArchitectureCanvas)
	}
	foundFallbackWarning := false
	for _, warning := range data.Warnings {
		if strings.Contains(warning, "using deterministic fallback") {
			foundFallbackWarning = true
		}
	}
	if !foundFallbackWarning {
		t.Fatalf("warnings = %#v, want invalid synthesis fallback warning", data.Warnings)
	}
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
	if data.ArchitectureCanvas == nil {
		t.Fatalf("architecture landscape missing without FlowProof; warnings = %#v", data.Warnings)
	}
	if len(data.ArchitectureCanvas.Components) == 0 || len(data.ArchitectureCanvas.Flows) != 0 {
		t.Fatalf("architecture canvas = %#v, want landscape with no flow overlay", data.ArchitectureCanvas)
	}
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
