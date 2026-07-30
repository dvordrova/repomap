package report

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/guidedtour"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

func TestBuildSemanticSearchIndexGroundsTargetsAndAliases(t *testing.T) {
	data := semanticSearchTestReport()

	index, err := BuildSemanticSearchIndex(data)
	if err != nil {
		t.Fatalf("BuildSemanticSearchIndex: %v", err)
	}
	if err := index.Validate(data); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(index.Suggestions) < minSemanticSearchSuggestions || len(index.Suggestions) > maxSemanticSearchSuggestions {
		t.Fatalf("suggestions = %d, want %d..%d", len(index.Suggestions), minSemanticSearchSuggestions, maxSemanticSearchSuggestions)
	}

	items := make(map[SemanticSearchKind][]SemanticSearchItem)
	itemByID := make(map[string]SemanticSearchItem)
	for _, item := range index.Items {
		items[item.Kind] = append(items[item.Kind], item)
		itemByID[item.ID] = item
	}
	component := findSemanticSearchItem(t, items[SemanticSearchKindComponent], "Analysis core")
	if !stringSliceContains(component.Aliases, "CollectFacts") {
		t.Fatalf("component aliases = %#v, want member name", component.Aliases)
	}
	step := findSemanticSearchItem(t, items[SemanticSearchKindFlowStep], "Collect local facts")
	if step.Target.Kind != SemanticSearchTargetFlowStep || step.Target.FlowID != "flow-main" || step.Target.StepID != "step-1" {
		t.Fatalf("flow step target = %#v", step.Target)
	}
	artifact := findSemanticSearchItem(t, items[SemanticSearchKindMechanism], "How facts become a report")
	if artifact.Question != "How does repomap turn local facts into HTML?" {
		t.Fatalf("artifact question = %q", artifact.Question)
	}
	if artifact.Target.Kind != SemanticSearchTargetArtifact || artifact.Target.ArtifactID != "artifact-report" {
		t.Fatalf("artifact target = %#v", artifact.Target)
	}
	if !stringSliceContains(artifact.Aliases, "Render saved report") {
		t.Fatalf("artifact aliases = %#v, want published mechanism step", artifact.Aliases)
	}
	if stringSliceContains(artifact.Aliases, "where is report rendering") {
		t.Fatalf("artifact aliases = %#v, raw artifact metadata leaked into user search", artifact.Aliases)
	}
	location := findSemanticSearchItem(t, items[SemanticSearchKindLocation], "internal/report/report.go")
	if location.Target.Location == nil || location.Target.Location.Path != "internal/report/report.go" {
		t.Fatalf("location target = %#v", location.Target)
	}

	for _, kind := range []SemanticSearchKind{
		SemanticSearchKindGuidedTour,
		SemanticSearchKindGuidedStep,
		SemanticSearchKindDirection,
		SemanticSearchKindDomainTerm,
		SemanticSearchKindWarning,
		SemanticSearchKindUnknown,
	} {
		if got := items[kind]; len(got) != 0 {
			t.Fatalf("internal kind %q leaked into user search: %#v", kind, got)
		}
	}

	for _, suggestionID := range index.Suggestions {
		item := itemByID[suggestionID]
		if !item.Target.directlyActionable() {
			t.Fatalf("suggestion %q target = %#v", suggestionID, item.Target)
		}
		if item.Kind == SemanticSearchKindMember {
			t.Fatalf("exact member %q leaked into semantic suggestions", suggestionID)
		}
	}

	again, err := BuildSemanticSearchIndex(data)
	if err != nil {
		t.Fatalf("second BuildSemanticSearchIndex: %v", err)
	}
	if !reflect.DeepEqual(index, again) {
		t.Fatal("semantic index is not deterministic")
	}
}

func TestSemanticArtifactSearchIndexVersion(t *testing.T) {
	if SemanticSearchIndexVersion != 5 {
		t.Fatalf("SemanticSearchIndexVersion = %d, want 5 for paved path targets", SemanticSearchIndexVersion)
	}
}

func TestSemanticSearchIndexesStudyDirectionsWithDirectRoutes(t *testing.T) {
	t.Parallel()

	data := semanticSearchTestReport()
	data.StudyMap = &RepositoryStudyMap{
		Directions: []StudyDirection{
			{
				ID: "study-reading", Question: "How should I study report generation?",
				LearningOutcome: "The reader can locate the public and core report code.",
				SearchQueries:   []string{"report generation reading path", "как изучить отчет"},
				PrincipalAnchors: []StudyCodeAnchor{
					{Path: "internal/report/report.go", Symbol: "Generate", Line: 88},
				},
			},
			{
				ID: "study-ready", Question: "How do local facts become HTML?",
				LearningOutcome: "The reader can open the accepted implementation path.",
				MechanismID:     "artifact-report",
			},
		},
		HiddenDirections: []StudyDirection{
			{
				ID: "study-hidden", Question: "What weak debugging path was hidden?",
				LearningOutcome: "This should remain debug-only.",
				SearchQueries:   []string{"hidden weak debugging path"},
			},
		},
	}
	index, err := BuildSemanticSearchIndex(data)
	if err != nil {
		t.Fatal(err)
	}
	reading := findSemanticSearchItem(t, index.Items, "How should I study report generation?")
	if reading.Kind != SemanticSearchKindStudyDirection || reading.Target.Kind != SemanticSearchTargetStudy ||
		reading.Target.DirectionID != "study-reading" {
		t.Fatalf("reading direction = %#v", reading)
	}
	for _, alias := range []string{"Generate", "internal/report/report.go", "как изучить отчет"} {
		if !stringSliceContains(reading.Aliases, alias) {
			t.Fatalf("reading aliases = %#v, want %q", reading.Aliases, alias)
		}
	}
	ready := findSemanticSearchItem(t, index.Items, "How do local facts become HTML?")
	if ready.Target.Kind != SemanticSearchTargetArtifact || ready.Target.ArtifactID != "artifact-report" {
		t.Fatalf("attached direction target = %#v", ready.Target)
	}
	for _, item := range index.Items {
		raw, err := json.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "weak debugging path") {
			t.Fatalf("hidden Study direction leaked into search: %#v", item)
		}
	}
}

func TestSemanticSearchIndexesAcceptedPhasesAndProductQueries(t *testing.T) {
	t.Parallel()

	data := semanticSearchTestReport()
	mechanism := &data.UserMechanisms[0]
	mechanism.SearchQueries = []string{"report rendering", "как строится отчёт"}
	mechanism.Phases = []UserMechanismPhase{{
		Title:                     "Validate and render",
		Explanation:               "Validated facts become the report.",
		ImplementationStepIndexes: []int{1},
		Sources: []SourceSnippet{{
			Path: "internal/report/report.go", EnclosingSymbol: "Generate",
			StartLine: 88, EndLine: 88,
			Lines: []SourceSnippetLine{{Line: 88, Text: "func Generate() {}"}},
		}},
	}}
	index, err := BuildSemanticSearchIndex(data)
	if err != nil {
		t.Fatal(err)
	}
	phase := findSemanticSearchItem(t, index.Items, "Validate and render")
	if phase.Target.Kind != SemanticSearchTargetArtifact || phase.Target.StepIndex == nil ||
		*phase.Target.StepIndex != 1 {
		t.Fatalf("phase target = %#v", phase.Target)
	}
	for _, alias := range []string{"Generate", "как строится отчёт"} {
		if !stringSliceContains(phase.Aliases, alias) {
			t.Fatalf("phase aliases = %#v, want %q", phase.Aliases, alias)
		}
	}
}

func TestSemanticSearchOmitsArchitectureWhenGuideCannotUseIt(t *testing.T) {
	t.Parallel()

	data := semanticSearchTestReport()
	data.RepositoryGuide = &RepositoryGuide{Purpose: "Explain the repository."}
	index, err := BuildSemanticSearchIndex(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range index.Items {
		switch item.Kind {
		case SemanticSearchKindMap, SemanticSearchKindSubsystem,
			SemanticSearchKindComponent, SemanticSearchKindMember,
			SemanticSearchKindFlow, SemanticSearchKindFlowStep,
			SemanticSearchKindSurface:
			t.Fatalf("hidden architecture item was indexed: %#v", item)
		}
	}
	findSemanticSearchItem(t, index.Items, "How facts become a report")
}

func TestSemanticSearchIndexRejectsUngroundedTargets(t *testing.T) {
	data := semanticSearchTestReport()
	data.SemanticArtifacts = append(data.SemanticArtifacts, semanticdiscovery.Artifact{
		Version:    semanticdiscovery.ArtifactVersion,
		ID:         "artifact-stale",
		Kind:       semanticdiscovery.ArtifactMechanism,
		Title:      "Unpublished raw mechanism",
		Question:   "Should raw artifacts be searchable?",
		Verdict:    semanticdiscovery.VerdictSupported,
		Confidence: semanticdiscovery.ConfidenceHigh,
	})
	valid, err := BuildSemanticSearchIndex(data)
	if err != nil {
		t.Fatalf("BuildSemanticSearchIndex: %v", err)
	}
	for _, item := range valid.Items {
		if item.Target.ArtifactID == "artifact-stale" || item.Title == "Unpublished raw mechanism" {
			t.Fatalf("raw artifact without a UserMechanism projection was indexed: %#v", item)
		}
	}

	tests := []struct {
		name   string
		mutate func(*SemanticSearchIndex)
		want   string
	}{
		{
			name: "unknown flow",
			mutate: func(index *SemanticSearchIndex) {
				item := semanticSearchItemIndex(t, index.Items, SemanticSearchKindFlow)
				index.Items[item].Target.FlowID = "missing"
			},
			want: "not present on the canvas",
		},
		{
			name: "unknown flow step",
			mutate: func(index *SemanticSearchIndex) {
				item := semanticSearchItemIndex(t, index.Items, SemanticSearchKindFlowStep)
				index.Items[item].Target.StepID = "missing"
			},
			want: "flow step target",
		},
		{
			name: "unopenable location",
			mutate: func(index *SemanticSearchIndex) {
				item := semanticSearchItemIndex(t, index.Items, SemanticSearchKindLocation)
				index.Items[item].Target.Location.Path = "ignored/secret"
			},
			want: "is not openable",
		},
		{
			name: "unknown user mechanism",
			mutate: func(index *SemanticSearchIndex) {
				item := semanticSearchItemIndex(t, index.Items, SemanticSearchKindMechanism)
				index.Items[item].Target.ArtifactID = "missing"
			},
			want: "not present in the user report",
		},
		{
			name: "raw artifact without user projection",
			mutate: func(index *SemanticSearchIndex) {
				item := semanticSearchItemIndex(t, index.Items, SemanticSearchKindMechanism)
				index.Items[item].Target.ArtifactID = "artifact-stale"
			},
			want: "not present in the user report",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			index := cloneSemanticSearchIndex(valid)
			test.mutate(&index)
			err := index.Validate(data)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestBuildSemanticSearchIndexBoundsAndTruncates(t *testing.T) {
	data := &ReportData{
		RepoName:     strings.Repeat("界", maxSemanticSearchTitleBytes),
		ProjectGuess: strings.Repeat("summary", maxSemanticSearchSummaryBytes),
	}
	for index := 0; index < maxSemanticSearchItems+100; index++ {
		data.OpenablePaths = append(data.OpenablePaths, fmt.Sprintf("pkg/%04d/file.go", index))
	}

	result, err := BuildSemanticSearchIndex(data)
	if err != nil {
		t.Fatalf("BuildSemanticSearchIndex: %v", err)
	}
	if !result.Truncated {
		t.Fatal("Truncated = false, want true")
	}
	if len(result.Items) != maxSemanticSearchItems {
		t.Fatalf("items = %d, want %d", len(result.Items), maxSemanticSearchItems)
	}
	for _, item := range result.Items {
		if len(item.Title) > maxSemanticSearchTitleBytes || !utf8.ValidString(item.Title) {
			t.Fatalf("invalid bounded title %q", item.Title)
		}
		if len(item.Summary) > maxSemanticSearchSummaryBytes {
			t.Fatalf("summary bytes = %d", len(item.Summary))
		}
		if len(item.Aliases) > maxSemanticSearchAliases {
			t.Fatalf("aliases = %d", len(item.Aliases))
		}
	}
	if len(result.Suggestions) != 1 {
		t.Fatalf("suggestions = %d, want only the semantic map for a locations-only report", len(result.Suggestions))
	}
}

func TestBuildSemanticSearchIndexOmitsInternalSemanticInputs(t *testing.T) {
	data := semanticSearchTestReport()
	data.CandidateDirections = append(data.CandidateDirections, CandidateDirection{
		ID:             "rejected-direction",
		Name:           "Rejected wrapper path",
		WhyInteresting: "The model proposed this path without enough local support.",
		Disposition:    flowexplain.DirectionRejected,
	})

	index, err := BuildSemanticSearchIndex(data)
	if err != nil {
		t.Fatalf("BuildSemanticSearchIndex: %v", err)
	}
	for _, item := range index.Items {
		switch item.Kind {
		case SemanticSearchKindDirection,
			SemanticSearchKindGuidedTour,
			SemanticSearchKindGuidedStep,
			SemanticSearchKindDomainTerm,
			SemanticSearchKindWarning,
			SemanticSearchKindUnknown:
			t.Fatalf("internal semantic item was indexed: %#v", item)
		}
	}
	findSemanticSearchItem(t, index.Items, "How facts become a report")
}

func TestBuildSemanticSearchIndexKeepsExactMembersBeyondAliasLimit(t *testing.T) {
	data := semanticSearchTestReport()
	component := &data.ArchitectureCanvas.Components[0]
	component.Members = nil
	for memberIndex := 0; memberIndex < maxSemanticSearchAliases+2; memberIndex++ {
		component.Members = append(component.Members, componentmap.Candidate{
			ID: componentmap.MemberID{
				Kind:  componentmap.MemberPackage,
				Value: fmt.Sprintf("opaque-package-%02d", memberIndex),
			},
			Name: fmt.Sprintf("github.com/example/repo/internal/pkg%02d", memberIndex),
		})
	}
	wantMember := component.Members[len(component.Members)-1].Name

	index, err := BuildSemanticSearchIndex(data)
	if err != nil {
		t.Fatalf("BuildSemanticSearchIndex: %v", err)
	}
	owner := findSemanticSearchItem(t, index.Items, component.Name)
	if stringSliceContains(owner.Aliases, wantMember) {
		t.Fatalf("test member %q unexpectedly survived the owner alias bound", wantMember)
	}
	member := findSemanticSearchItem(t, index.Items, wantMember)
	if member.Kind != SemanticSearchKindMember {
		t.Fatalf("member kind = %q, want %q", member.Kind, SemanticSearchKindMember)
	}
	if member.Target.Kind != SemanticSearchTargetComponent || member.Target.ComponentID != component.ID {
		t.Fatalf("member target = %#v, want owning component %q", member.Target, component.ID)
	}
	if member.ID == owner.ID {
		t.Fatalf("member item reused owning component id %q", owner.ID)
	}
}

func TestBuildSemanticSearchIndexRequiresReport(t *testing.T) {
	if _, err := BuildSemanticSearchIndex(nil); err == nil {
		t.Fatal("BuildSemanticSearchIndex(nil) succeeded")
	}
}

func semanticSearchTestReport() *ReportData {
	return &ReportData{
		RepoName:     "repomap",
		ProjectGuess: "repository orientation",
		OpenablePaths: []string{
			"cmd/repomap/main.go",
			"internal/report/report.go",
		},
		ArchitectureCanvas: &ArchitectureCanvas{
			Title:    "How repomap becomes a report",
			Subtitle: "Local facts are connected to verified presentation objects.",
			Subsystems: []ArchitectureSubsystem{{
				ID:           "subsystem-analysis",
				Name:         "Analysis",
				Description:  "Collects bounded repository facts.",
				ComponentIDs: []componentmap.ComponentID{"component-analysis"},
			}},
			Components: []ArchitectureComponent{{
				ID:          "component-analysis",
				SubsystemID: "subsystem-analysis",
				Name:        "Analysis core",
				Description: "Produces local facts.",
				Members: []componentmap.Candidate{{
					ID:   componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "member-collect"},
					Name: "CollectFacts",
				}},
			}},
			Flows: []ArchitectureFlow{{
				ID:          "flow-main",
				Name:        "CLI to report",
				Trigger:     "repomap ./repo",
				MentalModel: "Collect facts, interpret them, then render the report.",
				Steps: []ArchitectureFlowStep{{
					ID:            "step-1",
					Label:         "Collect local facts",
					QualifiedName: "snapshot.Collect",
				}},
			}},
			Surfaces: []ArchitectureSurface{{
				ID:       "surface-cli",
				Name:     "repomap CLI",
				Source:   surfaceSourceCatalog,
				Kind:     "cli_command",
				Category: "application",
			}},
			Suggestions: []ArchitectureSuggestion{{
				ID:                   "direction-report",
				Title:                "Inspect report rendering",
				Reason:               "The report is the final user-visible boundary.",
				RelevantComponentIDs: []componentmap.ComponentID{"component-analysis"},
			}},
		},
		GuidedTour: &guidedtour.Story{
			CandidateID:   "flow-main",
			CandidateName: "CLI to report",
			CandidateKind: guidedtour.CandidateSavedTrace,
			Trigger:       "repomap ./repo",
			Title:         "From checkout to report",
			Summary:       "Follow one verified path through the project.",
			Steps: []guidedtour.StoryStep{
				{
					Title:       "Collect facts",
					Explanation: "The CLI first records bounded local evidence.",
					Beats: []guidedtour.Beat{{
						ID:    "beat-1",
						Label: "Snapshot repository facts",
					}},
					Components: []guidedtour.Component{{ID: "component-analysis", Name: "Analysis core"}},
				},
				{Title: "Render report", Explanation: "The saved projection is embedded in HTML."},
			},
		},
		SemanticArtifacts: []semanticdiscovery.Artifact{{
			Version:         semanticdiscovery.ArtifactVersion,
			ID:              "artifact-report",
			Kind:            semanticdiscovery.ArtifactMechanism,
			Title:           "How facts become a report",
			Summary:         "Bounded local facts are projected into the existing HTML report.",
			Question:        "How does repomap turn local facts into HTML?",
			Verdict:         semanticdiscovery.VerdictSupported,
			Aliases:         []string{"report pipeline"},
			LikelyQuestions: []string{"where is report rendering"},
			Confidence:      semanticdiscovery.ConfidenceHigh,
		}},
		UserMechanisms: []UserMechanism{{
			ArtifactID: "artifact-report",
			Title:      "How facts become a report",
			Question:   "How does repomap turn local facts into HTML?",
			Answer:     "Source-backed path: Collect local facts → Render saved report.",
			Steps: []UserMechanismStep{
				{
					Title:       "Collect local facts",
					Explanation: "The CLI records bounded local facts before report rendering.",
					Locations: []UserCodeLocation{{
						Path: "cmd/repomap/main.go",
						Line: 42,
					}},
				},
				{
					Title:       "Render saved report",
					Explanation: "The saved projection is embedded in the production HTML report.",
					Locations: []UserCodeLocation{{
						Path: "internal/report/report.go",
						Line: 88,
					}},
				},
			},
			Files: []UserCodeLocation{
				{Path: "cmd/repomap/main.go", Line: 42},
				{Path: "internal/report/report.go", Line: 88},
			},
		}},
		CandidateDirections: []CandidateDirection{{
			ID:               "flow-main",
			Name:             "CLI to report",
			Trigger:          "repomap ./repo",
			LikelyEntrypoint: "cmd/repomap/main.go",
			WhyInteresting:   "It crosses the central product pipeline.",
		}},
		ImportantDomainWords: []DomainWord{{
			Word:     "surface",
			Guess:    "A locally discovered runtime entry.",
			Evidence: []string{"trigger_catalog.json"},
		}},
		Flows: []FlowData{{
			ID:       "flow-main",
			Name:     "CLI to report",
			Unknowns: []string{"Dynamic dispatch remains unresolved."},
			Warnings: []string{"One transition is partial."},
		}},
		Warnings: []string{"Architecture synthesis used a bounded fallback."},
	}
}

func findSemanticSearchItem(t *testing.T, items []SemanticSearchItem, title string) SemanticSearchItem {
	t.Helper()
	for _, item := range items {
		if item.Title == title {
			return item
		}
	}
	t.Fatalf("semantic search item %q not found in %#v", title, items)
	return SemanticSearchItem{}
}

func semanticSearchItemIndex(t *testing.T, items []SemanticSearchItem, kind SemanticSearchKind) int {
	t.Helper()
	for index, item := range items {
		if item.Kind == kind {
			return index
		}
	}
	t.Fatalf("semantic search kind %q not found", kind)
	return -1
}

func cloneSemanticSearchIndex(index SemanticSearchIndex) SemanticSearchIndex {
	clone := index
	clone.Items = append([]SemanticSearchItem(nil), index.Items...)
	clone.Suggestions = append([]string(nil), index.Suggestions...)
	for itemIndex := range clone.Items {
		clone.Items[itemIndex].Aliases = append([]string(nil), index.Items[itemIndex].Aliases...)
		if index.Items[itemIndex].Target.StepIndex != nil {
			step := *index.Items[itemIndex].Target.StepIndex
			clone.Items[itemIndex].Target.StepIndex = &step
		}
		if index.Items[itemIndex].Target.Location != nil {
			location := *index.Items[itemIndex].Target.Location
			clone.Items[itemIndex].Target.Location = &location
		}
	}
	return clone
}
