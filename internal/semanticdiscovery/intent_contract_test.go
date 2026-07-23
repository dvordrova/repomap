package semanticdiscovery

import (
	"slices"
	"strings"
	"testing"
)

const (
	goldenFactTrigger     = "fact-golden-trigger"
	goldenFactCollection  = "fact-golden-collection"
	goldenFactQuery       = "fact-golden-query"
	goldenFactTransform   = "fact-golden-transform"
	goldenFactOutput      = "fact-golden-output"
	goldenFactDebugStart  = "fact-golden-debug-start"
	goldenFactDebugStatus = "fact-golden-debug-status"
	goldenFactDebugEnd    = "fact-golden-debug-end"
)

type goldenMechanismFixture struct {
	bundle      Bundle
	opportunity OpportunityProposal
	candidate   OpportunityCandidate
	leaf        LeafResult
	artifact    FanInArtifact
}

func TestCapabilityContractAndEnrichmentIdentity(t *testing.T) {
	fixture := newGoldenMechanismFixture(t)
	if err := ValidateOpportunityProposal(fixture.bundle, fixture.opportunity); err != nil {
		t.Fatalf("ValidateOpportunityProposal() error = %v", err)
	}
	if got := opportunityID(fixture.candidate); got != fixture.candidate.ID {
		t.Fatalf("enrichment changed candidate identity: got %q, want %q", got, fixture.candidate.ID)
	}
	if slices.Contains(fixture.candidate.SupportIDs, goldenFactCollection) {
		t.Fatalf("enrichment fact leaked into initial support: %#v", fixture.candidate.SupportIDs)
	}
	if !slices.Contains(fixture.candidate.EnrichmentSupportIDs, goldenFactCollection) {
		t.Fatalf("candidate lost enrichment support: %#v", fixture.candidate.EnrichmentSupportIDs)
	}
	if len(fixture.leaf.Task.Facts) != len(candidateFactIDs(fixture.candidate)) {
		t.Fatalf(
			"leaf facts = %d, want support union %d",
			len(fixture.leaf.Task.Facts),
			len(candidateFactIDs(fixture.candidate)),
		)
	}

	tests := []struct {
		name      string
		mutate    func(*OpportunityCandidate)
		wantError string
	}{
		{
			name: "overlapping enrichment support",
			mutate: func(candidate *OpportunityCandidate) {
				candidate.EnrichmentSupportIDs = append(
					candidate.EnrichmentSupportIDs,
					goldenFactTrigger,
				)
			},
			wantError: "duplicates initial support",
		},
		{
			name: "capability in both partitions",
			mutate: func(candidate *OpportunityCandidate) {
				candidate.CapabilityContract.MissingCapabilities = append(
					candidate.CapabilityContract.MissingCapabilities,
					CapabilityDataRead,
				)
			},
			wantError: "both available and missing",
		},
		{
			name: "available capability without fact",
			mutate: func(candidate *OpportunityCandidate) {
				candidate.CapabilityContract.RequiredCapabilities = append(
					candidate.CapabilityContract.RequiredCapabilities,
					CapabilityOwnership,
				)
				candidate.CapabilityContract.AvailableCapabilities = append(
					candidate.CapabilityContract.AvailableCapabilities,
					CapabilityOwnership,
				)
			},
			wantError: "has no supporting fact",
		},
		{
			name: "duplicate answer aspect",
			mutate: func(candidate *OpportunityCandidate) {
				candidate.IntentContract.RequiredAnswerAspects = append(
					candidate.IntentContract.RequiredAnswerAspects,
					candidate.IntentContract.RequiredAnswerAspects[0],
				)
			},
			wantError: "duplicate answer aspect",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			proposal := cloneJSON(t, fixture.opportunity)
			test.mutate(&proposal.Candidates[0])
			err := ValidateOpportunityProposal(fixture.bundle, proposal)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("ValidateOpportunityProposal() error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestGranularCapabilitiesSupportBehavior(t *testing.T) {
	tests := []struct {
		name       string
		statement  string
		capability Capability
	}{
		{
			name:       "direct call",
			statement:  "The request handler calls the listing renderer",
			capability: CapabilityDirectCall,
		},
		{
			name:       "data read",
			statement:  "The listing handler reads directory entries",
			capability: CapabilityDataRead,
		},
		{
			name:       "output effect",
			statement:  "The response writer writes the prepared listing",
			capability: CapabilityOutputEffect,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fact := Fact{
				ID:           "fact-granular",
				Kind:         FactSourceSignal,
				Statement:    test.statement,
				SourceGroup:  "group-granular",
				Capabilities: []Capability{test.capability},
				Scope:        FactScopeLocal,
			}
			if err := validateSemanticSupport("granular claim", test.statement, []Fact{fact}); err != nil {
				t.Fatalf("validateSemanticSupport() error = %v", err)
			}
		})
	}
}

func TestGoldenMechanismIntentRetentionAndReplay(t *testing.T) {
	fixture := newGoldenMechanismFixture(t)
	if err := ValidateFanInArtifact(
		fixture.bundle,
		[]LeafResult{fixture.leaf},
		fixture.artifact,
	); err != nil {
		t.Fatalf("ValidateFanInArtifact() error = %v", err)
	}
	artifacts, err := MaterializeArtifacts(
		fixture.bundle,
		[]LeafResult{fixture.leaf},
		fixture.artifact,
	)
	if err != nil {
		t.Fatalf("MaterializeArtifacts() error = %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("materialized artifacts = %d, want 1", len(artifacts))
	}
	artifact := artifacts[0]
	if artifact.CandidateID != fixture.candidate.ID {
		t.Fatalf("candidate id = %q, want %q", artifact.CandidateID, fixture.candidate.ID)
	}
	if artifact.Question != fixture.candidate.QuestionAnswered {
		t.Fatalf("question = %q, want original %q", artifact.Question, fixture.candidate.QuestionAnswered)
	}
	if len(artifact.Steps) != 6 {
		t.Fatalf("steps = %d, want 6", len(artifact.Steps))
	}
	resolved := 0
	for _, statement := range artifact.Statements {
		if statement.Basis != ClaimUnresolved {
			resolved++
		}
	}
	if resolved < 3 {
		t.Fatalf("resolved statements = %d, want at least 3", resolved)
	}
	if len(artifact.CoveredAspectIDs) != 5 ||
		!slices.Equal(artifact.UncoveredAspectIDs, []string{"error"}) {
		t.Fatalf(
			"coverage = %#v / %#v, want five covered and error uncovered",
			artifact.CoveredAspectIDs,
			artifact.UncoveredAspectIDs,
		)
	}
	if !slices.Contains(artifact.Aliases, "как сервер строит список файлов") {
		t.Fatalf("materialized aliases lost local search intent: %#v", artifact.Aliases)
	}
	if !containsText(artifact.Unknowns, "Error handling") {
		t.Fatalf("materialized unknowns lost uncovered aspect: %#v", artifact.Unknowns)
	}
	wantUsed := []string{
		goldenFactCollection,
		goldenFactOutput,
		goldenFactQuery,
		goldenFactTransform,
		goldenFactTrigger,
	}
	if !slices.Equal(artifact.UsedFactIDs, wantUsed) {
		t.Fatalf("used facts = %#v, want %#v", artifact.UsedFactIDs, wantUsed)
	}
	wantUnused := []string{
		goldenFactDebugEnd,
		goldenFactDebugStart,
		goldenFactDebugStatus,
	}
	if !slices.Equal(artifact.UnusedAvailableFactIDs, wantUnused) {
		t.Fatalf(
			"unused available facts = %#v, want %#v",
			artifact.UnusedAvailableFactIDs,
			wantUnused,
		)
	}
	assessment, err := AssessClaimCoverage(
		fixture.bundle,
		[]LeafResult{fixture.leaf},
		fixture.artifact.Artifacts[0],
	)
	if err != nil {
		t.Fatalf("AssessClaimCoverage() error = %v", err)
	}
	if !slices.Equal(assessment.UsedFactIDs, wantUsed) ||
		!slices.Equal(assessment.UnusedAvailableFactIDs, wantUnused) ||
		!slices.Equal(assessment.UncoveredAspectIDs, []string{"error"}) {
		t.Fatalf("claim coverage assessment = %#v", assessment)
	}

	selected := []OpportunityCandidate{fixture.candidate}
	record, err := EncodeRecord(
		fixture.bundle,
		fixture.opportunity,
		selected,
		[]LeafResult{fixture.leaf},
		fixture.artifact,
	)
	if err != nil {
		t.Fatalf("EncodeRecord() error = %v", err)
	}
	replayed, err := ReplayRecord(fixture.bundle, record)
	if err != nil {
		t.Fatalf("ReplayRecord() error = %v", err)
	}
	if len(replayed) != 1 || replayed[0].CandidateID != fixture.candidate.ID ||
		replayed[0].Question != fixture.candidate.QuestionAnswered ||
		!slices.Equal(replayed[0].UncoveredAspectIDs, []string{"error"}) ||
		!slices.Equal(replayed[0].UsedFactIDs, wantUsed) ||
		!slices.Equal(replayed[0].UnusedAvailableFactIDs, wantUnused) {
		t.Fatalf("replay lost intent retention: %#v", replayed)
	}
}

func TestIntentCoverageRejectsUnrelatedSupportPadding(t *testing.T) {
	t.Parallel()

	fixture := newGoldenMechanismFixture(t)
	artifact := cloneJSON(t, fixture.artifact)
	proposal := &artifact.Artifacts[0]
	proposal.Claims[0].SupportIDs = append(
		proposal.Claims[0].SupportIDs,
		goldenFactOutput,
	)
	proposal.Claims[0].ObservationRefs = append(
		proposal.Claims[0].ObservationRefs,
		ObservationRef{
			TaskID:           fixture.leaf.Task.ID,
			ObservationIndex: 4,
		},
	)
	proposal.Claims = append(proposal.Claims[:4], proposal.Claims[5:]...)

	err := ValidateFanInArtifact(
		fixture.bundle,
		[]LeafResult{fixture.leaf},
		artifact,
	)
	if err == nil || !strings.Contains(err.Error(), "covers 4 aspects, requires 5") {
		t.Fatalf("ValidateFanInArtifact() error = %v", err)
	}
}

func TestGoldenMechanismRejectsDebugOnlyArtifact(t *testing.T) {
	fixture := newGoldenMechanismFixture(t)
	debugOnly := goldenDebugOnlyArtifact(fixture.leaf)
	err := ValidateFanInArtifact(fixture.bundle, []LeafResult{fixture.leaf}, debugOnly)
	if err == nil || !strings.Contains(err.Error(), "covers 0 aspects") {
		t.Fatalf("ValidateFanInArtifact(debug only) error = %v, want local coverage rejection", err)
	}
}

func TestBuildGoldenMechanismPromptRetainsQuestionAndRubric(t *testing.T) {
	fixture := newGoldenMechanismFixture(t)
	focused := fixture.leaf
	focused.Artifact.Observations = append(
		[]LeafObservation(nil),
		focused.Artifact.Observations[:5]...,
	)
	focused.Artifact.CandidateConnection.SupportIDs = []string{
		goldenFactTrigger,
		goldenFactCollection,
		goldenFactQuery,
		goldenFactTransform,
		goldenFactOutput,
	}
	prompt, err := BuildGoldenMechanismPrompt(fixture.bundle, focused)
	if err != nil {
		t.Fatalf("BuildGoldenMechanismPrompt() error = %v", err)
	}
	if prompt.Version != GoldenMechanismPromptVersion || prompt.ThinkingProfile != ThinkingMax {
		t.Fatalf("prompt contract = %q/%q", prompt.Version, prompt.ThinkingProfile)
	}
	if GoldenMechanismPromptVersion == GoldenMechanismPromptVersionV3 {
		t.Fatal("current golden prompt version did not advance beyond the legacy v3 contract")
	}
	for _, expected := range []string{
		fixture.candidate.QuestionAnswered,
		`"required_answer_aspects"`,
		`"local_search_aliases"`,
		`answer_aspect:`,
		`"known_editorial_gaps"`,
		`rather than slash-separated shorthand`,
		`Claim order is editorial`,
		`Do not promise a layer, effect, output, or user-visible result`,
		`only when the cited support facts are sequence-capable`,
		`begin its text with "Evidence gap:"`,
	} {
		if !strings.Contains(prompt.User, expected) {
			t.Fatalf("golden prompt omitted %q", expected)
		}
	}
	if strings.Contains(prompt.User, "Debug logging") {
		t.Fatal("golden prompt retained historical candidate facts unused by the validated leaf")
	}
	staticPrefix, _, found := strings.Cut(prompt.User, goldenMechanismPayloadMarker)
	if !found {
		t.Fatal("golden prompt omitted its variable payload marker")
	}
	for _, caddySpecific := range []string{
		`keep the words "directory listing"`,
		`local browse sequence`,
		`browse predicate`,
		`sorting and pagination`,
		`sort/page`,
	} {
		if strings.Contains(staticPrefix, caddySpecific) {
			t.Fatalf("golden prompt retained mechanism-specific rule %q", caddySpecific)
		}
	}
}

func newGoldenMechanismFixture(t *testing.T) goldenMechanismFixture {
	t.Helper()
	bundle := Bundle{
		Version:  BundleVersion,
		RepoName: "caddy",
		Facts: []Fact{
			goldenFact(
				goldenFactTrigger,
				"A directory request enters the listing handler",
				"group-trigger",
				[]Capability{CapabilityEntry, CapabilitySequence},
				"trigger",
			),
			goldenFact(
				goldenFactCollection,
				"The listing handler reads directory entries from local storage",
				"group-collection",
				[]Capability{CapabilityDataRead},
				"collection",
			),
			goldenFact(
				goldenFactQuery,
				"Request query values select sorting direction and result limits",
				"group-query",
				[]Capability{CapabilityDataRead, CapabilityBranch},
				"query",
			),
			goldenFact(
				goldenFactTransform,
				"The listing path transforms collected directory entries by sorting and limiting them",
				"group-transform",
				[]Capability{CapabilityDataTransformation},
				"transformation",
			),
			goldenFact(
				goldenFactOutput,
				"The response writer writes the prepared directory listing",
				"group-output",
				[]Capability{CapabilityOutputEffect},
				"output",
			),
			goldenFactWithoutAspect(
				goldenFactDebugStart,
				"Debug logging writes request messages",
				"group-debug-start",
				CapabilityOutputEffect,
			),
			goldenFactWithoutAspect(
				goldenFactDebugStatus,
				"Debug logging writes response status",
				"group-debug-status",
				CapabilityDataWrite,
			),
			goldenFactWithoutAspect(
				goldenFactDebugEnd,
				"Debug logging emits completion duration",
				"group-debug-end",
				CapabilityOutputEffect,
			),
		},
	}
	raw := OpportunityProposal{
		Version: OpportunityProposalVersion,
		Candidates: []OpportunityCandidate{{
			Kind:             ArtifactMechanism,
			Title:            "File Server Directory Listing",
			QuestionAnswered: "How does the file server build a directory listing?",
			SupportIDs:       []string{goldenFactTrigger},
			MissingInformation: []string{
				"Error handling remains unresolved",
			},
			ExpectedValue: ExpectedValueHigh,
			Confidence:    ConfidenceHigh,
		}},
	}
	normalized, _ := NormalizeOpportunityProposal(bundle, raw)
	candidate := normalized.Candidates[0]
	candidate.EnrichmentSupportIDs = []string{
		goldenFactCollection,
		goldenFactQuery,
		goldenFactTransform,
		goldenFactOutput,
		goldenFactDebugStart,
		goldenFactDebugStatus,
		goldenFactDebugEnd,
	}
	candidate.CapabilityContract = &CapabilityContract{
		RequiredCapabilities: []Capability{
			CapabilityEntry,
			CapabilitySequence,
			CapabilityDataRead,
			CapabilityBranch,
			CapabilityDataTransformation,
			CapabilityOutputEffect,
			CapabilityErrorPath,
		},
		AvailableCapabilities: []Capability{
			CapabilityEntry,
			CapabilitySequence,
			CapabilityDataRead,
			CapabilityBranch,
			CapabilityDataTransformation,
			CapabilityOutputEffect,
		},
		MissingCapabilities: []Capability{CapabilityErrorPath},
		Resolution:          CapabilityResolutionPartial,
	}
	candidate.IntentContract = &IntentContract{
		RequiredAnswerAspects: []AnswerAspect{
			{ID: "trigger", Label: "Entry trigger", RequiredCapabilities: []Capability{CapabilityEntry}, Key: true},
			{ID: "collection", Label: "Directory entry collection", RequiredCapabilities: []Capability{CapabilityDataRead}, Key: true},
			{ID: "query", Label: "Query sorting and limits", RequiredCapabilities: []Capability{CapabilityDataRead, CapabilityBranch}, Key: true},
			{ID: "transformation", Label: "Entry transformation", RequiredCapabilities: []Capability{CapabilityDataTransformation}},
			{ID: "output", Label: "Response output", RequiredCapabilities: []Capability{CapabilityOutputEffect}, Key: true},
			{ID: "error", Label: "Error handling", RequiredCapabilities: []Capability{CapabilityErrorPath}},
		},
		MinCovered:         5,
		MinKeyCovered:      3,
		LocalSearchAliases: []string{"directory listing mechanism", "как сервер строит список файлов"},
	}
	opportunity := OpportunityProposal{
		Version:    OpportunityProposalVersion,
		Candidates: []OpportunityCandidate{candidate},
	}
	if err := ValidateOpportunityProposal(bundle, opportunity); err != nil {
		t.Fatalf("golden opportunity: %v", err)
	}
	tasks, err := PlanLeafTasks(bundle, []OpportunityCandidate{candidate})
	if err != nil {
		t.Fatalf("PlanLeafTasks() error = %v", err)
	}
	task := tasks[0]
	observations := []LeafObservation{
		goldenObservation(bundle, goldenFactTrigger),
		goldenObservation(bundle, goldenFactCollection),
		goldenObservation(bundle, goldenFactQuery),
		goldenObservation(bundle, goldenFactTransform),
		goldenObservation(bundle, goldenFactOutput),
		goldenObservation(bundle, goldenFactDebugStart),
		goldenObservation(bundle, goldenFactDebugStatus),
		goldenObservation(bundle, goldenFactDebugEnd),
	}
	leafArtifact := LeafArtifact{
		Version:      LeafArtifactVersion,
		TaskID:       task.ID,
		CandidateID:  candidate.ID,
		Status:       LeafStatusUsable,
		Observations: observations,
		CandidateConnection: LeafCandidateConnection{
			CandidateID: candidate.ID,
			Relation:    connectionNeedsCombination,
			Explanation: "Global synthesis still needs to combine listing behavior and its unresolved error path",
			SupportIDs:  candidateFactIDs(candidate),
		},
		MissingEvidence: []LeafMissingEvidence{{
			Explanation:         "The prepared directory listing does not establish its error path",
			SupportIDs:          []string{goldenFactOutput},
			MissingCapabilities: []Capability{CapabilityErrorPath},
		}},
	}
	if err := ValidateLeafArtifact(task, leafArtifact); err != nil {
		t.Fatalf("ValidateLeafArtifact() error = %v", err)
	}
	leaf := LeafResult{Task: task, Artifact: leafArtifact}
	return goldenMechanismFixture{
		bundle:      bundle,
		opportunity: opportunity,
		candidate:   candidate,
		leaf:        leaf,
		artifact:    goldenSupportedArtifact(leaf),
	}
}

func goldenSupportedArtifact(leaf LeafResult) FanInArtifact {
	claims := make([]ProposedClaim, 0, 6)
	for index, title := range []string{
		"Entry trigger",
		"Collect entries",
		"Read query options",
		"Transform entries",
		"Write response",
	} {
		observation := leaf.Artifact.Observations[index]
		claims = append(claims, ProposedClaim{
			Title:      title,
			Text:       observation.Text,
			Basis:      ClaimDirect,
			SupportIDs: append([]string(nil), observation.SupportIDs...),
			ObservationRefs: []ObservationRef{{
				TaskID: leaf.Task.ID, ObservationIndex: index,
			}},
		})
	}
	missing := leaf.Artifact.MissingEvidence[0]
	claims = append(claims, ProposedClaim{
		Title:      "Unresolved error path",
		Text:       missing.Explanation,
		Basis:      ClaimUnresolved,
		SupportIDs: append([]string(nil), missing.SupportIDs...),
		MissingRefs: []MissingEvidenceRef{{
			TaskID: leaf.Task.ID, MissingIndex: 0,
		}},
	})
	return FanInArtifact{
		Version: FanInArtifactVersion,
		Artifacts: []ArtifactProposal{{
			CandidateID: leaf.Task.Candidate.ID,
			Verdict:     VerdictMixed,
			Title:       "File Server Directory Listing",
			Summary: "A directory request enters the listing handler, which reads directory entries, " +
				"selects query sorting and limits, transforms the collected entries, and writes the " +
				"prepared directory listing; the error path remains unresolved",
			Claims: claims,
			Aliases: append(
				[]string(nil),
				leaf.Task.Candidate.IntentContract.LocalSearchAliases...,
			),
			LikelyQuestions: []string{leaf.Task.Candidate.QuestionAnswered},
		}},
	}
}

func goldenDebugOnlyArtifact(leaf LeafResult) FanInArtifact {
	claims := make([]ProposedClaim, 0, 4)
	for index, title := range []string{"Request log", "Status log", "Duration log"} {
		observationIndex := index + 5
		observation := leaf.Artifact.Observations[observationIndex]
		claims = append(claims, ProposedClaim{
			Title:      title,
			Text:       observation.Text,
			Basis:      ClaimDirect,
			SupportIDs: append([]string(nil), observation.SupportIDs...),
			ObservationRefs: []ObservationRef{{
				TaskID: leaf.Task.ID, ObservationIndex: observationIndex,
			}},
		})
	}
	missing := leaf.Artifact.MissingEvidence[0]
	claims = append(claims, ProposedClaim{
		Title:       "Unresolved error path",
		Text:        missing.Explanation,
		Basis:       ClaimUnresolved,
		SupportIDs:  append([]string(nil), missing.SupportIDs...),
		MissingRefs: []MissingEvidenceRef{{TaskID: leaf.Task.ID, MissingIndex: 0}},
	})
	return FanInArtifact{
		Version: FanInArtifactVersion,
		Artifacts: []ArtifactProposal{{
			CandidateID: leaf.Task.Candidate.ID,
			Verdict:     VerdictMixed,
			Title:       "Debug Logging",
			Summary: "Debug logging writes request messages and response status and emits completion " +
				"duration; the error path remains unresolved",
			Claims: claims,
			Aliases: append(
				[]string(nil),
				leaf.Task.Candidate.IntentContract.LocalSearchAliases...,
			),
			LikelyQuestions: []string{leaf.Task.Candidate.QuestionAnswered},
		}},
	}
}

func goldenFact(
	id string,
	statement string,
	group string,
	capabilities []Capability,
	aspectID string,
) Fact {
	return Fact{
		ID:           id,
		Kind:         FactSourceSignal,
		Statement:    statement,
		Keywords:     []string{answerAspectKeywordPrefix + aspectID},
		SourceGroup:  group,
		Capabilities: capabilities,
		Scope:        FactScopeLocal,
	}
}

func goldenFactWithoutAspect(
	id string,
	statement string,
	group string,
	capability Capability,
) Fact {
	return Fact{
		ID:           id,
		Kind:         FactSourceSignal,
		Statement:    statement,
		Keywords:     []string{"debug logging"},
		SourceGroup:  group,
		Capabilities: []Capability{capability},
		Scope:        FactScopeLocal,
	}
}

func goldenObservation(bundle Bundle, id string) LeafObservation {
	fact := factIndex(bundle)[id]
	return LeafObservation{Text: fact.Statement, SupportIDs: []string{id}}
}

func containsText(values []string, fragment string) bool {
	for _, value := range values {
		if strings.Contains(value, fragment) {
			return true
		}
	}
	return false
}
