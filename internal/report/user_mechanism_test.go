package report

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/goldenmechanism"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

func TestProjectUserMechanismKeepsOnlySupportedSourceBackedSteps(t *testing.T) {
	t.Parallel()

	componentID := componentmap.ComponentID("component-router")
	data := &ReportData{
		OpenablePaths: []string{"gap.go", "interpretation.go", "mux.go", "tree.go"},
		ArchitectureCanvas: &ArchitectureCanvas{
			Components: []ArchitectureComponent{{ID: componentID}},
		},
	}
	artifact := semanticdiscovery.Artifact{
		ID:       "artifact-dispatch",
		Kind:     semanticdiscovery.ArtifactMechanism,
		Verdict:  semanticdiscovery.VerdictMixed,
		Title:    "Request dispatch",
		Question: "How does a request reach its handler?",
		Summary:  "ServeHTTP prepares the route context, and the router looks up and invokes the selected handler.",
		Focus:    semanticdiscovery.Focus{ComponentIDs: []string{string(componentID)}},
		Statements: []semanticdiscovery.Statement{
			{ID: "entry", Text: "ServeHTTP prepares the route context.", Basis: semanticdiscovery.ClaimDirect},
			{ID: "route", Text: "The router looks up and invokes the selected handler.", Basis: semanticdiscovery.ClaimCompositional},
			{ID: "gap", Text: "Evidence gap: middleware lifetime is unknown.", Basis: semanticdiscovery.ClaimUnresolved},
			{ID: "interpretation", Text: "This probably optimizes the common case.", Basis: semanticdiscovery.ClaimInterpretive},
		},
		Steps: []semanticdiscovery.Step{
			{
				ID: "step-entry", Title: "Prepare request context", Explanation: "Unvalidated model step prose.", StatementIDs: []string{"entry"},
				Focus: semanticdiscovery.Focus{ComponentIDs: []string{string(componentID)}},
				Evidence: []semanticdiscovery.EvidenceRef{
					{Path: "mux.go", Line: 63, Column: 2},
					{Path: "mux.go", Line: 63, Column: 2},
					{Path: "outside.go", Line: 1},
				},
			},
			{
				ID: "step-route", Title: "Select endpoint", StatementIDs: []string{"route"},
				Evidence: []semanticdiscovery.EvidenceRef{{Path: "tree.go", Line: 382}},
			},
			{
				ID: "step-gap", Title: "Evidence gap", StatementIDs: []string{"gap"},
				Evidence: []semanticdiscovery.EvidenceRef{{Path: "gap.go", Line: 90}},
			},
			{
				ID: "step-interpretation", Title: "Likely optimization", StatementIDs: []string{"interpretation"},
				Evidence: []semanticdiscovery.EvidenceRef{{Path: "interpretation.go", Line: 12}},
			},
		},
	}
	probe := userMechanismProbe(
		userMechanismSource{"mux.go", "ServeHTTP", 60, 8},
		userMechanismSource{"tree.go", "FindRoute", 379, 8},
		userMechanismSource{"gap.go", "Gap", 87, 8},
		userMechanismSource{"interpretation.go", "Interpretation", 9, 8},
	)

	mechanism, ok := projectUserMechanism(data, artifact, probe)
	if !ok {
		t.Fatal("expected supported mechanism to be user-visible")
	}
	if len(mechanism.Steps) != 2 {
		t.Fatalf("steps = %d, want 2: %#v", len(mechanism.Steps), mechanism.Steps)
	}
	if mechanism.Answer != artifact.Summary {
		t.Fatalf("answer = %q, want canonical summary %q", mechanism.Answer, artifact.Summary)
	}
	if mechanism.Answer == artifact.Statements[0].Text {
		t.Fatalf("first step masquerades as whole-mechanism answer: %q", mechanism.Answer)
	}
	for _, concept := range []string{"route context", "looks up", "invokes"} {
		if !strings.Contains(mechanism.Answer, concept) {
			t.Fatalf("answer %q does not cover %q", mechanism.Answer, concept)
		}
	}
	if strings.Contains(mechanism.Answer, "Source-backed path") {
		t.Fatalf("answer duplicates the step navigator: %q", mechanism.Answer)
	}
	if strings.Contains(strings.ToLower(mechanism.Answer), "evidence gap") ||
		strings.Contains(strings.ToLower(mechanism.Answer), "probably") {
		t.Fatalf("answer leaks internal gap wording: %q", mechanism.Answer)
	}
	if mechanism.Steps[0].Explanation != artifact.Statements[0].Text {
		t.Fatalf("step explanation = %q", mechanism.Steps[0].Explanation)
	}
	if len(mechanism.Steps[0].Locations) != 1 || mechanism.Steps[0].Locations[0].Path != "mux.go" {
		t.Fatalf("locations = %#v", mechanism.Steps[0].Locations)
	}
	if mechanism.Steps[0].MapTarget == nil || mechanism.Steps[0].MapTarget.ComponentID != componentID {
		t.Fatalf("map target = %#v", mechanism.Steps[0].MapTarget)
	}
	if mechanism.Steps[1].MapTarget != nil {
		t.Fatalf("empty focus produced map target: %#v", mechanism.Steps[1].MapTarget)
	}
	if len(mechanism.Files) != 2 ||
		mechanism.Files[0] != (UserCodeLocation{Path: "mux.go", Line: 63, Column: 2}) ||
		mechanism.Files[1] != (UserCodeLocation{Path: "tree.go", Line: 382}) {
		t.Fatalf("files = %#v", mechanism.Files)
	}
	for _, step := range mechanism.Steps {
		visible := strings.ToLower(step.Title + " " + step.Explanation)
		if strings.Contains(visible, "gap") || strings.Contains(visible, "probably") {
			t.Fatalf("unsupported step leaked into projection: %#v", step)
		}
		for _, location := range step.Locations {
			if location.Path == "gap.go" || location.Path == "interpretation.go" || location.Path == "outside.go" {
				t.Fatalf("unsupported or non-openable location leaked into projection: %#v", location)
			}
		}
	}
}

func TestUserMechanismAnswerOmitsUnsafeOrStepOnlySummary(t *testing.T) {
	t.Parallel()

	steps := []UserMechanismStep{{
		Explanation: "The handler prepares the route context.",
	}}
	tests := []struct {
		name    string
		summary string
		want    string
	}{
		{
			name:    "safe whole mechanism summary",
			summary: "The handler prepares context, finds a route, and calls the endpoint or a not-found fallback.",
			want:    "The handler prepares context, finds a route, and calls the endpoint or a not-found fallback.",
		},
		{name: "empty summary"},
		{
			name:    "gap-only summary",
			summary: "Evidence gap: full middleware behavior is not established.",
		},
		{
			name:    "summary repeats first step",
			summary: "The handler prepares the route context!",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := userMechanismAnswer(test.summary, steps); got != test.want {
				t.Fatalf("userMechanismAnswer() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestUserMechanismAnswerAddsOnlyMissingAcceptedStepTopic(t *testing.T) {
	t.Parallel()

	summary := "The file server enters directory browsing, collects entries, sorts and paginates them, and formats the output."
	steps := []UserMechanismStep{
		{canonicalTitle: "Directory browsing entry"},
		{canonicalTitle: "Directory listing item collection"},
		{canonicalTitle: "Sorting and pagination application"},
		{canonicalTitle: "Response format selection and output"},
		{
			canonicalTitle: "Redirect and error paths",
			Explanation:    "A long accepted claim describes redirects, forbidden results, not-found results, and template failures.",
		},
	}

	got := userMechanismAnswer(summary, steps)
	if !strings.HasPrefix(got, summary) || !strings.Contains(got, "redirect and error paths") {
		t.Fatalf("whole-mechanism answer = %q", got)
	}
	if strings.Contains(got, steps[len(steps)-1].Explanation) {
		t.Fatalf("answer copied the full step explanation instead of its accepted topic: %q", got)
	}
}

func TestUserMechanismStepTitleUsesRepositoryAgnosticActionLabels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		title       string
		explanation string
		want        string
	}{
		{
			name: "browse entry", title: "Directory browsing entry",
			explanation: "The request handler enters directory browsing when the target remains a directory.",
			want:        "Enter directory browsing",
		},
		{
			name: "query options", title: "Query option handling",
			explanation: "Query handling reads layout, limit, offset, sort, and order.",
			want:        "Read query options",
		},
		{
			name: "listing collection", title: "Directory listing item collection",
			explanation: "The collection step appends structured item records to the listing.",
			want:        "Collect listing items",
		},
		{
			name: "sorting and paging", title: "Sorting and pagination application",
			explanation: "Listing transformation sorts and slices the item list for pagination.",
			want:        "Sort and paginate",
		},
		{
			name: "response output", title: "Response format selection and output",
			explanation: "JSON encoding writes the selected representation to the response writer.",
			want:        "Encode and write the response",
		},
		{
			name: "alternate paths", title: "Redirect and error paths",
			explanation: "Material alternative paths include redirects and error outcomes.",
			want:        "Handle alternate paths",
		},
		{
			name: "routing context", title: "Request entry, context preparation, and computed handler invocation",
			explanation: "The handler obtains and resets a route context, attaches it, and invokes the computed handler.",
			want:        "Prepare the routing context",
		},
		{
			name: "route lookup", title: "Route lookup and parameter context update",
			explanation: "The routing method performs lookup and copies matched parameter values onto the request.",
			want:        "Find the route and copy parameters",
		},
		{
			name: "endpoint", title: "Selected endpoint handler invocation",
			explanation: "The successful branch invokes the selected endpoint handler.",
			want:        "Call the endpoint",
		},
		{
			name: "fallback", title: "Not-found and method-not-allowed fallback",
			explanation: "The routing method selects the appropriate fallback handler.",
			want:        "Choose a fallback",
		},
		{
			name: "unrecognized title is preserved", title: "Persist the generated manifest",
			explanation: "The writer persists the generated manifest.",
			want:        "Persist the generated manifest",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := userMechanismStepTitle(test.title, test.explanation); got != test.want {
				t.Fatalf("userMechanismStepTitle() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestProjectUserMechanismAppliesVerdictAndMinimumStepEligibility(t *testing.T) {
	t.Parallel()

	data := &ReportData{OpenablePaths: []string{"entry.go", "dispatch.go"}}
	probe := userMechanismProbe(
		userMechanismSource{"entry.go", "Entry", 8, 8},
		userMechanismSource{"dispatch.go", "Dispatch", 38, 8},
	)
	baseArtifact := func(verdict semanticdiscovery.Verdict) semanticdiscovery.Artifact {
		return semanticdiscovery.Artifact{
			ID:       "artifact-dispatch",
			Kind:     semanticdiscovery.ArtifactMechanism,
			Verdict:  verdict,
			Title:    "Request dispatch",
			Question: "How does a request reach its handler?",
			Statements: []semanticdiscovery.Statement{
				{ID: "entry", Text: "ServeHTTP receives the request.", Basis: semanticdiscovery.ClaimDirect},
				{ID: "dispatch", Text: "The selected handler receives the request.", Basis: semanticdiscovery.ClaimCompositional},
			},
			Steps: []semanticdiscovery.Step{
				{
					ID: "entry-step", Title: "Receive request", StatementIDs: []string{"entry"},
					Evidence: []semanticdiscovery.EvidenceRef{{Path: "entry.go", Line: 10}},
				},
				{
					ID: "dispatch-step", Title: "Invoke handler", StatementIDs: []string{"dispatch"},
					Evidence: []semanticdiscovery.EvidenceRef{{Path: "dispatch.go", Line: 40}},
				},
			},
		}
	}

	tests := []struct {
		name      string
		artifact  semanticdiscovery.Artifact
		wantShown bool
	}{
		{
			name:      "supported verdict is published",
			artifact:  baseArtifact(semanticdiscovery.VerdictSupported),
			wantShown: true,
		},
		{
			name:      "mixed verdict publishes supported path",
			artifact:  baseArtifact(semanticdiscovery.VerdictMixed),
			wantShown: true,
		},
		{
			name:     "unsupported verdict is hidden",
			artifact: baseArtifact(semanticdiscovery.VerdictUnsupported),
		},
		{
			name:     "insufficient evidence verdict is hidden",
			artifact: baseArtifact(semanticdiscovery.VerdictInsufficientEvidence),
		},
		{
			name: "one source backed step is not a mechanism",
			artifact: func() semanticdiscovery.Artifact {
				artifact := baseArtifact(semanticdiscovery.VerdictSupported)
				artifact.Steps = artifact.Steps[:1]
				return artifact
			}(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			mechanism, shown := projectUserMechanism(data, test.artifact, probe)
			if shown != test.wantShown {
				t.Fatalf("published = %v, want %v; mechanism = %#v", shown, test.wantShown, mechanism)
			}
			if shown && len(mechanism.Steps) != 2 {
				t.Fatalf("published steps = %d, want 2", len(mechanism.Steps))
			}
		})
	}
}

func TestProjectUserMechanismUsesHumanTitleWithoutChangingSemanticAuthority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		repoName      string
		modelTitle    string
		question      string
		expectedTitle string
	}{
		{
			name:          "caddy directory listing",
			repoName:      "caddy",
			modelTitle:    "File Server Directory Listing Sorting and Pagination Mechanism",
			question:      "How does the file server generate and sort directory listings?",
			expectedTitle: "How Caddy builds directory listings",
		},
		{
			name:          "chi request dispatch",
			repoName:      "chi",
			modelTitle:    "Chi Request Dispatch Mechanism v1",
			question:      "How does Mux dispatch an HTTP request to an endpoint, not found, or method not allowed?",
			expectedTitle: "How chi dispatches an HTTP request",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			data, artifact, probe := sourceProjectionFixture()
			data.RepoName = test.repoName
			artifact.Title = test.modelTitle
			artifact.Question = test.question
			artifact.Summary = "The primary function coordinates related helpers and returns the result."
			artifactBefore := string(mustSourceProjectionJSON(t, artifact))

			canonical := userMechanismCanonicalFixture(artifact.CandidateID)
			canonical.Payload.Proposal = semanticdiscovery.ArtifactProposal{
				CandidateID: artifact.CandidateID,
				Summary:     artifact.Summary,
			}
			canonicalID := canonical.ID
			hashBefore, err := semanticdiscovery.MechanismContentHash(canonical)
			if err != nil {
				t.Fatal(err)
			}

			mechanism, ok := projectUserMechanism(data, artifact, probe)
			if !ok {
				t.Fatal("expected source-backed mechanism to be user-visible")
			}
			if mechanism.Title != test.expectedTitle {
				t.Fatalf("title = %q, want %q", mechanism.Title, test.expectedTitle)
			}
			if mechanism.Title == test.modelTitle || strings.Contains(mechanism.Title, "Mechanism") {
				t.Fatalf("machine-like title leaked into user projection: %q", mechanism.Title)
			}
			if mechanism.ArtifactID != artifact.ID {
				t.Fatalf("artifact id = %q, want %q", mechanism.ArtifactID, artifact.ID)
			}
			if mechanism.Answer != artifact.Summary {
				t.Fatalf("answer = %q, want accepted summary %q", mechanism.Answer, artifact.Summary)
			}
			if got := string(mustSourceProjectionJSON(t, artifact)); got != artifactBefore {
				t.Fatalf("presentation projection changed canonical artifact:\nbefore: %s\nafter:  %s", artifactBefore, got)
			}
			hashAfter, err := semanticdiscovery.MechanismContentHash(canonical)
			if err != nil {
				t.Fatal(err)
			}
			if canonical.ID != canonicalID || hashAfter != hashBefore {
				t.Fatalf("canonical authority changed: id %q -> %q, hash %q -> %q",
					canonicalID, canonical.ID, hashBefore, hashAfter)
			}
		})
	}
}

func TestProjectUserMechanismAttachesOnlySourceSupportedNotices(t *testing.T) {
	t.Parallel()

	const sourcePath = "internal/router.go"
	data := &ReportData{
		OpenablePaths: []string{sourcePath},
		SemanticSupplementalFacts: []semanticdiscovery.Fact{
			{
				ID: "fact-check",
				Capabilities: []semanticdiscovery.Capability{
					semanticdiscovery.CapabilityBranch,
				},
				Evidence: []semanticdiscovery.EvidenceRef{
					{ID: "evidence-check", Path: sourcePath, Line: 42},
				},
			},
			{
				ID: "fact-dispatch",
				Capabilities: []semanticdiscovery.Capability{
					semanticdiscovery.CapabilityDirectCall,
				},
				Evidence: []semanticdiscovery.EvidenceRef{
					{ID: "evidence-dispatch", Path: sourcePath, Line: 45},
				},
			},
		},
	}
	artifact := semanticdiscovery.Artifact{
		ID: "artifact-notices", Kind: semanticdiscovery.ArtifactMechanism,
		Verdict:  semanticdiscovery.VerdictSupported,
		Question: "How does the router dispatch a request?",
		Statements: []semanticdiscovery.Statement{
			{
				ID: "statement-check", Text: "The handler checks the route.",
				Basis: semanticdiscovery.ClaimDirect, SupportIDs: []string{"fact-check"},
			},
			{
				ID: "statement-dispatch", Text: "The handler dispatches the matched endpoint.",
				Basis: semanticdiscovery.ClaimDirect, SupportIDs: []string{"fact-dispatch"},
			},
		},
		Steps: []semanticdiscovery.Step{
			{
				ID: "step-check", Title: "Check route", StatementIDs: []string{"statement-check"},
				Evidence: []semanticdiscovery.EvidenceRef{
					{ID: "evidence-check", Path: sourcePath, Line: 42},
				},
			},
			{
				ID: "step-dispatch", Title: "Dispatch endpoint", StatementIDs: []string{"statement-dispatch"},
				Evidence: []semanticdiscovery.EvidenceRef{
					{ID: "evidence-dispatch", Path: sourcePath, Line: 45},
				},
			},
		},
	}
	probe := userMechanismProbe(userMechanismSource{sourcePath, "Dispatch", 30, 30})
	probe.Observations = []goldenmechanism.Observation{
		sourceProjectionObservation(
			"observation-check", "evidence-check", sourcePath, 42,
			semanticdiscovery.CapabilityBranch, goldenmechanism.BasisBranch,
			"route != nil", "",
		),
		sourceProjectionObservation(
			"observation-dispatch", "evidence-dispatch", sourcePath, 45,
			semanticdiscovery.CapabilityDirectCall, goldenmechanism.BasisDirectCall,
			"", "endpoint.ServeHTTP",
		),
	}
	wantNotices := []string{"Checks route != nil.", "Calls endpoint.ServeHTTP."}

	mechanism, ok := projectUserMechanism(data, artifact, probe)
	if !ok {
		t.Fatal("expected source-backed mechanism to be user-visible")
	}
	for index, step := range mechanism.Steps {
		if len(step.WhatToNotice) != 1 {
			t.Fatalf("step[%d] notices = %#v, want one supported notice", index, step.WhatToNotice)
		}
		notice := step.WhatToNotice[0]
		if notice.Text != wantNotices[index] {
			t.Fatalf("step[%d] notice = %q, want line-scoped callout %q",
				index, notice.Text, wantNotices[index])
		}
		if err := notice.Validate(); err != nil {
			t.Fatalf("step[%d] notice is invalid: %v", index, err)
		}
		for _, sourceRange := range notice.SupportingRanges {
			if !sourceNoticeRangeVisible(step.Sources, notice.Path, sourceRange) {
				t.Fatalf("step[%d] notice range %#v is outside highlighted source", index, sourceRange)
			}
		}
	}
}

func sourceNoticeRangeVisible(sources []SourceSnippet, sourcePath string, sourceRange SourceHighlight) bool {
	for _, source := range sources {
		if source.Path == sourcePath && sourceSnippetContainsRange(source.Lines, sourceRange) &&
			sourceLineIsHighlighted(sourceRange.StartLine, source.HighlightRanges) &&
			sourceLineIsHighlighted(sourceRange.EndLine, source.HighlightRanges) {
			return true
		}
	}
	return false
}

type userMechanismSource struct {
	path      string
	symbol    string
	startLine int
	lineCount int
}

func userMechanismProbe(sources ...userMechanismSource) goldenmechanism.Result {
	functions := make([]goldenmechanism.Function, 0, len(sources))
	for _, source := range sources {
		functions = append(functions, sourceProjectionFunction(
			source.path,
			source.symbol,
			source.startLine,
			source.lineCount,
		))
	}
	return goldenmechanism.Result{Functions: functions}
}

func userMechanismCanonicalFixture(candidateID string) semanticdiscovery.Mechanism {
	return semanticdiscovery.Mechanism{
		Version: semanticdiscovery.MechanismVersion,
		ID:      "semantic-mechanism-user-title",
		Identity: semanticdiscovery.MechanismIdentity{
			RepositoryNamespace: "example.com/repository",
			IntentKey:           "request-dispatch",
			Scope: semanticdiscovery.MechanismScope{
				Kind:  semanticdiscovery.MechanismScopeGoPackage,
				Value: "example.com/repository/internal",
			},
		},
		Input: semanticdiscovery.MechanismInputManifest{
			Version:          semanticdiscovery.MechanismInputVersion,
			ValidatorVersion: semanticdiscovery.MechanismValidatorVersion,
		},
		Payload: semanticdiscovery.MechanismPayload{
			Version:       semanticdiscovery.MechanismPayloadVersion,
			OrderingBasis: semanticdiscovery.MechanismOrderingEditorial,
			Candidate: semanticdiscovery.OpportunityCandidate{
				ID: candidateID, Kind: semanticdiscovery.ArtifactMechanism,
			},
		},
	}
}

func TestProjectUserMechanismRejectsNonMechanismAndGapOnlyArtifacts(t *testing.T) {
	t.Parallel()

	data := &ReportData{OpenablePaths: []string{"source.go"}}
	tests := []struct {
		name     string
		artifact semanticdiscovery.Artifact
	}{
		{
			name: "non mechanism",
			artifact: semanticdiscovery.Artifact{
				ID: "pattern", Kind: semanticdiscovery.ArtifactRepositoryPattern,
				Verdict: semanticdiscovery.VerdictSupported,
			},
		},
		{
			name: "gap only",
			artifact: semanticdiscovery.Artifact{
				ID: "gap", Kind: semanticdiscovery.ArtifactMechanism,
				Verdict: semanticdiscovery.VerdictMixed,
				Statements: []semanticdiscovery.Statement{
					{ID: "unknown-entry", Text: "Unknown entry", Basis: semanticdiscovery.ClaimUnresolved},
					{ID: "unknown-exit", Text: "Unknown exit", Basis: semanticdiscovery.ClaimUnresolved},
				},
				Steps: []semanticdiscovery.Step{
					{
						ID: "gap-entry", StatementIDs: []string{"unknown-entry"},
						Evidence: []semanticdiscovery.EvidenceRef{{Path: "source.go", Line: 1}},
					},
					{
						ID: "gap-exit", StatementIDs: []string{"unknown-exit"},
						Evidence: []semanticdiscovery.EvidenceRef{{Path: "source.go", Line: 2}},
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if mechanism, ok := projectUserMechanism(data, test.artifact); ok {
				t.Fatalf("unexpected user mechanism: %#v", mechanism)
			}
		})
	}
}
