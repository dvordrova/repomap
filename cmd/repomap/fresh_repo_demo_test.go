package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/goldenmechanism"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/sourcewindowfacts"
)

func TestFreshRepoCandidatePipelineStaysBoundedAndLocallyVerifiable(t *testing.T) {
	repoRoot := t.TempDir()
	sourceLines := []string{
		"package pipeline",
		"",
		"import \"strings\"",
		"",
		"func entry(input string) string {",
		"\tvalue := load(input)",
		"\tif value == \"\" {",
		"\t\treturn fallback()",
		"\t}",
		"\treturn render(value)",
		"}",
		"",
		"func load(input string) string {",
		"\tvalue := strings.TrimSpace(input)",
		"\treturn value",
		"}",
		"",
		"func render(value string) string {",
		"\toutput := strings.ToUpper(value)",
		"\treturn output",
		"}",
		"",
		"func fallback() string {",
		"\treturn \"default\"",
		"}",
	}
	writeFile(t, filepath.Join(repoRoot, "pipeline.go"), joinLines(sourceLines))
	window, err := sourcewindowfacts.NewWindow(
		"evidence-pipeline",
		"pipeline.go",
		1,
		sourceLines,
	)
	if err != nil {
		t.Fatal(err)
	}
	functions, err := sourcewindowfacts.ExtractGoFunctions(window)
	if err != nil {
		t.Fatal(err)
	}
	if len(functions) != 4 {
		t.Fatalf("functions = %d, want 4", len(functions))
	}
	sources := make([]freshSourceFunction, 0, len(functions))
	for _, function := range functions {
		fact, factErr := freshWindowFunctionFact(function)
		if factErr != nil {
			t.Fatal(factErr)
		}
		sources = append(sources, freshSourceFunction{Function: function, Fact: fact})
	}

	planningBundle := semanticdiscovery.Bundle{
		Version:  semanticdiscovery.BundleVersion,
		RepoName: "fixture",
		Facts:    freshFacts(sources),
	}
	rawCandidate := semanticdiscovery.OpportunityCandidate{
		Kind:             semanticdiscovery.ArtifactMechanism,
		Title:            "Request processing",
		QuestionAnswered: "How does the command process an input and produce a result?",
		SupportIDs: []string{
			sources[0].Fact.ID,
			sources[1].Fact.ID,
			sources[2].Fact.ID,
		},
		MissingInformation: []string{},
		ExpectedValue:      semanticdiscovery.ExpectedValueHigh,
		Confidence:         semanticdiscovery.ConfidenceHigh,
	}
	proposal, normalization := semanticdiscovery.NormalizeOpportunityProposal(
		planningBundle,
		semanticdiscovery.OpportunityProposal{
			Version:    semanticdiscovery.OpportunityProposalVersion,
			Candidates: []semanticdiscovery.OpportunityCandidate{rawCandidate},
		},
	)
	if len(normalization.Issues) != 0 || len(proposal.Candidates) != 1 {
		t.Fatalf("normalize = %#v, proposal = %#v", normalization, proposal)
	}
	data := &report.ReportData{RepositoryGraph: &report.RepositoryGraph{
		Modules: []report.ModuleInfo{{Path: "example.com/pipeline"}},
		Packages: []report.PackageInfo{{
			CanonicalPath: "example.com/pipeline",
			ModulePath:    "example.com/pipeline",
			Locality:      "local",
			Files:         []string{"pipeline.go"},
		}},
	}}
	works := selectFreshCandidates(repoRoot, data, proposal, sources)
	if len(works) != 1 {
		t.Fatalf("selected works = %d, want 1", len(works))
	}
	plan := works[0].Plan.Probe
	if len(plan.Seeds) < 3 || len(plan.Seeds) > 6 ||
		plan.Limits.MaxFiles > 8 || plan.Limits.MaxFunctions > 15 ||
		plan.Limits.MaxDepth > 3 || plan.Limits.MaxSourceBytes > 128<<10 ||
		plan.Limits.Timeout > 5_000_000_000 {
		t.Fatalf("probe plan escaped product bounds: %#v", plan)
	}
	if len(plan.ExpansionAllowlist) > 2 {
		t.Fatalf("frontier followups = %d, want at most 2", len(plan.ExpansionAllowlist))
	}

	probe, err := goldenmechanism.Probe(context.Background(), repoRoot, plan)
	if err != nil {
		t.Fatal(err)
	}
	probeFacts, aspects, err := freshProbeFacts(works[0], probe)
	if err != nil {
		t.Fatal(err)
	}
	if len(probeFacts) < 3 || len(probeFacts) > freshRepoDemoMaxProbeFacts || len(aspects) != len(probeFacts) {
		t.Fatalf("probe facts/aspects = %d/%d", len(probeFacts), len(aspects))
	}
	facts, candidate, err := freshCandidateProjection(works[0], probeFacts, aspects)
	if err != nil {
		t.Fatal(err)
	}
	finalBundle := semanticdiscovery.Bundle{
		Version:  semanticdiscovery.BundleVersion,
		RepoName: "fixture",
		Facts:    facts,
	}
	finalProposal := semanticdiscovery.OpportunityProposal{
		Version:    semanticdiscovery.OpportunityProposalVersion,
		Candidates: []semanticdiscovery.OpportunityCandidate{candidate},
	}
	if err := semanticdiscovery.ValidateOpportunityProposal(finalBundle, finalProposal); err != nil {
		t.Fatal(err)
	}
	leaf, err := freshValidatedLeaf(finalBundle, candidate, probeFacts)
	if err != nil {
		t.Fatal(err)
	}
	probeObservationOffset := len(leaf.Artifact.Observations) - len(probeFacts)
	if len(leaf.Artifact.Observations) != len(candidate.SupportIDs)+len(probeFacts) ||
		probeObservationOffset != len(candidate.SupportIDs) ||
		len(leaf.Artifact.MissingEvidence) != 0 {
		t.Fatalf("leaf = %#v", leaf.Artifact)
	}

	claims := make([]semanticdiscovery.ProposedClaim, 0, len(probeFacts))
	for index, fact := range probeFacts {
		claims = append(claims, semanticdiscovery.ProposedClaim{
			Title:      aspects[index].Label,
			Text:       fact.Statement,
			Basis:      semanticdiscovery.ClaimDirect,
			SupportIDs: []string{fact.ID},
			ObservationRefs: []semanticdiscovery.ObservationRef{{
				TaskID: leaf.Task.ID, ObservationIndex: probeObservationOffset + index,
			}},
		})
	}
	fanIn := semanticdiscovery.FanInArtifact{
		Version: semanticdiscovery.FanInArtifactVersion,
		Artifacts: []semanticdiscovery.ArtifactProposal{{
			CandidateID:         candidate.ID,
			Verdict:             semanticdiscovery.VerdictSupported,
			Title:               candidate.Title,
			Summary:             "The entry function calls local operations.",
			Claims:              claims,
			Aliases:             candidate.IntentContract.LocalSearchAliases,
			LikelyQuestions:     []string{candidate.QuestionAnswered},
			RelatedCandidateIDs: []string{},
		}},
	}
	if err := semanticdiscovery.ValidateFanInArtifact(finalBundle, []semanticdiscovery.LeafResult{leaf}, fanIn); err != nil {
		t.Fatalf("validate fan-in: %v; fan-in = %#v", err, fanIn)
	}
	response, err := json.Marshal(fanIn)
	if err != nil {
		t.Fatal(err)
	}
	evaluated, err := evaluateGoldenMechanismResponse(
		finalBundle,
		finalProposal,
		leaf,
		response,
	)
	if err != nil {
		t.Fatalf("evaluate: %v; reduction = %#v", err, evaluated.Reduction)
	}
	if len(evaluated.Artifacts) != 1 || evaluated.Artifacts[0].Verdict != semanticdiscovery.VerdictSupported {
		t.Fatalf("evaluated artifacts = %#v", evaluated.Artifacts)
	}
	if _, err := summarizeGoldenMechanismArtifact(candidate, evaluated.Artifacts[0]); err != nil {
		t.Fatal(err)
	}
}

func TestFreshExpansionFrontierRetrievesExactLocalCalleeFromAnotherFile(t *testing.T) {
	repoRoot := t.TempDir()
	entryLines := []string{
		"package pipeline",
		"",
		"func entry(input string) string {",
		"\treturn helper(input)",
		"}",
		"",
		"func passthrough(input string) string {",
		"\treturn input",
		"}",
	}
	writeFile(t, filepath.Join(repoRoot, "entry.go"), joinLines(entryLines))
	writeFile(t, filepath.Join(repoRoot, "helper.go"), joinLines([]string{
		"package pipeline",
		"",
		"func helper(input string) string {",
		"\treturn input",
		"}",
	}))
	window, err := sourcewindowfacts.NewWindow(
		"evidence-entry",
		"entry.go",
		1,
		entryLines,
	)
	if err != nil {
		t.Fatal(err)
	}
	functions, err := sourcewindowfacts.ExtractGoFunctions(window)
	if err != nil {
		t.Fatal(err)
	}
	if len(functions) != 2 {
		t.Fatalf("functions = %d, want 2", len(functions))
	}
	sources := make([]freshSourceFunction, 0, len(functions))
	for _, function := range functions {
		fact, factErr := freshWindowFunctionFact(function)
		if factErr != nil {
			t.Fatal(factErr)
		}
		sources = append(sources, freshSourceFunction{Function: function, Fact: fact})
	}
	frontier := freshExpansionFrontier(
		repoRoot,
		report.PackageInfo{
			Name:          "pipeline",
			CanonicalPath: "example.com/pipeline",
			ModulePath:    "example.com/pipeline",
			Files:         []string{"entry.go", "helper.go"},
		},
		sources,
		sources,
	)
	if len(frontier.Seeds) != 1 || len(frontier.Symbols) != 1 ||
		frontier.Seeds[0].Path != "helper.go" || frontier.Seeds[0].Symbol != "helper" ||
		frontier.Symbols[0] != "helper" {
		t.Fatalf("frontier = %#v, want exact helper.go helper seed", frontier)
	}
	if frontier.ParsedBytes <= 0 || frontier.ParsedBytes > freshRepoDemoMaxParsedBytes {
		t.Fatalf("frontier parsed bytes = %d", frontier.ParsedBytes)
	}
	probe, err := goldenmechanism.Probe(context.Background(), repoRoot, goldenmechanism.Plan{
		MechanismID:        "cross-file-frontier",
		Seeds:              append(freshProbeSeeds(sources), frontier.Seeds...),
		ExpansionAllowlist: frontier.Symbols,
		Limits: goldenmechanism.Limits{
			MaxDepth: 3, MaxFiles: 6, MaxFunctions: 15,
			MaxParsedSourceBytes: freshRepoDemoMaxParsedBytes - frontier.ParsedBytes,
			MaxSourceBytes:       128 << 10, MaxFunctionLines: 220,
			MaxFunctionBytes: 48 << 10, Timeout: 5_000_000_000,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, function := range probe.Functions {
		if function.Path == "helper.go" && function.Symbol == "helper" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("probe functions = %#v, want exact cross-file helper", probe.Functions)
	}
}

func TestFreshProbeFactsMakesCandidateSupportedScopesKeyBeforeDiversification(t *testing.T) {
	work := freshCandidateWork{
		Candidate: semanticdiscovery.OpportunityCandidate{
			SupportIDs: []string{"fact-supported"},
		},
		Plan: freshRepoCandidatePlan{Identity: semanticdiscovery.MechanismIdentity{
			IntentKey: "supported-mechanism",
		}},
		InitialSources: []freshSourceFunction{{
			Function: sourcewindowfacts.Function{Path: "supported.go", Symbol: "supported"},
			Fact:     semanticdiscovery.Fact{ID: "fact-supported"},
		}},
	}
	observation := func(
		id string,
		functionID string,
		operation string,
		capability semanticdiscovery.Capability,
	) goldenmechanism.Observation {
		path := "supported.go"
		if functionID == "function-unrelated" {
			path = "unrelated.go"
		}
		return goldenmechanism.Observation{
			ID:           id,
			FunctionID:   functionID,
			Operation:    operation,
			Capability:   capability,
			Object:       "value",
			TargetSymbol: "helper",
			Evidence: []goldenmechanism.EvidenceRef{{
				ID:       id + "-evidence",
				Location: evidence.Location{Path: path, Line: 10, Column: 2},
			}},
		}
	}
	probe := goldenmechanism.Result{
		Functions: []goldenmechanism.Function{
			{ID: "function-supported", Path: "supported.go", Symbol: "supported"},
			{
				ID: "function-unrelated", Path: "unrelated.go", Symbol: "unrelated",
				OriginFactIDs: []string{"fact-unrelated"},
			},
		},
		Observations: []goldenmechanism.Observation{
			observation(
				"unrelated-high-priority",
				"function-unrelated",
				"http_handler_entry_signature",
				semanticdiscovery.CapabilityEntry,
			),
			observation(
				"supported-call",
				"function-supported",
				"direct_local_call",
				semanticdiscovery.CapabilityDirectCall,
			),
			observation(
				"supported-write",
				"function-supported",
				"assignment",
				semanticdiscovery.CapabilityDataWrite,
			),
			observation(
				"supported-branch",
				"function-supported",
				"branch_predicate",
				semanticdiscovery.CapabilityBranch,
			),
		},
	}

	facts, aspects, err := freshProbeFacts(work, probe)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 4 || len(aspects) != 4 {
		t.Fatalf("facts/aspects = %d/%d, want 4/4", len(facts), len(aspects))
	}
	for index := 0; index < 3; index++ {
		if facts[index].SourceGroup != "function-supported" {
			t.Fatalf("key fact %d source = %q, want candidate-supported function", index, facts[index].SourceGroup)
		}
		if !aspects[index].Key {
			t.Fatalf("aspect %d is not key: %#v", index, aspects[index])
		}
	}
	if facts[3].SourceGroup != "function-unrelated" || aspects[3].Key {
		t.Fatalf("diversification fact/aspect = %#v / %#v", facts[3], aspects[3])
	}
}

func joinLines(lines []string) string {
	result := ""
	for index, line := range lines {
		if index > 0 {
			result += "\n"
		}
		result += line
	}
	return result + "\n"
}

func TestFreshRepoCandidateProbeFollowsExactCrossFileCallee(t *testing.T) {
	repoRoot := t.TempDir()
	entryLines := []string{
		"package pipeline",
		"",
		"func entry() {",
		"\thandoff()",
		"}",
		"",
		"func prepare() {",
		"\treturn",
		"}",
		"",
		"func finish() {",
		"\treturn",
		"}",
	}
	workerSource := "package pipeline\n\nfunc handoff() {\n\tcomplete()\n}\n\nfunc complete() {}\n"
	writeFile(t, filepath.Join(repoRoot, "entry.go"), joinLines(entryLines))
	writeFile(t, filepath.Join(repoRoot, "worker.go"), workerSource)
	// This invalid file is deliberately absent from the saved package index.
	// The bounded resolver must not turn candidate probing into a tree scan.
	writeFile(t, filepath.Join(repoRoot, "unlisted.go"), "this is not Go")

	window, err := sourcewindowfacts.NewWindow("evidence-entry", "entry.go", 1, entryLines)
	if err != nil {
		t.Fatal(err)
	}
	functions, err := sourcewindowfacts.ExtractGoFunctions(window)
	if err != nil {
		t.Fatal(err)
	}
	sources := make([]freshSourceFunction, 0, len(functions))
	for _, function := range functions {
		fact, factErr := freshWindowFunctionFact(function)
		if factErr != nil {
			t.Fatal(factErr)
		}
		sources = append(sources, freshSourceFunction{Function: function, Fact: fact})
	}
	if len(sources) != 3 {
		t.Fatalf("saved source functions = %d, want 3", len(sources))
	}

	bundle := semanticdiscovery.Bundle{
		Version:  semanticdiscovery.BundleVersion,
		RepoName: "fixture",
		Facts:    freshFacts(sources),
	}
	proposal, normalization := semanticdiscovery.NormalizeOpportunityProposal(
		bundle,
		semanticdiscovery.OpportunityProposal{
			Version: semanticdiscovery.OpportunityProposalVersion,
			Candidates: []semanticdiscovery.OpportunityCandidate{{
				Kind:             semanticdiscovery.ArtifactMechanism,
				Title:            "Request handoff",
				QuestionAnswered: "How is a request handed to local processing?",
				SupportIDs: []string{
					sources[0].Fact.ID,
					sources[1].Fact.ID,
					sources[2].Fact.ID,
				},
				ExpectedValue: semanticdiscovery.ExpectedValueHigh,
				Confidence:    semanticdiscovery.ConfidenceHigh,
			}},
		},
	)
	if len(normalization.Issues) != 0 || len(proposal.Candidates) != 1 {
		t.Fatalf("normalize = %#v, proposal = %#v", normalization, proposal)
	}
	data := &report.ReportData{RepositoryGraph: &report.RepositoryGraph{
		Packages: []report.PackageInfo{{
			CanonicalPath: "example.com/pipeline",
			ModulePath:    "example.com/pipeline",
			Name:          "pipeline",
			Locality:      "local",
			Files:         []string{"entry.go", "worker.go"},
		}},
	}}
	works := selectFreshCandidates(repoRoot, data, proposal, sources)
	if len(works) != 1 {
		t.Fatalf("selected works = %d, want 1", len(works))
	}
	plan := works[0].Plan.Probe
	if !freshPlanHasSeed(plan, "worker.go", "handoff") {
		t.Fatalf("probe seeds = %#v, want exact worker.go handoff frontier", plan.Seeds)
	}
	if len(works[0].Plan.ExpectedFrontier) != 1 ||
		works[0].Plan.ExpectedFrontier[0] != "handoff" {
		t.Fatalf("expected frontier = %v, want [handoff]", works[0].Plan.ExpectedFrontier)
	}
	if spent := freshRepoDemoMaxParsedBytes - plan.Limits.MaxParsedSourceBytes; spent != len(workerSource) {
		t.Fatalf("frontier parsed bytes = %d, want %d", spent, len(workerSource))
	}

	probe, err := goldenmechanism.Probe(context.Background(), repoRoot, plan)
	if err != nil {
		t.Fatal(err)
	}
	if probe.Budget.ParsedSourceBytes+
		(freshRepoDemoMaxParsedBytes-plan.Limits.MaxParsedSourceBytes) > freshRepoDemoMaxParsedBytes {
		t.Fatalf("combined candidate parse budget escaped 128 KiB: %#v", probe.Budget)
	}
	found := false
	for _, function := range probe.Functions {
		if function.Path == "worker.go" && function.Symbol == "handoff" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("probe functions = %#v, want worker.go handoff", probe.Functions)
	}
}

func freshPlanHasSeed(plan goldenmechanism.Plan, path, symbol string) bool {
	for _, seed := range plan.Seeds {
		if seed.Path == path && seed.Symbol == symbol {
			return true
		}
	}
	return false
}

func TestFreshHumanLabelRemovesRepositoryReferences(t *testing.T) {
	for input, want := range map[string]string{
		"Mux.ServeHTTP":                  "mux serve http",
		"mx.NotFoundHandler().ServeHTTP": "not found handler serve http",
		"listing.applySortAndLimit":      "listing apply sort and limit",
	} {
		if got := freshHumanLabel(input); got != want {
			t.Errorf("freshHumanLabel(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestFreshRepresentativeWindowObservationsPreservesDomainCallsAndEffects(t *testing.T) {
	lines := []string{
		"package pipeline",
		"",
		"func execute(input string) string {",
		"\tctx, cancel := context.WithCancel(context.Background())",
		"\tlogger.Debug(fmt.Sprintf(\"input=%s\", input))",
		"\tresult := transform(input)",
		"\tpublisher.Store(result)",
		"\tstate.current = result",
		"\tcancel()",
		"\treturn result",
		"}",
	}
	window, err := sourcewindowfacts.NewWindow("evidence-execute", "pipeline.go", 1, lines)
	if err != nil {
		t.Fatal(err)
	}
	functions, err := sourcewindowfacts.ExtractGoFunctions(window)
	if err != nil {
		t.Fatal(err)
	}
	if len(functions) != 1 {
		t.Fatalf("functions = %d, want 1", len(functions))
	}

	selected := freshRepresentativeWindowObservations(functions[0].Observations, 4)
	if len(selected) != 4 {
		t.Fatalf("selected observations = %d, want 4: %#v", len(selected), selected)
	}
	wantedCalls := map[string]bool{"transform": false, "publisher.Store": false}
	hasOutput := false
	hasStateWrite := false
	for _, observation := range selected {
		if observation.Kind == sourcewindowfacts.ObservationDirectCall {
			if _, wanted := wantedCalls[observation.Target]; wanted {
				wantedCalls[observation.Target] = true
			}
			if freshAuxiliaryWindowCall(observation.Target) {
				t.Fatalf(
					"auxiliary call %q displaced representative behavior: %#v",
					observation.Target,
					selected,
				)
			}
		}
		if observation.Kind == sourcewindowfacts.ObservationReturn {
			hasOutput = true
		}
		if observation.Kind == sourcewindowfacts.ObservationAssignment &&
			observation.Object == "state.current" {
			hasStateWrite = true
		}
	}
	for target, found := range wantedCalls {
		if !found {
			t.Errorf("selected observations lost %q: %#v", target, selected)
		}
	}
	if !hasOutput {
		t.Errorf("selected observations lost the output effect: %#v", selected)
	}
	if !hasStateWrite {
		t.Errorf("selected observations lost the state effect: %#v", selected)
	}
}

func TestFreshPurposeOverlapUsesDocumentedPurpose(t *testing.T) {
	data := &report.ReportData{
		DocumentedPurpose: "Replicates database changes to durable storage.",
		ProjectGuess:      "A command-line utility.",
	}
	candidate := semanticdiscovery.OpportunityCandidate{
		Title:            "Database replication",
		QuestionAnswered: "How are database changes replicated to storage?",
	}
	if got := freshPurposeOverlap(data, candidate); got < 3 {
		t.Fatalf("purpose overlap = %d, want documented-purpose terms", got)
	}
	terms := freshOnboardingPurposeTerms(data)
	for _, term := range []string{"replicates", "database", "storage"} {
		if _, ok := terms[term]; !ok {
			t.Errorf("onboarding purpose terms omit %q: %#v", term, terms)
		}
	}
}

func TestFreshCandidateBoundaryCoverageUsesValidatedComponents(t *testing.T) {
	data := &report.ReportData{Components: []report.Component{
		{
			ID: "entry", Role: componentmap.RoleEntry,
			AnchorGroups: []report.AnchorGroup{{Path: "cmd/tool/main.go"}},
		},
		{
			ID: "core", Role: componentmap.RoleDomain,
			AnchorGroups: []report.AnchorGroup{{Path: "internal/core/run.go"}},
		},
		{
			ID: "store", Role: componentmap.RoleBoundary,
			AnchorGroups: []report.AnchorGroup{{Path: "internal/store/write.go"}},
		},
	}}
	sources := []freshSourceFunction{
		{Function: sourcewindowfacts.Function{Path: "cmd/tool/main.go"}},
		{Function: sourcewindowfacts.Function{Path: "cmd/tool/flags.go"}},
		{Function: sourcewindowfacts.Function{Path: "internal/core/run.go"}},
		{Function: sourcewindowfacts.Function{Path: "internal/store/write.go"}},
	}
	if got := freshCandidateBoundaryCoverage(data, sources); got != 3 {
		t.Fatalf("boundary coverage = %d, want entry + core + boundary", got)
	}
	if got := freshCandidateBoundaryCoverage(data, sources[:2]); got != 1 {
		t.Fatalf("same-component file coverage = %d, want 1", got)
	}
}

func TestFreshValidatedLeafCarriesPlannerSupportIntoSynthesis(t *testing.T) {
	grounding := semanticdiscovery.Fact{
		ID:          "fact-grounding",
		Kind:        semanticdiscovery.FactSourceSignal,
		Statement:   "The entry function directly calls the loader operation.",
		SourceGroup: "group-grounding",
		Capabilities: []semanticdiscovery.Capability{
			semanticdiscovery.CapabilityStatic,
			semanticdiscovery.CapabilityBehavior,
			semanticdiscovery.CapabilityDirectCall,
		},
		Scope: semanticdiscovery.FactScopeLocal,
		Source: &semanticdiscovery.FactSource{
			Path:            "pipeline.go",
			StartLine:       1,
			EndLine:         3,
			EnclosingSymbol: "entry",
			ContentSHA256:   strings.Repeat("a", 64),
		},
	}
	probeFact := semanticdiscovery.Fact{
		ID:          "fact-probe-output",
		Kind:        semanticdiscovery.FactSourceSignal,
		Statement:   "The loader function returns a parsed result.",
		Keywords:    []string{"answer_aspect:operation-1"},
		SourceGroup: "group-probe",
		Capabilities: []semanticdiscovery.Capability{
			semanticdiscovery.CapabilityStatic,
			semanticdiscovery.CapabilityBehavior,
			semanticdiscovery.CapabilityOutputEffect,
		},
		Scope: semanticdiscovery.FactScopeLocal,
	}
	bundle := semanticdiscovery.Bundle{
		Version:  semanticdiscovery.BundleVersion,
		RepoName: "fixture",
		Facts:    []semanticdiscovery.Fact{grounding, probeFact},
	}
	rawCandidate := semanticdiscovery.OpportunityCandidate{
		Kind:             semanticdiscovery.ArtifactMechanism,
		Title:            "Input processing",
		QuestionAnswered: "How does an input become a parsed result?",
		SupportIDs:       []string{grounding.ID},
		EnrichmentSupportIDs: []string{
			probeFact.ID,
		},
		MissingInformation: []string{},
		ExpectedValue:      semanticdiscovery.ExpectedValueHigh,
		Confidence:         semanticdiscovery.ConfidenceHigh,
		CapabilityContract: &semanticdiscovery.CapabilityContract{
			RequiredCapabilities: []semanticdiscovery.Capability{
				semanticdiscovery.CapabilityBehavior,
				semanticdiscovery.CapabilityDirectCall,
				semanticdiscovery.CapabilityOutputEffect,
			},
			AvailableCapabilities: []semanticdiscovery.Capability{
				semanticdiscovery.CapabilityBehavior,
				semanticdiscovery.CapabilityDirectCall,
				semanticdiscovery.CapabilityOutputEffect,
			},
			MissingCapabilities: []semanticdiscovery.Capability{},
			Resolution:          semanticdiscovery.CapabilityResolutionReady,
		},
		IntentContract: &semanticdiscovery.IntentContract{
			RequiredAnswerAspects: []semanticdiscovery.AnswerAspect{{
				ID:                   "operation-1",
				Label:                "Parsed output",
				RequiredCapabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityOutputEffect},
				Key:                  true,
			}},
			MinCovered:         1,
			MinKeyCovered:      1,
			LocalSearchAliases: []string{"Input processing"},
		},
	}
	proposal, normalization := semanticdiscovery.NormalizeOpportunityProposal(
		bundle,
		semanticdiscovery.OpportunityProposal{
			Version:    semanticdiscovery.OpportunityProposalVersion,
			Candidates: []semanticdiscovery.OpportunityCandidate{rawCandidate},
		},
	)
	if len(normalization.Issues) != 0 || len(proposal.Candidates) != 1 {
		t.Fatalf("normalize = %#v, proposal = %#v", normalization, proposal)
	}
	candidate := proposal.Candidates[0]
	leaf, err := freshValidatedLeaf(bundle, candidate, []semanticdiscovery.Fact{probeFact})
	if err != nil {
		t.Fatal(err)
	}
	if len(leaf.Artifact.Observations) != 2 {
		t.Fatalf("observations = %#v, want planner support and probe fact", leaf.Artifact.Observations)
	}
	if got := leaf.Artifact.Observations[0]; got.Text != grounding.Statement ||
		len(got.SupportIDs) != 1 || got.SupportIDs[0] != grounding.ID {
		t.Fatalf("planner support observation = %#v", got)
	}
	prompt, err := semanticdiscovery.BuildGoldenMechanismPrompt(bundle, leaf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt.User, `"id":"`+grounding.ID+`"`) ||
		!strings.Contains(prompt.User, grounding.Statement) {
		t.Fatalf("golden synthesis prompt lost initial support fact: %s", prompt.User)
	}

	repositoryGrounding := grounding
	repositoryGrounding.ID = "fact-repository-grounding"
	repositoryGrounding.Statement = "The command invokes restoreReplica.Restore with configured options."
	repositoryGrounding.SourceGroup = "group-repository-grounding"
	repositoryBundle := bundle
	repositoryBundle.Facts = []semanticdiscovery.Fact{repositoryGrounding, probeFact}
	repositoryCandidate := rawCandidate
	repositoryCandidate.SupportIDs = []string{repositoryGrounding.ID}
	repositoryProposal, repositoryNormalization := semanticdiscovery.NormalizeOpportunityProposal(
		repositoryBundle,
		semanticdiscovery.OpportunityProposal{
			Version:    semanticdiscovery.OpportunityProposalVersion,
			Candidates: []semanticdiscovery.OpportunityCandidate{repositoryCandidate},
		},
	)
	if len(repositoryNormalization.Issues) != 0 || len(repositoryProposal.Candidates) != 1 {
		t.Fatalf("repository normalize = %#v, proposal = %#v", repositoryNormalization, repositoryProposal)
	}
	repositoryLeaf, err := freshValidatedLeaf(
		repositoryBundle,
		repositoryProposal.Candidates[0],
		[]semanticdiscovery.Fact{probeFact},
	)
	if err != nil {
		t.Fatalf("repository-bearing local fact rejected: %v", err)
	}
	if got, want := repositoryLeaf.Artifact.Observations[0].Text,
		"The command invokes restoreReplica Restore with configured options."; got != want {
		t.Fatalf("repository grounding observation = %q, want %q", got, want)
	}
}

func TestFreshValidatedLeafPacksAllGroundingWithinItemLimit(t *testing.T) {
	const factCount = 5
	facts := make([]semanticdiscovery.Fact, 0, factCount*2)
	supportIDs := make([]string, 0, factCount)
	probeFacts := make([]semanticdiscovery.Fact, 0, factCount)
	enrichmentIDs := make([]string, 0, factCount)
	aspects := make([]semanticdiscovery.AnswerAspect, 0, factCount)
	for index := 0; index < factCount; index++ {
		grounding := semanticdiscovery.Fact{
			ID:          fmt.Sprintf("fact-grounding-%d", index+1),
			Kind:        semanticdiscovery.FactSourceSignal,
			Statement:   fmt.Sprintf("The input stage records grounded value %d.", index+1),
			SourceGroup: fmt.Sprintf("grounding-group-%d", index+1),
			Capabilities: []semanticdiscovery.Capability{
				semanticdiscovery.CapabilityStatic,
				semanticdiscovery.CapabilityBehavior,
				semanticdiscovery.CapabilityDataWrite,
			},
			Scope: semanticdiscovery.FactScopeLocal,
		}
		probe := semanticdiscovery.Fact{
			ID:          fmt.Sprintf("fact-probe-%d", index+1),
			Kind:        semanticdiscovery.FactSourceSignal,
			Statement:   fmt.Sprintf("The output stage writes bounded value %d.", index+1),
			Keywords:    []string{fmt.Sprintf("answer_aspect:operation-%d", index+1)},
			SourceGroup: fmt.Sprintf("probe-group-%d", index+1),
			Capabilities: []semanticdiscovery.Capability{
				semanticdiscovery.CapabilityStatic,
				semanticdiscovery.CapabilityBehavior,
				semanticdiscovery.CapabilityDataWrite,
			},
			Scope: semanticdiscovery.FactScopeLocal,
		}
		facts = append(facts, grounding, probe)
		supportIDs = append(supportIDs, grounding.ID)
		probeFacts = append(probeFacts, probe)
		enrichmentIDs = append(enrichmentIDs, probe.ID)
		aspects = append(aspects, semanticdiscovery.AnswerAspect{
			ID: fmt.Sprintf("operation-%d", index+1), Label: fmt.Sprintf("Bounded write %d", index+1),
			RequiredCapabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityDataWrite},
			Key:                  index < 3,
		})
	}
	bundle := semanticdiscovery.Bundle{
		Version: semanticdiscovery.BundleVersion, RepoName: "fixture", Facts: facts,
	}
	rawCandidate := semanticdiscovery.OpportunityCandidate{
		Kind: semanticdiscovery.ArtifactMechanism, Title: "Bounded processing",
		QuestionAnswered:     "How does bounded input become output?",
		SupportIDs:           supportIDs,
		EnrichmentSupportIDs: enrichmentIDs,
		MissingInformation:   []string{},
		ExpectedValue:        semanticdiscovery.ExpectedValueHigh,
		Confidence:           semanticdiscovery.ConfidenceHigh,
		CapabilityContract: &semanticdiscovery.CapabilityContract{
			RequiredCapabilities: []semanticdiscovery.Capability{
				semanticdiscovery.CapabilityBehavior,
				semanticdiscovery.CapabilityDataWrite,
			},
			AvailableCapabilities: []semanticdiscovery.Capability{
				semanticdiscovery.CapabilityBehavior,
				semanticdiscovery.CapabilityDataWrite,
			},
			MissingCapabilities: []semanticdiscovery.Capability{},
			Resolution:          semanticdiscovery.CapabilityResolutionReady,
		},
		IntentContract: &semanticdiscovery.IntentContract{
			RequiredAnswerAspects: aspects,
			MinCovered:            3,
			MinKeyCovered:         3,
			LocalSearchAliases:    []string{"Bounded processing"},
		},
	}
	proposal, normalization := semanticdiscovery.NormalizeOpportunityProposal(
		bundle,
		semanticdiscovery.OpportunityProposal{
			Version:    semanticdiscovery.OpportunityProposalVersion,
			Candidates: []semanticdiscovery.OpportunityCandidate{rawCandidate},
		},
	)
	if len(normalization.Issues) != 0 || len(proposal.Candidates) != 1 {
		t.Fatalf("normalize = %#v, proposal = %#v", normalization, proposal)
	}
	leaf, err := freshValidatedLeaf(bundle, proposal.Candidates[0], probeFacts)
	if err != nil {
		t.Fatal(err)
	}
	if len(leaf.Artifact.Observations) != 8 {
		t.Fatalf("observations = %d, want exact leaf limit 8", len(leaf.Artifact.Observations))
	}
	if len(leaf.Artifact.CandidateConnection.SupportIDs) != factCount*2 {
		t.Fatalf("support ids = %#v, want all grounding and probe facts", leaf.Artifact.CandidateConnection.SupportIDs)
	}
	prompt, err := semanticdiscovery.BuildGoldenMechanismPrompt(bundle, leaf)
	if err != nil {
		t.Fatal(err)
	}
	for _, fact := range facts {
		if !strings.Contains(prompt.User, `"id":"`+fact.ID+`"`) {
			t.Fatalf("golden synthesis prompt lost fact %q", fact.ID)
		}
	}
}

func TestFreshSourceFunctionsReturnsClearFallbackWhenAllWindowsAreTruncated(t *testing.T) {
	repoRoot := t.TempDir()
	runDir := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "pipeline.go"), "package pipeline\n\nfunc run() { helper() }\n")
	location := evidence.Location{Path: "pipeline.go", Line: 3}
	bundle := modelresearch.EvidenceBundle{
		Version: modelresearch.ContractVersion,
		RoundID: "research-truncated",
		Evidence: []modelresearch.EvidenceItem{{
			ID:        "evidence-truncated",
			Kind:      modelresearch.EvidenceSource,
			Statement: "bounded source window selected locally for the research question",
			Location:  &location,
			Certainty: evidence.CertaintyStatic,
			Window: &modelresearch.SourceWindow{
				StartLine:   3,
				EndLine:     3,
				Lines:       []string{"func run() { helper() }"},
				CodeBearing: true,
				Truncated:   true,
			},
		}},
	}
	raw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	bundleDir := filepath.Join(runDir, "research", bundle.RoundID)
	if err := os.MkdirAll(bundleDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "evidence_bundle.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	_, windows, functions, err := freshSourceFunctions(runDir, repoRoot)
	if !errors.Is(err, errFreshNoUsableSourceWindows) {
		t.Fatalf("freshSourceFunctions() error = %v, want clear no-usable-windows fallback", err)
	}
	if windows != 0 || functions != 0 {
		t.Fatalf("freshSourceFunctions() counts = %d/%d, want 0/0", windows, functions)
	}
}

func TestFreshCentralSourceFunctionsUseSavedFlowAnchors(t *testing.T) {
	repoRoot := t.TempDir()
	runDir := t.TempDir()
	source := joinLines([]string{
		"package pipeline",
		"",
		"func NewRegistry() map[string]string {",
		"\treturn map[string]string{}",
		"}",
		"",
		"func RunPipeline(input string) string {",
		"\tvalue := loadInput(input)",
		"\treturn persistOutput(value)",
		"}",
		"",
		"func loadInput(input string) string { return input }",
		"func persistOutput(value string) string { return value }",
	})
	writeFile(t, filepath.Join(repoRoot, "pipeline.go"), source)
	flowDir := filepath.Join(runDir, "flows", "main-behavior")
	if err := os.MkdirAll(flowDir, 0o700); err != nil {
		t.Fatal(err)
	}
	flow := freshFlowBundle{SourceSignals: []freshFlowBundleSource{{
		Path: "pipeline.go", Line: 7, Category: "background_loop",
		Snippet: "func RunPipeline(input string) string", Weight: 40,
		Reason: "saved central behavior anchor",
	}}}
	if err := writeGoldenJSON(filepath.Join(flowDir, "flow_bundle.json"), flow); err != nil {
		t.Fatal(err)
	}
	data := &report.ReportData{
		ProjectGuess:  "A pipeline persists processed input",
		OpenablePaths: []string{"pipeline.go"},
		FirstFilesToOpen: []report.FileItem{{
			Path: "pipeline.go",
		}},
	}

	functions, parsedBytes, err := freshCentralSourceFunctions(runDir, repoRoot, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(functions) == 0 {
		t.Fatal("central source functions are empty")
	}
	if got := functions[0].Function.Symbol; got != "RunPipeline" {
		t.Fatalf("first central function = %q, want RunPipeline", got)
	}
	if parsedBytes != len(source) {
		t.Fatalf("parsed bytes = %d, want %d", parsedBytes, len(source))
	}
	if parsedBytes > freshRepoOnboardingMaxParsedBytes {
		t.Fatalf("parsed bytes = %d, limit = %d", parsedBytes, freshRepoOnboardingMaxParsedBytes)
	}
}

func TestFreshCandidateCentralityPrefersPurposeInputAndEffectOverRegistry(t *testing.T) {
	data := &report.ReportData{
		ProjectGuess: "A service processes input and persists output",
		HighLevelMap: []report.Subsystem{{
			Name: "Core engine", Role: "domain",
			WhyItMatters: "Processes input and persists the resulting output.",
			Evidence:     []string{"engine.go"},
		}},
	}
	primarySources := []freshSourceFunction{
		freshCentralitySource(
			"fact-entry",
			"engine.go",
			"Run",
			semanticdiscovery.CapabilityEntry,
			semanticdiscovery.CapabilityDataRead,
		),
		freshCentralitySource(
			"fact-work",
			"engine.go",
			"Process",
			semanticdiscovery.CapabilityDataTransformation,
		),
		freshCentralitySource(
			"fact-effect",
			"storage.go",
			"Persist",
			semanticdiscovery.CapabilityDataWrite,
			semanticdiscovery.CapabilityOutputEffect,
		),
	}
	registrySources := []freshSourceFunction{
		freshCentralitySource(
			"fact-register",
			"registry.go",
			"RegisterFactory",
			semanticdiscovery.CapabilityDataWrite,
		),
		freshCentralitySource(
			"fact-lookup",
			"registry.go",
			"LookupPlugin",
			semanticdiscovery.CapabilityDataRead,
		),
		freshCentralitySource(
			"fact-create",
			"registry.go",
			"NewAdapter",
			semanticdiscovery.CapabilityOutputEffect,
		),
	}
	primary := semanticdiscovery.OpportunityCandidate{
		Title: "Input processing and persistence", QuestionAnswered: "How is input processed and persisted as output?",
		SupportIDs:    []string{"fact-entry", "fact-work", "fact-effect"},
		ExpectedValue: semanticdiscovery.ExpectedValueMedium, Confidence: semanticdiscovery.ConfidenceMedium,
	}
	registry := semanticdiscovery.OpportunityCandidate{
		Title: "Plugin factory registry", QuestionAnswered: "How are plugin adapters registered and created?",
		SupportIDs:    []string{"fact-register", "fact-lookup", "fact-create"},
		ExpectedValue: semanticdiscovery.ExpectedValueHigh, Confidence: semanticdiscovery.ConfidenceHigh,
	}
	primaryRank := deriveFreshCandidateCentrality(
		data, primary, primarySources, primarySources, freshPurposeOverlap(data, primary),
	)
	registryRank := deriveFreshCandidateCentrality(
		data, registry, registrySources, registrySources, freshPurposeOverlap(data, registry),
	)
	if comparison := compareFreshCandidateCentrality(primaryRank, registryRank); comparison <= 0 {
		t.Fatalf("primary rank = %#v, registry rank = %#v, comparison = %d", primaryRank, registryRank, comparison)
	}
	if registryRank.SecondaryPenalty == 0 {
		t.Fatalf("registry rank lacks a structural secondary penalty: %#v", registryRank)
	}
}

func freshCentralitySource(
	id string,
	path string,
	symbol string,
	capabilities ...semanticdiscovery.Capability,
) freshSourceFunction {
	return freshSourceFunction{
		Function: sourcewindowfacts.Function{Path: path, Symbol: symbol},
		Fact: semanticdiscovery.Fact{
			ID: id, Statement: symbol, SourceGroup: id + "-group", Capabilities: capabilities,
		},
	}
}

func TestFreshPublicationContinuesForEveryOnboardingRole(t *testing.T) {
	for _, test := range []struct {
		role report.OnboardingRole
	}{
		{role: report.OnboardingRolePrimaryBehavior},
		{role: report.OnboardingRoleSecondaryBehavior},
		{role: report.OnboardingRoleExtensionPoint},
		{role: report.OnboardingRoleOperationalSupport},
		{role: report.OnboardingRoleErrorDetail},
	} {
		if !freshContinueAfterPublication(test.role) {
			t.Errorf("freshContinueAfterPublication(%q) = false, want true", test.role)
		}
	}
}

func TestFreshPreferredArtifactID(t *testing.T) {
	if got := freshPreferredArtifactID(freshRepoDemoStatus{}); got != "" {
		t.Fatalf("empty status preferred artifact = %q", got)
	}
	status := freshRepoDemoStatus{
		PublishedArtifact: &goldenMechanismArtifactSummary{ID: "semantic-artifact-central"},
	}
	if got := freshPreferredArtifactID(status); got != "semantic-artifact-central" {
		t.Fatalf("preferred artifact = %q", got)
	}
}
