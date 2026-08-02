package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/artifactrole"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/sourcewindowfacts"
	"github.com/dvordrova/repomap/internal/studymap"
)

func TestStudyPromptsKeepReadingLabelsCanonicalUnderLocalization(t *testing.T) {
	t.Parallel()

	const want = "reading_anchors.label is a closed schema value"
	for name, prompt := range map[string]string{
		"legacy":    studyMapUserPrompt,
		"split-v32": studyMapDirectionTask,
	} {
		if !strings.Contains(prompt, want) ||
			!strings.Contains(prompt, "the report localizes it later") {
			t.Fatalf("%s Study prompt does not protect reading labels", name)
		}
	}
}

func TestStudyDirectionPromptExampleMatchesCompleteAnchorContract(t *testing.T) {
	t.Parallel()

	for _, suffix := range []string{"A", "B", "C"} {
		placeholder := "exact supplied code anchor ref " + suffix
		if strings.Count(studyMapDirectionTask, placeholder) != 2 {
			t.Fatalf("Study prompt placeholder %q must appear once in anchor_ids and once in reading_anchors", placeholder)
		}
	}
	if got := strings.Count(studyMapDirectionTask, `{"anchor_ref":`); got != 3 {
		t.Fatalf("Study prompt reading-anchor examples = %d, want 3", got)
	}
}

func TestStudyDirectionRequestIdentityBindsCatalogOrderAndContracts(t *testing.T) {
	t.Parallel()

	bundle, _ := studyMapV32ReviewFixture(t)
	firstCatalog, firstPrompt, err := buildStudyMapDirectionStage(bundle)
	if err != nil {
		t.Fatal(err)
	}
	firstRequest, err := json.Marshal(firstPrompt)
	if err != nil {
		t.Fatal(err)
	}
	reordered := bundle
	reordered.Anchors = append([]studymap.Anchor(nil), bundle.Anchors...)
	reordered.Anchors[0], reordered.Anchors[1] = reordered.Anchors[1], reordered.Anchors[0]
	secondCatalog, secondPrompt, err := buildStudyMapDirectionStage(reordered)
	if err != nil {
		t.Fatal(err)
	}
	secondRequest, err := json.Marshal(secondPrompt)
	if err != nil {
		t.Fatal(err)
	}
	firstHash := sha256.Sum256(firstRequest)
	secondHash := sha256.Sum256(secondRequest)
	if bytes.Equal(firstRequest, secondRequest) || firstHash == secondHash ||
		firstCatalog.Digest() == secondCatalog.Digest() ||
		firstCatalog.CatalogRef() == secondCatalog.CatalogRef() {
		t.Fatal("exact catalog order did not drift the candidate request/content identity")
	}
	var identity struct {
		CatalogContract   string `json:"catalog_contract"`
		ResponseContract  string `json:"response_contract"`
		ValidatorContract string `json:"validator_contract"`
		PromptContract    string `json:"prompt_contract"`
	}
	if err := json.Unmarshal(firstCatalog.IdentityJSON(), &identity); err != nil {
		t.Fatal(err)
	}
	if identity.CatalogContract != studymap.DirectionReferenceCatalogVersion ||
		identity.ResponseContract != studymap.DirectionReferenceResponseVersion ||
		identity.ValidatorContract != studymap.DirectionReferenceValidatorVersion ||
		identity.PromptContract != semanticdiscovery.StudyCandidatesPromptVersion ||
		firstPrompt.Version != semanticdiscovery.StudyCandidatesPromptVersion {
		t.Fatalf("candidate request contract identity = %#v, prompt %q", identity, firstPrompt.Version)
	}
}

type studyMapV32ReviewProviderStub struct {
	mu         sync.Mutex
	failPlanID string
	plans      []string
	calls      []string
}

type studyMapV32ResourceLimitProvider struct {
	calls int
}

type studyMapV32OversizeReviewProvider struct {
	calls    int
	response []byte
}

type studyMapV32DirectionResourceProvider struct {
	delegate *studyMapV32TypedRoundTripProvider
	calls    int
}

type studyMapMixedReviewResourceProvider struct {
	mu                sync.Mutex
	resourceDirection string
	calls             int
	stores            int
}

func (stub *studyMapV32ResourceLimitProvider) SemanticDiscoveryPromptJSON(
	prompt semanticdiscovery.Prompt,
) ([]byte, error) {
	return json.Marshal(prompt)
}

func (stub *studyMapV32ResourceLimitProvider) DiscoverSemanticsMeasured(
	_ context.Context,
	_ semanticdiscovery.Prompt,
) (modelresearch.ProviderResult, error) {
	stub.calls++
	return modelresearch.ProviderResult{
			Content: []byte(`{"version":1`), Attempts: 1,
		}, &deepseek.ResourceLimitError{
			Stage: "semantic_discovery", Kind: deepseek.ResourceLimitOutputTokens,
			Limit: 128, Observed: 128, ObservedKnown: true,
			FinishReason: "length",
		}
}

func (stub *studyMapV32OversizeReviewProvider) SemanticDiscoveryPromptJSON(
	prompt semanticdiscovery.Prompt,
) ([]byte, error) {
	return json.Marshal(prompt)
}

func (stub *studyMapV32OversizeReviewProvider) DiscoverSemanticsMeasured(
	_ context.Context,
	_ semanticdiscovery.Prompt,
) (modelresearch.ProviderResult, error) {
	stub.calls++
	return modelresearch.ProviderResult{
		Content: stub.response, Attempts: 1, ResponseBytes: len(stub.response),
	}, nil
}

func (stub *studyMapV32DirectionResourceProvider) SemanticDiscoveryPromptJSON(
	prompt semanticdiscovery.Prompt,
) ([]byte, error) {
	return stub.delegate.SemanticDiscoveryPromptJSON(prompt)
}

func (stub *studyMapV32DirectionResourceProvider) DiscoverSemanticsMeasured(
	ctx context.Context,
	prompt semanticdiscovery.Prompt,
) (modelresearch.ProviderResult, error) {
	stub.calls++
	if prompt.Version == semanticdiscovery.StudyCandidatesPromptVersion {
		return modelresearch.ProviderResult{
				Content: []byte(`{"version":1`), Attempts: 1,
			}, &deepseek.ResourceLimitError{
				Stage: "semantic_discovery", Kind: deepseek.ResourceLimitOutputTokens,
				Limit: 128, Observed: 128, ObservedKnown: true,
				FinishReason: "length",
			}
	}
	return stub.delegate.DiscoverSemanticsMeasured(ctx, prompt)
}

func (stub *studyMapMixedReviewResourceProvider) SemanticDiscoveryPromptJSON(
	prompt semanticdiscovery.Prompt,
) ([]byte, error) {
	return json.Marshal(prompt)
}

func (stub *studyMapMixedReviewResourceProvider) DiscoverSemanticsMeasured(
	_ context.Context,
	prompt semanticdiscovery.Prompt,
) (modelresearch.ProviderResult, error) {
	stub.mu.Lock()
	stub.calls++
	stub.mu.Unlock()
	if strings.Contains(prompt.User, stub.resourceDirection) {
		return modelresearch.ProviderResult{Attempts: 1}, &deepseek.ResourceLimitError{
			Stage: "reading_pack_review", Kind: deepseek.ResourceLimitOutputTokens,
			Limit: 128, Observed: 128, ObservedKnown: true, FinishReason: "length",
		}
	}
	const marker = "Fixed bounded review bundle JSON:\n"
	markerIndex := strings.LastIndex(prompt.User, marker)
	if markerIndex < 0 {
		return modelresearch.ProviderResult{}, errors.New("fixture review bundle is absent")
	}
	reviewBundle, err := studymap.DecodeReviewBundle([]byte(prompt.User[markerIndex+len(marker):]))
	if err != nil {
		return modelresearch.ProviderResult{}, err
	}
	proposal := studymap.ReviewProposal{
		Version: studymap.ReviewProposalVersion, DirectionID: reviewBundle.DirectionID,
	}
	roles := []studymap.ReadingRole{
		studymap.ReadingRolePublicOrCLIEntry,
		studymap.ReadingRoleCoreOrchestration,
		studymap.ReadingRoleEffectOrIntegrationBoundary,
	}
	for index, anchor := range reviewBundle.Anchors {
		proposal.Reviews = append(proposal.Reviews, studymap.AnchorReview{
			AnchorID: anchor.AnchorID, Fit: studymap.AnchorFitDirect,
			SupportedObservation: "This fragment defines the selected function.",
			Role:                 roles[index%len(roles)],
			OverclaimReasons:     []studymap.OverclaimReason{studymap.OverclaimNone},
		})
	}
	raw, err := json.Marshal(proposal)
	return modelresearch.ProviderResult{Content: raw, Attempts: 1}, err
}

func (stub *studyMapMixedReviewResourceProvider) loadStudyReview(
	semanticdiscovery.Prompt,
	[]byte,
	studymap.Bundle,
	studymap.DirectionCandidate,
) (modelresearch.ProviderResult, bool, error) {
	return modelresearch.ProviderResult{}, false, nil
}

func (stub *studyMapMixedReviewResourceProvider) storeStudyReview(
	context.Context,
	semanticdiscovery.Prompt,
	[]byte,
	studymap.Bundle,
	studymap.DirectionCandidate,
	[]byte,
) {
	stub.mu.Lock()
	stub.stores++
	stub.mu.Unlock()
}

func TestPrepareStudyMapV32ResourceLimitSuppressesLaterStages(t *testing.T) {
	t.Parallel()

	bundle, _ := studyMapV32ReviewFixture(t)
	provider := &studyMapV32ResourceLimitProvider{}
	runDir := t.TempDir()
	_, _, stages, err := prepareStudyMapV32(
		context.Background(), runDir, bundle, provider,
	)
	var limitErr *deepseek.ResourceLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("prepareStudyMapV32() error = %v, want ResourceLimitError", err)
	}
	if provider.calls != 1 || len(stages) != 1 || stages[0].Status != "failed_provider" {
		t.Fatalf("provider calls/stages = %d/%#v, want terminal first-stage failure", provider.calls, stages)
	}
	for _, name := range []string{
		studymap.RecordFile,
		studymap.AttemptFile,
		studymap.StatusFile,
		studyMapBriefShapeFile,
		studyMapBriefShapeAttempt,
		studyMapDirectionsFile,
		studyMapDirectionsAttempt,
		studyMapReviewsFile,
	} {
		if _, statErr := os.Stat(filepath.Join(runDir, name)); !os.IsNotExist(statErr) {
			t.Fatalf("terminal resource artifact %s exists or stat failed: %v", name, statErr)
		}
	}
	if _, statErr := os.Stat(filepath.Join(runDir, studyMapReviewAttemptsDir)); !os.IsNotExist(statErr) {
		t.Fatalf("terminal resource review attempts exist or stat failed: %v", statErr)
	}
}

func TestPrepareStudyMapV32ResourceLimitRetainsOnlySafeExchange(t *testing.T) {
	t.Parallel()

	bundle, _ := studyMapV32ReviewFixture(t)
	provider := &studyMapV32ResourceLimitProvider{}
	runDir := t.TempDir()
	writer := openSemanticJournalTestWriter(t, runDir)
	_, _, _, err := prepareStudyMapV32WithOptions(
		context.Background(),
		runDir,
		bundle,
		provider,
		studyMapRunOptions{exchangeWriter: writer},
	)
	var limitErr *deepseek.ResourceLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("prepareStudyMapV32WithOptions() error = %v, want ResourceLimitError", err)
	}
	entries := readSemanticJournalEntries(t, runDir)
	if len(entries) != 1 || entries[0].record.Stage != debugdump.SemanticStageStudyBrief ||
		entries[0].record.State != debugdump.SemanticStateProviderFailed ||
		entries[0].record.SemanticCalls != 1 ||
		entries[0].record.Response.Storage != "raw_unavailable" ||
		entries[0].record.Response.UnavailableCode != debugdump.SemanticUnavailableNoContent {
		t.Fatalf("resource-limited Study exchange = %#v", entries)
	}
	if bytes.Contains(entries[0].response, []byte(`{"version":1`)) {
		t.Fatalf("resource-limited Study exchange persisted unowned provider content: %q", entries[0].response)
	}
	if _, statErr := os.Stat(filepath.Join(runDir, studyMapBriefShapeAttempt)); !os.IsNotExist(statErr) {
		t.Fatalf("resource-limited Study retained one-off attempt: %v", statErr)
	}
}

func TestPrepareStudyMapV32DecodeResourceLimitSuppressesArtifacts(t *testing.T) {
	t.Parallel()

	bundle, _ := studyMapV32ReviewFixture(t)
	provider := &studyMapV32OversizeReviewProvider{
		response: bytes.Repeat([]byte("x"), modelresearch.ProviderResponseByteLimit+1),
	}
	runDir := t.TempDir()
	_, _, stages, err := prepareStudyMapV32(
		context.Background(), runDir, bundle, provider,
	)
	var limitErr *deepseek.ResourceLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("prepareStudyMapV32() error = %v, want ResourceLimitError", err)
	}
	if provider.calls != 1 || len(stages) != 1 || stages[0].Status != "failed_provider" {
		t.Fatalf("decode resource calls/stages = %d/%#v", provider.calls, stages)
	}
	for _, name := range []string{
		studyMapBriefShapeFile,
		studyMapBriefShapeAttempt,
		studyMapDirectionsFile,
		studyMapDirectionsAttempt,
		studyMapReviewsFile,
	} {
		if _, statErr := os.Stat(filepath.Join(runDir, name)); !os.IsNotExist(statErr) {
			t.Fatalf("decode resource artifact %s exists or stat failed: %v", name, statErr)
		}
	}
}

func TestPrepareStudyMapV32DirectionResourceLimitRemovesAcceptedBrief(t *testing.T) {
	t.Parallel()

	bundle, directions := studyMapV32ReviewFixture(t)
	provider := &studyMapV32DirectionResourceProvider{
		delegate: &studyMapV32TypedRoundTripProvider{
			t: t, bundle: bundle, directions: directions,
		},
	}
	runDir := t.TempDir()
	_, _, stages, err := prepareStudyMapV32(
		context.Background(), runDir, bundle, provider,
	)
	var limitErr *deepseek.ResourceLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("prepareStudyMapV32() error = %v, want ResourceLimitError", err)
	}
	if provider.calls != 2 || len(stages) != 2 || stages[0].Status != "accepted" ||
		stages[1].Status != "failed_provider" {
		t.Fatalf("provider calls/stages = %d/%#v, want accepted Brief then terminal directions", provider.calls, stages)
	}
	for _, name := range []string{
		studyMapBriefShapeFile,
		studyMapBriefShapeAttempt,
		studyMapDirectionsFile,
		studyMapDirectionsAttempt,
		studyMapReviewsFile,
	} {
		if _, statErr := os.Stat(filepath.Join(runDir, name)); !os.IsNotExist(statErr) {
			t.Fatalf("terminal direction resource artifact %s exists or stat failed: %v", name, statErr)
		}
	}
}

func TestPrepareStudyMapResourceLimitDoesNotPublishTopLevelState(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	runDir := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "service.go"), `package fixture

func Start(value string) string { return Middle(value) }
func Middle(value string) string { return Finish(value) }
func Finish(value string) string { return value + "!" }
`)
	writeFile(t, filepath.Join(runDir, "snapshot.json"), `{
  "repo_name":"go-fixture",
  "go_facts":{
    "modules":[{"id":"module-fixture","module_path":"example.com/fixture","module_dir":".","display_name":"."}],
    "packages":[{
      "canonical_package_path":"example.com/fixture",
      "name":"fixture",
      "owning_module_id":"module-fixture",
      "module_path":"example.com/fixture",
      "package_directory":".",
      "module_relative_path":".",
      "display_path":".",
      "locality":"local",
      "files":["service.go"]
    }]
  }
}`)
	writeFile(t, filepath.Join(runDir, "llm_bundle.json"), `{"allowed_paths":["service.go"]}`)
	provider := &studyMapV32ResourceLimitProvider{}
	_, err := prepareStudyMapWithProviderFactory(
		context.Background(),
		runDir,
		repoRoot,
		func() (semanticDiscoveryEditor, error) { return provider, nil },
	)
	var limitErr *deepseek.ResourceLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("prepareStudyMapWithProviderFactory() error = %v, want ResourceLimitError", err)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want one terminal call", provider.calls)
	}
	if _, statErr := os.Stat(filepath.Join(runDir, studymap.BundleFile)); statErr != nil {
		t.Fatalf("deterministic Study input bundle was not retained: %v", statErr)
	}
	for _, name := range []string{
		studymap.RecordFile,
		studymap.AttemptFile,
		studymap.StatusFile,
		studyMapBriefShapeFile,
		studyMapBriefShapeAttempt,
		studyMapDirectionsFile,
		studyMapDirectionsAttempt,
		studyMapReviewsFile,
	} {
		if _, statErr := os.Stat(filepath.Join(runDir, name)); !os.IsNotExist(statErr) {
			t.Fatalf("terminal top-level resource artifact %s exists or stat failed: %v", name, statErr)
		}
	}
}

func TestReviewStudyMapDirectionsDecodeResourceLimitIsTerminal(t *testing.T) {
	bundle, directions := studyMapV32ReviewFixture(t)
	directions.Directions = append(
		[]studymap.DirectionCandidate(nil),
		directions.Directions[:1]...,
	)
	provider := &studyMapV32OversizeReviewProvider{
		response: bytes.Repeat(
			[]byte("x"),
			modelresearch.ProviderResponseByteLimit+1,
		),
	}
	runDir := t.TempDir()
	reviews, summaries, stages, issues, err := reviewStudyMapDirections(
		context.Background(),
		runDir,
		bundle,
		directions,
		"fixture-bundle-sha",
		provider,
	)
	var limitErr *deepseek.ResourceLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("reviewStudyMapDirections() error = %v, want ResourceLimitError", err)
	}
	if limitErr.Kind != deepseek.ResourceLimitResponseBytes ||
		limitErr.Limit != modelresearch.ProviderResponseByteLimit ||
		provider.calls != 1 {
		t.Fatalf("resource error/provider calls = %#v/%d", limitErr, provider.calls)
	}
	if len(reviews) != 0 || len(summaries) != 0 || len(stages) != 0 || len(issues) != 0 {
		t.Fatalf(
			"terminal results = reviews:%d summaries:%d stages:%d issues:%d",
			len(reviews), len(summaries), len(stages), len(issues),
		)
	}
	if _, statErr := os.Stat(filepath.Join(runDir, studyMapReviewsFile)); !os.IsNotExist(statErr) {
		t.Fatalf("terminal resource limit published reviews: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(runDir, studyMapReviewAttemptsDir)); !os.IsNotExist(statErr) {
		t.Fatalf("terminal resource limit published review attempts: %v", statErr)
	}
}

func TestReviewStudyMapDirectionsResourceLimitDoesNotCacheValidSibling(t *testing.T) {
	t.Parallel()

	bundle, directions := studyMapV32ReviewFixture(t)
	directions.Directions = append(
		[]studymap.DirectionCandidate(nil),
		directions.Directions[:2]...,
	)
	provider := &studyMapMixedReviewResourceProvider{
		resourceDirection: directions.Directions[1].DirectionID,
	}
	runDir := t.TempDir()
	_, _, _, _, err := reviewStudyMapDirections(
		context.Background(), runDir, bundle, directions, "fixture-bundle-sha", provider,
	)
	var limitErr *deepseek.ResourceLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("reviewStudyMapDirections() error = %v, want ResourceLimitError", err)
	}
	provider.mu.Lock()
	calls, stores := provider.calls, provider.stores
	provider.mu.Unlock()
	if calls != 2 || stores != 0 {
		t.Fatalf("resource-limited review calls/cache stores = %d/%d, want 2/0", calls, stores)
	}
	if _, statErr := os.Stat(filepath.Join(runDir, studyMapReviewAttemptsDir)); !os.IsNotExist(statErr) {
		t.Fatalf("resource-limited mixed review left attempt directory: %v", statErr)
	}
}

func (stub *studyMapV32ReviewProviderStub) SemanticDiscoveryPromptJSON(
	prompt semanticdiscovery.Prompt,
) ([]byte, error) {
	stub.mu.Lock()
	stub.plans = append(stub.plans, prompt.Version)
	stub.mu.Unlock()
	if stub.failPlanID != "" && strings.Contains(prompt.User, stub.failPlanID) {
		return nil, errors.New("fixture request planning failure")
	}
	return json.Marshal(prompt)
}

func (stub *studyMapV32ReviewProviderStub) DiscoverSemanticsMeasured(
	_ context.Context,
	prompt semanticdiscovery.Prompt,
) (modelresearch.ProviderResult, error) {
	const marker = "Fixed bounded review bundle JSON:\n"
	markerIndex := strings.LastIndex(prompt.User, marker)
	if markerIndex < 0 {
		return modelresearch.ProviderResult{}, errors.New("fixture review bundle is absent")
	}
	bundle, err := studymap.DecodeReviewBundle([]byte(prompt.User[markerIndex+len(marker):]))
	if err != nil {
		return modelresearch.ProviderResult{}, err
	}
	roles := []studymap.ReadingRole{
		studymap.ReadingRolePublicOrCLIEntry,
		studymap.ReadingRoleCoreOrchestration,
		studymap.ReadingRoleEffectOrIntegrationBoundary,
	}
	proposal := studymap.ReviewProposal{
		Version: studymap.ReviewProposalVersion, DirectionID: bundle.DirectionID,
	}
	for index, anchor := range bundle.Anchors {
		proposal.Reviews = append(proposal.Reviews, studymap.AnchorReview{
			AnchorID:             anchor.AnchorID,
			Fit:                  studymap.AnchorFitDirect,
			SupportedObservation: "This fragment defines the selected function.",
			Role:                 roles[index%len(roles)],
			OverclaimReasons:     []studymap.OverclaimReason{studymap.OverclaimNone},
		})
	}
	raw, err := json.Marshal(proposal)
	stub.mu.Lock()
	stub.calls = append(stub.calls, bundle.DirectionID)
	stub.mu.Unlock()
	return modelresearch.ProviderResult{
		Content: raw, Attempts: 1, InputTokens: 20, OutputTokens: 10,
	}, err
}

func TestPrepareStudyMapV32SkipsImpossibleCompletePackBeforeProviderCall(t *testing.T) {
	t.Parallel()

	bundle, _ := studyMapV32ReviewFixture(t)
	bundle.Anchors = bundle.Anchors[:2]
	bundle.AllowedPaths = bundle.AllowedPaths[:2]
	provider := &studyMapV32ReviewProviderStub{}

	_, _, stages, err := prepareStudyMapV32(
		context.Background(),
		t.TempDir(),
		bundle,
		provider,
	)
	if err == nil || !strings.Contains(err.Error(), "insufficient code anchors") {
		t.Fatalf("sparse Study preflight error = %v", err)
	}
	provider.mu.Lock()
	plans := len(provider.plans)
	calls := len(provider.calls)
	provider.mu.Unlock()
	if plans != 0 || calls != 0 || len(stages) != 0 {
		t.Fatalf("sparse Study invoked provider: plans=%d calls=%d stages=%#v", plans, calls, stages)
	}
}

func TestStudyDirectionExampleShapeRetainsTwelveValidCandidates(t *testing.T) {
	t.Parallel()

	_, proposal := studyMapV32ReviewFixture(t)
	for index := range proposal.Directions {
		proposal.Directions[index].DirectionID = ""
	}
	decoded, diagnostics, err := studymap.DecodeDirectionProposalWithDiagnostics(
		mustJSON(t, proposal),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Directions) != studymap.MaxCandidates ||
		diagnostics.Accepted != studymap.MaxCandidates ||
		diagnostics.Rejected != 0 {
		t.Fatalf("complete example-shaped directions = %d, diagnostics %#v", len(decoded.Directions), diagnostics)
	}
}

type studyMapV32TypedRoundTripProvider struct {
	t          *testing.T
	bundle     studymap.Bundle
	directions studymap.DirectionProposal
	briefMode  string

	mu              sync.Mutex
	briefSystem     string
	briefPrompt     string
	directionSystem string
	directionPrompt string
	reviewPrompts   []string
}

func (stub *studyMapV32TypedRoundTripProvider) SemanticDiscoveryPromptJSON(
	prompt semanticdiscovery.Prompt,
) ([]byte, error) {
	return json.Marshal(prompt)
}

func (stub *studyMapV32TypedRoundTripProvider) DiscoverSemanticsMeasured(
	_ context.Context,
	prompt semanticdiscovery.Prompt,
) (modelresearch.ProviderResult, error) {
	stub.mu.Lock()
	switch prompt.Version {
	case semanticdiscovery.StudyBriefPromptVersion:
		stub.briefSystem = prompt.System
		stub.briefPrompt = prompt.User
	case semanticdiscovery.StudyCandidatesPromptVersion:
		stub.directionSystem = prompt.System
		stub.directionPrompt = prompt.User
	case semanticdiscovery.ReadingPackReviewPromptVersion:
		stub.reviewPrompts = append(stub.reviewPrompts, prompt.User)
	}
	stub.mu.Unlock()

	var raw []byte
	var err error
	switch prompt.Version {
	case semanticdiscovery.StudyBriefPromptVersion:
		raw, err = studyMapTypedBriefShapeResponse(
			stub.t, prompt.User, stub.bundle, stub.briefMode,
		)
	case semanticdiscovery.StudyCandidatesPromptVersion:
		raw, err = studyMapTypedDirectionResponse(stub.t, prompt.User, stub.bundle, stub.directions)
	case semanticdiscovery.ReadingPackReviewPromptVersion:
		const marker = "Fixed bounded review bundle JSON:\n"
		markerIndex := strings.LastIndex(prompt.User, marker)
		if markerIndex < 0 {
			return modelresearch.ProviderResult{}, errors.New("fixture review bundle is absent")
		}
		reviewBundle, decodeErr := studymap.DecodeReviewBundle(
			[]byte(prompt.User[markerIndex+len(marker):]),
		)
		if decodeErr != nil {
			return modelresearch.ProviderResult{}, decodeErr
		}
		proposal := studymap.ReviewProposal{
			Version: studymap.ReviewProposalVersion, DirectionID: reviewBundle.DirectionID,
		}
		roles := []studymap.ReadingRole{
			studymap.ReadingRolePublicOrCLIEntry,
			studymap.ReadingRoleCoreOrchestration,
			studymap.ReadingRoleEffectOrIntegrationBoundary,
		}
		for index, anchor := range reviewBundle.Anchors {
			proposal.Reviews = append(proposal.Reviews, studymap.AnchorReview{
				AnchorID: anchor.AnchorID, Fit: studymap.AnchorFitDirect,
				SupportedObservation: "This fragment defines the selected function.",
				Role:                 roles[index%len(roles)],
				OverclaimReasons:     []studymap.OverclaimReason{studymap.OverclaimNone},
			})
		}
		raw, err = json.Marshal(proposal)
	default:
		return modelresearch.ProviderResult{}, fmt.Errorf("unexpected prompt version %q", prompt.Version)
	}
	return modelresearch.ProviderResult{Content: raw, Attempts: 1}, err
}

func TestPrepareStudyMapV32UsesTypedDirectionSeamAndKeepsCanonicalPublication(t *testing.T) {
	t.Parallel()

	bundle, directions := studyMapV32ReviewFixture(t)
	provider := &studyMapV32TypedRoundTripProvider{
		t: t, bundle: bundle, directions: directions,
	}
	runDir := t.TempDir()
	record, reduction, stages, err := prepareStudyMapV32(
		context.Background(), runDir, bundle, provider,
	)
	if err != nil {
		t.Fatal(err)
	}
	if reduction.Proposed != len(directions.Directions) ||
		reduction.Reviewed != len(directions.Directions) ||
		len(record.Directions) == 0 || reduction.Selected != len(record.Directions) ||
		len(stages) != 2+len(directions.Directions) {
		t.Fatalf(
			"typed Study proposed/reviewed/published/stages = %d/%d/%d/%d",
			reduction.Proposed, reduction.Reviewed, len(record.Directions), len(stages),
		)
	}
	savedDirections, err := os.ReadFile(filepath.Join(runDir, studyMapDirectionsFile))
	if err != nil {
		t.Fatal(err)
	}
	replayedDirections, err := studymap.DecodeNormalizedDirectionProposal(savedDirections)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(replayedDirections, directions) {
		t.Fatal("typed provider round-trip changed canonical candidates, IDs, or order")
	}
	provider.mu.Lock()
	briefSystem := provider.briefSystem
	briefPrompt := provider.briefPrompt
	directionSystem := provider.directionSystem
	directionPrompt := provider.directionPrompt
	reviewPrompts := append([]string(nil), provider.reviewPrompts...)
	provider.mu.Unlock()
	if briefSystem != studyMapV32SystemPrompt ||
		!strings.Contains(briefSystem, "request-local typed references") ||
		!strings.Contains(briefPrompt, `"catalog_ref":"b_`) ||
		!strings.Contains(briefPrompt, `"anchor_ref":"a1"`) {
		t.Fatal("BriefShape provider request did not use exact typed references")
	}
	for _, anchor := range bundle.Anchors {
		if strings.Contains(briefPrompt, anchor.ID) {
			t.Fatalf("BriefShape provider prompt leaked canonical anchor id %q", anchor.ID)
		}
	}
	if directionSystem != studyMapDirectionSystemPrompt ||
		!strings.Contains(directionSystem, "request-local typed references") {
		t.Fatal("direction provider request did not use its candidate-only typed-reference system prompt")
	}
	for _, anchor := range bundle.Anchors {
		if strings.Contains(directionPrompt, anchor.ID) {
			t.Fatalf("direction provider prompt leaked canonical anchor id %q", anchor.ID)
		}
	}
	if !strings.Contains(directionPrompt, `"catalog_ref":"c_`) ||
		!strings.Contains(directionPrompt, `"anchor_ref":"a1"`) {
		t.Fatal("direction provider prompt omitted compact typed reference contract")
	}
	foundCanonicalReview := false
	for _, prompt := range reviewPrompts {
		if strings.Contains(prompt, directions.Directions[0].DirectionID) &&
			strings.Contains(prompt, bundle.Anchors[0].ID) {
			foundCanonicalReview = true
			break
		}
	}
	if len(reviewPrompts) != len(directions.Directions) || !foundCanonicalReview {
		t.Fatal("per-direction review splitting or canonical review contract changed")
	}
	if err := record.Validate(); err != nil {
		t.Fatalf("typed direction publication did not produce a valid canonical record: %v", err)
	}
}

func TestPrepareStudyMapV32OmitsDocumentFromShapeAndPublishesSixAreas(t *testing.T) {
	t.Parallel()

	bundle, directions := studyMapV32ReviewFixture(t)
	for index := 1; index < 6; index++ {
		bundle.Areas = append(bundle.Areas, studymap.Area{
			ID:             fmt.Sprintf("area-extra-%d", index),
			Name:           fmt.Sprintf("Extra %d", index),
			Responsibility: "An additional exact repository area.",
		})
	}
	bundle.Documents = []studymap.Document{{
		ID: "doc-b335630551682c19", Path: "README.md", Label: "README",
		Excerpt: "Repository documentation.",
	}}
	bundle.AllowedPaths = append(bundle.AllowedPaths, "README.md")
	provider := &studyMapV32TypedRoundTripProvider{
		t: t, bundle: bundle, directions: directions,
		briefMode: "document_in_shape",
	}
	runDir := t.TempDir()
	record, reduction, stages, err := prepareStudyMapV32(
		context.Background(), runDir, bundle, provider,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.ShapeAreaIDs) != 6 || len(record.Directions) == 0 ||
		reduction.Reviewed != len(directions.Directions) ||
		len(stages) != 2+len(directions.Directions) {
		t.Fatalf(
			"Casdoor-shape areas/directions/reviewed/stages = %d/%d/%d/%d",
			len(record.ShapeAreaIDs), len(record.Directions), reduction.Reviewed, len(stages),
		)
	}
	var attempt studyMapV32StageAttempt
	if err := readV32ReplayJSON(
		filepath.Join(runDir, studyMapBriefShapeAttempt), &attempt,
	); err != nil {
		t.Fatal(err)
	}
	if attempt.ValidationState != "accepted" || attempt.BriefDiagnostics == nil ||
		attempt.BriefDiagnostics.ShapeAccepted != 6 ||
		attempt.BriefDiagnostics.ShapeRejected != 1 ||
		len(attempt.BriefDiagnostics.Issues) != 1 ||
		attempt.BriefDiagnostics.Issues[0].Code != "wrong_kind_area_ref" {
		t.Fatalf("Casdoor-shape attempt = %#v", attempt)
	}
	for _, areaID := range record.ShapeAreaIDs {
		if areaID == bundle.Documents[0].ID {
			t.Fatal("document identity entered canonical Shape")
		}
	}
}

func TestPrepareStudyMapV32PublishesDirectionsWithExplicitEmptyShape(t *testing.T) {
	t.Parallel()

	bundle, directions := studyMapV32ReviewFixture(t)
	bundle.Documents = []studymap.Document{{
		ID: "doc-shape-invalid", Path: "README.md", Label: "README",
		Excerpt: "Repository documentation.",
	}}
	bundle.AllowedPaths = append(bundle.AllowedPaths, "README.md")
	provider := &studyMapV32TypedRoundTripProvider{
		t: t, bundle: bundle, directions: directions,
		briefMode: "all_invalid_shape",
	}
	runDir := t.TempDir()
	record, reduction, stages, err := prepareStudyMapV32(
		context.Background(), runDir, bundle, provider,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.ShapeAreaIDs) != 0 || len(record.Directions) == 0 ||
		reduction.Reviewed != len(directions.Directions) ||
		len(stages) != 2+len(directions.Directions) {
		t.Fatalf(
			"empty-shape areas/directions/reviewed/stages = %d/%d/%d/%d",
			len(record.ShapeAreaIDs), len(record.Directions), reduction.Reviewed, len(stages),
		)
	}
	if err := record.Validate(); err != nil {
		t.Fatalf("empty Shape blocked the reviewed canonical record: %v", err)
	}
	var attempt studyMapV32StageAttempt
	if err := readV32ReplayJSON(
		filepath.Join(runDir, studyMapBriefShapeAttempt), &attempt,
	); err != nil {
		t.Fatal(err)
	}
	if attempt.ValidationState != "accepted" || attempt.BriefDiagnostics == nil ||
		attempt.BriefDiagnostics.ShapeAccepted != 0 ||
		attempt.BriefDiagnostics.ShapeRejected != 1 {
		t.Fatalf("empty-shape attempt = %#v", attempt)
	}
}

func studyMapTypedBriefShapeResponse(
	t *testing.T,
	user string,
	bundle studymap.Bundle,
	mode string,
) ([]byte, error) {
	t.Helper()
	input := strings.TrimPrefix(user, studyMapV32SharedInput)
	marker := strings.Index(input, "\n\nTask:")
	if marker < 0 {
		return nil, errors.New("typed Brief/Shape fixture bundle marker is absent")
	}
	var wire struct {
		CatalogRef string `json:"catalog_ref"`
		Areas      []struct {
			AreaRef string `json:"area_ref"`
		} `json:"areas"`
		Anchors []struct {
			AnchorRef string `json:"anchor_ref"`
		} `json:"code_anchors"`
		Documents []struct {
			DocumentRef string `json:"document_ref"`
		} `json:"documents"`
	}
	if err := json.Unmarshal([]byte(input[:marker]), &wire); err != nil {
		return nil, err
	}
	if len(wire.Anchors) == 0 {
		return nil, errors.New("typed Brief/Shape fixture has no anchor ref")
	}
	statement := func(text string) map[string]any {
		return map[string]any{"text": text, "support_refs": []string{wire.Anchors[0].AnchorRef}}
	}
	shape := make([]string, 0, len(wire.Areas)+1)
	for _, area := range wire.Areas {
		shape = append(shape, area.AreaRef)
	}
	switch mode {
	case "":
	case "document_in_shape":
		if len(wire.Documents) == 0 {
			return nil, errors.New("typed Brief/Shape fixture has no document ref")
		}
		shape = append(shape, wire.Documents[0].DocumentRef)
	case "all_invalid_shape":
		if len(wire.Documents) == 0 {
			return nil, errors.New("typed Brief/Shape fixture has no document ref")
		}
		shape = []string{wire.Documents[0].DocumentRef}
	default:
		return nil, fmt.Errorf("unknown typed Brief/Shape fixture mode %q", mode)
	}
	return json.Marshal(map[string]any{
		"version": 1, "catalog_ref": wire.CatalogRef,
		"repository_type": studymap.RepositoryLibrary,
		"brief": map[string]any{
			"what_it_is":             statement("This is a bounded source fixture."),
			"problem":                statement("It demonstrates exact Study editing."),
			"main_input":             statement("A developer starts from source."),
			"central_responsibility": statement("The source defines bounded work."),
			"observable_result":      statement("The source exposes a result."),
		},
		"shape_area_refs": shape,
	})
}

func studyMapTypedDirectionResponse(
	t *testing.T,
	user string,
	bundle studymap.Bundle,
	directions studymap.DirectionProposal,
) ([]byte, error) {
	t.Helper()
	input := strings.TrimPrefix(user, studyMapV32SharedInput)
	marker := strings.Index(input, "\n\nTask:")
	if marker < 0 {
		return nil, errors.New("typed direction fixture bundle marker is absent")
	}
	var wire struct {
		CatalogRef string `json:"catalog_ref"`
		Areas      []struct {
			AreaRef string `json:"area_ref"`
		} `json:"areas"`
		Anchors []struct {
			AnchorRef string `json:"anchor_ref"`
		} `json:"code_anchors"`
		Documents []struct {
			DocumentRef string `json:"document_ref"`
		} `json:"documents"`
		Mechanisms []struct {
			MechanismRef string `json:"mechanism_ref"`
		} `json:"canonical_mechanisms"`
	}
	if err := json.Unmarshal([]byte(input[:marker]), &wire); err != nil {
		return nil, err
	}
	anchors := make(map[string]string, len(bundle.Anchors))
	for index, anchor := range bundle.Anchors {
		anchors[anchor.ID] = wire.Anchors[index].AnchorRef
	}
	areas := make(map[string]string, len(bundle.Areas))
	for index, area := range bundle.Areas {
		areas[area.ID] = wire.Areas[index].AreaRef
	}
	documents := make(map[string]string, len(bundle.Documents))
	for index, document := range bundle.Documents {
		documents[document.ID] = wire.Documents[index].DocumentRef
	}
	mechanisms := make(map[string]string, len(bundle.Mechanisms))
	for index, mechanism := range bundle.Mechanisms {
		mechanisms[mechanism.ID] = wire.Mechanisms[index].MechanismRef
	}
	response := map[string]any{
		"version": 1, "catalog_ref": wire.CatalogRef,
		"directions": []any{},
	}
	items := make([]any, 0, len(directions.Directions))
	for _, direction := range directions.Directions {
		anchorRefs := make([]string, 0, len(direction.AnchorIDs))
		for _, id := range direction.AnchorIDs {
			anchorRefs = append(anchorRefs, anchors[id])
		}
		documentRefs := make([]string, 0, len(direction.DocumentIDs))
		for _, id := range direction.DocumentIDs {
			documentRefs = append(documentRefs, documents[id])
		}
		areaRefs := make([]string, 0, len(direction.AreaIDs))
		for _, id := range direction.AreaIDs {
			areaRefs = append(areaRefs, areas[id])
		}
		reading := make([]any, 0, len(direction.ReadingAnchors))
		for _, item := range direction.ReadingAnchors {
			reading = append(reading, map[string]any{
				"anchor_ref": anchors[item.AnchorID], "label": item.Label,
				"what_to_look_for": item.WhatToLookFor,
			})
		}
		items = append(items, map[string]any{
			"question": direction.Question, "why_it_matters": direction.WhyItMatters,
			"learning_outcome": direction.LearningOutcome,
			"target_user_job":  direction.TargetJob, "learning_stage": direction.LearningStage,
			"anchor_refs": anchorRefs, "document_refs": documentRefs, "area_refs": areaRefs,
			"mechanism_ref": mechanisms[direction.MechanismID], "reading_anchors": reading,
			"search_queries": direction.SearchQueries,
		})
	}
	response["directions"] = items
	return json.Marshal(response)
}

func TestReviewStudyMapDirectionsReviewsEveryCandidateAndRecordsPreparationFailures(
	t *testing.T,
) {
	t.Parallel()

	bundle, directions := studyMapV32ReviewFixture(t)
	directions.Directions[0].AnchorIDs[2] = "fact-unknown"
	directions.Directions[0].ReadingAnchors[2].AnchorID = "fact-unknown"
	for index := range directions.Directions {
		directions.Directions[index].DirectionID = ""
	}
	var err error
	directions, err = studymap.NormalizeDirectionProposal(directions)
	if err != nil {
		t.Fatal(err)
	}
	provider := &studyMapV32ReviewProviderStub{
		failPlanID: directions.Directions[1].DirectionID,
	}
	runDir := t.TempDir()
	reviews, summaries, stages, issues, err := reviewStudyMapDirections(
		context.Background(),
		runDir,
		bundle,
		directions,
		"fixture-bundle-sha",
		provider,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != studymap.MaxCandidates || len(stages) != studymap.MaxCandidates {
		t.Fatalf("reviewed attempts/stages = %d/%d, want %d", len(summaries), len(stages), studymap.MaxCandidates)
	}
	if len(reviews) != studymap.MaxCandidates-2 {
		t.Fatalf("accepted reviews = %d, want %d", len(reviews), studymap.MaxCandidates-2)
	}
	provider.mu.Lock()
	providerCalls := len(provider.calls)
	provider.mu.Unlock()
	if providerCalls != studymap.MaxCandidates-2 {
		t.Fatalf("provider calls = %d, want %d", providerCalls, studymap.MaxCandidates-2)
	}
	if !hasStudyMapReviewIssue(issues, directions.Directions[0].DirectionID, "review_bundle_build_failed") {
		t.Fatalf("bundle failure issues = %#v", issues)
	}
	if !hasStudyMapReviewIssue(issues, directions.Directions[1].DirectionID, "review_request_plan_failed") {
		t.Fatalf("plan failure issues = %#v", issues)
	}
	for _, summary := range summaries[:2] {
		if summary.ValidationState != "rejected" || summary.IssueCode == "" || summary.Metrics.ProviderCall {
			t.Fatalf("local rejection summary = %#v", summary)
		}
	}
	attemptFiles, err := filepath.Glob(filepath.Join(runDir, studyMapReviewAttemptsDir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(attemptFiles) != studymap.MaxCandidates {
		t.Fatalf("saved attempt files = %d, want %d", len(attemptFiles), studymap.MaxCandidates)
	}
}

func TestReviewStudyMapDirectionsDoesNotReduceTwelveSingleFileAnchors(t *testing.T) {
	t.Parallel()

	bundle, directions := studyMapV32SingleFileReviewFixture(t)
	provider := &studyMapV32ReviewProviderStub{}
	reviews, summaries, stages, issues, err := reviewStudyMapDirections(
		context.Background(),
		t.TempDir(),
		bundle,
		directions,
		"single-file-bundle-sha",
		provider,
	)
	if err != nil {
		t.Fatal(err)
	}
	provider.mu.Lock()
	calls := append([]string(nil), provider.calls...)
	provider.mu.Unlock()
	if len(calls) != studymap.MaxCandidates ||
		len(reviews) != studymap.MaxCandidates ||
		len(summaries) != studymap.MaxCandidates ||
		len(stages) != studymap.MaxCandidates ||
		len(issues) != 0 {
		t.Fatalf(
			"single-file calls/reviews/summaries/stages/issues = %d/%d/%d/%d/%d, want %d/%d/%d/%d/0",
			len(calls), len(reviews), len(summaries), len(stages), len(issues),
			studymap.MaxCandidates, studymap.MaxCandidates,
			studymap.MaxCandidates, studymap.MaxCandidates,
		)
	}
	for index, summary := range summaries {
		if summary.DirectionID != directions.Directions[index].DirectionID {
			t.Fatalf(
				"summary[%d].direction_id = %q, want %q",
				index, summary.DirectionID, directions.Directions[index].DirectionID,
			)
		}
	}
}

func TestNormalizedDirectionArtifactRoundTripsWithoutRewritingRawAttempt(t *testing.T) {
	t.Parallel()

	_, normalized := studyMapV32ReviewFixture(t)
	rawProposal := normalized
	rawProposal.Directions = append([]studymap.DirectionCandidate(nil), normalized.Directions...)
	for index := range rawProposal.Directions {
		rawProposal.Directions[index].DirectionID = ""
	}
	rejectedPosition := len(rawProposal.Directions) - 1
	rawProposal.Directions[rejectedPosition].AnchorIDs =
		rawProposal.Directions[rejectedPosition].AnchorIDs[:2]
	rawProposal.Directions[rejectedPosition].ReadingAnchors =
		rawProposal.Directions[rejectedPosition].ReadingAnchors[:2]
	raw, err := json.Marshal(rawProposal)
	if err != nil {
		t.Fatal(err)
	}
	decoded, diagnostics, err := studymap.DecodeDirectionProposalWithDiagnostics(raw)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostics.Received != len(rawProposal.Directions) ||
		diagnostics.Accepted != len(rawProposal.Directions)-1 ||
		diagnostics.Rejected != 1 ||
		len(diagnostics.Issues) != 1 ||
		diagnostics.Issues[0].Position != rejectedPosition ||
		diagnostics.Issues[0].Code != "invalid_anchor_count" {
		t.Fatalf("direction diagnostics = %#v", diagnostics)
	}
	runDir := t.TempDir()
	if err := writeNormalizedDirectionProposal(filepath.Join(runDir, studyMapDirectionsFile), decoded); err != nil {
		t.Fatal(err)
	}
	attempt := studyMapV32StageAttempt{
		Version: 1, PromptVersion: semanticdiscovery.StudyCandidatesPromptVersion,
		ValidationState:      "accepted",
		DirectionDiagnostics: &diagnostics,
		Response:             append(json.RawMessage(nil), raw...),
	}
	if err := writeGoldenJSON(filepath.Join(runDir, studyMapDirectionsAttempt), attempt); err != nil {
		t.Fatal(err)
	}
	savedDirections, err := os.ReadFile(filepath.Join(runDir, studyMapDirectionsFile))
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := studymap.DecodeNormalizedDirectionProposal(savedDirections)
	if err != nil {
		t.Fatalf("decode saved normalized artifact: %v", err)
	}
	if replayed.Directions[0].DirectionID == "" {
		t.Fatal("saved normalized artifact omitted local direction ID")
	}
	savedAttemptRaw, err := os.ReadFile(filepath.Join(runDir, studyMapDirectionsAttempt))
	if err != nil {
		t.Fatal(err)
	}
	var savedAttempt studyMapV32StageAttempt
	if err := json.Unmarshal(savedAttemptRaw, &savedAttempt); err != nil {
		t.Fatal(err)
	}
	var compactSaved bytes.Buffer
	if err := json.Compact(&compactSaved, savedAttempt.Response); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(compactSaved.Bytes(), raw) {
		t.Fatal("saved attempt response was replaced by the normalized projection")
	}
	var replayedRaw studymap.DirectionProposal
	if err := json.Unmarshal(savedAttempt.Response, &replayedRaw); err != nil {
		t.Fatal(err)
	}
	if replayedRaw.Directions[0].DirectionID != "" {
		t.Fatal("raw attempt gained a locally derived direction ID")
	}
	if savedAttempt.DirectionDiagnostics == nil ||
		!reflect.DeepEqual(*savedAttempt.DirectionDiagnostics, diagnostics) {
		t.Fatalf(
			"saved direction diagnostics = %#v, want %#v",
			savedAttempt.DirectionDiagnostics,
			diagnostics,
		)
	}
}

func TestClearStudyMapV32OutputsLeavesUnrelatedRunArtifacts(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	stale := []string{
		studymap.RecordFile,
		studymap.BundleFile,
		studymap.AttemptFile,
		studymap.StatusFile,
		studyMapBriefShapeFile,
		studyMapBriefShapeAttempt,
		studyMapDirectionsFile,
		studyMapDirectionsAttempt,
		studyMapReviewsFile,
	}
	for _, name := range stale {
		if err := os.WriteFile(filepath.Join(runDir, name), []byte("stale"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	attemptDir := filepath.Join(runDir, studyMapReviewAttemptsDir)
	if err := os.MkdirAll(attemptDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(attemptDir, "stale.json"), []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(runDir, "report.json")
	if err := os.WriteFile(unrelated, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := clearStudyMapV32Outputs(runDir); err != nil {
		t.Fatal(err)
	}
	for _, name := range stale {
		if _, err := os.Stat(filepath.Join(runDir, name)); !os.IsNotExist(err) {
			t.Fatalf("stale output %q remains: %v", name, err)
		}
	}
	if _, err := os.Stat(attemptDir); !os.IsNotExist(err) {
		t.Fatalf("stale attempt directory remains: %v", err)
	}
	if data, err := os.ReadFile(unrelated); err != nil || string(data) != "keep" {
		t.Fatalf("unrelated run artifact = %q, %v", data, err)
	}
}

func TestAggregateStudyMapMetricsReflectsStagesAndOutcome(t *testing.T) {
	t.Parallel()

	accepted := semanticDiscoveryStageMetrics{
		Status: "accepted", ProviderCall: true, LatencyMillis: 30, InputTokens: 10,
	}
	rejected := semanticDiscoveryStageMetrics{
		Status: "rejected", ProviderCall: false, LatencyMillis: 7, InputTokens: 2,
	}
	tests := []struct {
		name       string
		stages     []semanticDiscoveryStageMetrics
		outcomeErr error
		wantStatus string
	}{
		{name: "all accepted", stages: []semanticDiscoveryStageMetrics{accepted, accepted}, wantStatus: "accepted"},
		{name: "accepted calls but rejected outcome", stages: []semanticDiscoveryStageMetrics{accepted, accepted}, outcomeErr: errors.New("reducer rejected"), wantStatus: "rejected"},
		{name: "partial stage results", stages: []semanticDiscoveryStageMetrics{accepted, rejected}, wantStatus: "partial"},
		{name: "local outcome rejected", stages: []semanticDiscoveryStageMetrics{accepted, rejected}, outcomeErr: errors.New("reducer rejected"), wantStatus: "rejected"},
		{name: "not run", wantStatus: "not_run"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			metrics := aggregateStudyMapMetrics(test.stages, test.outcomeErr)
			if metrics.Status != test.wantStatus {
				t.Fatalf("aggregate status = %q, want %q", metrics.Status, test.wantStatus)
			}
			wantLatency := int64(0)
			for _, stage := range test.stages {
				wantLatency += stage.LatencyMillis
			}
			if metrics.LatencyMillis != wantLatency {
				t.Fatalf("summed provider latency = %d, want %d", metrics.LatencyMillis, wantLatency)
			}
		})
	}
}

func studyMapV32ReviewFixture(t *testing.T) (studymap.Bundle, studymap.DirectionProposal) {
	t.Helper()

	area := studymap.Area{ID: "area-core", Name: "Core", Responsibility: "Central production code."}
	anchorSpecs := []struct {
		id     string
		path   string
		symbol string
		role   artifactrole.Role
	}{
		{id: "fact-entry", path: "entry.go", symbol: "enter", role: artifactrole.RolePublicAPI},
		{id: "fact-core", path: "core.go", symbol: "process", role: artifactrole.RoleProductionCore},
		{id: "fact-effect", path: "effect.go", symbol: "emit", role: artifactrole.RoleEffectBoundary},
	}
	bundle := studymap.Bundle{
		Version: studymap.BundleVersion, RepoName: "fixture", Areas: []studymap.Area{area},
	}
	for index, spec := range anchorSpecs {
		window, err := sourcewindowfacts.NewWindow(
			"window-"+spec.symbol,
			spec.path,
			10+index*10,
			[]string{
				"func " + spec.symbol + "() int {",
				"\tvalue := 1",
				"\treturn value",
				"}",
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		function, err := sourcewindowfacts.ExtractGoFunction(window, spec.symbol)
		if err != nil {
			t.Fatal(err)
		}
		bundle.Anchors = append(bundle.Anchors, studymap.Anchor{
			ID: spec.id, Path: spec.path, Symbol: spec.symbol, Line: function.StartLine + 1,
			Role: spec.role, Statement: spec.symbol + " is a bounded source anchor.",
			AreaIDs: []string{area.ID}, Function: function,
		})
		bundle.AllowedPaths = append(bundle.AllowedPaths, spec.path)
	}
	questions := []string{
		"How does alpha processing work?",
		"How does bravo processing work?",
		"How does charlie processing work?",
		"How does delta processing work?",
		"How does echo processing work?",
		"How does foxtrot processing work?",
		"How does golf processing work?",
		"How does hotel processing work?",
		"How does india processing work?",
		"How does juliet processing work?",
		"How does kilo processing work?",
		"How does lima processing work?",
	}
	directions := studymap.DirectionProposal{Version: studymap.DirectionProposalVersion}
	anchorIDs := []string{"fact-entry", "fact-core", "fact-effect"}
	for _, question := range questions {
		directions.Directions = append(directions.Directions, studymap.DirectionCandidate{
			Question: question, WhyItMatters: "This locates a central repository responsibility.",
			LearningOutcome: "The reader can identify the relevant production code.",
			TargetJob:       studymap.JobFirstContact, LearningStage: studymap.StageCentralOperation,
			AnchorIDs: append([]string(nil), anchorIDs...), AreaIDs: []string{area.ID},
			ReadingAnchors: []studymap.ReadingAnchor{
				{AnchorID: anchorIDs[0], Label: "Start here", WhatToLookFor: "Inspect the public entry declaration."},
				{AnchorID: anchorIDs[1], Label: "Then inspect", WhatToLookFor: "Inspect the central bounded implementation."},
				{AnchorID: anchorIDs[2], Label: "Related implementation", WhatToLookFor: "Inspect the visible output boundary."},
			},
			SearchQueries: []string{fmt.Sprintf("%s source", strings.TrimSuffix(question, "?"))},
		})
	}
	normalized, err := studymap.NormalizeDirectionProposal(directions)
	if err != nil {
		t.Fatal(err)
	}
	return bundle, normalized
}

func studyMapV32SingleFileReviewFixture(t *testing.T) (studymap.Bundle, studymap.DirectionProposal) {
	t.Helper()

	const filePath = "single.go"
	area := studymap.Area{
		ID: "area-single", Name: "Single file", Responsibility: "Bounded single-file fixture.",
	}
	bundle := studymap.Bundle{
		Version: studymap.BundleVersion, RepoName: "single-file-fixture",
		Areas: []studymap.Area{area}, AllowedPaths: []string{filePath},
	}
	anchorIDs := make([]string, 0, studymap.MaxCandidates)
	for index := 0; index < studymap.MaxCandidates; index++ {
		symbol := fmt.Sprintf("part%d", index)
		window, err := sourcewindowfacts.NewWindow(
			"window-"+symbol,
			filePath,
			1+index*5,
			[]string{
				"func " + symbol + "() int {",
				"\treturn 1",
				"}",
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		function, err := sourcewindowfacts.ExtractGoFunction(window, symbol)
		if err != nil {
			t.Fatal(err)
		}
		anchorID := "fact-" + symbol
		anchorIDs = append(anchorIDs, anchorID)
		bundle.Anchors = append(bundle.Anchors, studymap.Anchor{
			ID: anchorID, Path: filePath, Symbol: symbol, Line: function.StartLine,
			Role:      artifactrole.RoleProductionCore,
			Statement: symbol + " is an exact bounded function.",
			AreaIDs:   []string{area.ID}, Function: function,
		})
	}
	directions := studymap.DirectionProposal{Version: studymap.DirectionProposalVersion}
	for index := 0; index < studymap.MaxCandidates; index++ {
		selected := []string{
			anchorIDs[index],
			anchorIDs[(index+1)%len(anchorIDs)],
			anchorIDs[(index+2)%len(anchorIDs)],
		}
		directions.Directions = append(directions.Directions, studymap.DirectionCandidate{
			Question:        fmt.Sprintf("How does single-file direction %d work?", index+1),
			WhyItMatters:    "This retains a distinct bounded reading direction.",
			LearningOutcome: "The reader can locate the selected declarations.",
			TargetJob:       studymap.JobFirstContact,
			LearningStage:   studymap.StageCentralOperation,
			AnchorIDs:       selected,
			AreaIDs:         []string{area.ID},
			ReadingAnchors: []studymap.ReadingAnchor{
				{AnchorID: selected[0], Label: "Start here", WhatToLookFor: "Inspect the first declaration."},
				{AnchorID: selected[1], Label: "Then inspect", WhatToLookFor: "Inspect the second declaration."},
				{AnchorID: selected[2], Label: "Related implementation", WhatToLookFor: "Inspect the third declaration."},
			},
		})
	}
	normalized, err := studymap.NormalizeDirectionProposal(directions)
	if err != nil {
		t.Fatal(err)
	}
	return bundle, normalized
}

func hasStudyMapReviewIssue(issues []studymap.ReviewIssue, directionID, code string) bool {
	for _, issue := range issues {
		if issue.DirectionID == directionID && issue.Code == code {
			return true
		}
	}
	return false
}
