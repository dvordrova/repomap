package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
	"github.com/dvordrova/repomap/internal/secretscan"
)

func TestRunAtlasStudyProductAcceptsBriefAndValidSiblingRoute(t *testing.T) {
	product := atlasStudyRuntimeProduct(t, atlasStudyRuntimeInput())
	response := atlasStudyRuntimeResponse(t, product, true)
	client := newAtlasStudyRuntimeClient(response, nil)
	runsDir, runDir := atlasStudyRuntimeDirectories(t, "accepted")

	outcome, err := runAtlasStudyProductForRun(
		t.Context(), runDir, runsDir, atlasStudyRuntimeRepository(),
		modelresearch.DefaultPolicy(), false, true, product, nil,
		atlasStudyRuntimeFactory(client, client),
	)
	if err != nil {
		t.Fatalf("runAtlasStudyProductForRun: %v", err)
	}
	if outcome.State != atlasstudy.ProductStateAcceptedPartial || outcome.Cached ||
		outcome.DirectionCount != 1 || outcome.RejectedDirections != 1 ||
		outcome.TransportAttempts != 1 || client.calls != 1 {
		t.Fatalf("accepted outcome/client = %#v / %#v", outcome, client)
	}
	result := atlasStudyRuntimeReadResult(t, runDir)
	if len(result.Directions) != 1 || result.Diagnostics.DirectionsReceived != 2 ||
		result.Diagnostics.DirectionsAccepted != 1 || result.Diagnostics.DirectionsRejected != 1 ||
		len(result.Diagnostics.Issues) != 1 ||
		result.Diagnostics.Issues[0].Code != atlasstudy.IssueUnknownRef {
		t.Fatalf("sibling route diagnostics/result = %#v", result)
	}
	status := atlasStudyRuntimeReadStatus(t, runDir)
	if status.State != atlasstudy.ProductStateAcceptedPartial || status.DirectionCount != 1 {
		t.Fatalf("accepted status = %#v", status)
	}
	exchange := atlasStudyRuntimeReadExchange(t, runDir)
	if exchange.Stage != debugdump.SemanticStageAtlasStudy ||
		exchange.State != debugdump.SemanticStateAccepted ||
		exchange.SemanticCalls != 1 || exchange.TransportAttempts != 1 {
		t.Fatalf("accepted exchange = %#v", exchange)
	}
}

func TestRunAtlasStudyProductReplaysOnlyAcceptedExactCache(t *testing.T) {
	product := atlasStudyRuntimeProduct(t, atlasStudyRuntimeInput())
	response := atlasStudyRuntimeResponse(t, product, false)
	first := newAtlasStudyRuntimeClient(response, nil)
	runsDir, firstRun := atlasStudyRuntimeDirectories(t, "first")
	if _, err := runAtlasStudyProductForRun(
		t.Context(), firstRun, runsDir, atlasStudyRuntimeRepository(),
		modelresearch.DefaultPolicy(), false, true, product, nil,
		atlasStudyRuntimeFactory(first, first),
	); err != nil {
		t.Fatalf("populate exact cache: %v", err)
	}

	secondRun := filepath.Join(runsDir, "second")
	if err := os.Mkdir(secondRun, 0o700); err != nil {
		t.Fatal(err)
	}
	second := newAtlasStudyRuntimeClient(nil, errors.New("cache hit must not call provider"))
	outcome, err := runAtlasStudyProductForRun(
		t.Context(), secondRun, runsDir, atlasStudyRuntimeRepository(),
		modelresearch.DefaultPolicy(), false, true, product, nil,
		atlasStudyRuntimeFactory(second, second),
	)
	if err != nil {
		t.Fatalf("replay exact cache: %v", err)
	}
	if !outcome.Cached || outcome.DirectionCount != 1 || second.calls != 0 ||
		outcome.TransportAttempts != 0 {
		t.Fatalf("cached outcome/client = %#v / %#v", outcome, second)
	}
	exchange := atlasStudyRuntimeReadExchange(t, secondRun)
	if exchange.State != debugdump.SemanticStateCacheHit ||
		exchange.SemanticCalls != 0 || exchange.TransportAttempts != 0 {
		t.Fatalf("cache exchange = %#v", exchange)
	}
}

func TestRunAtlasStudyProductPreservesAcceptedWithNotSelectedSpansAcrossLiveAndCache(t *testing.T) {
	product := atlasStudyRuntimeProduct(t, atlasStudyRuntimePartialInput())
	response := atlasStudyRuntimeResponse(t, product, false)
	live := newAtlasStudyRuntimeClient(response, nil)
	runsDir, liveRun := atlasStudyRuntimeDirectories(t, "partial-live")

	liveOutcome, err := runAtlasStudyProductForRun(
		t.Context(), liveRun, runsDir, atlasStudyRuntimeRepository(),
		modelresearch.DefaultPolicy(), false, true, product, nil,
		atlasStudyRuntimeFactory(live, live),
	)
	if err != nil {
		t.Fatalf("run partial live Atlas Study: %v", err)
	}
	// Two advertised spans, one returned valid direction: the other advertised
	// span is normal not_selected under D211 and never turns the exact result
	// into accepted_partial.
	if liveOutcome.State != atlasstudy.ProductStateAccepted || liveOutcome.Cached ||
		liveOutcome.DirectionCount != 1 || live.calls != 1 {
		t.Fatalf("accepted live outcome/client = %#v / %#v", liveOutcome, live)
	}
	if result := atlasStudyRuntimeReadResult(t, liveRun); result.State != atlasstudy.ProductStateAccepted ||
		result.SpanCoverage.ConsideredSpanCount != 2 || result.SpanCoverage.AdvertisedSpanCount != 2 ||
		result.SpanCoverage.ModelSelectedSpanCount != 1 || result.SpanCoverage.AcceptedSpanCount != 1 ||
		!result.SpanCoverage.FrontierComplete || !result.SpanCoverage.SelectedItemsComplete ||
		!result.SpanCoverage.SupportCoverageComplete || result.SpanCoverage.PortfolioTargetMet {
		t.Fatalf("accepted live result = %#v", result)
	}
	if status := atlasStudyRuntimeReadStatus(t, liveRun); status.State != atlasstudy.ProductStateAccepted ||
		status.ConsideredSpanCount != 2 || status.AdvertisedSpanCount != 2 ||
		status.ModelSelectedSpanCount != 1 || status.AcceptedSpanCount != 1 ||
		!status.FrontierComplete || !status.SelectedItemsComplete ||
		!status.SupportCoverageComplete || status.PortfolioTargetMet {
		t.Fatalf("accepted live status = %#v", status)
	}

	cacheRun := filepath.Join(runsDir, "partial-cache")
	if err := os.Mkdir(cacheRun, 0o700); err != nil {
		t.Fatal(err)
	}
	cacheOnly := newAtlasStudyRuntimeClient(nil, errors.New("partial cache hit must not call provider"))
	cacheOutcome, err := runAtlasStudyProductForRun(
		t.Context(), cacheRun, runsDir, atlasStudyRuntimeRepository(),
		modelresearch.DefaultPolicy(), false, true, product, nil,
		atlasStudyRuntimeFactory(cacheOnly, cacheOnly),
	)
	if err != nil {
		t.Fatalf("replay partial Atlas Study cache: %v", err)
	}
	if cacheOutcome.State != atlasstudy.ProductStateAccepted || !cacheOutcome.Cached ||
		cacheOutcome.DirectionCount != 1 || cacheOnly.calls != 0 {
		t.Fatalf("accepted cache outcome/client = %#v / %#v", cacheOutcome, cacheOnly)
	}
	if status := atlasStudyRuntimeReadStatus(t, cacheRun); status.State != atlasstudy.ProductStateAccepted ||
		status.ConsideredSpanCount != 2 || status.AdvertisedSpanCount != 2 ||
		status.ModelSelectedSpanCount != 1 || status.AcceptedSpanCount != 1 ||
		!status.FrontierComplete || !status.SelectedItemsComplete ||
		!status.SupportCoverageComplete || status.PortfolioTargetMet {
		t.Fatalf("accepted cache status = %#v", status)
	}
}

func TestAtlasStudyCacheV7RejectsV6ContractAndDifferentCatalog(t *testing.T) {
	input := atlasStudyRuntimeInput()
	product := atlasStudyRuntimeProduct(t, input)
	client := newAtlasStudyRuntimeClient(atlasStudyRuntimeResponse(t, product, false), nil)
	runsDir := t.TempDir()
	endpointSHA, err := modelresearch.ProviderEndpointSHA256(client.config.Endpoint)
	if err != nil {
		t.Fatal(err)
	}
	current := atlasStudyStageCacheInput(
		runsDir, atlasStudyRuntimeRepository(), modelresearch.DefaultPolicy(),
		client.config, endpointSHA, product, client.request,
	)
	if current.Fingerprint.CacheContract != "atlas-study-accepted-v7" {
		t.Fatalf("current Atlas Study cache contract = %q", current.Fingerprint.CacheContract)
	}
	legacy := current
	legacy.Fingerprint.CacheContract = "atlas-study-accepted-v6"
	if _, err := modelresearch.SaveStageResponse(legacy, modelresearch.StageResponse{
		Content: []byte(`{"legacy":true}`),
	}); err != nil {
		t.Fatalf("save isolated legacy cache: %v", err)
	}
	if _, found, err := modelresearch.LoadStageResponse(current); err != nil || found {
		t.Fatalf("v7 lookup read v6 cache: found=%t err=%v", found, err)
	}

	if _, err := modelresearch.SaveStageResponse(current, modelresearch.StageResponse{
		Content: []byte(`{"current":true}`),
	}); err != nil {
		t.Fatalf("save current cache: %v", err)
	}
	driftedInput := atlasStudyRuntimeInput()
	driftedInput.ReadingTargets[0].Fact += " Changed exact catalog fact."
	driftedProduct := atlasStudyRuntimeProduct(t, driftedInput)
	drifted := atlasStudyStageCacheInput(
		runsDir, atlasStudyRuntimeRepository(), modelresearch.DefaultPolicy(),
		client.config, endpointSHA, driftedProduct, client.request,
	)
	if current.Fingerprint.EvidenceBundleHash == drifted.Fingerprint.EvidenceBundleHash {
		t.Fatal("different exact Study catalog retained the same cache identity")
	}
	if _, found, err := modelresearch.LoadStageResponse(drifted); err != nil || found {
		t.Fatalf("different catalog read current cache: found=%t err=%v", found, err)
	}
}

func TestRunAtlasStudyProductCanceledContextDoesNotApplyExactCache(t *testing.T) {
	product := atlasStudyRuntimeProduct(t, atlasStudyRuntimeInput())
	response := atlasStudyRuntimeResponse(t, product, false)
	first := newAtlasStudyRuntimeClient(response, nil)
	runsDir, firstRun := atlasStudyRuntimeDirectories(t, "cache-cancel-first")
	if _, err := runAtlasStudyProductForRun(
		t.Context(), firstRun, runsDir, atlasStudyRuntimeRepository(),
		modelresearch.DefaultPolicy(), false, true, product, nil,
		atlasStudyRuntimeFactory(first, first),
	); err != nil {
		t.Fatalf("populate exact cache: %v", err)
	}

	secondRun := filepath.Join(runsDir, "cache-cancel-second")
	if err := os.Mkdir(secondRun, 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	second := newAtlasStudyRuntimeClient(nil, errors.New("canceled cache hit must not call provider"))
	outcome, err := runAtlasStudyProductForRun(
		ctx, secondRun, runsDir, atlasStudyRuntimeRepository(),
		modelresearch.DefaultPolicy(), false, true, product, nil,
		atlasStudyRuntimeFactory(second, second),
	)
	if !errors.Is(err, context.Canceled) || outcome.State != atlasstudy.ProductStatePrepared ||
		outcome.Cached || second.calls != 0 {
		t.Fatalf("canceled cache outcome/client/error = %#v / %#v / %v", outcome, second, err)
	}
	atlasStudyRuntimeAssertNoFile(t, secondRun, atlasstudy.ResultArtifactFilename)
	atlasStudyRuntimeAssertNoFile(t, secondRun, atlasstudy.StatusArtifactFilename)
}

func TestRunAtlasStudyProductOfflineMakesZeroCallsAndPersistsZeroStageArtifacts(t *testing.T) {
	product := atlasStudyRuntimeProduct(t, atlasStudyRuntimeInput())
	client := newAtlasStudyRuntimeClient(atlasStudyRuntimeResponse(t, product, false), nil)
	runsDir, runDir := atlasStudyRuntimeDirectories(t, "offline")
	for _, name := range []string{
		atlasstudy.RequestArtifactFilename,
		atlasstudy.ResultArtifactFilename,
		atlasstudy.StatusArtifactFilename,
	} {
		if err := os.WriteFile(filepath.Join(runDir, name), []byte("stale"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	outcome, err := runAtlasStudyProductForRun(
		t.Context(), runDir, runsDir, atlasStudyRuntimeRepository(),
		modelresearch.DefaultPolicy(), false, false, product, nil,
		atlasStudyRuntimeFactory(client, client),
	)
	if err != nil || outcome.State != atlasstudy.ProductStateUnavailable ||
		!outcome.ProviderSkipped || client.calls != 0 {
		t.Fatalf("offline outcome/client/error = %#v / %d / %v", outcome, client.calls, err)
	}
	for _, name := range []string{
		atlasstudy.RequestArtifactFilename,
		atlasstudy.ResultArtifactFilename,
		atlasstudy.StatusArtifactFilename,
		debugdump.SemanticExchangesDir,
	} {
		atlasStudyRuntimeAssertNoFile(t, runDir, name)
	}
	atlasStudyRuntimeAssertCacheDirectoryAbsent(t, runsDir)
}

func TestAtlasStudyCandidateUnavailableIsProviderFreeAndClearsStaleArtifacts(t *testing.T) {
	runDir := t.TempDir()
	for _, name := range []string{
		atlasstudy.RequestArtifactFilename,
		atlasstudy.ResultArtifactFilename,
		atlasstudy.StatusArtifactFilename,
	} {
		if err := os.WriteFile(filepath.Join(runDir, name), []byte("stale"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	outcome, handled, err := atlasStudyCandidateUnavailableOutcome(
		&atlasstudy.CandidateUnavailableError{Reason: "no observed support roles"},
		runDir, nil,
	)
	if err != nil || !handled || outcome.State != atlasstudy.ProductStateUnavailable ||
		!outcome.ProviderSkipped || outcome.SemanticCalls != 0 || outcome.TransportAttempts != 0 {
		t.Fatalf("candidate unavailable outcome = %#v handled=%t err=%v", outcome, handled, err)
	}
	for _, name := range []string{
		atlasstudy.RequestArtifactFilename,
		atlasstudy.ResultArtifactFilename,
		atlasstudy.StatusArtifactFilename,
	} {
		atlasStudyRuntimeAssertNoFile(t, runDir, name)
	}

	notCandidate := errors.New("malformed catalog")
	if _, handled, err := atlasStudyCandidateUnavailableOutcome(notCandidate, runDir, nil); handled || err != nil {
		t.Fatalf("ordinary compile failure handled as unavailable: handled=%t err=%v", handled, err)
	}
}

func TestRunAtlasStudyProductInvalidResponseIsClosedAndNotCached(t *testing.T) {
	product := atlasStudyRuntimeProduct(t, atlasStudyRuntimeInput())
	response := atlasStudyRuntimeResponseWithOnlyInvalidRoute(t, product)
	client := newAtlasStudyRuntimeClient(response, nil)
	runsDir, runDir := atlasStudyRuntimeDirectories(t, "invalid")

	outcome, err := runAtlasStudyProductForRun(
		t.Context(), runDir, runsDir, atlasStudyRuntimeRepository(),
		modelresearch.DefaultPolicy(), false, true, product, nil,
		atlasStudyRuntimeFactory(client, client),
	)
	if err != nil {
		t.Fatalf("ordinary invalid response must be a closed product state: %v", err)
	}
	if outcome.State != atlasstudy.ProductStateFailed ||
		outcome.FailureCode != atlasstudy.FailureValidation || client.calls != 1 {
		t.Fatalf("invalid response outcome = %#v / calls=%d", outcome, client.calls)
	}
	atlasStudyRuntimeAssertNoFile(t, runDir, atlasstudy.ResultArtifactFilename)
	status := atlasStudyRuntimeReadStatus(t, runDir)
	if status.State != atlasstudy.ProductStateFailed ||
		status.FailureCode != atlasstudy.FailureValidation {
		t.Fatalf("invalid response status = %#v", status)
	}
	atlasStudyRuntimeAssertCacheAbsent(t, runsDir, product, client)
}

func TestValidateAtlasStudyResponseDistinguishesDecodeReferenceAndValidation(t *testing.T) {
	product := atlasStudyRuntimeProduct(t, atlasStudyRuntimeInput())

	_, _, failure, _, err := validateAtlasStudyResponse(product, []byte(`{"broken":`))
	if err == nil || failure != atlasstudy.FailureDecode {
		t.Fatalf("malformed JSON classification = %q / %v", failure, err)
	}

	var wrongRef map[string]any
	if err := json.Unmarshal(atlasStudyRuntimeResponse(t, product, false), &wrongRef); err != nil {
		t.Fatal(err)
	}
	wrongRef["brief"].(map[string]any)["what_it_is"].(map[string]any)["support_refs"] = []any{"unknown"}
	_, _, failure, _, err = validateAtlasStudyResponse(product, atlasStudyRuntimeJSON(t, wrongRef))
	if err == nil || failure != atlasstudy.FailureReference {
		t.Fatalf("wrong-ref classification = %q / %v", failure, err)
	}

	var semantic map[string]any
	if err := json.Unmarshal(atlasStudyRuntimeResponse(t, product, false), &semantic); err != nil {
		t.Fatal(err)
	}
	semantic["directions"].([]any)[0].(map[string]any)["learning_outcome"] = ""
	_, diagnostics, failure, _, err := validateAtlasStudyResponse(
		product, atlasStudyRuntimeJSON(t, semantic),
	)
	if err == nil || failure != atlasstudy.FailureValidation ||
		diagnostics.DirectionsReceived != 1 || diagnostics.Issues[0].Code != atlasstudy.IssueInvalidOutcome {
		t.Fatalf("semantic validation classification = %q / %#v / %v", failure, diagnostics, err)
	}
}

func TestRunAtlasStudyProductResourceLimitIsTypedTerminal(t *testing.T) {
	product := atlasStudyRuntimeProduct(t, atlasStudyRuntimeInput())
	resource := modelresearch.NewResourceLimitError(modelresearch.ResourceLimitError{
		Stage: "atlas_study", Kind: modelresearch.ResourceLimitOutputTokens,
		Limit: 64_000, Observed: 64_000, ObservedKnown: true,
		ConfiguredMaxTokens: 64_000, FinishReason: "length",
	}, nil)
	client := newAtlasStudyRuntimeClient(nil, resource)
	runsDir, runDir := atlasStudyRuntimeDirectories(t, "resource")

	outcome, err := runAtlasStudyProductForRun(
		t.Context(), runDir, runsDir, atlasStudyRuntimeRepository(),
		modelresearch.DefaultPolicy(), false, true, product, nil,
		atlasStudyRuntimeFactory(client, client),
	)
	var limitErr *modelresearch.ResourceLimitError
	if !errors.As(err, &limitErr) || limitErr.Kind != modelresearch.ResourceLimitOutputTokens ||
		outcome.State != atlasstudy.ProductStateFailed ||
		outcome.FailureCode != atlasstudy.FailureResource || client.calls != 1 {
		t.Fatalf("terminal resource outcome/error = %#v / %#v / %v", outcome, limitErr, err)
	}
	atlasStudyRuntimeAssertNoFile(t, runDir, atlasstudy.ResultArtifactFilename)
	if status := atlasStudyRuntimeReadStatus(t, runDir); status.FailureCode != atlasstudy.FailureResource {
		t.Fatalf("resource status = %#v", status)
	}
	atlasStudyRuntimeAssertCacheAbsent(t, runsDir, product, client)
	if exchange := atlasStudyRuntimeReadExchange(t, runDir); exchange.SemanticCalls != 1 || exchange.TransportAttempts != 1 {
		t.Fatalf("resource exchange = %#v", exchange)
	}
}

func TestAtlasStudyCatalogCardinalityLimitIsNotReportedAsRequestBytes(t *testing.T) {
	input := atlasStudyRuntimeInput()
	input.Architecture.Components = append(input.Architecture.Components, atlasstudy.Component{
		ID: "component-over-limit-runtime", SubsystemID: "subsystem-core-runtime", Name: "Overflow",
		Authority: repositoryatlas.AuthorityInferred,
	})
	input.Limits.MaxComponents = 1
	_, compileErr := atlasstudy.Compile(input)
	if compileErr == nil {
		t.Fatal("over-limit Atlas Study catalog compiled")
	}

	err := atlasStudyTerminalResource(compileErr, 64_000)
	var limitErr *modelresearch.ResourceLimitError
	if !errors.As(err, &limitErr) || limitErr.Kind != modelresearch.ResourceLimitCatalogItems ||
		limitErr.Limit != 1 || limitErr.Observed != 2 || !limitErr.ObservedKnown ||
		limitErr.ConfiguredMaxTokens != 64_000 {
		t.Fatalf("catalog cardinality error = %#v / %v", limitErr, err)
	}
}

func TestRunAtlasStudyProductCancellationIsTerminal(t *testing.T) {
	product := atlasStudyRuntimeProduct(t, atlasStudyRuntimeInput())
	client := newAtlasStudyRuntimeClient(nil, context.Canceled)
	runsDir, runDir := atlasStudyRuntimeDirectories(t, "canceled")

	outcome, err := runAtlasStudyProductForRun(
		t.Context(), runDir, runsDir, atlasStudyRuntimeRepository(),
		modelresearch.DefaultPolicy(), false, true, product, nil,
		atlasStudyRuntimeFactory(client, client),
	)
	if !errors.Is(err, context.Canceled) || outcome.State != atlasstudy.ProductStateFailed ||
		outcome.FailureCode != atlasstudy.FailureCanceled || client.calls != 1 {
		t.Fatalf("canceled outcome/error = %#v / %d / %v", outcome, client.calls, err)
	}
	atlasStudyRuntimeAssertNoFile(t, runDir, atlasstudy.ResultArtifactFilename)
	if status := atlasStudyRuntimeReadStatus(t, runDir); status.FailureCode != atlasstudy.FailureCanceled {
		t.Fatalf("canceled status = %#v", status)
	}
	atlasStudyRuntimeAssertCacheAbsent(t, runsDir, product, client)
	if exchange := atlasStudyRuntimeReadExchange(t, runDir); exchange.State != debugdump.SemanticStateCanceled ||
		exchange.SemanticCalls != 1 || exchange.TransportAttempts != 1 {
		t.Fatalf("canceled exchange = %#v", exchange)
	}
}

func TestRunAtlasStudyMandatorySecretBoundaryIgnoresNoSecrets(t *testing.T) {
	restore := secretscan.SetDisabled(true)
	defer restore()

	t.Run("unsafe request", func(t *testing.T) {
		const sentinel = "ghp_abcdefghijklmnopqrstuvwxyz"
		input := atlasStudyRuntimeInput()
		input.Documents[0].Claim = sentinel
		product := atlasStudyRuntimeProduct(t, input)
		client := newAtlasStudyRuntimeClient(atlasStudyRuntimeResponse(t, product, false), nil)
		runsDir, runDir := atlasStudyRuntimeDirectories(t, "unsafe-request")

		outcome, err := runAtlasStudyProductForRun(
			t.Context(), runDir, runsDir, atlasStudyRuntimeRepository(),
			modelresearch.DefaultPolicy(), false, true, product, nil,
			atlasStudyRuntimeFactory(client, client),
		)
		if err == nil || strings.Contains(err.Error(), sentinel) ||
			outcome.State != atlasstudy.ProductStateFailed || client.calls != 0 {
			t.Fatalf("unsafe request outcome/error = %#v / %d / %v", outcome, client.calls, err)
		}
		atlasStudyRuntimeAssertNoFile(t, runDir, atlasstudy.RequestArtifactFilename)
		atlasStudyRuntimeAssertNoFile(t, runDir, atlasstudy.ResultArtifactFilename)
		if status := atlasStudyRuntimeReadStatus(t, runDir); status.State != atlasstudy.ProductStateFailed || status.FailureCode != atlasstudy.FailureProvider {
			t.Fatalf("unsafe request status = %#v", status)
		}
		if exchange := atlasStudyRuntimeReadExchange(t, runDir); exchange.ValidationCode != debugdump.SemanticValidationSecret || exchange.Request.UnsafeKind == "" {
			t.Fatalf("unsafe request exchange = %#v", exchange)
		}
		atlasStudyRuntimeAssertTreeDoesNotContain(t, runDir, sentinel)
		atlasStudyRuntimeAssertCacheDirectoryAbsent(t, runsDir)
	})

	t.Run("unsafe provider envelope", func(t *testing.T) {
		const sentinel = "ghp_abcdefghijklmnopqrstuvwxyz"
		product := atlasStudyRuntimeProduct(t, atlasStudyRuntimeInput())
		client := newAtlasStudyRuntimeClient(atlasStudyRuntimeResponse(t, product, false), nil)
		client.request = []byte(`{"model":"fixture-model","credential":"` + sentinel + `"}`)
		runsDir, runDir := atlasStudyRuntimeDirectories(t, "unsafe-provider-request")

		outcome, err := runAtlasStudyProductForRun(
			t.Context(), runDir, runsDir, atlasStudyRuntimeRepository(),
			modelresearch.DefaultPolicy(), false, true, product, nil,
			atlasStudyRuntimeFactory(client, client),
		)
		if err == nil || strings.Contains(err.Error(), sentinel) ||
			outcome.State != atlasstudy.ProductStateFailed || client.calls != 0 {
			t.Fatalf("unsafe provider envelope outcome/error = %#v / %d / %v", outcome, client.calls, err)
		}
		atlasStudyRuntimeAssertNoFile(t, runDir, atlasstudy.RequestArtifactFilename)
		atlasStudyRuntimeAssertNoFile(t, runDir, atlasstudy.ResultArtifactFilename)
		atlasStudyRuntimeAssertTreeDoesNotContain(t, runDir, sentinel)
		atlasStudyRuntimeAssertCacheDirectoryAbsent(t, runsDir)
	})

	t.Run("unsafe response", func(t *testing.T) {
		const sentinel = "ghp_abcdefghijklmnopqrstuvwxyz"
		product := atlasStudyRuntimeProduct(t, atlasStudyRuntimeInput())
		response := atlasStudyRuntimeResponse(t, product, false)
		response = []byte(strings.Replace(
			string(response), "A server for identity workflows.",
			sentinel, 1,
		))
		client := newAtlasStudyRuntimeClient(response, nil)
		runsDir, runDir := atlasStudyRuntimeDirectories(t, "unsafe-response")

		outcome, err := runAtlasStudyProductForRun(
			t.Context(), runDir, runsDir, atlasStudyRuntimeRepository(),
			modelresearch.DefaultPolicy(), false, true, product, nil,
			atlasStudyRuntimeFactory(client, client),
		)
		if err == nil || strings.Contains(err.Error(), sentinel) ||
			outcome.State != atlasstudy.ProductStateFailed || client.calls != 1 {
			t.Fatalf("unsafe response outcome/error = %#v / %d / %v", outcome, client.calls, err)
		}
		atlasStudyRuntimeAssertNoFile(t, runDir, atlasstudy.ResultArtifactFilename)
		if status := atlasStudyRuntimeReadStatus(t, runDir); status.State != atlasstudy.ProductStateFailed || status.FailureCode != atlasstudy.FailureDecode {
			t.Fatalf("unsafe response status = %#v", status)
		}
		if exchange := atlasStudyRuntimeReadExchange(t, runDir); exchange.ValidationCode != debugdump.SemanticValidationSecret || exchange.Response.UnsafeKind == "" {
			t.Fatalf("unsafe response exchange = %#v", exchange)
		}
		atlasStudyRuntimeAssertTreeDoesNotContain(t, runDir, sentinel)
		atlasStudyRuntimeAssertCacheDirectoryAbsent(t, runsDir)
	})
}

type atlasStudyRuntimeClient struct {
	config  deepseek.EffectiveConfig
	request []byte
	result  modelresearch.ProviderResult
	err     error
	calls   int
}

func newAtlasStudyRuntimeClient(response []byte, err error) *atlasStudyRuntimeClient {
	return &atlasStudyRuntimeClient{
		config: deepseek.EffectiveConfig{
			Endpoint: "https://provider.example.test/v1/chat/completions",
			Model:    "fixture-model", AuthMode: "none", MaxTokens: 64_000,
		},
		request: []byte(`{"model":"fixture-model","max_tokens":64000,"messages":[{"role":"user","content":"atlas-study-fixture"}]}`),
		result: modelresearch.ProviderResult{
			Content: response, ResponseBytes: len(response), Attempts: 1,
			InputTokens: 123, OutputTokens: 456,
		},
		err: err,
	}
}

func (client *atlasStudyRuntimeClient) AtlasStudyPromptJSON(
	prompt atlasstudy.Prompt,
	maxRequestBytes int,
) ([]byte, error) {
	if prompt.Version != atlasstudy.PromptVersion || maxRequestBytes < len(client.request) {
		return nil, errors.New("invalid Atlas Study prompt fixture")
	}
	return append([]byte(nil), client.request...), nil
}

func (client *atlasStudyRuntimeClient) AtlasStudyMeasured(
	context.Context,
	atlasstudy.Prompt,
	int,
) (modelresearch.ProviderResult, error) {
	client.calls++
	return client.result, client.err
}

func (client *atlasStudyRuntimeClient) EffectiveConfig() deepseek.EffectiveConfig {
	return client.config
}

func atlasStudyRuntimeFactory(
	prompt atlasStudyClient,
	live atlasStudyClient,
) atlasStudyClientFactory {
	return func(requireCredentials bool) (atlasStudyClient, error) {
		if requireCredentials {
			return live, nil
		}
		return prompt, nil
	}
}

func atlasStudyRuntimeProduct(t *testing.T, input atlasstudy.Input) atlasstudy.Product {
	t.Helper()
	product, err := atlasstudy.Compile(input)
	if err != nil {
		t.Fatalf("Compile runtime fixture: %v", err)
	}
	return product
}

func atlasStudyRuntimeInput() atlasstudy.Input {
	atlas := repositoryatlas.Atlas{
		Version: repositoryatlas.Version,
		Units: []repositoryatlas.Unit{
			{ID: "unit-repository-runtime", Kind: repositoryatlas.UnitRepository, Name: "Runtime fixture"},
			{ID: "unit-module-runtime", Kind: repositoryatlas.UnitModule, ParentID: "unit-repository-runtime", Name: "Runtime module"},
			{ID: "unit-app-runtime", Kind: repositoryatlas.UnitApp, ParentID: "unit-module-runtime", Name: "Runtime app"},
		},
		Entities: []repositoryatlas.Entity{
			{ID: "surface-start-runtime", Kind: repositoryatlas.EntitySurface, UnitID: "unit-app-runtime"},
			{ID: "operation-start-runtime", Kind: repositoryatlas.EntityOperation, UnitID: "unit-app-runtime"},
		},
		Evidence: []repositoryatlas.Evidence{{
			ID: "evidence-start-runtime", UnitID: "unit-app-runtime",
			Location: evidence.Location{Path: "cmd/server/main.go", Line: 20}, Symbol: "RunServer",
			Provenance: evidence.Provenance{Provider: "fixture", Operation: "observe_start"},
		}},
		Relations: []repositoryatlas.Relation{{
			ID: "relation-start-runtime", UnitID: "unit-app-runtime",
			Kind: repositoryatlas.RelationExposes, Phase: repositoryatlas.PhaseStartup,
			Authority:    repositoryatlas.AuthorityResolved,
			Source:       repositoryatlas.EntityRef{Kind: repositoryatlas.EntitySurface, ID: "surface-start-runtime"},
			Target:       repositoryatlas.EntityRef{Kind: repositoryatlas.EntityOperation, ID: "operation-start-runtime"},
			EvidenceRefs: []string{"evidence-start-runtime"},
		}},
	}
	return atlasstudy.Input{
		Atlas: atlas, Language: atlasstudy.LanguageEnglish, Limits: atlasstudy.DefaultLimits(),
		Architecture: atlasstudy.ArchitectureInput{
			Version: 5, Source: "normalized_model", Title: "Runtime anatomy",
			Subsystems: []atlasstudy.Subsystem{{
				ID: "subsystem-core-runtime", Name: "Core",
				Authority:    repositoryatlas.AuthorityInferred,
				ComponentIDs: []string{"component-api-runtime"},
			}},
			Components: []atlasstudy.Component{{
				ID: "component-api-runtime", SubsystemID: "subsystem-core-runtime", Name: "API",
				Authority:        repositoryatlas.AuthorityInferred,
				ReadingTargetIDs: []string{"anchor-config-runtime", "anchor-route-runtime", "anchor-start-runtime"},
			}},
		},
		Surfaces: []atlasstudy.Surface{{
			ID: "surface-start-runtime", UnitID: "unit-app-runtime", Name: "Server entry",
			Kind: "process_entry", Authority: repositoryatlas.AuthorityResolved,
		}},
		ReadingTargets: []atlasstudy.ReadingTarget{
			{ID: "anchor-start-runtime", Owner: atlasstudy.CanonicalRef{Kind: atlasstudy.RefComponent, ID: "component-api-runtime"}, RelatedComponentIDs: []string{"component-api-runtime"}, PrincipalRefs: []atlasstudy.CanonicalRef{{Kind: atlasstudy.RefComponent, ID: "component-api-runtime"}}, Kind: atlasstudy.ReadingTargetEntrypoint, Label: "Server startup", Fact: "Initializes the application shell.", Authority: repositoryatlas.AuthorityObserved, Location: evidence.Location{Path: "cmd/server/main.go", Line: 20}, Symbol: "RunServer"},
			{ID: "anchor-config-runtime", Owner: atlasstudy.CanonicalRef{Kind: atlasstudy.RefComponent, ID: "component-api-runtime"}, RelatedComponentIDs: []string{"component-api-runtime"}, PrincipalRefs: []atlasstudy.CanonicalRef{{Kind: atlasstudy.RefComponent, ID: "component-api-runtime"}}, Kind: atlasstudy.ReadingTargetFunction, Label: "Configuration", Fact: "Loads settings.", Authority: repositoryatlas.AuthorityObserved, Location: evidence.Location{Path: "internal/config/load.go", Line: 14}, Symbol: "Load"},
			{ID: "anchor-route-runtime", Owner: atlasstudy.CanonicalRef{Kind: atlasstudy.RefComponent, ID: "component-api-runtime"}, RelatedComponentIDs: []string{"component-api-runtime"}, PrincipalRefs: []atlasstudy.CanonicalRef{{Kind: atlasstudy.RefComponent, ID: "component-api-runtime"}}, Kind: atlasstudy.ReadingTargetFunction, Label: "Routes", Fact: "Registers handlers.", Authority: repositoryatlas.AuthorityObserved, Location: evidence.Location{Path: "internal/server/routes.go", Line: 31}, Symbol: "RegisterRoutes"},
		},
		ReadingSupports: []atlasstudy.ReadingSupport{{
			ID: "support-start-runtime", TargetID: "anchor-start-runtime",
			PackageBucket: "package-server-runtime", Role: atlasstudy.SupportProcessEntry,
			Authority: repositoryatlas.AuthorityObserved,
		}},
		RouteSpans: []atlasstudy.RouteSpan{{
			ID: "span-start-runtime", Kind: atlasstudy.RouteSpanFocused,
			QuestionEnglish: "Where does this application start?",
			QuestionRussian: "Где запускается это приложение?",
			TargetJob:       atlasstudy.JobFirstContact, LearningStage: atlasstudy.StageOrientation,
			RequiredSupportIDs: []string{"support-start-runtime"},
			AllowedTargetIDs:   []string{"anchor-start-runtime"},
		}},
		Evidence: []atlasstudy.EvidenceFact{{
			ID:          "evidence-start-runtime",
			SubjectRefs: []atlasstudy.CanonicalRef{{Kind: atlasstudy.RefSurface, ID: "surface-start-runtime"}},
			Authority:   repositoryatlas.AuthorityResolved, Fact: "The application exposes a startup surface.",
		}},
		Documents: []atlasstudy.DocumentClaim{{
			ID: "document-purpose-runtime", Label: "Documented purpose",
			Claim:     "The project provides a server for identity workflows.",
			Authority: repositoryatlas.AuthorityObserved,
		}},
	}
}

func atlasStudyRuntimePartialInput() atlasstudy.Input {
	input := atlasStudyRuntimeInput()
	input.ReadingSupports = append(input.ReadingSupports, atlasstudy.ReadingSupport{
		ID: "support-config-runtime", TargetID: "anchor-config-runtime",
		PackageBucket: "package-config-runtime", Role: atlasstudy.SupportObservedCallBoundary,
		Authority: repositoryatlas.AuthorityObserved,
	})
	input.RouteSpans = append(input.RouteSpans, atlasstudy.RouteSpan{
		ID: "span-config-runtime", Kind: atlasstudy.RouteSpanFocused,
		QuestionEnglish: "Where is configuration loaded?",
		QuestionRussian: "Где загружается конфигурация?",
		TargetJob:       atlasstudy.JobMaintain, LearningStage: atlasstudy.StageOperations,
		RequiredSupportIDs: []string{"support-config-runtime"},
		AllowedTargetIDs:   []string{"anchor-config-runtime"},
	})
	return input
}

func atlasStudyRuntimeResponse(
	t *testing.T,
	product atlasstudy.Product,
	badSibling bool,
) []byte {
	t.Helper()
	component := atlasStudyRuntimeRef(t, product, atlasstudy.RefComponent, "component-api-runtime")
	surface := atlasStudyRuntimeRef(t, product, atlasstudy.RefSurface, "surface-start-runtime")
	evidenceRef := atlasStudyRuntimeRef(t, product, atlasstudy.RefEvidence, "evidence-start-runtime")
	document := atlasStudyRuntimeRef(t, product, atlasstudy.RefDocument, "document-purpose-runtime")
	targets := []string{
		atlasStudyRuntimeRef(t, product, atlasstudy.RefReadingTarget, "anchor-start-runtime"),
	}
	span := atlasStudyRuntimeRef(t, product, atlasstudy.RefRouteSpan, "span-start-runtime")
	valid := atlasStudyRuntimeDirection(component, span, targets)
	directions := []any{valid}
	if badSibling {
		bad := atlasStudyRuntimeDirection(component, span, targets)
		bad["reading"].([]any)[0].(map[string]any)["target_ref"] = "rt999"
		directions = append(directions, bad)
	}
	return atlasStudyRuntimeJSON(t, map[string]any{
		"repository_type": string(atlasstudy.RepositoryService),
		"brief": map[string]any{
			"what_it_is":             map[string]any{"text": "A server for identity workflows.", "support_refs": []string{document}},
			"problem":                map[string]any{"text": "It centralizes identity request handling.", "support_refs": []string{component}},
			"main_input":             map[string]any{"text": "Configured requests enter an application surface.", "support_refs": []string{surface}},
			"central_responsibility": map[string]any{"text": "The API component owns request setup.", "support_refs": []string{component}},
			"observable_result":      map[string]any{"text": "A startup surface becomes available.", "support_refs": []string{evidenceRef}},
		},
		"directions": directions,
	})
}

func atlasStudyRuntimeResponseWithOnlyInvalidRoute(
	t *testing.T,
	product atlasstudy.Product,
) []byte {
	t.Helper()
	var response map[string]any
	if err := json.Unmarshal(atlasStudyRuntimeResponse(t, product, false), &response); err != nil {
		t.Fatal(err)
	}
	direction := response["directions"].([]any)[0].(map[string]any)
	direction["reading"].([]any)[0].(map[string]any)["target_ref"] = "rt999"
	return atlasStudyRuntimeJSON(t, response)
}

func atlasStudyRuntimeDirection(component string, span string, targets []string) map[string]any {
	return map[string]any{
		"span_ref":         span,
		"why_it_matters":   "This route connects the accepted component to exact reading targets.",
		"learning_outcome": "The reader can identify the exact application entry.",
		"target_job":       string(atlasstudy.JobFirstContact),
		"learning_stage":   string(atlasstudy.StageOrientation),
		"principal_refs":   []string{component},
		"reading": []any{
			map[string]any{"target_ref": targets[0], "label": string(atlasstudy.ReadingStart), "what_to_look_for": "Confirm startup."},
		},
	}
}

func atlasStudyRuntimeRef(
	t *testing.T,
	product atlasstudy.Product,
	kind atlasstudy.RefKind,
	id string,
) string {
	t.Helper()
	for _, object := range product.Catalog() {
		if object.Kind == kind && object.CanonicalID == id {
			return object.Ref
		}
	}
	t.Fatalf("missing runtime catalog ref %s/%s", kind, id)
	return ""
}

func atlasStudyRuntimeJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func atlasStudyRuntimeDirectories(t *testing.T, name string) (string, string) {
	t.Helper()
	runsDir := t.TempDir()
	runDir := filepath.Join(runsDir, name)
	if err := os.Mkdir(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	return runsDir, runDir
}

func atlasStudyRuntimeRepository() modelresearch.RepositoryContext {
	return modelresearch.RepositoryContext{
		Identity: "fixture-repository", Revision: "fixture-revision", Scenario: "go-default",
	}
}

func atlasStudyRuntimeReadResult(t *testing.T, runDir string) atlasstudy.ResultRecord {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(runDir, atlasstudy.ResultArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	record, err := atlasstudy.DecodeResultRecord(data)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func atlasStudyRuntimeReadStatus(t *testing.T, runDir string) atlasstudy.Status {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(runDir, atlasstudy.StatusArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	status, err := atlasstudy.DecodeStatus(data)
	if err != nil {
		t.Fatal(err)
	}
	return status
}

func atlasStudyRuntimeReadExchange(t *testing.T, runDir string) debugdump.SemanticExchangeRecord {
	t.Helper()
	directory := filepath.Join(runDir, debugdump.SemanticExchangesDir)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("semantic exchange entries = %d, want 1", len(entries))
	}
	data, err := os.ReadFile(filepath.Join(directory, entries[0].Name(), debugdump.SemanticExchangeMetaFile))
	if err != nil {
		t.Fatal(err)
	}
	var record debugdump.SemanticExchangeRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	return record
}

func atlasStudyRuntimeAssertNoFile(t *testing.T, runDir, name string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(runDir, name)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s exists or has unexpected stat error: %v", name, err)
	}
}

func atlasStudyRuntimeAssertCacheAbsent(
	t *testing.T,
	runsDir string,
	product atlasstudy.Product,
	client *atlasStudyRuntimeClient,
) {
	t.Helper()
	endpointSHA, err := modelresearch.ProviderEndpointSHA256(client.config.Endpoint)
	if err != nil {
		t.Fatal(err)
	}
	input := atlasStudyStageCacheInput(
		runsDir, atlasStudyRuntimeRepository(), modelresearch.DefaultPolicy(),
		client.config, endpointSHA, product, client.request,
	)
	if _, found, err := modelresearch.LoadStageResponse(input); err != nil || found {
		t.Fatalf("cache lookup found/error = %v / %v", found, err)
	}
}

func atlasStudyRuntimeAssertCacheDirectoryAbsent(t *testing.T, runsDir string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(runsDir, ".model-research")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cache directory exists or has unexpected stat error: %v", err)
	}
}

func atlasStudyRuntimeAssertTreeDoesNotContain(t *testing.T, root, sentinel string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), sentinel) {
			t.Fatalf("unsafe sentinel persisted in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
