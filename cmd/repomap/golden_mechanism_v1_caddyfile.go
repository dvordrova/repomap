package main

import (
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/dvordrova/repomap/internal/goldenmechanism"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

const (
	goldenCaddyfileBaseCandidateID  = "semantic-candidate-0ff9d7dccd8e43ce6809fa26"
	goldenCaddyfileErrorCandidateID = "semantic-candidate-713caa3e1852750a725079be"
	goldenCaddyfileErrorTitle       = "Caddyfile Request Matcher Error Propagation"
	goldenCaddyfileErrorQuestion    = "How does a top-level Caddyfile request matcher error acquire source location and propagate through the built-in adapter?"
)

var goldenCaddyfileErrorAliases = []string{
	"caddyfile syntax error",
	"caddyfile parse error source location",
	"caddyfile request matcher error",
	"built-in caddyfile adapter parse error",
	"ошибка синтаксиса caddyfile",
}

type goldenCaddyfileAspectRole string

const (
	goldenCaddyfileAspectKey        goldenCaddyfileAspectRole = "key"
	goldenCaddyfileAspectSupporting goldenCaddyfileAspectRole = "supporting"
	goldenCaddyfileAspectOptional   goldenCaddyfileAspectRole = "explicitly_optional"
)

type goldenCaddyfileAspectDefinition struct {
	Aspect semanticdiscovery.AnswerAspect `json:"aspect"`
	Role   goldenCaddyfileAspectRole      `json:"role"`
}

func goldenCaddyfileErrorAspectDefinitions() []goldenCaddyfileAspectDefinition {
	return []goldenCaddyfileAspectDefinition{
		{
			Aspect: semanticdiscovery.AnswerAspect{
				ID: "error_origin", Label: "Where the selected matcher error originates",
				RequiredCapabilities: []semanticdiscovery.Capability{
					semanticdiscovery.CapabilityStatic,
					semanticdiscovery.CapabilityBranch,
					semanticdiscovery.CapabilityErrorPath,
				},
				Key: true,
			},
			Role: goldenCaddyfileAspectKey,
		},
		{
			Aspect: semanticdiscovery.AnswerAspect{
				ID: "source_location", Label: "How file and line are attached",
				RequiredCapabilities: []semanticdiscovery.Capability{
					semanticdiscovery.CapabilityStatic,
					semanticdiscovery.CapabilityErrorPath,
				},
				Key: true,
			},
			Role: goldenCaddyfileAspectKey,
		},
		{
			Aspect: semanticdiscovery.AnswerAspect{
				ID: "parser_propagation", Label: "How the same local error binding crosses the parser entry chain",
				RequiredCapabilities: []semanticdiscovery.Capability{
					semanticdiscovery.CapabilityDirectCall,
					semanticdiscovery.CapabilityErrorPath,
					semanticdiscovery.CapabilitySequence,
				},
				Key: true,
			},
			Role: goldenCaddyfileAspectKey,
		},
		{
			Aspect: semanticdiscovery.AnswerAspect{
				ID: "adapter_propagation", Label: "How the built-in adapter returns the parse error",
				RequiredCapabilities: []semanticdiscovery.Capability{
					semanticdiscovery.CapabilityDirectCall,
					semanticdiscovery.CapabilityErrorPath,
					semanticdiscovery.CapabilitySequence,
				},
				Key: true,
			},
			Role: goldenCaddyfileAspectKey,
		},
		{
			Aspect: semanticdiscovery.AnswerAspect{
				ID: "context_enrichment", Label: "How wrapping preserves the cause and adds import context",
				RequiredCapabilities: []semanticdiscovery.Capability{
					semanticdiscovery.CapabilityStatic,
					semanticdiscovery.CapabilityErrorPath,
				},
			},
			Role: goldenCaddyfileAspectSupporting,
		},
		{
			Aspect: semanticdiscovery.AnswerAspect{
				ID: "error_classification", Label: "Why this error is specifically a top-level request matcher error",
				RequiredCapabilities: []semanticdiscovery.Capability{
					semanticdiscovery.CapabilityStatic,
					semanticdiscovery.CapabilityBranch,
					semanticdiscovery.CapabilityErrorPath,
				},
			},
			Role: goldenCaddyfileAspectSupporting,
		},
		{
			Aspect: semanticdiscovery.AnswerAspect{
				ID: "important_alternatives", Label: "Which neighboring adapter failures are distinct from this parser error",
				RequiredCapabilities: []semanticdiscovery.Capability{
					semanticdiscovery.CapabilityBranch,
					semanticdiscovery.CapabilityErrorPath,
					semanticdiscovery.CapabilityLimitation,
				},
			},
			Role: goldenCaddyfileAspectSupporting,
		},
		{
			Aspect: semanticdiscovery.AnswerAspect{
				ID: "test_evidence", Label: "A local assertion of the exact matcher message and source line",
				RequiredCapabilities: []semanticdiscovery.Capability{
					semanticdiscovery.CapabilityStatic,
					semanticdiscovery.CapabilityErrorPath,
					semanticdiscovery.CapabilityTestEvidence,
				},
			},
			Role: goldenCaddyfileAspectOptional,
		},
		{
			Aspect: semanticdiscovery.AnswerAspect{
				ID: "known_unknowns", Label: "Where the bounded proof stops and which error families it does not generalize",
				RequiredCapabilities: []semanticdiscovery.Capability{
					semanticdiscovery.CapabilityLimitation,
				},
			},
			Role: goldenCaddyfileAspectOptional,
		},
	}
}

func caddyfileErrorPlan(
	bundle semanticdiscovery.Bundle,
	data *report.ReportData,
) (goldenmechanism.Plan, error) {
	if data == nil {
		return goldenmechanism.Plan{}, fmt.Errorf("golden mechanism v1: report data is required")
	}
	openable := make(map[string]struct{}, len(data.OpenablePaths))
	for _, path := range data.OpenablePaths {
		openable[path] = struct{}{}
	}
	baseFacts := make(map[string]semanticdiscovery.Fact, len(bundle.Facts))
	for _, fact := range bundle.Facts {
		baseFacts[fact.ID] = fact
	}
	type seedSpec struct {
		path             string
		symbol           string
		originFactID     string
		originEvidenceID string
	}
	specs := []seedSpec{
		{
			path: "caddyconfig/caddyfile/parse.go", symbol: "Parse",
			originFactID: "sf-91fea5c94796f8ea", originEvidenceID: "se-4ce7a31846c42f68",
		},
		{
			path: "caddyconfig/caddyfile/parse.go", symbol: "parser.addresses",
			originFactID: "sf-39135c6825a94f47", originEvidenceID: "se-be14a39b40516dee",
		},
		{
			path: "caddyconfig/caddyfile/dispenser.go", symbol: "Dispenser.Errf",
			originFactID: "sf-787f7c6a2a517a39", originEvidenceID: "se-3a611c4825bb18d2",
		},
		{path: "caddyconfig/caddyfile/adapter.go", symbol: "Adapter.Adapt"},
		{path: "caddyconfig/caddyfile/parse_test.go", symbol: "TestRejectsGlobalMatcher"},
	}
	seeds := make([]goldenmechanism.Seed, 0, len(specs))
	for _, spec := range specs {
		if _, exists := openable[spec.path]; !exists {
			return goldenmechanism.Plan{}, fmt.Errorf(
				"golden mechanism v1: saved report does not authorize seed path %q",
				spec.path,
			)
		}
		if spec.originFactID != "" {
			fact, exists := baseFacts[spec.originFactID]
			if !exists || !goldenFactHasEvidence(fact, spec.originEvidenceID, spec.path) {
				return goldenmechanism.Plan{}, fmt.Errorf(
					"golden mechanism v1: saved source seed %q lost its evidence binding",
					spec.symbol,
				)
			}
		} else {
			spec.originFactID = goldenStableID("gmsf", "saved-openable-path", spec.path)
			spec.originEvidenceID = goldenStableID("gmse", "saved-openable-path", spec.path)
		}
		seeds = append(seeds, goldenmechanism.Seed{
			OriginFactID: spec.originFactID, OriginEvidenceID: spec.originEvidenceID,
			Path: spec.path, Symbol: spec.symbol,
		})
	}
	return goldenmechanism.Plan{
		MechanismID: "caddyfile-request-matcher-error",
		Seeds:       seeds,
		ExpansionAllowlist: []string{
			"Parse",
			"parser.parseAll",
			"parser.parseOne",
			"parser.begin",
			"parser.addresses",
			"Dispenser.Errf",
			"Dispenser.WrapErr",
			"Adapter.Adapt",
			"TestRejectsGlobalMatcher",
		},
		Limits: goldenmechanism.Limits{
			MaxDepth: 3, MaxFiles: 4, MaxFunctions: 12,
			MaxParsedSourceBytes: 192 << 10,
			MaxSourceBytes:       64 << 10,
			MaxFunctionLines:     128,
			MaxFunctionBytes:     24 << 10,
			Timeout:              3 * time.Second,
		},
	}, nil
}

func goldenFactHasEvidence(
	fact semanticdiscovery.Fact,
	evidenceID string,
	path string,
) bool {
	for _, reference := range fact.Evidence {
		if reference.ID == evidenceID && reference.Path == path {
			return true
		}
	}
	return false
}

func projectCaddyfileError(
	bundle semanticdiscovery.Bundle,
	original semanticdiscovery.OpportunityCandidate,
	probe goldenmechanism.Result,
) (goldenProjection, error) {
	if original.ID != goldenCaddyfileBaseCandidateID ||
		original.Kind != semanticdiscovery.ArtifactMechanism {
		return goldenProjection{}, fmt.Errorf("golden mechanism v1: selected saved candidate is unavailable")
	}
	index, err := newGoldenObservationIndex(probe)
	if err != nil {
		return goldenProjection{}, err
	}

	origin, err := index.requiredObservations(
		"top-level matcher error origin",
		goldenObservationSelector{
			symbol: "parser.addresses", operation: "branch_predicate",
			contains: `strings.HasPrefix(value, "@")`,
		},
		goldenObservationSelector{
			symbol: "parser.addresses", operation: "error_return",
			contains: "request matchers may not be defined globally",
		},
	)
	if err != nil {
		return goldenProjection{}, err
	}
	context, err := index.requiredObservations(
		"source context attachment",
		goldenObservationSelector{
			symbol: "Dispenser.Errf", operation: "direct_local_call",
			target: "Dispenser.WrapErr",
		},
		goldenObservationSelector{
			symbol: "Dispenser.Errf", operation: "local_error_handoff",
			target: "Dispenser.WrapErr",
		},
		goldenObservationSelector{
			symbol: "Dispenser.WrapErr", operation: "branch_predicate",
			contains: "len(d.Token().imports) > 0",
		},
		goldenObservationSelector{
			symbol: "Dispenser.WrapErr", operation: "error_return",
			contains: "import chain",
		},
		goldenObservationSelector{
			symbol: "Dispenser.WrapErr", operation: "error_return",
			object: `fmt.Errorf("%w, at %s:%d", err, d.File(), d.Line())`,
		},
	)
	if err != nil {
		return goldenProjection{}, err
	}
	parserPropagation, err := index.requiredObservations(
		"parser error propagation",
		goldenObservationSelector{
			symbol: "parser.begin", operation: "local_error_handoff",
			target: "parser.addresses",
		},
		goldenObservationSelector{
			symbol: "parser.parseOne", operation: "local_error_handoff",
			target: "parser.begin",
		},
		goldenObservationSelector{
			symbol: "parser.parseAll", operation: "local_error_handoff",
			target: "parser.parseOne",
		},
		goldenObservationSelector{
			symbol: "Parse", operation: "local_error_handoff",
			target: "parser.parseAll",
		},
	)
	if err != nil {
		return goldenProjection{}, err
	}
	adapterPropagation, err := index.requiredObservations(
		"adapter parse error propagation",
		goldenObservationSelector{
			symbol: "Adapter.Adapt", operation: "direct_local_call",
			target: "Parse",
		},
		goldenObservationSelector{
			symbol: "Adapter.Adapt", operation: "local_error_handoff",
			target: "Parse",
		},
	)
	if err != nil {
		return goldenProjection{}, err
	}
	testEvidence, err := index.requiredObservations(
		"matcher error test",
		goldenObservationSelector{
			symbol: "TestRejectsGlobalMatcher", operation: "branch_predicate",
			contains: "err == nil",
		},
		goldenObservationSelector{
			symbol: "TestRejectsGlobalMatcher", operation: "branch_predicate",
			contains: "err.Error() != expected",
		},
	)
	if err != nil {
		return goldenProjection{}, err
	}
	testExpected, err := index.requiredSourceLine(
		"exact matcher test expectation",
		"TestRejectsGlobalMatcher",
		"at Testfile:2",
	)
	if err != nil {
		return goldenProjection{}, err
	}
	noServerType, err := index.requiredObservation(
		"missing server type alternative",
		goldenObservationSelector{
			symbol: "Adapter.Adapt", operation: "branch_predicate",
			contains: "a.ServerType == nil",
		},
	)
	if err != nil {
		return goldenProjection{}, err
	}
	setupCall, err := index.requiredSourceLine(
		"server type setup alternative",
		"Adapter.Adapt",
		"a.ServerType.Setup(serverBlocks, options)",
	)
	if err != nil {
		return goldenProjection{}, err
	}

	facts := []semanticdiscovery.Fact{
		newGoldenFact(
			"caddyfile-error-origin",
			"Top-level address parsing classifies a token beginning with @ as an invalid globally defined request matcher and returns a formatted parser error for that branch.",
			[]string{"error_origin", "error_classification"},
			[]string{"top-level matcher", "parser error origin"},
			[]semanticdiscovery.Capability{
				semanticdiscovery.CapabilityStatic,
				semanticdiscovery.CapabilityBranch,
				semanticdiscovery.CapabilityErrorPath,
			},
			origin,
			nil,
		),
		newGoldenFact(
			"caddyfile-error-context",
			"The formatted matcher error is passed into a local wrapping helper. That helper preserves the existing error through an explicit wrapping marker and attaches the current token's file and line; when imports are present it also appends import-chain context.",
			[]string{"source_location", "context_enrichment"},
			[]string{"file line", "error wrapping", "import chain"},
			[]semanticdiscovery.Capability{
				semanticdiscovery.CapabilityStatic,
				semanticdiscovery.CapabilityErrorPath,
			},
			context,
			nil,
		),
		newGoldenFact(
			"caddyfile-parser-propagation",
			"Within the retained parser source, the selected error is handed from matcher parsing through the parser entry chain to the public parse function. Each retained hop is either direct result passthrough or an exact same-function assign, non-nil check, and return of the same local error binding.",
			[]string{"parser_propagation"},
			[]string{"parser call chain", "same error binding"},
			[]semanticdiscovery.Capability{
				semanticdiscovery.CapabilityDirectCall,
				semanticdiscovery.CapabilityErrorPath,
				semanticdiscovery.CapabilitySequence,
			},
			parserPropagation,
			nil,
		),
		newGoldenFact(
			"caddyfile-adapter-propagation",
			"The built-in adapter calls the public parser. If parsing yields a non-nil error, the immediately following guard returns that same local error binding before server-type setup.",
			[]string{"adapter_propagation"},
			[]string{"built-in adapter", "parse error return"},
			[]semanticdiscovery.Capability{
				semanticdiscovery.CapabilityDirectCall,
				semanticdiscovery.CapabilityErrorPath,
				semanticdiscovery.CapabilitySequence,
			},
			adapterPropagation,
			nil,
		),
		newGoldenFact(
			"caddyfile-matcher-test",
			"A parser unit test supplies a top-level @rejected matcher, requires an error, and compares the complete message with an expectation ending in at Testfile:2.",
			[]string{"test_evidence"},
			[]string{"parser test", "Testfile line", "exact error message"},
			[]semanticdiscovery.Capability{
				semanticdiscovery.CapabilityStatic,
				semanticdiscovery.CapabilityErrorPath,
				semanticdiscovery.CapabilityTestEvidence,
			},
			testEvidence,
			[]goldenmechanism.SourceLine{testExpected},
		),
		newGoldenFact(
			"caddyfile-adapter-alternatives",
			"The same adapter has distinct neighboring failure paths when no server type is configured and when setup fails with parsed server blocks. Those alternatives are not evidence about the selected matcher error.",
			[]string{"important_alternatives"},
			[]string{"adapter alternatives", "setup error", "scope limitation"},
			[]semanticdiscovery.Capability{
				semanticdiscovery.CapabilityBranch,
				semanticdiscovery.CapabilityErrorPath,
				semanticdiscovery.CapabilityLimitation,
			},
			[]goldenmechanism.Observation{noServerType},
			[]goldenmechanism.SourceLine{setupCall},
		),
		newGoldenFact(
			"caddyfile-error-boundary",
			"The bounded proof stops when the built-in adapter returns the parser error. It does not establish CLI presentation, and it does not generalize this request-matcher path to lexer errors, generic syntax helpers, or other Caddyfile error families.",
			[]string{"known_unknowns"},
			[]string{"evidence boundary", "CLI unknown", "other error families"},
			[]semanticdiscovery.Capability{
				semanticdiscovery.CapabilityLimitation,
			},
			adapterPropagation,
			nil,
		),
	}
	groups := []string{
		"caddyfile-parser-addresses",
		"caddyfile-dispenser-errors",
		"caddyfile-parser-stack",
		"caddyfile-adapter",
		"caddyfile-parser-test",
		"caddyfile-adapter",
		"caddyfile-boundary",
	}
	for index := range facts {
		facts[index].SourceGroup = goldenStableID("gmsg", groups[index])
	}

	aspectDefinitions := goldenCaddyfileErrorAspectDefinitions()
	aspects := make([]semanticdiscovery.AnswerAspect, 0, len(aspectDefinitions))
	for _, definition := range aspectDefinitions {
		aspects = append(aspects, definition.Aspect)
	}
	candidate := original
	candidate.Title = goldenCaddyfileErrorTitle
	candidate.QuestionAnswered = goldenCaddyfileErrorQuestion
	candidate.MissingInformation = []string{
		"CLI presentation is outside the bounded proof.",
		"The selected request-matcher path does not represent lexer errors, generic syntax helpers, or every Caddyfile error family.",
	}
	candidate.ExpectedValue = semanticdiscovery.ExpectedValueHigh
	candidate.Confidence = semanticdiscovery.ConfidenceHigh
	candidate.EnrichmentSupportIDs = make([]string, 0, len(facts))
	for _, fact := range facts {
		candidate.EnrichmentSupportIDs = append(candidate.EnrichmentSupportIDs, fact.ID)
	}
	sort.Strings(candidate.EnrichmentSupportIDs)
	candidate.CapabilityContract = &semanticdiscovery.CapabilityContract{
		RequiredCapabilities: []semanticdiscovery.Capability{
			semanticdiscovery.CapabilityStatic,
			semanticdiscovery.CapabilityDirectCall,
			semanticdiscovery.CapabilitySequence,
			semanticdiscovery.CapabilityBranch,
			semanticdiscovery.CapabilityErrorPath,
			semanticdiscovery.CapabilityOutputEffect,
			semanticdiscovery.CapabilityTestEvidence,
			semanticdiscovery.CapabilityLimitation,
		},
		AvailableCapabilities: []semanticdiscovery.Capability{
			semanticdiscovery.CapabilityStatic,
			semanticdiscovery.CapabilityDirectCall,
			semanticdiscovery.CapabilitySequence,
			semanticdiscovery.CapabilityBranch,
			semanticdiscovery.CapabilityErrorPath,
			semanticdiscovery.CapabilityTestEvidence,
			semanticdiscovery.CapabilityLimitation,
		},
		MissingCapabilities: []semanticdiscovery.Capability{
			semanticdiscovery.CapabilityOutputEffect,
		},
		Resolution: semanticdiscovery.CapabilityResolutionPartial,
	}
	candidate.IntentContract = &semanticdiscovery.IntentContract{
		RequiredAnswerAspects: aspects,
		MinCovered:            6,
		MinKeyCovered:         4,
		LocalSearchAliases:    append([]string(nil), goldenCaddyfileErrorAliases...),
	}
	projectedBundle := bundle
	projectedBundle.Facts = append(append([]semanticdiscovery.Fact(nil), bundle.Facts...), facts...)
	normalized, normalization := semanticdiscovery.NormalizeOpportunityProposal(
		projectedBundle,
		semanticdiscovery.OpportunityProposal{
			Version:    semanticdiscovery.OpportunityProposalVersion,
			Candidates: []semanticdiscovery.OpportunityCandidate{candidate},
		},
	)
	if len(normalization.Issues) != 0 || len(normalized.Candidates) != 1 ||
		normalized.Candidates[0].ID != goldenCaddyfileErrorCandidateID {
		return goldenProjection{}, fmt.Errorf(
			"golden mechanism v1: locally normalized candidate identity changed",
		)
	}
	candidate = normalized.Candidates[0]
	proposal := semanticdiscovery.OpportunityProposal{
		Version:    semanticdiscovery.OpportunityProposalVersion,
		Candidates: []semanticdiscovery.OpportunityCandidate{candidate},
	}
	if err := semanticdiscovery.ValidateOpportunityProposal(projectedBundle, proposal); err != nil {
		return goldenProjection{}, fmt.Errorf("golden mechanism v1: projected candidate contract: %w", err)
	}
	return goldenProjection{Candidate: candidate, Facts: facts}, nil
}

func buildCaddyfileErrorLeaf(
	bundle semanticdiscovery.Bundle,
	candidate semanticdiscovery.OpportunityCandidate,
) (semanticdiscovery.LeafResult, error) {
	tasks, err := semanticdiscovery.PlanLeafTasks(
		bundle,
		[]semanticdiscovery.OpportunityCandidate{candidate},
	)
	if err != nil {
		return semanticdiscovery.LeafResult{}, err
	}
	if len(tasks) != 1 {
		return semanticdiscovery.LeafResult{}, fmt.Errorf("golden mechanism v1: expected one leaf task")
	}
	facts := make(map[string]semanticdiscovery.Fact, len(tasks[0].Facts))
	for _, fact := range tasks[0].Facts {
		facts[fact.ID] = fact
	}
	limitationID := ""
	artifact := semanticdiscovery.LeafArtifact{
		Version:     semanticdiscovery.LeafArtifactVersion,
		TaskID:      tasks[0].ID,
		CandidateID: candidate.ID,
		Status:      semanticdiscovery.LeafStatusUsable,
	}
	for _, id := range candidate.EnrichmentSupportIDs {
		fact, exists := facts[id]
		if !exists {
			return semanticdiscovery.LeafResult{}, fmt.Errorf(
				"golden mechanism v1: enriched fact %q was lost before leaf planning",
				id,
			)
		}
		if slices.Contains(fact.Keywords, "answer_aspect:known_unknowns") {
			limitationID = id
			continue
		}
		artifact.Observations = append(
			artifact.Observations,
			semanticdiscovery.LeafObservation{Text: fact.Statement, SupportIDs: []string{id}},
		)
	}
	if limitationID == "" {
		return semanticdiscovery.LeafResult{}, fmt.Errorf("golden mechanism v1: boundary fact is unavailable")
	}
	artifact.MissingEvidence = []semanticdiscovery.LeafMissingEvidence{{
		Explanation: "The bounded evidence stops at the built-in adapter return and does not establish user-visible CLI output or the behavior of other Caddyfile error families.",
		SupportIDs:  []string{limitationID},
		MissingCapabilities: []semanticdiscovery.Capability{
			semanticdiscovery.CapabilityOutputEffect,
		},
	}}
	artifact.CandidateConnection = semanticdiscovery.LeafCandidateConnection{
		CandidateID: candidate.ID,
		Relation:    "needs_combination",
		Explanation: "The locally verified error mechanism facts require one bounded editorial synthesis.",
		SupportIDs:  append([]string(nil), candidate.EnrichmentSupportIDs...),
	}
	artifact = semanticdiscovery.NormalizeLeafArtifact(artifact)
	if err := semanticdiscovery.ValidateLeafArtifact(tasks[0], artifact); err != nil {
		order := make([]string, 0, len(artifact.Observations))
		for _, observation := range artifact.Observations {
			fact := facts[observation.SupportIDs[0]]
			order = append(order, fact.ID+": "+fact.Statement)
		}
		return semanticdiscovery.LeafResult{}, fmt.Errorf(
			"golden mechanism v1: validate leaf over observation order %v: %w",
			order,
			err,
		)
	}
	return semanticdiscovery.LeafResult{Task: tasks[0], Artifact: artifact}, nil
}
