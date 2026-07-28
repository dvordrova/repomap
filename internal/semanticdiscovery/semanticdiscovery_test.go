package semanticdiscovery

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

const (
	testFactStatic    = "fact-static-import"
	testFactLoad      = "fact-load-metadata"
	testFactSequence  = "fact-report-sequence"
	testFactValidate  = "fact-local-validation"
	testFactHTTP      = "fact-http-request"
	testFactGap       = "fact-runtime-gap"
	testGroupValidate = "group-validation"
)

func TestBundleEvidenceIdentityUsesLocalNavigationNotLabel(t *testing.T) {
	t.Parallel()

	shared := EvidenceRef{
		ID: "evidence-shared-surface", Kind: "runtime_surface", Label: "HTTP server",
		Path: "contrib/raftexample/main.go", Line: 24, Column: 6,
	}
	bundle := Bundle{
		Version:  BundleVersion,
		RepoName: "sample",
		Facts: []Fact{
			{
				ID: "fact-http-server", Kind: FactSourceSignal,
				Statement:    "A bounded runtime surface identifies the server.",
				SourceGroup:  "group-http-server",
				Capabilities: []Capability{CapabilityStatic},
				Scope:        FactScopeLocal,
				Evidence:     []EvidenceRef{shared},
			},
			{
				ID: "fact-raft-route", Kind: FactSourceSignal,
				Statement:    "A bounded runtime surface identifies a route.",
				SourceGroup:  "group-raft-route",
				Capabilities: []Capability{CapabilityStatic},
				Scope:        FactScopeLocal,
				Evidence: []EvidenceRef{{
					ID: shared.ID, Kind: shared.Kind, Label: "Raft stream route",
					Path: shared.Path, Line: shared.Line, Column: shared.Column,
				}},
			},
		},
	}
	if _, _, err := BundleHash(bundle); err != nil {
		t.Fatalf("BundleHash() rejected contextual labels on shared navigation: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*EvidenceRef)
	}{
		{name: "kind", mutate: func(reference *EvidenceRef) { reference.Kind = "flow_step" }},
		{name: "path", mutate: func(reference *EvidenceRef) { reference.Path = "internal/other.go" }},
		{name: "line", mutate: func(reference *EvidenceRef) { reference.Line++ }},
		{name: "column", mutate: func(reference *EvidenceRef) { reference.Column++ }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			conflicting := cloneJSON(t, bundle)
			test.mutate(&conflicting.Facts[1].Evidence[0])
			if _, _, err := BundleHash(conflicting); err == nil ||
				!strings.Contains(err.Error(), "conflicting local navigation") {
				t.Fatalf("BundleHash() conflict error = %v", err)
			}
		})
	}
}

func TestOpportunityNormalizationSelectionAndStrictJSON(t *testing.T) {
	bundle := semanticTestBundle()
	base := semanticTestOpportunity(bundle)
	duplicate := base.Candidates[0]
	duplicate.ID = ""
	duplicate.Title = "Another validation title"
	unknownOnly := base.Candidates[1]
	unknownOnly.ID = ""
	unknownOnly.SupportIDs = []string{"unknown-fact"}
	mixed := base.Candidates[2]
	mixed.ID = ""
	mixed.SupportIDs = append(mixed.SupportIDs, "unknown-fact")

	normalized, report := NormalizeOpportunityProposal(bundle, OpportunityProposal{
		Version: OpportunityProposalVersion,
		Candidates: []OpportunityCandidate{
			withoutCandidateID(base.Candidates[0]),
			withoutCandidateID(base.Candidates[1]),
			withoutCandidateID(base.Candidates[2]),
			duplicate,
			unknownOnly,
			mixed,
		},
	})
	if err := ValidateOpportunityProposal(bundle, normalized); err != nil {
		t.Fatalf("ValidateOpportunityProposal() error = %v", err)
	}
	if len(normalized.Candidates) != 3 {
		t.Fatalf("normalized candidates = %d, want 3", len(normalized.Candidates))
	}
	if len(report.Issues) < 3 {
		t.Fatalf("normalization issues = %#v, want unknown and duplicate diagnostics", report.Issues)
	}
	for _, candidate := range normalized.Candidates {
		if slices.Contains(candidate.SupportIDs, "unknown-fact") {
			t.Fatalf("unknown support survived normalization: %#v", candidate)
		}
	}

	selected, err := SelectOpportunities(bundle, normalized, 3)
	if err != nil {
		t.Fatalf("SelectOpportunities() error = %v", err)
	}
	kinds := map[ArtifactKind]bool{}
	for _, candidate := range selected {
		kinds[candidate.Kind] = true
	}
	if len(kinds) != 3 {
		t.Fatalf("selected kinds = %#v, want three distinct kinds", kinds)
	}

	if _, err := ParseOpportunityProposal([]byte(`{"version":1,"candidates":[],"extra":true}`)); err == nil {
		t.Fatal("ParseOpportunityProposal() accepted an unknown field")
	}
	if _, err := ParseOpportunityProposal([]byte(`{"version":1,"candidates":[]} {}`)); err == nil {
		t.Fatal("ParseOpportunityProposal() accepted trailing JSON")
	}
}

func TestOpportunityProductIntentIsLocallyValidated(t *testing.T) {
	t.Parallel()

	bundle := semanticTestBundle()
	candidate := OpportunityCandidate{
		Kind:             ArtifactMechanism,
		Title:            "Request to validated report",
		QuestionAnswered: "How does a report request become validated output?",
		SupportIDs:       []string{testFactHTTP, testFactValidate, testFactSequence},
		ExpectedValue:    ExpectedValueHigh,
		Confidence:       ConfidenceHigh,
		CapabilityContract: &CapabilityContract{
			RequiredCapabilities:  []Capability{CapabilityBehavior, CapabilitySequence},
			AvailableCapabilities: []Capability{CapabilityBehavior, CapabilitySequence},
			MissingCapabilities:   []Capability{},
			Resolution:            CapabilityResolutionReady,
		},
		ProductIntent: &OpportunityProductIntent{
			OpportunityKind:  OpportunityKindCentralBehavior,
			TargetUserJob:    OpportunityUserJobFirstContact,
			CentralAnchorIDs: []string{testFactHTTP, testFactValidate, testFactSequence},
			ExpectedPath: OpportunityExpectedPath{
				InputTrigger: OpportunityExpectation{
					Description: "An HTTP request enters the report server.",
					SupportIDs:  []string{testFactHTTP}, RequiredCapabilities: []Capability{CapabilityBehavior},
				},
				CoreWork: OpportunityExpectation{
					Description: "Local validation checks evidence identifiers.",
					SupportIDs:  []string{testFactValidate}, RequiredCapabilities: []Capability{CapabilityBehavior},
				},
				ObservableEffect: OpportunityExpectation{
					Description:          "The validated data reaches report rendering.",
					SupportIDs:           []string{testFactSequence},
					RequiredCapabilities: []Capability{CapabilityBehavior, CapabilitySequence},
				},
			},
			ArchitectureAreaAnchorIDs: []string{testFactValidate, testFactSequence},
			OnboardingRationale:       "This crosses the public boundary, core check, and visible result.",
			InvestigationRationale:    "Each stage has bounded local evidence.",
			EstimatedCost:             OpportunityEstimatedCostLow,
			SearchQueries:             []string{"report validation", "как проверяется отчёт"},
		},
	}

	normalized, report := NormalizeOpportunityProposal(bundle, OpportunityProposal{
		Version: OpportunityProposalVersion, Candidates: []OpportunityCandidate{candidate},
	})
	if len(report.Issues) != 0 || len(normalized.Candidates) != 1 {
		t.Fatalf("valid product intent normalization = %#v, %#v", normalized, report)
	}
	if err := ValidateOpportunityProposal(bundle, normalized); err != nil {
		t.Fatalf("ValidateOpportunityProposal() error = %v", err)
	}

	tests := []struct {
		name      string
		discarded string
		mutate    func(*OpportunityProductIntent)
	}{
		{
			name:      "repository-bearing description",
			discarded: "etcdmain.main",
			mutate: func(intent *OpportunityProductIntent) {
				intent.ExpectedPath.InputTrigger.Description =
					"The server entry delegates to etcdmain.main."
			},
		},
		{
			name: "missing required capability",
			mutate: func(intent *OpportunityProductIntent) {
				intent.ExpectedPath.InputTrigger.RequiredCapabilities = nil
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invalid := candidate
			invalid.ProductIntent = cloneJSON(t, candidate.ProductIntent)
			test.mutate(invalid.ProductIntent)
			retained, invalidReport := NormalizeOpportunityProposal(bundle, OpportunityProposal{
				Version: OpportunityProposalVersion, Candidates: []OpportunityCandidate{invalid},
			})
			if len(retained.Candidates) != 1 || retained.Candidates[0].ProductIntent != nil ||
				len(invalidReport.Issues) != 1 ||
				invalidReport.Issues[0].Code != "invalid_product_intent" {
				t.Fatalf("invalid product intent handling = %#v, %#v", retained, invalidReport)
			}
			if err := ValidateOpportunityProposal(bundle, retained); err != nil {
				t.Fatalf("retained candidate is invalid: %v", err)
			}
			encoded, err := json.Marshal(retained)
			if err != nil {
				t.Fatal(err)
			}
			reportEncoded, err := json.Marshal(invalidReport)
			if err != nil {
				t.Fatal(err)
			}
			if test.discarded != "" &&
				(strings.Contains(string(encoded), test.discarded) ||
					strings.Contains(string(reportEncoded), test.discarded)) {
				t.Fatalf("discarded product intent leaked: normalized=%s report=%s",
					encoded, reportEncoded)
			}
		})
	}
}

func TestOpportunityCapabilityCoverageIsDerivedLocally(t *testing.T) {
	t.Parallel()

	bundle := semanticTestBundle()
	candidate := OpportunityCandidate{
		Kind: ArtifactMechanism, Title: "Validated report path",
		QuestionAnswered: "How does a request become a validated report?",
		SupportIDs:       []string{testFactHTTP},
		ExpectedValue:    ExpectedValueHigh,
		Confidence:       ConfidenceMedium,
		CapabilityContract: &CapabilityContract{
			RequiredCapabilities:  []Capability{CapabilityBehavior, CapabilityOutputEffect},
			AvailableCapabilities: []Capability{CapabilityBehavior, CapabilityDataWrite},
			Resolution:            CapabilityResolutionReady,
		},
	}

	normalized, report := NormalizeOpportunityProposal(bundle, OpportunityProposal{
		Version: OpportunityProposalVersion, Candidates: []OpportunityCandidate{candidate},
	})
	if len(normalized.Candidates) != 1 || len(report.Issues) != 1 ||
		report.Issues[0].Code != "capability_contract_derived" {
		t.Fatalf("normalization = %#v, %#v", normalized, report)
	}
	contract := normalized.Candidates[0].CapabilityContract
	if contract == nil || contract.Resolution != CapabilityResolutionPartial ||
		len(contract.AvailableCapabilities) != 1 ||
		contract.AvailableCapabilities[0] != CapabilityBehavior ||
		len(contract.MissingCapabilities) != 1 ||
		contract.MissingCapabilities[0] != CapabilityOutputEffect {
		t.Fatalf("derived capability contract = %#v", contract)
	}
	if err := ValidateOpportunityProposal(bundle, normalized); err != nil {
		t.Fatalf("ValidateOpportunityProposal() error = %v", err)
	}
}

func TestOpportunityNormalizationRetainsMissingExpectedPathCapability(t *testing.T) {
	t.Parallel()

	bundle := semanticTestBundle()
	candidate := OpportunityCandidate{
		Kind: ArtifactMechanism, Title: "Request to validated report",
		QuestionAnswered: "How does a report request become validated output?",
		SupportIDs:       []string{testFactHTTP, testFactValidate, testFactSequence},
		ExpectedValue:    ExpectedValueHigh,
		Confidence:       ConfidenceHigh,
		CapabilityContract: &CapabilityContract{
			RequiredCapabilities:  []Capability{CapabilityEntry, CapabilityBehavior, CapabilitySequence},
			AvailableCapabilities: []Capability{CapabilityBehavior, CapabilitySequence},
			MissingCapabilities:   []Capability{CapabilityEntry},
			Resolution:            CapabilityResolutionPartial,
		},
		ProductIntent: &OpportunityProductIntent{
			OpportunityKind:  OpportunityKindCentralBehavior,
			TargetUserJob:    OpportunityUserJobFirstContact,
			CentralAnchorIDs: []string{testFactHTTP, testFactValidate, testFactSequence},
			ExpectedPath: OpportunityExpectedPath{
				InputTrigger: OpportunityExpectation{
					Description: "A request reaches the public boundary.",
					SupportIDs:  []string{testFactHTTP}, RequiredCapabilities: []Capability{CapabilityEntry},
				},
				CoreWork: OpportunityExpectation{
					Description: "Local validation checks evidence identifiers.",
					SupportIDs:  []string{testFactValidate}, RequiredCapabilities: []Capability{CapabilityBehavior},
				},
				ObservableEffect: OpportunityExpectation{
					Description:          "The validated data reaches report rendering.",
					SupportIDs:           []string{testFactSequence},
					RequiredCapabilities: []Capability{CapabilityBehavior, CapabilitySequence},
				},
			},
			ArchitectureAreaAnchorIDs: []string{testFactValidate, testFactSequence, testFactLoad},
			OnboardingRationale:       "This crosses a public boundary, core check, and visible result.",
			InvestigationRationale:    "The missing entry capability has a bounded local frontier.",
			EstimatedCost:             OpportunityEstimatedCostLow,
			SearchQueries:             []string{"report validation", "как проверяется отчёт"},
		},
	}

	normalized, report := NormalizeOpportunityProposal(bundle, OpportunityProposal{
		Version: OpportunityProposalVersion, Candidates: []OpportunityCandidate{candidate},
	})
	if len(normalized.Candidates) != 1 {
		t.Fatalf("candidate with an unsupported expectation binding was dropped: %#v", report)
	}
	if len(report.Issues) != 2 ||
		report.Issues[0].Code != "architecture_anchor_support_reduced" ||
		report.Issues[0].Detail != testFactLoad ||
		report.Issues[1].Code != "expected_path_support_reduced" ||
		report.Issues[1].Detail != "input_trigger: entry" {
		t.Fatalf("normalization issues = %#v", report.Issues)
	}
	got := normalized.Candidates[0]
	if len(got.ProductIntent.ExpectedPath.InputTrigger.SupportIDs) != 0 {
		t.Fatalf("unsupported input binding survived: %#v", got.ProductIntent.ExpectedPath.InputTrigger)
	}
	if !slices.Equal(got.ProductIntent.ExpectedPath.CoreWork.SupportIDs, []string{testFactValidate}) ||
		!slices.Equal(got.ProductIntent.ExpectedPath.ObservableEffect.SupportIDs, []string{testFactSequence}) {
		t.Fatalf("supported expectations changed: %#v", got.ProductIntent.ExpectedPath)
	}
	if !slices.Equal(
		got.ProductIntent.ArchitectureAreaAnchorIDs,
		[]string{testFactValidate, testFactSequence},
	) {
		t.Fatalf("architecture anchors were not reduced to candidate support: %#v",
			got.ProductIntent.ArchitectureAreaAnchorIDs)
	}
	if got.CapabilityContract.Resolution != CapabilityResolutionPartial ||
		!slices.Equal(got.CapabilityContract.MissingCapabilities, []Capability{CapabilityEntry}) {
		t.Fatalf("missing capability was not preserved: %#v", got.CapabilityContract)
	}
	if err := ValidateOpportunityProposal(bundle, normalized); err != nil {
		t.Fatalf("ValidateOpportunityProposal() error = %v", err)
	}
}

func TestBundleHashAndPromptProfilesAreCanonical(t *testing.T) {
	bundle := semanticTestBundle()
	shuffled := cloneJSON(t, bundle)
	slices.Reverse(shuffled.Facts)
	slices.Reverse(shuffled.PlannerContext)
	for index := range shuffled.Facts {
		slices.Reverse(shuffled.Facts[index].Keywords)
		slices.Reverse(shuffled.Facts[index].Capabilities)
	}
	leftHash, _, err := BundleHash(bundle)
	if err != nil {
		t.Fatalf("BundleHash() error = %v", err)
	}
	rightHash, _, err := BundleHash(shuffled)
	if err != nil {
		t.Fatalf("BundleHash(shuffled) error = %v", err)
	}
	if leftHash != rightHash {
		t.Fatalf("canonical bundle hashes differ: %q != %q", leftHash, rightHash)
	}

	opportunity := semanticTestOpportunity(bundle)
	selected, err := SelectOpportunities(bundle, opportunity, 3)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := PlanLeafTasks(bundle, selected)
	if err != nil {
		t.Fatal(err)
	}
	first, err := BuildLeafPrompt(tasks[0])
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildLeafPrompt(tasks[1])
	if err != nil {
		t.Fatal(err)
	}
	if first.System != second.System ||
		strings.Split(first.User, leafTaskMarker)[0] != strings.Split(second.User, leafTaskMarker)[0] {
		t.Fatal("leaf prompts do not preserve a stable common prefix")
	}
	if first.ThinkingProfile != ThinkingHigh {
		t.Fatalf("leaf thinking = %q, want high", first.ThinkingProfile)
	}
	if !strings.HasPrefix(first.ProgressLabel, "semantic leaf "+string(tasks[0].Candidate.Kind)+" ") ||
		first.ProgressLabel == second.ProgressLabel {
		t.Fatalf("leaf progress labels = %q and %q", first.ProgressLabel, second.ProgressLabel)
	}
	opportunityPrompt, err := BuildOpportunityPrompt(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if opportunityPrompt.ThinkingProfile != ThinkingMax {
		t.Fatalf("opportunity thinking = %q, want max", opportunityPrompt.ThinkingProfile)
	}
	if opportunityPrompt.ProgressLabel != "semantic opportunity scan" {
		t.Fatalf("opportunity progress label = %q", opportunityPrompt.ProgressLabel)
	}
	if opportunityPrompt.Version != OpportunityPromptVersion {
		t.Fatalf("opportunity prompt version = %q, want %q", opportunityPrompt.Version, OpportunityPromptVersion)
	}
	for _, want := range []string{
		"Propose central mechanism questions",
		`"kind": "mechanism"`,
		"Return between one and three candidates",
		"Every candidate kind must be mechanism",
		`"opportunity_kind": "central_behavior | question_path | extension_path | maintenance_boundary"`,
		"The first candidate must use opportunity_kind central_behavior",
	} {
		if !strings.Contains(opportunityPrompt.User, want) {
			t.Fatalf("opportunity prompt omitted %q", want)
		}
	}
	for _, unwanted := range []string{
		"at most 20 candidates",
		"dependency_usage",
		"repository_pattern",
		"contribution_guide",
		"go_learning",
		"repository_story",
	} {
		if strings.Contains(opportunityPrompt.User, unwanted) {
			t.Fatalf("opportunity prompt still requests mixed or oversized output %q", unwanted)
		}
	}
	if !strings.Contains(opportunityPrompt.User, "Existing orientation calls this a repository analyzer") {
		t.Fatal("opportunity prompt omitted planner-only editorial context")
	}
	for _, prompt := range []Prompt{opportunityPrompt, first, second} {
		if strings.Contains(prompt.User, "internal/analyzer") ||
			strings.Contains(prompt.User, "component-analysis") ||
			strings.Contains(prompt.User, "evidence-import") {
			t.Fatalf("provider prompt contains local navigation: %s", prompt.Version)
		}
	}

	leaves := semanticTestLeaves(t, bundle, selected)
	fanInPrompt, err := BuildFanInPrompt(bundle, leaves)
	if err != nil {
		t.Fatal(err)
	}
	if fanInPrompt.ThinkingProfile != ThinkingMax || fanInPrompt.Version != FanInPromptVersion {
		t.Fatalf("fan-in prompt profile = %#v", fanInPrompt)
	}
	if fanInPrompt.ProgressLabel != "semantic fan-in synthesis" {
		t.Fatalf("fan-in progress label = %q", fanInPrompt.ProgressLabel)
	}
	monolithicPrompt, err := BuildMonolithicPrompt(bundle, selected)
	if err != nil {
		t.Fatal(err)
	}
	if monolithicPrompt.ThinkingProfile != ThinkingMax || monolithicPrompt.Version != MonolithicPromptVersion {
		t.Fatalf("monolithic prompt profile = %#v", monolithicPrompt)
	}
	if monolithicPrompt.ProgressLabel != "semantic monolithic baseline" {
		t.Fatalf("monolithic progress label = %q", monolithicPrompt.ProgressLabel)
	}
	for _, prompt := range []Prompt{fanInPrompt, monolithicPrompt} {
		if strings.Contains(prompt.User, "Existing orientation calls this a repository analyzer") {
			t.Fatalf("synthesis prompt received planner-only editorial context: %s", prompt.Version)
		}
		if strings.Contains(prompt.User, "internal/analyzer") ||
			strings.Contains(prompt.User, "component-analysis") ||
			strings.Contains(prompt.User, "evidence-import") {
			t.Fatalf("synthesis prompt contains local navigation: %s", prompt.Version)
		}
	}

	missingOnly := []LeafResult{leafByKind(t, leaves, ArtifactRepositoryPattern)}
	if _, err := BuildFanInPrompt(bundle, missingOnly); err != nil {
		t.Fatalf("BuildFanInPrompt(missing-only) error = %v", err)
	}
}

func TestDownstreamPromptsDoNotForwardOpportunityEditorialProse(t *testing.T) {
	bundle := semanticTestBundle()
	editorial := []string{
		"Existing orientation calls this a repository analyzer",
		"How does existing orientation describe the repository analyzer?",
		"Existing guided material says collection starts locally",
	}
	proposal, normalization := NormalizeOpportunityProposal(bundle, OpportunityProposal{
		Version: OpportunityProposalVersion,
		Candidates: []OpportunityCandidate{{
			Kind:               ArtifactMechanism,
			Title:              editorial[0],
			QuestionAnswered:   editorial[1],
			SupportIDs:         []string{testFactLoad},
			MissingInformation: []string{editorial[2]},
			ExpectedValue:      ExpectedValueHigh,
			Confidence:         ConfidenceMedium,
		}},
	})
	if len(normalization.Issues) != 0 || len(proposal.Candidates) != 1 {
		t.Fatalf("NormalizeOpportunityProposal() = %#v, %#v", proposal, normalization)
	}
	tasks, err := PlanLeafTasks(bundle, proposal.Candidates)
	if err != nil {
		t.Fatal(err)
	}
	leafPrompt, err := BuildLeafPrompt(tasks[0])
	if err != nil {
		t.Fatal(err)
	}
	leaf := LeafResult{
		Task: tasks[0],
		Artifact: LeafArtifact{
			Version: LeafArtifactVersion, TaskID: tasks[0].ID,
			CandidateID: tasks[0].Candidate.ID, Status: LeafStatusUsable,
			Observations: []LeafObservation{{
				Text:       "The analysis stage loads package metadata through the package loader",
				SupportIDs: []string{testFactLoad},
			}},
			CandidateConnection: LeafCandidateConnection{
				CandidateID: tasks[0].Candidate.ID, Relation: connectionNeedsCombination,
				Explanation: "The bounded observation remains available for synthesis",
				SupportIDs:  []string{testFactLoad},
			},
		},
	}
	fanInPrompt, err := BuildFanInPrompt(bundle, []LeafResult{leaf})
	if err != nil {
		t.Fatal(err)
	}
	monolithicPrompt, err := BuildMonolithicPrompt(bundle, proposal.Candidates)
	if err != nil {
		t.Fatal(err)
	}
	var monolithicPayload monolithicPromptPayload
	if err := json.Unmarshal(
		[]byte(strings.TrimPrefix(
			strings.SplitN(monolithicPrompt.User, monolithicPayloadMarker, 2)[1],
			"\n",
		)),
		&monolithicPayload,
	); err != nil {
		t.Fatalf("decode monolithic payload: %v", err)
	}
	if len(monolithicPayload.Candidates) != 1 ||
		!equalStringSets(monolithicPayload.Candidates[0].SupportIDs, []string{testFactLoad}) {
		t.Fatalf("monolithic candidate fact scope = %#v", monolithicPayload.Candidates)
	}
	for _, prompt := range []Prompt{leafPrompt, fanInPrompt, monolithicPrompt} {
		for _, text := range editorial {
			if strings.Contains(prompt.User, text) {
				t.Fatalf("%s forwarded opportunity editorial prose %q", prompt.Version, text)
			}
		}
		if !strings.Contains(prompt.User, testFactLoad) {
			t.Fatalf("%s omitted original local support fact", prompt.Version)
		}
	}
}

func TestLeafValidationTreatsMissingEvidenceAsUsefulAndRejectsStaticBehavior(t *testing.T) {
	bundle := semanticTestBundle()
	selected, _ := SelectOpportunities(bundle, semanticTestOpportunity(bundle), 3)
	leaves := semanticTestLeaves(t, bundle, selected)
	missingOnly := leafByKind(t, leaves, ArtifactRepositoryPattern)
	if missingOnly.Artifact.Status != LeafStatusInsufficientEvidence {
		t.Fatalf("missing-only leaf status = %q", missingOnly.Artifact.Status)
	}
	if err := ValidateLeafArtifact(missingOnly.Task, missingOnly.Artifact); err != nil {
		t.Fatalf("ValidateLeafArtifact(missing-only) error = %v", err)
	}
	partial := leafByKind(t, leaves, ArtifactDependencyUsage)
	partial.Artifact.Status = LeafStatusInsufficientEvidence
	if err := ValidateLeafArtifact(partial.Task, partial.Artifact); err != nil {
		t.Fatalf("ValidateLeafArtifact(insufficient with observations) error = %v", err)
	}

	invalid := cloneJSON(t, missingOnly.Artifact)
	invalid.Status = LeafStatusUsable
	invalid.Observations = []LeafObservation{{
		Text:       "The analyzer loads package metadata from the package loading dependency",
		SupportIDs: []string{testFactStatic},
	}}
	invalid.MissingEvidence = nil
	invalid.CandidateConnection.SupportIDs = []string{testFactStatic}
	if err := ValidateLeafArtifact(missingOnly.Task, invalid); err == nil ||
		!strings.Contains(err.Error(), "behavior-capable") {
		t.Fatalf("static behavioral observation error = %v, want behavior-capability rejection", err)
	}

	capabilitySmuggling, _ := NormalizeOpportunityProposal(bundle, OpportunityProposal{
		Version: OpportunityProposalVersion,
		Candidates: []OpportunityCandidate{{
			Kind: ArtifactMechanism, Title: "Package loading signal",
			QuestionAnswered: "What do package loading facts establish?",
			SupportIDs:       []string{testFactStatic, testFactHTTP},
			ExpectedValue:    ExpectedValueMedium, Confidence: ConfidenceMedium,
		}},
	})
	smugglingTasks, err := PlanLeafTasks(bundle, capabilitySmuggling.Candidates)
	if err != nil {
		t.Fatal(err)
	}
	smuggling := LeafArtifact{
		Version: LeafArtifactVersion, TaskID: smugglingTasks[0].ID,
		CandidateID: smugglingTasks[0].Candidate.ID, Status: LeafStatusUsable,
		Observations: []LeafObservation{{
			Text:       "The analyzer loads package metadata",
			SupportIDs: []string{testFactStatic, testFactHTTP},
		}},
		CandidateConnection: LeafCandidateConnection{
			CandidateID: smugglingTasks[0].Candidate.ID, Relation: connectionNeedsCombination,
			Explanation: "The selected question needs combined evidence",
			SupportIDs:  []string{testFactStatic, testFactHTTP},
		},
	}
	if err := ValidateLeafArtifact(smugglingTasks[0], smuggling); err == nil ||
		!strings.Contains(err.Error(), "behavior-capable") {
		t.Fatalf("unrelated behavior capability error = %v, want semantic capability rejection", err)
	}

	invalid = cloneJSON(t, missingOnly.Artifact)
	invalid.MissingEvidence[0].SupportIDs = []string{testFactHTTP}
	invalid.CandidateConnection.SupportIDs = []string{testFactHTTP}
	if err := ValidateLeafArtifact(missingOnly.Task, invalid); err == nil {
		t.Fatal("leaf validation accepted an existing bundle fact outside its bounded task")
	}

	duplicate := cloneJSON(t, missingOnly.Artifact)
	duplicate.MissingEvidence[0].SupportIDs = append(
		duplicate.MissingEvidence[0].SupportIDs,
		duplicate.MissingEvidence[0].SupportIDs[0],
	)
	duplicate = NormalizeLeafArtifact(duplicate)
	if err := ValidateLeafArtifact(missingOnly.Task, duplicate); err == nil {
		t.Fatal("leaf normalization silently repaired duplicate support IDs")
	}
}

func TestSelectOpportunitiesPrefersBehaviorSupportOverStaticVolume(t *testing.T) {
	bundle := semanticTestBundle()
	proposal, normalization := NormalizeOpportunityProposal(bundle, OpportunityProposal{
		Version: OpportunityProposalVersion,
		Candidates: []OpportunityCandidate{
			{
				Kind: ArtifactMechanism, Title: "Static inventory",
				QuestionAnswered: "What static items are listed?",
				SupportIDs:       []string{testFactStatic, testFactGap},
				ExpectedValue:    ExpectedValueHigh, Confidence: ConfidenceMedium,
			},
			{
				Kind: ArtifactMechanism, Title: "Behavior explanation",
				QuestionAnswered: "What behavior is established?",
				SupportIDs:       []string{testFactLoad},
				ExpectedValue:    ExpectedValueHigh, Confidence: ConfidenceMedium,
			},
		},
	})
	if len(normalization.Issues) != 1 ||
		normalization.Issues[0].Code != "candidate_kind_not_locally_grounded" {
		t.Fatalf("NormalizeOpportunityProposal() = %#v", normalization)
	}
	selected, err := SelectOpportunities(bundle, proposal, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 1 || selected[0].Title != "Behavior explanation" {
		t.Fatalf("SelectOpportunities() = %#v", selected)
	}
}

func TestProjectTrustedFactStatement(t *testing.T) {
	tests := []struct {
		name      string
		statement string
		want      string
	}{
		{
			name:      "already safe prose remains byte-identical",
			statement: "The entry function directly calls the loader operation.",
			want:      "The entry function directly calls the loader operation.",
		},
		{
			name:      "qualified symbol keeps lexical support",
			statement: "The command invokes restoreReplica.Restore with configured options.",
			want:      "The command invokes restoreReplica Restore with configured options.",
		},
		{
			name:      "path keeps lexical support",
			statement: "The bounded source comes from cmd/tool/restore.go.",
			want:      "The bounded source comes from cmd tool restore go.",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ProjectTrustedFactStatement(test.statement); got != test.want {
				t.Fatalf("projection = %q, want %q", got, test.want)
			}
			if repositoryReferencePattern.MatchString(ProjectTrustedFactStatement(test.statement)) {
				t.Fatalf("projection retains repository reference: %q", test.statement)
			}
		})
	}
}

func TestReduceLeafArtifactKeepsValidItemsAndRejectsEmptyReduction(t *testing.T) {
	bundle := semanticTestBundle()
	selected, _ := SelectOpportunities(bundle, semanticTestOpportunity(bundle), 3)
	leaves := semanticTestLeaves(t, bundle, selected)
	mechanism := leafByKind(t, leaves, ArtifactMechanism)
	partial := cloneJSON(t, mechanism.Artifact)
	partial.Observations[0].Text = "internal/analyzer.go validates model output"
	partial.CandidateConnection = LeafCandidateConnection{
		CandidateID: "invented-candidate", Relation: "invented-relation",
		Explanation: "invented connection", SupportIDs: []string{"invented-fact"},
	}
	reduced, report, err := ReduceLeafArtifact(mechanism.Task, partial)
	if err != nil {
		t.Fatalf("ReduceLeafArtifact() error = %v", err)
	}
	if len(reduced.Observations) != 1 ||
		reduced.Observations[0].SupportIDs[0] != testFactSequence ||
		reduced.CandidateConnection.CandidateID != mechanism.Task.Candidate.ID ||
		!equalStringSets(reduced.CandidateConnection.SupportIDs, []string{testFactSequence}) {
		t.Fatalf("reduced leaf = %#v", reduced)
	}
	if report.DroppedObservations != 1 || report.KeptObservations != 1 ||
		len(report.Issues) != 1 || report.Issues[0].Code != "repository_reference" {
		t.Fatalf("reduction report = %#v", report)
	}
	if err := ValidateLeafArtifact(mechanism.Task, reduced); err != nil {
		t.Fatalf("ValidateLeafArtifact(reduced) error = %v", err)
	}

	missingOnly := leafByKind(t, leaves, ArtifactRepositoryPattern)
	invalid := cloneJSON(t, missingOnly.Artifact)
	invalid.MissingEvidence[0].SupportIDs = []string{"invented-fact"}
	_, emptyReport, err := ReduceLeafArtifact(missingOnly.Task, invalid)
	if err == nil {
		t.Fatal("ReduceLeafArtifact() accepted no valid content")
	}
	if emptyReport.DroppedMissingEvidence != 1 || len(emptyReport.Issues) != 2 ||
		emptyReport.Issues[0].Code != "unknown_support_id" ||
		emptyReport.Issues[1].Code != "no_valid_content" {
		t.Fatalf("empty reduction report = %#v", emptyReport)
	}
}

func TestFanInCompositionMaterializationAndStrictSupport(t *testing.T) {
	bundle := semanticTestBundle()
	opportunity := semanticTestOpportunity(bundle)
	selected, _ := SelectOpportunities(bundle, opportunity, 3)
	leaves := semanticTestLeaves(t, bundle, selected)
	fanIn := semanticTestFanIn(t, leaves)

	if err := ValidateFanInArtifact(bundle, leaves, fanIn); err != nil {
		t.Fatalf("ValidateFanInArtifact() error = %v", err)
	}
	if got := UnsupportedClaimCount(bundle, leaves, fanIn); got != 0 {
		t.Fatalf("UnsupportedClaimCount() = %d, want 0", got)
	}
	artifacts, err := MaterializeArtifacts(bundle, leaves, fanIn)
	if err != nil {
		t.Fatalf("MaterializeArtifacts() error = %v", err)
	}
	if len(artifacts) != 3 {
		t.Fatalf("materialized artifacts = %d, want 3", len(artifacts))
	}
	mechanism := artifactByKind(t, artifacts, ArtifactMechanism)
	if mechanism.Verdict != VerdictSupported || mechanism.Confidence != ConfidenceHigh {
		t.Fatalf("mechanism verdict/confidence = %q/%q", mechanism.Verdict, mechanism.Confidence)
	}
	if mechanism.Question != "How is model output checked before report rendering?" {
		t.Fatalf("materialized question = %q, want original candidate question", mechanism.Question)
	}
	for index, step := range mechanism.Steps {
		if step.Explanation != mechanism.Statements[index].Text {
			t.Fatalf("step explanation was not derived from statement: %#v", step)
		}
	}
	if len(mechanism.Focus.ComponentIDs) == 0 || len(mechanism.Evidence) == 0 {
		t.Fatalf("mechanism navigation was not derived locally: %#v", mechanism)
	}
	for _, relatedID := range mechanism.RelatedArtifactIDs {
		for _, statement := range mechanism.Statements {
			if slices.Contains(statement.SupportIDs, relatedID) {
				t.Fatalf("related artifact id became evidence: %q", relatedID)
			}
		}
	}
	dependency := artifactByKind(t, artifacts, ArtifactDependencyUsage)
	if dependency.Verdict != VerdictMixed || dependency.Confidence != ConfidenceMedium || len(dependency.Unknowns) < 2 {
		t.Fatalf("mixed dependency artifact did not retain gaps: %#v", dependency)
	}
	if !slices.Contains(dependency.Aliases, "golang.org/x/tools/go/packages") {
		t.Fatalf("dependency artifact lost its locally derived import alias: %#v", dependency.Aliases)
	}
	insufficient := artifactByKind(t, artifacts, ArtifactRepositoryPattern)
	if insufficient.Verdict != VerdictInsufficientEvidence || insufficient.Confidence != ConfidenceLow || len(insufficient.Unknowns) == 0 {
		t.Fatalf("insufficient artifact was not preserved: %#v", insufficient)
	}

	unrelated := cloneJSON(t, fanIn)
	mechanismProposal := proposalByCandidateKind(t, &unrelated, leaves, ArtifactMechanism)
	dependencyLeaf := leafByKind(t, leaves, ArtifactDependencyUsage)
	mechanismProposal.Claims[0].SupportIDs = []string{testFactLoad}
	mechanismProposal.Claims[0].ObservationRefs = []ObservationRef{{
		TaskID: dependencyLeaf.Task.ID, ObservationIndex: 0,
	}}
	if err := ValidateFanInArtifact(bundle, leaves, unrelated); err == nil ||
		!strings.Contains(err.Error(), "candidate leaf") {
		t.Fatalf("foreign direct support error = %v, want candidate-lineage rejection", err)
	}
	if got := UnsupportedClaimCount(bundle, leaves, unrelated); got != 1 {
		t.Fatalf("UnsupportedClaimCount(unrelated) = %d, want 1", got)
	}

	foreignMissing := cloneJSON(t, fanIn)
	dependencyProposal := proposalByCandidateKind(t, &foreignMissing, leaves, ArtifactDependencyUsage)
	patternLeaf := leafByKind(t, leaves, ArtifactRepositoryPattern)
	dependencyProposal.Claims[1].Text = "Package metadata loading behavior remains unresolved"
	dependencyProposal.Claims[1].SupportIDs = []string{testFactGap, testFactStatic}
	dependencyProposal.Claims[1].MissingRefs = []MissingEvidenceRef{{
		TaskID: patternLeaf.Task.ID, MissingIndex: 0,
	}}
	if err := ValidateFanInArtifact(bundle, leaves, foreignMissing); err == nil ||
		!strings.Contains(err.Error(), "candidate leaf") {
		t.Fatalf("foreign missing-evidence error = %v", err)
	}

	unknown := cloneJSON(t, fanIn)
	proposalByCandidateKind(t, &unknown, leaves, ArtifactMechanism).Claims[0].SupportIDs = []string{"unknown-fact"}
	if err := ValidateFanInArtifact(bundle, leaves, unknown); err == nil ||
		!strings.Contains(err.Error(), "unknown support id") {
		t.Fatalf("unknown support error = %v", err)
	}

	duplicate := cloneJSON(t, fanIn)
	duplicateProposal := proposalByCandidateKind(t, &duplicate, leaves, ArtifactMechanism)
	duplicateProposal.Claims[0].SupportIDs = append(
		duplicateProposal.Claims[0].SupportIDs,
		duplicateProposal.Claims[0].SupportIDs[0],
	)
	duplicate = NormalizeFanInArtifact(duplicate)
	if err := ValidateFanInArtifact(bundle, leaves, duplicate); err == nil {
		t.Fatal("fan-in normalization silently repaired duplicate support IDs")
	}
}

func TestFanInCompositionalClaimNeedsIndependentSourceGroups(t *testing.T) {
	bundle := semanticTestBundle()
	for index := range bundle.Facts {
		if bundle.Facts[index].ID == testFactSequence || bundle.Facts[index].ID == testFactLoad {
			bundle.Facts[index].SourceGroup = testGroupValidate
		}
	}
	selected, _ := SelectOpportunities(bundle, semanticTestOpportunity(bundle), 3)
	leaves := semanticTestLeaves(t, bundle, selected)
	fanIn := semanticTestFanIn(t, leaves)
	if err := ValidateFanInArtifact(bundle, leaves, fanIn); err == nil ||
		!strings.Contains(err.Error(), "two source groups") {
		t.Fatalf("same-group compositional error = %v", err)
	}
}

func TestFanInRejectsForeignOnlyResolutionOfMissingLeaf(t *testing.T) {
	bundle := semanticTestBundle()
	selected, _ := SelectOpportunities(bundle, semanticTestOpportunity(bundle), 3)
	leaves := semanticTestLeaves(t, bundle, selected)
	fanIn := semanticTestFanIn(t, leaves)
	pattern := proposalByCandidateKind(t, &fanIn, leaves, ArtifactRepositoryPattern)
	dependency := leafByKind(t, leaves, ArtifactDependencyUsage)
	*pattern = ArtifactProposal{
		CandidateID: pattern.CandidateID,
		Verdict:     VerdictSupported,
		Title:       "Package loading behavior",
		Summary:     "The analysis stage loads package metadata through the package loader",
		Claims: []ProposedClaim{{
			Title: "Observed loading",
			Text:  "The analysis stage loads package metadata through the package loader",
			Basis: ClaimDirect, SupportIDs: []string{testFactLoad},
			ObservationRefs: []ObservationRef{{
				TaskID: dependency.Task.ID, ObservationIndex: 0,
			}},
		}},
	}
	if err := ValidateFanInArtifact(bundle, leaves, fanIn); err == nil ||
		!strings.Contains(err.Error(), "candidate leaf") {
		t.Fatalf("foreign-only resolution error = %v, want candidate-lineage rejection", err)
	}
}

func TestMonolithicBaselineUsesOriginalFactsAndRejectsUnrelatedIDs(t *testing.T) {
	bundle := semanticTestBundle()
	selected, _ := SelectOpportunities(bundle, semanticTestOpportunity(bundle), 3)
	artifact := semanticTestMonolithic(t, selected)
	if err := ValidateMonolithicArtifact(bundle, selected, artifact); err != nil {
		t.Fatalf("ValidateMonolithicArtifact() error = %v", err)
	}
	if got := UnsupportedMonolithicClaimCount(bundle, selected, artifact); got != 0 {
		t.Fatalf("UnsupportedMonolithicClaimCount() = %d, want 0", got)
	}
	materialized, err := MaterializeMonolithicArtifacts(bundle, selected, artifact)
	if err != nil {
		t.Fatalf("MaterializeMonolithicArtifacts() error = %v", err)
	}
	if len(materialized) != 3 {
		t.Fatalf("monolithic materialized artifacts = %d, want 3", len(materialized))
	}

	invalid := cloneJSON(t, artifact)
	proposalBySelectedKind(t, &invalid, selected, ArtifactMechanism).Claims[0].SupportIDs = []string{testFactStatic}
	if err := ValidateMonolithicArtifact(bundle, selected, invalid); err == nil ||
		!strings.Contains(err.Error(), "candidate fact scope") {
		t.Fatalf("monolithic foreign support error = %v", err)
	}
	if got := UnsupportedMonolithicClaimCount(bundle, selected, invalid); got != 1 {
		t.Fatalf("UnsupportedMonolithicClaimCount(invalid) = %d, want 1", got)
	}

	foreignInterpretation := cloneJSON(t, artifact)
	foreignProposal := proposalBySelectedKind(
		t,
		&foreignInterpretation,
		selected,
		ArtifactMechanism,
	)
	foreignProposal.Summary = "The analysis stage loads package metadata through the package loader"
	foreignProposal.Claims = []ProposedClaim{{
		Title: "Metadata loading",
		Text:  "The analysis stage loads package metadata through the package loader",
		Basis: ClaimInterpretive, SupportIDs: []string{testFactLoad},
	}}
	if err := ValidateMonolithicArtifact(bundle, selected, foreignInterpretation); err == nil ||
		!strings.Contains(err.Error(), "candidate fact scope") {
		t.Fatalf("monolithic foreign interpretation error = %v", err)
	}

	withRefs := cloneJSON(t, artifact)
	proposalBySelectedKind(t, &withRefs, selected, ArtifactMechanism).Claims[0].ObservationRefs = []ObservationRef{{
		TaskID: "made-up-task", ObservationIndex: 0,
	}}
	if err := ValidateMonolithicArtifact(bundle, selected, withRefs); err == nil {
		t.Fatal("monolithic validation accepted fake leaf lineage")
	}
}

func TestRecordReplayIsCanonicalAndRejectsStaleOrUnknownFacts(t *testing.T) {
	bundle := semanticTestBundle()
	opportunity := semanticTestOpportunity(bundle)
	selected, _ := SelectOpportunities(bundle, opportunity, 3)
	leaves := semanticTestLeaves(t, bundle, selected)
	fanIn := semanticTestFanIn(t, leaves)
	raw, err := EncodeRecord(bundle, opportunity, selected, leaves, fanIn)
	if err != nil {
		t.Fatalf("EncodeRecord() error = %v", err)
	}
	artifacts, err := ReplayRecord(bundle, raw)
	if err != nil {
		t.Fatalf("ReplayRecord() error = %v", err)
	}
	if len(artifacts) != 3 {
		t.Fatalf("ReplayRecord() artifacts = %d, want 3", len(artifacts))
	}

	shuffledOpportunity := cloneJSON(t, opportunity)
	slices.Reverse(shuffledOpportunity.Candidates)
	shuffledLeaves := cloneJSON(t, leaves)
	slices.Reverse(shuffledLeaves)
	shuffledFanIn := cloneJSON(t, fanIn)
	slices.Reverse(shuffledFanIn.Artifacts)
	canonicalAgain, err := EncodeRecord(
		bundle, shuffledOpportunity, selected, shuffledLeaves, shuffledFanIn,
	)
	if err != nil {
		t.Fatalf("EncodeRecord(shuffled) error = %v", err)
	}
	if string(raw) != string(canonicalAgain) {
		t.Fatal("canonical record bytes changed after harmless input reordering")
	}

	stale := cloneJSON(t, bundle)
	stale.Facts[0].Statement += " changed"
	if _, err := ReplayRecord(stale, raw); err == nil || !strings.Contains(err.Error(), "bundle hash") {
		t.Fatalf("ReplayRecord(stale) error = %v", err)
	}

	record, err := DecodeRecord(raw)
	if err != nil {
		t.Fatal(err)
	}
	record.FanIn.Artifacts[0].Claims[0].SupportIDs = []string{"unknown-fact"}
	tampered, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReplayRecord(bundle, tampered); err == nil {
		t.Fatal("ReplayRecord() accepted an unknown support ID")
	}
	if _, err := ReplayRecord(bundle, append(raw, []byte(` {}`)...)); err == nil {
		t.Fatal("ReplayRecord() accepted trailing JSON")
	}

	partialLeaves := []LeafResult{leafByKind(t, leaves, ArtifactRepositoryPattern)}
	partialFanIn := semanticTestFanIn(t, leaves)
	partialProposal := *proposalByCandidateKind(
		t,
		&partialFanIn,
		leaves,
		ArtifactRepositoryPattern,
	)
	partialFanIn.Artifacts = []ArtifactProposal{partialProposal}
	partialRaw, err := EncodeRecord(
		bundle,
		opportunity,
		selected,
		partialLeaves,
		partialFanIn,
	)
	if err != nil {
		t.Fatalf("EncodeRecord(partial leaves) error = %v", err)
	}
	partialArtifacts, err := ReplayRecord(bundle, partialRaw)
	if err != nil {
		t.Fatalf("ReplayRecord(partial leaves) error = %v", err)
	}
	if len(partialArtifacts) != 1 || partialArtifacts[0].Kind != ArtifactRepositoryPattern {
		t.Fatalf("partial replay artifacts = %#v", partialArtifacts)
	}
}

func semanticTestBundle() Bundle {
	return Bundle{
		Version:  BundleVersion,
		RepoName: "sample",
		PlannerContext: []PlannerContext{
			{Kind: PlannerContextOrientation, Text: "Existing orientation calls this a repository analyzer"},
			{Kind: PlannerContextGuidedTour, Text: "Existing guided material starts with local collection"},
		},
		Facts: []Fact{
			{
				ID: testFactStatic, Kind: FactPackageImport,
				Statement:    "The analyzer package imports the package loading dependency",
				Keywords:     []string{"analyzer import", "golang.org/x/tools/go/packages", "package loading dependency"},
				SourceGroup:  "group-import-catalog",
				Capabilities: []Capability{CapabilityStatic}, Scope: FactScopeComponent,
				Focus: Focus{ComponentIDs: []string{"component-analysis"}},
				Evidence: []EvidenceRef{{
					ID: "evidence-import", Kind: "source", Label: "Package import",
					Path: "internal/analyzer/analyzer.go", Line: 8,
				}},
			},
			{
				ID: testFactLoad, Kind: FactSourceSignal,
				Statement:    "The analysis stage loads package metadata through the package loader",
				Keywords:     []string{"analysis stage", "package metadata loading"},
				SourceGroup:  "group-source-call",
				Capabilities: []Capability{CapabilityStatic, CapabilityBehavior}, Scope: FactScopeComponent,
				Focus: Focus{ComponentIDs: []string{"component-analysis"}},
				Evidence: []EvidenceRef{{
					ID: "evidence-load", Kind: "source", Label: "Metadata load",
					Path: "internal/analyzer/load.go", Line: 23,
				}},
			},
			{
				ID: testFactSequence, Kind: FactFlowStep,
				Statement:    "The report flow builds local facts before rendering the report",
				Keywords:     []string{"local facts", "report rendering"},
				SourceGroup:  "group-report-flow",
				Capabilities: []Capability{CapabilityBehavior, CapabilitySequence}, Scope: FactScopeFlow,
				Focus: Focus{FlowIDs: []string{"flow-report"}},
				Evidence: []EvidenceRef{{
					ID: "evidence-report", Kind: "flow", Label: "Report flow",
					Path: "internal/report/render.go", Line: 41,
				}},
			},
			{
				ID: testFactValidate, Kind: FactSourceSignal,
				Statement:    "Local validation checks model output evidence identifiers",
				Keywords:     []string{"model output", "local evidence validation"},
				SourceGroup:  testGroupValidate,
				Capabilities: []Capability{CapabilityBehavior}, Scope: FactScopeComponent,
				Focus: Focus{ComponentIDs: []string{"component-validation"}},
				Evidence: []EvidenceRef{{
					ID: "evidence-validation", Kind: "source", Label: "Output validation",
					Path: "internal/validation/check.go", Line: 17,
				}},
			},
			{
				ID: testFactHTTP, Kind: FactRuntimeSurface,
				Statement:    "The HTTP server handles report requests",
				Keywords:     []string{"HTTP server", "report request"},
				SourceGroup:  "group-http",
				Capabilities: []Capability{CapabilityBehavior}, Scope: FactScopeLocal,
			},
			{
				ID: testFactGap, Kind: FactWarning,
				Statement:    "Saved facts do not establish runtime ordering for package metadata loading",
				Keywords:     []string{"runtime ordering gap", "package metadata loading"},
				SourceGroup:  "group-runtime-warning",
				Capabilities: []Capability{CapabilityLimitation}, Scope: FactScopeComponent,
			},
		},
	}
}

func semanticTestOpportunity(bundle Bundle) OpportunityProposal {
	raw := OpportunityProposal{
		Version: OpportunityProposalVersion,
		Candidates: []OpportunityCandidate{
			{
				Kind: ArtifactMechanism, Title: "Validated report explanation",
				QuestionAnswered: "How is model output checked before report rendering?",
				SupportIDs:       []string{testFactValidate, testFactSequence},
				ExpectedValue:    ExpectedValueHigh, Confidence: ConfidenceHigh,
			},
			{
				Kind: ArtifactDependencyUsage, Title: "Package metadata loading",
				QuestionAnswered:   "How does the analysis stage use package metadata?",
				SupportIDs:         []string{testFactStatic, testFactLoad, testFactGap},
				MissingInformation: []string{"Runtime ordering remains unresolved"},
				ExpectedValue:      ExpectedValueHigh, Confidence: ConfidenceMedium,
			},
			{
				Kind: ArtifactRepositoryPattern, Title: "Package loading boundary",
				QuestionAnswered:   "What is established about package loading behavior?",
				SupportIDs:         []string{testFactStatic, testFactGap},
				MissingInformation: []string{"Behavior remains unresolved"},
				ExpectedValue:      ExpectedValueMedium, Confidence: ConfidenceMedium,
			},
		},
	}
	normalized, _ := NormalizeOpportunityProposal(bundle, raw)
	return normalized
}

func semanticTestLeaves(
	t *testing.T,
	bundle Bundle,
	selected []OpportunityCandidate,
) []LeafResult {
	t.Helper()
	tasks, err := PlanLeafTasks(bundle, selected)
	if err != nil {
		t.Fatalf("PlanLeafTasks() error = %v", err)
	}
	results := make([]LeafResult, 0, len(tasks))
	for _, task := range tasks {
		artifact := LeafArtifact{
			Version: LeafArtifactVersion, TaskID: task.ID, CandidateID: task.Candidate.ID,
			CandidateConnection: LeafCandidateConnection{
				CandidateID: task.Candidate.ID, Relation: connectionNeedsCombination,
				Explanation: "The selected question needs combined evidence",
			},
		}
		switch task.Candidate.Kind {
		case ArtifactMechanism:
			artifact.Status = LeafStatusUsable
			artifact.Observations = []LeafObservation{
				{Text: "Local validation checks model output evidence identifiers", SupportIDs: []string{testFactValidate}},
				{Text: "The report flow builds local facts before rendering the report", SupportIDs: []string{testFactSequence}},
			}
			artifact.CandidateConnection.SupportIDs = []string{testFactSequence, testFactValidate}
		case ArtifactDependencyUsage:
			artifact.Status = LeafStatusUsable
			artifact.Observations = []LeafObservation{{
				Text:       "The analysis stage loads package metadata through the package loader",
				SupportIDs: []string{testFactLoad},
			}}
			artifact.MissingEvidence = []LeafMissingEvidence{{
				Explanation:         "Saved facts do not establish runtime ordering for package metadata loading",
				SupportIDs:          []string{testFactGap, testFactLoad},
				MissingCapabilities: []Capability{CapabilitySequence},
			}}
			artifact.CandidateConnection.SupportIDs = []string{testFactGap, testFactLoad}
		case ArtifactRepositoryPattern:
			artifact.Status = LeafStatusInsufficientEvidence
			artifact.MissingEvidence = []LeafMissingEvidence{{
				Explanation:         "Package metadata loading behavior is not established",
				SupportIDs:          []string{testFactGap, testFactStatic},
				MissingCapabilities: []Capability{CapabilityBehavior},
			}}
			artifact.CandidateConnection.SupportIDs = []string{testFactGap, testFactStatic}
		default:
			t.Fatalf("unexpected candidate kind %q", task.Candidate.Kind)
		}
		if err := ValidateLeafArtifact(task, artifact); err != nil {
			t.Fatalf("ValidateLeafArtifact(%s) error = %v", task.Candidate.Kind, err)
		}
		results = append(results, LeafResult{Task: task, Artifact: artifact})
	}
	return results
}

func semanticTestFanIn(t *testing.T, leaves []LeafResult) FanInArtifact {
	t.Helper()
	mechanism := leafByKind(t, leaves, ArtifactMechanism)
	dependency := leafByKind(t, leaves, ArtifactDependencyUsage)
	pattern := leafByKind(t, leaves, ArtifactRepositoryPattern)
	return FanInArtifact{
		Version: FanInArtifactVersion,
		Artifacts: []ArtifactProposal{
			{
				CandidateID: mechanism.Task.Candidate.ID, Verdict: VerdictSupported,
				Title:   "Validated report explanation",
				Summary: "Local validation checks model output before report rendering and package metadata loading",
				Claims: []ProposedClaim{
					{
						Title: "Local validation", Text: "Local validation checks model output evidence identifiers",
						Basis: ClaimDirect, SupportIDs: []string{testFactValidate},
						ObservationRefs: []ObservationRef{{TaskID: mechanism.Task.ID, ObservationIndex: 0}},
					},
					{
						Title:      "Combined explanation",
						Text:       "Local validation checks model output before report rendering and package metadata loading",
						Basis:      ClaimCompositional,
						SupportIDs: []string{testFactLoad, testFactSequence, testFactValidate},
						ObservationRefs: []ObservationRef{
							{TaskID: mechanism.Task.ID, ObservationIndex: 0},
							{TaskID: mechanism.Task.ID, ObservationIndex: 1},
							{TaskID: dependency.Task.ID, ObservationIndex: 0},
						},
					},
				},
				Aliases:             []string{"model validation", "report explanation"},
				LikelyQuestions:     []string{"How is model output checked?"},
				RelatedCandidateIDs: []string{dependency.Task.Candidate.ID},
			},
			{
				CandidateID: dependency.Task.Candidate.ID, Verdict: VerdictMixed,
				Title:   "Package metadata loading",
				Summary: "Package metadata loading has an unresolved runtime ordering gap",
				Claims: []ProposedClaim{
					{
						Title: "Metadata loading",
						Text:  "The analysis stage loads package metadata through the package loader",
						Basis: ClaimDirect, SupportIDs: []string{testFactLoad},
						ObservationRefs: []ObservationRef{{TaskID: dependency.Task.ID, ObservationIndex: 0}},
					},
					{
						Title: "Ordering gap",
						Text:  "Package metadata loading runtime ordering remains unresolved",
						Basis: ClaimUnresolved, SupportIDs: []string{testFactGap, testFactLoad},
						MissingRefs: []MissingEvidenceRef{{TaskID: dependency.Task.ID, MissingIndex: 0}},
					},
				},
				Aliases:             []string{"metadata loading"},
				LikelyQuestions:     []string{"How is package metadata loaded?"},
				RelatedCandidateIDs: []string{mechanism.Task.Candidate.ID},
			},
			{
				CandidateID: pattern.Task.Candidate.ID, Verdict: VerdictInsufficientEvidence,
				Title:   "Package loading boundary",
				Summary: "Package metadata loading behavior remains unresolved",
				Claims: []ProposedClaim{{
					Title: "Behavior gap", Text: "Package metadata loading behavior is not established",
					Basis: ClaimUnresolved, SupportIDs: []string{testFactGap, testFactStatic},
					MissingRefs: []MissingEvidenceRef{{TaskID: pattern.Task.ID, MissingIndex: 0}},
				}},
				Aliases:         []string{"package loading boundary"},
				LikelyQuestions: []string{"Is package loading behavior established?"},
			},
		},
	}
}

func semanticTestMonolithic(
	t *testing.T,
	selected []OpportunityCandidate,
) FanInArtifact {
	t.Helper()
	mechanism := candidateByKind(t, selected, ArtifactMechanism)
	dependency := candidateByKind(t, selected, ArtifactDependencyUsage)
	pattern := candidateByKind(t, selected, ArtifactRepositoryPattern)
	return FanInArtifact{
		Version: FanInArtifactVersion,
		Artifacts: []ArtifactProposal{
			{
				CandidateID: mechanism.ID, Verdict: VerdictSupported,
				Title:   "Validated report explanation",
				Summary: "Local validation checks model output before report rendering and package metadata loading",
				Claims: []ProposedClaim{
					{Title: "Local validation", Text: "Local validation checks model output evidence identifiers", Basis: ClaimDirect, SupportIDs: []string{testFactValidate}},
					{Title: "Combined explanation", Text: "Local validation checks model output before report rendering and package metadata loading", Basis: ClaimCompositional, SupportIDs: []string{testFactLoad, testFactSequence, testFactValidate}},
				},
				Aliases:             []string{"model validation"},
				RelatedCandidateIDs: []string{dependency.ID},
			},
			{
				CandidateID: dependency.ID, Verdict: VerdictMixed,
				Title:   "Package metadata loading",
				Summary: "Saved facts do not establish runtime ordering for package metadata loading",
				Claims: []ProposedClaim{
					{Title: "Metadata loading", Text: "The analysis stage loads package metadata through the package loader", Basis: ClaimDirect, SupportIDs: []string{testFactLoad}},
					{Title: "Ordering gap", Text: "Saved facts do not establish runtime ordering for package metadata loading", Basis: ClaimUnresolved, SupportIDs: []string{testFactGap}},
				},
			},
			{
				CandidateID: pattern.ID, Verdict: VerdictInsufficientEvidence,
				Title:   "Package loading boundary",
				Summary: "Saved facts do not establish package loading dependency behavior",
				Claims: []ProposedClaim{{
					Title: "Behavior gap", Text: "Saved facts do not establish package loading dependency behavior",
					Basis: ClaimUnresolved, SupportIDs: []string{testFactGap, testFactStatic},
				}},
			},
		},
	}
}

func withoutCandidateID(candidate OpportunityCandidate) OpportunityCandidate {
	candidate.ID = ""
	return candidate
}

func leafByKind(t *testing.T, leaves []LeafResult, kind ArtifactKind) LeafResult {
	t.Helper()
	for _, leaf := range leaves {
		if leaf.Task.Candidate.Kind == kind {
			return leaf
		}
	}
	t.Fatalf("leaf kind %q not found", kind)
	return LeafResult{}
}

func candidateByKind(
	t *testing.T,
	candidates []OpportunityCandidate,
	kind ArtifactKind,
) OpportunityCandidate {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.Kind == kind {
			return candidate
		}
	}
	t.Fatalf("candidate kind %q not found", kind)
	return OpportunityCandidate{}
}

func proposalByCandidateKind(
	t *testing.T,
	artifact *FanInArtifact,
	leaves []LeafResult,
	kind ArtifactKind,
) *ArtifactProposal {
	t.Helper()
	candidateID := leafByKind(t, leaves, kind).Task.Candidate.ID
	for index := range artifact.Artifacts {
		if artifact.Artifacts[index].CandidateID == candidateID {
			return &artifact.Artifacts[index]
		}
	}
	t.Fatalf("proposal kind %q not found", kind)
	return nil
}

func proposalBySelectedKind(
	t *testing.T,
	artifact *FanInArtifact,
	selected []OpportunityCandidate,
	kind ArtifactKind,
) *ArtifactProposal {
	t.Helper()
	candidateID := candidateByKind(t, selected, kind).ID
	for index := range artifact.Artifacts {
		if artifact.Artifacts[index].CandidateID == candidateID {
			return &artifact.Artifacts[index]
		}
	}
	t.Fatalf("proposal kind %q not found", kind)
	return nil
}

func artifactByKind(t *testing.T, artifacts []Artifact, kind ArtifactKind) Artifact {
	t.Helper()
	for _, artifact := range artifacts {
		if artifact.Kind == kind {
			return artifact
		}
	}
	t.Fatalf("artifact kind %q not found", kind)
	return Artifact{}
}

func cloneJSON[T any](t *testing.T, value T) T {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var clone T
	if err := json.Unmarshal(raw, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}
