package programcategorization

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/documentationreduce"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
)

type presetProvider struct {
	mu           sync.Mutex
	requests     []Request
	prompts      []llm.Prompt
	maxOwnedRefs int
	maxDocuments int
	respond      func(Request, int) []byte
}

func (provider *presetProvider) State() []byte {
	return []byte(`{"provider":"program-categorization-preset-v1"}`)
}

func (provider *presetProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	var request Request
	if err := json.Unmarshal([]byte(prompt.User), &request); err != nil {
		return llm.Prepared{}, err
	}
	if provider.maxOwnedRefs > 0 && len(request.CategorizeRefs) > provider.maxOwnedRefs {
		return llm.Prepared{}, llm.NewResourceLimitError(llm.ResourceLimitError{
			Kind: llm.ResourceLimitRequestBytes, Limit: provider.maxOwnedRefs,
			Observed: len(request.CategorizeRefs), ObservedKnown: true,
		})
	}
	if provider.maxDocuments > 0 && len(request.Documentation) > provider.maxDocuments {
		return llm.Prepared{}, llm.NewResourceLimitError(llm.ResourceLimitError{
			Kind: llm.ResourceLimitRequestBytes, Limit: provider.maxDocuments,
			Observed: len(request.Documentation), ObservedKnown: true,
		})
	}
	wire, err := json.Marshal(struct {
		System    string `json:"system"`
		User      string `json:"user"`
		JSON      bool   `json:"json"`
		MaxTokens int    `json:"max_tokens"`
	}{prompt.System, prompt.User, prompt.ResponseFormatJSON, limits.MaxOutputTokens})
	if err != nil {
		return llm.Prepared{}, err
	}
	if len(wire) > limits.MaxRequestBytes {
		return llm.Prepared{}, llm.NewResourceLimitError(llm.ResourceLimitError{
			Kind: llm.ResourceLimitRequestBytes, Limit: limits.MaxRequestBytes,
			Observed: len(wire), ObservedKnown: true,
		})
	}
	return llm.NewPrepared(wire)
}

func (provider *presetProvider) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var envelope struct {
		System string `json:"system"`
		User   string `json:"user"`
	}
	if err := json.Unmarshal(prepared.Bytes(), &envelope); err != nil {
		return llm.Completion{}, err
	}
	var request Request
	if err := json.Unmarshal([]byte(envelope.User), &request); err != nil {
		return llm.Completion{}, err
	}
	if err := validatePresetRequest(request); err != nil {
		return llm.Completion{}, err
	}
	provider.mu.Lock()
	position := len(provider.requests)
	provider.requests = append(provider.requests, request)
	provider.prompts = append(provider.prompts, llm.Prompt{System: envelope.System, User: envelope.User, ResponseFormatJSON: true})
	provider.mu.Unlock()
	response := []byte(`{"assignments":[]}`)
	if provider.respond != nil {
		response = provider.respond(request, position)
	}
	return llm.Completion{
		Response: response, FinishReason: llm.FinishStop, ChoiceCount: 1,
		Metrics: llm.Metrics{Attempts: 1, Latency: time.Millisecond},
	}, nil
}

func validatePresetRequest(request Request) error {
	if request.Version != requestVersion || request.Subjects == nil || request.Edges == nil ||
		request.Documentation == nil || request.CategorizeRefs == nil {
		return fmt.Errorf("preset: incomplete request")
	}
	subjects := make(map[string]struct{}, len(request.Subjects))
	arguments := make(map[string]struct{})
	for _, subject := range request.Subjects {
		if subject.Ref == "" {
			return fmt.Errorf("preset: empty subject ref")
		}
		subjects[subject.Ref] = struct{}{}
		for _, argument := range subject.Arguments {
			if argument.Ref == "" {
				return fmt.Errorf("preset: empty argument ref")
			}
			if _, duplicate := arguments[argument.Ref]; duplicate {
				return fmt.Errorf("preset: duplicate argument ref %s", argument.Ref)
			}
			arguments[argument.Ref] = struct{}{}
		}
	}
	for _, ref := range request.CategorizeRefs {
		if _, known := subjects[ref]; !known {
			return fmt.Errorf("preset: owned ref %s has no subject row", ref)
		}
	}
	for _, edge := range request.Edges {
		if _, known := subjects[edge.FromRef]; !known {
			return fmt.Errorf("preset: edge %s has no from subject", edge.Ref)
		}
		if _, known := subjects[edge.ToRef]; !known {
			return fmt.Errorf("preset: edge %s has no to subject", edge.Ref)
		}
	}
	for _, subject := range request.Subjects {
		for _, argument := range subject.Arguments {
			for _, ref := range argument.ObjectRefs {
				if _, known := subjects[ref]; !known {
					return fmt.Errorf("preset: argument has unknown object ref %s", ref)
				}
			}
			for _, candidate := range argument.ValueCandidates {
				for _, ref := range candidate.SourceObjectRefs {
					if _, known := subjects[ref]; !known {
						return fmt.Errorf("preset: candidate has unknown source object ref %s", ref)
					}
				}
				for _, ref := range candidate.SourceArgumentRefs {
					if _, known := arguments[ref]; !known {
						return fmt.Errorf("preset: candidate has unknown source argument ref %s", ref)
					}
				}
			}
		}
	}
	return nil
}

func TestRunRestoresSparseCategoriesAndDiscardsUnsupportedRows(t *testing.T) {
	index := categorizationTestIndex(t, "go")
	documentation := reducedDocumentationFixture(t)
	provider := &presetProvider{}
	provider.respond = func(request Request, _ int) []byte {
		var configureRef, handlerRef, patternRef string
		for _, subject := range request.Subjects {
			switch {
			case subject.Name == "configure":
				configureRef = subject.Ref
			case subject.Name == "handleOrder":
				handlerRef = subject.Ref
			case subject.Selector == "HandleFunc":
				patternRef = subject.Ref
			}
		}
		return []byte(fmt.Sprintf(`{
  "assignments": [
    {"ref": %q, "categories": ["core", "inbound", "core"]},
    {"ref": %q, "categories": ["dependency"]},
    {"ref": %q, "categories": ["background_activity"]},
    {"ref": %q, "categories": ["support"]},
    {"ref": %q, "categories": []},
    {"ref": "g999", "categories": ["core"]},
    {"ref": 42, "categories": ["core"]}
  ]
}`, configureRef, configureRef, patternRef, configureRef, handlerRef))
	}

	result, err := Run(t.Context(), llm.Executor{Enabled: false}, provider, index, documentation)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	configureID := subjectIDBySourceRef(t, index, "configure")
	patternID := patternIDBySourceRef(t, index, "registration-pattern")
	wantAssignments := []Assignment{
		{SubjectID: configureID, Categories: []Category{CategoryCore, CategoryDependency, CategoryInbound}},
		{SubjectID: patternID, Categories: []Category{CategoryBackgroundActivity}},
	}
	sort.Slice(wantAssignments, func(i, j int) bool { return wantAssignments[i].SubjectID < wantAssignments[j].SubjectID })
	if !reflect.DeepEqual(result.Assignments, wantAssignments) {
		t.Fatalf("assignments = %#v, want %#v", result.Assignments, wantAssignments)
	}
	wantDiagnostics := []Diagnostic{
		{Kind: DiagnosticEmptyCategories, Count: 1},
		{Kind: DiagnosticInvalidCategory, Count: 1},
		{Kind: DiagnosticMalformedRow, Count: 1},
		{Kind: DiagnosticUnknownRef, Count: 1},
	}
	if !reflect.DeepEqual(result.Diagnostics, wantDiagnostics) {
		t.Fatalf("diagnostics = %#v, want %#v", result.Diagnostics, wantDiagnostics)
	}
	if result.ReducedDocumentationSHA256 != documentation.ReductionSHA256 {
		t.Fatalf("documentation digest = %q", result.ReducedDocumentationSHA256)
	}
	enriched, err := result.Enrich(index, documentation)
	if err != nil {
		t.Fatalf("ProgramIndex.Enrich: %v", err)
	}
	if enriched.Categorization == nil ||
		enriched.Categorization.ReducedDocumentationSHA256 != documentation.ReductionSHA256 || !reflect.DeepEqual(
		enriched.Categorization.Assignments, result.EnrichmentAssignments(),
	) {
		t.Fatalf("enriched categorization = %#v", enriched.Categorization)
	}

	provider.mu.Lock()
	requests := append([]Request(nil), provider.requests...)
	prompts := append([]llm.Prompt(nil), provider.prompts...)
	provider.mu.Unlock()
	if len(requests) != 1 || len(requests[0].Documentation) != 3 {
		t.Fatalf("provider requests/documentation = %d / %#v", len(requests), requests)
	}
	kinds := make(map[documentationKind]bool)
	for _, row := range requests[0].Documentation {
		kinds[row.Kind] = true
	}
	if !kinds[documentationOverview] || !kinds[documentationClaim] || !kinds[documentationConcept] {
		t.Fatalf("documentation arrow lost reducer fields: %#v", requests[0].Documentation)
	}
	if strings.Contains(prompts[0].User, configureID) || strings.Contains(prompts[0].User, patternID) {
		t.Fatal("provider request leaked canonical ProgramIndex identities")
	}
	if strings.Contains(prompts[0].System, `"entrypoint"`) ||
		strings.Contains(prompts[0].System, `"support"`) {
		t.Fatal("prompt advertises a removed category")
	}
	if !strings.Contains(prompts[0].System, "external_authority_kind") ||
		!strings.Contains(prompts[0].System, `"platform"`) ||
		!strings.Contains(prompts[0].System, "complete exact external target") ||
		!strings.Contains(prompts[0].System, "Do not assign `dependency`") {
		t.Fatal("prompt lost reserved-platform dependency exclusion")
	}
}

func TestRunAcceptsSparseEmptyResponseForEveryLanguage(t *testing.T) {
	for _, language := range []string{"go", "python", "jsts"} {
		t.Run(language, func(t *testing.T) {
			index := categorizationNeutralIndex(t, language, 1)
			provider := &presetProvider{}
			documentation, err := documentationreduce.Run(
				t.Context(), llm.Executor{}, nil, readmetargetscout.GuidanceSnapshot{},
			)
			if err != nil {
				t.Fatalf("empty documentation reduction: %v", err)
			}
			result, err := Run(
				t.Context(), llm.Executor{Enabled: false}, provider, index, documentation,
			)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if len(result.Assignments) != 0 || result.Assignments == nil || result.Diagnostics == nil {
				t.Fatalf("sparse result = %#v", result)
			}
			provider.mu.Lock()
			requests := append([]Request(nil), provider.requests...)
			prompts := append([]llm.Prompt(nil), provider.prompts...)
			provider.mu.Unlock()
			if len(requests) != 1 || requests[0].Target.Language != language ||
				len(requests[0].CategorizeRefs) == 0 {
				t.Fatalf("language-neutral request = %#v", requests)
			}
			if len(prompts) != 1 ||
				!strings.Contains(prompts[0].System, "Evaluate the four categories") ||
				!strings.Contains(prompts[0].System, "positively supported") ||
				!strings.Contains(prompts[0].System, "only when no owned") ||
				!strings.Contains(prompts[0].System, "merely to acknowledge every ref") {
				t.Fatalf("sparse-complete prompt contract = %#v", prompts)
			}
		})
	}
}

func TestRunDiscardsReservedPlatformDependencyClaims(t *testing.T) {
	index := categorizationPlatformIndex(t)
	documentation := reducedDocumentationFixture(t)
	provider := &presetProvider{respond: func(request Request, _ int) []byte {
		rows := make([]string, 0, 4)
		for _, subject := range request.Subjects {
			switch {
			case subject.ExternalAuthority == programindex.ExternalAuthorityPlatform:
				rows = append(rows, fmt.Sprintf(
					`{"ref":%q,"categories":["dependency","core"]}`, subject.Ref,
				))
			case subject.ExternalPackage == "axios":
				rows = append(rows, fmt.Sprintf(
					`{"ref":%q,"categories":["dependency"]}`, subject.Ref,
				))
			case subject.PatternForm != "" && subject.Selector == "requestAnimationFrame":
				rows = append(rows, fmt.Sprintf(
					`{"ref":%q,"categories":["dependency","background_activity"]}`, subject.Ref,
				))
			case subject.PatternForm != "" && subject.Selector == "post":
				rows = append(rows, fmt.Sprintf(
					`{"ref":%q,"categories":["dependency"]}`, subject.Ref,
				))
			}
		}
		return []byte(`{"assignments":[` + strings.Join(rows, ",") + `]}`)
	}}

	result, err := Run(t.Context(), llm.Executor{Enabled: false}, provider, index, documentation)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	wantAssignments := []Assignment{
		{SubjectID: subjectIDBySourceRef(t, index, "axios-post"), Categories: []Category{CategoryDependency}},
		{SubjectID: subjectIDBySourceRef(t, index, "platform-raf"), Categories: []Category{CategoryCore}},
		{SubjectID: patternIDBySourceRef(t, index, "axios-call-pattern"), Categories: []Category{CategoryDependency}},
		{SubjectID: patternIDBySourceRef(t, index, "platform-call-pattern"), Categories: []Category{CategoryBackgroundActivity}},
	}
	sort.Slice(wantAssignments, func(i, j int) bool {
		return wantAssignments[i].SubjectID < wantAssignments[j].SubjectID
	})
	if !reflect.DeepEqual(result.Assignments, wantAssignments) {
		t.Fatalf("assignments = %#v, want %#v", result.Assignments, wantAssignments)
	}
	if !reflect.DeepEqual(result.Diagnostics, []Diagnostic{{
		Kind: DiagnosticUnsupportedCategory, Count: 2,
	}}) {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	if _, err := result.Enrich(index, documentation); err != nil {
		t.Fatalf("Enrich after filtering reserved platform claims: %v", err)
	}
}

func TestResultValidateRejectsReservedPlatformDependencyClaim(t *testing.T) {
	index := categorizationPlatformIndex(t)
	documentation := reducedDocumentationFixture(t)
	result := Result{
		ProgramTargetID:            index.Target.ID,
		BaseProgramIndexSHA256:     index.SHA256,
		ReducedDocumentationSHA256: documentation.ReductionSHA256,
		Assignments: []Assignment{{
			SubjectID:  subjectIDBySourceRef(t, index, "platform-raf"),
			Categories: []Category{CategoryDependency},
		}},
		Diagnostics: []Diagnostic{},
	}
	if err := result.Validate(index, documentation); err == nil ||
		!strings.Contains(err.Error(), "unsupported for subject") {
		t.Fatalf("Validate reserved platform dependency error = %v", err)
	}
}

func TestRunPartitionsLargePositiveCoverWhenProviderEnvelopeFits(t *testing.T) {
	index := categorizationNeutralIndex(t, "jsts", ownedSubjectsPerRequest*2+1)
	documentation := reducedDocumentationFixture(t)
	provider := &presetProvider{respond: func(request Request, _ int) []byte {
		return []byte(fmt.Sprintf(`{"assignments":[{"ref":%q,"categories":["core"]}]}`,
			request.CategorizeRefs[0]))
	}}
	result, err := Run(t.Context(), llm.Executor{
		Enabled: false, BatchConcurrency: 4, BatchController: &llm.BatchController{},
	}, provider, index, documentation)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Assignments) != 3 {
		t.Fatalf("cross-request positive union = %#v, want one row from each of 3 requests", result.Assignments)
	}
	for _, assignment := range result.Assignments {
		if !reflect.DeepEqual(assignment.Categories, []Category{CategoryCore}) {
			t.Fatalf("cross-request categories = %#v", result.Assignments)
		}
	}
	provider.mu.Lock()
	requests := append([]Request(nil), provider.requests...)
	provider.mu.Unlock()
	if len(requests) != 3 {
		t.Fatalf("semantic request count = %d, want 3", len(requests))
	}
	covered := make(map[string]int)
	batchSizes := make([]int, 0, len(requests))
	var documentationRefs []string
	for _, request := range requests {
		if len(request.CategorizeRefs) == 0 || len(request.CategorizeRefs) > ownedSubjectsPerRequest {
			t.Fatalf("owned ref batch size = %d", len(request.CategorizeRefs))
		}
		batchSizes = append(batchSizes, len(request.CategorizeRefs))
		refs := make([]string, 0, len(request.Documentation))
		for _, row := range request.Documentation {
			refs = append(refs, row.Ref)
		}
		if documentationRefs == nil {
			documentationRefs = refs
		} else if !reflect.DeepEqual(refs, documentationRefs) {
			t.Fatalf("complete documentation was not repeated: %#v versus %#v", refs, documentationRefs)
		}
		for _, ref := range request.CategorizeRefs {
			covered[ref]++
		}
	}
	sort.Ints(batchSizes)
	if !reflect.DeepEqual(batchSizes, []int{1, ownedSubjectsPerRequest, ownedSubjectsPerRequest}) ||
		len(documentationRefs) != 3 {
		t.Fatalf("request sizes/documentation = %#v / %#v", batchSizes, documentationRefs)
	}
	compilation, err := Compile(index, documentation)
	if err != nil {
		t.Fatal(err)
	}
	if len(covered) != len(compilation.subjects) {
		t.Fatalf("covered subjects = %d, want %d", len(covered), len(compilation.subjects))
	}
	for _, subject := range compilation.subjects {
		if covered[subject.ref] != 1 {
			t.Fatalf("subject %s appears %d times in disjoint cover", subject.ref, covered[subject.ref])
		}
	}
	oversized := make([]string, 0, ownedSubjectsPerRequest+1)
	for _, subject := range compilation.subjects[:ownedSubjectsPerRequest+1] {
		oversized = append(oversized, subject.ref)
	}
	if err := compilation.validatePlan([]batch{{
		subjectRefs: oversized, documentationRefs: documentationRefs,
	}}); err == nil || !strings.Contains(err.Error(), "above semantic request granularity") {
		t.Fatalf("oversized semantic request plan error = %v", err)
	}
}

func TestRequestProjectsValueProvenanceWithOnlyRequestLocalRefs(t *testing.T) {
	index := categorizationValueProvenanceIndex(t)
	documentation := reducedDocumentationFixture(t)
	compilation, err := Compile(index, documentation)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	var useRef string
	for _, subject := range compilation.subjects {
		if subject.pattern != nil && subject.pattern.SourceRef == "formal-use-pattern" {
			useRef = subject.ref
		}
	}
	if useRef == "" {
		t.Fatal("formal-use pattern has no subject ref")
	}
	request, err := compilation.request([]string{useRef}, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}

	arguments := make(map[string]argumentWire)
	var use argumentWire
	for _, subject := range request.Subjects {
		for _, argument := range subject.Arguments {
			arguments[argument.Ref] = argument
			if subject.Selector == "get" && argument.Position == 1 {
				use = argument
			}
		}
	}
	if use.Ref == "" || len(use.ValueCandidates) != 2 || use.ValueCandidatesObserved != 2 || use.ValueCandidatesOmitted != 0 {
		t.Fatalf("formal argument provenance = %#v", use)
	}
	var initializer, actual valueCandidateWire
	for _, candidate := range use.ValueCandidates {
		switch candidate.SourceKind {
		case programindex.PatternValueSourceInitializer:
			initializer = candidate
		case programindex.PatternValueSourceActualArgument:
			actual = candidate
		}
	}
	if initializer.Value != "/api/dynamic" || initializer.Resolution != programindex.PatternValuePossible ||
		len(initializer.SourceObjectRefs) != 1 || initializer.SourceObjectsObserved != 1 ||
		initializer.SourceObjectsOmitted != 0 || len(initializer.SourceArgumentRefs) != 0 {
		t.Fatalf("initializer projection = %#v", initializer)
	}
	if actual.Value != "/products/runtime" || actual.Resolution != programindex.PatternValuePossible ||
		len(actual.SourceArgumentRefs) != 1 || actual.SourceArgumentsObserved != 1 ||
		actual.SourceArgumentsOmitted != 0 || len(actual.SourceObjectRefs) != 0 {
		t.Fatalf("actual-argument projection = %#v", actual)
	}
	source, known := arguments[actual.SourceArgumentRefs[0]]
	if !known || source.Value != "/products/runtime" || source.Kind != programindex.PatternLiteralString {
		t.Fatalf("actual source ref does not resolve inside request: %q => %#v", actual.SourceArgumentRefs[0], source)
	}

	wire, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, relation := range index.Relations {
		for _, pattern := range relation.Patterns {
			for _, argument := range pattern.Arguments {
				if strings.Contains(string(wire), argument.ID) {
					t.Fatalf("provider request leaked canonical argument ID %q", argument.ID)
				}
				for _, candidate := range argument.ValueCandidates {
					if strings.Contains(string(wire), candidate.ID) {
						t.Fatalf("provider request leaked canonical candidate ID %q", candidate.ID)
					}
				}
			}
		}
	}
}

func TestRunExhaustivelyCrossesSubjectsWithDocumentationShards(t *testing.T) {
	index := categorizationTestIndex(t, "python")
	documentation := reducedDocumentationFixture(t)
	provider := &presetProvider{maxOwnedRefs: 1, maxDocuments: 1}
	provider.respond = func(request Request, _ int) []byte {
		category := "core"
		switch request.Documentation[0].Kind {
		case documentationOverview:
			category = "inbound"
		case documentationClaim:
			category = "dependency"
		case documentationConcept:
			category = "core"
		}
		return []byte(fmt.Sprintf(`{"assignments":[{"ref":%q,"categories":[%q]}]}`,
			request.CategorizeRefs[0], category))
	}
	result, err := Run(t.Context(), llm.Executor{
		Enabled: false, BatchConcurrency: 4, BatchController: &llm.BatchController{},
	}, provider, index, documentation)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	compilation, err := Compile(index, documentation)
	if err != nil {
		t.Fatal(err)
	}
	wantCalls := len(compilation.subjects) * len(compilation.documentationRows)
	provider.mu.Lock()
	requests := append([]Request(nil), provider.requests...)
	provider.mu.Unlock()
	if len(requests) != wantCalls {
		t.Fatalf("provider calls = %d, want subject×documentation cover %d", len(requests), wantCalls)
	}
	pairs := make(map[string]int)
	for _, request := range requests {
		if len(request.CategorizeRefs) != 1 || len(request.Documentation) != 1 {
			t.Fatalf("non-atomic forced shard = %#v", request)
		}
		pairs[request.CategorizeRefs[0]+"/"+request.Documentation[0].Ref]++
	}
	for _, subject := range compilation.subjects {
		for _, document := range compilation.documentationRows {
			if pairs[subject.ref+"/"+document.ref] != 1 {
				t.Fatalf("pair %s/%s count = %d", subject.ref, document.ref, pairs[subject.ref+"/"+document.ref])
			}
		}
	}
	for _, assignment := range result.Assignments {
		if !reflect.DeepEqual(assignment.Categories, []Category{CategoryCore, CategoryDependency, CategoryInbound}) {
			t.Fatalf("shard union for %s = %#v", assignment.SubjectID, assignment.Categories)
		}
	}
}

func categorizationTestIndex(t *testing.T, language string) programindex.Index {
	t.Helper()
	location := func(line int) *programindex.Location {
		return &programindex.Location{Path: "src/routes.lang", Line: line, Column: 2}
	}
	objects := []programindex.ObjectInput{
		{SourceRef: "module", Kind: programindex.ObjectModule, Name: "routes", Visibility: programindex.VisibilityPublic, Location: location(1)},
		{SourceRef: "configure", Kind: programindex.ObjectFunction, Name: "configure", Visibility: programindex.VisibilityInternal, ContainerRef: "module", Location: location(5)},
		{SourceRef: "handler", Kind: programindex.ObjectFunction, Name: "handleOrder", Visibility: programindex.VisibilityInternal, ContainerRef: "module", Location: location(20)},
		{SourceRef: "register", Kind: programindex.ObjectExternalSymbol, Name: "Router.HandleFunc", Visibility: programindex.VisibilityPublic, External: &programindex.ExternalSymbol{AuthorityKind: programindex.ExternalAuthorityPackage, PackagePath: "example.org/router", Receiver: "Router", Name: "HandleFunc"}},
	}
	relations := []programindex.RelationInput{{
		SourceRef: "registration", Kind: programindex.RelationInvokesExternal, FromRef: "configure",
		ToRefs: []string{"register"}, Resolution: programindex.ResolutionExact,
		Location: location(10), TargetsObserved: 1,
		Witnesses: []programindex.Witness{{Kind: "call", Location: location(10)}}, WitnessesObserved: 1,
		Patterns: []programindex.RelationPatternInput{{
			SourceRef: "registration-pattern", Form: programindex.PatternCall,
			Selector: "HandleFunc", Location: location(10), ArgumentsObserved: 2,
			Arguments: []programindex.PatternArgumentInput{
				{Position: 1, Kind: programindex.PatternLiteralString, Value: "/orders"},
				{Position: 2, Kind: programindex.PatternDynamic, ObjectRefs: []string{"handler"}, Resolution: programindex.ResolutionExact, ObjectsObserved: 1},
			},
		}}, PatternsObserved: 1,
	}}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: language, Kind: "application", Name: "orders", Selector: language + ":orders",
			Sources: []programindex.TargetSource{{FileRef: "f1", Path: "src/routes.lang"}}, AnchorFileRef: "f1",
			Seeds: []programindex.TargetSeedInput{{ObjectRef: "configure", Kind: programindex.SeedCallable, Location: location(5)}},
		},
		Objects: objects, Relations: relations,
		Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: len(objects), RelationsObserved: len(relations)},
	})
	if err != nil {
		t.Fatalf("programindex.New: %v", err)
	}
	return index
}

func categorizationPlatformIndex(t *testing.T) programindex.Index {
	t.Helper()
	location := func(line int) *programindex.Location {
		return &programindex.Location{Path: "src/runtime.ts", Line: line, Column: 1}
	}
	objects := []programindex.ObjectInput{
		{
			SourceRef: "animate", Kind: programindex.ObjectFunction,
			Name: "animate", Visibility: programindex.VisibilityInternal, Location: location(1),
		},
		{
			SourceRef: "platform-raf", Kind: programindex.ObjectExternalSymbol,
			Name: "platform:javascript.requestAnimationFrame", Visibility: programindex.VisibilityPublic,
			External: &programindex.ExternalSymbol{
				AuthorityKind: programindex.ExternalAuthorityPlatform,
				PackagePath:   "platform:javascript", Name: "requestAnimationFrame",
			},
		},
		{
			SourceRef: "axios-post", Kind: programindex.ObjectExternalSymbol,
			Name: "axios.default.post", Visibility: programindex.VisibilityPublic,
			External: &programindex.ExternalSymbol{
				AuthorityKind: programindex.ExternalAuthorityPackage,
				PackagePath:   "axios", Receiver: "default", Name: "post",
			},
		},
	}
	relations := []programindex.RelationInput{
		{
			SourceRef: "platform-call", Kind: programindex.RelationInvokesExternal,
			FromRef: "animate", ToRefs: []string{"platform-raf"},
			Resolution: programindex.ResolutionExact, Location: location(3), TargetsObserved: 1,
			Witnesses:         []programindex.Witness{{Kind: "compiler", Location: location(3)}},
			WitnessesObserved: 1, PatternsObserved: 1,
			Patterns: []programindex.RelationPatternInput{{
				SourceRef: "platform-call-pattern", Form: programindex.PatternCall,
				Selector: "requestAnimationFrame", Location: location(3),
				Arguments: []programindex.PatternArgumentInput{}, ArgumentsObserved: 0,
			}},
		},
		{
			SourceRef: "axios-call", Kind: programindex.RelationInvokesExternal,
			FromRef: "animate", ToRefs: []string{"axios-post"},
			Resolution: programindex.ResolutionExact, Location: location(4), TargetsObserved: 1,
			Witnesses:         []programindex.Witness{{Kind: "compiler", Location: location(4)}},
			WitnessesObserved: 1, PatternsObserved: 1,
			Patterns: []programindex.RelationPatternInput{{
				SourceRef: "axios-call-pattern", Form: programindex.PatternCall,
				Selector: "post", Location: location(4),
				Arguments: []programindex.PatternArgumentInput{}, ArgumentsObserved: 0,
			}},
		},
	}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("1", 64), SourceSHA256: strings.Repeat("2", 64),
		Target: programindex.TargetInput{
			Language: "jsts", Kind: "application", Name: "runtime", Selector: "jsts:runtime",
			Sources:       []programindex.TargetSource{{FileRef: "f1", Path: "src/runtime.ts"}},
			AnchorFileRef: "f1",
			Seeds: []programindex.TargetSeedInput{{
				ObjectRef: "animate", Kind: programindex.SeedCallable, Location: location(1),
			}},
		},
		Objects: objects, Relations: relations,
		Coverage: programindex.CoverageInput{
			Measured: true, ObjectsObserved: len(objects), RelationsObserved: len(relations),
		},
	})
	if err != nil {
		t.Fatalf("programindex.New platform fixture: %v", err)
	}
	return index
}

func categorizationNeutralIndex(t *testing.T, language string, objectCount int) programindex.Index {
	t.Helper()
	if objectCount < 1 {
		t.Fatal("neutral categorization fixture requires at least one object")
	}
	objects := make([]programindex.ObjectInput, 0, objectCount)
	for position := 0; position < objectCount; position++ {
		objects = append(objects, programindex.ObjectInput{
			SourceRef:  fmt.Sprintf("neutral-%04d", position),
			Kind:       programindex.ObjectVariable,
			Name:       fmt.Sprintf("neutralValue%04d", position),
			Visibility: programindex.VisibilityInternal,
			Location: &programindex.Location{
				Path: "src/neutral.lang", Line: position + 1, Column: 1,
			},
		})
	}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("7", 64), SourceSHA256: strings.Repeat("8", 64),
		Target: programindex.TargetInput{
			Language: language, Kind: "library", Name: "neutral", Selector: language + ":neutral",
			Sources: []programindex.TargetSource{{FileRef: "f1", Path: "src/neutral.lang"}}, AnchorFileRef: "f1",
		},
		Objects: objects, Relations: []programindex.RelationInput{},
		Coverage: programindex.CoverageInput{
			Measured: true, ObjectsObserved: len(objects), RelationsObserved: 0,
		},
	})
	if err != nil {
		t.Fatalf("programindex.New neutral fixture: %v", err)
	}
	return index
}

func categorizationValueProvenanceIndex(t *testing.T) programindex.Index {
	t.Helper()
	location := func(line int) *programindex.Location {
		return &programindex.Location{Path: "src/runtime.ts", Line: line, Column: 1}
	}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("e", 64), SourceSHA256: strings.Repeat("f", 64),
		Target: programindex.TargetInput{
			Language: "jsts", Kind: "application", Name: "runtime", Selector: "jsts:runtime",
			Sources: []programindex.TargetSource{{FileRef: "f1", Path: "src/runtime.ts"}}, AnchorFileRef: "f1",
			Seeds: []programindex.TargetSeedInput{{ObjectRef: "caller", Kind: programindex.SeedCallable, Location: location(1)}},
		},
		Objects: []programindex.ObjectInput{
			{SourceRef: "caller", Kind: programindex.ObjectFunction, Name: "boot", Visibility: programindex.VisibilityInternal, Location: location(1)},
			{SourceRef: "formal", Kind: programindex.ObjectFunction, Name: "register", Visibility: programindex.VisibilityInternal, Location: location(5)},
			{SourceRef: "get", Kind: programindex.ObjectExternalSymbol, Name: "router.get", Visibility: programindex.VisibilityPublic, External: &programindex.ExternalSymbol{AuthorityKind: programindex.ExternalAuthorityPackage, PackagePath: "router", Receiver: "router", Name: "get"}},
			{SourceRef: "path-constant", Kind: programindex.ObjectVariable, Name: "dynamicPath", Visibility: programindex.VisibilityInternal, Location: location(4)},
		},
		Relations: []programindex.RelationInput{
			{
				SourceRef: "formal-use", Kind: programindex.RelationInvokesExternal, FromRef: "formal", ToRefs: []string{"get"},
				Resolution: programindex.ResolutionExact, TargetsObserved: 1,
				Witnesses: []programindex.Witness{{Kind: "syntax", Location: location(6)}}, WitnessesObserved: 1,
				PatternsObserved: 1, Patterns: []programindex.RelationPatternInput{{
					SourceRef: "formal-use-pattern", Form: programindex.PatternCall, Selector: "get", Location: location(6),
					ArgumentsObserved: 1, Arguments: []programindex.PatternArgumentInput{{
						Position: 1, Kind: programindex.PatternDynamic, ValueCandidatesObserved: 2,
						ValueCandidates: []programindex.PatternValueCandidateInput{
							{Kind: programindex.PatternLiteralString, Value: "/api/dynamic", Resolution: programindex.PatternValuePossible, SourceKind: programindex.PatternValueSourceInitializer, SourceObjectRefs: []string{"path-constant"}, SourceObjectsObserved: 1},
							{Kind: programindex.PatternLiteralString, Value: "/products/runtime", Resolution: programindex.PatternValuePossible, SourceKind: programindex.PatternValueSourceActualArgument, SourceArgumentRefs: []programindex.PatternArgumentRefInput{{RelationSourceRef: "actual-call", PatternSourceRef: "actual-pattern", Position: 1}}, SourceArgumentsObserved: 1},
						},
					}},
				}},
			},
			{
				SourceRef: "actual-call", Kind: programindex.RelationCalls, FromRef: "caller", ToRefs: []string{"formal"},
				Resolution: programindex.ResolutionExact, TargetsObserved: 1,
				Witnesses: []programindex.Witness{{Kind: "syntax", Location: location(2)}}, WitnessesObserved: 1,
				PatternsObserved: 1, Patterns: []programindex.RelationPatternInput{{
					SourceRef: "actual-pattern", Form: programindex.PatternCall, Selector: "register", Location: location(2),
					ArgumentsObserved: 1, Arguments: []programindex.PatternArgumentInput{{Position: 1, Kind: programindex.PatternLiteralString, Value: "/products/runtime"}},
				}},
			},
		},
		Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: 4, RelationsObserved: 2},
	})
	if err != nil {
		t.Fatalf("programindex.New provenance fixture: %v", err)
	}
	return index
}

func reducedDocumentationFixture(t *testing.T) documentationreduce.Result {
	t.Helper()
	result := documentationreduce.Result{
		GuidanceSHA256: strings.Repeat("c", 64),
		Overview:       "Processes customer orders through an HTTP API.",
		Sources: []documentationreduce.Source{{
			Path: "README.md", Kind: readmetargetscout.GuidanceReadme,
			Claims:   []string{"Orders are persisted through the storage adapter."},
			Concepts: []string{"Order"},
		}},
	}
	wire, err := json.Marshal(struct {
		Version        int                          `json:"version"`
		GuidanceSHA256 string                       `json:"guidance_sha256"`
		Overview       string                       `json:"overview"`
		Sources        []documentationreduce.Source `json:"sources"`
	}{
		Version: documentationreduce.Version, GuidanceSHA256: result.GuidanceSHA256,
		Overview: result.Overview, Sources: result.Sources,
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(wire)
	result.ReductionSHA256 = hex.EncodeToString(digest[:])
	if err := result.Validate(); err != nil {
		t.Fatalf("documentation fixture: %v", err)
	}
	return result
}

func subjectIDBySourceRef(t *testing.T, index programindex.Index, sourceRef string) string {
	t.Helper()
	for _, object := range index.Objects {
		if object.SourceRef == sourceRef {
			return object.ID
		}
	}
	t.Fatalf("no object source ref %q", sourceRef)
	return ""
}

func patternIDBySourceRef(t *testing.T, index programindex.Index, sourceRef string) string {
	t.Helper()
	for _, relation := range index.Relations {
		for _, pattern := range relation.Patterns {
			if pattern.SourceRef == sourceRef {
				return pattern.ID
			}
		}
	}
	t.Fatalf("no pattern source ref %q", sourceRef)
	return ""
}
