package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

func TestSemanticDiscoveryReportFormatVersion(t *testing.T) {
	if CurrentFormatVersion != 31 {
		t.Fatalf("CurrentFormatVersion = %d, want 31 for theme Study publication", CurrentFormatVersion)
	}
}

func TestBuildSemanticDiscoveryBundleUsesOnlyBoundedSavedFacts(t *testing.T) {
	data := semanticSearchTestReport()
	data.RepositoryGraph = &RepositoryGraph{PackageEdges: []EdgeInfo{{
		From: "github.com/example/repomap/internal/report",
		To:   "github.com/example/repomap/internal/semanticdiscovery",
	}}}
	data.sourceSignals = []SourceSignal{
		{Path: "internal/report/report.go", Line: 12, Category: "declaration", Reason: "report data boundary"},
		{Path: "internal/report/report.go", Line: 13, Category: "background_loop", Reason: "context cancellation check", Snippet: "<-ctx.Done()"},
		{Path: "ignored/secret.go", Line: 2, Category: "declaration", Reason: "must not become navigation evidence"},
	}
	data.externalImports = []externalImportUsage{{
		ImportPath: "golang.org/x/tools/go/packages", UsedByCount: 2,
	}}
	data.ArchitectureCanvas.Surfaces = append(data.ArchitectureCanvas.Surfaces, ArchitectureSurface{
		ID:       "surface-model-trace",
		Name:     "Model-authored trace start",
		Source:   surfaceSourceTraceStart,
		Category: surfaceCategoryApplication,
	})
	data.OpenablePaths = append(
		data.OpenablePaths,
		"internal/report/semantic_discovery_test.go",
		"docs/semantic-discovery.md",
	)
	data.Flows[0].BundleTests = []FileItem{{
		Path: "internal/report/semantic_discovery_test.go", Reason: "saved flow test",
	}}
	data.Flows[0].BundleDocs = []FileItem{{
		Path: "docs/semantic-discovery.md", Reason: "saved flow documentation",
	}}
	data.Flows[0].BundlePackages = []string{"github.com/example/repomap/internal/report"}
	data.Flows[0].BundleEdges = []EdgeInfo{{
		From: "github.com/example/repomap/cmd/repomap",
		To:   "github.com/example/repomap/internal/report",
	}}

	bundle, err := BuildSemanticDiscoveryBundle(data)
	if err != nil {
		t.Fatalf("BuildSemanticDiscoveryBundle: %v", err)
	}
	if err := bundle.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if bundle.Version != semanticdiscovery.BundleVersion || bundle.RepoName != "repomap" {
		t.Fatalf("bundle header = version %d repo %q", bundle.Version, bundle.RepoName)
	}
	if len(bundle.Facts) == 0 || len(bundle.Facts) > maxSemanticDiscoveryFacts {
		t.Fatalf("fact count = %d", len(bundle.Facts))
	}

	kinds := make(map[semanticdiscovery.FactKind]bool)
	behaviorSignal := false
	dependencyLimitation := false
	savedTest := false
	savedDoc := false
	savedFlowEdge := false
	modelProseInFacts := false
	sourceGroupsByKind := make(map[semanticdiscovery.FactKind]string)
	for _, fact := range bundle.Facts {
		kinds[fact.Kind] = true
		if !strings.HasPrefix(fact.ID, "sf-") || !strings.HasPrefix(fact.SourceGroup, "sg-") {
			t.Fatalf("fact identities are not opaque: id=%q group=%q", fact.ID, fact.SourceGroup)
		}
		if existing := sourceGroupsByKind[fact.Kind]; existing != "" && existing != fact.SourceGroup {
			t.Fatalf("fact kind %q split one extractor family into source groups %q and %q", fact.Kind, existing, fact.SourceGroup)
		}
		sourceGroupsByKind[fact.Kind] = fact.SourceGroup
		for _, reference := range fact.Evidence {
			if !stringSliceContains(data.OpenablePaths, reference.Path) {
				t.Fatalf("evidence path %q is outside the report allowlist", reference.Path)
			}
		}
		if fact.Kind == semanticdiscovery.FactSourceSignal &&
			strings.Contains(fact.Statement, "context cancellation") &&
			stringSliceContains(capabilityStrings(fact.Capabilities), string(semanticdiscovery.CapabilityBehavior)) {
			behaviorSignal = true
		}
		if fact.Kind == semanticdiscovery.FactDependency &&
			strings.Contains(fact.Statement, "golang.org/x/tools/go/packages") &&
			stringSliceContains(capabilityStrings(fact.Capabilities), string(semanticdiscovery.CapabilityLimitation)) {
			dependencyLimitation = true
		}
		if fact.Kind == semanticdiscovery.FactTestReference &&
			strings.Contains(fact.Statement, "allowlisted test reference") {
			savedTest = true
		}
		if fact.Kind == semanticdiscovery.FactSourceSignal &&
			strings.Contains(fact.Statement, "allowlisted documentation reference") {
			savedDoc = true
		}
		if fact.Kind == semanticdiscovery.FactPackageImport &&
			strings.Contains(fact.Statement, "does not establish runtime order") {
			savedFlowEdge = true
		}
		for _, modelOnly := range []string{
			data.ProjectGuess,
			data.ArchitectureCanvas.Components[0].Description,
			data.ArchitectureCanvas.Flows[0].MentalModel,
			data.ArchitectureCanvas.Flows[0].Name,
			data.GuidedTour.Steps[0].Explanation,
			data.ImportantDomainWords[0].Guess,
			data.ArchitectureCanvas.Surfaces[1].Name,
			data.Warnings[0],
		} {
			if strings.Contains(fact.Statement, modelOnly) {
				modelProseInFacts = true
			}
		}
	}
	if !behaviorSignal {
		t.Fatal("exact saved cancellation signal did not retain behavior capability")
	}
	if !dependencyLimitation {
		t.Fatal("saved external import aggregate did not retain its ownership/API limitation")
	}
	if !savedTest || !savedDoc || !savedFlowEdge {
		t.Fatalf("saved focused bundle facts = test:%t doc:%t edge:%t", savedTest, savedDoc, savedFlowEdge)
	}
	if modelProseInFacts {
		t.Fatal("saved model-authored prose was promoted to authoritative facts")
	}
	for _, kind := range []semanticdiscovery.FactKind{
		semanticdiscovery.FactFlow,
		semanticdiscovery.FactFlowStep,
		semanticdiscovery.FactRuntimeSurface,
		semanticdiscovery.FactPackageImport,
		semanticdiscovery.FactSourceSignal,
	} {
		if !kinds[kind] {
			t.Errorf("bundle is missing saved fact kind %q", kind)
		}
	}
	contextKinds := make(map[semanticdiscovery.PlannerContextKind]bool)
	for _, item := range bundle.PlannerContext {
		contextKinds[item.Kind] = true
	}
	for _, kind := range []semanticdiscovery.PlannerContextKind{
		semanticdiscovery.PlannerContextOrientation,
		semanticdiscovery.PlannerContextComponent,
		semanticdiscovery.PlannerContextFlow,
		semanticdiscovery.PlannerContextGuidedTour,
		semanticdiscovery.PlannerContextVocabulary,
		semanticdiscovery.PlannerContextLimitation,
	} {
		if !contextKinds[kind] {
			t.Errorf("bundle is missing planner context kind %q", kind)
		}
	}
	opportunityPrompt, err := semanticdiscovery.BuildOpportunityPrompt(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(opportunityPrompt.User, data.ProjectGuess) {
		t.Fatal("opportunity scan did not receive bounded editorial planner context")
	}
	var localSupport semanticdiscovery.Fact
	for _, fact := range bundle.Facts {
		if fact.Kind == semanticdiscovery.FactFlowStep {
			localSupport = fact
			break
		}
	}
	proposal, _ := semanticdiscovery.NormalizeOpportunityProposal(bundle, semanticdiscovery.OpportunityProposal{
		Version: semanticdiscovery.OpportunityProposalVersion,
		Candidates: []semanticdiscovery.OpportunityCandidate{{
			Kind: semanticdiscovery.ArtifactMechanism, Title: "Local proof",
			QuestionAnswered: "What does the local proof establish?",
			SupportIDs:       []string{localSupport.ID}, ExpectedValue: semanticdiscovery.ExpectedValueMedium,
			Confidence: semanticdiscovery.ConfidenceMedium,
		}},
	})
	tasks, err := semanticdiscovery.PlanLeafTasks(bundle, proposal.Candidates)
	if err != nil {
		t.Fatal(err)
	}
	leafPrompt, err := semanticdiscovery.BuildLeafPrompt(tasks[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(leafPrompt.User, data.ProjectGuess) ||
		strings.Contains(leafPrompt.User, data.GuidedTour.Steps[0].Explanation) {
		t.Fatal("leaf prompt received model-authored planner context")
	}

	encoded, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "ignored/secret.go") {
		t.Fatalf("non-allowlisted path leaked into bundle: %s", encoded)
	}

	again, err := BuildSemanticDiscoveryBundle(data)
	if err != nil {
		t.Fatalf("second BuildSemanticDiscoveryBundle: %v", err)
	}
	if !reflect.DeepEqual(bundle, again) {
		t.Fatal("semantic discovery bundle is not deterministic")
	}
}

func TestSelectSemanticDiscoveryFactsReservesProductionRoles(t *testing.T) {
	facts := []semanticdiscovery.Fact{
		{
			ID: "fixture", Kind: semanticdiscovery.FactSourceSignal,
			Capabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityBehavior},
			Evidence:     []semanticdiscovery.EvidenceRef{{Path: "testdata/fixture/main.go"}},
		},
		{
			ID: "preview", Kind: semanticdiscovery.FactSourceSignal,
			Capabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityBehavior},
			Evidence:     []semanticdiscovery.EvidenceRef{{Path: "cmd/app-preview/main.go"}},
		},
		{
			ID: "entry", Kind: semanticdiscovery.FactFlowStep,
			Capabilities: []semanticdiscovery.Capability{
				semanticdiscovery.CapabilityBehavior,
				semanticdiscovery.CapabilityEntry,
			},
			Evidence: []semanticdiscovery.EvidenceRef{{Path: "cmd/app/main.go"}},
		},
		{
			ID: "core", Kind: semanticdiscovery.FactFlowStep,
			Capabilities: []semanticdiscovery.Capability{
				semanticdiscovery.CapabilityBehavior,
				semanticdiscovery.CapabilityDirectCall,
			},
			Evidence: []semanticdiscovery.EvidenceRef{{Path: "internal/scheduler/run.go"}},
		},
		{
			ID: "effect", Kind: semanticdiscovery.FactFlowStep,
			Capabilities: []semanticdiscovery.Capability{
				semanticdiscovery.CapabilityBehavior,
				semanticdiscovery.CapabilityOutputEffect,
			},
			Evidence: []semanticdiscovery.EvidenceRef{{Path: "internal/storage/write.go"}},
		},
	}

	selected := selectSemanticDiscoveryFacts(facts, 3)
	got := make(map[string]bool, len(selected))
	for _, fact := range selected {
		got[fact.ID] = true
	}
	for _, required := range []string{"entry", "core", "effect"} {
		if !got[required] {
			t.Fatalf("selected facts = %#v, missing production role %q", selected, required)
		}
	}
	if got["fixture"] || got["preview"] {
		t.Fatalf("auxiliary fact consumed a production slot: %#v", selected)
	}
}

func capabilityStrings(values []semanticdiscovery.Capability) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, string(value))
	}
	return result
}

func TestBuildSemanticDiscoveryBundleRejectsEmptyReport(t *testing.T) {
	if _, err := BuildSemanticDiscoveryBundle(nil); err == nil {
		t.Fatal("nil report succeeded")
	}
	if _, err := BuildSemanticDiscoveryBundle(&ReportData{}); err == nil {
		t.Fatal("empty report succeeded")
	}
}

func TestBuildSemanticDiscoveryBundleAddsBoundedSupplementalFactsWithExistingOwner(t *testing.T) {
	data := semanticSearchTestReport()
	data.RepositoryGraph = &RepositoryGraph{Packages: []PackageInfo{{
		CanonicalPath: "github.com/example/repomap/internal/report",
		Dir:           "internal/report",
		Files:         []string{"internal/report/report.go"},
	}}}
	data.ArchitectureCanvas.Components[0].Members = append(
		data.ArchitectureCanvas.Components[0].Members,
		componentmap.Candidate{
			ID:   componentmap.MemberID{Kind: componentmap.MemberPackage, Value: "member-report"},
			Name: "internal/report",
			Facts: []componentmap.LocalFact{{
				Kind:  componentmap.FactDeclaration,
				Value: "github.com/example/repomap/internal/report",
			}},
		},
	)
	data.SemanticSupplementalFacts = []semanticdiscovery.Fact{{
		ID: "gmf-supplemental-01", Kind: semanticdiscovery.FactSourceSignal,
		Statement:    "The bounded report step writes one generated representation to the client.",
		Keywords:     []string{"answer_aspect:response_output"},
		SourceGroup:  "gmsg-report-output",
		Capabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityStatic},
		Scope:        semanticdiscovery.FactScopeLocal,
		Evidence: []semanticdiscovery.EvidenceRef{{
			ID: "gme-report-output", Kind: "probe", Label: "bounded output",
			Path: "internal/report/report.go", Line: 12,
		}},
	}}

	bundle, err := BuildSemanticDiscoveryBundle(data)
	if err != nil {
		t.Fatal(err)
	}
	var found semanticdiscovery.Fact
	for _, fact := range bundle.Facts {
		if fact.ID == "gmf-supplemental-01" {
			found = fact
			break
		}
	}
	if found.ID == "" {
		t.Fatal("supplemental fact was not retained")
	}
	if !reflect.DeepEqual(found.Focus.ComponentIDs, []string{"component-analysis"}) {
		t.Fatalf("supplemental focus = %#v", found.Focus)
	}
}

func TestReplaySavedSemanticArtifactsValidatesCurrentFacts(t *testing.T) {
	data := semanticSearchTestReport()
	data.SemanticArtifacts = nil
	bundle, err := BuildSemanticDiscoveryBundle(data)
	if err != nil {
		t.Fatal(err)
	}
	var support semanticdiscovery.Fact
	for _, fact := range bundle.Facts {
		if fact.Kind == semanticdiscovery.FactFlowStep {
			support = fact
			break
		}
	}
	if support.ID == "" {
		t.Fatal("local flow-proof fact not found")
	}
	observationText := "The saved exact flow proof identifies a collection step"

	opportunity, normalization := semanticdiscovery.NormalizeOpportunityProposal(
		bundle,
		semanticdiscovery.OpportunityProposal{
			Version: semanticdiscovery.OpportunityProposalVersion,
			Candidates: []semanticdiscovery.OpportunityCandidate{{
				Kind:             semanticdiscovery.ArtifactMechanism,
				Title:            "Saved local proof",
				QuestionAnswered: "What does the local proof establish?",
				SupportIDs:       []string{support.ID},
				ExpectedValue:    semanticdiscovery.ExpectedValueHigh,
				Confidence:       semanticdiscovery.ConfidenceHigh,
			}},
		},
	)
	if len(normalization.Issues) != 0 || len(opportunity.Candidates) != 1 {
		t.Fatalf("normalization = %#v, opportunity = %#v", normalization, opportunity)
	}
	selected, err := semanticdiscovery.SelectOpportunities(bundle, opportunity, 1)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := semanticdiscovery.PlanLeafTasks(bundle, selected)
	if err != nil {
		t.Fatal(err)
	}
	leaf := semanticdiscovery.LeafArtifact{
		Version:     semanticdiscovery.LeafArtifactVersion,
		TaskID:      tasks[0].ID,
		CandidateID: tasks[0].Candidate.ID,
		Status:      semanticdiscovery.LeafStatusUsable,
		Observations: []semanticdiscovery.LeafObservation{{
			Text:       observationText,
			SupportIDs: []string{support.ID},
		}},
		CandidateConnection: semanticdiscovery.LeafCandidateConnection{
			CandidateID: tasks[0].Candidate.ID,
			Relation:    "needs_combination",
			Explanation: "This bounded observation may need combination.",
			SupportIDs:  []string{support.ID},
		},
	}
	result := semanticdiscovery.LeafResult{Task: tasks[0], Artifact: leaf}
	fanIn := semanticdiscovery.FanInArtifact{
		Version: semanticdiscovery.FanInArtifactVersion,
		Artifacts: []semanticdiscovery.ArtifactProposal{{
			CandidateID: tasks[0].Candidate.ID,
			Verdict:     semanticdiscovery.VerdictSupported,
			Title:       "Saved local proof",
			Summary:     observationText,
			Claims: []semanticdiscovery.ProposedClaim{{
				Title:      "Saved local evidence",
				Text:       observationText,
				Basis:      semanticdiscovery.ClaimDirect,
				SupportIDs: []string{support.ID},
				ObservationRefs: []semanticdiscovery.ObservationRef{{
					TaskID: tasks[0].ID, ObservationIndex: 0,
				}},
			}},
		}},
	}
	raw, err := semanticdiscovery.EncodeRecord(bundle, opportunity, selected, []semanticdiscovery.LeafResult{result}, fanIn)
	if err != nil {
		t.Fatalf("EncodeRecord: %v", err)
	}
	path := filepath.Join(t.TempDir(), semanticDiscoveryRecordFile)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	if warning := replaySavedSemanticArtifacts(data, path); warning != "" {
		t.Fatalf("replay warning = %q", warning)
	}
	if len(data.SemanticArtifacts) != 1 || data.SemanticArtifacts[0].Question != "What does the local proof establish?" {
		t.Fatalf("replayed artifacts = %#v", data.SemanticArtifacts)
	}

	data.ProjectGuess = "a changed saved purpose"
	if warning := replaySavedSemanticArtifacts(data, path); !strings.Contains(warning, "stale or invalid") {
		t.Fatalf("stale replay warning = %q", warning)
	}
	if len(data.SemanticArtifacts) != 0 {
		t.Fatalf("stale replay retained artifacts: %#v", data.SemanticArtifacts)
	}
}

func TestReplaySavedSemanticArtifactsMissingRecordIsOptional(t *testing.T) {
	data := semanticSearchTestReport()
	if warning := replaySavedSemanticArtifacts(data, filepath.Join(t.TempDir(), semanticDiscoveryRecordFile)); warning != "" {
		t.Fatalf("missing record warning = %q", warning)
	}
	if len(data.SemanticArtifacts) != 0 {
		t.Fatalf("missing record retained artifacts: %#v", data.SemanticArtifacts)
	}
}
