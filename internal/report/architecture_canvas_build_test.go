package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/flowproof"
	"github.com/dvordrova/repomap/internal/modelresearch"
)

type architectureTestWireResponse struct {
	Subsystems []architectureTestWireSubsystem `json:"subsystems"`
}

type architectureTestWireSubsystem struct {
	Name        string                          `json:"name"`
	Description string                          `json:"description,omitempty"`
	Components  []architectureTestWireComponent `json:"components"`
}

type architectureTestWireComponent struct {
	Name        string                            `json:"name"`
	Description string                            `json:"description,omitempty"`
	MemberRefs  []componentmap.SynthesisMemberRef `json:"member_refs"`
	AnchorRefs  []componentmap.SynthesisAnchorRef `json:"anchor_refs,omitempty"`
	Hypothesis  bool                              `json:"hypothesis,omitempty"`
}

func architectureTestWireRefs(
	t *testing.T,
	bundle componentmap.CandidateBundle,
) ([]componentmap.SynthesisMemberRef, []componentmap.SynthesisAnchorRef) {
	t.Helper()
	request, _, err := componentmap.BuildSynthesisRequest(bundle)
	if err != nil {
		t.Fatal(err)
	}
	members := make([]componentmap.SynthesisMemberRef, 0, len(request.Candidates))
	for _, candidate := range request.Candidates {
		members = append(members, candidate.Ref)
	}
	anchors := make([]componentmap.SynthesisAnchorRef, 0, len(request.BehaviorAnchors))
	for _, anchor := range request.BehaviorAnchors {
		anchors = append(anchors, anchor.Ref)
	}
	return members, anchors
}

func marshalArchitectureTestWireResponse(
	t *testing.T,
	response architectureTestWireResponse,
) []byte {
	t.Helper()
	type subsystemRecord struct {
		Kind        string `json:"kind"`
		Ref         string `json:"ref"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	type componentRecord struct {
		Kind         string                            `json:"kind"`
		SubsystemRef string                            `json:"subsystem_ref"`
		Name         string                            `json:"name"`
		Description  string                            `json:"description"`
		MemberRefs   []componentmap.SynthesisMemberRef `json:"member_refs"`
		AnchorRefs   []componentmap.SynthesisAnchorRef `json:"anchor_refs"`
		Hypothesis   bool                              `json:"hypothesis"`
	}
	records := make([]any, 0, len(response.Subsystems)*2)
	for index, subsystem := range response.Subsystems {
		ref := fmt.Sprintf("g%d", index+1)
		records = append(records, subsystemRecord{
			Kind: "subsystem", Ref: ref,
			Name: subsystem.Name, Description: subsystem.Description,
		})
		for _, component := range subsystem.Components {
			memberRefs := append([]componentmap.SynthesisMemberRef(nil), component.MemberRefs...)
			if memberRefs == nil {
				memberRefs = []componentmap.SynthesisMemberRef{}
			}
			anchorRefs := append([]componentmap.SynthesisAnchorRef(nil), component.AnchorRefs...)
			if anchorRefs == nil {
				anchorRefs = []componentmap.SynthesisAnchorRef{}
			}
			records = append(records, componentRecord{
				Kind: "component", SubsystemRef: ref,
				Name: component.Name, Description: component.Description,
				MemberRefs: memberRefs, AnchorRefs: anchorRefs,
				Hypothesis: component.Hypothesis,
			})
		}
	}
	encoded, err := json.Marshal(struct {
		Records []any `json:"records"`
	}{Records: records})
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func architectureTestExactProviderRequestBytes(
	t *testing.T,
	bundle componentmap.CandidateBundle,
	outputLanguage string,
) int {
	t.Helper()
	prompt, err := componentmap.BuildSynthesisPromptForLanguage(bundle, outputLanguage)
	if err != nil {
		t.Fatal(err)
	}
	body, err := (&deepseek.Client{
		Endpoint:  "https://example.invalid/chat/completions",
		Model:     "test-model",
		MaxTokens: 64_000,
	}).ComponentSynthesisPromptJSON(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if promptBytes := len(prompt.System) + len(prompt.User); len(body) <= promptBytes {
		t.Fatalf("exact provider body bytes = %d, prompt bytes = %d", len(body), promptBytes)
	}
	return len(body)
}

func TestReplayArchitectureSynthesisChangesOnlyValidatedConceptualMembership(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		RepositoryGraph: &RepositoryGraph{PackageEdges: []EdgeInfo{{
			From: "example.com/project/cmd", To: "example.com/project/internal/repo",
		}}},
		CandidateDirections: []CandidateDirection{{
			ID: "backup", Name: "Backup",
			LocalProof: &flowproof.Session{Version: flowproof.SessionVersion, Proof: flowproof.Proof{
				Version: flowproof.Version, ID: "backup", Archetype: flowproof.ArchetypeCLI,
				SeedSurfaceID: "surface-backup",
				Anchors:       []flowproof.Anchor{{ID: "unknown", Kind: flowproof.AnchorOperation, Label: "unknown"}},
			}},
		}},
	}
	bindArchitectureTestDirection(data, "surface-backup")
	input, err := BuildArchitectureCanvasInput(data)
	if err != nil {
		t.Fatal(err)
	}
	memberRefs, _ := architectureTestWireRefs(t, input.CandidateBundle)
	response := marshalArchitectureTestWireResponse(t, architectureTestWireResponse{
		Subsystems: []architectureTestWireSubsystem{{
			Name: "Data protection",
			Components: []architectureTestWireComponent{{
				Name: "Backup execution", MemberRefs: memberRefs,
			}},
		}},
	})
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
	result = bindArchitectureBuildSynthesisProviderIdentity(t, input.CandidateBundle, "revision-test", result)
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

func TestProjectSavedArchitectureCanvasPreservesExactD177Substrate(t *testing.T) {
	t.Parallel()

	scenario := architectureGroundingScenario{ID: "go:test", GOOS: "darwin", GOARCH: "arm64"}
	producer := evidence.Provenance{Provider: "go_ssa", Version: "test", Operation: "fixture"}
	data := &ReportData{
		RepositoryGraph: &RepositoryGraph{
			Modules: []ModuleInfo{{Path: "example.com/project"}},
			PackageEdges: []EdgeInfo{
				{From: "example.com/project/cmd", To: "example.com/project/internal/config"},
				{From: "example.com/project/internal/config", To: "example.com/project/internal/runtime"},
			},
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
		DiscoveredSurfaces: &DiscoveredSurfaces{Triggers: []DiscoveredTrigger{{
			ID: "surface-main", Kind: "process_entry", ExecutableRole: ExecutableRolePrimaryApplication,
			Resolution: "exact", Status: "available", Certainty: string(evidence.CertaintyStatic),
			SurfaceRole: SurfaceRoleEntrySurface,
			ProcessEntrypoint: SurfaceSymbol{
				ID: "example.com/project/cmd.main", Package: "example.com/project/cmd", Name: "main",
				Location: &SurfaceLocation{Path: "cmd/main.go", Line: 10, Column: 1},
			},
		}}},
	}
	if warning := projectCanonicalArchitectureCanvas(data); warning != "" {
		t.Fatalf("project canonical canvas: %s", warning)
	}
	linkArchitectureProductObjects(data)

	beforeInput, err := BuildArchitectureCanvasInput(data)
	if err != nil {
		t.Fatal(err)
	}
	beforeCandidates := flattenedArchitectureCandidates(data.ArchitectureCanvas)
	if !reflect.DeepEqual(beforeCandidates, beforeInput.CandidateBundle.Candidates) {
		t.Fatalf("canonical flattened candidates differ from exact input:\ncanvas=%#v\ninput=%#v", beforeCandidates, beforeInput.CandidateBundle.Candidates)
	}
	beforeRelations := beforeInput.CandidateBundle.Relations
	beforeBindings := beforeInput.CandidateBundle.AnchorBindings
	beforeAnchors := append([]componentmap.BehaviorAnchor(nil), data.ArchitectureCanvas.BehaviorAnchors...)
	beforeStructuralFacts := append([]componentmap.LocalRelation(nil), data.ArchitectureCanvas.StructuralFacts...)
	beforeSurfaces := exactArchitectureSurfaceEvidence(data.ArchitectureCanvas.Surfaces)

	memberRefs, anchorRefs := architectureTestWireRefs(t, beforeInput.CandidateBundle)
	response := marshalArchitectureTestWireResponse(t, architectureTestWireResponse{
		Subsystems: []architectureTestWireSubsystem{{
			Name: "Model conceptual group", Description: "Provider-authored conceptual description",
			Components: []architectureTestWireComponent{{
				Name: "Model component", Description: "Provider-authored component description",
				MemberRefs: memberRefs, AnchorRefs: anchorRefs,
			}},
		}},
	})
	result, err := componentmap.RecordSynthesisResponse(
		beforeInput.CandidateBundle,
		"revision-d177-substrate",
		"openai-compatible/bearer",
		"test-model",
		12*time.Millisecond,
		response,
	)
	if err != nil {
		t.Fatal(err)
	}
	result = bindArchitectureBuildSynthesisProviderIdentity(t, beforeInput.CandidateBundle, "revision-d177-substrate", result)
	saved, err := json.Marshal(result.Record)
	if err != nil {
		t.Fatal(err)
	}
	if warning := projectSavedArchitectureCanvasBytes(data, saved); warning != "" {
		t.Fatalf("project accepted saved synthesis: %s", warning)
	}
	linkArchitectureProductObjects(data)

	afterInput, err := BuildArchitectureCanvasInput(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := flattenedArchitectureCandidates(data.ArchitectureCanvas); !reflect.DeepEqual(got, beforeCandidates) {
		t.Fatalf("accepted synthesis changed exact Candidate values (including facts, participations, or parent):\nbefore=%#v\nafter=%#v", beforeCandidates, got)
	}
	if !reflect.DeepEqual(afterInput.CandidateBundle.Candidates, beforeInput.CandidateBundle.Candidates) ||
		!reflect.DeepEqual(afterInput.CandidateBundle.Relations, beforeRelations) ||
		!reflect.DeepEqual(afterInput.CandidateBundle.AnchorBindings, beforeBindings) {
		t.Fatalf("accepted synthesis changed exact candidate-bundle substrate:\nbefore=%#v\nafter=%#v", beforeInput.CandidateBundle, afterInput.CandidateBundle)
	}
	if !reflect.DeepEqual(data.ArchitectureCanvas.BehaviorAnchors, beforeAnchors) ||
		!reflect.DeepEqual(data.ArchitectureCanvas.StructuralFacts, beforeStructuralFacts) {
		t.Fatalf("accepted synthesis changed local canvas evidence:\nanchors=%#v\nstructural=%#v", data.ArchitectureCanvas.BehaviorAnchors, data.ArchitectureCanvas.StructuralFacts)
	}
	if got := exactArchitectureSurfaceEvidence(data.ArchitectureCanvas.Surfaces); !reflect.DeepEqual(got, beforeSurfaces) {
		t.Fatalf("accepted synthesis changed exact Surface IDs/evidence:\nbefore=%#v\nafter=%#v", beforeSurfaces, got)
	}
	if data.ArchitectureCanvas.ArchitectureSource != componentmap.SourceValidatedModel ||
		len(data.ArchitectureCanvas.Subsystems) != 1 ||
		data.ArchitectureCanvas.Subsystems[0].Name != "Model conceptual group" ||
		len(data.ArchitectureCanvas.Components) != 1 ||
		data.ArchitectureCanvas.Components[0].Name != "Model component" {
		t.Fatalf("accepted synthesis did not replace only the conceptual grouping/wording: %#v", data.ArchitectureCanvas)
	}
}

func flattenedArchitectureCandidates(canvas *ArchitectureCanvas) []componentmap.Candidate {
	if canvas == nil {
		return nil
	}
	result := make([]componentmap.Candidate, 0)
	for _, component := range canvas.Components {
		result = append(result, component.Members...)
	}
	for _, locator := range canvas.StructuralLocators {
		result = append(result, locator.Locator)
	}
	slices.SortFunc(result, func(left, right componentmap.Candidate) int {
		if left.ID.Kind != right.ID.Kind {
			return strings.Compare(string(left.ID.Kind), string(right.ID.Kind))
		}
		return strings.Compare(left.ID.Value, right.ID.Value)
	})
	return result
}

type architectureSurfaceExactEvidence struct {
	ID       string
	Evidence []SurfaceLocation
}

func exactArchitectureSurfaceEvidence(surfaces []ArchitectureSurface) []architectureSurfaceExactEvidence {
	result := make([]architectureSurfaceExactEvidence, 0, len(surfaces))
	for _, surface := range surfaces {
		result = append(result, architectureSurfaceExactEvidence{
			ID: surface.ID, Evidence: append([]SurfaceLocation(nil), surface.Evidence...),
		})
	}
	slices.SortFunc(result, func(left, right architectureSurfaceExactEvidence) int {
		return strings.Compare(left.ID, right.ID)
	})
	return result
}

func TestBuildArchitectureCanvasKeepsExactSurfaceRoleWithoutPublishingDirectionOverlay(t *testing.T) {
	t.Parallel()

	location := evidence.Location{Path: "cmd/inspect/main.go", Line: 12}
	proof := flowproof.BuildProcess(flowproof.ProcessSeed{
		FlowID: "inspect", Goal: "Inspect service startup", SeedSurfaceID: "surface-inspect",
		Entrypoint: flowproof.StaticSurfaceFact{
			ID: "surface-inspect", Kind: "process_entry", Label: "main",
			QualifiedName: "example.com/project/cmd/inspect.main", Location: location,
		},
		CurrentFrontier: "downstream runtime handoff remains unresolved",
	})
	session := flowproof.Start(proof, flowproof.DefaultBudget(), "go:test", flowproof.SurfaceCollectorVersion)
	data := &ReportData{
		ArchitectureGrounding: &ArchitectureGrounding{
			Version:             ArchitectureGroundingVersion,
			RepositoryArchetype: ArchitectureArchetype{Selected: componentmap.ArchetypeApplication},
			GroundingMode:       componentmap.GroundingBehavior,
			BehaviorAnchors: []ArchitectureBehaviorAnchor{{
				ID: "inspect-entry", Kind: componentmap.AnchorProcessEntry,
				ProofMode: componentmap.AnchorProofProcessEntry,
				Label:     "process entry example.com/project/cmd/inspect.main", Location: location,
				Scenario:  architectureGroundingScenario{ID: "go:test", GOOS: "test", GOARCH: "test"},
				Producer:  evidence.Provenance{Provider: "gofacts", Version: "entrypoint-anchor-v1", Operation: "classify_exact_process_entry"},
				Certainty: evidence.CertaintyStatic,
				AssociatedMembers: []ArchitectureAnchorMember{{
					ID: "example.com/project/cmd/inspect.main", Package: "example.com/project/cmd/inspect",
					Name: "main", Location: location,
				}},
				Limitations: []string{"execution not observed"},
			}},
		},
		DiscoveredSurfaces: &DiscoveredSurfaces{Triggers: []DiscoveredTrigger{{
			ID: "surface-inspect", Kind: "process_entry", ExecutableRole: ExecutableRoleTooling,
			Resolution: "exact", SurfaceRole: SurfaceRoleEntrySurface,
			ProcessEntrypoint: SurfaceSymbol{
				ID:       "example.com/project/cmd/inspect.main",
				Location: &SurfaceLocation{Path: location.Path, Line: location.Line},
			},
		}}},
		CandidateDirections: []CandidateDirection{{
			ID: "inspect", Name: "Inspect service startup", LocalProof: &session,
			CandidateBasis: flowexplain.CandidateBasisLocalEntrypoint,
		}},
	}

	input, err := BuildArchitectureCanvasInput(data)
	if err != nil {
		t.Fatal(err)
	}
	foundRole := false
	var roleMemberID componentmap.MemberID
	for _, candidate := range input.CandidateBundle.Candidates {
		for _, fact := range candidate.Facts {
			if fact.Kind == componentmap.FactExecutableRole && fact.Value == ExecutableRoleTooling {
				foundRole = true
				roleMemberID = candidate.ID
			}
		}
	}
	if !foundRole {
		t.Fatal("exact tooling role was not joined to the process-entry candidate")
	}
	if len(input.CandidateBundle.AnchorBindings) != 0 || len(input.Flows) != 0 ||
		len(input.CandidateBundle.Flows) != 0 {
		t.Fatalf("CandidateDirection proof became an architecture overlay: role=%#v bindings=%#v flows=%#v", roleMemberID, input.CandidateBundle.AnchorBindings, input.Flows)
	}
}

func TestCandidateDirectionTraceQualityRemainsOutsideArchitectureOverlay(t *testing.T) {
	t.Parallel()

	proof := flowproof.BuildProcess(flowproof.ProcessSeed{
		FlowID: "startup", Goal: "Application startup", SeedSurfaceID: "surface-main",
		Entrypoint: flowproof.StaticSurfaceFact{
			ID: "surface-main", Kind: "process_entry", Label: "main",
			QualifiedName: "example.com/project/cmd/app.main",
			Location:      evidence.Location{Path: "cmd/app/main.go", Line: 10},
		},
		CurrentFrontier: "downstream runtime handoff remains unresolved",
	})
	session := flowproof.Start(proof, flowproof.DefaultBudget(), "go-default", flowproof.SurfaceCollectorVersion)
	data := &ReportData{
		FormatVersion: CurrentFormatVersion, RepoName: "project",
		RepositoryGraph: &RepositoryGraph{PackageEdges: []EdgeInfo{{
			From: "example.com/project/cmd/app", To: "example.com/project/internal/runtime",
		}}},
		CandidateDirections: []CandidateDirection{{
			ID: "startup", Name: "Application startup", LocalProof: &session,
		}},
	}
	bindArchitectureTestDirection(data, "surface-main")
	input, err := BuildArchitectureCanvasInput(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Flows) != 0 || len(input.CandidateBundle.Flows) != 0 ||
		len(input.CandidateBundle.AnchorBindings) != 0 {
		t.Fatalf("CandidateDirection trace became an architecture overlay: %#v", input)
	}
	if data.CandidateDirections[0].LocalProof.Proof.TraceQuality != proof.TraceQuality ||
		data.CandidateDirections[0].LocalProof.Proof.CurrentFrontier != proof.CurrentFrontier {
		t.Fatalf("Study/debug proof was mutated: %#v", data.CandidateDirections[0].LocalProof)
	}
}

func TestBuildArchitectureCanvasInputConsumesAcceptedResearchAsInterpretation(t *testing.T) {
	t.Parallel()

	location := evidence.Location{Path: "cmd/backup.go", Line: 20}
	state := modelresearch.NewState(modelresearch.DefaultPolicy(), modelresearch.RepositoryContext{
		Identity: "/fixture", Revision: "abc", Scenario: "go-default",
	})
	state.Theory.GroundedFacts = []modelresearch.EvidenceItem{{
		ID: "evidence-backup", Kind: modelresearch.EvidenceCallsite,
		Statement: "exact backup callsite", Location: &location, Certainty: evidence.CertaintyStatic,
	}}
	state.Rounds = []modelresearch.ResearchRound{{
		Version: modelresearch.ContractVersion, ID: "research-backup", Question: "How does backup run?",
		Status: modelresearch.RoundCompleted,
		ValidatedFindings: []modelresearch.ValidatedFinding{{
			ID: "finding-backup", Interpretation: "backup coordinates repository work",
			HypothesisAssessment: "supported", EvidenceIDs: []string{"evidence-backup"},
		}},
	}}
	data := &ReportData{
		ModelResearch: &state,
		ArchitectureGrounding: &ArchitectureGrounding{
			Version:             ArchitectureGroundingVersion,
			RepositoryArchetype: ArchitectureArchetype{Selected: componentmap.ArchetypeApplication},
			GroundingMode:       componentmap.GroundingBehavior,
			BehaviorAnchors: []ArchitectureBehaviorAnchor{architectureGroundingTestAnchor(
				"backup-call", componentmap.AnchorRequestDispatchRoot,
				"cmd/backup.go", 20, "cmd.runBackup", architectureGroundingScenario{ID: "go:test"},
				evidence.Provenance{Provider: "go_types", Version: "test", Operation: "exact_callsite"},
			)},
		},
		CandidateDirections: []CandidateDirection{{
			ID: "backup", Name: "Backup", LocalProof: &flowproof.Session{
				Version: flowproof.SessionVersion, Proof: architectureBuildTestProof(),
			},
		}},
	}
	bindArchitectureTestDirection(data, "surface-backup")
	input, err := BuildArchitectureCanvasInput(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(input.CandidateBundle.ResearchFindings) != 1 {
		t.Fatalf("research findings = %#v, want one", input.CandidateBundle.ResearchFindings)
	}
	finding := input.CandidateBundle.ResearchFindings[0]
	if finding.Interpretation != "backup coordinates repository work" || len(finding.MemberIDs) == 0 {
		t.Fatalf("research finding = %#v, want interpretation bound to exact members", finding)
	}
	for _, candidate := range input.CandidateBundle.Candidates {
		for _, fact := range candidate.Facts {
			if fact.Value == finding.Interpretation {
				t.Fatal("model interpretation was incorrectly promoted to a local fact")
			}
		}
	}
}

func TestReadRunDirKeepsCanonicalArchitectureCanvasWhenSavedSynthesisIsUnavailable(t *testing.T) {
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
	if data.ArchitectureCanvas == nil || data.ArchitectureCanvas.Fallback ||
		data.ArchitectureCanvas.ArchitectureSource != componentmap.SourceLocalPackages ||
		len(data.ArchitectureCanvas.Components) != 2 || len(data.ArchitectureCanvas.Flows) != 0 {
		t.Fatalf("canonical architecture canvas = %#v", data.ArchitectureCanvas)
	}
	writeArchitectureBuildSynthesis(t, runDir, data, "revision-proof")
	data, err = ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if data.ArchitectureCanvas == nil || data.ArchitectureCanvas.Fallback ||
		len(data.ArchitectureCanvas.Components) != 2 || len(data.ArchitectureCanvas.Flows) != 0 {
		t.Fatalf("architecture canvas = %#v", data.ArchitectureCanvas)
	}

	writeArchitectureBuildFixture(t, runDir, ArchitectureSynthesisFile, []byte(`{"broken"`))
	if _, err = ReadRunDir(runDir); err == nil {
		t.Fatal("accepted Architecture status allowed a malformed saved synthesis")
	}
}

func TestReadRunDirRetainsValidProducerArchitectureGroundingV4InCanvas(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	writeArchitectureBuildFixture(t, runDir, "snapshot.json", []byte(`{"repo_name":"fixture"}`))
	scenario := architectureGroundingScenario{ID: "go:test", GOOS: "darwin", GOARCH: "arm64"}
	producer := evidence.Provenance{
		Provider: "go_ssa", Version: "fixture-v4", Operation: "producer_grounding",
	}
	process := architectureGroundingTestAnchor(
		"process", componentmap.AnchorProcessEntry,
		"cmd/main.go", 10, "example.com/project/cmd.main", scenario, producer,
	)
	family := architectureGroundingTestAnchor(
		"lifecycle-family", componentmap.AnchorLifecycleStart,
		"service/start.go", 20, "example.com/project/service.Start", scenario, producer,
	)
	family.ProofMode = componentmap.AnchorProofDeclarationFamily
	grounding := ArchitectureGrounding{
		Version: typedArchitectureGroundingVersion,
		RepositoryArchetype: ArchitectureArchetype{
			Selected: componentmap.ArchetypeApplication,
			Evidence: []string{"Exact producer-owned process entry."},
		},
		GroundingMode:   componentmap.GroundingMixed,
		BehaviorAnchors: []ArchitectureBehaviorAnchor{process, family},
		Coverage: ArchitectureGroundingCoverage{
			Complete:          true,
			AnchorsConsidered: 2, AnchorsPublished: 2,
			DeclarationFamilyMembersConsidered: 1,
			DeclarationFamilyMembersPublished:  1,
		},
	}
	encoded, err := json.Marshal(grounding)
	if err != nil {
		t.Fatal(err)
	}
	writeArchitectureBuildFixture(t, runDir, ArchitectureGroundingFile, encoded)

	data, err := ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if data.ArchitectureGrounding == nil || !reflect.DeepEqual(*data.ArchitectureGrounding, grounding) {
		t.Fatalf("producer grounding was substituted:\ngot  %#v\nwant %#v", data.ArchitectureGrounding, grounding)
	}
	if containsWarning(data.Warnings, "architecture grounding:") {
		t.Fatalf("valid producer grounding emitted a replay warning: %#v", data.Warnings)
	}
	if data.ArchitectureCanvas == nil || data.ArchitectureCanvas.Fallback ||
		data.ArchitectureCanvas.GroundingMode != componentmap.GroundingMixed ||
		len(data.ArchitectureCanvas.BehaviorAnchors) != 2 {
		t.Fatalf("producer-grounded architecture canvas = %#v", data.ArchitectureCanvas)
	}
	anchorModes := make(map[string]componentmap.AnchorProofMode, len(data.ArchitectureCanvas.BehaviorAnchors))
	for _, anchor := range data.ArchitectureCanvas.BehaviorAnchors {
		anchorModes[anchor.ID] = anchor.ProofMode
	}
	if anchorModes[process.ID] != componentmap.AnchorProofProcessEntry ||
		anchorModes[family.ID] != componentmap.AnchorProofDeclarationFamily {
		t.Fatalf("canvas behavior anchors = %#v", data.ArchitectureCanvas.BehaviorAnchors)
	}
	input, err := BuildArchitectureCanvasInput(data)
	if err != nil {
		t.Fatal(err)
	}
	if input.CandidateBundle.GroundingMode != componentmap.GroundingMixed ||
		len(input.CandidateBundle.BehaviorAnchors) != 2 {
		t.Fatalf("replayed producer grounding input = %#v", input.CandidateBundle)
	}
}

func TestReadRunDirReportsFailedArchitectureSynthesisWithoutProductFallback(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	writeArchitectureBuildFixture(t, runDir, "snapshot.json", []byte(`{"repo_name":"fixture"}`))
	writeArchitectureBuildFixture(t, runDir, "llm_bundle.json", []byte(`{
		"go": {
			"module_summaries": [{"module_path":"example.com/project","module_dir":"."}],
			"important_edges": [{"from":"example.com/project/cmd","to":"example.com/project/internal/repo"}]
		}
	}`))
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
	if data.ArchitectureCanvas == nil || data.ArchitectureCanvas.Fallback ||
		data.ArchitectureCanvas.ArchitectureSource != componentmap.SourceLocalPackages {
		t.Fatalf("canonical architecture canvas = %#v", data.ArchitectureCanvas)
	}
	if data.ArchitectureSynthesis == nil || data.ArchitectureSynthesis.State != ArchitectureSynthesisFailed {
		t.Fatalf("architecture status = %#v, want failed", data.ArchitectureSynthesis)
	}
	if data.Run == nil || data.Run.ProviderRequestCount != 2 {
		t.Fatalf("run = %#v, want both orientation and architecture provider attempts", data.Run)
	}
	if containsWarning(data.Warnings, "grouping request returned no content") ||
		containsWarning(data.Warnings, "architecture map unavailable") {
		t.Fatalf("ordinary warnings exposed optional synthesis diagnostics: %#v", data.Warnings)
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

func architectureCanvasComponentIDs(canvas *ArchitectureCanvas) []componentmap.ComponentID {
	if canvas == nil {
		return nil
	}
	ids := make([]componentmap.ComponentID, 0, len(canvas.Components))
	for _, component := range canvas.Components {
		ids = append(ids, component.ID)
	}
	slices.Sort(ids)
	return ids
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
	if data.ArchitectureCanvas == nil || data.ArchitectureCanvas.Fallback ||
		data.ArchitectureCanvas.ArchitectureSource != componentmap.SourceLocalPackages ||
		len(data.ArchitectureCanvas.Components) == 0 || len(data.ArchitectureCanvas.Flows) != 0 {
		t.Fatalf("canonical architecture canvas = %#v", data.ArchitectureCanvas)
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
	memberRefs, _ := architectureTestWireRefs(t, input.CandidateBundle)
	components := make([]architectureTestWireComponent, 0, len(memberRefs))
	for index, memberRef := range memberRefs {
		components = append(components, architectureTestWireComponent{
			Name:       fmt.Sprintf("Component %d", index+1),
			MemberRefs: []componentmap.SynthesisMemberRef{memberRef},
			Hypothesis: input.CandidateBundle.GroundingMode != componentmap.GroundingPackages,
		})
	}
	response := marshalArchitectureTestWireResponse(t, architectureTestWireResponse{
		Subsystems: []architectureTestWireSubsystem{{
			Name: "Runtime", Components: components,
		}},
	})
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
	result.Record.Call.Metadata.UsageReported = true
	result.Record.Call.Metadata.InputTokens = 25
	result.Record.Call.Metadata.OutputTokens = 11
	result.Record.Call.Metadata.FinishReason = "stop"
	result.Record.Call.Metadata.TransportAttempts = 1
	result.Record.Call.Metadata.ResponseComplete = true
	result = bindArchitectureBuildSynthesisProviderIdentity(t, input.CandidateBundle, revision, result)
	saved, err := json.Marshal(result.Record)
	if err != nil {
		t.Fatal(err)
	}
	writeArchitectureBuildFixture(t, runDir, ArchitectureSynthesisFile, saved)
	status := architectureSynthesisV4AcceptedFixture()
	status.RequestBytes = architectureTestExactProviderRequestBytes(t, input.CandidateBundle, "en")
	status.ResponseBytes = result.Record.Call.ResponseBytes
	status.ResponseContentBytes = len(result.Record.Call.Response)
	conceptualCount, structuralLocatorCount := input.CandidateBundle.CandidateRoleCounts()
	status.LocalCandidateCount = len(input.CandidateBundle.Candidates)
	status.RequestedConceptualCount = conceptualCount
	status.StructuralLocatorCount = structuralLocatorCount
	status.AnchorCount = len(input.CandidateBundle.BehaviorAnchors)
	status.MemberOccurrences = conceptualCount
	status.DistinctMembers = conceptualCount
	status.ProposalAccepted = result.Landscape.ValidationOutcome == componentmap.ValidationAccepted ||
		result.Landscape.ValidationOutcome == componentmap.ValidationAcceptedNormalized
	status.ProposalNormalized = result.Landscape.ValidationOutcome == componentmap.ValidationAcceptedNormalized
	status.ArchitectureSource = string(result.Landscape.Source)
	status.ArchitectureLevel = result.Landscape.Level
	status.NormalizationCount = len(result.Landscape.Normalizations)
	statusJSON, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	writeArchitectureBuildFixture(t, runDir, ArchitectureSynthesisStatusFile, statusJSON)
}

func bindArchitectureBuildSynthesisProviderIdentity(
	t *testing.T,
	bundle componentmap.CandidateBundle,
	revision string,
	result componentmap.SynthesisResult,
) componentmap.SynthesisResult {
	t.Helper()
	client := &deepseek.Client{
		Endpoint:  "https://example.invalid/chat/completions",
		Model:     "test-model",
		MaxTokens: 64_000,
	}
	prompt, err := componentmap.BuildSynthesisPrompt(bundle)
	if err != nil {
		t.Fatal(err)
	}
	body, err := client.ComponentSynthesisPromptJSON(prompt)
	if err != nil {
		t.Fatal(err)
	}
	endpointSHA, err := modelresearch.ProviderEndpointSHA256(client.Endpoint)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := componentmap.BindSynthesisProviderIdentity(
		bundle,
		revision,
		result,
		componentmap.SynthesisProviderIdentity{
			RequestSHA256:  modelresearch.SHA256(body),
			EndpointSHA256: endpointSHA,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return bound
}

func writeArchitectureBuildFixture(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestBuildArchitectureCanvasInputUsesExactPackageWitnessesWithoutDirectionBindings(t *testing.T) {
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
	bindArchitectureTestDirection(data, "surface-backup")

	input, err := BuildArchitectureCanvasInput(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := input.CandidateBundle.Validate(); err != nil {
		t.Fatalf("candidate bundle is invalid: %v", err)
	}
	if input.Landscape.Fallback || input.Landscape.FallbackReason != "" ||
		input.Landscape.Source != componentmap.SourceLocalPackages {
		t.Fatalf(
			"canonical landscape = fallback %v (%q), source %q",
			input.Landscape.Fallback,
			input.Landscape.FallbackReason,
			input.Landscape.Source,
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

	if len(input.CandidateBundle.AnchorBindings) != 0 || len(input.Flows) != 0 ||
		len(input.CandidateBundle.Flows) != 0 {
		t.Fatalf("CandidateDirection proof became an architecture overlay: %#v", input)
	}
}

func TestBuildArchitectureCanvasInputPreservesCandidateBundleLimitError(t *testing.T) {
	t.Parallel()

	const candidateLimit = 512
	packages := make([]PackageInfo, 0, candidateLimit+1)
	for index := 0; index <= candidateLimit; index++ {
		packagePath := fmt.Sprintf("example.com/project/package-%03d", index)
		packages = append(packages, PackageInfo{
			CanonicalPath: packagePath,
			Name:          fmt.Sprintf("package-%03d", index),
			ModulePath:    "example.com/project",
			DisplayPath:   fmt.Sprintf("package-%03d", index),
			Locality:      "local",
		})
	}

	_, err := BuildArchitectureCanvasInput(&ReportData{RepositoryGraph: &RepositoryGraph{
		Packages: packages,
	}})
	var limitErr *componentmap.CandidateBundleLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("BuildArchitectureCanvasInput() error = %v, want CandidateBundleLimitError", err)
	}
	if limitErr.Kind != componentmap.CandidateBundleLimitCandidates ||
		limitErr.Observed != candidateLimit+1 || limitErr.Limit != candidateLimit {
		t.Fatalf("preserved limit error = %#v", limitErr)
	}
	if !strings.Contains(err.Error(), "architecture canvas build: candidate bundle") {
		t.Fatalf("wrapped error lost build context: %v", err)
	}
}

func TestBuildArchitectureCanvasInputOmitsNestedFixtureModuleAtomically(t *testing.T) {
	t.Parallel()

	const (
		rootPackageCount    = 92
		fixturePackageCount = 893
		rootEdgeCount       = 206
	)
	graph := &RepositoryGraph{
		Version: 2,
		Modules: []ModuleInfo{
			{ID: "root", Path: "example.com/product", Dir: ""},
			{ID: "fixture", Path: "example.com/product/endtoend", Dir: "internal/endtoend/testdata"},
		},
		Packages: make([]PackageInfo, 0, rootPackageCount+fixturePackageCount),
	}
	rootPaths := make([]string, rootPackageCount)
	for index := range rootPackageCount {
		packagePath := fmt.Sprintf("example.com/product/pkg/p%03d", index)
		rootPaths[index] = packagePath
		graph.Packages = append(graph.Packages, PackageInfo{
			CanonicalPath: packagePath,
			Name:          fmt.Sprintf("p%03d", index),
			ModuleID:      "root",
			ModulePath:    "example.com/product",
			Dir:           fmt.Sprintf("pkg/p%03d", index),
			DisplayPath:   fmt.Sprintf("pkg/p%03d", index),
			Locality:      "local",
		})
	}
	fixturePaths := make([]string, fixturePackageCount)
	for index := range fixturePackageCount {
		packagePath := fmt.Sprintf("example.com/product/endtoend/case%03d", index)
		fixturePaths[index] = packagePath
		graph.Packages = append(graph.Packages, PackageInfo{
			CanonicalPath: packagePath,
			Name:          fmt.Sprintf("case%03d", index),
			ModuleID:      "fixture",
			ModulePath:    "example.com/product/endtoend",
			Dir:           fmt.Sprintf("internal/endtoend/testdata/case%03d", index),
			DisplayPath:   fmt.Sprintf("internal/endtoend/testdata/case%03d", index),
			Locality:      "local",
		})
	}
	for offset := 1; len(graph.PackageEdges) < rootEdgeCount; offset++ {
		for index := 0; index < rootPackageCount && len(graph.PackageEdges) < rootEdgeCount; index++ {
			graph.PackageEdges = append(graph.PackageEdges, EdgeInfo{
				From: rootPaths[index], To: rootPaths[(index+offset)%rootPackageCount],
			})
		}
	}
	graph.PackageEdges = append(graph.PackageEdges,
		EdgeInfo{From: fixturePaths[0], To: fixturePaths[1]},
		EdgeInfo{From: fixturePaths[1], To: fixturePaths[2]},
		EdgeInfo{From: fixturePaths[2], To: fixturePaths[0]},
	)

	scenario := architectureGroundingScenario{ID: "go:test", GOOS: "linux", GOARCH: "amd64"}
	producer := evidence.Provenance{Provider: "go_ssa", Version: "fixture", Operation: "entry_handoff"}
	rootAnchor := architectureGroundingTestAnchor(
		"root-entry", componentmap.AnchorProcessEntry,
		"pkg/p000/main.go", 7, "example.com/product/pkg/p000.main", scenario, producer,
	)
	fixtureAnchor := architectureGroundingTestAnchor(
		"fixture-entry", componentmap.AnchorProcessEntry,
		"internal/endtoend/testdata/case000/main.go", 9,
		"example.com/product/endtoend/case000.main", scenario, producer,
	)
	crossModuleAnchor := architectureGroundingTestAnchor(
		"fixture-callsite-production-target", componentmap.AnchorLifecycleStart,
		"internal/endtoend/testdata/case001/fixture.go", 11,
		"example.com/product/pkg/p001.Start", scenario, producer,
	)
	crossModuleAnchor.AssociatedMembers[0].Location = evidence.Location{
		Path: "pkg/p001/start.go", Line: 13, Column: 1,
	}
	input, err := BuildArchitectureCanvasInput(&ReportData{
		RepositoryGraph: graph,
		ArchitectureGrounding: &ArchitectureGrounding{
			RepositoryArchetype: ArchitectureArchetype{Selected: componentmap.ArchetypeApplication},
			GroundingMode:       componentmap.GroundingMixed,
			BehaviorAnchors: []ArchitectureBehaviorAnchor{
				rootAnchor, fixtureAnchor, crossModuleAnchor,
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildArchitectureCanvasInput: %v", err)
	}
	if got := len(input.CandidateBundle.Candidates); got != rootPackageCount+2 {
		t.Fatalf("Architecture candidates = %d, want %d root packages plus root file/symbol", got, rootPackageCount+2)
	}
	if got := len(input.CandidateBundle.Relations); got != rootEdgeCount {
		t.Fatalf("Architecture relations = %d, want %d retained root edges", got, rootEdgeCount)
	}
	if len(input.CandidateBundle.BehaviorAnchors) != 1 ||
		input.CandidateBundle.BehaviorAnchors[0].ID != rootAnchor.ID {
		t.Fatalf("scoped behavior anchors = %#v, fixture anchor re-entered Architecture scope", input.CandidateBundle.BehaviorAnchors)
	}
	for _, candidate := range input.CandidateBundle.Candidates {
		if strings.Contains(candidate.Name, "internal/endtoend/testdata") ||
			strings.Contains(candidate.Name, "endtoend/case000") {
			t.Fatalf("fixture grounding candidate re-entered Architecture scope: %#v", candidate)
		}
	}
	if len(graph.Packages) != rootPackageCount+fixturePackageCount ||
		len(graph.PackageEdges) != rootEdgeCount+3 {
		t.Fatalf("complete saved graph was mutated: packages=%d edges=%d", len(graph.Packages), len(graph.PackageEdges))
	}
	wantScope := ArchitectureProductScope{
		ObservedModules: 2, RetainedModules: 1, OmittedModules: 1,
		ObservedPackages: rootPackageCount + fixturePackageCount,
		RetainedPackages: rootPackageCount,
		ObservedEdges:    rootEdgeCount + 3,
		RetainedEdges:    rootEdgeCount,
	}
	if got := DescribeArchitectureProductScope(graph); !reflect.DeepEqual(got, wantScope) {
		t.Fatalf("Architecture scope = %#v, want %#v", got, wantScope)
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

func TestBuildArchitectureCanvasInputKeepsExactPackageWithoutImportEdge(t *testing.T) {
	t.Parallel()

	input, err := BuildArchitectureCanvasInput(&ReportData{RepositoryGraph: &RepositoryGraph{
		Version: 2,
		Modules: []ModuleInfo{{ID: "module-project", Path: "github.com/example/project/v2"}},
		Packages: []PackageInfo{{
			CanonicalPath: "github.com/example/project/v2/internal/server", Name: "server",
			ModuleID: "module-project", ModulePath: "github.com/example/project/v2",
			Dir: "internal/server", ModuleRelativeDir: "internal/server",
			DisplayPath: "internal/server", Locality: "local",
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range input.CandidateBundle.Candidates {
		if candidate.ID.Kind != componentmap.MemberPackage {
			continue
		}
		if candidate.Name != "internal/server" || len(candidate.Facts) == 0 ||
			candidate.Facts[0].Value != "github.com/example/project/v2/internal/server" {
			t.Fatalf("exact package candidate = %#v", candidate)
		}
		return
	}
	t.Fatal("exact package without an import edge was omitted")
}

func TestBuildArchitectureCanvasInputKeepsRootAnchorWhenPackageDirIsDot(t *testing.T) {
	t.Parallel()

	scenario := architectureGroundingScenario{ID: "go:test", GOOS: "linux", GOARCH: "amd64"}
	producer := evidence.Provenance{Provider: "go_ssa", Version: "fixture", Operation: "root_entry"}
	rootAnchor := architectureGroundingTestAnchor(
		"root-entry", componentmap.AnchorProcessEntry,
		"main.go", 7, "example.com/product.main", scenario, producer,
	)
	input, err := BuildArchitectureCanvasInput(&ReportData{
		RepositoryGraph: &RepositoryGraph{
			Version: 2,
			Modules: []ModuleInfo{{ID: "root", Path: "example.com/product", Dir: "."}},
			Packages: []PackageInfo{{
				CanonicalPath: "example.com/product", Name: "main",
				ModuleID: "root", ModulePath: "example.com/product", Dir: ".",
				DisplayPath: ".", Locality: "local",
			}},
		},
		ArchitectureGrounding: &ArchitectureGrounding{
			RepositoryArchetype: ArchitectureArchetype{Selected: componentmap.ArchetypeApplication},
			GroundingMode:       componentmap.GroundingMixed,
			BehaviorAnchors:     []ArchitectureBehaviorAnchor{rootAnchor},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(input.CandidateBundle.BehaviorAnchors) != 1 ||
		input.CandidateBundle.BehaviorAnchors[0].ID != rootAnchor.ID {
		t.Fatalf("root behavior anchors = %#v", input.CandidateBundle.BehaviorAnchors)
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
	bindArchitectureTestDirection(data, "surface-backup")

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
	if canvas.Title != "Evidence-backed architecture skeleton" || len(canvas.StructuralFacts) != 2 || len(canvas.StructuralEdges) != 2 {
		t.Fatalf("canvas grounding/edges = %q / %#v / %#v", canvas.Title, canvas.StructuralFacts, canvas.StructuralEdges)
	}
	kinds := make(map[componentmap.StructuralRelationKind]bool, len(canvas.StructuralEdges))
	for _, edge := range canvas.StructuralEdges {
		kinds[edge.Witness.Kind] = true
	}
	if !kinds[componentmap.StructuralRelationBehaviorHandoff] || !kinds[componentmap.StructuralRelationPackageImport] {
		t.Fatalf("grounded structural edge kinds = %#v", canvas.StructuralEdges)
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
	proofMode := componentmap.AnchorProofCallTarget
	if kind == componentmap.AnchorProcessEntry {
		proofMode = componentmap.AnchorProofProcessEntry
	}
	return ArchitectureBehaviorAnchor{
		ID: id, Kind: kind, ProofMode: proofMode, Label: symbol, Location: location, Scenario: scenario,
		Producer: producer, Certainty: evidence.CertaintyStatic,
		AssociatedMembers: []ArchitectureAnchorMember{{ID: symbol, Package: symbol, Name: symbol, Location: location}},
		Limitations:       []string{"Static fixture evidence; runtime execution is not observed."},
	}
}

func TestBuildArchitectureCanvasInputDoesNotPublishCandidateDirectionProofFrontiers(t *testing.T) {
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
	bindArchitectureTestDirection(data, "surface-backup")

	input, err := BuildArchitectureCanvasInput(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(input.CandidateBundle.AnchorBindings) != 0 || len(input.CandidateBundle.Flows) != 0 ||
		len(input.Flows) != 0 {
		t.Fatalf("CandidateDirection proof became an architecture overlay: %#v", input)
	}

	canvas, err := ProjectArchitectureCanvas(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(canvas.Flows) != 0 || len(canvas.FlowEdges) != 0 || len(canvas.Frontiers) != 0 {
		t.Fatalf("CandidateDirection proof leaked into projected architecture: %#v", canvas)
	}
}

func architectureBuildTestProof() flowproof.Proof {
	return flowproof.Proof{
		Version: flowproof.Version, ID: "backup", Archetype: flowproof.ArchetypeCLI,
		Goal: "save repository data", Command: "backup", SeedSurfaceID: "surface-backup",
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

func bindArchitectureTestDirection(data *ReportData, surfaceID string) {
	for index := range data.CandidateDirections {
		if data.CandidateDirections[index].LocalProof != nil &&
			data.CandidateDirections[index].LocalProof.Proof.SeedSurfaceID == surfaceID {
			data.CandidateDirections[index].CandidateBasis = flowexplain.CandidateBasisLocalEntrypoint
		}
	}
	if data.DiscoveredSurfaces == nil {
		data.DiscoveredSurfaces = &DiscoveredSurfaces{}
	}
	data.DiscoveredSurfaces.Triggers = append(data.DiscoveredSurfaces.Triggers, DiscoveredTrigger{
		ID: surfaceID, Kind: "process_entry", Resolution: "exact",
		SurfaceRole: SurfaceRoleEntrySurface,
	})
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

func TestBuildArchitectureCanvasInputOmitsEntirelyUnboundFlowOverlay(t *testing.T) {
	t.Parallel()

	input, err := BuildArchitectureCanvasInput(&ReportData{
		RepositoryGraph: &RepositoryGraph{
			Modules: []ModuleInfo{{Path: "example.com/project"}},
			PackageEdges: []EdgeInfo{{
				From: "example.com/project/cmd", To: "example.com/project/internal/repo",
			}},
		},
		CandidateDirections: []CandidateDirection{{
			ID: "unknown", Name: "Unknown", CandidateBasis: flowexplain.CandidateBasisModelOrientation,
			LocalProof: &flowproof.Session{Version: flowproof.SessionVersion, Proof: flowproof.Proof{
				Version: flowproof.Version, ID: "unknown", Archetype: flowproof.ArchetypeCLI,
				SeedSurfaceID: "heuristic-surface",
				Anchors:       []flowproof.Anchor{{ID: "opaque", Kind: flowproof.AnchorOperation, Label: "opaque"}},
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Flows) != 0 || len(input.CandidateBundle.Flows) != 0 {
		t.Fatalf("unbound flow was published: %#v / %#v", input.Flows, input.CandidateBundle.Flows)
	}
	if len(input.CandidateBundle.AnchorBindings) != 0 {
		t.Fatalf("bindings = %#v, want no invented member for locationless anchor", input.CandidateBundle.AnchorBindings)
	}
	canvas, err := ProjectArchitectureCanvas(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(canvas.Flows) != 0 || len(canvas.FlowEdges) != 0 {
		t.Fatalf("unbound flow overlay = %#v / %#v", canvas.Flows, canvas.FlowEdges)
	}
}
