package runtimeportfolio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/secretscan"
)

func TestSimpleSingleTargetTopology(t *testing.T) {
	input := singleTargetInput()
	compilation := mustCompile(t, input)
	wire := compilation.wire
	for _, localOnly := range []string{
		input.CapturedRevision,
		input.TargetPagePortfolioSHA256,
		input.Targets[0].ProgramTargetID,
	} {
		if bytes.Contains(wire, []byte(localOnly)) {
			t.Fatalf("provider request leaked local authority %q: %s", localOnly, wire)
		}
	}
	if compilation.Request.TargetCount != 1 || len(compilation.Request.Targets) != 1 ||
		compilation.Request.Targets[0].Ref != "t1" || len(compilation.Request.EvidenceCatalog) != 3 {
		t.Fatalf("request = %#v", compilation.Request)
	}
	if !strings.Contains(systemPrompt, "Prefer a simple topology") ||
		!strings.Contains(systemPrompt, "Do not promote every package") ||
		!strings.Contains(systemPrompt, "Use only advertised `t*` and `e*` refs") {
		t.Fatalf("system prompt lost topology or closed-ref contract: %s", systemPrompt)
	}

	result := mustResolve(t, compilation, responseRole{
		Name: "Authorization service", Purpose: "Serves authorization decisions.",
		Prominence: ProminencePrimary, Kind: RoleKindService,
		Requiredness: RequirednessRequired, Confidence: ConfidenceHigh,
		MappingStatus:   MappingMapped,
		Implementations: []responseImplementation{{TargetRef: "t1"}},
		EvidenceRefs:    []string{evidenceRefForLabel(t, compilation, "Starts the API server")},
	})
	if len(result.Roles) != 1 || len(result.Roles[0].Implementations) != 1 ||
		result.Roles[0].Implementations[0].ProgramTargetID != input.Targets[0].ProgramTargetID ||
		len(result.UnclassifiedTargetIDs) != 0 {
		t.Fatalf("result = %#v", result)
	}
	if result.Coverage != (Coverage{
		TargetsObserved: 1, TargetsMapped: 1, TargetsUnclassified: 0, Roles: 1,
		EvidenceAdvertised: 3, EvidenceSelected: 1,
	}) {
		t.Fatalf("coverage = %#v", result.Coverage)
	}
	if err := result.ValidateAgainst(input); err != nil {
		t.Fatalf("ValidateAgainst: %v", err)
	}
}

func TestResolvePreservesSeveralModesOnOneTarget(t *testing.T) {
	compilation := mustCompile(t, singleTargetInput())
	result := mustResolve(t, compilation, responseRole{
		Name: "Authorization service", Purpose: "Runs the API and migration modes.",
		Prominence: ProminencePrimary, Kind: RoleKindService,
		Requiredness: RequirednessRequired, Confidence: ConfidenceHigh,
		MappingStatus: MappingMapped,
		Implementations: []responseImplementation{
			{TargetRef: "t1", Mode: "serve"},
			{TargetRef: "t1", Mode: "migrate"},
		},
		EvidenceRefs: []string{evidenceRefForLabel(t, compilation, "Starts the API server")},
	})
	got := result.Roles[0].Implementations
	want := []Implementation{
		{ProgramTargetID: "program-target-server", Mode: "migrate"},
		{ProgramTargetID: "program-target-server", Mode: "serve"},
	}
	if !reflect.DeepEqual(got, want) || result.Coverage.TargetsMapped != 1 {
		t.Fatalf("implementations = %#v, coverage = %#v", got, result.Coverage)
	}
}

func TestResolvePreservesOneRoleAcrossSeveralTargets(t *testing.T) {
	input := twoTargetInput()
	compilation := mustCompile(t, input)
	serverEvidence := evidenceRefForLabel(t, compilation, "Starts the API server")
	workerEvidence := evidenceRefForLabel(t, compilation, "Starts the tuple worker")
	result := mustResolve(t, compilation, responseRole{
		Name: "Authorization runtime", Purpose: "Serves requests and processes tuple work.",
		Prominence: ProminencePrimary, Kind: RoleKindService,
		Requiredness: RequirednessRequired, Confidence: ConfidenceHigh,
		MappingStatus:   MappingMapped,
		Implementations: []responseImplementation{{TargetRef: "t2"}, {TargetRef: "t1"}},
		EvidenceRefs:    []string{workerEvidence, serverEvidence},
	})
	if got := result.Roles[0].Implementations; !reflect.DeepEqual(got, []Implementation{
		{ProgramTargetID: "program-target-server"},
		{ProgramTargetID: "program-target-worker"},
	}) {
		t.Fatalf("implementations = %#v", got)
	}
	if len(result.UnclassifiedTargetIDs) != 0 || result.Coverage.TargetsMapped != 2 {
		t.Fatalf("coverage = %#v, unclassified = %v", result.Coverage, result.UnclassifiedTargetIDs)
	}
}

func TestResolveFiltersUnknownRefsAndDeduplicatesSets(t *testing.T) {
	compilation := mustCompile(t, singleTargetInput())
	evidenceRef := evidenceRefForLabel(t, compilation, "Starts the API server")
	result := mustResolve(t, compilation, responseRole{
		Name: "Authorization service", Purpose: "Serves authorization decisions.",
		Prominence: ProminencePrimary, Kind: RoleKindService,
		Requiredness: RequirednessRequired, Confidence: ConfidenceHigh,
		MappingStatus: MappingMapped,
		Implementations: []responseImplementation{
			{TargetRef: "unknown-target"},
			{TargetRef: "t1", Mode: "serve"},
			{TargetRef: "t1", Mode: "serve"},
		},
		EvidenceRefs: []string{"unknown-evidence", evidenceRef, evidenceRef},
	})
	if got := result.Roles[0]; len(got.Implementations) != 1 || len(got.Evidence) != 1 ||
		got.Implementations[0].ProgramTargetID != "program-target-server" ||
		got.Implementations[0].Mode != "serve" {
		t.Fatalf("filtered role = %#v", got)
	}
}

func TestResolveRejectsUnresolvedOrIncompatibleMandatoryMappings(t *testing.T) {
	compilation := mustCompile(t, singleTargetInput())
	evidenceRef := evidenceRefForLabel(t, compilation, "Starts the API server")
	tests := map[string]responseRole{
		"mapped role loses every target": {
			Name: "Service", Purpose: "Serves requests.", Prominence: ProminencePrimary,
			Kind: RoleKindService, Requiredness: RequirednessRequired, Confidence: ConfidenceHigh,
			MappingStatus:   MappingMapped,
			Implementations: []responseImplementation{{TargetRef: "unknown-target"}},
			EvidenceRefs:    []string{evidenceRef},
		},
		"unknown mapping selects known target": {
			Name: "Unresolved role", Purpose: "May serve requests.", Prominence: ProminenceUnknown,
			Kind: RoleKindUnknown, Requiredness: RequirednessUnknown, Confidence: ConfidenceLow,
			MappingStatus:   MappingUnknown,
			Implementations: []responseImplementation{{TargetRef: "t1"}},
			EvidenceRefs:    []string{evidenceRef},
		},
		"role loses every evidence ref": {
			Name: "Service", Purpose: "Serves requests.", Prominence: ProminencePrimary,
			Kind: RoleKindService, Requiredness: RequirednessRequired, Confidence: ConfidenceHigh,
			MappingStatus:   MappingMapped,
			Implementations: []responseImplementation{{TargetRef: "t1"}},
			EvidenceRefs:    []string{"unknown-evidence"},
		},
	}
	for name, role := range tests {
		t.Run(name, func(t *testing.T) {
			raw := mustMarshalResponse(t, role)
			if _, err := ResolveResponse(compilation, raw); err == nil {
				t.Fatal("ResolveResponse accepted unresolved mandatory semantics")
			}
		})
	}
}

func TestResolveAllowsUnknownMappingAndEmptyRoles(t *testing.T) {
	input := twoTargetInput()
	compilation := mustCompile(t, input)
	evidenceRef := evidenceRefForLabel(t, compilation, "Repository runs one server and one worker")
	unknown := mustResolve(t, compilation, responseRole{
		Name: "Unresolved operator", Purpose: "The repository mentions an operator role.",
		Prominence: ProminenceUnknown, Kind: RoleKindUnknown,
		Requiredness: RequirednessUnknown, Confidence: ConfidenceLow,
		MappingStatus: MappingUnknown, Implementations: []responseImplementation{},
		EvidenceRefs: []string{evidenceRef},
	})
	if len(unknown.Roles) != 1 || unknown.Roles[0].Implementations == nil ||
		len(unknown.Roles[0].Implementations) != 0 ||
		!slices.Equal(unknown.UnclassifiedTargetIDs, []string{"program-target-server", "program-target-worker"}) {
		t.Fatalf("unknown mapping = %#v", unknown)
	}

	empty, err := ResolveResponse(compilation, []byte(`{"roles":[]}`))
	if err != nil {
		t.Fatalf("empty roles: %v", err)
	}
	if empty.Roles == nil || len(empty.Roles) != 0 ||
		!slices.Equal(empty.UnclassifiedTargetIDs, []string{"program-target-server", "program-target-worker"}) ||
		empty.Coverage.Roles != 0 || empty.Coverage.TargetsMapped != 0 ||
		empty.Coverage.TargetsUnclassified != 2 {
		t.Fatalf("empty result = %#v", empty)
	}
}

func TestLibraryOnlyTargetAllowsEmptyPortfolioAndRejectsLibraryRole(t *testing.T) {
	input := singleTargetInput()
	input.Targets[0].DisplayName = "client API"
	input.Targets[0].Kind = "module_library"
	input.Targets[0].Selector = "example.com/authorization/client::module_library"
	input.Targets[0].ActivityStarts = 0
	compilation := mustCompile(t, input)

	empty, err := ResolveResponse(compilation, []byte(`{"roles":[]}`))
	if err != nil {
		t.Fatalf("library-only empty portfolio: %v", err)
	}
	if len(empty.Roles) != 0 ||
		!slices.Equal(empty.UnclassifiedTargetIDs, []string{"program-target-server"}) ||
		empty.Coverage.TargetsMapped != 0 || empty.Coverage.TargetsUnclassified != 1 {
		t.Fatalf("library-only empty result = %#v", empty)
	}

	evidenceRef := evidenceRefForLabel(t, compilation, "Starts the API server")
	invalid := responseRole{
		Name: "Client library", Purpose: "Provides the client API.",
		Prominence: ProminencePrimary, Kind: RoleKind("library"),
		Requiredness: RequirednessRequired, Confidence: ConfidenceHigh,
		MappingStatus:   MappingMapped,
		Implementations: []responseImplementation{{TargetRef: "t1"}},
		EvidenceRefs:    []string{evidenceRef},
	}
	if _, err := ResolveResponse(compilation, mustMarshalResponse(t, invalid)); err == nil {
		t.Fatal("runtime portfolio accepted library as a runtime role")
	}
}

func TestArtifactEncodeDecodeAndAuthorityTamper(t *testing.T) {
	input := singleTargetInput()
	compilation := mustCompile(t, input)
	result := mustResolve(t, compilation, responseRole{
		Name: "Authorization service", Purpose: "Serves authorization decisions.",
		Prominence: ProminencePrimary, Kind: RoleKindService,
		Requiredness: RequirednessRequired, Confidence: ConfidenceHigh,
		MappingStatus:   MappingMapped,
		Implementations: []responseImplementation{{TargetRef: "t1"}},
		EvidenceRefs:    []string{evidenceRefForLabel(t, compilation, "Starts the API server")},
	})
	encoded, err := Encode(result)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil || !reflect.DeepEqual(decoded, result) {
		t.Fatalf("Decode = %#v, err = %v", decoded, err)
	}
	if digest, err := result.ArtifactSHA256(); err != nil || !validSHA256(digest) {
		t.Fatalf("ArtifactSHA256 = %q, %v", digest, err)
	}
	for name, suffix := range map[string][]byte{
		"second value":     []byte(`{"extra":true}`),
		"malformed tail":   []byte(`{`),
		"extra whitespace": []byte(` `),
	} {
		t.Run(name, func(t *testing.T) {
			tampered := append(append([]byte(nil), encoded...), suffix...)
			if _, err := Decode(tampered); err == nil {
				t.Fatal("Decode accepted trailing artifact data")
			}
		})
	}
	minified, err := result.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(minified); err == nil {
		t.Fatal("Decode accepted valid but noncanonical artifact bytes")
	}
	if _, err := Decode(make([]byte, MaxArtifactBytes+1)); err == nil {
		t.Fatal("Decode accepted an artifact outside the domain byte envelope")
	}

	tests := map[string]func(Result) Result{
		"target metadata": func(value Result) Result {
			value.Targets[0].DisplayName = "substituted server"
			return value
		},
		"portfolio identity": func(value Result) Result {
			value.TargetPagePortfolioSHA256 = strings.Repeat("c", 64)
			return value
		},
		"advertised evidence count": func(value Result) Result {
			value.Coverage.EvidenceAdvertised++
			return value
		},
		"invented evidence": func(value Result) Result {
			value.Roles[0].Evidence[0].Label = "Invented source claim"
			value.Roles[0].Evidence[0].ID, _ = evidenceID(value.Roles[0].Evidence[0])
			value.Roles[0].ID, _ = roleID(value.Roles[0])
			return value
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			tampered := mutate(cloneResult(result))
			if err := tampered.Validate(); err != nil {
				t.Fatalf("tamper should remain standalone-valid to exercise authority check: %v", err)
			}
			if err := tampered.ValidateAgainst(input); err == nil {
				t.Fatal("ValidateAgainst accepted substituted authority")
			}
		})
	}
}

func TestResolveRejectsLocalRestorationBeyondArtifactEnvelope(t *testing.T) {
	input := singleTargetInput()
	input.Targets[0].Evidence[0].Label = strings.Repeat("e", 500)
	input.Targets[0].Evidence[0].Location.Path = "evidence/" + strings.Repeat("a", 490)
	compilation := mustCompile(t, input)
	evidenceRef := evidenceRefForLabel(t, compilation, strings.Repeat("e", 500))
	roles := make([]responseRole, 0, 4000)
	for index := range 4000 {
		roles = append(roles, responseRole{
			Name: fmt.Sprintf("Runtime role %04d", index), Purpose: "Runs one evidenced mode.",
			Prominence: ProminenceSupporting, Kind: RoleKindSupportingTool,
			Requiredness: RequirednessUnknown, Confidence: ConfidenceLow,
			MappingStatus:   MappingMapped,
			Implementations: []responseImplementation{{TargetRef: "t1"}},
			EvidenceRefs:    []string{evidenceRef},
		})
	}
	raw := mustMarshalResponse(t, roles...)
	if len(raw) >= MaxResponseBytes {
		t.Fatalf("test response no longer exercises local expansion: %d bytes", len(raw))
	}
	if _, err := ResolveResponse(compilation, raw); err == nil || !strings.Contains(err.Error(), "artifact") {
		t.Fatalf("ResolveResponse accepted an unpersistable restored result: %v", err)
	}
}

func TestCompileCanonicalizesInputAndBindsCacheIdentity(t *testing.T) {
	input := twoTargetInput()
	input.RepositoryEvidence = append(input.RepositoryEvidence, EvidenceInput{
		Kind: EvidenceConfiguration, Label: "Worker mode is configured explicitly",
		Location: Location{Path: "config/defaults.yaml", Line: 7},
	})
	canonical := mustCompile(t, input)

	permuted := cloneInput(input)
	slices.Reverse(permuted.Targets)
	slices.Reverse(permuted.RepositoryEvidence)
	permuted.RepositoryEvidence = append(permuted.RepositoryEvidence, permuted.RepositoryEvidence[0])
	permuted.Targets[0].Responsibilities = append(
		permuted.Targets[0].Responsibilities,
		permuted.Targets[0].Responsibilities[0],
	)
	permuted.Targets[0].Evidence = append(permuted.Targets[0].Evidence, permuted.Targets[0].Evidence[0])
	reordered := mustCompile(t, permuted)
	if canonical.RequestSHA256 != reordered.RequestSHA256 ||
		!bytes.Equal(canonical.wire, reordered.wire) || !bytes.Equal(canonical.state, reordered.state) ||
		canonical.seal != reordered.seal {
		t.Fatalf("equivalent input changed identity:\ncanonical=%s\nreordered=%s", canonical.wire, reordered.wire)
	}

	revisionChanged := cloneInput(input)
	revisionChanged.CapturedRevision = strings.Repeat("d", 40)
	revision := mustCompile(t, revisionChanged)
	if revision.RequestSHA256 != canonical.RequestSHA256 || bytes.Equal(revision.state, canonical.state) ||
		revision.seal == canonical.seal {
		t.Fatal("captured revision should remain provider-local and invalidate cube cache state")
	}

	portfolioChanged := cloneInput(input)
	portfolioChanged.TargetPagePortfolioSHA256 = strings.Repeat("e", 64)
	portfolio := mustCompile(t, portfolioChanged)
	if portfolio.RequestSHA256 != canonical.RequestSHA256 || !bytes.Equal(portfolio.wire, canonical.wire) ||
		!bytes.Equal(portfolio.state, canonical.state) || portfolio.seal == canonical.seal {
		t.Fatal("publication-only portfolio identity changed semantic cache identity or lost its local compilation binding")
	}

	factsChanged := cloneInput(input)
	factsChanged.Targets[0].ProgramObjects++
	facts := mustCompile(t, factsChanged)
	if facts.RequestSHA256 == canonical.RequestSHA256 || bytes.Equal(facts.state, canonical.state) ||
		facts.seal == canonical.seal {
		t.Fatal("provider-visible fact change did not invalidate request and cache state")
	}

	evidenceChanged := cloneInput(input)
	evidenceChanged.RepositoryEvidence[0].Label = "Repository runs a server and an independently configured worker"
	evidence := mustCompile(t, evidenceChanged)
	if evidence.RequestSHA256 == canonical.RequestSHA256 || bytes.Equal(evidence.state, canonical.state) ||
		evidence.seal == canonical.seal {
		t.Fatal("advertised evidence change did not invalidate request and cache state")
	}
}

func TestExecutionIdentityChangesInvalidateSemanticState(t *testing.T) {
	compilation := mustCompile(t, singleTargetInput())
	current := currentExecutionIdentity()
	baseline, err := executionStateWithIdentity(compilation.input, compilation.wire, current)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]executionIdentity{
		"execution contract":       current,
		"prompt identity":          current,
		"preparation identity":     current,
		"response schema identity": current,
	}
	changed := tests["execution contract"]
	changed.Contract += "-changed"
	tests["execution contract"] = changed
	changed = tests["prompt identity"]
	changed.PromptVersion += "-changed"
	tests["prompt identity"] = changed
	changed = tests["preparation identity"]
	changed.PreparationVersion++
	tests["preparation identity"] = changed
	changed = tests["response schema identity"]
	changed.ResponseSchemaVersion++
	tests["response schema identity"] = changed
	for name, identity := range tests {
		t.Run(name, func(t *testing.T) {
			state, stateErr := executionStateWithIdentity(compilation.input, compilation.wire, identity)
			if stateErr != nil {
				t.Fatal(stateErr)
			}
			if bytes.Equal(state, baseline) {
				t.Fatal("identity change did not invalidate semantic state")
			}
		})
	}
}

func TestRunUsesSharedExecutorCacheWithCubeIdentity(t *testing.T) {
	input := singleTargetInput()
	compilation := mustCompile(t, input)
	provider := &runtimeTestProvider{response: mustMarshalResponse(t, responseRole{
		Name: "Authorization service", Purpose: "Serves authorization decisions.",
		Prominence: ProminencePrimary, Kind: RoleKindService,
		Requiredness: RequirednessRequired, Confidence: ConfidenceHigh,
		MappingStatus:   MappingMapped,
		Implementations: []responseImplementation{{TargetRef: "t1"}},
		EvidenceRefs:    []string{evidenceRefForLabel(t, compilation, "Starts the API server")},
	})}
	executor := llm.Executor{RootDir: t.TempDir(), Enabled: true}
	first, err := Run(t.Context(), executor, provider, input)
	if err != nil || first.Cached || provider.completeCalls != 1 {
		t.Fatalf("first = %#v, calls = %d, err = %v", first, provider.completeCalls, err)
	}
	warmInput := cloneInput(input)
	warmInput.TargetPagePortfolioSHA256 = strings.Repeat("c", 64)
	warm, err := Run(t.Context(), executor, provider, warmInput)
	if err != nil || !warm.Cached || warm.CacheKey != first.CacheKey || provider.completeCalls != 1 {
		t.Fatalf("warm = %#v, calls = %d, err = %v", warm, provider.completeCalls, err)
	}
	if !bytes.Equal(first.Request, warm.Request) || first.RequestSHA256 != warm.RequestSHA256 ||
		first.ResponseSHA256 != warm.ResponseSHA256 || !reflect.DeepEqual(first.Value.Roles, warm.Value.Roles) ||
		!reflect.DeepEqual(first.Value.Targets, warm.Value.Targets) || first.Value.Coverage != warm.Value.Coverage {
		t.Fatal("publication-only warm execution changed provider bytes or canonical semantic role payload")
	}
	if first.Value.TargetPagePortfolioSHA256 != input.TargetPagePortfolioSHA256 ||
		warm.Value.TargetPagePortfolioSHA256 != warmInput.TargetPagePortfolioSHA256 ||
		first.Value.ValidateAgainst(input) != nil || warm.Value.ValidateAgainst(warmInput) != nil ||
		warm.Value.ValidateAgainst(input) == nil || first.Value.ValidateAgainst(warmInput) == nil {
		t.Fatal("live or cached result lost its current publication binding")
	}
	changed := cloneInput(input)
	changed.CapturedRevision = strings.Repeat("d", 40)
	invalidated, err := Run(t.Context(), executor, provider, changed)
	if err != nil || invalidated.Cached || invalidated.CacheKey == first.CacheKey || provider.completeCalls != 2 {
		t.Fatalf("invalidated = %#v, calls = %d, err = %v", invalidated, provider.completeCalls, err)
	}
	changedFacts := cloneInput(input)
	changedFacts.Targets[0].ProgramObjects++
	factsInvalidated, err := Run(t.Context(), executor, provider, changedFacts)
	if err != nil || factsInvalidated.Cached || factsInvalidated.CacheKey == first.CacheKey || provider.completeCalls != 3 {
		t.Fatalf("facts invalidated = %#v, calls = %d, err = %v", factsInvalidated, provider.completeCalls, err)
	}
}

func TestCompileHasNoEntityCountLimit(t *testing.T) {
	const targetCount = 5000
	input := Input{
		RepositoryName: "large-runtime-repository", CapturedRevision: strings.Repeat("a", 40),
		TargetPagePortfolioSHA256: strings.Repeat("b", 64),
		Targets:                   make([]TargetInput, 0, targetCount), RepositoryEvidence: []EvidenceInput{},
	}
	for index := range targetCount {
		input.Targets = append(input.Targets, TargetInput{
			ProgramTargetID: fmt.Sprintf("program-target-%05d", index),
			DisplayName:     fmt.Sprintf("target-%05d", index), Language: "Go", Kind: "command",
			Selector: fmt.Sprintf("./cmd/target-%05d", index), Default: index == 0,
			Responsibilities: []ResponsibilityInput{}, Evidence: []EvidenceInput{},
		})
	}
	compilation := mustCompile(t, input)
	if compilation.Request.TargetCount != targetCount || len(compilation.Request.Targets) != targetCount ||
		len(compilation.wire) >= MaxRequestBytes {
		t.Fatalf("large compilation = targets %d/%d, bytes %d", compilation.Request.TargetCount, targetCount, len(compilation.wire))
	}
}

func TestCompileRequiresExactTargetEvidenceBinding(t *testing.T) {
	input := twoTargetInput()
	input.Targets[0].Evidence[0].ProgramTargetID = input.Targets[1].ProgramTargetID
	if _, err := Compile(input); err == nil {
		t.Fatal("Compile accepted target evidence bound to a different target")
	}
	input = twoTargetInput()
	input.Targets[0].Responsibilities[0].Evidence[0].ProgramTargetID = ""
	if _, err := Compile(input); err == nil {
		t.Fatal("Compile accepted responsibility evidence without exact target binding")
	}
}

func TestCompilationAndSecretEnvelopesFailClosed(t *testing.T) {
	compilation := mustCompile(t, singleTargetInput())
	tampered := compilation
	tampered.Request.Targets = append([]wireTarget(nil), compilation.Request.Targets...)
	tampered.Request.Targets[0].DisplayName = "substituted"
	if _, err := ResolveResponse(tampered, []byte(`{"roles":[]}`)); err == nil {
		t.Fatal("ResolveResponse accepted a tampered compilation")
	}
	publicationTampered := compilation
	publicationTampered.input.TargetPagePortfolioSHA256 = strings.Repeat("c", 64)
	if _, err := ResolveResponse(publicationTampered, []byte(`{"roles":[]}`)); err == nil {
		t.Fatal("ResolveResponse accepted a publication-substituted compilation with a stale local seal")
	}

	restore := secretscan.SetEnabled(true)
	defer restore()
	secret := "sk-ABCDEFGHIJKLMNOPQRSTUVWX"
	input := singleTargetInput()
	input.RepositoryEvidence[0].Label = "configured with " + secret
	if _, err := Compile(input); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("credential-shaped request error = %v", err)
	}
	role := responseRole{
		Name: "Authorization service", Purpose: "Configured with " + secret,
		Prominence: ProminencePrimary, Kind: RoleKindService,
		Requiredness: RequirednessRequired, Confidence: ConfidenceHigh,
		MappingStatus:   MappingMapped,
		Implementations: []responseImplementation{{TargetRef: "t1"}},
		EvidenceRefs:    []string{evidenceRefForLabel(t, compilation, "Starts the API server")},
	}
	if _, err := ResolveResponse(compilation, mustMarshalResponse(t, role)); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("credential-shaped response error = %v", err)
	}
}

type runtimeTestProvider struct {
	response      []byte
	completeCalls int
}

func (provider *runtimeTestProvider) State() []byte {
	return []byte(`{"endpoint":"https://provider.test/v1/chat","model":"runtime-test"}`)
}

func (provider *runtimeTestProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	raw, err := json.Marshal(struct {
		System string `json:"system"`
		User   string `json:"user"`
		JSON   bool   `json:"json"`
		Tokens int    `json:"max_tokens"`
	}{prompt.System, prompt.User, prompt.ResponseFormatJSON, limits.MaxOutputTokens})
	if err != nil {
		return llm.Prepared{}, err
	}
	return llm.NewPrepared(raw)
}

func (provider *runtimeTestProvider) Complete(_ context.Context, _ llm.Prepared) (llm.Completion, error) {
	provider.completeCalls++
	return llm.Completion{
		Response: append([]byte(nil), provider.response...), FinishReason: llm.FinishStop,
		ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1},
	}, nil
}

func singleTargetInput() Input {
	const targetID = "program-target-server"
	return Input{
		RepositoryName: "authorization-server", CapturedRevision: strings.Repeat("a", 40),
		TargetPagePortfolioSHA256: strings.Repeat("b", 64),
		Targets: []TargetInput{{
			ProgramTargetID: targetID, DisplayName: "server", Language: "Go", Kind: "command",
			Selector: "./cmd/server", Default: true, ProgramObjects: 41, ProgramRelations: 73,
			ActivityStarts: 3, IntegrationUses: 12,
			Responsibilities: []ResponsibilityInput{{
				Name: "Authorization decisions", Purpose: "Evaluates authorization requests.",
				Evidence: []EvidenceInput{{
					Kind: EvidenceResponsibility, Label: "Evaluates authorization requests",
					Location:        Location{Path: "internal/server/check.go", Line: 21, Column: 2},
					ProgramTargetID: targetID,
				}},
			}},
			Evidence: []EvidenceInput{{
				Kind: EvidenceTargetEntrypoint, Label: "Starts the API server",
				Location:        Location{Path: "cmd/server/main.go", Line: 17, Column: 1},
				ProgramTargetID: targetID,
			}},
		}},
		RepositoryEvidence: []EvidenceInput{{
			Kind: EvidenceRepositoryGuidance, Label: "Repository runs one authorization server",
			Location: Location{Path: "README.md", Line: 9},
		}},
	}
}

func twoTargetInput() Input {
	input := singleTargetInput()
	input.RepositoryEvidence[0].Label = "Repository runs one server and one worker"
	const targetID = "program-target-worker"
	input.Targets = append(input.Targets, TargetInput{
		ProgramTargetID: targetID, DisplayName: "worker", Language: "Go", Kind: "command",
		Selector: "./cmd/worker", ProgramObjects: 19, ProgramRelations: 31,
		ActivityStarts: 1, IntegrationUses: 4,
		Responsibilities: []ResponsibilityInput{{
			Name: "Tuple processing", Purpose: "Processes queued tuple updates.",
			Evidence: []EvidenceInput{{
				Kind: EvidenceResponsibility, Label: "Processes queued tuple updates",
				Location:        Location{Path: "internal/worker/tuples.go", Line: 12, Column: 1},
				ProgramTargetID: targetID,
			}},
		}},
		Evidence: []EvidenceInput{{
			Kind: EvidenceTargetEntrypoint, Label: "Starts the tuple worker",
			Location:        Location{Path: "cmd/worker/main.go", Line: 14, Column: 1},
			ProgramTargetID: targetID,
		}},
	})
	return input
}

func mustCompile(t *testing.T, input Input) Compilation {
	t.Helper()
	compilation, err := Compile(input)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return compilation
}

func evidenceRefForLabel(t *testing.T, compilation Compilation, label string) string {
	t.Helper()
	for _, evidence := range compilation.Request.EvidenceCatalog {
		if evidence.Label == label {
			return evidence.Ref
		}
	}
	t.Fatalf("evidence label %q is not advertised", label)
	return ""
}

func mustResolve(t *testing.T, compilation Compilation, roles ...responseRole) Result {
	t.Helper()
	result, err := ResolveResponse(compilation, mustMarshalResponse(t, roles...))
	if err != nil {
		t.Fatalf("ResolveResponse: %v", err)
	}
	return result
}

func mustMarshalResponse(t *testing.T, roles ...responseRole) []byte {
	t.Helper()
	raw, err := json.Marshal(response{Roles: roles})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func cloneInput(input Input) Input {
	result := input
	result.RepositoryEvidence = append([]EvidenceInput(nil), input.RepositoryEvidence...)
	result.Targets = append([]TargetInput(nil), input.Targets...)
	for targetIndex := range result.Targets {
		result.Targets[targetIndex].Evidence = append([]EvidenceInput(nil), input.Targets[targetIndex].Evidence...)
		result.Targets[targetIndex].Responsibilities = append(
			[]ResponsibilityInput(nil), input.Targets[targetIndex].Responsibilities...,
		)
		for responsibilityIndex := range result.Targets[targetIndex].Responsibilities {
			result.Targets[targetIndex].Responsibilities[responsibilityIndex].Evidence = append(
				[]EvidenceInput(nil),
				input.Targets[targetIndex].Responsibilities[responsibilityIndex].Evidence...,
			)
		}
	}
	return result
}

func cloneResult(result Result) Result {
	cloned := result
	cloned.Targets = append([]Target(nil), result.Targets...)
	cloned.UnclassifiedTargetIDs = append([]string{}, result.UnclassifiedTargetIDs...)
	cloned.Roles = append([]Role{}, result.Roles...)
	for roleIndex := range cloned.Roles {
		cloned.Roles[roleIndex].Implementations = append(
			[]Implementation(nil), result.Roles[roleIndex].Implementations...,
		)
		cloned.Roles[roleIndex].Evidence = append([]Evidence(nil), result.Roles[roleIndex].Evidence...)
	}
	return cloned
}
